//go:build !rust && !(js && wasm) && wgpu_nosoftware

package wgputest_test

import (
	"errors"
	"testing"

	"github.com/gogpu/wgpu/wgputest"
)

// TestSoftwareUnavailable: with wgpu_nosoftware the software constructor
// reports ErrUnavailable and the noop constructor still works.
func TestSoftwareUnavailable(t *testing.T) {
	device, _, cleanup, err := wgputest.NewSoftwareDevice()
	if device != nil || cleanup != nil {
		t.Error("NewSoftwareDevice returned non-nil values")
	}
	if !errors.Is(err, wgputest.ErrUnavailable) {
		t.Fatalf("NewSoftwareDevice error = %v, want ErrUnavailable", err)
	}

	noopDevice, _, noopCleanup, err := wgputest.NewNoopDevice()
	if err != nil {
		t.Fatalf("NewNoopDevice: %v", err)
	}
	defer noopCleanup()
	if noopDevice == nil {
		t.Fatal("NewNoopDevice returned a nil device")
	}
}
