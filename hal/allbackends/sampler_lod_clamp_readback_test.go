// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
	"github.com/gogpu/wgpu/hal"
)

// lodLevels is the mip level count of the sampled texture (8x8 down to 1x1).
const lodLevels = 4

// lodLevelColors gives every mip level its own RGBA8 color, packed as the
// little-endian word a readback returns.
var lodLevelColors = [lodLevels][4]byte{
	{255, 0, 0, 255},   // level 0: red
	{0, 255, 0, 255},   // level 1: green
	{0, 0, 255, 255},   // level 2: blue
	{255, 255, 0, 255}, // level 3: yellow
}

// lodClampShader samples the texture at its center with textureSampleLevel.
// The 64-pixel-wide target is split into four 16-pixel columns, and column c
// asks for level of detail c, so one draw reads lod 0, 1, 2 and 3.
const lodClampShader = `
@group(0) @binding(0) var t: texture_2d<f32>;
@group(0) @binding(1) var s: sampler;

struct VOut { @builtin(position) pos: vec4<f32> }

@vertex
fn vs(@builtin(vertex_index) v: u32) -> VOut {
    var quad = array<vec2<f32>, 6>(
        vec2<f32>(-1.0, -1.0), vec2<f32>(1.0, -1.0), vec2<f32>(-1.0, 1.0),
        vec2<f32>(1.0, -1.0), vec2<f32>(1.0, 1.0), vec2<f32>(-1.0, 1.0),
    );
    return VOut(vec4<f32>(quad[v % 6u], 0.0, 1.0));
}

@fragment
fn fs(in: VOut) -> @location(0) vec4<f32> {
    let lod = floor(in.pos.x / 16.0);
    return textureSampleLevel(t, s, vec2<f32>(0.5, 0.5), lod);
}
`

// lodClampBackends are the backends the LOD clamp test runs on: every
// overrideBackends entry, plus DX12 on WARP (the CPU adapter of a GPU-less
// CI runner), with FXC and with naga's DXIL.
var lodClampBackends = func() []lodClampBackend {
	out := make([]lodClampBackend, 0, len(overrideBackends)+2)
	for _, b := range overrideBackends {
		out = append(out, lodClampBackend{overrideBackend: b})
	}
	return append(out,
		lodClampBackend{overrideBackend: overrideBackend{name: "dx12-fxc-warp", backends: gputypes.BackendsDX12, backend: gputypes.BackendDX12}, warp: true},
		lodClampBackend{overrideBackend: overrideBackend{name: "dx12-dxil-warp", backends: gputypes.BackendsDX12, backend: gputypes.BackendDX12, dxil: true}, warp: true},
	)
}()

type lodClampBackend struct {
	overrideBackend
	warp bool // open the DX12 CPU adapter (WARP) instead of RequestAdapter's pick
}

// TestSamplerLodClamp samples a mip chain whose levels have distinct colors
// at lod 0..3 with four samplers and checks which level each lod read.
//
// The zero value of SamplerDescriptor.LodMaxClamp is the WebGPU default 32 on
// every backend: before the root package resolved it, DX12 passed MaxLOD 0
// and read the base level at every lod, and Metal ignored both clamps.
func TestSamplerLodClamp(t *testing.T) {
	nearest := wgpu.SamplerDescriptor{
		MagFilter: gputypes.FilterModeNearest, MinFilter: gputypes.FilterModeNearest,
		MipmapFilter: gputypes.FilterModeNearest,
	}
	lodMin1 := nearest
	lodMin1.LodMinClamp = 1
	lodMax1 := nearest
	lodMax1.LodMaxClamp = 1
	cases := []struct {
		name string
		desc *wgpu.SamplerDescriptor
		want [4]int // the mip level read at lod 0, 1, 2, 3
	}{
		{"nil descriptor", nil, [4]int{0, 1, 2, 3}},
		{"zero value", &wgpu.SamplerDescriptor{}, [4]int{0, 1, 2, 3}},
		{"nearest, zero clamps", &nearest, [4]int{0, 1, 2, 3}},
		{"lodMinClamp 1", &lodMin1, [4]int{1, 1, 2, 3}},
		{"lodMaxClamp 1", &lodMax1, [4]int{0, 1, 1, 1}},
	}
	forEachLodClampDevice(t, func(t *testing.T, device *wgpu.Device) {
		view := newLodChainView(t, device)
		frag := wgpu.ShaderStageFragment
		bgl := newBindGroupLayout(t, device,
			gputypes.BindGroupLayoutEntry{Binding: 0, Visibility: frag, Texture: &gputypes.TextureBindingLayout{
				SampleType: gputypes.TextureSampleTypeFloat, ViewDimension: gputypes.TextureViewDimension2D,
			}},
			gputypes.BindGroupLayoutEntry{Binding: 1, Visibility: frag, Sampler: &gputypes.SamplerBindingLayout{Type: gputypes.SamplerBindingTypeFiltering}},
		)
		pipeline := newRebindRenderPipeline(t, device, lodClampShader, bgl)
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				sampler, err := device.CreateSampler(c.desc)
				if err != nil {
					t.Fatalf("CreateSampler: %v", err)
				}
				t.Cleanup(sampler.Release)
				group := newBindGroup(t, device, bgl,
					wgpu.BindGroupEntry{Binding: 0, TextureView: view},
					wgpu.BindGroupEntry{Binding: 1, Sampler: sampler},
				)
				px := renderDynamicTarget(t, device, func(pass *wgpu.RenderPassEncoder) {
					pass.SetPipeline(pipeline)
					pass.SetBindGroup(0, group, nil)
					pass.Draw(gputypes.DrawArgs{VertexCount: 6, InstanceCount: 1})
				})
				row := featureTargetHeight / 2 * featureTargetWidth
				for lod, level := range c.want {
					got := px[row+lod*16+8]
					if want := lodLevelWord(level); got != want {
						t.Errorf("lod %d: pixel %#08x (level %d), want %#08x (level %d)", lod, got, lodLevelOf(got), want, level)
					}
				}
			})
		}
	})
}

// forEachLodClampDevice runs fn with a device of every lodClampBackends entry
// that has an adapter here, with the Vulkan validation layer and the DX12
// debug layer on.
func forEachLodClampDevice(t *testing.T, fn func(t *testing.T, device *wgpu.Device)) {
	t.Helper()
	for _, b := range lodClampBackends {
		t.Run(b.name, func(t *testing.T) {
			if b.dxil {
				t.Setenv("GOGPU_DX12_DXIL", "1")
				t.Setenv("GOGPU_DX12_DXIL_VALIDATE", "1")
			} else {
				t.Setenv("GOGPU_DX12_DXIL", "0")
			}
			if !b.warp {
				device, _ := openDebugBackendDevice(t, b.overrideBackend, 0, gputypes.PowerPreferenceLowPower)
				fn(t, device)
				return
			}
			fn(t, openWarpDevice(t, b.overrideBackend))
		})
	}
}

// openWarpDevice opens a public device on WARP, the DX12 CPU adapter.
// RequestAdapter picks WARP only on a machine without a DX12 GPU (it ranks
// the GPU first, and the software backend first under ForceFallbackAdapter),
// so the adapter is taken from a DX12 HAL instance and its device wrapped:
// CreateSampler still runs through the root package.
func openWarpDevice(t *testing.T, b overrideBackend) *wgpu.Device {
	t.Helper()
	backend, ok := hal.GetBackend(b.backend)
	if !ok {
		t.Skip("DX12 backend not registered on this platform")
	}
	instance, err := backend.CreateInstance(&hal.InstanceDescriptor{Backends: b.backends, Flags: enableDX12DebugLayer(t)})
	if err != nil {
		t.Skipf("CreateInstance: %v", err)
	}
	t.Cleanup(instance.Destroy)
	adapters := instance.EnumerateAdapters(nil)
	for i := range adapters {
		a := &adapters[i]
		if a.Info.DeviceType != gputypes.DeviceTypeCPU {
			continue
		}
		open, err := a.Adapter.Open(0, a.Capabilities.Limits)
		if err != nil {
			t.Fatalf("Open %q: %v", a.Info.Name, err)
		}
		device, err := wgpu.NewDeviceFromHAL(open.Device, open.Queue, 0, a.Capabilities.Limits, "lod-warp")
		if err != nil {
			t.Fatalf("NewDeviceFromHAL: %v", err)
		}
		t.Cleanup(device.Release)
		t.Logf("adapter: %s (%v)", a.Info.Name, a.Info.Backend)
		return device
	}
	t.Skip("no WARP adapter")
	return nil
}

// newLodChainView creates an 8x8 RGBA8 texture with lodLevels mip levels,
// fills each level with its lodLevelColors entry and returns a view of the
// whole chain.
func newLodChainView(t *testing.T, device *wgpu.Device) *wgpu.TextureView {
	t.Helper()
	const size = 8
	tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "lod-chain",
		Size:          wgpu.Extent3D{Width: size, Height: size, DepthOrArrayLayers: 1},
		MipLevelCount: lodLevels,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         wgpu.TextureUsageTextureBinding | wgpu.TextureUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	t.Cleanup(tex.Release)
	for level := range uint32(lodLevels) {
		side := uint32(size) >> level
		texels := make([]byte, side*side*4)
		for i := 0; i < len(texels); i += 4 {
			copy(texels[i:], lodLevelColors[level][:])
		}
		if err := device.Queue().WriteTexture(&wgpu.ImageCopyTexture{Texture: tex, MipLevel: level}, texels,
			&wgpu.ImageDataLayout{BytesPerRow: side * 4, RowsPerImage: side},
			&wgpu.Extent3D{Width: side, Height: side, DepthOrArrayLayers: 1}); err != nil {
			t.Fatalf("WriteTexture level %d: %v", level, err)
		}
	}
	view, err := device.CreateTextureView(tex, nil)
	if err != nil {
		t.Fatalf("CreateTextureView: %v", err)
	}
	t.Cleanup(view.Release)
	return view
}

func lodLevelWord(level int) uint32 {
	c := lodLevelColors[level]
	return uint32(c[0]) | uint32(c[1])<<8 | uint32(c[2])<<16 | uint32(c[3])<<24
}

// lodLevelOf names the level a read pixel came from, or -1.
func lodLevelOf(px uint32) int {
	for level := range lodLevels {
		if lodLevelWord(level) == px {
			return level
		}
	}
	return -1
}
