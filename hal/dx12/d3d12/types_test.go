//go:build windows && !(js && wasm)

package d3d12

import (
	"testing"
	"unsafe"
)

func TestIndirectArgumentDescABI(t *testing.T) {
	if got := unsafe.Sizeof(D3D12_INDIRECT_ARGUMENT_DESC{}); got != 16 {
		t.Fatalf("D3D12_INDIRECT_ARGUMENT_DESC size = %d, want 16", got)
	}
	if got := unsafe.Offsetof(D3D12_INDIRECT_ARGUMENT_DESC{}.Union); got != 4 {
		t.Fatalf("D3D12_INDIRECT_ARGUMENT_DESC.Union offset = %d, want 4", got)
	}
}
