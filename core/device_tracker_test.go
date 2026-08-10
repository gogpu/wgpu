//go:build !(js && wasm)

package core

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/core/track"
	"github.com/gogpu/wgpu/hal"
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

// =============================================================================
// ADR-060: Submit-time barrier generation (texture tracker wiring)
// =============================================================================

// TestDeviceTracker_BarrierCBFromTransitions_SkipsNilTextures verifies that
// BarrierCBFromTransitions safely skips destroyed textures (nil resolver).
func TestDeviceTracker_BarrierCBFromTransitions_SkipsNilTextures(t *testing.T) {
	t.Parallel()

	transitions := []track.TexturePendingTransition{
		{
			Index: track.TrackerIndex(0),
			Usage: track.TextureStateTransition{
				From: track.TextureUsesColorTarget,
				To:   track.TextureUsesCopySrc,
			},
		},
		{
			Index: track.TrackerIndex(1),
			Usage: track.TextureStateTransition{
				From: track.TextureUsesResource,
				To:   track.TextureUsesColorTarget,
			},
		},
	}

	// Resolver returns nil for index 0 (destroyed) and a stub for index 1.
	encoder := &noopEncoder{}
	cb, err := BarrierCBFromTransitions(encoder, transitions, func(idx track.TrackerIndex) hal.Texture {
		if idx == 1 {
			return &noopTexture{}
		}
		return nil
	})

	if err != nil {
		t.Fatalf("BarrierCBFromTransitions error: %v", err)
	}
	if cb == nil {
		t.Fatal("Expected non-nil command buffer")
	}
	// Verify that TransitionTextures was called with 1 barrier (not 2)
	if encoder.transitionCount != 1 {
		t.Errorf("TransitionTextures barrier count = %d, want 1 (skipped nil texture)", encoder.transitionCount)
	}
}

// TestDeviceTracker_BarrierCBFromTransitions_EmptyAfterFiltering verifies that
// when all textures resolve to nil, TransitionTextures is not called.
func TestDeviceTracker_BarrierCBFromTransitions_EmptyAfterFiltering(t *testing.T) {
	t.Parallel()

	transitions := []track.TexturePendingTransition{
		{
			Index: track.TrackerIndex(0),
			Usage: track.TextureStateTransition{
				From: track.TextureUsesColorTarget,
				To:   track.TextureUsesCopySrc,
			},
		},
	}

	encoder := &noopEncoder{}
	cb, err := BarrierCBFromTransitions(encoder, transitions, func(_ track.TrackerIndex) hal.Texture {
		return nil
	})

	if err != nil {
		t.Fatalf("BarrierCBFromTransitions error: %v", err)
	}
	if cb == nil {
		t.Fatal("Expected non-nil command buffer even with all nil textures")
	}
	if encoder.transitionCount != 0 {
		t.Errorf("TransitionTextures should not be called when all textures are nil, got %d", encoder.transitionCount)
	}
}

// TestDeviceTracker_PresentTransitionFlow simulates the full ADR-060 flow:
// 1. Texture starts as ColorTarget (render pass output)
// 2. Command buffer scope records ColorTarget usage
// 3. MergeTextureScope produces no transition (same state)
// 4. TrackPresentTexture produces ColorTarget->Present transition
func TestDeviceTracker_PresentTransitionFlow(t *testing.T) {
	t.Parallel()

	dt := NewDeviceTracker()
	idx := track.TrackerIndex(0)

	// Step 1: Texture acquired from swapchain, starts as ColorTarget
	dt.InsertTexture(idx, track.TextureUsesColorTarget)

	// Step 2: Command buffer records ColorTarget usage
	scope := track.NewTextureUsageScope()
	_ = scope.SetUsage(idx, track.TextureUsesColorTarget)

	// Step 3: Merge — same ordered state, no barrier needed
	transitions := dt.MergeTextureScope(scope)
	if len(transitions) != 0 {
		t.Fatalf("Step 3: Expected 0 transitions for same ColorTarget, got %d", len(transitions))
	}

	// Step 4: Track present transition
	trans := dt.TrackPresentTexture(idx)
	if trans == nil {
		t.Fatal("Step 4: Expected ColorTarget->Present transition")
	}
	if trans.Usage.From != track.TextureUsesColorTarget {
		t.Errorf("Step 4: From = %d, want ColorTarget", trans.Usage.From)
	}
	if trans.Usage.To != track.TextureUsesPresent {
		t.Errorf("Step 4: To = %d, want Present", trans.Usage.To)
	}

	// Device tracker should now be in Present state
	if dt.Textures().GetUsage(idx) != track.TextureUsesPresent {
		t.Error("Device tracker should be in Present state after TrackPresentTexture")
	}
}

// =============================================================================
// ADR-060: Submit-time barrier injection flow tests
// =============================================================================

// TestDeviceTracker_SubmitFlowSingleCB simulates the full submit-time barrier
// injection flow with one command buffer:
// 1. Create device with DeviceTracker
// 2. Allocate a texture with TrackerIndex
// 3. Insert texture into device tracker (initial state)
// 4. Build a TextureUsageScope (simulating populateTextureScope)
// 5. Merge scope into device tracker
// 6. Verify transitions are generated when state changes
func TestDeviceTracker_SubmitFlowSingleCB(t *testing.T) {
	t.Parallel()

	dt := NewDeviceTracker()
	idx := track.TrackerIndex(0)

	// Texture starts as UNINITIALIZED (fresh from swapchain acquire).
	dt.InsertTexture(idx, track.TextureUsesUninitialized)

	// Command buffer uses it as COLOR_TARGET (render pass attachment).
	scope := track.NewTextureUsageScope()
	_ = scope.SetUsage(idx, track.TextureUsesColorTarget)

	// Merge scope — should produce UNINITIALIZED -> COLOR_TARGET transition.
	transitions := dt.MergeTextureScope(scope)
	if len(transitions) != 1 {
		t.Fatalf("Expected 1 transition, got %d", len(transitions))
	}
	if transitions[0].Usage.From != track.TextureUsesUninitialized {
		t.Errorf("From = %d, want Uninitialized", transitions[0].Usage.From)
	}
	if transitions[0].Usage.To != track.TextureUsesColorTarget {
		t.Errorf("To = %d, want ColorTarget", transitions[0].Usage.To)
	}

	// Device tracker should now be in ColorTarget state.
	if dt.Textures().GetUsage(idx) != track.TextureUsesColorTarget {
		t.Error("Device tracker should be in ColorTarget state")
	}

	// Second submit with same usage — no barrier (same ordered state).
	scope2 := track.NewTextureUsageScope()
	_ = scope2.SetUsage(idx, track.TextureUsesColorTarget)
	transitions2 := dt.MergeTextureScope(scope2)
	if len(transitions2) != 0 {
		t.Errorf("Expected 0 transitions for same ordered state, got %d", len(transitions2))
	}
}

// TestDeviceTracker_SubmitFlowMultipleCBs simulates merging scopes from
// multiple command buffers in one submit, verifying that the device tracker
// accumulates state changes correctly.
func TestDeviceTracker_SubmitFlowMultipleCBs(t *testing.T) {
	t.Parallel()

	dt := NewDeviceTracker()
	idxA := track.TrackerIndex(0)
	idxB := track.TrackerIndex(1)

	// Both textures start as RESOURCE (previously used for sampling).
	dt.InsertTexture(idxA, track.TextureUsesResource)
	dt.InsertTexture(idxB, track.TextureUsesResource)

	// First CB: uses A as ColorTarget
	scope1 := track.NewTextureUsageScope()
	_ = scope1.SetUsage(idxA, track.TextureUsesColorTarget)

	// Second CB: uses B as CopyDst
	scope2 := track.NewTextureUsageScope()
	_ = scope2.SetUsage(idxB, track.TextureUsesCopyDst)

	// Merge both scopes (simulating Submit loop)
	trans1 := dt.MergeTextureScope(scope1)
	trans2 := dt.MergeTextureScope(scope2)

	all := make([]track.TexturePendingTransition, 0, len(trans1)+len(trans2))
	all = append(all, trans1...)
	all = append(all, trans2...)
	if len(all) != 2 {
		t.Fatalf("Expected 2 transitions from 2 CBs, got %d", len(all))
	}

	// Verify final states
	if dt.Textures().GetUsage(idxA) != track.TextureUsesColorTarget {
		t.Error("Texture A should be in ColorTarget state")
	}
	if dt.Textures().GetUsage(idxB) != track.TextureUsesCopyDst {
		t.Error("Texture B should be in CopyDst state")
	}
}

// TestDeviceTracker_SubmitFlowEmptyScope verifies that an empty scope
// produces no transitions and does not affect the device tracker.
func TestDeviceTracker_SubmitFlowEmptyScope(t *testing.T) {
	t.Parallel()

	dt := NewDeviceTracker()
	idx := track.TrackerIndex(0)
	dt.InsertTexture(idx, track.TextureUsesColorTarget)

	scope := track.NewTextureUsageScope()
	if !scope.IsEmpty() {
		t.Fatal("New scope should be empty")
	}

	transitions := dt.MergeTextureScope(scope)
	if len(transitions) != 0 {
		t.Errorf("Empty scope should produce 0 transitions, got %d", len(transitions))
	}

	// State should be unchanged
	if dt.Textures().GetUsage(idx) != track.TextureUsesColorTarget {
		t.Error("Device tracker state should be unchanged after empty scope merge")
	}
}

// TestDeviceTracker_SubmitFlowNewTextureAutoInsert verifies that when a scope
// references a texture not yet in the device tracker, it is auto-inserted
// without producing a transition.
func TestDeviceTracker_SubmitFlowNewTextureAutoInsert(t *testing.T) {
	t.Parallel()

	dt := NewDeviceTracker()
	idx := track.TrackerIndex(42)

	// Scope references a texture not yet tracked
	scope := track.NewTextureUsageScope()
	_ = scope.SetUsage(idx, track.TextureUsesColorTarget)

	// Should auto-insert without transition
	transitions := dt.MergeTextureScope(scope)
	if len(transitions) != 0 {
		t.Errorf("Expected 0 transitions for auto-insert, got %d", len(transitions))
	}

	// Texture should now be tracked
	if !dt.Textures().IsTracked(idx) {
		t.Error("Texture should be tracked after auto-insert")
	}
	if dt.Textures().GetUsage(idx) != track.TextureUsesColorTarget {
		t.Errorf("Usage = %d, want ColorTarget", dt.Textures().GetUsage(idx))
	}
}

// TestDeviceTracker_BarrierCBFromTransitions_FullFlow verifies the complete
// barrier CB generation: transitions -> HAL barriers -> command buffer.
func TestDeviceTracker_BarrierCBFromTransitions_FullFlow(t *testing.T) {
	t.Parallel()

	dt := NewDeviceTracker()
	idx0 := track.TrackerIndex(0)
	idx1 := track.TrackerIndex(1)

	dt.InsertTexture(idx0, track.TextureUsesResource)
	dt.InsertTexture(idx1, track.TextureUsesResource)

	scope := track.NewTextureUsageScope()
	_ = scope.SetUsage(idx0, track.TextureUsesColorTarget) // Resource->ColorTarget
	_ = scope.SetUsage(idx1, track.TextureUsesResource)    // same, skip

	transitions := dt.MergeTextureScope(scope)
	if len(transitions) != 1 {
		t.Fatalf("Expected 1 transition, got %d", len(transitions))
	}

	tex0 := &noopTexture{}
	tex1 := &noopTexture{}
	encoder := &noopEncoder{}
	cb, err := BarrierCBFromTransitions(encoder, transitions, func(idx track.TrackerIndex) hal.Texture {
		switch idx {
		case 0:
			return tex0
		case 1:
			return tex1
		default:
			return nil
		}
	})

	if err != nil {
		t.Fatalf("BarrierCBFromTransitions: %v", err)
	}
	if cb == nil {
		t.Fatal("Expected non-nil barrier command buffer")
	}
	if encoder.transitionCount != 1 {
		t.Errorf("TransitionTextures called with %d barriers, want 1", encoder.transitionCount)
	}
}

// noopEncoder is a minimal hal.CommandEncoder for testing BarrierCBFromTransitions.
type noopEncoder struct {
	transitionCount int
}

func (e *noopEncoder) BeginEncoding(_ string) error            { return nil }
func (e *noopEncoder) EndEncoding() (hal.CommandBuffer, error) { return &noopCB{}, nil }
func (e *noopEncoder) DiscardEncoding()                        {}
func (e *noopEncoder) ResetAll(_ []hal.CommandBuffer)          {}
func (e *noopEncoder) Destroy()                                {}
func (e *noopEncoder) CopyBufferToBuffer(_, _ hal.Buffer, _ []hal.BufferCopy) {
}
func (e *noopEncoder) CopyTextureToTexture(_ hal.Texture, _ hal.Texture, _ []hal.TextureCopy) {
}
func (e *noopEncoder) CopyBufferToTexture(_ hal.Buffer, _ hal.Texture, _ []hal.BufferTextureCopy) {
}
func (e *noopEncoder) CopyTextureToBuffer(_ hal.Texture, _ hal.Buffer, _ []hal.BufferTextureCopy) {
}
func (e *noopEncoder) ClearBuffer(_ hal.Buffer, _ uint64, _ uint64) {}
func (e *noopEncoder) BeginRenderPass(_ *hal.RenderPassDescriptor) hal.RenderPassEncoder {
	return nil
}
func (e *noopEncoder) BeginComputePass(_ *hal.ComputePassDescriptor) hal.ComputePassEncoder {
	return nil
}
func (e *noopEncoder) TransitionBuffers(_ []hal.BufferBarrier) {}
func (e *noopEncoder) TransitionTextures(barriers []hal.TextureBarrier) {
	e.transitionCount = len(barriers)
}
func (e *noopEncoder) ResolveQuerySet(_ hal.QuerySet, _ uint32, _ uint32, _ hal.Buffer, _ uint64) {
}

// noopCB implements hal.CommandBuffer for testing.
type noopCB struct{}

func (c *noopCB) Destroy() {}

// noopTexture implements hal.Texture for testing.
type noopTexture struct{}

func (t *noopTexture) NativeHandle() uintptr               { return 0 }
func (t *noopTexture) Destroy()                            {}
func (t *noopTexture) CurrentUsage() gputypes.TextureUsage { return 0 }
func (t *noopTexture) AddPendingRef()                      {}
func (t *noopTexture) DecPendingRef()                      {}
