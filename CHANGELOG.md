# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed

- **Vulkan: zero-extent swapchain on window minimize** (VK-VAL-001) — `createSwapchain()` used
  `capabilities.CurrentExtent` as primary extent source. NVIDIA drivers report `CurrentExtent = {0, 0}`
  when minimized, passing zero directly to `vkCreateSwapchainKHR` and violating
  `VUID-VkSwapchainCreateInfoKHR-imageExtent-01274`. Now uses `config` dimensions as primary source
  (matching Rust wgpu-hal `native.rs:189-197` pattern), with `CurrentExtent` only for clamping to
  the valid range. Returns `hal.ErrZeroArea` when clamped extent is zero.
  ([gogpu#98](https://github.com/gogpu/gogpu/issues/98))

- **Vulkan: unconditional viewport/scissor in BeginRenderPass** — viewport and scissor dynamic state
  was conditionally set only when render dimensions > 0. When zero-extent frames slipped through,
  the pipeline's dynamic state was never initialized, causing `VUID-vkCmdDraw-None-07831` and
  `VUID-vkCmdDraw-None-07832` validation errors. Now always sets viewport/scissor using
  `max(dim, 1)` as safety net.
  ([gogpu#98](https://github.com/gogpu/gogpu/issues/98))

## [0.16.10] - 2026-02-22

### Fixed

- **Vulkan: pre-acquire semaphore wait** (VK-IMPL-004) — Acquire semaphores are rotated across
  frames, but nothing guaranteed the GPU had consumed the previous wait before reuse, violating
  `VUID-vkAcquireNextImageKHR-semaphore-01779` on some drivers. Now tracks the submission fence
  value per acquire semaphore and waits before reuse, matching Rust wgpu's
  `previously_used_submission_index` pattern. Also adds binary fence pool tracking to
  `SubmitForPresent` which previously submitted with no fence at all.
  ([gogpu#98](https://github.com/gogpu/gogpu/issues/98))

### Dependencies

- naga v0.14.1 → v0.14.2 (GLSL GL_ARB_separate_shader_objects fix, golden snapshot tests)

## [0.16.9] - 2026-02-21

### Dependencies

- naga v0.14.0 → v0.14.1 (HLSL row_major matrices for DX12, GLSL namedExpressions leak fix for GLES)

## [0.16.8] - 2026-02-21

### Fixed

- **Metal: blank window on macOS** ([gogpu#89](https://github.com/gogpu/gogpu/issues/89)) —
  `Queue.Present()` only released the drawable reference without calling `presentDrawable:`.
  Now creates a dedicated command buffer for presentation matching the Rust wgpu pattern:
  `commandBuffer` → `presentDrawable:` → `commit`. Fixes blank rendering on macOS Tahoe M2 Max.

## [0.16.7] - 2026-02-21

### Dependencies

- naga v0.13.1 → v0.14.0 (Essential 15/15 reference shaders, 48 type aliases, 25 math ops, 20+ SPIR-V fixes)

## [0.16.6] - 2026-02-18

### Added

- **Metal backend debug logging** — 23 new `hal.Logger()` calls across the critical
  rendering path: `AcquireTexture`, `Submit`, `Present`, `BeginEncoding`/`EndEncoding`,
  `CreateCommandEncoder`, `Wait`/`WaitIdle`, `Destroy`, and all three ObjC block callback
  invocations (shared event, completion handler, frame completion). Enables diagnosis of
  blank window issues on macOS (gogpu/gogpu#89) and validates goffi callback delivery
  (go-webgpu/goffi#16). Metal backend now has ~38 log points, matching Vulkan/DX12 coverage.

### Changed

- **goffi** v0.3.8 → v0.3.9

## [0.16.5] - 2026-02-18

### Fixed

- **Vulkan per-encoder command pools** (VK-POOL-001) — Each `CreateCommandEncoder` now gets
  its own dedicated `VkCommandPool` + `VkCommandBuffer` pair, matching Rust wgpu-hal architecture.
  Eliminates race condition between per-frame bulk pool reset (`vkResetCommandPool`) and individual
  command buffer freeing (`vkFreeCommandBuffers`) that caused `vkBeginCommandBuffer(): Couldn't find
  VkCommandBuffer Object` access violation crashes. Pools are recycled via a thread-safe free list
  with lazy reset on next acquire. No API changes — `hal.Device` interface unchanged.

## [0.16.4] - 2026-02-18

Vulkan timeline semaphore fences, binary fence pool, hot-path allocation optimization,
and enterprise benchmarks. Internal performance improvements — no API changes.

### Added

- **Enterprise hot-path benchmarks** — 44+ benchmarks with `ReportAllocs()` covering Vulkan
  Submit/Present/Encoding cycle, descriptor operations, memory allocator, noop backend overhead,
  and cross-backend HAL interface. Table-driven sub-benchmarks for different sizes and workloads.
- **Compute shader SDF integration test** — End-to-end GPU test: WGSL SDF shader → naga compile →
  Vulkan compute pipeline → dispatch → ReadBuffer → CPU reference verification (256 pixels, ±0.01).
- **Compute shader examples** — `examples/compute-sum/` (parallel pairwise reduction) and
  `examples/compute-copy/` (scaled buffer copy) demonstrating the compute pipeline API.
- **Timestamp queries for compute passes** — `ComputePassTimestampWrites`, `CreateQuerySet`,
  `ResolveQuerySet` with full Vulkan implementation (`vkCmdWriteTimestamp`, `vkCmdCopyQueryPoolResults`).
  Other backends return `ErrTimestampsNotSupported`.
- **Software backend compute error** — `ErrComputeNotSupported` sentinel error with `errors.Is` support.
- **Compute shader documentation** — `docs/compute-shaders.md` (getting started guide) and
  `docs/compute-backends.md` (backend support matrix).

### Changed

- **Vulkan timeline semaphore fence** (VK-IMPL-001) — Single `VkSemaphore` with monotonic `uint64`
  counter replaces binary `VkFence` ring buffer on Vulkan 1.2+. Signal attached to real
  `vkQueueSubmit` (eliminates empty submit per frame). Replaces transfer fence state machine.
  Graceful fallback to binary fences on pre-1.2 drivers. Based on Rust wgpu-hal `Fence::TimelineSemaphore`.
- **Vulkan command buffer batch allocation** (VK-IMPL-002) — Batch-allocate 16 command buffers
  per `vkAllocateCommandBuffers` call (matches wgpu-hal `ALLOCATION_GRANULARITY`). Free/used list
  recycling per frame slot. Handles are valid after `vkResetCommandPool` (flag 0).
- **Vulkan binary fence pool** (VK-IMPL-003) — `fencePool` with per-submission tracking for
  Vulkan <1.2 where timeline semaphores are unavailable. Active/free lists with non-blocking
  `maintain()` polling, `signal()` fence acquisition, `wait()` with watermark fast-path.
  Replaces 2-slot binary fence ring buffer and separate transfer fence. Mirrors Rust wgpu-hal
  `FencePool` pattern. `deviceFence` now always created (never nil) — unified dual-path dispatch.
- **Vulkan hot-path allocation reduction** — `sync.Pool` for CommandEncoder, CommandBuffer,
  ComputePassEncoder, RenderPassEncoder. Stack-allocated `[3]vk.ClearValue` in BeginRenderPass.
  Removed CommandPool wrapper struct. Per-frame Submit uses pooled `[]vk.CommandBuffer` slices.
  Result: BeginEndEncoding 15→13 allocs, ComputePassBeginEnd 25→22 allocs, EncodeSubmitCycle 28→26 allocs.

### Fixed

- **Vulkan transfer fence race condition** — `Submit()` now waits for previous GPU work before
  resetting transfer fence, preventing "vkResetFences: pFences[0] is in use" validation error.
- **Vulkan swapchain image view leak** — `createSwapchain()` now calls `destroyResources()`
  (semaphores + image views) instead of `releaseSyncResources()` (semaphores only) when
  reconfiguring, preventing "VkImageView has not been destroyed" validation errors on shutdown.
- **Vulkan device destroy fence wait** — `Destroy()` waits for all in-flight frame slots
  before destroying fences, preventing fence-in-use errors during cleanup.

## [0.16.3] - 2026-02-16

### Added

- **`hal.Device.WaitIdle()` interface method** — Waits for all GPU work to complete before
  resource destruction. Implemented across all backends: Vulkan (`vkDeviceWaitIdle`),
  DX12 (`waitForGPU`), Metal (`waitUntilCompleted`), GLES (`glFinish`), noop/software (no-op).

### Fixed

- **Vulkan per-frame fence tracking** — Replaced single shared `frameFence` with per-slot
  `VkFence` objects (one per frame-in-flight). Each fence is only reset after `vkWaitForFences`
  confirms it is signaled. Fixes `vkResetFences(): pFences[0] is in use` validation error.
  Frame fence signaling moved from `Submit()` to `Present()` to avoid fence reuse across
  multiple submits per frame. Pattern based on Rust wgpu-hal FencePool design.

- **DX12 per-frame fence tracking** — Per-frame command allocator pool with timeline fence.
  `advanceFrame()` waits only for the specific old frame slot instead of all GPU work.
  Eliminates two `waitForGPU()` stalls per frame (in `BeginEncoding` and `Present`).

- **Metal per-frame fence tracking** — `maxFramesInFlight` semaphore (capacity 2) limits
  CPU-ahead-of-GPU buffering. `frameCompletionHandler` signals semaphore on GPU completion.
  Event-based `Wait()` replaces polling loop. Async `WriteTexture` via staging buffer and
  blit encoder.

- **GLES VSync on Windows** — Load `wglSwapIntervalEXT` via `wglGetProcAddress` during
  `Surface.Configure()`. Maps `PresentMode` to swap interval: Fifo=1 (VSync on),
  Immediate=0 (VSync off). Fixes 100% GPU usage on the GLES Windows backend.

## [0.16.2] - 2026-02-16

### Fixed

- **Metal autorelease pool LIFO violation** — Replaced stored autorelease pools with
  scoped pools that drain immediately within the same function. Previously, pools were
  stored in `CommandBuffer` structs and drained asynchronously via `FencePool`, causing
  LIFO violations when frame N+1 overlapped with frame N on the ObjC pool stack.
  macOS Tahoe (26.2) upgraded this from a warning to fatal SIGABRT. Fix matches the
  Rust wgpu-hal Metal backend pattern. Fixes gogpu/gogpu#83.

## [0.16.1] - 2026-02-15

### Fixed

- **Vulkan framebuffer cache invalidation** — `DestroyTextureView` now invalidates
  cached framebuffers before calling `vkDestroyImageView`, ensuring framebuffers that
  reference the view are destroyed first. Fixes Vulkan validation error:
  `vkDestroyImageView`/`vkDestroyFramebuffer` in use by `VkCommandBuffer`.

## [0.16.0] - 2026-02-15

Major release: full GLES rendering pipeline, structured logging across all backends,
MSAA support, and cross-backend stability fixes.

### Added

#### Structured Logging
- **`log/slog` integration** — All HAL backends now emit structured diagnostic logs
  via Go's standard `log/slog` package. Silent by default; enable with
  `slog.SetLogLoggerLevel(slog.LevelDebug)` or a custom handler. Zero overhead
  when logging is disabled.

#### OpenGL ES Backend (Full Rendering Pipeline)
- **WGSL-to-GLSL shader compilation** — End-to-end shader pipeline via naga:
  WGSL source is compiled to GLSL, then loaded via `glShaderSource`/`glCompileShader`.
  Includes VAO creation, FBO setup, and triangle rendering.
- **Offscreen FBO and MSAA textures** — Framebuffer objects for off-screen rendering,
  multi-sample texture support, and `CopyTextureToBuffer` readback path.
- **Vertex attributes, stencil state, color mask** — Full vertex attribute layout
  configuration, stencil test state, per-channel color write masks, and BGRA readback
  format conversion.
- **VAO, viewport, blend state, bind group commands** — Vertex Array Objects,
  viewport/scissor state, blend equation/factor configuration, and bind group
  resource binding.

#### Metal Backend
- **SetBindGroup** — Bind group resource binding for render and compute encoders.
- **WriteTexture** — GPU texture upload via staging buffer and blit encoder.
- **Fence synchronization** — CPU-GPU synchronization for command completion.

#### DX12 Backend
- **CreateBindGroup** — Bind group creation with SRV/CBV/sampler descriptor
  mapping to root parameter slots.
- **InfoQueue debug messages** — `ID3D12InfoQueue` captures validation
  errors/warnings when debug layer is enabled. `DrainDebugMessages()` reads
  and logs all pending messages after Submit and Present.

#### Vulkan Backend
- **MSAA render pass support** — Multi-sample render pass with automatic resolve
  attachment configuration. Includes depth/stencil usage flag fixes for MSAA targets.

### Fixed

#### DX12 Backend
- **GPU hang causing DPC_WATCHDOG_VIOLATION BSOD** — Resolved a device hang that
  triggered a Windows kernel watchdog timeout on some hardware configurations.
- **Texture resource state tracking** — Correct resource barriers via per-texture
  state tracking. Fixes rendering corruption from missing or incorrect
  COMMON/COPY_DEST/SHADER_RESOURCE transitions. Also fixes a COM reference leak.
- **MSAA resolve, view dimensions, descriptor recycling** — MSAA resolve copies
  now target the correct subresource. Texture view dimensions match the underlying
  resource. Descriptor recycling frees slots from the correct staging heaps.
- **Readback pitch alignment and barrier states** — Buffer readback row pitch is
  now aligned to D3D12_TEXTURE_DATA_PITCH_ALIGNMENT (256 bytes). Resource barriers
  use correct before/after states for copy operations.
- **Staging descriptor heaps** — SRV and sampler descriptors are now created in
  non-shader-visible staging heaps, then copied to shader-visible heaps via
  `CopyDescriptorsSimple`. Follows the DX12 specification requirement that
  `CopyDescriptorsSimple` source must be non-shader-visible. Prevents subtle
  rendering corruption on some hardware.
- **Descriptor recycling** — `TextureView.Destroy()` and `Sampler.Destroy()` now
  free descriptors from the correct staging heaps, enabling proper slot reuse.

#### Vulkan Backend
- **Descriptor pool allocation** — Always include all descriptor types (uniform buffer,
  storage buffer, sampled image, sampler, storage image) in pool creation. Fixes
  `VK_ERROR_OUT_OF_POOL_MEMORY` when bind groups reference previously unused types.
- **vkCmdSetBlendConstants FFI signature** — Corrected goffi calling convention to
  pass blend constants by pointer, matching the Vulkan specification.
- **Dynamic pipeline states** — All 4 dynamic states (viewport, scissor, stencil
  reference, blend constants) are now declared on every render pipeline. Prevents
  validation errors on drivers that require complete dynamic state declarations.

#### Metal Backend
- **Command buffer creation deferred to BeginEncoding** — `CreateCommandEncoder`
  eagerly created a Metal command buffer, conflicting with `BeginEncoding`'s guard
  (`cmdBuffer != 0`). Every `BeginEncoding` call returned "already recording" error,
  and the pre-allocated command buffer + autorelease pool were never released.
  At 60fps this leaked ~30GB in minutes. Fix: defer command buffer creation to
  `BeginEncoding`, matching the two-step pattern used by Vulkan and DX12 backends.
  (Fixes [#55])

#### GLES Backend
- **Surface resolve** — Correct resolve blit from MSAA renderbuffer to single-sample
  surface texture for presentation.

### Changed

- **Metal queue** — Eliminated `go vet` unsafe.Pointer warnings by using typed
  wrapper functions for Objective-C message sends.
- **DX12 descriptor heap management** — Free list recycling for descriptor slots,
  reducing allocation overhead for long-running applications.
- **naga v0.12.0 → v0.13.0** — GLSL backend improvements, HLSL/SPIR-V fixes

## [0.15.1] - 2026-02-13

Critical fixes across DX12, Metal, and Vulkan backends.

### Fixed

- **DX12 WriteBuffer** was a no-op stub, causing blank renders with uniform data
  - Staging buffer + `CopyBufferRegion` for DEFAULT heap (GPU-only) buffers
  - Direct CPU mapping for UPLOAD heap buffers (zero-copy path)
  - D3D12 auto-promotion from COMMON state for buffer copies
- **DX12 WriteTexture** was a no-op stub, textures never uploaded to GPU
  - Staging buffer + `CopyTextureRegion` with 256-byte row pitch alignment
  - Resource barriers: COMMON → COPY_DEST → SHADER_RESOURCE
- **DX12 shader compilation** produced empty DXBC bytecode
  - Added `d3dcompile` package — Pure Go bindings to d3dcompiler_47.dll
  - Wired `compileWGSLModule`: WGSL → naga HLSL → D3DCompile → DXBC
- **Metal memory leak** — 30GB+ memory usage on macOS (Issue #55)
  - `FreeCommandBuffer` was a no-op — command buffers never released after submit
  - NSString labels leaked in `BeginEncoding`, `BeginComputePass`, `CreateBuffer`, `CreateTexture`

### Added

- **Vulkan debug messenger** — validation errors now logged via `log.Printf` (Issue #53)
  - `VK_EXT_debug_utils` messenger created when `InstanceFlagsDebug` is set
  - Captures ERROR and WARNING severity from validation layers
  - Cross-platform callback via `goffi/ffi.NewCallback`
  - Zero overhead when debug mode is off

## [0.15.0] - 2026-02-10

HAL Queue ReadBuffer for GPU→CPU data transfer, enabling compute shader result readback.

### Added

#### HAL Interface
- **`ReadBuffer`** on `Queue` interface — GPU→CPU buffer readback for storage/staging buffers
  - Maps buffer memory, copies data to Go byte slice, unmaps
  - Enables compute shader pipelines (e.g., SDF rendering) to read results back to CPU
  - Implemented in Vulkan backend via `vkMapMemory`/`vkUnmapMemory`

### Changed

- **naga** dependency updated v0.11.1 → v0.12.0 — adds `OpFunctionCall`, compute shader codegen fixes
- **golang.org/x/sys** updated v0.39.0 → v0.41.0

## [0.14.0] - 2026-02-09

Debug toolkit for GPU resource management and error handling.

### Added

#### Debug & Diagnostics (`core/`)
- **GPU Resource Leak Detection** — Track unreleased GPU resources at runtime
  - `SetDebugMode(true)` enables tracking with zero overhead when disabled (~1ns atomic load)
  - `ReportLeaks()` returns `LeakReport` with per-type counts (Buffer, Texture, Device, etc.)
  - `ResetLeakTracker()` for test cleanup
  - Integrated into Device, Buffer, Instance, CommandEncoder lifecycle
- **W3C WebGPU Error Scopes** — Programmatic GPU error capture per the WebGPU spec
  - `ErrorScopeManager` with LIFO stack-based scopes
  - `ErrorFilter`: Validation, OutOfMemory, Internal
  - `GPUError` type implementing Go `error` interface
  - Device integration: `device.PushErrorScope()`, `device.PopErrorScope()`
  - Lazy initialization, thread-safe via internal mutex
- **Thread Safety Tests** — Concurrent access validation
  - Concurrent leak tracking (track/untrack from 50+ goroutines)
  - Concurrent error scope operations (push/pop/report)
  - Concurrent instance creation and adapter requests

### Changed

- **naga** dependency updated v0.11.0 → v0.11.1 — fixes SPIR-V OpLogicalAnd, comparison/shift opcodes, variable initializers, runtime-sized arrays

## [0.13.2] - 2026-02-07

### Changed

- **naga** dependency updated v0.10.0 → v0.11.0 — fixes SPIR-V `if/else` GPU hang, adds 55 new WGSL built-in functions

## [0.13.1] - 2026-02-06

### Fixed

- **Render pass InitialLayout for LoadOpLoad** — Set correct `InitialLayout` when `LoadOp` is `Load` instead of unconditional `ImageLayoutUndefined`. Previously, Vulkan was allowed to discard image contents between render passes, causing ClearColor output to be lost (black background instead of the expected color). Affects both color and depth/stencil attachments.

## [0.13.0] - 2026-02-01

Major HAL interface additions: format capabilities, array textures, and render bundles.

### Added

#### Format & Surface Capabilities
- **GetTextureFormatCapabilities** — Query actual Vulkan format capabilities
  - Returns TextureFormatCapabilityFlags based on `vkGetPhysicalDeviceFormatProperties`
  - No more hardcoded flags — real hardware support detection
- **GetSurfaceCapabilities** — Query surface capabilities from Vulkan
  - Uses `vkGetPhysicalDeviceSurfaceFormatsKHR` and `vkGetPhysicalDeviceSurfacePresentModesKHR`
  - Returns real supported formats, present modes, and alpha modes

#### Array Textures & Cubemaps
- **Array texture support** — Proper VkImageViewType selection
  - `VK_IMAGE_VIEW_TYPE_2D_ARRAY` for 2D array textures
  - `VK_IMAGE_VIEW_TYPE_CUBE` for cubemaps (6 layers)
  - `VK_IMAGE_VIEW_TYPE_CUBE_ARRAY` for cubemap arrays
- **ArrayLayers tracking** — Separate from depth dimension in Texture struct

#### Render Bundles
- **RenderBundleEncoder interface** — Pre-record render commands for reuse
  - SetPipeline, SetBindGroup, SetVertexBuffer, SetIndexBuffer
  - Draw, DrawIndexed, Finish
- **RenderBundle interface** — Execute pre-recorded commands
- **Vulkan implementation** — Secondary command buffers with `VK_COMMAND_BUFFER_USAGE_RENDER_PASS_CONTINUE_BIT`
- **ExecuteBundle** — Execute render bundles via `vkCmdExecuteCommands`

#### HAL Interface Extensions
- **ResetFence** — Reset fence to unsignaled state
- **GetFenceStatus** — Non-blocking fence status check
- **FreeCommandBuffer** — Explicit command buffer cleanup
- **CreateRenderBundleEncoder** / **DestroyRenderBundle** — Bundle lifecycle

### Changed
- All HAL backends updated with stub implementations for new interface methods

## [0.12.0] - 2026-01-30

### Added

- **NativeHandle interface** (`hal/`) — Access raw GPU handles for interop
  - `NativeTextureHandle()` returns platform-specific texture handle
  - Enables integration with external graphics libraries

### Fixed

- **Vulkan texture rendering** — Critical BufferRowLength fix
  - `BufferRowLength` now correctly specified in **texels**, not bytes
  - Fixes aspect ratio distortion (squashed circles → proper circles)
  - Root cause: Vulkan `VkBufferImageCopy` expects texel count, not byte count

- **WriteBuffer support** — Buffer memory mapping implementation
  - Proper staging buffer creation and memory mapping
  - Fixes texture upload pipeline

### Changed

- **Vulkan pipeline creation** — Code cleanup and refactoring
- **Update naga v0.8.4 → v0.9.0** — Sampler types, swizzle, SPIR-V fixes

## [0.11.2] - 2026-01-29

### Changed

- **Update gputypes to v0.2.0** for webgpu.h spec-compliant enum values
  - All enum values now match official WebGPU C header specification
  - Binary compatibility with wgpu-native and other WebGPU implementations

### Fixed

- **CompositeAlphaMode naming** — Fixed `PreMultiplied` → `Premultiplied` in all HAL adapters
  - Matches webgpu.h spec naming convention
  - Affected: Vulkan, DX12, GLES, Metal, Noop, Software adapters

## [0.11.1] - 2026-01-29

### Breaking Changes

- **Removed `types/` package** — Use `github.com/gogpu/gputypes` instead
  - All WebGPU types now come from shared `gputypes` package
  - Import `github.com/gogpu/gputypes` for TextureFormat, BufferUsage, etc.
  - 1,745 lines removed, unified ecosystem types

### Changed

- All packages now import `gputypes` for WebGPU type definitions
- **HAL types are now gputypes aliases** — No more type converters needed!
  - `hal.PresentMode` = `gputypes.PresentMode`
  - `hal.CompositeAlphaMode` = `gputypes.CompositeAlphaMode`
- 97 files updated for consistent type usage

### Migration

```go
// Before (wgpu v0.10.x)
import "github.com/gogpu/wgpu/types"
types.TextureFormatRGBA8Unorm

// After (wgpu v0.11.1)
import "github.com/gogpu/gputypes"
gputypes.TextureFormatRGBA8Unorm
```

## [0.10.3] - 2026-01-28

Enterprise-level multi-thread architecture for window responsiveness.

### Added

#### Internal
- **Thread Package** (`internal/thread/`) — Cross-platform thread abstraction
  - `Thread` — Dedicated OS thread with `runtime.LockOSThread()` for GPU operations
  - `RenderLoop` — Manages UI/render thread separation with deferred resize
  - `Call()`, `CallVoid()`, `CallAsync()` — Sync/async thread communication
  - `RequestResize()` / `ConsumePendingResize()` — Thread-safe resize coordination
  - Comprehensive tests (`thread_test.go`)

#### Vulkan Triangle Demo
- **Multi-Thread Architecture** — Ebiten-style separation for responsive windows
  - Main thread: Win32 message pump only (`runtime.LockOSThread()` in `init()`)
  - Render thread: All GPU operations including `vkDeviceWaitIdle`
  - Deferred swapchain resize: size captured in WM_SIZE, applied on render thread
  - No more "Not Responding" during resize/drag operations

#### Windows Platform
- **WM_SETCURSOR Handling** — Proper cursor restoration after resize
  - Fixes resize cursor staying 5-10 seconds after resize ends
  - Arrow cursor explicitly set when mouse enters client area

### Changed

#### HAL/Vulkan
- Removed unused fence wrapper functions from `swapchain.go`
  - `vkCreateFenceSwapchain`, `vkDestroyFenceSwapchain`
  - `vkWaitForFencesSwapchain`, `vkResetFencesSwapchain`
  - `vkGetFenceStatusSwapchain`

### Architecture

The multi-thread pattern follows Ebiten/Gio best practices:

```
Main Thread (OS Thread 0)     Render Thread (Dedicated)
├─ runtime.LockOSThread()     ├─ runtime.LockOSThread()
├─ Win32 Message Pump         ├─ Vulkan Device Operations
├─ WM_SIZE → RequestResize()  ├─ ConsumePendingResize()
└─ PollEvents()               ├─ vkDeviceWaitIdle (non-blocking UI!)
                              └─ Acquire → Render → Present
```

This architecture ensures:
- Window remains responsive during GPU operations
- Swapchain recreation doesn't freeze UI
- Proper handling of modal resize loops (WM_ENTERSIZEMOVE/WM_EXITSIZEMOVE)

## [0.10.2] - 2026-01-24

### Changed

- **goffi v0.3.8** — Fixed CGO build tag consistency ([#43](https://github.com/gogpu/wgpu/issues/43))
  - Clear error message when building with CGO enabled: `undefined: GOFFI_REQUIRES_CGO_ENABLED_0`
  - Consistent `!cgo` build tags across all FFI files
  - See [goffi v0.3.8 release notes](https://github.com/go-webgpu/goffi/releases/tag/v0.3.8)

## [0.10.1] - 2026-01-16

Window responsiveness fix for Vulkan swapchain.

### Added

#### HAL
- **ErrNotReady Error** — New error for non-blocking acquire signaling
  - Returned when swapchain image is not ready yet
  - Signals caller to skip frame without error

### Changed

#### HAL/Vulkan
- **Non-blocking swapchain acquire** — Improved window responsiveness
  - Use 16ms timeout instead of infinite wait in `acquireNextImage()`
  - Return `ErrNotReady` on timeout instead of blocking forever
  - Don't advance semaphore rotation on timeout (matches wgpu-hal pattern)
  - Based on wgpu-hal `vulkan/swapchain/native.rs` implementation

### Fixed
- Window lag during resize/drag operations on Windows
- "Not responding" window state during GPU-bound rendering

## [0.10.0] - 2026-01-15

New HAL backend integration layer for unified multi-backend support.

### Added

#### Core
- **Backend Interface** — New abstraction for HAL backend management
  - `Backend` interface with `Name()`, `CreateInstance()`, `SupportsWindow()` methods
  - `Resource` interface for GPU resource lifecycle management
  - Platform-independent backend selection

- **HAL Backend Integration** — Seamless backend auto-registration
  - `hal/allbackends` package for platform-specific registration
  - Vulkan backend auto-registered on Windows/Linux
  - Metal backend auto-registered on macOS
  - Import `_ "github.com/gogpu/wgpu/hal/allbackends"` to enable all available backends

- **Enhanced Instance** — HAL backend support in core.Instance
  - `Instance.Backend()` returns active backend
  - `Instance.AvailableBackends()` lists registered backends
  - Automatic backend selection based on platform

#### HAL
- **Backend Init Functions** — Auto-registration via `init()`
  - `hal/vulkan/init.go` — Registers Vulkan backend
  - `hal/metal/init.go` — Registers Metal backend

### Changed
- Instance creation now uses HAL backend abstraction internally

## [0.9.3] - 2026-01-10

Critical Intel Vulkan fixes: VkRenderPass support, wgpu-style swapchain synchronization.

### Added

#### HAL
- **ErrDriverBug Error** — New error type for driver specification violations
  - Returned when GPU driver violates API spec (e.g., returns success but invalid handle)
  - Provides actionable guidance: update driver, try different backend, or use software rendering

#### Vulkan Backend
- **VkRenderPass Support** — Classic render pass implementation for Intel compatibility
  - New `renderpass.go` with VkRenderPass and VkFramebuffer management
  - Switched from VK_KHR_dynamic_rendering (broken on Intel) to classic approach
  - Works across all GPU vendors
- **wgpu-Style Swapchain Synchronization** — Proper frame pacing for Windows/Intel
  - Rotating acquire semaphores (one per max frames in flight)
  - Per-image present semaphores
  - Post-acquire fence wait (fixes "Not Responding" on Windows)
  - Per-acquire fence tracking for stutter-free rendering
- **Fence Status Optimization** — Skip unnecessary fence waits
  - `vkGetFenceStatus` check before blocking wait
  - Improves frame latency when GPU is already done
- **Device Management** — New methods for resource management
  - `Device.WaitIdle()` — Wait for all GPU operations
  - `Device.ResetCommandPool()` — Reset all command buffers
- **WSI Function Loading** — Explicit loading of Window System Integration functions

### Fixed

#### Vulkan Backend
- **Intel Null Pipeline Workaround** — Defensive check for Intel Vulkan driver bug
  - Intel Iris Xe drivers may return `VK_SUCCESS` but write `VK_NULL_HANDLE` to pipeline
  - Returns `hal.ErrDriverBug` instead of crashing
- **goffi Pointer Argument Passing** — Fixed FFI calling convention
  - goffi expects pointer-to-pointer pattern for pointer arguments
- **vkGetDeviceProcAddr Loading** — Fixed device function loading on Intel
- **Validation Layer Availability** — Gracefully skip validation if Vulkan SDK not installed

### Changed
- Updated naga dependency v0.8.3 → v0.8.4 (SPIR-V instruction ordering fix)

### Dependencies
- `github.com/gogpu/naga` v0.8.4 (was v0.8.3)

## [0.9.2] - 2026-01-05

### Fixed

#### Metal Backend
- **NSString Double-Free** — Fix crash on autorelease pool drain ([#39])
  - `NSString()` used `stringWithUTF8String:` returning autoreleased object
  - Callers called `Release()` causing double-free when pool drained
  - Fix: Use `alloc/initWithUTF8String:` for +1 retained ownership

[#39]: https://github.com/gogpu/wgpu/pull/39

## [0.9.1] - 2026-01-05

### Fixed

#### Vulkan Backend
- **vkDestroyDevice Memory Leak** — Fixed memory leak when destroying Vulkan devices ([#32])
  - Device was not properly destroyed due to missing goffi call
  - Now correctly calls `vkDestroyDevice` via `ffi.CallFunction` with `SigVoidHandlePtr` signature
- **Features Mapping** — Implemented `featuresFromPhysicalDevice()` ([#33])
  - Maps 9 Vulkan features to WebGPU features (BC, ETC2, ASTC, IndirectFirstInstance, etc.)
  - Reference: wgpu-hal/src/vulkan/adapter.rs:584-829
- **Limits Mapping** — Implemented proper Vulkan→WebGPU limits mapping ([#34])
  - Maps 25+ hardware limits from `VkPhysicalDeviceLimits`
  - Includes: texture dimensions, descriptor limits, buffer limits, compute limits
  - Reference: wgpu-hal/src/vulkan/adapter.rs:1254-1392

[#32]: https://github.com/gogpu/wgpu/issues/32
[#33]: https://github.com/gogpu/wgpu/issues/33
[#34]: https://github.com/gogpu/wgpu/issues/34

## [0.9.0] - 2026-01-05

### Added

#### Core-HAL Bridge
- **Snatchable Pattern** — Safe deferred resource destruction with `Snatchable[T]` wrapper
- **TrackerIndex Allocator** — Efficient dense index allocation for resource state tracking
- **Buffer State Tracker** — Tracks buffer usage states for validation
- **Core Device with HAL** — `NewDevice()` creates device with HAL backend integration
- **Core Buffer with HAL** — `Device.CreateBuffer()` creates GPU-backed buffers
- **Core CommandEncoder** — Command recording with HAL dispatch

### Changed
- **Code Quality** — Replaced 58 TODO comments with proper documentation notes
  - Core layer: Deprecated legacy ID-based API functions with HAL-based alternatives
  - HAL backends: Documented feature gaps with version targets (v0.5.0, v0.6.0)

### Known Limitations (Vulkan Backend)

The following features are not yet fully implemented in the Vulkan backend:

| Feature | Status | Target |
|---------|--------|--------|
| Feature Detection | ~~Returns 0~~ **Fixed in v0.9.1** | ✅ |
| Limits Mapping | ~~Uses defaults~~ **Fixed in v0.9.1** | ✅ |
| Array Textures | Single layer only | v0.10.0 |
| Render Bundles | Not implemented | v0.10.0 |
| Timestamp Period | Hardcoded to 1.0 | v0.10.0 |

**Note:** Basic rendering (triangles, textures, compute) works correctly. These limitations affect capability reporting and advanced features only.

## [0.8.8] - 2026-01-04

### Fixed

#### CI
- **Metal Tests on CI** — Skip Metal tests on GitHub Actions (Metal unavailable in virtualized macOS)
  - See: https://github.com/actions/runner-images/discussions/6138

### Changed
- Updated dependency: `github.com/gogpu/naga` v0.8.2 → v0.8.3
  - Fixes MSL `[[position]]` attribute placement (now on struct member, not function)

## [0.8.7] - 2026-01-04

### Fixed

#### Metal Backend (ARM64)
- **ObjC Typed Arguments** — Proper type-safe wrappers for ARM64 AAPCS64 ABI compliance
- **Shader Creation** — Improved error handling in Metal shader module creation
- **Pipeline Creation** — Better error messages for render pipeline failures

### Added
- **Metal ObjC Tests** — Comprehensive test coverage for ObjC interop (`objc_test.go`)
- **Surface Tests** — Metal surface creation and configuration tests (`surface_test.go`)

### Changed
- Updated dependency: `github.com/go-webgpu/goffi` v0.3.6 → v0.3.7
- Updated dependency: `github.com/gogpu/naga` v0.8.1 → v0.8.2

### Contributors
- @ppoage — ARM64 ObjC fixes and Metal backend testing

## [0.8.6] - 2025-12-29

### Fixed
- **Metal Double Present Issue** — Removed duplicate `[drawable present]` call in `Queue.Present()`
  - `presentDrawable:` is already scheduled in `Submit()` before command buffer commit
  - Duplicate present was causing synchronization issues on some Metal drivers

### Changed
- Updated dependency: `github.com/go-webgpu/goffi` v0.3.5 → v0.3.6
  - **ARM64 HFA Returns** — `NSRect` (4×float64) now correctly returns all values on Apple Silicon
  - **Large Struct Returns** — Structs >16 bytes properly use X8 register for implicit pointer
  - **Fixes macOS ARM64 blank window** — `GetSize()` no longer returns (0,0) on M1/M2/M3/M4 Macs
  - Resolves [gogpu/gogpu#24](https://github.com/gogpu/gogpu/issues/24)

## [0.8.5] - 2025-12-29

### Added
- **DX12 Backend Registration** — DirectX 12 backend now auto-registers on Windows
  - Added `hal/dx12/init.go` with `RegisterBackend()` call
  - DX12 backend (~12.7K LOC) now available alongside Vulkan on Windows
  - Windows backend priority: Vulkan → DX12 → GLES → Software

## [0.8.4] - 2025-12-29

### Changed
- Updated dependency: `github.com/gogpu/naga` v0.8.0 → v0.8.1
  - Fixes missing `clamp()` built-in function in WGSL shader compilation
  - Adds comprehensive math function tests

## [0.8.3] - 2025-12-29

### Fixed
- **Metal macOS Blank Window** (Issue [gogpu/gogpu#24](https://github.com/gogpu/gogpu/issues/24))
  - Root cause: `[drawable present]` called separately after command buffer commit
  - Fix: Schedule `presentDrawable:` on command buffer BEFORE `commit` (Metal requirement)
  - Added `SetDrawable()` method to CommandBuffer for drawable attachment
  - Added `Drawable()` accessor to SurfaceTexture

- **Metal TextureView NSRange Parameters**
  - Root cause: `newTextureViewWithPixelFormat:textureType:levels:slices:` expects `NSRange` structs
  - Fix: Pass `NSRange` struct pointers instead of raw integers
  - Fixed array layer count calculation (was previously ignored)

## [0.8.2] - 2025-12-29

### Changed
- Updated dependency: `github.com/gogpu/naga` v0.6.0 → v0.8.0
  - HLSL backend for DirectX 11/12
  - Code quality and SPIR-V bug fixes
  - All 4 shader backends now stable
- Updated dependency: `github.com/go-webgpu/goffi` v0.3.3 → v0.3.5

## [0.8.1] - 2025-12-28

### Fixed
- **DX12 COM Calling Convention Bug** — Fixes device operations on Intel GPUs
  - Root cause: D3D12 methods returning structs require `this` pointer first, output pointer second
  - Affected methods: `GetCPUDescriptorHandleForHeapStart`, `GetGPUDescriptorHandleForHeapStart`,
    `GetDesc` (multiple types), `GetResourceAllocationInfo`
  - Reference: [D3D12 Struct Return Convention](https://joshstaiger.org/notes/C-Language-Problems-in-Direct3D-12-GetCPUDescriptorHandleForHeapStart.html)

- **Vulkan goffi Argument Passing Bug** — Fixes Windows crash (Exception 0xc0000005)
  - Root cause: vk-gen generated incorrect FFI calls after syscall→goffi migration
  - Before: `unsafe.Pointer(ptr)` passed pointer value directly
  - After: `unsafe.Pointer(&ptr)` passes pointer TO pointer (goffi requirement)
  - Affected all Vulkan functions with pointer parameters

### Added
- **DX12 Integration Test** (`cmd/dx12-test`) — Validates DX12 backend on Windows
  - Tests: backend creation, instance, adapter enumeration, device, pipeline layout

- **Compute Shader Support (Phase 2)** — Core API implementation
  - `ComputePipelineDescriptor` and `ProgrammableStage` types
  - `DeviceCreateComputePipeline()` and `DeviceDestroyComputePipeline()` functions
  - `ComputePassEncoder` with SetPipeline, SetBindGroup, Dispatch, DispatchIndirect
  - `CommandEncoderImpl.BeginComputePass()` for compute pass creation
  - Bind group index validation (0-3 per WebGPU spec)
  - Indirect dispatch offset alignment validation (4-byte)
  - Comprehensive tests (~700 LOC) with concurrent access testing

- **HAL Compute Infrastructure (Phase 1)**
  - GLES: `glDispatchCompute`, `glMemoryBarrier`, compute shader constants
  - DX12: `SetBindGroup` for ComputePassEncoder/RenderPassEncoder
  - Metal: Pipeline workgroup size extraction from naga IR

## [0.8.0] - 2025-12-26

### Added
- **DirectX 12 Backend** — Complete HAL implementation (~12K LOC)
  - Pure Go COM bindings via syscall (no CGO!)
  - D3D12 API access via COM interface vtables
  - DXGI integration for swapchain and adapter enumeration
  - Descriptor heap management (CBV/SRV/UAV, Sampler, RTV, DSV)
  - Flip model swapchain with tearing support (VRR)
  - Command list recording with resource barriers
  - Root signature and PSO creation
  - Buffer, Texture, TextureView, Sampler resources
  - RenderPipeline, ComputePipeline creation
  - Full format conversion (WebGPU → DXGI)

- **Metal CommandEncoder Test** — Regression test for Issue #24

### Changed
- All 5 HAL backends now complete:
  - Vulkan (~27K LOC) — Windows, Linux, macOS
  - Metal (~3K LOC) — macOS, iOS
  - DX12 (~12K LOC) — Windows
  - GLES (~7.5K LOC) — Windows, Linux
  - Software (~10K LOC) — All platforms

### Fixed
- Metal encoder test updated to use `IsRecording()` method instead of non-existent field

## [0.7.2] - 2025-12-26

### Fixed
- **Metal CommandEncoder State Bug** — Fixes Issue [#24](https://github.com/gogpu/wgpu/issues/24)
  - Root cause: `isRecording` flag was not set in `CreateCommandEncoder()`
  - Caused `BeginRenderPass()` to return `nil` on macOS
  - Fix: Removed boolean flag, use `cmdBuffer != 0` as state indicator
  - Follows wgpu-rs pattern where `Option<CommandBuffer>` presence indicates state
  - Added `IsRecording()` method for explicit state checking

### Changed
- Updated `github.com/gogpu/naga` dependency from v0.5.0 to v0.6.0

## [0.7.1] - 2025-12-26

### Added
- **ErrZeroArea error** — Sentinel error for zero-dimension surface configuration
  - Matches wgpu-core `ConfigureSurfaceError::ZeroArea` pattern
  - Comprehensive unit tests in `hal/error_test.go`

### Fixed
- **macOS Zero Dimension Crash** — Fixes Issue [#20](https://github.com/gogpu/gogpu/issues/20)
  - Added zero-dimension validation to all `Surface.Configure()` implementations
  - Returns `ErrZeroArea` when width or height is zero
  - Affected backends: Metal, Vulkan, GLES (Linux/Windows), Software
  - Follows wgpu-core pattern: "Wait to recreate the Surface until the window has non-zero area"

### Notes
- This fix allows proper handling of minimized windows and macOS timing issues
- Window becomes visible asynchronously on macOS; initial dimensions may be 0,0

## [0.7.0] - 2025-12-24

### Added
- **Metal WGSL→MSL Compilation** — Full shader compilation pipeline via naga v0.5.0
  - Parse WGSL source
  - Lower to intermediate representation
  - Compile to Metal Shading Language (MSL)
  - Create MTLLibrary from MSL source
- **CreateRenderPipeline** — Complete Metal implementation (~120 LOC)
  - Get vertex/fragment functions from library
  - Configure color attachments and blending
  - Create MTLRenderPipelineState

### Changed
- Added `github.com/gogpu/naga v0.5.0` dependency

## [0.6.1] - 2025-12-24

### Fixed
- **macOS ARM64 SIGBUS crash** — Corrected goffi API usage in Metal backend
  - Fixed pointer argument passing pattern for Objective-C runtime calls
  - Resolved SIGBUS errors on Apple Silicon (M1/M2/M3) systems
- **GLES/EGL CI integration tests** — Implemented EGL surfaceless platform
  - Added `EGL_MESA_platform_surfaceless` support for headless testing
  - Added `QueryClientExtensions()` and `HasSurfacelessSupport()` functions
  - Updated `DetectWindowKind()` to prioritize surfaceless in CI environments
  - Removed Xvfb dependency, using Mesa llvmpipe software renderer
- **staticcheck SA5011 warnings** — Added explicit returns after `t.Fatal()` calls

### Changed
- Updated goffi to v0.3.2 for ARM64 macOS compatibility
- CI workflow now uses `LIBGL_ALWAYS_SOFTWARE=1` for reliable headless EGL

## [0.6.0] - 2025-12-23

### Added
- **Metal backend** (`hal/metal/`) — Pure Go via goffi (~3K LOC)
  - Objective-C runtime bindings via goffi (go-webgpu/goffi)
  - Metal framework access: MTLDevice, MTLCommandQueue, MTLCommandBuffer
  - Render encoder: MTLRenderCommandEncoder, MTLRenderPassDescriptor
  - Resource management: MTLBuffer, MTLTexture, MTLSampler
  - Pipeline state: MTLRenderPipelineState, MTLDepthStencilState
  - Surface presentation via CAMetalLayer
  - Format conversion: WebGPU → Metal texture formats
  - Cross-compilable from Windows/Linux to macOS

### Changed
- Updated ecosystem: gogpu v0.5.0 (macOS Cocoa), naga v0.5.0 (MSL backend)
- Pre-release check script now uses kolkov/racedetector (Pure Go, no CGO)

### Notes
- **Community Testing Requested**: Metal backend needs testing on real macOS systems (12+ Monterey)
- Requires naga v0.5.0 for MSL shader compilation

## [0.5.0] - 2025-12-19

### Added
- **Software rasterization pipeline** (`hal/software/raster/`) — Full CPU-based triangle rendering
  - Edge function (Pineda) algorithm with top-left fill rule
  - Perspective-correct attribute interpolation
  - Depth buffer with 8 compare functions (Never, Less, Equal, LessEqual, etc.)
  - Stencil buffer with 8 operations (Keep, Zero, Replace, IncrementClamp, etc.)
  - 13 blend factors, 5 blend operations (WebGPU spec compliant)
  - 6-plane frustum clipping (Sutherland-Hodgman algorithm)
  - Backface culling (CW/CCW winding)
  - 8x8 tile-based rasterization for cache locality
  - Parallel rasterization with worker pool
  - Incremental edge evaluation (O(1) per pixel stepping)
  - ~6K new lines of code, 70+ tests
- **Callback-based shader system** (`hal/software/shader/`)
  - `VertexShaderFunc` and `FragmentShaderFunc` interfaces
  - Built-in shaders: SolidColor, VertexColor, Textured
  - Custom shader support for flexible rendering
  - Matrix utilities (Mat4, transforms)
  - ~1K new lines of code, 30+ tests

### Changed
- Pre-release check script now matches CI behavior for go vet exclusions
- Improved WSL fallback for race detector tests

## [0.4.0] - 2025-12-13

### Added
- **Linux support for OpenGL ES backend** (`hal/gles/`) via EGL
  - EGL bindings using goffi (Pure Go FFI)
  - Platform detection: X11, Wayland, Surfaceless (headless)
  - Full Device and Queue HAL implementations
  - CI integration tests with Mesa software renderer
  - ~4000 new lines of code

## [0.3.0] - 2025-12-10

### Added
- **Software backend** (`hal/software/`) - CPU-based rendering for headless scenarios
  - Real data storage for buffers and textures
  - Clear operations (fill framebuffer with color)
  - Buffer/texture copy operations
  - Thread-safe access with `sync.RWMutex`
  - `Surface.GetFramebuffer()` for pixel readback
  - 11 unit tests
  - Build tag: `-tags software`
- Use cases: CI/CD testing, server-side image generation, embedded systems

## [0.2.0] - 2025-12-08

### Added
- **Vulkan backend** (`hal/vulkan/`) - Complete HAL implementation (~27K LOC)
  - Auto-generated bindings from official Vulkan XML specification
  - Memory allocator with buddy allocation
  - Vulkan 1.3 dynamic rendering
  - Swapchain management with automatic recreation
  - Complete resource support: Buffer, Texture, Sampler, Pipeline, etc.
  - 93 unit tests
- Native Go backend integration with gogpu/gogpu

### Changed
- Backend registration system improved

## [0.1.0] - 2025-12-07

### Added
- Initial release
- **Types package** (`types/`) - WebGPU type definitions
  - Backend types (Vulkan, Metal, DX12, GL)
  - 100+ texture formats
  - Buffer, sampler, shader types
  - Vertex formats with size calculations
- **Core package** (`core/`) - Validation and state management
  - Type-safe ID system with generics
  - Epoch-based use-after-free prevention
  - Hub with 17 resource registries
  - 127 tests with 95% coverage
- **HAL package** (`hal/`) - Hardware abstraction layer
  - Backend, Instance, Adapter, Device, Queue interfaces
  - Resource interfaces
  - Command encoding
  - Backend registration system
  - 54 tests with 94% coverage
- **Noop backend** (`hal/noop/`) - Reference implementation for testing
- **OpenGL ES backend** (`hal/gles/`) - Pure Go via goffi (~3.5K LOC)

[#55]: https://github.com/gogpu/wgpu/issues/55
[Unreleased]: https://github.com/gogpu/wgpu/compare/v0.16.6...HEAD
[0.16.6]: https://github.com/gogpu/wgpu/compare/v0.16.5...v0.16.6
[0.16.5]: https://github.com/gogpu/wgpu/compare/v0.16.4...v0.16.5
[0.16.4]: https://github.com/gogpu/wgpu/compare/v0.16.3...v0.16.4
[0.16.3]: https://github.com/gogpu/wgpu/compare/v0.16.2...v0.16.3
[0.16.2]: https://github.com/gogpu/wgpu/compare/v0.16.1...v0.16.2
[0.16.1]: https://github.com/gogpu/wgpu/compare/v0.16.0...v0.16.1
[0.16.0]: https://github.com/gogpu/wgpu/compare/v0.15.1...v0.16.0
[0.15.1]: https://github.com/gogpu/wgpu/compare/v0.15.0...v0.15.1
[0.15.0]: https://github.com/gogpu/wgpu/compare/v0.14.0...v0.15.0
[0.14.0]: https://github.com/gogpu/wgpu/compare/v0.13.2...v0.14.0
[0.13.2]: https://github.com/gogpu/wgpu/compare/v0.13.1...v0.13.2
[0.13.1]: https://github.com/gogpu/wgpu/compare/v0.13.0...v0.13.1
[0.13.0]: https://github.com/gogpu/wgpu/compare/v0.12.0...v0.13.0
[0.12.0]: https://github.com/gogpu/wgpu/compare/v0.11.2...v0.12.0
[0.11.2]: https://github.com/gogpu/wgpu/compare/v0.11.1...v0.11.2
[0.11.1]: https://github.com/gogpu/wgpu/compare/v0.10.3...v0.11.1
[0.10.3]: https://github.com/gogpu/wgpu/compare/v0.10.2...v0.10.3
[0.10.2]: https://github.com/gogpu/wgpu/compare/v0.10.1...v0.10.2
[0.10.1]: https://github.com/gogpu/wgpu/compare/v0.10.0...v0.10.1
[0.10.0]: https://github.com/gogpu/wgpu/compare/v0.9.3...v0.10.0
[0.9.3]: https://github.com/gogpu/wgpu/compare/v0.9.2...v0.9.3
[0.9.2]: https://github.com/gogpu/wgpu/compare/v0.9.1...v0.9.2
[0.9.1]: https://github.com/gogpu/wgpu/compare/v0.9.0...v0.9.1
[0.9.0]: https://github.com/gogpu/wgpu/compare/v0.8.8...v0.9.0
[0.8.8]: https://github.com/gogpu/wgpu/compare/v0.8.7...v0.8.8
[0.8.7]: https://github.com/gogpu/wgpu/compare/v0.8.6...v0.8.7
[0.8.6]: https://github.com/gogpu/wgpu/compare/v0.8.5...v0.8.6
[0.8.5]: https://github.com/gogpu/wgpu/compare/v0.8.4...v0.8.5
[0.8.4]: https://github.com/gogpu/wgpu/compare/v0.8.3...v0.8.4
[0.8.3]: https://github.com/gogpu/wgpu/compare/v0.8.2...v0.8.3
[0.8.2]: https://github.com/gogpu/wgpu/compare/v0.8.1...v0.8.2
[0.8.1]: https://github.com/gogpu/wgpu/compare/v0.8.0...v0.8.1
[0.8.0]: https://github.com/gogpu/wgpu/compare/v0.7.2...v0.8.0
[0.7.2]: https://github.com/gogpu/wgpu/compare/v0.7.1...v0.7.2
[0.7.1]: https://github.com/gogpu/wgpu/compare/v0.6.1...v0.7.1
[0.6.1]: https://github.com/gogpu/wgpu/compare/v0.6.0...v0.6.1
[0.6.0]: https://github.com/gogpu/wgpu/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/gogpu/wgpu/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/gogpu/wgpu/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/gogpu/wgpu/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/gogpu/wgpu/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/gogpu/wgpu/releases/tag/v0.1.0
