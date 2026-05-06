//go:build !(js && wasm)

package software

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// =============================================================================
// State Tracking Tests
// =============================================================================

func TestRenderPassSetPipeline(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	rp, _ := dev.CreateRenderPipeline(&hal.RenderPipelineDescriptor{Label: "test-pipeline"})

	enc, _ := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
	pass := enc.BeginRenderPass(&hal.RenderPassDescriptor{
		ColorAttachments: []hal.RenderPassColorAttachment{},
	})

	encoder := pass.(*RenderPassEncoder)
	if encoder.pipeline != nil {
		t.Error("pipeline should be nil before SetPipeline")
	}

	pass.SetPipeline(rp)
	if encoder.pipeline == nil {
		t.Error("pipeline should be set after SetPipeline")
	}
	if encoder.pipeline.desc.Label != "test-pipeline" {
		t.Errorf("pipeline label = %q, want %q", encoder.pipeline.desc.Label, "test-pipeline")
	}

	pass.End()
}

func TestRenderPassSetBindGroup(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	tex, _ := dev.CreateTexture(&hal.TextureDescriptor{
		Size: hal.Extent3D{Width: 8, Height: 8, DepthOrArrayLayers: 1},
	})
	defer dev.DestroyTexture(tex)

	view, _ := dev.CreateTextureView(tex, &hal.TextureViewDescriptor{})
	defer dev.DestroyTextureView(view)

	bg, _ := dev.CreateBindGroup(&hal.BindGroupDescriptor{
		Label: "test-bg",
		Entries: []gputypes.BindGroupEntry{
			{
				Binding:  0,
				Resource: gputypes.TextureViewBinding{TextureView: view.NativeHandle()},
			},
		},
	})
	defer dev.DestroyBindGroup(bg)

	enc, _ := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
	pass := enc.BeginRenderPass(&hal.RenderPassDescriptor{
		ColorAttachments: []hal.RenderPassColorAttachment{},
	})

	encoder := pass.(*RenderPassEncoder)
	pass.SetBindGroup(0, bg, nil)

	if encoder.bindGroups[0] == nil {
		t.Error("bind group 0 should be set after SetBindGroup")
	}

	// Verify texture view was resolved
	resolvedBG := encoder.bindGroups[0]
	if resolvedBG.textureViews[0] == nil {
		t.Error("bind group should have resolved texture view at binding 0")
	}

	pass.End()
}

func TestRenderPassSetBindGroupOutOfRange(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	bg, _ := dev.CreateBindGroup(&hal.BindGroupDescriptor{Label: "test"})
	defer dev.DestroyBindGroup(bg)

	enc, _ := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
	pass := enc.BeginRenderPass(&hal.RenderPassDescriptor{
		ColorAttachments: []hal.RenderPassColorAttachment{},
	})

	// Index 4 is out of range (max 4 bind groups: 0-3)
	pass.SetBindGroup(4, bg, nil)

	encoder := pass.(*RenderPassEncoder)
	for i := range encoder.bindGroups {
		if encoder.bindGroups[i] != nil {
			t.Errorf("bind group %d should be nil (out of range set)", i)
		}
	}

	pass.End()
}

// =============================================================================
// Resource ID and Registry Tests
// =============================================================================

func TestResourceIDsAreUnique(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	tex1, _ := dev.CreateTexture(&hal.TextureDescriptor{
		Size: hal.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1},
	})
	tex2, _ := dev.CreateTexture(&hal.TextureDescriptor{
		Size: hal.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1},
	})
	defer dev.DestroyTexture(tex1)
	defer dev.DestroyTexture(tex2)

	if tex1.NativeHandle() == tex2.NativeHandle() {
		t.Error("two textures should have different native handles")
	}
	if tex1.NativeHandle() == 0 {
		t.Error("texture created via Device should have non-zero NativeHandle")
	}
}

func TestTextureViewRegistryResolution(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	tex, _ := dev.CreateTexture(&hal.TextureDescriptor{
		Size: hal.Extent3D{Width: 8, Height: 8, DepthOrArrayLayers: 1},
	})
	defer dev.DestroyTexture(tex)

	view, _ := dev.CreateTextureView(tex, &hal.TextureViewDescriptor{})
	defer dev.DestroyTextureView(view)

	handle := view.NativeHandle()
	if handle == 0 {
		t.Fatal("TextureView created via Device should have non-zero handle")
	}

	resolved := dev.lookupTextureView(handle)
	if resolved == nil {
		t.Fatal("lookupTextureView should find registered view")
	}
	if resolved.texture == nil {
		t.Fatal("resolved view should reference the texture")
	}
}

func TestBufferRegistryResolution(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	buf, _ := dev.CreateBuffer(&hal.BufferDescriptor{Size: 64})
	defer dev.DestroyBuffer(buf)

	handle := buf.NativeHandle()
	if handle == 0 {
		t.Fatal("Buffer created via Device should have non-zero handle")
	}

	resolved := dev.lookupBuffer(handle)
	if resolved == nil {
		t.Fatal("lookupBuffer should find registered buffer")
	}
}

// =============================================================================
// Draw() Tests
// =============================================================================

func TestDrawTexturedQuad(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	// Create source texture (4x4 red pixels).
	srcTex, _ := dev.CreateTexture(&hal.TextureDescriptor{
		Size:   hal.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1},
		Format: gputypes.TextureFormatRGBA8Unorm,
	})
	defer dev.DestroyTexture(srcTex)

	src := srcTex.(*Texture)
	src.Clear(gputypes.Color{R: 1, G: 0, B: 0, A: 1})

	srcView, _ := dev.CreateTextureView(srcTex, &hal.TextureViewDescriptor{})
	defer dev.DestroyTextureView(srcView)

	// Create target texture (4x4).
	dstTex, _ := dev.CreateTexture(&hal.TextureDescriptor{
		Size:   hal.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1},
		Format: gputypes.TextureFormatRGBA8Unorm,
		Usage:  gputypes.TextureUsageRenderAttachment,
	})
	defer dev.DestroyTexture(dstTex)

	dstView, _ := dev.CreateTextureView(dstTex, &hal.TextureViewDescriptor{})
	defer dev.DestroyTextureView(dstView)

	// Create pipeline and bind group.
	pipeline, _ := dev.CreateRenderPipeline(&hal.RenderPipelineDescriptor{Label: "textured-quad"})
	defer dev.DestroyRenderPipeline(pipeline)

	bg, _ := dev.CreateBindGroup(&hal.BindGroupDescriptor{
		Entries: []gputypes.BindGroupEntry{
			{
				Binding:  0,
				Resource: gputypes.TextureViewBinding{TextureView: srcView.NativeHandle()},
			},
		},
	})
	defer dev.DestroyBindGroup(bg)

	// Execute render pass with Draw.
	enc, _ := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
	pass := enc.BeginRenderPass(&hal.RenderPassDescriptor{
		ColorAttachments: []hal.RenderPassColorAttachment{
			{
				View:       dstView,
				LoadOp:     gputypes.LoadOpClear,
				StoreOp:    gputypes.StoreOpStore,
				ClearValue: gputypes.Color{R: 0, G: 0, B: 0, A: 0},
			},
		},
	})
	pass.SetPipeline(pipeline)
	pass.SetBindGroup(0, bg, nil)
	pass.Draw(6, 1, 0, 0)
	pass.End()

	// Verify the destination texture has the source's red pixels.
	dst := dstTex.(*Texture)
	data := dst.GetData()
	if data[0] != 255 || data[1] != 0 || data[2] != 0 || data[3] != 255 {
		t.Errorf("after Draw: pixel = (%d,%d,%d,%d), want (255,0,0,255)",
			data[0], data[1], data[2], data[3])
	}

	// Check last pixel too.
	lastIdx := len(data) - 4
	if data[lastIdx] != 255 || data[lastIdx+1] != 0 || data[lastIdx+2] != 0 || data[lastIdx+3] != 255 {
		t.Errorf("last pixel = (%d,%d,%d,%d), want (255,0,0,255)",
			data[lastIdx], data[lastIdx+1], data[lastIdx+2], data[lastIdx+3])
	}
}

func TestDrawClearsBeforeBlit(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	// Create source texture (2x2 green).
	srcTex, _ := dev.CreateTexture(&hal.TextureDescriptor{
		Size:   hal.Extent3D{Width: 2, Height: 2, DepthOrArrayLayers: 1},
		Format: gputypes.TextureFormatRGBA8Unorm,
	})
	defer dev.DestroyTexture(srcTex)

	src := srcTex.(*Texture)
	src.Clear(gputypes.Color{R: 0, G: 1, B: 0, A: 1})

	srcView, _ := dev.CreateTextureView(srcTex, &hal.TextureViewDescriptor{})
	defer dev.DestroyTextureView(srcView)

	// Create target (2x2) pre-filled with white.
	dstTex, _ := dev.CreateTexture(&hal.TextureDescriptor{
		Size:   hal.Extent3D{Width: 2, Height: 2, DepthOrArrayLayers: 1},
		Format: gputypes.TextureFormatRGBA8Unorm,
		Usage:  gputypes.TextureUsageRenderAttachment,
	})
	defer dev.DestroyTexture(dstTex)
	dstTex.(*Texture).Clear(gputypes.Color{R: 1, G: 1, B: 1, A: 1})

	dstView, _ := dev.CreateTextureView(dstTex, &hal.TextureViewDescriptor{})
	defer dev.DestroyTextureView(dstView)

	pipeline, _ := dev.CreateRenderPipeline(&hal.RenderPipelineDescriptor{Label: "test"})
	defer dev.DestroyRenderPipeline(pipeline)

	bg, _ := dev.CreateBindGroup(&hal.BindGroupDescriptor{
		Entries: []gputypes.BindGroupEntry{
			{Binding: 0, Resource: gputypes.TextureViewBinding{TextureView: srcView.NativeHandle()}},
		},
	})
	defer dev.DestroyBindGroup(bg)

	enc, _ := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
	pass := enc.BeginRenderPass(&hal.RenderPassDescriptor{
		ColorAttachments: []hal.RenderPassColorAttachment{
			{
				View:       dstView,
				LoadOp:     gputypes.LoadOpClear,
				ClearValue: gputypes.Color{R: 0, G: 0, B: 1, A: 1}, // clear to blue
			},
		},
	})
	pass.SetPipeline(pipeline)
	pass.SetBindGroup(0, bg, nil)
	pass.Draw(6, 1, 0, 0)
	pass.End()

	// The result should be green (from blit), NOT blue (from clear) or white (pre-fill).
	// Clear happens before Draw, then blit overwrites.
	data := dstTex.(*Texture).GetData()
	if data[0] != 0 || data[1] != 255 || data[2] != 0 || data[3] != 255 {
		t.Errorf("pixel = (%d,%d,%d,%d), want green (0,255,0,255)",
			data[0], data[1], data[2], data[3])
	}
}

func TestDrawWithoutPipeline(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	dstTex, _ := dev.CreateTexture(&hal.TextureDescriptor{
		Size:   hal.Extent3D{Width: 2, Height: 2, DepthOrArrayLayers: 1},
		Format: gputypes.TextureFormatRGBA8Unorm,
	})
	defer dev.DestroyTexture(dstTex)
	dstView, _ := dev.CreateTextureView(dstTex, &hal.TextureViewDescriptor{})
	defer dev.DestroyTextureView(dstView)

	enc, _ := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
	pass := enc.BeginRenderPass(&hal.RenderPassDescriptor{
		ColorAttachments: []hal.RenderPassColorAttachment{
			{View: dstView, LoadOp: gputypes.LoadOpClear, ClearValue: gputypes.Color{R: 1, G: 0, B: 0, A: 1}},
		},
	})

	// Draw without SetPipeline — should be a no-op (not panic).
	pass.Draw(6, 1, 0, 0)
	pass.End()

	// Only clear should have happened.
	data := dstTex.(*Texture).GetData()
	if data[0] != 255 || data[1] != 0 || data[2] != 0 || data[3] != 255 {
		t.Errorf("expected clear to red, got (%d,%d,%d,%d)", data[0], data[1], data[2], data[3])
	}
}

func TestDrawWithoutTexture(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	dstTex, _ := dev.CreateTexture(&hal.TextureDescriptor{
		Size:   hal.Extent3D{Width: 2, Height: 2, DepthOrArrayLayers: 1},
		Format: gputypes.TextureFormatRGBA8Unorm,
	})
	defer dev.DestroyTexture(dstTex)
	dstView, _ := dev.CreateTextureView(dstTex, &hal.TextureViewDescriptor{})
	defer dev.DestroyTextureView(dstView)

	pipeline, _ := dev.CreateRenderPipeline(&hal.RenderPipelineDescriptor{Label: "test"})
	defer dev.DestroyRenderPipeline(pipeline)

	// Bind group with only a buffer (no texture).
	buf, _ := dev.CreateBuffer(&hal.BufferDescriptor{Size: 64})
	defer dev.DestroyBuffer(buf)

	bg, _ := dev.CreateBindGroup(&hal.BindGroupDescriptor{
		Entries: []gputypes.BindGroupEntry{
			{Binding: 0, Resource: gputypes.BufferBinding{Buffer: buf.NativeHandle(), Size: 64}},
		},
	})
	defer dev.DestroyBindGroup(bg)

	enc, _ := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
	pass := enc.BeginRenderPass(&hal.RenderPassDescriptor{
		ColorAttachments: []hal.RenderPassColorAttachment{
			{View: dstView, LoadOp: gputypes.LoadOpClear, ClearValue: gputypes.Color{R: 0, G: 0, B: 1, A: 1}},
		},
	})
	pass.SetPipeline(pipeline)
	pass.SetBindGroup(0, bg, nil)
	pass.Draw(6, 1, 0, 0) // no source texture — should only clear
	pass.End()

	// Only clear to blue should have happened.
	data := dstTex.(*Texture).GetData()
	if data[0] != 0 || data[1] != 0 || data[2] != 255 || data[3] != 255 {
		t.Errorf("expected clear to blue, got (%d,%d,%d,%d)", data[0], data[1], data[2], data[3])
	}
}

func TestDrawBGRAToRGBAConversion(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	// Source is RGBA.
	srcTex, _ := dev.CreateTexture(&hal.TextureDescriptor{
		Size:   hal.Extent3D{Width: 2, Height: 2, DepthOrArrayLayers: 1},
		Format: gputypes.TextureFormatRGBA8Unorm,
	})
	defer dev.DestroyTexture(srcTex)

	src := srcTex.(*Texture)
	// Write a known pixel: R=255, G=0, B=128, A=200.
	for i := 0; i < len(src.data); i += 4 {
		src.data[i+0] = 255 // R
		src.data[i+1] = 0   // G
		src.data[i+2] = 128 // B
		src.data[i+3] = 200 // A
	}

	srcView, _ := dev.CreateTextureView(srcTex, &hal.TextureViewDescriptor{})
	defer dev.DestroyTextureView(srcView)

	// Target is BGRA.
	dstTex, _ := dev.CreateTexture(&hal.TextureDescriptor{
		Size:   hal.Extent3D{Width: 2, Height: 2, DepthOrArrayLayers: 1},
		Format: gputypes.TextureFormatBGRA8Unorm,
		Usage:  gputypes.TextureUsageRenderAttachment,
	})
	defer dev.DestroyTexture(dstTex)
	dstView, _ := dev.CreateTextureView(dstTex, &hal.TextureViewDescriptor{})
	defer dev.DestroyTextureView(dstView)

	pipeline, _ := dev.CreateRenderPipeline(&hal.RenderPipelineDescriptor{Label: "test"})
	defer dev.DestroyRenderPipeline(pipeline)

	bg, _ := dev.CreateBindGroup(&hal.BindGroupDescriptor{
		Entries: []gputypes.BindGroupEntry{
			{Binding: 0, Resource: gputypes.TextureViewBinding{TextureView: srcView.NativeHandle()}},
		},
	})
	defer dev.DestroyBindGroup(bg)

	enc, _ := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
	pass := enc.BeginRenderPass(&hal.RenderPassDescriptor{
		ColorAttachments: []hal.RenderPassColorAttachment{
			{View: dstView, LoadOp: gputypes.LoadOpLoad},
		},
	})
	pass.SetPipeline(pipeline)
	pass.SetBindGroup(0, bg, nil)
	pass.Draw(6, 1, 0, 0)
	pass.End()

	// RGBA(255,0,128,200) -> BGRA should be (128,0,255,200).
	data := dstTex.(*Texture).GetData()
	if data[0] != 128 || data[1] != 0 || data[2] != 255 || data[3] != 200 {
		t.Errorf("BGRA conversion: got (%d,%d,%d,%d), want (128,0,255,200)",
			data[0], data[1], data[2], data[3])
	}
}

func TestDrawScaling(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	// Source: 2x2 with distinct pixels.
	srcTex, _ := dev.CreateTexture(&hal.TextureDescriptor{
		Size:   hal.Extent3D{Width: 2, Height: 2, DepthOrArrayLayers: 1},
		Format: gputypes.TextureFormatRGBA8Unorm,
	})
	defer dev.DestroyTexture(srcTex)

	src := srcTex.(*Texture)
	// TL=red, TR=green, BL=blue, BR=white
	copy(src.data[0:4], []byte{255, 0, 0, 255})
	copy(src.data[4:8], []byte{0, 255, 0, 255})
	copy(src.data[8:12], []byte{0, 0, 255, 255})
	copy(src.data[12:16], []byte{255, 255, 255, 255})

	srcView, _ := dev.CreateTextureView(srcTex, &hal.TextureViewDescriptor{})
	defer dev.DestroyTextureView(srcView)

	// Target: 4x4 (scaled up from 2x2).
	dstTex, _ := dev.CreateTexture(&hal.TextureDescriptor{
		Size:   hal.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1},
		Format: gputypes.TextureFormatRGBA8Unorm,
		Usage:  gputypes.TextureUsageRenderAttachment,
	})
	defer dev.DestroyTexture(dstTex)
	dstView, _ := dev.CreateTextureView(dstTex, &hal.TextureViewDescriptor{})
	defer dev.DestroyTextureView(dstView)

	pipeline, _ := dev.CreateRenderPipeline(&hal.RenderPipelineDescriptor{Label: "test"})
	defer dev.DestroyRenderPipeline(pipeline)

	bg, _ := dev.CreateBindGroup(&hal.BindGroupDescriptor{
		Entries: []gputypes.BindGroupEntry{
			{Binding: 0, Resource: gputypes.TextureViewBinding{TextureView: srcView.NativeHandle()}},
		},
	})
	defer dev.DestroyBindGroup(bg)

	enc, _ := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
	pass := enc.BeginRenderPass(&hal.RenderPassDescriptor{
		ColorAttachments: []hal.RenderPassColorAttachment{
			{View: dstView, LoadOp: gputypes.LoadOpLoad},
		},
	})
	pass.SetPipeline(pipeline)
	pass.SetBindGroup(0, bg, nil)
	pass.Draw(6, 1, 0, 0)
	pass.End()

	// Verify corners with nearest-neighbor scaling.
	data := dstTex.(*Texture).GetData()

	// Top-left (0,0) should be red (from src 0,0).
	if data[0] != 255 || data[1] != 0 || data[2] != 0 {
		t.Errorf("TL = (%d,%d,%d), want red", data[0], data[1], data[2])
	}

	// Top-right (3,0) should be green (from src 1,0).
	idx := 3 * 4
	if data[idx] != 0 || data[idx+1] != 255 || data[idx+2] != 0 {
		t.Errorf("TR = (%d,%d,%d), want green", data[idx], data[idx+1], data[idx+2])
	}

	// Bottom-left (0,3) should be blue (from src 0,1).
	idx = 3*4*4 + 0
	if data[idx] != 0 || data[idx+1] != 0 || data[idx+2] != 255 {
		t.Errorf("BL = (%d,%d,%d), want blue", data[idx], data[idx+1], data[idx+2])
	}
}

func TestDrawWithSurfaceTexture(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	// Configure a surface.
	backend := API{}
	instance, _ := backend.CreateInstance(&hal.InstanceDescriptor{})
	defer instance.Destroy()

	surface, _ := instance.CreateSurface(0, 0)
	defer surface.Destroy()

	_ = surface.Configure(dev, &hal.SurfaceConfiguration{
		Width:       4,
		Height:      4,
		Format:      gputypes.TextureFormatRGBA8Unorm,
		PresentMode: hal.PresentModeImmediate,
	})

	acquired, _ := surface.AcquireTexture(nil)
	surfTex := acquired.Texture

	// Create a view of the surface texture.
	surfView, _ := dev.CreateTextureView(surfTex, &hal.TextureViewDescriptor{})
	defer dev.DestroyTextureView(surfView)

	// Create source texture (green).
	srcTex, _ := dev.CreateTexture(&hal.TextureDescriptor{
		Size:   hal.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1},
		Format: gputypes.TextureFormatRGBA8Unorm,
	})
	defer dev.DestroyTexture(srcTex)
	srcTex.(*Texture).Clear(gputypes.Color{R: 0, G: 1, B: 0, A: 1})

	srcView, _ := dev.CreateTextureView(srcTex, &hal.TextureViewDescriptor{})
	defer dev.DestroyTextureView(srcView)

	pipeline, _ := dev.CreateRenderPipeline(&hal.RenderPipelineDescriptor{Label: "test"})
	defer dev.DestroyRenderPipeline(pipeline)

	bg, _ := dev.CreateBindGroup(&hal.BindGroupDescriptor{
		Entries: []gputypes.BindGroupEntry{
			{Binding: 0, Resource: gputypes.TextureViewBinding{TextureView: srcView.NativeHandle()}},
		},
	})
	defer dev.DestroyBindGroup(bg)

	enc, _ := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
	pass := enc.BeginRenderPass(&hal.RenderPassDescriptor{
		ColorAttachments: []hal.RenderPassColorAttachment{
			{View: surfView, LoadOp: gputypes.LoadOpLoad},
		},
	})
	pass.SetPipeline(pipeline)
	pass.SetBindGroup(0, bg, nil)
	pass.Draw(6, 1, 0, 0)
	pass.End()

	// The surface framebuffer should now have green pixels.
	surf := surface.(*Surface)
	fb := surf.GetFramebuffer()
	if fb[0] != 0 || fb[1] != 255 || fb[2] != 0 || fb[3] != 255 {
		t.Errorf("surface pixel = (%d,%d,%d,%d), want green (0,255,0,255)",
			fb[0], fb[1], fb[2], fb[3])
	}
}

func TestDrawMultipleBindGroups(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	// Source texture in bind group 1 (like ggcanvas pattern).
	srcTex, _ := dev.CreateTexture(&hal.TextureDescriptor{
		Size:   hal.Extent3D{Width: 2, Height: 2, DepthOrArrayLayers: 1},
		Format: gputypes.TextureFormatRGBA8Unorm,
	})
	defer dev.DestroyTexture(srcTex)
	srcTex.(*Texture).Clear(gputypes.Color{R: 0.5, G: 0.5, B: 0.5, A: 1})

	srcView, _ := dev.CreateTextureView(srcTex, &hal.TextureViewDescriptor{})
	defer dev.DestroyTextureView(srcView)

	// Bind group 0: uniform buffer only.
	buf, _ := dev.CreateBuffer(&hal.BufferDescriptor{Size: 64})
	defer dev.DestroyBuffer(buf)

	bg0, _ := dev.CreateBindGroup(&hal.BindGroupDescriptor{
		Entries: []gputypes.BindGroupEntry{
			{Binding: 0, Resource: gputypes.BufferBinding{Buffer: buf.NativeHandle(), Size: 64}},
		},
	})
	defer dev.DestroyBindGroup(bg0)

	// Bind group 1: texture.
	bg1, _ := dev.CreateBindGroup(&hal.BindGroupDescriptor{
		Entries: []gputypes.BindGroupEntry{
			{Binding: 0, Resource: gputypes.TextureViewBinding{TextureView: srcView.NativeHandle()}},
		},
	})
	defer dev.DestroyBindGroup(bg1)

	// Target.
	dstTex, _ := dev.CreateTexture(&hal.TextureDescriptor{
		Size:   hal.Extent3D{Width: 2, Height: 2, DepthOrArrayLayers: 1},
		Format: gputypes.TextureFormatRGBA8Unorm,
		Usage:  gputypes.TextureUsageRenderAttachment,
	})
	defer dev.DestroyTexture(dstTex)
	dstView, _ := dev.CreateTextureView(dstTex, &hal.TextureViewDescriptor{})
	defer dev.DestroyTextureView(dstView)

	pipeline, _ := dev.CreateRenderPipeline(&hal.RenderPipelineDescriptor{Label: "test"})
	defer dev.DestroyRenderPipeline(pipeline)

	enc, _ := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
	pass := enc.BeginRenderPass(&hal.RenderPassDescriptor{
		ColorAttachments: []hal.RenderPassColorAttachment{
			{View: dstView, LoadOp: gputypes.LoadOpLoad},
		},
	})
	pass.SetPipeline(pipeline)
	pass.SetBindGroup(0, bg0, nil) // no texture
	pass.SetBindGroup(1, bg1, nil) // texture is here
	pass.Draw(6, 1, 0, 0)
	pass.End()

	// Should find texture from bind group 1 and blit.
	data := dstTex.(*Texture).GetData()
	// 0.5 * 255 = 127
	if data[0] != 127 {
		t.Errorf("pixel R = %d, want ~127 (gray blit)", data[0])
	}
}

// =============================================================================
// Vertex Buffer Draw Tests
// =============================================================================

// writeFloat32 writes a float32 to a byte slice at the given offset (little-endian).
func writeFloat32(b []byte, offset int, v float32) {
	binary.LittleEndian.PutUint32(b[offset:], math.Float32bits(v))
}

func TestDrawTriangleFromVertexBuffer(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	// Target: 8x8 texture, cleared to black.
	dstTex, _ := dev.CreateTexture(&hal.TextureDescriptor{
		Size:   hal.Extent3D{Width: 8, Height: 8, DepthOrArrayLayers: 1},
		Format: gputypes.TextureFormatRGBA8Unorm,
		Usage:  gputypes.TextureUsageRenderAttachment,
	})
	defer dev.DestroyTexture(dstTex)
	dstView, _ := dev.CreateTextureView(dstTex, &hal.TextureViewDescriptor{})
	defer dev.DestroyTextureView(dstView)

	// Vertex buffer: 3 vertices in NDC.
	// Triangle covering the top-left quadrant:
	//   V0=(-1, 1, 0)  -> screen (0, 0)
	//   V1=( 0, 1, 0)  -> screen (4, 0)
	//   V2=(-1, 0, 0)  -> screen (0, 4)
	stride := uint64(12) // 3 x float32
	vbData := make([]byte, stride*3)
	writeFloat32(vbData, 0, -1.0)
	writeFloat32(vbData, 4, 1.0)
	writeFloat32(vbData, 8, 0.0)
	writeFloat32(vbData, 12, 0.0)
	writeFloat32(vbData, 16, 1.0)
	writeFloat32(vbData, 20, 0.0)
	writeFloat32(vbData, 24, -1.0)
	writeFloat32(vbData, 28, 0.0)
	writeFloat32(vbData, 32, 0.0)

	vb, _ := dev.CreateBuffer(&hal.BufferDescriptor{Size: uint64(len(vbData))})
	defer dev.DestroyBuffer(vb)
	vb.(*Buffer).WriteData(0, vbData)

	// Pipeline with vertex layout.
	pipeline, _ := dev.CreateRenderPipeline(&hal.RenderPipelineDescriptor{
		Label: "triangle",
		Vertex: hal.VertexState{
			Buffers: []gputypes.VertexBufferLayout{
				{
					ArrayStride: stride,
					StepMode:    gputypes.VertexStepModeVertex,
					Attributes: []gputypes.VertexAttribute{
						{Format: gputypes.VertexFormatFloat32x3, Offset: 0, ShaderLocation: 0},
					},
				},
			},
		},
	})
	defer dev.DestroyRenderPipeline(pipeline)

	enc, _ := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
	pass := enc.BeginRenderPass(&hal.RenderPassDescriptor{
		ColorAttachments: []hal.RenderPassColorAttachment{
			{
				View:       dstView,
				LoadOp:     gputypes.LoadOpClear,
				ClearValue: gputypes.Color{R: 0, G: 0, B: 0, A: 0},
			},
		},
	})
	pass.SetPipeline(pipeline)
	pass.SetVertexBuffer(0, vb, 0)
	pass.Draw(3, 1, 0, 0)
	pass.End()

	// The triangle covers the top-left area. Check pixel (1,1) is white (default color).
	data := dstTex.(*Texture).GetData()
	idx := (1*8 + 1) * 4
	if data[idx+0] != 255 || data[idx+1] != 255 || data[idx+2] != 255 || data[idx+3] != 255 {
		t.Errorf("pixel(1,1) = (%d,%d,%d,%d), want white (255,255,255,255)",
			data[idx], data[idx+1], data[idx+2], data[idx+3])
	}

	// Bottom-right pixel (7,7) should still be black (clear color, outside triangle).
	idx = (7*8 + 7) * 4
	if data[idx+0] != 0 || data[idx+1] != 0 || data[idx+2] != 0 {
		t.Errorf("pixel(7,7) = (%d,%d,%d,%d), want black (0,0,0,0)",
			data[idx], data[idx+1], data[idx+2], data[idx+3])
	}
}

func TestDrawTriangleWithVertexColors(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	// 4x4 target.
	dstTex, _ := dev.CreateTexture(&hal.TextureDescriptor{
		Size:   hal.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1},
		Format: gputypes.TextureFormatRGBA8Unorm,
		Usage:  gputypes.TextureUsageRenderAttachment,
	})
	defer dev.DestroyTexture(dstTex)
	dstView, _ := dev.CreateTextureView(dstTex, &hal.TextureViewDescriptor{})
	defer dev.DestroyTextureView(dstView)

	// Fullscreen triangle with vertex colors: pos(3 floats) + color(4 floats) = 28 bytes.
	stride := uint64(28)
	// Three vertices covering the entire viewport.
	// NDC: (-1,-1)=BL  (3,−1)=far right  (−1,3)=far top
	// This oversized triangle covers the full viewport.
	vbData := make([]byte, stride*3)
	// V0: position(-1, -1, 0), color(1, 0, 0, 1) = red
	writeFloat32(vbData, 0, -1.0)
	writeFloat32(vbData, 4, -1.0)
	writeFloat32(vbData, 8, 0.0)
	writeFloat32(vbData, 12, 1.0)
	writeFloat32(vbData, 16, 0.0)
	writeFloat32(vbData, 20, 0.0)
	writeFloat32(vbData, 24, 1.0)
	// V1: position(3, -1, 0), color(0, 1, 0, 1) = green
	writeFloat32(vbData, 28, 3.0)
	writeFloat32(vbData, 32, -1.0)
	writeFloat32(vbData, 36, 0.0)
	writeFloat32(vbData, 40, 0.0)
	writeFloat32(vbData, 44, 1.0)
	writeFloat32(vbData, 48, 0.0)
	writeFloat32(vbData, 52, 1.0)
	// V2: position(-1, 3, 0), color(0, 0, 1, 1) = blue
	writeFloat32(vbData, 56, -1.0)
	writeFloat32(vbData, 60, 3.0)
	writeFloat32(vbData, 64, 0.0)
	writeFloat32(vbData, 68, 0.0)
	writeFloat32(vbData, 72, 0.0)
	writeFloat32(vbData, 76, 1.0)
	writeFloat32(vbData, 80, 1.0)

	vb, _ := dev.CreateBuffer(&hal.BufferDescriptor{Size: uint64(len(vbData))})
	defer dev.DestroyBuffer(vb)
	vb.(*Buffer).WriteData(0, vbData)

	pipeline, _ := dev.CreateRenderPipeline(&hal.RenderPipelineDescriptor{
		Label: "vertex-color",
		Vertex: hal.VertexState{
			Buffers: []gputypes.VertexBufferLayout{
				{
					ArrayStride: stride,
					StepMode:    gputypes.VertexStepModeVertex,
					Attributes: []gputypes.VertexAttribute{
						{Format: gputypes.VertexFormatFloat32x3, Offset: 0, ShaderLocation: 0},
						{Format: gputypes.VertexFormatFloat32x4, Offset: 12, ShaderLocation: 1},
					},
				},
			},
		},
	})
	defer dev.DestroyRenderPipeline(pipeline)

	enc, _ := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
	pass := enc.BeginRenderPass(&hal.RenderPassDescriptor{
		ColorAttachments: []hal.RenderPassColorAttachment{
			{View: dstView, LoadOp: gputypes.LoadOpClear, ClearValue: gputypes.Color{}},
		},
	})
	pass.SetPipeline(pipeline)
	pass.SetVertexBuffer(0, vb, 0)
	pass.Draw(3, 1, 0, 0)
	pass.End()

	// Center pixel should be a blend of R/G/B. Verify it is not black (was rendered).
	data := dstTex.(*Texture).GetData()
	idx := (2*4 + 2) * 4 // pixel (2,2)
	if data[idx+3] == 0 {
		t.Error("center pixel alpha = 0, expected non-zero (triangle should cover it)")
	}
	// Alpha should be 255 (all three vertices have A=1.0).
	if data[idx+3] != 255 {
		t.Errorf("center pixel alpha = %d, want 255", data[idx+3])
	}
	// At least one channel should be non-zero (interpolated color).
	if data[idx+0] == 0 && data[idx+1] == 0 && data[idx+2] == 0 {
		t.Error("center pixel is black, expected interpolated color")
	}
}

func TestDrawMultipleTriangles(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	// 4x4 target.
	dstTex, _ := dev.CreateTexture(&hal.TextureDescriptor{
		Size:   hal.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1},
		Format: gputypes.TextureFormatRGBA8Unorm,
		Usage:  gputypes.TextureUsageRenderAttachment,
	})
	defer dev.DestroyTexture(dstTex)
	dstView, _ := dev.CreateTextureView(dstTex, &hal.TextureViewDescriptor{})
	defer dev.DestroyTextureView(dstView)

	// Two triangles forming a fullscreen quad (6 vertices).
	stride := uint64(12)
	vbData := make([]byte, stride*6)

	// Triangle 1: top-left
	writeFloat32(vbData, 0, -1.0)
	writeFloat32(vbData, 4, 1.0)
	writeFloat32(vbData, 8, 0.0)
	writeFloat32(vbData, 12, 1.0)
	writeFloat32(vbData, 16, 1.0)
	writeFloat32(vbData, 20, 0.0)
	writeFloat32(vbData, 24, -1.0)
	writeFloat32(vbData, 28, -1.0)
	writeFloat32(vbData, 32, 0.0)

	// Triangle 2: bottom-right
	writeFloat32(vbData, 36, 1.0)
	writeFloat32(vbData, 40, 1.0)
	writeFloat32(vbData, 44, 0.0)
	writeFloat32(vbData, 48, 1.0)
	writeFloat32(vbData, 52, -1.0)
	writeFloat32(vbData, 56, 0.0)
	writeFloat32(vbData, 60, -1.0)
	writeFloat32(vbData, 64, -1.0)
	writeFloat32(vbData, 68, 0.0)

	vb, _ := dev.CreateBuffer(&hal.BufferDescriptor{Size: uint64(len(vbData))})
	defer dev.DestroyBuffer(vb)
	vb.(*Buffer).WriteData(0, vbData)

	pipeline, _ := dev.CreateRenderPipeline(&hal.RenderPipelineDescriptor{
		Label: "quad",
		Vertex: hal.VertexState{
			Buffers: []gputypes.VertexBufferLayout{
				{
					ArrayStride: stride,
					StepMode:    gputypes.VertexStepModeVertex,
					Attributes: []gputypes.VertexAttribute{
						{Format: gputypes.VertexFormatFloat32x3, Offset: 0, ShaderLocation: 0},
					},
				},
			},
		},
	})
	defer dev.DestroyRenderPipeline(pipeline)

	enc, _ := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
	pass := enc.BeginRenderPass(&hal.RenderPassDescriptor{
		ColorAttachments: []hal.RenderPassColorAttachment{
			{View: dstView, LoadOp: gputypes.LoadOpClear, ClearValue: gputypes.Color{}},
		},
	})
	pass.SetPipeline(pipeline)
	pass.SetVertexBuffer(0, vb, 0)
	pass.Draw(6, 1, 0, 0)
	pass.End()

	// All pixels should be white (two triangles cover full viewport, default white color).
	data := dstTex.(*Texture).GetData()
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			idx := (y*4 + x) * 4
			if data[idx+0] != 255 || data[idx+1] != 255 || data[idx+2] != 255 || data[idx+3] != 255 {
				t.Errorf("pixel(%d,%d) = (%d,%d,%d,%d), want white",
					x, y, data[idx], data[idx+1], data[idx+2], data[idx+3])
			}
		}
	}
}

func TestDrawWithVertexBufferOffset(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	dstTex, _ := dev.CreateTexture(&hal.TextureDescriptor{
		Size:   hal.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1},
		Format: gputypes.TextureFormatRGBA8Unorm,
		Usage:  gputypes.TextureUsageRenderAttachment,
	})
	defer dev.DestroyTexture(dstTex)
	dstView, _ := dev.CreateTextureView(dstTex, &hal.TextureViewDescriptor{})
	defer dev.DestroyTextureView(dstView)

	// Buffer with 64 bytes of padding before 3 vertices.
	stride := uint64(12)
	padding := uint64(64)
	vbData := make([]byte, padding+stride*3)

	// Write a fullscreen triangle after the padding.
	off := int(padding)
	writeFloat32(vbData, off+0, -1.0)
	writeFloat32(vbData, off+4, -1.0)
	writeFloat32(vbData, off+8, 0.0)
	writeFloat32(vbData, off+12, 3.0)
	writeFloat32(vbData, off+16, -1.0)
	writeFloat32(vbData, off+20, 0.0)
	writeFloat32(vbData, off+24, -1.0)
	writeFloat32(vbData, off+28, 3.0)
	writeFloat32(vbData, off+32, 0.0)

	vb, _ := dev.CreateBuffer(&hal.BufferDescriptor{Size: uint64(len(vbData))})
	defer dev.DestroyBuffer(vb)
	vb.(*Buffer).WriteData(0, vbData)

	pipeline, _ := dev.CreateRenderPipeline(&hal.RenderPipelineDescriptor{
		Label: "offset-test",
		Vertex: hal.VertexState{
			Buffers: []gputypes.VertexBufferLayout{
				{
					ArrayStride: stride,
					StepMode:    gputypes.VertexStepModeVertex,
					Attributes: []gputypes.VertexAttribute{
						{Format: gputypes.VertexFormatFloat32x3, Offset: 0, ShaderLocation: 0},
					},
				},
			},
		},
	})
	defer dev.DestroyRenderPipeline(pipeline)

	enc, _ := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
	pass := enc.BeginRenderPass(&hal.RenderPassDescriptor{
		ColorAttachments: []hal.RenderPassColorAttachment{
			{View: dstView, LoadOp: gputypes.LoadOpClear, ClearValue: gputypes.Color{}},
		},
	})
	pass.SetPipeline(pipeline)
	pass.SetVertexBuffer(0, vb, padding) // offset=64
	pass.Draw(3, 1, 0, 0)
	pass.End()

	// Center pixel should be white (triangle covers full viewport).
	data := dstTex.(*Texture).GetData()
	idx := (2*4 + 2) * 4
	if data[idx+0] != 255 || data[idx+1] != 255 || data[idx+2] != 255 {
		t.Errorf("pixel(2,2) = (%d,%d,%d,%d), want white",
			data[idx], data[idx+1], data[idx+2], data[idx+3])
	}
}

func TestDrawClearBeforeDraw(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	dstTex, _ := dev.CreateTexture(&hal.TextureDescriptor{
		Size:   hal.Extent3D{Width: 2, Height: 2, DepthOrArrayLayers: 1},
		Format: gputypes.TextureFormatRGBA8Unorm,
		Usage:  gputypes.TextureUsageRenderAttachment,
	})
	defer dev.DestroyTexture(dstTex)
	// Pre-fill with red.
	dstTex.(*Texture).Clear(gputypes.Color{R: 1, G: 0, B: 0, A: 1})

	dstView, _ := dev.CreateTextureView(dstTex, &hal.TextureViewDescriptor{})
	defer dev.DestroyTextureView(dstView)

	// Pipeline with no vertex buffer layout (will trigger blit path but no texture = clear only).
	pipeline, _ := dev.CreateRenderPipeline(&hal.RenderPipelineDescriptor{Label: "clear-test"})
	defer dev.DestroyRenderPipeline(pipeline)

	enc, _ := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
	pass := enc.BeginRenderPass(&hal.RenderPassDescriptor{
		ColorAttachments: []hal.RenderPassColorAttachment{
			{
				View:       dstView,
				LoadOp:     gputypes.LoadOpClear,
				ClearValue: gputypes.Color{R: 0, G: 0, B: 1, A: 1}, // clear to blue
			},
		},
	})
	pass.SetPipeline(pipeline)
	pass.Draw(6, 1, 0, 0) // No vertex buffer, no texture -> just clear
	pass.End()

	// Should be blue (clear happened before draw, no texture to blit).
	data := dstTex.(*Texture).GetData()
	if data[0] != 0 || data[1] != 0 || data[2] != 255 || data[3] != 255 {
		t.Errorf("pixel = (%d,%d,%d,%d), want blue (0,0,255,255)",
			data[0], data[1], data[2], data[3])
	}
}

func TestDrawWithFirstVertex(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	dstTex, _ := dev.CreateTexture(&hal.TextureDescriptor{
		Size:   hal.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1},
		Format: gputypes.TextureFormatRGBA8Unorm,
		Usage:  gputypes.TextureUsageRenderAttachment,
	})
	defer dev.DestroyTexture(dstTex)
	dstView, _ := dev.CreateTextureView(dstTex, &hal.TextureViewDescriptor{})
	defer dev.DestroyTextureView(dstView)

	// Buffer with 6 vertices: first 3 form a small triangle, next 3 a fullscreen triangle.
	stride := uint64(12)
	vbData := make([]byte, stride*6)

	// First 3 vertices: tiny degenerate triangle at origin.
	// Skip these via firstVertex=3.

	// Vertices 3-5: fullscreen triangle.
	writeFloat32(vbData, 36, -1.0)
	writeFloat32(vbData, 40, -1.0)
	writeFloat32(vbData, 44, 0.0)
	writeFloat32(vbData, 48, 3.0)
	writeFloat32(vbData, 52, -1.0)
	writeFloat32(vbData, 56, 0.0)
	writeFloat32(vbData, 60, -1.0)
	writeFloat32(vbData, 64, 3.0)
	writeFloat32(vbData, 68, 0.0)

	vb, _ := dev.CreateBuffer(&hal.BufferDescriptor{Size: uint64(len(vbData))})
	defer dev.DestroyBuffer(vb)
	vb.(*Buffer).WriteData(0, vbData)

	pipeline, _ := dev.CreateRenderPipeline(&hal.RenderPipelineDescriptor{
		Label: "first-vertex",
		Vertex: hal.VertexState{
			Buffers: []gputypes.VertexBufferLayout{
				{
					ArrayStride: stride,
					StepMode:    gputypes.VertexStepModeVertex,
					Attributes: []gputypes.VertexAttribute{
						{Format: gputypes.VertexFormatFloat32x3, Offset: 0, ShaderLocation: 0},
					},
				},
			},
		},
	})
	defer dev.DestroyRenderPipeline(pipeline)

	enc, _ := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
	pass := enc.BeginRenderPass(&hal.RenderPassDescriptor{
		ColorAttachments: []hal.RenderPassColorAttachment{
			{View: dstView, LoadOp: gputypes.LoadOpClear, ClearValue: gputypes.Color{}},
		},
	})
	pass.SetPipeline(pipeline)
	pass.SetVertexBuffer(0, vb, 0)
	pass.Draw(3, 1, 3, 0) // firstVertex=3
	pass.End()

	// Center pixel should be white.
	data := dstTex.(*Texture).GetData()
	idx := (2*4 + 2) * 4
	if data[idx+0] != 255 || data[idx+1] != 255 || data[idx+2] != 255 {
		t.Errorf("pixel(2,2) = (%d,%d,%d,%d), want white",
			data[idx], data[idx+1], data[idx+2], data[idx+3])
	}
}

func TestSetVertexBufferSlot(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	buf, _ := dev.CreateBuffer(&hal.BufferDescriptor{Size: 64})
	defer dev.DestroyBuffer(buf)

	enc, _ := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
	pass := enc.BeginRenderPass(&hal.RenderPassDescriptor{
		ColorAttachments: []hal.RenderPassColorAttachment{},
	})

	encoder := pass.(*RenderPassEncoder)

	pass.SetVertexBuffer(0, buf, 0)
	pass.SetVertexBuffer(3, buf, 32)

	if encoder.vertexBufs[0].buffer == nil {
		t.Error("slot 0 should have buffer")
	}
	if encoder.vertexBufs[3].buffer == nil {
		t.Error("slot 3 should have buffer")
	}
	if encoder.vertexBufs[3].offset != 32 {
		t.Errorf("slot 3 offset = %d, want 32", encoder.vertexBufs[3].offset)
	}

	// Out of range slot.
	pass.SetVertexBuffer(8, buf, 0)
	// Should not panic, slot 8 out of range.

	pass.End()
}

func TestSetIndexBuffer(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	buf, _ := dev.CreateBuffer(&hal.BufferDescriptor{Size: 64})
	defer dev.DestroyBuffer(buf)

	enc, _ := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
	pass := enc.BeginRenderPass(&hal.RenderPassDescriptor{
		ColorAttachments: []hal.RenderPassColorAttachment{},
	})

	encoder := pass.(*RenderPassEncoder)

	pass.SetIndexBuffer(buf, gputypes.IndexFormatUint16, 10)

	if encoder.indexBuffer == nil {
		t.Error("index buffer should be set")
	}
	if encoder.indexFormat != gputypes.IndexFormatUint16 {
		t.Errorf("index format = %v, want Uint16", encoder.indexFormat)
	}
	if encoder.indexOffset != 10 {
		t.Errorf("index offset = %d, want 10", encoder.indexOffset)
	}

	pass.End()
}

func TestSetViewport(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	enc, _ := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
	pass := enc.BeginRenderPass(&hal.RenderPassDescriptor{
		ColorAttachments: []hal.RenderPassColorAttachment{},
	})

	encoder := pass.(*RenderPassEncoder)
	if encoder.hasViewport {
		t.Error("hasViewport should be false initially")
	}

	pass.SetViewport(10, 20, 800, 600, 0.0, 1.0)

	if !encoder.hasViewport {
		t.Error("hasViewport should be true after SetViewport")
	}
	if encoder.viewport[0] != 10 || encoder.viewport[1] != 20 ||
		encoder.viewport[2] != 800 || encoder.viewport[3] != 600 {
		t.Errorf("viewport = %v, want [10 20 800 600 0 1]", encoder.viewport)
	}

	pass.End()
}

// =============================================================================
// Typed Resource Tests
// =============================================================================

func TestCreateRenderPipelineReturnsTyped(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	rp, err := dev.CreateRenderPipeline(&hal.RenderPipelineDescriptor{Label: "typed-test"})
	if err != nil {
		t.Fatalf("CreateRenderPipeline failed: %v", err)
	}

	pipeline, ok := rp.(*RenderPipeline)
	if !ok {
		t.Fatal("expected *RenderPipeline type")
	}
	if pipeline.desc.Label != "typed-test" {
		t.Errorf("pipeline label = %q, want %q", pipeline.desc.Label, "typed-test")
	}
}

func TestCreateBindGroupReturnsTyped(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	bg, err := dev.CreateBindGroup(&hal.BindGroupDescriptor{Label: "typed-bg"})
	if err != nil {
		t.Fatalf("CreateBindGroup failed: %v", err)
	}

	bindGroup, ok := bg.(*BindGroup)
	if !ok {
		t.Fatal("expected *BindGroup type")
	}
	if bindGroup.desc.Label != "typed-bg" {
		t.Errorf("bind group label = %q, want %q", bindGroup.desc.Label, "typed-bg")
	}
}

func TestCreateShaderModuleReturnsTyped(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	sm, err := dev.CreateShaderModule(&hal.ShaderModuleDescriptor{Label: "typed-sm"})
	if err != nil {
		t.Fatalf("CreateShaderModule failed: %v", err)
	}

	shader, ok := sm.(*ShaderModule)
	if !ok {
		t.Fatal("expected *ShaderModule type")
	}
	if shader.desc.Label != "typed-sm" {
		t.Errorf("shader label = %q, want %q", shader.desc.Label, "typed-sm")
	}
}

// TestParticlesComputeAndRender is a full HAL-level integration test that
// reproduces the gogpu/examples/particles pipeline: compute shader updates
// particle positions, then render pipeline draws particles via instanced
// triangle-strip with instance-rate vertex buffer.
//
// This test covers two bugs that were fixed:
//  1. SubPointer: OpAccessChain on function-local struct variables created
//     disconnected copies — OpStore to p.pos did not update p (SPIR-V semantics
//     require write-through to the parent composite).
//  2. typeByteSize: struct size computed from naive i*4 fallback instead of
//     MemberDecorate Offset — caused wrong stride for RuntimeArray<struct>,
//     corrupting adjacent storage buffer elements.
func TestParticlesComputeAndRender(t *testing.T) {
	dev, q, cleanup := createSoftwareDevice(t)
	defer cleanup()

	const numParticles = 4
	const particleBytes = 16
	const bufSize = uint64(numParticles * particleBytes)

	// Simple compute shader: move particles by velocity.
	const computeWGSL = `
struct Particle { pos: vec2<f32>, vel: vec2<f32>, }
struct Params { dt: f32, count: u32, }

@group(0) @binding(0) var<storage, read> pin: array<Particle>;
@group(0) @binding(1) var<storage, read_write> pout: array<Particle>;
@group(0) @binding(2) var<uniform> params: Params;

@compute @workgroup_size(64)
fn main(@builtin(global_invocation_id) id: vec3<u32>) {
    let i = id.x;
    if (i >= params.count) { return; }
    var p = pin[i];
    p.pos += p.vel * params.dt;
    pout[i] = p;
}
`

	// Render shader: instanced triangle-strip quads from particle positions.
	const renderWGSL = `
struct Out {
    @builtin(position) pos: vec4<f32>,
    @location(0) col: vec3<f32>,
}
@vertex
fn vs_main(@builtin(vertex_index) vid: u32, @location(0) center: vec2<f32>, @location(1) vel: vec2<f32>) -> Out {
    let x = f32(vid & 1u) * 2.0 - 1.0;
    let y = f32((vid >> 1u) & 1u) * 2.0 - 1.0;
    let size = 0.1;
    var o: Out;
    o.pos = vec4<f32>(center.x + x * size, center.y + y * size, 0.0, 1.0);
    o.col = vec3<f32>(1.0, 0.3, 0.5);
    return o;
}
@fragment
fn fs_main(@location(0) col: vec3<f32>) -> @location(0) vec4<f32> {
    return vec4<f32>(col, 1.0);
}
`

	// --- Create buffers ---
	usage := gputypes.BufferUsageStorage | gputypes.BufferUsageVertex | gputypes.BufferUsageCopyDst
	bufA, err := dev.CreateBuffer(&hal.BufferDescriptor{Label: "A", Size: bufSize, Usage: usage})
	if err != nil {
		t.Fatalf("CreateBuffer A: %v", err)
	}
	bufB, err := dev.CreateBuffer(&hal.BufferDescriptor{Label: "B", Size: bufSize, Usage: usage})
	if err != nil {
		t.Fatalf("CreateBuffer B: %v", err)
	}
	uniformBuf, err := dev.CreateBuffer(&hal.BufferDescriptor{
		Label: "params", Size: 8,
		Usage: gputypes.BufferUsageUniform | gputypes.BufferUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateBuffer uniform: %v", err)
	}

	// Init particles at known positions with known velocities.
	type particle struct{ px, py, vx, vy float32 }
	particles := []particle{
		{0.5, 0.5, 0.1, 0.0},
		{-0.5, -0.5, 0.0, 0.1},
		{0.3, -0.3, -0.1, 0.0},
		{-0.3, 0.3, 0.0, -0.1},
	}
	initData := make([]byte, bufSize)
	for i, p := range particles {
		o := i * particleBytes
		binary.LittleEndian.PutUint32(initData[o:], math.Float32bits(p.px))
		binary.LittleEndian.PutUint32(initData[o+4:], math.Float32bits(p.py))
		binary.LittleEndian.PutUint32(initData[o+8:], math.Float32bits(p.vx))
		binary.LittleEndian.PutUint32(initData[o+12:], math.Float32bits(p.vy))
	}
	q.WriteBuffer(bufA, 0, initData)

	paramData := make([]byte, 8)
	binary.LittleEndian.PutUint32(paramData[0:], math.Float32bits(1.0)) // dt=1.0
	binary.LittleEndian.PutUint32(paramData[4:], numParticles)
	q.WriteBuffer(uniformBuf, 0, paramData)

	// --- Compute pipeline ---
	cs, err := dev.CreateShaderModule(&hal.ShaderModuleDescriptor{
		Label:  "compute",
		Source: hal.ShaderSource{WGSL: computeWGSL},
	})
	if err != nil {
		t.Fatalf("CreateShaderModule (compute): %v", err)
	}
	cp, err := dev.CreateComputePipeline(&hal.ComputePipelineDescriptor{
		Label:   "compute-pipe",
		Compute: hal.ComputeState{Module: cs, EntryPoint: "main"},
	})
	if err != nil {
		t.Fatalf("CreateComputePipeline: %v", err)
	}

	// Bind group: pin=A, pout=B, params=uniform
	bg, err := dev.CreateBindGroup(&hal.BindGroupDescriptor{
		Label: "compute-bg",
		Entries: []gputypes.BindGroupEntry{
			{Binding: 0, Resource: gputypes.BufferBinding{Buffer: bufA.NativeHandle(), Size: bufSize}},
			{Binding: 1, Resource: gputypes.BufferBinding{Buffer: bufB.NativeHandle(), Size: bufSize}},
			{Binding: 2, Resource: gputypes.BufferBinding{Buffer: uniformBuf.NativeHandle(), Size: 8}},
		},
	})
	if err != nil {
		t.Fatalf("CreateBindGroup: %v", err)
	}

	// Dispatch compute.
	enc, _ := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
	cpass := enc.BeginComputePass(nil)
	cpass.SetPipeline(cp)
	cpass.SetBindGroup(0, bg, nil)
	cpass.Dispatch(1, 1, 1)
	cpass.End()

	// Verify compute results: pos_new = pos_old + vel * dt.
	outBuf := bufB.(*Buffer)
	outData := outBuf.GetData()
	for i, p := range particles {
		o := i * particleBytes
		gotPx := math.Float32frombits(binary.LittleEndian.Uint32(outData[o:]))
		gotPy := math.Float32frombits(binary.LittleEndian.Uint32(outData[o+4:]))
		gotVx := math.Float32frombits(binary.LittleEndian.Uint32(outData[o+8:]))
		gotVy := math.Float32frombits(binary.LittleEndian.Uint32(outData[o+12:]))
		wantPx := p.px + p.vx
		wantPy := p.py + p.vy

		if math.Abs(float64(gotPx-wantPx)) > 0.001 ||
			math.Abs(float64(gotPy-wantPy)) > 0.001 {
			t.Errorf("particle %d position: got (%.4f,%.4f), want (%.4f,%.4f)",
				i, gotPx, gotPy, wantPx, wantPy)
		}
		if math.Abs(float64(gotVx-p.vx)) > 0.001 ||
			math.Abs(float64(gotVy-p.vy)) > 0.001 {
			t.Errorf("particle %d velocity: got (%.4f,%.4f), want (%.4f,%.4f)",
				i, gotVx, gotVy, p.vx, p.vy)
		}
	}

	// --- Render pipeline (instanced triangle-strip) ---
	const texW, texH = 100, 100
	tex, _ := dev.CreateTexture(&hal.TextureDescriptor{
		Label:  "target",
		Size:   hal.Extent3D{Width: texW, Height: texH, DepthOrArrayLayers: 1},
		Format: gputypes.TextureFormatRGBA8Unorm,
		Usage:  gputypes.TextureUsageRenderAttachment,
	})
	texView, _ := dev.CreateTextureView(tex, &hal.TextureViewDescriptor{})

	rs, err := dev.CreateShaderModule(&hal.ShaderModuleDescriptor{
		Label:  "render",
		Source: hal.ShaderSource{WGSL: renderWGSL},
	})
	if err != nil {
		t.Fatalf("CreateShaderModule (render): %v", err)
	}

	rp, err := dev.CreateRenderPipeline(&hal.RenderPipelineDescriptor{
		Label: "render-pipe",
		Vertex: hal.VertexState{
			Module: rs, EntryPoint: "vs_main",
			Buffers: []gputypes.VertexBufferLayout{{
				ArrayStride: particleBytes,
				StepMode:    gputypes.VertexStepModeInstance,
				Attributes: []gputypes.VertexAttribute{
					{Format: gputypes.VertexFormatFloat32x2, Offset: 0, ShaderLocation: 0},
					{Format: gputypes.VertexFormatFloat32x2, Offset: 8, ShaderLocation: 1},
				},
			}},
		},
		Primitive: gputypes.PrimitiveState{Topology: gputypes.PrimitiveTopologyTriangleStrip},
		Fragment: &hal.FragmentState{
			Module: rs, EntryPoint: "fs_main",
			Targets: []gputypes.ColorTargetState{{
				Format:    gputypes.TextureFormatRGBA8Unorm,
				WriteMask: gputypes.ColorWriteMaskAll,
			}},
		},
	})
	if err != nil {
		t.Fatalf("CreateRenderPipeline: %v", err)
	}

	enc2, _ := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
	rpEnc := enc2.BeginRenderPass(&hal.RenderPassDescriptor{
		ColorAttachments: []hal.RenderPassColorAttachment{{
			View: texView, LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore,
			ClearValue: gputypes.Color{R: 0, G: 0, B: 0, A: 1},
		}},
	})
	rpEnc.SetPipeline(rp)
	rpEnc.SetVertexBuffer(0, bufB, 0)
	rpEnc.Draw(4, numParticles, 0, 0) // 4 vertices per quad, numParticles instances
	rpEnc.End()

	// Verify render result: at least some pixels should be non-black.
	texData := tex.(*Texture).GetData()
	nonBlack := 0
	for i := 0; i < len(texData); i += 4 {
		if texData[i] > 0 || texData[i+1] > 0 || texData[i+2] > 0 {
			nonBlack++
		}
	}
	if nonBlack == 0 {
		t.Error("render produced black screen — no particles visible")
	}
}
