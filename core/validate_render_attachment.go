//go:build !(js && wasm)

package core

import "github.com/gogpu/gputypes"

// TextureViewNotRenderableReason is the requirement of a "renderable texture
// view" that a render pass attachment's view does not meet.
//
// Reference: WebGPU spec "renderable texture view"; Rust wgpu-core
// resource.rs TextureViewNotRenderableReason.
type TextureViewNotRenderableReason int

const (
	// TextureViewNotRenderableUsage: the texture lacks RENDER_ATTACHMENT.
	TextureViewNotRenderableUsage TextureViewNotRenderableReason = iota + 1
	// TextureViewNotRenderableDimension: the view is not 2d, 2d-array or 3d
	// (a resolve target may not be 3d either).
	TextureViewNotRenderableDimension
	// TextureViewNotRenderableMipLevelCount: the view has more than one mip.
	TextureViewNotRenderableMipLevelCount
	// TextureViewNotRenderableArrayLayerCount: the view has more than one
	// array layer.
	TextureViewNotRenderableArrayLayerCount
	// TextureViewNotRenderableAspects: the view does not cover every aspect
	// of its texture.
	TextureViewNotRenderableAspects
)

// renderableViewError checks that view is a renderable texture view. A view
// whose descriptor is not known (Dimension undefined: a view wrapped from a
// HAL handle) is not checked, and a mip or layer count of 0 means not known.
// A resolve target must also not be 3d. It returns the error for the
// attachment at index (-1 for the depth/stencil attachment), or nil.
//
// The spec allows only one array layer; Rust wgpu-core allows more with
// Features::MULTIVIEW, which gputypes does not have.
//
// Reference: WebGPU spec "renderable texture view"; Rust wgpu-core
// resource.rs create_texture_view (render_extent).
func renderableViewError(label string, index int, resolve bool, view *TextureView) error {
	if view.Dimension == gputypes.TextureViewDimensionUndefined {
		return nil
	}
	tex := view.Parent
	err := &RenderPassValidationError{
		Kind: RenderPassErrorTextureViewNotRenderable, Label: label,
		AttachmentIndex: index, ResolveTarget: resolve,
	}
	switch {
	case !tex.Usage().Contains(gputypes.TextureUsageRenderAttachment):
		err.Reason, err.Usage = TextureViewNotRenderableUsage, tex.Usage()
	case !renderableViewDimension(view.Dimension, resolve):
		err.Reason, err.ViewDimension = TextureViewNotRenderableDimension, view.Dimension
	case view.MipLevelCount > 1:
		err.Reason, err.Given = TextureViewNotRenderableMipLevelCount, view.MipLevelCount
	case view.ArrayLayerCount > 1:
		err.Reason, err.Given = TextureViewNotRenderableArrayLayerCount, view.ArrayLayerCount
	case !coversAllAspects(tex.Format(), view.Aspect):
		err.Reason, err.Aspect = TextureViewNotRenderableAspects, view.Aspect
	default:
		return nil
	}
	return err
}

func renderableViewDimension(dim gputypes.TextureViewDimension, resolve bool) bool {
	switch dim {
	case gputypes.TextureViewDimension2D, gputypes.TextureViewDimension2DArray:
		return true
	case gputypes.TextureViewDimension3D:
		return !resolve
	default:
		return false
	}
}

// coversAllAspects reports whether a view with aspect refers to every aspect
// of a texture of format: "all", or the one aspect a single-aspect format has.
func coversAllAspects(format gputypes.TextureFormat, aspect gputypes.TextureAspect) bool {
	switch aspect {
	case gputypes.TextureAspectUndefined, gputypes.TextureAspectAll:
		return true
	case gputypes.TextureAspectDepthOnly:
		return format.HasDepth() && !format.HasStencil()
	case gputypes.TextureAspectStencilOnly:
		return format.HasStencil() && !format.HasDepth()
	default:
		return false
	}
}

// attachmentViewFormat is the format of an attachment's view: its own, or
// its texture's when the view did not name one.
func attachmentViewFormat(view *TextureView) gputypes.TextureFormat {
	if view.Format != gputypes.TextureFormatUndefined {
		return view.Format
	}
	return view.Parent.Format()
}

// validateColorAttachmentView checks the view of color attachment i: a color
// renderable format, a renderable texture view, then its depthSlice.
//
// Reference: WebGPU spec "GPURenderPassColorAttachment Valid Usage"; Rust
// wgpu-core command/render.rs (TextureViewIsNotRenderable,
// ColorAttachmentError::InvalidFormat).
func validateColorAttachmentView(label string, i int, ca *RenderPassColorAttachment) error {
	if format := attachmentViewFormat(ca.View); format != gputypes.TextureFormatUndefined && !isColorRenderableFormat(format) {
		return &RenderPassValidationError{
			Kind: RenderPassErrorInvalidColorAttachmentFormat, Label: label,
			AttachmentIndex: i, Format: format,
		}
	}
	if err := renderableViewError(label, i, false, ca.View); err != nil {
		return err
	}
	return validateColorAttachmentDepthSlice(label, i, ca)
}

// validateResolveTarget checks the resolve target of color attachment i, if
// it has one: a multisampled view resolved into a single-sampled, non-3d
// renderable view of the same render extent, with equal view and texture
// formats that support resolve.
//
// Reference: WebGPU spec "GPURenderPassColorAttachment Valid Usage"
// (resolveTarget steps); Rust wgpu-core command/render.rs
// (InvalidResolveSampleCounts, MismatchedResolveTextureFormat,
// UnsupportedResolveTargetFormat).
func validateResolveTarget(label string, i int, ca *RenderPassColorAttachment) error {
	resolve := ca.ResolveTarget
	if resolve == nil || resolve.Parent == nil {
		return nil
	}
	view := ca.View
	if src, dst := view.Parent.SampleCount(), resolve.Parent.SampleCount(); src <= 1 || dst != 1 {
		return &RenderPassValidationError{
			Kind: RenderPassErrorInvalidResolveSampleCounts, Label: label,
			AttachmentIndex: i, ResolveTarget: true, ActualSamples: src, ResolveSamples: dst,
		}
	}
	if err := renderableViewError(label, i, true, resolve); err != nil {
		return err
	}
	if want, got := renderExtent(view), renderExtent(resolve); want.Width != got.Width || want.Height != got.Height {
		return &RenderPassValidationError{
			Kind: RenderPassErrorDimensionMismatch, Label: label,
			AttachmentIndex: i, ResolveTarget: true, ReferenceIndex: i,
			ExpectedWidth: want.Width, ExpectedHeight: want.Height,
			ActualWidth: got.Width, ActualHeight: got.Height,
		}
	}
	return resolveFormatError(label, i, view, resolve)
}

// resolveFormatError checks the formats of a resolve: equal view formats,
// equal texture formats, and a view format that supports resolve.
func resolveFormatError(label string, i int, view, resolve *TextureView) error {
	src, dst := attachmentViewFormat(view), attachmentViewFormat(resolve)
	texture := false
	if src == dst {
		src, dst, texture = view.Parent.Format(), resolve.Parent.Format(), true
	}
	if src != dst {
		return &RenderPassValidationError{
			Kind: RenderPassErrorMismatchedResolveFormat, Label: label,
			AttachmentIndex: i, ResolveTarget: true, Format: src, ResolveFormat: dst,
			TextureFormats: texture,
		}
	}
	if format := attachmentViewFormat(resolve); format != gputypes.TextureFormatUndefined && !isResolvableFormat(format) {
		return &RenderPassValidationError{
			Kind: RenderPassErrorUnsupportedResolveFormat, Label: label,
			AttachmentIndex: i, ResolveTarget: true, Format: format,
		}
	}
	return nil
}

// validateDepthStencilView checks the depth/stencil attachment's view: a
// depth-or-stencil format and a renderable texture view.
//
// Reference: WebGPU spec "GPURenderPassDepthStencilAttachment Valid Usage";
// Rust wgpu-core command/render.rs
// (AttachmentError::InvalidDepthStencilAttachmentFormat).
func validateDepthStencilView(label string, view *TextureView) error {
	if format := attachmentViewFormat(view); format != gputypes.TextureFormatUndefined && !format.IsDepthStencil() {
		return &RenderPassValidationError{
			Kind: RenderPassErrorInvalidDepthStencilAttachmentFormat, Label: label,
			AttachmentIndex: -1, Format: format,
		}
	}
	return renderableViewError(label, -1, false, view)
}

// isColorRenderableFormat reports whether format is a color renderable
// format: a plain color format with RENDER_ATTACHMENT in the WebGPU spec
// table "Plain color formats", for a core device. The 8-bit snorm formats
// need "texture-formats-tier1", which gputypes does not have; rg11b10ufloat
// needs "rg11b10ufloat-renderable", which CreateTexture already requires for
// a RENDER_ATTACHMENT texture, and a view cannot reinterpret another format
// as it.
func isColorRenderableFormat(format gputypes.TextureFormat) bool {
	switch format {
	case gputypes.TextureFormatR8Unorm, gputypes.TextureFormatR8Uint, gputypes.TextureFormatR8Sint,
		gputypes.TextureFormatRG8Unorm, gputypes.TextureFormatRG8Uint, gputypes.TextureFormatRG8Sint,
		gputypes.TextureFormatRGBA8Unorm, gputypes.TextureFormatRGBA8UnormSrgb,
		gputypes.TextureFormatRGBA8Uint, gputypes.TextureFormatRGBA8Sint,
		gputypes.TextureFormatBGRA8Unorm, gputypes.TextureFormatBGRA8UnormSrgb,
		gputypes.TextureFormatR16Unorm, gputypes.TextureFormatR16Snorm,
		gputypes.TextureFormatR16Uint, gputypes.TextureFormatR16Sint, gputypes.TextureFormatR16Float,
		gputypes.TextureFormatRG16Unorm, gputypes.TextureFormatRG16Snorm,
		gputypes.TextureFormatRG16Uint, gputypes.TextureFormatRG16Sint, gputypes.TextureFormatRG16Float,
		gputypes.TextureFormatRGBA16Unorm, gputypes.TextureFormatRGBA16Snorm,
		gputypes.TextureFormatRGBA16Uint, gputypes.TextureFormatRGBA16Sint, gputypes.TextureFormatRGBA16Float,
		gputypes.TextureFormatR32Uint, gputypes.TextureFormatR32Sint, gputypes.TextureFormatR32Float,
		gputypes.TextureFormatRG32Uint, gputypes.TextureFormatRG32Sint, gputypes.TextureFormatRG32Float,
		gputypes.TextureFormatRGBA32Uint, gputypes.TextureFormatRGBA32Sint, gputypes.TextureFormatRGBA32Float,
		gputypes.TextureFormatRGB10A2Uint, gputypes.TextureFormatRGB10A2Unorm,
		gputypes.TextureFormatRG11B10Ufloat:
		return true
	default:
		return false
	}
}

// isResolvableFormat reports whether format supports resolve: the "resolve"
// column of the WebGPU spec table "Plain color formats", for a core device
// (rgba16float resolves with "core-features-and-limits"; the snorm formats
// need "texture-formats-tier1", which gputypes does not have).
func isResolvableFormat(format gputypes.TextureFormat) bool {
	switch format {
	case gputypes.TextureFormatR8Unorm, gputypes.TextureFormatRG8Unorm,
		gputypes.TextureFormatRGBA8Unorm, gputypes.TextureFormatRGBA8UnormSrgb,
		gputypes.TextureFormatBGRA8Unorm, gputypes.TextureFormatBGRA8UnormSrgb,
		gputypes.TextureFormatR16Float, gputypes.TextureFormatRG16Float, gputypes.TextureFormatRGBA16Float,
		gputypes.TextureFormatRGB10A2Unorm, gputypes.TextureFormatRG11B10Ufloat:
		return true
	default:
		return false
	}
}
