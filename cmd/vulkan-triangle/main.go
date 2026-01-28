// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

// Command vulkan-triangle is a full integration test for the Pure Go Vulkan backend.
// It renders a red triangle to validate the entire rendering pipeline.
//
// This demo uses enterprise-level multi-thread architecture (Ebiten pattern):
// - Main thread: Window events (Win32 message pump)
// - Render thread: All GPU operations (Vulkan calls)
//
// This separation ensures window responsiveness during heavy GPU operations
// like swapchain recreation, which requires vkDeviceWaitIdle.
package main

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan"
	"github.com/gogpu/wgpu/internal/thread"
)

const (
	windowWidth  = 800
	windowHeight = 600
	windowTitle  = "Vulkan Triangle - Pure Go (Multi-Thread)"
)

func init() {
	// Lock the main goroutine to the OS main thread.
	// Required for Win32 window operations.
	runtime.LockOSThread()
}

func main() {
	if err := run(); err != nil {
		fmt.Printf("FATAL: %v\n", err)
		os.Exit(1)
	}
}

// GPU resources (created and used on render thread only).
type gpuResources struct {
	instance       hal.Instance
	surface        hal.Surface
	device         hal.Device
	queue          hal.Queue
	pipeline       hal.RenderPipeline
	pipelineLayout hal.PipelineLayout
	vertexShader   hal.ShaderModule
	fragmentShader hal.ShaderModule
	surfaceConfig  *hal.SurfaceConfiguration
	currentWidth   uint32
	currentHeight  uint32
}

func run() error {
	fmt.Println("=== Vulkan Triangle Integration Test (Multi-Thread) ===")
	fmt.Println()

	// Step 1: Create window (main thread)
	fmt.Print("1. Creating window... ")
	window, err := NewWindow(windowTitle, windowWidth, windowHeight)
	if err != nil {
		return fmt.Errorf("creating window: %w", err)
	}
	defer window.Destroy()
	fmt.Println("OK")

	// Step 2: Create render loop with dedicated render thread
	fmt.Print("2. Creating render thread... ")
	renderLoop := thread.NewRenderLoop()
	defer renderLoop.Stop()
	fmt.Println("OK")

	// Step 3-10: Initialize GPU resources on render thread
	var gpu *gpuResources
	var initErr error

	renderLoop.RunOnRenderThreadVoid(func() {
		gpu, initErr = initGPU(window)
	})

	if initErr != nil {
		return initErr
	}

	// Cleanup GPU resources on render thread
	defer func() {
		renderLoop.RunOnRenderThreadVoid(func() {
			cleanupGPU(gpu)
		})
	}()

	fmt.Println()
	fmt.Println("=== Starting Render Loop ===")
	fmt.Println("Press ESC or close window to exit")
	fmt.Println()

	frameCount := 0
	startTime := time.Now()

	// Main render loop
	for window.PollEvents() {
		// Check for pending resize from UI thread
		if window.NeedsResize() && !window.InSizeMove() {
			width, height := window.Size()
			newW, newH := safeUint32(width), safeUint32(height)
			if newW > 0 && newH > 0 {
				// Queue resize for render thread (non-blocking)
				renderLoop.RequestResize(newW, newH)
			}
		}

		// Render frame on render thread
		var frameErr error
		renderLoop.RunOnRenderThreadVoid(func() {
			// Apply pending resize (deferred from UI thread)
			applyPendingResize(renderLoop, gpu)

			// Render frame
			frameErr = renderFrame(gpu)

			// Reset command pool periodically (every 3 seconds at 60 FPS)
			resetCommandPoolIfNeeded(gpu, frameCount)
		})

		if frameErr != nil {
			// Handle surface outdated
			if errors.Is(frameErr, hal.ErrSurfaceOutdated) {
				width, height := window.Size()
				renderLoop.RequestResize(safeUint32(width), safeUint32(height))
				continue
			}
			// Log other errors but continue
			if !errors.Is(frameErr, hal.ErrNotReady) {
				fmt.Printf("Frame error: %v\n", frameErr)
			}
			continue
		}

		frameCount++

		// Print FPS every second
		if frameCount%60 == 0 {
			elapsed := time.Since(startTime).Seconds()
			fps := float64(frameCount) / elapsed
			fmt.Printf("Rendered %d frames (%.1f FPS)\n", frameCount, fps)
		}
	}

	fmt.Println()
	fmt.Println("=== Test Complete ===")
	elapsed := time.Since(startTime).Seconds()
	avgFPS := float64(frameCount) / elapsed
	fmt.Printf("Total frames: %d\n", frameCount)
	fmt.Printf("Average FPS: %.1f\n", avgFPS)

	return nil
}

// applyPendingResize applies any pending resize from the UI thread.
func applyPendingResize(renderLoop *thread.RenderLoop, gpu *gpuResources) {
	w, h, ok := renderLoop.ConsumePendingResize()
	if !ok {
		return
	}
	if w == gpu.currentWidth && h == gpu.currentHeight {
		return
	}

	gpu.surfaceConfig.Width = w
	gpu.surfaceConfig.Height = h
	if err := gpu.surface.Configure(gpu.device, gpu.surfaceConfig); err != nil {
		fmt.Printf("Failed to reconfigure surface: %v\n", err)
		return
	}
	gpu.currentWidth = w
	gpu.currentHeight = h
}

// resetCommandPoolIfNeeded resets the command pool periodically.
func resetCommandPoolIfNeeded(gpu *gpuResources, frameCount int) {
	if frameCount == 0 || frameCount%180 != 0 {
		return
	}
	vkDev, ok := gpu.device.(*vulkan.Device)
	if !ok {
		return
	}
	_ = vkDev.WaitIdle()
	_ = vkDev.ResetCommandPool()
}

// initGPU initializes all GPU resources. Called on render thread.
//
//nolint:funlen // Sequential initialization steps
func initGPU(window *Window) (*gpuResources, error) {
	gpu := &gpuResources{}

	// Create Vulkan backend
	fmt.Print("3. Creating Vulkan backend... ")
	backend := vulkan.Backend{}
	fmt.Println("OK")

	// Create instance
	fmt.Print("4. Creating Vulkan instance... ")
	instance, err := backend.CreateInstance(&hal.InstanceDescriptor{
		Backends: gputypes.BackendsVulkan,
		Flags:    gputypes.InstanceFlagsDebug,
	})
	if err != nil {
		return nil, fmt.Errorf("creating instance: %w", err)
	}
	gpu.instance = instance
	fmt.Println("OK")

	// Create surface
	fmt.Print("5. Creating surface... ")
	surface, err := instance.CreateSurface(0, window.Handle())
	if err != nil {
		return nil, fmt.Errorf("creating surface: %w", err)
	}
	gpu.surface = surface
	fmt.Println("OK")

	// Enumerate adapters
	fmt.Print("6. Enumerating adapters... ")
	adapters := instance.EnumerateAdapters(surface)
	if len(adapters) == 0 {
		return nil, fmt.Errorf("no adapters found")
	}
	fmt.Printf("OK (found %d)\n", len(adapters))

	for i := range adapters {
		exposed := &adapters[i]
		fmt.Printf("   - Adapter %d: %s (%s %s)\n",
			i, exposed.Info.Name, exposed.Info.Vendor, exposed.Info.DriverInfo)
	}

	// Open device
	fmt.Print("7. Opening device... ")
	openDev, err := adapters[0].Adapter.Open(0, adapters[0].Capabilities.Limits)
	if err != nil {
		return nil, fmt.Errorf("opening device: %w", err)
	}
	gpu.device = openDev.Device
	gpu.queue = openDev.Queue
	fmt.Println("OK")

	// Configure surface
	fmt.Print("8. Configuring surface... ")
	width, height := window.Size()
	gpu.currentWidth = safeUint32(width)
	gpu.currentHeight = safeUint32(height)
	gpu.surfaceConfig = &hal.SurfaceConfiguration{
		Width:       gpu.currentWidth,
		Height:      gpu.currentHeight,
		Format:      gputypes.TextureFormatBGRA8Unorm,
		Usage:       gputypes.TextureUsageRenderAttachment,
		PresentMode: hal.PresentModeFifo,
		AlphaMode:   hal.CompositeAlphaModeOpaque,
	}
	if err := surface.Configure(gpu.device, gpu.surfaceConfig); err != nil {
		return nil, fmt.Errorf("configuring surface: %w", err)
	}
	fmt.Println("OK")

	// Create shader modules
	fmt.Print("9. Creating shader modules... ")
	vertexShader, err := gpu.device.CreateShaderModule(&hal.ShaderModuleDescriptor{
		Label:  "Vertex Shader",
		Source: hal.ShaderSource{WGSL: vertexShaderWGSL},
	})
	if err != nil {
		return nil, fmt.Errorf("creating vertex shader: %w", err)
	}
	gpu.vertexShader = vertexShader

	fragmentShader, err := gpu.device.CreateShaderModule(&hal.ShaderModuleDescriptor{
		Label:  "Fragment Shader",
		Source: hal.ShaderSource{WGSL: fragmentShaderWGSL},
	})
	if err != nil {
		return nil, fmt.Errorf("creating fragment shader: %w", err)
	}
	gpu.fragmentShader = fragmentShader
	fmt.Println("OK")

	// Create pipeline layout
	fmt.Print("10. Creating pipeline layout... ")
	pipelineLayout, err := gpu.device.CreatePipelineLayout(&hal.PipelineLayoutDescriptor{
		Label:            "Triangle Pipeline Layout",
		BindGroupLayouts: nil,
	})
	if err != nil {
		return nil, fmt.Errorf("creating pipeline layout: %w", err)
	}
	gpu.pipelineLayout = pipelineLayout
	fmt.Println("OK")

	// Create render pipeline
	fmt.Print("11. Creating render pipeline... ")
	pipeline, err := gpu.device.CreateRenderPipeline(&hal.RenderPipelineDescriptor{
		Label:  "Triangle Pipeline",
		Layout: pipelineLayout,
		Vertex: hal.VertexState{
			Module:     vertexShader,
			EntryPoint: "main",
			Buffers:    nil,
		},
		Primitive: gputypes.PrimitiveState{
			Topology:         gputypes.PrimitiveTopologyTriangleList,
			StripIndexFormat: nil,
			FrontFace:        gputypes.FrontFaceCCW,
			CullMode:         gputypes.CullModeNone,
		},
		DepthStencil: nil,
		Multisample: gputypes.MultisampleState{
			Count:                  1,
			Mask:                   0xFFFFFFFF,
			AlphaToCoverageEnabled: false,
		},
		Fragment: &hal.FragmentState{
			Module:     fragmentShader,
			EntryPoint: "main",
			Targets: []gputypes.ColorTargetState{
				{
					Format:    gputypes.TextureFormatBGRA8Unorm,
					Blend:     nil,
					WriteMask: gputypes.ColorWriteMaskAll,
				},
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("creating render pipeline: %w", err)
	}
	gpu.pipeline = pipeline
	fmt.Println("OK")

	return gpu, nil
}

// cleanupGPU cleans up all GPU resources. Called on render thread.
func cleanupGPU(gpu *gpuResources) {
	if gpu == nil {
		return
	}

	if gpu.pipeline != nil {
		gpu.device.DestroyRenderPipeline(gpu.pipeline)
	}
	if gpu.pipelineLayout != nil {
		gpu.device.DestroyPipelineLayout(gpu.pipelineLayout)
	}
	if gpu.fragmentShader != nil {
		gpu.device.DestroyShaderModule(gpu.fragmentShader)
	}
	if gpu.vertexShader != nil {
		gpu.device.DestroyShaderModule(gpu.vertexShader)
	}
	if gpu.surface != nil {
		gpu.surface.Unconfigure(gpu.device)
		gpu.surface.Destroy()
	}
	if gpu.device != nil {
		gpu.device.Destroy()
	}
	if gpu.instance != nil {
		gpu.instance.Destroy()
	}
}

// renderFrame renders a single frame. Called on render thread.
func renderFrame(gpu *gpuResources) error {
	// Acquire swapchain image with retry
	var acquired *hal.AcquiredSurfaceTexture
	for attempts := 0; attempts < 2; attempts++ {
		var err error
		acquired, err = gpu.surface.AcquireTexture(nil)
		if err == nil {
			break
		}

		if errors.Is(err, hal.ErrNotReady) {
			continue
		}

		return err
	}

	if acquired == nil {
		return hal.ErrNotReady
	}

	// Create texture view
	textureView, err := gpu.device.CreateTextureView(acquired.Texture, &hal.TextureViewDescriptor{
		Label:           "Swapchain View",
		Format:          gputypes.TextureFormatBGRA8Unorm,
		Dimension:       gputypes.TextureViewDimension2D,
		Aspect:          gputypes.TextureAspectAll,
		BaseMipLevel:    0,
		MipLevelCount:   1,
		BaseArrayLayer:  0,
		ArrayLayerCount: 1,
	})
	if err != nil {
		gpu.surface.DiscardTexture(acquired.Texture)
		return fmt.Errorf("create texture view: %w", err)
	}
	defer gpu.device.DestroyTextureView(textureView)

	// Create command encoder
	encoder, err := gpu.device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{
		Label: "Triangle Encoder",
	})
	if err != nil {
		gpu.surface.DiscardTexture(acquired.Texture)
		return fmt.Errorf("create command encoder: %w", err)
	}

	// Begin encoding
	if err := encoder.BeginEncoding("Triangle Rendering"); err != nil {
		gpu.surface.DiscardTexture(acquired.Texture)
		return fmt.Errorf("begin encoding: %w", err)
	}

	// Begin render pass
	renderPass := encoder.BeginRenderPass(&hal.RenderPassDescriptor{
		Label: "Triangle Render Pass",
		ColorAttachments: []hal.RenderPassColorAttachment{
			{
				View:    textureView,
				LoadOp:  gputypes.LoadOpClear,
				StoreOp: gputypes.StoreOpStore,
				ClearValue: gputypes.Color{
					R: 0.0,
					G: 0.0,
					B: 0.5,
					A: 1.0,
				},
			},
		},
	})

	renderPass.SetPipeline(gpu.pipeline)
	renderPass.Draw(3, 1, 0, 0)
	renderPass.End()

	// End encoding
	cmdBuffer, err := encoder.EndEncoding()
	if err != nil {
		gpu.surface.DiscardTexture(acquired.Texture)
		return fmt.Errorf("end encoding: %w", err)
	}

	// Submit
	if err := gpu.queue.Submit([]hal.CommandBuffer{cmdBuffer}, nil, 0); err != nil {
		gpu.surface.DiscardTexture(acquired.Texture)
		return fmt.Errorf("submit: %w", err)
	}

	// Present
	if err := gpu.queue.Present(gpu.surface, acquired.Texture); err != nil {
		return fmt.Errorf("present: %w", err)
	}

	return nil
}

// safeUint32 converts int32 to uint32 safely.
func safeUint32(v int32) uint32 {
	if v < 0 {
		return 0
	}
	return uint32(v)
}
