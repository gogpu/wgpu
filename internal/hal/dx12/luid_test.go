//go:build windows && !(js && wasm)

package dx12

import (
	"testing"

	"github.com/gogpu/wgpu/internal/hal/dx12/dxgi"
)

// TestLUIDPack pins the DXGI LUID packing into the AdapterLUID numeric form
// (ADR-084 §2.1): uint64(uint32(HighPart))<<32 | uint64(LowPart), with the
// signed HighPart keeping its bit pattern.
func TestLUIDPack(t *testing.T) {
	tests := []struct {
		name string
		luid dxgi.LUID
		want uint64
	}{
		{name: "low only", luid: dxgi.LUID{LowPart: 0x12345678, HighPart: 0}, want: 0x0000000012345678},
		{name: "both parts", luid: dxgi.LUID{LowPart: 0x9abcdef0, HighPart: 0x12345678}, want: 0x123456789abcdef0},
		{name: "high sign bit", luid: dxgi.LUID{LowPart: 1, HighPart: -2147483648}, want: 0x8000000000000001},
		{name: "negative high", luid: dxgi.LUID{LowPart: 0xffffffff, HighPart: -1}, want: 0xffffffffffffffff},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := luidPack(tt.luid); got != tt.want {
				t.Fatalf("luidPack(%+v) = %#016x, want %#016x", tt.luid, got, tt.want)
			}
		})
	}
}
