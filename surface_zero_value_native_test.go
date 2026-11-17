//go:build !rust && !(js && wasm)

package wgpu_test

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// TestZeroValueSurfaceConfigureKeepsPreMoveAnswer pins the zero-value answer of
// the public Configure: before the move an instance-owned device was rejected
// as belonging to a different instance before any surface state was read.
func TestZeroValueSurfaceConfigureKeepsPreMoveAnswer(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()

	var s wgpu.Surface
	err := s.Configure(device, &wgpu.SurfaceConfiguration{
		Width:  4,
		Height: 4,
		Format: gputypes.TextureFormatRGBA8Unorm,
		Usage:  wgpu.TextureUsageRenderAttachment,
	})
	if err == nil || err.Error() != "wgpu: surface and device belong to different instances" {
		t.Fatalf("Configure on a zero-value surface = %v, want \"wgpu: surface and device belong to different instances\"", err)
	}
}
