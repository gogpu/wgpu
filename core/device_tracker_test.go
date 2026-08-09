//go:build !(js && wasm)

package core

import (
	"testing"

	"github.com/gogpu/wgpu/core/track"
)

func TestDeviceTracker_New(t *testing.T) {
	t.Parallel()

	dt := NewDeviceTracker()
	if dt == nil {
		t.Fatal("NewDeviceTracker returned nil")
	}
	if dt.Textures() == nil {
		t.Fatal("Textures() returned nil")
	}
	if dt.Textures().Size() != 0 {
		t.Errorf("Initial texture tracker size = %d, want 0", dt.Textures().Size())
	}
}

func TestDeviceTracker_InsertRemoveTexture(t *testing.T) {
	t.Parallel()

	dt := NewDeviceTracker()
	idx := track.TrackerIndex(0)

	dt.InsertTexture(idx, track.TextureUsesColorTarget)
	if dt.Textures().Size() != 1 {
		t.Errorf("Size after insert = %d, want 1", dt.Textures().Size())
	}
	if !dt.Textures().IsTracked(idx) {
		t.Error("Texture should be tracked after insert")
	}

	dt.RemoveTexture(idx)
	if dt.Textures().IsTracked(idx) {
		t.Error("Texture should not be tracked after remove")
	}
}

func TestDeviceTracker_MergeTextureScope_NilSafe(t *testing.T) {
	t.Parallel()

	dt := NewDeviceTracker()
	transitions := dt.MergeTextureScope(nil)
	if len(transitions) != 0 {
		t.Errorf("Nil scope should produce 0 transitions, got %d", len(transitions))
	}
}

func TestDeviceTracker_MergeTextureScope_Transition(t *testing.T) {
	t.Parallel()

	dt := NewDeviceTracker()
	idx := track.TrackerIndex(0)

	// Register texture with initial color target usage
	dt.InsertTexture(idx, track.TextureUsesColorTarget)

	// Command buffer uses it as copy source
	scope := track.NewTextureUsageScope()
	_ = scope.SetUsage(idx, track.TextureUsesCopySrc)

	transitions := dt.MergeTextureScope(scope)

	if len(transitions) != 1 {
		t.Fatalf("Expected 1 transition, got %d", len(transitions))
	}
	if transitions[0].Usage.From != track.TextureUsesColorTarget {
		t.Errorf("From = %d, want ColorTarget", transitions[0].Usage.From)
	}
	if transitions[0].Usage.To != track.TextureUsesCopySrc {
		t.Errorf("To = %d, want CopySrc", transitions[0].Usage.To)
	}
}

func TestDeviceTracker_MergeTextureScope_NoTransition(t *testing.T) {
	t.Parallel()

	dt := NewDeviceTracker()
	idx := track.TrackerIndex(0)

	// Same ordered state
	dt.InsertTexture(idx, track.TextureUsesResource)
	scope := track.NewTextureUsageScope()
	_ = scope.SetUsage(idx, track.TextureUsesResource)

	transitions := dt.MergeTextureScope(scope)
	if len(transitions) != 0 {
		t.Errorf("Expected 0 transitions for same ordered state, got %d", len(transitions))
	}
}

func TestDeviceTracker_TrackPresentTexture(t *testing.T) {
	t.Parallel()

	dt := NewDeviceTracker()
	idx := track.TrackerIndex(0)

	// Start with color target (simulates render pass output)
	dt.InsertTexture(idx, track.TextureUsesColorTarget)

	// Track present transition
	trans := dt.TrackPresentTexture(idx)

	if trans == nil {
		t.Fatal("Expected a transition for present")
	}
	if trans.Usage.From != track.TextureUsesColorTarget {
		t.Errorf("From = %d, want ColorTarget", trans.Usage.From)
	}
	if trans.Usage.To != track.TextureUsesPresent {
		t.Errorf("To = %d, want Present", trans.Usage.To)
	}

	// Device tracker should now be in Present state
	if dt.Textures().GetUsage(idx) != track.TextureUsesPresent {
		t.Error("Device tracker should be in Present state after TrackPresentTexture")
	}
}

func TestDeviceTracker_TrackPresentTexture_AlreadyPresent(t *testing.T) {
	t.Parallel()

	dt := NewDeviceTracker()
	idx := track.TrackerIndex(0)

	// Already in present state (unordered, so barrier is still needed)
	dt.InsertTexture(idx, track.TextureUsesPresent)
	trans := dt.TrackPresentTexture(idx)

	// Present is unordered, so same-to-same still produces a barrier
	if trans == nil {
		t.Fatal("Expected a transition for same unordered state")
	}
}

func TestDeviceTracker_MultipleTextures(t *testing.T) {
	t.Parallel()

	dt := NewDeviceTracker()
	idx0 := track.TrackerIndex(0)
	idx1 := track.TrackerIndex(1)
	idx2 := track.TrackerIndex(2)

	dt.InsertTexture(idx0, track.TextureUsesColorTarget)
	dt.InsertTexture(idx1, track.TextureUsesResource)
	dt.InsertTexture(idx2, track.TextureUsesResource)

	scope := track.NewTextureUsageScope()
	_ = scope.SetUsage(idx0, track.TextureUsesCopySrc)     // transition needed
	_ = scope.SetUsage(idx1, track.TextureUsesResource)    // same ordered, skip
	_ = scope.SetUsage(idx2, track.TextureUsesColorTarget) // transition needed

	transitions := dt.MergeTextureScope(scope)

	if len(transitions) != 2 {
		t.Fatalf("Expected 2 transitions, got %d", len(transitions))
	}
}
