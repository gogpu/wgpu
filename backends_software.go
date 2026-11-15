//go:build ((windows && (386 || amd64 || arm64)) || (((linux && !android) || darwin) && (amd64 || arm64)) || (android && arm64)) && !rust && !(js && wasm) && !wgpu_nosoftware

package wgpu

// The Pure Go software backend is registered by default (ADR-083 §2.3): after
// hal moves under internal/, a program that needs a GPU-less path has no public
// backend package to import. It is never part of the default adapter set — mask
// 0 means the hardware backends — so a program that does not ask for software
// keeps its adapters and picks. Select it with
// RequestAdapterOptions.ForceFallbackAdapter or a BackendsSoftware mask; build
// with -tags wgpu_nosoftware to leave it out of the binary.
import _ "github.com/gogpu/wgpu/internal/hal/software"
