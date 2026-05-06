//go:build !(js && wasm)

package shader

import (
	"math"
	"testing"
)

// =============================================================================
// Phase 5 Tests: Control Flow (loops, phi, function calls, switch)
// =============================================================================

// buildLoopSumSPIRVFixed constructs SPIR-V for a shader that sums integers 1..N using a loop:
//
//	@group(0) @binding(0) var<uniform> params: Params;
//	struct Params { n: u32 }
//	@fragment fn fs_main() -> @location(0) vec4<f32> {
//	    var sum: u32 = 0;
//	    var i: u32 = 1;
//	    loop {
//	        if (i > n) { break; }
//	        sum = sum + i;
//	        i = i + 1;
//	    }
//	    return vec4(f32(sum), 0, 0, 1);
//	}
func buildLoopSumSPIRVFixed() []uint32 {
	inst := spirvInst
	str := spirvString
	f := math.Float32bits

	const (
		idVoid      = 1
		idFloat     = 2
		idVec4      = 3
		idUint      = 4
		idBool      = 5
		idPtrV4Out  = 6
		idFuncTy    = 7
		idFunc      = 8
		idColorOut  = 9
		idStruct    = 10
		idPtrStruct = 11
		idParamsVar = 12
		idConst0U   = 13
		idConst1U   = 14
		idPtrUint   = 15
		idChainN    = 16
		idLoadN     = 17
		idConst0F   = 18
		idConst1F   = 19
		idLblEntry  = 20
		idLblHeader = 21
		idLblBody   = 22
		idLblMerge  = 23
		idLblCont   = 24
		idPhiSum    = 25
		idPhiI      = 26
		idCmpGT     = 27
		idNewSum    = 28
		idNewI      = 29
		idSumFloat  = 30
		idResult    = 31
		idBound     = 32
	)

	nameWords := str("fs_main")
	epLen := uint16(3 + len(nameWords) + 1)
	epInst := append([]uint32{inst(epLen, OpEntryPoint), ExecutionModelFragment, idFunc}, nameWords...)
	epInst = append(epInst, idColorOut)

	words := make([]uint32, 0, 300)
	words = append(words,
		spirvMagic, 0x00010300, 0, idBound, 0,
		inst(2, OpCapability), 1,
		inst(3, OpMemoryModel), 0, 1,
	)
	words = append(words, epInst...)
	words = append(words,
		inst(3, OpExecutionMode), idFunc, 7,
		inst(4, OpDecorate), idColorOut, DecorationLocation, 0,
		inst(4, OpDecorate), idParamsVar, DecorationBinding, 0,
		inst(4, OpDecorate), idParamsVar, DecorationDescriptorSet, 0,
		inst(3, OpDecorate), idStruct, DecorationBlock,
		inst(5, OpMemberDecorate), idStruct, 0, DecorationOffset, 0,

		// Types.
		inst(2, OpTypeVoid), idVoid,
		inst(3, OpTypeFloat), idFloat, 32,
		inst(4, OpTypeInt), idUint, 32, 0,
		inst(2, OpTypeBool), idBool,
		inst(4, OpTypeVector), idVec4, idFloat, 4,
		inst(3, OpTypeStruct), idStruct, idUint,
		inst(4, OpTypePointer), idPtrV4Out, StorageClassOutput, idVec4,
		inst(4, OpTypePointer), idPtrStruct, StorageClassUniform, idStruct,
		inst(4, OpTypePointer), idPtrUint, StorageClassUniform, idUint,
		inst(3, OpTypeFunction), idFuncTy, idVoid,

		// Constants.
		inst(4, OpConstant), idUint, idConst0U, 0,
		inst(4, OpConstant), idUint, idConst1U, 1,
		inst(4, OpConstant), idFloat, idConst0F, f(0),
		inst(4, OpConstant), idFloat, idConst1F, f(1),

		// Variables.
		inst(4, OpVariable), idPtrV4Out, idColorOut, StorageClassOutput,
		inst(4, OpVariable), idPtrStruct, idParamsVar, StorageClassUniform,

		// Function.
		inst(5, OpFunction), idVoid, idFunc, 0, idFuncTy,

		// Entry block.
		inst(2, OpLabel), idLblEntry,
		inst(5, OpAccessChain), idPtrUint, idChainN, idParamsVar, idConst0U,
		inst(4, OpLoad), idUint, idLoadN, idChainN,
		inst(2, OpBranch), idLblHeader,

		// Loop header with phi.
		inst(2, OpLabel), idLblHeader,
		inst(7, OpPhi), idUint, idPhiSum, idConst0U, idLblEntry, idNewSum, idLblCont,
		inst(7, OpPhi), idUint, idPhiI, idConst1U, idLblEntry, idNewI, idLblCont,
		inst(4, OpLoopMerge), idLblMerge, idLblCont, 0,
		// Branch: if i > n -> merge (exit loop); else -> body
		inst(5, OpUGreaterThan), idBool, idCmpGT, idPhiI, idLoadN,
		inst(4, OpBranchConditional), idCmpGT, idLblMerge, idLblBody,

		// Body block: sum += i
		inst(2, OpLabel), idLblBody,
		inst(5, OpIAdd), idUint, idNewSum, idPhiSum, idPhiI,
		inst(5, OpIAdd), idUint, idNewI, idPhiI, idConst1U,
		inst(2, OpBranch), idLblCont,

		// Continue block: branch back to header
		inst(2, OpLabel), idLblCont,
		inst(2, OpBranch), idLblHeader,

		// Merge block: output result
		inst(2, OpLabel), idLblMerge,
		inst(4, OpConvertUToF), idFloat, idSumFloat, idPhiSum,
		inst(7, OpCompositeConstruct), idVec4, idResult, idSumFloat, idConst0F, idConst0F, idConst1F,
		inst(3, OpStore), idColorOut, idResult,
		inst(1, OpReturn),
		inst(1, OpFunctionEnd),
	)
	return words
}

func TestSPIRVLoopSum(t *testing.T) {
	tests := []struct {
		name string
		n    uint32
		want float32 // sum = n*(n+1)/2
	}{
		{"n=0", 0, 0},
		{"n=1", 1, 1},
		{"n=5", 5, 15},
		{"n=10", 10, 55},
		{"n=100", 100, 5050},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			words := buildLoopSumSPIRVFixed()
			m, err := ParseModule(words)
			if err != nil {
				t.Fatalf("ParseModule failed: %v", err)
			}

			buf := make([]byte, 4)
			buf[0] = byte(tt.n)
			buf[1] = byte(tt.n >> 8)
			buf[2] = byte(tt.n >> 16)
			buf[3] = byte(tt.n >> 24)

			ctx := &ExecutionContext{
				Buffers: map[BindingKey][]byte{{Group: 0, Binding: 0}: buf},
			}

			outputs, err := m.ExecuteWithContext("fs_main", ctx)
			if err != nil {
				t.Fatalf("Execute failed: %v", err)
			}

			ep := m.EntryPoints["fs_main"]
			for _, varID := range ep.InterfaceIDs {
				vi := m.Variables[varID]
				if vi != nil && vi.StorageClass == StorageClassOutput {
					color := Vec4ToFloat32(outputs[varID])
					if math.Abs(float64(color[0]-tt.want)) > 0.5 {
						t.Errorf("sum(%d) = %v, want %v", tt.n, color[0], tt.want)
					}
					return
				}
			}
			t.Fatal("output not found")
		})
	}
}

// buildFunctionCallSPIRV constructs SPIR-V for a shader with a function call:
//
//	fn double(x: f32) -> f32 { return x * 2.0; }
//	@fragment fn fs_main() -> @location(0) vec4<f32> {
//	    let r = double(params.value);
//	    return vec4(r, 0, 0, 1);
//	}
func buildFunctionCallSPIRV() []uint32 {
	inst := spirvInst
	str := spirvString
	f := math.Float32bits

	const (
		idVoid        = 1
		idFloat       = 2
		idVec4        = 3
		idPtrV4Out    = 4
		idFuncTyVoid  = 5
		idFuncTyFloat = 6
		idFuncMain    = 7
		idFuncDouble  = 8
		idColorOut    = 9
		idStruct      = 10
		idPtrStruct   = 11
		idParamsVar   = 12
		idUint        = 13
		idConst0U     = 14
		idConst2F     = 15
		idConst0F     = 16
		idConst1F     = 17
		idPtrFloat    = 18
		idChain       = 19
		idLoadVal     = 20
		idCallResult  = 21
		idResult      = 22
		// Double function
		idDblLabel  = 23
		idDblParam  = 24
		idDblResult = 25
		// Main labels
		idMainLabel = 26
		idBound     = 27
	)

	nameWords := str("fs_main")
	epLen := uint16(3 + len(nameWords) + 1)
	epInst := append([]uint32{inst(epLen, OpEntryPoint), ExecutionModelFragment, idFuncMain}, nameWords...)
	epInst = append(epInst, idColorOut)

	words := make([]uint32, 0, 200)
	words = append(words,
		spirvMagic, 0x00010300, 0, idBound, 0,
		inst(2, OpCapability), 1,
		inst(3, OpMemoryModel), 0, 1,
	)
	words = append(words, epInst...)
	words = append(words,
		inst(3, OpExecutionMode), idFuncMain, 7,
		inst(4, OpDecorate), idColorOut, DecorationLocation, 0,
		inst(4, OpDecorate), idParamsVar, DecorationBinding, 0,
		inst(4, OpDecorate), idParamsVar, DecorationDescriptorSet, 0,
		inst(3, OpDecorate), idStruct, DecorationBlock,
		inst(5, OpMemberDecorate), idStruct, 0, DecorationOffset, 0,

		inst(2, OpTypeVoid), idVoid,
		inst(3, OpTypeFloat), idFloat, 32,
		inst(4, OpTypeInt), idUint, 32, 0,
		inst(4, OpTypeVector), idVec4, idFloat, 4,
		inst(3, OpTypeStruct), idStruct, idFloat,
		inst(4, OpTypePointer), idPtrV4Out, StorageClassOutput, idVec4,
		inst(4, OpTypePointer), idPtrStruct, StorageClassUniform, idStruct,
		inst(4, OpTypePointer), idPtrFloat, StorageClassUniform, idFloat,
		inst(3, OpTypeFunction), idFuncTyVoid, idVoid,
		inst(4, OpTypeFunction), idFuncTyFloat, idFloat, idFloat, // f32 -> f32

		inst(4, OpConstant), idUint, idConst0U, 0,
		inst(4, OpConstant), idFloat, idConst2F, f(2.0),
		inst(4, OpConstant), idFloat, idConst0F, f(0),
		inst(4, OpConstant), idFloat, idConst1F, f(1),

		inst(4, OpVariable), idPtrV4Out, idColorOut, StorageClassOutput,
		inst(4, OpVariable), idPtrStruct, idParamsVar, StorageClassUniform,

		// double(x) function.
		inst(5, OpFunction), idFloat, idFuncDouble, 0, idFuncTyFloat,
		inst(3, OpFunctionParameter), idFloat, idDblParam,
		inst(2, OpLabel), idDblLabel,
		inst(5, OpFMul), idFloat, idDblResult, idDblParam, idConst2F,
		inst(2, OpReturnValue), idDblResult,
		inst(1, OpFunctionEnd),

		// Main function.
		inst(5, OpFunction), idVoid, idFuncMain, 0, idFuncTyVoid,
		inst(2, OpLabel), idMainLabel,
		inst(5, OpAccessChain), idPtrFloat, idChain, idParamsVar, idConst0U,
		inst(4, OpLoad), idFloat, idLoadVal, idChain,
		// Call double(loadVal)
		inst(5, OpFunctionCall), idFloat, idCallResult, idFuncDouble, idLoadVal,
		inst(7, OpCompositeConstruct), idVec4, idResult, idCallResult, idConst0F, idConst0F, idConst1F,
		inst(3, OpStore), idColorOut, idResult,
		inst(1, OpReturn),
		inst(1, OpFunctionEnd),
	)
	return words
}

func TestSPIRVFunctionCall(t *testing.T) {
	words := buildFunctionCallSPIRV()
	m, err := ParseModule(words)
	if err != nil {
		t.Fatalf("ParseModule failed: %v", err)
	}

	tests := []struct {
		name  string
		input float32
		want  float32
	}{
		{"double_5", 5.0, 10.0},
		{"double_0", 0.0, 0.0},
		{"double_neg", -3.0, -6.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := make([]byte, 4)
			putFloat32LE(buf, tt.input)
			ctx := &ExecutionContext{
				Buffers: map[BindingKey][]byte{{Group: 0, Binding: 0}: buf},
			}

			outputs, err := m.ExecuteWithContext("fs_main", ctx)
			if err != nil {
				t.Fatalf("Execute failed: %v", err)
			}

			ep := m.EntryPoints["fs_main"]
			for _, varID := range ep.InterfaceIDs {
				vi := m.Variables[varID]
				if vi != nil && vi.StorageClass == StorageClassOutput {
					color := Vec4ToFloat32(outputs[varID])
					if math.Abs(float64(color[0]-tt.want)) > 1e-5 {
						t.Errorf("double(%v) = %v, want %v", tt.input, color[0], tt.want)
					}
					return
				}
			}
			t.Fatal("output not found")
		})
	}
}

// buildSwitchSPIRV constructs SPIR-V for a shader with OpSwitch:
//
//	@group(0) @binding(0) var<uniform> params: Params;
//	struct Params { selector: u32 }
//	@fragment fn fs_main() -> @location(0) vec4<f32> {
//	    switch(params.selector) {
//	        case 1: return vec4(1, 0, 0, 1);  // Red
//	        case 2: return vec4(0, 1, 0, 1);  // Green
//	        default: return vec4(0, 0, 0, 1); // Black
//	    }
//	}
func buildSwitchSPIRV() []uint32 {
	inst := spirvInst
	str := spirvString
	f := math.Float32bits

	const (
		idVoid       = 1
		idFloat      = 2
		idVec4       = 3
		idUint       = 4
		idPtrV4Out   = 5
		idFuncTy     = 6
		idFunc       = 7
		idColorOut   = 8
		idStruct     = 9
		idPtrStruct  = 10
		idParamsVar  = 11
		idConst0U    = 12
		idPtrUint    = 13
		idChain      = 14
		idLoadSel    = 15
		idConst0F    = 16
		idConst1F    = 17
		idLblEntry   = 18
		idLblCase1   = 19
		idLblCase2   = 20
		idLblDefault = 21
		idLblMerge   = 22
		idRedVec     = 23
		idGreenVec   = 24
		idBlackVec   = 25
		idBound      = 26
	)

	nameWords := str("fs_main")
	epLen := uint16(3 + len(nameWords) + 1)
	epInst := append([]uint32{inst(epLen, OpEntryPoint), ExecutionModelFragment, idFunc}, nameWords...)
	epInst = append(epInst, idColorOut)

	words := make([]uint32, 0, 250)
	words = append(words,
		spirvMagic, 0x00010300, 0, idBound, 0,
		inst(2, OpCapability), 1,
		inst(3, OpMemoryModel), 0, 1,
	)
	words = append(words, epInst...)
	words = append(words,
		inst(3, OpExecutionMode), idFunc, 7,
		inst(4, OpDecorate), idColorOut, DecorationLocation, 0,
		inst(4, OpDecorate), idParamsVar, DecorationBinding, 0,
		inst(4, OpDecorate), idParamsVar, DecorationDescriptorSet, 0,
		inst(3, OpDecorate), idStruct, DecorationBlock,
		inst(5, OpMemberDecorate), idStruct, 0, DecorationOffset, 0,

		inst(2, OpTypeVoid), idVoid,
		inst(3, OpTypeFloat), idFloat, 32,
		inst(4, OpTypeInt), idUint, 32, 0,
		inst(4, OpTypeVector), idVec4, idFloat, 4,
		inst(3, OpTypeStruct), idStruct, idUint,
		inst(4, OpTypePointer), idPtrV4Out, StorageClassOutput, idVec4,
		inst(4, OpTypePointer), idPtrStruct, StorageClassUniform, idStruct,
		inst(4, OpTypePointer), idPtrUint, StorageClassUniform, idUint,
		inst(3, OpTypeFunction), idFuncTy, idVoid,

		inst(4, OpConstant), idUint, idConst0U, 0,
		inst(4, OpConstant), idFloat, idConst0F, f(0),
		inst(4, OpConstant), idFloat, idConst1F, f(1),

		inst(4, OpVariable), idPtrV4Out, idColorOut, StorageClassOutput,
		inst(4, OpVariable), idPtrStruct, idParamsVar, StorageClassUniform,

		inst(5, OpFunction), idVoid, idFunc, 0, idFuncTy,
		inst(2, OpLabel), idLblEntry,
		inst(5, OpAccessChain), idPtrUint, idChain, idParamsVar, idConst0U,
		inst(4, OpLoad), idUint, idLoadSel, idChain,
		inst(3, OpSelectionMerge), idLblMerge, 0,
		// OpSwitch: selector default lit0 target0 lit1 target1
		inst(7, OpSwitch), idLoadSel, idLblDefault, 1, idLblCase1, 2, idLblCase2,

		// Case 1: red
		inst(2, OpLabel), idLblCase1,
		inst(7, OpCompositeConstruct), idVec4, idRedVec, idConst1F, idConst0F, idConst0F, idConst1F,
		inst(3, OpStore), idColorOut, idRedVec,
		inst(2, OpBranch), idLblMerge,

		// Case 2: green
		inst(2, OpLabel), idLblCase2,
		inst(7, OpCompositeConstruct), idVec4, idGreenVec, idConst0F, idConst1F, idConst0F, idConst1F,
		inst(3, OpStore), idColorOut, idGreenVec,
		inst(2, OpBranch), idLblMerge,

		// Default: black
		inst(2, OpLabel), idLblDefault,
		inst(7, OpCompositeConstruct), idVec4, idBlackVec, idConst0F, idConst0F, idConst0F, idConst1F,
		inst(3, OpStore), idColorOut, idBlackVec,
		inst(2, OpBranch), idLblMerge,

		// Merge
		inst(2, OpLabel), idLblMerge,
		inst(1, OpReturn),
		inst(1, OpFunctionEnd),
	)
	return words
}

func TestSPIRVSwitch(t *testing.T) {
	tests := []struct {
		name     string
		selector uint32
		wantR    float32
		wantG    float32
	}{
		{"case_1_red", 1, 1.0, 0.0},
		{"case_2_green", 2, 0.0, 1.0},
		{"default_black", 99, 0.0, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			words := buildSwitchSPIRV()
			m, err := ParseModule(words)
			if err != nil {
				t.Fatalf("ParseModule failed: %v", err)
			}

			buf := make([]byte, 4)
			buf[0] = byte(tt.selector)
			buf[1] = byte(tt.selector >> 8)
			buf[2] = byte(tt.selector >> 16)
			buf[3] = byte(tt.selector >> 24)

			ctx := &ExecutionContext{
				Buffers: map[BindingKey][]byte{{Group: 0, Binding: 0}: buf},
			}

			outputs, err := m.ExecuteWithContext("fs_main", ctx)
			if err != nil {
				t.Fatalf("Execute failed: %v", err)
			}

			ep := m.EntryPoints["fs_main"]
			for _, varID := range ep.InterfaceIDs {
				vi := m.Variables[varID]
				if vi != nil && vi.StorageClass == StorageClassOutput {
					color := Vec4ToFloat32(outputs[varID])
					if math.Abs(float64(color[0]-tt.wantR)) > 0.01 {
						t.Errorf("R = %v, want %v", color[0], tt.wantR)
					}
					if math.Abs(float64(color[1]-tt.wantG)) > 0.01 {
						t.Errorf("G = %v, want %v", color[1], tt.wantG)
					}
					return
				}
			}
			t.Fatal("output not found")
		})
	}
}

func TestMaxIterationGuard(t *testing.T) {
	// Create a simple infinite loop to test the safety limit.
	inst := spirvInst
	str := spirvString

	const (
		idVoid   = 1
		idFuncTy = 2
		idFunc   = 3
		idLbl1   = 4
		idLbl2   = 5
		idBound  = 6
	)

	nameWords := str("fs_main")
	epLen := uint16(3 + len(nameWords))
	epInst := append([]uint32{inst(epLen, OpEntryPoint), ExecutionModelFragment, idFunc}, nameWords...)

	words := make([]uint32, 0, 40)
	words = append(words,
		spirvMagic, 0x00010300, 0, idBound, 0,
		inst(2, OpCapability), 1,
		inst(3, OpMemoryModel), 0, 1,
	)
	words = append(words, epInst...)
	words = append(words,
		inst(3, OpExecutionMode), idFunc, 7,
		inst(2, OpTypeVoid), idVoid,
		inst(3, OpTypeFunction), idFuncTy, idVoid,
		inst(5, OpFunction), idVoid, idFunc, 0, idFuncTy,
		inst(2, OpLabel), idLbl1,
		inst(2, OpBranch), idLbl2,
		inst(2, OpLabel), idLbl2,
		inst(2, OpBranch), idLbl1, // Infinite loop
		inst(1, OpFunctionEnd),
	)

	m, err := ParseModule(words)
	if err != nil {
		t.Fatalf("ParseModule failed: %v", err)
	}

	_, err = m.Execute("fs_main", nil)
	if err == nil {
		t.Fatal("expected error for infinite loop, got nil")
	}
	if err.Error() != "spirv: exceeded maximum iterations (100000)" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPhiNodeResolution(t *testing.T) {
	// The loop sum test already validates Phi nodes.
	// This is a direct unit test for currentBlockLabel.
	fn := &Function{
		Instructions: []Instruction{
			{Opcode: OpLabel, ResultID: 100},
			{Opcode: OpIAdd, ResultID: 101},
			{Opcode: OpLabel, ResultID: 200},
			{Opcode: OpFAdd, ResultID: 201},
		},
	}
	interp := &interpreter{fn: fn}

	// Instruction at index 1 is in block 100.
	if got := interp.currentBlockLabel(1); got != 100 {
		t.Errorf("currentBlockLabel(1) = %d, want 100", got)
	}
	// Instruction at index 3 is in block 200.
	if got := interp.currentBlockLabel(3); got != 200 {
		t.Errorf("currentBlockLabel(3) = %d, want 200", got)
	}
}
