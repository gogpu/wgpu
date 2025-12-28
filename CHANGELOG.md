# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[Unreleased]: https://github.com/gogpu/wgpu/compare/v0.8.3...HEAD
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
