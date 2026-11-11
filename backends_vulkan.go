//go:build ((windows && (386 || amd64 || arm64)) || (linux && !android && (amd64 || arm64)) || (android && arm64)) && !rust && !(js && wasm) && !wgpu_novulkan

package wgpu

// The Vulkan backend is registered by default (ADR-083 §2.1). Build with
// -tags wgpu_novulkan to leave it out and shrink the binary.
import _ "github.com/gogpu/wgpu/hal/vulkan"
