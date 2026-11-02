// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows && !(js && wasm)

package dx12

import (
	"encoding/binary"
	"testing"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/dx12/d3d12"
)

// TestTexture3DRTVDesc checks the per-pass render target view of a 3D color
// attachment: D3D12_TEX3D_RTV{MipSlice, FirstWSlice, WSize} (d3d12.h) built
// from the view's base mip and the attachment's depth slice, one slice wide,
// as Rust wgpu-hal dx12/command.rs begin_render_pass builds it.
func TestTexture3DRTVDesc(t *testing.T) {
	slice := func(v uint32) *uint32 { return &v }
	tests := []struct {
		name       string
		baseMip    uint32
		depthSlice *uint32
		want       [3]uint32 // MipSlice, FirstWSlice, WSize
	}{
		{"mip 0, no depth slice", 0, nil, [3]uint32{0, 0, 1}},
		{"mip 0, slice 3", 0, slice(3), [3]uint32{0, 3, 1}},
		{"mip 2, slice 1", 2, slice(1), [3]uint32{2, 1, 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			view := &TextureView{format: gputypes.TextureFormatRGBA8Unorm, dimension: gputypes.TextureViewDimension3D, baseMip: tt.baseMip}
			desc := texture3DRTVDesc(view, tt.depthSlice)
			if desc.ViewDimension != d3d12.D3D12_RTV_DIMENSION_TEXTURE3D {
				t.Errorf("ViewDimension = %d, want TEXTURE3D", desc.ViewDimension)
			}
			if want := textureFormatToD3D12(gputypes.TextureFormatRGBA8Unorm); desc.Format != want {
				t.Errorf("Format = %d, want %d", desc.Format, want)
			}
			le := binary.LittleEndian
			got := [3]uint32{le.Uint32(desc.Union[0:4]), le.Uint32(desc.Union[4:8]), le.Uint32(desc.Union[8:12])}
			if got != tt.want {
				t.Errorf("TEX3D_RTV = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestTempRTVsFreedOnReset checks that the per-pass 3D RTVs go back to the
// RTV heap when the encoder is reset (the GPU is done with its command
// buffers) or destroyed, and only once.
func TestTempRTVsFreedOnReset(t *testing.T) {
	for _, tc := range []struct {
		name string
		end  func(e *CommandEncoder)
	}{
		{"ResetAll", func(e *CommandEncoder) { e.ResetAll(nil); e.ResetAll(nil) }},
		{"Destroy", func(e *CommandEncoder) { e.Destroy(); e.Destroy() }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dev := &Device{rtvHeap: &DescriptorHeap{}}
			e := &CommandEncoder{device: dev, tempRTVs: []uint32{4, 7}}
			tc.end(e)
			if len(e.tempRTVs) != 0 {
				t.Errorf("tempRTVs = %v after %s, want empty", e.tempRTVs, tc.name)
			}
			if got := dev.rtvHeap.freeList; len(got) != 2 || got[0] != 4 || got[1] != 7 {
				t.Errorf("RTV free list = %v, want [4 7]", got)
			}
		})
	}
}

// TestRender3DSliceReadback renders to single depth slices of a 3D texture
// through the DX12 HAL: it clears every slice to blue, then slice k to red,
// copies the texture out and checks that only slice k is red. Before the
// per-pass RTV, every pass rendered through one view-time RTV, so the
// DepthSlice of the attachment had no effect.
func TestRender3DSliceReadback(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping GPU integration test in short mode")
	}
	raw, err := Backend{}.CreateInstance(&hal.InstanceDescriptor{Backends: gputypes.BackendsDX12})
	if err != nil {
		t.Skipf("DX12 unavailable: %v", err)
	}
	defer raw.Destroy()
	adapters := raw.EnumerateAdapters(nil)
	if len(adapters) == 0 {
		t.Skip("no DX12 adapters")
	}
	for _, a := range adapters {
		t.Run(a.Info.Name, func(t *testing.T) {
			open, err := a.Adapter.Open(0, a.Capabilities.Limits)
			if err != nil {
				t.Skipf("Open: %v", err)
			}
			t.Cleanup(open.Device.Destroy)
			render3DSlices(t, open.Device, open.Queue)
		})
	}
}

func render3DSlices(t *testing.T, dev hal.Device, queue hal.Queue) {
	t.Helper()
	const size, depth, k = 4, 4, uint32(2)
	const bytesPerRow = 256 // D3D12_TEXTURE_DATA_PITCH_ALIGNMENT
	const bufSize = bytesPerRow * size * depth

	tex, err := dev.CreateTexture(&hal.TextureDescriptor{
		Label: "3d-target", Size: hal.Extent3D{Width: size, Height: size, DepthOrArrayLayers: depth},
		MipLevelCount: 1, SampleCount: 1, Dimension: gputypes.TextureDimension3D,
		Format: gputypes.TextureFormatRGBA8Unorm,
		Usage:  gputypes.TextureUsageRenderAttachment | gputypes.TextureUsageCopySrc,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	t.Cleanup(func() { dev.DestroyTexture(tex) })
	view, err := dev.CreateTextureView(tex, &hal.TextureViewDescriptor{Dimension: gputypes.TextureViewDimension3D})
	if err != nil {
		t.Fatalf("CreateTextureView: %v", err)
	}
	t.Cleanup(func() { dev.DestroyTextureView(view) })
	if view.(*TextureView).hasRTV {
		t.Fatal("3D view has a view-time RTV; it must be created per pass")
	}
	staging, err := dev.CreateBuffer(&hal.BufferDescriptor{Label: "readback", Size: bufSize, Usage: gputypes.BufferUsageCopyDst | gputypes.BufferUsageMapRead})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	t.Cleanup(func() { dev.DestroyBuffer(staging) })

	enc, err := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{Label: "3d-slices"})
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	t.Cleanup(enc.(*CommandEncoder).Destroy)
	if err := enc.BeginEncoding("3d-slices"); err != nil {
		t.Fatalf("BeginEncoding: %v", err)
	}
	clearSlice := func(slice uint32, c gputypes.Color) {
		pass := enc.BeginRenderPass(&hal.RenderPassDescriptor{ColorAttachments: []hal.RenderPassColorAttachment{{
			View: view, DepthSlice: &slice, LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore, ClearValue: c,
		}}})
		pass.End()
	}
	for s := uint32(0); s < depth; s++ {
		clearSlice(s, gputypes.Color{B: 1, A: 1})
	}
	clearSlice(k, gputypes.Color{R: 1, A: 1})
	if n := len(enc.(*CommandEncoder).tempRTVs); n != depth+1 {
		t.Errorf("per-pass RTVs = %d, want %d", n, depth+1)
	}
	enc.CopyTextureToBuffer(tex, staging, []hal.BufferTextureCopy{{
		BufferLayout: hal.ImageDataLayout{BytesPerRow: bytesPerRow, RowsPerImage: size},
		TextureBase:  hal.ImageCopyTexture{Texture: tex, Aspect: gputypes.TextureAspectAll},
		Size:         hal.Extent3D{Width: size, Height: size, DepthOrArrayLayers: depth},
	}})
	cmd, err := enc.EndEncoding()
	if err != nil {
		t.Fatalf("EndEncoding: %v", err)
	}
	if _, err := queue.Submit([]hal.CommandBuffer{cmd}); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if err := dev.WaitIdle(); err != nil {
		t.Fatalf("WaitIdle: %v", err)
	}
	enc.ResetAll([]hal.CommandBuffer{cmd})
	if n := len(enc.(*CommandEncoder).tempRTVs); n != 0 {
		t.Errorf("per-pass RTVs after ResetAll = %d, want 0", n)
	}

	m, err := dev.MapBuffer(staging, 0, bufSize)
	if err != nil {
		t.Fatalf("MapBuffer: %v", err)
	}
	got := make([]byte, bufSize)
	copy(got, unsafe.Slice((*byte)(m.Ptr), bufSize))
	if err := dev.UnmapBuffer(staging); err != nil {
		t.Fatalf("UnmapBuffer: %v", err)
	}
	red, blue := [4]byte{255, 0, 0, 255}, [4]byte{0, 0, 255, 255}
	for z := uint32(0); z < depth; z++ {
		want := blue
		if z == k {
			want = red
		}
		for y := uint32(0); y < size; y++ {
			for x := uint32(0); x < size; x++ {
				off := (z*size+y)*bytesPerRow + x*4
				if px := [4]byte(got[off : off+4]); px != want {
					t.Fatalf("slice %d texel (%d,%d) = %v, want %v", z, x, y, px, want)
				}
			}
		}
	}
}
