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

## Current State: v0.16.16

✅ **All 5 HAL backends complete** (~80K LOC, ~100K total):

**New in v0.16.16:**
- Vulkan X11/macOS surface creation pointer fix — `unsafe.Pointer(&display)` → `unsafe.Pointer(display)`. Old code passed Go stack address instead of Display*/CAMetalLayer* value (gogpu#106)

**New in v0.16.15:**
- Software backend always compiled — removed `//go:build software` from all 34 files. No build tags required, always available as fallback (gogpu#106)

**New in v0.16.14:**
- Vulkan null surface handle guard — prevents SIGSEGV on Linux when surface creation fails (gogpu#106)
- naga v0.14.3 (5 SPIR-V compute shader bug fixes)

**New in v0.16.13:**
- Vulkan: load VK_EXT_debug_utils via `GetInstanceProcAddr` — fixes "Invalid VkDescriptorPool" validation errors on NVIDIA (gogpu#98)
- Debug messenger callback now works (was missing function pointer loading)

**New in v0.16.12:**
- Vulkan debug object naming (VK-VAL-002) — labels every Vulkan object via `vkSetDebugUtilsObjectNameEXT`, eliminates false-positive validation errors on NVIDIA (gogpu#98)

**New in v0.16.11:**
- Vulkan zero-extent swapchain fix (VK-VAL-001) — config-primary extent, unconditional viewport/scissor (gogpu#98)
- Public examples moved from `cmd/` to `examples/`

**New in v0.16.10:**
- Vulkan pre-acquire semaphore wait (VK-IMPL-004) — fixes `VUID-vkAcquireNextImageKHR-semaphore-01779` (gogpu#98)
- naga v0.14.2 (GLSL GL_ARB_separate_shader_objects fix, golden snapshot tests)

**New in v0.16.6:**
- Metal backend debug logging — 23 new log points across rendering path, callbacks, and lifecycle (gogpu/gogpu#89, go-webgpu/goffi#16)
- goffi v0.3.9

**New in v0.16.5:**
- Vulkan per-encoder command pools — dedicated VkCommandPool per encoder, eliminates VkCommandBuffer crash

**New in v0.16.4:**
- Vulkan timeline semaphore fence — single VkSemaphore replaces binary fence ring buffer (Vulkan 1.2+)
- Vulkan binary fence pool — FencePool with per-submission tracking (Vulkan <1.2 fallback)
- Vulkan command buffer batch allocation — 16 per call, free/used list recycling
- Hot-path allocation reduction — sync.Pool for encoders, stack-allocated ClearValues
- 44+ enterprise hot-path benchmarks with ReportAllocs()
- Compute shader examples, docs, SDF integration test
- naga v0.13.1 (OpArrayLength fix, −32% compiler allocations)

**New in v0.16.3:**
- Per-frame fence tracking — eliminates GPU stalls in Vulkan, DX12, Metal hot paths
- `hal.Device.WaitIdle()` — safe GPU drain before resource destruction
- GLES VSync via `wglSwapIntervalEXT` on Windows (fixes 100% GPU usage)

**New in v0.16.2:**
- Metal autorelease pool LIFO fix — scoped pools instead of stored pools (fixes macOS Tahoe crash, gogpu/gogpu#83)

**New in v0.16.0:**
- Full GLES rendering pipeline — WGSL→GLSL shaders, VAO, FBO, MSAA, blend, stencil
- Structured logging via `log/slog` across all backends (silent by default)
- Vulkan MSAA render pass with automatic resolve
- Metal SetBindGroup, WriteTexture, Fence synchronization
- DX12 CreateBindGroup, staging descriptor heaps, BSOD fix
- Cross-backend stability fixes (DX12, Vulkan, Metal, GLES)

| Backend | Platform | Status |
|---------|----------|--------|
| Vulkan | Windows, Linux, macOS | ✅ Stable |
| Metal | macOS, iOS | ✅ Stable |
| DX12 | Windows | ✅ Stable |
| GLES | Windows, Linux | ✅ Stable |
| Software | All | ✅ Stable |

---

## Upcoming

### v1.0.0 — Production Release
- [ ] Full WebGPU specification compliance
- [ ] Compute shader support in all backends
- [ ] API stability guarantee
- [x] Performance benchmarks — 115+ benchmarks, hot-path allocation optimization
- [x] Vulkan timeline semaphore fence (VK_KHR_timeline_semaphore, Vulkan 1.2 core)
- [x] Vulkan command buffer batch allocation (16 per call, wgpu-hal pattern)
- [x] Vulkan binary fence pool (FencePool with per-submission tracking, Vulkan <1.2 fallback)
- [ ] Comprehensive documentation

### Future
- [ ] WebAssembly support (browser WebGPU)
- [ ] Android (Vulkan/GLES)
- [ ] iOS (Metal)

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
