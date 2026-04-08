//go:build windows

// Command sw-triangle renders a red triangle on a blue background using
// ONLY the software rasterizer backend. The rendered framebuffer is displayed
// in a Win32 window via GDI SetDIBitsToDevice — no GPU required.
package main

import (
	"fmt"
	"log"
	"math"
	"os"
	"runtime"
	"time"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"

	_ "github.com/gogpu/wgpu/hal/allbackends" // register all backends
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

// surfaceConfig returns the standard surface configuration for the given dimensions.
func surfaceConfig(w, h uint32) *wgpu.SurfaceConfiguration {
	return &wgpu.SurfaceConfiguration{
		Format:      gputypes.TextureFormatRGBA8Unorm,
		Usage:       gputypes.TextureUsageRenderAttachment,
		Width:       w,
		Height:      h,
		PresentMode: gputypes.PresentModeFifo,
		AlphaMode:   gputypes.CompositeAlphaModeOpaque,
	}
}

// renderFrame renders one frame: clear to blue, draw triangle, present.
// Returns false if the frame was skipped due to a recoverable error.
func renderFrame(
	surface *wgpu.Surface,
	device *wgpu.Device,
	pipeline *wgpu.RenderPipeline,
	vertexBuffer *wgpu.Buffer,
) bool {
	surfaceTex, _, err := surface.GetCurrentTexture()
	if err != nil {
		return false
	}
	view, err := surfaceTex.CreateView(nil)
	if err != nil {
		surface.DiscardTexture()
		return false
	}
	defer view.Release()

	encoder, err := device.CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{Label: "Frame"})
	if err != nil {
		return false
	}
	renderPass, err := encoder.BeginRenderPass(&wgpu.RenderPassDescriptor{
		ColorAttachments: []wgpu.RenderPassColorAttachment{{
			View:       view,
			LoadOp:     gputypes.LoadOpClear,
			StoreOp:    gputypes.StoreOpStore,
			ClearValue: gputypes.Color{R: 0, G: 0, B: 0.5, A: 1}, // Blue background
		}},
	})
	if err != nil {
		return false
	}
	renderPass.SetPipeline(pipeline)
	renderPass.SetVertexBuffer(0, vertexBuffer, 0)
	renderPass.Draw(3, 1, 0, 0)
	_ = renderPass.End()
	commands, _ := encoder.Finish()
	_, _ = device.Queue().Submit(commands)
	// Present() auto-blits framebuffer to window via GDI (hal/software/queue.go).
	// Same API as GPU backends — no manual framebuffer access needed.
	if presentErr := surface.Present(surfaceTex); presentErr != nil {
		log.Printf("Present: %v", presentErr)
		return false
	}
	return true
}

//nolint:funlen // example code — sequential setup is intentionally verbose
func run() error {
	log.Println("=== Software Triangle (GDI Blit) ===")

	window, err := NewWindow("Software Triangle", 800, 600)
	if err != nil {
		return fmt.Errorf("window: %w", err)
	}
	defer window.Destroy()

	// Use software-only backend mask. Creating Vulkan/DX12 instances (even without
	// surfaces) loads GPU drivers that interfere with GDI StretchDIBits on some
	// systems (Intel Iris Xe). 1 << BackendEmpty filters to software only.
	instance, err := wgpu.CreateInstance(&wgpu.InstanceDescriptor{
		Backends: wgpu.Backends(1 << gputypes.BackendEmpty),
	})
	if err != nil {
		return fmt.Errorf("instance: %w", err)
	}
	defer instance.Release()

	surface, err := instance.CreateSurface(0, window.Handle())
	if err != nil {
		return fmt.Errorf("surface: %w", err)
	}
	defer surface.Release()

	adapter, err := instance.RequestAdapter(&wgpu.RequestAdapterOptions{
		ForceFallbackAdapter: true,
	})
	if err != nil {
		return fmt.Errorf("adapter: %w", err)
	}
	defer adapter.Release()

	info := adapter.Info()
	log.Printf("Adapter: %s (type=%v)", info.Name, info.DeviceType)
	if info.DeviceType != gputypes.DeviceTypeCPU {
		return fmt.Errorf("expected CPU adapter, got %v — software backend not loaded?", info.DeviceType)
	}

	device, err := adapter.RequestDevice(nil)
	if err != nil {
		return fmt.Errorf("device: %w", err)
	}
	defer device.Release()

	w, h := window.Size()
	if err = surface.Configure(device, surfaceConfig(safeUint32(w), safeUint32(h))); err != nil {
		return fmt.Errorf("configure: %w", err)
	}

	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label: "Triangle",
		WGSL:  triangleShaderWGSL,
	})
	if err != nil {
		return fmt.Errorf("shader: %w", err)
	}
	defer shader.Release()

	// Create vertex buffer with explicit positions + colors.
	// Software backend doesn't interpret shaders — it needs vertex data
	// in buffers (no @builtin(vertex_index) support in fixed-function rasterizer).
	vertices := []float32{
		// x, y, r, g, b
		0.0, 0.5, 1.0, 0.0, 0.0, // top — red
		-0.5, -0.5, 1.0, 0.0, 0.0, // bottom-left — red
		0.5, -0.5, 1.0, 0.0, 0.0, // bottom-right — red
	}
	vertexData := float32SliceToBytes(vertices)

	vertexBuffer, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label:            "Vertices",
		Size:             uint64(len(vertexData)),
		Usage:            gputypes.BufferUsageVertex | gputypes.BufferUsageCopyDst,
		MappedAtCreation: true,
	})
	if err != nil {
		return fmt.Errorf("vertex buffer: %w", err)
	}
	defer vertexBuffer.Release()

	if err = device.Queue().WriteBuffer(vertexBuffer, 0, vertexData); err != nil {
		return fmt.Errorf("write vertices: %w", err)
	}

	pipelineLayout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{Label: "Layout"})
	if err != nil {
		return fmt.Errorf("layout: %w", err)
	}
	defer pipelineLayout.Release()

	pipeline, err := device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label:  "Triangle",
		Layout: pipelineLayout,
		Vertex: wgpu.VertexState{
			Module:     shader,
			EntryPoint: "vs_main",
			Buffers: []gputypes.VertexBufferLayout{{
				ArrayStride: 5 * 4, // 5 floats per vertex
				StepMode:    gputypes.VertexStepModeVertex,
				Attributes: []gputypes.VertexAttribute{
					{Format: gputypes.VertexFormatFloat32x2, Offset: 0, ShaderLocation: 0},     // position
					{Format: gputypes.VertexFormatFloat32x3, Offset: 2 * 4, ShaderLocation: 1}, // color
				},
			}},
		},
		Fragment: &wgpu.FragmentState{
			Module: shader, EntryPoint: "fs_main",
			Targets: []wgpu.ColorTargetState{{
				Format:    gputypes.TextureFormatRGBA8Unorm,
				WriteMask: gputypes.ColorWriteMaskAll,
			}},
		},
		Primitive: gputypes.PrimitiveState{
			Topology: gputypes.PrimitiveTopologyTriangleList,
		},
	})
	if err != nil {
		return fmt.Errorf("pipeline: %w", err)
	}
	defer pipeline.Release()

	log.Println("Render loop started")
	frameCount := 0
	startTime := time.Now()

	for window.PollEvents() {
		// Handle window resize.
		if window.NeedsResize() {
			rw, rh := window.Size()
			if rw > 0 && rh > 0 {
				if err = surface.Configure(device, surfaceConfig(safeUint32(rw), safeUint32(rh))); err != nil {
					log.Printf("reconfigure: %v", err)
					continue
				}
			}
		}

		if !renderFrame(surface, device, pipeline, vertexBuffer) {
			continue
		}

		frameCount++
		if frameCount%60 == 0 {
			fps := float64(frameCount) / time.Since(startTime).Seconds()
			log.Printf("Frame %d (%.1f FPS)", frameCount, fps)
		}
	}

	log.Printf("Done. %d frames", frameCount)
	return nil
}

// Shader uses explicit vertex attributes (not @builtin(vertex_index)) because
// the software rasterizer uses fixed-function vertex fetch from bound buffers.
const triangleShaderWGSL = `
struct VertexInput {
    @location(0) position: vec2<f32>,
    @location(1) color: vec3<f32>,
}

struct VertexOutput {
    @builtin(position) position: vec4<f32>,
    @location(0) color: vec3<f32>,
}

@vertex
fn vs_main(in: VertexInput) -> VertexOutput {
    var out: VertexOutput;
    out.position = vec4<f32>(in.position, 0.0, 1.0);
    out.color = in.color;
    return out;
}

@fragment
fn fs_main(@location(0) color: vec3<f32>) -> @location(0) vec4<f32> {
    return vec4<f32>(color, 1.0);
}
`

func float32SliceToBytes(data []float32) []byte {
	buf := make([]byte, len(data)*4)
	for i, v := range data {
		bits := math.Float32bits(v)
		buf[i*4+0] = byte(bits)       //nolint:gosec // deliberate truncation for little-endian encoding
		buf[i*4+1] = byte(bits >> 8)  //nolint:gosec // deliberate truncation
		buf[i*4+2] = byte(bits >> 16) //nolint:gosec // deliberate truncation
		buf[i*4+3] = byte(bits >> 24)
	}
	return buf
}

func safeUint32(v int32) uint32 {
	if v < 0 {
		return 0
	}
	return uint32(v)
}
