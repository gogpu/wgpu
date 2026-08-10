//go:build !(js && wasm)

package core

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/core/track"
)

// =============================================================================
// Texture TrackerIndex Allocation Tests
// =============================================================================

func TestTexture_TrackerIndex_Allocated(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	tex := NewTexture(
		mockTexture{}, device,
		gputypes.TextureFormatRGBA8Unorm, gputypes.TextureDimension2D,
		gputypes.TextureUsageRenderAttachment,
		gputypes.Extent3D{Width: 64, Height: 64, DepthOrArrayLayers: 1},
		1, 1, "TrackerTest",
	)

	td := tex.TrackingData()
	if td == nil {
		t.Fatal("TrackingData() should not be nil")
	}
	if !td.Index().IsValid() {
		t.Fatal("TrackerIndex should be valid (allocated from device allocator)")
	}

	// First allocated texture should get index 0.
	if td.Index() != 0 {
		t.Errorf("First texture TrackerIndex = %d, want 0", td.Index())
	}
}

func TestTexture_TrackerIndex_UniquePerTexture(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	tex1 := NewTexture(
		mockTexture{}, device,
		gputypes.TextureFormatRGBA8Unorm, gputypes.TextureDimension2D,
		gputypes.TextureUsageRenderAttachment,
		gputypes.Extent3D{Width: 64, Height: 64, DepthOrArrayLayers: 1},
		1, 1, "Tex1",
	)
	tex2 := NewTexture(
		mockTexture{}, device,
		gputypes.TextureFormatRGBA8Unorm, gputypes.TextureDimension2D,
		gputypes.TextureUsageRenderAttachment,
		gputypes.Extent3D{Width: 128, Height: 128, DepthOrArrayLayers: 1},
		1, 1, "Tex2",
	)

	idx1 := tex1.TrackingData().Index()
	idx2 := tex2.TrackingData().Index()

	if idx1 == idx2 {
		t.Errorf("Two textures should have different indices: both got %d", idx1)
	}
}

func TestTexture_TrackerIndex_Freed(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	tex := NewTexture(
		mockTexture{}, device,
		gputypes.TextureFormatRGBA8Unorm, gputypes.TextureDimension2D,
		gputypes.TextureUsageRenderAttachment,
		gputypes.Extent3D{Width: 64, Height: 64, DepthOrArrayLayers: 1},
		1, 1, "FreeTest",
	)

	td := tex.TrackingData()
	if td == nil || !td.Index().IsValid() {
		t.Fatal("TrackingData and index should be valid before destroy")
	}
	if td.IsReleased() {
		t.Fatal("TrackingData should not be released before destroy")
	}

	tex.Destroy()

	if !td.IsReleased() {
		t.Error("TrackingData should be released after destroy")
	}
}

func TestTexture_TrackerIndex_FreedAndReused(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	// Create and destroy a texture to free its index.
	tex1 := NewTexture(
		mockTexture{}, device,
		gputypes.TextureFormatRGBA8Unorm, gputypes.TextureDimension2D,
		gputypes.TextureUsageRenderAttachment,
		gputypes.Extent3D{Width: 64, Height: 64, DepthOrArrayLayers: 1},
		1, 1, "ReuseTex1",
	)
	idx1 := tex1.TrackingData().Index()
	tex1.Destroy()

	// Create a new texture — it should reuse the freed index.
	tex2 := NewTexture(
		mockTexture{}, device,
		gputypes.TextureFormatRGBA8Unorm, gputypes.TextureDimension2D,
		gputypes.TextureUsageRenderAttachment,
		gputypes.Extent3D{Width: 64, Height: 64, DepthOrArrayLayers: 1},
		1, 1, "ReuseTex2",
	)
	idx2 := tex2.TrackingData().Index()

	if idx2 != idx1 {
		t.Errorf("Freed index %d should be reused, got %d", idx1, idx2)
	}
}

// =============================================================================
// TextureView Parent Reference Tests
// =============================================================================

func TestTextureView_ParentReference(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	tex := NewTexture(
		mockTexture{}, device,
		gputypes.TextureFormatRGBA8Unorm, gputypes.TextureDimension2D,
		gputypes.TextureUsageRenderAttachment,
		gputypes.Extent3D{Width: 64, Height: 64, DepthOrArrayLayers: 1},
		1, 1, "ParentTest",
	)

	view := &TextureView{
		HAL:    mockTextureView{},
		Parent: tex,
	}

	if view.Parent == nil {
		t.Fatal("TextureView.Parent should reference the parent texture")
	}
	if view.Parent != tex {
		t.Error("TextureView.Parent should point to the creating texture")
	}

	// Should be able to get TrackerIndex through the parent chain.
	td := view.Parent.TrackingData()
	if td == nil || !td.Index().IsValid() {
		t.Error("TrackerIndex should be accessible through view.Parent.TrackingData()")
	}
}

func TestTextureView_NilParent(t *testing.T) {
	// Swapchain views may not have a parent texture.
	view := &TextureView{
		HAL:    mockTextureView{},
		Parent: nil,
	}

	if view.Parent != nil {
		t.Error("TextureView without parent should have nil Parent")
	}
}

// =============================================================================
// BeginRenderPass TextureScope Population Tests
// =============================================================================

func TestBeginRenderPass_PopulatesTextureScope_ColorAttachment(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	tex := NewTexture(
		mockTexture{}, device,
		gputypes.TextureFormatRGBA8Unorm, gputypes.TextureDimension2D,
		gputypes.TextureUsageRenderAttachment,
		gputypes.Extent3D{Width: 256, Height: 256, DepthOrArrayLayers: 1},
		1, 1, "ColorTarget",
	)

	encoder, err := device.CreateCommandEncoder("ScopeTest")
	if err != nil {
		t.Fatalf("CreateCommandEncoder failed: %v", err)
	}

	desc := &RenderPassDescriptor{
		Label: "ColorPass",
		ColorAttachments: []RenderPassColorAttachment{
			{
				View:       &TextureView{HAL: mockTextureView{}, Parent: tex},
				LoadOp:     gputypes.LoadOpClear,
				StoreOp:    gputypes.StoreOpStore,
				ClearValue: gputypes.Color{R: 0, G: 0, B: 0, A: 1},
			},
		},
	}

	pass, err := encoder.BeginRenderPass(desc)
	if err != nil {
		t.Fatalf("BeginRenderPass failed: %v", err)
	}

	// End the pass and finish the encoder to get the command buffer.
	if err := pass.End(); err != nil {
		t.Fatalf("End failed: %v", err)
	}
	cmdBuf, err := encoder.Finish()
	if err != nil {
		t.Fatalf("Finish failed: %v", err)
	}

	// Verify the texture scope has the color attachment tracked.
	scope := cmdBuf.TextureScope()
	if scope == nil {
		t.Fatal("TextureScope should not be nil")
	}

	trackerIdx := tex.TrackingData().Index()
	if !scope.IsUsed(trackerIdx) {
		t.Fatal("Texture should be tracked in scope after BeginRenderPass")
	}

	usage := scope.GetUsage(trackerIdx)
	if !usage.Contains(track.TextureUsesColorTarget) {
		t.Errorf("Usage = %d, want COLOR_TARGET (%d) set", usage, track.TextureUsesColorTarget)
	}
}

func TestBeginRenderPass_PopulatesTextureScope_DepthStencil(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	depthTex := NewTexture(
		mockTexture{}, device,
		gputypes.TextureFormatDepth24Plus, gputypes.TextureDimension2D,
		gputypes.TextureUsageRenderAttachment,
		gputypes.Extent3D{Width: 256, Height: 256, DepthOrArrayLayers: 1},
		1, 1, "DepthTarget",
	)

	encoder, err := device.CreateCommandEncoder("DepthScopeTest")
	if err != nil {
		t.Fatalf("CreateCommandEncoder failed: %v", err)
	}

	desc := &RenderPassDescriptor{
		Label: "DepthPass",
		DepthStencilAttachment: &RenderPassDepthStencilAttachment{
			View:            &TextureView{HAL: mockTextureView{}, Parent: depthTex},
			DepthLoadOp:     gputypes.LoadOpClear,
			DepthStoreOp:    gputypes.StoreOpStore,
			DepthClearValue: 1.0,
			DepthReadOnly:   false,
			StencilReadOnly: false,
		},
	}

	pass, err := encoder.BeginRenderPass(desc)
	if err != nil {
		t.Fatalf("BeginRenderPass failed: %v", err)
	}
	if err := pass.End(); err != nil {
		t.Fatalf("End failed: %v", err)
	}
	cmdBuf, err := encoder.Finish()
	if err != nil {
		t.Fatalf("Finish failed: %v", err)
	}

	scope := cmdBuf.TextureScope()
	if scope == nil {
		t.Fatal("TextureScope should not be nil")
	}

	trackerIdx := depthTex.TrackingData().Index()
	if !scope.IsUsed(trackerIdx) {
		t.Fatal("Depth texture should be tracked in scope")
	}

	usage := scope.GetUsage(trackerIdx)
	if !usage.Contains(track.TextureUsesDepthStencilWrite) {
		t.Errorf("Usage = %d, want DEPTH_STENCIL_WRITE (%d) set", usage, track.TextureUsesDepthStencilWrite)
	}
}

func TestBeginRenderPass_PopulatesTextureScope_DepthReadOnly(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	depthTex := NewTexture(
		mockTexture{}, device,
		gputypes.TextureFormatDepth24Plus, gputypes.TextureDimension2D,
		gputypes.TextureUsageRenderAttachment|gputypes.TextureUsageTextureBinding,
		gputypes.Extent3D{Width: 256, Height: 256, DepthOrArrayLayers: 1},
		1, 1, "ReadOnlyDepth",
	)

	encoder, err := device.CreateCommandEncoder("ReadOnlyDepthTest")
	if err != nil {
		t.Fatalf("CreateCommandEncoder failed: %v", err)
	}

	desc := &RenderPassDescriptor{
		Label: "ReadOnlyDepthPass",
		DepthStencilAttachment: &RenderPassDepthStencilAttachment{
			View:            &TextureView{HAL: mockTextureView{}, Parent: depthTex},
			DepthLoadOp:     gputypes.LoadOpLoad,
			DepthStoreOp:    gputypes.StoreOpStore,
			DepthReadOnly:   true,
			StencilReadOnly: true,
		},
	}

	pass, err := encoder.BeginRenderPass(desc)
	if err != nil {
		t.Fatalf("BeginRenderPass failed: %v", err)
	}
	if err := pass.End(); err != nil {
		t.Fatalf("End failed: %v", err)
	}
	cmdBuf, err := encoder.Finish()
	if err != nil {
		t.Fatalf("Finish failed: %v", err)
	}

	scope := cmdBuf.TextureScope()
	trackerIdx := depthTex.TrackingData().Index()
	usage := scope.GetUsage(trackerIdx)

	// When both depth and stencil are read-only, usage should be DEPTH_STENCIL_READ.
	if !usage.Contains(track.TextureUsesDepthStencilRead) {
		t.Errorf("Usage = %d, want DEPTH_STENCIL_READ (%d) set", usage, track.TextureUsesDepthStencilRead)
	}
	if usage.Contains(track.TextureUsesDepthStencilWrite) {
		t.Error("Read-only depth should not have DEPTH_STENCIL_WRITE")
	}
}

func TestBeginRenderPass_PopulatesTextureScope_MultipleAttachments(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	colorTex1 := NewTexture(
		mockTexture{}, device,
		gputypes.TextureFormatRGBA8Unorm, gputypes.TextureDimension2D,
		gputypes.TextureUsageRenderAttachment,
		gputypes.Extent3D{Width: 256, Height: 256, DepthOrArrayLayers: 1},
		1, 1, "Color1",
	)
	colorTex2 := NewTexture(
		mockTexture{}, device,
		gputypes.TextureFormatRGBA8Unorm, gputypes.TextureDimension2D,
		gputypes.TextureUsageRenderAttachment,
		gputypes.Extent3D{Width: 256, Height: 256, DepthOrArrayLayers: 1},
		1, 1, "Color2",
	)
	depthTex := NewTexture(
		mockTexture{}, device,
		gputypes.TextureFormatDepth24Plus, gputypes.TextureDimension2D,
		gputypes.TextureUsageRenderAttachment,
		gputypes.Extent3D{Width: 256, Height: 256, DepthOrArrayLayers: 1},
		1, 1, "Depth",
	)

	encoder, err := device.CreateCommandEncoder("MultiTest")
	if err != nil {
		t.Fatalf("CreateCommandEncoder failed: %v", err)
	}

	desc := &RenderPassDescriptor{
		Label: "MultiPass",
		ColorAttachments: []RenderPassColorAttachment{
			{
				View:    &TextureView{HAL: mockTextureView{}, Parent: colorTex1},
				LoadOp:  gputypes.LoadOpClear,
				StoreOp: gputypes.StoreOpStore,
			},
			{
				View:    &TextureView{HAL: mockTextureView{}, Parent: colorTex2},
				LoadOp:  gputypes.LoadOpClear,
				StoreOp: gputypes.StoreOpStore,
			},
		},
		DepthStencilAttachment: &RenderPassDepthStencilAttachment{
			View:         &TextureView{HAL: mockTextureView{}, Parent: depthTex},
			DepthLoadOp:  gputypes.LoadOpClear,
			DepthStoreOp: gputypes.StoreOpStore,
		},
	}

	pass, err := encoder.BeginRenderPass(desc)
	if err != nil {
		t.Fatalf("BeginRenderPass failed: %v", err)
	}
	if err := pass.End(); err != nil {
		t.Fatalf("End failed: %v", err)
	}
	cmdBuf, err := encoder.Finish()
	if err != nil {
		t.Fatalf("Finish failed: %v", err)
	}

	scope := cmdBuf.TextureScope()
	if scope == nil {
		t.Fatal("TextureScope should not be nil")
	}

	// All three textures should be tracked.
	for _, tc := range []struct {
		name    string
		tex     *Texture
		usage   track.TextureUses
		useName string
	}{
		{"Color1", colorTex1, track.TextureUsesColorTarget, "COLOR_TARGET"},
		{"Color2", colorTex2, track.TextureUsesColorTarget, "COLOR_TARGET"},
		{"Depth", depthTex, track.TextureUsesDepthStencilWrite, "DEPTH_STENCIL_WRITE"},
	} {
		idx := tc.tex.TrackingData().Index()
		if !scope.IsUsed(idx) {
			t.Errorf("%s: texture not tracked in scope", tc.name)
			continue
		}
		usage := scope.GetUsage(idx)
		if !usage.Contains(tc.usage) {
			t.Errorf("%s: usage = %d, want %s (%d) set", tc.name, usage, tc.useName, tc.usage)
		}
	}
}

func TestBeginRenderPass_PopulatesTextureScope_ResolveTarget(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	msaaTex := NewTexture(
		mockTexture{}, device,
		gputypes.TextureFormatRGBA8Unorm, gputypes.TextureDimension2D,
		gputypes.TextureUsageRenderAttachment,
		gputypes.Extent3D{Width: 256, Height: 256, DepthOrArrayLayers: 1},
		1, 4, "MSAATex",
	)
	resolveTex := NewTexture(
		mockTexture{}, device,
		gputypes.TextureFormatRGBA8Unorm, gputypes.TextureDimension2D,
		gputypes.TextureUsageRenderAttachment,
		gputypes.Extent3D{Width: 256, Height: 256, DepthOrArrayLayers: 1},
		1, 1, "ResolveTex",
	)

	encoder, err := device.CreateCommandEncoder("ResolveTest")
	if err != nil {
		t.Fatalf("CreateCommandEncoder failed: %v", err)
	}

	desc := &RenderPassDescriptor{
		Label: "MSAAPass",
		ColorAttachments: []RenderPassColorAttachment{
			{
				View:          &TextureView{HAL: mockTextureView{}, Parent: msaaTex},
				ResolveTarget: &TextureView{HAL: mockTextureView{}, Parent: resolveTex},
				LoadOp:        gputypes.LoadOpClear,
				StoreOp:       gputypes.StoreOpStore,
			},
		},
	}

	pass, err := encoder.BeginRenderPass(desc)
	if err != nil {
		t.Fatalf("BeginRenderPass failed: %v", err)
	}
	if err := pass.End(); err != nil {
		t.Fatalf("End failed: %v", err)
	}
	cmdBuf, err := encoder.Finish()
	if err != nil {
		t.Fatalf("Finish failed: %v", err)
	}

	scope := cmdBuf.TextureScope()

	// Both MSAA and resolve textures should be tracked as COLOR_TARGET.
	msaaIdx := msaaTex.TrackingData().Index()
	resolveIdx := resolveTex.TrackingData().Index()

	if !scope.IsUsed(msaaIdx) {
		t.Error("MSAA texture should be tracked in scope")
	}
	if !scope.IsUsed(resolveIdx) {
		t.Error("Resolve target should be tracked in scope")
	}

	msaaUsage := scope.GetUsage(msaaIdx)
	if !msaaUsage.Contains(track.TextureUsesColorTarget) {
		t.Errorf("MSAA usage = %d, want COLOR_TARGET set", msaaUsage)
	}

	resolveUsage := scope.GetUsage(resolveIdx)
	if !resolveUsage.Contains(track.TextureUsesColorTarget) {
		t.Errorf("Resolve usage = %d, want COLOR_TARGET set", resolveUsage)
	}
}

func TestBeginRenderPass_NilViewParent_Skipped(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	encoder, err := device.CreateCommandEncoder("NilParentTest")
	if err != nil {
		t.Fatalf("CreateCommandEncoder failed: %v", err)
	}

	// View without Parent (e.g., swapchain view) should not crash.
	desc := &RenderPassDescriptor{
		Label: "SwapchainPass",
		ColorAttachments: []RenderPassColorAttachment{
			{
				View:    &TextureView{HAL: mockTextureView{}, Parent: nil},
				LoadOp:  gputypes.LoadOpClear,
				StoreOp: gputypes.StoreOpStore,
			},
		},
	}

	pass, err := encoder.BeginRenderPass(desc)
	if err != nil {
		t.Fatalf("BeginRenderPass should succeed with nil parent: %v", err)
	}
	if err := pass.End(); err != nil {
		t.Fatalf("End failed: %v", err)
	}
	cmdBuf, err := encoder.Finish()
	if err != nil {
		t.Fatalf("Finish failed: %v", err)
	}

	// Scope should exist but have no tracked textures.
	scope := cmdBuf.TextureScope()
	if scope == nil {
		t.Fatal("TextureScope should not be nil")
	}
}

func TestBeginRenderPass_NilView_Skipped(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	encoder, err := device.CreateCommandEncoder("NilViewTest")
	if err != nil {
		t.Fatalf("CreateCommandEncoder failed: %v", err)
	}

	// Nil view should not crash.
	desc := &RenderPassDescriptor{
		Label: "NilViewPass",
		ColorAttachments: []RenderPassColorAttachment{
			{
				View:    nil,
				LoadOp:  gputypes.LoadOpClear,
				StoreOp: gputypes.StoreOpStore,
			},
		},
	}

	pass, err := encoder.BeginRenderPass(desc)
	if err != nil {
		t.Fatalf("BeginRenderPass should succeed with nil view: %v", err)
	}
	if err := pass.End(); err != nil {
		t.Fatalf("End failed: %v", err)
	}
	_, err = encoder.Finish()
	if err != nil {
		t.Fatalf("Finish failed: %v", err)
	}
}
