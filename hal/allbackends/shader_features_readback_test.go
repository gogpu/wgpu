// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"encoding/binary"
	"math"
	"strings"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// These tests prove each shader capability feature (P1-API-008) by readback
// on every backend and adapter that offers it: a backend offers a feature
// only where one of these tests passes on it. Vulkan runs under the
// validation layer, DX12's DXIL path under the DXIL validator.

// featureBackend is one backend configuration a feature test runs on.
type featureBackend struct {
	overrideBackend
	dxilValidate bool
}

var featureBackends = []featureBackend{
	{overrideBackend: overrideBackend{name: "vulkan", backends: gputypes.BackendsVulkan, backend: gputypes.BackendVulkan}},
	{overrideBackend: overrideBackend{name: "dx12-fxc", backends: gputypes.BackendsDX12, backend: gputypes.BackendDX12}},
	{overrideBackend: overrideBackend{name: "dx12-dxil", backends: gputypes.BackendsDX12, backend: gputypes.BackendDX12, dxil: true}, dxilValidate: true},
	{overrideBackend: overrideBackend{name: "gles", backends: gputypes.BackendsGL, backend: gputypes.BackendGL}},
}

var featurePowerPreferences = []struct {
	name string
	pref gputypes.PowerPreference
}{
	{"low-power", gputypes.PowerPreferenceLowPower},
	{"high-performance", gputypes.PowerPreferenceHighPerformance},
}

// forEachFeatureDevice runs fn on a device with features enabled, for every
// backend and adapter that offers all of them; the others are skipped.
func forEachFeatureDevice(t *testing.T, features gputypes.Features, required gputypes.DownlevelFlags, fn func(t *testing.T, device *wgpu.Device)) {
	t.Helper()
	forEachFeatureDeviceWith(t, features, 0, required, fn)
}

// forEachFeatureDeviceWith is forEachFeatureDevice that also enables the
// optional features where the adapter offers them.
func forEachFeatureDeviceWith(t *testing.T, features, optional gputypes.Features, required gputypes.DownlevelFlags, fn func(t *testing.T, device *wgpu.Device)) {
	t.Helper()
	for _, b := range featureBackends {
		for _, pref := range featurePowerPreferences {
			t.Run(b.name+"/"+pref.name, func(t *testing.T) {
				if b.dxil {
					t.Setenv("GOGPU_DX12_DXIL", "1")
				} else {
					t.Setenv("GOGPU_DX12_DXIL", "0")
				}
				if b.dxilValidate {
					t.Setenv("GOGPU_DX12_DXIL_VALIDATE", "1")
				}
				fn(t, openFeatureDevice(t, b.overrideBackend, pref.pref, features, optional, required))
			})
		}
	}
}

func openFeatureDevice(t *testing.T, b overrideBackend, pref gputypes.PowerPreference, features, optional gputypes.Features, required gputypes.DownlevelFlags) *wgpu.Device {
	t.Helper()
	var flags gputypes.InstanceFlags
	if b.backend == gputypes.BackendVulkan {
		flags = enableVulkanValidation(t)
	}
	instance, err := wgpu.CreateInstance(&wgpu.InstanceDescriptor{Backends: b.backends, Flags: flags})
	if err != nil {
		t.Skipf("CreateInstance: %v", err)
	}
	t.Cleanup(instance.Release)
	adapter, err := instance.RequestAdapter(&wgpu.RequestAdapterOptions{PowerPreference: pref})
	if err != nil {
		t.Skipf("no adapter: %v", err)
	}
	t.Cleanup(adapter.Release)
	info := adapter.Info()
	if info.Backend != b.backend {
		t.Skipf("no %s adapter (got %s, backend %v)", b.name, info.Name, info.Backend)
	}
	if missing := features &^ adapter.Features(); missing != 0 {
		t.Skipf("%s does not offer %v", info.Name, missing)
	}
	if missing := required &^ adapter.DownlevelCapabilities().Flags; missing != 0 {
		t.Skipf("%s lacks downlevel flags %#x", info.Name, uint32(missing))
	}
	device, err := adapter.RequestDevice(&wgpu.DeviceDescriptor{Label: "shader-features", RequiredFeatures: features | optional&adapter.Features()})
	if err != nil {
		t.Fatalf("RequestDevice(%v) on %s: %v", features, info.Name, err)
	}
	t.Cleanup(device.Release)
	t.Logf("adapter: %s (%v)", info.Name, info.Backend)
	return device
}

// The target is 64x8: a 64-pixel RGBA8 row is 256 bytes, the bytesPerRow
// alignment a texture-to-buffer copy needs.
const (
	featureTargetWidth  = 64
	featureTargetHeight = 8
)

// renderFeatureTarget draws with the shader's vs/fs entry points into an
// 8x8 RGBA8 target cleared to white and returns its pixels, row-major.
func renderFeatureTarget(t *testing.T, device *wgpu.Device, wgsl string, draw func(pass *wgpu.RenderPassEncoder)) []uint32 {
	t.Helper()
	target, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "feature-target",
		Size:          wgpu.Extent3D{Width: featureTargetWidth, Height: featureTargetHeight, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         wgpu.TextureUsageRenderAttachment | wgpu.TextureUsageCopySrc,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	t.Cleanup(target.Release)
	view, err := device.CreateTextureView(target, nil)
	if err != nil {
		t.Fatalf("CreateTextureView: %v", err)
	}
	t.Cleanup(view.Release)
	pipeline := newOverrideRenderPipeline(t, device, wgsl)

	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	pass, err := enc.BeginRenderPass(&wgpu.RenderPassDescriptor{
		ColorAttachments: []wgpu.RenderPassColorAttachment{{
			View: view, LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore,
			ClearValue: gputypes.Color{R: 1, G: 1, B: 1, A: 1},
		}},
	})
	if err != nil {
		t.Fatalf("BeginRenderPass: %v", err)
	}
	pass.SetPipeline(pipeline)
	draw(pass)
	if err := pass.End(); err != nil {
		t.Fatalf("End: %v", err)
	}
	const words = featureTargetWidth * featureTargetHeight
	staging := newPublicBuffer(t, device, words*4, wgpu.BufferUsageMapRead|wgpu.BufferUsageCopyDst)
	enc.CopyTextureToBuffer(target, staging, []wgpu.BufferTextureCopy{{
		BufferLayout: wgpu.ImageDataLayout{BytesPerRow: featureTargetWidth * 4, RowsPerImage: featureTargetHeight},
		TextureBase:  wgpu.ImageCopyTexture{Texture: target},
		Size:         wgpu.Extent3D{Width: featureTargetWidth, Height: featureTargetHeight, DepthOrArrayLayers: 1},
	}})
	cmd, err := enc.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if _, err := device.Queue().Submit(cmd); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	return readPublicBuffer(t, staging, words)
}

// red returns the red channel of an RGBA8 pixel word.
func red(px uint32) uint32 { return px & 0xff }

const primitiveIndexShader = `enable primitive_index;

// Two triangles: primitive 0 covers the left part of the middle row,
// primitive 1 the right part.
@vertex
fn vs(@builtin(vertex_index) i: u32) -> @builtin(position) vec4f {
    var p = array<vec2f, 6>(
        vec2f(-1.0, -1.0), vec2f(0.0, -1.0), vec2f(-1.0, 3.0),
        vec2f(0.0, -1.0), vec2f(1.0, -1.0), vec2f(0.0, 3.0),
    );
    return vec4f(p[i], 0.0, 1.0);
}

@fragment
fn fs(@builtin(primitive_index) prim: u32) -> @location(0) vec4f {
    return vec4f(f32(prim) / 255.0, 0.0, 0.0, 1.0);
}
`

// TestFeaturePrimitiveIndex reads @builtin(primitive_index) back from two
// triangles of one draw.
func TestFeaturePrimitiveIndex(t *testing.T) {
	forEachFeatureDevice(t, gputypes.Features(gputypes.FeaturePrimitiveIndex), 0, func(t *testing.T, device *wgpu.Device) {
		px := renderFeatureTarget(t, device, primitiveIndexShader, func(pass *wgpu.RenderPassEncoder) {
			pass.Draw(gputypes.DrawArgs{VertexCount: 6, InstanceCount: 1})
		})
		row := featureTargetHeight / 2 * featureTargetWidth
		if got := red(px[row+8]); got != 0 {
			t.Errorf("left triangle: primitive_index %d (pixel %#08x), want 0", got, px[row+8])
		}
		if got := red(px[row+40]); got != 1 {
			t.Errorf("right triangle: primitive_index %d (pixel %#08x), want 1", got, px[row+40])
		}
	})
}

const clipDistancesShader = `enable clip_distances;

struct VOut {
    @builtin(position) pos: vec4f,
    @builtin(clip_distances) clip: array<f32, 1>,
}

// A full-screen triangle whose clip distance is its x: everything left of
// the vertical center line is clipped.
@vertex
fn vs(@builtin(vertex_index) i: u32) -> VOut {
    var p = array<vec2f, 3>(vec2f(-1.0, -1.0), vec2f(3.0, -1.0), vec2f(-1.0, 3.0));
    return VOut(vec4f(p[i], 0.0, 1.0), array<f32, 1>(p[i].x));
}

@fragment
fn fs() -> @location(0) vec4f {
    return vec4f(0.0, 1.0, 0.0, 1.0);
}
`

// TestFeatureClipDistances clips half of a full-screen triangle with a
// negative clip distance.
func TestFeatureClipDistances(t *testing.T) {
	forEachFeatureDevice(t, gputypes.Features(gputypes.FeatureClipDistances), 0, func(t *testing.T, device *wgpu.Device) {
		px := renderFeatureTarget(t, device, clipDistancesShader, func(pass *wgpu.RenderPassEncoder) {
			pass.Draw(gputypes.DrawArgs{VertexCount: 3, InstanceCount: 1})
		})
		const white, green = 0xffffffff, 0xff00ff00
		for y := 0; y < featureTargetHeight; y++ {
			for x := 0; x < featureTargetWidth; x++ {
				want := uint32(green)
				if x < featureTargetWidth/2 {
					want = white // clipped: the clear color stays
				}
				if got := px[y*featureTargetWidth+x]; got != want {
					t.Fatalf("pixel (%d,%d) = %#08x, want %#08x", x, y, got, want)
				}
			}
		}
	})
}

const int64Shader = `@group(0) @binding(0) var<storage, read_write> data: array<u32, 4>;

@compute @workgroup_size(1)
fn main() {
    let a = u64(data[0]) * 4000000000lu;
    data[1] = u32(a >> 32u);
    data[2] = u32(a & 0xFFFFFFFFlu);
    let s = -i64(data[0]) * 3000000000li;
    data[3] = bitcast<u32>(i32(s >> 32u));
}
`

// TestFeatureShaderInt64 checks u64 and i64 arithmetic past 32 bits.
func TestFeatureShaderInt64(t *testing.T) {
	forEachFeatureDevice(t, gputypes.Features(gputypes.FeatureShaderInt64), gputypes.DownlevelFlagsComputeShaders, func(t *testing.T, device *wgpu.Device) {
		out, err := runPublicCompute(t, device, int64Shader, nil, [][]uint32{{3, 0, 0, 0}})
		if err != nil {
			t.Fatal(err)
		}
		// 3 * 4e9 = 12e9 = 2<<32 + 3410065408; -3 * 3e9 >> 32 = -3.
		want := []uint32{3, 2, 3410065408, 0xFFFFFFFD}
		for i := range want {
			if out[0][i] != want[i] {
				t.Fatalf("data = %v, want %v", out[0], want)
			}
		}
	})
}

const int64AtomicMinMaxShader = `@group(0) @binding(0) var<storage, read_write> m: atomic<u64>;

@compute @workgroup_size(64)
fn main(@builtin(local_invocation_index) i: u32) {
    atomicMax(&m, u64(i) << 33u);
}
`

const int64AtomicAllOpsShader = `@group(0) @binding(0) var<storage, read_write> m: atomic<u64>;

@compute @workgroup_size(64)
fn main() {
    atomicAdd(&m, 1lu << 32u);
}
`

// TestFeatureShaderInt64Atomics checks atomicMax (MIN_MAX) and atomicAdd
// (ALL_OPS) on a u64 in a storage buffer, each with only its own feature.
func TestFeatureShaderInt64Atomics(t *testing.T) {
	for _, tc := range []struct {
		name    string
		feature gputypes.Feature
		wgsl    string
		want    [2]uint32 // low, high word
	}{
		{"min-max", gputypes.FeatureShaderInt64AtomicMinMax, int64AtomicMinMaxShader, [2]uint32{0, 63 << 1}},
		{"all-ops", gputypes.FeatureShaderInt64AtomicAllOps, int64AtomicAllOpsShader, [2]uint32{0, 64}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			forEachFeatureDevice(t, gputypes.Features(tc.feature)|gputypes.Features(gputypes.FeatureShaderInt64), gputypes.DownlevelFlagsComputeShaders, func(t *testing.T, device *wgpu.Device) {
				out, err := runPublicCompute(t, device, tc.wgsl, nil, [][]uint32{{0, 0}})
				if err != nil {
					t.Fatal(err)
				}
				if out[0][0] != tc.want[0] || out[0][1] != tc.want[1] {
					t.Fatalf("u64 words = %v, want %v", out[0], tc.want)
				}
			})
		})
	}
}

const float32AtomicShader = `@group(0) @binding(0) var<storage, read_write> f: atomic<f32>;

@compute @workgroup_size(64)
fn main() {
    atomicAdd(&f, 1.5);
}
`

// TestFeatureShaderFloat32Atomic checks atomicAdd on an f32 in a storage
// buffer from 64 invocations.
func TestFeatureShaderFloat32Atomic(t *testing.T) {
	forEachFeatureDevice(t, gputypes.Features(gputypes.FeatureShaderFloat32Atomic), gputypes.DownlevelFlagsComputeShaders, func(t *testing.T, device *wgpu.Device) {
		out, err := runPublicCompute(t, device, float32AtomicShader, nil, [][]uint32{{0}})
		if err != nil {
			t.Fatal(err)
		}
		if got := math.Float32frombits(out[0][0]); got != 96 {
			t.Fatalf("sum = %v, want 96", got)
		}
	})
}

const barycentricsShader = `@vertex
fn vs(@builtin(vertex_index) i: u32) -> @builtin(position) vec4f {
    var p = array<vec2f, 3>(vec2f(-1.0, -1.0), vec2f(3.0, -1.0), vec2f(-1.0, 3.0));
    return vec4f(p[i], 0.0, 1.0);
}

@fragment
fn fs(@builtin(barycentric) b: vec3f) -> @location(0) vec4f {
    return vec4f(b, 1.0);
}
`

const perVertexShader = `enable wgpu_per_vertex;

struct VOut {
    @builtin(position) pos: vec4f,
    @location(0) v: f32,
}

// Each vertex carries its own index scaled to a byte.
@vertex
fn vs(@builtin(vertex_index) i: u32) -> VOut {
    var p = array<vec2f, 3>(vec2f(-1.0, -1.0), vec2f(3.0, -1.0), vec2f(-1.0, 3.0));
    return VOut(vec4f(p[i], 0.0, 1.0), f32(i + 1u) * 10.0 / 255.0);
}

@fragment
fn fs(@location(0) @interpolate(per_vertex) v: array<f32, 3>) -> @location(0) vec4f {
    return vec4f(v[0], v[1], v[2], 1.0);
}
`

// TestFeatureShaderBarycentrics checks that @builtin(barycentric) weights
// sum to one and vary across the triangle.
func TestFeatureShaderBarycentrics(t *testing.T) {
	forEachFeatureDevice(t, gputypes.Features(gputypes.FeatureShaderBarycentrics), 0, func(t *testing.T, device *wgpu.Device) {
		px := renderFeatureTarget(t, device, barycentricsShader, func(pass *wgpu.RenderPassEncoder) {
			pass.Draw(gputypes.DrawArgs{VertexCount: 3, InstanceCount: 1})
		})
		distinct := map[uint32]bool{}
		for i, p := range px {
			sum := int(p&0xff) + int(p>>8&0xff) + int(p>>16&0xff)
			if sum < 252 || sum > 258 {
				t.Fatalf("pixel %d = %#08x: barycentric weights sum to %d/255, want 255", i, p, sum)
			}
			distinct[p&0xffffff] = true
		}
		if len(distinct) < featureTargetWidth {
			t.Fatalf("only %d distinct barycentric values across the target", len(distinct))
		}
	})
}

// TestFeatureShaderPerVertex reads the three per-vertex values of the
// triangle in the fragment shader: every pixel sees all three, in vertex
// order.
func TestFeatureShaderPerVertex(t *testing.T) {
	forEachFeatureDevice(t, gputypes.Features(gputypes.FeatureShaderPerVertex), 0, func(t *testing.T, device *wgpu.Device) {
		px := renderFeatureTarget(t, device, perVertexShader, func(pass *wgpu.RenderPassEncoder) {
			pass.Draw(gputypes.DrawArgs{VertexCount: 3, InstanceCount: 1})
		})
		const want = 0xff1e140a // R=10, G=20, B=30
		for i, p := range px {
			if p != want {
				t.Fatalf("pixel %d = %#08x, want %#08x", i, p, uint32(want))
			}
		}
	})
}

const drawIndexShader = `enable draw_index;

struct VOut {
    @builtin(position) pos: vec4f,
    @location(0) @interpolate(flat) draw: u32,
}

// Draw 0 covers the left part of the middle row, draw 1 the right part.
@vertex
fn vs(@builtin(vertex_index) i: u32, @builtin(draw_index) d: u32) -> VOut {
    var p = array<vec2f, 6>(
        vec2f(-1.0, -1.0), vec2f(0.0, -1.0), vec2f(-1.0, 3.0),
        vec2f(0.0, -1.0), vec2f(1.0, -1.0), vec2f(0.0, 3.0),
    );
    return VOut(vec4f(p[i], 0.0, 1.0), d);
}

@fragment
fn fs(in: VOut) -> @location(0) vec4f {
    return vec4f(f32(in.draw + 1u) / 255.0, 0.0, 0.0, 1.0);
}
`

// TestFeatureShaderDrawIndex reads @builtin(draw_index) back from the two
// draws of one MultiDrawIndirect, or, where the device has no
// FeatureMultiDrawIndirect, from two separate draws, which are each draw 0.
func TestFeatureShaderDrawIndex(t *testing.T) {
	features := gputypes.Features(gputypes.FeatureShaderDrawIndex)
	optional := gputypes.Features(gputypes.FeatureMultiDrawIndirect)
	forEachFeatureDeviceWith(t, features, optional, 0, func(t *testing.T, device *wgpu.Device) {
		multiDraw := device.Features().Contains(gputypes.FeatureMultiDrawIndirect)
		args := newPublicBuffer(t, device, 32, wgpu.BufferUsageIndirect|wgpu.BufferUsageCopyDst)
		words := []uint32{3, 1, 0, 0, 3, 1, 3, 0} // vertexCount, instanceCount, firstVertex, firstInstance
		data := make([]byte, len(words)*4)
		for i, w := range words {
			binary.LittleEndian.PutUint32(data[i*4:], w)
		}
		if err := device.Queue().WriteBuffer(args, 0, data); err != nil {
			t.Fatalf("WriteBuffer: %v", err)
		}
		px := renderFeatureTarget(t, device, drawIndexShader, func(pass *wgpu.RenderPassEncoder) {
			if multiDraw {
				pass.MultiDrawIndirect(args, 0, 2)
			} else {
				// GLES has no multi-draw, and its DrawIndirect is a no-op.
				pass.Draw(gputypes.DrawArgs{VertexCount: 3, InstanceCount: 1})
				pass.Draw(gputypes.DrawArgs{VertexCount: 3, InstanceCount: 1, FirstVertex: 3})
			}
		})
		secondWant := uint32(2)
		if !multiDraw {
			secondWant = 1
		}
		row := featureTargetHeight / 2 * featureTargetWidth
		if got := red(px[row+8]); got != 1 {
			t.Errorf("first draw: draw_index+1 = %d (pixel %#08x), want 1", got, px[row+8])
		}
		if got := red(px[row+40]); got != secondWant {
			t.Errorf("second draw (multi-draw %v): draw_index+1 = %d (pixel %#08x), want %d", multiDraw, got, px[row+40], secondWant)
		}
	})
}

// earlyDepthShader counts fragment invocations. Every fragment fails the
// depth test, so with forced early tests none runs; without them the
// storage write keeps the depth test after the shader and every one runs.
const earlyDepthShader = `@group(0) @binding(0) var<storage, read_write> count: atomic<u32>;

@vertex
fn vs(@builtin(vertex_index) i: u32) -> @builtin(position) vec4f {
    var p = array<vec2f, 3>(vec2f(-1.0, -1.0), vec2f(3.0, -1.0), vec2f(-1.0, 3.0));
    return vec4f(p[i], 0.5, 1.0);
}

@fragment ATTRIBUTE
fn fs() -> @location(0) vec4f {
    atomicAdd(&count, 1u);
    return vec4f(1.0);
}
`

// TestFeatureShaderEarlyDepthTest proves @early_depth_test(force): with it a
// fragment that fails the depth test does not run, without it it does.
func TestFeatureShaderEarlyDepthTest(t *testing.T) {
	forEachFeatureDevice(t, gputypes.Features(gputypes.FeatureShaderEarlyDepthTest), gputypes.DownlevelFlagsFragmentWritableStorage, func(t *testing.T, device *wgpu.Device) {
		if got := countDepthFailingFragments(t, device, "@early_depth_test(force)"); got != 0 {
			t.Errorf("forced early depth test: %d fragments ran, want 0", got)
		}
		// Control: the same draw without the attribute runs the shader, so
		// the case above can fail.
		if got := countDepthFailingFragments(t, device, ""); got == 0 {
			t.Errorf("without early depth test: no fragment ran, want %d", featureTargetWidth*featureTargetHeight)
		}
	})
}

func countDepthFailingFragments(t *testing.T, device *wgpu.Device, attribute string) uint32 {
	t.Helper()
	wgsl := strings.Replace(earlyDepthShader, "ATTRIBUTE", attribute, 1)
	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{Label: "early-depth", WGSL: wgsl})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	t.Cleanup(shader.Release)
	bgl, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{Entries: []gputypes.BindGroupLayoutEntry{{
		Binding: 0, Visibility: gputypes.ShaderStageFragment,
		Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeStorage},
	}}})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout: %v", err)
	}
	t.Cleanup(bgl.Release)
	layout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{BindGroupLayouts: []*wgpu.BindGroupLayout{bgl}})
	if err != nil {
		t.Fatalf("CreatePipelineLayout: %v", err)
	}
	t.Cleanup(layout.Release)
	pipeline, err := device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label:  "early-depth",
		Layout: layout,
		Vertex: wgpu.VertexState{Module: shader, EntryPoint: "vs"},
		// A masked color target: core refuses a fragment stage without one
		// even when a depth attachment is the target (Rust accepts it).
		Fragment: &wgpu.FragmentState{Module: shader, EntryPoint: "fs", Targets: []gputypes.ColorTargetState{
			{Format: gputypes.TextureFormatRGBA8Unorm, WriteMask: gputypes.ColorWriteMaskNone},
		}},
		DepthStencil: &wgpu.DepthStencilState{
			Format: gputypes.TextureFormatDepth32Float, DepthCompare: gputypes.CompareFunctionLess,
			StencilFront: keepStencil, StencilBack: keepStencil,
		},
		Primitive:   gputypes.PrimitiveState{Topology: gputypes.PrimitiveTopologyTriangleList, CullMode: gputypes.CullModeNone},
		Multisample: gputypes.MultisampleState{Count: 1, Mask: 0xFFFFFFFF},
	})
	if err != nil {
		t.Fatalf("CreateRenderPipeline: %v", err)
	}
	t.Cleanup(pipeline.Release)

	counter := newPublicBuffer(t, device, 4, wgpu.BufferUsageStorage|wgpu.BufferUsageCopySrc|wgpu.BufferUsageCopyDst)
	if err := device.Queue().WriteBuffer(counter, 0, make([]byte, 4)); err != nil {
		t.Fatalf("WriteBuffer: %v", err)
	}
	bg, err := device.CreateBindGroup(&wgpu.BindGroupDescriptor{Layout: bgl, Entries: []wgpu.BindGroupEntry{{Binding: 0, Buffer: counter, Size: 4}}})
	if err != nil {
		t.Fatalf("CreateBindGroup: %v", err)
	}
	t.Cleanup(bg.Release)
	depth, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label: "early-depth", Size: wgpu.Extent3D{Width: featureTargetWidth, Height: featureTargetHeight, DepthOrArrayLayers: 1},
		MipLevelCount: 1, SampleCount: 1, Dimension: gputypes.TextureDimension2D,
		Format: gputypes.TextureFormatDepth32Float, Usage: wgpu.TextureUsageRenderAttachment,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	t.Cleanup(depth.Release)
	depthView, err := device.CreateTextureView(depth, nil)
	if err != nil {
		t.Fatalf("CreateTextureView: %v", err)
	}
	t.Cleanup(depthView.Release)

	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	color, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label: "early-depth-color", Size: wgpu.Extent3D{Width: featureTargetWidth, Height: featureTargetHeight, DepthOrArrayLayers: 1},
		MipLevelCount: 1, SampleCount: 1, Dimension: gputypes.TextureDimension2D,
		Format: gputypes.TextureFormatRGBA8Unorm, Usage: wgpu.TextureUsageRenderAttachment,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	t.Cleanup(color.Release)
	colorView, err := device.CreateTextureView(color, nil)
	if err != nil {
		t.Fatalf("CreateTextureView: %v", err)
	}
	t.Cleanup(colorView.Release)

	pass, err := enc.BeginRenderPass(&wgpu.RenderPassDescriptor{
		ColorAttachments: []wgpu.RenderPassColorAttachment{{
			View: colorView, LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore,
		}},
		DepthStencilAttachment: &wgpu.RenderPassDepthStencilAttachment{
			View: depthView, DepthLoadOp: gputypes.LoadOpClear, DepthStoreOp: gputypes.StoreOpStore, DepthClearValue: 0,
		},
	})
	if err != nil {
		t.Fatalf("BeginRenderPass: %v", err)
	}
	pass.SetPipeline(pipeline)
	pass.SetBindGroup(0, bg, nil)
	pass.Draw(gputypes.DrawArgs{VertexCount: 3, InstanceCount: 1})
	if err := pass.End(); err != nil {
		t.Fatalf("End: %v", err)
	}
	staging := newPublicBuffer(t, device, 4, wgpu.BufferUsageMapRead|wgpu.BufferUsageCopyDst)
	enc.CopyBufferToBuffer(counter, 0, staging, 0, 4)
	cmd, err := enc.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if _, err := device.Queue().Submit(cmd); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	return readPublicBuffer(t, staging, 1)[0]
}
