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

	views, samplers, err := orderBindGroupEntries(layout, entries)
	runtime.KeepAlive([]any{ro, rw, ub, s0, s1, tex})
	if err != nil {
		t.Fatalf("orderBindGroupEntries: %v", err)
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
			_, _, err := orderBindGroupEntries(&BindGroupLayout{entries: tt.layout}, tt.entries)
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
