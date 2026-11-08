//go:build !(js && wasm)

// SPIR-V module parser for the software backend.
//
// Parses a SPIR-V binary into types, constants, functions, entry points,
// and decorations. Only handles the subset needed for the triangle shader:
// scalar/vector/array types, constant composites, simple functions with
// OpLoad/OpStore/OpAccessChain/OpCompositeConstruct.

package shader

import (
	"fmt"
	"math"
	"slices"
	"sync"
)

// Module is a parsed SPIR-V module ready for interpretation.
type Module struct {
	// Types maps result ID to type information.
	Types map[uint32]*TypeInfo

	// Constants maps result ID to constant value.
	Constants map[uint32]Value

	// Functions maps entry point name to function body.
	Functions map[string]*Function

	// EntryPoints maps entry point name to its metadata.
	EntryPoints map[string]*EntryPoint

	// Decorations maps (target ID, decoration kind) to decoration value.
	Decorations map[decorationKey]uint32

	// MemberDecorations maps (struct type ID, member index, decoration kind) to value.
	// Used for OpMemberDecorate, primarily for struct member Offset decorations.
	MemberDecorations map[memberDecorationKey]uint32

	// Variables maps result ID to variable metadata.
	Variables map[uint32]*VariableInfo

	// FunctionsByID maps function result ID to function body.
	// Used by OpFunctionCall to dispatch to non-entry-point functions.
	FunctionsByID map[uint32]*Function

	// ExtInstImports maps result ID to the imported instruction set name.
	// The primary use is "GLSL.std.450" for math intrinsics.
	ExtInstImports map[uint32]string

	// ExecutionModes maps (function ID, execution mode) to operands.
	// Used for LocalSize in compute shaders.
	ExecutionModes map[executionModeKey][]uint32

	// ResultTypes maps the result ID of a constant or a typed function
	// instruction to its result type ID. Runtime values do not carry their
	// SPIR-V type (an integer vector component is held as a float32), so an
	// instruction whose meaning depends on its operand's type, such as
	// OpBitcast, looks the type up here.
	ResultTypes map[uint32]uint32

	// Bound is the upper bound of all IDs in the module.
	Bound uint32

	// valueSlots is the length of an interpreter's value slice: the largest
	// ID the module defines plus one (validateIDs), at most Bound. A header
	// Bound far above the IDs in use is legal; sizing by it would allocate
	// for IDs nothing can name.
	valueSlots uint32

	// interpPool recycles interpreter structs across Execute calls to avoid
	// allocating the values slice (sized to Bound) on every invocation.
	interpPool sync.Pool

	// prepState is the per-invocation setup built once (see prepared).
	prepState modulePrepState
}

// memberDecorationKey uniquely identifies a member decoration.
type memberDecorationKey struct {
	StructTypeID uint32
	MemberIndex  uint32
	Decoration   uint32
}

// executionModeKey uniquely identifies an execution mode on a function.
type executionModeKey struct {
	FunctionID    uint32
	ExecutionMode uint32
}

// BindingKey identifies a resource binding by descriptor set and binding number.
type BindingKey struct {
	Group   uint32
	Binding uint32
}

// GetBinding returns the (group, binding) for a variable, or ok=false if not decorated.
func (m *Module) GetBinding(varID uint32) (BindingKey, bool) {
	bindKey := decorationKey{TargetID: varID, Decoration: DecorationBinding}
	binding, hasBinding := m.Decorations[bindKey]
	setKey := decorationKey{TargetID: varID, Decoration: DecorationDescriptorSet}
	group, hasSet := m.Decorations[setKey]
	if !hasBinding {
		return BindingKey{}, false
	}
	if !hasSet {
		group = 0 // Default descriptor set is 0.
	}
	return BindingKey{Group: group, Binding: binding}, true
}

// GetMemberOffset returns the byte offset of a struct member, or 0 if not decorated.
func (m *Module) GetMemberOffset(structTypeID, memberIndex uint32) uint32 {
	key := memberDecorationKey{
		StructTypeID: structTypeID,
		MemberIndex:  memberIndex,
		Decoration:   DecorationOffset,
	}
	if v, ok := m.MemberDecorations[key]; ok {
		return v
	}
	return 0
}

// memberMatrixStride returns the MatrixStride decoration of a struct member
// (the column stride of the matrices it holds, directly or in an array), or
// 0 if the member is not decorated.
func (m *Module) memberMatrixStride(structTypeID, memberIndex uint32) uint32 {
	return m.MemberDecorations[memberDecorationKey{
		StructTypeID: structTypeID,
		MemberIndex:  memberIndex,
		Decoration:   DecorationMatrixStride,
	}]
}

// TypeInfo describes a SPIR-V type.
type TypeInfo struct {
	Kind       TypeKind
	Width      uint32   // bit width for int/float
	Signed     bool     // for OpTypeInt
	Components uint32   // for OpTypeVector
	ElemType   uint32   // element type ID for vector/array/pointer
	Length     uint32   // for OpTypeArray (number of elements)
	Storage    uint32   // for OpTypePointer (storage class)
	ParamIDs   []uint32 // for OpTypeFunction (parameter type IDs)
	ReturnType uint32   // for OpTypeFunction
	MemberIDs  []uint32 // for OpTypeStruct

	// ArrayStride is the ArrayStride decoration of an array type: the byte
	// distance between its elements in an explicitly laid-out buffer. 0 when
	// the array is not decorated (Function, Private and Workgroup arrays).
	ArrayStride uint32
}

// TypeKind classifies SPIR-V types.
type TypeKind int

const (
	TypeVoid TypeKind = iota
	TypeBool
	TypeInt
	TypeFloat
	TypeVector
	TypeMatrix
	TypeArray
	TypePointer
	TypeFunction
	TypeStruct
	TypeImage
	TypeSampler
	TypeSampledImage
)

// EntryPoint describes a SPIR-V entry point.
type EntryPoint struct {
	ExecutionModel uint32
	FunctionID     uint32
	Name           string
	InterfaceIDs   []uint32 // IDs of interface variables
}

// Function is a parsed SPIR-V function body.
type Function struct {
	ID           uint32
	ReturnType   uint32
	FunctionType uint32
	Instructions []Instruction

	// Labels maps label ID to instruction index. Built once during ParseModule
	// to avoid rebuilding on every Execute call. OpBranch and OpBranchConditional
	// use this for control flow.
	Labels map[uint32]int
}

// VariableInfo describes an OpVariable.
type VariableInfo struct {
	TypeID       uint32 // pointer type ID
	StorageClass uint32
	Initializer  uint32 // constant ID of the initializer; 0 for none
}

// decorationKey uniquely identifies a decoration on a target ID.
type decorationKey struct {
	TargetID   uint32
	Decoration uint32
}

// ParseModule parses SPIR-V binary (as []uint32 words) into a Module.
//
// The SPIR-V format is: 5-word header (magic, version, generator, bound, reserved)
// followed by a stream of instructions. Each instruction's first word encodes
// (wordCount << 16 | opcode).
func ParseModule(words []uint32) (*Module, error) {
	if len(words) < 5 {
		return nil, fmt.Errorf("spirv: module too short (%d words)", len(words))
	}
	if words[0] != spirvMagic {
		return nil, fmt.Errorf("spirv: bad magic 0x%08x, expected 0x%08x", words[0], spirvMagic)
	}

	m := &Module{
		Types:             make(map[uint32]*TypeInfo),
		Constants:         make(map[uint32]Value),
		Functions:         make(map[string]*Function),
		EntryPoints:       make(map[string]*EntryPoint),
		Decorations:       make(map[decorationKey]uint32),
		MemberDecorations: make(map[memberDecorationKey]uint32),
		Variables:         make(map[uint32]*VariableInfo),
		FunctionsByID:     make(map[uint32]*Function),
		ExtInstImports:    make(map[uint32]string),
		ExecutionModes:    make(map[executionModeKey][]uint32),
		ResultTypes:       make(map[uint32]uint32),
		Bound:             words[3],
	}

	// Maps function ID to function body (built during parse, keyed to entry points later).
	funcsByID := make(map[uint32]*Function)

	pos := 5 // skip header
	var currentFunc *Function

	for pos < len(words) {
		word := words[pos]
		wordCount := int(word >> 16)
		opcode := uint16(word & 0xFFFF)

		if wordCount == 0 {
			return nil, fmt.Errorf("spirv: zero word count at position %d", pos)
		}
		if pos+wordCount > len(words) {
			return nil, fmt.Errorf("spirv: instruction at %d extends past end (need %d, have %d)",
				pos, wordCount, len(words)-pos)
		}

		operands := words[pos+1 : pos+wordCount]

		switch opcode {
		case OpTypeVoid:
			if len(operands) < 1 {
				break
			}
			m.Types[operands[0]] = &TypeInfo{Kind: TypeVoid}

		case OpTypeBool:
			if len(operands) < 1 {
				break
			}
			m.Types[operands[0]] = &TypeInfo{Kind: TypeBool}

		case OpTypeInt:
			if len(operands) < 3 {
				break
			}
			m.Types[operands[0]] = &TypeInfo{
				Kind:   TypeInt,
				Width:  operands[1],
				Signed: operands[2] != 0,
			}

		case OpCapability:
			// f16 and ray query are not executed (ADR-078 rule 6): reject
			// them here, before any of their values is computed as f32 or
			// skipped.
			if len(operands) >= 1 {
				if name, ok := rejectedCapabilities[operands[0]]; ok {
					return nil, unsupportedf("SPIR-V capability %d (%s)", operands[0], name)
				}
			}

		case opTypeRayQueryKHR, opTypeAccelerationStructureKHR:
			return nil, unsupportedf("SPIR-V type %s", opcodeName(opcode))

		case OpNop, OpLine, OpNoLine:
			// No semantics; dropped wherever they appear.

		case OpTypeFloat:
			if len(operands) < 2 {
				break
			}
			if operands[1] == 16 {
				return nil, unsupportedf("SPIR-V type OpTypeFloat 16 (f16)")
			}
			m.Types[operands[0]] = &TypeInfo{
				Kind:  TypeFloat,
				Width: operands[1],
			}

		case OpTypeVector:
			if len(operands) < 3 {
				break
			}
			if err := m.requireType(operands[0], "component type", operands[1]); err != nil {
				return nil, err
			}
			if operands[2] < 2 || operands[2] > 4 {
				return nil, fmt.Errorf("spirv: vector type %d has %d components, want 2 to 4", operands[0], operands[2])
			}
			m.Types[operands[0]] = &TypeInfo{
				Kind:       TypeVector,
				ElemType:   operands[1],
				Components: operands[2],
			}

		case OpTypeMatrix:
			// OpTypeMatrix: resultID columnTypeID columnCount
			// SPIR-V matrices are arrays of column vectors.
			// mat4x4<f32> = 4 columns of vec4<f32>.
			if len(operands) < 3 {
				break
			}
			if err := m.requireType(operands[0], "column type", operands[1]); err != nil {
				return nil, err
			}
			if operands[2] < 2 || operands[2] > 4 {
				return nil, fmt.Errorf("spirv: matrix type %d has %d columns, want 2 to 4", operands[0], operands[2])
			}
			m.Types[operands[0]] = &TypeInfo{
				Kind:       TypeMatrix,
				ElemType:   operands[1], // column vector type (e.g. vec4<f32>)
				Components: operands[2], // number of columns
			}

		case OpTypeArray:
			if len(operands) < 3 {
				break
			}
			if err := m.requireType(operands[0], "element type", operands[1]); err != nil {
				return nil, err
			}
			// operands[2] is the ID of the constant holding the length.
			length := uint32(0)
			if cval, ok := m.Constants[operands[2]]; ok {
				length = toUint32(cval)
			}
			m.Types[operands[0]] = &TypeInfo{
				Kind:     TypeArray,
				ElemType: operands[1],
				Length:   length,
			}

		case OpTypeRuntimeArray:
			// OpTypeRuntimeArray: result elementType
			// Runtime arrays are unsized (length determined by buffer size at runtime).
			// Represented as TypeArray with Length=0; the buffer access chain
			// steps through it by its ArrayStride.
			if len(operands) < 2 {
				break
			}
			if err := m.requireType(operands[0], "element type", operands[1]); err != nil {
				return nil, err
			}
			m.Types[operands[0]] = &TypeInfo{
				Kind:     TypeArray,
				ElemType: operands[1],
				Length:   0, // Unsized: length determined by bound buffer size.
			}

		case OpTypePointer:
			if len(operands) < 3 {
				break
			}
			// OpTypeForwardPointer (physical storage buffer pointers) is
			// not supported, so a pointee is always declared first.
			if err := m.requireType(operands[0], "pointee type", operands[2]); err != nil {
				return nil, err
			}
			m.Types[operands[0]] = &TypeInfo{
				Kind:     TypePointer,
				Storage:  operands[1],
				ElemType: operands[2],
			}

		case OpTypeFunction:
			if len(operands) < 2 {
				break
			}
			for _, id := range operands[1:] {
				if err := m.requireType(operands[0], "return or parameter type", id); err != nil {
					return nil, err
				}
			}
			ti := &TypeInfo{
				Kind:       TypeFunction,
				ReturnType: operands[1],
			}
			if len(operands) > 2 {
				ti.ParamIDs = make([]uint32, len(operands)-2)
				copy(ti.ParamIDs, operands[2:])
			}
			m.Types[operands[0]] = ti

		case OpTypeStruct:
			if len(operands) < 1 {
				break
			}
			for _, id := range operands[1:] {
				if err := m.requireType(operands[0], "member type", id); err != nil {
					return nil, err
				}
			}
			ti := &TypeInfo{Kind: TypeStruct}
			if len(operands) > 1 {
				ti.MemberIDs = make([]uint32, len(operands)-1)
				copy(ti.MemberIDs, operands[1:])
			}
			m.Types[operands[0]] = ti

		case OpTypeImage:
			// OpTypeImage: result sampledType dim depth arrayed ms sampled format [access]
			if len(operands) >= 2 {
				if err := m.requireType(operands[0], "sampled type", operands[1]); err != nil {
					return nil, err
				}
				m.Types[operands[0]] = &TypeInfo{Kind: TypeImage, ElemType: operands[1]}
			}

		case OpTypeSampler:
			// OpTypeSampler: result
			if len(operands) >= 1 {
				m.Types[operands[0]] = &TypeInfo{Kind: TypeSampler}
			}

		case OpTypeSampledImage:
			// OpTypeSampledImage: result imageType
			if len(operands) >= 2 {
				if err := m.requireType(operands[0], "image type", operands[1]); err != nil {
					return nil, err
				}
				m.Types[operands[0]] = &TypeInfo{Kind: TypeSampledImage, ElemType: operands[1]}
			}

		case OpConstant:
			// OpConstant: resultType resultID literal...
			if len(operands) < 2 {
				break
			}
			typeID := operands[0]
			resultID := operands[1]
			m.Constants[resultID] = resolveConstant(m.Types, typeID, operands[2:])
			m.ResultTypes[resultID] = typeID

		case OpConstantComposite:
			// OpConstantComposite: resultType resultID constituents...
			if len(operands) < 2 {
				break
			}
			resultID := operands[1]
			constituents := operands[2:]
			m.ResultTypes[resultID] = operands[0]
			if s := m.typeShape(operands[0]); s.lanes() >= 2 {
				// A vector constant is a vector of its type's kind, not an
				// array of scalars (ADR-078 rule 1).
				var p lanePool
				for _, cid := range constituents {
					p.add(m.Constants[cid], s.kind())
				}
				m.Constants[resultID] = wordsValue(s, [4]uint32(p.w[:4]))
				break
			}
			elems := make([]Value, len(constituents))
			for i, cid := range constituents {
				if v, ok := m.Constants[cid]; ok {
					elems[i] = v
				}
			}
			m.Constants[resultID] = ValArray(elems)

		case OpConstantTrue:
			if len(operands) >= 2 {
				m.Constants[operands[1]] = ValBool(true)
			}

		case OpConstantFalse:
			if len(operands) >= 2 {
				m.Constants[operands[1]] = ValBool(false)
			}

		case OpConstantNull:
			if len(operands) < 2 {
				break
			}
			typeID := operands[0]
			resultID := operands[1]
			m.Constants[resultID] = zeroValue(m.Types, typeID)
			m.ResultTypes[resultID] = typeID

		case OpEntryPoint:
			// OpEntryPoint: executionModel functionID "name" interfaceIDs...
			if len(operands) < 2 {
				break
			}
			execModel := operands[0]
			funcID := operands[1]
			name, nameWords := decodeString(operands[2:])
			interfaceStart := 2 + nameWords
			var interfaceIDs []uint32
			if int(interfaceStart) < len(operands) {
				interfaceIDs = make([]uint32, len(operands)-int(interfaceStart))
				copy(interfaceIDs, operands[interfaceStart:])
			}
			m.EntryPoints[name] = &EntryPoint{
				ExecutionModel: execModel,
				FunctionID:     funcID,
				Name:           name,
				InterfaceIDs:   interfaceIDs,
			}

		case OpDecorate:
			// OpDecorate: target decoration [extra words]
			if len(operands) < 2 {
				break
			}
			key := decorationKey{
				TargetID:   operands[0],
				Decoration: operands[1],
			}
			if len(operands) >= 3 {
				m.Decorations[key] = operands[2]
			} else {
				m.Decorations[key] = 0
			}

		case OpMemberDecorate:
			// OpMemberDecorate: structType member decoration [extra words]
			if len(operands) < 3 {
				break
			}
			key := memberDecorationKey{
				StructTypeID: operands[0],
				MemberIndex:  operands[1],
				Decoration:   operands[2],
			}
			if len(operands) >= 4 {
				m.MemberDecorations[key] = operands[3]
			} else {
				m.MemberDecorations[key] = 0
			}

		case OpExtInstImport:
			// OpExtInstImport: resultID "name"
			if len(operands) < 2 {
				break
			}
			resultID := operands[0]
			name, _ := decodeString(operands[1:])
			m.ExtInstImports[resultID] = name

		case OpExecutionMode:
			// OpExecutionMode: entryPoint mode [extra words]
			if len(operands) < 2 {
				break
			}
			key := executionModeKey{
				FunctionID:    operands[0],
				ExecutionMode: operands[1],
			}
			if len(operands) > 2 {
				extra := make([]uint32, len(operands)-2)
				copy(extra, operands[2:])
				m.ExecutionModes[key] = extra
			}

		case OpVariable:
			// OpVariable: resultType resultID storageClass [initializer]
			if len(operands) < 3 {
				break
			}
			typeID := operands[0]
			resultID := operands[1]
			storageClass := operands[2]
			m.Variables[resultID] = &VariableInfo{
				TypeID:       typeID,
				StorageClass: storageClass,
			}
			if len(operands) > 3 {
				m.Variables[resultID].Initializer = operands[3]
			}
			// If inside a function, record as an instruction too.
			if currentFunc != nil {
				inst := Instruction{
					Opcode:   opcode,
					TypeID:   typeID,
					ResultID: resultID,
					Operands: []uint32{storageClass},
				}
				if len(operands) > 3 {
					inst.Operands = append(inst.Operands, operands[3:]...)
				}
				currentFunc.Instructions = append(currentFunc.Instructions, inst)
			}

		case OpFunction:
			// OpFunction: resultType resultID functionControl functionType
			if len(operands) < 4 {
				break
			}
			currentFunc = &Function{
				ReturnType:   operands[0],
				ID:           operands[1],
				FunctionType: operands[3],
			}
			funcsByID[operands[1]] = currentFunc

		case OpFunctionEnd:
			currentFunc = nil

		case OpLabel:
			// OpLabel is a basic block start — record as instruction for the interpreter.
			if currentFunc != nil && len(operands) >= 1 {
				currentFunc.Instructions = append(currentFunc.Instructions, Instruction{
					Opcode:   opcode,
					ResultID: operands[0],
				})
			}

		default:
			// Every other instruction in a function body is recorded for
			// run(), which must have a case for it: an opcode outside
			// supportedOps fails the parse instead of being skipped.
			if currentFunc != nil {
				if _, ok := supportedOps[opcode]; !ok {
					return nil, unsupportedOpcode(opcode)
				}
				inst := decodeInstruction(opcode, operands)
				// Types precede function bodies in SPIR-V, so the result
				// type is known here.
				inst.resShape = m.typeShape(inst.TypeID)
				currentFunc.Instructions = append(currentFunc.Instructions, inst)
			}
		}

		pos += wordCount
	}

	// Map entry points to their function bodies.
	for name, ep := range m.EntryPoints {
		if fn, ok := funcsByID[ep.FunctionID]; ok {
			m.Functions[name] = fn
		}
	}

	// Store all functions by ID for OpFunctionCall dispatch.
	m.FunctionsByID = funcsByID

	// Attach each array type's ArrayStride decoration, so buffer addressing
	// needs no decoration lookup by type ID.
	for id, ti := range m.Types {
		if ti.Kind == TypeArray {
			ti.ArrayStride = m.Decorations[decorationKey{TargetID: id, Decoration: DecorationArrayStride}]
		}
	}
	m.layOutUndecoratedStructs()

	// Record result types first: validateIDs reads an OpSwitch selector's type.
	for _, fn := range funcsByID {
		for _, inst := range fn.Instructions {
			if inst.TypeID != 0 && inst.ResultID != 0 {
				m.ResultTypes[inst.ResultID] = inst.TypeID
			}
		}
	}
	if err := m.validateIDs(); err != nil {
		return nil, err
	}

	// Pre-build label indexes for all functions. This avoids rebuilding
	// the label map on every Execute call (saves one map alloc per call).
	for _, fn := range funcsByID {
		fn.Labels = make(map[uint32]int, len(fn.Instructions)/4)
		for i, inst := range fn.Instructions {
			if inst.Opcode == OpLabel {
				fn.Labels[inst.ResultID] = i
			}
		}
	}

	return m, nil
}

// GetBuiltIn returns the BuiltIn decoration value for a variable ID, or -1 if none.
func (m *Module) GetBuiltIn(varID uint32) int {
	key := decorationKey{TargetID: varID, Decoration: DecorationBuiltIn}
	if val, ok := m.Decorations[key]; ok {
		return int(val)
	}
	return -1
}

// MayDiscard reports whether any function of the module contains OpKill or
// OpTerminateInvocation, so a fragment entry point may discard. The answer is
// conservative: it does not check that the function is reachable from the
// entry point.
func (m *Module) MayDiscard() bool {
	for _, fn := range m.FunctionsByID {
		for i := range fn.Instructions {
			if op := fn.Instructions[i].Opcode; op == OpKill || op == OpTerminateInvocation {
				return true
			}
		}
	}
	for _, fn := range m.Functions {
		for i := range fn.Instructions {
			if op := fn.Instructions[i].Opcode; op == OpKill || op == OpTerminateInvocation {
				return true
			}
		}
	}
	return false
}

// layOutUndecoratedStructs gives the members of every struct type without
// Offset decorations an Offset, so the struct can live in a byte buffer.
// Such a struct is a Workgroup type: SPIR-V forbids explicit layout there,
// and naga emits a separate layout-free type for it. Its byte layout is the
// interpreter's own business (no host ever sees workgroup memory), so the
// members are packed in order at their natural alignment; what matters is
// that they do not overlap and that typeByteSize, readValueFromBuffer,
// writeValueToBufferTyped and bufferAccessChain agree, which they do by all
// reading the same Offset decorations. Arrays need nothing: arrayStride
// already packs an undecorated array at its element size.
func (m *Module) layOutUndecoratedStructs() {
	ids := make([]uint32, 0, len(m.Types))
	for id, ti := range m.Types {
		if ti.Kind == TypeStruct {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	done := make(map[uint32]bool, len(ids))
	for _, id := range ids {
		m.layOutStruct(id, done)
	}
}

// layOutStruct lays out struct id after the structs it contains (see
// layOutUndecoratedStructs). done marks the structs already handled.
func (m *Module) layOutStruct(id uint32, done map[uint32]bool) {
	if done[id] {
		return
	}
	done[id] = true
	ti := m.Types[id]
	for _, memberID := range ti.MemberIDs {
		if nested := innermostStruct(m, memberID); nested != 0 {
			m.layOutStruct(nested, done)
		}
	}
	for i := range ti.MemberIDs {
		if _, ok := m.MemberDecorations[memberDecorationKey{StructTypeID: id, MemberIndex: uint32(i), Decoration: DecorationOffset}]; ok {
			return // explicitly laid out
		}
	}
	var offset uint32
	for i, memberID := range ti.MemberIDs {
		member := m.Types[memberID]
		if member == nil {
			return
		}
		align := typeAlignment(m, member)
		offset = (offset + align - 1) / align * align
		m.MemberDecorations[memberDecorationKey{StructTypeID: id, MemberIndex: uint32(i), Decoration: DecorationOffset}] = offset
		offset += typeByteSize(m, member)
	}
}

// innermostStruct returns the ID of the struct type typeID is or holds
// through arrays, matrices and vectors, or 0 if there is none.
func innermostStruct(m *Module, typeID uint32) uint32 {
	for depth := 0; depth < 16; depth++ {
		ti := m.Types[typeID]
		if ti == nil {
			return 0
		}
		switch ti.Kind {
		case TypeStruct:
			return typeID
		case TypeArray, TypeMatrix, TypeVector:
			typeID = ti.ElemType
		default:
			return 0
		}
	}
	return 0
}

// typeAlignment is the natural alignment of ti for layOutStruct: the size
// of its scalars, at least one byte.
func typeAlignment(m *Module, ti *TypeInfo) uint32 {
	for depth := 0; depth < 16 && ti != nil; depth++ {
		switch ti.Kind {
		case TypeFloat, TypeInt:
			return max(ti.Width/8, 1)
		case TypeArray, TypeMatrix, TypeVector:
			ti = m.Types[ti.ElemType]
		case TypeStruct:
			var align uint32 = 1
			for _, memberID := range ti.MemberIDs {
				if member := m.Types[memberID]; member != nil {
					align = max(align, typeAlignment(m, member))
				}
			}
			return align
		default:
			return 4 // bool and the rest are 32-bit words
		}
	}
	return 4
}

// GetLocation returns the Location decoration value for a variable ID, or -1 if none.
func (m *Module) GetLocation(varID uint32) int {
	key := decorationKey{TargetID: varID, Decoration: DecorationLocation}
	if val, ok := m.Decorations[key]; ok {
		return int(val)
	}
	return -1
}

// GetTypeComponentCount returns the number of scalar components for the type
// pointed to by a variable. For vec2 returns 2, vec3 returns 3, vec4 returns 4,
// scalars return 1. Returns 0 if the variable or type is unknown.
func (m *Module) GetTypeComponentCount(varID uint32) int {
	vi, ok := m.Variables[varID]
	if !ok {
		return 0
	}
	pointee := m.PointeeType(vi.TypeID)
	if pointee == nil {
		return 0
	}
	switch pointee.Kind {
	case TypeVector:
		return int(pointee.Components)
	case TypeFloat, TypeInt, TypeBool:
		return 1
	default:
		return 0
	}
}

// PointeeType dereferences a pointer type, returning the type it points to.
func (m *Module) PointeeType(ptrTypeID uint32) *TypeInfo {
	ptrType, ok := m.Types[ptrTypeID]
	if !ok || ptrType.Kind != TypePointer {
		return nil
	}
	return m.Types[ptrType.ElemType]
}

// typeShape returns the shape of type typeID: a 32-bit float, integer or
// bool scalar, or a vector of 2 to 4 of them. Anything else (no type, f16,
// f64, a matrix, array, struct, pointer or image) has no shape.
func (m *Module) typeShape(typeID uint32) shape {
	if m == nil {
		return 0
	}
	return typesShape(m.Types, typeID)
}

// typesShape is typeShape over a type table.
func typesShape(types map[uint32]*TypeInfo, typeID uint32) shape {
	ti := types[typeID]
	if ti == nil {
		return 0
	}
	if ti.Kind == TypeVector {
		elem := types[ti.ElemType]
		if elem == nil {
			return 0
		}
		return shapeOf(scalarKind(elem), int(ti.Components))
	}
	return shapeOf(scalarKind(ti), 1)
}

// scalarKind returns the lane kind of a 32-bit scalar type, kindNone for any
// other type.
func scalarKind(ti *TypeInfo) laneKind {
	switch {
	case ti.Kind == TypeBool:
		return kindBool
	case ti.Kind == TypeFloat && ti.Width == 32:
		return kindFloat
	case ti.Kind == TypeInt && ti.Width == 32 && ti.Signed:
		return kindSint
	case ti.Kind == TypeInt && ti.Width == 32:
		return kindUint
	}
	return kindNone
}

// decodeInstruction builds an Instruction from an opcode and its operands.
// A typed opcode of supportedOps starts with its result type and result ID
// (type resultID operands...); every other instruction keeps all its
// operands. run() checks each opcode's operand count itself.
func decodeInstruction(opcode uint16, operands []uint32) Instruction {
	inst := Instruction{Opcode: opcode}
	if supportedOps[opcode].typed {
		if len(operands) < 2 {
			return inst
		}
		inst.TypeID = operands[0]
		inst.ResultID = operands[1]
		operands = operands[2:]
	}
	if len(operands) > 0 {
		inst.Operands = make([]uint32, len(operands))
		copy(inst.Operands, operands)
	}
	return inst
}

// decodeString reads a null-terminated UTF-8 string from SPIR-V words.
// Returns the string and the number of words consumed (including padding).
func decodeString(words []uint32) (string, uint32) {
	var bytes []byte
	for i, w := range words {
		b0 := byte(w)
		b1 := byte(w >> 8)
		b2 := byte(w >> 16)
		b3 := byte(w >> 24)
		if b0 == 0 {
			return string(bytes), uint32(i + 1)
		}
		bytes = append(bytes, b0)
		if b1 == 0 {
			return string(bytes), uint32(i + 1)
		}
		bytes = append(bytes, b1)
		if b2 == 0 {
			return string(bytes), uint32(i + 1)
		}
		bytes = append(bytes, b2)
		if b3 == 0 {
			return string(bytes), uint32(i + 1)
		}
		bytes = append(bytes, b3)
	}
	return string(bytes), uint32(len(words))
}

// resolveConstant converts SPIR-V constant literal words to a Go value
// based on the declared type.
func resolveConstant(types map[uint32]*TypeInfo, typeID uint32, literals []uint32) Value {
	ti, ok := types[typeID]
	if !ok || len(literals) == 0 {
		return ValUint(0)
	}
	switch ti.Kind {
	case TypeFloat:
		return ValFloat(math.Float32frombits(literals[0]))
	case TypeInt:
		if ti.Signed {
			return ValInt(int32(literals[0]))
		}
		return ValUint(literals[0])
	default:
		return ValUint(literals[0])
	}
}
