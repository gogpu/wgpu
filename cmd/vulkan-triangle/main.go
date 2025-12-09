// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

// Command vulkan-triangle is a full integration test for the Pure Go Vulkan backend.
// It renders a red triangle to validate the entire rendering pipeline.
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan"
	"github.com/gogpu/wgpu/types"
)

const (
	windowWidth  = 800
	windowHeight = 600
	windowTitle  = "Vulkan Triangle - Pure Go"
)

func main() {
	if err := run(); err != nil {
		fmt.Printf("FATAL: %v\n", err)
		os.Exit(1)
	}
}

//nolint:gocyclo,cyclop,funlen,maintidx // Integration test needs sequential setup steps
func run() error {
	fmt.Println("=== Vulkan Triangle Integration Test ===")
	fmt.Println()

	// Step 1: Create window
	fmt.Print("1. Creating window... ")
	window, err := NewWindow(windowTitle, windowWidth, windowHeight)
	if err != nil {
		return fmt.Errorf("creating window: %w", err)
	}
	defer window.Destroy()
	fmt.Println("OK")

	// Step 2: Create Vulkan backend
	fmt.Print("2. Creating Vulkan backend... ")
	backend := vulkan.Backend{}
	fmt.Println("OK")

	// Step 3: Create instance
	fmt.Print("3. Creating Vulkan instance... ")
	instance, err := backend.CreateInstance(&hal.InstanceDescriptor{
		Backends: types.BackendsVulkan,
		Flags:    types.InstanceFlagsDebug,
	})
	if err != nil {
		return fmt.Errorf("creating instance: %w", err)
	}
	defer instance.Destroy()
	fmt.Println("OK")

	// Step 4: Create surface
	fmt.Print("4. Creating surface... ")
	surface, err := instance.CreateSurface(0, window.Handle())
	if err != nil {
		return fmt.Errorf("creating surface: %w", err)
	}
	defer surface.Destroy()
	fmt.Println("OK")

	// Step 5: Enumerate adapters
	fmt.Print("5. Enumerating adapters... ")
	adapters := instance.EnumerateAdapters(surface)
	if len(adapters) == 0 {
		return fmt.Errorf("no adapters found")
	}
	fmt.Printf("OK (found %d)\n", len(adapters))

	// Print adapter info
	for i := range adapters {
		exposed := &adapters[i]
		fmt.Printf("   - Adapter %d: %s (%s %s)\n",
			i, exposed.Info.Name, exposed.Info.Vendor, exposed.Info.DriverInfo)
	}

	// Step 6: Open device
	fmt.Print("6. Opening device... ")
	openDev, err := adapters[0].Adapter.Open(0, adapters[0].Capabilities.Limits)
	if err != nil {
		return fmt.Errorf("opening device: %w", err)
	}
	device := openDev.Device
	queue := openDev.Queue
	defer device.Destroy()
	fmt.Println("OK")

	// Step 7: Configure surface
	fmt.Print("7. Configuring surface... ")
	width, height := window.Size()
	surfaceConfig := &hal.SurfaceConfiguration{
		Width:       safeUint32(width),
		Height:      safeUint32(height),
		Format:      types.TextureFormatBGRA8Unorm,
		Usage:       types.TextureUsageRenderAttachment,
		PresentMode: hal.PresentModeFifo,
		AlphaMode:   hal.CompositeAlphaModeOpaque,
	}
	if err := surface.Configure(device, surfaceConfig); err != nil {
		return fmt.Errorf("configuring surface: %w", err)
	}
	defer surface.Unconfigure(device)
	fmt.Println("OK")

	// Step 8: Create shader modules
	fmt.Print("8. Creating shader modules... ")
	vertexShader, err := device.CreateShaderModule(&hal.ShaderModuleDescriptor{
		Label: "Vertex Shader",
		Source: hal.ShaderSource{
			SPIRV: vertexShaderSPIRV,
		},
	})
	if err != nil {
		return fmt.Errorf("creating vertex shader: %w", err)
	}
	defer device.DestroyShaderModule(vertexShader)

	fragmentShader, err := device.CreateShaderModule(&hal.ShaderModuleDescriptor{
		Label: "Fragment Shader",
		Source: hal.ShaderSource{
			SPIRV: fragmentShaderSPIRV,
		},
	})
	if err != nil {
		return fmt.Errorf("creating fragment shader: %w", err)
	}
	defer device.DestroyShaderModule(fragmentShader)
	fmt.Println("OK")

	// Step 9: Create pipeline layout (empty - no bindings)
	fmt.Print("9. Creating pipeline layout... ")
	pipelineLayout, err := device.CreatePipelineLayout(&hal.PipelineLayoutDescriptor{
		Label:            "Triangle Pipeline Layout",
		BindGroupLayouts: nil, // No bind groups
	})
	if err != nil {
		return fmt.Errorf("creating pipeline layout: %w", err)
	}
	defer device.DestroyPipelineLayout(pipelineLayout)
	fmt.Println("OK")

	// Step 10: Create render pipeline
	fmt.Print("10. Creating render pipeline... ")
	pipeline, err := device.CreateRenderPipeline(&hal.RenderPipelineDescriptor{
		Label:  "Triangle Pipeline",
		Layout: pipelineLayout,
		Vertex: hal.VertexState{
			Module:     vertexShader,
			EntryPoint: "main",
			Buffers:    nil, // No vertex buffers - positions hardcoded in shader
		},
		Primitive: types.PrimitiveState{
			Topology:         types.PrimitiveTopologyTriangleList,
			StripIndexFormat: nil, // Not using strip topology
			FrontFace:        types.FrontFaceCCW,
			CullMode:         types.CullModeNone,
		},
		DepthStencil: nil, // No depth testing
		Multisample: types.MultisampleState{
			Count:                  1,
			Mask:                   0xFFFFFFFF,
			AlphaToCoverageEnabled: false,
		},
		Fragment: &hal.FragmentState{
			Module:     fragmentShader,
			EntryPoint: "main",
			Targets: []types.ColorTargetState{
				{
					Format:    types.TextureFormatBGRA8Unorm,
					Blend:     nil, // No blending
					WriteMask: types.ColorWriteMaskAll,
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("creating render pipeline: %w", err)
	}
	defer device.DestroyRenderPipeline(pipeline)
	fmt.Println("OK")

	fmt.Println()
	fmt.Println("=== Starting Render Loop ===")
	fmt.Println("Press ESC or close window to exit")
	fmt.Println()

	frameCount := 0
	startTime := time.Now()

	// Render loop
	for window.PollEvents() {
		// Acquire swapchain image
		acquired, err := surface.AcquireTexture(nil)
		if err != nil {
			fmt.Printf("Failed to acquire texture: %v\n", err)
			continue
		}

		// Create texture view for the acquired image
		textureView, err := device.CreateTextureView(acquired.Texture, &hal.TextureViewDescriptor{
			Label:           "Swapchain View",
			Format:          types.TextureFormatBGRA8Unorm,
			Dimension:       types.TextureViewDimension2D,
			Aspect:          types.TextureAspectAll,
			BaseMipLevel:    0,
			MipLevelCount:   1,
			BaseArrayLayer:  0,
			ArrayLayerCount: 1,
		})
		if err != nil {
			fmt.Printf("Failed to create texture view: %v\n", err)
			surface.DiscardTexture(acquired.Texture)
			continue
		}

		// Create command encoder
		encoder, err := device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{
			Label: "Triangle Encoder",
		})
		if err != nil {
			fmt.Printf("Failed to create command encoder: %v\n", err)
			device.DestroyTextureView(textureView)
			surface.DiscardTexture(acquired.Texture)
			continue
		}

		// Begin encoding
		if err := encoder.BeginEncoding("Triangle Rendering"); err != nil {
			fmt.Printf("Failed to begin encoding: %v\n", err)
			device.DestroyTextureView(textureView)
			surface.DiscardTexture(acquired.Texture)
			continue
		}

		// Begin render pass with blue clear color
		renderPass := encoder.BeginRenderPass(&hal.RenderPassDescriptor{
			Label: "Triangle Render Pass",
			ColorAttachments: []hal.RenderPassColorAttachment{
				{
					View:    textureView,
					LoadOp:  types.LoadOpClear,
					StoreOp: types.StoreOpStore,
					ClearValue: types.Color{
						R: 0.0,
						G: 0.0,
						B: 0.5, // Blue background
						A: 1.0,
					},
				},
			},
		})

		// Set pipeline
		renderPass.SetPipeline(pipeline)

		// Draw triangle (3 vertices, 1 instance)
		renderPass.Draw(3, 1, 0, 0)

		// End render pass
		renderPass.End()

		// End encoding
		cmdBuffer, err := encoder.EndEncoding()
		if err != nil {
			fmt.Printf("Failed to end encoding: %v\n", err)
			device.DestroyTextureView(textureView)
			surface.DiscardTexture(acquired.Texture)
			continue
		}

		// Submit command buffer
		if err := queue.Submit([]hal.CommandBuffer{cmdBuffer}, nil, 0); err != nil {
			fmt.Printf("Failed to submit commands: %v\n", err)
			device.DestroyTextureView(textureView)
			surface.DiscardTexture(acquired.Texture)
			continue
		}

		// Present
		if err := queue.Present(surface, acquired.Texture); err != nil {
			fmt.Printf("Failed to present: %v\n", err)
		}

		// Clean up
		device.DestroyTextureView(textureView)

		frameCount++

		// Print FPS every second
		if frameCount%60 == 0 {
			elapsed := time.Since(startTime).Seconds()
			fps := float64(frameCount) / elapsed
			fmt.Printf("Rendered %d frames (%.1f FPS)\n", frameCount, fps)
		}

		// Small sleep to avoid 100% CPU usage
		time.Sleep(16 * time.Millisecond) // ~60 FPS
	}

	fmt.Println()
	fmt.Println("=== Test Complete ===")
	elapsed := time.Since(startTime).Seconds()
	avgFPS := float64(frameCount) / elapsed
	fmt.Printf("Total frames: %d\n", frameCount)
	fmt.Printf("Average FPS: %.1f\n", avgFPS)

	return nil
}

// safeUint32 converts int32 to uint32 safely.
func safeUint32(v int32) uint32 {
	if v < 0 {
		return 0
	}
	return uint32(v)
}
