//go:build darwin && (amd64 || arm64) && wgpu_moltenvk && !rust && !(js && wasm) && !wgpu_novulkan

package wgpu

// MoltenVK is opt-in on macOS (ADR-083 §2.1): Metal is the default backend and
// Vulkan is registered only with -tags wgpu_moltenvk.
import _ "github.com/gogpu/wgpu/hal/vulkan"
