//go:build !(js && wasm)

package software

import (
	"image"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

func TestRenderPassStats_ScissorAndDrawCount(t *testing.T) {
	backend := API{}
	instance, _ := backend.CreateInstance(&hal.InstanceDescriptor{})
	defer instance.Destroy()
	adapters := instance.EnumerateAdapters(nil)
	dev, _ := adapters[0].Adapter.Open(0, gputypes.DefaultLimits())
	defer dev.Device.Destroy()

	tex, _ := dev.Device.CreateTexture(&hal.TextureDescriptor{
		Size:          hal.Extent3D{Width: 100, Height: 100, DepthOrArrayLayers: 1},
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         gputypes.TextureUsageRenderAttachment,
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		ViewFormats:   nil,
	})
	defer tex.Destroy()
	view, _ := dev.Device.CreateTextureView(tex, nil)
	defer view.Destroy()

	enc, _ := dev.Device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{Label: "test"})
	pass := enc.BeginRenderPass(&hal.RenderPassDescriptor{
		ColorAttachments: []hal.RenderPassColorAttachment{
			{
				View:   view,
				LoadOp: gputypes.LoadOpLoad,
			},
		},
	})

	pass.SetScissorRect(24, 64, 48, 48)
	pass.Draw(6, 1, 0, 0)
	pass.Draw(6, 1, 0, 0)
	pass.Draw(3, 1, 0, 0)
	pass.End()

	stats := pass.(*RenderPassEncoder).Stats()

	if stats.DrawCount != 3 {
		t.Errorf("DrawCount = %d, want 3", stats.DrawCount)
	}
	if !stats.HasScissor {
		t.Error("HasScissor = false, want true")
	}
	want := image.Rect(24, 64, 72, 112)
	if stats.ScissorRect != want {
		t.Errorf("ScissorRect = %v, want %v", stats.ScissorRect, want)
	}
	if stats.Width != 100 || stats.Height != 100 {
		t.Errorf("Size = %dx%d, want 100x100", stats.Width, stats.Height)
	}
	if stats.ColorLoadOp != gputypes.LoadOpLoad {
		t.Errorf("ColorLoadOp = %v, want LoadOpLoad", stats.ColorLoadOp)
	}
}

func TestRenderPassStats_NoScissor(t *testing.T) {
	backend := API{}
	instance, _ := backend.CreateInstance(&hal.InstanceDescriptor{})
	defer instance.Destroy()
	adapters := instance.EnumerateAdapters(nil)
	dev, _ := adapters[0].Adapter.Open(0, gputypes.DefaultLimits())
	defer dev.Device.Destroy()

	tex, _ := dev.Device.CreateTexture(&hal.TextureDescriptor{
		Size:          hal.Extent3D{Width: 200, Height: 150, DepthOrArrayLayers: 1},
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         gputypes.TextureUsageRenderAttachment,
		MipLevelCount: 1, SampleCount: 1, Dimension: gputypes.TextureDimension2D,
	})
	defer tex.Destroy()
	view, _ := dev.Device.CreateTextureView(tex, nil)
	defer view.Destroy()

	enc, _ := dev.Device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{Label: "test"})
	pass := enc.BeginRenderPass(&hal.RenderPassDescriptor{
		ColorAttachments: []hal.RenderPassColorAttachment{
			{View: view, LoadOp: gputypes.LoadOpClear},
		},
	})
	pass.Draw(6, 1, 0, 0)
	pass.End()

	stats := pass.(*RenderPassEncoder).Stats()

	if stats.DrawCount != 1 {
		t.Errorf("DrawCount = %d, want 1", stats.DrawCount)
	}
	if stats.HasScissor {
		t.Error("HasScissor = true, want false")
	}
	if stats.ScissorRect != (image.Rectangle{}) {
		t.Errorf("ScissorRect = %v, want zero", stats.ScissorRect)
	}
	if stats.ColorLoadOp != gputypes.LoadOpClear {
		t.Errorf("ColorLoadOp = %v, want LoadOpClear", stats.ColorLoadOp)
	}
}
