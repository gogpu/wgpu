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

## Current State: v0.15.1

✅ **All 5 HAL backends complete** (~70K LOC):

**New in v0.15.1:**
- DX12 WriteBuffer/WriteTexture fix — were no-op stubs
- DX12 shader compilation fix (WGSL → HLSL → DXBC)
- D3DCompile shader compiler bindings

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
- [ ] Performance benchmarks
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
| **v0.15.1** | 2026-02 | DX12 WriteBuffer/WriteTexture fix, shader pipeline fix |
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
