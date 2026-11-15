//go:build !(js && wasm)

package track

import (
	"testing"

	"github.com/gogpu/gputypes"
)

// TestEncoderTextureTracker_NeverConflicts is the core of the WebGPU usage
// scope rule: consecutive commands may use a texture with "incompatible"
// usages. The command buffer tracker returns transitions, never errors.
func TestEncoderTextureTracker_NeverConflicts(t *testing.T) {
	t.Parallel()

	tr := NewEncoderTextureTracker()
	const idx TrackerIndex = 3

	if _, needed := tr.SetSingle(idx, TextureUsesColorTarget); needed {
		t.Fatal("first use must not produce a transition (covered at submit)")
	}
	transition, needed := tr.SetSingle(idx, TextureUsesCopySrc)
	if !needed {
		t.Fatal("ColorTarget -> CopySrc must produce a transition")
	}
	if transition.Index != idx || transition.Usage.From != TextureUsesColorTarget || transition.Usage.To != TextureUsesCopySrc {
		t.Fatalf("transition = %+v, want %d ColorTarget -> CopySrc", transition, idx)
	}
	if got := tr.StartUsage(idx); got != TextureUsesColorTarget {
		t.Errorf("start = %v, want ColorTarget", got)
	}
	if got := tr.EndUsage(idx); got != TextureUsesCopySrc {
		t.Errorf("end = %v, want CopySrc", got)
	}
}

func TestEncoderTextureTracker_SkipsOrderedSameState(t *testing.T) {
	t.Parallel()

	tr := NewEncoderTextureTracker()
	tr.SetSingle(0, TextureUsesColorTarget)
	if _, needed := tr.SetSingle(0, TextureUsesColorTarget); needed {
		t.Error("ColorTarget -> ColorTarget is ordered, no transition expected")
	}
	tr.SetSingle(1, TextureUsesStorageWrite)
	if _, needed := tr.SetSingle(1, TextureUsesStorageWrite); !needed {
		t.Error("StorageWrite -> StorageWrite is unordered, a transition is required")
	}
}

func TestEncoderTextureTracker_SetFromScope(t *testing.T) {
	t.Parallel()

	tr := NewEncoderTextureTracker()
	tr.SetSingle(2, TextureUsesCopyDst) // earlier upload

	pass := NewTextureUsageScope()
	if err := pass.SetUsage(2, TextureUsesColorTarget); err != nil {
		t.Fatal(err)
	}
	if err := pass.SetUsage(5, TextureUsesDepthStencilWrite); err != nil {
		t.Fatal(err)
	}
	transitions := tr.SetFromScope(pass)
	if len(transitions) != 1 {
		t.Fatalf("transitions = %+v, want only the CopyDst -> ColorTarget one", transitions)
	}
	if got := transitions[0].Usage; got.From != TextureUsesCopyDst || got.To != TextureUsesColorTarget {
		t.Errorf("transition = %v -> %v, want CopyDst -> ColorTarget", got.From, got.To)
	}
	if got := tr.StartUsage(5); got != TextureUsesDepthStencilWrite {
		t.Errorf("new attachment start = %v, want DepthStencilWrite", got)
	}
	if tr.SetFromScope(nil) != nil {
		t.Error("nil scope must produce no transitions")
	}
}

func TestEncoderTextureTracker_RecordTransition(t *testing.T) {
	t.Parallel()

	tr := NewEncoderTextureTracker()
	tr.RecordTransition(0, TextureUsesColorTarget, TextureUsesCopySrc)
	if tr.StartUsage(0) != TextureUsesColorTarget || tr.EndUsage(0) != TextureUsesCopySrc {
		t.Errorf("first explicit transition: start=%v end=%v, want ColorTarget/CopySrc", tr.StartUsage(0), tr.EndUsage(0))
	}
	tr.RecordTransition(0, TextureUsesCopySrc, TextureUsesColorTarget)
	if tr.StartUsage(0) != TextureUsesColorTarget || tr.EndUsage(0) != TextureUsesColorTarget {
		t.Errorf("second explicit transition: start=%v end=%v, want ColorTarget/ColorTarget", tr.StartUsage(0), tr.EndUsage(0))
	}
	tr.RecordTransition(1, TextureUsesNone, TextureUsesCopyDst)
	if got := tr.StartUsage(1); got != TextureUsesCopyDst {
		t.Errorf("undefined old usage: start = %v, want CopyDst", got)
	}
}

func TestEncoderTextureTracker_Queries(t *testing.T) {
	t.Parallel()

	tr := NewEncoderTextureTracker()
	if !tr.IsEmpty() || tr.IsUsed(0) || tr.StartUsage(7) != TextureUsesNone || tr.EndUsage(7) != TextureUsesNone {
		t.Fatal("empty tracker reports usage")
	}
	tr.SetSingle(4, TextureUsesResource)
	if tr.IsEmpty() || !tr.IsUsed(4) || tr.IsUsed(3) {
		t.Fatal("tracker usage queries are wrong after SetSingle")
	}
}

// TestTextureTracker_MergeEncoder checks the submit-time contract: barrier
// from the device state into the command buffer's START state, then adopt
// its END state.
func TestTextureTracker_MergeEncoder(t *testing.T) {
	t.Parallel()

	device := NewTextureTracker()
	device.InsertSingle(0, TextureUsesCopySrc)     // left by the previous frame
	device.InsertSingle(1, TextureUsesColorTarget) // already in start state

	cmd := NewEncoderTextureTracker()
	cmd.SetSingle(0, TextureUsesColorTarget) // render ...
	cmd.SetSingle(0, TextureUsesCopySrc)     // ... then read back
	cmd.SetSingle(1, TextureUsesColorTarget)
	cmd.SetSingle(2, TextureUsesCopyDst) // unknown to the device

	transitions := device.MergeEncoder(cmd)
	if len(transitions) != 1 {
		t.Fatalf("transitions = %+v, want exactly one", transitions)
	}
	if got := transitions[0]; got.Index != 0 || got.Usage.From != TextureUsesCopySrc || got.Usage.To != TextureUsesColorTarget {
		t.Errorf("transition = %+v, want 0 CopySrc -> ColorTarget (into the start state)", got)
	}
	if got := device.GetUsage(0); got != TextureUsesCopySrc {
		t.Errorf("device state 0 = %v, want end state CopySrc", got)
	}
	if got := device.GetUsage(2); got != TextureUsesCopyDst {
		t.Errorf("device state 2 = %v, want inserted end state CopyDst", got)
	}
	if device.MergeEncoder(nil) != nil {
		t.Error("nil command tracker must produce no transitions")
	}
}

func TestEncoderBufferTracker(t *testing.T) {
	t.Parallel()

	tr := NewEncoderBufferTracker()
	if !tr.IsEmpty() || tr.StartUsage(0) != BufferUsesNone || tr.EndUsage(0) != BufferUsesNone {
		t.Fatal("empty tracker reports usage")
	}
	if _, needed := tr.SetSingle(0, BufferUsesCopyDst); needed {
		t.Fatal("first use must not produce a transition")
	}
	transition, needed := tr.SetSingle(0, BufferUsesVertex)
	if !needed || transition.Usage.From != BufferUsesCopyDst || transition.Usage.To != BufferUsesVertex {
		t.Fatalf("CopyDst -> Vertex = (%+v, %v), want a transition", transition, needed)
	}
	if _, needed := tr.SetSingle(0, BufferUsesVertex); needed {
		t.Error("Vertex -> Vertex is read-only and unchanged, no transition expected")
	}

	pass := NewBufferUsageScope()
	_ = pass.SetUsage(0, BufferUsesIndex)
	_ = pass.SetUsage(1, BufferUsesUniform)
	if got := tr.SetFromScope(pass); len(got) != 1 || got[0].Usage.To != BufferUsesIndex {
		t.Errorf("SetFromScope transitions = %+v, want Vertex -> Index only", got)
	}
	if tr.SetFromScope(nil) != nil {
		t.Error("nil scope must produce no transitions")
	}
	if tr.StartUsage(0) != BufferUsesCopyDst || tr.EndUsage(0) != BufferUsesIndex || !tr.IsUsed(1) {
		t.Errorf("start/end = %v/%v, want CopyDst/Index", tr.StartUsage(0), tr.EndUsage(0))
	}
}

func TestBufferTracker_MergeEncoder(t *testing.T) {
	t.Parallel()

	device := NewBufferTracker()
	device.InsertSingle(0, BufferUsesVertex)
	device.InsertSingle(1, BufferUsesStorageWrite)

	cmd := NewEncoderBufferTracker()
	cmd.SetSingle(0, BufferUsesCopyDst)
	cmd.SetSingle(0, BufferUsesVertex)
	cmd.SetSingle(1, BufferUsesStorageWrite) // unordered: barrier even if unchanged
	cmd.SetSingle(2, BufferUsesUniform)

	transitions := device.MergeEncoder(cmd)
	if len(transitions) != 2 {
		t.Fatalf("transitions = %+v, want 2", transitions)
	}
	if got := transitions[0].Usage; got.From != BufferUsesVertex || got.To != BufferUsesCopyDst {
		t.Errorf("buffer 0 transition = %v -> %v, want Vertex -> CopyDst", got.From, got.To)
	}
	if got := device.GetUsage(0); got != BufferUsesVertex {
		t.Errorf("device state 0 = %v, want end state Vertex", got)
	}
	if got := device.GetUsage(2); got != BufferUsesUniform {
		t.Errorf("device state 2 = %v, want inserted Uniform", got)
	}
	if device.MergeEncoder(nil) != nil {
		t.Error("nil command tracker must produce no transitions")
	}
}

func TestSkipBufferBarrier(t *testing.T) {
	t.Parallel()

	if !SkipBufferBarrier(BufferUsesVertex, BufferUsesVertex) {
		t.Error("unchanged read-only state needs no barrier")
	}
	if SkipBufferBarrier(BufferUsesCopyDst, BufferUsesCopyDst) {
		t.Error("unchanged write state still needs a barrier")
	}
	if SkipBufferBarrier(BufferUsesVertex, BufferUsesIndex) {
		t.Error("state change needs a barrier")
	}
}

func TestTextureUsesFromUsage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		usage        gputypes.TextureUsage
		depthStencil bool
		want         TextureUses
	}{
		{0, false, TextureUsesNone},
		{gputypes.TextureUsageCopySrc, false, TextureUsesCopySrc},
		{gputypes.TextureUsageCopyDst, false, TextureUsesCopyDst},
		{gputypes.TextureUsageTextureBinding, false, TextureUsesResource},
		{gputypes.TextureUsageStorageBinding, false, TextureUsesStorageWrite},
		{gputypes.TextureUsageRenderAttachment, false, TextureUsesColorTarget},
		{gputypes.TextureUsageRenderAttachment, true, TextureUsesDepthStencilWrite},
		{gputypes.TextureUsageCopySrc | gputypes.TextureUsageTextureBinding, false, TextureUsesCopySrc | TextureUsesResource},
	}
	for _, tt := range tests {
		if got := TextureUsesFromUsage(tt.usage, tt.depthStencil); got != tt.want {
			t.Errorf("TextureUsesFromUsage(%v, %v) = %v, want %v", tt.usage, tt.depthStencil, got, tt.want)
		}
	}
}

func TestTrackerIndexAt(t *testing.T) {
	t.Parallel()

	if idx, ok := trackerIndexAt(5); !ok || idx != 5 {
		t.Errorf("trackerIndexAt(5) = %d, %v", idx, ok)
	}
	if _, ok := trackerIndexAt(-1); ok {
		t.Error("negative position accepted")
	}
}
