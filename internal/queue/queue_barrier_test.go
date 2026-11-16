//go:build !rust && !(js && wasm)

package queue

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core/track"
	"github.com/gogpu/wgpu/internal/encoder"
	"github.com/gogpu/wgpu/internal/resource"
)

// =============================================================================
// ADR-060: Submit-time barrier injection via DeviceTracker
// =============================================================================
//
// These tests moved here with the queue (ADR-070 S9). They are ported onto the
// destination's own doubles: the core encoder and the tracked texture are
// internal objects, so the fixtures build them directly instead of going
// through the public device.

// TestInjectTextureBarriers_NilDevice verifies that injectBarriers
// returns nil when the queue has no device.
func TestInjectTextureBarriers_NilDevice(t *testing.T) {
	t.Parallel()

	q := New(nil, nil)
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
	q := newTestQueue(&testHost{})
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

	host, device := newTestHost(t, true)
	q := newTestQueue(host)

	// Create a CB with an empty texture scope via the core encoder.
	// No BeginRenderPass -> scope remains empty.
	coreEncoder, err := device.CreateCommandEncoder("test")
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	coreCB, err := coreEncoder.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}

	cb := encoder.NewTestCommandBuffer(coreCB, host)
	cb.SetUsedSetsForTest(nil, make(map[*resource.Texture]struct{}), nil)

	barrierCB, err := q.injectBarriers([]*encoder.CommandBuffer{cb})
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

	host, device := newTestHost(t, true)
	q := newTestQueue(host)

	// Create a texture with core.Texture for tracking
	tex := newTestTrackedTexture(t, device, host, "barrier-test")

	tracker := device.Tracker()
	if tracker == nil {
		t.Fatal("device tracker should not be nil")
	}

	// Register texture in device tracker with initial RESOURCE state
	td := tex.CoreTexture().TrackingData()
	if td == nil || !td.Index().IsValid() {
		t.Fatal("texture should have valid tracker index")
	}
	tracker.InsertTexture(td.Index(), track.TextureUsesResource)

	// Build a CB whose textureScope requests ColorTarget for this texture.
	// We create the encoder, finish it, then manually set usage on the
	// resulting CB's scope. This is valid because TextureScope() returns
	// the mutable scope that was transferred from the encoder.
	coreEncoder, err := device.CreateCommandEncoder("test")
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
	scope.AppendSingle(nil, td.Index(), track.TextureSelector{}, track.TextureUsesColorTarget)

	cb := encoder.NewTestCommandBuffer(coreCB, host)
	cb.SetUsedSetsForTest(nil, map[*resource.Texture]struct{}{tex: {}}, nil)

	barrierCB, err := q.injectBarriers([]*encoder.CommandBuffer{cb})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Barrier CB should be non-nil because Resource -> ColorTarget needs a barrier
	if barrierCB == nil {
		t.Fatal("expected non-nil barrier CB for Resource->ColorTarget transition")
	}

	// Device tracker should now reflect ColorTarget state
	if tracker.TextureUsage(td.Index()) != track.TextureUsesColorTarget {
		t.Error("device tracker should be in ColorTarget state after merge")
	}
}

// TestInjectTextureBarriers_NoTransitionSameState verifies that no barriers
// are generated when the texture remains in the same ordered state.
func TestInjectTextureBarriers_NoTransitionSameState(t *testing.T) {
	t.Parallel()

	host, device := newTestHost(t, true)
	q := newTestQueue(host)

	tex := newTestTrackedTexture(t, device, host, "same-state")

	tracker := device.Tracker()
	td := tex.CoreTexture().TrackingData()
	tracker.InsertTexture(td.Index(), track.TextureUsesResource)

	// CB uses texture as Resource (same ordered state)
	coreEncoder, err := device.CreateCommandEncoder("test")
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	coreCB, err := coreEncoder.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}

	scope := coreCB.TextureScope()
	scope.AppendSingle(nil, td.Index(), track.TextureSelector{}, track.TextureUsesResource)

	cb := encoder.NewTestCommandBuffer(coreCB, host)
	cb.SetUsedSetsForTest(nil, map[*resource.Texture]struct{}{tex: {}}, nil)

	barrierCB, err := q.injectBarriers([]*encoder.CommandBuffer{cb})
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

	host, device := newTestHost(t, true)
	q := newTestQueue(host)

	tex1 := newTestTrackedTexture(t, device, host, "tex1")
	tex2 := newTestTrackedTexture(t, device, host, "tex2")

	tracker := device.Tracker()
	td1 := tex1.CoreTexture().TrackingData()
	td2 := tex2.CoreTexture().TrackingData()

	tracker.InsertTexture(td1.Index(), track.TextureUsesResource)
	tracker.InsertTexture(td2.Index(), track.TextureUsesResource)

	// CB1: uses tex1 as ColorTarget (transition), tex2 as Resource (no transition)
	coreEnc1, err := device.CreateCommandEncoder("cb1")
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	coreCB1, err := coreEnc1.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}

	scope1 := coreCB1.TextureScope()
	scope1.AppendSingle(nil, td1.Index(), track.TextureSelector{}, track.TextureUsesColorTarget)
	scope1.AppendSingle(nil, td2.Index(), track.TextureSelector{}, track.TextureUsesResource)

	cb1 := encoder.NewTestCommandBuffer(coreCB1, host)
	cb1.SetUsedSetsForTest(nil, map[*resource.Texture]struct{}{tex1: {}, tex2: {}}, nil)

	barrierCB, err := q.injectBarriers([]*encoder.CommandBuffer{cb1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only tex1 needs a transition (Resource -> ColorTarget)
	if barrierCB == nil {
		t.Fatal("expected non-nil barrier CB when one texture needs transition")
	}

	// Both textures should have updated state in device tracker
	if tracker.TextureUsage(td1.Index()) != track.TextureUsesColorTarget {
		t.Error("tex1 should be in ColorTarget state")
	}
	if tracker.TextureUsage(td2.Index()) != track.TextureUsesResource {
		t.Error("tex2 should remain in Resource state")
	}
}

// TestInjectTextureBarriers_NilCommandBuffers verifies graceful handling
// of nil entries in the command buffer slice.
func TestInjectTextureBarriers_NilCommandBuffers(t *testing.T) {
	t.Parallel()

	host, _ := newTestHost(t, true)
	q := newTestQueue(host)

	barrierCB, err := q.injectBarriers([]*encoder.CommandBuffer{nil, nil})
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

	host, device := newTestHost(t, true)
	q := newTestQueue(host)

	// Clear the pool to simulate a device without one
	savedPool := host.pool
	host.pool = nil
	defer func() { host.pool = savedPool }()

	tex := newTestTrackedTexture(t, device, host, "no-pool")
	tracker := device.Tracker()
	td := tex.CoreTexture().TrackingData()
	tracker.InsertTexture(td.Index(), track.TextureUsesResource)

	coreEncoder, err := device.CreateCommandEncoder("test")
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	coreCB, err := coreEncoder.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}

	scope := coreCB.TextureScope()
	scope.AppendSingle(nil, td.Index(), track.TextureSelector{}, track.TextureUsesColorTarget)

	cb := encoder.NewTestCommandBuffer(coreCB, host)
	cb.SetUsedSetsForTest(nil, map[*resource.Texture]struct{}{tex: {}}, nil)

	barrierCB, err := q.injectBarriers([]*encoder.CommandBuffer{cb})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// No encoder pool -> cannot create barrier CB, returns nil
	if barrierCB != nil {
		t.Error("expected nil barrier CB when no encoder pool available")
	}

	// The device tracker should still have merged the state
	if tracker.TextureUsage(td.Index()) != track.TextureUsesColorTarget {
		t.Error("device tracker should still merge even without encoder pool")
	}
}

// TestInjectTextureBarriers_TextureWithoutCoreTexture verifies that
// textures without a coreTexture (no tracker index) are silently skipped.
func TestInjectTextureBarriers_TextureWithoutCoreTexture(t *testing.T) {
	t.Parallel()

	host, device := newTestHost(t, true)
	q := newTestQueue(host)

	// Create a texture without coreTexture (simulates a test stub)
	tex := resource.NewTexture(nil, host, gputypes.TextureFormatRGBA8Unorm, nil, nil)

	coreEncoder, err := device.CreateCommandEncoder("test")
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	coreCB, err := coreEncoder.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}

	cb := encoder.NewTestCommandBuffer(coreCB, host)
	cb.SetUsedSetsForTest(nil, map[*resource.Texture]struct{}{tex: {}}, nil)

	barrierCB, err := q.injectBarriers([]*encoder.CommandBuffer{cb})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if barrierCB != nil {
		t.Error("expected nil barrier CB for texture without coreTexture")
	}
}
