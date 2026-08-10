//go:build !rust && !(js && wasm)

package wgpu

import (
	"testing"

	"github.com/gogpu/wgpu/core/track"

	_ "github.com/gogpu/wgpu/hal/noop"
)

// =============================================================================
// ADR-060: Submit-time barrier injection via DeviceTracker
// =============================================================================

// TestInjectTextureBarriers_NilDevice verifies that injectBarriers
// returns nil when the queue has no device.
func TestInjectTextureBarriers_NilDevice(t *testing.T) {
	t.Parallel()

	q := &Queue{}
	cb, err := q.injectBarriers(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cb != nil {
		t.Error("expected nil barrier CB for nil device")
	}
}

// TestInjectTextureBarriers_NilTracker verifies that injectBarriers
// returns nil when the device has no tracker.
func TestInjectTextureBarriers_NilTracker(t *testing.T) {
	t.Parallel()

	// Device with nil core -> Tracker() returns nil
	q := &Queue{device: &Device{}}
	cb, err := q.injectBarriers(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cb != nil {
		t.Error("expected nil barrier CB for nil tracker")
	}
}

// TestInjectTextureBarriers_EmptyScope verifies that no barriers are
// generated when command buffers have empty texture scopes.
func TestInjectTextureBarriers_EmptyScope(t *testing.T) {
	t.Parallel()

	_, _, device := newTestDeviceWithTracker(t)
	defer device.Release()
	q := device.Queue()

	// Create a CB with an empty texture scope via the core encoder.
	// No BeginRenderPass -> scope remains empty.
	coreEncoder, err := device.core.CreateCommandEncoder("test")
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	coreCB, err := coreEncoder.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}

	cb := &CommandBuffer{
		core:         coreCB,
		device:       device,
		usedTextures: make(map[*Texture]struct{}),
	}

	barrierCB, err := q.injectBarriers([]*CommandBuffer{cb})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if barrierCB != nil {
		t.Error("expected nil barrier CB for empty scope")
	}
}

// TestInjectTextureBarriers_WithTransition verifies that barriers are
// generated when a command buffer's scope requires a state transition.
func TestInjectTextureBarriers_WithTransition(t *testing.T) {
	t.Parallel()

	_, _, device := newTestDeviceWithTracker(t)
	defer device.Release()
	q := device.Queue()

	// Create a texture with core.Texture for tracking
	tex := createTrackedTexture(t, device, "barrier-test")
	defer tex.Release()

	tracker := device.core.Tracker()
	if tracker == nil {
		t.Fatal("device tracker should not be nil")
	}

	// Register texture in device tracker with initial RESOURCE state
	td := tex.coreTexture.TrackingData()
	if td == nil || !td.Index().IsValid() {
		t.Fatal("texture should have valid tracker index")
	}
	tracker.InsertTexture(td.Index(), track.TextureUsesResource)

	// Build a CB whose textureScope requests ColorTarget for this texture.
	// We create the encoder, finish it, then manually set usage on the
	// resulting CB's scope. This is valid because TextureScope() returns
	// the mutable scope that was transferred from the encoder.
	coreEncoder, err := device.core.CreateCommandEncoder("test")
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	coreCB, err := coreEncoder.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}

	scope := coreCB.TextureScope()
	if scope == nil {
		t.Fatal("textureScope should not be nil")
	}
	_ = scope.SetUsage(td.Index(), track.TextureUsesColorTarget)

	cb := &CommandBuffer{
		core:         coreCB,
		device:       device,
		usedTextures: map[*Texture]struct{}{tex: {}},
	}

	barrierCB, err := q.injectBarriers([]*CommandBuffer{cb})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Barrier CB should be non-nil because Resource -> ColorTarget needs a barrier
	if barrierCB == nil {
		t.Fatal("expected non-nil barrier CB for Resource->ColorTarget transition")
	}

	// Device tracker should now reflect ColorTarget state
	if tracker.Textures().GetUsage(td.Index()) != track.TextureUsesColorTarget {
		t.Error("device tracker should be in ColorTarget state after merge")
	}
}

// TestInjectTextureBarriers_NoTransitionSameState verifies that no barriers
// are generated when the texture remains in the same ordered state.
func TestInjectTextureBarriers_NoTransitionSameState(t *testing.T) {
	t.Parallel()

	_, _, device := newTestDeviceWithTracker(t)
	defer device.Release()
	q := device.Queue()

	tex := createTrackedTexture(t, device, "same-state")
	defer tex.Release()

	tracker := device.core.Tracker()
	td := tex.coreTexture.TrackingData()
	tracker.InsertTexture(td.Index(), track.TextureUsesResource)

	// CB uses texture as Resource (same ordered state)
	coreEncoder, err := device.core.CreateCommandEncoder("test")
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	coreCB, err := coreEncoder.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}

	scope := coreCB.TextureScope()
	_ = scope.SetUsage(td.Index(), track.TextureUsesResource)

	cb := &CommandBuffer{
		core:         coreCB,
		device:       device,
		usedTextures: map[*Texture]struct{}{tex: {}},
	}

	barrierCB, err := q.injectBarriers([]*CommandBuffer{cb})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if barrierCB != nil {
		t.Error("expected nil barrier CB for same ordered state (no transition needed)")
	}
}

// TestInjectTextureBarriers_MultipleTextures verifies barrier generation
// with multiple textures across command buffers.
func TestInjectTextureBarriers_MultipleTextures(t *testing.T) {
	t.Parallel()

	_, _, device := newTestDeviceWithTracker(t)
	defer device.Release()
	q := device.Queue()

	tex1 := createTrackedTexture(t, device, "tex1")
	defer tex1.Release()
	tex2 := createTrackedTexture(t, device, "tex2")
	defer tex2.Release()

	tracker := device.core.Tracker()
	td1 := tex1.coreTexture.TrackingData()
	td2 := tex2.coreTexture.TrackingData()

	tracker.InsertTexture(td1.Index(), track.TextureUsesResource)
	tracker.InsertTexture(td2.Index(), track.TextureUsesResource)

	// CB1: uses tex1 as ColorTarget (transition), tex2 as Resource (no transition)
	coreEnc1, err := device.core.CreateCommandEncoder("cb1")
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	coreCB1, err := coreEnc1.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}

	scope1 := coreCB1.TextureScope()
	_ = scope1.SetUsage(td1.Index(), track.TextureUsesColorTarget)
	_ = scope1.SetUsage(td2.Index(), track.TextureUsesResource)

	cb1 := &CommandBuffer{
		core:         coreCB1,
		device:       device,
		usedTextures: map[*Texture]struct{}{tex1: {}, tex2: {}},
	}

	barrierCB, err := q.injectBarriers([]*CommandBuffer{cb1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only tex1 needs a transition (Resource -> ColorTarget)
	if barrierCB == nil {
		t.Fatal("expected non-nil barrier CB when one texture needs transition")
	}

	// Both textures should have updated state in device tracker
	if tracker.Textures().GetUsage(td1.Index()) != track.TextureUsesColorTarget {
		t.Error("tex1 should be in ColorTarget state")
	}
	if tracker.Textures().GetUsage(td2.Index()) != track.TextureUsesResource {
		t.Error("tex2 should remain in Resource state")
	}
}

// TestInjectTextureBarriers_NilCommandBuffers verifies graceful handling
// of nil entries in the command buffer slice.
func TestInjectTextureBarriers_NilCommandBuffers(t *testing.T) {
	t.Parallel()

	_, _, device := newTestDeviceWithTracker(t)
	defer device.Release()
	q := device.Queue()

	barrierCB, err := q.injectBarriers([]*CommandBuffer{nil, nil})
	if err != nil {
		t.Fatalf("unexpected error with nil CBs: %v", err)
	}
	if barrierCB != nil {
		t.Error("expected nil barrier CB for all-nil command buffers")
	}
}

// TestInjectTextureBarriers_NoEncoderPool verifies that when the device has
// no encoder pool, transitions are detected but no barrier CB is created.
func TestInjectTextureBarriers_NoEncoderPool(t *testing.T) {
	t.Parallel()

	_, _, device := newTestDeviceWithTracker(t)
	defer device.Release()
	q := device.Queue()

	// Save and clear the encoder pool to simulate a device without one
	savedPool := device.cmdEncoderPool
	device.cmdEncoderPool = nil
	defer func() { device.cmdEncoderPool = savedPool }()

	tex := createTrackedTexture(t, device, "no-pool")
	defer tex.Release()
	tracker := device.core.Tracker()
	td := tex.coreTexture.TrackingData()
	tracker.InsertTexture(td.Index(), track.TextureUsesResource)

	coreEncoder, err := device.core.CreateCommandEncoder("test")
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	coreCB, err := coreEncoder.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}

	scope := coreCB.TextureScope()
	_ = scope.SetUsage(td.Index(), track.TextureUsesColorTarget)

	cb := &CommandBuffer{
		core:         coreCB,
		device:       device,
		usedTextures: map[*Texture]struct{}{tex: {}},
	}

	barrierCB, err := q.injectBarriers([]*CommandBuffer{cb})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// No encoder pool -> cannot create barrier CB, returns nil
	if barrierCB != nil {
		t.Error("expected nil barrier CB when no encoder pool available")
	}

	// The device tracker should still have merged the state
	if tracker.Textures().GetUsage(td.Index()) != track.TextureUsesColorTarget {
		t.Error("device tracker should still merge even without encoder pool")
	}
}

// TestInjectTextureBarriers_TextureWithoutCoreTexture verifies that
// textures without a coreTexture (no tracker index) are silently skipped.
func TestInjectTextureBarriers_TextureWithoutCoreTexture(t *testing.T) {
	t.Parallel()

	_, _, device := newTestDeviceWithTracker(t)
	defer device.Release()
	q := device.Queue()

	// Create a texture without coreTexture (simulates browser or test stub)
	tex := &Texture{device: device}

	coreEncoder, err := device.core.CreateCommandEncoder("test")
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	coreCB, err := coreEncoder.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}

	cb := &CommandBuffer{
		core:         coreCB,
		device:       device,
		usedTextures: map[*Texture]struct{}{tex: {}},
	}

	barrierCB, err := q.injectBarriers([]*CommandBuffer{cb})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if barrierCB != nil {
		t.Error("expected nil barrier CB for texture without coreTexture")
	}
}

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

	if tex.coreTexture == nil {
		t.Fatal("CreateTexture should allocate a coreTexture")
	}

	td := tex.coreTexture.TrackingData()
	if td == nil {
		t.Fatal("coreTexture should have TrackingData")
	}
	if !td.Index().IsValid() {
		t.Error("coreTexture TrackerIndex should be valid")
	}
}

// TestCoreTextureViewFrom_WiresParent verifies that coreTextureViewFrom
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

	cv := coreTextureViewFrom(view)
	if cv == nil {
		t.Fatal("coreTextureViewFrom should not return nil")
	}
	if cv.Parent == nil {
		t.Fatal("core.TextureView.Parent should be set from texture.coreTexture")
	}

	// Parent's TrackerIndex should match the texture's
	if cv.Parent.TrackingData().Index() != tex.coreTexture.TrackingData().Index() {
		t.Error("core.TextureView.Parent TrackerIndex should match texture's")
	}
}

// TestCoreTextureViewFrom_NilTexture verifies that coreTextureViewFrom
// handles a view with no texture gracefully.
func TestCoreTextureViewFrom_NilTexture(t *testing.T) {
	v := &TextureView{}
	cv := coreTextureViewFrom(v)
	if cv == nil {
		t.Fatal("coreTextureViewFrom should not return nil")
	}
	if cv.Parent != nil {
		t.Error("Parent should be nil when texture is nil")
	}
}

// =============================================================================
// Test helpers
// =============================================================================

// newTestDeviceWithTracker creates a device that has a DeviceTracker and
// encoder pool, using the noop backend.
func newTestDeviceWithTracker(t *testing.T) (*Instance, *Adapter, *Device) {
	t.Helper()
	inst, adapter, device := newTestDevice(t)
	if device.core == nil || device.core.Tracker() == nil {
		t.Skip("device has no tracker (no HAL integration)")
	}
	return inst, adapter, device
}

// newTestDevice creates a test device (internal package wgpu access).
func newTestDevice(t *testing.T) (*Instance, *Adapter, *Device) {
	t.Helper()
	inst, err := CreateInstance(nil)
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	adapter, err := inst.RequestAdapter(nil)
	if err != nil {
		t.Fatalf("RequestAdapter: %v", err)
	}
	device, err := adapter.RequestDevice(nil)
	if err != nil {
		t.Fatalf("RequestDevice: %v", err)
	}
	return inst, adapter, device
}

// createTrackedTexture creates a texture with a valid coreTexture and
// TrackerIndex for barrier injection tests.
func createTrackedTexture(t *testing.T, device *Device, label string) *Texture {
	t.Helper()
	tex, err := device.CreateTexture(&TextureDescriptor{
		Label:         label,
		Size:          Extent3D{Width: 64, Height: 64, DepthOrArrayLayers: 1},
		Dimension:     TextureDimension2D,
		Format:        TextureFormatRGBA8Unorm,
		Usage:         TextureUsageRenderAttachment | TextureUsageTextureBinding,
		MipLevelCount: 1,
		SampleCount:   1,
	})
	if err != nil {
		t.Fatalf("CreateTexture(%s): %v", label, err)
	}
	if tex.coreTexture == nil {
		t.Fatalf("texture %s has no coreTexture", label)
	}
	return tex
}
