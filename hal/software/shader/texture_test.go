//go:build !(js && wasm)

package shader

import (
	"math"
	"testing"
)

// =============================================================================
// Phase 3 Tests: Texture Sampling
// =============================================================================

// makeTestTexture creates a 4x4 RGBA test texture with known pixel values.
// Layout (row-major, top-to-bottom):
//
//	(0,0)=Red   (1,0)=Green  (2,0)=Blue   (3,0)=White
//	(0,1)=Black (1,1)=Yellow (2,1)=Cyan   (3,1)=Magenta
//	(0,2)=Gray  (1,2)=Orange (2,2)=Purple (3,2)=Teal
//	(0,3)=half  (1,3)=qtr    (2,3)=3qtr   (3,3)=zero-alpha
func makeTestTexture() *Texture2D {
	data := []byte{
		// Row 0
		255, 0, 0, 255, // Red
		0, 255, 0, 255, // Green
		0, 0, 255, 255, // Blue
		255, 255, 255, 255, // White
		// Row 1
		0, 0, 0, 255, // Black
		255, 255, 0, 255, // Yellow
		0, 255, 255, 255, // Cyan
		255, 0, 255, 255, // Magenta
		// Row 2
		128, 128, 128, 255, // Gray
		255, 165, 0, 255, // Orange
		128, 0, 128, 255, // Purple
		0, 128, 128, 255, // Teal
		// Row 3
		128, 128, 128, 128, // Half-alpha gray
		64, 64, 64, 64, // Quarter
		192, 192, 192, 192, // Three-quarter
		255, 255, 255, 0, // Zero alpha
	}
	return &Texture2D{Width: 4, Height: 4, Data: data}
}

func TestSampleNearest(t *testing.T) {
	tex := makeTestTexture()
	tests := []struct {
		name string
		u, v float32
		want Vec4 // approximate expected color
	}{
		{"top_left", 0.0, 0.0, Vec4{1, 0, 0, 1}},                     // Red
		{"top_right", 0.99, 0.0, Vec4{1, 1, 1, 1}},                   // White (column 3)
		{"bottom_left", 0.0, 0.99, Vec4{0.502, 0.502, 0.502, 0.502}}, // Half-alpha gray
		{"center", 0.375, 0.375, Vec4{1, 1, 0, 1}},                   // Yellow (1,1)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sampleNearest(tex, tt.u, tt.v)
			for i := 0; i < 4; i++ {
				if math.Abs(float64(got[i]-tt.want[i])) > 0.01 {
					t.Errorf("sampleNearest(%.3f, %.3f)[%d] = %.3f, want %.3f",
						tt.u, tt.v, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestSampleBilinear(t *testing.T) {
	// 2x2 texture: red, green, blue, white
	tex := &Texture2D{
		Width: 2, Height: 2,
		Data: []byte{
			255, 0, 0, 255, // (0,0) red
			0, 255, 0, 255, // (1,0) green
			0, 0, 255, 255, // (0,1) blue
			255, 255, 255, 255, // (1,1) white
		},
	}

	tests := []struct {
		name string
		u, v float32
		want Vec4
	}{
		// Center of 2x2 should be average of all 4 texels.
		{"center", 0.5, 0.5, Vec4{0.502, 0.502, 0.502, 1.0}},
		// Top-left corner is the red texel.
		{"top_left", 0.0, 0.0, Vec4{1, 0, 0, 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sampleBilinear(tex, tt.u, tt.v)
			for i := 0; i < 4; i++ {
				if math.Abs(float64(got[i]-tt.want[i])) > 0.05 {
					t.Errorf("sampleBilinear(%.3f, %.3f)[%d] = %.3f, want ~%.3f",
						tt.u, tt.v, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestApplyWrapMode(t *testing.T) {
	tests := []struct {
		name  string
		coord float32
		mode  uint32
		want  float32
	}{
		// Repeat mode
		{"repeat_normal", 0.5, WrapRepeat, 0.5},
		{"repeat_wrap", 1.5, WrapRepeat, 0.5},
		{"repeat_neg", -0.25, WrapRepeat, 0.75},

		// Clamp to edge
		{"clamp_normal", 0.5, WrapClampToEdge, 0.5},
		{"clamp_over", 1.5, WrapClampToEdge, 1.0},
		{"clamp_under", -0.5, WrapClampToEdge, 0.0},

		// Mirrored repeat
		{"mirror_normal", 0.3, WrapMirroredRepeat, 0.3},
		{"mirror_reflect", 1.3, WrapMirroredRepeat, 0.7},
		{"mirror_double", 2.3, WrapMirroredRepeat, 0.3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := applyWrapMode(tt.coord, tt.mode)
			if math.Abs(float64(got-tt.want)) > 0.01 {
				t.Errorf("applyWrapMode(%f, %d) = %f, want %f", tt.coord, tt.mode, got, tt.want)
			}
		})
	}
}

func TestReadTexel(t *testing.T) {
	tex := &Texture2D{
		Width: 2, Height: 1,
		Data: []byte{
			0, 128, 255, 255, // (0,0)
			255, 0, 0, 128, // (1,0)
		},
	}

	got := readTexel(tex, 0, 0)
	if math.Abs(float64(got[0])) > 0.01 || math.Abs(float64(got[1]-0.502)) > 0.01 ||
		math.Abs(float64(got[2]-1.0)) > 0.01 || math.Abs(float64(got[3]-1.0)) > 0.01 {
		t.Errorf("readTexel(0,0) = %v, want ~{0, 0.502, 1.0, 1.0}", got)
	}

	got = readTexel(tex, 1, 0)
	if math.Abs(float64(got[0]-1.0)) > 0.01 || math.Abs(float64(got[3]-0.502)) > 0.01 {
		t.Errorf("readTexel(1,0) = %v, want ~{1, 0, 0, 0.502}", got)
	}

	// Out of bounds returns zero.
	got = readTexel(tex, 5, 5)
	if got != (Vec4{0, 0, 0, 0}) {
		t.Errorf("readTexel OOB = %v, want zero", got)
	}
}

// buildTextureSampleSPIRV constructs SPIR-V for a fragment shader that samples
// a texture:
//
//	@group(0) @binding(0) var tex: texture_2d<f32>;
//	@group(0) @binding(1) var samp: sampler;
//	@fragment fn fs_main(@location(0) uv: vec2<f32>) -> @location(0) vec4<f32> {
//	    return textureSample(tex, samp, uv);
//	}
func buildTextureSampleSPIRV() []uint32 {
	inst := spirvInst
	str := spirvString

	const (
		idVoid         = 1
		idFloat        = 2
		idVec2         = 3
		idVec4         = 4
		idPtrVec4Out   = 5
		idPtrVec2In    = 6
		idFuncType     = 7
		idFunc         = 8
		idLabel1       = 9
		idColorOut     = 10
		idUVIn         = 11
		idImgType      = 12
		idSamplerType  = 13
		idSampledImgTy = 14
		idPtrImg       = 15
		idPtrSampler   = 16
		idTexVar       = 17
		idSampVar      = 18
		idLoadTex      = 19
		idLoadSamp     = 20
		idSampledImg   = 21
		idLoadUV       = 22
		idSampleResult = 23
		idBound        = 24
	)

	nameWords := str("fs_main")
	epLen := uint16(3 + len(nameWords) + 2)
	epInst := append([]uint32{inst(epLen, OpEntryPoint), ExecutionModelFragment, idFunc}, nameWords...)
	epInst = append(epInst, idColorOut, idUVIn)

	words := make([]uint32, 0, 160)
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
		inst(4, OpDecorate), idUVIn, DecorationLocation, 0,
		inst(4, OpDecorate), idTexVar, DecorationBinding, 0,
		inst(4, OpDecorate), idTexVar, DecorationDescriptorSet, 0,
		inst(4, OpDecorate), idSampVar, DecorationBinding, 1,
		inst(4, OpDecorate), idSampVar, DecorationDescriptorSet, 0,

		// Types.
		inst(2, OpTypeVoid), idVoid,
		inst(3, OpTypeFloat), idFloat, 32,
		inst(4, OpTypeVector), idVec2, idFloat, 2,
		inst(4, OpTypeVector), idVec4, idFloat, 4,
		// OpTypeImage: result sampledType Dim Depth Arrayed MS Sampled Format
		inst(9, OpTypeImage), idImgType, idFloat, 1, 0, 0, 0, 1, 0, // Dim2D, sampled
		inst(2, OpTypeSampler), idSamplerType,
		inst(3, OpTypeSampledImage), idSampledImgTy, idImgType,
		inst(4, OpTypePointer), idPtrVec4Out, StorageClassOutput, idVec4,
		inst(4, OpTypePointer), idPtrVec2In, StorageClassInput, idVec2,
		inst(4, OpTypePointer), idPtrImg, StorageClassUniformConstant, idImgType,
		inst(4, OpTypePointer), idPtrSampler, StorageClassUniformConstant, idSamplerType,
		inst(3, OpTypeFunction), idFuncType, idVoid,

		// Variables.
		inst(4, OpVariable), idPtrVec4Out, idColorOut, StorageClassOutput,
		inst(4, OpVariable), idPtrVec2In, idUVIn, StorageClassInput,
		inst(4, OpVariable), idPtrImg, idTexVar, StorageClassUniformConstant,
		inst(4, OpVariable), idPtrSampler, idSampVar, StorageClassUniformConstant,

		// Function body.
		inst(5, OpFunction), idVoid, idFunc, 0, idFuncType,
		inst(2, OpLabel), idLabel1,
		inst(4, OpLoad), idImgType, idLoadTex, idTexVar,
		inst(4, OpLoad), idSamplerType, idLoadSamp, idSampVar,
		inst(5, OpSampledImage), idSampledImgTy, idSampledImg, idLoadTex, idLoadSamp,
		inst(4, OpLoad), idVec2, idLoadUV, idUVIn,
		inst(5, OpImageSampleImplicitLod), idVec4, idSampleResult, idSampledImg, idLoadUV,
		inst(3, OpStore), idColorOut, idSampleResult,
		inst(1, OpReturn),
		inst(1, OpFunctionEnd),
	)
	return words
}

func TestSPIRVTextureSample(t *testing.T) {
	words := buildTextureSampleSPIRV()
	m, err := ParseModule(words)
	if err != nil {
		t.Fatalf("ParseModule failed: %v", err)
	}

	// Create a 2x2 texture: red, green, blue, white.
	tex := &Texture2D{
		Width: 2, Height: 2,
		Data: []byte{
			255, 0, 0, 255,
			0, 255, 0, 255,
			0, 0, 255, 255,
			255, 255, 255, 255,
		},
	}

	samp := &Sampler{
		MinFilter: FilterNearest,
		MagFilter: FilterNearest,
		WrapU:     WrapRepeat,
		WrapV:     WrapRepeat,
	}

	// Find UV input variable.
	ep := m.EntryPoints["fs_main"]
	var uvVarID, colorVarID uint32
	for _, varID := range ep.InterfaceIDs {
		vi := m.Variables[varID]
		if vi == nil {
			continue
		}
		if vi.StorageClass == StorageClassInput {
			uvVarID = varID
		}
		if vi.StorageClass == StorageClassOutput {
			colorVarID = varID
		}
	}

	tests := []struct {
		name string
		uv   Vec2
		want Vec4
	}{
		{"red", Vec2{0.0, 0.0}, Vec4{1, 0, 0, 1}},
		{"green", Vec2{0.99, 0.0}, Vec4{0, 1, 0, 1}},
		{"blue", Vec2{0.0, 0.99}, Vec4{0, 0, 1, 1}},
		{"white", Vec2{0.99, 0.99}, Vec4{1, 1, 1, 1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &ExecutionContext{
				Inputs: map[uint32]Value{uvVarID: testval(tt.uv)},
				Textures: map[BindingKey]*Texture2D{
					{Group: 0, Binding: 0}: tex,
				},
				Samplers: map[BindingKey]*Sampler{
					{Group: 0, Binding: 1}: samp,
				},
			}

			outputs, err := m.ExecuteWithContext("fs_main", ctx)
			if err != nil {
				t.Fatalf("ExecuteWithContext failed: %v", err)
			}

			color := Vec4ToFloat32(outputs[colorVarID])
			for i := 0; i < 4; i++ {
				if math.Abs(float64(color[i]-tt.want[i])) > 0.05 {
					t.Errorf("color[%d] = %.3f, want %.3f", i, color[i], tt.want[i])
				}
			}
		})
	}
}

func TestSPIRVTextureSampleBilinear(t *testing.T) {
	words := buildTextureSampleSPIRV()
	m, err := ParseModule(words)
	if err != nil {
		t.Fatalf("ParseModule failed: %v", err)
	}

	// 2x2 texture: corners are red, green, blue, white.
	tex := &Texture2D{
		Width: 2, Height: 2,
		Data: []byte{
			255, 0, 0, 255,
			0, 255, 0, 255,
			0, 0, 255, 255,
			255, 255, 255, 255,
		},
	}

	samp := &Sampler{
		MinFilter: FilterLinear,
		MagFilter: FilterLinear,
		WrapU:     WrapClampToEdge,
		WrapV:     WrapClampToEdge,
	}

	ep := m.EntryPoints["fs_main"]
	var uvVarID, colorVarID uint32
	for _, varID := range ep.InterfaceIDs {
		vi := m.Variables[varID]
		if vi == nil {
			continue
		}
		if vi.StorageClass == StorageClassInput {
			uvVarID = varID
		}
		if vi.StorageClass == StorageClassOutput {
			colorVarID = varID
		}
	}

	// Bilinear at center of 2x2: average of all four texels.
	ctx := &ExecutionContext{
		Inputs: map[uint32]Value{uvVarID: ValVec2(0.5, 0.5)},
		Textures: map[BindingKey]*Texture2D{
			{Group: 0, Binding: 0}: tex,
		},
		Samplers: map[BindingKey]*Sampler{
			{Group: 0, Binding: 1}: samp,
		},
	}

	outputs, err := m.ExecuteWithContext("fs_main", ctx)
	if err != nil {
		t.Fatalf("ExecuteWithContext failed: %v", err)
	}

	color := Vec4ToFloat32(outputs[colorVarID])
	// Average RGBA = ((255+0+0+255)/4, (0+255+0+255)/4, (0+0+255+255)/4, 255)/255
	// = (127.5, 127.5, 127.5, 255) / 255 ~ (0.5, 0.5, 0.5, 1.0)
	for i := 0; i < 3; i++ {
		if math.Abs(float64(color[i]-0.5)) > 0.1 {
			t.Errorf("bilinear center color[%d] = %.3f, want ~0.5", i, color[i])
		}
	}
	if math.Abs(float64(color[3]-1.0)) > 0.05 {
		t.Errorf("bilinear center alpha = %.3f, want ~1.0", color[3])
	}
}

func TestSPIRVTextureMissing(t *testing.T) {
	words := buildTextureSampleSPIRV()
	m, err := ParseModule(words)
	if err != nil {
		t.Fatalf("ParseModule failed: %v", err)
	}

	ep := m.EntryPoints["fs_main"]
	var uvVarID, colorVarID uint32
	for _, varID := range ep.InterfaceIDs {
		vi := m.Variables[varID]
		if vi == nil {
			continue
		}
		if vi.StorageClass == StorageClassInput {
			uvVarID = varID
		}
		if vi.StorageClass == StorageClassOutput {
			colorVarID = varID
		}
	}

	// No texture bound -- should return magenta.
	ctx := &ExecutionContext{
		Inputs: map[uint32]Value{uvVarID: ValVec2(0.5, 0.5)},
	}
	outputs, err := m.ExecuteWithContext("fs_main", ctx)
	if err != nil {
		t.Fatalf("ExecuteWithContext failed: %v", err)
	}

	color := Vec4ToFloat32(outputs[colorVarID])
	// Magenta = (1, 0, 1, 1)
	want := Vec4{1, 0, 1, 1}
	for i := 0; i < 4; i++ {
		if math.Abs(float64(color[i]-want[i])) > 0.01 {
			t.Errorf("missing texture color[%d] = %.3f, want %.3f", i, color[i], want[i])
		}
	}
}

func TestFetchTexel(t *testing.T) {
	tex := &Texture2D{
		Width: 2, Height: 2,
		Data: []byte{
			255, 0, 0, 255, // (0,0) red
			0, 255, 0, 255, // (1,0) green
			0, 0, 255, 255, // (0,1) blue
			255, 255, 255, 255, // (1,1) white
		},
	}
	bk := BindingKey{Group: 0, Binding: 0}
	interp := &interpreter{ctx: &ExecutionContext{Textures: map[BindingKey]*Texture2D{bk: tex}}}

	tests := []struct {
		name  string
		coord Value
		want  Vec4
	}{
		{"pixel_00", ValVec2(0, 0), Vec4{1, 0, 0, 1}},
		{"pixel_10", ValVec2(1, 0), Vec4{0, 1, 0, 1}},
		{"pixel_01", ValVec2(0, 1), Vec4{0, 0, 1, 1}},
		{"pixel_11", ValVec2(1, 1), Vec4{1, 1, 1, 1}},
		{"clamped", ValVec2(5, 5), Vec4{1, 1, 1, 1}}, // Clamped to (1,1)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := interp.fetchTexel(ValBindingKey(bk), tt.coord)
			_ = bk // used in test setup
			gv := Vec4ToFloat32(got)
			for i := 0; i < 4; i++ {
				if math.Abs(float64(gv[i]-tt.want[i])) > 0.01 {
					t.Errorf("fetchTexel %s [%d] = %.3f, want %.3f", tt.name, i, gv[i], tt.want[i])
				}
			}
		})
	}
}

func TestSampledImageValue(t *testing.T) {
	// Verify SampledImageValue correctly combines image and sampler references.
	si := &SampledImageValue{
		Image:   ValBindingKey(BindingKey{Group: 0, Binding: 0}),
		Sampler: ValBindingKey(BindingKey{Group: 0, Binding: 1}),
	}

	if si.Image.Tag != TagBindingKey || si.Image.AsBindingKey().Binding != 0 {
		t.Errorf("SampledImageValue.Image = %v, want BindingKey{0,0}", si.Image)
	}
	if si.Sampler.Tag != TagBindingKey || si.Sampler.AsBindingKey().Binding != 1 {
		t.Errorf("SampledImageValue.Sampler = %v, want BindingKey{0,1}", si.Sampler)
	}
}

func TestQueryImageSize(t *testing.T) {
	tex := &Texture2D{Width: 512, Height: 256}
	bk := BindingKey{Group: 0, Binding: 0}
	interp := &interpreter{ctx: &ExecutionContext{Textures: map[BindingKey]*Texture2D{bk: tex}}}

	got := interp.queryImageSize(ValBindingKey(bk))
	gv, ok := testIsVec2(got)
	if !ok {
		t.Fatalf("queryImageSize returned %T, want Vec2", got)
	}
	if gv[0] != 512 || gv[1] != 256 {
		t.Errorf("queryImageSize = %v, want {512, 256}", gv)
	}

	// Nil texture returns zero.
	got = interp.queryImageSize(Value{})
	gv = got.AsVec2()
	if got.Tag != TagVec2 || gv != (Vec2{0, 0}) {
		t.Errorf("queryImageSize(nil) = %v, want {0, 0}", got)
	}
}
