//go:build !rust && !(js && wasm)

package wgpu_test

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/noop"
)

// viewDescRecorder is a HAL device that records the descriptor of every
// texture view it creates.
type viewDescRecorder struct {
	hal.Device
	descs []hal.TextureViewDescriptor
}

func (r *viewDescRecorder) CreateTextureView(tex hal.Texture, desc *hal.TextureViewDescriptor) (hal.TextureView, error) {
	r.descs = append(r.descs, *desc)
	return r.Device.CreateTextureView(tex, desc)
}

// TestCreateTextureViewPassesResolvedDimension checks that CreateTextureView
// gives the HAL the view dimension and counts WebGPU resolves from the
// texture when the descriptor leaves them undefined ("resolving
// GPUTextureViewDescriptor defaults"): 2d-array for a 2D texture of several
// layers, one layer for a 2d view, the remaining mips. Each backend's own
// fallback infers 2d from the texture's dimension alone, so a default view of
// a layered texture became a single-layer 2D SRV/RTV on DX12 and a 2D image
// view of several layers on Vulkan. Rust wgpu-core resource.rs
// create_texture_view passes resolved_dimension and resolved_range to the HAL.
func TestCreateTextureViewPassesResolvedDimension(t *testing.T) {
	instance, err := noop.NewBackend().CreateInstance(nil)
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	defer instance.Destroy()
	open, err := instance.EnumerateAdapters(nil)[0].Adapter.Open(0, gputypes.DefaultLimits())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	rec := &viewDescRecorder{Device: open.Device}
	device, err := wgpu.NewDeviceFromHAL(rec, open.Queue, 0, gputypes.DefaultLimits(), "recorder")
	if err != nil {
		t.Fatalf("NewDeviceFromHAL: %v", err)
	}
	defer device.Release()

	tests := []struct {
		name       string
		dim        gputypes.TextureDimension
		layers     uint32
		desc       *wgpu.TextureViewDescriptor
		want       gputypes.TextureViewDimension
		wantLayers uint32
		wantMips   uint32
	}{
		{"2D one layer, nil descriptor", gputypes.TextureDimension2D, 1, nil,
			gputypes.TextureViewDimension2D, 1, 3},
		{"2D four layers, nil descriptor", gputypes.TextureDimension2D, 4, nil,
			gputypes.TextureViewDimension2DArray, 4, 3},
		{"2D six layers, undefined dimension, base layer 2, base mip 1", gputypes.TextureDimension2D, 6,
			&wgpu.TextureViewDescriptor{BaseArrayLayer: 2, BaseMipLevel: 1},
			gputypes.TextureViewDimension2DArray, 4, 2},
		{"2D four layers, explicit 2d, no layer count", gputypes.TextureDimension2D, 4,
			&wgpu.TextureViewDescriptor{Dimension: gputypes.TextureViewDimension2D, BaseArrayLayer: 1},
			gputypes.TextureViewDimension2D, 1, 3},
		{"2D six layers, explicit cube", gputypes.TextureDimension2D, 6,
			&wgpu.TextureViewDescriptor{Dimension: gputypes.TextureViewDimensionCube},
			gputypes.TextureViewDimensionCube, 6, 3},
		{"3D, nil descriptor", gputypes.TextureDimension3D, 4, nil, gputypes.TextureViewDimension3D, 1, 3},
		{"1D, nil descriptor", gputypes.TextureDimension1D, 1, nil, gputypes.TextureViewDimension1D, 1, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			height, mips := uint32(8), uint32(3)
			if tt.dim == gputypes.TextureDimension1D {
				height, mips = 1, 1
			}
			tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
				Size:          wgpu.Extent3D{Width: 8, Height: height, DepthOrArrayLayers: tt.layers},
				MipLevelCount: mips, SampleCount: 1, Dimension: tt.dim,
				Format: gputypes.TextureFormatRGBA8Unorm, Usage: wgpu.TextureUsageTextureBinding,
			})
			if err != nil {
				t.Fatalf("CreateTexture: %v", err)
			}
			defer tex.Release()
			rec.descs = rec.descs[:0]
			view, err := device.CreateTextureView(tex, tt.desc)
			if err != nil {
				t.Fatalf("CreateTextureView: %v", err)
			}
			defer view.Release()
			if len(rec.descs) != 1 {
				t.Fatalf("HAL got %d view descriptors, want 1", len(rec.descs))
			}
			got := rec.descs[0]
			if got.Dimension != tt.want || got.ArrayLayerCount != tt.wantLayers || got.MipLevelCount != tt.wantMips {
				t.Fatalf("HAL view = %v, %d layers, %d mips; want %v, %d layers, %d mips",
					got.Dimension, got.ArrayLayerCount, got.MipLevelCount, tt.want, tt.wantLayers, tt.wantMips)
			}
		})
	}
}
