// Command sw-test validates the software rasterizer backend.
// Headless — no window needed. Creates instance, adapter, device,
// compiles a shader, and creates a render pipeline to verify the
// software backend is functional.
package main

import (
	"fmt"
	"log"
	"log/slog"
	"os"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
	"github.com/gogpu/wgpu/hal"
	_ "github.com/gogpu/wgpu/hal/software"
)

const wgslShader = `
@vertex
fn vs_main(@builtin(vertex_index) idx: u32) -> @builtin(position) vec4<f32> {
    var pos = array<vec2<f32>, 3>(
        vec2<f32>( 0.0, -0.5),
        vec2<f32>( 0.5,  0.5),
        vec2<f32>(-0.5,  0.5),
    );
    return vec4<f32>(pos[idx], 0.0, 1.0);
}

@fragment
fn fs_main() -> @location(0) vec4<f32> {
    return vec4<f32>(1.0, 0.0, 0.0, 1.0);
}
`

func main() {
	hal.SetLogger(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: %v\n", err)
		os.Exit(1)
	}
	log.Println("sw-test: PASS")
}

func run() error {
	log.Println("=== Software Backend Test ===")

	instance, err := wgpu.CreateInstance(nil)
	if err != nil {
		return fmt.Errorf("instance: %w", err)
	}
	defer instance.Release()

	adapter, err := instance.RequestAdapter(&wgpu.RequestAdapterOptions{
		ForceFallbackAdapter: true,
	})
	if err != nil {
		return fmt.Errorf("adapter: %w", err)
	}
	defer adapter.Release()

	info := adapter.Info()
	log.Printf("Adapter: %s (type=%v, backend=%v)", info.Name, info.DeviceType, info.Backend)

	if info.DeviceType != gputypes.DeviceTypeCPU {
		return fmt.Errorf("expected CPU adapter, got %v — is software backend imported?", info.DeviceType)
	}
	log.Println("OK: CPU adapter found")

	device, err := adapter.RequestDevice(nil)
	if err != nil {
		return fmt.Errorf("device: %w", err)
	}
	defer device.Release()
	log.Println("OK: device created")

	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label: "sw-test shader",
		WGSL:  wgslShader,
	})
	if err != nil {
		return fmt.Errorf("shader: %w", err)
	}
	defer shader.Release()
	log.Println("OK: shader compiled")

	pipeline, err := device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label: "sw-test pipeline",
		Vertex: wgpu.VertexState{
			Module:     shader,
			EntryPoint: "vs_main",
		},
		Fragment: &wgpu.FragmentState{
			Module:     shader,
			EntryPoint: "fs_main",
			Targets: []wgpu.ColorTargetState{{
				Format: gputypes.TextureFormatRGBA8Unorm,
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
	log.Println("OK: render pipeline created")

	return nil
}
