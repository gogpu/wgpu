//go:build !(js && wasm)

package shader

import (
	"math"
	"testing"
)

// spirvInst encodes a SPIR-V instruction header word: (wordCount << 16) | opcode.
func spirvInst(wordCount, opcode uint16) uint32 {
	return uint32(wordCount)<<16 | uint32(opcode)
}

// spirvString encodes a string as null-terminated, word-aligned SPIR-V words.
func spirvString(s string) []uint32 {
	b := append([]byte(s), 0)
	for len(b)%4 != 0 {
		b = append(b, 0)
	}
	w := make([]uint32, len(b)/4)
	for i := range w {
		w[i] = uint32(b[i*4]) |
			uint32(b[i*4+1])<<8 |
			uint32(b[i*4+2])<<16 |
			uint32(b[i*4+3])<<24
	}
	return w
}

// buildTriangleVertexSPIRV constructs a minimal SPIR-V module equivalent to:
//
//	@vertex fn vs_main(@builtin(vertex_index) idx: u32) -> @builtin(position) vec4<f32> {
//	    var positions = array<vec2<f32>, 3>(
//	        vec2(0.0, 0.5), vec2(-0.5, -0.5), vec2(0.5, -0.5)
//	    );
//	    return vec4<f32>(positions[idx], 0.0, 1.0);
//	}
//
// The SPIR-V uses explicit IDs and follows the standard instruction encoding.
func buildTriangleVertexSPIRV() []uint32 {
	inst := spirvInst
	str := spirvString
	f := math.Float32bits

	// ID assignments.
	const (
		idVoid       = 1
		idFloat      = 2
		idUint       = 3
		idVec2       = 4
		idVec4       = 5
		idPtrVec4Out = 6  // pointer to vec4 (Output)
		idPtrUintIn  = 7  // pointer to uint (Input)
		idPosition   = 8  // @builtin(position) output variable
		idVertexIdx  = 9  // @builtin(vertex_index) input variable
		idFuncType   = 10 // void function type
		idFunc       = 11 // vs_main function
		idLabel1     = 12 // first label
		idConst0     = 13 // 0.0f
		idConst05    = 14 // 0.5f
		idConstN05   = 15 // -0.5f
		idConst1     = 16 // 1.0f
		idUint3      = 17 // constant uint 3
		idArrVec2    = 18 // array<vec2, 3>
		idPtrArrFunc = 19 // pointer to array (Function)
		idVec2_0     = 20 // vec2(0.0, 0.5)
		idVec2_1     = 21 // vec2(-0.5, -0.5)
		idVec2_2     = 22 // vec2(0.5, -0.5)
		idArrConst   = 23 // constant array
		idLocalArr   = 24 // local variable (function scope array)
		idLoadIdx    = 25 // loaded vertex_index
		idChainPtr   = 26 // access chain into array
		idLoadElem   = 27 // loaded vec2 element
		idExtractX   = 28 // extracted X component
		idExtractY   = 29 // extracted Y component
		idResult     = 30 // composed vec4 result
		idBound      = 31
	)

	// Build entry point instruction (variable-length due to name encoding).
	nameWords := str("vs_main")
	epLen := uint16(3 + len(nameWords) + 2)
	epInst := append([]uint32{inst(epLen, OpEntryPoint), ExecutionModelVertex, idFunc}, nameWords...)
	epInst = append(epInst, idPosition, idVertexIdx)

	// Header + preamble.
	words := make([]uint32, 0, 200) //nolint:mnd // estimated capacity for triangle SPIR-V
	words = append(words,
		spirvMagic, 0x00010300, 0, idBound, 0, // header
		inst(2, OpCapability), 1, // Shader capability
		inst(3, OpMemoryModel), 0, 1, // Logical GLSL450
	)
	words = append(words, epInst...)
	words = append(words,
		// Decorations.
		inst(4, OpDecorate), idPosition, DecorationBuiltIn, BuiltInPosition,
		inst(4, OpDecorate), idVertexIdx, DecorationBuiltIn, BuiltInVertexIndex,
		inst(4, OpDecorate), idArrVec2, DecorationArrayStride, 8,
		// Types.
		inst(2, OpTypeVoid), idVoid,
		inst(3, OpTypeFloat), idFloat, 32,
		inst(4, OpTypeInt), idUint, 32, 0,
		inst(4, OpTypeVector), idVec2, idFloat, 2,
		inst(4, OpTypeVector), idVec4, idFloat, 4,
		inst(4, OpTypePointer), idPtrVec4Out, StorageClassOutput, idVec4,
		inst(4, OpTypePointer), idPtrUintIn, StorageClassInput, idUint,
		inst(3, OpTypeFunction), idFuncType, idVoid,
		// Constants.
		inst(4, OpConstant), idFloat, idConst0, f(0.0),
		inst(4, OpConstant), idFloat, idConst05, f(0.5),
		inst(4, OpConstant), idFloat, idConstN05, f(-0.5),
		inst(4, OpConstant), idFloat, idConst1, f(1.0),
		inst(4, OpConstant), idUint, idUint3, 3,
		// Array and pointer types.
		inst(4, OpTypeArray), idArrVec2, idVec2, idUint3,
		inst(4, OpTypePointer), idPtrArrFunc, StorageClassFunction, idArrVec2,
		// Constant composites.
		inst(5, OpConstantComposite), idVec2, idVec2_0, idConst0, idConst05,
		inst(5, OpConstantComposite), idVec2, idVec2_1, idConstN05, idConstN05,
		inst(5, OpConstantComposite), idVec2, idVec2_2, idConst05, idConstN05,
		inst(6, OpConstantComposite), idArrVec2, idArrConst, idVec2_0, idVec2_1, idVec2_2,
		// Global variables.
		inst(4, OpVariable), idPtrVec4Out, idPosition, StorageClassOutput,
		inst(4, OpVariable), idPtrUintIn, idVertexIdx, StorageClassInput,
		// Function body.
		inst(5, OpFunction), idVoid, idFunc, 0, idFuncType,
		inst(2, OpLabel), idLabel1,
		inst(4, OpVariable), idPtrArrFunc, idLocalArr, StorageClassFunction,
		inst(3, OpStore), idLocalArr, idArrConst,
		inst(4, OpLoad), idUint, idLoadIdx, idVertexIdx,
		inst(5, OpAccessChain), idVec2, idChainPtr, idLocalArr, idLoadIdx,
		inst(4, OpLoad), idVec2, idLoadElem, idChainPtr,
		inst(5, OpCompositeExtract), idFloat, idExtractX, idLoadElem, 0,
		inst(5, OpCompositeExtract), idFloat, idExtractY, idLoadElem, 1,
		inst(7, OpCompositeConstruct), idVec4, idResult, idExtractX, idExtractY, idConst0, idConst1,
		inst(3, OpStore), idPosition, idResult,
		inst(1, OpReturn),
		inst(1, OpFunctionEnd),
	)
	return words
}

// buildTriangleFragmentSPIRV constructs SPIR-V for:
//
//	@fragment fn fs_main() -> @location(0) vec4<f32> {
//	    return vec4<f32>(1.0, 0.0, 0.0, 1.0);
//	}
func buildTriangleFragmentSPIRV() []uint32 {
	inst := spirvInst
	str := spirvString
	f := math.Float32bits

	const (
		idVoid       = 1
		idFloat      = 2
		idVec4       = 3
		idPtrVec4Out = 4
		idFuncType   = 5
		idFunc       = 6
		idLabel1     = 7
		idConst0     = 8
		idConst1     = 9
		idColorOut   = 10
		idResult     = 11
		idBound      = 12
	)

	// Build entry point instruction.
	nameWords := str("fs_main")
	epLen := uint16(3 + len(nameWords) + 1)
	epInst := append([]uint32{inst(epLen, OpEntryPoint), ExecutionModelFragment, idFunc}, nameWords...)
	epInst = append(epInst, idColorOut)

	words := make([]uint32, 0, 80) //nolint:mnd // estimated capacity for fragment SPIR-V
	words = append(words,
		spirvMagic, 0x00010300, 0, idBound, 0, // header
		inst(2, OpCapability), 1, // Shader
		inst(3, OpMemoryModel), 0, 1, // Logical GLSL450
	)
	words = append(words, epInst...)
	words = append(words,
		inst(3, OpExecutionMode), idFunc, 7, // OriginUpperLeft
		inst(4, OpDecorate), idColorOut, DecorationLocation, 0,
		// Types.
		inst(2, OpTypeVoid), idVoid,
		inst(3, OpTypeFloat), idFloat, 32,
		inst(4, OpTypeVector), idVec4, idFloat, 4,
		inst(4, OpTypePointer), idPtrVec4Out, StorageClassOutput, idVec4,
		inst(3, OpTypeFunction), idFuncType, idVoid,
		// Constants.
		inst(4, OpConstant), idFloat, idConst0, f(0.0),
		inst(4, OpConstant), idFloat, idConst1, f(1.0),
		// Global variable.
		inst(4, OpVariable), idPtrVec4Out, idColorOut, StorageClassOutput,
		// Function body.
		inst(5, OpFunction), idVoid, idFunc, 0, idFuncType,
		inst(2, OpLabel), idLabel1,
		inst(7, OpCompositeConstruct), idVec4, idResult, idConst1, idConst0, idConst0, idConst1,
		inst(3, OpStore), idColorOut, idResult,
		inst(1, OpReturn),
		inst(1, OpFunctionEnd),
	)
	return words
}

func TestSPIRVParseModule(t *testing.T) {
	words := buildTriangleVertexSPIRV()
	m, err := ParseModule(words)
	if err != nil {
		t.Fatalf("ParseModule failed: %v", err)
	}

	// Check entry point exists.
	ep, ok := m.EntryPoints["vs_main"]
	if !ok {
		t.Fatal("entry point vs_main not found")
	}
	if ep.ExecutionModel != ExecutionModelVertex {
		t.Errorf("execution model = %d, want %d", ep.ExecutionModel, ExecutionModelVertex)
	}

	// Check function body exists.
	_, ok = m.Functions["vs_main"]
	if !ok {
		t.Fatal("function body for vs_main not found")
	}

	// Verify decorations.
	var posVarID, idxVarID uint32
	for _, varID := range ep.InterfaceIDs {
		bi := m.GetBuiltIn(varID)
		switch bi {
		case BuiltInPosition:
			posVarID = varID
		case BuiltInVertexIndex:
			idxVarID = varID
		}
	}
	if posVarID == 0 {
		t.Fatal("position variable not found")
	}
	if idxVarID == 0 {
		t.Fatal("vertex_index variable not found")
	}
}

func TestSPIRVTriangleVertexShader(t *testing.T) {
	words := buildTriangleVertexSPIRV()
	m, err := ParseModule(words)
	if err != nil {
		t.Fatalf("ParseModule failed: %v", err)
	}

	ep := m.EntryPoints["vs_main"]

	// Find variable IDs.
	var posVarID, idxVarID uint32
	for _, varID := range ep.InterfaceIDs {
		bi := m.GetBuiltIn(varID)
		if bi == BuiltInPosition {
			posVarID = varID
		}
		if bi == BuiltInVertexIndex {
			idxVarID = varID
		}
	}

	// Expected positions from the triangle shader.
	// positions = [vec2(0.0, 0.5), vec2(-0.5, -0.5), vec2(0.5, -0.5)]
	// output = vec4(positions[idx].x, positions[idx].y, 0.0, 1.0)
	expected := [][4]float32{
		{0.0, 0.5, 0.0, 1.0},
		{-0.5, -0.5, 0.0, 1.0},
		{0.5, -0.5, 0.0, 1.0},
	}

	for idx, want := range expected {
		inputs := map[uint32]Value{
			idxVarID: uint32(idx),
		}

		outputs, err := m.Execute("vs_main", inputs)
		if err != nil {
			t.Fatalf("vertex %d: Execute failed: %v", idx, err)
		}

		posVal, ok := outputs[posVarID]
		if !ok {
			t.Fatalf("vertex %d: position output not found", idx)
		}

		pos := Vec4ToFloat32(posVal)
		for c := 0; c < 4; c++ {
			if math.Abs(float64(pos[c]-want[c])) > 1e-6 {
				t.Errorf("vertex %d: component %d = %f, want %f", idx, c, pos[c], want[c])
			}
		}
	}
}

func TestSPIRVTriangleFragmentShader(t *testing.T) {
	words := buildTriangleFragmentSPIRV()
	m, err := ParseModule(words)
	if err != nil {
		t.Fatalf("ParseModule failed: %v", err)
	}

	ep := m.EntryPoints["fs_main"]

	// Find color output variable.
	var colorVarID uint32
	for _, varID := range ep.InterfaceIDs {
		vi, ok := m.Variables[varID]
		if !ok {
			continue
		}
		if vi.StorageClass == StorageClassOutput {
			loc := m.GetLocation(varID)
			if loc == 0 {
				colorVarID = varID
				break
			}
		}
	}
	if colorVarID == 0 {
		t.Fatal("color output variable (location 0) not found")
	}

	outputs, err := m.Execute("fs_main", nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	colorVal, ok := outputs[colorVarID]
	if !ok {
		t.Fatal("color output not found in outputs")
	}

	color := Vec4ToFloat32(colorVal)
	want := [4]float32{1.0, 0.0, 0.0, 1.0} // Red
	for c := 0; c < 4; c++ {
		if math.Abs(float64(color[c]-want[c])) > 1e-6 {
			t.Errorf("color[%d] = %f, want %f", c, color[c], want[c])
		}
	}
}

func TestSPIRVBadMagic(t *testing.T) {
	words := []uint32{0xDEADBEEF, 0, 0, 10, 0}
	_, err := ParseModule(words)
	if err == nil {
		t.Error("expected error for bad magic number")
	}
}

func TestSPIRVTooShort(t *testing.T) {
	words := []uint32{spirvMagic, 0}
	_, err := ParseModule(words)
	if err == nil {
		t.Error("expected error for short module")
	}
}

func TestSPIRVEmptyEntryPoint(t *testing.T) {
	words := buildTriangleVertexSPIRV()
	m, err := ParseModule(words)
	if err != nil {
		t.Fatalf("ParseModule failed: %v", err)
	}

	_, err = m.Execute("nonexistent", nil)
	if err == nil {
		t.Error("expected error for nonexistent entry point")
	}
}

func TestSPIRVAccessChainOutOfBounds(t *testing.T) {
	words := buildTriangleVertexSPIRV()
	m, err := ParseModule(words)
	if err != nil {
		t.Fatalf("ParseModule failed: %v", err)
	}

	ep := m.EntryPoints["vs_main"]
	var idxVarID uint32
	for _, varID := range ep.InterfaceIDs {
		if m.GetBuiltIn(varID) == BuiltInVertexIndex {
			idxVarID = varID
		}
	}

	// Index 10 is out of bounds for a 3-element array.
	// The interpreter should not crash; it returns a zero element.
	inputs := map[uint32]Value{
		idxVarID: Uint32(10),
	}
	_, err = m.Execute("vs_main", inputs)
	// Should not error — out-of-bounds returns zero.
	if err != nil {
		t.Fatalf("Execute with OOB index failed: %v", err)
	}
}

func TestSPIRVConvertUToF(t *testing.T) {
	// Test the convertToFloat helper directly.
	tests := []struct {
		name string
		val  Value
		want float32
	}{
		{"uint32_0", Uint32(0), 0},
		{"uint32_42", Uint32(42), 42},
		{"int32_neg", Int32(-5), -5},
		{"float32", Float32(3.14), 3.14},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertToFloat(tt.val)
			f, ok := result.(Float32)
			if !ok {
				t.Fatalf("convertToFloat returned %T, want Float32", result)
			}
			if math.Abs(float64(f-tt.want)) > 1e-5 {
				t.Errorf("convertToFloat(%v) = %f, want %f", tt.val, f, tt.want)
			}
		})
	}
}

func TestSPIRVCompositeConstruct(t *testing.T) {
	// Test compositeConstruct via a simple module execution.
	words := buildTriangleVertexSPIRV()
	m, err := ParseModule(words)
	if err != nil {
		t.Fatalf("ParseModule failed: %v", err)
	}

	// Vertex 0 constructs vec4(0.0, 0.5, 0.0, 1.0).
	ep := m.EntryPoints["vs_main"]
	var idxVarID, posVarID uint32
	for _, varID := range ep.InterfaceIDs {
		switch m.GetBuiltIn(varID) {
		case BuiltInVertexIndex:
			idxVarID = varID
		case BuiltInPosition:
			posVarID = varID
		}
	}

	inputs := map[uint32]Value{idxVarID: Uint32(0)}
	outputs, err := m.Execute("vs_main", inputs)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	pos := Vec4ToFloat32(outputs[posVarID])
	if pos[3] != 1.0 {
		t.Errorf("W component = %f, want 1.0 (CompositeConstruct should set it)", pos[3])
	}
}

func TestDecodeString(t *testing.T) {
	tests := []struct {
		name      string
		words     []uint32
		wantStr   string
		wantWords uint32
	}{
		{
			name:      "main",
			words:     []uint32{0x6E69616D, 0x00000000}, // "main\0\0\0\0"
			wantStr:   "main",
			wantWords: 2,
		},
		{
			name:      "vs",
			words:     []uint32{0x00007376}, // "vs\0\0"
			wantStr:   "vs",
			wantWords: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, n := decodeString(tt.words)
			if s != tt.wantStr {
				t.Errorf("decodeString = %q, want %q", s, tt.wantStr)
			}
			if n != tt.wantWords {
				t.Errorf("words consumed = %d, want %d", n, tt.wantWords)
			}
		})
	}
}

func TestVec4ToFloat32(t *testing.T) {
	tests := []struct {
		name string
		val  Value
		want [4]float32
	}{
		{"vec4", Vec4{1, 2, 3, 4}, [4]float32{1, 2, 3, 4}},
		{"vec3", Vec3{1, 2, 3}, [4]float32{1, 2, 3, 0}},
		{"vec2", Vec2{1, 2}, [4]float32{1, 2, 0, 0}},
		{"float", Float32(5), [4]float32{5, 0, 0, 0}},
		{"nil", nil, [4]float32{0, 0, 0, 0}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Vec4ToFloat32(tt.val)
			if got != tt.want {
				t.Errorf("Vec4ToFloat32(%v) = %v, want %v", tt.val, got, tt.want)
			}
		})
	}
}

func TestIndexComposite(t *testing.T) {
	arr := Array{Float32(10), Float32(20), Float32(30)}
	v := indexComposite(arr, 1)
	f, ok := v.(Float32)
	if !ok || f != 20 {
		t.Errorf("indexComposite(arr, 1) = %v, want Float32(20)", v)
	}

	vec := Vec4{1, 2, 3, 4}
	v = indexComposite(vec, 2)
	f, ok = v.(Float32)
	if !ok || f != 3 {
		t.Errorf("indexComposite(vec4, 2) = %v, want Float32(3)", v)
	}
}

func TestFloatBinOp(t *testing.T) {
	add := func(a, b float32) float32 { return a + b }

	result := floatBinOp(Float32(3), Float32(4), add)
	if f, ok := result.(Float32); !ok || f != 7 {
		t.Errorf("floatBinOp scalar = %v, want 7", result)
	}

	result = floatBinOp(Vec2{1, 2}, Vec2{3, 4}, add)
	if v, ok := result.(Vec2); !ok || v != (Vec2{4, 6}) {
		t.Errorf("floatBinOp vec2 = %v, want [4, 6]", result)
	}
}

func TestIntBinOp(t *testing.T) {
	add := func(a, b uint32) uint32 { return a + b }

	result := intBinOp(Uint32(3), Uint32(4), add)
	if u, ok := result.(Uint32); !ok || u != 7 {
		t.Errorf("intBinOp = %v, want 7", result)
	}
}

func BenchmarkSPIRVVertexShaderExecution(b *testing.B) {
	words := buildTriangleVertexSPIRV()
	m, err := ParseModule(words)
	if err != nil {
		b.Fatalf("ParseModule failed: %v", err)
	}

	ep := m.EntryPoints["vs_main"]
	var idxVarID uint32
	for _, varID := range ep.InterfaceIDs {
		if m.GetBuiltIn(varID) == BuiltInVertexIndex {
			idxVarID = varID
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		inputs := map[uint32]Value{
			idxVarID: uint32(i % 3),
		}
		_, _ = m.Execute("vs_main", inputs)
	}
}

func BenchmarkSPIRVFragmentShaderExecution(b *testing.B) {
	words := buildTriangleFragmentSPIRV()
	m, err := ParseModule(words)
	if err != nil {
		b.Fatalf("ParseModule failed: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = m.Execute("fs_main", nil)
	}
}
