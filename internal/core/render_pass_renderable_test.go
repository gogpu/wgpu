//go:build !(js && wasm)

package core

import (
	"errors"
	"strings"
	"testing"

	"github.com/gogpu/gputypes"
)

// rpTexture is a 2D render target texture for the renderable-view tests.
func rpTexture(format gputypes.TextureFormat, usage gputypes.TextureUsage, size gputypes.Extent3D, mips, samples uint32) *Texture {
	return NewTexture(&mockTexture{}, nil, format, gputypes.TextureDimension2D, usage, size, mips, samples, "renderable")
}

// rpView is a view with a known descriptor: dimension, one mip, one layer.
func rpView(tex *Texture, dim gputypes.TextureViewDimension) *TextureView {
	return &TextureView{HAL: &mockTextureView{}, Parent: tex, Dimension: dim, MipLevelCount: 1, ArrayLayerCount: 1}
}

func rpColorPass(view, resolve *TextureView, depth *TextureView) *RenderPassDescriptor {
	desc := &RenderPassDescriptor{Label: "renderable"}
	if view != nil {
		desc.ColorAttachments = []RenderPassColorAttachment{{View: view, ResolveTarget: resolve,
			LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore}}
		if view.Dimension == gputypes.TextureViewDimension3D {
			desc.ColorAttachments[0].DepthSlice = depthSlicePtr(0)
		}
	}
	if depth != nil {
		desc.DepthStencilAttachment = &RenderPassDepthStencilAttachment{View: depth,
			DepthLoadOp: gputypes.LoadOpClear, DepthStoreOp: gputypes.StoreOpStore}
	}
	return desc
}

type renderableCase struct {
	name    string
	desc    *RenderPassDescriptor
	kind    RenderPassValidationErrorKind
	reason  TextureViewNotRenderableReason
	index   int
	resolve bool
	texture bool // MismatchedResolveFormat on texture formats
	msg     string
	valid   bool
}

// TestValidateRenderPassDescriptor_RenderableView covers the WebGPU
// "renderable texture view" rule for color, resolve and depth/stencil
// attachments (RENDER_ATTACHMENT usage; 2d, 2d-array or 3d, a resolve target
// not 3d; one mip; one array layer; every aspect), with the messages of Rust
// wgpu-core TextureViewIsNotRenderable.
func TestValidateRenderPassDescriptor_RenderableView(t *testing.T) {
	const ra = gputypes.TextureUsageRenderAttachment
	flatSize := gputypes.Extent3D{Width: 16, Height: 16, DepthOrArrayLayers: 1}
	layered := gputypes.Extent3D{Width: 16, Height: 16, DepthOrArrayLayers: 6}
	color := rpTexture(gputypes.TextureFormatRGBA8Unorm, ra, flatSize, 2, 1)
	sampledOnly := rpTexture(gputypes.TextureFormatRGBA8Unorm, gputypes.TextureUsageTextureBinding, flatSize, 1, 1)
	array := rpTexture(gputypes.TextureFormatRGBA8Unorm, ra, layered, 1, 1)
	volume := depthSliceTexture3D()
	depthStencil := rpTexture(gputypes.TextureFormatDepth24PlusStencil8, ra, flatSize, 1, 1)
	depthOnly := rpTexture(gputypes.TextureFormatDepth32Float, ra, flatSize, 1, 1)
	copyDstDepthStencil := rpTexture(gputypes.TextureFormatDepth24PlusStencil8, gputypes.TextureUsageCopyDst, flatSize, 1, 1)

	with := func(v *TextureView, f func(*TextureView)) *TextureView { f(v); return v }
	twoMips := with(rpView(color, gputypes.TextureViewDimension2D), func(v *TextureView) { v.MipLevelCount = 2 })
	threeLayers := with(rpView(array, gputypes.TextureViewDimension2DArray), func(v *TextureView) { v.ArrayLayerCount = 3 })
	depthAspect := func(tex *Texture) *TextureView {
		return with(rpView(tex, gputypes.TextureViewDimension2D), func(v *TextureView) { v.Aspect = gputypes.TextureAspectDepthOnly })
	}
	unknown := &TextureView{HAL: &mockTextureView{}, Parent: sampledOnly, MipLevelCount: 2}

	tests := []renderableCase{
		{name: "2d view", desc: rpColorPass(rpView(color, gputypes.TextureViewDimension2D), nil, nil), valid: true},
		{name: "one layer of a 2d-array view", desc: rpColorPass(rpView(array, gputypes.TextureViewDimension2DArray), nil, nil), valid: true},
		{name: "3d view", desc: rpColorPass(rpView(volume, gputypes.TextureViewDimension3D), nil, nil), valid: true},
		{name: "view without a known descriptor is not checked", desc: rpColorPass(unknown, nil, nil), valid: true},
		{name: "texture without RENDER_ATTACHMENT", desc: rpColorPass(rpView(sampledOnly, gputypes.TextureViewDimension2D), nil, nil),
			kind: RenderPassErrorTextureViewNotRenderable, reason: TextureViewNotRenderableUsage,
			msg: "color attachment at index 0's texture view is not renderable: the texture this view references doesn't include the RENDER_ATTACHMENT usage"},
		{name: "internal RENDER_ATTACHMENT usage of the frontend's own view", desc: rpColorPass(nil, nil,
			with(rpView(copyDstDepthStencil, gputypes.TextureViewDimension2D), func(v *TextureView) { v.InternalUsage = ra })), valid: true},
		{name: "depth-stencil texture without RENDER_ATTACHMENT", desc: rpColorPass(nil, nil, rpView(copyDstDepthStencil, gputypes.TextureViewDimension2D)),
			kind: RenderPassErrorTextureViewNotRenderable, reason: TextureViewNotRenderableUsage, index: -1},
		{name: "cube view", desc: rpColorPass(rpView(array, gputypes.TextureViewDimensionCube), nil, nil),
			kind: RenderPassErrorTextureViewNotRenderable, reason: TextureViewNotRenderableDimension,
			msg: "the dimension of this texture view is not 2D, 2D array or 3D"},
		{name: "1d view", desc: rpColorPass(rpView(color, gputypes.TextureViewDimension1D), nil, nil),
			kind: RenderPassErrorTextureViewNotRenderable, reason: TextureViewNotRenderableDimension},
		{name: "two mip levels", desc: rpColorPass(twoMips, nil, nil),
			kind: RenderPassErrorTextureViewNotRenderable, reason: TextureViewNotRenderableMipLevelCount,
			msg: "this texture view has more than one mipmap level; view mipmap levels: 2"},
		{name: "three array layers", desc: rpColorPass(threeLayers, nil, nil),
			kind: RenderPassErrorTextureViewNotRenderable, reason: TextureViewNotRenderableArrayLayerCount,
			msg: "this texture view has more than one array layer; view array layers: 3"},
		{name: "depth aspect of a depth-stencil texture", desc: rpColorPass(nil, nil, depthAspect(depthStencil)),
			kind: RenderPassErrorTextureViewNotRenderable, reason: TextureViewNotRenderableAspects, index: -1,
			msg: "the depth attachment's texture view is not renderable: the aspects of this texture view are a subset"},
		{name: "depth aspect of a depth-only texture", desc: rpColorPass(nil, nil, depthAspect(depthOnly)), valid: true},
		{name: "depth attachment with two mips", desc: rpColorPass(nil, nil,
			with(rpView(depthOnly, gputypes.TextureViewDimension2D), func(v *TextureView) { v.MipLevelCount = 2 })),
			kind: RenderPassErrorTextureViewNotRenderable, reason: TextureViewNotRenderableMipLevelCount, index: -1},
	}
	runRenderableCases(t, tests)
}

// TestValidateRenderPassDescriptor_AttachmentFormats covers the format rules
// of the WebGPU spec: a color attachment's view has a color renderable format
// (the "Plain color formats" table), the depth/stencil attachment's view a
// depth-or-stencil format.
func TestValidateRenderPassDescriptor_AttachmentFormats(t *testing.T) {
	const ra = gputypes.TextureUsageRenderAttachment
	size := gputypes.Extent3D{Width: 8, Height: 8, DepthOrArrayLayers: 1}
	view := func(format gputypes.TextureFormat) *TextureView {
		return rpView(rpTexture(format, ra, size, 1, 1), gputypes.TextureViewDimension2D)
	}
	srgbView := withFormat(view(gputypes.TextureFormatRGBA8Unorm), gputypes.TextureFormatRGBA8UnormSrgb)

	tests := []renderableCase{
		{name: "rgba8unorm", desc: rpColorPass(view(gputypes.TextureFormatRGBA8Unorm), nil, nil), valid: true},
		{name: "rgba32float", desc: rpColorPass(view(gputypes.TextureFormatRGBA32Float), nil, nil), valid: true},
		{name: "srgb view of rgba8unorm", desc: rpColorPass(srgbView, nil, nil), valid: true},
		{name: "depth32float as color", desc: rpColorPass(view(gputypes.TextureFormatDepth32Float), nil, nil),
			kind: RenderPassErrorInvalidColorAttachmentFormat,
			msg:  "the format of the color attachment at index 0's texture view (Depth32Float) is not a color renderable format"},
		{name: "rgba8snorm without texture-formats-tier1", desc: rpColorPass(view(gputypes.TextureFormatRGBA8Snorm), nil, nil),
			kind: RenderPassErrorInvalidColorAttachmentFormat},
		{name: "rgb9e5ufloat", desc: rpColorPass(view(gputypes.TextureFormatRGB9E5Ufloat), nil, nil),
			kind: RenderPassErrorInvalidColorAttachmentFormat},
		{name: "rgba8unorm as depth", desc: rpColorPass(nil, nil, view(gputypes.TextureFormatRGBA8Unorm)),
			kind: RenderPassErrorInvalidDepthStencilAttachmentFormat, index: -1,
			msg: "the format of the depth-stencil attachment (RGBA8Unorm) is not a depth-or-stencil format"},
		{name: "depth24plus-stencil8", desc: rpColorPass(nil, nil, view(gputypes.TextureFormatDepth24PlusStencil8)), valid: true},
	}
	runRenderableCases(t, tests)
}

func withFormat(v *TextureView, format gputypes.TextureFormat) *TextureView {
	v.Format = format
	return v
}

// TestValidateRenderPassDescriptor_ResolveTarget covers the resolveTarget
// steps of GPURenderPassColorAttachment Valid Usage, as Rust wgpu-core
// command/render.rs checks them: a multisampled source, a single-sampled,
// non-3d renderable target of the same render extent, equal view and texture
// formats, and a format that supports resolve.
func TestValidateRenderPassDescriptor_ResolveTarget(t *testing.T) {
	const ra = gputypes.TextureUsageRenderAttachment
	size := gputypes.Extent3D{Width: 16, Height: 16, DepthOrArrayLayers: 1}
	tex := func(format gputypes.TextureFormat, samples uint32) *Texture {
		return rpTexture(format, ra, size, 2, samples)
	}
	view := func(tex *Texture) *TextureView { return rpView(tex, gputypes.TextureViewDimension2D) }
	msaa := view(tex(gputypes.TextureFormatRGBA8Unorm, 4))
	single := view(tex(gputypes.TextureFormatRGBA8Unorm, 1))
	mip1 := view(tex(gputypes.TextureFormatRGBA8Unorm, 1))
	mip1.BaseMipLevel = 1
	twoMips := view(tex(gputypes.TextureFormatRGBA8Unorm, 1))
	twoMips.MipLevelCount = 2
	volume := rpView(depthSliceTexture3D(), gputypes.TextureViewDimension3D)
	srgbOfUnorm := withFormat(view(tex(gputypes.TextureFormatRGBA8Unorm, 1)), gputypes.TextureFormatRGBA8UnormSrgb)
	msaaSrgbOfUnorm := withFormat(view(tex(gputypes.TextureFormatRGBA8Unorm, 4)), gputypes.TextureFormatRGBA8UnormSrgb)
	srgb := view(tex(gputypes.TextureFormatRGBA8UnormSrgb, 1))

	tests := []renderableCase{
		{name: "4 samples into 1", desc: rpColorPass(msaa, single, nil), valid: true},
		{name: "single-sampled source", desc: rpColorPass(single, view(tex(gputypes.TextureFormatRGBA8Unorm, 1)), nil),
			kind: RenderPassErrorInvalidResolveSampleCounts, resolve: true,
			msg: "the resolve source, color attachment at index 0's resolve texture view, must be multi-sampled (has 1 samples) while the resolve destination must not be multisampled (has 1 samples)"},
		{name: "multisampled target", desc: rpColorPass(msaa, view(tex(gputypes.TextureFormatRGBA8Unorm, 4)), nil),
			kind: RenderPassErrorInvalidResolveSampleCounts, resolve: true, msg: "(has 4 samples)"},
		{name: "3d target", desc: rpColorPass(msaa, volume, nil),
			kind: RenderPassErrorTextureViewNotRenderable, reason: TextureViewNotRenderableDimension, resolve: true,
			msg: "the color attachment at index 0's resolve texture view is not renderable: the dimension of this resolve target view is not 2D or 2D array"},
		{name: "target with two mips", desc: rpColorPass(msaa, twoMips, nil),
			kind: RenderPassErrorTextureViewNotRenderable, reason: TextureViewNotRenderableMipLevelCount, resolve: true},
		{name: "target at another render extent", desc: rpColorPass(msaa, mip1, nil),
			kind: RenderPassErrorDimensionMismatch, resolve: true},
		{name: "view formats differ", desc: rpColorPass(msaa, srgbOfUnorm, nil),
			kind: RenderPassErrorMismatchedResolveFormat, resolve: true,
			msg: "view format (RGBA8Unorm) must match the resolve destination view format (RGBA8UnormSrgb)"},
		{name: "texture formats differ, view formats equal", desc: rpColorPass(msaaSrgbOfUnorm, srgb, nil),
			kind: RenderPassErrorMismatchedResolveFormat, resolve: true, texture: true,
			msg: "texture format (RGBA8Unorm) must match the resolve destination texture format (RGBA8UnormSrgb)"},
		{name: "r32float does not resolve", desc: rpColorPass(view(tex(gputypes.TextureFormatR32Float, 4)),
			view(tex(gputypes.TextureFormatR32Float, 1)), nil),
			kind: RenderPassErrorUnsupportedResolveFormat, resolve: true,
			msg: "the format of the color attachment at index 0's resolve texture view (R32Float) is not resolvable"},
		{name: "rgba8uint does not resolve", desc: rpColorPass(view(tex(gputypes.TextureFormatRGBA8Uint, 4)),
			view(tex(gputypes.TextureFormatRGBA8Uint, 1)), nil),
			kind: RenderPassErrorUnsupportedResolveFormat, resolve: true},
		{name: "rgba16float resolves", desc: rpColorPass(view(tex(gputypes.TextureFormatRGBA16Float, 4)),
			view(tex(gputypes.TextureFormatRGBA16Float, 1)), nil), valid: true},
	}
	runRenderableCases(t, tests)
}

func runRenderableCases(t *testing.T, tests []renderableCase) {
	t.Helper()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ValidateRenderPassDescriptor(tt.desc, gputypes.DefaultLimits())
			checkRenderableErr(t, err, tt)
		})
	}
}

func checkRenderableErr(t *testing.T, err error, tt renderableCase) {
	t.Helper()
	if tt.valid {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return
	}
	var rpve *RenderPassValidationError
	if !errors.As(err, &rpve) {
		t.Fatalf("error = %v, want *RenderPassValidationError", err)
	}
	if rpve.Kind != tt.kind || rpve.Reason != tt.reason || rpve.AttachmentIndex != tt.index ||
		rpve.ResolveTarget != tt.resolve || rpve.TextureFormats != tt.texture {
		t.Fatalf("got kind %v reason %v index %d resolve %v texture %v; want %v %v %d %v %v (%v)",
			rpve.Kind, rpve.Reason, rpve.AttachmentIndex, rpve.ResolveTarget, rpve.TextureFormats,
			tt.kind, tt.reason, tt.index, tt.resolve, tt.texture, err)
	}
	if !strings.Contains(err.Error(), tt.msg) {
		t.Fatalf("error text %q does not contain %q", err.Error(), tt.msg)
	}
}
