//go:build !rust && !(js && wasm)

package wgpu_test

import (
	"testing"

	"github.com/gogpu/wgpu"
	"github.com/gogpu/wgpu/internal/shaderir"
)

// TestCreateShaderModuleLowersOnce checks that a WGSL module is parsed and
// validated exactly once, in core with the device's capabilities, and that
// the HAL compiles that IR instead of lowering the source again, as Rust
// wgpu-hal takes hal::ShaderInput::Naga from wgpu-core (P1-CORE-013). Runs
// on the default adapter and on the fallback (software) adapter.
func TestCreateShaderModuleLowersOnce(t *testing.T) {
	const plain = `@group(0) @binding(0) var<storage, read_write> out: array<u32>;
@compute @workgroup_size(1) fn main() { out[0] = 1u; }`
	const withOverride = `override k: u32 = 3u;
@group(0) @binding(0) var<storage, read_write> out: array<u32>;
@compute @workgroup_size(1) fn main() { out[0] = k; }`

	for _, adapterCase := range []struct {
		name string
		opts *wgpu.RequestAdapterOptions
	}{
		{"default adapter", nil},
		{"fallback adapter", &wgpu.RequestAdapterOptions{ForceFallbackAdapter: true}},
	} {
		t.Run(adapterCase.name, func(t *testing.T) {
			inst, err := wgpu.CreateInstance(nil)
			if err != nil {
				t.Fatalf("CreateInstance: %v", err)
			}
			defer inst.Release()
			adapter, err := inst.RequestAdapter(adapterCase.opts)
			if err != nil {
				t.Fatalf("RequestAdapter: %v", err)
			}
			defer adapter.Release()
			device, err := adapter.RequestDevice(nil)
			if err != nil {
				t.Fatalf("RequestDevice: %v", err)
			}
			defer device.Release()

			for _, source := range []string{plain, withOverride} {
				before := shaderir.Lowerings()
				module, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{WGSL: source})
				if err != nil {
					t.Fatalf("CreateShaderModule: %v", err)
				}
				lowerings := shaderir.Lowerings() - before
				module.Release()
				if lowerings != 1 {
					t.Errorf("%s: module lowered %d times, want 1 (backend %v)", adapter.Info().Name, lowerings, adapter.Info().Backend)
				}
			}
		})
	}
}
