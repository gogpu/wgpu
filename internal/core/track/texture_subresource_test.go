//go:build !(js && wasm)

package track

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
)

// selectMipLevel selects one mip level of a texture of 3 levels and 2 layers.
func selectMipLevel(level uint32) TextureSelector {
	return TextureSelector{
		TextureMipLevelCount: 3, TextureArrayLayerCount: 2,
		BaseMipLevel: level, MipLevelCount: 1, ArrayLayerCount: 0,
	}
}

// selectArrayLayer selects one array layer of a texture of 3 levels and 2 layers.
func selectArrayLayer(layer uint32) TextureSelector {
	return TextureSelector{
		TextureMipLevelCount: 3, TextureArrayLayerCount: 2,
		BaseArrayLayer: layer, ArrayLayerCount: 1,
	}
}

func TestTextureSelector_SelectsAll(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		selector TextureSelector
		want     bool
	}{
		{"zero value", TextureSelector{}, true},
		{"counts to the end", TextureSelector{TextureMipLevelCount: 3, TextureArrayLayerCount: 2}, true},
		{"explicit counts", TextureSelector{TextureMipLevelCount: 3, TextureArrayLayerCount: 2, MipLevelCount: 3, ArrayLayerCount: 2}, true},
		{"one level", selectMipLevel(1), false},
		{"one layer", selectArrayLayer(0), false},
		{"from level 1 on", TextureSelector{TextureMipLevelCount: 3, TextureArrayLayerCount: 1, BaseMipLevel: 1}, false},
	}
	for _, c := range cases {
		if got := c.selector.SelectsAll(); got != c.want {
			t.Errorf("%s: SelectsAll = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestTextureUsageScope_SubresourcesDoNotConflict is the rule mipmap
// generation relies on: a pass may draw into one mip level or layer while
// sampling another, and only usages of the same subresource conflict.
func TestTextureUsageScope_SubresourcesDoNotConflict(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name          string
		drawn, sample TextureSelector
		overlap       TextureSelector
		conflictAt    TextureSelector
	}{
		// The conflict names the drawn layers of the first conflicting
		// level: both layers of level 1, or layer 1 of level 0.
		{"mip levels", selectMipLevel(1), selectMipLevel(0), selectMipLevel(1), TextureSelector{
			TextureMipLevelCount: 3, TextureArrayLayerCount: 2,
			BaseMipLevel: 1, MipLevelCount: 1, BaseArrayLayer: 0, ArrayLayerCount: 2,
		}},
		{"array layers", selectArrayLayer(1), selectArrayLayer(0), TextureSelector{}, subresourceSelector(3, 2, 0, 1)},
	} {
		scope := NewTextureUsageScope()
		if err := scope.SetSubresourceUsage(4, c.drawn, TextureUsesColorTarget); err != nil {
			t.Fatalf("%s: attachment: %v", c.name, err)
		}
		if err := scope.SetSubresourceUsage(4, c.sample, TextureUsesResource); err != nil {
			t.Fatalf("%s: sampling another subresource: %v", c.name, err)
		}
		if got := scope.GetUsage(4); got != TextureUsesNone {
			t.Errorf("%s: GetUsage = %v, want None while the subresources differ", c.name, got)
		}
		err := scope.SetSubresourceUsage(4, c.overlap, TextureUsesResource)
		var conflict *TextureUsageConflictError
		if !errors.As(err, &conflict) {
			t.Fatalf("%s: sampling the drawn subresource: err = %v, want a conflict", c.name, err)
		}
		if conflict.Existing != TextureUsesColorTarget || conflict.New != TextureUsesResource {
			t.Errorf("%s: conflict = %+v, want ColorTarget against Resource", c.name, conflict)
		}
		if conflict.Selector != c.conflictAt {
			t.Errorf("%s: conflict on %+v, want %+v", c.name, conflict.Selector, c.conflictAt)
		}
	}

	// A usage of part of a texture whose subresources share one usage
	// conflicts on every subresource it covers.
	whole := NewTextureUsageScope()
	_ = whole.SetSubresourceUsage(2, TextureSelector{}, TextureUsesColorTarget)
	lowerLevels := TextureSelector{
		TextureMipLevelCount: 3, TextureArrayLayerCount: 2,
		BaseMipLevel: 1, MipLevelCount: 2, BaseArrayLayer: 0, ArrayLayerCount: 2,
	}
	var conflict *TextureUsageConflictError
	if err := whole.SetSubresourceUsage(2, lowerLevels, TextureUsesResource); !errors.As(err, &conflict) {
		t.Fatalf("sampling two levels of a drawn texture: err = %v, want a conflict", err)
	}
	if want := lowerLevels; conflict.Selector != want {
		t.Errorf("conflict on %v, want %v", conflict.Selector, want)
	}

	// A refused usage changes nothing, even on the subresources it does
	// not conflict on.
	scope := NewTextureUsageScope()
	_ = scope.SetSubresourceUsage(1, selectMipLevel(1), TextureUsesColorTarget)
	if err := scope.SetSubresourceUsage(1, TextureSelector{}, TextureUsesCopyDst); err == nil {
		t.Fatal("CopyDst over the drawn level: want a conflict")
	}
	if got := scope.states[1].subresourceUsage(0, 0); got != TextureUsesNone {
		t.Errorf("level 0 after a refused usage = %v, want None", got)
	}
}

// TestEncoderTextureTracker_SubresourceTransitions follows the passes of
// mipmap generation through one command buffer: each pass moves only the
// level it draws and the one it samples, each from its own state.
func TestEncoderTextureTracker_SubresourceTransitions(t *testing.T) {
	t.Parallel()
	tracker := NewEncoderTextureTracker()
	const index TrackerIndex = 2

	// The upload: every subresource is CopyDst from the start.
	if got := tracker.AppendSingle(nil, index, TextureSelector{}, TextureUsesCopyDst); len(got) != 0 {
		t.Fatalf("first use: %+v, want no transition", got)
	}
	pass := func(drawn, sampled uint32) []TexturePendingTransition {
		scope := NewTextureUsageScope()
		_ = scope.SetSubresourceUsage(index, selectMipLevel(drawn), TextureUsesColorTarget)
		_ = scope.SetSubresourceUsage(index, selectMipLevel(sampled), TextureUsesResource)
		return tracker.SetFromScope(scope)
	}

	// Level 1 drawn from level 0: one transition each, over both layers.
	want := []TexturePendingTransition{
		{Index: index, Selector: TextureSelector{TextureMipLevelCount: 3, TextureArrayLayerCount: 2, BaseMipLevel: 0, MipLevelCount: 1, ArrayLayerCount: 2},
			Usage: TextureStateTransition{From: TextureUsesCopyDst, To: TextureUsesResource}},
		{Index: index, Selector: TextureSelector{TextureMipLevelCount: 3, TextureArrayLayerCount: 2, BaseMipLevel: 1, MipLevelCount: 1, ArrayLayerCount: 2},
			Usage: TextureStateTransition{From: TextureUsesCopyDst, To: TextureUsesColorTarget}},
	}
	checkTransitions(t, "level 1 from level 0", pass(1, 0), want)

	// Level 2 drawn from level 1: level 1 leaves the color target state;
	// level 0, sampled before, stays as it is.
	want = []TexturePendingTransition{
		{Index: index, Selector: TextureSelector{TextureMipLevelCount: 3, TextureArrayLayerCount: 2, BaseMipLevel: 1, MipLevelCount: 1, ArrayLayerCount: 2},
			Usage: TextureStateTransition{From: TextureUsesColorTarget, To: TextureUsesResource}},
		{Index: index, Selector: TextureSelector{TextureMipLevelCount: 3, TextureArrayLayerCount: 2, BaseMipLevel: 2, MipLevelCount: 1, ArrayLayerCount: 2},
			Usage: TextureStateTransition{From: TextureUsesCopyDst, To: TextureUsesColorTarget}},
	}
	checkTransitions(t, "level 2 from level 1", pass(2, 1), want)

	if got := tracker.end[index].subresourceUsage(0, 1); got != TextureUsesResource {
		t.Errorf("level 0 end = %v, want Resource", got)
	}
	if got := tracker.end[index].subresourceUsage(2, 0); got != TextureUsesColorTarget {
		t.Errorf("level 2 end = %v, want ColorTarget", got)
	}
	if got := tracker.StartUsage(index); got != TextureUsesCopyDst {
		t.Errorf("start = %v, want CopyDst for every subresource", got)
	}

	// Sampling the whole texture moves only level 2: each subresource from
	// its own state, and the state is whole again.
	want = []TexturePendingTransition{
		{Index: index, Selector: TextureSelector{TextureMipLevelCount: 3, TextureArrayLayerCount: 2, BaseMipLevel: 2, MipLevelCount: 1, ArrayLayerCount: 2},
			Usage: TextureStateTransition{From: TextureUsesColorTarget, To: TextureUsesResource}},
	}
	checkTransitions(t, "the whole texture sampled", tracker.AppendSingle(nil, index, TextureSelector{}, TextureUsesResource), want)
	if got := tracker.EndUsage(index); got != TextureUsesResource {
		t.Errorf("end = %v, want Resource for every subresource", got)
	}
}

// TestEncoderTextureTracker_SubresourceFirstUse checks that a subresource's
// first use in a command buffer becomes its start state with no transition,
// while one used before transitions.
func TestEncoderTextureTracker_SubresourceFirstUse(t *testing.T) {
	t.Parallel()
	tracker := NewEncoderTextureTracker()
	if got := tracker.AppendSingle(nil, 0, selectArrayLayer(0), TextureUsesColorTarget); len(got) != 0 {
		t.Fatalf("layer 0 first use: %+v, want no transition", got)
	}
	got := tracker.AppendSingle(nil, 0, TextureSelector{}, TextureUsesResource)
	want := []TexturePendingTransition{{
		Index:    0,
		Selector: TextureSelector{TextureMipLevelCount: 3, TextureArrayLayerCount: 2, BaseMipLevel: 0, MipLevelCount: 1, BaseArrayLayer: 0, ArrayLayerCount: 1},
		Usage:    TextureStateTransition{From: TextureUsesColorTarget, To: TextureUsesResource},
	}, {
		Index:    0,
		Selector: TextureSelector{TextureMipLevelCount: 3, TextureArrayLayerCount: 2, BaseMipLevel: 1, MipLevelCount: 1, BaseArrayLayer: 0, ArrayLayerCount: 1},
		Usage:    TextureStateTransition{From: TextureUsesColorTarget, To: TextureUsesResource},
	}, {
		Index:    0,
		Selector: TextureSelector{TextureMipLevelCount: 3, TextureArrayLayerCount: 2, BaseMipLevel: 2, MipLevelCount: 1, BaseArrayLayer: 0, ArrayLayerCount: 1},
		Usage:    TextureStateTransition{From: TextureUsesColorTarget, To: TextureUsesResource},
	}}
	checkTransitions(t, "whole texture after layer 0", got, want)
	if got := tracker.start[0].subresourceUsage(1, 1); got != TextureUsesResource {
		t.Errorf("layer 1 start = %v, want Resource, its first use", got)
	}
	if got := tracker.start[0].subresourceUsage(1, 0); got != TextureUsesColorTarget {
		t.Errorf("layer 0 start = %v, want ColorTarget", got)
	}
}

// TestTextureTracker_MergeEncoderPerSubresource checks that a submit moves
// each subresource the command buffer uses from the device's state of it,
// and leaves the others as they are.
func TestTextureTracker_MergeEncoderPerSubresource(t *testing.T) {
	t.Parallel()
	device := NewTextureTracker()
	device.InsertSingle(5, TextureUsesUninitialized)
	if got := device.AppendSingle(nil, 5, TextureSelector{}, TextureUsesCopyDst); len(got) != 1 || !got[0].Selector.SelectsAll() {
		t.Fatalf("upload: %+v, want one transition of the whole texture", got)
	}

	encoder := NewEncoderTextureTracker()
	scope := NewTextureUsageScope()
	_ = scope.SetSubresourceUsage(5, selectMipLevel(1), TextureUsesColorTarget)
	_ = scope.SetSubresourceUsage(5, selectMipLevel(0), TextureUsesResource)
	encoder.SetFromScope(scope)

	got := device.MergeEncoder(encoder)
	want := []TexturePendingTransition{
		{Index: 5, Selector: TextureSelector{TextureMipLevelCount: 3, TextureArrayLayerCount: 2, BaseMipLevel: 0, MipLevelCount: 1, ArrayLayerCount: 2},
			Usage: TextureStateTransition{From: TextureUsesCopyDst, To: TextureUsesResource}},
		{Index: 5, Selector: TextureSelector{TextureMipLevelCount: 3, TextureArrayLayerCount: 2, BaseMipLevel: 1, MipLevelCount: 1, ArrayLayerCount: 2},
			Usage: TextureStateTransition{From: TextureUsesCopyDst, To: TextureUsesColorTarget}},
	}
	checkTransitions(t, "submit", got, want)
	for _, c := range []struct {
		level uint32
		want  TextureUses
	}{{0, TextureUsesResource}, {1, TextureUsesColorTarget}, {2, TextureUsesCopyDst}} {
		if got := device.states[5].subresourceUsage(c.level, 1); got != c.want {
			t.Errorf("device level %d = %v, want %v", c.level, got, c.want)
		}
	}
	if got := device.GetUsage(5); got != TextureUsesNone {
		t.Errorf("GetUsage = %v, want None while the levels differ", got)
	}

	// A queue write of the whole texture moves each level from its own
	// state; level 2, written before, needs a barrier between the writes.
	got = device.AppendSingle(nil, 5, TextureSelector{}, TextureUsesCopyDst)
	if len(got) != 3 || got[0].Usage.From != TextureUsesResource || got[1].Usage.From != TextureUsesColorTarget ||
		got[2].Usage.From != TextureUsesCopyDst {
		t.Fatalf("write after submit: %+v, want levels 0, 1 and 2 moved from Resource, ColorTarget and CopyDst", got)
	}
	if usage := device.GetUsage(5); usage != TextureUsesCopyDst {
		t.Errorf("GetUsage after the write = %v, want CopyDst for every subresource", usage)
	}
}

func TestTexturePendingTransition_IntoHALRange(t *testing.T) {
	t.Parallel()
	whole := TexturePendingTransition{Usage: TextureStateTransition{From: TextureUsesCopyDst, To: TextureUsesResource}}
	if got := whole.IntoHAL(nil).Range; got.MipLevelCount != 0 || got.ArrayLayerCount != 0 || got.BaseMipLevel != 0 {
		t.Errorf("whole texture range = %+v, want the zero range", got)
	}
	level := TexturePendingTransition{
		Selector: TextureSelector{TextureMipLevelCount: 3, TextureArrayLayerCount: 2, BaseMipLevel: 1, MipLevelCount: 1, BaseArrayLayer: 1, ArrayLayerCount: 1},
		Usage:    TextureStateTransition{From: TextureUsesColorTarget, To: TextureUsesResource},
	}
	barrier := level.IntoHAL(nil)
	if r := barrier.Range; r.BaseMipLevel != 1 || r.MipLevelCount != 1 || r.BaseArrayLayer != 1 || r.ArrayLayerCount != 1 {
		t.Errorf("range = %+v, want level 1, layer 1", r)
	}
	if barrier.Usage.NewUsage != gputypes.TextureUsageTextureBinding {
		t.Errorf("new usage = %v, want TextureBinding", barrier.Usage.NewUsage)
	}
}

func checkTransitions(t *testing.T, what string, got, want []TexturePendingTransition) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: transitions = %+v, want %+v", what, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s: transition %d = %+v, want %+v", what, i, got[i], want[i])
		}
	}
}

// TestTextureTracker_AppendSingleSubresources checks a queue write of some
// subresources: an untracked texture starts Uninitialized, a write of one
// level of a whole texture moves that level alone with one barrier over it,
// and a later write moves each level from its own state.
func TestTextureTracker_AppendSingleSubresources(t *testing.T) {
	t.Parallel()
	device := NewTextureTracker()
	scratch := make([]TexturePendingTransition, 0, 4)

	got := device.AppendSingle(scratch, 3, selectMipLevel(1), TextureUsesCopyDst)
	want := []TexturePendingTransition{{
		Index:    3,
		Selector: selectMipLevel(1),
		Usage:    TextureStateTransition{From: TextureUsesUninitialized, To: TextureUsesCopyDst},
	}}
	checkTransitions(t, "level 1 of an untracked texture", got, want)
	if &got[0] != &scratch[:1][0] {
		t.Error("AppendSingle did not append to the caller's slice")
	}
	if usage := device.states[3].subresourceUsage(1, 1); usage != TextureUsesCopyDst {
		t.Errorf("level 1 = %v, want CopyDst", usage)
	}
	if usage := device.states[3].subresourceUsage(0, 0); usage != TextureUsesUninitialized {
		t.Errorf("level 0 = %v, want Uninitialized", usage)
	}
	if usage := device.states[3].subresourceUsage(3, 0); usage != TextureUsesNone {
		t.Errorf("level 3 of a texture of 3 = %v, want None", usage)
	}

	// The same usage again over the whole texture: levels 0 and 2 move from
	// Uninitialized, level 1 needs a barrier between two writes.
	got = device.AppendSingle(nil, 3, TextureSelector{}, TextureUsesCopyDst)
	if len(got) != 3 || got[0].Usage.From != TextureUsesUninitialized || got[1].Usage.From != TextureUsesCopyDst ||
		got[2].Usage.From != TextureUsesUninitialized {
		t.Fatalf("whole write: %+v, want levels 0, 1 and 2 from Uninitialized, CopyDst and Uninitialized", got)
	}

	// A usage the whole texture already has, over one level, needs no split.
	device.InsertSingle(4, TextureUsesResource)
	if got := device.AppendSingle(nil, 4, selectMipLevel(2), TextureUsesResource); len(got) != 0 {
		t.Errorf("Resource over Resource: %+v, want no transition", got)
	}
	if !device.states[4].isWhole() {
		t.Error("a usage the texture already has split its state")
	}
}

// TestTextureTracker_MergeEncoderInsertsSplitEnd checks a texture the device
// does not track yet: a command buffer that first used it whole and left its
// levels in different states hands the device those states, with no
// barrier.
func TestTextureTracker_MergeEncoderInsertsSplitEnd(t *testing.T) {
	t.Parallel()
	encoder := NewEncoderTextureTracker()
	encoder.AppendSingle(nil, 6, TextureSelector{}, TextureUsesCopyDst)
	encoder.AppendSingle(nil, 6, selectMipLevel(2), TextureUsesResource)

	device := NewTextureTracker()
	if got := device.MergeEncoder(encoder); len(got) != 0 {
		t.Fatalf("untracked texture: %+v, want no transition", got)
	}
	if usage := device.states[6].subresourceUsage(2, 1); usage != TextureUsesResource {
		t.Errorf("level 2 = %v, want Resource", usage)
	}
	if usage := device.states[6].subresourceUsage(0, 1); usage != TextureUsesCopyDst {
		t.Errorf("level 0 = %v, want CopyDst", usage)
	}

	// The device's split storage is reused when the texture is tracked
	// again from the same command buffer.
	device.Remove(6)
	if got := device.MergeEncoder(encoder); len(got) != 0 {
		t.Fatalf("texture tracked again: %+v, want no transition", got)
	}
	if usage := device.states[6].subresourceUsage(2, 0); usage != TextureUsesResource {
		t.Errorf("level 2 after reuse = %v, want Resource", usage)
	}
}

// TestTextureUsageScope_NoUsage checks that a usage of nothing changes
// nothing: the scope owns the texture with no usage, and merging the scope
// moves nothing.
func TestTextureUsageScope_NoUsage(t *testing.T) {
	t.Parallel()
	scope := NewTextureUsageScope()
	if got := scope.GetUsage(2); got != TextureUsesNone {
		t.Errorf("unused texture = %v, want None", got)
	}
	if err := scope.SetUsage(2, TextureUsesNone); err != nil {
		t.Fatal(err)
	}
	encoder := NewEncoderTextureTracker()
	if got := encoder.SetFromScope(scope); len(got) != 0 {
		t.Errorf("SetFromScope: %+v, want nothing", got)
	}
	device := NewTextureTracker()
	device.InsertSingle(2, TextureUsesResource)
	if got := device.MergeEncoder(encoder); len(got) != 0 {
		t.Errorf("MergeEncoder: %+v, want nothing", got)
	}
	if got := device.GetUsage(2); got != TextureUsesResource {
		t.Errorf("device usage = %v, want Resource as before", got)
	}
}

// TestTextureUsageScope_GrowsAndReusesStates checks a scope past its first
// capacity, and that states a Clear dropped start afresh when taken back.
func TestTextureUsageScope_GrowsAndReusesStates(t *testing.T) {
	t.Parallel()
	scope := NewTextureUsageScope()
	const index TrackerIndex = 40
	if err := scope.SetSubresourceUsage(index, selectMipLevel(0), TextureUsesColorTarget); err != nil {
		t.Fatal(err)
	}
	if err := scope.SetSubresourceUsage(index, selectMipLevel(0), TextureUsesColorTarget); err != nil {
		t.Fatalf("the same usage again: %v", err)
	}
	scope.Clear()
	if err := scope.SetUsage(index, TextureUsesResource); err != nil {
		t.Fatalf("after Clear: %v, want the dropped state forgotten", err)
	}
	if got := scope.GetUsage(index); got != TextureUsesResource {
		t.Errorf("after Clear = %v, want Resource for every subresource", got)
	}
}

// TestTextureState_CollapseWhole checks that collapsing a whole state keeps
// it as it is.
func TestTextureState_CollapseWhole(t *testing.T) {
	t.Parallel()
	state := textureState{usage: TextureUsesResource}
	state.collapse()
	if !state.isWhole() || state.usage != TextureUsesResource {
		t.Errorf("collapse of a whole state = %+v, want it unchanged", state)
	}
}
