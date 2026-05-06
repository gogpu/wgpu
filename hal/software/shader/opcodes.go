//go:build !(js && wasm)

// SPIR-V opcode constants and instruction representation.
//
// Only opcodes needed for the triangle shader are defined here.
// See SPIR-V specification section 3.32 for the full opcode list.

package shader

// SPIR-V opcodes relevant to the triangle shader.
// Reference: SPIR-V Specification v1.6, section 3.32 (Instructions).
const (
	OpNop                  = 0
	OpExtInstImport        = 11
	OpMemoryModel          = 14
	OpEntryPoint           = 15
	OpExecutionMode        = 16
	OpCapability           = 17
	OpTypeVoid             = 19
	OpTypeBool             = 20
	OpTypeInt              = 21
	OpTypeFloat            = 22
	OpTypeVector           = 23
	OpTypeArray            = 28
	OpTypeStruct           = 30
	OpTypePointer          = 32
	OpTypeFunction         = 33
	OpConstant             = 43
	OpConstantComposite    = 44
	OpConstantNull         = 46
	OpFunction             = 54
	OpFunctionParameter    = 55
	OpFunctionEnd          = 56
	OpVariable             = 59
	OpLoad                 = 61
	OpStore                = 62
	OpAccessChain          = 65
	OpDecorate             = 71
	OpMemberDecorate       = 72
	OpCompositeExtract     = 81
	OpCompositeConstruct   = 80
	OpReturn               = 253
	OpReturnValue          = 254
	OpLabel                = 248
	OpBranch               = 249
	OpSource               = 3
	OpSourceExtension      = 4
	OpName                 = 5
	OpMemberName           = 6
	OpString               = 7
	OpLine                 = 8
	OpNoLine               = 317
	OpModuleProcessed      = 330
	OpUndef                = 1
	OpExtInst              = 12
	OpBitcast              = 124
	OpConvertUToF          = 112
	OpConvertFToU          = 109
	OpConvertSToF          = 111
	OpIAdd                 = 128
	OpISub                 = 130
	OpIMul                 = 132
	OpFAdd                 = 129
	OpFSub                 = 131
	OpFMul                 = 133
	OpFDiv                 = 136
	OpSDiv                 = 135
	OpUDiv                 = 134
	OpFNegate              = 127
	OpSelect               = 169
	OpBranchConditional    = 250
	OpSelectionMerge       = 247
	OpLoopMerge            = 246
	OpPhi                  = 245
	OpIEqual               = 170
	OpINotEqual            = 171
	OpFOrdEqual            = 180
	OpFOrdLessThan         = 184
	OpFOrdGreaterThan      = 186
	OpFOrdLessThanEqual    = 188
	OpFOrdGreaterThanEqual = 190
)

// SPIR-V storage classes.
const (
	StorageClassUniformConstant = 0
	StorageClassInput           = 1
	StorageClassUniform         = 2
	StorageClassOutput          = 3
	StorageClassWorkgroup       = 4
	StorageClassCrossWorkgroup  = 5
	StorageClassPrivate         = 6
	StorageClassFunction        = 7
	StorageClassGeneric         = 8
	StorageClassPushConstant    = 9
	StorageClassAtomicCounter   = 10
	StorageClassImage           = 11
	StorageClassStorageBuffer   = 12
)

// SPIR-V decoration constants.
const (
	DecorationBuiltIn     = 11
	DecorationLocation    = 30
	DecorationBinding     = 33
	DecorationArrayStride = 6
)

// SPIR-V BuiltIn values.
const (
	BuiltInPosition    = 0
	BuiltInVertexIndex = 42
)

// SPIR-V execution model constants.
const (
	ExecutionModelVertex   = 0
	ExecutionModelFragment = 4
)

// spirvMagic is the SPIR-V binary magic number.
const spirvMagic = 0x07230203

// Instruction represents a decoded SPIR-V instruction.
type Instruction struct {
	Opcode   uint16
	ResultID uint32   // 0 if instruction produces no result
	TypeID   uint32   // 0 if instruction has no result type
	Operands []uint32 // remaining operands after result/type
}
