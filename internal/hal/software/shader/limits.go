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
	// MaxInvocationBytes bounds what one invocation may hold live: its
	// value slots (48 bytes each, one per ID up to the module's largest),
	// plus 12 times the byte size of every typed result, its Private and
	// Function variables and its Uniform and push-constant data, the
	// interpreter holding up to 12 times a type's size as values.
	MaxInvocationBytes = 256 << 20
	// MaxWorkgroupLiveBytes bounds a compute workgroup: MaxInvocationBytes
	// is per invocation, and every invocation of a workgroup is live
	// between barriers. Checked at compute pipeline creation against the
	// entry point's workgroup size.
	MaxWorkgroupLiveBytes = 1 << 30
)

// valueBytes is the size of one interpreter Value; valueAmplification is how
// many times a type's byte size its values may take (a 4-byte scalar in a
// 48-byte Value).
const (
	valueBytes         = 48
	valueAmplification = 12
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
	seen      map[uint32]bool   // result IDs defined so far
	constants uint64
	private   uint64
	function  uint64 // of the function being parsed
	// workgroupVars are the pointee types of the Workgroup variables.
	workgroupVars []uint32
	functions     uint64 // of every function: bounds any call chain (no recursion)
	resources     uint64 // Uniform and push-constant data, materialized as values
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
	// Every type takes at least one value: an image, sampler or pointer
	// handle, an empty struct, an array whose length is not a constant it
	// knows, even void or a function type as an element. Counting any of them
	// as 0 bytes let array<image, 2^32> or array<void, 2^32-1> through at no
	// cost while each element still allocated a value.
	size = max(size, 4)
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
		p.functions = addSize(p.functions, size)
		total, limit, what = &p.function, p.limits.MaxFunctionBytes, "Function variables of a function exceed the software backend's limit"
	case StorageClassWorkgroup:
		// Bounded after the parse by its laid-out size (checkWorkgroup).
		p.workgroupVars = append(p.workgroupVars, ptr.ElemType)
		return nil
	case StorageClassUniform, StorageClassPushConstant:
		// Read into values for every invocation; bounded per invocation.
		p.resources = addSize(p.resources, size)
		return nil
	default:
		return nil // storage buffers, images and I/O are not materialized from the type
	}
	*total = addSize(*total, size)
	if *total > limit {
		return fmt.Errorf("spirv: %s of %d bytes", what, limit)
	}
	return nil
}

// checkWorkgroup bounds the module's Workgroup memory as newWorkgroupMemory
// lays it out: by the ArrayStride and Offset decorations, which the parse
// only knows in full once every instruction is read (SPIR-V allows a
// decoration anywhere before its use only by convention; Rust naga emits
// ArrayStride on workgroup arrays), with the variables aligned to
// workgroupVarAlign. Computed in 64 bits: typeByteSize's 32-bit arithmetic
// wrapped on a stride of 2^32-1, and the memory asked for 4 GB per dispatch.
func (p *parseBudget) checkWorkgroup(m *Module) error {
	var total uint64
	for _, pointee := range p.workgroupVars {
		total = addSize(mulSize(addSize(total, workgroupVarAlign-1)/workgroupVarAlign, workgroupVarAlign), laidOutSize(m, pointee))
	}
	if total > p.limits.MaxWorkgroupBytes {
		return fmt.Errorf("spirv: Workgroup variables take %s, above the software backend's limit of %d bytes",
			sizeText(total), p.limits.MaxWorkgroupBytes)
	}
	return nil
}

// laidOutSize is typeByteSize in saturating 64-bit arithmetic.
func laidOutSize(m *Module, id uint32) uint64 {
	ti := m.Types[id]
	if ti == nil {
		return 0
	}
	switch ti.Kind {
	case TypeFloat, TypeInt:
		return uint64(ti.Width / 8)
	case TypeBool:
		return 4
	case TypeVector, TypeMatrix:
		if m.Types[ti.ElemType] == nil && ti.Kind == TypeVector {
			return mulSize(4, uint64(ti.Components))
		}
		return mulSize(laidOutSize(m, ti.ElemType), uint64(ti.Components))
	case TypeArray:
		stride := uint64(ti.ArrayStride)
		if stride == 0 {
			stride = laidOutSize(m, ti.ElemType)
		}
		return mulSize(stride, uint64(ti.Length))
	case TypeStruct:
		var end uint64
		for i, member := range ti.MemberIDs {
			if m.Types[member] == nil {
				continue
			}
			offset := uint64(m.GetMemberOffset(id, uint32(i)))
			if offset == 0 && i > 0 {
				offset = uint64(i) * 4 // typeByteSize's fallback
			}
			end = max(end, addSize(offset, laidOutSize(m, member)))
		}
		return end
	}
	return 4
}

// define fails when result ID id is defined a second time (SPIR-V 2.4: each
// result ID is defined once). A redefined type kept the old size in the
// composites sized from it while the interpreter used the new one.
func (p *parseBudget) define(id uint32) error {
	if p.seen[id] {
		return fmt.Errorf("spirv: result ID %d is defined more than once", id)
	}
	p.seen[id] = true
	return nil
}

// declaredResult returns the result ID an instruction of the module level
// or a function body defines, for the opcodes the parser keeps.
func declaredResult(opcode uint16, operands []uint32) (uint32, bool) {
	switch {
	case isTypeDeclaration(opcode), opcode == OpExtInstImport, opcode == OpLabel:
		if len(operands) >= 1 {
			return operands[0], true
		}
	case opcode == OpConstant, opcode == OpConstantComposite, opcode == OpConstantTrue,
		opcode == OpConstantFalse, opcode == OpConstantNull, opcode == OpFunction,
		supportedOps[opcode].typed:
		if len(operands) >= 2 {
			return operands[1], true
		}
	}
	return 0, false
}

// checkLive computes what one invocation of m may hold live and fails above
// MaxInvocationBytes. It also rejects recursion (SPIR-V 2.16.1: a shader's
// static call graph has no cycles), so that every function's variables
// bound any call chain.
func (p *parseBudget) checkLive(m *Module) error {
	if err := noRecursion(m); err != nil {
		return err
	}
	var results uint64
	for _, fn := range m.FunctionsByID {
		for i := range fn.Instructions {
			if inst := &fn.Instructions[i]; inst.TypeID != 0 {
				results = addSize(results, p.sizes[inst.TypeID])
			}
		}
	}
	held := addSize(addSize(results, p.private), addSize(p.functions, p.resources))
	live := addSize(mulSize(uint64(m.valueSlots), valueBytes), mulSize(held, valueAmplification))
	if live > MaxInvocationBytes {
		return fmt.Errorf("spirv: an invocation would hold %s, above the software backend's limit of %d bytes",
			sizeText(live), MaxInvocationBytes)
	}
	m.invocationBytes = live
	return nil
}

// noRecursion fails when a function of m calls itself, directly or not.
func noRecursion(m *Module) error {
	const (
		unvisited = iota
		active
		done
	)
	state := make(map[uint32]int, len(m.FunctionsByID))
	var visit func(id uint32) error
	visit = func(id uint32) error {
		switch state[id] {
		case active:
			return fmt.Errorf("spirv: function %d is recursive", id)
		case done:
			return nil
		}
		state[id] = active
		if fn := m.FunctionsByID[id]; fn != nil {
			for i := range fn.Instructions {
				if inst := &fn.Instructions[i]; inst.Opcode == OpFunctionCall && len(inst.Operands) > 0 {
					if err := visit(inst.Operands[0]); err != nil {
						return err
					}
				}
			}
		}
		state[id] = done
		return nil
	}
	for id := range m.FunctionsByID {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}

// WorkgroupLiveBytes is what a workgroup of invocations of m may hold live,
// to compare with MaxWorkgroupLiveBytes.
func (m *Module) WorkgroupLiveBytes(invocations uint64) uint64 {
	return mulSize(m.invocationBytes, invocations)
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
