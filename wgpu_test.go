package wgpu_test

import (
	"errors"
	"testing"

	"github.com/gogpu/wgpu"

	// Import noop backend. Note: the noop backend (BackendEmpty) is skipped by
	// core.Instance during real adapter enumeration. A mock adapter is created
	// instead. Tests that require HAL integration (CreateBuffer, CreateTexture,
	// CreateShaderModule, etc.) are skipped when running on mock devices.
	_ "github.com/gogpu/wgpu/hal/noop"
)

// --- helpers ---

// newInstance creates a fresh Instance for tests.
func newInstance(t *testing.T) *wgpu.Instance {
	t.Helper()
	inst, err := wgpu.CreateInstance(nil)
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	if inst == nil {
		t.Fatal("CreateInstance returned nil")
	}
	return inst
}

// newAdapter requests an adapter from a fresh instance.
func newAdapter(t *testing.T) (*wgpu.Instance, *wgpu.Adapter) {
	t.Helper()
	inst := newInstance(t)
	adapter, err := inst.RequestAdapter(nil)
	if err != nil {
		t.Fatalf("RequestAdapter: %v", err)
	}
	if adapter == nil {
		t.Fatal("RequestAdapter returned nil")
	}
	return inst, adapter
}

// newDevice requests a device from a fresh adapter.
func newDevice(t *testing.T) (*wgpu.Instance, *wgpu.Adapter, *wgpu.Device) {
	t.Helper()
	inst, adapter := newAdapter(t)
	device, err := adapter.RequestDevice(nil)
	if err != nil {
		t.Fatalf("RequestDevice: %v", err)
	}
	if device == nil {
		t.Fatal("RequestDevice returned nil")
	}
	return inst, adapter, device
}

// requireHAL skips the test if the device was created via the mock adapter path
// (no HAL integration). The mock path is used when no real GPU backends are
// available, which is common in CI and headless environments.
func requireHAL(t *testing.T, device *wgpu.Device) {
	t.Helper()
	if device.Queue() == nil {
		t.Skip("skipping: device has no HAL integration (mock adapter; no real GPU backend available)")
	}
}

// --- Instance tests ---

func TestCreateInstance(t *testing.T) {
	inst, err := wgpu.CreateInstance(nil)
	if err != nil {
		t.Fatalf("CreateInstance(nil) returned error: %v", err)
	}
	if inst == nil {
		t.Fatal("CreateInstance(nil) returned nil Instance")
	}
	inst.Release()
}

func TestCreateInstanceWithDescriptor(t *testing.T) {
	tests := []struct {
		name     string
		backends wgpu.Backends
	}{
		{"all backends", wgpu.BackendsAll},
		{"primary backends", wgpu.BackendsPrimary},
		{"vulkan only", wgpu.BackendsVulkan},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inst, err := wgpu.CreateInstance(&wgpu.InstanceDescriptor{
				Backends: tt.backends,
			})
			if err != nil {
				t.Fatalf("CreateInstance returned error: %v", err)
			}
			if inst == nil {
				t.Fatal("CreateInstance returned nil Instance")
			}
			inst.Release()
		})
	}
}

func TestInstanceRelease(t *testing.T) {
	inst := newInstance(t)

	// First release should succeed.
	inst.Release()

	// Second release should be a no-op (idempotent).
	inst.Release()
}

// --- Adapter tests ---

func TestInstanceRequestAdapter(t *testing.T) {
	inst := newInstance(t)
	defer inst.Release()

	adapter, err := inst.RequestAdapter(nil)
	if err != nil {
		t.Fatalf("RequestAdapter: %v", err)
	}
	if adapter == nil {
		t.Fatal("RequestAdapter returned nil Adapter")
	}
	adapter.Release()
}

func TestAdapterInfo(t *testing.T) {
	_, adapter := newAdapter(t)
	defer adapter.Release()

	info := adapter.Info()
	if info.Name == "" {
		t.Error("AdapterInfo.Name is empty")
	}
	// Verify other fields are populated.
	if info.Driver == "" {
		t.Error("AdapterInfo.Driver is empty")
	}
}

func TestAdapterFeatures(t *testing.T) {
	_, adapter := newAdapter(t)
	defer adapter.Release()

	// Features() should not panic. Mock and noop both return 0.
	_ = adapter.Features()
}

func TestAdapterLimits(t *testing.T) {
	_, adapter := newAdapter(t)
	defer adapter.Release()

	limits := adapter.Limits()
	// Both mock and noop adapters return DefaultLimits with non-zero MaxBufferSize.
	if limits.MaxBufferSize == 0 {
		t.Error("Limits.MaxBufferSize should be non-zero")
	}
	if limits.MaxTextureDimension2D == 0 {
		t.Error("Limits.MaxTextureDimension2D should be non-zero")
	}
	if limits.MaxBindGroups == 0 {
		t.Error("Limits.MaxBindGroups should be non-zero")
	}
}

func TestAdapterRelease(t *testing.T) {
	_, adapter := newAdapter(t)

	adapter.Release()
	// Idempotent release.
	adapter.Release()
}

// --- Device tests ---

func TestRequestDevice(t *testing.T) {
	_, adapter := newAdapter(t)
	defer adapter.Release()

	device, err := adapter.RequestDevice(nil)
	if err != nil {
		t.Fatalf("RequestDevice(nil): %v", err)
	}
	if device == nil {
		t.Fatal("RequestDevice returned nil Device")
	}
	device.Release()
}

func TestRequestDeviceWithDescriptor(t *testing.T) {
	_, adapter := newAdapter(t)
	defer adapter.Release()

	device, err := adapter.RequestDevice(&wgpu.DeviceDescriptor{
		Label:          "test-device",
		RequiredLimits: wgpu.DefaultLimits(),
	})
	if err != nil {
		t.Fatalf("RequestDevice with descriptor: %v", err)
	}
	if device == nil {
		t.Fatal("RequestDevice returned nil Device")
	}
	device.Release()
}

func TestDeviceQueue(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	q := device.Queue()
	if q == nil {
		t.Fatal("device.Queue() returned nil")
	}
}

func TestDeviceFeaturesAndLimits(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()

	// Features and Limits work even on mock devices (they read core.Device fields).
	_ = device.Features()

	limits := device.Limits()
	if limits.MaxBufferSize == 0 {
		t.Error("Device Limits.MaxBufferSize should be non-zero")
	}
}

func TestDeviceRelease(t *testing.T) {
	_, _, device := newDevice(t)

	device.Release()
	// Idempotent release.
	device.Release()
}

// --- Buffer tests (require HAL) ---

func TestDeviceCreateBuffer(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "test-buffer",
		Size:  256,
		Usage: wgpu.BufferUsageVertex | wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	if buf == nil {
		t.Fatal("CreateBuffer returned nil")
	}
	buf.Release()
}

func TestDeviceCreateBufferNilDesc(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()

	// nil descriptor should always return an error, even on mock devices.
	buf, err := device.CreateBuffer(nil)
	if err == nil {
		t.Fatal("CreateBuffer(nil) should return error")
	}
	if buf != nil {
		t.Fatal("CreateBuffer(nil) should return nil buffer")
	}
}

func TestBufferProperties(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	desc := &wgpu.BufferDescriptor{
		Label: "props-buffer",
		Size:  512,
		Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopyDst,
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

func TestBufferRelease(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "release-buf",
		Size:  64,
		Usage: wgpu.BufferUsageVertex,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}

	buf.Release()
	// Idempotent release.
	buf.Release()
}

// --- Texture tests (require HAL) ---

func TestDeviceCreateTexture(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "test-texture",
		Size:          wgpu.Extent3D{Width: 64, Height: 64, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Format:        wgpu.TextureFormatRGBA8Unorm,
		Usage:         wgpu.TextureUsageTextureBinding | wgpu.TextureUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	if tex == nil {
		t.Fatal("CreateTexture returned nil")
	}
	defer tex.Release()

	if tex.Format() != wgpu.TextureFormatRGBA8Unorm {
		t.Errorf("Format() = %v, want RGBA8Unorm", tex.Format())
	}
}

func TestDeviceCreateTextureNilDesc(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()

	tex, err := device.CreateTexture(nil)
	if err == nil {
		t.Fatal("CreateTexture(nil) should return error")
	}
	if tex != nil {
		t.Fatal("CreateTexture(nil) should return nil texture")
	}
}

// --- Sampler tests (require HAL) ---

func TestDeviceCreateSampler(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	sampler, err := device.CreateSampler(&wgpu.SamplerDescriptor{
		Label: "test-sampler",
	})
	if err != nil {
		t.Fatalf("CreateSampler: %v", err)
	}
	if sampler == nil {
		t.Fatal("CreateSampler returned nil")
	}
	sampler.Release()
}

func TestDeviceCreateSamplerNilDesc(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	// nil descriptor creates a default sampler.
	sampler, err := device.CreateSampler(nil)
	if err != nil {
		t.Fatalf("CreateSampler(nil): %v", err)
	}
	if sampler == nil {
		t.Fatal("CreateSampler(nil) returned nil")
	}
	sampler.Release()
}

// --- ShaderModule tests (require HAL) ---

func TestDeviceCreateShaderModule(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	mod, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label: "test-shader",
		WGSL:  "@vertex fn vs_main() -> @builtin(position) vec4f { return vec4f(0.0); }",
	})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	if mod == nil {
		t.Fatal("CreateShaderModule returned nil")
	}
	mod.Release()
}

func TestDeviceCreateShaderModuleNilDesc(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()

	mod, err := device.CreateShaderModule(nil)
	if err == nil {
		t.Fatal("CreateShaderModule(nil) should return error")
	}
	if mod != nil {
		t.Fatal("CreateShaderModule(nil) should return nil module")
	}
}

// --- BindGroupLayout tests (require HAL) ---

func TestDeviceCreateBindGroupLayout(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	layout, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label:   "test-bgl",
		Entries: []wgpu.BindGroupLayoutEntry{},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout: %v", err)
	}
	if layout == nil {
		t.Fatal("CreateBindGroupLayout returned nil")
	}
	layout.Release()
}

func TestDeviceCreateBindGroupLayoutNilDesc(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()

	layout, err := device.CreateBindGroupLayout(nil)
	if err == nil {
		t.Fatal("CreateBindGroupLayout(nil) should return error")
	}
	if layout != nil {
		t.Fatal("CreateBindGroupLayout(nil) should return nil layout")
	}
}

// --- CommandEncoder tests (require HAL) ---

func TestDeviceCreateCommandEncoder(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	encoder, err := device.CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{
		Label: "test-encoder",
	})
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	if encoder == nil {
		t.Fatal("CreateCommandEncoder returned nil")
	}
}

func TestDeviceCreateCommandEncoderNilDesc(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	encoder, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder(nil): %v", err)
	}
	if encoder == nil {
		t.Fatal("CreateCommandEncoder(nil) returned nil")
	}
}

func TestCommandEncoderFinish(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	encoder, err := device.CreateCommandEncoder(nil)
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

func TestCommandEncoderFinishTwiceFails(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	encoder, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}

	_, err = encoder.Finish()
	if err != nil {
		t.Fatalf("first Finish: %v", err)
	}

	_, err = encoder.Finish()
	if err == nil {
		t.Fatal("second Finish should fail because encoder is consumed")
	}
}

// --- ComputePass tests (require HAL) ---

func TestCommandEncoderBeginComputePass(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	encoder, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}

	pass, err := encoder.BeginComputePass(&wgpu.ComputePassDescriptor{
		Label: "test-compute-pass",
	})
	if err != nil {
		t.Fatalf("BeginComputePass: %v", err)
	}
	if pass == nil {
		t.Fatal("BeginComputePass returned nil")
	}

	err = pass.End()
	if err != nil {
		t.Fatalf("End: %v", err)
	}
}

func TestCommandEncoderBeginComputePassNilDesc(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	encoder, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}

	pass, err := encoder.BeginComputePass(nil)
	if err != nil {
		t.Fatalf("BeginComputePass(nil): %v", err)
	}
	if pass == nil {
		t.Fatal("BeginComputePass(nil) returned nil")
	}
	_ = pass.End()
}

// --- Pipeline tests (require HAL) ---

func TestDeviceCreateComputePipeline(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label: "compute-shader",
		WGSL:  "@compute @workgroup_size(1) fn main() {}",
	})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	defer shader.Release()

	pipeline, err := device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
		Label:      "test-compute-pipeline",
		Module:     shader,
		EntryPoint: "main",
	})
	if err != nil {
		// Software backend does not support compute pipelines.
		t.Skipf("CreateComputePipeline not supported by this backend: %v", err)
	}
	if pipeline == nil {
		t.Fatal("CreateComputePipeline returned nil")
	}
	pipeline.Release()
}

func TestDeviceCreateComputePipelineNilDesc(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()

	pipeline, err := device.CreateComputePipeline(nil)
	if err == nil {
		t.Fatal("CreateComputePipeline(nil) should return error")
	}
	if pipeline != nil {
		t.Fatal("CreateComputePipeline(nil) should return nil pipeline")
	}
}

func TestDeviceCreateRenderPipeline(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	vertShader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label: "vert-shader",
		WGSL:  "@vertex fn vs_main() -> @builtin(position) vec4f { return vec4f(0.0); }",
	})
	if err != nil {
		t.Fatalf("CreateShaderModule (vertex): %v", err)
	}
	defer vertShader.Release()

	pipeline, err := device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label: "test-render-pipeline",
		Vertex: wgpu.VertexState{
			Module:     vertShader,
			EntryPoint: "vs_main",
		},
	})
	if err != nil {
		t.Fatalf("CreateRenderPipeline: %v", err)
	}
	if pipeline == nil {
		t.Fatal("CreateRenderPipeline returned nil")
	}
	pipeline.Release()
}

func TestDeviceCreateRenderPipelineNilDesc(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()

	pipeline, err := device.CreateRenderPipeline(nil)
	if err == nil {
		t.Fatal("CreateRenderPipeline(nil) should return error")
	}
	if pipeline != nil {
		t.Fatal("CreateRenderPipeline(nil) should return nil pipeline")
	}
}

// --- Queue tests (require HAL) ---

func TestQueueWriteBuffer(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "write-buf",
		Size:  64,
		Usage: wgpu.BufferUsageVertex | wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer buf.Release()

	q := device.Queue()
	if q == nil {
		t.Fatal("Queue is nil")
	}

	// WriteBuffer should not panic.
	data := []byte{1, 2, 3, 4}
	if err := q.WriteBuffer(buf, 0, data); err != nil {
		t.Fatalf("WriteBuffer failed: %v", err)
	}
}

// --- WaitIdle tests (require HAL) ---

func TestDeviceWaitIdle(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	err := device.WaitIdle()
	if err != nil {
		t.Fatalf("WaitIdle: %v", err)
	}
}

// --- ErrorScope tests ---

func TestDeviceErrorScope(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()

	// Push an error scope, then pop it. No error should be captured.
	device.PushErrorScope(wgpu.ErrorFilterValidation)
	gpuErr := device.PopErrorScope()
	if gpuErr != nil {
		t.Errorf("PopErrorScope returned non-nil error: %v", gpuErr)
	}
}

func TestDeviceErrorScopeMultiple(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()

	// Push multiple error scopes and pop them in reverse order.
	device.PushErrorScope(wgpu.ErrorFilterValidation)
	device.PushErrorScope(wgpu.ErrorFilterOutOfMemory)

	gpuErr2 := device.PopErrorScope()
	if gpuErr2 != nil {
		t.Errorf("PopErrorScope (inner) returned non-nil error: %v", gpuErr2)
	}

	gpuErr1 := device.PopErrorScope()
	if gpuErr1 != nil {
		t.Errorf("PopErrorScope (outer) returned non-nil error: %v", gpuErr1)
	}
}

func TestDeviceErrorScopeAllFilters(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()

	filters := []struct {
		name   string
		filter wgpu.ErrorFilter
	}{
		{"Validation", wgpu.ErrorFilterValidation},
		{"OutOfMemory", wgpu.ErrorFilterOutOfMemory},
		{"Internal", wgpu.ErrorFilterInternal},
	}

	for _, f := range filters {
		t.Run(f.name, func(t *testing.T) {
			device.PushErrorScope(f.filter)
			gpuErr := device.PopErrorScope()
			if gpuErr != nil {
				t.Errorf("PopErrorScope for %s returned non-nil error: %v", f.name, gpuErr)
			}
		})
	}
}

// --- Released resource tests ---

func TestReleasedDeviceReturnsError(t *testing.T) {
	_, _, device := newDevice(t)
	device.Release()

	t.Run("CreateBuffer", func(t *testing.T) {
		_, err := device.CreateBuffer(&wgpu.BufferDescriptor{
			Label: "should-fail",
			Size:  64,
			Usage: wgpu.BufferUsageVertex,
		})
		if !errors.Is(err, wgpu.ErrReleased) {
			t.Errorf("CreateBuffer after Release: got %v, want ErrReleased", err)
		}
	})

	t.Run("CreateTexture", func(t *testing.T) {
		_, err := device.CreateTexture(&wgpu.TextureDescriptor{
			Label:         "should-fail",
			Size:          wgpu.Extent3D{Width: 1, Height: 1, DepthOrArrayLayers: 1},
			MipLevelCount: 1,
			SampleCount:   1,
			Format:        wgpu.TextureFormatRGBA8Unorm,
			Usage:         wgpu.TextureUsageTextureBinding,
		})
		if !errors.Is(err, wgpu.ErrReleased) {
			t.Errorf("CreateTexture after Release: got %v, want ErrReleased", err)
		}
	})

	t.Run("CreateSampler", func(t *testing.T) {
		_, err := device.CreateSampler(nil)
		if !errors.Is(err, wgpu.ErrReleased) {
			t.Errorf("CreateSampler after Release: got %v, want ErrReleased", err)
		}
	})

	t.Run("CreateShaderModule", func(t *testing.T) {
		_, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
			Label: "should-fail",
			WGSL:  "test",
		})
		if !errors.Is(err, wgpu.ErrReleased) {
			t.Errorf("CreateShaderModule after Release: got %v, want ErrReleased", err)
		}
	})

	t.Run("CreateBindGroupLayout", func(t *testing.T) {
		_, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
			Label: "should-fail",
		})
		if !errors.Is(err, wgpu.ErrReleased) {
			t.Errorf("CreateBindGroupLayout after Release: got %v, want ErrReleased", err)
		}
	})

	t.Run("CreateCommandEncoder", func(t *testing.T) {
		_, err := device.CreateCommandEncoder(nil)
		if !errors.Is(err, wgpu.ErrReleased) {
			t.Errorf("CreateCommandEncoder after Release: got %v, want ErrReleased", err)
		}
	})

	t.Run("CreateComputePipeline", func(t *testing.T) {
		_, err := device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
			Label:      "should-fail",
			EntryPoint: "main",
		})
		if !errors.Is(err, wgpu.ErrReleased) {
			t.Errorf("CreateComputePipeline after Release: got %v, want ErrReleased", err)
		}
	})

	t.Run("CreateRenderPipeline", func(t *testing.T) {
		_, err := device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
			Label: "should-fail",
		})
		if !errors.Is(err, wgpu.ErrReleased) {
			t.Errorf("CreateRenderPipeline after Release: got %v, want ErrReleased", err)
		}
	})

	t.Run("WaitIdle", func(t *testing.T) {
		err := device.WaitIdle()
		if !errors.Is(err, wgpu.ErrReleased) {
			t.Errorf("WaitIdle after Release: got %v, want ErrReleased", err)
		}
	})
}

// --- Sentinel error tests ---

func TestErrorSentinels(t *testing.T) {
	// All three sentinel errors must be distinct.
	sentinels := []struct {
		name string
		err  error
	}{
		{"ErrReleased", wgpu.ErrReleased},
		{"ErrNoAdapters", wgpu.ErrNoAdapters},
		{"ErrNoBackends", wgpu.ErrNoBackends},
	}

	for i, a := range sentinels {
		for j, b := range sentinels {
			if i != j && errors.Is(a.err, b.err) {
				t.Errorf("%s and %s should be distinct errors", a.name, b.name)
			}
		}
	}

	// Each sentinel should have a non-empty message.
	for _, s := range sentinels {
		if s.err.Error() == "" {
			t.Errorf("%s has empty error message", s.name)
		}
	}
}

func TestErrorSentinelMessages(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantSub string
	}{
		{"ErrReleased", wgpu.ErrReleased, "released"},
		{"ErrNoAdapters", wgpu.ErrNoAdapters, "adapter"},
		{"ErrNoBackends", wgpu.ErrNoBackends, "backend"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := tt.err.Error()
			if msg == "" {
				t.Errorf("%s has empty error message", tt.name)
			}
		})
	}
}

// --- HAL error sentinels re-exported ---

func TestHALErrorSentinels(t *testing.T) {
	// Verify HAL error sentinels are accessible and non-nil.
	halErrors := []struct {
		name string
		err  error
	}{
		{"ErrDeviceLost", wgpu.ErrDeviceLost},
		{"ErrOutOfMemory", wgpu.ErrOutOfMemory},
		{"ErrSurfaceLost", wgpu.ErrSurfaceLost},
		{"ErrSurfaceOutdated", wgpu.ErrSurfaceOutdated},
		{"ErrTimeout", wgpu.ErrTimeout},
	}

	for _, e := range halErrors {
		t.Run(e.name, func(t *testing.T) {
			if e.err == nil {
				t.Errorf("%s is nil", e.name)
			}
			if e.err.Error() == "" {
				t.Errorf("%s has empty error message", e.name)
			}
		})
	}
}

// --- Released adapter tests ---

func TestReleasedAdapterRequestDeviceFails(t *testing.T) {
	_, adapter := newAdapter(t)
	adapter.Release()

	_, err := adapter.RequestDevice(nil)
	if !errors.Is(err, wgpu.ErrReleased) {
		t.Errorf("RequestDevice after adapter.Release: got %v, want ErrReleased", err)
	}
}

// --- Released instance tests ---

func TestReleasedInstanceRequestAdapterFails(t *testing.T) {
	inst := newInstance(t)
	inst.Release()

	_, err := inst.RequestAdapter(nil)
	if !errors.Is(err, wgpu.ErrReleased) {
		t.Errorf("RequestAdapter after instance.Release: got %v, want ErrReleased", err)
	}
}

// --- Full lifecycle tests (require HAL) ---

func TestFullLifecycleCreateAndRelease(t *testing.T) {
	inst, err := wgpu.CreateInstance(nil)
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}

	adapter, err := inst.RequestAdapter(nil)
	if err != nil {
		t.Fatalf("RequestAdapter: %v", err)
	}

	device, err := adapter.RequestDevice(nil)
	if err != nil {
		t.Fatalf("RequestDevice: %v", err)
	}
	requireHAL(t, device)

	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "lifecycle-buf",
		Size:  128,
		Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}

	encoder, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}

	cmdBuf, err := encoder.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	_ = cmdBuf

	// Release in reverse order of creation.
	buf.Release()
	device.Release()
	adapter.Release()
	inst.Release()
}

// --- Compute pass full workflow (require HAL) ---

func TestComputePassFullWorkflow(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label: "compute-wf-shader",
		WGSL:  "@compute @workgroup_size(1) fn main() {}",
	})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	defer shader.Release()

	pipeline, err := device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
		Label:      "compute-wf-pipeline",
		Module:     shader,
		EntryPoint: "main",
	})
	if err != nil {
		// Software backend does not support compute pipelines.
		t.Skipf("CreateComputePipeline not supported by this backend: %v", err)
	}
	defer pipeline.Release()

	encoder, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}

	pass, err := encoder.BeginComputePass(nil)
	if err != nil {
		t.Fatalf("BeginComputePass: %v", err)
	}

	pass.SetPipeline(pipeline)
	pass.Dispatch(1, 1, 1)

	err = pass.End()
	if err != nil {
		t.Fatalf("End: %v", err)
	}

	cmdBuf, err := encoder.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	_ = cmdBuf
}

// --- Table-driven buffer creation tests (require HAL) ---

func TestDeviceCreateBufferTableDriven(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	tests := []struct {
		name    string
		desc    *wgpu.BufferDescriptor
		wantErr bool
	}{
		{
			name: "valid vertex buffer",
			desc: &wgpu.BufferDescriptor{
				Label: "vertex",
				Size:  256,
				Usage: wgpu.BufferUsageVertex,
			},
			wantErr: false,
		},
		{
			name: "valid storage buffer",
			desc: &wgpu.BufferDescriptor{
				Label: "storage",
				Size:  1024,
				Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopyDst,
			},
			wantErr: false,
		},
		{
			name: "valid uniform buffer",
			desc: &wgpu.BufferDescriptor{
				Label: "uniform",
				Size:  64,
				Usage: wgpu.BufferUsageUniform | wgpu.BufferUsageCopyDst,
			},
			wantErr: false,
		},
		{
			name:    "nil descriptor",
			desc:    nil,
			wantErr: true,
		},
		{
			name: "zero size",
			desc: &wgpu.BufferDescriptor{
				Label: "zero",
				Size:  0,
				Usage: wgpu.BufferUsageVertex,
			},
			wantErr: true,
		},
		{
			name: "zero usage",
			desc: &wgpu.BufferDescriptor{
				Label: "no-usage",
				Size:  64,
				Usage: 0,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf, err := device.CreateBuffer(tt.desc)
			if tt.wantErr { //nolint:nestif // table-driven test validation
				if err == nil {
					t.Error("expected error, got nil")
				}
				if buf != nil {
					buf.Release()
					t.Error("expected nil buffer on error")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if buf == nil {
					t.Error("expected non-nil buffer")
				} else {
					buf.Release()
				}
			}
		})
	}
}

// --- Mapped at creation buffer test (require HAL) ---

func TestDeviceCreateBufferMappedAtCreation(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label:            "mapped-buf",
		Size:             64,
		Usage:            wgpu.BufferUsageMapRead | wgpu.BufferUsageCopyDst,
		MappedAtCreation: true,
	})
	if err != nil {
		t.Fatalf("CreateBuffer with MappedAtCreation: %v", err)
	}
	if buf == nil {
		t.Fatal("CreateBuffer with MappedAtCreation returned nil")
	}
	buf.Release()
}

// --- Type alias tests ---

func TestBackendConstants(t *testing.T) {
	// Verify backend constants are accessible and have expected values.
	backends := []struct {
		name string
		b    wgpu.Backend
	}{
		{"Vulkan", wgpu.BackendVulkan},
		{"Metal", wgpu.BackendMetal},
		{"DX12", wgpu.BackendDX12},
		{"GL", wgpu.BackendGL},
	}

	seen := make(map[wgpu.Backend]string)
	for _, b := range backends {
		t.Run(b.name, func(t *testing.T) {
			if prev, exists := seen[b.b]; exists {
				t.Errorf("Backend %s has same value as %s", b.name, prev)
			}
			seen[b.b] = b.name
		})
	}
}

func TestBackendMasks(t *testing.T) {
	// BackendsAll should be non-zero.
	if wgpu.BackendsAll == 0 {
		t.Error("BackendsAll should be non-zero")
	}
	// BackendsPrimary should be non-zero.
	if wgpu.BackendsPrimary == 0 {
		t.Error("BackendsPrimary should be non-zero")
	}
}

func TestBufferUsageConstants(t *testing.T) {
	// Verify buffer usage constants are distinct flags.
	usages := []struct {
		name string
		u    wgpu.BufferUsage
	}{
		{"MapRead", wgpu.BufferUsageMapRead},
		{"MapWrite", wgpu.BufferUsageMapWrite},
		{"CopySrc", wgpu.BufferUsageCopySrc},
		{"CopyDst", wgpu.BufferUsageCopyDst},
		{"Index", wgpu.BufferUsageIndex},
		{"Vertex", wgpu.BufferUsageVertex},
		{"Uniform", wgpu.BufferUsageUniform},
		{"Storage", wgpu.BufferUsageStorage},
		{"Indirect", wgpu.BufferUsageIndirect},
		{"QueryResolve", wgpu.BufferUsageQueryResolve},
	}

	for _, u := range usages {
		t.Run(u.name, func(t *testing.T) {
			if u.u == 0 {
				t.Errorf("BufferUsage%s should be non-zero", u.name)
			}
		})
	}

	// Verify combinability: two flags combined should differ from either alone.
	combined := wgpu.BufferUsageVertex | wgpu.BufferUsageCopyDst
	if combined == wgpu.BufferUsageVertex || combined == wgpu.BufferUsageCopyDst {
		t.Error("Combined buffer usages should differ from individual flags")
	}
}

func TestTextureUsageConstants(t *testing.T) {
	usages := []struct {
		name string
		u    wgpu.TextureUsage
	}{
		{"CopySrc", wgpu.TextureUsageCopySrc},
		{"CopyDst", wgpu.TextureUsageCopyDst},
		{"TextureBinding", wgpu.TextureUsageTextureBinding},
		{"StorageBinding", wgpu.TextureUsageStorageBinding},
		{"RenderAttachment", wgpu.TextureUsageRenderAttachment},
	}

	for _, u := range usages {
		t.Run(u.name, func(t *testing.T) {
			if u.u == 0 {
				t.Errorf("TextureUsage%s should be non-zero", u.name)
			}
		})
	}
}

func TestShaderStageConstants(t *testing.T) {
	stages := []struct {
		name string
		s    wgpu.ShaderStages
	}{
		{"Vertex", wgpu.ShaderStageVertex},
		{"Fragment", wgpu.ShaderStageFragment},
		{"Compute", wgpu.ShaderStageCompute},
	}

	for _, s := range stages {
		t.Run(s.name, func(t *testing.T) {
			if s.s == 0 {
				t.Errorf("ShaderStage%s should be non-zero", s.name)
			}
		})
	}
}

func TestPowerPreferenceConstants(t *testing.T) {
	// PowerPreferenceNone should be the zero value.
	if wgpu.PowerPreferenceNone != 0 {
		t.Error("PowerPreferenceNone should be 0")
	}
	// Others should be distinct.
	if wgpu.PowerPreferenceLowPower == wgpu.PowerPreferenceHighPerformance {
		t.Error("LowPower and HighPerformance should be distinct")
	}
}

func TestErrorFilterConstants(t *testing.T) {
	// Verify error filter constants are accessible.
	filters := []wgpu.ErrorFilter{
		wgpu.ErrorFilterValidation,
		wgpu.ErrorFilterOutOfMemory,
		wgpu.ErrorFilterInternal,
	}

	seen := make(map[wgpu.ErrorFilter]bool)
	for _, f := range filters {
		if seen[f] {
			t.Errorf("Duplicate error filter value: %v", f)
		}
		seen[f] = true
	}
}

func TestDefaultLimitsFunction(t *testing.T) {
	limits := wgpu.DefaultLimits()
	if limits.MaxBufferSize == 0 {
		t.Error("DefaultLimits().MaxBufferSize should be non-zero")
	}
	if limits.MaxTextureDimension2D == 0 {
		t.Error("DefaultLimits().MaxTextureDimension2D should be non-zero")
	}
	if limits.MaxBindGroups == 0 {
		t.Error("DefaultLimits().MaxBindGroups should be non-zero")
	}
}

func TestPresentModeConstants(t *testing.T) {
	modes := []struct {
		name string
		m    wgpu.PresentMode
	}{
		{"Immediate", wgpu.PresentModeImmediate},
		{"Mailbox", wgpu.PresentModeMailbox},
		{"Fifo", wgpu.PresentModeFifo},
		{"FifoRelaxed", wgpu.PresentModeFifoRelaxed},
	}

	seen := make(map[wgpu.PresentMode]string)
	for _, m := range modes {
		t.Run(m.name, func(t *testing.T) {
			if prev, exists := seen[m.m]; exists {
				t.Errorf("PresentMode%s has same value as %s", m.name, prev)
			}
			seen[m.m] = m.name
		})
	}
}

func TestTextureFormatConstants(t *testing.T) {
	formats := []struct {
		name string
		f    wgpu.TextureFormat
	}{
		{"RGBA8Unorm", wgpu.TextureFormatRGBA8Unorm},
		{"RGBA8UnormSrgb", wgpu.TextureFormatRGBA8UnormSrgb},
		{"BGRA8Unorm", wgpu.TextureFormatBGRA8Unorm},
		{"BGRA8UnormSrgb", wgpu.TextureFormatBGRA8UnormSrgb},
		{"Depth24Plus", wgpu.TextureFormatDepth24Plus},
		{"Depth32Float", wgpu.TextureFormatDepth32Float},
	}

	for _, f := range formats {
		t.Run(f.name, func(t *testing.T) {
			if f.f == 0 {
				t.Errorf("TextureFormat%s should be non-zero", f.name)
			}
		})
	}
}
