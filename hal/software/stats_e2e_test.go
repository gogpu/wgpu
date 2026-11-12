//go:build !(js && wasm)

package software

import (
	"image"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// These three tests come from gogpu/ui (desktop/software_e2e_test.go,
// ui:P2-TEST-001) and are adopted here, at the layer that owns the property.
// ui deletes its copies once these are merged.

func newStatsDevice(t *testing.T) (hal.Device, func()) {
	t.Helper()
	instance, err := NewBackend().CreateInstance(&hal.InstanceDescriptor{})
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	adapters := instance.EnumerateAdapters(nil)
	if len(adapters) == 0 {
		instance.Destroy()
		t.Fatal("software backend: no adapters")
	}
	open, err := adapters[0].Adapter.Open(0, gputypes.DefaultLimits())
	if err != nil {
		instance.Destroy()
		t.Fatalf("Open: %v", err)
	}
	return open.Device, func() {
		open.Device.Destroy()
		instance.Destroy()
	}
}

func newStatsTarget(t *testing.T, device hal.Device, width, height uint32) (hal.Texture, hal.TextureView) {
	t.Helper()
	texture, err := device.CreateTexture(&hal.TextureDescriptor{
		Size:          hal.Extent3D{Width: width, Height: height, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         gputypes.TextureUsageRenderAttachment,
	})
	if err != nil {
		t.Fatalf("CreateTexture %dx%d: %v", width, height, err)
	}
	view, err := device.CreateTextureView(texture, nil)
	if err != nil {
		texture.Destroy()
		t.Fatalf("CreateTextureView %dx%d: %v", width, height, err)
	}
	return texture, view
}

// TestDamageAwareBlit_ScissorRect_MatchesSpinnerBounds proves that the damage
// pipeline sends the dirty bounds, not the full window, to the software
// rasterizer: a full clear frame carries no scissor, a damage frame carries
// exactly the spinner's rectangle and re-renders one draw.
func TestDamageAwareBlit_ScissorRect_MatchesSpinnerBounds(t *testing.T) {
	device, cleanup := newStatsDevice(t)
	defer cleanup()
	const (
		surfW, surfH       = 200, 200
		spinnerX, spinnerY = 80, 80
		spinnerW, spinnerH = 48, 48
	)
	texture, view := newStatsTarget(t, device, surfW, surfH)
	defer texture.Destroy()
	defer view.Destroy()

	encoder1, err := device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{Label: "frame1"})
	if err != nil {
		t.Fatalf("CreateCommandEncoder frame1: %v", err)
	}
	pass1 := encoder1.BeginRenderPass(&hal.RenderPassDescriptor{
		Label: "frame1-full",
		ColorAttachments: []hal.RenderPassColorAttachment{{
			View:       view,
			LoadOp:     gputypes.LoadOpClear,
			StoreOp:    gputypes.StoreOpStore,
			ClearValue: gputypes.Color{R: 1, A: 1},
		}},
	})
	pass1.End()
	stats1 := pass1.(*RenderPassEncoder).Stats()
	if stats1.ColorLoadOp != gputypes.LoadOpClear {
		t.Errorf("frame1: ColorLoadOp = %v, want Clear", stats1.ColorLoadOp)
	}
	if stats1.HasScissor {
		t.Error("frame1: HasScissor = true, want false (full window render)")
	}
	if stats1.Width != surfW || stats1.Height != surfH {
		t.Errorf("frame1: target = %dx%d, want %dx%d", stats1.Width, stats1.Height, surfW, surfH)
	}

	encoder2, err := device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{Label: "frame2"})
	if err != nil {
		t.Fatalf("CreateCommandEncoder frame2: %v", err)
	}
	pass2 := encoder2.BeginRenderPass(&hal.RenderPassDescriptor{
		Label: "frame2-damage",
		ColorAttachments: []hal.RenderPassColorAttachment{{
			View:    view,
			LoadOp:  gputypes.LoadOpLoad,
			StoreOp: gputypes.StoreOpStore,
		}},
	})
	pass2.SetViewport(gputypes.Viewport{Width: surfW, Height: surfH, MaxDepth: 1})
	pass2.SetScissorRect(gputypes.ScissorRect{X: spinnerX, Y: spinnerY, Width: spinnerW, Height: spinnerH})
	pass2.Draw(gputypes.DrawArgs{VertexCount: 6, InstanceCount: 1})
	pass2.End()
	stats2 := pass2.(*RenderPassEncoder).Stats()
	if stats2.ColorLoadOp != gputypes.LoadOpLoad {
		t.Errorf("frame2: ColorLoadOp = %v, want Load", stats2.ColorLoadOp)
	}
	if !stats2.HasScissor {
		t.Error("frame2: HasScissor = false, want true (damage-aware scissor)")
	}
	wantRect := image.Rect(spinnerX, spinnerY, spinnerX+spinnerW, spinnerY+spinnerH)
	if stats2.ScissorRect != wantRect {
		t.Errorf("frame2: ScissorRect = %v, want %v (spinner bounds)", stats2.ScissorRect, wantRect)
	}
	if stats2.DrawCount != 1 {
		t.Errorf("frame2: DrawCount = %d, want 1 (only spinner re-rendered)", stats2.DrawCount)
	}
	if stats2.Width != surfW || stats2.Height != surfH {
		t.Errorf("frame2: target = %dx%d, want %dx%d", stats2.Width, stats2.Height, surfW, surfH)
	}
	if stats2.ScissorRect.Dx() != spinnerW || stats2.ScissorRect.Dy() != spinnerH {
		t.Errorf("scissor size = %dx%d, want %dx%d (spinner only, NOT full window %dx%d)",
			stats2.ScissorRect.Dx(), stats2.ScissorRect.Dy(), spinnerW, spinnerH, surfW, surfH)
	}
}

// TestDamageAwareBlit_LoadOpLoad_Confirmed proves the foundational damage
// guarantee: consecutive frames record Clear, Load, Clear as requested.
func TestDamageAwareBlit_LoadOpLoad_Confirmed(t *testing.T) {
	device, cleanup := newStatsDevice(t)
	defer cleanup()
	const width, height = 100, 100
	texture, view := newStatsTarget(t, device, width, height)
	defer texture.Destroy()
	defer view.Destroy()

	tests := []struct {
		name   string
		loadOp gputypes.LoadOp
	}{
		{"frame1-clear", gputypes.LoadOpClear},
		{"frame2-load", gputypes.LoadOpLoad},
		{"frame3-clear-again", gputypes.LoadOpClear},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoder, err := device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{Label: tt.name})
			if err != nil {
				t.Fatalf("CreateCommandEncoder: %v", err)
			}
			pass := encoder.BeginRenderPass(&hal.RenderPassDescriptor{
				Label: tt.name,
				ColorAttachments: []hal.RenderPassColorAttachment{{
					View:       view,
					LoadOp:     tt.loadOp,
					StoreOp:    gputypes.StoreOpStore,
					ClearValue: gputypes.Color{A: 1},
				}},
			})
			pass.End()
			stats := pass.(*RenderPassEncoder).Stats()
			if stats.ColorLoadOp != tt.loadOp {
				t.Errorf("ColorLoadOp = %v, want %v", stats.ColorLoadOp, tt.loadOp)
			}
		})
	}
}

// TestScissorRect_ExactBounds proves the scissor keeps the exact dirty
// boundary pixels: origin, the far corner, odd sizes and a single pixel.
func TestScissorRect_ExactBounds(t *testing.T) {
	device, cleanup := newStatsDevice(t)
	defer cleanup()
	const surfW, surfH = 300, 300
	texture, view := newStatsTarget(t, device, surfW, surfH)
	defer texture.Destroy()
	defer view.Destroy()

	tests := []struct {
		name         string
		x, y, w, h   uint32
		wantMinX     int
		wantMinY     int
		wantMaxX     int
		wantMaxY     int
		wantScissorW int
		wantScissorH int
	}{
		{"standard-48x48-spinner", 80, 80, 48, 48, 80, 80, 128, 128, 48, 48},
		{"small-30x25-at-offset", 50, 70, 30, 25, 50, 70, 80, 95, 30, 25},
		{"origin-corner", 0, 0, 16, 16, 0, 0, 16, 16, 16, 16},
		{"bottom-right-corner", 260, 260, 40, 40, 260, 260, 300, 300, 40, 40},
		{"odd-dimensions-37x53", 100, 100, 37, 53, 100, 100, 137, 153, 37, 53},
		{"single-pixel", 150, 150, 1, 1, 150, 150, 151, 151, 1, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoder, err := device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{Label: tt.name})
			if err != nil {
				t.Fatalf("CreateCommandEncoder: %v", err)
			}
			pass := encoder.BeginRenderPass(&hal.RenderPassDescriptor{
				Label: tt.name,
				ColorAttachments: []hal.RenderPassColorAttachment{{
					View:    view,
					LoadOp:  gputypes.LoadOpLoad,
					StoreOp: gputypes.StoreOpStore,
				}},
			})
			pass.SetScissorRect(gputypes.ScissorRect{X: tt.x, Y: tt.y, Width: tt.w, Height: tt.h})
			pass.Draw(gputypes.DrawArgs{VertexCount: 6, InstanceCount: 1})
			pass.End()
			stats := pass.(*RenderPassEncoder).Stats()
			if !stats.HasScissor {
				t.Fatal("HasScissor = false, want true")
			}
			wantRect := image.Rect(tt.wantMinX, tt.wantMinY, tt.wantMaxX, tt.wantMaxY)
			if stats.ScissorRect != wantRect {
				t.Errorf("ScissorRect = %v, want %v", stats.ScissorRect, wantRect)
			}
			if stats.ScissorRect.Dx() != tt.wantScissorW || stats.ScissorRect.Dy() != tt.wantScissorH {
				t.Errorf("scissor size = %dx%d, want %dx%d",
					stats.ScissorRect.Dx(), stats.ScissorRect.Dy(), tt.wantScissorW, tt.wantScissorH)
			}
			if stats.DrawCount != 1 {
				t.Errorf("DrawCount = %d, want 1", stats.DrawCount)
			}
		})
	}
}
