# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[Unreleased]: https://github.com/gogpu/wgpu/compare/v0.7.2...HEAD
[0.7.2]: https://github.com/gogpu/wgpu/compare/v0.7.1...v0.7.2
[0.7.1]: https://github.com/gogpu/wgpu/compare/v0.6.1...v0.7.1
[0.6.1]: https://github.com/gogpu/wgpu/compare/v0.6.0...v0.6.1
[0.6.0]: https://github.com/gogpu/wgpu/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/gogpu/wgpu/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/gogpu/wgpu/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/gogpu/wgpu/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/gogpu/wgpu/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/gogpu/wgpu/releases/tag/v0.1.0
