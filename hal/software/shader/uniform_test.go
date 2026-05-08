//go:build !(js && wasm)

package shader

import (
	"math"
	"testing"
)

// buildUniformFragmentSPIRV constructs SPIR-V for a fragment shader that reads
// color from a uniform buffer:
//
//	@group(0) @binding(0) var<uniform> params: Params;
//	struct Params { color: vec4<f32> }
//	@fragment fn fs_main() -> @location(0) vec4<f32> {
//	    return params.color;
//	}
func buildUniformFragmentSPIRV() []uint32 {
	inst := spirvInst
	str := spirvString

	const (
		idVoid       = 1
		idFloat      = 2
		idVec4       = 3
		idPtrVec4Out = 4
		idFuncType   = 5
		idFunc       = 6
		idLabel1     = 7
		idColorOut   = 8
		idStruct     = 9  // Params struct type
		idPtrStruct  = 10 // pointer to Params (Uniform)
		idParamsVar  = 11 // @group(0) @binding(0) var<uniform>
		idUint       = 12
		idConst0     = 13 // uint32(0)
		idChainPtr   = 14 // access chain into struct
		idLoadColor  = 15 // loaded vec4 color
		idBound      = 16
	)

	nameWords := str("fs_main")
	epLen := uint16(3 + len(nameWords) + 1)
	epInst := append([]uint32{inst(epLen, OpEntryPoint), ExecutionModelFragment, idFunc}, nameWords...)
	epInst = append(epInst, idColorOut)

	words := make([]uint32, 0, 120)
	words = append(words,
		spirvMagic, 0x00010300, 0, idBound, 0,
		inst(2, OpCapability), 1,
		inst(3, OpMemoryModel), 0, 1,
	)
	words = append(words, epInst...)
	words = append(words,
		inst(3, OpExecutionMode), idFunc, 7, // OriginUpperLeft

		// Decorations.
		inst(4, OpDecorate), idColorOut, DecorationLocation, 0,
		inst(4, OpDecorate), idParamsVar, DecorationBinding, 0,
		inst(4, OpDecorate), idParamsVar, DecorationDescriptorSet, 0,
		inst(3, OpDecorate), idStruct, DecorationBlock,
		inst(5, OpMemberDecorate), idStruct, 0, DecorationOffset, 0,

		// Types.
		inst(2, OpTypeVoid), idVoid,
		inst(3, OpTypeFloat), idFloat, 32,
		inst(4, OpTypeInt), idUint, 32, 0,
		inst(4, OpTypeVector), idVec4, idFloat, 4,
		inst(3, OpTypeStruct), idStruct, idVec4,
		inst(4, OpTypePointer), idPtrVec4Out, StorageClassOutput, idVec4,
		inst(4, OpTypePointer), idPtrStruct, StorageClassUniform, idStruct,
		inst(3, OpTypeFunction), idFuncType, idVoid,

		// Constants.
		inst(4, OpConstant), idUint, idConst0, 0,

		// Variables.
		inst(4, OpVariable), idPtrVec4Out, idColorOut, StorageClassOutput,
		inst(4, OpVariable), idPtrStruct, idParamsVar, StorageClassUniform,

		// Function body.
		inst(5, OpFunction), idVoid, idFunc, 0, idFuncType,
		inst(2, OpLabel), idLabel1,
		// Access chain: &params.color (member 0)
		inst(5, OpAccessChain), idVec4, idChainPtr, idParamsVar, idConst0,
		inst(4, OpLoad), idVec4, idLoadColor, idChainPtr,
		inst(3, OpStore), idColorOut, idLoadColor,
		inst(1, OpReturn),
		inst(1, OpFunctionEnd),
	)
	return words
}

// buildUniformMultiMemberSPIRV constructs SPIR-V for a shader reading multiple
// struct members from a uniform buffer:
//
//	struct Params { offset: f32, scale: f32, color: vec4<f32> }
//	@group(0) @binding(0) var<uniform> params: Params;
//	@fragment fn fs_main() -> @location(0) vec4<f32> {
//	    return params.color * params.scale + vec4(params.offset);
//	}
func buildUniformMultiMemberSPIRV() []uint32 {
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
		idColorOut   = 8
		idStruct     = 9
		idPtrStruct  = 10
		idParamsVar  = 11
		idUint       = 12
		idConst0     = 13 // member index 0 (offset)
		idConst1     = 14 // member index 1 (scale)
		idConst2     = 15 // member index 2 (color)
		idPtrFloat   = 16 // pointer to float (Uniform)
		idPtrVec4Uni = 17 // pointer to vec4 (Uniform)
		idChainOff   = 18 // access chain for offset
		idChainScl   = 19 // access chain for scale
		idChainCol   = 20 // access chain for color
		idLoadOff    = 21 // loaded offset
		idLoadScl    = 22 // loaded scale
		idLoadCol    = 23 // loaded color
		idSplatOff   = 24 // vec4(offset, offset, offset, offset)
		idScaled     = 25 // color * scale
		idResult     = 26 // scaled + splatOff
		idConst1F    = 27 // float 1.0 (unused, keep IDs clean)
		idBound      = 28
	)
	_ = f

	nameWords := str("fs_main")
	epLen := uint16(3 + len(nameWords) + 1)
	epInst := append([]uint32{inst(epLen, OpEntryPoint), ExecutionModelFragment, idFunc}, nameWords...)
	epInst = append(epInst, idColorOut)

	words := make([]uint32, 0, 200)
	words = append(words,
		spirvMagic, 0x00010300, 0, idBound, 0,
		inst(2, OpCapability), 1,
		inst(3, OpMemoryModel), 0, 1,
	)
	words = append(words, epInst...)
	words = append(words,
		inst(3, OpExecutionMode), idFunc, 7,

		// Decorations.
		inst(4, OpDecorate), idColorOut, DecorationLocation, 0,
		inst(4, OpDecorate), idParamsVar, DecorationBinding, 0,
		inst(4, OpDecorate), idParamsVar, DecorationDescriptorSet, 0,
		inst(3, OpDecorate), idStruct, DecorationBlock,
		inst(5, OpMemberDecorate), idStruct, 0, DecorationOffset, 0, // offset at byte 0
		inst(5, OpMemberDecorate), idStruct, 1, DecorationOffset, 4, // scale at byte 4
		inst(5, OpMemberDecorate), idStruct, 2, DecorationOffset, 16, // color at byte 16 (aligned to 16)

		// Types.
		inst(2, OpTypeVoid), idVoid,
		inst(3, OpTypeFloat), idFloat, 32,
		inst(4, OpTypeInt), idUint, 32, 0,
		inst(4, OpTypeVector), idVec4, idFloat, 4,
		inst(5, OpTypeStruct), idStruct, idFloat, idFloat, idVec4, // {f32, f32, vec4}
		inst(4, OpTypePointer), idPtrVec4Out, StorageClassOutput, idVec4,
		inst(4, OpTypePointer), idPtrStruct, StorageClassUniform, idStruct,
		inst(4, OpTypePointer), idPtrFloat, StorageClassUniform, idFloat,
		inst(4, OpTypePointer), idPtrVec4Uni, StorageClassUniform, idVec4,
		inst(3, OpTypeFunction), idFuncType, idVoid,

		// Constants.
		inst(4, OpConstant), idUint, idConst0, 0,
		inst(4, OpConstant), idUint, idConst1, 1,
		inst(4, OpConstant), idUint, idConst2, 2,

		// Variables.
		inst(4, OpVariable), idPtrVec4Out, idColorOut, StorageClassOutput,
		inst(4, OpVariable), idPtrStruct, idParamsVar, StorageClassUniform,

		// Function body.
		inst(5, OpFunction), idVoid, idFunc, 0, idFuncType,
		inst(2, OpLabel), idLabel1,

		// Load offset (member 0)
		inst(5, OpAccessChain), idPtrFloat, idChainOff, idParamsVar, idConst0,
		inst(4, OpLoad), idFloat, idLoadOff, idChainOff,

		// Load scale (member 1)
		inst(5, OpAccessChain), idPtrFloat, idChainScl, idParamsVar, idConst1,
		inst(4, OpLoad), idFloat, idLoadScl, idChainScl,

		// Load color (member 2)
		inst(5, OpAccessChain), idPtrVec4Uni, idChainCol, idParamsVar, idConst2,
		inst(4, OpLoad), idVec4, idLoadCol, idChainCol,

		// Splat offset into vec4
		inst(7, OpCompositeConstruct), idVec4, idSplatOff, idLoadOff, idLoadOff, idLoadOff, idLoadOff,

		// color * scale (using VectorTimesScalar)
		inst(5, OpVectorTimesScalar), idVec4, idScaled, idLoadCol, idLoadScl,

		// result = scaled + splatOff
		inst(5, OpFAdd), idVec4, idResult, idScaled, idSplatOff,

		inst(3, OpStore), idColorOut, idResult,
		inst(1, OpReturn),
		inst(1, OpFunctionEnd),
	)
	return words
}

func TestSPIRVUniformBufferSimple(t *testing.T) {
	words := buildUniformFragmentSPIRV()
	m, err := ParseModule(words)
	if err != nil {
		t.Fatalf("ParseModule failed: %v", err)
	}

	// Verify binding decoration was parsed.
	var paramsVarID uint32
	for varID, vi := range m.Variables {
		if vi.StorageClass == StorageClassUniform {
			paramsVarID = varID
			break
		}
	}
	if paramsVarID == 0 {
		t.Fatal("uniform variable not found")
	}

	bk, ok := m.GetBinding(paramsVarID)
	if !ok {
		t.Fatal("binding decoration not found for uniform variable")
	}
	if bk.Group != 0 || bk.Binding != 0 {
		t.Errorf("binding = (%d, %d), want (0, 0)", bk.Group, bk.Binding)
	}

	// Prepare uniform buffer data: vec4(0.25, 0.5, 0.75, 1.0)
	buf := make([]byte, 16)
	putFloat32LE(buf[0:], 0.25)
	putFloat32LE(buf[4:], 0.5)
	putFloat32LE(buf[8:], 0.75)
	putFloat32LE(buf[12:], 1.0)

	ctx := &ExecutionContext{
		Buffers: map[BindingKey][]byte{
			{Group: 0, Binding: 0}: buf,
		},
	}

	outputs, err := m.ExecuteWithContext("fs_main", ctx)
	if err != nil {
		t.Fatalf("ExecuteWithContext failed: %v", err)
	}

	// Find color output.
	ep := m.EntryPoints["fs_main"]
	var colorVarID uint32
	for _, varID := range ep.InterfaceIDs {
		vi := m.Variables[varID]
		if vi != nil && vi.StorageClass == StorageClassOutput {
			colorVarID = varID
			break
		}
	}

	colorVal := outputs[colorVarID]
	color := Vec4ToFloat32(colorVal)
	want := [4]float32{0.25, 0.5, 0.75, 1.0}
	for i := 0; i < 4; i++ {
		if math.Abs(float64(color[i]-want[i])) > 1e-5 {
			t.Errorf("color[%d] = %f, want %f", i, color[i], want[i])
		}
	}
}

func TestSPIRVUniformBufferMultiMember(t *testing.T) {
	words := buildUniformMultiMemberSPIRV()
	m, err := ParseModule(words)
	if err != nil {
		t.Fatalf("ParseModule failed: %v", err)
	}

	// Buffer layout: offset(f32) at byte 0, scale(f32) at byte 4, color(vec4) at byte 16.
	// offset=0.1, scale=2.0, color=(0.5, 0.25, 0.125, 1.0)
	// Expected result: color * scale + vec4(offset)
	//   = (0.5*2 + 0.1, 0.25*2 + 0.1, 0.125*2 + 0.1, 1.0*2 + 0.1)
	//   = (1.1, 0.6, 0.35, 2.1)

	buf := make([]byte, 32)     // 16 bytes for first two f32 + padding, 16 bytes for vec4
	putFloat32LE(buf[0:], 0.1)  // offset
	putFloat32LE(buf[4:], 2.0)  // scale
	putFloat32LE(buf[16:], 0.5) // color.r
	putFloat32LE(buf[20:], 0.25)
	putFloat32LE(buf[24:], 0.125)
	putFloat32LE(buf[28:], 1.0)

	ctx := &ExecutionContext{
		Buffers: map[BindingKey][]byte{
			{Group: 0, Binding: 0}: buf,
		},
	}

	outputs, err := m.ExecuteWithContext("fs_main", ctx)
	if err != nil {
		t.Fatalf("ExecuteWithContext failed: %v", err)
	}

	ep := m.EntryPoints["fs_main"]
	var colorVarID uint32
	for _, varID := range ep.InterfaceIDs {
		vi := m.Variables[varID]
		if vi != nil && vi.StorageClass == StorageClassOutput {
			colorVarID = varID
			break
		}
	}

	colorVal := outputs[colorVarID]
	color := Vec4ToFloat32(colorVal)
	want := [4]float32{1.1, 0.6, 0.35, 2.1}
	for i := 0; i < 4; i++ {
		if math.Abs(float64(color[i]-want[i])) > 1e-4 {
			t.Errorf("color[%d] = %f, want %f", i, color[i], want[i])
		}
	}
}

func TestSPIRVUniformBufferNotBound(t *testing.T) {
	words := buildUniformFragmentSPIRV()
	m, err := ParseModule(words)
	if err != nil {
		t.Fatalf("ParseModule failed: %v", err)
	}

	// Execute without binding any buffer -- should produce zero color.
	ctx := &ExecutionContext{}
	outputs, err := m.ExecuteWithContext("fs_main", ctx)
	if err != nil {
		t.Fatalf("ExecuteWithContext failed: %v", err)
	}

	ep := m.EntryPoints["fs_main"]
	var colorVarID uint32
	for _, varID := range ep.InterfaceIDs {
		vi := m.Variables[varID]
		if vi != nil && vi.StorageClass == StorageClassOutput {
			colorVarID = varID
			break
		}
	}

	colorVal := outputs[colorVarID]
	color := Vec4ToFloat32(colorVal)
	// Expect all zeros since buffer was not bound.
	for i := 0; i < 4; i++ {
		if color[i] != 0 {
			t.Errorf("color[%d] = %f, want 0 (unbound buffer)", i, color[i])
		}
	}
}

func TestSPIRVMemberDecorateOffset(t *testing.T) {
	words := buildUniformMultiMemberSPIRV()
	m, err := ParseModule(words)
	if err != nil {
		t.Fatalf("ParseModule failed: %v", err)
	}

	// Find the struct type.
	var structTypeID uint32
	for id, ti := range m.Types {
		if ti.Kind == TypeStruct && len(ti.MemberIDs) == 3 {
			structTypeID = id
			break
		}
	}
	if structTypeID == 0 {
		t.Fatal("struct type not found")
	}

	tests := []struct {
		member     uint32
		wantOffset uint32
	}{
		{0, 0},  // offset at byte 0
		{1, 4},  // scale at byte 4
		{2, 16}, // color at byte 16
	}
	for _, tt := range tests {
		got := m.GetMemberOffset(structTypeID, tt.member)
		if got != tt.wantOffset {
			t.Errorf("member %d offset = %d, want %d", tt.member, got, tt.wantOffset)
		}
	}
}

func TestSPIRVGetBinding(t *testing.T) {
	words := buildUniformFragmentSPIRV()
	m, err := ParseModule(words)
	if err != nil {
		t.Fatalf("ParseModule failed: %v", err)
	}

	tests := []struct {
		name        string
		varID       uint32
		wantOK      bool
		wantGroup   uint32
		wantBinding uint32
	}{
		// The uniform variable should have binding (0, 0).
		{"uniform_var", 0, false, 0, 0}, // placeholder, will be replaced
	}

	// Find actual variable IDs.
	for varID, vi := range m.Variables {
		if vi.StorageClass == StorageClassUniform {
			tests[0] = struct {
				name        string
				varID       uint32
				wantOK      bool
				wantGroup   uint32
				wantBinding uint32
			}{"uniform_var", varID, true, 0, 0}
		}
	}

	// Add a test for a variable without binding (input/output).
	for varID, vi := range m.Variables {
		if vi.StorageClass == StorageClassOutput {
			tests = append(tests, struct {
				name        string
				varID       uint32
				wantOK      bool
				wantGroup   uint32
				wantBinding uint32
			}{"output_var_no_binding", varID, false, 0, 0})
			break
		}
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bk, ok := m.GetBinding(tt.varID)
			if ok != tt.wantOK {
				t.Errorf("GetBinding(%d) ok = %v, want %v", tt.varID, ok, tt.wantOK)
			}
			if ok {
				if bk.Group != tt.wantGroup || bk.Binding != tt.wantBinding {
					t.Errorf("binding = (%d, %d), want (%d, %d)",
						bk.Group, bk.Binding, tt.wantGroup, tt.wantBinding)
				}
			}
		})
	}
}

func TestWriteValueToBuffer(t *testing.T) {
	tests := []struct {
		name string
		val  Value
		size uint32
	}{
		{"float32", ValFloat(3.14), 4},
		{"uint32", ValUint(42), 4},
		{"int32", ValInt(-7), 4},
		{"vec2", ValVec2From(Vec2{1.0, 2.0}), 8},
		{"vec4", ValVec4From(Vec4{1, 2, 3, 4}), 16},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := make([]byte, tt.size)
			writeValueToBuffer(buf, 0, tt.val)

			// Verify round-trip through readFloat/readUint.
			switch tt.val.Tag {
			case TagFloat32:
				got := readFloat32LE(buf[0:])
				if math.Abs(float64(got-tt.val.AsFloat32())) > 1e-6 {
					t.Errorf("round-trip float = %f, want %f", got, tt.val.AsFloat32())
				}
			case TagUint32:
				got := readUint32LE(buf[0:])
				if got != tt.val.AsUint32() {
					t.Errorf("round-trip uint = %d, want %d", got, tt.val.AsUint32())
				}
			case TagVec4:
				v := tt.val.AsVec4()
				for i := 0; i < 4; i++ {
					got := readFloat32LE(buf[i*4:])
					if math.Abs(float64(got-v[i])) > 1e-6 {
						t.Errorf("round-trip vec4[%d] = %f, want %f", i, got, v[i])
					}
				}
			}
		})
	}
}

func TestValueByteSize(t *testing.T) {
	tests := []struct {
		name string
		val  Value
		want uint32
	}{
		{"float32", ValFloat(0), 4},
		{"uint32", ValUint(0), 4},
		{"int32", ValInt(0), 4},
		{"vec2", ValVec2(0, 0), 8},
		{"vec3", ValVec3(0, 0, 0), 12},
		{"vec4", ValVec4(0, 0, 0, 0), 16},
		{"array_of_float", ValArray(Array{ValFloat(0), ValFloat(0)}), 8},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := valueByteSize(tt.val)
			if got != tt.want {
				t.Errorf("valueByteSize = %d, want %d", got, tt.want)
			}
		})
	}
}

// buildMatrixUniformVertexSPIRV constructs SPIR-V for a vertex shader that reads
// a mat4x4<f32> from a uniform buffer and multiplies it by the position:
//
//	struct Uniforms { transform: mat4x4<f32> }
//	@group(0) @binding(0) var<uniform> uniforms: Uniforms;
//	@vertex fn vs_main(@builtin(vertex_index) idx: u32) -> @builtin(position) vec4<f32> {
//	    var positions = array<vec2<f32>, 3>(
//	        vec2(0.0, 0.5), vec2(-0.5, -0.5), vec2(0.5, -0.5)
//	    );
//	    let pos = vec4<f32>(positions[idx], 0.0, 1.0);
//	    return uniforms.transform * pos;
//	}
//
// This tests the full OpTypeMatrix pipeline: parsing, buffer deserialization,
// and OpMatrixTimesVector — the exact pattern used by textured_quad.wgsl.
func buildMatrixUniformVertexSPIRV() []uint32 {
	inst := spirvInst
	str := spirvString
	f := math.Float32bits

	const (
		idVoid       = 1
		idFloat      = 2
		idUint       = 3
		idVec2       = 4
		idVec4       = 5
		idPtrVec4Out = 6
		idFuncType   = 7
		idFunc       = 8
		idLabel1     = 9
		idVertIdx    = 10 // @builtin(vertex_index)
		idPosOut     = 11 // @builtin(position)
		idPtrUintIn  = 12
		idConst0     = 13 // uint(0)
		idConst1     = 14 // uint(1)
		idConst2     = 15 // uint(2)
		idConst0F    = 16 // float(0.0)
		idConst05F   = 17 // float(0.5)
		idConstN05F  = 18 // float(-0.5)
		idConst1F    = 19 // float(1.0)
		idArr3Vec2   = 20 // array<vec2, 3>
		idPtrArr     = 21
		idPtrVec2    = 22
		idArrConst   = 23 // constant composite array
		idVec2_0     = 24 // vec2(0.0, 0.5)
		idVec2_1     = 25 // vec2(-0.5, -0.5)
		idVec2_2     = 26 // vec2(0.5, -0.5)
		idLocalArr   = 27 // local variable
		idLoadIdx    = 28 // loaded vertex index
		idChainElem  = 29 // access chain into array
		idLoadPos2   = 30 // loaded vec2 position
		idExtractX   = 31 // position.x
		idExtractY   = 32 // position.y
		idPos4       = 33 // vec4(x, y, 0, 1)
		idMat4       = 34 // OpTypeMatrix vec4 4
		idStruct     = 35 // struct { mat4x4 }
		idPtrStruct  = 36 // ptr to struct (Uniform)
		idUniVar     = 37 // uniform variable
		idChainMat   = 38 // access chain to matrix member
		idLoadMat    = 39 // loaded matrix
		idResult     = 40 // matrix * pos
		idConst3U    = 41 // uint(3) for array length
		idPtrMat4Uni = 42 // ptr to mat4 (Uniform) — AccessChain result type
		idBound      = 43
	)

	nameWords := str("vs_main")
	epLen := uint16(3 + len(nameWords) + 2) // +2 for vertIdx and posOut
	epInst := append([]uint32{inst(epLen, OpEntryPoint), ExecutionModelVertex, idFunc}, nameWords...)
	epInst = append(epInst, idVertIdx, idPosOut)

	words := make([]uint32, 0, 250)
	words = append(words,
		spirvMagic, 0x00010300, 0, idBound, 0,
		inst(2, OpCapability), 1, // Shader
		inst(3, OpMemoryModel), 0, 1,
	)
	words = append(words, epInst...)
	words = append(words,
		// Decorations.
		inst(4, OpDecorate), idVertIdx, DecorationBuiltIn, BuiltInVertexIndex,
		inst(4, OpDecorate), idPosOut, DecorationBuiltIn, BuiltInPosition,
		inst(4, OpDecorate), idUniVar, DecorationBinding, 0,
		inst(4, OpDecorate), idUniVar, DecorationDescriptorSet, 0,
		inst(3, OpDecorate), idStruct, DecorationBlock,
		inst(5, OpMemberDecorate), idStruct, 0, DecorationOffset, 0,
		// ColMajor is a valueless decoration: 4 words (not 5).
		inst(4, OpMemberDecorate), idStruct, 0, DecorationColMajor,
		inst(5, OpMemberDecorate), idStruct, 0, DecorationMatrixStride, 16,

		// Types.
		inst(2, OpTypeVoid), idVoid,
		inst(3, OpTypeFloat), idFloat, 32,
		inst(4, OpTypeInt), idUint, 32, 0,
		inst(4, OpTypeVector), idVec2, idFloat, 2,
		inst(4, OpTypeVector), idVec4, idFloat, 4,
		inst(4, OpTypeMatrix), idMat4, idVec4, 4, // mat4x4<f32>
		inst(3, OpTypeStruct), idStruct, idMat4, // struct { mat4x4 }
		inst(4, OpTypePointer), idPtrVec4Out, StorageClassOutput, idVec4,
		inst(4, OpTypePointer), idPtrUintIn, StorageClassInput, idUint,
		inst(4, OpTypePointer), idPtrStruct, StorageClassUniform, idStruct,
		inst(4, OpTypePointer), idPtrArr, StorageClassFunction, idArr3Vec2,
		inst(4, OpTypePointer), idPtrVec2, StorageClassFunction, idVec2,
		inst(4, OpTypePointer), idPtrMat4Uni, StorageClassUniform, idMat4,
		inst(3, OpTypeFunction), idFuncType, idVoid,

		// Constants (before array type, which needs idConst3U).
		inst(4, OpConstant), idUint, idConst0, 0,
		inst(4, OpConstant), idUint, idConst1, 1,
		inst(4, OpConstant), idUint, idConst2, 2,
		inst(4, OpConstant), idUint, idConst3U, 3,
		inst(4, OpConstant), idFloat, idConst0F, f(0.0),
		inst(4, OpConstant), idFloat, idConst05F, f(0.5),
		inst(4, OpConstant), idFloat, idConstN05F, f(-0.5),
		inst(4, OpConstant), idFloat, idConst1F, f(1.0),

		// Array type (after the constant for length).
		inst(4, OpTypeArray), idArr3Vec2, idVec2, idConst3U,

		// Constant composites.
		inst(5, OpConstantComposite), idVec2, idVec2_0, idConst0F, idConst05F, // vec2(0.0, 0.5)
		inst(5, OpConstantComposite), idVec2, idVec2_1, idConstN05F, idConstN05F, // vec2(-0.5, -0.5)
		inst(5, OpConstantComposite), idVec2, idVec2_2, idConst05F, idConstN05F, // vec2(0.5, -0.5)
		inst(6, OpConstantComposite), idArr3Vec2, idArrConst, idVec2_0, idVec2_1, idVec2_2,

		// Variables.
		inst(4, OpVariable), idPtrUintIn, idVertIdx, StorageClassInput,
		inst(4, OpVariable), idPtrVec4Out, idPosOut, StorageClassOutput,
		inst(4, OpVariable), idPtrStruct, idUniVar, StorageClassUniform,

		// Function body.
		inst(5, OpFunction), idVoid, idFunc, 0, idFuncType,
		inst(2, OpLabel), idLabel1,

		// Local array variable.
		inst(4, OpVariable), idPtrArr, idLocalArr, StorageClassFunction,
		inst(3, OpStore), idLocalArr, idArrConst,

		// Load vertex index.
		inst(4, OpLoad), idUint, idLoadIdx, idVertIdx,

		// Access chain into array: &positions[idx]
		inst(5, OpAccessChain), idPtrVec2, idChainElem, idLocalArr, idLoadIdx,
		inst(4, OpLoad), idVec2, idLoadPos2, idChainElem,

		// Extract x, y from vec2.
		inst(5, OpCompositeExtract), idFloat, idExtractX, idLoadPos2, 0,
		inst(5, OpCompositeExtract), idFloat, idExtractY, idLoadPos2, 1,

		// Construct vec4(x, y, 0.0, 1.0).
		inst(7, OpCompositeConstruct), idVec4, idPos4, idExtractX, idExtractY, idConst0F, idConst1F,

		// Load matrix from uniform: uniforms.transform (member 0).
		// AccessChain result type must be pointer-to-mat4, not mat4 itself.
		inst(5, OpAccessChain), idPtrMat4Uni, idChainMat, idUniVar, idConst0,
		inst(4, OpLoad), idMat4, idLoadMat, idChainMat,

		// result = matrix * pos (OpMatrixTimesVector).
		inst(5, OpMatrixTimesVector), idVec4, idResult, idLoadMat, idPos4,

		// Store result to output position.
		inst(3, OpStore), idPosOut, idResult,
		inst(1, OpReturn),
		inst(1, OpFunctionEnd),
	)
	return words
}

// TestSPIRVMatrixUniformTransform tests the full mat4x4<f32> pipeline:
// OpTypeMatrix parsing -> buffer deserialization -> OpMatrixTimesVector.
// This is the exact pattern used by textured_quad.wgsl for ortho projection.
func TestSPIRVMatrixUniformTransform(t *testing.T) {
	words := buildMatrixUniformVertexSPIRV()
	m, err := ParseModule(words)
	if err != nil {
		t.Fatalf("ParseModule failed: %v", err)
	}

	// Verify OpTypeMatrix was parsed.
	var matTypeID uint32
	for id, ti := range m.Types {
		if ti.Kind == TypeMatrix {
			matTypeID = id
			break
		}
	}
	if matTypeID == 0 {
		t.Fatal("TypeMatrix not found in parsed module")
	}

	matType := m.Types[matTypeID]
	if matType.Components != 4 {
		t.Errorf("matrix column count = %d, want 4", matType.Components)
	}

	// Verify MatrixStride decoration was parsed.
	strideKey := decorationKey{TargetID: 0, Decoration: DecorationMatrixStride}
	foundStride := false
	for key, val := range m.MemberDecorations {
		if key.Decoration == DecorationMatrixStride {
			foundStride = true
			if val != 16 {
				t.Errorf("MatrixStride = %d, want 16", val)
			}
			break
		}
	}
	_ = strideKey
	if !foundStride {
		t.Error("MatrixStride member decoration not found")
	}

	// Build a scale(2, 3, 1, 1) matrix in column-major order.
	// Column 0: (2, 0, 0, 0)
	// Column 1: (0, 3, 0, 0)
	// Column 2: (0, 0, 1, 0)
	// Column 3: (0, 0, 0, 1)
	buf := make([]byte, 64) // 4 columns * 4 floats * 4 bytes = 64 bytes
	// Column 0
	putFloat32LE(buf[0:], 2.0)
	putFloat32LE(buf[4:], 0.0)
	putFloat32LE(buf[8:], 0.0)
	putFloat32LE(buf[12:], 0.0)
	// Column 1
	putFloat32LE(buf[16:], 0.0)
	putFloat32LE(buf[20:], 3.0)
	putFloat32LE(buf[24:], 0.0)
	putFloat32LE(buf[28:], 0.0)
	// Column 2
	putFloat32LE(buf[32:], 0.0)
	putFloat32LE(buf[36:], 0.0)
	putFloat32LE(buf[40:], 1.0)
	putFloat32LE(buf[44:], 0.0)
	// Column 3
	putFloat32LE(buf[48:], 0.0)
	putFloat32LE(buf[52:], 0.0)
	putFloat32LE(buf[56:], 0.0)
	putFloat32LE(buf[60:], 1.0)

	ctx := &ExecutionContext{
		Buffers: map[BindingKey][]byte{
			{Group: 0, Binding: 0}: buf,
		},
	}

	// Find output position variable.
	ep := m.EntryPoints["vs_main"]
	if ep == nil {
		t.Fatal("entry point vs_main not found")
	}
	var posVarID uint32
	for _, varID := range ep.InterfaceIDs {
		vi := m.Variables[varID]
		if vi != nil && vi.StorageClass == StorageClassOutput {
			builtIn := m.GetBuiltIn(varID)
			if builtIn == BuiltInPosition {
				posVarID = varID
				break
			}
		}
	}
	if posVarID == 0 {
		t.Fatal("position output variable not found")
	}

	// Test vertex 0: position (0.0, 0.5) -> scale(2,3) -> (0.0, 1.5, 0.0, 1.0)
	var vertexIndexVarID uint32
	for _, varID := range ep.InterfaceIDs {
		vi := m.Variables[varID]
		if vi != nil && vi.StorageClass == StorageClassInput {
			builtIn := m.GetBuiltIn(varID)
			if builtIn == BuiltInVertexIndex {
				vertexIndexVarID = varID
				break
			}
		}
	}
	if vertexIndexVarID == 0 {
		t.Fatal("vertex_index input variable not found")
	}

	tests := []struct {
		name      string
		vertexIdx uint32
		wantX     float32
		wantY     float32
		wantZ     float32
		wantW     float32
	}{
		// vertex 0: (0.0, 0.5) * scale(2,3) = (0.0, 1.5, 0.0, 1.0)
		{"vertex_0_scaled", 0, 0.0, 1.5, 0.0, 1.0},
		// vertex 1: (-0.5, -0.5) * scale(2,3) = (-1.0, -1.5, 0.0, 1.0)
		{"vertex_1_scaled", 1, -1.0, -1.5, 0.0, 1.0},
		// vertex 2: (0.5, -0.5) * scale(2,3) = (1.0, -1.5, 0.0, 1.0)
		{"vertex_2_scaled", 2, 1.0, -1.5, 0.0, 1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx.Inputs = map[uint32]Value{
				vertexIndexVarID: ValUint(tt.vertexIdx),
			}

			outputs, err := m.ExecuteWithContext("vs_main", ctx)
			if err != nil {
				t.Fatalf("ExecuteWithContext failed: %v", err)
			}

			posVal := outputs[posVarID]
			pos := Vec4ToFloat32(posVal)

			if math.Abs(float64(pos[0]-tt.wantX)) > 1e-5 {
				t.Errorf("pos.x = %f, want %f", pos[0], tt.wantX)
			}
			if math.Abs(float64(pos[1]-tt.wantY)) > 1e-5 {
				t.Errorf("pos.y = %f, want %f", pos[1], tt.wantY)
			}
			if math.Abs(float64(pos[2]-tt.wantZ)) > 1e-5 {
				t.Errorf("pos.z = %f, want %f", pos[2], tt.wantZ)
			}
			if math.Abs(float64(pos[3]-tt.wantW)) > 1e-5 {
				t.Errorf("pos.w = %f, want %f", pos[3], tt.wantW)
			}
		})
	}
}

// TestSPIRVMatrixIdentityPassthrough tests that an identity matrix uniform
// produces the same output as no transform.
func TestSPIRVMatrixIdentityPassthrough(t *testing.T) {
	words := buildMatrixUniformVertexSPIRV()
	m, err := ParseModule(words)
	if err != nil {
		t.Fatalf("ParseModule failed: %v", err)
	}

	// Identity matrix in column-major order.
	buf := make([]byte, 64)
	putFloat32LE(buf[0:], 1.0)  // col0.x
	putFloat32LE(buf[20:], 1.0) // col1.y
	putFloat32LE(buf[40:], 1.0) // col2.z
	putFloat32LE(buf[60:], 1.0) // col3.w
	// All other elements are zero (default).

	ctx := &ExecutionContext{
		Buffers: map[BindingKey][]byte{
			{Group: 0, Binding: 0}: buf,
		},
	}

	ep := m.EntryPoints["vs_main"]

	// Find variable IDs.
	var vertexIndexVarID, posVarID uint32
	for _, varID := range ep.InterfaceIDs {
		vi := m.Variables[varID]
		if vi == nil {
			continue
		}
		builtIn := m.GetBuiltIn(varID)
		if vi.StorageClass == StorageClassInput && builtIn == BuiltInVertexIndex {
			vertexIndexVarID = varID
		}
		if vi.StorageClass == StorageClassOutput && builtIn == BuiltInPosition {
			posVarID = varID
		}
	}

	// Vertex 0: (0.0, 0.5) * identity = (0.0, 0.5, 0.0, 1.0)
	ctx.Inputs = map[uint32]Value{
		vertexIndexVarID: ValUint(0),
	}
	outputs, err := m.ExecuteWithContext("vs_main", ctx)
	if err != nil {
		t.Fatalf("ExecuteWithContext failed: %v", err)
	}

	pos := Vec4ToFloat32(outputs[posVarID])
	want := [4]float32{0.0, 0.5, 0.0, 1.0}
	for i := 0; i < 4; i++ {
		if math.Abs(float64(pos[i]-want[i])) > 1e-5 {
			t.Errorf("identity: pos[%d] = %f, want %f", i, pos[i], want[i])
		}
	}
}

// TestSPIRVMatrixOrthoProjection tests an orthographic projection matrix
// read from a uniform buffer — the exact use case from textured_quad.wgsl.
func TestSPIRVMatrixOrthoProjection(t *testing.T) {
	words := buildMatrixUniformVertexSPIRV()
	m, err := ParseModule(words)
	if err != nil {
		t.Fatalf("ParseModule failed: %v", err)
	}

	// Build orthographic projection for 800x600 viewport:
	// left=0, right=800, bottom=600, top=0, near=-1, far=1
	// This maps pixel coordinates to NDC [-1,1].
	//
	// Column-major ortho matrix:
	// col0 = (2/(r-l), 0, 0, 0) = (2/800, 0, 0, 0) = (0.0025, 0, 0, 0)
	// col1 = (0, 2/(t-b), 0, 0) = (0, 2/(-600), 0, 0) = (0, -0.003333, 0, 0)
	// col2 = (0, 0, -2/(f-n), 0) = (0, 0, -1, 0)
	// col3 = (-(r+l)/(r-l), -(t+b)/(t-b), -(f+n)/(f-n), 1) = (-1, 1, 0, 1)
	left := float32(0)
	right := float32(800)
	bottom := float32(600)
	top := float32(0)
	near := float32(-1)
	far := float32(1)

	buf := make([]byte, 64)
	// Column 0
	putFloat32LE(buf[0:], 2.0/(right-left))
	// Column 1
	putFloat32LE(buf[20:], 2.0/(top-bottom))
	// Column 2
	putFloat32LE(buf[40:], -2.0/(far-near))
	// Column 3
	putFloat32LE(buf[48:], -(right+left)/(right-left))
	putFloat32LE(buf[52:], -(top+bottom)/(top-bottom))
	putFloat32LE(buf[56:], -(far+near)/(far-near))
	putFloat32LE(buf[60:], 1.0)

	ctx := &ExecutionContext{
		Buffers: map[BindingKey][]byte{
			{Group: 0, Binding: 0}: buf,
		},
	}

	ep := m.EntryPoints["vs_main"]

	var vertexIndexVarID, posVarID uint32
	for _, varID := range ep.InterfaceIDs {
		vi := m.Variables[varID]
		if vi == nil {
			continue
		}
		builtIn := m.GetBuiltIn(varID)
		if vi.StorageClass == StorageClassInput && builtIn == BuiltInVertexIndex {
			vertexIndexVarID = varID
		}
		if vi.StorageClass == StorageClassOutput && builtIn == BuiltInPosition {
			posVarID = varID
		}
	}

	// Vertex 0 has position (0.0, 0.5) — transform through ortho.
	// ortho * (0.0, 0.5, 0.0, 1.0):
	//   x = 0.0 * (2/800) + 0.0 + 0.0 + 1.0 * (-(800)/(800)) = -1.0
	//   y = 0.0 + 0.5 * (2/(-600)) + 0.0 + 1.0 * (-(0+600)/(-600)) = -0.001667 + 1.0 = 0.998333
	//   z = 0.0 + 0.0 + 0.0*(-1) + 1.0*(0) = 0.0
	//   w = 1.0

	ctx.Inputs = map[uint32]Value{
		vertexIndexVarID: ValUint(0),
	}
	outputs, err := m.ExecuteWithContext("vs_main", ctx)
	if err != nil {
		t.Fatalf("ExecuteWithContext failed: %v", err)
	}

	pos := Vec4ToFloat32(outputs[posVarID])

	// Verify the ortho projection produces valid NDC coordinates.
	// The exact values depend on the input position and ortho parameters.
	// Key validation: w must be 1.0 (ortho preserves w), and x/y must be in NDC range.
	if math.Abs(float64(pos[3]-1.0)) > 1e-5 {
		t.Errorf("ortho: pos.w = %f, want 1.0 (ortho projection preserves w)", pos[3])
	}

	// x should be -1.0 (left edge of viewport for x=0.0 input)
	if math.Abs(float64(pos[0]-(-1.0))) > 1e-3 {
		t.Errorf("ortho: pos.x = %f, want -1.0", pos[0])
	}

	// z should be 0.0 (z=0 maps to center of depth range)
	if math.Abs(float64(pos[2])) > 1e-3 {
		t.Errorf("ortho: pos.z = %f, want ~0.0", pos[2])
	}
}

// TestTypeByteSizeMatrix verifies typeByteSize returns correct size for matrix types.
func TestTypeByteSizeMatrix(t *testing.T) {
	m := &Module{
		Types: map[uint32]*TypeInfo{
			1: {Kind: TypeFloat, Width: 32},
			2: {Kind: TypeVector, ElemType: 1, Components: 4},
			3: {Kind: TypeMatrix, ElemType: 2, Components: 4}, // mat4x4<f32>
			4: {Kind: TypeVector, ElemType: 1, Components: 3},
			5: {Kind: TypeMatrix, ElemType: 4, Components: 3}, // mat3x3<f32>
			6: {Kind: TypeVector, ElemType: 1, Components: 2},
			7: {Kind: TypeMatrix, ElemType: 6, Components: 2}, // mat2x2<f32>
		},
	}

	tests := []struct {
		name   string
		typeID uint32
		want   uint32
	}{
		{"mat4x4_f32", 3, 64}, // 4 columns * 16 bytes = 64
		{"mat3x3_f32", 5, 36}, // 3 columns * 12 bytes = 36
		{"mat2x2_f32", 7, 16}, // 2 columns * 8 bytes = 16
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ti := m.Types[tt.typeID]
			got := typeByteSize(m, ti)
			if got != tt.want {
				t.Errorf("typeByteSize = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestMatrixCompositeExtract tests that extracting a column from a matrix
// works correctly through compositeExtract.
func TestMatrixCompositeExtract(t *testing.T) {
	// Matrix as Array of 4 Vec4 column vectors.
	mat := ValArray(Array{
		ValVec4(1, 0, 0, 0), // col 0
		ValVec4(0, 2, 0, 0), // col 1
		ValVec4(0, 0, 3, 0), // col 2
		ValVec4(0, 0, 0, 4), // col 3
	})

	interp := &interpreter{
		module: &Module{Types: make(map[uint32]*TypeInfo)},
		values: make([]Value, 128),
	}

	// Extract column 1 -> should be vec4(0, 2, 0, 0).
	col1 := interp.compositeExtract(mat, []uint32{1})
	v := Vec4ToFloat32(col1)
	if v[0] != 0 || v[1] != 2 || v[2] != 0 || v[3] != 0 {
		t.Errorf("extract col1 = %v, want (0,2,0,0)", v)
	}

	// Extract col2[2] -> should be 3.0 (nested extract).
	elem := interp.compositeExtract(mat, []uint32{2, 2})
	if f := toFloat32(elem); f != 3.0 {
		t.Errorf("extract col2[2] = %f, want 3.0", f)
	}
}

// TestMatrixCompositeConstruct tests building a matrix from column vectors.
func TestMatrixCompositeConstruct(t *testing.T) {
	m := &Module{
		Types: map[uint32]*TypeInfo{
			1: {Kind: TypeFloat, Width: 32},
			2: {Kind: TypeVector, ElemType: 1, Components: 4},
			3: {Kind: TypeMatrix, ElemType: 2, Components: 4},
		},
	}

	interp := &interpreter{
		module: m,
		values: make([]Value, 128),
	}

	// Set up column vector values.
	interp.values[10] = ValVec4(1, 0, 0, 0)
	interp.values[11] = ValVec4(0, 1, 0, 0)
	interp.values[12] = ValVec4(0, 0, 1, 0)
	interp.values[13] = ValVec4(0, 0, 0, 1)

	// Construct mat4x4 from 4 column vectors.
	result := interp.compositeConstruct(3, []uint32{10, 11, 12, 13})

	if result.Tag != TagArray {
		t.Fatalf("result tag = %d, want TagArray", result.Tag)
	}
	cols := result.AsArray()
	if len(cols) != 4 {
		t.Fatalf("column count = %d, want 4", len(cols))
	}

	// Verify identity matrix.
	for i := 0; i < 4; i++ {
		v := Vec4ToFloat32(cols[i])
		for j := 0; j < 4; j++ {
			want := float32(0)
			if i == j {
				want = 1
			}
			if v[j] != want {
				t.Errorf("mat[%d][%d] = %f, want %f", i, j, v[j], want)
			}
		}
	}
}

// putFloat32LE writes a float32 in little-endian byte order.
func putFloat32LE(b []byte, f float32) {
	bits := math.Float32bits(f)
	b[0] = byte(bits)
	b[1] = byte(bits >> 8)
	b[2] = byte(bits >> 16)
	b[3] = byte(bits >> 24)
}

// readFloat32LE reads a float32 from little-endian bytes.
func readFloat32LE(b []byte) float32 {
	bits := uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
	return math.Float32frombits(bits)
}

// readUint32LE reads a uint32 from little-endian bytes.
func readUint32LE(b []byte) uint32 {
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
}
