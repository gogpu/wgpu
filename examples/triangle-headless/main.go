// Command triangle-headless renders a triangle to an offscreen texture and
// writes the result to a PNG file. Useful for autonomous verification that
// the GPU stack can create a render pipeline and produce expected pixels —
// no window needed.
//
// Usage:
//
//	GOGPU_GRAPHICS_API=dx12 GOGPU_DX12_DXIL=1 go run . [output.png]
//
// Exit codes:
//
//	0 — rendered, PNG written, non-trivial pixel count found
//	1 — pipeline/render failed (D3D12 rejected DXIL, mapping failed, etc.)
//	2 — rendered but no non-background pixels (pipeline created but nothing drew)
package main

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"

	_ "github.com/gogpu/wgpu/hal/allbackends"
)

// Shader bisection table.
// Select via SHADER_LEVEL=0..2 env var. Default is 2 (full gogpu triangle).
//
// Level 0: no vertex input, single hardcoded position
// Level 1: vertex_index input + if/else (no dynamic array)
// Level 2: dynamic-indexed positions array (gogpu DrawTriangle default)
const shaderLevel0 = `
@vertex
fn vs_main() -> @builtin(position) vec4<f32> {
    return vec4<f32>(0.0, 0.0, 0.0, 1.0);
}

@fragment
fn fs_main() -> @location(0) vec4<f32> {
    return vec4<f32>(1.0, 0.0, 0.0, 1.0);
}
`

const shaderLevel1 = `
@vertex
fn vs_main(@builtin(vertex_index) idx: u32) -> @builtin(position) vec4<f32> {
    if (idx == 0u) { return vec4<f32>(0.0, 0.5, 0.0, 1.0); }
    if (idx == 1u) { return vec4<f32>(-0.5, -0.5, 0.0, 1.0); }
    return vec4<f32>(0.5, -0.5, 0.0, 1.0);
}

@fragment
fn fs_main() -> @location(0) vec4<f32> {
    return vec4<f32>(1.0, 0.0, 0.0, 1.0);
}
`

const shaderLevel2 = `
@vertex
fn vs_main(@builtin(vertex_index) idx: u32) -> @builtin(position) vec4<f32> {
    var positions = array<vec2<f32>, 3>(
        vec2<f32>(0.0, 0.5),
        vec2<f32>(-0.5, -0.5),
        vec2<f32>(0.5, -0.5)
    );
    return vec4<f32>(positions[idx], 0.0, 1.0);
}

@fragment
fn fs_main() -> @location(0) vec4<f32> {
    return vec4<f32>(1.0, 0.0, 0.0, 1.0);
}
`

func pickShader() (string, string) {
	switch os.Getenv("SHADER_LEVEL") {
	case "0":
		return shaderLevel0, "level0-no-input"
	case "1":
		return shaderLevel1, "level1-if-else"
	default:
		return shaderLevel2, "level2-dynamic-array"
	}
}

const (
	texWidth      = 256
	texHeight     = 256
	bytesPerPixel = 4 // RGBA8Unorm
)

func main() {
	outputPath := "triangle.png"
	if len(os.Args) > 1 {
		outputPath = os.Args[1]
	}
	if err := run(outputPath); err != nil {
		log.Fatalf("FATAL: %v", err)
	}
}

func run(outputPath string) error {
	fmt.Println("=== Headless Triangle ===")

	device, cleanup, err := initDevice()
	if err != nil {
		return err
	}
	defer cleanup()

	// bytesPerRow must be a multiple of 256 per D3D12 copy alignment.
	bytesPerRow := align(texWidth*bytesPerPixel, 256)
	bufferSize := uint64(bytesPerRow * texHeight)

	texture, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label: "render-target",
		Size: wgpu.Extent3D{
			Width:              texWidth,
			Height:             texHeight,
			DepthOrArrayLayers: 1,
		},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         gputypes.TextureUsageRenderAttachment | gputypes.TextureUsageCopySrc,
	})
	if err != nil {
		return fmt.Errorf("create texture: %w", err)
	}
	defer texture.Release()

	view, err := device.CreateTextureView(texture, nil)
	if err != nil {
		return fmt.Errorf("create view: %w", err)
	}
	defer view.Release()

	stagingBuf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "readback",
		Size:  bufferSize,
		Usage: wgpu.BufferUsageCopyDst | wgpu.BufferUsageMapRead,
	})
	if err != nil {
		return fmt.Errorf("create staging: %w", err)
	}
	defer stagingBuf.Release()

	if err := renderTriangle(device, view, texture, stagingBuf, bytesPerRow); err != nil {
		return err
	}

	pixels, err := readbackPixels(device, stagingBuf, bufferSize)
	if err != nil {
		return err
	}

	return writeImage(filepath.Clean(outputPath), pixels, bytesPerRow)
}

// renderTriangle creates the shader/pipeline, records a render pass drawing a
// triangle, copies the result into stagingBuf, and submits to the GPU.
func renderTriangle(device *wgpu.Device, view *wgpu.TextureView, texture *wgpu.Texture, stagingBuf *wgpu.Buffer, bytesPerRow uint32) error {
	wgsl, shaderName := pickShader()
	fmt.Printf("Shader: %s\n", shaderName)
	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label: "triangle",
		WGSL:  wgsl,
	})
	if err != nil {
		return fmt.Errorf("create shader: %w", err)
	}
	defer shader.Release()

	pipelineLayout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{
		Label: "triangle-layout",
	})
	if err != nil {
		return fmt.Errorf("create pipeline layout: %w", err)
	}
	defer pipelineLayout.Release()

	pipeline, err := device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label:  "triangle",
		Layout: pipelineLayout,
		Vertex: wgpu.VertexState{
			Module:     shader,
			EntryPoint: "vs_main",
		},
		Fragment: &wgpu.FragmentState{
			Module:     shader,
			EntryPoint: "fs_main",
			Targets: []gputypes.ColorTargetState{
				{
					Format:    gputypes.TextureFormatRGBA8Unorm,
					WriteMask: gputypes.ColorWriteMaskAll,
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("create pipeline: %w", err)
	}
	defer pipeline.Release()

	fmt.Println("Pipeline created — DXIL accepted by D3D12")

	encoder, err := device.CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{
		Label: "triangle-encoder",
	})
	if err != nil {
		return fmt.Errorf("create encoder: %w", err)
	}

	pass, err := encoder.BeginRenderPass(&wgpu.RenderPassDescriptor{
		ColorAttachments: []wgpu.RenderPassColorAttachment{
			{
				View:       view,
				LoadOp:     gputypes.LoadOpClear,
				StoreOp:    gputypes.StoreOpStore,
				ClearValue: gputypes.Color{R: 0.15, G: 0.15, B: 0.15, A: 1.0},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("begin render pass: %w", err)
	}

	pass.SetPipeline(pipeline)
	pass.Draw(3, 1, 0, 0)

	if err := pass.End(); err != nil {
		return fmt.Errorf("end render pass: %w", err)
	}

	encoder.CopyTextureToBuffer(texture, stagingBuf, []wgpu.BufferTextureCopy{
		{
			BufferLayout: wgpu.ImageDataLayout{
				Offset:       0,
				BytesPerRow:  bytesPerRow,
				RowsPerImage: texHeight,
			},
			TextureBase: wgpu.ImageCopyTexture{
				Texture: texture,
			},
			Size: wgpu.Extent3D{
				Width:              texWidth,
				Height:             texHeight,
				DepthOrArrayLayers: 1,
			},
		},
	})

	cmd, err := encoder.Finish()
	if err != nil {
		return fmt.Errorf("finish encoder: %w", err)
	}

	if _, err := device.Queue().Submit(cmd); err != nil {
		return fmt.Errorf("submit: %w", err)
	}
	return nil
}

// readbackPixels maps the staging buffer and copies the pixel data to a byte slice.
func readbackPixels(device *wgpu.Device, stagingBuf *wgpu.Buffer, bufferSize uint64) ([]byte, error) {
	_ = device // reserved for future use (e.g. device.Poll)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := stagingBuf.Map(ctx, wgpu.MapModeRead, 0, bufferSize); err != nil {
		return nil, fmt.Errorf("map staging: %w", err)
	}
	rng, err := stagingBuf.MappedRange(0, bufferSize)
	if err != nil {
		_ = stagingBuf.Unmap()
		return nil, fmt.Errorf("mapped range: %w", err)
	}

	pixels := make([]byte, bufferSize)
	copy(pixels, rng.Bytes())
	if err := stagingBuf.Unmap(); err != nil {
		return nil, fmt.Errorf("unmap: %w", err)
	}
	return pixels, nil
}

// writeImage converts raw RGBA pixels to a PNG file and verifies the triangle
// rendered non-background pixels.
func writeImage(outputPath string, pixels []byte, bytesPerRow uint32) error {
	img := image.NewNRGBA(image.Rect(0, 0, texWidth, texHeight))
	nonBg := 0
	for y := 0; y < texHeight; y++ {
		for x := 0; x < texWidth; x++ {
			srcOff := uint32(y)*bytesPerRow + uint32(x)*bytesPerPixel
			r, g, b, a := pixels[srcOff], pixels[srcOff+1], pixels[srcOff+2], pixels[srcOff+3]
			img.SetNRGBA(x, y, color.NRGBA{R: r, G: g, B: b, A: a})
			if r > 50 || g > 50 || b > 50 {
				if !isBackground(r, g, b) {
					nonBg++
				}
			}
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return fmt.Errorf("encode png: %w", err)
	}
	if err := os.WriteFile(outputPath, buf.Bytes(), 0o600); err != nil {
		return fmt.Errorf("write png: %w", err)
	}
	fmt.Printf("PNG written: %s (%d bytes)\n", outputPath, buf.Len())
	fmt.Printf("Non-background pixels: %d / %d\n", nonBg, texWidth*texHeight)

	if nonBg == 0 {
		return fmt.Errorf("no non-background pixels — triangle did not render")
	}
	fmt.Println("SUCCESS: triangle visible in output")
	return nil
}

func isBackground(r, g, b byte) bool {
	// Background is (0.15, 0.15, 0.15, 1.0) → roughly (38, 38, 38) in RGBA8.
	return r < 50 && g < 50 && b < 50
}

func align(n uint32, a uint32) uint32 {
	return (n + a - 1) / a * a
}

func initDevice() (*wgpu.Device, func(), error) {
	backends := wgpu.BackendsAll
	if s := os.Getenv("GOGPU_GRAPHICS_API"); s != "" {
		switch s {
		case "dx12", "d3d12":
			backends = wgpu.BackendsDX12
		case "vulkan", "vk":
			backends = wgpu.BackendsVulkan
		case "metal":
			backends = wgpu.BackendsMetal
		case "gl", "gles":
			backends = wgpu.BackendsGL
		}
	}
	instance, err := wgpu.CreateInstance(&wgpu.InstanceDescriptor{
		Backends: backends,
		Flags:    gputypes.InstanceFlagsDebug,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("CreateInstance: %w", err)
	}

	adapter, err := instance.RequestAdapter(nil)
	if err != nil {
		instance.Release()
		return nil, nil, fmt.Errorf("RequestAdapter: %w", err)
	}
	fmt.Printf("Adapter: %s (%v)\n", adapter.Info().Name, adapter.Info().Backend)

	device, err := adapter.RequestDevice(nil)
	if err != nil {
		adapter.Release()
		instance.Release()
		return nil, nil, fmt.Errorf("RequestDevice: %w", err)
	}

	cleanup := func() {
		device.Release()
		adapter.Release()
		instance.Release()
	}
	return device, cleanup, nil
}
