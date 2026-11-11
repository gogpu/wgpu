//go:build darwin && (amd64 || arm64) && !rust && !(js && wasm) && !wgpu_nometal

package wgpu

// The Metal backend is registered by default (ADR-083 §2.1). Build with
// -tags wgpu_nometal to leave it out and shrink the binary.
import _ "github.com/gogpu/wgpu/hal/metal"
