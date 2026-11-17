//go:build !rust && !(js && wasm)

package encoder

import (
	"fmt"
	"slices"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/hal"
	"github.com/gogpu/wgpu/internal/resource"
)

// testHALBindGroup is a HAL bind group with no behavior: the binder only
// checks that the handle is non-nil before binding it.
type testHALBindGroup struct{ hal.BindGroup }

// bindCall is one SetBindGroup the binder made on the backend pass.
type bindCall struct {
	index   uint32
	group   hal.BindGroup
	offsets []uint32
}

// recordingBindPass records the SetBindGroup calls the binder makes.
type recordingBindPass struct {
	calls []bindCall
}

func (r *recordingBindPass) SetBindGroup(index uint32, group hal.BindGroup, offsets []uint32) {
	r.calls = append(r.calls, bindCall{index: index, group: group, offsets: offsets})
}

// take returns the indices bound since the last take.
func (r *recordingBindPass) take() []uint32 {
	indices := make([]uint32, len(r.calls))
	for i, c := range r.calls {
		indices[i] = c.index
	}
	r.calls = r.calls[:0]
	return indices
}

// rebindLayout returns a layout with one uniform buffer entry; layouts made
// with the same dynamic flag are compatible, entry by entry.
func rebindLayout(dynamic bool) *resource.BindGroupLayout {
	return resource.NewBindGroupLayout(nil, nil, []gputypes.BindGroupLayoutEntry{{
		Binding:    0,
		Visibility: gputypes.ShaderStageCompute,
		Buffer:     &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform, HasDynamicOffset: dynamic},
	}})
}

func rebindGroup(layout *resource.BindGroupLayout) *resource.BindGroup {
	return resource.NewBindGroup(&resource.BindGroupInit{HAL: &testHALBindGroup{}, Layout: layout})
}

func expectBound(t *testing.T, step string, pass *recordingBindPass, want ...uint32) {
	t.Helper()
	if got := pass.take(); !slices.Equal(got, want) {
		t.Errorf("%s: bound %v, want %v", step, got, want)
	}
}

// TestBinderFlushRebind walks the binder through the WebGPU cases of Rust
// wgpu-core's Binder: a group set before the pipeline or at an index the
// pipeline lacks reaches the backend only once a pipeline layout uses it, and
// a pipeline change rebinds from the first index whose layout changed.
func TestBinderFlushRebind(t *testing.T) {
	la, laTwin, ld := rebindLayout(true), rebindLayout(true), rebindLayout(false)
	lb, lc := rebindLayout(true), rebindLayout(false)
	gA, gB, gC, gD := rebindGroup(la), rebindGroup(lb), rebindGroup(lc), rebindGroup(ld)
	var b Binder
	pass := &recordingBindPass{}

	b.SetGroup(0, gA, []uint32{256})
	b.Flush(pass)
	expectBound(t, "group set before any pipeline", pass)

	b.UpdateExpectations([]*resource.BindGroupLayout{la})
	b.SetGroup(1, gB, []uint32{0})
	b.Flush(pass)
	expectBound(t, "pipeline with group 0 only", pass, 0)

	b.Flush(pass)
	expectBound(t, "second draw, nothing changed", pass)

	b.UpdateExpectations([]*resource.BindGroupLayout{la, lb})
	b.Flush(pass)
	expectBound(t, "pipeline gains group 1", pass, 1)

	b.UpdateExpectations([]*resource.BindGroupLayout{laTwin, lc})
	b.Flush(pass)
	expectBound(t, "equal layout at 0, other layout at 1 (gB does not fit it)", pass)

	b.SetGroup(1, gC, nil)
	b.Flush(pass)
	expectBound(t, "gC set at 1", pass, 1)

	b.UpdateExpectations([]*resource.BindGroupLayout{la, lb})
	b.SetGroup(1, gB, []uint32{512})
	b.Flush(pass)
	expectBound(t, "gB set again at 1", pass, 1)

	b.UpdateExpectations([]*resource.BindGroupLayout{ld, lb})
	b.SetGroup(0, gD, nil)
	b.Flush(pass)
	expectBound(t, "change at 0 takes 1 with it", pass, 0, 1)

	b.reset()
	b.UpdateExpectations([]*resource.BindGroupLayout{ld, lb})
	b.Flush(pass)
	expectBound(t, "after reset (ExecuteBundles)", pass)
}

// TestBinderFlushOffsets checks that the binder binds each group with the
// offsets it was set with, copied: the caller may reuse its slice, and a
// backend may keep the slice it was bound with until playback.
func TestBinderFlushOffsets(t *testing.T) {
	l := rebindLayout(true)
	g := rebindGroup(l)
	var b Binder
	pass := &recordingBindPass{}

	offsets := []uint32{256}
	b.SetGroup(0, g, offsets)
	offsets[0] = 768
	b.UpdateExpectations([]*resource.BindGroupLayout{l})
	b.Flush(pass)

	offsets[0] = 512
	b.SetGroup(0, g, offsets)
	b.Flush(pass)

	if len(pass.calls) != 2 {
		t.Fatalf("bound %d times, want 2", len(pass.calls))
	}
	for i, want := range []uint32{256, 512} {
		c := pass.calls[i]
		if got := fmt.Sprint(c.offsets); got != fmt.Sprint([]uint32{want}) {
			t.Errorf("bind %d: offsets %s, want [%d]", i, got, want)
		}
		if c.group != g.HAL() {
			t.Errorf("bind %d: group %v, want %v", i, c.group, g.HAL())
		}
	}
}

// TestBinderBindValidRestores checks BindValid(raw, 0), which the compute
// pass uses after indirect dispatch validation to restore every binding on a
// new backend pass: it binds all valid groups, whatever was flushed before.
func TestBinderBindValidRestores(t *testing.T) {
	l0, l1 := rebindLayout(true), rebindLayout(false)
	var b Binder
	pass := &recordingBindPass{}
	b.UpdateExpectations([]*resource.BindGroupLayout{l0, l1})
	b.SetGroup(0, rebindGroup(l0), []uint32{0})
	b.SetGroup(1, rebindGroup(l1), nil)
	b.SetGroup(2, rebindGroup(l1), nil) // not used by the pipeline
	b.Flush(pass)
	expectBound(t, "flush", pass, 0, 1)

	b.BindValid(pass, 0)
	expectBound(t, "restore", pass, 0, 1)

	if b.ActiveGroup(2) != nil {
		t.Error("activeGroup(2) is set, want nil: the pipeline does not use group 2")
	}
	if b.ActiveGroup(1) == nil {
		t.Error("activeGroup(1) is nil, want the group set at 1")
	}
}
