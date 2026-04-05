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

## Current State: v0.23.8

✅ **All 5 HAL backends complete** (~127K LOC)
✅ **Three-layer WebGPU stack** — wgpu API → wgpu/core → wgpu/hal
✅ **Complete public API** — consumers never import `wgpu/hal`
✅ **Core validation layer** — 15/17 Rust wgpu-core checks
✅ **Text rendering on all 3 GPU backends** — Vulkan, DX12, GLES
✅ **DX12 TDR fixed** — deferred resource destruction + DRED diagnostics
✅ **PendingWrites batching** — Rust wgpu-core pattern for WriteBuffer/WriteTexture
✅ **Enterprise fence architecture** — HAL owns fences, SubmissionIndex tracking
✅ **Deferred resource destruction** — ResourceRef (Go Arc) + DestroyQueue (Rust LifetimeTracker)
✅ **Per-command-buffer resource tracking** — Clone/Drop in encoders (Rust EncoderInFlight)
✅ **DX12 HLSL shader cache** — in-memory SHA-256 keyed, LRU eviction
✅ **DX12 DRED diagnostics** — auto-breadcrumbs + page fault tracking on TDR
✅ **Blend constant draw-time validation** — Rust wgpu-core OptionalState pattern
✅ **Vulkan fence pool recycling** — matches Rust wgpu-hal maintain() before submit

### Remaining validation (planned)
- Late buffer binding size (SPIR-V reflection → min binding size)

| Backend | Platform | Status |
|---------|----------|--------|
| Vulkan | Windows, Linux, macOS | ✅ Stable — text, compute, MSAA |
| Metal | macOS | ✅ Stable — naga MSL 91/91 |
| DX12 | Windows | ✅ Stable — TDR fixed, PendingWrites, deferred destruction |
| GLES | Windows, Linux | ✅ Stable — text rendering, SamplerBindMap, texture completeness |
| Software | All | ✅ Partial (SW-002) |

→ **See [CHANGELOG.md](CHANGELOG.md) for detailed per-version notes**

---

## Upcoming

### Next: v0.25.0

- [ ] DX12 DeviceTextureTracker for proper barrier state tracking
- [x] GLES BindingMap refactor → per-type sequential counters (v0.23.8)
- [ ] GLES global UNPACK_ALIGNMENT=1 (Rust pattern — set once at device open)
- [ ] Vulkan relay semaphores for multi-submission ordering (VK-SYNC-001)

### v0.25.0 — WebGPU Compliance

- [x] Blend constant tracking (pipeline blend state → draw-time check)
- [x] Resource usage conflict detection (BufferTracker with UsageConflictError)
- [ ] Full render/compute pass validation (resource transitions)
- [ ] Late buffer binding size validation (SPIR-V reflection → min binding size)
- [ ] GetSurfaceCapabilities on all backends (currently Vulkan-only)

### v1.0.0 — Production Release

- [ ] Full WebGPU specification compliance
- [ ] Compute shader support in all backends (Metal compute pending)
- [ ] API stability guarantee
- [x] Performance benchmarks — 115+ benchmarks, hot-path allocation optimization
- [x] Enterprise fence architecture — HAL owns fences, SubmissionIndex tracking
- [x] PendingWrites batching — Rust wgpu-core pattern
- [x] Public API root package — safe, ergonomic user-facing API
- [x] Text rendering on all GPU backends
- [ ] Comprehensive documentation
- [ ] Conformance test suite

### Future — Platform Expansion

- [ ] **WebAssembly (browser WebGPU)** — ADR approved, research complete. Top-level
  `backend/browser/` via `syscall/js` → `navigator.gpu` (bypasses core/hal, like Rust
  wgpu `ContextWebGpu`). WebGL2 fallback via GLES backend + `_js.go`.
  See `docs/dev/research/ADR-WASM-WEBGPU-ARCHITECTURE.md`
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
