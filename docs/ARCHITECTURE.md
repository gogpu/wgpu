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

Key types: `Instance`, `Adapter`, `Device`, `Queue`, `Buffer`, `Texture`, `TextureView`, `Sampler`, `ShaderModule`, `BindGroupLayout`, `PipelineLayout`, `BindGroup`, `RenderPipeline`, `ComputePipeline`, `CommandEncoder`, `RenderPassEncoder`, `ComputePassEncoder`, `Surface`.

### `core/` — Validation & State Tracking

Internal validation layer between the public API and HAL.

- **Validation** — Validates descriptors, usage flags, and state before forwarding to HAL
- **Error scopes** — WebGPU error handling model (`PushErrorScope` / `PopErrorScope`)
- **Resource tracking** — Leak detection in debug builds
- **Structured logging** — `log/slog` integration, silent by default

Key types: `Instance`, `Adapter`, `Device`, `Queue`, `Buffer`, `Texture`, `RenderPipeline`, `ComputePipeline`, `CommandEncoder`.

### `hal/` — Hardware Abstraction Layer

Backend-agnostic interfaces that each graphics API implements. The HAL prioritizes portability over safety — validation is handled by the `core/` layer above.

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
- Windows-only (`//go:build windows`)

### `hal/gles/` — OpenGL ES Backend

Pure Go OpenGL ES 3.0+ implementation.

- `gl/` — OpenGL function bindings
- `egl/` — EGL context and display management
- `wgl/` — WGL context for Windows
- Shader compilation: WGSL → GLSL via naga

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
device.Queue().Submit(cmdBuf)
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

External dependency: `github.com/gogpu/naga` (shader compiler, also Pure Go).
