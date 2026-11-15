//go:build rust

package wgpu

import (
	"testing"

	publichal "github.com/gogpu/wgpu/hal"
)

// TestHALRustReleasedAdapter pins the zero-Handles promise for the rust build:
// hal_interop.go states it and ADR-070 §15.5 repeats it. The live check first
// proves the guard is scoped; then the adapter-release and instance-release
// paths each yield the zero Handles. A cached adapter is instance-owned, so
// without the guard either path reports a stale wgpu-native pointer.
func TestHALRustReleasedAdapter(t *testing.T) {
	t.Run("AdapterRelease", func(t *testing.T) {
		instance, adapter := newRustTestAdapter(t)
		if got := adapter.HAL(); got.Kind != publichal.KindWebGPU {
			t.Fatalf("live adapter HAL() = %+v, want KindWebGPU", got)
		}
		adapter.Release()
		if got := adapter.HAL(); got != (publichal.Handles{}) {
			t.Fatalf("released adapter HAL() = %+v, want zero Handles", got)
		}
		instance.Release()
	})

	t.Run("InstanceRelease", func(t *testing.T) {
		instance, adapter := newRustTestAdapter(t)
		if got := adapter.HAL(); got.Kind != publichal.KindWebGPU {
			t.Fatalf("live adapter HAL() = %+v, want KindWebGPU", got)
		}
		instance.Release()
		if got := adapter.HAL(); got != (publichal.Handles{}) {
			t.Fatalf("adapter of a released instance HAL() = %+v, want zero Handles", got)
		}
	})
}

func newRustTestAdapter(t *testing.T) (*Instance, *Adapter) {
	t.Helper()
	instance, err := CreateInstance(nil)
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	adapter, err := instance.RequestAdapter(nil)
	if err != nil {
		instance.Release()
		t.Fatalf("RequestAdapter: %v", err)
	}
	return instance, adapter
}
