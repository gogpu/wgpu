# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.23.8] - 2026-04-05

### Fixed

#### Metal

- **Descending vertex buffer indices** — Metal vertex, uniform and storage buffers
  share the same index range. Vertex buffers now use descending indices from the
  end of the range (`maxVertexBuffers - 1 - slot`) to avoid collisions with
  uniform/storage buffers assigned from the start. Matches Rust wgpu-hal pattern.
  Contributed by @jdbann. (gogpu/gogpu#165)

#### GLES

- **Per-type sequential binding counters** — Replaced hardcoded `group*16+binding`
  formula with per-type sequential counters (samplers, textures, images, uniform
  buffers, storage buffers) computed at PipelineLayout creation. Fixes binding
  collision when >16 bindings per group. Removed all `maxBindingsPerGroup=16`
  constants. Matches Rust wgpu-hal `device.rs:1154-1221`. (GLES-001)

- **StagingBelt configurable alignment** — Default alignment changed from 16 to 8
  bytes (Rust wgpu `MAP_ALIGNMENT` parity). Alignment now configurable per-belt.
  WebGPU `COPY_BUFFER_ALIGNMENT` is 4. (TASK-WGPU-BELT-002)

#### Core

- **Default limits when RequiredLimits is zero struct** — `RequestDevice()` with a
  descriptor containing zero-value `RequiredLimits` caused all device limits to be 0,
  rejecting all bind group layouts ("binding count N exceeds maximum 0"). Now detects
  zero struct and falls back to WebGPU spec defaults. Matches Rust wgpu
  `DeviceDescriptor::default()` behavior. (BUG-WGPU-LIMITS-001)

- **PowerPreference fallback per WebGPU spec** — `RequestAdapter()` with
  `PowerPreference: HighPerformance` on systems with only integrated GPU returned
  error instead of falling back. WebGPU spec: powerPreference is a hint, "must not
  cause requestAdapter() to fail if there is at least one available adapter."
  Now uses two-pass selection: prefer matching, fall back to any GPU. Matches Rust
  wgpu sort-not-filter approach. (BUG-WGPU-ADAPTER-002)

- **Device inherits adapter limits instead of WebGPU defaults** — `RequestDevice()`
  with empty `RequiredLimits` returned WebGPU spec minimums (e.g., 8 storage buffers)
  instead of the adapter's actual capabilities (e.g., 200 on Intel Iris Xe). Blocked
  gg Vello coarse shader (9 storage buffer bindings). Device now inherits adapter
  limits when no explicit limits requested. (BUG-WGPU-LIMITS-002)

## [0.23.7] - 2026-04-04

### Changed

- **naga v0.16.1 → v0.16.4** — HLSL 72/72 parity, ForceLoopBounding,
  per-element workgroup array zero-init (330× faster FXC for arrays ≥256).
  GLSL same fix for GL driver compiler slowdown.

## [0.23.6] - 2026-04-04

### Added

#### Core

- **Deferred resource destruction (ResourceRef + DestroyQueue)** — All GPU
  resources (Buffer, Texture, TextureView, BindGroup, Pipeline, Sampler, etc.)
  now defer HAL destruction until the GPU completes the submission that was active
  when `Release()` was called. Prevents use-after-free crashes on DX12 (TDR) and
  validation errors on Vulkan when resources are released while the GPU is still
  rendering with them. Implements Go equivalent of Rust wgpu-core's
  `LifetimeTracker` pattern with `ResourceRef` (atomic refcount, Go analog of
  Rust `Arc`) and `DestroyQueue` (submission-scoped triage).
  (TASK-WGPU-CORE-LIFETIME-001)

- **Per-command-buffer resource tracking** — Command encoders now Clone()
  ResourceRef for every resource bound during render/compute pass encoding
  (SetVertexBuffer, SetBindGroup, SetPipeline, etc.). Refs transfer through
  `End()` → `Finish()` → `Submit()` → `DestroyQueue.TrackSubmission()` and are
  Drop()'d when GPU completes the submission. Matches Rust wgpu-core
  `EncoderInFlight` pattern where `Arc` refs in trackers keep resources alive.
  (TASK-WGPU-CORE-LIFETIME-002)

- **Unified destruction mechanism** — Migrated TextureView and BindGroup from
  duplicate `pendingWrites` deferred mechanism to `core.DestroyQueue`. All 9
  resource types now use a single destruction path. Removed 234 lines of
  duplicate code. (TASK-WGPU-CORE-LIFETIME-003)

#### DX12

- **In-memory HLSL→DXBC shader cache** — Caches FXC compilation results keyed
  by SHA-256(HLSL source) + entry point + stage + target profile. 30 pipelines
  sharing 8 unique shaders → 8 FXC calls instead of 30. LRU eviction at 200
  entries retaining last 100. Matches Rust wgpu ShaderCache pattern
  (wgpu-hal/src/dx12/mod.rs:1136). Fixes DEVICE_HUNG on first frame for complex
  UI (Gallery with 15-30 PSOs). (TASK-DX12-PSO-CACHE-001)

- **DRED diagnostics (Device Removed Extended Data)** — When DX12 debug mode is
  enabled (`InstanceFlagsDebug`), auto-breadcrumbs and page fault tracking are
  activated. On TDR/device removal, logs which GPU command was executing
  (breadcrumb context window around hang point) and which allocation was accessed
  (use-after-free detection via recently freed allocations list). Provides
  enterprise-level GPU crash diagnostics not available in Rust wgpu.
  (TASK-DX12-DRED-001)

### Fixed

#### Core

- **Deferred resource destruction — use-after-free fix** — `Buffer.Release()`,
  `Texture.Release()`, and all other resource `Release()` methods were calling HAL
  destroy immediately while the GPU was still using the resource. On DX12 this caused
  TDR (DEVICE_HUNG) after ~300-700 frames in gallery app. Root cause: missing
  LifetimeTracker after wgpu core API migration (v0.21.0). Now all 9 resource types
  defer destruction via `core.DestroyQueue` until GPU completes the associated
  submission. (TASK-WGPU-CORE-LIFETIME-001)

## [0.23.5] - 2026-04-04

### Fixed

#### GLES

- **ADJUST_COORDINATE_SPACE for correct gl_FragCoord Y convention** — GLES backend
  was missing naga's `ADJUST_COORDINATE_SPACE` flag, causing `gl_FragCoord.y` to use
  OpenGL convention (Y=0 at bottom) instead of WebGPU (Y=0 at top). This broke
  `rrect_clip_coverage()` in fragment shaders and required a fragile manual scissor
  Y-flip. Now matches Rust wgpu-hal GLES with 4 coordinated changes: naga Y-flip
  in vertex shader, scissor pass-through, front face CW↔CCW swap, and MSAA resolve
  blit Y-flip for presentation. Fixes invisible scrollbar and dividers in UI on GLES.
  (BUG-GLES-SCROLLBAR-001)

- **Normalized vertex format support (unorm/snorm)** — `vertexFormatToGL()` was
  missing `Unorm8x4`, `Snorm8x4`, `Unorm16x2`, `Snorm16x2` etc. These fell back
  to float formats (16 bytes instead of 4), causing incorrect per-vertex color
  rendering. Added all normalized variants and `normalized=true` parameter in
  `glVertexAttribPointer`. Required for text batching per-vertex color (unorm8x4).

#### Vulkan

- **Fence pool recycling before allocation** — `fencePool.signal()` was not calling
  `maintain()` before allocating a fence, causing signaled fences to accumulate in
  the active list instead of being recycled to the free list. On the binary fence
  path (Vulkan 1.0/1.1 without timeline semaphores), every `vkQueueSubmit` created
  a new `VkFence` via `vkCreateFence`. On NVIDIA Linux drivers, each `VkFence`
  consumes a file descriptor — ~1000 unrecycled fences exhaust the default FD limit
  and crash with `VK_ERROR_OUT_OF_HOST_MEMORY`. Now calls `maintain()` before
  allocation, matching Rust wgpu-hal `Queue::submit`. No impact on timeline
  semaphore path (Vulkan 1.2+). (VK-SYNC-002)

### Added

- **Blend constant draw-time validation** — `Draw`, `DrawIndexed`, `DrawIndirect`,
  and `DrawIndexedIndirect` now validate that `SetBlendConstant()` has been called
  when the current pipeline uses `BlendFactorConstant` or
  `BlendFactorOneMinusConstant`. Without this, the GPU uses undefined blend constant
  values, causing silent rendering errors. Pipeline creation scans color targets to
  detect constant blend factor usage. Matches Rust wgpu-core `OptionalState` pattern
  and `DrawError::MissingBlendConstant`. (VAL-005)

### Changed

- **gputypes v0.3.0 → v0.4.0** — adds `BlendComponent.UsesConstant()` used by
  blend constant validation (VAL-005).
- **naga v0.16.0 → v0.16.1** — SPIR-V backend 164/164 validation pass (100%).
  Fixes atomics, barriers, images, pointer spill, binding-arrays, depth sampling,
  integer ops, matrix decomposition.

## [0.23.4] - 2026-04-02

### Fixed

#### GLES

- **Texture completeness for non-mipmapped textures** — `GL_TEXTURE_MAX_LEVEL`
  defaulted to 1000, making single-mip textures incomplete (invisible text, missing
  UI elements). Now set to `MipLevelCount-1`. Texture uploads use `glTexSubImage2D`
  after pre-allocated `glTexImage2D` storage, matching Rust wgpu-hal pattern.
  (BUG-GLES-TEXT-001)

- **DYNAMIC_DRAW for all writable buffers** — `GL_STATIC_DRAW` was used for
  non-MAP_READ buffers. Some vendors (Intel) take the hint literally, causing
  stale data on frequently rewritten uniform/storage buffers. Now uses
  `GL_DYNAMIC_DRAW` for all non-read-only buffers, matching Rust wgpu-hal.

#### DX12

- **Deferred BindGroup/TextureView descriptor destruction** — Root cause of DX12
  TDR (GPU timeout) with `maxFramesInFlight=2`: descriptors in SRV/sampler heaps
  were freed immediately on `Release()` while the GPU was still referencing them.
  Descriptors now tracked via `AddPendingRef`/`DecPendingRef` and freed only after
  GPU fence confirms completion. (BUG-DX12-007)

- **Staging SRV/sampler heap descriptor recycling** — Heap descriptor slots were
  not returned to the free list after GPU completion, causing gradual exhaustion.
  (BUG-DX12-008)

- **Texture initial state COMMON instead of COPY_DEST** — DX12 textures were
  created in `D3D12_RESOURCE_STATE_COPY_DEST`. Correct initial state is
  `D3D12_RESOURCE_STATE_COMMON`, which is implicitly promotable to any read state.
  (BUG-DX12-009)

- **Buffer barriers after PendingWrites copies** — Buffers stayed in
  `COPY_DEST` state after staging copies, causing undefined behavior on
  subsequent shader reads. Added `COPY_DEST -> VERTEX/INDEX/CONSTANT/SRV`
  transition barriers after copy commands. (BUG-DX12-010)

### Added

- **GLES: SamplerBindMap for combined texture-sampler binding** — GLES lacks
  separate texture and sampler objects. WGSL `texture_sample(t, s, uv)` now
  correctly maps to combined GLSL `sampler2D` via `SamplerBindMap` derived from
  naga `TextureMappings`. Matches Rust wgpu-hal GLES architecture.

- **DX12: GPU-based validation** — `InstanceFlagsValidation` enables D3D12
  GPU-based validation (GBV) for catching shader-level resource access errors.

- **DX12: encoder pool (Rust wgpu-core CommandAllocator pattern)** — Command
  allocators are pooled and recycled after GPU fence completion instead of
  allocated per-encoder. Reduces DX12 memory churn.

- **StagingBelt ring-buffer allocator (Rust wgpu util::StagingBelt pattern)** —
  Replaces per-WriteBuffer staging buffer creation with bump-pointer sub-allocation
  from reusable 256KB chunks. Zero heap allocations in steady state (0 allocs/op,
  22ns — 15× faster than per-write staging). Oversized writes (> chunkSize) fall
  back to one-off buffers. Chunks recycled after GPU completion via recall().

- **Instance flags propagation** — `InstanceFlags` (debug layer, validation)
  now propagated from `wgpu.CreateInstance` through to HAL backends.

### Changed

- **naga v0.15.2 → v0.16.0** — GLSL TextureMappings for SamplerBindMap, 34 SPIR-V
  validation fixes (spirv-val 52% → 73%), depth texture combined sampler fix.

## [0.23.3] - 2026-04-01

### Fixed

- **GLES: blurred text on Qualcomm Adreno** — Unconditional `GL_LINEAR` texture
  defaults caused blurry font rendering when sampler override was incomplete.
  Now aligned with Rust wgpu: only set `GL_NEAREST` for non-filterable formats
  (integer, depth, 32-bit float), let sampler objects control filterable textures.

- **DX12/Vulkan/Metal: PendingWrites batching (Rust wgpu-core pattern)** —
  `WriteBuffer`/`WriteTexture` previously did per-call staging→submit→WaitIdle
  (20+ GPU round-trips per frame). Now batched into a single shared command
  encoder, flushed once at `Queue.Submit`. Staging buffers freed asynchronously
  via fence tracking. Reduces DX12 submits from 120 to ~10 per frame.

### Added

- **Enterprise logging system (Rust wgpu parity)** — Comprehensive diagnostic
  logging across DX12 and GLES backends, matching Rust wgpu's tracing patterns:
  - **DX12**: adapter capabilities (ResourceBindingTier, TiledResourcesTier),
    descriptor heap creation, pipeline layout, HLSL compilation preview,
    pipeline creation timing, submit/present timing, fence signal timing,
    descriptor heap exhaustion errors, texture label in error logs
  - **GLES**: GL_VENDOR/GL_RENDERER/GL_VERSION at device init, generated GLSL
    source preview, shader compile/link info log on success (driver warnings),
    texture creation, sampler creation/binding, WriteTexture, pipeline timing
  - Enable with `GOGPU_LOG=debug` or `GOGPU_WGPU_LOG=debug`

## [0.23.2] - 2026-03-31

### Fixed

- **DX12: vertex input semantic mismatch** — Changed input layout semantic from
  `TEXCOORD{N}` to `LOC{N}` to match naga HLSL output. DX12 validates exact
  semantic name match between shader and input layout. Previous mismatch caused
  `CreateGraphicsPipelineState` to fail with `E_INVALIDARG` on all render pipelines.

- **GLES: texture sampling broken — BindingMap not passed to GLSL compiler** —
  Without BindingMap, naga GLSL emitted default `layout(binding=0)` for all
  samplers. Runtime bound textures at `group*16+binding` (unit 1+). Shader
  sampled unit 0 (empty) → invisible text and textures on all GLES backends.

- **GLES (Linux): WriteTexture used wrong internalFormat** — Linux path discarded
  `internalFormat` from `textureFormatToGL()` and passed `GL_TEXTURE_2D` (0x0DE1)
  as internal format to `glTexImage2D`. Texture upload silently failed.

- **GLES: missing GL_UNPACK_ALIGNMENT for R8 textures** — Added
  `glPixelStorei(GL_UNPACK_ALIGNMENT, 1)` before R8 uploads to prevent corrupted
  font glyphs on non-power-of-2 widths.

- **DX12: proper sampler heap** — Implemented global sampler pool + per-group
  sampler index buffers (matching Rust wgpu-hal architecture). Deferred HLSL
  compilation with SamplerBufferBindingMap. Fixes invisible text/textures on DX12.

### Known Issues

- **DX12: rendering noticeably slower than Vulkan/GLES** — Needs profiling.
  Possible causes: HLSL compilation overhead, descriptor heap allocation, sync.
  Tracked in PERF-DX12-001.

## [0.23.0] - 2026-03-30

### Added

- **`Adapter.GetSurfaceCapabilities(surface)`** — New public API. Returns supported
  texture formats, present modes, and composite alpha modes for a surface. Queries
  HAL capabilities (`vkGetPhysicalDeviceSurfacePresentModesKHR` on Vulkan). Follows
  Rust wgpu `surface.get_capabilities(&adapter)` pattern.

- **`Queue.Poll()`** — Non-blocking completion query. Returns the highest submission
  index known to be completed by the GPU.

### Changed

- **Enterprise fence architecture** — HAL `Queue.Submit()` no longer accepts user
  fence parameter. Returns `(submissionIndex uint64, err error)` instead. HAL owns
  all fence management internally (binary fence pool or timeline semaphore). Matches
  Rust wgpu, Dawn, Godot, DXVK, vkd3d-proton architecture. All 6 backends updated
  (Vulkan, DX12, Metal, GLES, Software, Noop). Eliminates double `vkQueueSubmit`
  on binary fence path that caused first-frame loss on llvmpipe Vulkan 1.0.2.
  (BUG-GOGPU-004, fixes ui#66)

- **`Queue.Submit()` is now non-blocking** — Returns `(uint64, error)` with
  submission index for deferred resource tracking. Use `Queue.Poll()` to check
  completion.

- **deps: naga v0.14.8 → v0.15.0** — Full Rust parity: IR 144/144, SPIR-V 87/87,
  MSL 91/91, HLSL 58/58, GLSL 68/68.

### Removed

- **`Queue.SubmitWithFence()`** — Replaced by `Queue.Submit()` + `Queue.Poll()`.
  HAL manages fences internally, application layer uses submission indices.

## [0.22.2] - 2026-03-29

### Fixed

- **Metal: per-type sequential slot indices in SetBindGroup** — Fixed descriptor set
  binding for Metal backend when multiple bind groups use different resource types
  (samplers, textures, buffers). Previously all resources shared a single index counter,
  causing incorrect slot assignments. (PR #112 by @timzifer)

- **GLES: disable scissor test before MSAA resolve blit** — Prevents clipped resolve
  when scissor rect from previous draw call was still active during MSAA resolve.
  Fixes rendering artifacts on NVIDIA GPUs. (gg#226)

### Changed

- **deps: goffi v0.4.2 → v0.5.0** — Adds Windows ARM64 (Snapdragon X) support.
  First Go GPU framework with Windows ARM64 support. (go-webgpu/goffi#31, tested by @SideFx)

## [0.22.1] - 2026-03-20

### Fixed

- **Vulkan: null command buffer guards** — Defense-in-depth checks across 19 methods
  prevent SIGSEGV if vkAllocateCommandBuffers silently returns null handle. 17 unit tests.

- **GLES: disable scissor before glClear** — Prevents garbage/noise pixels during
  window resize. glClear was clipped by stale scissor rect from previous frame.

### Performance

- **Vulkan: restore post-acquire fence wait** — Re-enabled fence in vkAcquireNextImageKHR
  for proper frame pacing on Windows (Intel driver timeouts fixed in 2026 drivers).
  Matches Rust wgpu pattern (issues #8310, #8354).

- **DX12: waitable swapchain frame latency** — GetFrameLatencyWaitableObject + wait
  in AcquireTexture. Was flag-only (no-op). Now provides proper CPU frame pacing.

## [0.22.0] - 2026-03-20

### Added

- **GLES: GL sampler objects** — Proper sampler state via `glGenSamplers`/`glBindSampler`
  (GL 3.3+). Samplers now honor `FilterModeLinear`/`FilterModeNearest`, address modes,
  LOD clamp, anisotropy, and compare functions. Previously all textures were hardcoded
  to `GL_NEAREST` and sampler bindings were no-ops. Matches Rust wgpu GLES approach.

- **GLES: texture unit overflow validation** — Warns via `slog` when flattened binding
  index exceeds `GL_MAX_TEXTURE_IMAGE_UNITS` (typically 8 on Intel). Reports actual
  hardware limit in adapter `Limits.MaxSampledTexturesPerShaderStage`.

### Fixed

- **GLES: scissor Y-coordinate flip** — `glScissor` now correctly converts WebGPU
  top-left origin to OpenGL bottom-left origin (`glY = fbHeight - y - height`).
  Previously the scissor was vertically mirrored, clipping out content in complex
  UI layouts with nested clip rects. Includes clamp to 0 for safety.

- **GLES: Linux colorWriteMask** — `CreateRenderPipeline` on Linux was missing
  `colorWriteMask` extraction from fragment targets, causing all color writes to
  be masked (black screen). Now matches Windows implementation.

- **GLES: Linux CreateBuffer nil check** — Added nil descriptor guard matching
  the Windows version to prevent nil pointer panic.

- **GLES: texture defaults changed to LINEAR** — Default texture filter changed
  from `GL_NEAREST` to `GL_LINEAR`. GL sampler objects override this when bound.

### Performance

- **DX12: batch CopyDescriptors** — `CreateBindGroup` now batches descriptor copies
  via the full `CopyDescriptors` D3D12 API instead of calling `CopyDescriptorsSimple`
  per descriptor (~800 syscalls/frame → ~200). Estimated +20-30% FPS for complex UI.

- **DX12: frame pacing** — GPU wait moved from `Present` to `AcquireTexture`,
  allowing CPU/GPU overlap. Matches Rust wgpu approach. Estimated +15-25% FPS
  when GPU is the bottleneck.

- **DX12: pool descriptor heap slice** — Replaced heap-allocated slice in
  `ensureDescriptorHeapsBound` with fixed `[2]` array field on `CommandEncoder`.

## [0.21.3] - 2026-03-16

### Added

- **software: Draw() with vertex rasterization + textured blit** — Software backend
  now renders textured quads (fullscreen blit) and vertex-buffer-based triangles via
  `raster.Pipeline`. Resource registry for handle→resource lookup. MSAA resolve in End().
  21 tests.

- **core: entry-by-entry BindGroupLayout compatibility** — Layouts compared by entries,
  not pointer equality, matching WebGPU spec and Rust wgpu-core. 7 tests.

- **core: lazy GLES adapter enumeration with surface hint** — GLES backends defer
  adapter enumeration until `RequestAdapter` with `CompatibleSurface`. OpenGL requires
  GL context which only exists after surface creation.

- **RequestAdapterOptions** — Proper struct with `CompatibleSurface *Surface` field
  (was alias to gputypes). Follows WebGPU spec `requestAdapter({compatibleSurface})`.

### Fixed

- **DX12: reduce CBV/SRV/UAV heap to 1M** — D3D12 Tier 1/2 spec maximum. Was 1,048,576.
  Fixes `E_INVALIDARG` on NVIDIA. ([wgpu#106](https://github.com/gogpu/wgpu/issues/106))

- **GLES: nil context guard in Adapter.Open** — Returns error instead of panic when
  adapter created without surface. ([wgpu#107](https://github.com/gogpu/wgpu/issues/107))

- **GLES: match naga flattened binding indices** — GL binding = `group * 16 + binding`,
  matching naga GLSL output. Fixes SDF shapes invisible on GLES.

- **core: prefer GPU adapters over Software in RequestAdapter** — GPU adapters selected
  before CPU/Software. ForceFallbackAdapter correctly returns CPU. 3 tests.

### Dependencies

- naga v0.14.7 → v0.14.8 (GLSL bind group collision fix)

## [0.21.2] - 2026-03-16

### Added

- **core: Binder struct for render/compute pass validation** — Tracks assigned vs expected
  bind group layouts per slot (matching Rust wgpu-core pattern). At draw/dispatch time,
  `checkCompatibility()` verifies all expected slots have compatible bind groups assigned.
  13 binder tests.

- **core: comprehensive render/compute pass state validation** — SetBindGroup validates
  MAX_BIND_GROUPS hard cap (8), pipeline bind group count, and dynamic offset alignment
  (256 bytes). Draw/DrawIndexed validate pipeline is set, vertex buffer count, and index
  buffer presence. Dispatch validates pipeline set + bind group compatibility.
  25+ new tests.

### Fixed

- **core: SetBindGroup index bounds validation** — Prevents `vkCmdBindDescriptorSets`
  crash on AMD/NVIDIA GPUs when bind group index exceeds pipeline layout set count.
  Intel silently tolerates this spec violation; AMD/NVIDIA crash with access violation.
  Fixes [ui#52](https://github.com/gogpu/ui/issues/52).

## [0.21.1] - 2026-03-15

### Fixed

- **core: per-stage resource limit validation in CreateBindGroupLayout** — Validates
  storage buffer, uniform buffer, sampler, sampled texture, and storage texture counts
  per shader stage against device limits before calling HAL. Prevents wgpu-native abort
  when Vello compute requests 9 storage buffers on devices with limit 8. Error is now
  returned gracefully, enabling fallback to SDF renderer.

## [0.21.0] - 2026-03-15

### Added

- **public API: complete three-layer WebGPU stack** — The root `wgpu` package now
  provides a full typed API for GPU programming. All operations go through
  wgpu (public) → wgpu/core (validation) → wgpu/hal (backend). Consumers never
  need to import `wgpu/hal` for standard use.

- **public API: SetLogger / Logger** — `wgpu.SetLogger()` and `wgpu.Logger()`
  propagate the logger to the entire stack (API, core, HAL backends).

- **public API: Fence and async submission** — `Fence` type, `Device.CreateFence()`,
  `WaitForFence()`, `ResetFence()`, `GetFenceStatus()`, `FreeCommandBuffer()`.
  `Queue.SubmitWithFence()` for non-blocking GPU submission with fence signaling.

- **public API: Surface lifecycle** — `Surface.SetPrepareFrame()` for platform
  HiDPI/DPI hooks. `Surface.DiscardTexture()` for canceled frames. `Surface.HAL()`
  escape hatch. Delegates to `core.Surface` state machine.

- **public API: CommandEncoder extensions** — `CopyTextureToBuffer()`,
  `TransitionTextures()`, `DiscardEncoding()`. All use wgpu types (no hal in signatures).

- **public API: HAL accessors** — `Device.HalDevice()`, `Device.HalQueue()`,
  `Texture.HalTexture()`, `TextureView.HalTextureView()` for advanced interop.

- **public API: proper type definitions** — Replaced hal type aliases with proper
  structs: `Extent3D`, `Origin3D`, `ImageDataLayout`, `DepthStencilState`,
  `StencilFaceState`, `TextureBarrier`, `TextureRange`, `TextureUsageTransition`,
  `BufferTextureCopy`. Unexported `toHAL()` converters. No hal leakage in godoc.

- **core: complete resource types (CORE-001)** — All 12 stub resource types
  (Texture, Sampler, BindGroupLayout, PipelineLayout, BindGroup, ShaderModule,
  RenderPipeline, ComputePipeline, CommandEncoder, CommandBuffer, QuerySet, Surface)
  now have full struct definitions with HAL handle wrapping.

- **core: Surface state machine (CORE-002)** — Unconfigured → Configured → Acquired
  lifecycle with PrepareFrameFunc hook and auto-reconfigure on dimension changes.

- **core: CommandEncoder state machine (CORE-003)** — Recording/InRenderPass/
  InComputePass/Finished/Error states with validated transitions.

- **core: resource accessors (CORE-004)** — Read-only accessors and idempotent
  Destroy() for all resource types.

- **cmd/wgpu-triangle** — Single-threaded wgpu API triangle example.

- **cmd/wgpu-triangle-mt** — Multi-threaded wgpu API triangle example.

### Changed

- **Updated naga v0.14.6 → v0.14.7** — Fixes MSL sequential per-type binding
  indices across bind groups.

## [0.20.2] - 2026-03-12

### Fixed

- **Vulkan: validate WSI query functions in LoadInstance** — `vkGetPhysicalDevice-
  SurfaceCapabilitiesKHR`, `vkGetPhysicalDeviceSurfaceFormatsKHR`, and
  `vkGetPhysicalDeviceSurfacePresentModesKHR` are now verified during instance
  initialization. Previously, if any WSI function failed to load (returned nil),
  the error was silent until a later SIGSEGV via goffi nil function pointer call.
  Now fails fast with a clear error message.

## [0.20.1] - 2026-03-11

### Fixed

- **Metal: missing stencil attachment in render pass** — `BeginRenderPass` configured
  only the depth attachment, completely skipping the stencil attachment. On Apple Silicon
  TBDR GPUs, this left the stencil load action as `MTLLoadActionDontCare`, causing
  undefined stencil values and progressive rendering artifacts on Retina displays.
  Now configures `rpDesc.stencilAttachment` with texture, load/store actions, and clear
  value — matching the Vulkan and DX12 backends.
  ([#171](https://github.com/gogpu/gg/issues/171))

- **Metal: missing `setClearDepth:` call** — depth clear value was never explicitly set,
  relying on Metal's default of 1.0. Now calls `setClearDepth:` when `DepthLoadOp` is
  `LoadOpClear` for correctness.

## [0.20.0] - 2026-03-10

### Added

- **Core validation layer** (VAL-002) — exhaustive spec-level validation before
  HAL calls. 7 validation functions in `core/validate.go` covering 30+ WebGPU
  rules for textures, samplers, shaders, pipelines, bind groups, and bind group
  layouts. Validates dimensions, limits, multisampling, formats, and usage flags.

- **Typed error types** (VAL-002) — 7 new typed errors with specific error kinds
  and context fields: `CreateTextureError` (13 kinds), `CreateSamplerError` (5),
  `CreateShaderModuleError` (3), `CreateRenderPipelineError` (8),
  `CreateComputePipelineError` (3), `CreateBindGroupLayoutError` (3),
  `CreateBindGroupError` (2). All support `errors.As()` for programmatic handling.

- **Deferred nil error detection** (VAL-003) — 10 pass encoder and command encoder
  methods that previously silently ignored nil inputs now record deferred errors
  following the WebGPU spec pattern. Errors surface at `End()` / `Finish()`:
  `RenderPass.SetPipeline`, `SetBindGroup`, `SetVertexBuffer`, `SetIndexBuffer`,
  `DrawIndirect`, `DrawIndexedIndirect`, `ComputePass.SetPipeline`, `SetBindGroup`,
  `DispatchIndirect`, `CommandEncoder.CopyBufferToBuffer`.

- **Format conversion tests** (COV-001) — 26 new test functions across Metal (20),
  Vulkan (4), DX12 (2), and GLES (5 format cases) backends.

### Fixed

- **5 nil panic paths** (VAL-001) — added nil checks in `CreateBindGroup` (nil layout),
  `CreatePipelineLayout` (nil bind group layout element), `Queue.Submit` (nil command
  buffer), `Surface.Configure` (nil device), `Surface.Present` (nil texture).

- **Metal: CopyDst buffer storage mode** — buffers with `CopyDst` usage were
  allocated with `StorageModePrivate` (GPU-only), causing "buffer not mappable"
  errors on Apple Silicon when `Queue.WriteBuffer()` tried to write. Now uses
  `StorageModeShared` for `CopyDst` and `MappedAtCreation` buffers, matching
  the Vulkan backend behavior. On UMA (all Apple Silicon) this is zero-cost.
  ([gg#170](https://github.com/gogpu/gg/issues/170))

- **Metal: staging buffer fallback for ReadBuffer/WriteBuffer** — defense-in-depth:
  if a buffer is `StorageModePrivate`, `WriteBuffer` and `ReadBuffer` now fall
  back to a temporary staging buffer + blit instead of failing. Mirrors the
  pattern already used by `WriteTexture` and matches Rust wgpu behavior.

- **Metal: zero-length data guard** — `WriteBuffer` and `ReadBuffer` now return
  early for empty data slices, preventing a potential panic in the staging path.

### Changed

- **HAL defense-in-depth** (VAL-004) — HAL nil checks now use `"BUG: ..."` prefix
  to signal core validation gaps. Removed 6 redundant spec checks (buffer size,
  texture dimensions) from Vulkan, Metal, DX12 — core validates these. Added 9
  missing nil checks to GLES, Software, and Noop backends.

### Dependencies

- **gputypes v0.2.0 → v0.3.0** — `TextureUsage.ContainsUnknownBits()` method,
  used by core validation for texture descriptor validation (VAL-002).

## [0.19.7] - 2026-03-07

### Added

- **Queue.WriteTexture** — public API for writing data to textures. Includes
  `ImageCopyTexture` descriptor, `ImageDataLayout` alias, and full nil validation
  with specific error messages.
  ([#95](https://github.com/gogpu/wgpu/pull/95) by [@Carmen-Shannon](https://github.com/Carmen-Shannon))

### Changed

- **Update naga v0.14.5 → v0.14.6** — MSL pass-through globals fix: helper
  functions now receive texture/sampler as extra parameters instead of using
  `[[binding]]` attributes. Fixes black screen on M3 Mac.
  ([naga#40](https://github.com/gogpu/naga/pull/40))

## [0.19.6] - 2026-03-05

### Fixed

- **Metal: MSAA resolve store action** — when a render pass has a resolve target
  (MSAA → single-sample), Metal requires `MTLStoreActionMultisampleResolve` or
  `MTLStoreActionStoreAndMultisampleResolve`. We were setting `MTLStoreActionStore`,
  causing Metal to silently skip the resolve. The surface texture stayed
  uninitialized (purple/magenta screen).
  ([ui#23](https://github.com/gogpu/ui/issues/23))

## [0.19.5] - 2026-03-05

### Fixed

- **Metal: add vertex descriptor to render pipeline creation** — Metal requires
  an explicit `MTLVertexDescriptor` when the vertex function has input attributes.
  Without it, pipeline creation fails with "Vertex function has input attributes
  but no vertex descriptor was set." Added `buildVertexDescriptor()` that maps
  WebGPU `VertexBufferLayout` to Metal vertex attributes and buffer layouts.
  ([ui#23](https://github.com/gogpu/ui/issues/23))

### Added

- **Complete Metal vertex format mapping** — all WebGPU vertex formats (8/16/32-bit
  int/uint/float, normalized, packed 10-10-10-2) now map to corresponding
  `MTLVertexFormat` constants.

### Changed

- **Update goffi v0.4.1 → v0.4.2**
- **Update naga v0.14.4 → v0.14.5**

## [0.19.4] - 2026-03-02

### Changed

- **Update goffi v0.3.9 → v0.4.1** — fix SIGSEGV on Linux/macOS for Vulkan
  functions with >6 arguments (`vkCmdPipelineBarrier`, etc.)
  ([goffi#19](https://github.com/go-webgpu/goffi/issues/19),
  [gogpu#119](https://github.com/gogpu/gogpu/issues/119))

## [0.19.3] - 2026-03-01

### Changed

- **Update naga v0.14.3 → v0.14.4** — MSL backend fixes: vertex `[[stage_in]]`
  for struct-typed arguments, `metal::discard_fragment()` namespace prefix
  ([naga#38](https://github.com/gogpu/naga/pull/38),
  [ui#23](https://github.com/gogpu/ui/issues/23))

## [0.19.2] - 2026-03-01

### Fixed

- **Metal: SIGBUS crash on Apple Silicon from ObjC block PAC re-signing** —
  ObjC blocks were constructed with `_NSConcreteStackBlock` but allocated on the
  Go heap. When Metal calls `Block_copy()` during `addCompletedHandler:`, ARM64e
  Pointer Authentication (PAC) re-signs the invoke function pointer. Since
  `ffi.NewCallback` pointers are unsigned, authentication fails and produces a
  corrupted pointer that causes SIGBUS ~0.7s after launch when Metal's completion
  queue invokes the callback. Fixed by switching to `_NSConcreteGlobalBlock` with
  `BLOCK_IS_GLOBAL` flag, which makes `Block_copy()` a complete no-op (no memmove,
  no PAC re-signing). Added `blockPinRegistry` to prevent GC collection of block
  literals while Metal holds references. Removed stale `runtime.KeepAlive(uintptr)`
  calls that were no-ops (GC doesn't track `uintptr` as roots).
  ([wgpu#89](https://github.com/gogpu/wgpu/issues/89),
  [ui#23](https://github.com/gogpu/ui/issues/23))

### Changed

- **CI: upgraded codecov-action v4 → v5**, added `codecov.yml` configuration
- **Tests: added coverage tests** for core, HAL backends, and format conversion

## [0.19.1] - 2026-03-01

### Fixed

- **Metal: crash on Apple Silicon (M1/M2/M3/M4) with depth/stencil textures** —
  `Depth24PlusStencil8` was hardcoded to `MTLPixelFormatDepth24UnormStencil8` (255),
  which is unsupported on Apple Silicon GPUs (only available on legacy AMD GPUs in
  Intel-era Macs). Metal rejected the invalid pixel format with SIGABRT. Additionally,
  `Depth24Plus` was completely missing from the format mapping, returning
  `MTLPixelFormatInvalid` (0). Fixed by detecting device capability via
  `isDepth24Stencil8PixelFormatSupported` at adapter enumeration and choosing
  `Depth32Float`/`Depth32FloatStencil8` (universally supported) when Depth24 is
  unavailable. Follows the same pattern as wgpu-rs (`wgpu-hal/src/metal/adapter.rs`).
  ([ui#23](https://github.com/gogpu/ui/issues/23))

## [0.19.0] - 2026-03-01

### Changed

- **BREAKING: `hal.Queue.WriteBuffer` now returns `error`** — previously a silent void method
  that could swallow errors from all backends (Vulkan `FlushMappedMemoryRanges`, Metal/DX12
  buffer mapping, etc.). All 7 backend implementations (vulkan, metal, dx12, gles, gles_linux,
  software, noop) updated. All callers in tests and examples now check errors.
- **BREAKING: `hal.Queue.WriteTexture` now returns `error`** — previously a void method.
  All 7 backend implementations updated with proper error propagation from staging buffer
  allocation, data copy, and submission. Callers updated across the ecosystem.
- **BREAKING: `wgpu.Queue.WriteBuffer` now returns `error`** — public API wrapper updated
  to propagate errors from HAL layer.
- **BREAKING: `wgpu.Queue.WriteTexture` now returns `error`** — public API wrapper updated
  to propagate errors from HAL layer.

### Fixed

- **Vulkan: `WriteTexture` consumes swapchain acquire semaphore** — `WriteTexture` performs
  an internal staging `Submit()` that consumed the swapchain acquire semaphore meant for the
  render pass. This caused `vkQueueSubmit` to fail or produce undefined behavior when the
  render pass subsequently tried to use the already-consumed semaphore. Fixed by saving and
  restoring `activeSwapchain`/`acquireUsed` state around the staging submit, protected by mutex.
  ([gogpu#119](https://github.com/gogpu/gogpu/issues/119))
- **Vulkan: `VK_ERROR_DEVICE_LOST` masked by void `WriteTexture`** — Vulkan staging submit
  errors were silently discarded because `WriteTexture` returned void. Now all Vulkan errors
  (buffer mapping, memory flush, queue submit) propagate to the caller.
- **Vulkan: `CmdSetBlendConstants` codegen regression** — auto-generated binding used scalar
  float signature instead of pointer-to-float-array. Vulkan ABI expects `const float[4]` as
  pointer, not scalar. Caused SIGSEGV in `BeginRenderPass` for any application using blend.
- **Noop: `WriteBuffer` rejects non-mapped buffers** — noop `CreateBuffer` returns `*Resource`
  (not `*Buffer`) for non-mapped buffers. `WriteBuffer` type assertion now handles both types.

## [0.18.1] - 2026-02-27

### Fixed

- **Vulkan: buffer-to-image copy row stride corruption** — `convertBufferImageCopyRegions` incorrectly
  inferred `bytesPerTexel` via integer division `BytesPerRow / Width` instead of using the texture
  format's known block size. When `BytesPerRow` was padded to 256-byte alignment, the division
  produced wrong results for most image widths (126 out of 204 possible widths for RGBA8).
  For example, width=204: `1024 / 204 = 5` (should be 4) → Vulkan received wrong `bufferRowLength`
  → pixel corruption on rounded rectangles and other non-power-of-2 width textures.
  Fixed by adding `blockCopySize()` static lookup matching the Rust wgpu reference implementation's
  `TextureFormat::block_copy_size()`. Covers all non-compressed WebGPU texture formats.
  ([gogpu#96](https://github.com/gogpu/gogpu/discussions/96))

## [0.18.0] - 2026-02-27

### Added

- **Public API root package** — `import "github.com/gogpu/wgpu"` provides a safe, ergonomic,
  WebGPU-spec-aligned API for third-party applications. Wraps `core/` and `hal/` into 20 public
  types: Instance, Adapter, Device, Queue, Buffer, Texture, TextureView, Sampler, ShaderModule,
  BindGroupLayout, PipelineLayout, BindGroup, RenderPipeline, ComputePipeline, CommandEncoder,
  CommandBuffer, RenderPassEncoder, ComputePassEncoder, Surface, SurfaceTexture.
  - `wgpu.CreateInstance()` → `instance.RequestAdapter()` → `adapter.RequestDevice()` flow
  - All `Create*` methods on Device with `(T, error)` returns
  - Synchronous `Queue.Submit()` with internal fence management
  - `Queue.WriteBuffer()` / `Queue.ReadBuffer()` for CPU↔GPU data transfer
  - Type aliases re-exported from `gputypes` (no need to import `gputypes` directly)
  - Deterministic cleanup via `Release()` on all resource types
  - Backend registration via blank import (`_ "github.com/gogpu/wgpu/hal/allbackends"`)
  - Full command recording: `RenderPassEncoder.SetPipeline/SetBindGroup`,
    `ComputePassEncoder.SetPipeline/SetBindGroup`, `CommandEncoder.CopyBufferToBuffer`
    delegate to HAL via new `RawPass()` / `RawEncoder()` core accessors
  - Examples rewritten to use public API (`examples/compute-copy/`, `examples/compute-sum/`)
  - Integration tests with software backend (15 tests covering full Instance→Submit flow)
  - `core/instance.go`: software backend now enumerated as real adapter (noop still skipped)

## [0.17.1] - 2026-02-27

### Fixed

- **Metal: MSAA texture view crash** — `CreateTextureView` crashed on Apple Silicon (M3) when
  creating a `TextureViewDimension2D` view from a multisampled (4x MSAA) source texture. Metal
  requires the view type to match the source texture's multisample state
  (`MTLTextureType2DMultisample`), unlike Vulkan which handles this implicitly.
  ([ui#23](https://github.com/gogpu/ui/issues/23), [#80](https://github.com/gogpu/wgpu/issues/80))

## [0.17.0] - 2026-02-27

### Added

- **Wayland Vulkan surface creation** — `CreateWaylandSurface()` method on Vulkan API for creating
  surfaces from `wl_display*` and `wl_surface*` C pointers via `VK_KHR_wayland_surface` extension.
  Function pointer `vkCreateWaylandSurfaceKHR` loaded via `vkGetInstanceProcAddr`, following the
  same pattern as X11, XCB, and Metal surface creation.

## [0.16.17] - 2026-02-26

### Fixed

- **Vulkan: load platform surface creation functions** — `vkCreateXlibSurfaceKHR`,
  `vkCreateXcbSurfaceKHR`, `vkCreateWaylandSurfaceKHR`, and `vkCreateMetalSurfaceEXT` were never
  loaded via `GetInstanceProcAddr` — only `vkCreateWin32SurfaceKHR` was. On Linux/macOS the
  function pointer stayed nil, and goffi FFI returned zeros (result=0, surface=0x0) instead of
  crashing, causing "null surface" errors downstream.
  ([gogpu#106](https://github.com/gogpu/gogpu/issues/106))

## [0.16.16] - 2026-02-25

### Fixed

- **Vulkan: X11/macOS surface creation pointer bug** — `CreateSurface` passed the Go stack address
  of the `display` parameter (`unsafe.Pointer(&display)`) instead of the actual `Display*` value
  (`unsafe.Pointer(display)`). This caused `vkCreateXlibSurfaceKHR` to receive a Go stack pointer
  instead of the real Xlib `Display*`, resulting in null surfaces or SIGSEGV. Same fix applied to
  macOS `CAMetalLayer*` in the Vulkan-on-MoltenVK path.
  ([gogpu#106](https://github.com/gogpu/gogpu/issues/106))

## [0.16.15] - 2026-02-25

### Changed

- **Software backend: always compiled** — removed `//go:build software` build tags from all 34 files
  in `hal/software/`, `hal/software/raster/`, and `hal/software/shader/`. The software backend is now
  always available without `-tags software`. Pure Go, zero system dependencies — ideal for CI/CD,
  headless rendering, and fallback when no GPU is available.
  ([gogpu#106](https://github.com/gogpu/gogpu/issues/106))

### Fixed

- **Software backend: nestif complexity** — extracted `clearDepthStencilAttachment()` helper in
  `RenderPassEncoder.End()` to reduce nesting depth (pre-existing issue exposed by build tag removal).

## [0.16.14] - 2026-02-25

### Fixed

- **Vulkan: null surface handle guard** — `EnumerateAdapters`, `SurfaceCapabilities`, and
  `createSwapchain` now check for null `VkSurfaceKHR` handle before calling Vulkan surface
  functions. Prevents SIGSEGV on Linux when surface creation fails (e.g., X11 connection issues).
  ([gogpu#106](https://github.com/gogpu/gogpu/issues/106))

### Changed

- **Dependencies:** naga v0.14.2 → v0.14.3 (5 SPIR-V compute shader bug fixes)

## [0.16.13] - 2026-02-24

### Fixed

- **Vulkan: load VK_EXT_debug_utils via GetInstanceProcAddr** — `vkSetDebugUtilsObjectNameEXT`
  was loaded via `GetDeviceProcAddr`, which bypasses the validation layer's handle wrapping on
  NVIDIA drivers, causing `VUID-VkDebugUtilsObjectNameInfoEXT-objectType-02590` ("Invalid
  VkDescriptorPool Object") errors. Now loaded via `GetInstanceProcAddr` as required for instance
  extensions. Also loads `vkCreateDebugUtilsMessengerEXT` and `vkDestroyDebugUtilsMessengerEXT`
  which were previously missing — debug messenger callback now works correctly.
  ([gogpu#98](https://github.com/gogpu/gogpu/issues/98))

## [0.16.12] - 2026-02-23

### Fixed

- **Vulkan: debug object naming** (VK-VAL-002) — added `setObjectName` helper that calls
  `vkSetDebugUtilsObjectNameEXT` after every Vulkan object creation. Labels buffers, textures,
  pipelines, render passes, framebuffers, descriptor pools, swapchain images, semaphores, and
  more with human-readable names. Eliminates false-positive `VUID-VkDebugUtilsObjectNameInfoEXT-objectType-02590`
  validation errors on NVIDIA where the validation layer's handle tracking lost sync with packed
  non-dispatchable handles. No-op when `VK_EXT_debug_utils` is unavailable. Resources display
  named labels in RenderDoc/Nsight captures.
  ([gogpu#98](https://github.com/gogpu/gogpu/issues/98))

## [0.16.11] - 2026-02-23

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

### Changed

- **Public examples moved to `examples/`** — `compute-copy` and `compute-sum` moved from `cmd/` to
  `examples/` following Go project layout conventions. `cmd/` retains internal tools (vk-gen, backend tests).

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
[Unreleased]: https://github.com/gogpu/wgpu/compare/v0.17.0...HEAD
[0.17.0]: https://github.com/gogpu/wgpu/compare/v0.16.17...v0.17.0
[0.16.14]: https://github.com/gogpu/wgpu/compare/v0.16.13...v0.16.14
[0.16.13]: https://github.com/gogpu/wgpu/compare/v0.16.12...v0.16.13
[0.16.12]: https://github.com/gogpu/wgpu/compare/v0.16.11...v0.16.12
[0.16.11]: https://github.com/gogpu/wgpu/compare/v0.16.10...v0.16.11
[0.16.10]: https://github.com/gogpu/wgpu/compare/v0.16.9...v0.16.10
[0.16.9]: https://github.com/gogpu/wgpu/compare/v0.16.8...v0.16.9
[0.16.8]: https://github.com/gogpu/wgpu/compare/v0.16.7...v0.16.8
[0.16.7]: https://github.com/gogpu/wgpu/compare/v0.16.6...v0.16.7
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
