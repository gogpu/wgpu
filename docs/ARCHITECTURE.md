# Architecture

This document describes the architecture of `wgpu` — the unified Go WebGPU package with three independent implementations (ADR-038).

## Design Principle

**If implementation CAN be hidden — it MUST be hidden.**

The root package (`github.com/gogpu/wgpu`) exposes only exported types and thin delegation methods. All implementation logic lives in `internal/` sub-packages. pkg.go.dev shows one clean public API package. Contributors working within the module have full access to `internal/`. External users interact exclusively through the public API.

This is enforced by Go's `internal/` package mechanism and documented in ADR-070.

## Overview

Like Chrome (Dawn) and Firefox (wgpu) implementing the same W3C WebGPU spec, `wgpu` provides three backend paths selected by build tags:

```
┌─────────────────────────────────────────────────┐
│                   User Code                     │
│   import "github.com/gogpu/wgpu"                │
│   Same *Device, *Buffer, *Texture on all paths  │
└──────────┬──────────────────┬──────────────┬────┘
           │ (default)        │ -tags rust   │ js,wasm
           │ *_native.go      │ *_rust.go    │ *_browser.go
┌──────────▼──────────┐ ┌────▼──────────┐ ┌─▼─────────────┐
│       core/         │ │ go-webgpu/    │ │internal/browser│
│  Validation, state  │ │ webgpu v0.5+  │ │ syscall/js →   │
│  tracking, scopes   │ │ → wgpu-native │ │ navigator.gpu  │
└──────────┬──────────┘ │   v29 (Rust)  │ └────────────────┘
           │            └───────────────┘
┌──────────▼──────────────────────────────────────┐
│                  hal/                           │
│     Hardware Abstraction Layer (interfaces)     │
└──────┬────────┬────────┬────────┬────────┬──────┘
       │        │        │        │        │
┌──────▼──┐┌───▼────┐┌──▼───┐┌────▼───┐┌───▼──────┐
│ vulkan/ ││ metal/ ││ dx12/││ gles/  ││software/ │
│ Vulkan  ││ Metal  ││ DX12 ││OpenGLES││  CPU     │
│1.0+ API ││ macOS  ││ Win  ││ 3.0+   ││rasterizer│
└─────────┘└────────┘└──────┘└────────┘└──────────┘

Native Go path: core/ → hal/ → GPU drivers (default, zero dependencies)
Rust FFI path:  go-webgpu/webgpu → wgpu-native (battle-tested drivers)
Browser path:   syscall/js → Browser WebGPU API (WASM target)
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

- **Spec validation** — `core/validate.go` implements 45+ WebGPU spec rules (Phase A+B): textures (dimensions, limits, multisampling, formats, depth/stencil aspects), samplers (LOD, anisotropy), shaders (source presence), pipelines (stages, targets, format type guards), bind groups (entry matching, buffer usage/alignment/bounds, MinBindingSize), pipeline layouts (bind group count). Draw-time validation includes pipeline/bind group/vertex buffer state, index buffer format matching, indirect buffer bounds, blend constant tracking (VAL-005), and resource usage conflict detection (BufferTracker). Queue.Submit validates buffer/texture/bind group lifecycle.
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

Pure Go Vulkan implementation using goffi for dynamic function loading.

- `vk/` — Low-level Vulkan bindings (generated types, function signatures, loader)
- `memory/` — GPU memory allocator (buddy allocation, `maxMemoryAllocationSize` enforcement). Future: replace BuddyAllocator with [gogpu/galloc](https://github.com/gogpu/galloc) O(1) offset allocator.
- Command encoder: free list of pre-allocated VkCommandBuffers (batch 16), `vkResetCommandPool` for batch reset (Rust wgpu-hal parity)
- Swapchain fail-closed lifecycle: transactional reconfiguration, capability snapshot validation, broken-state tracking
- Surface-qualified adapter selection: `vkGetPhysicalDeviceSurfaceSupportKHR` across all queue families (ahead of Rust wgpu)
- Platform surface: VkWin32, VkXlib/VkWayland, VkMetal, and Android `ANativeWindow` (arm64/API 29+ preview; see [ANDROID.md](ANDROID.md))
- Multi-draw indirect: native `vkCmdDrawIndirect`/`vkCmdDrawIndexedIndirect` with `drawCount`, feature-gated fallback loop

### `hal/metal/` — Metal Backend

Pure Go Metal implementation via Objective-C runtime message sending.

- `objc.go` — Objective-C runtime (`objc_msgSend`, `NSAutoreleasePool` with OS thread pinning, selectors)
- `encoder.go` — Command encoder, render/compute pass encoders, deferred render encoder for ICB
- `icb_indexed.go` — Metal Indirect Command Buffers for large indexed multi-draws (1024-52428 commands). Original optimization beyond Rust wgpu.
- `texture_copy.go` — metalCopyPlan: array layer vs 3D depth decomposition, block-compressed format support
- `device.go` — Device, resource creation, fence management, `isAppleGPU` via `MTLGPUFamilyApple1`
- `queue.go` — Command submission, texture writes
- Uses scoped autorelease pools with `LockOSThread` (create + drain on same OS thread, Go 1.14+ preemption safe)

### `hal/dx12/` — DirectX 12 Backend

Pure Go DX12 implementation via COM interfaces.

- `d3d12/` — D3D12 COM interfaces, GUID definitions, DRED diagnostics, loader
- `dxgi/` — DXGI factory, adapter enumeration
- `device.go` — Device, resource creation, descriptor heaps (SRV/sampler), dual shader compilation (HLSL→FXC or DXIL direct)
- `state_tracker.go` — Submission-ordered resource state reconciliation: command-local tracking, preamble barriers at submit time, per-plane depth/stencil. Replaces recording-time `currentState`.
- `copy_plan.go` + `copy_commands.go` — 2D-array vs 3D volume copy decomposition, 512-byte placement alignment, block-compressed row counting
- `command.go` — Command encoder with resource barriers (via state tracker)
- `queue.go` — Command submission with fence-based GPU completion tracking, preamble pool (per-frame reuse, capped at maxFramesInFlight)
- `resource.go` — Buffers (upload/default heaps), textures with deferred destruction
- `shader_cache.go` — In-memory SHA-256 keyed LRU cache (works for both HLSL and DXIL paths)
- **Shader compilation:** dual path — HLSL→FXC (default, SM 5.1) or DXIL direct via naga (opt-in `GOGPU_DX12_DXIL=1`, SM 6.0+, zero external dependencies)
- **DRED diagnostics:** auto-breadcrumbs + page fault tracking on TDR (debug mode)
- Deferred descriptor destruction: heap slots freed after GPU completion (BUG-DX12-007)
- Texture pending refs: prevents premature Release while GPU copies in-flight (BUG-DX12-006)
- Buffer barriers: COPY_DEST → read-state transitions after PendingWrites (BUG-DX12-010)
- Windows-only (`//go:build windows`)

### `hal/gles/` — OpenGL ES Backend

Pure Go OpenGL ES 3.0+ / OpenGL 4.3+ implementation.

**Context Architecture (Rust wgpu parity):**
- GL context lives on a hidden 1×1 window (WGL/Windows) owned by Instance
- `AdapterContext` — `sync.Mutex`-protected wrapper shared by Adapter → Device → Queue
- `Lock()` → `wglMakeCurrent(hiddenDC)` for resource creation and command execution
- `LockForDC(userDC)` → `wglMakeCurrent(userDC)` for presentation (blit + SwapBuffers)
- `Unlock()` → `wglMakeCurrent(NULL)` — context unmade current between operations
- Surface is lightweight — stores only user HWND + reference to shared AdapterContext
- Adapter, Device, Queue survive user window destruction (context lives on hidden window)
- Follows Rust wgpu-hal/src/gles/wgl.rs `AdapterContext::lock()` / `lock_with_dc()` pattern

**Packages:**
- `gl/` — OpenGL function bindings (Windows syscall + Linux goffi)
- `egl/` — EGL context and display management (Linux)
- `wgl/` — WGL context + hidden window lifecycle (Windows)
- `shader.go` — WGSL → GLSL 4.30 via naga, with BindingMap for flat binding indices
- `sampler.go` — GL sampler objects (glGenSamplers/glBindSampler, GL 3.3+)
- `command.go` — SamplerBindMap: maps WGSL separate texture+sampler to GLSL combined sampler2D (from naga TextureMappings)

**Key patterns:**
- Texture completeness: `GL_TEXTURE_MAX_LEVEL = MipLevelCount-1` at creation (default 1000 makes non-mipmapped textures incomplete)
- Texture updates via `glTexSubImage2D` (not `glTexImage2D`) — matches Rust wgpu-hal pattern
- `GL_DYNAMIC_DRAW` for all writable buffers (Rust wgpu-hal parity — some vendors freeze STATIC_DRAW buffers)
- Scissor Y-flip: WebGPU top-left → OpenGL bottom-left origin conversion
- MSAA resolve via `glBlitFramebuffer`
- Texture unit validation: warns when binding exceeds GL_MAX_TEXTURE_IMAGE_UNITS

### `hal/software/` — Software Backend

CPU-based rasterizer with SPIR-V interpreter. Always compiled (no build tags required). Pure Go, zero system dependencies.

- `raster/` — Triangle rasterization, blending, depth/stencil, tiling, per-pixel fragment shader callback
- `shader/` — Full SPIR-V interpreter (~10K LOC): vertex, fragment, compute shaders. GLSL.std.450 math intrinsics (30+), texture sampling, control flow, atomics, workgroup shared memory. Shader debugger with breakpoints and JSON trace. **Not for production rendering** — interpreted execution is ~100× slower than JIT (SwiftShader/llvmpipe). Designed for shader debugging, CI/CD testing, and GPU-less fallback.
- `compute_test.go` — Naga WGSL→SPIR-V integration tests for compute shaders
- `blit_windows.go` — Windows presentation: CreateDIBSection + BitBlt (SDL3/Qt6 pattern)
- `blit_linux.go` — Linux X11 presentation: XPutImage via goffi (Skia pattern)
- `blit_darwin.go` — macOS presentation: CGImage + CALayer, or Metal nextDrawable + replaceRegion for CAMetalLayer. Contributor: @k-chimi

**Extensions (non-standard):**
- `Surface.PresentPixels()` — atomic CPU pixel write + present. Single-pass RGBA→BGRA swizzle + platform blit. `hal.PixelPresenter` / `hal.PixelWriter` optional interfaces.
- `Surface.ReadPixels()` — headless surface pixel readback for golden image testing. `hal.PixelReader` optional interface. Returns owned RGBA8 snapshot. Requires `HeadlessSurfaceTarget`.
- `HeadlessSurfaceTarget` — zero-sized safe target for display-free software rendering.

Use cases: **shader debugging** (step through every SPIR-V instruction), **CI/CD testing** (no GPU required), **headless rendering** (servers), **GPU-less fallback** (embedded systems). NOT for real-time production rendering — use GPU backends (Vulkan/DX12/Metal/GLES) for that. Verified: triangle + 4096-particle compute+render simulation. All 3 desktop platforms (Windows, Linux, macOS) have windowed presentation.

### `hal/noop/` — No-op Backend

Stub implementation for testing. All operations succeed without GPU interaction.

### `internal/browser/` — Browser WebGPU Backend

Browser WebGPU via `syscall/js` → `navigator.gpu`. Bypasses `core/` and `hal/` entirely — browser validates internally (same W3C spec as our public API). Matches Rust wgpu's `backend/webgpu.rs` top-level bypass architecture.

```
wgpu public API
  ├── [native]  core/ → hal/ → Vulkan/Metal/DX12/GLES/Software
  └── [browser] internal/browser/ → syscall/js → navigator.gpu
```

- Build tags: `//go:build js && wasm` on all browser files
- Root `*_browser.go` files are thin wrappers delegating to `internal/browser/`
- Pre-bound JS methods (Ebiten pattern): `method.Call("bind", obj)` at construction, avoiding `.Get()` on hot paths
- Promise→goroutine: `AwaitPromise()` blocks via `Promise.then/catch` + channel
- Data transfer: `js.CopyBytesToGo`/`js.CopyBytesToJS` for GPU↔CPU
- Shaders: WGSL string passthrough to browser `createShaderModule()` — no naga on browser path
- Surface: HTML Canvas + `GPUCanvasContext`, present is no-op (browser auto-presents)
- ~6500 LOC total (4000 internal/browser + 2500 root wrappers), zero external dependencies

Key files: `promise.go` (async→sync), `convert_enums.go` (97 TextureFormats, 31 VertexFormats + all WebGPU enums), `convert_resources.go` (JS descriptor builders), `surface.go` (Canvas + GPUCanvasContext).

### `internal/raytracing/` — Ray Tracing Resources & Validation (ADR-062)

Ray tracing build orchestration, compaction state machine, and validation — isolated in `internal/` per ADR-069 (struct ownership + callback interface pattern). Core holds pointers, calls directly.

- `types.go` — CompactionState (Idle→Waiting→Ready→Compacted), BlasAction enums
- `blas.go`, `tlas.go` — BLAS/TLAS resource structs with helpers (IsBuilt, AllowsCompaction)
- `build.go` — BuildContext: scratch buffer alignment, BLAS→TLAS dependency tracking (built_index)
- `compaction.go` — per-state handler functions (nestif ≤4 compliance)
- `validate.go` — 9 validation checks: feature gate, geometry/instance counts, build ordering, alignment, compact state
- `errors.go` — typed ValidationError with operation constants
- `context.go` — DeviceContext callback interface (core.Device implements without import cycle)

HAL RT interface in `hal/raytracing.go`: AccelerationStructure, 12 descriptor types, TlasInstance. All 4 GPU backends implement real API calls (Vulkan VK_KHR, DX12 DXR, Metal MTL, Software CPU BVH). GLES/Noop return ErrUnsupported.

~1,300 LOC internal, 73 tests, 96.8% coverage. Example: `examples/raytracing-headless/`.

## DownlevelCapabilities (ADR-071)

### Why This Exists

The W3C WebGPU specification assumes all adapters meet a baseline: compute shaders, indirect draw, base vertex, independent blend — all mandatory. `requestAdapter()` simply doesn't return adapters that can't meet this baseline. In a browser, the user sees "WebGPU not supported."

**We can't do that.** As a native Go library, we run on hardware where the only available backend may be GLES 3.0 (no compute), an old Metal GPU (no fragment writable storage), or our own CPU software rasterizer. Refusing to run is not an option — we must **degrade gracefully**.

DownlevelCapabilities tracks exactly what each backend can and cannot do, enabling consumers like gg to make informed decisions: use GPU compute path when available, fall back to CPU rasterizer when not. This is the Skia Graphite pattern (`caps->computeSupport()` gates the Vello compute renderer) and the Flutter Impeller pattern (`SupportsCompute()` gates compute-dependent features).

**How the three WebGPU implementations handle non-conformant hardware:**

| Implementation | Approach |
|---------------|----------|
| **W3C Spec / Dawn (browsers)** | Non-conformant adapters excluded from `requestAdapter()`. No degradation — just "not supported" |
| **Rust wgpu** | `DownlevelCapabilities` — 27 granular flags. Supports GLES/WebGL below spec baseline |
| **gogpu/wgpu** | Follows Rust — supports GLES 3.0 + Software. Graceful degradation via capability queries |

### Technical Details

**This is a Rust wgpu extension — not a W3C WebGPU spec concept** (the term "downlevel" does not appear in the 18.5K-line spec). Of 27 flags, 24 track capabilities REQUIRED by the spec for core adapters, 1 (AnisotropicFiltering) is correctly not required, and 2 (MSL21, SurfaceViewFormats) are backend-specific.

**Types:** Defined in `gputypes/downlevel.go` — 27 `DownlevelFlags` with explicit `1 << N` bit positions matching Rust wgpu-types (`limits.rs:1102-1246`). `DownlevelCapabilities` struct (Flags, Limits, ShaderModel). `IsWebGPUCompliant()` checks compliance.

**Data flow:**
```
HAL backend (per-adapter)
  → hal.ExposedAdapter.Capabilities.DownlevelCapabilities
  → core.Adapter.DownlevelCapabilities (extracted at enumeration)
  → core.Device.downlevel (copied at device creation)
  → wgpu.Adapter.DownlevelCapabilities() (public API)
  → gpucontext.DeviceProvider.DownlevelCapabilities() (ecosystem interface)
  → gg.CheckGPUComputeSupport(provider) (consumer)
```

**Per-backend implementation:**

| Backend | Approach | Flags |
|---------|----------|-------|
| Vulkan | 18 unconditional + 8 conditional from `VkPhysicalDeviceFeatures` | Rust adapter.rs:684-719 parity |
| Metal | `DefaultDownlevelCapabilities()` | All conditionals pass on macOS 15+ |
| DX12 | `DefaultDownlevelCapabilities()` | FL 11.0+ guarantees all |
| GLES | Dynamic `queryDownlevelFlags()` (~20 checks) | Rust adapter.rs:387-452 parity |
| Software | 13 explicit flags | Each verified against implementation code |
| Noop | `DefaultDownlevelCapabilities()` | Rust noop/mod.rs parity |
| Browser/Rust | `DefaultDownlevelCapabilities()` | WebGPU/Rust fully compliant |

**Validation:** `core.Device.RequireDownlevelFlags()` rejects operations when flags are missing. Called in `CreateComputePipeline` (Rust `resource.rs:4367` parity). GLES 3.0 gets clean error instead of HAL crash.

**Consumer gate:** gg checks `CheckGPUComputeSupport(provider)` BEFORE creating compute pipelines in both init paths (SetDeviceProvider + standalone). Matches Skia Graphite `computeSupport()` gate (`AtlasProvider.cpp:42`).

## Pipeline Disk Cache (#331)

Persists driver-compiled GPU ISA across process launches for faster cold starts. Extends the existing in-memory shader cache (`hal/dx12/shader_cache.go`) to disk.

**Vulkan:** One device-wide `VkPipelineCache` created at device init, passed to all `vkCreateGraphicsPipelines`/`vkCreateComputePipelines`, saved via `vkGetPipelineCacheData` on device destroy. Matches Rust wgpu and Dawn monolithic cache pattern.

**DX12:** `GetCachedBlob` after each PSO creation → per-PSO `.pso` blobs on disk. On next launch: `D3D12_CACHED_PIPELINE_STATE` with saved blob. Stale blob → `E_INVALIDARG` → automatic retry without cache. **Ahead of Rust wgpu** (which has an empty DX12 pipeline cache stub).

**Shared infrastructure:** `internal/pipelinecache/` — atomic blob I/O (`write-tmp + rename`), adapter-scoped cache paths (`os.UserCacheDir()/gogpu/{backend}/{adapterKey}/`). Follows ADR-069 internal package pattern.

**Non-fatal:** Pipeline cache init failure logs warning and continues with uncached pipelines. Cache is a performance optimization, not a correctness requirement.

**Race hardening:** `RegisterHALBackends()` wrapped in `sync.Once`, `instanceEnumerateMu` serializes concurrent adapter probing.

## Typed Surface Targets (Rust v29 Parity)

Surface creation uses typed targets instead of raw `uintptr` handles:

```go
// Safe targets (recommended)
target := wgpu.SurfaceTargetFromWindowsHWND(hwnd)
surface, _ := instance.CreateSurfaceFromTarget(target)

// Unsafe targets (platform-specific)
target := wgpu.SurfaceTargetUnsafeAndroidNDK(nativeWindow)
surface, _ := instance.CreateSurfaceUnsafe(target)

// Legacy compatibility (preserved)
surface, _ := instance.CreateSurface(displayHandle, windowHandle)
```

`hal.SurfaceTarget.RequireKind()` discriminator ensures every backend rejects foreign handle kinds before pointer access. Platform constructors: Win32 HWND, Xlib Window, Wayland wl_surface, Android ANativeWindow, Metal CAMetalLayer, Web Canvas.

See [SURFACE-TARGETS.md](SURFACE-TARGETS.md) for the exhaustive API mapping.

## Backend Registration

Backends register via `init()` functions. Import `hal/allbackends` to auto-register platform-appropriate backends:

```go
import _ "github.com/gogpu/wgpu/hal/allbackends"
```

Platform selection (`hal/allbackends/`):

| Platform | Backends |
|----------|----------|
| Windows | Vulkan, DX12, GLES, Software |
| macOS | Metal, Vulkan, Software |
| Linux | Vulkan, GLES, Software |
| Android/arm64 | Vulkan only (preview) |

The no-op backend is imported explicitly for tests; `hal/allbackends` does not
register it automatically.

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

**StagingBelt** (`staging_belt.go`): ring-buffer of reusable 256KB staging chunks with bump-pointer sub-allocation. Matches Rust wgpu `util::StagingBelt` (belt.rs). Zero heap allocations in steady state — chunks are pre-allocated and recycled after GPU completion. Oversized writes (> chunkSize) are automatically chunked into multiple staging buffers capped at 64MB (Rust wgpu parity: `1 << 26`), each followed by a `CopyBufferToBuffer` command. This prevents SIGSEGV when writes exceed `maxMemoryAllocationSize`.

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

Resources should be explicitly Released for deterministic cleanup. Buffer destruction is **refcount-driven** (Rust `Arc<Buffer>` Drop parity): `ResourceRef.Clone()` during encoding (SetBindGroup, SetVertexBuffer, CopyBufferToBuffer), `Drop()` on GPU completion via `DestroyQueue.Triage`. The `onZero` callback fires `core.Buffer.Destroy()` only when the last reference drops — safe even if `Release()` is called before `Submit()`. `runtime.AddCleanup` (Go 1.24+) provides a GC-based safety net: unreleased resources trigger `Ref.Drop()` when collected, with `slog.Warn` for leak detection (ADR-018).

**Tracking architecture:** Pass encoders (ComputePassEncoder, RenderPassEncoder) write tracked ResourceRefs directly to the parent CommandEncoder's `trackedRefs` slice — no per-pass intermediate storage. At `Finish()`, the slice moves to CommandBuffer; at `Submit()`, to `DestroyQueue.TrackSubmission`. This matches Rust wgpu where trackers live in the command encoder throughout recording (zero intermediate copies, zero abandoned backing arrays).

## Pure Go Approach

All backends are implemented without CGO:

- **Function loading** — `cgo_import_dynamic` + `go-webgpu/goffi` for symbol resolution
- **Windows APIs** — `syscall.LazyDLL` for DX12/DXGI COM
- **Objective-C** — `objc_msgSend` via FFI for Metal
- **Build** — `CGO_ENABLED=0 go build` works everywhere

## Dependencies

```
naga (shader compiler) — WGSL → SPIR-V / MSL / GLSL / HLSL / DXIL
  ↑
wgpu (this library)
  ↑
gogpu (app framework) / gg (2D graphics)
```

External dependencies:
- `github.com/gogpu/naga` v0.19.0 — shader compiler (WGSL → SPIR-V / MSL / GLSL / HLSL / DXIL), Pure Go
- `github.com/gogpu/gputypes` v0.5.2 — shared WebGPU type definitions
- `github.com/gogpu/gpucontext` v0.28.0 — shared interfaces (DeviceProvider, PlatformProvider)
- `github.com/go-webgpu/goffi` v0.6.3 — Pure Go FFI for Vulkan/Metal/DX12 symbol loading (Android Bionic support)
- `github.com/go-webgpu/webgpu` v0.5.5 — Rust FFI backend (wgpu-native v29, Android surface, timestamp period)
- `github.com/gogpu/galloc` v0.1.0 — O(1) offset allocator (planned integration for memory sub-allocation)
- `golang.org/x/sys` v0.47.0 — platform syscall definitions
