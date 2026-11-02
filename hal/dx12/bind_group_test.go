// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows && !(js && wasm)

package dx12

import (
	"runtime"
	"strings"
	"testing"
	"unsafe"

	"github.com/gogpu/gputypes"
)

func bufferEntry(binding uint32, buf *Buffer) gputypes.BindGroupEntry {
	return gputypes.BindGroupEntry{
		Binding:  binding,
		Resource: gputypes.BufferBinding{Buffer: uintptr(unsafe.Pointer(buf)), Size: buf.size},
	}
}

// TestOrderBindGroupEntries_LayoutOrder checks that descriptor-table entries
// follow the layout (= root signature range) order, not the caller's entry
// order, and that each keeps the layout entry that selects its view kind.
func TestOrderBindGroupEntries_LayoutOrder(t *testing.T) {
	ro, rw, ub := &Buffer{size: 1024}, &Buffer{size: 512}, &Buffer{size: 4}
	s0, s1 := &Sampler{samplerPoolSlot: 7}, &Sampler{samplerPoolSlot: 3}
	tex := &TextureView{}

	layout := &BindGroupLayout{entries: []BindGroupLayoutEntry{
		{Binding: 0, Type: BindingTypeReadOnlyStorageBuffer},
		{Binding: 1, Type: BindingTypeStorageBuffer},
		{Binding: 2, Type: BindingTypeUniformBuffer},
		{Binding: 3, Type: BindingTypeSampler},
		{Binding: 4, Type: BindingTypeSampledTexture},
		{Binding: 5, Type: BindingTypeComparisonSampler},
	}}
	// Deliberately shuffled.
	entries := []gputypes.BindGroupEntry{
		{Binding: 5, Resource: gputypes.SamplerBinding{Sampler: uintptr(unsafe.Pointer(s1))}},
		bufferEntry(2, ub),
		{Binding: 4, Resource: gputypes.TextureViewBinding{TextureView: uintptr(unsafe.Pointer(tex))}},
		bufferEntry(1, rw),
		{Binding: 3, Resource: gputypes.SamplerBinding{Sampler: uintptr(unsafe.Pointer(s0))}},
		bufferEntry(0, ro),
	}

	ordered, err := orderBindGroupEntries(layout, entries)
	runtime.KeepAlive([]any{ro, rw, ub, s0, s1, tex})
	if err != nil {
		t.Fatalf("orderBindGroupEntries: %v", err)
	}
	views, samplers := ordered.views, ordered.samplerPoolIndices
	if len(ordered.dynamicBuffers) != 0 {
		t.Errorf("dynamic buffers = %+v, want none", ordered.dynamicBuffers)
	}

	wantViews := []struct {
		binding uint32
		typ     BindingType
	}{
		{0, BindingTypeReadOnlyStorageBuffer},
		{1, BindingTypeStorageBuffer},
		{2, BindingTypeUniformBuffer},
		{4, BindingTypeSampledTexture},
	}
	if len(views) != len(wantViews) {
		t.Fatalf("got %d view entries, want %d", len(views), len(wantViews))
	}
	for i, w := range wantViews {
		if views[i].entry.Binding != w.binding || views[i].layout.Binding != w.binding || views[i].layout.Type != w.typ {
			t.Errorf("view %d = binding %d/%d type %d, want binding %d type %d",
				i, views[i].entry.Binding, views[i].layout.Binding, views[i].layout.Type, w.binding, w.typ)
		}
	}

	// Sampler slots in layout order: binding 3 (slot 7), then binding 5 (slot 3).
	if len(samplers) != 2 || samplers[0] != 7 || samplers[1] != 3 {
		t.Errorf("sampler pool indices = %v, want [7 3]", samplers)
	}
}

// TestOrderBindGroupEntries_DynamicBuffers checks that a dynamic uniform
// buffer gets no table descriptor but a root CBV address, that a dynamic
// storage buffer keeps its descriptor and records its binding size, and that
// the dynamic buffers come in layout order, the order of SetBindGroup's
// offsets.
func TestOrderBindGroupEntries_DynamicBuffers(t *testing.T) {
	ub := &Buffer{size: 512, gpuVA: 0x10000}
	sb := &Buffer{size: 1024, gpuVA: 0x20000}
	plain := &Buffer{size: 64, gpuVA: 0x30000}
	defer runtime.KeepAlive([]any{ub, sb, plain})

	layout := &BindGroupLayout{entries: []BindGroupLayoutEntry{
		{Binding: 0, Type: BindingTypeReadOnlyStorageBuffer, HasDynamicOffset: true},
		{Binding: 1, Type: BindingTypeUniformBuffer},
		{Binding: 2, Type: BindingTypeUniformBuffer, HasDynamicOffset: true},
	}}
	entries := []gputypes.BindGroupEntry{
		{Binding: 2, Resource: gputypes.BufferBinding{Buffer: uintptr(unsafe.Pointer(ub)), Offset: 256, Size: 32}},
		{Binding: 0, Resource: gputypes.BufferBinding{Buffer: uintptr(unsafe.Pointer(sb)), Offset: 256, Size: 16}},
		bufferEntry(1, plain),
	}
	ordered, err := orderBindGroupEntries(layout, entries)
	if err != nil {
		t.Fatalf("orderBindGroupEntries: %v", err)
	}
	if len(ordered.views) != 2 || ordered.views[0].layout.Binding != 0 || ordered.views[1].layout.Binding != 1 {
		t.Errorf("views = %+v, want bindings 0 and 1 (no descriptor for the dynamic uniform buffer)", ordered.views)
	}
	want := []dynamicBuffer{{size: 16}, {uniform: true, gpuVA: 0x10000 + 256}}
	if len(ordered.dynamicBuffers) != len(want) {
		t.Fatalf("dynamic buffers = %+v, want %+v", ordered.dynamicBuffers, want)
	}
	for i := range want {
		if ordered.dynamicBuffers[i] != want[i] {
			t.Errorf("dynamic buffer %d = %+v, want %+v", i, ordered.dynamicBuffers[i], want[i])
		}
	}
}

func TestOrderBindGroupEntries_Errors(t *testing.T) {
	buf := &Buffer{size: 64}
	tex := &TextureView{}
	defer runtime.KeepAlive([]any{buf, tex})

	tests := []struct {
		name    string
		layout  []BindGroupLayoutEntry
		entries []gputypes.BindGroupEntry
		want    string
	}{
		{
			name:    "missing entry",
			layout:  []BindGroupLayoutEntry{{Binding: 0, Type: BindingTypeStorageBuffer}, {Binding: 1, Type: BindingTypeUniformBuffer}},
			entries: []gputypes.BindGroupEntry{bufferEntry(0, buf)},
			want:    "no entry for layout binding 1",
		},
		{
			name:   "texture for buffer slot",
			layout: []BindGroupLayoutEntry{{Binding: 0, Type: BindingTypeStorageBuffer}},
			entries: []gputypes.BindGroupEntry{
				{Binding: 0, Resource: gputypes.TextureViewBinding{TextureView: uintptr(unsafe.Pointer(tex))}},
			},
			want: "expects a buffer",
		},
		{
			name:    "buffer for sampler slot",
			layout:  []BindGroupLayoutEntry{{Binding: 0, Type: BindingTypeSampler}},
			entries: []gputypes.BindGroupEntry{bufferEntry(0, buf)},
			want:    "expects a sampler",
		},
		{
			name:    "buffer for texture slot",
			layout:  []BindGroupLayoutEntry{{Binding: 0, Type: BindingTypeSampledTexture}},
			entries: []gputypes.BindGroupEntry{bufferEntry(0, buf)},
			want:    "expects a texture view",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := orderBindGroupEntries(&BindGroupLayout{entries: tt.layout}, tt.entries)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestBufferBindingSize(t *testing.T) {
	tests := []struct {
		bufSize, offset, size, want uint64
	}{
		{bufSize: 1024, offset: 0, size: 0, want: 1024},
		{bufSize: 1024, offset: 256, size: 0, want: 768},
		{bufSize: 1024, offset: 256, size: 128, want: 128},
		{bufSize: 256, offset: 512, size: 0, want: 0},
	}
	for _, tt := range tests {
		got := bufferBindingSize(tt.bufSize, gputypes.BufferBinding{Offset: tt.offset, Size: tt.size})
		if got != tt.want {
			t.Errorf("bufferBindingSize(%d, off=%d, size=%d) = %d, want %d", tt.bufSize, tt.offset, tt.size, got, tt.want)
		}
	}
}

func TestConstantBufferViewSize(t *testing.T) {
	for _, tt := range []struct {
		in   uint64
		want uint32
	}{{4, 256}, {256, 256}, {257, 512}, {8, 256}} {
		if got := constantBufferViewSize(tt.in); got != tt.want {
			t.Errorf("constantBufferViewSize(%d) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestRawBufferViewRange(t *testing.T) {
	first, num := rawBufferViewRange(256, 512)
	if first != 64 || num != 128 {
		t.Errorf("rawBufferViewRange(256, 512) = (%d, %d), want (64, 128)", first, num)
	}
	// Rust wgpu-hal: FirstElement = offset/4, NumElements = size/4.
	first, num = rawBufferViewRange(0, 12)
	if first != 0 || num != 3 {
		t.Errorf("rawBufferViewRange(0, 12) = (%d, %d), want (0, 3)", first, num)
	}
}
