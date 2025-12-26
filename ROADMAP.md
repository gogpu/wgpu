# wgpu Roadmap

> Pure Go WebGPU Implementation — Development Roadmap

---

## Current Status: v0.8.0

| Component | Status | LOC | Coverage |
|-----------|--------|-----|----------|
| `types/` | ✅ Complete | ~3K | 95% |
| `core/` | ✅ Complete | ~4K | 95% |
| `hal/noop/` | ✅ Complete | ~1K | 90% |
| `hal/software/` | ✅ Complete | ~10K | 94% |
| `hal/gles/` | ✅ Complete | ~7.5K | 80% |
| `hal/vulkan/` | ✅ Complete | ~27K | 85% |
| `hal/metal/` | ✅ Complete | ~3K | — |
| `hal/dx12/` | ✅ Complete | ~12K | — |

**Total: ~67K LOC** — All 5 HAL backends complete!

---

## Completed Milestones

### v0.1.0 — Foundation
- [x] WebGPU type definitions
- [x] Type-safe ID system with generics
- [x] Epoch-based use-after-free prevention

### v0.2.0 — Core Validation
- [x] Instance, Adapter, Device, Queue management
- [x] Hub with 17 resource registries
- [x] Comprehensive error handling

### v0.3.0 — HAL Interface
- [x] Backend abstraction layer
- [x] Resource interfaces
- [x] Command encoding interfaces
- [x] Noop backend for testing

### v0.4.0 — GPU Backends
- [x] OpenGL ES backend (Pure Go, goffi)
- [x] Vulkan backend (Pure Go, vk-gen)
- [x] Cross-platform: Windows, Linux, macOS

### v0.5.0 — Software Rasterization
- [x] Edge function triangle rasterization
- [x] Perspective-correct interpolation
- [x] Depth/stencil buffers
- [x] WebGPU-compliant blending
- [x] Frustum clipping (Sutherland-Hodgman)
- [x] Tile-based parallel rasterization
- [x] Callback-based shader system

### v0.6.0 — Metal Backend
- [x] Metal API bindings via goffi (Objective-C bridge)
- [x] Device enumeration and capabilities
- [x] Command buffer and render encoder
- [x] Texture and buffer management
- [x] Surface presentation (CAMetalLayer)

### v0.7.0 — Metal Shader Pipeline
- [x] WGSL→MSL compilation via naga v0.5.0
- [x] Parse WGSL, lower to IR, generate MSL
- [x] Create MTLLibrary from MSL source
- [x] CreateRenderPipeline implementation (~120 LOC)
- [x] Vertex/fragment function binding
- [x] Color attachment and blending configuration

### v0.7.1 — ErrZeroArea Validation (Current)
- [x] Added `ErrZeroArea` sentinel error matching wgpu-core pattern
- [x] All `Surface.Configure()` validate dimensions before configuring
- [x] Affected backends: Metal, Vulkan, GLES, Software
- [x] Unit tests for zero-dimension handling

---

## Upcoming Releases

### v0.8.0 — DirectX 12 Backend
**Target: Q2 2025**

- [ ] DX12 bindings via goffi (COM interfaces)
- [ ] Device and adapter enumeration
- [ ] Command list and allocator
- [ ] Root signature and PSO
- [ ] Descriptor heaps
- [ ] Shader compilation (DXIL from SPIR-V via naga)

### v0.10.0 — Compute Shaders
**Target: Q3 2025**

- [ ] Compute pipeline support in all backends
- [ ] Dispatch and indirect dispatch
- [ ] Storage buffers and images
- [ ] Atomic operations
- [ ] Workgroup shared memory

### v0.11.0 — WebAssembly Support
**Target: Q4 2025**

- [ ] WASM build target
- [ ] Browser WebGPU API bindings
- [ ] Shader pre-compilation for web
- [ ] Examples running in browser

### v1.0.0 — Production Ready
**Target: Q4 2025**

- [ ] Full WebGPU specification compliance
- [ ] All backends stable
- [ ] Comprehensive documentation
- [ ] Performance benchmarks
- [ ] Migration guide from wgpu-native

---

## Long-term Vision

### WebGPU Compliance
- [ ] Validation layer matching spec behavior
- [ ] Error handling per WebGPU error scopes
- [ ] Limits enforcement per adapter

### Performance
- [ ] SIMD optimizations in software backend
- [ ] GPU memory pooling and caching
- [ ] Pipeline state object caching
- [ ] Multi-threaded command recording

### Ecosystem Integration
- [ ] gogpu/gogpu framework integration
- [ ] gogpu/gg 2D graphics GPU backend
- [ ] Game engine support (Ebitengine, etc.)

### Platform Support
- [ ] Android (OpenGL ES / Vulkan)
- [ ] iOS (Metal)
- [ ] Embedded Linux (EGL)

---

## How to Contribute

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

Priority areas:
1. Metal backend implementation
2. DX12 backend implementation
3. Compute shader support
4. WebAssembly support
5. Documentation and examples

---

## Version History

| Version | Date | Highlights |
|---------|------|------------|
| v0.6.0 | 2025-12 | Metal backend for macOS |
| v0.5.0 | 2025-12 | Software rasterization pipeline |
| v0.4.0 | 2025-12 | OpenGL ES (Linux EGL + Windows WGL) |
| v0.3.0 | 2025-11 | Vulkan backend, HAL interface |
| v0.2.0 | 2025-11 | Core validation layer |
| v0.1.0 | 2025-10 | Initial types package |

---

<p align="center">
  <b>wgpu</b> — WebGPU in Pure Go
</p>
