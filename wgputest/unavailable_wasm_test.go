//go:build js && wasm

package wgputest_test

import (
	"errors"
	"testing"

	"github.com/gogpu/wgpu/wgputest"
)

// TestUnavailableOnWasm: the Pure Go backends do not build for js/wasm.
func TestUnavailableOnWasm(t *testing.T) {
	if _, _, _, err := wgputest.NewSoftwareDevice(); !errors.Is(err, wgputest.ErrUnavailable) {
		t.Errorf("NewSoftwareDevice error = %v, want ErrUnavailable", err)
	}
	if _, _, _, err := wgputest.NewNoopDevice(); !errors.Is(err, wgputest.ErrUnavailable) {
		t.Errorf("NewNoopDevice error = %v, want ErrUnavailable", err)
	}
}
