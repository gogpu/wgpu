// Package wgpu provides a safe, ergonomic WebGPU API for Go applications.
//
// This package wraps the lower-level implementation packages (internal/hal and
// internal/core) into a user-friendly API aligned with the W3C WebGPU
// specification.
//
// # Quick Start
//
// Import this package; the platform backends are registered automatically:
//
//	import "github.com/gogpu/wgpu"
//
//	instance, err := wgpu.CreateInstance(nil)
//	// ...
//
// # Resource Lifecycle
//
// All GPU resources must be explicitly released with Release().
// Resources are reference-counted internally. Release is idempotent; methods
// on a released resource that return an error report ErrReleased.
//
// # Error Scopes
//
// Device.PushErrorScope and Device.PopErrorScope implement the WebGPU error
// scope model. PopErrorScope returns (*GPUError, error), and at most one side
// is non-nil: a captured error; (nil, nil) when the scope captured nothing, the
// common path; or (nil, ErrErrorScopeEmpty) on an empty stack — the spec's
// OperationError case, never a panic. A captured *GPUError answers errors.Is
// against ErrValidation, ErrOutOfMemory and ErrInternal through its Unwrap, and
// carries the category and message itself. Scopes capture the validation
// errors raised while recording commands and by Submit. On the browser build
// the scopes drive the JS device's own pushErrorScope and popErrorScope.
//
// # Backend Registration
//
// The platform hardware backends register by default (ADR-083): Vulkan, DX12
// and OpenGL ES on Windows (386, amd64, arm64), Vulkan and OpenGL ES on Linux
// (amd64, arm64), Metal on macOS (amd64, arm64; MoltenVK only with
// -tags wgpu_moltenvk), Vulkan and OpenGL ES on Android (arm64), and the
// browser's WebGPU on js/wasm. The Pure Go software backend registers by
// default too, outside the default adapter set: an empty Backends mask never
// picks it. Select it by naming gputypes.BackendsSoftware in the mask;
// RequestAdapterOptions.ForceFallbackAdapter then prefers it over other CPU
// adapters. Other GOOS/GOARCH combinations are outside the default registration
// set, and the backend packages are not importable outside the module; some of
// them cannot build the Pure Go backends at all (goffi has no syscall files for
// them).
//
// Build tags remove a backend for a smaller binary: wgpu_novulkan,
// wgpu_nodx12, wgpu_nometal, wgpu_nogles. wgpu_nosoftware removes the Pure Go
// software backend from the default registration and from wgputest.
//
// GOOS=ios satisfies the darwin build tag, so iOS builds in the default set
// register Metal.
//
// # Thread Safety
//
// Instance, Adapter, and Device are safe for concurrent use.
// Encoders (CommandEncoder, RenderPassEncoder, ComputePassEncoder) are NOT thread-safe.
//
// Resources (Buffer, Texture, TextureView, Sampler, ShaderModule, QuerySet,
// Fence, BindGroup, BindGroupLayout, PipelineLayout and the pipelines) are
// safe to Release concurrently, from any number of goroutines, together with
// their other methods: the released flag is a compare-and-swap guard, so
// exactly one caller wins and at most one HAL destroy is scheduled — the HAL
// object goes when the last reference drops (on GPU completion for work still
// in flight). Release is idempotent, and Release on a zero-value resource is
// a no-op. Every other method of a resource is read-only after construction
// and safe concurrently, except buffer mapping, where the spec's rule stands:
// one mapping and one mapped range per buffer at a time, invalid after Unmap
// or Release.
package wgpu
