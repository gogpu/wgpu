//go:build !(js && wasm)

package shader

import (
	"math"
	"testing"
)

// =============================================================================
// Phase 4 Tests: GLSL.std.450 Math Intrinsics
// =============================================================================

// buildGLSLExtSPIRV constructs a minimal SPIR-V module that imports GLSL.std.450,
// calls a single ExtInst with two float inputs, and outputs the result as a vec4.
// This is used for testing individual GLSL intrinsics via the SPIR-V interpreter.
//
// Shader equivalent:
//
//	@group(0) @binding(0) var<uniform> params: Params;
//	struct Params { a: f32, b: f32, c: f32 }
//	@fragment fn fs_main() -> @location(0) vec4<f32> {
//	    let r = glsl_op(params.a, params.b);  // or unary(params.a)
//	    return vec4(r, 0, 0, 1);
//	}
func buildGLSLUnaryExtSPIRV(glslInstNum uint32) []uint32 {
	inst := spirvInst
	str := spirvString

	const (
		idVoid      = 1
		idFloat     = 2
		idVec4      = 3
		idPtrV4Out  = 4
		idFuncTy    = 5
		idFunc      = 6
		idLabel     = 7
		idColorOut  = 8
		idStruct    = 9
		idPtrStruct = 10
		idParamsVar = 11
		idUint      = 12
		idConst0    = 13
		idPtrFloat  = 14
		idChainA    = 15
		idLoadA     = 16
		idExtResult = 17
		idConst0F   = 18
		idConst1F   = 19
		idResult    = 20
		idGLSLSet   = 21
		idBound     = 22
	)

	nameWords := str("fs_main")
	epLen := uint16(3 + len(nameWords) + 1)
	epInst := append([]uint32{inst(epLen, OpEntryPoint), ExecutionModelFragment, idFunc}, nameWords...)
	epInst = append(epInst, idColorOut)

	glslName := str("GLSL.std.450")
	importLen := uint16(2 + len(glslName))

	words := make([]uint32, 0, 180)
	words = append(words,
		spirvMagic, 0x00010300, 0, idBound, 0,
		inst(2, OpCapability), 1,
	)
	// OpExtInstImport
	importInst := append([]uint32{inst(importLen, OpExtInstImport), idGLSLSet}, glslName...)
	words = append(words, importInst...)
	words = append(words, inst(3, OpMemoryModel), 0, 1)
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
		inst(3, OpTypeStruct), idStruct, idFloat,
		inst(4, OpTypePointer), idPtrV4Out, StorageClassOutput, idVec4,
		inst(4, OpTypePointer), idPtrStruct, StorageClassUniform, idStruct,
		inst(4, OpTypePointer), idPtrFloat, StorageClassUniform, idFloat,
		inst(3, OpTypeFunction), idFuncTy, idVoid,

		inst(4, OpConstant), idUint, idConst0, 0,
		inst(4, OpConstant), idFloat, idConst0F, math.Float32bits(0),
		inst(4, OpConstant), idFloat, idConst1F, math.Float32bits(1),

		inst(4, OpVariable), idPtrV4Out, idColorOut, StorageClassOutput,
		inst(4, OpVariable), idPtrStruct, idParamsVar, StorageClassUniform,

		inst(5, OpFunction), idVoid, idFunc, 0, idFuncTy,
		inst(2, OpLabel), idLabel,
		inst(5, OpAccessChain), idPtrFloat, idChainA, idParamsVar, idConst0,
		inst(4, OpLoad), idFloat, idLoadA, idChainA,

		// OpExtInst: type result setID instNum operand
		inst(6, OpExtInst), idFloat, idExtResult, idGLSLSet, glslInstNum, idLoadA,

		inst(7, OpCompositeConstruct), idVec4, idResult, idExtResult, idConst0F, idConst0F, idConst1F,
		inst(3, OpStore), idColorOut, idResult,
		inst(1, OpReturn),
		inst(1, OpFunctionEnd),
	)
	return words
}

func buildGLSLBinaryExtSPIRV(glslInstNum uint32) []uint32 {
	inst := spirvInst
	str := spirvString

	const (
		idVoid      = 1
		idFloat     = 2
		idVec4      = 3
		idPtrV4Out  = 4
		idFuncTy    = 5
		idFunc      = 6
		idLabel     = 7
		idColorOut  = 8
		idStruct    = 9
		idPtrStruct = 10
		idParamsVar = 11
		idUint      = 12
		idConst0    = 13
		idConst1    = 14
		idPtrFloat  = 15
		idChainA    = 16
		idChainB    = 17
		idLoadA     = 18
		idLoadB     = 19
		idExtResult = 20
		idConst0F   = 21
		idConst1F   = 22
		idResult    = 23
		idGLSLSet   = 24
		idBound     = 25
	)

	nameWords := str("fs_main")
	epLen := uint16(3 + len(nameWords) + 1)
	epInst := append([]uint32{inst(epLen, OpEntryPoint), ExecutionModelFragment, idFunc}, nameWords...)
	epInst = append(epInst, idColorOut)

	glslName := str("GLSL.std.450")
	importLen := uint16(2 + len(glslName))

	words := make([]uint32, 0, 200)
	words = append(words,
		spirvMagic, 0x00010300, 0, idBound, 0,
		inst(2, OpCapability), 1,
	)
	importInst := append([]uint32{inst(importLen, OpExtInstImport), idGLSLSet}, glslName...)
	words = append(words, importInst...)
	words = append(words, inst(3, OpMemoryModel), 0, 1)
	words = append(words, epInst...)
	words = append(words,
		inst(3, OpExecutionMode), idFunc, 7,
		inst(4, OpDecorate), idColorOut, DecorationLocation, 0,
		inst(4, OpDecorate), idParamsVar, DecorationBinding, 0,
		inst(4, OpDecorate), idParamsVar, DecorationDescriptorSet, 0,
		inst(3, OpDecorate), idStruct, DecorationBlock,
		inst(5, OpMemberDecorate), idStruct, 0, DecorationOffset, 0,
		inst(5, OpMemberDecorate), idStruct, 1, DecorationOffset, 4,

		inst(2, OpTypeVoid), idVoid,
		inst(3, OpTypeFloat), idFloat, 32,
		inst(4, OpTypeInt), idUint, 32, 0,
		inst(4, OpTypeVector), idVec4, idFloat, 4,
		inst(4, OpTypeStruct), idStruct, idFloat, idFloat,
		inst(4, OpTypePointer), idPtrV4Out, StorageClassOutput, idVec4,
		inst(4, OpTypePointer), idPtrStruct, StorageClassUniform, idStruct,
		inst(4, OpTypePointer), idPtrFloat, StorageClassUniform, idFloat,
		inst(3, OpTypeFunction), idFuncTy, idVoid,

		inst(4, OpConstant), idUint, idConst0, 0,
		inst(4, OpConstant), idUint, idConst1, 1,
		inst(4, OpConstant), idFloat, idConst0F, math.Float32bits(0),
		inst(4, OpConstant), idFloat, idConst1F, math.Float32bits(1),

		inst(4, OpVariable), idPtrV4Out, idColorOut, StorageClassOutput,
		inst(4, OpVariable), idPtrStruct, idParamsVar, StorageClassUniform,

		inst(5, OpFunction), idVoid, idFunc, 0, idFuncTy,
		inst(2, OpLabel), idLabel,
		inst(5, OpAccessChain), idPtrFloat, idChainA, idParamsVar, idConst0,
		inst(4, OpLoad), idFloat, idLoadA, idChainA,
		inst(5, OpAccessChain), idPtrFloat, idChainB, idParamsVar, idConst1,
		inst(4, OpLoad), idFloat, idLoadB, idChainB,

		// OpExtInst: type result setID instNum operand1 operand2
		inst(7, OpExtInst), idFloat, idExtResult, idGLSLSet, glslInstNum, idLoadA, idLoadB,

		inst(7, OpCompositeConstruct), idVec4, idResult, idExtResult, idConst0F, idConst0F, idConst1F,
		inst(3, OpStore), idColorOut, idResult,
		inst(1, OpReturn),
		inst(1, OpFunctionEnd),
	)
	return words
}

// runGLSLUnary builds and executes a SPIR-V module with a unary GLSL intrinsic.
func runGLSLUnary(t *testing.T, instNum uint32, inputA float32) float32 {
	t.Helper()
	words := buildGLSLUnaryExtSPIRV(instNum)
	m, err := ParseModule(words)
	if err != nil {
		t.Fatalf("ParseModule failed: %v", err)
	}

	buf := make([]byte, 4)
	putFloat32LE(buf, inputA)
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
			return color[0] // Result is in the R channel.
		}
	}
	t.Fatal("output not found")
	return 0
}

// runGLSLBinary builds and executes a SPIR-V module with a binary GLSL intrinsic.
func runGLSLBinary(t *testing.T, instNum uint32, inputA, inputB float32) float32 {
	t.Helper()
	words := buildGLSLBinaryExtSPIRV(instNum)
	m, err := ParseModule(words)
	if err != nil {
		t.Fatalf("ParseModule failed: %v", err)
	}

	buf := make([]byte, 8)
	putFloat32LE(buf[0:], inputA)
	putFloat32LE(buf[4:], inputB)
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
			return color[0]
		}
	}
	t.Fatal("output not found")
	return 0
}

func TestGLSLUnaryIntrinsics(t *testing.T) {
	tests := []struct {
		name    string
		instNum uint32
		input   float32
		want    float32
		eps     float64
	}{
		{"round_up", GLSLRound, 2.7, 3.0, 1e-5},
		{"round_down", GLSLRound, 2.3, 2.0, 1e-5},
		{"floor", GLSLFloor, 2.7, 2.0, 1e-5},
		{"floor_neg", GLSLFloor, -0.5, -1.0, 1e-5},
		{"ceil", GLSLCeil, 2.3, 3.0, 1e-5},
		{"fract", GLSLFract, 2.75, 0.75, 1e-5},
		{"fabs", GLSLFAbs, -3.5, 3.5, 1e-5},
		{"fsign_pos", GLSLFSign, 5.0, 1.0, 1e-5},
		{"fsign_neg", GLSLFSign, -5.0, -1.0, 1e-5},
		{"fsign_zero", GLSLFSign, 0.0, 0.0, 1e-5},
		{"sqrt", GLSLSqrt, 9.0, 3.0, 1e-5},
		{"inversesqrt", GLSLInverseSqrt, 4.0, 0.5, 1e-5},
		{"sin_0", GLSLSin, 0.0, 0.0, 1e-5},
		{"cos_0", GLSLCos, 0.0, 1.0, 1e-5},
		{"exp_0", GLSLExp, 0.0, 1.0, 1e-5},
		{"log_1", GLSLLog, 1.0, 0.0, 1e-5},
		{"exp2_3", GLSLExp2, 3.0, 8.0, 1e-4},
		{"log2_8", GLSLLog2, 8.0, 3.0, 1e-4},
		{"trunc_pos", GLSLTrunc, 2.9, 2.0, 1e-5},
		{"trunc_neg", GLSLTrunc, -2.9, -2.0, 1e-5},
		{"atan_0", GLSLAtan, 0.0, 0.0, 1e-5},
		{"asin_0", GLSLAsin, 0.0, 0.0, 1e-5},
		{"acos_1", GLSLAcos, 1.0, 0.0, 1e-5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := runGLSLUnary(t, tt.instNum, tt.input)
			if math.Abs(float64(got)-float64(tt.want)) > tt.eps {
				t.Errorf("%s(%v) = %v, want %v", tt.name, tt.input, got, tt.want)
			}
		})
	}
}

func TestGLSLBinaryIntrinsics(t *testing.T) {
	tests := []struct {
		name    string
		instNum uint32
		a, b    float32
		want    float32
		eps     float64
	}{
		{"pow", GLSLPow, 2.0, 3.0, 8.0, 1e-4},
		{"atan2", GLSLAtan2, 1.0, 1.0, float32(math.Pi / 4), 1e-5},
		{"fmin", GLSLFMin, 3.0, 5.0, 3.0, 1e-6},
		{"fmax", GLSLFMax, 3.0, 5.0, 5.0, 1e-6},
		{"step_below", GLSLStep, 0.5, 0.3, 0.0, 1e-6},
		{"step_above", GLSLStep, 0.5, 0.7, 1.0, 1e-6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := runGLSLBinary(t, tt.instNum, tt.a, tt.b)
			if math.Abs(float64(got)-float64(tt.want)) > tt.eps {
				t.Errorf("%s(%v, %v) = %v, want %v", tt.name, tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestVectorLength(t *testing.T) {
	tests := []struct {
		name string
		val  Value
		want float32
	}{
		{"scalar", Float32(3), 3},
		{"vec2", Vec2{3, 4}, 5},
		{"vec3", Vec3{1, 2, 2}, 3},
		{"vec4", Vec4{1, 0, 0, 0}, 1},
		{"zero", Vec3{0, 0, 0}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := vectorLength(tt.val)
			if math.Abs(float64(got-tt.want)) > 1e-5 {
				t.Errorf("vectorLength = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNormalizeVector(t *testing.T) {
	// Normalize (3, 4, 0) -> (0.6, 0.8, 0)
	got := normalizeVector(Vec3{3, 4, 0})
	gv, ok := got.(Vec3)
	if !ok {
		t.Fatalf("normalizeVector returned %T, want Vec3", got)
	}
	if math.Abs(float64(gv[0]-0.6)) > 1e-5 || math.Abs(float64(gv[1]-0.8)) > 1e-5 {
		t.Errorf("normalize(3,4,0) = %v, want (0.6, 0.8, 0)", gv)
	}

	// Zero-length vector should return zero vector unchanged.
	got = normalizeVector(Vec3{0, 0, 0})
	gv, _ = got.(Vec3)
	if gv != (Vec3{0, 0, 0}) {
		t.Errorf("normalize(0,0,0) = %v, want (0,0,0)", gv)
	}
}

func TestCrossProduct(t *testing.T) {
	tests := []struct {
		name string
		a, b Vec3
		want Vec3
	}{
		{"x_cross_y", Vec3{1, 0, 0}, Vec3{0, 1, 0}, Vec3{0, 0, 1}},
		{"y_cross_x", Vec3{0, 1, 0}, Vec3{1, 0, 0}, Vec3{0, 0, -1}},
		{"parallel", Vec3{1, 0, 0}, Vec3{2, 0, 0}, Vec3{0, 0, 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := crossProduct(tt.a, tt.b)
			gv, ok := got.(Vec3)
			if !ok {
				t.Fatalf("crossProduct returned %T, want Vec3", got)
			}
			for i := 0; i < 3; i++ {
				if math.Abs(float64(gv[i]-tt.want[i])) > 1e-5 {
					t.Errorf("cross[%d] = %v, want %v", i, gv[i], tt.want[i])
				}
			}
		})
	}
}

func TestReflectVector(t *testing.T) {
	// Reflect (1, -1, 0) around normal (0, 1, 0) -> (1, 1, 0)
	got := reflectVector(Vec3{1, -1, 0}, Vec3{0, 1, 0})
	gv, ok := got.(Vec3)
	if !ok {
		t.Fatalf("reflect returned %T, want Vec3", got)
	}
	want := Vec3{1, 1, 0}
	for i := 0; i < 3; i++ {
		if math.Abs(float64(gv[i]-want[i])) > 1e-5 {
			t.Errorf("reflect[%d] = %v, want %v", i, gv[i], want[i])
		}
	}
}

func TestGLSLIntOps(t *testing.T) {
	// Test SAbs and SSign directly via the executeGLSLExtInst.
	m := &Module{
		Types:          map[uint32]*TypeInfo{},
		Constants:      map[uint32]Value{},
		ExtInstImports: map[uint32]string{1: "GLSL.std.450"},
	}
	interp := &interpreter{
		module: m,
		values: map[uint32]Value{
			10: Int32(-42),
			11: Int32(0),
			12: Int32(7),
		},
	}

	// SAbs(-42) = 42
	got := interp.executeGLSLExtInst(GLSLSAbs, []uint32{10})
	if v, ok := got.(Int32); !ok || v != 42 {
		t.Errorf("SAbs(-42) = %v, want 42", got)
	}

	// SSign(-42) = -1
	got = interp.executeGLSLExtInst(GLSLSSign, []uint32{10})
	if v, ok := got.(Int32); !ok || v != -1 {
		t.Errorf("SSign(-42) = %v, want -1", got)
	}

	// SSign(0) = 0
	got = interp.executeGLSLExtInst(GLSLSSign, []uint32{11})
	if v, ok := got.(Int32); !ok || v != 0 {
		t.Errorf("SSign(0) = %v, want 0", got)
	}

	// SSign(7) = 1
	got = interp.executeGLSLExtInst(GLSLSSign, []uint32{12})
	if v, ok := got.(Int32); !ok || v != 1 {
		t.Errorf("SSign(7) = %v, want 1", got)
	}
}
