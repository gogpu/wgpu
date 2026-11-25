//go:build integration && darwin && !rust

package metal_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

const metalDepthBiasShader = `
fn corner(idx: u32) -> vec2<f32> {
    if (idx == 0u) { return vec2<f32>(-1.0, -1.0); }
    if (idx == 1u) { return vec2<f32>(3.0, -1.0); }
    return vec2<f32>(-1.0, 3.0);
}
@vertex fn vs_far(@builtin(vertex_index) idx: u32) -> @builtin(position) vec4<f32> {
    return vec4<f32>(corner(idx), 0.5, 1.0);
}
@vertex fn vs_near(@builtin(vertex_index) idx: u32) -> @builtin(position) vec4<f32> {
    return vec4<f32>(corner(idx), 0.4, 1.0);
}
@vertex fn vs_behind(@builtin(vertex_index) idx: u32) -> @builtin(position) vec4<f32> {
    return vec4<f32>(corner(idx), 0.50003, 1.0);
}
@vertex fn vs_tilt(@builtin(vertex_index) idx: u32) -> @builtin(position) vec4<f32> {
    let c = corner(idx);
    return vec4<f32>(c, 0.5 + 0.1 * c.x, 1.0);
}
@fragment fn fs_red() -> @location(0) vec4<f32> { return vec4<f32>(1.0, 0.0, 0.0, 1.0); }
@fragment fn fs_green() -> @location(0) vec4<f32> { return vec4<f32>(0.0, 1.0, 0.0, 1.0); }
`

const (
	depthBiasTargetSize  = 64
	depthBiasBytesPerRow = 256 // depthBiasTargetSize*4 rounded up to the copy alignment.
	// metalBiasUnits is large enough to be many depth steps on Depth32Float:
	// enough to bring vs_behind, 3e-5 behind, in front.
	metalBiasUnits = -1000
	// metalSlopeScale pulls vs_tilt, whose depth changes by about 0.003 a
	// pixel, forward by about 0.0125, and a flat triangle not at all.
	metalSlopeScale = -4
	// metalBiasClamp holds a bias to 1e-5, short of the 3e-5 vs_behind
	// stands behind.
	metalBiasClamp = -1e-5
)

// metalDepthBiasCase draws a red triangle from first and then a green one
// from second, with the bias given, and names the middle pixel it expects.
// Each of the three arguments has a case that fails if that argument is
// lost or swapped with another.
type metalDepthBiasCase struct {
	name          string
	first, second string
	bias          int32
	slope, clamp  float32
	expected      string
}

func TestMetalDepthBiasReadback(t *testing.T) {
	cases := []metalDepthBiasCase{
		{"equal depth without bias", "vs_far", "vs_far", 0, 0, 0, "red"},
		{"equal depth with negative bias", "vs_far", "vs_far", metalBiasUnits, 0, 0, "green"},
		{"nearer depth without bias", "vs_far", "vs_near", 0, 0, 0, "green"},
		{"sloped equal depth with slope scale", "vs_tilt", "vs_tilt", 0, metalSlopeScale, 0, "green"},
		{"flat equal depth with slope scale only", "vs_far", "vs_far", 0, metalSlopeScale, 0, "red"},
		{"behind with negative bias", "vs_far", "vs_behind", metalBiasUnits, 0, 0, "green"},
		{"behind with negative bias clamped", "vs_far", "vs_behind", metalBiasUnits, 0, metalBiasClamp, "red"},
	}
	instance, err := wgpu.CreateInstance(&wgpu.InstanceDescriptor{Backends: wgpu.BackendsMetal})
	requireDepthBiasOperation(t, err)
	defer instance.Release()

	adapter, err := instance.RequestAdapter(nil)
	if err != nil {
		t.Skipf("Metal adapter unavailable: %v", err)
	}
	defer adapter.Release()
	if info := adapter.Info(); info.Backend != gputypes.BackendMetal {
		t.Skipf("native Metal adapter unavailable: %+v", info)
	}
	t.Logf("Metal adapter: %s", adapter.Info().Name)
	device, err := adapter.RequestDevice(nil)
	requireDepthBiasOperation(t, err)
	defer device.Release()

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := readMetalDepthBias(t, device, c)
			if got != c.expected {
				t.Fatalf("center pixel = %s, want %s", got, c.expected)
			}
		})
	}
}

func readMetalDepthBias(t *testing.T, device *wgpu.Device, c metalDepthBiasCase) string {
	t.Helper()
	extent := wgpu.Extent3D{Width: depthBiasTargetSize, Height: depthBiasTargetSize, DepthOrArrayLayers: 1}
	color, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Size: extent, MipLevelCount: 1, SampleCount: 1,
		Dimension: gputypes.TextureDimension2D, Format: gputypes.TextureFormatRGBA8Unorm,
		Usage: gputypes.TextureUsageRenderAttachment | gputypes.TextureUsageCopySrc,
	})
	requireDepthBiasOperation(t, err)
	defer color.Release()
	depth, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Size: extent, MipLevelCount: 1, SampleCount: 1,
		Dimension: gputypes.TextureDimension2D, Format: gputypes.TextureFormatDepth32Float,
		Usage: gputypes.TextureUsageRenderAttachment,
	})
	requireDepthBiasOperation(t, err)
	defer depth.Release()
	colorView, err := device.CreateTextureView(color, nil)
	requireDepthBiasOperation(t, err)
	defer colorView.Release()
	depthView, err := device.CreateTextureView(depth, nil)
	requireDepthBiasOperation(t, err)
	defer depthView.Release()
	staging, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Size: depthBiasBytesPerRow * depthBiasTargetSize, Usage: wgpu.BufferUsageCopyDst | wgpu.BufferUsageMapRead,
	})
	requireDepthBiasOperation(t, err)
	defer staging.Release()
	module, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{WGSL: metalDepthBiasShader})
	requireDepthBiasOperation(t, err)
	defer module.Release()
	layout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{})
	requireDepthBiasOperation(t, err)
	defer layout.Release()

	first := depthBiasPipeline(t, device, module, layout, c.first, "fs_red",
		metalDepthBiasCase{})
	defer first.Release()
	second := depthBiasPipeline(t, device, module, layout, c.second, "fs_green", c)
	defer second.Release()

	encoder, err := device.CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{})
	requireDepthBiasOperation(t, err)
	pass, err := encoder.BeginRenderPass(&wgpu.RenderPassDescriptor{
		ColorAttachments: []wgpu.RenderPassColorAttachment{{
			View: colorView, LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore,
		}},
		DepthStencilAttachment: &wgpu.RenderPassDepthStencilAttachment{
			View: depthView, DepthLoadOp: gputypes.LoadOpClear,
			DepthStoreOp: gputypes.StoreOpStore, DepthClearValue: 1,
		},
	})
	requireDepthBiasOperation(t, err)
	pass.SetPipeline(first)
	pass.Draw(gputypes.DrawArgs{VertexCount: 3, InstanceCount: 1})
	pass.SetPipeline(second)
	pass.Draw(gputypes.DrawArgs{VertexCount: 3, InstanceCount: 1})
	requireDepthBiasOperation(t, pass.End())
	encoder.CopyTextureToBuffer(color, staging, []wgpu.BufferTextureCopy{{
		BufferLayout: wgpu.ImageDataLayout{BytesPerRow: depthBiasBytesPerRow, RowsPerImage: depthBiasTargetSize},
		TextureBase:  wgpu.ImageCopyTexture{Texture: color},
		Size:         extent,
	}})
	commands, err := encoder.Finish()
	requireDepthBiasOperation(t, err)
	defer commands.Release()
	_, err = device.Queue().Submit(commands)
	requireDepthBiasOperation(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	length := uint64(depthBiasBytesPerRow * depthBiasTargetSize)
	requireDepthBiasOperation(t, staging.Map(ctx, wgpu.MapModeRead, 0, length))
	mapped, err := staging.MappedRange(0, length)
	requireDepthBiasOperation(t, err)
	offset := (depthBiasTargetSize/2)*depthBiasBytesPerRow + (depthBiasTargetSize/2)*4
	pixel := mapped.Bytes()[offset : offset+4]
	name := depthBiasPixel(pixel[0], pixel[1])
	requireDepthBiasOperation(t, staging.Unmap())
	return name
}

func depthBiasPipeline(t *testing.T, device *wgpu.Device, module *wgpu.ShaderModule, layout *wgpu.PipelineLayout, vertex, fragment string, bias metalDepthBiasCase) *wgpu.RenderPipeline {
	t.Helper()
	result, err := device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Layout: layout,
		Vertex: wgpu.VertexState{Module: module, EntryPoint: vertex},
		Fragment: &wgpu.FragmentState{
			Module: module, EntryPoint: fragment,
			Targets: []gputypes.ColorTargetState{{
				Format: gputypes.TextureFormatRGBA8Unorm, WriteMask: gputypes.ColorWriteMaskAll,
			}},
		},
		DepthStencil: &wgpu.DepthStencilState{
			Format: gputypes.TextureFormatDepth32Float, DepthWriteEnabled: true,
			DepthCompare: gputypes.CompareFunctionLess, DepthBias: bias.bias,
			DepthBiasSlopeScale: bias.slope, DepthBiasClamp: bias.clamp,
		},
	})
	requireDepthBiasOperation(t, err)
	return result
}

func depthBiasPixel(red, green byte) string {
	switch {
	case red > 200 && green < 50:
		return "red"
	case green > 200 && red < 50:
		return "green"
	}
	return fmt.Sprintf("r%d,g%d", red, green)
}

func requireDepthBiasOperation(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
