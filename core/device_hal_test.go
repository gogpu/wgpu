package core

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// Mock HAL types to satisfy interfaces without returning nil, nil.
type (
	mockBuffer             struct{}
	mockTexture            struct{}
	mockTextureView        struct{}
	mockSampler            struct{}
	mockBindGroupLayout    struct{}
	mockBindGroup          struct{}
	mockPipelineLayout     struct{}
	mockShaderModule       struct{}
	mockRenderPipeline     struct{}
	mockComputePipeline    struct{}
	mockCommandEncoder     struct{}
	mockCommandBuffer      struct{}
	mockFence              struct{}
	mockRenderPassEncoder  struct{}
	mockComputePassEncoder struct{}
)

// Resource interface implementations (Destroy method)
func (mockBuffer) Destroy()          {}
func (mockTexture) Destroy()         {}
func (mockTextureView) Destroy()     {}
func (mockSampler) Destroy()         {}
func (mockBindGroupLayout) Destroy() {}
func (mockBindGroup) Destroy()       {}
func (mockPipelineLayout) Destroy()  {}
func (mockShaderModule) Destroy()    {}
func (mockRenderPipeline) Destroy()  {}
func (mockComputePipeline) Destroy() {}
func (mockCommandBuffer) Destroy()   {}
func (mockFence) Destroy()           {}

// NativeHandle interface implementations
func (mockBuffer) NativeHandle() uintptr      { return 0 }
func (mockTexture) NativeHandle() uintptr     { return 0 }
func (mockTextureView) NativeHandle() uintptr { return 0 }
func (mockSampler) NativeHandle() uintptr     { return 0 }

// mockCommandEncoder implements hal.CommandEncoder
func (mockCommandEncoder) BeginEncoding(_ string) error                           { return nil }
func (mockCommandEncoder) EndEncoding() (hal.CommandBuffer, error)                { return mockCommandBuffer{}, nil }
func (mockCommandEncoder) DiscardEncoding()                                       {}
func (mockCommandEncoder) ResetAll(_ []hal.CommandBuffer)                         {}
func (mockCommandEncoder) TransitionBuffers(_ []hal.BufferBarrier)                {}
func (mockCommandEncoder) TransitionTextures(_ []hal.TextureBarrier)              {}
func (mockCommandEncoder) ClearBuffer(_ hal.Buffer, _, _ uint64)                  {}
func (mockCommandEncoder) CopyBufferToBuffer(_, _ hal.Buffer, _ []hal.BufferCopy) {}
func (mockCommandEncoder) CopyBufferToTexture(_ hal.Buffer, _ hal.Texture, _ []hal.BufferTextureCopy) {
}
func (mockCommandEncoder) CopyTextureToBuffer(_ hal.Texture, _ hal.Buffer, _ []hal.BufferTextureCopy) {
}
func (mockCommandEncoder) CopyTextureToTexture(_, _ hal.Texture, _ []hal.TextureCopy) {}
func (mockCommandEncoder) BeginRenderPass(_ *hal.RenderPassDescriptor) hal.RenderPassEncoder {
	return mockRenderPassEncoder{}
}
func (mockCommandEncoder) BeginComputePass(_ *hal.ComputePassDescriptor) hal.ComputePassEncoder {
	return mockComputePassEncoder{}
}

// mockRenderPassEncoder implements hal.RenderPassEncoder
func (mockRenderPassEncoder) End()                                                          {}
func (mockRenderPassEncoder) SetPipeline(_ hal.RenderPipeline)                              {}
func (mockRenderPassEncoder) SetBindGroup(_ uint32, _ hal.BindGroup, _ []uint32)            {}
func (mockRenderPassEncoder) SetVertexBuffer(_ uint32, _ hal.Buffer, _ uint64)              {}
func (mockRenderPassEncoder) SetIndexBuffer(_ hal.Buffer, _ gputypes.IndexFormat, _ uint64) {}
func (mockRenderPassEncoder) SetViewport(_, _, _, _, _, _ float32)                          {}
func (mockRenderPassEncoder) SetScissorRect(_, _, _, _ uint32)                              {}
func (mockRenderPassEncoder) SetBlendConstant(_ *gputypes.Color)                            {}
func (mockRenderPassEncoder) SetStencilReference(_ uint32)                                  {}
func (mockRenderPassEncoder) Draw(_, _, _, _ uint32)                                        {}
func (mockRenderPassEncoder) DrawIndexed(_, _, _ uint32, _ int32, _ uint32)                 {}
func (mockRenderPassEncoder) DrawIndirect(_ hal.Buffer, _ uint64)                           {}
func (mockRenderPassEncoder) DrawIndexedIndirect(_ hal.Buffer, _ uint64)                    {}
func (mockRenderPassEncoder) ExecuteBundle(_ hal.RenderBundle)                              {}

// mockComputePassEncoder implements hal.ComputePassEncoder (minimal)
func (mockComputePassEncoder) End()                                               {}
func (mockComputePassEncoder) SetPipeline(_ hal.ComputePipeline)                  {}
func (mockComputePassEncoder) SetBindGroup(_ uint32, _ hal.BindGroup, _ []uint32) {}
func (mockComputePassEncoder) Dispatch(_, _, _ uint32)                            {}
func (mockComputePassEncoder) DispatchIndirect(_ hal.Buffer, _ uint64)            {}
func (mockComputePassEncoder) PushDebugGroup(_ string)                            {}
func (mockComputePassEncoder) PopDebugGroup()                                     {}
func (mockComputePassEncoder) InsertDebugMarker(_ string)                         {}

type mockHALDevice struct {
	destroyed bool
}

func (m *mockHALDevice) CreateBuffer(_ *hal.BufferDescriptor) (hal.Buffer, error) {
	return mockBuffer{}, nil
}
func (m *mockHALDevice) DestroyBuffer(_ hal.Buffer) {}
func (m *mockHALDevice) CreateTexture(_ *hal.TextureDescriptor) (hal.Texture, error) {
	return mockTexture{}, nil
}
func (m *mockHALDevice) DestroyTexture(_ hal.Texture) {}
func (m *mockHALDevice) CreateTextureView(_ hal.Texture, _ *hal.TextureViewDescriptor) (hal.TextureView, error) {
	return mockTextureView{}, nil
}
func (m *mockHALDevice) DestroyTextureView(_ hal.TextureView) {}
func (m *mockHALDevice) CreateSampler(_ *hal.SamplerDescriptor) (hal.Sampler, error) {
	return mockSampler{}, nil
}
func (m *mockHALDevice) DestroySampler(_ hal.Sampler) {}
func (m *mockHALDevice) CreateBindGroupLayout(_ *hal.BindGroupLayoutDescriptor) (hal.BindGroupLayout, error) {
	return mockBindGroupLayout{}, nil
}
func (m *mockHALDevice) DestroyBindGroupLayout(_ hal.BindGroupLayout) {}
func (m *mockHALDevice) CreateBindGroup(_ *hal.BindGroupDescriptor) (hal.BindGroup, error) {
	return mockBindGroup{}, nil
}
func (m *mockHALDevice) DestroyBindGroup(_ hal.BindGroup) {}
func (m *mockHALDevice) CreatePipelineLayout(_ *hal.PipelineLayoutDescriptor) (hal.PipelineLayout, error) {
	return mockPipelineLayout{}, nil
}
func (m *mockHALDevice) DestroyPipelineLayout(_ hal.PipelineLayout) {}
func (m *mockHALDevice) CreateShaderModule(_ *hal.ShaderModuleDescriptor) (hal.ShaderModule, error) {
	return mockShaderModule{}, nil
}
func (m *mockHALDevice) DestroyShaderModule(_ hal.ShaderModule) {}
func (m *mockHALDevice) CreateRenderPipeline(_ *hal.RenderPipelineDescriptor) (hal.RenderPipeline, error) {
	return mockRenderPipeline{}, nil
}
func (m *mockHALDevice) DestroyRenderPipeline(_ hal.RenderPipeline) {}
func (m *mockHALDevice) CreateComputePipeline(_ *hal.ComputePipelineDescriptor) (hal.ComputePipeline, error) {
	return mockComputePipeline{}, nil
}
func (m *mockHALDevice) DestroyComputePipeline(_ hal.ComputePipeline) {}
func (m *mockHALDevice) CreateCommandEncoder(_ *hal.CommandEncoderDescriptor) (hal.CommandEncoder, error) {
	return mockCommandEncoder{}, nil
}
func (m *mockHALDevice) CreateFence() (hal.Fence, error) { return mockFence{}, nil }
func (m *mockHALDevice) DestroyFence(_ hal.Fence)        {}
func (m *mockHALDevice) Wait(_ hal.Fence, _ uint64, _ time.Duration) (bool, error) {
	return true, nil
}
func (m *mockHALDevice) ResetFence(_ hal.Fence) error             { return nil }
func (m *mockHALDevice) GetFenceStatus(_ hal.Fence) (bool, error) { return true, nil }
func (m *mockHALDevice) FreeCommandBuffer(_ hal.CommandBuffer)    {}
func (m *mockHALDevice) CreateRenderBundleEncoder(_ *hal.RenderBundleEncoderDescriptor) (hal.RenderBundleEncoder, error) {
	return nil, fmt.Errorf("mock: render bundles not supported")
}
func (m *mockHALDevice) DestroyRenderBundle(_ hal.RenderBundle) {}
func (m *mockHALDevice) WaitIdle() error                        { return nil }
func (m *mockHALDevice) Destroy()                               { m.destroyed = true }

func TestDevice_NewDevice(t *testing.T) {
	adapter := &Adapter{Info: gputypes.AdapterInfo{Name: "Test"}}
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, adapter, gputypes.Features(0), gputypes.DefaultLimits(), "Test")
	if device == nil {
		t.Fatal("NewDevice returned nil")
	}
	if !device.IsValid() {
		t.Error("New device should be valid")
	}
	if !device.HasHAL() {
		t.Error("Device.HasHAL() should return true")
	}
	if device.SnatchLock() == nil {
		t.Error("Device.SnatchLock() should not return nil")
	}
}

func TestDevice_RawAccess(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "Test")
	lock := device.SnatchLock()
	guard := lock.Read()
	defer guard.Release()
	raw := device.Raw(guard)
	if raw == nil {
		t.Error("Raw() should not return nil")
	}
}

func TestDevice_Destroy(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "Test")
	if !device.IsValid() {
		t.Error("Device should be valid before destroy")
	}
	device.Destroy()
	if device.IsValid() {
		t.Error("Device should not be valid after destroy")
	}
	if !halDevice.destroyed {
		t.Error("HAL device should be destroyed")
	}
}

func TestDevice_DestroyIdempotent(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "Test")
	device.Destroy()
	device.Destroy()
	device.Destroy()
	if device.IsValid() {
		t.Error("Device should not be valid")
	}
}

func TestDevice_IsValid(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "Test")
	if !device.IsValid() {
		t.Error("Should be valid")
	}
	device.Destroy()
	if device.IsValid() {
		t.Error("Should be invalid")
	}
}

func TestDevice_SnatchLock(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "Test")
	if device.SnatchLock() == nil {
		t.Error("Should have lock")
	}
	nonHAL := &Device{Label: "Test"}
	if nonHAL.SnatchLock() != nil {
		t.Error("Should not have lock")
	}
}

func TestDevice_ConcurrentRawAccess(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "Test")
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			lock := device.SnatchLock()
			guard := lock.Read()
			defer guard.Release()
			_ = device.Raw(guard)
		}()
	}
	wg.Wait()
}

func TestDevice_checkValid(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "Test")
	if err := device.checkValid(); err != nil {
		t.Error("Should be valid")
	}
	device.Destroy()
	if err := device.checkValid(); !errors.Is(err, ErrDeviceDestroyed) {
		t.Error("Should be ErrDeviceDestroyed")
	}
}

func TestDevice_AssociatedQueue(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "Test")
	if device.AssociatedQueue() != nil {
		t.Error("Should be nil initially")
	}
	queue := &Queue{Label: "Test Queue"}
	device.SetAssociatedQueue(queue)
	if device.AssociatedQueue() != queue {
		t.Error("Should return set queue")
	}
}
