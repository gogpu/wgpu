//go:build !(js && wasm)

package software

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// runSubCompute dispatches one invocation of wgsl's main with entries bound
// to group 0 and returns the floats of the storage buffer out, which the
// caller binds at outBinding with n vec4 slots.
func runSubCompute(t *testing.T, dev *Device, wgsl string, outBinding uint32, n int, entries ...gputypes.BindGroupEntry) []float32 {
	t.Helper()
	sm, err := dev.CreateShaderModule(&hal.ShaderModuleDescriptor{Label: "sub", Source: hal.ShaderSource{WGSL: wgsl}})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	pipeline, err := dev.CreateComputePipeline(&hal.ComputePipelineDescriptor{
		Compute: hal.ComputeState{Module: sm, EntryPoint: "main"},
	})
	if err != nil {
		t.Fatalf("CreateComputePipeline: %v", err)
	}
	out, err := dev.CreateBuffer(&hal.BufferDescriptor{Size: uint64(n * 16), Usage: gputypes.BufferUsageStorage})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	entries = append(entries, gputypes.BindGroupEntry{Binding: outBinding, Resource: gputypes.BufferBinding{Buffer: out.NativeHandle()}})
	bg, err := dev.CreateBindGroup(&hal.BindGroupDescriptor{Entries: entries})
	if err != nil {
		t.Fatalf("CreateBindGroup: %v", err)
	}
	enc, err := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	pass := enc.BeginComputePass(&hal.ComputePassDescriptor{})
	pass.SetPipeline(pipeline)
	pass.SetBindGroup(0, bg, nil)
	pass.Dispatch(1, 1, 1)
	pass.End()
	submit(t, enc)

	data := out.(*Buffer).GetData()
	f := make([]float32, n*4)
	for i := range f {
		f[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[i*4:]))
	}
	return f
}

func newSubSampler(t *testing.T, dev *Device, desc hal.SamplerDescriptor) hal.Sampler {
	t.Helper()
	s, err := dev.CreateSampler(&desc)
	if err != nil {
		t.Fatalf("CreateSampler: %v", err)
	}
	return s
}

func texBinding(b uint32, v hal.TextureView) gputypes.BindGroupEntry {
	return gputypes.BindGroupEntry{Binding: b, Resource: gputypes.TextureViewBinding{TextureView: v.NativeHandle()}}
}

func samplerBinding(b uint32, s hal.Sampler) gputypes.BindGroupEntry {
	return gputypes.BindGroupEntry{Binding: b, Resource: gputypes.SamplerBinding{Sampler: s.NativeHandle()}}
}

// unormColor is the RGBA8 color c as the floats a sample of it returns.
func unormColor(c [4]byte) [4]float32 {
	return [4]float32{float32(c[0]) / 255, float32(c[1]) / 255, float32(c[2]) / 255, float32(c[3]) / 255}
}

// checkVec4 compares slot i of out with want within 1/512.
func checkVec4(t *testing.T, out []float32, i int, what string, want [4]float32) {
	t.Helper()
	for c := range 4 {
		if math.Abs(float64(out[i*4+c]-want[c])) > 1.0/512 {
			t.Errorf("%s = %v, want %v", what, out[i*4:i*4+4], want)
			return
		}
	}
}

// mipColor is the solid color of mip level l in the LOD tests.
func mipColor(l uint32) [4]byte {
	return [4]byte{byte(40 + 60*l), byte(250 - 60*l), byte(20 * l), 255}
}

// subSampleLevelWGSL samples a 4-level texture (8, 4, 2, 1 texels), each
// level a solid mipColor, and a view of its levels 1 and 2.
const subSampleLevelWGSL = `
@group(0) @binding(0) var t: texture_2d<f32>;
@group(0) @binding(1) var sn: sampler;
@group(0) @binding(2) var sl: sampler;
@group(0) @binding(3) var smin: sampler;
@group(0) @binding(4) var smax: sampler;
@group(0) @binding(5) var tv: texture_2d<f32>;
@group(0) @binding(6) var<storage, read_write> out: array<vec4<f32>>;

@compute @workgroup_size(1)
fn main() {
    let uv = vec2<f32>(0.3, 0.6);
    out[0] = textureSampleLevel(t, sn, uv, 0.0);
    out[1] = textureSampleLevel(t, sn, uv, 1.0);
    out[2] = textureSampleLevel(t, sn, uv, 2.0);
    out[3] = textureSampleLevel(t, sn, uv, 3.0);
    out[4] = textureSampleLevel(t, sn, uv, 1.4);
    out[5] = textureSampleLevel(t, sn, uv, 9.0);
    out[6] = textureSampleLevel(t, sl, uv, 1.5);
    out[7] = textureSampleLevel(t, smin, uv, 0.0);
    out[8] = textureSampleLevel(t, smax, uv, 3.0);
    out[9] = textureLoad(t, vec2<i32>(1, 1), 2);
    let d = textureDimensions(t, 1);
    out[10] = vec4<f32>(f32(d.x), f32(d.y), f32(textureNumLevels(t)), 0.0);
    out[11] = textureSampleLevel(tv, sn, uv, 0.0);
    out[12] = textureSampleLevel(tv, sn, uv, 5.0);
    let dv = textureDimensions(tv);
    out[13] = vec4<f32>(f32(dv.x), f32(dv.y), f32(textureNumLevels(tv)), 0.0);
}
`

// TestSampleLevelSelectsMip uploads a full mip chain, each level a solid
// color, and samples it with textureSampleLevel at each level, between
// levels with nearest and linear mipmap filters, past the last level, with
// the sampler's lodMinClamp and lodMaxClamp, through a view of levels 1-2,
// with textureLoad at a level, and queries the level count and a level's
// size. The old interpreter sampled mip level 0 for every LOD.
func TestSampleLevelSelectsMip(t *testing.T) {
	dev, q, cleanup := createSoftwareDevice(t)
	defer cleanup()
	const mips = 4
	tex := newSubTexture(t, dev, gputypes.TextureDimension2D, 8, 8, 1, mips, gputypes.TextureFormatRGBA8Unorm)
	for level := range uint32(mips) {
		w, h, _ := subExtent(gputypes.TextureDimension2D, 8, 8, 1, level)
		fillLevel(t, q, tex, level, 0, w, h, 1, func(_, _, _ uint32) [4]byte { return mipColor(level) })
	}
	nearest := hal.SamplerDescriptor{MipmapFilter: gputypes.FilterModeNearest, LodMaxClamp: 32}
	linear := nearest
	linear.MipmapFilter = gputypes.FilterModeLinear
	minClamp := nearest
	minClamp.LodMinClamp = 2
	maxClamp := nearest
	maxClamp.LodMaxClamp = 1

	out := runSubCompute(t, dev, subSampleLevelWGSL, 6, 14,
		texBinding(0, newSubView(t, dev, tex, hal.TextureViewDescriptor{})),
		samplerBinding(1, newSubSampler(t, dev, nearest)),
		samplerBinding(2, newSubSampler(t, dev, linear)),
		samplerBinding(3, newSubSampler(t, dev, minClamp)),
		samplerBinding(4, newSubSampler(t, dev, maxClamp)),
		texBinding(5, newSubView(t, dev, tex, hal.TextureViewDescriptor{BaseMipLevel: 1, MipLevelCount: 2})),
	)

	for l := range uint32(mips) {
		checkVec4(t, out, int(l), "textureSampleLevel at level "+string(rune('0'+l)), unormColor(mipColor(l)))
	}
	checkVec4(t, out, 4, "nearest mipmap at LOD 1.4", unormColor(mipColor(1)))
	checkVec4(t, out, 5, "LOD 9 past the last level", unormColor(mipColor(3)))
	c1, c2 := unormColor(mipColor(1)), unormColor(mipColor(2))
	checkVec4(t, out, 6, "linear mipmap at LOD 1.5",
		[4]float32{(c1[0] + c2[0]) / 2, (c1[1] + c2[1]) / 2, (c1[2] + c2[2]) / 2, (c1[3] + c2[3]) / 2})
	checkVec4(t, out, 7, "lodMinClamp 2 at LOD 0", unormColor(mipColor(2)))
	checkVec4(t, out, 8, "lodMaxClamp 1 at LOD 3", unormColor(mipColor(1)))
	checkVec4(t, out, 9, "textureLoad at level 2", unormColor(mipColor(2)))
	checkVec4(t, out, 10, "textureDimensions(t, 1), textureNumLevels(t)", [4]float32{4, 4, mips, 0})
	checkVec4(t, out, 11, "view of levels 1-2 at LOD 0", unormColor(mipColor(1)))
	checkVec4(t, out, 12, "view of levels 1-2 at LOD 5", unormColor(mipColor(2)))
	checkVec4(t, out, 13, "view of levels 1-2: size and level count", [4]float32{4, 4, 2, 0})
}

// layerColor is the solid color of array layer, cube face or depth slice i.
func layerColor(i uint32) [4]byte {
	return [4]byte{byte(30 * (i + 1)), byte(200 - 30*i), byte(17 * i), 255}
}

// subSampleDimsWGSL samples a 2D array, a view of two of its layers, a cube
// and a 3D texture whose layers, faces and slices are solid layerColors.
const subSampleDimsWGSL = `
@group(0) @binding(0) var ta: texture_2d_array<f32>;
@group(0) @binding(1) var s: sampler;
@group(0) @binding(2) var tc: texture_cube<f32>;
@group(0) @binding(3) var t3: texture_3d<f32>;
@group(0) @binding(4) var tav: texture_2d_array<f32>;
@group(0) @binding(5) var<storage, read_write> out: array<vec4<f32>>;

@compute @workgroup_size(1)
fn main() {
    let uv = vec2<f32>(0.4, 0.7);
    for (var i = 0; i < 3; i++) {
        out[i] = textureSampleLevel(ta, s, uv, i, 0.0);
    }
    out[3] = textureLoad(ta, vec2<i32>(1, 2), 2, 0);
    out[4] = textureSampleLevel(tc, s, vec3<f32>(1.0, 0.2, -0.3), 0.0);
    out[5] = textureSampleLevel(tc, s, vec3<f32>(-1.0, 0.2, 0.3), 0.0);
    out[6] = textureSampleLevel(tc, s, vec3<f32>(0.1, 1.0, 0.3), 0.0);
    out[7] = textureSampleLevel(tc, s, vec3<f32>(0.1, -1.0, 0.3), 0.0);
    out[8] = textureSampleLevel(tc, s, vec3<f32>(0.1, 0.2, 1.0), 0.0);
    out[9] = textureSampleLevel(tc, s, vec3<f32>(0.1, 0.2, -1.0), 0.0);
    for (var z = 0; z < 4; z++) {
        out[10 + z] = textureSampleLevel(t3, s, vec3<f32>(0.5, 0.5, (f32(z) + 0.5) / 4.0), 0.0);
    }
    out[14] = textureLoad(t3, vec3<i32>(1, 2, 3), 0);
    let d3 = textureDimensions(t3);
    out[15] = vec4<f32>(f32(d3.x), f32(d3.y), f32(d3.z), f32(textureNumLayers(ta)));
    out[16] = textureSampleLevel(tav, s, uv, 0, 0.0);
    out[17] = textureSampleLevel(tav, s, uv, 1, 0.0);
    out[18] = vec4<f32>(f32(textureNumLayers(tav)), f32(textureDimensions(tc).x), 0.0, 0.0);
}
`

// TestSampleArrayCubeAnd3D samples each layer of a 2D array, each face of a
// cube view on a 6-layer texture, each depth slice of a 3D texture, and two
// layers of an array view starting at layer 1; loads a texel of a layer and
// of a slice; and queries their sizes and layer counts. The old interpreter
// read layer 0, slice 0 for every lookup.
func TestSampleArrayCubeAnd3D(t *testing.T) {
	dev, q, cleanup := createSoftwareDevice(t)
	defer cleanup()
	solid := func(_, _, z uint32) [4]byte { return layerColor(z) }
	arr := newSubTexture(t, dev, gputypes.TextureDimension2D, 4, 4, 3, 1, gputypes.TextureFormatRGBA8Unorm)
	fillLevel(t, q, arr, 0, 0, 4, 4, 3, solid)
	cube := newSubTexture(t, dev, gputypes.TextureDimension2D, 4, 4, 6, 1, gputypes.TextureFormatRGBA8Unorm)
	fillLevel(t, q, cube, 0, 0, 4, 4, 6, solid)
	vol := newSubTexture(t, dev, gputypes.TextureDimension3D, 4, 4, 4, 1, gputypes.TextureFormatRGBA8Unorm)
	fillLevel(t, q, vol, 0, 0, 4, 4, 4, solid)

	out := runSubCompute(t, dev, subSampleDimsWGSL, 5, 19,
		texBinding(0, newSubView(t, dev, arr, hal.TextureViewDescriptor{Dimension: gputypes.TextureViewDimension2DArray})),
		samplerBinding(1, newSubSampler(t, dev, hal.SamplerDescriptor{
			AddressModeU: gputypes.AddressModeClampToEdge, AddressModeV: gputypes.AddressModeClampToEdge,
			AddressModeW: gputypes.AddressModeClampToEdge,
		})),
		texBinding(2, newSubView(t, dev, cube, hal.TextureViewDescriptor{Dimension: gputypes.TextureViewDimensionCube})),
		texBinding(3, newSubView(t, dev, vol, hal.TextureViewDescriptor{})),
		texBinding(4, newSubView(t, dev, arr, hal.TextureViewDescriptor{
			Dimension: gputypes.TextureViewDimension2DArray, BaseArrayLayer: 1, ArrayLayerCount: 2,
		})),
	)

	for i := range uint32(3) {
		checkVec4(t, out, int(i), "2d-array layer "+string(rune('0'+i)), unormColor(layerColor(i)))
	}
	checkVec4(t, out, 3, "textureLoad of layer 2", unormColor(layerColor(2)))
	for f, name := range []string{"+x", "-x", "+y", "-y", "+z", "-z"} {
		checkVec4(t, out, 4+f, "cube face "+name, unormColor(layerColor(uint32(f))))
	}
	for z := range uint32(4) {
		checkVec4(t, out, int(10+z), "3d slice "+string(rune('0'+z)), unormColor(layerColor(z)))
	}
	checkVec4(t, out, 14, "textureLoad of 3d slice 3", unormColor(layerColor(3)))
	checkVec4(t, out, 15, "textureDimensions(t3), textureNumLayers(ta)", [4]float32{4, 4, 4, 3})
	checkVec4(t, out, 16, "array view from layer 1, layer 0", unormColor(layerColor(1)))
	checkVec4(t, out, 17, "array view from layer 1, layer 1", unormColor(layerColor(2)))
	checkVec4(t, out, 18, "textureNumLayers(tav), cube size", [4]float32{2, 4, 0, 0})
}

// subStoreWGSL stores into layer 1 of a storage 2D-array view, slice 2 of a
// storage 3D view, and a storage view of mip level 1.
const subStoreWGSL = `
@group(0) @binding(0) var sa: texture_storage_2d_array<rgba8unorm, write>;
@group(0) @binding(1) var s3: texture_storage_3d<rgba8unorm, write>;
@group(0) @binding(2) var sm: texture_storage_2d<rgba8unorm, write>;
@group(0) @binding(3) var<storage, read_write> out: array<vec4<f32>>;

@compute @workgroup_size(1)
fn main() {
    textureStore(sa, vec2<i32>(1, 0), 1, vec4<f32>(0.0, 1.0, 0.0, 1.0));
    textureStore(s3, vec3<i32>(0, 1, 2), vec4<f32>(0.0, 0.0, 1.0, 1.0));
    textureStore(sm, vec2<i32>(1, 1), vec4<f32>(1.0, 1.0, 0.0, 1.0));
    out[0] = vec4<f32>(1.0);
}
`

// TestStorageTextureSubresources stores a texel into an array layer, a 3D
// depth slice and mip level 1 through storage texture views: each store
// lands in its own subresource and nowhere else.
func TestStorageTextureSubresources(t *testing.T) {
	dev, q, cleanup := createSoftwareDevice(t)
	defer cleanup()
	sentinel := func(_, _, _ uint32) [4]byte { return subSentinel }
	arr := newSubTexture(t, dev, gputypes.TextureDimension2D, 2, 2, 2, 1, gputypes.TextureFormatRGBA8Unorm)
	fillLevel(t, q, arr, 0, 0, 2, 2, 2, sentinel)
	vol := newSubTexture(t, dev, gputypes.TextureDimension3D, 2, 2, 3, 1, gputypes.TextureFormatRGBA8Unorm)
	fillLevel(t, q, vol, 0, 0, 2, 2, 3, sentinel)
	mip := newSubTexture(t, dev, gputypes.TextureDimension2D, 4, 4, 1, 2, gputypes.TextureFormatRGBA8Unorm)
	fillLevel(t, q, mip, 0, 0, 4, 4, 1, sentinel)
	fillLevel(t, q, mip, 1, 0, 2, 2, 1, sentinel)

	runSubCompute(t, dev, subStoreWGSL, 3, 1,
		texBinding(0, newSubView(t, dev, arr, hal.TextureViewDescriptor{Dimension: gputypes.TextureViewDimension2DArray})),
		texBinding(1, newSubView(t, dev, vol, hal.TextureViewDescriptor{})),
		texBinding(2, newSubView(t, dev, mip, hal.TextureViewDescriptor{BaseMipLevel: 1, MipLevelCount: 1})),
	)

	at := func(tx, ty uint32, c [4]byte) func(x, y uint32) [4]byte {
		return func(x, y uint32) [4]byte {
			if x == tx && y == ty {
				return c
			}
			return subSentinel
		}
	}
	none := func(_, _ uint32) [4]byte { return subSentinel }
	checkImage(t, readImage(t, dev, arr, 0, 0, 2, 2, 4), 0, 0, 2, 2, none)
	checkImage(t, readImage(t, dev, arr, 0, 1, 2, 2, 4), 0, 1, 2, 2, at(1, 0, [4]byte{0, 255, 0, 255}))
	for z := range uint32(3) {
		want := none
		if z == 2 {
			want = at(0, 1, [4]byte{0, 0, 255, 255})
		}
		checkImage(t, readImage(t, dev, vol, 0, z, 2, 2, 4), 0, z, 2, 2, want)
	}
	checkImage(t, readImage(t, dev, mip, 0, 0, 4, 4, 4), 0, 0, 4, 4, none)
	checkImage(t, readImage(t, dev, mip, 1, 0, 2, 2, 4), 1, 0, 2, 2, at(1, 1, [4]byte{255, 255, 0, 255}))
}
