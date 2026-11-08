//go:build !(js && wasm)

// Bounds on what a module may make the interpreter allocate.

package shader

import (
	"fmt"
	"math"

	"github.com/gogpu/gputypes"
)

// The interpreter materializes every constant composite and every Private,
// Function and Workgroup variable as values: one 48-byte Value per scalar or
// vector, so up to 12 times the type's byte size. A raw SPIR-V module states
// array lengths with any constant: array<f32, 0xFFFFFFFF> as an OpConstantNull
// asked for 206 GB inside ParseModule and ended the process with a fatal out
// of memory. ParseModule rejects a module that exceeds Limits instead.
//
// WebGPU limits the buffers a shader addresses and the workgroup storage; it
// sets no limit for the Private and Function address spaces or for constants,
// so the software backend names its own, above the WGSL spec's guarantees
// (WGSL "Limits": 8192 bytes of Private variables statically accessed by a
// shader, 8192 bytes of Function variables declared in a function). They
// bound the memory of a 256-invocation workgroup, whose invocations are all
// live between barriers, to a few hundred MB at worst.
const (
	// MaxPrivateBytes is the combined byte size of a module's Private
	// variables: 8 times the WGSL guarantee.
	MaxPrivateBytes = 64 << 10
	// MaxFunctionBytes is the combined byte size of the Function variables
	// declared in one function: 8 times the WGSL guarantee.
	MaxFunctionBytes = 64 << 10
	// MaxConstantBytes is the combined byte size of a module's composite and
	// null constants, materialized once per module.
	MaxConstantBytes = 1 << 20
	// MaxWorkgroupBytes is the combined byte size of all a module's
	// Workgroup variables, used by an entry point or not: the interpreter
	// allocates them all for every dispatch. What one entry point uses is
	// bounded by the device's maxComputeWorkgroupStorageSize (WebGPU counts
	// only the variables the entry point statically uses), checked at
	// pipeline creation; this is 64 times its default of 16384.
	MaxWorkgroupBytes = 1 << 20
)

// sizeOverflow marks a byte size beyond any limit (saturated arithmetic).
const sizeOverflow = math.MaxUint64

// Limits bound the byte sizes ParseModuleLimits accepts. A type's byte size
// is its natural size (scalars of their width, arrays of their length, no
// padding), a lower bound of any explicit layout.
type Limits struct {
	// MaxTypeBytes bounds every type: the device's maxBufferSize, the
	// largest thing WebGPU lets a shader address.
	MaxTypeBytes uint64
	// MaxWorkgroupBytes, MaxPrivateBytes, MaxFunctionBytes and
	// MaxConstantBytes are the software backend's limits of the same names.
	MaxWorkgroupBytes uint64
	MaxPrivateBytes   uint64
	MaxFunctionBytes  uint64
	MaxConstantBytes  uint64
}

// LimitsFor returns the Limits of a device with limits l.
func LimitsFor(l gputypes.Limits) Limits {
	return Limits{
		MaxTypeBytes:      l.MaxBufferSize,
		MaxWorkgroupBytes: MaxWorkgroupBytes,
		MaxPrivateBytes:   MaxPrivateBytes,
		MaxFunctionBytes:  MaxFunctionBytes,
		MaxConstantBytes:  MaxConstantBytes,
	}
}

// parseBudget is what a module has used of its Limits while it is parsed.
type parseBudget struct {
	limits    Limits
	sizes     map[uint32]uint64 // byte size by type ID
	constants uint64
	private   uint64
	workgroup uint64
	function  uint64 // of the function being parsed
}

func addSize(a, b uint64) uint64 {
	if a > sizeOverflow-b {
		return sizeOverflow
	}
	return a + b
}

func mulSize(a, b uint64) uint64 {
	if a != 0 && b > sizeOverflow/a {
		return sizeOverflow
	}
	return a * b
}

// sizeType records the byte size of type id, just declared, and fails when
// it exceeds MaxTypeBytes. The types it is made of are declared before it
// (requireType), so their sizes are known.
func (p *parseBudget) sizeType(m *Module, id uint32) error {
	ti := m.Types[id]
	var size uint64
	switch ti.Kind {
	case TypeBool:
		size = 4
	case TypeInt, TypeFloat:
		size = uint64(max(ti.Width/8, 1))
	case TypeVector, TypeMatrix:
		size = mulSize(uint64(ti.Components), p.sizes[ti.ElemType])
	case TypeArray:
		// A runtime array (Length 0) lives only in a buffer.
		size = mulSize(uint64(ti.Length), p.sizes[ti.ElemType])
	case TypeStruct:
		for _, member := range ti.MemberIDs {
			size = addSize(size, p.sizes[member])
		}
	}
	if size > p.limits.MaxTypeBytes {
		return fmt.Errorf("spirv: type %d is %s, above the device's maxBufferSize of %d bytes",
			id, sizeText(size), p.limits.MaxTypeBytes)
	}
	p.sizes[id] = size
	return nil
}

// chargeConstant adds a composite or null constant of type typeID to the
// module's constants, before it is materialized.
func (p *parseBudget) chargeConstant(typeID uint32) error {
	p.constants = addSize(p.constants, p.sizes[typeID])
	if p.constants > p.limits.MaxConstantBytes {
		return fmt.Errorf("spirv: the module's composite and null constants exceed the software backend's limit of %d bytes",
			p.limits.MaxConstantBytes)
	}
	return nil
}

// chargeVariable adds a variable of pointer type ptrTypeID in storage class
// storage to its address space's total.
func (p *parseBudget) chargeVariable(m *Module, ptrTypeID, storage uint32) error {
	ptr := m.Types[ptrTypeID]
	if ptr == nil || ptr.Kind != TypePointer {
		return fmt.Errorf("spirv: variable type %d is not a pointer type", ptrTypeID)
	}
	size := p.sizes[ptr.ElemType]
	var total *uint64
	var limit uint64
	var what string
	switch storage {
	case StorageClassPrivate:
		total, limit, what = &p.private, p.limits.MaxPrivateBytes, "Private variables exceed the software backend's limit"
	case StorageClassFunction:
		total, limit, what = &p.function, p.limits.MaxFunctionBytes, "Function variables of a function exceed the software backend's limit"
	case StorageClassWorkgroup:
		total, limit, what = &p.workgroup, p.limits.MaxWorkgroupBytes, "Workgroup variables exceed the software backend's limit"
	default:
		return nil // buffers, images and I/O are not materialized from the type
	}
	*total = addSize(*total, size)
	if *total > limit {
		return fmt.Errorf("spirv: %s of %d bytes", what, limit)
	}
	return nil
}

// isTypeDeclaration reports whether opcode declares a type that ParseModule
// keeps in Module.Types.
func isTypeDeclaration(opcode uint16) bool {
	switch opcode {
	case OpTypeVoid, OpTypeBool, OpTypeInt, OpTypeFloat, OpTypeVector, OpTypeMatrix,
		OpTypeImage, OpTypeSampler, OpTypeSampledImage, OpTypeArray, OpTypeRuntimeArray,
		OpTypeStruct, OpTypePointer, OpTypeFunction:
		return true
	}
	return false
}

// sizeText is a byte size for an error message.
func sizeText(size uint64) string {
	if size == sizeOverflow {
		return "more than 2^64 bytes"
	}
	return fmt.Sprintf("%d bytes", size)
}
