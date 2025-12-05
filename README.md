# wgpu

[![Go Reference](https://pkg.go.dev/badge/github.com/gogpu/wgpu.svg)](https://pkg.go.dev/github.com/gogpu/wgpu)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

**Pure Go WebGPU Implementation** — No Rust, No CGO, Just Go.

> 🔮 **Future** — Long-term goal of the GoGPU ecosystem

---

## ✨ Vision

A complete WebGPU implementation in pure Go:

- **No wgpu-native dependency** — Standalone Go library
- **Direct GPU access** — Vulkan, Metal, DX12 backends
- **WebGPU compliant** — Following the W3C specification
- **WASM compatible** — Run in browsers via WebAssembly

## 🏗️ Architecture (Planned)

```
wgpu/
├── core/          # Validation, state tracking
├── hal/           # Hardware abstraction layer
│   ├── vulkan/    # Vulkan backend
│   ├── metal/     # Metal backend (macOS/iOS)
│   ├── dx12/      # DirectX 12 backend (Windows)
│   └── gl/        # OpenGL fallback
└── types/         # WebGPU types
```

## 🔗 Dependencies (Planned)

| Backend | Go Library |
|---------|------------|
| Vulkan | [vulkan-go/vulkan](https://github.com/vulkan-go/vulkan) |
| Metal | TBD (FFI to Objective-C) |
| DX12 | TBD (syscall to COM APIs) |
| OpenGL | [go-gl/gl](https://github.com/go-gl/gl) |

## 🗺️ Roadmap

1. **Phase 1:** Types package (port wgpu-types)
2. **Phase 2:** Core validation (port wgpu-core)
3. **Phase 3:** OpenGL backend (easiest, uses go-gl)
4. **Phase 4:** Vulkan backend (uses vulkan-go)
5. **Phase 5:** Metal/DX12 backends

## 📚 References

- [wgpu (Rust)](https://github.com/gfx-rs/wgpu) — Reference implementation
- [WebGPU Specification](https://www.w3.org/TR/webgpu/)
- [Dawn (C++)](https://dawn.googlesource.com/dawn) — Google's implementation

## 🔗 Related Projects

| Project | Description |
|---------|-------------|
| [go-webgpu/webgpu](https://github.com/go-webgpu/webgpu) | FFI bindings (current solution) |
| [gogpu/naga](https://github.com/gogpu/naga) | Pure Go shader compiler |
| [gogpu/gogpu](https://github.com/gogpu/gogpu) | Graphics framework |

## 📄 License

MIT License

---

<p align="center">
  <b>wgpu</b> — WebGPU in Pure Go
</p>
