# wgpu Roadmap

> Pure Go WebGPU Implementation — Development Roadmap

---

## Current Status: v0.9.0

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

### v0.7.1 — ErrZeroArea Validation
- [x] Added `ErrZeroArea` sentinel error matching wgpu-core pattern
- [x] All `Surface.Configure()` validate dimensions before configuring
- [x] Affected backends: Metal, Vulkan, GLES, Software
- [x] Unit tests for zero-dimension handling

### v0.8.0 — DirectX 12 Backend
- [x] DX12 bindings via syscall (Pure Go COM, no CGO!)
- [x] Device and adapter enumeration via DXGI
- [x] Command list and allocator management
- [x] Root signature and PSO creation
- [x] Descriptor heaps (CBV/SRV/UAV, Sampler, RTV, DSV)
- [x] Flip model swapchain with tearing support
- [x] Full format conversion (WebGPU → DXGI)

### v0.8.1 — DX12 & Vulkan Fixes
- [x] DX12 COM calling convention fix for Intel GPUs
- [x] Vulkan goffi argument passing fix (Windows crash)
- [x] Compute shader support (Phase 2 — Core API)

### v0.8.2 — Naga Update
- [x] Updated naga v0.6.0 → v0.8.0 (HLSL backend, SPIR-V fixes)

### v0.8.3 — Metal macOS Fixes
- [x] Metal present timing: schedule `presentDrawable:` before `commit`
- [x] TextureView NSRange parameter fix

### v0.8.4 — Naga Clamp Fix
- [x] Updated naga v0.8.0 → v0.8.1 (clamp() built-in function fix)

### v0.8.5 — DX12 Backend Registration
- [x] DX12 backend auto-registers on Windows via `hal/dx12/init.go`
- [x] Windows backend priority: Vulkan → DX12 → GLES → Software
- [x] All 5 HAL backends now fully integrated

### v0.8.6 — Metal ARM64 goffi Fix
- [x] Updated goffi v0.3.5 → v0.3.6 (ARM64 HFA returns)
- [x] Fixed Metal double present issue in Queue.Present()

### v0.8.7 — Metal ARM64 ObjC Fixes
- [x] Typed ObjC argument wrappers for ARM64 AAPCS64 ABI
- [x] Metal ObjC tests and surface tests
- [x] Updated goffi v0.3.6 → v0.3.7
- [x] Updated naga v0.8.1 → v0.8.2

### v0.8.8 — CI Fixes & MSL Position Fix
- [x] Skip Metal tests on CI (Metal unavailable in virtualized macOS)
- [x] Updated naga v0.8.2 → v0.8.3 (MSL `[[position]]` attribute fix)

### v0.9.0 — Core-HAL Bridge (Current)
- [x] **Snatchable Pattern** — Safe deferred resource destruction
- [x] **TrackerIndex Allocator** — Dense index allocation for state tracking
- [x] **Buffer State Tracker** — Buffer usage state validation
- [x] **Core Device + HAL** — `NewDevice()` with HAL backend integration
- [x] **Core Buffer + HAL** — `Device.CreateBuffer()` GPU-backed buffers
- [x] **Core CommandEncoder** — Command recording with HAL dispatch
- [x] **Code Quality** — 58 TODO comments replaced with proper documentation

---

## Upcoming Releases

### v0.9.0 — Compute Shaders
**Target: Q1 2025**

- [ ] Compute pipeline support in all backends
- [ ] Dispatch and indirect dispatch
- [ ] Storage buffers and images
- [ ] Atomic operations
- [ ] Workgroup shared memory

### v0.10.0 — WebAssembly Support
**Target: Q2 2025**

- [ ] WASM build target
- [ ] Browser WebGPU API bindings
- [ ] Shader pre-compilation for web
- [ ] Examples running in browser

### v1.0.0 — Production Ready
**Target: Q3 2025**

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
1. Compute shader support
2. WebAssembly support
3. Documentation and examples
4. Performance optimization
5. WebGPU specification compliance

---

## Version History

| Version | Date | Highlights |
|---------|------|------------|
| v0.8.4 | 2025-12 | Naga v0.8.1 (clamp() fix) |
| v0.8.3 | 2025-12 | Metal macOS blank window fix |
| v0.8.2 | 2025-12 | Naga v0.8.0 (HLSL backend) |
| v0.8.1 | 2025-12 | DX12/Vulkan fixes, compute API |
| v0.8.0 | 2025-12 | DirectX 12 backend, all 5 HAL backends complete |
| v0.7.0 | 2025-12 | Metal shader pipeline (WGSL→MSL) |
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
