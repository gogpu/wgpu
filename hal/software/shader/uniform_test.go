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
