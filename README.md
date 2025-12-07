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

> **Status:** Types package complete, core validation in progress.

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

## Architecture

```
wgpu/
├── types/         # WebGPU type definitions (done)
├── core/          # Validation, state tracking (planned)
├── hal/           # Hardware abstraction layer
│   ├── vulkan/    # Vulkan backend
│   ├── metal/     # Metal backend (macOS/iOS)
│   ├── dx12/      # DirectX 12 backend (Windows)
│   └── gl/        # OpenGL fallback
└── internal/      # Platform-specific code
```

## Roadmap

**Phase 1: Types Package**
- [x] Backend types (Vulkan, Metal, DX12, GL)
- [x] Adapter and device types
- [x] Feature flags
- [x] GPU limits with presets
- [x] Texture formats (100+)
- [x] Buffer, sampler, shader types
- [x] Bind group and render state types
- [x] Vertex formats with size calculations

**Phase 2: Core Validation**
- [ ] Instance validation
- [ ] Adapter/device state tracking
- [ ] Resource validation
- [ ] Error handling

**Phase 3: HAL Interface**
- [ ] Backend abstraction layer
- [ ] Platform detection
- [ ] Memory management

**Phase 4: Backends**
- [ ] OpenGL backend (easiest, uses go-gl)
- [ ] Vulkan backend (uses vulkan-go)
- [ ] Metal backend (macOS/iOS)
- [ ] DX12 backend (Windows)

## Dependencies (Planned)

| Backend | Go Library |
|---------|------------|
| Vulkan | [vulkan-go/vulkan](https://github.com/vulkan-go/vulkan) |
| Metal | TBD (FFI to Objective-C) |
| DX12 | TBD (syscall to COM APIs) |
| OpenGL | [go-gl/gl](https://github.com/go-gl/gl) |

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
