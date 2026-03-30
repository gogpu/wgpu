//go:build windows

// Command wgpu-triangle tests the wgpu public API rendering pipeline.
// Single-threaded — validates wgpu API works correctly.
package main

import (
	"fmt"
	"log"
	"os"
	"runtime"
	"time"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
	_ "github.com/gogpu/wgpu/hal/vulkan"
)

func init() {
	runtime.LockOSThread()
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: %v\n", err)
		os.Exit(1)
	}
}

//nolint:funlen // example code — intentionally sequential
func run() error {
	log.Println("=== wgpu Public API Triangle Test ===")

	window, err := NewWindow("wgpu API Triangle Test", 800, 600)
	if err != nil {
		return fmt.Errorf("window: %w", err)
	}
	defer window.Destroy()

	instance, err := wgpu.CreateInstance(&wgpu.InstanceDescriptor{Backends: gputypes.BackendsVulkan})
	if err != nil {
		return fmt.Errorf("instance: %w", err)
	}

	surface, err := instance.CreateSurface(0, window.Handle())
	if err != nil {
		return fmt.Errorf("surface: %w", err)
	}

	adapter, err := instance.RequestAdapter(nil)
	if err != nil {
		return fmt.Errorf("adapter: %w", err)
	}
	log.Printf("Adapter: %s", adapter.Info().Name)

	device, err := adapter.RequestDevice(nil)
	if err != nil {
		return fmt.Errorf("device: %w", err)
	}

	w, h := window.Size()
	err = surface.Configure(device, &wgpu.SurfaceConfiguration{
		Format:      gputypes.TextureFormatBGRA8Unorm,
		Usage:       gputypes.TextureUsageRenderAttachment,
		Width:       safeUint32(w),
		Height:      safeUint32(h),
		PresentMode: gputypes.PresentModeFifo,
		AlphaMode:   gputypes.CompositeAlphaModeOpaque,
	})
	if err != nil {
		return fmt.Errorf("configure: %w", err)
	}

	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label: "Triangle",
		WGSL:  triangleShaderWGSL,
	})
	if err != nil {
		return fmt.Errorf("shader: %w", err)
	}

	pipelineLayout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{Label: "Layout"})
	if err != nil {
		return fmt.Errorf("layout: %w", err)
	}

	pipeline, err := device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label:  "Triangle",
		Layout: pipelineLayout,
		Vertex: wgpu.VertexState{Module: shader, EntryPoint: "vs_main"},
		Fragment: &wgpu.FragmentState{
			Module: shader, EntryPoint: "fs_main",
			Targets: []gputypes.ColorTargetState{{
				Format: gputypes.TextureFormatBGRA8Unorm, WriteMask: gputypes.ColorWriteMaskAll,
			}},
		},
	})
	if err != nil {
		return fmt.Errorf("pipeline: %w", err)
	}

	log.Println("Render loop started")
	frameCount := 0
	startTime := time.Now()

	for window.PollEvents() {
		surfaceTex, _, err := surface.GetCurrentTexture()
		if err != nil {
			continue
		}
		view, err := surfaceTex.CreateView(nil)
		if err != nil {
			surface.DiscardTexture()
			continue
		}
		encoder, err := device.CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{Label: "Frame"})
		if err != nil {
			view.Release()
			continue
		}
		renderPass, err := encoder.BeginRenderPass(&wgpu.RenderPassDescriptor{
			ColorAttachments: []wgpu.RenderPassColorAttachment{{
				View: view, LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore,
				ClearValue: gputypes.Color{R: 0, G: 0, B: 0.5, A: 1},
			}},
		})
		if err != nil {
			view.Release()
			continue
		}
		renderPass.SetPipeline(pipeline)
		renderPass.Draw(3, 1, 0, 0)
		_ = renderPass.End()
		commands, _ := encoder.Finish()
		_, _ = device.Queue().Submit(commands)
		_ = surface.Present(surfaceTex)
		view.Release()

		frameCount++
		if frameCount%60 == 0 {
			fps := float64(frameCount) / time.Since(startTime).Seconds()
			log.Printf("Frame %d (%.1f FPS)", frameCount, fps)
		}
	}

	pipeline.Release()
	pipelineLayout.Release()
	shader.Release()

	log.Printf("Done. %d frames", frameCount)
	return nil
}

const triangleShaderWGSL = `
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

func safeUint32(v int32) uint32 {
	if v < 0 {
		return 0
	}
	return uint32(v)
}
