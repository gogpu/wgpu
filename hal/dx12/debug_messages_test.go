//go:build windows && !(js && wasm)

package dx12

import (
	"testing"

	"github.com/gogpu/wgpu/hal/dx12/d3d12"
)

// TestIsIgnoredDebugMessage pins the debug-layer filter to the messages Rust
// wgpu-hal drops (wgpu-hal/src/auxil/dxgi/exception.rs): only the #820 and
// #821 warnings. The same IDs at error severity, and every other message, are
// still reported.
func TestIsIgnoredDebugMessage(t *testing.T) {
	cases := []struct {
		name     string
		severity d3d12.D3D12MessageSeverity
		id       int32
		want     bool
	}{
		{"render target clear value warning", d3d12.D3D12MessageSeverityWarning, 820, true},
		{"depth stencil clear value warning", d3d12.D3D12MessageSeverityWarning, 821, true},
		{"820 as an error", d3d12.D3D12MessageSeverityError, 820, false},
		{"821 as corruption", d3d12.D3D12MessageSeverityCorruption, 821, false},
		{"neighboring warning 819", d3d12.D3D12MessageSeverityWarning, 819, false},
		{"neighboring warning 822", d3d12.D3D12MessageSeverityWarning, 822, false},
		{"resource barrier mismatch error 524", d3d12.D3D12MessageSeverityError, 524, false},
	}
	for _, c := range cases {
		if got := isIgnoredDebugMessage(c.severity, c.id); got != c.want {
			t.Errorf("%s: isIgnoredDebugMessage(%v, %d) = %v, want %v", c.name, c.severity, c.id, got, c.want)
		}
	}
}
