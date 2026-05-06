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

	// Bound is the upper bound of all IDs in the module.
	Bound uint32

	// interpPool recycles interpreter structs across Execute calls to avoid
	// allocating the values slice (sized to Bound) on every invocation.
	interpPool sync.Pool
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
}

// TypeKind classifies SPIR-V types.
type TypeKind int

const (
	TypeVoid TypeKind = iota
	TypeBool
	TypeInt
	TypeFloat
	TypeVector
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
//
//nolint:maintidx // SPIR-V opcode dispatch switch is inherently large.
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

		case OpTypeFloat:
			if len(operands) < 2 {
				break
			}
			m.Types[operands[0]] = &TypeInfo{
				Kind:  TypeFloat,
				Width: operands[1],
			}

		case OpTypeVector:
			if len(operands) < 3 {
				break
			}
			m.Types[operands[0]] = &TypeInfo{
				Kind:       TypeVector,
				ElemType:   operands[1],
				Components: operands[2],
			}

		case OpTypeArray:
			if len(operands) < 3 {
				break
			}
			// operands[2] is the ID of the constant holding the length.
			length := uint32(0)
			if cval, ok := m.Constants[operands[2]]; ok {
				switch cv := cval.(type) {
				case Uint32:
					length = cv
				case Int32:
					length = uint32(cv)
				}
			}
			m.Types[operands[0]] = &TypeInfo{
				Kind:     TypeArray,
				ElemType: operands[1],
				Length:   length,
			}

		case OpTypeRuntimeArray:
			// OpTypeRuntimeArray: result elementType
			// Runtime arrays are unsized (length determined by buffer size at runtime).
			// Represented as TypeArray with Length=0, which the buffer access chain
			// handles by computing byte offsets from element type size.
			if len(operands) < 2 {
				break
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
			m.Types[operands[0]] = &TypeInfo{
				Kind:     TypePointer,
				Storage:  operands[1],
				ElemType: operands[2],
			}

		case OpTypeFunction:
			if len(operands) < 2 {
				break
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
			ti := &TypeInfo{Kind: TypeStruct}
			if len(operands) > 1 {
				ti.MemberIDs = make([]uint32, len(operands)-1)
				copy(ti.MemberIDs, operands[1:])
			}
			m.Types[operands[0]] = ti

		case OpTypeImage:
			// OpTypeImage: result sampledType dim depth arrayed ms sampled format [access]
			if len(operands) >= 2 {
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

		case OpConstantComposite:
			// OpConstantComposite: resultType resultID constituents...
			if len(operands) < 2 {
				break
			}
			resultID := operands[1]
			constituents := operands[2:]
			elems := make(Array, len(constituents))
			for i, cid := range constituents {
				if v, ok := m.Constants[cid]; ok {
					elems[i] = v
				}
			}
			m.Constants[resultID] = elems

		case OpConstantTrue:
			if len(operands) >= 2 {
				m.Constants[operands[1]] = true
			}

		case OpConstantFalse:
			if len(operands) >= 2 {
				m.Constants[operands[1]] = false
			}

		case OpConstantNull:
			if len(operands) < 2 {
				break
			}
			typeID := operands[0]
			resultID := operands[1]
			m.Constants[resultID] = zeroValue(m.Types, typeID)

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
			// All other instructions inside a function body are recorded for interpretation.
			if currentFunc != nil {
				inst := decodeInstruction(opcode, operands)
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

// GetLocation returns the Location decoration value for a variable ID, or -1 if none.
func (m *Module) GetLocation(varID uint32) int {
	key := decorationKey{TargetID: varID, Decoration: DecorationLocation}
	if val, ok := m.Decorations[key]; ok {
		return int(val)
	}
	return -1
}

// PointeeType dereferences a pointer type, returning the type it points to.
func (m *Module) PointeeType(ptrTypeID uint32) *TypeInfo {
	ptrType, ok := m.Types[ptrTypeID]
	if !ok || ptrType.Kind != TypePointer {
		return nil
	}
	return m.Types[ptrType.ElemType]
}

// decodeInstruction builds an Instruction from an opcode and its operands.
// Instructions with result type and result ID follow the pattern:
// resultType resultID operands... for most typed instructions.
func decodeInstruction(opcode uint16, operands []uint32) Instruction {
	inst := Instruction{Opcode: opcode}

	switch opcode {
	// Instructions with result type AND result ID (type resultID operands...).
	case OpLoad, OpAccessChain, OpCompositeConstruct, OpCompositeExtract,
		OpConvertUToF, OpConvertFToU, OpConvertSToF, OpConvertFToS, OpBitcast,
		OpSConvert, OpUConvert, OpFConvert,
		OpFAdd, OpFSub, OpFMul, OpFDiv, OpFNegate, OpFMod, OpFRem,
		OpIAdd, OpISub, OpIMul, OpSDiv, OpUDiv, OpSMod, OpUMod, OpSRem, OpSNegate,
		OpSelect, OpPhi,
		OpIEqual, OpINotEqual,
		OpFOrdEqual, OpFOrdLessThan, OpFOrdGreaterThan,
		OpFOrdLessThanEqual, OpFOrdGreaterThanEqual,
		OpULessThan, OpUGreaterThan, OpULessThanEqual, OpUGreaterThanEqual,
		OpSLessThan, OpSGreaterThan, OpSLessThanEqual, OpSGreaterThanEqual,
		OpLogicalAnd, OpLogicalOr, OpLogicalNot,
		OpBitwiseAnd, OpBitwiseOr, OpBitwiseXor, OpNot,
		OpShiftLeftLogical, OpShiftRightLogical, OpShiftRightArithmetic,
		OpDot, OpVectorTimesScalar, OpMatrixTimesVector, OpMatrixTimesScalar,
		OpMatrixTimesMatrix, OpTranspose, OpVectorShuffle,
		OpCopyObject,
		OpSampledImage, OpImageSampleImplicitLod, OpImageSampleExplicitLod,
		OpImageFetch, OpImageQuerySize,
		OpAtomicLoad, OpAtomicExchange, OpAtomicCompareExchange,
		OpAtomicIIncrement, OpAtomicIDecrement,
		OpAtomicIAdd, OpAtomicISub,
		OpAtomicSMin, OpAtomicUMin, OpAtomicSMax, OpAtomicUMax,
		OpFunctionCall,
		OpUndef, OpExtInst:
		if len(operands) >= 2 {
			inst.TypeID = operands[0]
			inst.ResultID = operands[1]
			if len(operands) > 2 {
				inst.Operands = make([]uint32, len(operands)-2)
				copy(inst.Operands, operands[2:])
			}
		}

	// OpStore: pointer value [memory access]
	case OpStore:
		if len(operands) >= 2 {
			inst.Operands = make([]uint32, len(operands))
			copy(inst.Operands, operands)
		}

	// OpReturn / OpReturnValue
	case OpReturn:
		// no operands

	case OpReturnValue:
		if len(operands) >= 1 {
			inst.Operands = []uint32{operands[0]}
		}

	// OpBranch: target label
	case OpBranch:
		if len(operands) >= 1 {
			inst.Operands = []uint32{operands[0]}
		}

	case OpBranchConditional:
		if len(operands) >= 3 {
			inst.Operands = make([]uint32, len(operands))
			copy(inst.Operands, operands)
		}

	case OpSelectionMerge, OpLoopMerge:
		if len(operands) >= 1 {
			inst.Operands = make([]uint32, len(operands))
			copy(inst.Operands, operands)
		}

	// Instructions with only operands (no result type/ID).
	case OpAtomicStore, OpControlBarrier, OpMemoryBarrier,
		OpSwitch, OpKill, OpUnreachable:
		if len(operands) > 0 {
			inst.Operands = make([]uint32, len(operands))
			copy(inst.Operands, operands)
		}

	// OpFunctionParameter: resultType resultID
	case OpFunctionParameter:
		if len(operands) >= 2 {
			inst.TypeID = operands[0]
			inst.ResultID = operands[1]
		}

	default:
		// Unknown instruction — store all operands for forward compatibility.
		if len(operands) > 0 {
			inst.Operands = make([]uint32, len(operands))
			copy(inst.Operands, operands)
		}
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
		return Uint32(0)
	}
	switch ti.Kind {
	case TypeFloat:
		return math.Float32frombits(literals[0])
	case TypeInt:
		if ti.Signed {
			return int32(literals[0])
		}
		return literals[0]
	default:
		return literals[0]
	}
}

// zeroValue returns the zero/default value for a given type.
func zeroValue(types map[uint32]*TypeInfo, typeID uint32) Value {
	ti, ok := types[typeID]
	if !ok {
		return Uint32(0)
	}
	switch ti.Kind {
	case TypeFloat:
		return Float32(0)
	case TypeInt:
		if ti.Signed {
			return Int32(0)
		}
		return Uint32(0)
	case TypeVector:
		switch ti.Components {
		case 2:
			return Vec2{}
		case 3:
			return Vec3{}
		case 4:
			return Vec4{}
		}
	case TypeArray:
		if ti.Length > 0 {
			arr := make(Array, ti.Length)
			for i := range arr {
				arr[i] = zeroValue(types, ti.ElemType)
			}
			return arr
		}
	}
	return Uint32(0)
}
