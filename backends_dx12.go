//go:build windows && (386 || amd64 || arm64) && !rust && !(js && wasm) && !wgpu_nodx12

package wgpu

// The DX12 backend is registered by default (ADR-083 §2.1). Build with
// -tags wgpu_nodx12 to leave it out and shrink the binary.
import _ "github.com/gogpu/wgpu/hal/dx12"
