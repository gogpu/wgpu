//go:build !(js && wasm)

// SPIR-V shader debugger for the software backend.
//
// Provides breakpoints, single-stepping, variable watches, and JSON trace
// output for SPIR-V shader execution. This is a unique feature of the software
// backend -- GPU backends execute shaders opaquely, but our CPU interpreter
// can expose every instruction, every SSA value, every control-flow decision.
//
// The debugger is designed for zero overhead when disabled: a single nil check
// on the DebugContext pointer gates all debug logic in the instruction loop.
//
// Usage:
//
//	debug := &shader.DebugContext{
//	    Breakpoints: map[int]bool{42: true},
//	    OnBreakpoint: func(event shader.InstructionEvent) {
//	        fmt.Printf("Break at PC %d: %s\n", event.PC, event.Instruction.Opcode)
//	    },
//	}
//	outputs, err := module.ExecuteWithDebug("vs_main", ctx, debug)

package shader

import (
	"encoding/json"
	"io"
)

// Type name constants for trace output.
const (
	typeNameFloat32 = "float32"
	typeNameUint32  = "uint32"
	typeNameInt32   = "int32"
	typeNameBool    = "bool"
	typeNameVec2    = "vec2"
	typeNameVec3    = "vec3"
	typeNameVec4    = "vec4"
	typeNamePtr     = "ptr"
	typeNameArray   = "array"
)

// Opcode name constants for trace output.
const (
	opNameUnknown            = "OpUnknown"
	opNameLoad               = "OpLoad"
	opNameCompositeConstruct = "OpCompositeConstruct"
	glslExtSetName           = "GLSL.std.450"
)

// DebugAction controls execution flow after a debug callback.
type DebugAction int

const (
	// DebugContinue resumes execution until the next breakpoint or watch trigger.
	DebugContinue DebugAction = iota

	// DebugStep executes one instruction, then fires the OnInstruction callback.
	DebugStep

	// DebugAbort stops execution immediately. ExecuteWithDebug returns
	// a nil output map and an ErrDebugAbort error.
	DebugAbort
)

// DebugContext configures shader debugging for a single ExecuteWithDebug call.
//
// All fields are optional. When DebugContext is nil, the interpreter runs at
// full speed with zero debug overhead.
type DebugContext struct {
	// OnInstruction is called before each instruction executes (when stepping
	// or when a watched variable changed). The returned DebugAction controls
	// whether to continue, step, or abort.
	OnInstruction func(event InstructionEvent) DebugAction

	// OnBreakpoint is called when execution reaches a breakpoint instruction
	// index. If both OnBreakpoint and OnInstruction are set, OnBreakpoint fires
	// first, then OnInstruction fires and its return value controls flow.
	OnBreakpoint func(event InstructionEvent)

	// OnError is called when an instruction produces a runtime error (e.g.,
	// exceeding max iterations). The error is still returned from ExecuteWithDebug.
	OnError func(event ErrorEvent)

	// Breakpoints maps instruction index (0-based within the function) to
	// a boolean indicating whether a breakpoint is set. The instruction at
	// the given index triggers the OnBreakpoint callback before execution.
	Breakpoints map[int]bool

	// WatchVariables maps SSA result IDs to watch status. When a watched
	// variable's value changes after an instruction, OnInstruction is called
	// with DebugStep regardless of breakpoints.
	WatchVariables map[uint32]bool

	// TraceEnabled enables JSON-lines trace output. Each instruction that
	// produces a result writes one JSON line to TraceWriter after execution.
	TraceEnabled bool

	// TraceWriter receives JSON-lines trace output when TraceEnabled is true.
	// Each line is a self-contained JSON object terminated by a newline.
	TraceWriter io.Writer
}

// InstructionEvent provides information about the current instruction during
// a debug callback. The Values slice is a read-only snapshot -- modifications
// are ignored by the interpreter.
type InstructionEvent struct {
	// PC is the 0-based instruction index within the current function.
	PC int

	// Instruction is the SPIR-V instruction about to execute (for OnInstruction
	// and OnBreakpoint) or just executed (for watch triggers).
	Instruction Instruction

	// Values is the current SSA value slice indexed by SPIR-V ID. This is a
	// direct reference to the interpreter's live values -- callers should
	// treat it as read-only.
	Values []Value

	// BlockLabel is the result ID of the current basic block's OpLabel.
	BlockLabel uint32
}

// ErrorEvent provides information about a runtime error during debug execution.
type ErrorEvent struct {
	// PC is the instruction index where the error occurred.
	PC int

	// Instruction is the instruction that caused the error.
	Instruction Instruction

	// Err is the runtime error.
	Err error
}

// traceEntry is the JSON structure written for each traced instruction.
// Fields are pre-allocated to avoid per-line allocations.
type traceEntry struct {
	PC     int    `json:"pc"`
	Op     string `json:"op"`
	Result uint32 `json:"result,omitempty"`
	Type   string `json:"type,omitempty"`
	Value  any    `json:"value,omitempty"`
}

// writeTrace writes a single JSON-lines trace entry for the executed instruction.
// Called after instruction execution when TraceEnabled is true and the instruction
// produces a result (ResultID != 0).
func writeTrace(_ io.Writer, enc *json.Encoder, entry *traceEntry, pc int, inst Instruction, values []Value) {
	if inst.ResultID == 0 {
		return
	}

	var val Value
	if int(inst.ResultID) < len(values) {
		val = values[inst.ResultID]
	}

	entry.PC = pc
	entry.Op = opcodeName(inst.Opcode)
	entry.Result = inst.ResultID
	entry.Value = formatTraceValue(val)
	entry.Type = valueTypeName(val)

	// Encoder writes one JSON object followed by a newline.
	_ = enc.Encode(entry)
}

// formatTraceValue converts a runtime Value into a JSON-friendly representation.
// Vectors become float arrays, scalars become their native type.
func formatTraceValue(val Value) any {
	switch val.Tag {
	case TagFloat32:
		return val.F[0]
	case TagUint32:
		return val.U[0]
	case TagInt32:
		return int32(val.U[0])
	case TagBool:
		return val.U[0] != 0
	case TagVec2:
		return [2]float32{val.F[0], val.F[1]}
	case TagVec3:
		return [3]float32{val.F[0], val.F[1], val.F[2]}
	case TagVec4:
		return [4]float32{val.F[0], val.F[1], val.F[2], val.F[3]}
	case TagPointer:
		ptr := val.AsPointer()
		if ptr != nil {
			return formatTraceValue(ptr.Val)
		}
		return nil
	default:
		return nil
	}
}

// valueTypeName returns a human-readable type name for trace output.
func valueTypeName(val Value) string {
	switch val.Tag {
	case TagFloat32:
		return typeNameFloat32
	case TagUint32:
		return typeNameUint32
	case TagInt32:
		return typeNameInt32
	case TagBool:
		return typeNameBool
	case TagVec2:
		return typeNameVec2
	case TagVec3:
		return typeNameVec3
	case TagVec4:
		return typeNameVec4
	case TagPointer:
		return typeNamePtr
	case TagArray:
		return typeNameArray
	default:
		return ""
	}
}

// valuesEqual compares two Values for equality, used by watch variable tracking.
// Returns true if the values are structurally identical.
func valuesEqual(a, b Value) bool {
	if a.Tag != b.Tag {
		return false
	}
	switch a.Tag {
	case TagNone:
		return true
	case TagFloat32:
		return a.F[0] == b.F[0]
	case TagUint32, TagInt32, TagBool:
		return a.U[0] == b.U[0]
	case TagVec2:
		return a.F[0] == b.F[0] && a.F[1] == b.F[1]
	case TagVec3:
		return a.F[0] == b.F[0] && a.F[1] == b.F[1] && a.F[2] == b.F[2]
	case TagVec4:
		return a.F == b.F
	default:
		// For complex types (Array, Pointer), fall back to pointer equality.
		return a.Ref == b.Ref
	}
}

// opcodeName returns a human-readable name for a SPIR-V opcode.
// Covers all opcodes implemented by the interpreter.
func opcodeName(op uint16) string {
	if name, ok := opcodeNames[op]; ok {
		return name
	}
	return opNameUnknown
}

// opcodeNames maps SPIR-V opcode numbers to their string names.
var opcodeNames = map[uint16]string{
	OpNop:                    "OpNop",
	OpUndef:                  "OpUndef",
	OpLabel:                  "OpLabel",
	OpVariable:               "OpVariable",
	OpLoad:                   opNameLoad,
	OpStore:                  "OpStore",
	OpAccessChain:            "OpAccessChain",
	OpCompositeConstruct:     opNameCompositeConstruct,
	OpCompositeExtract:       "OpCompositeExtract",
	OpCopyObject:             "OpCopyObject",
	OpVectorShuffle:          "OpVectorShuffle",
	OpConvertUToF:            "OpConvertUToF",
	OpConvertSToF:            "OpConvertSToF",
	OpConvertFToU:            "OpConvertFToU",
	OpConvertFToS:            "OpConvertFToS",
	OpBitcast:                "OpBitcast",
	OpSConvert:               "OpSConvert",
	OpUConvert:               "OpUConvert",
	OpFConvert:               "OpFConvert",
	OpFAdd:                   "OpFAdd",
	OpFSub:                   "OpFSub",
	OpFMul:                   "OpFMul",
	OpFDiv:                   "OpFDiv",
	OpFNegate:                "OpFNegate",
	OpFMod:                   "OpFMod",
	OpFRem:                   "OpFRem",
	OpIAdd:                   "OpIAdd",
	OpISub:                   "OpISub",
	OpIMul:                   "OpIMul",
	OpSDiv:                   "OpSDiv",
	OpUDiv:                   "OpUDiv",
	OpSMod:                   "OpSMod",
	OpUMod:                   "OpUMod",
	OpSRem:                   "OpSRem",
	OpSNegate:                "OpSNegate",
	OpDot:                    "OpDot",
	OpVectorTimesScalar:      "OpVectorTimesScalar",
	OpMatrixTimesVector:      "OpMatrixTimesVector",
	OpMatrixTimesScalar:      "OpMatrixTimesScalar",
	OpMatrixTimesMatrix:      "OpMatrixTimesMatrix",
	OpTranspose:              "OpTranspose",
	OpSelect:                 "OpSelect",
	OpPhi:                    "OpPhi",
	OpBranch:                 "OpBranch",
	OpBranchConditional:      "OpBranchConditional",
	OpSelectionMerge:         "OpSelectionMerge",
	OpLoopMerge:              "OpLoopMerge",
	OpSwitch:                 "OpSwitch",
	OpReturn:                 "OpReturn",
	OpReturnValue:            "OpReturnValue",
	OpKill:                   "OpKill",
	OpUnreachable:            "OpUnreachable",
	OpIEqual:                 "OpIEqual",
	OpINotEqual:              "OpINotEqual",
	OpFOrdEqual:              "OpFOrdEqual",
	OpFOrdLessThan:           "OpFOrdLessThan",
	OpFOrdGreaterThan:        "OpFOrdGreaterThan",
	OpFOrdLessThanEqual:      "OpFOrdLessThanEqual",
	OpFOrdGreaterThanEqual:   "OpFOrdGreaterThanEqual",
	OpULessThan:              "OpULessThan",
	OpUGreaterThan:           "OpUGreaterThan",
	OpULessThanEqual:         "OpULessThanEqual",
	OpUGreaterThanEqual:      "OpUGreaterThanEqual",
	OpSLessThan:              "OpSLessThan",
	OpSGreaterThan:           "OpSGreaterThan",
	OpSLessThanEqual:         "OpSLessThanEqual",
	OpSGreaterThanEqual:      "OpSGreaterThanEqual",
	OpLogicalAnd:             "OpLogicalAnd",
	OpLogicalOr:              "OpLogicalOr",
	OpLogicalNot:             "OpLogicalNot",
	OpBitwiseAnd:             "OpBitwiseAnd",
	OpBitwiseOr:              "OpBitwiseOr",
	OpBitwiseXor:             "OpBitwiseXor",
	OpNot:                    "OpNot",
	OpShiftLeftLogical:       "OpShiftLeftLogical",
	OpShiftRightLogical:      "OpShiftRightLogical",
	OpShiftRightArithmetic:   "OpShiftRightArithmetic",
	OpSampledImage:           "OpSampledImage",
	OpImageSampleImplicitLod: "OpImageSampleImplicitLod",
	OpImageSampleExplicitLod: "OpImageSampleExplicitLod",
	OpImageFetch:             "OpImageFetch",
	OpImageQuerySize:         "OpImageQuerySize",
	OpAtomicLoad:             "OpAtomicLoad",
	OpAtomicStore:            "OpAtomicStore",
	OpAtomicExchange:         "OpAtomicExchange",
	OpAtomicCompareExchange:  "OpAtomicCompareExchange",
	OpAtomicIIncrement:       "OpAtomicIIncrement",
	OpAtomicIDecrement:       "OpAtomicIDecrement",
	OpAtomicIAdd:             "OpAtomicIAdd",
	OpAtomicISub:             "OpAtomicISub",
	OpAtomicSMin:             "OpAtomicSMin",
	OpAtomicUMin:             "OpAtomicUMin",
	OpAtomicSMax:             "OpAtomicSMax",
	OpAtomicUMax:             "OpAtomicUMax",
	OpControlBarrier:         "OpControlBarrier",
	OpMemoryBarrier:          "OpMemoryBarrier",
	OpExtInst:                "OpExtInst",
	OpFunctionCall:           "OpFunctionCall",
	OpFunctionParameter:      "OpFunctionParameter",
	OpFunctionEnd:            "OpFunctionEnd",
}
