//go:build !rust && !(js && wasm)

package wgpu

import (
	"testing"

	"github.com/gogpu/gputypes"
)

// newIdentityFixture returns a texture, its view and the device that made
// them, for the TextureView.Texture identity and allocation tests.
func newIdentityFixture(t *testing.T) (*Device, *Texture, *TextureView) {
	t.Helper()
	_, _, device := newTestDeviceWithTracker(t)
	tex, err := device.CreateTexture(&TextureDescriptor{
		Label:         "identity",
		Size:          Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1},
		Dimension:     TextureDimension2D,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         TextureUsageTextureBinding | TextureUsageRenderAttachment,
		MipLevelCount: 1,
		SampleCount:   1,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	view, err := device.CreateTextureView(tex, nil)
	if err != nil {
		tex.Release()
		t.Fatalf("CreateTextureView: %v", err)
	}
	return device, tex, view
}

// TestTextureViewTextureDoesNotAllocate pins the hot accessor: Texture() is a
// cached-slot read, not a wrapper construction per call. The identity slot is
// what makes that possible (see the identity test next to it).
func TestTextureViewTextureDoesNotAllocate(t *testing.T) {
	device, tex, view := newIdentityFixture(t)
	defer device.Release()
	defer tex.Release()
	defer view.Release()

	if got := testing.AllocsPerRun(100, func() { _ = view.Texture() }); got != 0 {
		t.Fatalf("Texture() allocated %v times per call, want 0", got)
	}
}

// TestTextureViewTextureReturnsSameWrapper: Texture() returns the very
// wrapper the device handed out for the parent. Two wrappers for one texture
// would break the identity comparisons the render bundle and copy validation
// make, besides allocating.
func TestTextureViewTextureReturnsSameWrapper(t *testing.T) {
	device, tex, view := newIdentityFixture(t)
	defer device.Release()
	defer tex.Release()
	defer view.Release()

	if got := view.Texture(); got != tex {
		t.Fatalf("Texture() = %p, want the wrapper the device returned (%p)", got, tex)
	}
	if got, want := view.Texture().Format(), tex.Format(); got != want {
		t.Fatalf("Texture().Format() = %v, want %v", got, want)
	}
}

// TestTextureViewTextureAfterRelease: a released view has no parent texture
// to report (the native guard the pre-move type had).
func TestTextureViewTextureAfterRelease(t *testing.T) {
	device, tex, view := newIdentityFixture(t)
	defer device.Release()
	defer tex.Release()

	view.Release()
	if got := view.Texture(); got != nil {
		t.Fatalf("Texture() after Release = %p, want nil", got)
	}
}
