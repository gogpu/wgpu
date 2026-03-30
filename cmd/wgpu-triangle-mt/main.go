//go:build windows

// Command wgpu-triangle tests the wgpu public API rendering pipeline.
// Multi-threaded: main thread = window events, render thread = GPU ops.
// Same architecture as gogpu renderer.
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
	"github.com/gogpu/wgpu/internal/thread"
)

const (
	windowWidth  = 800
	windowHeight = 600
	windowTitle  = "wgpu API Triangle Test (Multi-Thread)"
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

//nolint:gocognit,gocyclo,cyclop,funlen // example code — intentionally sequential
func run() error {
	log.Println("=== wgpu Multi-Thread Triangle Test ===")

	// 1. Window (main thread)
	window, err := NewWindow(windowTitle, windowWidth, windowHeight)
	if err != nil {
		return fmt.Errorf("window: %w", err)
	}
	defer window.Destroy()
	log.Println("1. Window created")

	// 2. Render thread
	renderLoop := thread.NewRenderLoop()
	defer renderLoop.Stop()
	log.Println("2. Render thread created")

	// 3-9. Init GPU on render thread
	var instance *wgpu.Instance
	var surface *wgpu.Surface
	var device *wgpu.Device
	var pipeline *wgpu.RenderPipeline
	var pipelineLayout *wgpu.PipelineLayout
	var shader *wgpu.ShaderModule
	var initErr error

	renderLoop.RunOnRenderThreadVoid(func() {
		instance, err = wgpu.CreateInstance(&wgpu.InstanceDescriptor{
			Backends: gputypes.BackendsVulkan,
		})
		if err != nil {
			initErr = fmt.Errorf("instance: %w", err)
			return
		}

		surface, err = instance.CreateSurface(0, window.Handle())
		if err != nil {
			initErr = fmt.Errorf("surface: %w", err)
			return
		}

		adapter, err := instance.RequestAdapter(nil)
		if err != nil {
			initErr = fmt.Errorf("adapter: %w", err)
			return
		}
		log.Printf("   Adapter: %s", adapter.Info().Name)

		device, err = adapter.RequestDevice(nil)
		if err != nil {
			initErr = fmt.Errorf("device: %w", err)
			return
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
			initErr = fmt.Errorf("configure: %w", err)
			return
		}

		shader, err = device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
			Label: "Triangle",
			WGSL:  triangleShaderWGSL,
		})
		if err != nil {
			initErr = fmt.Errorf("shader: %w", err)
			return
		}

		pipelineLayout, err = device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{
			Label: "Triangle Layout",
		})
		if err != nil {
			initErr = fmt.Errorf("layout: %w", err)
			return
		}

		pipeline, err = device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
			Label:  "Triangle Pipeline",
			Layout: pipelineLayout,
			Vertex: wgpu.VertexState{
				Module:     shader,
				EntryPoint: "vs_main",
			},
			Fragment: &wgpu.FragmentState{
				Module:     shader,
				EntryPoint: "fs_main",
				Targets: []gputypes.ColorTargetState{{
					Format:    gputypes.TextureFormatBGRA8Unorm,
					WriteMask: gputypes.ColorWriteMaskAll,
				}},
			},
		})
		if err != nil {
			initErr = fmt.Errorf("pipeline: %w", err)
			return
		}

		log.Println("3-9. GPU initialized on render thread")
	})

	if initErr != nil {
		return initErr
	}

	// 10. Render loop
	log.Println("=== Render loop started ===")
	frameCount := 0
	startTime := time.Now()

	for window.PollEvents() {
		var frameErr error

		renderLoop.RunOnRenderThreadVoid(func() {
			// Acquire
			surfaceTex, _, err := surface.GetCurrentTexture()
			if err != nil {
				frameErr = fmt.Errorf("GetCurrentTexture: %w", err)
				return
			}

			view, err := surfaceTex.CreateView(nil)
			if err != nil {
				frameErr = fmt.Errorf("CreateView: %w", err)
				surface.DiscardTexture()
				return
			}

			// Encode
			encoder, err := device.CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{Label: "Frame"})
			if err != nil {
				frameErr = fmt.Errorf("CreateCommandEncoder: %w", err)
				view.Release()
				return
			}

			renderPass, err := encoder.BeginRenderPass(&wgpu.RenderPassDescriptor{
				ColorAttachments: []wgpu.RenderPassColorAttachment{{
					View:       view,
					LoadOp:     gputypes.LoadOpClear,
					StoreOp:    gputypes.StoreOpStore,
					ClearValue: gputypes.Color{R: 0, G: 0, B: 0.5, A: 1},
				}},
			})
			if err != nil {
				frameErr = fmt.Errorf("BeginRenderPass: %w", err)
				view.Release()
				return
			}

			renderPass.SetPipeline(pipeline)
			renderPass.Draw(3, 1, 0, 0)
			if err := renderPass.End(); err != nil {
				frameErr = fmt.Errorf("end: %w", err)
				view.Release()
				return
			}

			commands, err := encoder.Finish()
			if err != nil {
				frameErr = fmt.Errorf("finish: %w", err)
				view.Release()
				return
			}

			if _, err := device.Queue().Submit(commands); err != nil {
				frameErr = fmt.Errorf("submit: %w", err)
			}

			if err := surface.Present(surfaceTex); err != nil {
				frameErr = fmt.Errorf("present: %w", err)
			}

			view.Release()
		})

		if frameErr != nil {
			log.Printf("Frame error: %v", frameErr)
			continue
		}

		frameCount++
		if frameCount%60 == 0 {
			fps := float64(frameCount) / time.Since(startTime).Seconds()
			log.Printf("Frame %d (%.1f FPS)", frameCount, fps)
		}
	}

	// Cleanup on render thread
	renderLoop.RunOnRenderThreadVoid(func() {
		pipeline.Release()
		pipelineLayout.Release()
		shader.Release()
		surface.Unconfigure()
		surface.Release()
	})

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
