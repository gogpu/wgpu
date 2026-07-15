//go:build integration && darwin && !rust && !(js && wasm)

package wgpu_test

import (
	"bytes"
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
struct Params {
    expected_instance: u32,
    _padding: vec3<u32>,
};

@group(0) @binding(0)
var<uniform> params: Params;

@group(0) @binding(1)
var<storage, read> shifts: array<f32>;

@vertex
fn vs_main(@location(0) pos: vec2<f32>, @builtin(instance_index) instance: u32) -> @builtin(position) vec4<f32> {
    let x = select(pos.x + 4.0, pos.x + shifts[0], instance == params.expected_instance);
    return vec4<f32>(x, pos.y, 0.0, 1.0);
}

@fragment
fn fs_main() -> @location(0) vec4<f32> {
    return vec4<f32>(0.0, 1.0, 0.0, 1.0);
}
`

const countedIndirectWriterShader = `
@group(0) @binding(0)
var<storage, read_write> args: array<u32>;

@compute @workgroup_size(1)
fn main() {
    args[1] = 1u;
    args[6] = 1u;
}
`

func TestMultiDrawIndexedIndirectRendersDistinctRecords(t *testing.T) {
	uint16Indices := []byte{0, 0, 1, 0, 2, 0, 2, 0, 3, 0, 0, 0}
	uint32Indices := make([]byte, 4+6*4)
	for i, index := range []uint32{0, 1, 2, 2, 3, 0} {
		binary.LittleEndian.PutUint32(uint32Indices[4+i*4:], index)
	}
	for _, test := range []struct {
		name        string
		vertices    []float32
		indices     []byte
		format      gputypes.IndexFormat
		indexOffset uint64
		baseVertex  int32
	}{
		{
			name:     "uint16",
			vertices: []float32{-1, -1, 1, -1, 1, 1, -1, 1},
			indices:  uint16Indices, format: gputypes.IndexFormatUint16,
		},
		{
			name:     "uint32_nonzero_index_offset_base_vertex",
			vertices: []float32{9, 9, -1, -1, 1, -1, 1, 1, -1, 1},
			indices:  uint32Indices, format: gputypes.IndexFormatUint32, indexOffset: 4, baseVertex: 1,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			icbPixels := runCountedIndexedICBCase(t, test.vertices, test.indices, test.format, test.indexOffset, test.baseVertex, false)
			loopPixels := runCountedIndexedICBCase(t, test.vertices, test.indices, test.format, test.indexOffset, test.baseVertex, true)
			if !bytes.Equal(icbPixels, loopPixels) {
				t.Fatal("first-draw ICB pixels differ from later-draw loop oracle")
			}
		})
	}
}

func runCountedIndexedICBCase(t *testing.T, vertices []float32, indexData []byte, indexFormat gputypes.IndexFormat, indexOffset uint64, baseVertex int32, forceLoop bool) []byte {
	t.Helper()
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

	vertexData := make([]byte, len(vertices)*4)
	for i, value := range vertices {
		binary.LittleEndian.PutUint32(vertexData[i*4:], math.Float32bits(value))
	}
	// Aligned padding makes the starting offset observable. The first two
	// records select different triangles; the remaining zero-instance records
	// keep the 1,024-command batch visually inert while exercising Metal's ICB
	// threshold. A compute pass changes the first two instance counts from zero
	// to one before the render pass, proving the translator consumes GPU-written
	// arguments.
	const drawCount = uint32(1024)
	const argumentsOffset = uint64(256)
	indirectData := make([]byte, argumentsOffset+uint64(drawCount)*20)
	putCountedIndexedIndirectRecord(indirectData[argumentsOffset:], 3, 0, 0, baseVertex, 3)
	putCountedIndexedIndirectRecord(indirectData[argumentsOffset+20:], 3, 0, 3, baseVertex, 3)

	vertexBuffer := createCountedIndirectTestBuffer(t, device, queue, vertexData, wgpu.BufferUsageVertex|wgpu.BufferUsageCopyDst)
	defer vertexBuffer.Release()
	indexBuffer := createCountedIndirectTestBuffer(t, device, queue, indexData, wgpu.BufferUsageIndex|wgpu.BufferUsageCopyDst)
	defer indexBuffer.Release()
	indirectBuffer := createCountedIndirectTestBuffer(t, device, queue, indirectData, wgpu.BufferUsageIndirect|wgpu.BufferUsageStorage|wgpu.BufferUsageCopyDst)
	defer indirectBuffer.Release()

	writerShader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{WGSL: countedIndirectWriterShader})
	if err != nil {
		t.Fatalf("CreateShaderModule(writer): %v", err)
	}
	defer writerShader.Release()
	writerBGL, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{Entries: []wgpu.BindGroupLayoutEntry{{
		Binding: 0, Visibility: wgpu.ShaderStageCompute,
		Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeStorage},
	}}})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout(writer): %v", err)
	}
	defer writerBGL.Release()
	writerLayout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{BindGroupLayouts: []*wgpu.BindGroupLayout{writerBGL}})
	if err != nil {
		t.Fatalf("CreatePipelineLayout(writer): %v", err)
	}
	defer writerLayout.Release()
	writerPipeline, err := device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
		Layout: writerLayout, Module: writerShader, EntryPoint: "main",
	})
	if err != nil {
		t.Fatalf("CreateComputePipeline(writer): %v", err)
	}
	defer writerPipeline.Release()
	writerGroup, err := device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Layout: writerBGL,
		Entries: []wgpu.BindGroupEntry{{
			Binding: 0, Buffer: indirectBuffer, Offset: argumentsOffset, Size: uint64(drawCount) * 20,
		}},
	})
	if err != nil {
		t.Fatalf("CreateBindGroup(writer): %v", err)
	}
	defer writerGroup.Release()

	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{WGSL: countedIndexedShader})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	defer shader.Release()
	renderBGL, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{Entries: []wgpu.BindGroupLayoutEntry{
		{
			Binding: 0, Visibility: gputypes.ShaderStageVertex,
			Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform, MinBindingSize: 16},
		},
		{
			Binding: 1, Visibility: gputypes.ShaderStageVertex,
			Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeReadOnlyStorage, MinBindingSize: 4},
		},
	}})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout(render): %v", err)
	}
	defer renderBGL.Release()
	layout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{BindGroupLayouts: []*wgpu.BindGroupLayout{renderBGL}})
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

	uniformData := make([]byte, 16)
	binary.LittleEndian.PutUint32(uniformData, 3)
	storageData := make([]byte, 4)
	renderUniform := createCountedIndirectTestBuffer(t, device, queue, uniformData, wgpu.BufferUsageUniform|wgpu.BufferUsageCopyDst)
	defer renderUniform.Release()
	renderStorage := createCountedIndirectTestBuffer(t, device, queue, storageData, wgpu.BufferUsageStorage|wgpu.BufferUsageCopyDst)
	defer renderStorage.Release()
	renderGroup, err := device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Layout: renderBGL,
		Entries: []wgpu.BindGroupEntry{
			{Binding: 0, Buffer: renderUniform, Size: 16},
			{Binding: 1, Buffer: renderStorage, Size: 4},
		},
	})
	if err != nil {
		t.Fatalf("CreateBindGroup(render): %v", err)
	}
	defer renderGroup.Release()

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
	clearOnlyTarget, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Size:          wgpu.Extent3D{Width: width, Height: 1, DepthOrArrayLayers: 1},
		MipLevelCount: 1, SampleCount: 1, Dimension: gputypes.TextureDimension2D,
		Format: gputypes.TextureFormatRGBA8Unorm,
		Usage:  gputypes.TextureUsageRenderAttachment | gputypes.TextureUsageCopySrc,
	})
	if err != nil {
		t.Fatalf("CreateTexture(clear-only): %v", err)
	}
	defer clearOnlyTarget.Release()
	clearOnlyView, err := device.CreateTextureView(clearOnlyTarget, nil)
	if err != nil {
		t.Fatalf("CreateTextureView(clear-only): %v", err)
	}
	defer clearOnlyView.Release()

	encoder, err := device.CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{})
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	compute, err := encoder.BeginComputePass(&wgpu.ComputePassDescriptor{})
	if err != nil {
		t.Fatalf("BeginComputePass: %v", err)
	}
	compute.SetPipeline(writerPipeline)
	compute.SetBindGroup(0, writerGroup, nil)
	compute.Dispatch(1, 1, 1)
	if err := compute.End(); err != nil {
		t.Fatalf("ComputePass.End: %v", err)
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
	pass.SetBindGroup(0, renderGroup, nil)
	pass.SetVertexBuffer(0, vertexBuffer, 0)
	pass.SetIndexBuffer(indexBuffer, indexFormat, indexOffset)
	if forceLoop {
		pass.Draw(0, 0, 0, 0)
	}
	pass.MultiDrawIndexedIndirect(indirectBuffer, argumentsOffset, drawCount)
	if err := pass.End(); err != nil {
		t.Fatalf("RenderPass.End: %v", err)
	}
	clearOnlyPass, err := encoder.BeginRenderPass(&wgpu.RenderPassDescriptor{
		ColorAttachments: []wgpu.RenderPassColorAttachment{{
			View: clearOnlyView, LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore,
			ClearValue: gputypes.Color{R: 1, G: 0.25, B: 0.5, A: 1},
		}},
	})
	if err != nil {
		t.Fatalf("BeginRenderPass(clear-only): %v", err)
	}
	if err := clearOnlyPass.End(); err != nil {
		t.Fatalf("RenderPass.End(clear-only): %v", err)
	}
	encoder.TransitionTextures([]wgpu.TextureBarrier{
		{
			Texture: target,
			Usage: wgpu.TextureUsageTransition{
				OldUsage: gputypes.TextureUsageRenderAttachment,
				NewUsage: gputypes.TextureUsageCopySrc,
			},
		},
		{
			Texture: clearOnlyTarget,
			Usage: wgpu.TextureUsageTransition{
				OldUsage: gputypes.TextureUsageRenderAttachment,
				NewUsage: gputypes.TextureUsageCopySrc,
			},
		},
	})

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
	clearOnlyReadback, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Size: uint64(width * 4), Usage: gputypes.BufferUsageMapRead | gputypes.BufferUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateBuffer(clear-only readback): %v", err)
	}
	defer clearOnlyReadback.Release()
	encoder.CopyTextureToBuffer(clearOnlyTarget, clearOnlyReadback, []wgpu.BufferTextureCopy{{
		BufferLayout: wgpu.ImageDataLayout{BytesPerRow: width * 4, RowsPerImage: 1},
		TextureBase:  wgpu.ImageCopyTexture{Texture: clearOnlyTarget},
		Size:         wgpu.Extent3D{Width: width, Height: 1, DepthOrArrayLayers: 1},
	}})

	commands, err := encoder.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if _, err := queue.Submit(commands); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	clearMapContext, cancelClearMap := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelClearMap()
	if err := clearOnlyReadback.Map(clearMapContext, wgpu.MapModeRead, 0, uint64(width*4)); err != nil {
		t.Fatalf("Map(clear-only readback): %v", err)
	}
	clearMapped, err := clearOnlyReadback.MappedRange(0, uint64(width*4))
	if err != nil {
		t.Fatalf("MappedRange(clear-only readback): %v", err)
	}
	clearPixel := clearMapped.Bytes()[:4]
	if clearPixel[0] < 250 || clearPixel[1] < 60 || clearPixel[1] > 68 || clearPixel[2] < 124 || clearPixel[2] > 132 || clearPixel[3] < 250 {
		t.Fatalf("clear-only pass pixel = %v, want approximately [255 64 128 255]", clearPixel)
	}
	if err := clearOnlyReadback.Unmap(); err != nil {
		t.Fatalf("Unmap(clear-only readback): %v", err)
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
	result := append([]byte(nil), pixels...)
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
	return result
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
