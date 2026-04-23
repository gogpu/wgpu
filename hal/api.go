package hal

import (
	"time"
	"unsafe"

	"github.com/gogpu/gputypes"
)

// BufferMapping describes a CPU-visible mapping of a GPU buffer.
//
// Returned by Device.MapBuffer. The memory remains valid until the
// corresponding Device.UnmapBuffer call. Callers must ensure the GPU
// is not writing to the mapped region during CPU access — this is the
// caller's responsibility (core coordinates this via submission fences).
//
// Matches Rust wgpu-hal's hal::BufferMapping (wgpu-hal/src/lib.rs:826).
type BufferMapping struct {
	// Ptr is the host-visible pointer to the start of the mapped range.
	// Never nil on success.
	Ptr unsafe.Pointer

	// IsCoherent indicates whether the underlying memory is coherent
	// with GPU writes without explicit flush/invalidate.
	//   - true  → DX12 (always), Metal Shared storage, coherent Vulkan memory
	//   - false → non-coherent Vulkan memory; caller must invalidate before
	//             reading GPU-written data and flush after writing
	//
	// When false, core is responsible for calling FlushMappedRanges /
	// InvalidateMappedRanges at the appropriate times.
	IsCoherent bool
}

// Backend identifies a graphics backend implementation.
// Backends are registered globally and provide factory methods for instances.
type Backend interface {
	// Variant returns the backend type identifier.
	Variant() gputypes.Backend

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
	Info gputypes.AdapterInfo

	// Features are the supported optional features.
	Features gputypes.Features

	// Capabilities contains detailed capability information.
	Capabilities Capabilities
}

// Adapter represents a physical GPU.
// Adapters are enumerated from instances and provide capability queries.
type Adapter interface {
	// Open opens a logical device with the requested features and limits.
	// Returns an error if the adapter cannot support the requested configuration.
	Open(features gputypes.Features, limits gputypes.Limits) (OpenDevice, error)

	// TextureFormatCapabilities returns capabilities for a specific texture format.
	TextureFormatCapabilities(format gputypes.TextureFormat) TextureFormatCapabilities

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

	// MapBuffer establishes a CPU-visible mapping for the given byte range
	// of a host-visible buffer.
	//
	// The buffer must have been created with BufferUsageMapRead or
	// BufferUsageMapWrite (or have been created with MappedAtCreation: true,
	// in which case backends may return the existing mapping directly).
	//
	// Thread-safety contract: the caller (core) must ensure the GPU is not
	// writing to the mapped range while the mapping is active. Typically
	// this means core has observed a submission fence reaching completion
	// before calling MapBuffer.
	//
	// Some backends (Vulkan, Metal Shared, DX12 UPLOAD/READBACK, software)
	// keep buffers persistently mapped; MapBuffer for those backends returns
	// the cached pointer and is cheap. Backends without persistent mappings
	// call the native map API on each call.
	//
	// Returns ErrInvalidMapRange if offset+size exceeds the buffer size or
	// the buffer has no host-visible memory.
	MapBuffer(buffer Buffer, offset, size uint64) (BufferMapping, error)

	// UnmapBuffer releases a CPU-visible mapping established by MapBuffer.
	//
	// For backends with persistent mappings this is a no-op that simply
	// returns nil. For backends without persistent mappings the native
	// unmap API is called.
	//
	// Calling UnmapBuffer on a buffer that is not mapped is undefined —
	// core guarantees the state machine never calls this on an unmapped
	// buffer.
	UnmapBuffer(buffer Buffer) error

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

	// CreateQuerySet creates a query set for timestamp or occlusion queries.
	// Returns ErrTimestampsNotSupported if the backend does not support the query type.
	CreateQuerySet(desc *QuerySetDescriptor) (QuerySet, error)

	// DestroyQuerySet destroys a query set.
	DestroyQuerySet(querySet QuerySet)

	// CreateCommandEncoder creates a command encoder.
	CreateCommandEncoder(desc *CommandEncoderDescriptor) (CommandEncoder, error)

	// CreateRenderBundleEncoder creates a render bundle encoder.
	// Render bundles are pre-recorded command sequences that can be replayed
	// multiple times for better performance.
	CreateRenderBundleEncoder(desc *RenderBundleEncoderDescriptor) (RenderBundleEncoder, error)

	// DestroyRenderBundle destroys a render bundle.
	DestroyRenderBundle(bundle RenderBundle)

	// FreeCommandBuffer returns a command buffer to the command pool.
	// This must be called after the GPU has finished using the command buffer.
	// The command buffer handle becomes invalid after this call.
	FreeCommandBuffer(cmdBuffer CommandBuffer)

	// CreateFence creates a synchronization fence.
	CreateFence() (Fence, error)

	// DestroyFence destroys a fence.
	DestroyFence(fence Fence)

	// Wait waits for a fence to reach the specified value.
	// Returns true if the fence reached the value, false if timeout.
	// Returns ErrDeviceLost if the device is lost.
	Wait(fence Fence, value uint64, timeout time.Duration) (bool, error)

	// ResetFence resets a fence to the unsignaled state.
	// The fence must not be in use by the GPU.
	ResetFence(fence Fence) error

	// GetFenceStatus returns true if the fence is signaled (non-blocking).
	// This is used for polling completion without blocking.
	GetFenceStatus(fence Fence) (bool, error)

	// WaitIdle waits for all GPU work to complete.
	// Call this before destroying resources to ensure the GPU is not using them.
	WaitIdle() error

	// Destroy releases the device.
	// All resources created from this device must be destroyed first.
	Destroy()
}

// Queue handles command submission and presentation.
// Queues are typically thread-safe (backend-specific).
type Queue interface {
	// Submit submits command buffers to the GPU for execution.
	// Returns a monotonically increasing submission index that can be used
	// with PollCompleted to determine when the GPU has finished the work.
	// The HAL manages its own internal fences/synchronization.
	Submit(commandBuffers []CommandBuffer) (submissionIndex uint64, err error)

	// PollCompleted returns the highest submission index known to be completed
	// by the GPU. Non-blocking. Returns 0 if no submissions have completed.
	PollCompleted() uint64

	// WriteBuffer writes data to a buffer immediately.
	// This is a convenience method that creates a staging buffer internally.
	// Returns an error if the buffer is invalid or the write fails.
	WriteBuffer(buffer Buffer, offset uint64, data []byte) error

	// WriteTexture writes data to a texture immediately.
	// This is a convenience method that creates a staging buffer internally.
	// Returns an error if any step in the upload pipeline fails (VK-003).
	WriteTexture(dst *ImageCopyTexture, data []byte, layout *ImageDataLayout, size *Extent3D) error

	// Present presents a surface texture to the screen.
	// The texture must have been acquired via Surface.AcquireTexture.
	// After this call, the texture is consumed and must not be used.
	Present(surface Surface, texture SurfaceTexture) error

	// GetTimestampPeriod returns the timestamp period in nanoseconds.
	// Used to convert timestamp query results to real time.
	GetTimestampPeriod() float32

	// SupportsCommandBufferCopies reports whether this queue uses command-buffer-based
	// copy operations (true for DX12, Vulkan, Metal) or direct API calls (false for
	// GLES, Software). When false, PendingWrites passes WriteBuffer/WriteTexture
	// directly to the HAL without batching.
	SupportsCommandBufferCopies() bool
}

// MaxStagingBufferSizer is an optional interface implemented by HAL devices
// that can report the maximum safe staging buffer allocation size.
//
// For Vulkan, this returns min(64MB, maxMemoryAllocationSize) from
// VkPhysicalDeviceMaintenance3Properties. Without this limit, staging belt
// allocations can silently fail when they exceed the driver's maximum
// allocation size, leading to SIGSEGV (BUG-VK-001).
//
// Backends that do not implement this interface default to 64MB
// (stagingBeltMaxOversizedSize), which is safe for DX12, Metal, and GLES.
type MaxStagingBufferSizer interface {
	// MaxStagingBufferSize returns the maximum size in bytes for a single
	// staging buffer allocation. Returns 0 to use the default (64MB).
	MaxStagingBufferSize() uint64
}
