//go:build js && wasm

package wgpu

import (
	"testing"

	publichal "github.com/gogpu/wgpu/hal"
)

// TestHALBrowserReleasedAdapter pins the per-context rule for js/wasm: the
// adapter holds no instance link and WebGPU has no instance destroy, so
// releasing the instance does not invalidate the adapter — it keeps reporting
// its handles until Adapter.Release(), which yields the zero Handles.
func TestHALBrowserReleasedAdapter(t *testing.T) {
	// An adapter whose instance was released: the zero adapter stands in for
	// one obtained before the release, since the browser adapter keeps no
	// instance link and its report does not depend on the instance.
	adapter := &Adapter{}
	if got := adapter.HAL(); got.Kind != publichal.KindBrowser {
		t.Fatalf("adapter of a released instance HAL() = %+v, want KindBrowser", got)
	}

	adapter.Release()
	if got := adapter.HAL(); got != (publichal.Handles{}) {
		t.Fatalf("released adapter HAL() = %+v, want zero Handles", got)
	}
}
