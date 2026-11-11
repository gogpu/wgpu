//go:build ((windows && (386 || amd64 || arm64)) || (((linux && !android) || darwin) && (amd64 || arm64)) || (android && arm64)) && !rust && !(js && wasm) && !wgpu_nosoftware

package wgpu

// The Pure Go software backend is registered by default (ADR-083 §2.1) but is
// not part of the default adapter mask: it is chosen explicitly, or through
// ForceFallbackAdapter. Build with -tags wgpu_nosoftware to leave it out.
import _ "github.com/gogpu/wgpu/hal/software"
