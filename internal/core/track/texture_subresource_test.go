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
	}{
		{"mip levels", selectMipLevel(1), selectMipLevel(0), selectMipLevel(1)},
		{"array layers", selectArrayLayer(1), selectArrayLayer(0), TextureSelector{}},
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
