// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows && !(js && wasm)

package dx12

import (
	"errors"
	"testing"
)

func TestTexture_AddPendingRef(t *testing.T) {
	tex := &Texture{}

	tex.AddPendingRef()
	if tex.pendingRefs != 1 {
		t.Errorf("after AddPendingRef: pendingRefs = %d, want 1", tex.pendingRefs)
	}

	tex.AddPendingRef()
	if tex.pendingRefs != 2 {
		t.Errorf("after second AddPendingRef: pendingRefs = %d, want 2", tex.pendingRefs)
	}
}

func TestTexture_DecPendingRef_NoDefer(t *testing.T) {
	tex := &Texture{}
	tex.pendingRefs = 2

	tex.DecPendingRef()
	if tex.pendingRefs != 1 {
		t.Errorf("after DecPendingRef: pendingRefs = %d, want 1", tex.pendingRefs)
	}
	// pendingDeath is false, so no destroy should happen.
	if tex.pendingDeath {
		t.Error("pendingDeath should be false when Destroy was not called")
	}
}

func TestTexture_Destroy_NoPending(t *testing.T) {
	// When pendingRefs == 0, Destroy should call doDestroy immediately.
	// We verify by checking that raw becomes nil (non-external texture).
	tex := &Texture{
		raw:        nil, // already nil, but pendingDeath should NOT be set
		isExternal: false,
	}

	tex.Destroy()

	if tex.pendingDeath {
		t.Error("pendingDeath should be false when refs == 0 (immediate destroy)")
	}
}

func TestTexture_Destroy_WithPending(t *testing.T) {
	// When pendingRefs > 0, Destroy should defer (set pendingDeath=true) and NOT release.
	tex := &Texture{
		pendingRefs: 2,
		isExternal:  false,
	}

	tex.Destroy()

	if !tex.pendingDeath {
		t.Error("pendingDeath should be true when Destroy called with pendingRefs > 0")
	}
	// pendingRefs should be unchanged.
	if tex.pendingRefs != 2 {
		t.Errorf("pendingRefs should remain 2, got %d", tex.pendingRefs)
	}
}

func TestTexture_DecPendingRef_TriggersDefer(t *testing.T) {
	// Simulate: AddPendingRef, Destroy (deferred), then DecPendingRef triggers release.
	tex := &Texture{
		raw:        nil, // nil raw so doDestroy is safe (no COM Release)
		isExternal: false,
	}

	tex.AddPendingRef() // pendingRefs = 1
	tex.Destroy()       // pendingDeath = true, but refs > 0 so deferred

	if !tex.pendingDeath {
		t.Fatal("pendingDeath should be true after Destroy with pending refs")
	}

	tex.DecPendingRef() // pendingRefs = 0, pendingDeath = true => doDestroy fires

	// After doDestroy, raw should be nil (it was already nil, but the path was exercised).
	// The key assertion is that we reached doDestroy without panic.
	if tex.pendingRefs > 0 {
		t.Errorf("pendingRefs should be <= 0 after final DecPendingRef, got %d", tex.pendingRefs)
	}
}

func TestTexture_DecPendingRef_NoDeferWithoutDestroy(t *testing.T) {
	// DecPendingRef with pendingDeath=false should NOT trigger doDestroy
	// even when pendingRefs reaches 0.
	tex := &Texture{
		pendingRefs: 1,
		isExternal:  false,
	}

	tex.DecPendingRef()

	if tex.pendingRefs != 0 {
		t.Errorf("pendingRefs should be 0, got %d", tex.pendingRefs)
	}
	// pendingDeath was never set, so no destroy should trigger.
	if tex.pendingDeath {
		t.Error("pendingDeath should remain false")
	}
}

func TestFailTextureViewCreationRecyclesAllocatedDescriptors(t *testing.T) {
	device := &Device{
		rtvHeap:         &DescriptorHeap{},
		dsvHeap:         &DescriptorHeap{},
		stagingViewHeap: &DescriptorHeap{},
	}
	view := &TextureView{
		texture:        &Texture{},
		device:         device,
		hasRTV:         true,
		rtvHeapIndex:   2,
		hasDSV:         true,
		hasDSVVariants: [4]bool{true},
		dsvHeapIndex:   [4]uint32{3},
		hasSRV:         true,
		srvHeapIndex:   4,
	}

	result, err := failTextureViewCreation(view, errors.New("view creation failed"))
	if result != nil {
		t.Fatal("failed texture view should not be returned")
	}
	if err == nil || err.Error() != "view creation failed" {
		t.Fatalf("error = %v, want original view creation error", err)
	}
	if len(device.rtvHeap.freeList) != 1 || device.rtvHeap.freeList[0] != 2 {
		t.Fatalf("RTV free list = %v, want [2]", device.rtvHeap.freeList)
	}
	if len(device.dsvHeap.freeList) != 1 || device.dsvHeap.freeList[0] != 3 {
		t.Fatalf("DSV free list = %v, want [3]", device.dsvHeap.freeList)
	}
	if len(device.stagingViewHeap.freeList) != 1 || device.stagingViewHeap.freeList[0] != 4 {
		t.Fatalf("SRV free list = %v, want [4]", device.stagingViewHeap.freeList)
	}
}

// TestDestroyTwiceFreesSlotsOnce destroys each descriptor-owning resource
// twice. Its heap slots must go back to the free list once: a slot freed
// twice sits in the LIFO free list twice, and two later allocations would
// then share one descriptor.
func TestDestroyTwiceFreesSlotsOnce(t *testing.T) {
	newDevice := func() *Device {
		return &Device{
			viewHeap:           &DescriptorHeap{},
			samplerHeap:        &DescriptorHeap{},
			rtvHeap:            &DescriptorHeap{},
			dsvHeap:            &DescriptorHeap{},
			stagingViewHeap:    &DescriptorHeap{},
			stagingSamplerHeap: &DescriptorHeap{},
		}
	}
	freeLists := func(d *Device) map[string]int {
		return map[string]int{
			"view":           len(d.viewHeap.freeList),
			"sampler":        len(d.samplerHeap.freeList),
			"rtv":            len(d.rtvHeap.freeList),
			"dsv":            len(d.dsvHeap.freeList),
			"stagingView":    len(d.stagingViewHeap.freeList),
			"stagingSampler": len(d.stagingSamplerHeap.freeList),
		}
	}

	tests := []struct {
		name    string
		destroy func(d *Device) func()
		want    map[string]int
	}{
		{
			name: "sampler",
			destroy: func(d *Device) func() {
				s := &Sampler{heapIndex: 5, samplerPoolSlot: 9, device: d}
				s.nativeHandle = d.handles.samplers.Add(s)
				return s.Destroy
			},
			want: map[string]int{"sampler": 1, "stagingSampler": 1},
		},
		{
			name: "texture view",
			destroy: func(d *Device) func() {
				v := &TextureView{
					texture: &Texture{}, device: d,
					hasRTV: true, rtvHeapIndex: 1,
					hasDSV: true, hasDSVVariants: [4]bool{true, true}, dsvHeapIndex: [4]uint32{2, 3},
					hasSRV: true, srvHeapIndex: 4,
				}
				v.nativeHandle = d.handles.views.Add(v)
				return v.Destroy
			},
			want: map[string]int{"rtv": 1, "dsv": 2, "stagingView": 1},
		},
		{
			name: "bind group",
			destroy: func(d *Device) func() {
				g := &BindGroup{device: d, viewHeapIndex: 10, viewCount: 3}
				return g.Destroy
			},
			want: map[string]int{"view": 3},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newDevice()
			destroy := tt.destroy(d)
			destroy()
			destroy()
			got := freeLists(d)
			for heap, n := range got {
				if n != tt.want[heap] {
					t.Errorf("%s heap free list has %d entries after two Destroy calls, want %d", heap, n, tt.want[heap])
				}
			}
		})
	}
}
