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

// noopEncoder is a minimal hal.CommandEncoder for testing barrier generation.
type noopEncoder struct {
	transitionCount       int
	bufferTransitionCount int
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
func (e *noopEncoder) TransitionBuffers(barriers []hal.BufferBarrier) {
	e.bufferTransitionCount = len(barriers)
}
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

// noopHALBuffer implements hal.Buffer for testing.
type noopHALBuffer struct{}

func (b *noopHALBuffer) NativeHandle() uintptr { return 0 }
func (b *noopHALBuffer) Destroy()              {}

// =============================================================================
// Task #13: Buffer Tracker Integration Tests
// =============================================================================

func TestDeviceTracker_NewHasBufferTracker(t *testing.T) {
	t.Parallel()

	dt := NewDeviceTracker()
	if dt.Buffers() == nil {
		t.Fatal("Buffers() returned nil")
	}
	if dt.Buffers().Size() != 0 {
		t.Errorf("Initial buffer tracker size = %d, want 0", dt.Buffers().Size())
	}
}

func TestDeviceTracker_InsertRemoveBuffer(t *testing.T) {
	t.Parallel()

	dt := NewDeviceTracker()
	idx := track.TrackerIndex(0)

	dt.InsertBuffer(idx, track.BufferUsesVertex)
	if dt.Buffers().Size() != 1 {
		t.Errorf("Size after insert = %d, want 1", dt.Buffers().Size())
	}
	if !dt.Buffers().IsTracked(idx) {
		t.Error("Buffer should be tracked after insert")
	}

	dt.RemoveBuffer(idx)
	if dt.Buffers().IsTracked(idx) {
		t.Error("Buffer should not be tracked after remove")
	}
}

func TestDeviceTracker_MergeBufferScope_NilSafe(t *testing.T) {
	t.Parallel()

	dt := NewDeviceTracker()
	transitions := dt.MergeBufferScope(nil)
	if len(transitions) != 0 {
		t.Errorf("Nil scope should produce 0 transitions, got %d", len(transitions))
	}
}

func TestDeviceTracker_MergeBufferScope_Transition(t *testing.T) {
	t.Parallel()

	dt := NewDeviceTracker()
	idx := track.TrackerIndex(0)

	// Register buffer with initial vertex usage
	dt.InsertBuffer(idx, track.BufferUsesVertex)

	// Command buffer uses it as copy destination
	scope := track.NewBufferUsageScope()
	_ = scope.SetUsage(idx, track.BufferUsesCopyDst)

	transitions := dt.MergeBufferScope(scope)

	if len(transitions) != 1 {
		t.Fatalf("Expected 1 transition, got %d", len(transitions))
	}
	if transitions[0].Usage.From != track.BufferUsesVertex {
		t.Errorf("From = %d, want Vertex", transitions[0].Usage.From)
	}
	if transitions[0].Usage.To != track.BufferUsesCopyDst {
		t.Errorf("To = %d, want CopyDst", transitions[0].Usage.To)
	}
}

func TestDeviceTracker_MergeBufferScope_NoTransition_SameState(t *testing.T) {
	t.Parallel()

	dt := NewDeviceTracker()
	idx := track.TrackerIndex(0)

	dt.InsertBuffer(idx, track.BufferUsesVertex)
	scope := track.NewBufferUsageScope()
	_ = scope.SetUsage(idx, track.BufferUsesVertex)

	transitions := dt.MergeBufferScope(scope)
	if len(transitions) != 0 {
		t.Errorf("Expected 0 transitions for same state, got %d", len(transitions))
	}
}

func TestDeviceTracker_MergeBufferScope_NewBufferAutoInsert(t *testing.T) {
	t.Parallel()

	dt := NewDeviceTracker()
	idx := track.TrackerIndex(42)

	scope := track.NewBufferUsageScope()
	_ = scope.SetUsage(idx, track.BufferUsesUniform)

	transitions := dt.MergeBufferScope(scope)
	if len(transitions) != 0 {
		t.Errorf("Expected 0 transitions for auto-insert, got %d", len(transitions))
	}
	if !dt.Buffers().IsTracked(idx) {
		t.Error("Buffer should be tracked after auto-insert")
	}
	if dt.Buffers().GetUsage(idx) != track.BufferUsesUniform {
		t.Errorf("Usage = %d, want Uniform", dt.Buffers().GetUsage(idx))
	}
}

// TestBuffer_TrackerIndex_Allocated verifies that NewBuffer allocates a
// real TrackerIndex from the device's buffer allocator.
func TestBuffer_TrackerIndex_Allocated(t *testing.T) {
	t.Parallel()

	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	buf := NewBuffer(mockBuffer{}, device, gputypes.BufferUsageVertex, 256, "buf")
	td := buf.TrackingData()
	if td == nil {
		t.Fatal("TrackingData() returned nil")
	}
	if !td.Index().IsValid() {
		t.Error("Tracker index should be valid (allocated from device)")
	}
}

// TestBuffer_TrackerIndex_Freed verifies that releasing tracking data
// frees the index for reuse.
func TestBuffer_TrackerIndex_Freed(t *testing.T) {
	t.Parallel()

	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	buf := NewBuffer(mockBuffer{}, device, gputypes.BufferUsageVertex, 256, "buf")
	td := buf.TrackingData()
	idx := td.Index()
	if !idx.IsValid() {
		t.Fatal("Expected valid index")
	}

	// Release should make the index recyclable
	td.Release()
	if !td.IsReleased() {
		t.Error("TrackingData should be released")
	}

	// Allocate again — should reuse the freed index
	buf2 := NewBuffer(mockBuffer{}, device, gputypes.BufferUsageVertex, 256, "buf2")
	td2 := buf2.TrackingData()
	if td2.Index() != idx {
		t.Errorf("Expected reused index %d, got %d", idx, td2.Index())
	}
}

// TestSubmit_MergesBufferScope verifies that the DeviceTracker merges
// buffer scopes and produces transitions.
func TestSubmit_MergesBufferScope(t *testing.T) {
	t.Parallel()

	dt := NewDeviceTracker()
	idx := track.TrackerIndex(0)

	// Buffer starts as vertex
	dt.InsertBuffer(idx, track.BufferUsesVertex)

	// Scope: now used as copy source
	scope := track.NewBufferUsageScope()
	_ = scope.SetUsage(idx, track.BufferUsesCopySrc)

	transitions := dt.MergeBufferScope(scope)
	if len(transitions) != 1 {
		t.Fatalf("Expected 1 transition, got %d", len(transitions))
	}

	// After merge, device tracker should have the new state
	if dt.Buffers().GetUsage(idx) != track.BufferUsesCopySrc {
		t.Error("Device tracker should be in CopySrc state after merge")
	}
}

// TestSubmit_BufferTransition_GeneratesBarrier verifies that buffer
// transitions are correctly converted to HAL barriers.
func TestSubmit_BufferTransition_GeneratesBarrier(t *testing.T) {
	t.Parallel()

	dt := NewDeviceTracker()
	idx := track.TrackerIndex(0)

	dt.InsertBuffer(idx, track.BufferUsesVertex)

	scope := track.NewBufferUsageScope()
	_ = scope.SetUsage(idx, track.BufferUsesCopyDst)

	transitions := dt.MergeBufferScope(scope)
	if len(transitions) != 1 {
		t.Fatalf("Expected 1 transition, got %d", len(transitions))
	}

	buf0 := &noopHALBuffer{}
	encoder := &noopEncoder{}
	cb, err := BarrierCBFromAllTransitions(
		encoder,
		nil, // no texture transitions
		transitions,
		nil, // no texture resolver
		func(idx track.TrackerIndex) hal.Buffer {
			if idx == 0 {
				return buf0
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("BarrierCBFromAllTransitions: %v", err)
	}
	if cb == nil {
		t.Fatal("Expected non-nil barrier command buffer")
	}
	if encoder.bufferTransitionCount != 1 {
		t.Errorf("TransitionBuffers called with %d barriers, want 1", encoder.bufferTransitionCount)
	}
}

// TestBarrierCBFromAllTransitions_BothTypes verifies that a single barrier
// CB can contain both texture and buffer barriers.
func TestBarrierCBFromAllTransitions_BothTypes(t *testing.T) {
	t.Parallel()

	texTransitions := []track.TexturePendingTransition{
		{
			Index: track.TrackerIndex(0),
			Usage: track.TextureStateTransition{
				From: track.TextureUsesResource,
				To:   track.TextureUsesColorTarget,
			},
		},
	}
	bufTransitions := []track.PendingTransition{
		{
			Index: track.TrackerIndex(0),
			Usage: track.StateTransition{
				From: track.BufferUsesVertex,
				To:   track.BufferUsesCopyDst,
			},
		},
	}

	encoder := &noopEncoder{}
	cb, err := BarrierCBFromAllTransitions(
		encoder,
		texTransitions,
		bufTransitions,
		func(_ track.TrackerIndex) hal.Texture { return &noopTexture{} },
		func(_ track.TrackerIndex) hal.Buffer { return &noopHALBuffer{} },
	)
	if err != nil {
		t.Fatalf("BarrierCBFromAllTransitions: %v", err)
	}
	if cb == nil {
		t.Fatal("Expected non-nil barrier command buffer")
	}
	if encoder.transitionCount != 1 {
		t.Errorf("texture transitions = %d, want 1", encoder.transitionCount)
	}
	if encoder.bufferTransitionCount != 1 {
		t.Errorf("buffer transitions = %d, want 1", encoder.bufferTransitionCount)
	}
}

// =============================================================================
// Task #14: Usage Conflict Validation Tests
// =============================================================================

// TestBeginRenderPass_TextureConflict_Error verifies that using a texture as
// both COLOR_TARGET and RESOURCE in the same render pass produces an error.
func TestBeginRenderPass_TextureConflict_Error(t *testing.T) {
	t.Parallel()

	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	// Create a texture with a valid tracker index
	alloc := device.TrackerIndices().Textures()
	td := track.NewTrackingData(alloc)

	tex := &Texture{
		raw:          NewSnatchable[hal.Texture](nil),
		device:       device,
		trackingData: td,
	}

	view := &TextureView{Parent: tex}

	enc, err := device.CreateCommandEncoder("test")
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}

	// Set up a scope conflict: try to use the same texture as COLOR_TARGET
	// through two different color attachments, which is actually allowed
	// (same exclusive usage). So instead, manually set up the scope first.
	// First, mark the texture as RESOURCE in the scope.
	enc.RecordBufferUsage(nil, 0) // no-op, just to access mutable
	_ = enc.Mutable().TextureScope().SetUsage(td.Index(), track.TextureUsesResource)

	// Now BeginRenderPass tries to add COLOR_TARGET to the same texture.
	// This should produce a conflict error (RESOURCE + COLOR_TARGET).
	desc := &RenderPassDescriptor{
		ColorAttachments: []RenderPassColorAttachment{
			{View: view, LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore},
		},
	}
	_, passErr := enc.BeginRenderPass(desc)
	if passErr == nil {
		t.Fatal("Expected texture usage conflict error, got nil")
	}
}

// TestBeginRenderPass_NoConflict_SameUsage verifies that using a texture as
// COLOR_TARGET in two color attachments is allowed (same exclusive usage).
func TestBeginRenderPass_NoConflict_SameUsage(t *testing.T) {
	t.Parallel()

	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	alloc := device.TrackerIndices().Textures()
	td := track.NewTrackingData(alloc)

	tex := &Texture{
		raw:          NewSnatchable[hal.Texture](nil),
		device:       device,
		trackingData: td,
	}
	view := &TextureView{Parent: tex}

	enc, err := device.CreateCommandEncoder("test")
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}

	// Two color attachments pointing to the same texture (same exclusive usage).
	// This should NOT produce a conflict.
	desc := &RenderPassDescriptor{
		ColorAttachments: []RenderPassColorAttachment{
			{View: view, LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore},
			{View: view, LoadOp: gputypes.LoadOpLoad, StoreOp: gputypes.StoreOpStore},
		},
	}
	_, passErr := enc.BeginRenderPass(desc)
	if passErr != nil {
		t.Fatalf("Unexpected error for same-usage: %v", passErr)
	}
}

// TestBuffer_UsageConflict_Error verifies that using a buffer as both
// STORAGE_WRITE and UNIFORM in the same command buffer produces an error.
func TestBuffer_UsageConflict_Error(t *testing.T) {
	t.Parallel()

	scope := track.NewBufferUsageScope()
	idx := track.TrackerIndex(0)

	// First usage: storage write (exclusive)
	if err := scope.SetUsage(idx, track.BufferUsesStorageWrite); err != nil {
		t.Fatalf("First SetUsage (StorageWrite) should succeed: %v", err)
	}

	// Second usage: uniform (read-only) — incompatible with write
	err := scope.SetUsage(idx, track.BufferUsesUniform)
	if err == nil {
		t.Fatal("Expected usage conflict error for StorageWrite + Uniform, got nil")
	}
}

// TestBuffer_NoConflict_ReadOnly verifies that multiple read-only buffer
// usages in the same scope are compatible.
func TestBuffer_NoConflict_ReadOnly(t *testing.T) {
	t.Parallel()

	scope := track.NewBufferUsageScope()
	idx := track.TrackerIndex(0)

	// Vertex + Index are both read-only — should be compatible
	if err := scope.SetUsage(idx, track.BufferUsesVertex); err != nil {
		t.Fatalf("Vertex usage should succeed: %v", err)
	}
	if err := scope.SetUsage(idx, track.BufferUsesIndex); err != nil {
		t.Fatalf("Index usage should succeed (both read-only): %v", err)
	}

	// Verify merged usage
	usage := scope.GetUsage(idx)
	if usage&track.BufferUsesVertex == 0 {
		t.Error("Expected Vertex flag in merged usage")
	}
	if usage&track.BufferUsesIndex == 0 {
		t.Error("Expected Index flag in merged usage")
	}
}
