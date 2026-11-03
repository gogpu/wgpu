// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build (windows || linux) && !(js && wasm)

package gles

import (
	"strings"
	"testing"

	"github.com/gogpu/gputypes"
)

// TestNewBindGroupResolvesViewHandles checks that a bind group resolves
// texture view handles through the device's table: a live view resolves, a
// surface view (no GL texture) is skipped, and an unknown or destroyed
// handle is an error rather than a cast of an arbitrary integer to a pointer.
func TestNewBindGroupResolvesViewHandles(t *testing.T) {
	dev := &Device{}
	tex := &Texture{id: 7}
	view, err := dev.CreateTextureView(tex, &TextureViewDescriptor{BaseMipLevel: 2})
	if err != nil {
		t.Fatalf("CreateTextureView: %v", err)
	}
	surfaceView, err := dev.CreateTextureView(&SurfaceTexture{}, nil)
	if err != nil {
		t.Fatalf("CreateTextureView(surface): %v", err)
	}
	if view.NativeHandle() == 0 || surfaceView.NativeHandle() == 0 || view.NativeHandle() == surfaceView.NativeHandle() {
		t.Fatalf("handles %d, %d: want distinct and non-zero", view.NativeHandle(), surfaceView.NativeHandle())
	}

	layout := &BindGroupLayout{}
	entries := []gputypes.BindGroupEntry{
		{Binding: 0, Resource: gputypes.TextureViewBinding{TextureView: view.NativeHandle()}},
		{Binding: 1, Resource: gputypes.SamplerBinding{Sampler: 3}},
		{Binding: 2, Resource: gputypes.TextureViewBinding{TextureView: surfaceView.NativeHandle()}},
	}
	bg, err := newBindGroup(&dev.views, layout, entries)
	if err != nil {
		t.Fatalf("newBindGroup: %v", err)
	}
	if bg.views[0] != view.(*TextureView) || bg.views[1] != nil || bg.views[2] != nil {
		t.Fatalf("resolved views = %v, want [view nil nil]", bg.views)
	}
	if bg.views[0].baseMip != 2 {
		t.Fatalf("resolved view baseMip = %d, want 2", bg.views[0].baseMip)
	}

	stale := view.NativeHandle()
	dev.DestroyTextureView(view)
	dev.DestroyTextureView(view) // idempotent
	if view.NativeHandle() != 0 {
		t.Fatalf("NativeHandle after destroy = %d, want 0", view.NativeHandle())
	}

	for _, tc := range []struct {
		name   string
		handle uintptr
	}{
		{"destroyed", stale},
		{"unknown", 0xdead},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := newBindGroup(&dev.views, layout, []gputypes.BindGroupEntry{
				{Binding: 4, Resource: gputypes.TextureViewBinding{TextureView: tc.handle}},
			})
			if err == nil || !strings.Contains(err.Error(), "gles: binding 4: unknown texture view handle") {
				t.Fatalf("err = %v, want unknown texture view handle", err)
			}
		})
	}
	if n := dev.views.Len(); n != 1 {
		t.Fatalf("table holds %d views, want 1 (the surface view)", n)
	}
}
