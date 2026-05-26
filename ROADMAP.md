# wgpu Roadmap

> **Pure Go WebGPU Implementation**
>
> All 5 HAL backends: Vulkan, Metal, DX12, GLES, Software. Zero CGO.

---

## Vision

**wgpu** is a complete WebGPU implementation in Pure Go. No CGO required — single binary deployment on all platforms.

### Core Principles

1. **Pure Go** — No CGO, FFI via goffi library
2. **Multi-Backend** — Vulkan, Metal, DX12, GLES, Software
3. **WebGPU Spec** — Follow W3C WebGPU specification
4. **Production-Ready** — Tested on Intel, NVIDIA, AMD, Apple

---

## Current State: v0.28.10

✅ **All 5 HAL backends complete** (~127K LOC)
✅ **Three-layer WebGPU stack** — wgpu API → wgpu/core → wgpu/hal
✅ **Complete public API** — consumers never import `wgpu/hal`
✅ **Core validation layer** — 15/17 Rust wgpu-core checks
✅ **Text rendering on all 3 GPU backends** — Vulkan, DX12, GLES
✅ **DX12 TDR fixed** — deferred resource destruction + DRED diagnostics
✅ **PendingWrites batching** — Rust wgpu-core pattern for WriteBuffer/WriteTexture
✅ **Enterprise fence architecture** — HAL owns fences, SubmissionIndex tracking
✅ **Deferred resource destruction** — ResourceRef (Go Arc) + DestroyQueue (Rust LifetimeTracker). Buffer onZero callback (Rust Drop parity) — refcount-driven HAL destruction, no stale submission index risk.
✅ **Per-command-buffer resource tracking** — Clone/Drop in encoders (Rust EncoderInFlight)
✅ **DX12 HLSL shader cache** — in-memory SHA-256 keyed, LRU eviction
✅ **DX12 DRED diagnostics** — auto-breadcrumbs + page fault tracking on TDR
✅ **DX12 DXIL direct compilation** — naga DXIL backend, SM 6.0+, zero external dependencies, first Pure Go DXIL generator. Full 2D rendering (text, SDF shapes, widgets) verified on Intel Iris Xe.
✅ **Blend constant draw-time validation** — Rust wgpu-core OptionalState pattern
✅ **Vulkan fence pool recycling** — matches Rust wgpu-hal maintain() before submit
✅ **WebGPU Buffer mapping API** — `Buffer.Map`/`MapAsync`/`MappedRange`/`Unmap`, `Device.Poll`
✅ **GLES Y-flip fix** — swapchain offscreen FBO + Present blit (Rust wgpu-hal parity)
✅ **Encoder pool** — per-Device shared pool (Rust CommandAllocator pattern)
✅ **Software DWM-safe presentation** — CreateDIBSection+BitBlt (SDL3/Qt6 pattern)
✅ **Software Linux X11 presentation** — XPutImage via goffi (Skia pattern)
✅ **Vulkan maxMemoryAllocationSize enforcement** — prevents SIGSEGV on large writes
✅ **Staging belt auto-chunking** — writes >64MB automatically split (Rust wgpu parity)
✅ **Late buffer binding validation** — draw/dispatch-time MinBindingSize=0 checks (Rust parity)
✅ **Compute dispatch barriers** — per-dispatch memory barriers on all GPU backends (Rust parity)
✅ **Dispatch workgroup validation** — count + size limits checked before dispatch
✅ **Validation Phase A — crash prevention** — 18 P0 checks (WriteBuffer bounds, BindGroup buffer validation, draw-time state with typed sentinel errors, PipelineLayout count, format type guards, Queue.Submit resource state)
✅ **Validation Phase B — correctness** — MinBindingSize early check, index buffer format mismatch, indirect buffer bounds overrun, depth/stencil aspect granularity, BindGroup destruction at Submit. Coverage 22% → ~45% of Rust wgpu-core.
✅ **DX12 buffer state tracking** — per-buffer D3D12_RESOURCE_STATES with automatic transition barriers (Rust BufferTracker pattern)
✅ **Pipeline constants passthrough** — Constants map flows from public API through HAL
✅ **Zero-init workgroup memory** — WebGPU spec default, plumbed through all layers
✅ **CopyTextureToTexture public API** — DMA hardware copy with sub-region support
✅ **Vulkan relay semaphores** — GPU-side submission ordering (Mesa ANV workaround)
✅ **WASM platform split** — root package _native.go/_browser.go, core/hal excluded from WASM build
✅ **Vulkan command buffer free list** — batch alloc 16 CBs, pool reset (Khronos/NVIDIA/ARM/Mesa/Rust parity)
✅ **Damage-aware surface presentation** — `PresentWithDamage()` with compositor dirty rects. First WebGPU implementation. Software + Vulkan + DX12 + GLES backends.
✅ **Automatic resource lifecycle** — `runtime.AddCleanup` for Buffer/BindGroup (ADR-018, Rust Arc+Drop pattern). GC safety net prevents per-frame resource leaks.
✅ **Zero-allocation WriteBuffer batching** — pre-allocated BufferCopy + stack barrier arrays. All PendingWrites hot paths 0 allocs/op.
✅ **Full SPIR-V interpreter** — 7 phases (~10K LOC): vertex/fragment/compute on CPU, texture sampling, GLSL.std.450 intrinsics, control flow, atomics, workgroup shared memory. Shader debugger with breakpoints and JSON trace. For debugging/CI, not production.
✅ **DX12 timestamp queries** — CreateQuerySet, EndQuery, ResolveQueryData (Rust wgpu-hal parity)
✅ **Queue thread safety** — Submit/WriteBuffer/WriteTexture serialized via sync.Mutex (Rust wgpu-core parity)
✅ **GLES compute memory barriers** — glMemoryBarrier for storage→draw/dispatch transitions (Rust parity)
✅ **Software render pass instrumentation** — slog debug events + RenderPassStats for CI e2e assertions
✅ **Browser WebGPU backend** — complete `syscall/js` → `navigator.gpu` implementation (~6500 LOC). Instance, Adapter, Device, Resources, Pipelines, Command Recording, Queue Submit, Surface/Canvas, Buffer Mapping. Bypasses core/hal (Rust wgpu pattern). 97 TextureFormats, 31 VertexFormats, 29+ tests. Zero external dependencies.
✅ **GLES hidden window context (Windows)** — GL context owned by Instance on hidden 1×1 HWND, shared via mutex-protected `AdapterContext`. Adapter/Device/Queue survive Surface destruction. Follows Rust wgpu-hal `wgl.rs` `AdapterContext::lock()`/`lock_with_dc()` pattern. Surface lightweight — no context ownership.

### Remaining validation (planned)
- **Phase C** (P2): Spec compliance edge cases, feature gates
- See [ADR-VALIDATION-PHASES.md](docs/dev/research/ADR-VALIDATION-PHASES.md)

| Backend | Platform | Status |
|---------|----------|--------|
| Vulkan | Windows, Linux, macOS | ✅ Stable — text, compute, MSAA |
| Metal | macOS | ✅ Stable — naga MSL 91/91 |
| DX12 | Windows | ✅ Stable — TDR fixed, PendingWrites, deferred destruction |
| GLES | Windows, Linux | ✅ Stable — hidden window context (Rust parity), glFenceSync, copy commands, timestamps, real adapter capabilities, compute barriers |
| Software | Windows, Linux, macOS | ✅ Stable — windowed presentation (GDI/X11/CG+Metal), SPIR-V interpreter. All 3 desktop platforms. |

→ **See [CHANGELOG.md](CHANGELOG.md) for detailed per-version notes**

---

## Upcoming

### Next

- [x] GLES hidden window context (Windows) — Instance-owned GL context, Rust wgpu parity (FEAT-GLES-002)
- [ ] GLES hidden window context (Linux) — EGL surfaceless/pbuffer, Phase 2 of FEAT-GLES-002
- [x] macOS software presentation — CGImage + CAMetalLayer (PR #187, @k-chimi, v0.28.4)
- [ ] DX12 DeviceTextureTracker for proper barrier state tracking
- [ ] GLES global UNPACK_ALIGNMENT=1 (Rust pattern — set once at device open)
- [ ] GetSurfaceCapabilities on all backends (currently Vulkan-only)
- [ ] DXIL as default DX12 shader path (currently opt-in via `GOGPU_DX12_DXIL=1`)

### v1.0.0 — Production Release

- [ ] Full WebGPU specification compliance
- [ ] API stability guarantee
- [x] Compute shader support in all GPU backends (Vulkan, DX12, Metal, GLES)
- [x] Performance benchmarks — 115+ benchmarks, hot-path allocation optimization
- [x] Enterprise fence architecture — HAL owns fences, SubmissionIndex tracking
- [x] PendingWrites batching — Rust wgpu-core pattern
- [x] Public API root package — safe, ergonomic user-facing API
- [x] Text rendering on all GPU backends
- [x] Blend constant tracking + resource usage conflict detection
- [ ] Full render/compute pass validation (resource transitions)
- [x] Late buffer binding size validation (VAL-006, draw/dispatch-time checks)
- [ ] Comprehensive documentation
- [ ] Conformance test suite

### Future — Platform Expansion

- [x] **WebAssembly (browser WebGPU)** — DONE (v0.28.0). `internal/browser/` via `syscall/js` →
  `navigator.gpu` (bypasses core/hal, Rust wgpu `ContextWebGpu` pattern). ~6500 LOC, zero deps.
- [ ] **WebGL2 fallback** — GLES backend `_js.go` files for browsers without WebGPU.
- [ ] **Android** — Vulkan surface via `vkCreateAndroidSurfaceKHR` (S estimate).
  Depends on gogpu platform layer
- [ ] **iOS** — Metal backend ready (naga MSL 91/91), needs platform integration

### Future — Advanced Features

- [ ] Ray tracing extensions
- [ ] Bindless resources

---

## Architecture

```
                    WebGPU API (core/)
                          │
          ┌───────────────┼───────────────┐
          │               │               │
          ▼               ▼               ▼
      Instance        Device           Queue
          │               │               │
          └───────────────┼───────────────┘
                          │
                   HAL Interface
                          │
     ┌──────┬──────┬──────┼──────┬──────┐
     ▼      ▼      ▼      ▼      ▼      ▼
  Vulkan  Metal   DX12   GLES  Software Noop
```

---

## Released Versions

| Version | Date | Highlights |
|---------|------|------------|
| **v0.28.10** | 2026-05 | Core: pre-allocate trackedRefs in pass encoders (ML OOM fix). |
| **v0.28.9** | 2026-05 | Core: refcount-driven Buffer destruction via onZero (Rust Drop parity). Eliminates Phase 1 stale index risk. |
| **v0.28.8** | 2026-05 | Core: Clone buffer ResourceRefs in SetBindGroup — prevents use-after-free (Rust wgpu parity). |
| **v0.28.7** | 2026-05 | Core: deferred GLES enumeration in RequestAdapter (adapter name fix). |
| **v0.28.6** | 2026-05 | **GLES hidden window context** (Rust parity). Instance-owned GL context, AdapterContext mutex, Surface lightweight. Fixes context death on window close. |
| **v0.28.5** | 2026-05 | Metal: defer pool.Drain, drawable count. Core: indirect validation nil guard (#189). |
| **v0.28.4** | 2026-05 | macOS blit (@k-chimi), GLES Rust parity (fence+copies+timestamps+adapter), GPU dispatch indirect validation. |
| **v0.28.0** | 2026-05 | **Browser WebGPU backend** (WASM-001). Complete `syscall/js` → `navigator.gpu`. 6500 LOC, 5 phases, zero deps. First Pure Go WebGPU in the browser. |
| **v0.27.5** | 2026-05 | Defensive NULL handle guard in TransitionTextures/Buffers (Vulkan, DX12, public API). Prevents crash on destroyed resource barriers. |
| **v0.27.4** | 2026-05 | goffi v0.5.1 (struct ABI, XMM return, CGO_ENABLED=1), x/sys v0.44.0, flaky TestThread_CallAsync fix |
| **v0.27.3** | 2026-05 | Software render pass instrumentation (slog + RenderPassStats), Metal MsgSend docs |
| **v0.27.2** | 2026-05 | DX12 timestamp queries, Queue mutex, GLES compute barriers, Vulkan timestampPeriod fix |
| **v0.27.1** | 2026-05 | MSAA resolve LoadOp=CLEAR, Vulkan offscreen ImageLayoutGeneral, persistent stencil, naga v0.17.13 |
| **v0.27.0** | 2026-05 | **Full SPIR-V interpreter** (7 phases, ~10K LOC), shader debugger, compute HAL, particles rendering, tagged union optimization, naga v0.17.11, flaky test fix |
| **v0.26.12** | 2026-05 | **Test coverage** (core 85.5%, root 78.4%), Metal entry point fix (#168 by @k-chimi), naga v0.17.10 |
| **v0.26.11** | 2026-04 | **DX12 indirect dispatch/draw** — ExecuteIndirect + CommandSignature (was last GPU backend with stubs) |
| **v0.26.10** | 2026-04 | **Validation Phase B** — 5 P1 correctness checks (MinBindingSize, index format mismatch, indirect bounds, depth/stencil aspects, BindGroup Submit). Coverage 37% → 45%. |
| **v0.26.9** | 2026-04 | **Validation Phase A** — 18 P0 crash prevention checks (WriteBuffer bounds, BindGroup buffer, draw-time state, PipelineLayout, format guards, Submit resource state). Coverage 22% → 37%. |
| **v0.25.6** | 2026-04 | Vulkan command buffer free list (VK-CMD-001): batch alloc, pool reset, enterprise parity |
| **v0.25.5** | 2026-04 | WASM platform split (Phase 0): _native.go/_browser.go file split, core/hal excluded from WASM |
| **v0.25.4** | 2026-04 | Late buffer binding validation (VAL-006) + Vulkan relay semaphores (VK-SYNC-001) |
| **v0.25.3** | 2026-04 | Fix SIGSEGV on large WriteBuffer (#142): maxMemoryAllocationSize enforcement, staging belt auto-chunking, MappedSize bounds checking |
| **v0.25.2** | 2026-04 | gputypes v0.5.0 (PrimitiveState zero defaults) |
| **v0.25.1** | 2026-04 | Linux X11 software presentation via XPutImage (Skia pattern) |
| **v0.25.0** | 2026-04 | **WebGPU Buffer mapping API**, **DXIL full rendering** (naga v0.17.4), GLES Y-flip fix, sampler heap plumbing, pipeline error logging. Breaking: `Queue.ReadBuffer` removed. |
| **v0.24.7** | 2026-04 | DWM-safe software presentation (CreateDIBSection+BitBlt), rendering optimizations |
| **v0.23.8** | 2026-04 | Metal vertex buffer fix, GLES per-type binding counters, StagingBelt alignment |
| **v0.23.7** | 2026-04 | naga v0.16.4 (HLSL 72/72 parity, 330× faster FXC array init) |
| **v0.23.6** | 2026-04 | Deferred resource destruction, DX12 shader cache, DRED diagnostics |
| **v0.23.5** | 2026-04 | GLES coordinate space, Vulkan fence recycling, blend constant validation |
| **v0.23.4** | 2026-04 | GLES text fix, DX12 TDR (descriptor UAF), StagingBelt |
| **v0.23.3** | 2026-04 | GLES blur fix, enterprise logging system |
| **v0.23.2** | 2026-04 | DX12 sampler heap (Rust pattern), GLES BindingMap |
| **v0.23.1** | 2026-04 | Text/texture rendering on all non-Vulkan backends |
| **v0.23.0** | 2026-03 | Enterprise fence architecture, naga v0.15.0 |
| **v0.22.2** | 2026-03 | Metal per-type slots, GLES scissor, goffi v0.5.0 |
| **v0.22.1** | 2026-03 | Vulkan/GLES/DX12 patches |
| **v0.21.3** | 2026-03 | Validation layer + GLES/DX12/Software fixes |
| **v0.21.0** | 2026-03 | wgpu public API migration |
| **v0.18.0** | 2026-02 | Public API root package (20 types, WebGPU-aligned) |
| v0.17.1 | 2026-02 | Metal MSAA texture view crash fix |
| v0.17.0 | 2026-02 | Wayland Vulkan surface creation |
| **v0.16.16** | 2026-02 | Vulkan X11/macOS surface pointer fix (gogpu#106) |
| v0.16.15 | 2026-02 | Software backend always compiled, no build tags (gogpu#106) |
| v0.16.14 | 2026-02 | Vulkan null surface handle guard (gogpu#106), naga v0.14.3 |
| v0.16.13 | 2026-02 | Vulkan: debug_utils via GetInstanceProcAddr (gogpu#98) |
| v0.16.12 | 2026-02 | Vulkan debug object naming (VK-VAL-002, gogpu#98) |
| v0.16.11 | 2026-02 | Vulkan zero-extent swapchain fix (VK-VAL-001, gogpu#98) |
| v0.16.10 | 2026-02 | Vulkan pre-acquire semaphore wait (VK-IMPL-004) |
| v0.16.6 | 2026-02 | Metal debug logging (23 log points), goffi v0.3.9 |
| v0.16.5 | 2026-02 | Vulkan per-encoder command pools |
| v0.16.4 | 2026-02 | Timeline semaphore, FencePool, batch alloc, hot-path benchmarks |
| v0.16.3 | 2026-02 | Per-frame fence tracking, GLES VSync, WaitIdle interface |
| v0.16.2 | 2026-02 | Metal autorelease pool LIFO fix (macOS Tahoe crash) |
| v0.16.1 | 2026-02 | Vulkan framebuffer cache invalidation fix |
| v0.16.0 | 2026-02 | Full GLES pipeline, structured logging, MSAA, Metal/DX12 features |
| v0.15.1 | 2026-02 | DX12 WriteBuffer/WriteTexture fix, shader pipeline fix |
| v0.15.0 | 2026-02 | ReadBuffer for compute shader readback |
| v0.14.0 | 2026-02 | Leak detection, error scopes, thread safety |
| v0.13.x | 2026-02 | Format capabilities, render bundles, naga v0.11.1 |
| v0.12.0 | 2026-01 | BufferRowLength fix, NativeHandle, WriteBuffer |
| v0.11.x | 2026-01 | gputypes migration, webgpu.h compliance |
| v0.10.x | 2026-01 | HAL integration, multi-thread architecture |
| v0.9.x | 2026-01 | Vulkan fixes (Intel, features, limits) |
| v0.8.x | 2025-12 | DX12 backend, 5 HAL backends complete |
| v0.7.x | 2025-12 | Metal shader pipeline (WGSL→MSL) |
| v0.6.0 | 2025-12 | Metal backend |
| v0.5.0 | 2025-12 | Software rasterization |
| v0.4.0 | 2025-12 | Vulkan + GLES backends |
| v0.1-3 | 2025-10 | Core types, validation, HAL interface |

→ **See [CHANGELOG.md](CHANGELOG.md) for detailed release notes**

---

## Contributing

We welcome contributions! Priority areas:

1. **Compute Shaders** — Full compute pipeline support
2. **WebAssembly** — Browser WebGPU bindings
3. **Mobile** — Android and iOS support
4. **Performance** — Optimization and benchmarks

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

---

## Non-Goals

- **Game engine** — See gogpu/gogpu
- **2D graphics** — See gogpu/gg
- **GUI toolkit** — See gogpu/ui (planned)

---

## License

MIT License — see [LICENSE](LICENSE) for details.
