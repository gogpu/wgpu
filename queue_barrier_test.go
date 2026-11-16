//go:build !rust && !(js && wasm)

package wgpu

import (
	"testing"

	"github.com/gogpu/wgpu/internal/resource"
)

// The barrier-injection half of this file moved to internal/queue with the
// queue (ADR-070 S9): the nine TestInjectTextureBarriers_* drive the queue's
// own injection path and are ported onto the destination's doubles. What stays
// here is the texture/core-texture wiring the public device performs, which is
// the device's surface and not the queue's.

// =============================================================================
// ADR-060: Texture coreTexture wiring tests
// =============================================================================

// TestCreateTexture_HasCoreTexture verifies that CreateTexture allocates a
// core.Texture with a valid TrackerIndex.
func TestCreateTexture_HasCoreTexture(t *testing.T) {
	_, _, device := newTestDeviceWithTracker(t)
	defer device.Release()

	tex, err := device.CreateTexture(&TextureDescriptor{
		Label:         "tracker-test",
		Size:          Extent3D{Width: 64, Height: 64, DepthOrArrayLayers: 1},
		Dimension:     TextureDimension2D,
		Format:        TextureFormatRGBA8Unorm,
		Usage:         TextureUsageRenderAttachment,
		MipLevelCount: 1,
		SampleCount:   1,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	defer tex.Release()

	if tex.impl.CoreTexture() == nil {
		t.Fatal("CreateTexture should allocate a coreTexture")
	}

	td := tex.impl.CoreTexture().TrackingData()
	if td == nil {
		t.Fatal("coreTexture should have TrackingData")
	}
	if !td.Index().IsValid() {
		t.Error("coreTexture TrackerIndex should be valid")
	}
}

// TestCoreTextureViewFrom_WiresParent verifies that resource.CoreTextureViewFrom
// sets the Parent on the core.TextureView.
func TestCoreTextureViewFrom_WiresParent(t *testing.T) {
	_, _, device := newTestDeviceWithTracker(t)
	defer device.Release()

	tex, err := device.CreateTexture(&TextureDescriptor{
		Label:         "view-parent-test",
		Size:          Extent3D{Width: 64, Height: 64, DepthOrArrayLayers: 1},
		Dimension:     TextureDimension2D,
		Format:        TextureFormatRGBA8Unorm,
		Usage:         TextureUsageRenderAttachment | TextureUsageTextureBinding,
		MipLevelCount: 1,
		SampleCount:   1,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	defer tex.Release()

	view, err := device.CreateTextureView(tex, nil)
	if err != nil {
		t.Fatalf("CreateTextureView: %v", err)
	}
	defer view.Release()

	cv := resource.CoreTextureViewFrom(view.impl)
	if cv == nil {
		t.Fatal("resource.CoreTextureViewFrom should not return nil")
	}
	if cv.Parent == nil {
		t.Fatal("core.TextureView.Parent should be set from texture.coreTexture")
	}

	// Parent's TrackerIndex should match the texture's
	if cv.Parent.TrackingData().Index() != tex.impl.CoreTexture().TrackingData().Index() {
		t.Error("core.TextureView.Parent TrackerIndex should match texture's")
	}
}

// TestCoreTextureViewFrom_NilTexture verifies that resource.CoreTextureViewFrom
// handles a view with no texture gracefully.
func TestCoreTextureViewFrom_NilTexture(t *testing.T) {
	v := &resource.TextureView{}
	cv := resource.CoreTextureViewFrom(v)
	if cv == nil {
		t.Fatal("resource.CoreTextureViewFrom should not return nil")
	}
	if cv.Parent != nil {
		t.Error("Parent should be nil when texture is nil")
	}
}
