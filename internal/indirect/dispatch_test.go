// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package indirect

import (
	"errors"
	"testing"

	"github.com/gogpu/wgpu/hal"
)

func TestDispatchSourceFor(t *testing.T) {
	for _, tc := range []struct {
		offset, size uint64
		align        uint32
		want         DispatchSource
	}{
		{0, 12, 256, DispatchSource{0, 12, 0}},
		{0, 512, 256, DispatchSource{0, 264, 0}},
		{12, 512, 256, DispatchSource{0, 264, 3}},
		{252, 512, 256, DispatchSource{0, 264, 63}},
		{256, 512, 256, DispatchSource{256, 256, 0}},
		{268, 512, 256, DispatchSource{256, 256, 3}},
		{500, 512, 256, DispatchSource{256, 256, 61}},
		{500, 514, 256, DispatchSource{256, 256, 61}}, // size not a multiple of 4
		{40, 64, 32, DispatchSource{32, 32, 2}},
		{8, 20, 4, DispatchSource{8, 12, 0}},
	} {
		got := DispatchSourceFor(tc.offset, tc.size, tc.align)
		if got != tc.want {
			t.Errorf("DispatchSourceFor(%d, %d, %d) = %+v, want %+v", tc.offset, tc.size, tc.align, got, tc.want)
			continue
		}
		// The binding holds the arguments and stays inside the buffer.
		if end := got.BindingOffset + got.BindingSize; end > tc.size || uint64(got.Index)*4+DispatchArgsSize > got.BindingSize {
			t.Errorf("DispatchSourceFor(%d, %d, %d) = %+v does not hold the arguments inside the buffer", tc.offset, tc.size, tc.align, got)
		}
		if got.Index >= tc.align/4 {
			t.Errorf("DispatchSourceFor(%d, %d, %d): index %d not below alignment/4", tc.offset, tc.size, tc.align, got.Index)
		}
	}
}

// fakeBindGroup is a distinct hal.BindGroup per id.
type fakeBindGroup struct{ id int }

func (fakeBindGroup) Destroy() {}

// destroyRecorder is a hal.Device that records DestroyBindGroup calls; the
// embedded nil interface panics on any other method.
type destroyRecorder struct {
	hal.Device
	destroyed []hal.BindGroup
}

func (d *destroyRecorder) DestroyBindGroup(g hal.BindGroup) { d.destroyed = append(d.destroyed, g) }

func TestBindGroupCache(t *testing.T) {
	var c BindGroupCache
	created := 0
	create := func() (hal.BindGroup, error) {
		created++
		return fakeBindGroup{created}, nil
	}
	a1, _ := c.Get(0, create)
	a2, _ := c.Get(0, create)
	b, _ := c.Get(256, create)
	if a1 != a2 || a1 == b || created != 2 {
		t.Fatalf("Get: a1=%v a2=%v b=%v after %d creations, want one group per key", a1, a2, b, created)
	}
	if _, err := c.Get(512, func() (hal.BindGroup, error) { return nil, errors.New("boom") }); err == nil {
		t.Fatal("Get: creation error not returned")
	}

	dev := &destroyRecorder{}
	c.Destroy(dev)
	if len(dev.destroyed) != 2 {
		t.Fatalf("Destroy destroyed %d groups, want 2", len(dev.destroyed))
	}
	c.Destroy(dev)
	if len(dev.destroyed) != 2 {
		t.Fatalf("second Destroy destroyed again: %d", len(dev.destroyed))
	}
}
