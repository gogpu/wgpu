# wgpu Roadmap

> Pure Go WebGPU Implementation — Development Roadmap

---

## Current Status: v0.11.2

| Component | Status | LOC | Coverage |
|-----------|--------|-----|----------|
| `gputypes` | ✅ External | — | — |
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

### v0.9.0 — Core-HAL Bridge
- [x] **Snatchable Pattern** — Safe deferred resource destruction
- [x] **TrackerIndex Allocator** — Dense index allocation for state tracking
- [x] **Buffer State Tracker** — Buffer usage state validation
- [x] **Core Device + HAL** — `NewDevice()` with HAL backend integration
- [x] **Core Buffer + HAL** — `Device.CreateBuffer()` GPU-backed buffers
- [x] **Core CommandEncoder** — Command recording with HAL dispatch
- [x] **Code Quality** — 58 TODO comments replaced with proper documentation

### v0.9.1 — Vulkan Backend Fixes
- [x] **vkDestroyDevice Fix** — Fixed memory leak when destroying Vulkan devices ([#32])
  - Device was not properly destroyed due to missing goffi call
  - Now correctly invokes `vkDestroyDevice` via `ffi.CallFunction`
- [x] **Features Mapping** — Implemented `featuresFromPhysicalDevice()` ([#33])
  - Maps 9 Vulkan features to WebGPU features
  - BC, ETC2, ASTC compression, IndirectFirstInstance, MultiDrawIndirect, etc.
- [x] **Limits Mapping** — Implemented proper Vulkan→WebGPU limits ([#34])
  - Maps 25+ hardware limits from `VkPhysicalDeviceLimits`
  - Texture dimensions, descriptor limits, buffer limits, compute limits

[#32]: https://github.com/gogpu/wgpu/issues/32
[#33]: https://github.com/gogpu/wgpu/issues/33
[#34]: https://github.com/gogpu/wgpu/issues/34

### v0.9.2 — Metal NSString Fix
- [x] **NSString Double-Free Fix** — Fixed memory corruption in Metal backend
  - Fixed NSString lifecycle management in ObjC calls

### v0.9.3 — Intel Vulkan Fix
- [x] **VkRenderPass Fix** — Fixed rendering on Intel GPUs
  - Proper VkRenderPass creation with wgpu-style synchronization

### v0.10.0 — HAL Backend Integration
- [x] **Backend Interface** — New abstraction for HAL backend management
- [x] **HAL Backend Integration** — Seamless backend auto-registration
- [x] **Enhanced Instance** — HAL backend support in core.Instance
- [x] **Device Extensions** — HAL device in core.Device
- [x] **Buffer Extensions** — HAL buffer in core.Buffer
- [x] **CommandEncoder Extensions** — HAL command encoding

### v0.10.1 — Vulkan Swapchain Fix
- [x] **Non-blocking swapchain acquire** — Window responsiveness fix
- [x] **ErrNotReady Error** — New error for timeout signaling

### v0.10.2 — FFI Build Tag Fix
- [x] **goffi v0.3.8** — Fixed CGO build tag consistency
- [x] **Clear error message** — `undefined: GOFFI_REQUIRES_CGO_ENABLED_0`
- [x] **Documentation** — Added `CGO_ENABLED=0` requirement to README

### v0.10.3 — Multi-Thread Architecture
- [x] **Thread Package** — Cross-platform thread abstraction for GPU operations
- [x] **Ebiten-style Architecture** — Main thread for Win32, render thread for GPU
- [x] **Responsive Windows** — No "Not Responding" during resize/drag

### v0.11.0 — gputypes Migration
- [x] **Unified WebGPU Types** — Import from `github.com/gogpu/gputypes`
- [x] **Removed `types/` package** — 1,745 lines removed
- [x] **Ecosystem Compatibility** — Single source of truth for types

### v0.11.2 — gputypes v0.2.0 (Current)
- [x] **webgpu.h Compliance** — Update gputypes to v0.2.0 with spec-compliant enum values
- [x] **CompositeAlphaMode Fix** — `PreMultiplied` → `Premultiplied` in all HAL adapters

---

## Upcoming Releases

### v0.12.0 — Compute Shaders
**Target: Q1 2026**

- [ ] Compute pipeline support in all backends
- [ ] Dispatch and indirect dispatch
- [ ] Storage buffers and images
- [ ] Atomic operations
- [ ] Workgroup shared memory

### v0.12.0 — WebAssembly Support
**Target: Q2 2026**

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
| v0.11.2 | 2026-01 | gputypes v0.2.0, webgpu.h spec compliance |
| v0.11.0 | 2026-01 | gputypes migration, types/ removed |
| v0.10.3 | 2026-01 | Multi-thread architecture |
| v0.10.2 | 2026-01 | goffi v0.3.8, CGO build tag fix |
| v0.10.1 | 2026-01 | Vulkan swapchain responsiveness fix |
| v0.10.0 | 2026-01 | HAL Backend Integration layer |
| v0.9.3 | 2026-01 | Intel Vulkan fix: VkRenderPass, wgpu-style sync |
| v0.9.2 | 2026-01 | Metal NSString double-free fix |
| v0.9.1 | 2026-01 | Vulkan backend fixes |
| v0.9.0 | 2026-01 | Major Vulkan improvements |
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
