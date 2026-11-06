//go:build !(js && wasm)

// SPIR-V opcode constants, the table of opcodes the interpreter executes,
// and the instruction representation.
// See SPIR-V specification section 3.32 for the full opcode list.

package shader

import (
	"errors"
	"fmt"
)

// SPIR-V opcodes the parser and the interpreter use.
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
	OpTypeMatrix           = 24
	OpTypeArray            = 28
	OpTypeRuntimeArray     = 29
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
	OpArrayLength          = 68
	OpDecorate             = 71
	OpMemberDecorate       = 72
	OpCompositeExtract     = 81
	OpCompositeConstruct   = 80
	OpCompositeInsert      = 82
	OpVectorExtractDynamic = 77
	OpVectorInsertDynamic  = 78
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

	// Additional opcodes for Phases 2-6.
	OpDot               = 148
	OpVectorTimesScalar = 142
	OpMatrixTimesVector = 145
	OpVectorTimesMatrix = 144
	OpMatrixTimesScalar = 143
	OpMatrixTimesMatrix = 146
	OpTranspose         = 84
	OpVectorShuffle     = 79

	OpFunctionCall = 57

	OpSwitch              = 251
	OpKill                = 252
	OpUnreachable         = 255
	OpTerminateInvocation = 4416 // SPV_KHR_terminate_invocation, core in SPIR-V 1.6

	// Comparison ops (integer, unsigned).
	OpULessThan         = 176
	OpUGreaterThan      = 172
	OpULessThanEqual    = 178
	OpUGreaterThanEqual = 174
	OpSLessThan         = 177
	OpSGreaterThan      = 173
	OpSLessThanEqual    = 179
	OpSGreaterThanEqual = 175

	// Logical ops.
	OpLogicalAnd      = 167
	OpLogicalOr       = 166
	OpLogicalNot      = 168
	OpLogicalEqual    = 164
	OpLogicalNotEqual = 165

	// Float inequality, ordered (false with a NaN) and unordered (true).
	OpFOrdNotEqual   = 182
	OpFUnordNotEqual = 183

	// Bit field ops.
	OpBitFieldInsert   = 201
	OpBitFieldSExtract = 202
	OpBitFieldUExtract = 203
	OpBitReverse       = 204
	OpBitCount         = 205

	// Derivatives (fragment stage).
	OpDPdx         = 207
	OpDPdy         = 208
	OpFwidth       = 209
	OpDPdxFine     = 210
	OpDPdyFine     = 211
	OpFwidthFine   = 212
	OpDPdxCoarse   = 213
	OpDPdyCoarse   = 214
	OpFwidthCoarse = 215

	// Bitwise ops.
	OpBitwiseAnd           = 199
	OpBitwiseOr            = 197
	OpBitwiseXor           = 198
	OpNot                  = 200
	OpShiftLeftLogical     = 196
	OpShiftRightLogical    = 194
	OpShiftRightArithmetic = 195

	// Image / sampler ops.
	OpTypeImage              = 25
	OpTypeSampler            = 26
	OpTypeSampledImage       = 27
	OpSampledImage           = 86
	OpImageSampleImplicitLod = 87
	OpImageSampleExplicitLod = 88
	// The Dref forms compare a reference depth with each texel before
	// filtering (WGSL textureSampleCompare and textureSampleCompareLevel).
	OpImageSampleDrefImplicitLod = 89
	OpImageSampleDrefExplicitLod = 90
	OpImageFetch                 = 95
	OpImageGather                = 96
	OpImageDrefGather            = 97
	OpImageRead                  = 98
	OpImageWrite                 = 99
	OpImageQuerySizeLod          = 103
	OpImageQuerySize             = 104
	OpImageQueryLevels           = 106
	OpImageQuerySamples          = 107

	// Atomic ops.
	OpAtomicLoad            = 227
	OpAtomicStore           = 228
	OpAtomicExchange        = 229
	OpAtomicCompareExchange = 230
	OpAtomicIIncrement      = 232
	OpAtomicIDecrement      = 233
	OpAtomicIAdd            = 234
	OpAtomicISub            = 235
	OpAtomicSMin            = 236
	OpAtomicUMin            = 237
	OpAtomicSMax            = 238
	OpAtomicUMax            = 239
	OpAtomicAnd             = 240
	OpAtomicOr              = 241
	OpAtomicXor             = 242

	// Barrier ops.
	OpControlBarrier = 224
	OpMemoryBarrier  = 225

	// Type conversion ops.
	OpConvertFToS = 110
	OpSConvert    = 114
	OpUConvert    = 113
	OpFConvert    = 115

	// Misc ops.
	OpCopyObject = 83
	OpFMod       = 141
	OpSMod       = 139
	OpUMod       = 137
	OpSRem       = 138
	OpFRem       = 140
	OpSNegate    = 126

	// Float classification and rounding.
	OpIsNan         = 156
	OpIsInf         = 157
	OpQuantizeToF16 = 116

	// Packed 4x8-bit integer dot products (SPV_KHR_integer_dot_product,
	// core in SPIR-V 1.6): WGSL dot4I8Packed and dot4U8Packed.
	OpSDotKHR = 4450
	OpUDotKHR = 4451

	// Reductions of a bool vector: naga's loop guard, its workgroup
	// zero-init prologue, and WGSL all() and any().
	OpAny = 154
	OpAll = 155

	// Constant ops.
	OpConstantTrue      = 41
	OpConstantFalse     = 42
	OpSpecConstant      = 48
	OpSpecConstantTrue  = 49
	OpSpecConstantFalse = 50

	// Image operand masks.
	ImageOperandBiasMask = 0x1
	ImageOperandLodMask  = 0x2
	ImageOperandGradMask = 0x4
	// ImageOperandConstOffsetMask and ImageOperandOffsetMask add a texel
	// offset to the coordinate; ConstOffsets, Sample and MinLod follow them.
	ImageOperandConstOffsetMask = 0x8
	ImageOperandOffsetMask      = 0x10
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
	DecorationArrayStride   = 6
	DecorationBuiltIn       = 11
	DecorationFlat          = 14
	DecorationBinding       = 33
	DecorationDescriptorSet = 34
	DecorationLocation      = 30
	DecorationOffset        = 35
	DecorationBlock         = 2
	DecorationBufferBlock   = 3
	DecorationColMajor      = 5
	DecorationMatrixStride  = 7
	DecorationNonWritable   = 24
	DecorationNonReadable   = 25
)

// SPIR-V BuiltIn values.
const (
	BuiltInPosition           = 0
	BuiltInPointSize          = 1
	BuiltInVertexIndex        = 42
	BuiltInInstanceIndex      = 43
	BuiltInFragCoord          = 15
	BuiltInFrontFacing        = 17
	BuiltInSampleID           = 18
	BuiltInSampleMask         = 20
	BuiltInFragDepth          = 22
	BuiltInGlobalInvocationID = 28
	BuiltInLocalInvocationID  = 27
	BuiltInWorkgroupID        = 26
	BuiltInNumWorkgroups      = 24
	BuiltInLocalInvocationIdx = 29
	BuiltInWorkgroupSize      = 25
)

// SPIR-V execution model constants.
const (
	ExecutionModelVertex    = 0
	ExecutionModelFragment  = 4
	ExecutionModelGLCompute = 5
)

// Execution mode constants.
const (
	ExecutionModeLocalSize = 17
)

// spirvMagic is the SPIR-V binary magic number.
const spirvMagic = 0x07230203

// SPIR-V operands ParseModule rejects at module level: f16 and ray query are
// not executed by the interpreter, and the software adapter advertises
// neither (ADR-078 rule 6).
const (
	opTypeRayQueryKHR              = 4472
	opTypeAccelerationStructureKHR = 5341
)

// rejectedCapabilities names the SPIR-V capabilities ParseModule rejects.
var rejectedCapabilities = map[uint32]string{
	9:    "Float16",
	4471: "RayQueryProvisionalKHR",
	4472: "RayQueryKHR",
}

// ErrUnsupported is wrapped by every error the interpreter returns for valid
// SPIR-V it does not execute: an opcode in a function body that run() has no
// case for, an f16 type or capability, or ray query. A malformed binary
// gives a plain parse error instead. Callers match it with errors.Is.
var ErrUnsupported = errors.New("not supported")

// unsupportedf returns an error that wraps ErrUnsupported:
// "software: <what> is not supported".
func unsupportedf(format string, args ...any) error {
	return fmt.Errorf("software: "+format+" is %w", append(args, ErrUnsupported)...)
}

// unsupportedOpcode is the error for an opcode that is not in supportedOps.
func unsupportedOpcode(op uint16) error {
	return unsupportedf("SPIR-V opcode %d (%s)", op, opcodeName(op))
}

// opInfo is one entry of supportedOps.
type opInfo struct {
	name string
	// typed is set when the instruction's first two words are its result
	// type and result ID, which decodeInstruction splits off.
	typed bool
}

// supportedOps lists every opcode a function body may hold: exactly the
// opcodes run() has a case for. ParseModule rejects any other opcode in a
// function body (SwiftShader's analysis-time check, SpirvShader.cpp:812-813),
// and decodeInstruction takes its typed layout from here. TestSupportedOps
// checks the table against run()'s case labels.
var supportedOps = map[uint16]opInfo{
	// Blocks, variables, control flow.
	OpLabel:               {"OpLabel", false},
	OpVariable:            {"OpVariable", true},
	OpFunctionParameter:   {"OpFunctionParameter", true},
	OpFunctionCall:        {"OpFunctionCall", true},
	OpBranch:              {"OpBranch", false},
	OpBranchConditional:   {"OpBranchConditional", false},
	OpSelectionMerge:      {"OpSelectionMerge", false},
	OpLoopMerge:           {"OpLoopMerge", false},
	OpSwitch:              {"OpSwitch", false},
	OpPhi:                 {"OpPhi", true},
	OpReturn:              {"OpReturn", false},
	OpReturnValue:         {"OpReturnValue", false},
	OpKill:                {"OpKill", false},
	OpTerminateInvocation: {"OpTerminateInvocation", false},
	OpUnreachable:         {"OpUnreachable", false},

	// Memory and composites.
	OpLoad:                 {opNameLoad, true},
	OpStore:                {"OpStore", false},
	OpAccessChain:          {"OpAccessChain", true},
	OpArrayLength:          {"OpArrayLength", true},
	OpCompositeConstruct:   {opNameCompositeConstruct, true},
	OpCompositeExtract:     {"OpCompositeExtract", true},
	OpCompositeInsert:      {"OpCompositeInsert", true},
	OpVectorExtractDynamic: {"OpVectorExtractDynamic", true},
	OpVectorInsertDynamic:  {"OpVectorInsertDynamic", true},
	OpVectorShuffle:        {"OpVectorShuffle", true},
	OpCopyObject:           {"OpCopyObject", true},
	OpUndef:                {"OpUndef", true},

	// Conversions.
	OpConvertUToF: {"OpConvertUToF", true},
	OpConvertSToF: {"OpConvertSToF", true},
	OpConvertFToU: {"OpConvertFToU", true},
	OpConvertFToS: {"OpConvertFToS", true},
	OpBitcast:     {"OpBitcast", true},
	OpSConvert:    {"OpSConvert", true},
	OpUConvert:    {"OpUConvert", true},
	OpFConvert:    {"OpFConvert", true},

	// Float arithmetic, matrices and classification.
	OpFAdd:              {"OpFAdd", true},
	OpFSub:              {"OpFSub", true},
	OpFMul:              {"OpFMul", true},
	OpFDiv:              {"OpFDiv", true},
	OpFNegate:           {"OpFNegate", true},
	OpFMod:              {"OpFMod", true},
	OpFRem:              {"OpFRem", true},
	OpDot:               {"OpDot", true},
	OpVectorTimesScalar: {"OpVectorTimesScalar", true},
	OpMatrixTimesVector: {"OpMatrixTimesVector", true},
	OpVectorTimesMatrix: {"OpVectorTimesMatrix", true},
	OpMatrixTimesScalar: {"OpMatrixTimesScalar", true},
	OpMatrixTimesMatrix: {"OpMatrixTimesMatrix", true},
	OpTranspose:         {"OpTranspose", true},
	OpIsNan:             {"OpIsNan", true},
	OpIsInf:             {"OpIsInf", true},
	OpQuantizeToF16:     {"OpQuantizeToF16", true},

	// Integer arithmetic and bits.
	OpIAdd:                 {"OpIAdd", true},
	OpISub:                 {"OpISub", true},
	OpIMul:                 {"OpIMul", true},
	OpSDiv:                 {"OpSDiv", true},
	OpUDiv:                 {"OpUDiv", true},
	OpSMod:                 {"OpSMod", true},
	OpUMod:                 {"OpUMod", true},
	OpSRem:                 {"OpSRem", true},
	OpSNegate:              {"OpSNegate", true},
	OpSDotKHR:              {"OpSDotKHR", true},
	OpUDotKHR:              {"OpUDotKHR", true},
	OpBitwiseAnd:           {"OpBitwiseAnd", true},
	OpBitwiseOr:            {"OpBitwiseOr", true},
	OpBitwiseXor:           {"OpBitwiseXor", true},
	OpNot:                  {"OpNot", true},
	OpShiftLeftLogical:     {"OpShiftLeftLogical", true},
	OpShiftRightLogical:    {"OpShiftRightLogical", true},
	OpShiftRightArithmetic: {"OpShiftRightArithmetic", true},
	OpBitFieldInsert:       {"OpBitFieldInsert", true},
	OpBitFieldSExtract:     {"OpBitFieldSExtract", true},
	OpBitFieldUExtract:     {"OpBitFieldUExtract", true},
	OpBitReverse:           {"OpBitReverse", true},
	OpBitCount:             {"OpBitCount", true},

	// Comparison, selection and logic.
	OpSelect:               {"OpSelect", true},
	OpIEqual:               {"OpIEqual", true},
	OpINotEqual:            {"OpINotEqual", true},
	OpULessThan:            {"OpULessThan", true},
	OpUGreaterThan:         {"OpUGreaterThan", true},
	OpULessThanEqual:       {"OpULessThanEqual", true},
	OpUGreaterThanEqual:    {"OpUGreaterThanEqual", true},
	OpSLessThan:            {"OpSLessThan", true},
	OpSGreaterThan:         {"OpSGreaterThan", true},
	OpSLessThanEqual:       {"OpSLessThanEqual", true},
	OpSGreaterThanEqual:    {"OpSGreaterThanEqual", true},
	OpFOrdEqual:            {"OpFOrdEqual", true},
	OpFOrdLessThan:         {"OpFOrdLessThan", true},
	OpFOrdGreaterThan:      {"OpFOrdGreaterThan", true},
	OpFOrdLessThanEqual:    {"OpFOrdLessThanEqual", true},
	OpFOrdGreaterThanEqual: {"OpFOrdGreaterThanEqual", true},
	OpFOrdNotEqual:         {"OpFOrdNotEqual", true},
	OpFUnordNotEqual:       {"OpFUnordNotEqual", true},
	OpLogicalAnd:           {"OpLogicalAnd", true},
	OpLogicalOr:            {"OpLogicalOr", true},
	OpLogicalEqual:         {"OpLogicalEqual", true},
	OpLogicalNotEqual:      {"OpLogicalNotEqual", true},
	OpLogicalNot:           {"OpLogicalNot", true},
	OpAny:                  {"OpAny", true},
	OpAll:                  {"OpAll", true},

	// Fragment derivatives.
	OpDPdx:         {"OpDPdx", true},
	OpDPdy:         {"OpDPdy", true},
	OpFwidth:       {"OpFwidth", true},
	OpDPdxFine:     {"OpDPdxFine", true},
	OpDPdyFine:     {"OpDPdyFine", true},
	OpFwidthFine:   {"OpFwidthFine", true},
	OpDPdxCoarse:   {"OpDPdxCoarse", true},
	OpDPdyCoarse:   {"OpDPdyCoarse", true},
	OpFwidthCoarse: {"OpFwidthCoarse", true},

	// Images.
	OpSampledImage:               {"OpSampledImage", true},
	OpImageSampleImplicitLod:     {"OpImageSampleImplicitLod", true},
	OpImageSampleExplicitLod:     {"OpImageSampleExplicitLod", true},
	OpImageSampleDrefImplicitLod: {"OpImageSampleDrefImplicitLod", true},
	OpImageSampleDrefExplicitLod: {"OpImageSampleDrefExplicitLod", true},
	OpImageGather:                {"OpImageGather", true},
	OpImageDrefGather:            {"OpImageDrefGather", true},
	OpImageFetch:                 {"OpImageFetch", true},
	OpImageRead:                  {"OpImageRead", true},
	OpImageWrite:                 {"OpImageWrite", false},
	OpImageQuerySize:             {"OpImageQuerySize", true},
	OpImageQuerySizeLod:          {"OpImageQuerySizeLod", true},
	OpImageQueryLevels:           {"OpImageQueryLevels", true},
	OpImageQuerySamples:          {"OpImageQuerySamples", true},

	// Atomics and barriers.
	OpAtomicLoad:            {"OpAtomicLoad", true},
	OpAtomicStore:           {"OpAtomicStore", false},
	OpAtomicExchange:        {"OpAtomicExchange", true},
	OpAtomicCompareExchange: {"OpAtomicCompareExchange", true},
	OpAtomicIIncrement:      {"OpAtomicIIncrement", true},
	OpAtomicIDecrement:      {"OpAtomicIDecrement", true},
	OpAtomicIAdd:            {"OpAtomicIAdd", true},
	OpAtomicISub:            {"OpAtomicISub", true},
	OpAtomicSMin:            {"OpAtomicSMin", true},
	OpAtomicUMin:            {"OpAtomicUMin", true},
	OpAtomicSMax:            {"OpAtomicSMax", true},
	OpAtomicUMax:            {"OpAtomicUMax", true},
	OpAtomicAnd:             {"OpAtomicAnd", true},
	OpAtomicOr:              {"OpAtomicOr", true},
	OpAtomicXor:             {"OpAtomicXor", true},
	OpControlBarrier:        {"OpControlBarrier", false},
	OpMemoryBarrier:         {"OpMemoryBarrier", false},

	// GLSL.std.450 builtins.
	OpExtInst: {"OpExtInst", true},
}

// otherOpNames names opcodes that are not in supportedOps, so that traces
// and the unsupported-opcode error name them: the no-op and debug-line
// instructions ParseModule drops, and opcodes the interpreter refuses. naga
// defines the projective samples and OpImageQueryLod but never emits them
// (WGSL has neither); the others need features the software adapter does
// not advertise (f32 and texture atomics, SPIR-V 1.4, ray query).
var otherOpNames = map[uint16]string{
	OpNop:                          "OpNop",
	OpLine:                         "OpLine",
	OpNoLine:                       "OpNoLine",
	OpFunctionEnd:                  "OpFunctionEnd",
	60:                             "OpImageTexelPointer",
	91:                             "OpImageSampleProjImplicitLod",
	92:                             "OpImageSampleProjExplicitLod",
	93:                             "OpImageSampleProjDrefImplicitLod",
	94:                             "OpImageSampleProjDrefExplicitLod",
	105:                            "OpImageQueryLod",
	400:                            "OpCopyLogical",
	4473:                           "OpRayQueryInitializeKHR",
	4474:                           "OpRayQueryTerminateKHR",
	4475:                           "OpRayQueryGenerateIntersectionKHR",
	4476:                           "OpRayQueryConfirmIntersectionKHR",
	4477:                           "OpRayQueryProceedKHR",
	4479:                           "OpRayQueryGetIntersectionTypeKHR",
	6035:                           "OpAtomicFAddEXT",
	opTypeRayQueryKHR:              "OpTypeRayQueryKHR",
	opTypeAccelerationStructureKHR: "OpTypeAccelerationStructureKHR",
}

// opcodeName returns the name of a SPIR-V opcode, "OpUnknown" for one the
// interpreter has no name for.
func opcodeName(op uint16) string {
	if info, ok := supportedOps[op]; ok {
		return info.name
	}
	if name, ok := otherOpNames[op]; ok {
		return name
	}
	return opNameUnknown
}

// Instruction represents a decoded SPIR-V instruction.
type Instruction struct {
	Opcode uint16
	// resShape is the shape of the result type (TypeID), precomputed by
	// ParseModule; zero when the result is not a 32-bit scalar or vector. It
	// sits in the padding after Opcode, so Instruction stays 40 bytes.
	resShape shape
	ResultID uint32   // 0 if instruction produces no result
	TypeID   uint32   // 0 if instruction has no result type
	Operands []uint32 // remaining operands after result/type
}
