//go:build !rust && !(js && wasm)

package wgputest_test

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
	"github.com/gogpu/wgpu/wgputest"
)

// TestDetachedTextureDeviceOperation pins the second half of the contract: an
// operation that needs the GPU reports the error wgpu reports for a texture
// without a HAL handle, and never panics.
func TestDetachedTextureDeviceOperation(t *testing.T) {
	device, _, cleanup, err := wgputest.NewNoopDevice()
	if err != nil {
		t.Fatalf("NewNoopDevice: %v", err)
	}
	defer cleanup()

	tex := wgputest.NewDetachedTexture(&wgpu.TextureDescriptor{Format: gputypes.TextureFormatRGBA8Unorm})
	defer tex.Release()

	if _, err := device.CreateTextureView(tex, nil); !errors.Is(err, wgpu.ErrReleased) {
		t.Fatalf("CreateTextureView on a detached texture = %v, want ErrReleased", err)
	}
}
