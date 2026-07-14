//go:build integration && darwin && !rust && !(js && wasm)

package wgpu_test

import (
	"context"
	"encoding/binary"
	"math"
	"testing"
	"time"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
	_ "github.com/gogpu/wgpu/hal/metal"
)

const countedIndexedShader = `
@vertex
fn vs_main(@location(0) pos: vec2<f32>) -> @builtin(position) vec4<f32> {
    return vec4<f32>(pos, 0.0, 1.0);
}

@fragment
fn fs_main() -> @location(0) vec4<f32> {
    return vec4<f32>(0.0, 1.0, 0.0, 1.0);
}
`

func TestMultiDrawIndexedIndirectRendersDistinctRecords(t *testing.T) {
	instance, err := wgpu.CreateInstance(&wgpu.InstanceDescriptor{Backends: wgpu.BackendsPrimary})
	if err != nil {
		t.Skipf("CreateInstance: %v", err)
	}
	defer instance.Release()

	adapter, err := instance.RequestAdapter(nil)
	if err != nil {
		t.Skipf("RequestAdapter: %v", err)
	}
	defer adapter.Release()
	if info := adapter.Info(); info.Backend != gputypes.BackendMetal {
		t.Skipf("native Metal adapter unavailable: %+v", info)
	}

	device, err := adapter.RequestDevice(nil)
	if err != nil {
		t.Skipf("RequestDevice: %v", err)
	}
	defer device.Release()
	queue := device.Queue()

	vertices := []float32{-1, -1, 1, -1, 1, 1, -1, 1}
	vertexData := make([]byte, len(vertices)*4)
	for i, value := range vertices {
		binary.LittleEndian.PutUint32(vertexData[i*4:], math.Float32bits(value))
	}
	indexData := []byte{0, 0, 1, 0, 2, 0, 2, 0, 3, 0, 0, 0}

	// Four padding bytes make the starting offset observable. The two records
	// select different triangles; both are required to fill the target.
	indirectData := make([]byte, 4+2*20)
	putCountedIndexedIndirectRecord(indirectData[4:], 3, 1, 0, 0, 0)
	putCountedIndexedIndirectRecord(indirectData[24:], 3, 1, 3, 0, 0)

	vertexBuffer := createCountedIndirectTestBuffer(t, device, queue, vertexData, wgpu.BufferUsageVertex|wgpu.BufferUsageCopyDst)
	defer vertexBuffer.Release()
	indexBuffer := createCountedIndirectTestBuffer(t, device, queue, indexData, wgpu.BufferUsageIndex|wgpu.BufferUsageCopyDst)
	defer indexBuffer.Release()
	indirectBuffer := createCountedIndirectTestBuffer(t, device, queue, indirectData, wgpu.BufferUsageIndirect|wgpu.BufferUsageCopyDst)
	defer indirectBuffer.Release()

	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{WGSL: countedIndexedShader})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	defer shader.Release()
	layout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{})
	if err != nil {
		t.Fatalf("CreatePipelineLayout: %v", err)
	}
	defer layout.Release()
	pipeline, err := device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Layout: layout,
		Vertex: wgpu.VertexState{
			Module: shader, EntryPoint: "vs_main",
			Buffers: []gputypes.VertexBufferLayout{{
				ArrayStride: 8,
				StepMode:    gputypes.VertexStepModeVertex,
				Attributes: []gputypes.VertexAttribute{{
					Format: gputypes.VertexFormatFloat32x2, ShaderLocation: 0,
				}},
			}},
		},
		Fragment: &wgpu.FragmentState{
			Module: shader, EntryPoint: "fs_main",
			Targets: []gputypes.ColorTargetState{{
				Format: gputypes.TextureFormatRGBA8Unorm, WriteMask: gputypes.ColorWriteMaskAll,
			}},
		},
		Primitive:   gputypes.PrimitiveState{Topology: gputypes.PrimitiveTopologyTriangleList, CullMode: gputypes.CullModeNone},
		Multisample: gputypes.MultisampleState{Count: 1, Mask: 0xFFFFFFFF},
	})
	if err != nil {
		t.Fatalf("CreateRenderPipeline: %v", err)
	}
	defer pipeline.Release()

	const width, height = uint32(64), uint32(16)
	target, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Size:          wgpu.Extent3D{Width: width, Height: height, DepthOrArrayLayers: 1},
		MipLevelCount: 1, SampleCount: 1, Dimension: gputypes.TextureDimension2D,
		Format: gputypes.TextureFormatRGBA8Unorm,
		Usage:  gputypes.TextureUsageRenderAttachment | gputypes.TextureUsageCopySrc,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	defer target.Release()
	targetView, err := device.CreateTextureView(target, nil)
	if err != nil {
		t.Fatalf("CreateTextureView: %v", err)
	}
	defer targetView.Release()

	encoder, err := device.CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{})
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	pass, err := encoder.BeginRenderPass(&wgpu.RenderPassDescriptor{
		ColorAttachments: []wgpu.RenderPassColorAttachment{{
			View: targetView, LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore,
			ClearValue: gputypes.Color{A: 1},
		}},
	})
	if err != nil {
		t.Fatalf("BeginRenderPass: %v", err)
	}
	pass.SetPipeline(pipeline)
	pass.SetVertexBuffer(0, vertexBuffer, 0)
	pass.SetIndexBuffer(indexBuffer, gputypes.IndexFormatUint16, 0)
	pass.MultiDrawIndexedIndirect(indirectBuffer, 4, 2)
	if err := pass.End(); err != nil {
		t.Fatalf("RenderPass.End: %v", err)
	}
	encoder.TransitionTextures([]wgpu.TextureBarrier{{
		Texture: target,
		Usage: wgpu.TextureUsageTransition{
			OldUsage: gputypes.TextureUsageRenderAttachment,
			NewUsage: gputypes.TextureUsageCopySrc,
		},
	}})

	readbackSize := uint64(width * height * 4)
	readback, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Size: readbackSize, Usage: gputypes.BufferUsageMapRead | gputypes.BufferUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateBuffer(readback): %v", err)
	}
	defer readback.Release()
	encoder.CopyTextureToBuffer(target, readback, []wgpu.BufferTextureCopy{{
		BufferLayout: wgpu.ImageDataLayout{BytesPerRow: width * 4, RowsPerImage: height},
		TextureBase:  wgpu.ImageCopyTexture{Texture: target},
		Size:         wgpu.Extent3D{Width: width, Height: height, DepthOrArrayLayers: 1},
	}})

	commands, err := encoder.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if _, err := queue.Submit(commands); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	mapContext, cancelMap := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelMap()
	if err := readback.Map(mapContext, wgpu.MapModeRead, 0, readbackSize); err != nil {
		t.Fatalf("Map: %v", err)
	}
	mapped, err := readback.MappedRange(0, readbackSize)
	if err != nil {
		_ = readback.Unmap()
		t.Fatalf("MappedRange: %v", err)
	}
	pixels := mapped.Bytes()
	filled := 0
	for pixel := 0; pixel < int(width*height); pixel++ {
		if pixels[pixel*4+1] > 128 {
			filled++
		}
	}
	if err := readback.Unmap(); err != nil {
		t.Fatalf("Unmap: %v", err)
	}
	if minimum := int(width*height) * 3 / 4; filled < minimum {
		t.Fatalf("counted indexed indirect filled %d/%d pixels, want at least %d; both records did not render", filled, width*height, minimum)
	}
}

func createCountedIndirectTestBuffer(t *testing.T, device *wgpu.Device, queue *wgpu.Queue, data []byte, usage wgpu.BufferUsage) *wgpu.Buffer {
	t.Helper()
	buffer, err := device.CreateBuffer(&wgpu.BufferDescriptor{Size: uint64(len(data)), Usage: usage})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	if err := queue.WriteBuffer(buffer, 0, data); err != nil {
		buffer.Release()
		t.Fatalf("WriteBuffer: %v", err)
	}
	return buffer
}

func putCountedIndexedIndirectRecord(dst []byte, indexCount, instanceCount, firstIndex uint32, baseVertex int32, firstInstance uint32) {
	binary.LittleEndian.PutUint32(dst[0:], indexCount)
	binary.LittleEndian.PutUint32(dst[4:], instanceCount)
	binary.LittleEndian.PutUint32(dst[8:], firstIndex)
	binary.LittleEndian.PutUint32(dst[12:], uint32(baseVertex))
	binary.LittleEndian.PutUint32(dst[16:], firstInstance)
}
