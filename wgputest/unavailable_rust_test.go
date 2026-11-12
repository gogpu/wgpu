//go:build rust

package wgputest_test

import (
	"errors"
	"testing"

	"github.com/gogpu/wgpu/wgputest"
)

// TestUnavailableUnderRust: the Pure Go device implementation is not compiled
// with -tags rust, so both constructors report ErrUnavailable instead of
// returning a Rust-backed device.
func TestUnavailableUnderRust(t *testing.T) {
	if _, _, _, err := wgputest.NewSoftwareDevice(); !errors.Is(err, wgputest.ErrUnavailable) {
		t.Errorf("NewSoftwareDevice error = %v, want ErrUnavailable", err)
	}
	if _, _, _, err := wgputest.NewNoopDevice(); !errors.Is(err, wgputest.ErrUnavailable) {
		t.Errorf("NewNoopDevice error = %v, want ErrUnavailable", err)
	}
}
