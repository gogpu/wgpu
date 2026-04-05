# Architecture

This document describes the architecture of `wgpu` — a Pure Go WebGPU implementation.

## Overview

```
┌─────────────────────────────────────────────────┐
│                   User Code                     │
│   import "github.com/gogpu/wgpu"                │
│   _ "github.com/gogpu/wgpu/hal/allbackends"     │
└──────────────────────┬──────────────────────────┘
                       │
┌──────────────────────▼──────────────────────────┐
│              Root Package (wgpu/)                │
│  Safe, ergonomic public API (WebGPU-aligned)    │
│  Instance · Adapter · Device · Queue · Buffer   │
│  Texture · Pipeline · CommandEncoder · Surface   │
└──────────────────────┬──────────────────────────┘
                       │ wraps
┌──────────────────────▼──────────────────────────┐
│                  core/                          │
│      Validation, state tracking, error scopes   │
│   (Instance, Adapter, Device, Queue, Pipeline)  │
└──────────────────────┬──────────────────────────┘
                       │
┌──────────────────────▼──────────────────────────┐
│                  hal/                           │
│     Hardware Abstraction Layer (interfaces)     │
│  Backend · Instance · Adapter · Device · Queue  │
│  CommandEncoder · RenderPass · ComputePass      │
└──────┬────────┬────────┬────────┬────────┬──────┘
       │        │        │        │        │
┌──────▼──┐┌───▼────┐┌──▼───┐┌────▼───┐┌───▼──────┐
│ vulkan/ ││ metal/ ││ dx12/││ gles/  ││software/ │
│ Vulkan  ││ Metal  ││ DX12 ││OpenGLES││  CPU     │
│1.0+ API ││ macOS  ││ Win  ││ 3.0+   ││rasterizer│
└─────────┘└────────┘└──────┘└────────┘└──────────┘
```

## Layers

### Root Package (`wgpu/`) — Public API

The user-facing API layer. Wraps `core/` and `hal/` into safe, ergonomic types aligned with the W3C WebGPU specification.

- **Type safety** — Public types hide internal HAL handles; users never touch `unsafe.Pointer`
- **Go-idiomatic errors** — All fallible methods return `(T, error)`
- **Deterministic cleanup** — `Release()` on all resource types
- **Type aliases** — Re-exports from `gputypes` so users don't need a separate import
- **Descriptor conversion** — Public descriptors auto-convert to HAL descriptors via `toHAL()` methods

Key types: `Instance`, `Adapter`, `Device`, `Queue`, `Buffer`, `Texture`, `TextureView`, `Sampler`, `ShaderModule`, `BindGroupLayout`, `PipelineLayout`, `BindGroup`, `RenderPipeline`, `ComputePipeline`, `CommandEncoder`, `CommandBuffer`, `RenderPassEncoder`, `ComputePassEncoder`, `Surface`, `SurfaceTexture`.

### `core/` — Validation & State Tracking

Validation layer between the public API and HAL. Core validates exhaustively — HAL assumes validated input.

- **Spec validation** — `core/validate.go` implements 30+ WebGPU spec rules for textures (dimensions, limits, multisampling, formats), samplers (LOD, anisotropy), shaders (source presence), pipelines (stages, targets), bind groups and layouts. Draw-time validation includes blend constant tracking (VAL-005) and resource usage conflict detection (BufferTracker)
- **Typed errors** — `core/error.go` defines 7 typed error types (`CreateTextureError`, `CreateSamplerError`, `CreateShaderModuleError`, `CreateRenderPipelineError`, `CreateComputePipelineError`, `CreateBindGroupLayoutError`, `CreateBindGroupError`) with specific error kinds and context fields, supporting `errors.As()` for programmatic handling
- **Deferred errors** — WebGPU pattern: encoding-phase errors are recorded via `SetError()` and surface at `End()` / `Finish()`
- **Error scopes** — WebGPU error handling model (`PushErrorScope` / `PopErrorScope`)
- **Resource tracking** — Leak detection in debug builds
- **Structured logging** — `log/slog` integration, silent by default

Key types: `Instance`, `Adapter`, `Device`, `Queue`, `Buffer`, `Texture`, `RenderPipeline`, `ComputePipeline`, `CommandEncoder`, `CommandBuffer`, `Surface`.

- **Surface lifecycle** — `core.Surface` manages the Unconfigured → Configured → Acquired state machine with mutex-protected transitions. Validates state (can't acquire twice, can't present without acquire). Includes `PrepareFrameFunc` hook for platform HiDPI/DPI integration (Metal contentsScale, Windows WM_DPICHANGED, Wayland wl_output.scale).
- **CommandEncoder lifecycle** — `core.CommandEncoder` tracks pass state (Recording → InRenderPass/InComputePass → Finished) with validated transitions.
- **Resource types** — All 17 resource types have full struct definitions with HAL handles wrapped in `Snatchable` for safe destruction, device references, and WebGPU properties.

### `hal/` — Hardware Abstraction Layer

Backend-agnostic interfaces that each graphics API implements. HAL methods assume input is validated by `core/` — they retain only nil pointer guards as defense-in-depth (prefixed with `"BUG: ..."` to signal core validation gaps if triggered).

Key interfaces (defined in `hal/api.go`):

| Interface | Responsibility |
|-----------|---------------|
| `Backend` | Factory for creating instances |
| `Instance` | Surface creation, adapter enumeration |
| `Adapter` | Physical GPU, capability queries |
| `Device` | Resource creation (buffers, textures, pipelines) |
| `Queue` | Command submission, presentation |
| `CommandEncoder` | Command recording |
| `RenderPassEncoder` | Render pass commands |
| `ComputePassEncoder` | Compute dispatch commands |

### `hal/vulkan/` — Vulkan Backend

Pure Go Vulkan 1.0+ implementation using `cgo_import_dynamic` for function loading.

- `vk/` — Low-level Vulkan bindings (generated types, function signatures, loader)
- `memory/` — GPU memory allocator (buddy allocation)
- Platform surface: VkWin32, VkXlib, VkMetal

### `hal/metal/` — Metal Backend

Pure Go Metal implementation via Objective-C runtime message sending.

- `objc.go` — Objective-C runtime (`objc_msgSend`, `NSAutoreleasePool`, selectors)
- `encoder.go` — Command encoder, render/compute pass encoders
- `device.go` — Device, resource creation, fence management
- `queue.go` — Command submission, texture writes
- Uses scoped autorelease pools (create + drain in same function)

### `hal/dx12/` — DirectX 12 Backend

Pure Go DX12 implementation via COM interfaces.

- `d3d12/` — D3D12 COM interfaces, GUID definitions, loader
- `dxgi/` — DXGI factory, adapter enumeration
- `device.go` — Device, resource creation, descriptor heaps (SRV/sampler)
- `command.go` — Command encoder with resource barriers (buffer/texture state transitions)
- `queue.go` — Command submission with fence-based GPU completion tracking
- `resource.go` — Buffers (upload/default heaps), textures with deferred destruction
- Deferred descriptor destruction: BindGroup/TextureView heap slots freed only after GPU completion (BUG-DX12-007)
- Texture pending refs: prevents premature Release while GPU copies in-flight (BUG-DX12-006)
- Buffer barriers: COPY_DEST → read-state transitions after PendingWrites (BUG-DX12-010)
- Windows-only (`//go:build windows`)

### `hal/gles/` — OpenGL ES Backend

Pure Go OpenGL ES 3.0+ / OpenGL 4.3+ implementation.

- `gl/` — OpenGL function bindings (Windows syscall + Linux goffi)
- `egl/` — EGL context and display management (Linux)
- `wgl/` — WGL context for Windows
- `shader.go` — WGSL → GLSL 4.30 via naga, with BindingMap for flat binding indices
- `sampler.go` — GL sampler objects (glGenSamplers/glBindSampler, GL 3.3+)
- `command.go` — SamplerBindMap: maps WGSL separate texture+sampler to GLSL combined sampler2D (from naga TextureMappings)
- Texture completeness: `GL_TEXTURE_MAX_LEVEL = MipLevelCount-1` at creation (default 1000 makes non-mipmapped textures incomplete)
- Texture updates via `glTexSubImage2D` (not `glTexImage2D`) — matches Rust wgpu-hal pattern
- `GL_DYNAMIC_DRAW` for all writable buffers (Rust wgpu-hal parity — some vendors freeze STATIC_DRAW buffers)
- Scissor Y-flip: WebGPU top-left → OpenGL bottom-left origin conversion
- MSAA resolve via `glBlitFramebuffer`
- Texture unit validation: warns when binding exceeds GL_MAX_TEXTURE_IMAGE_UNITS

### `hal/software/` — Software Backend

CPU-based rasterizer. Always compiled (no build tags required). Pure Go, zero system dependencies.

- `raster/` — Triangle rasterization, blending, depth/stencil, tiling
- `shader/` — Software shader execution (callback-based)

Use cases: headless rendering (servers, CI/CD), testing without GPU, embedded systems, fallback when no GPU available.

### `hal/noop/` — No-op Backend

Stub implementation for testing. All operations succeed without GPU interaction.

## Backend Registration

Backends register via `init()` functions. Import `hal/allbackends` to auto-register platform-appropriate backends:

```go
import _ "github.com/gogpu/wgpu/hal/allbackends"
```

Platform selection (`hal/allbackends/`):

| Platform | Backends |
|----------|----------|
| Windows | Vulkan, DX12, GLES, Software, Noop |
| macOS | Metal, Software, Noop |
| Linux | Vulkan, GLES, Software, Noop |

Backend priority for auto-selection: Vulkan > Metal > DX12 > GLES > Software > Noop.

## PendingWrites (Rust wgpu-core Pattern)

`pending_writes.go` batches `WriteBuffer`/`WriteTexture` operations into a single command encoder, prepended before user command buffers at `Submit()`. Matches Rust wgpu-core's `PendingWrites` architecture.

```
WriteBuffer(buf, data) ──┐
WriteBuffer(buf2, data) ─┤ accumulated in shared encoder
WriteTexture(tex, data) ─┘
                          │
Queue.Submit(userCmds)    │
  ├─ flush() ─────────────┘ → pendingCmdBuf
  ├─ HAL Submit([pendingCmdBuf, userCmds...])
  └─ track inflight resources (staging, encoders, deferred descriptors)
```

**Batching backends** (DX12, Vulkan, Metal): sub-allocate from StagingBelt chunks, record `CopyBufferToBuffer`/`CopyBufferToTexture` via command encoder. Encoder pool recycles allocators after GPU completion.

**StagingBelt** (`staging_belt.go`): ring-buffer of reusable 256KB staging chunks with bump-pointer sub-allocation. Matches Rust wgpu `util::StagingBelt` (belt.rs). Zero heap allocations in steady state — chunks are pre-allocated and recycled after GPU completion. Oversized writes (> chunkSize) fall back to one-off buffers.

```
Chunk lifecycle:  free → active (sub-allocating) → closed (GPU in-flight) → free (recycled)
Steady-state:     0 allocs/op, 22ns — 15× faster than per-write staging
```

**Direct-write backends** (GLES, Software): `usesBatching=false`, delegate directly to `hal.Queue.WriteBuffer()`/`WriteTexture()`. No staging, no command encoder, no belt.

**Deferred destruction** (BUG-DX12-007): BindGroup/TextureView descriptor heap slots are deferred via `core.DestroyQueue.Defer()` (same mechanism as all other resources) and freed only after GPU completes the submission. Prevents descriptor use-after-free with `maxFramesInFlight=2`.

## Resource Lifecycle

### Public API (recommended)

```go
instance, _ := wgpu.CreateInstance(nil)
defer instance.Release()

adapter, _ := instance.RequestAdapter(nil)
defer adapter.Release()

device, _ := adapter.RequestDevice(nil)
defer device.Release()

buffer, _ := device.CreateBuffer(&wgpu.BufferDescriptor{...})
defer buffer.Release()

encoder, _ := device.CreateCommandEncoder(nil)
pass, _ := encoder.BeginComputePass(nil)
// ... record commands ...
pass.End()
cmdBuf, _ := encoder.Finish()
_, _ = device.Queue().Submit(cmdBuf)  // non-blocking, returns submissionIndex
```

### Internal HAL flow

```
Backend.CreateInstance()
  → Instance.EnumerateAdapters()
    → Adapter.Open()
      → Device + Queue
        → Device.Create*(desc)     // create resources
        → CommandEncoder.Begin*()  // record commands
        → Queue.Submit()           // execute
        → Device.Destroy*(res)     // release
```

All resources must be explicitly released. The `core/` layer provides leak detection.

## Pure Go Approach

All backends are implemented without CGO:

- **Function loading** — `cgo_import_dynamic` + `go-webgpu/goffi` for symbol resolution
- **Windows APIs** — `syscall.LazyDLL` for DX12/DXGI COM
- **Objective-C** — `objc_msgSend` via FFI for Metal
- **Build** — `CGO_ENABLED=0 go build` works everywhere

## Dependencies

```
naga (shader compiler) — WGSL → SPIR-V / MSL / GLSL
  ↑
wgpu (this library)
  ↑
gogpu (app framework) / gg (2D graphics)
```

External dependencies:
- `github.com/gogpu/naga` v0.16.4 — shader compiler (WGSL → SPIR-V / MSL / GLSL / HLSL), Pure Go
- `github.com/gogpu/gputypes` v0.4.0 — shared WebGPU type definitions
- `github.com/go-webgpu/goffi` v0.5.0 — Pure Go FFI for Vulkan/Metal symbol loading
