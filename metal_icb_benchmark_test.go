//go:build integration && darwin && !rust && !(js && wasm)

package wgpu_test

import (
	"encoding/binary"
	"runtime"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
	_ "github.com/gogpu/wgpu/hal/metal"
)

const metalICBBenchmarkShader = `
@vertex
fn vs_main(@location(0) pos: vec2<f32>) -> @builtin(position) vec4<f32> {
    return vec4<f32>(pos, 0.0, 1.0);
}
@fragment
fn fs_main() -> @location(0) vec4<f32> {
    return vec4<f32>(0.0, 1.0, 0.0, 1.0);
}
`

func BenchmarkMetalIndexedFrame(b *testing.B) {
	for _, test := range []struct {
		name      string
		count     uint32
		forceLoop bool
	}{
		{name: "direct"},
		{name: "count_1", count: 1},
		{name: "count_20", count: 20},
		{name: "count_1024", count: 1024},
		{name: "count_1024_loop", count: 1024, forceLoop: true},
		{name: "count_10000", count: 10000},
		{name: "count_10000_loop", count: 10000, forceLoop: true},
	} {
		b.Run(test.name, func(b *testing.B) {
			benchmarkMetalIndexedFrame(b, test.count, test.forceLoop)
		})
	}
}

func benchmarkMetalIndexedFrame(b *testing.B, count uint32, forceLoop bool) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	instance, err := wgpu.CreateInstance(&wgpu.InstanceDescriptor{Backends: wgpu.BackendsPrimary})
	if err != nil {
		b.Skipf("CreateInstance: %v", err)
	}
	defer instance.Release()
	adapter, err := instance.RequestAdapter(nil)
	if err != nil {
		b.Skipf("RequestAdapter: %v", err)
	}
	defer adapter.Release()
	if info := adapter.Info(); info.Backend != gputypes.BackendMetal {
		b.Skipf("native Metal adapter unavailable: %+v", info)
	}
	device, err := adapter.RequestDevice(nil)
	if err != nil {
		b.Fatal(err)
	}
	defer device.Release()
	queue := device.Queue()

	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{WGSL: metalICBBenchmarkShader})
	if err != nil {
		b.Fatal(err)
	}
	defer shader.Release()
	layout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{})
	if err != nil {
		b.Fatal(err)
	}
	defer layout.Release()
	pipeline, err := device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Layout: layout,
		Vertex: wgpu.VertexState{
			Module: shader, EntryPoint: "vs_main",
			Buffers: []gputypes.VertexBufferLayout{{
				ArrayStride: 8,
				Attributes:  []gputypes.VertexAttribute{{Format: gputypes.VertexFormatFloat32x2}},
			}},
		},
		Fragment: &wgpu.FragmentState{
			Module: shader, EntryPoint: "fs_main",
			Targets: []gputypes.ColorTargetState{{
				Format: gputypes.TextureFormatRGBA8Unorm, WriteMask: gputypes.ColorWriteMaskAll,
			}},
		},
		Primitive:   gputypes.PrimitiveState{Topology: gputypes.PrimitiveTopologyTriangleList},
		Multisample: gputypes.MultisampleState{Count: 1, Mask: 0xFFFFFFFF},
	})
	if err != nil {
		b.Fatal(err)
	}
	defer pipeline.Release()

	vertex := benchmarkMetalBuffer(b, device, queue, make([]byte, 3*8), wgpu.BufferUsageVertex|wgpu.BufferUsageCopyDst)
	defer vertex.Release()
	index := benchmarkMetalBuffer(b, device, queue, make([]byte, 8), wgpu.BufferUsageIndex|wgpu.BufferUsageCopyDst)
	defer index.Release()
	argumentCount := max(count, 1)
	arguments := make([]byte, uint64(argumentCount)*20)
	for i := uint32(0); i < count; i++ {
		binary.LittleEndian.PutUint32(arguments[uint64(i)*20:], 3)
	}
	indirect := benchmarkMetalBuffer(b, device, queue, arguments, wgpu.BufferUsageIndirect|wgpu.BufferUsageCopyDst)
	defer indirect.Release()
	target, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Size:          wgpu.Extent3D{Width: 1, Height: 1, DepthOrArrayLayers: 1},
		MipLevelCount: 1, SampleCount: 1, Dimension: gputypes.TextureDimension2D,
		Format: gputypes.TextureFormatRGBA8Unorm,
		Usage:  gputypes.TextureUsageRenderAttachment,
	})
	if err != nil {
		b.Fatal(err)
	}
	defer target.Release()
	view, err := device.CreateTextureView(target, nil)
	if err != nil {
		b.Fatal(err)
	}
	defer view.Release()

	frame := func() {
		encoder, err := device.CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{})
		if err != nil {
			b.Fatal(err)
		}
		pass, err := encoder.BeginRenderPass(&wgpu.RenderPassDescriptor{
			ColorAttachments: []wgpu.RenderPassColorAttachment{{
				View: view, LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore,
			}},
		})
		if err != nil {
			b.Fatal(err)
		}
		pass.SetPipeline(pipeline)
		pass.SetVertexBuffer(0, vertex, 0)
		pass.SetIndexBuffer(index, gputypes.IndexFormatUint16, 0)
		if count == 0 {
			pass.DrawIndexed(3, 1, 0, 0, 0)
		} else {
			if forceLoop {
				pass.Draw(0, 0, 0, 0)
			}
			pass.MultiDrawIndexedIndirect(indirect, 0, count)
		}
		if err := pass.End(); err != nil {
			b.Fatal(err)
		}
		commands, err := encoder.Finish()
		if err != nil {
			b.Fatal(err)
		}
		if _, err := queue.Submit(commands); err != nil {
			b.Fatal(err)
		}
		if err := device.WaitIdle(); err != nil {
			b.Fatal(err)
		}
	}

	for range 5 {
		frame()
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		frame()
	}
}

func benchmarkMetalBuffer(b *testing.B, device *wgpu.Device, queue *wgpu.Queue, data []byte, usage wgpu.BufferUsage) *wgpu.Buffer {
	b.Helper()
	buffer, err := device.CreateBuffer(&wgpu.BufferDescriptor{Size: uint64(len(data)), Usage: usage})
	if err != nil {
		b.Fatal(err)
	}
	if err := queue.WriteBuffer(buffer, 0, data); err != nil {
		buffer.Release()
		b.Fatal(err)
	}
	return buffer
}
