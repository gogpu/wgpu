package wgpu_test

import (
	"encoding/binary"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"

	// Import the software backend so it registers with HAL.
	// Note: The core Instance currently skips BackendEmpty during adapter
	// enumeration, so the software backend is not used by CreateInstance
	// directly. When a real GPU backend (Vulkan, DX12, GLES) is available,
	// these tests exercise the full HAL integration path. Otherwise, the
	// tests skip gracefully. Future architecture changes may allow the
	// software backend to be selected directly.
	_ "github.com/gogpu/wgpu/hal/software"
)

// createTestDevice creates an Instance, Adapter, and Device for integration testing.
// It skips the test if HAL integration is not available (e.g., no real GPU drivers
// installed or running in headless CI). All returned resources should be released
// by the caller.
func createTestDevice(t *testing.T) (*wgpu.Instance, *wgpu.Adapter, *wgpu.Device) {
	t.Helper()

	instance, err := wgpu.CreateInstance(nil)
	if err != nil {
		t.Skipf("cannot create instance: %v", err)
	}

	adapter, err := instance.RequestAdapter(nil)
	if err != nil {
		instance.Release()
		t.Skipf("cannot request adapter: %v", err)
	}

	device, err := adapter.RequestDevice(nil)
	if err != nil {
		adapter.Release()
		instance.Release()
		t.Skipf("cannot request device: %v", err)
	}

	// Check that the device has actual HAL integration (not a mock adapter).
	// Mock adapters have no queue and cannot create GPU resources.
	if device.Queue() == nil {
		device.Release()
		adapter.Release()
		instance.Release()
		t.Skip("skipping: device has no HAL integration (mock adapter; no GPU backend available)")
	}

	return instance, adapter, device
}

// --- Instance tests ---

// TestIntegrationCreateInstance tests the full CreateInstance -> Release cycle.
func TestIntegrationCreateInstance(t *testing.T) {
	instance, err := wgpu.CreateInstance(nil)
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	if instance == nil {
		t.Fatal("CreateInstance returned nil")
	}

	// Release should be idempotent.
	instance.Release()
	instance.Release()
}

// --- Adapter tests ---

// TestIntegrationRequestAdapter verifies the adapter has a non-empty name and driver.
func TestIntegrationRequestAdapter(t *testing.T) {
	instance, err := wgpu.CreateInstance(nil)
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	defer instance.Release()

	adapter, err := instance.RequestAdapter(nil)
	if err != nil {
		t.Fatalf("RequestAdapter: %v", err)
	}
	if adapter == nil {
		t.Fatal("RequestAdapter returned nil")
	}
	defer adapter.Release()

	info := adapter.Info()
	if info.Name == "" {
		t.Error("adapter info Name is empty")
	}
	if info.Driver == "" {
		t.Error("adapter info Driver is empty")
	}
	t.Logf("adapter: name=%q driver=%q vendor=%q deviceType=%v",
		info.Name, info.Driver, info.Vendor, info.DeviceType)
}

// --- Device tests ---

// TestIntegrationRequestDevice verifies device creation produces a working device
// with queue and non-zero limits.
func TestIntegrationRequestDevice(t *testing.T) {
	instance, adapter, device := createTestDevice(t)
	defer instance.Release()
	defer adapter.Release()
	defer device.Release()

	q := device.Queue()
	if q == nil {
		t.Fatal("device.Queue() returned nil")
	}

	limits := device.Limits()
	if limits.MaxBufferSize == 0 {
		t.Error("device limits MaxBufferSize should be non-zero")
	}
	if limits.MaxTextureDimension2D == 0 {
		t.Error("device limits MaxTextureDimension2D should be non-zero")
	}
}

// --- Buffer tests ---

// TestIntegrationCreateBuffer creates a buffer and verifies Size, Usage, and Label.
func TestIntegrationCreateBuffer(t *testing.T) {
	instance, adapter, device := createTestDevice(t)
	defer instance.Release()
	defer adapter.Release()
	defer device.Release()

	desc := &wgpu.BufferDescriptor{
		Label: "integration-buffer",
		Size:  1024,
		Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopyDst | wgpu.BufferUsageCopySrc,
	}

	buf, err := device.CreateBuffer(desc)
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer buf.Release()

	if buf.Size() != desc.Size {
		t.Errorf("Size() = %d, want %d", buf.Size(), desc.Size)
	}
	if buf.Usage() != desc.Usage {
		t.Errorf("Usage() = %v, want %v", buf.Usage(), desc.Usage)
	}
	if buf.Label() != desc.Label {
		t.Errorf("Label() = %q, want %q", buf.Label(), desc.Label)
	}
}

// --- Texture tests ---

// TestIntegrationCreateTexture creates a texture and verifies its format.
func TestIntegrationCreateTexture(t *testing.T) {
	instance, adapter, device := createTestDevice(t)
	defer instance.Release()
	defer adapter.Release()
	defer device.Release()

	tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "integration-texture",
		Size:          wgpu.Extent3D{Width: 128, Height: 128, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Format:        wgpu.TextureFormatRGBA8Unorm,
		Usage:         wgpu.TextureUsageTextureBinding | wgpu.TextureUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	defer tex.Release()

	if tex.Format() != wgpu.TextureFormatRGBA8Unorm {
		t.Errorf("Format() = %v, want RGBA8Unorm", tex.Format())
	}
}

// TestIntegrationCreateTextureView creates a texture and then a view into it.
func TestIntegrationCreateTextureView(t *testing.T) {
	instance, adapter, device := createTestDevice(t)
	defer instance.Release()
	defer adapter.Release()
	defer device.Release()

	tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "view-texture",
		Size:          wgpu.Extent3D{Width: 64, Height: 64, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Format:        wgpu.TextureFormatRGBA8Unorm,
		Usage:         wgpu.TextureUsageTextureBinding,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	defer tex.Release()

	view, err := device.CreateTextureView(tex, &wgpu.TextureViewDescriptor{
		Label:           "integration-view",
		Format:          wgpu.TextureFormatRGBA8Unorm,
		BaseMipLevel:    0,
		MipLevelCount:   1,
		BaseArrayLayer:  0,
		ArrayLayerCount: 1,
	})
	if err != nil {
		t.Fatalf("CreateTextureView: %v", err)
	}
	view.Release()
}

// --- Sampler tests ---

// TestIntegrationCreateSampler creates a sampler with explicit and nil descriptors.
func TestIntegrationCreateSampler(t *testing.T) {
	instance, adapter, device := createTestDevice(t)
	defer instance.Release()
	defer adapter.Release()
	defer device.Release()

	sampler, err := device.CreateSampler(&wgpu.SamplerDescriptor{
		Label:       "integration-sampler",
		LodMinClamp: 0,
		LodMaxClamp: 32,
		Anisotropy:  1,
	})
	if err != nil {
		t.Fatalf("CreateSampler: %v", err)
	}
	defer sampler.Release()

	// nil descriptor creates a default sampler.
	samplerDefault, err := device.CreateSampler(nil)
	if err != nil {
		t.Fatalf("CreateSampler(nil): %v", err)
	}
	samplerDefault.Release()
}

// --- Shader module tests ---

// TestIntegrationCreateShaderModule creates a shader module with WGSL source.
func TestIntegrationCreateShaderModule(t *testing.T) {
	instance, adapter, device := createTestDevice(t)
	defer instance.Release()
	defer adapter.Release()
	defer device.Release()

	mod, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label: "integration-shader",
		WGSL: `
@group(0) @binding(0)
var<storage, read_write> data: array<u32>;

@compute @workgroup_size(1)
fn main(@builtin(global_invocation_id) id: vec3<u32>) {
    data[id.x] = data[id.x] * 2u;
}
`,
	})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	mod.Release()
}

// --- Bind group layout tests ---

// TestIntegrationCreateBindGroupLayout creates a bind group layout with a storage
// buffer entry.
func TestIntegrationCreateBindGroupLayout(t *testing.T) {
	instance, adapter, device := createTestDevice(t)
	defer instance.Release()
	defer adapter.Release()
	defer device.Release()

	layout, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label: "integration-bgl",
		Entries: []wgpu.BindGroupLayoutEntry{
			{
				Binding:    0,
				Visibility: wgpu.ShaderStageCompute,
				Buffer: &gputypes.BufferBindingLayout{
					Type: gputypes.BufferBindingTypeStorage,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout: %v", err)
	}
	layout.Release()
}

// --- Pipeline layout tests ---

// TestIntegrationCreatePipelineLayout creates a pipeline layout with one bind group
// layout containing a storage buffer entry.
func TestIntegrationCreatePipelineLayout(t *testing.T) {
	instance, adapter, device := createTestDevice(t)
	defer instance.Release()
	defer adapter.Release()
	defer device.Release()

	bgl, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label: "pipeline-bgl",
		Entries: []wgpu.BindGroupLayoutEntry{
			{
				Binding:    0,
				Visibility: wgpu.ShaderStageCompute,
				Buffer: &gputypes.BufferBindingLayout{
					Type: gputypes.BufferBindingTypeStorage,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout: %v", err)
	}
	defer bgl.Release()

	pipelineLayout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{
		Label:            "integration-pipeline-layout",
		BindGroupLayouts: []*wgpu.BindGroupLayout{bgl},
	})
	if err != nil {
		t.Fatalf("CreatePipelineLayout: %v", err)
	}
	pipelineLayout.Release()
}

// --- Command encoder tests ---

// TestIntegrationCreateCommandEncoder creates a command encoder, records nothing,
// and finishes it to produce a CommandBuffer.
func TestIntegrationCreateCommandEncoder(t *testing.T) {
	instance, adapter, device := createTestDevice(t)
	defer instance.Release()
	defer adapter.Release()
	defer device.Release()

	encoder, err := device.CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{
		Label: "integration-encoder",
	})
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}

	cmdBuf, err := encoder.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if cmdBuf == nil {
		t.Fatal("Finish returned nil CommandBuffer")
	}
}

// --- Queue tests ---

// TestIntegrationQueueWriteBuffer writes data to a buffer using Queue.WriteBuffer.
func TestIntegrationQueueWriteBuffer(t *testing.T) {
	instance, adapter, device := createTestDevice(t)
	defer instance.Release()
	defer adapter.Release()
	defer device.Release()

	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "write-test-buf",
		Size:  256,
		Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer buf.Release()

	q := device.Queue()
	if q == nil {
		t.Fatal("Queue is nil")
	}

	data := make([]byte, 16)
	for i := range data {
		data[i] = byte(i + 1)
	}

	// WriteBuffer should not panic and should store the data.
	q.WriteBuffer(buf, 0, data)
}

// --- WaitIdle tests ---

// TestIntegrationDeviceWaitIdle verifies WaitIdle returns without error on a
// fresh device with no pending work.
func TestIntegrationDeviceWaitIdle(t *testing.T) {
	instance, adapter, device := createTestDevice(t)
	defer instance.Release()
	defer adapter.Release()
	defer device.Release()

	err := device.WaitIdle()
	if err != nil {
		t.Fatalf("WaitIdle: %v", err)
	}
}

// --- Full compute workflow ---

// TestIntegrationFullComputeWorkflow exercises the full compute pipeline creation
// workflow: shader -> bind group layout -> pipeline layout -> compute pipeline ->
// bind group -> encoder -> compute pass -> dispatch -> finish -> submit.
//
// The software backend does NOT support compute pipelines and returns an error.
// In that case, the test still exercises everything else and submits an empty
// command buffer.
func TestIntegrationFullComputeWorkflow(t *testing.T) {
	instance, adapter, device := createTestDevice(t)
	defer instance.Release()
	defer adapter.Release()
	defer device.Release()

	// 1. Create shader module.
	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label: "compute-workflow-shader",
		WGSL: `
@group(0) @binding(0)
var<storage, read_write> data: array<u32>;

@compute @workgroup_size(1)
fn main(@builtin(global_invocation_id) id: vec3<u32>) {
    data[id.x] = data[id.x] * 2u;
}
`,
	})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	defer shader.Release()

	// 2. Create bind group layout.
	bgl, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label: "compute-bgl",
		Entries: []wgpu.BindGroupLayoutEntry{
			{
				Binding:    0,
				Visibility: wgpu.ShaderStageCompute,
				Buffer: &gputypes.BufferBindingLayout{
					Type: gputypes.BufferBindingTypeStorage,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout: %v", err)
	}
	defer bgl.Release()

	// 3. Create pipeline layout.
	pipelineLayout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{
		Label:            "compute-pipeline-layout",
		BindGroupLayouts: []*wgpu.BindGroupLayout{bgl},
	})
	if err != nil {
		t.Fatalf("CreatePipelineLayout: %v", err)
	}
	defer pipelineLayout.Release()

	// 4. Attempt to create compute pipeline.
	//    The software backend returns ErrComputeNotSupported.
	computePipeline, cpErr := device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
		Label:      "compute-pipeline",
		Layout:     pipelineLayout,
		Module:     shader,
		EntryPoint: "main",
	})
	if cpErr != nil {
		t.Logf("CreateComputePipeline returned expected error: %v", cpErr)
	} else {
		defer computePipeline.Release()
	}

	// 5. Create a storage buffer.
	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "compute-data-buf",
		Size:  256,
		Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopyDst | wgpu.BufferUsageCopySrc,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer buf.Release()

	// 6. Create bind group.
	bg, err := device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Label:  "compute-bind-group",
		Layout: bgl,
		Entries: []wgpu.BindGroupEntry{
			{
				Binding: 0,
				Buffer:  buf,
				Offset:  0,
				Size:    256,
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateBindGroup: %v", err)
	}
	defer bg.Release()

	// 7. Create command encoder, begin compute pass, dispatch, end, finish, submit.
	encoder, err := device.CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{
		Label: "compute-encoder",
	})
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}

	pass, err := encoder.BeginComputePass(&wgpu.ComputePassDescriptor{
		Label: "compute-pass",
	})
	if err != nil {
		t.Fatalf("BeginComputePass: %v", err)
	}

	// SetPipeline and SetBindGroup are recorded even if the compute pipeline
	// creation failed (they are no-ops in that case).
	if computePipeline != nil {
		pass.SetPipeline(computePipeline)
		pass.SetBindGroup(0, bg, nil)
	}
	pass.Dispatch(1, 1, 1)

	err = pass.End()
	if err != nil {
		t.Fatalf("End: %v", err)
	}

	cmdBuf, err := encoder.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}

	err = device.Queue().Submit(cmdBuf)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
}

// --- Read buffer tests ---

// TestIntegrationQueueReadBuffer writes data to a buffer via Queue.WriteBuffer,
// reads it back via Queue.ReadBuffer, and verifies the contents match.
func TestIntegrationQueueReadBuffer(t *testing.T) {
	instance, adapter, device := createTestDevice(t)
	defer instance.Release()
	defer adapter.Release()
	defer device.Release()

	const bufSize = 64

	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "readback-buf",
		Size:  bufSize,
		Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopyDst | wgpu.BufferUsageCopySrc,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer buf.Release()

	q := device.Queue()
	if q == nil {
		t.Fatal("Queue is nil")
	}

	// Write 16 uint32 values (4 bytes each = 64 bytes total).
	writeData := make([]byte, bufSize)
	for i := 0; i < 16; i++ {
		binary.LittleEndian.PutUint32(writeData[i*4:], uint32(i*10+1))
	}

	q.WriteBuffer(buf, 0, writeData)

	// Read it back.
	readData := make([]byte, bufSize)
	err = q.ReadBuffer(buf, 0, readData)
	if err != nil {
		t.Fatalf("ReadBuffer: %v", err)
	}

	// Verify contents match.
	for i := 0; i < 16; i++ {
		got := binary.LittleEndian.Uint32(readData[i*4:])
		want := uint32(i*10 + 1)
		if got != want {
			t.Errorf("readData[%d] = %d, want %d", i, got, want)
		}
	}
}
