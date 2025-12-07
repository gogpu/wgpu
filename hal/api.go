package hal

import (
	"time"

	"github.com/gogpu/wgpu/types"
)

// Backend identifies a graphics backend implementation.
// Backends are registered globally and provide factory methods for instances.
type Backend interface {
	// Variant returns the backend type identifier.
	Variant() types.Backend

	// CreateInstance creates a new GPU instance with the given configuration.
	// Returns an error if instance creation fails (e.g., drivers not available).
	CreateInstance(desc *InstanceDescriptor) (Instance, error)
}

// Instance is the entry point for GPU operations.
// An instance manages adapter enumeration and surface creation.
type Instance interface {
	// CreateSurface creates a rendering surface from platform handles.
	// displayHandle is platform-specific (HDC on Windows, NSWindow* on macOS, etc.).
	// windowHandle is the window handle (HWND on Windows, NSView* on macOS, etc.).
	CreateSurface(displayHandle, windowHandle uintptr) (Surface, error)

	// EnumerateAdapters enumerates available physical GPUs.
	// surfaceHint is optional - if provided, only adapters compatible with
	// the surface are returned.
	EnumerateAdapters(surfaceHint Surface) []ExposedAdapter

	// Destroy releases the instance.
	// All adapters and surfaces created from this instance must be destroyed first.
	Destroy()
}

// ExposedAdapter bundles an adapter with its capabilities.
// This is returned by Instance.EnumerateAdapters.
type ExposedAdapter struct {
	// Adapter is the physical GPU.
	Adapter Adapter

	// Info contains adapter metadata (name, vendor, device type).
	Info types.AdapterInfo

	// Features are the supported optional features.
	Features types.Features

	// Capabilities contains detailed capability information.
	Capabilities Capabilities
}

// Adapter represents a physical GPU.
// Adapters are enumerated from instances and provide capability queries.
type Adapter interface {
	// Open opens a logical device with the requested features and limits.
	// Returns an error if the adapter cannot support the requested configuration.
	Open(features types.Features, limits types.Limits) (OpenDevice, error)

	// TextureFormatCapabilities returns capabilities for a specific texture format.
	TextureFormatCapabilities(format types.TextureFormat) TextureFormatCapabilities

	// SurfaceCapabilities returns capabilities for a specific surface.
	// Returns nil if the adapter is not compatible with the surface.
	SurfaceCapabilities(surface Surface) *SurfaceCapabilities

	// Destroy releases the adapter.
	// Any devices created from this adapter must be destroyed first.
	Destroy()
}

// OpenDevice is returned when Adapter.Open succeeds.
// It bundles the device and queue together since they're created atomically.
type OpenDevice struct {
	// Device is the logical GPU device.
	Device Device

	// Queue is the device's command queue.
	Queue Queue
}

// Device represents a logical GPU device.
// Devices are used to create resources and command encoders.
type Device interface {
	// CreateBuffer creates a GPU buffer.
	CreateBuffer(desc *BufferDescriptor) (Buffer, error)

	// DestroyBuffer destroys a GPU buffer.
	DestroyBuffer(buffer Buffer)

	// CreateTexture creates a GPU texture.
	CreateTexture(desc *TextureDescriptor) (Texture, error)

	// DestroyTexture destroys a GPU texture.
	DestroyTexture(texture Texture)

	// CreateTextureView creates a view into a texture.
	CreateTextureView(texture Texture, desc *TextureViewDescriptor) (TextureView, error)

	// DestroyTextureView destroys a texture view.
	DestroyTextureView(view TextureView)

	// CreateSampler creates a texture sampler.
	CreateSampler(desc *SamplerDescriptor) (Sampler, error)

	// DestroySampler destroys a sampler.
	DestroySampler(sampler Sampler)

	// CreateBindGroupLayout creates a bind group layout.
	CreateBindGroupLayout(desc *BindGroupLayoutDescriptor) (BindGroupLayout, error)

	// DestroyBindGroupLayout destroys a bind group layout.
	DestroyBindGroupLayout(layout BindGroupLayout)

	// CreateBindGroup creates a bind group.
	CreateBindGroup(desc *BindGroupDescriptor) (BindGroup, error)

	// DestroyBindGroup destroys a bind group.
	DestroyBindGroup(group BindGroup)

	// CreatePipelineLayout creates a pipeline layout.
	CreatePipelineLayout(desc *PipelineLayoutDescriptor) (PipelineLayout, error)

	// DestroyPipelineLayout destroys a pipeline layout.
	DestroyPipelineLayout(layout PipelineLayout)

	// CreateShaderModule creates a shader module.
	CreateShaderModule(desc *ShaderModuleDescriptor) (ShaderModule, error)

	// DestroyShaderModule destroys a shader module.
	DestroyShaderModule(module ShaderModule)

	// CreateRenderPipeline creates a render pipeline.
	CreateRenderPipeline(desc *RenderPipelineDescriptor) (RenderPipeline, error)

	// DestroyRenderPipeline destroys a render pipeline.
	DestroyRenderPipeline(pipeline RenderPipeline)

	// CreateComputePipeline creates a compute pipeline.
	CreateComputePipeline(desc *ComputePipelineDescriptor) (ComputePipeline, error)

	// DestroyComputePipeline destroys a compute pipeline.
	DestroyComputePipeline(pipeline ComputePipeline)

	// CreateCommandEncoder creates a command encoder.
	CreateCommandEncoder(desc *CommandEncoderDescriptor) (CommandEncoder, error)

	// CreateFence creates a synchronization fence.
	CreateFence() (Fence, error)

	// DestroyFence destroys a fence.
	DestroyFence(fence Fence)

	// Wait waits for a fence to reach the specified value.
	// Returns true if the fence reached the value, false if timeout.
	// Returns ErrDeviceLost if the device is lost.
	Wait(fence Fence, value uint64, timeout time.Duration) (bool, error)

	// Destroy releases the device.
	// All resources created from this device must be destroyed first.
	Destroy()
}

// Queue handles command submission and presentation.
// Queues are typically thread-safe (backend-specific).
type Queue interface {
	// Submit submits command buffers to the GPU.
	// If fence is not nil, it will be signaled with fenceValue when commands complete.
	Submit(commandBuffers []CommandBuffer, fence Fence, fenceValue uint64) error

	// WriteBuffer writes data to a buffer immediately.
	// This is a convenience method that creates a staging buffer internally.
	WriteBuffer(buffer Buffer, offset uint64, data []byte)

	// WriteTexture writes data to a texture immediately.
	// This is a convenience method that creates a staging buffer internally.
	WriteTexture(dst *ImageCopyTexture, data []byte, layout *ImageDataLayout, size *Extent3D)

	// Present presents a surface texture to the screen.
	// The texture must have been acquired via Surface.AcquireTexture.
	// After this call, the texture is consumed and must not be used.
	Present(surface Surface, texture SurfaceTexture) error

	// GetTimestampPeriod returns the timestamp period in nanoseconds.
	// Used to convert timestamp query results to real time.
	GetTimestampPeriod() float32
}
