package hal

// Resource is the base interface for all GPU resources.
// Resources must be explicitly destroyed to free GPU memory.
type Resource interface {
	// Destroy releases the GPU resource.
	// After this call, the resource must not be used.
	// Calling Destroy multiple times is undefined behavior.
	Destroy()
}

// Buffer represents a GPU buffer.
// Buffers are contiguous memory regions accessible by the GPU.
type Buffer interface {
	Resource
}

// Texture represents a GPU texture.
// Textures are multi-dimensional images with specific formats.
type Texture interface {
	Resource
}

// TextureView represents a view into a texture.
// Views specify how a texture is interpreted (format, dimensions, layers).
type TextureView interface {
	Resource
}

// Sampler represents a texture sampler.
// Samplers define how textures are filtered and addressed.
type Sampler interface {
	Resource
}

// ShaderModule represents a compiled shader module.
// Shader modules contain executable GPU code in a backend-specific format.
type ShaderModule interface {
	Resource
}

// BindGroupLayout defines the layout of a bind group.
// Layouts specify the structure of resource bindings for shaders.
type BindGroupLayout interface {
	Resource
}

// BindGroup represents bound resources.
// Bind groups associate actual resources with bind group layouts.
type BindGroup interface {
	Resource
}

// PipelineLayout defines the layout of a pipeline.
// Pipeline layouts specify the bind group layouts used by a pipeline.
type PipelineLayout interface {
	Resource
}

// RenderPipeline is a configured render pipeline.
// Render pipelines define the complete graphics pipeline state.
type RenderPipeline interface {
	Resource
}

// ComputePipeline is a configured compute pipeline.
// Compute pipelines define the compute shader and resource layout.
type ComputePipeline interface {
	Resource
}

// CommandBuffer holds recorded GPU commands.
// Command buffers are immutable after encoding and can be submitted to a queue.
type CommandBuffer interface {
	Resource
}

// Fence is a GPU synchronization primitive.
// Fences allow CPU-GPU synchronization via signaled values.
type Fence interface {
	Resource
}

// Surface represents a rendering surface.
// Surfaces are platform-specific presentation targets (windows).
type Surface interface {
	Resource

	// Configure configures the surface with the given device and settings.
	// Must be called before acquiring textures.
	Configure(device Device, config *SurfaceConfiguration) error

	// Unconfigure removes the surface configuration.
	// Call before destroying the device.
	Unconfigure(device Device)

	// AcquireTexture acquires the next surface texture for rendering.
	// The texture must be presented via Queue.Present or discarded via DiscardTexture.
	// Returns ErrSurfaceOutdated if the surface needs reconfiguration.
	// Returns ErrSurfaceLost if the surface has been destroyed.
	// Returns ErrTimeout if the timeout expires before a texture is available.
	AcquireTexture(fence Fence) (*AcquiredSurfaceTexture, error)

	// DiscardTexture discards a surface texture without presenting it.
	// Use this if rendering failed or was canceled.
	DiscardTexture(texture SurfaceTexture)
}

// SurfaceTexture is a texture acquired from a surface.
// Surface textures have special lifetime constraints - they must be presented
// or discarded before the next frame.
type SurfaceTexture interface {
	Texture
}

// AcquiredSurfaceTexture bundles a surface texture with metadata.
type AcquiredSurfaceTexture struct {
	// Texture is the acquired surface texture.
	Texture SurfaceTexture

	// Suboptimal indicates the surface configuration is suboptimal but usable.
	// Consider reconfiguring the surface at a convenient time.
	Suboptimal bool
}
