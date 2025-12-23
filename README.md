<h1 align="center">wgpu</h1>

<p align="center">
  <strong>Pure Go WebGPU Implementation</strong><br>
  No Rust, No CGO, Just Go.
</p>

<p align="center">
  <a href="https://github.com/gogpu/wgpu/actions/workflows/ci.yml"><img src="https://github.com/gogpu/wgpu/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://codecov.io/gh/gogpu/wgpu"><img src="https://codecov.io/gh/gogpu/wgpu/branch/main/graph/badge.svg" alt="codecov"></a>
  <a href="https://pkg.go.dev/github.com/gogpu/wgpu"><img src="https://pkg.go.dev/badge/github.com/gogpu/wgpu.svg" alt="Go Reference"></a>
  <a href="https://goreportcard.com/report/github.com/gogpu/wgpu"><img src="https://goreportcard.com/badge/github.com/gogpu/wgpu" alt="Go Report Card"></a>
  <a href="https://opensource.org/licenses/MIT"><img src="https://img.shields.io/badge/License-MIT-yellow.svg" alt="License"></a>
  <a href="https://github.com/gogpu/wgpu"><img src="https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go" alt="Go Version"></a>
  <a href="https://github.com/gogpu/wgpu"><img src="https://img.shields.io/badge/CGO-none-success" alt="Zero CGO"></a>
</p>

<p align="center">
  <sub>Part of the <a href="https://github.com/gogpu">GoGPU</a> ecosystem</sub>
</p>

> **Status:** v0.6.0 — Metal backend for macOS!

---

## Vision

A complete WebGPU implementation in pure Go:

- **No wgpu-native dependency** — Standalone Go library
- **Direct GPU access** — Vulkan, Metal, DX12 backends
- **WebGPU compliant** — Following the W3C specification
- **WASM compatible** — Run in browsers via WebAssembly

## Installation

```bash
go get github.com/gogpu/wgpu
```

## Usage (Preview)

```go
import (
    "github.com/gogpu/wgpu/core"
    "github.com/gogpu/wgpu/types"
)

// Create instance for GPU discovery
instance := core.NewInstance(&types.InstanceDescriptor{
    Backends: types.BackendsVulkan | types.BackendsMetal,
})

// Request high-performance GPU
adapterID, _ := instance.RequestAdapter(&types.RequestAdapterOptions{
    PowerPreference: types.PowerPreferenceHighPerformance,
})

// Get adapter info
info, _ := core.GetAdapterInfo(adapterID)
fmt.Printf("GPU: %s\n", info.Name)

// Create device
deviceID, _ := core.RequestDevice(adapterID, &types.DeviceDescriptor{
    Label: "My Device",
})

// Get queue for command submission
queueID, _ := core.GetDeviceQueue(deviceID)
```

## Architecture

```
wgpu/
├── types/         # WebGPU type definitions ✓
├── core/          # Validation, state tracking ✓
├── hal/           # Hardware abstraction layer ✓
│   ├── noop/      # No-op backend (testing) ✓
│   ├── software/  # Software backend ✓ (Full rasterizer, ~10K LOC)
│   ├── gles/      # OpenGL ES backend ✓ (Pure Go, ~7500 LOC, Windows + Linux)
│   ├── vulkan/    # Vulkan backend ✓ (Pure Go, ~27K LOC)
│   │   ├── vk/        # Generated Vulkan bindings (~20K LOC)
│   │   └── memory/    # GPU memory allocator (~1.8K LOC)
│   ├── metal/     # Metal backend ✓ (Pure Go, ~3K LOC, macOS)
│   └── dx12/      # DirectX 12 backend (planned)
└── cmd/
    ├── vk-gen/           # Vulkan bindings generator from vk.xml
    └── vulkan-triangle/  # Vulkan integration test (red triangle) ✓
```

## Roadmap

**Phase 1: Types Package** ✓
- [x] Backend types (Vulkan, Metal, DX12, GL)
- [x] Adapter and device types
- [x] Feature flags
- [x] GPU limits with presets
- [x] Texture formats (100+)
- [x] Buffer, sampler, shader types
- [x] Bind group and render state types
- [x] Vertex formats with size calculations

**Phase 2: Core Validation** ✓
- [x] Type-safe ID system with generics
- [x] Epoch-based use-after-free prevention
- [x] Instance, Adapter, Device, Queue management
- [x] Hub with 17 resource registries
- [x] Comprehensive error handling
- [x] 127 tests with 95% coverage

**Phase 3: HAL Interface** ✓
- [x] Backend abstraction layer (Backend, Instance, Adapter, Device, Queue)
- [x] Resource interfaces (Buffer, Texture, Surface, Sampler, etc.)
- [x] Command encoding (CommandEncoder, RenderPassEncoder, ComputePassEncoder)
- [x] Backend registration system
- [x] Noop backend for testing
- [x] 54 tests with 94% coverage

**Phase 4: Pure Go Backends** ✓
- [x] OpenGL ES backend (`hal/gles/`) — Pure Go via goffi, Windows (WGL) + Linux (EGL), ~7.5K LOC
- [x] Vulkan backend (`hal/vulkan/`) — Pure Go via goffi, cross-platform (Windows/Linux/macOS), ~27K LOC
- [x] Software backend (`hal/software/`) — Full rasterization pipeline, ~10K LOC, 100+ tests
- [x] Metal backend (`hal/metal/`) — Pure Go via goffi, macOS, ~3K LOC
- [ ] DX12 backend (`hal/dx12/`) — Windows high-performance (planned)

## Pure Go Approach

All backends implemented without CGO:

| Backend | Status | Approach | Platforms |
|---------|--------|----------|-----------|
| Software | **Done** | Pure Go CPU rendering | All (headless) |
| OpenGL ES | **Done** | goffi + WGL/EGL | Windows, Linux |
| Vulkan | **Done** | goffi + vk-gen from vk.xml | Windows, Linux, macOS |
| Metal | **Done** | goffi (Obj-C bridge) | macOS, iOS |
| DX12 | Planned | goffi + COM | Windows |

### Software Backend

Full-featured CPU rasterizer for headless rendering:

```bash
# Build with software backend
go build -tags software ./...
```

```go
import _ "github.com/gogpu/wgpu/hal/software"

// Use cases:
// - CI/CD testing without GPU
// - Server-side image generation
// - Embedded systems without GPU
// - Fallback when no GPU available
// - Reference implementation for testing

// Key feature: read rendered pixels
surface.GetFramebuffer() // Returns []byte (RGBA8)
```

**Rasterization Pipeline** (`hal/software/raster/`):
- Edge function (Pineda) triangle rasterization with top-left fill rule
- Perspective-correct attribute interpolation
- Depth buffer with 8 compare functions
- Stencil buffer with 8 operations
- 13 blend factors, 5 blend operations (WebGPU spec compliant)
- 6-plane frustum clipping (Sutherland-Hodgman)
- Backface culling (CW/CCW)
- 8x8 tile-based rasterization for cache locality
- Parallel rasterization with worker pool
- Incremental edge evaluation (O(1) per pixel)

**Shader System** (`hal/software/shader/`):
- Callback-based vertex/fragment shaders
- Built-in shaders: SolidColor, VertexColor, Textured
- Custom shader support via `VertexShaderFunc` / `FragmentShaderFunc`

**Metrics**: ~10K LOC, 100+ tests, 94% coverage

### Vulkan Backend Features

- **Auto-generated bindings** from official Vulkan XML specification
- **Memory allocator** with buddy allocation (O(log n), minimal fragmentation)
- **Vulkan 1.3 dynamic rendering** — No render pass objects needed
- **Swapchain management** with automatic recreation
- **Semaphore synchronization** for frame presentation
- **Complete HAL implementation**:
  - Buffer, Texture, TextureView, Sampler
  - ShaderModule, BindGroupLayout, BindGroup
  - PipelineLayout, RenderPipeline, ComputePipeline
  - CommandEncoder, RenderPassEncoder, ComputePassEncoder
  - Fence synchronization, WriteTexture immediate upload
- **Comprehensive unit tests** (93 tests, 2200+ LOC):
  - Conversion functions (formats, usage, blend modes)
  - Descriptor allocator logic
  - Resource structures
  - Memory allocator (buddy allocation)

### Metal Backend Features (New in v0.6.0)

- **Pure Go Objective-C bridge** via goffi
- **Metal API access** via Objective-C runtime
- **Device and adapter enumeration**
- **Command buffer and render encoder**
- **Shader compilation** (MSL via naga v0.5.0)
- **Texture and buffer management**
- **Surface presentation** (CAMetalLayer integration)
- **~3K lines of code**

## References

- [wgpu (Rust)](https://github.com/gfx-rs/wgpu) — Reference implementation
- [WebGPU Specification](https://www.w3.org/TR/webgpu/)
- [Dawn (C++)](https://dawn.googlesource.com/dawn) — Google's implementation

## Related Projects

| Project | Description | Status |
|---------|-------------|--------|
| [gogpu/gogpu](https://github.com/gogpu/gogpu) | Graphics framework | **v0.5.0** |
| [gogpu/naga](https://github.com/gogpu/naga) | Pure Go shader compiler (WGSL → SPIR-V, MSL) | **v0.5.0** |
| [gogpu/gg](https://github.com/gogpu/gg) | 2D graphics with GPU backend, scene graph, SIMD | **v0.9.2** |
| [go-webgpu/webgpu](https://github.com/go-webgpu/webgpu) | FFI bindings (current solution) | Stable |

## Contributing

We welcome contributions! See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

MIT License — see [LICENSE](LICENSE) for details.

---

<p align="center">
  <b>wgpu</b> — WebGPU in Pure Go
</p>
