// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin

package metal

import (
	"testing"

	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/gputypes"
)

func TestSurfaceTextureCreateView(t *testing.T) {
	if err := Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	pool := NewAutoreleasePool()
	defer pool.Drain()

	layer := MsgSend(ID(GetClass("CAMetalLayer")), Sel("new"))
	if layer == 0 {
		t.Fatal("CAMetalLayer new returned nil")
	}
	defer Release(layer)

	backend := Backend{}
	inst, err := backend.CreateInstance(&hal.InstanceDescriptor{
		Backends: gputypes.Backends(1 << gputypes.BackendMetal),
	})
	if err != nil {
		t.Fatalf("CreateInstance failed: %v", err)
	}
	instance := inst.(*Instance)

	surface, err := instance.CreateSurface(0, uintptr(layer))
	if err != nil {
		t.Fatalf("CreateSurface failed: %v", err)
	}
	defer surface.Destroy()

	adapters := instance.EnumerateAdapters(surface)
	if len(adapters) == 0 {
		t.Skip("no Metal adapters available")
	}
	adapter := adapters[0].Adapter
	defer adapter.Destroy()

	open, err := adapter.Open(gputypes.Features(0), gputypes.DefaultLimits())
	if err != nil {
		t.Fatalf("Adapter.Open failed: %v", err)
	}
	defer open.Device.Destroy()

	config := &hal.SurfaceConfiguration{
		Format:      gputypes.TextureFormatBGRA8Unorm,
		Width:       64,
		Height:      64,
		PresentMode: hal.PresentModeFifo,
		Usage:       gputypes.TextureUsageRenderAttachment,
		AlphaMode:   hal.CompositeAlphaModeOpaque,
	}
	if err := surface.Configure(open.Device, config); err != nil {
		t.Fatalf("Configure failed: %v", err)
	}

	acquired, err := surface.AcquireTexture(nil)
	if err != nil {
		t.Fatalf("AcquireTexture failed: %v", err)
	}
	if acquired.Texture == nil {
		t.Fatal("AcquireTexture returned nil texture")
	}

	view, err := open.Device.CreateTextureView(acquired.Texture, nil)
	if err != nil {
		t.Fatalf("CreateTextureView failed: %v", err)
	}
	if view == nil {
		t.Fatal("CreateTextureView returned nil")
	}
	tv, ok := view.(*TextureView)
	if !ok || tv.raw == 0 {
		t.Fatal("CreateTextureView returned nil raw texture view")
	}
	view.Destroy()

	surface.DiscardTexture(acquired.Texture)
}
