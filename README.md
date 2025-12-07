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

> **Status:** v0.1.0-alpha — Types, Core, and HAL packages complete. Backends next.

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
│   ├── gles/      # OpenGL ES backend (planned)
│   ├── vulkan/    # Vulkan backend (planned)
│   ├── metal/     # Metal backend (planned)
│   └── dx12/      # DirectX 12 backend (planned)
└── internal/      # Platform-specific code
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

**Phase 4: Pure Go Backends** (Next)
- [ ] OpenGL backend (`hal/gles/`) — Most portable, easiest
- [ ] Vulkan backend (`hal/vulkan/`) — Primary for Linux/Windows
- [ ] Metal backend (`hal/metal/`) — Required for macOS/iOS
- [ ] DX12 backend (`hal/dx12/`) — Windows high-performance

## Pure Go Approach

All backends implemented without CGO:

| Backend | Approach | Reference |
|---------|----------|-----------|
| OpenGL | purego / go-gl patterns | [Gio](https://gioui.org), [go-gl](https://github.com/go-gl/gl) |
| Vulkan | purego / syscall | [vulkan-go](https://github.com/vulkan-go/vulkan) |
| Metal | purego (Obj-C bridge) | [Ebitengine](https://ebitengine.org) |
| DX12 | syscall + COM | [Gio DX11](https://gioui.org) |

## References

- [wgpu (Rust)](https://github.com/gfx-rs/wgpu) — Reference implementation
- [WebGPU Specification](https://www.w3.org/TR/webgpu/)
- [Dawn (C++)](https://dawn.googlesource.com/dawn) — Google's implementation

## Related Projects

| Project | Description |
|---------|-------------|
| [gogpu/gogpu](https://github.com/gogpu/gogpu) | Graphics framework |
| [gogpu/naga](https://github.com/gogpu/naga) | Pure Go shader compiler |
| [go-webgpu/webgpu](https://github.com/go-webgpu/webgpu) | FFI bindings (current solution) |

## Contributing

We welcome contributions! See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

MIT License — see [LICENSE](LICENSE) for details.

---

<p align="center">
  <b>wgpu</b> — WebGPU in Pure Go
</p>
