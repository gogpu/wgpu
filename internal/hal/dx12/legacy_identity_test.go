//go:build windows && !(js && wasm)

package dx12

import (
	"testing"

	"github.com/gogpu/wgpu/internal/hal/dx12/dxgi"
)

// TestLegacyAdapterIdentity pins the DX12 legacy enumeration path's identity
// wiring: the legacy adapter must report the same AdapterLUID the primary path
// does for the same DXGI adapter (ADR-084 §2.3). Adopted from the
// implementation gate's probe (P3-1): removing the wiring left the suite green
// before this test existed.
func TestLegacyAdapterIdentity(t *testing.T) {
	a := &AdapterLegacy{desc: dxgi.DXGI_ADAPTER_DESC1{
		AdapterLuid: dxgi.LUID{LowPart: 0x11223344, HighPart: 1},
	}}
	got := a.toExposedAdapter().Identity.AdapterLUID
	const want = uint64(0x0000000111223344)
	if got != want {
		t.Fatalf("legacy toExposedAdapter AdapterLUID = %#x, want %#x (the primary Adapter path sets it, ADR-084 §2.3)", got, want)
	}
}
