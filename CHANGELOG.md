# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[Unreleased]: https://github.com/gogpu/wgpu/compare/v0.4.0...HEAD
[0.4.0]: https://github.com/gogpu/wgpu/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/gogpu/wgpu/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/gogpu/wgpu/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/gogpu/wgpu/releases/tag/v0.1.0
