<h1 align="center">wgpu</h1>

<p align="center">
  <strong>Pure Go WebGPU Implementation</strong><br>
  No Rust. No CGO. Just Go.
</p>

<p align="center">
  <a href="https://github.com/gogpu/wgpu/actions/workflows/ci.yml"><img src="https://github.com/gogpu/wgpu/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://codecov.io/gh/gogpu/wgpu"><img src="https://codecov.io/gh/gogpu/wgpu/branch/main/graph/badge.svg" alt="codecov"></a>
  <a href="https://pkg.go.dev/github.com/gogpu/wgpu"><img src="https://pkg.go.dev/badge/github.com/gogpu/wgpu.svg" alt="Go Reference"></a>
  <a href="https://goreportcard.com/report/github.com/gogpu/wgpu"><img src="https://goreportcard.com/badge/github.com/gogpu/wgpu" alt="Go Report Card"></a>
  <a href="https://opensource.org/licenses/MIT"><img src="https://img.shields.io/badge/License-MIT-yellow.svg" alt="License"></a>
  <a href="https://github.com/gogpu/wgpu/releases"><img src="https://img.shields.io/github/v/release/gogpu/wgpu" alt="Latest Release"></a>
  <a href="https://github.com/gogpu/wgpu"><img src="https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go" alt="Go Version"></a>
  <a href="https://github.com/gogpu/wgpu"><img src="https://img.shields.io/badge/CGO-none-success" alt="Zero CGO"></a>
</p>

<p align="center">
  <sub>Part of the <a href="https://github.com/gogpu">GoGPU</a> ecosystem</sub>
</p>

---

## Overview

**wgpu** is a complete WebGPU implementation written entirely in Go. It provides direct GPU access through multiple hardware abstraction layer (HAL) backends without requiring Rust, CGO, or any external dependencies.

### Key Features

| Category | Capabilities |
|----------|--------------|
| **Backends** | Vulkan, Metal, DirectX 12, OpenGL ES, Software |
| **Platforms** | Windows, Linux, macOS, iOS |
| **API** | WebGPU-compliant (W3C specification) |
| **Shaders** | WGSL via gogpu/naga compiler |
| **Compute** | Full compute shader support, GPU→CPU readback |
| **Debug** | Leak detection, error scopes, validation layers, structured logging (`log/slog`) |
| **Build** | Zero CGO, simple `go build` |

---

## Installation

```bash
go get github.com/gogpu/wgpu
```

**Requirements:** Go 1.25+

**Build:**
```bash
CGO_ENABLED=0 go build
```

> **Note:** wgpu uses Pure Go FFI via `cgo_import_dynamic`, which requires `CGO_ENABLED=0`. This enables zero C compiler dependency and easy cross-compilation.

---

## Quick Start

```go
package main

import (
    "fmt"

    "github.com/gogpu/wgpu"
    _ "github.com/gogpu/wgpu/hal/allbackends" // Auto-register platform backends
)

func main() {
    // Create instance
    instance, _ := wgpu.CreateInstance(nil)
    defer instance.Release()

    // Request high-performance GPU
    adapter, _ := instance.RequestAdapter(&wgpu.RequestAdapterOptions{
        PowerPreference: wgpu.PowerPreferenceHighPerformance,
    })
    defer adapter.Release()

    // Get adapter info
    info := adapter.Info()
    fmt.Printf("GPU: %s (%s)\n", info.Name, info.Backend)

    // Create device
    device, _ := adapter.RequestDevice(nil)
    defer device.Release()

    // Create a GPU buffer
    buffer, _ := device.CreateBuffer(&wgpu.BufferDescriptor{
        Label: "My Buffer",
        Size:  1024,
        Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopyDst,
    })
    defer buffer.Release()

    // Write data to buffer
    if err := device.Queue().WriteBuffer(buffer, 0, []byte{1, 2, 3, 4}); err != nil {
        panic(err)
    }
}
```

### Compute Shaders

```go
// Create shader module from WGSL
shader, _ := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
    Label: "Compute Shader",
    WGSL:  wgslSource,
})
defer shader.Release()

// Create compute pipeline
pipeline, _ := device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
    Label:      "Compute Pipeline",
    Layout:     pipelineLayout,
    Module:     shader,
    EntryPoint: "main",
})
defer pipeline.Release()

// Record and submit commands
encoder, _ := device.CreateCommandEncoder(nil)
pass, _ := encoder.BeginComputePass(nil)
pass.SetPipeline(pipeline)
pass.SetBindGroup(0, bindGroup, nil)
pass.Dispatch(64, 1, 1)
pass.End()

cmdBuffer, _ := encoder.Finish()
device.Queue().Submit(cmdBuffer)
```

**Guides:** [Getting Started](docs/compute-shaders.md) | [Backend Differences](docs/compute-backends.md)

Features: WGSL compute shaders, storage/uniform buffers, indirect dispatch, GPU timestamp queries (Vulkan), GPU-to-CPU readback.

---

## Architecture

```
wgpu/
├── *.go                # Public API (import "github.com/gogpu/wgpu")
├── core/               # Validation, state tracking, resource management
├── hal/                # Hardware Abstraction Layer
│   ├── allbackends/    # Platform-specific backend auto-registration
│   ├── noop/           # No-op backend (testing)
│   ├── software/       # CPU software rasterizer (~11K LOC)
│   ├── gles/           # OpenGL ES 3.0+ (~10K LOC)
│   ├── vulkan/         # Vulkan 1.3 (~38K LOC)
│   ├── metal/          # Metal (~5K LOC)
│   └── dx12/           # DirectX 12 (~14K LOC)
├── examples/
│   ├── compute-copy/   # GPU buffer copy with compute shader
│   └── compute-sum/    # Parallel reduction on GPU
└── cmd/
    ├── vk-gen/         # Vulkan bindings generator
    └── ...             # Backend integration tests
```

### Public API

The root package (`import "github.com/gogpu/wgpu"`) provides a safe, ergonomic API aligned with the W3C WebGPU specification. It wraps `core/` and `hal/` into user-friendly types:

```
User Application
  ↓ import "github.com/gogpu/wgpu"
Root Package (public API)
  ↓ wraps
core/ (validation) + hal/ (backend interfaces)
  ↓
vulkan/ | metal/ | dx12/ | gles/ | software/
```

### HAL Backend Integration

Backends auto-register via blank imports:

```go
import _ "github.com/gogpu/wgpu/hal/allbackends"

// Platform-specific backends auto-registered:
// - Windows: Vulkan, DX12, GLES, Software
// - Linux:   Vulkan, GLES, Software
// - macOS:   Metal, Software
```

---

## Backend Details

### Platform Support

| Backend | Windows | Linux | macOS | iOS | Notes |
|---------|:-------:|:-----:|:-----:|:---:|-------|
| **Vulkan** | Yes | Yes | Yes | - | MoltenVK on macOS |
| **Metal** | - | - | Yes | Yes | Native Apple GPU |
| **DX12** | Yes | - | - | - | Windows 10+ |
| **GLES** | Yes | Yes | - | - | OpenGL ES 3.0+ |
| **Software** | Yes | Yes | Yes | Yes | CPU fallback |

### Vulkan Backend

Full Vulkan 1.3 implementation with:

- Auto-generated bindings from official `vk.xml`
- Buddy allocator for GPU memory (O(log n), minimal fragmentation)
- Dynamic rendering (VK_KHR_dynamic_rendering)
- Classic render pass fallback for Intel compatibility
- wgpu-style swapchain synchronization
- MSAA render pass with automatic resolve
- Complete resource management (Buffer, Texture, Pipeline, BindGroup)
- Surface creation: Win32, X11, Wayland, Metal (MoltenVK)
- Debug messenger for validation layer error capture (`VK_EXT_debug_utils`)
- Structured diagnostic logging via `log/slog`

### Metal Backend

Native Apple GPU access via:

- Pure Go Objective-C bridge (goffi)
- Metal API via runtime message dispatch
- CAMetalLayer integration for surface presentation
- MSL shader compilation via naga

### DirectX 12 Backend

Windows GPU access via:

- Pure Go COM bindings (syscall, no CGO)
- DXGI integration for swapchain and adapters
- Flip model with VRR support
- Descriptor heap management
- WGSL shader compilation (WGSL → HLSL via naga → DXBC via d3dcompiler_47.dll)
- Staging buffer GPU data transfer (WriteBuffer, WriteTexture)

### OpenGL ES Backend

Cross-platform GPU access via OpenGL ES 3.0+:

- Pure Go EGL/GL bindings (goffi)
- Full rendering pipeline: VAO, FBO, MSAA, blend, stencil, depth
- WGSL shader compilation (WGSL → GLSL via naga)
- CopyTextureToBuffer readback for GPU → CPU data transfer
- Platform detection: X11, Wayland, Surfaceless (headless CI)
- Works with Mesa llvmpipe for software-only environments

### Software Backend

Full-featured CPU rasterizer for headless rendering. Always compiled — no build tags or GPU hardware required.

```go
// Software backend auto-registers via init().
// No explicit import needed when using hal/allbackends.
// For standalone usage:
import _ "github.com/gogpu/wgpu/hal/software"

// Use cases:
// - CI/CD testing without GPU
// - Server-side image generation
// - Reference implementation
// - Fallback when GPU unavailable
// - Embedded systems without GPU
```

**Rasterization Features:**
- Edge function triangle rasterization (Pineda algorithm)
- Perspective-correct interpolation
- Depth buffer (8 compare functions)
- Stencil buffer (8 operations)
- Blending (13 factors, 5 operations)
- 6-plane frustum clipping (Sutherland-Hodgman)
- 8x8 tile-based parallel rendering

---

## Ecosystem

**wgpu** is the foundation of the [GoGPU](https://github.com/gogpu) ecosystem.

| Project | Description |
|---------|-------------|
| [gogpu/gogpu](https://github.com/gogpu/gogpu) | GPU framework with windowing and input |
| **gogpu/wgpu** | **Pure Go WebGPU (this repo)** |
| [gogpu/naga](https://github.com/gogpu/naga) | Shader compiler (WGSL to SPIR-V, HLSL, MSL, GLSL) |
| [gogpu/gg](https://github.com/gogpu/gg) | 2D graphics library |
| [go-webgpu/webgpu](https://github.com/go-webgpu/webgpu) | wgpu-native FFI bindings |
| [go-webgpu/goffi](https://github.com/go-webgpu/goffi) | Pure Go FFI library |

---

## Documentation

- **[Compute Shaders Guide](docs/compute-shaders.md)** — Getting started with compute
- **[Compute Backend Differences](docs/compute-backends.md)** — Per-backend capabilities
- **[ARCHITECTURE.md](docs/architecture.md)** — System architecture
- **[ROADMAP.md](ROADMAP.md)** — Development milestones
- **[CHANGELOG.md](CHANGELOG.md)** — Release notes
- **[CONTRIBUTING.md](CONTRIBUTING.md)** — Contribution guidelines
- **[pkg.go.dev](https://pkg.go.dev/github.com/gogpu/wgpu)** — API reference

---

## References

- [WebGPU Specification](https://www.w3.org/TR/webgpu/) — W3C standard
- [wgpu (Rust)](https://github.com/gfx-rs/wgpu) — Reference implementation
- [Dawn (C++)](https://dawn.googlesource.com/dawn) — Google's implementation

---

## Contributing

Contributions welcome! See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

**Priority areas:**
- Cross-platform testing
- Performance benchmarks
- Documentation improvements
- Bug reports and fixes

---

## License

MIT License — see [LICENSE](LICENSE) for details.

---

<p align="center">
  <strong>wgpu</strong> — WebGPU in Pure Go
</p>
