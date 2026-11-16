//go:build !rust && !(js && wasm)

package wgpu

// The render bundle's record model and its constructor live here, in the root
// package, until the S6 Move relocates them into internal/encoder. They are
// named from staying root files — device_native.go creates the encoder through
// the constructor, and the root-side bundle execution path and the white-box
// tests name the target and contents types — and a move may not leave an
// unexported package-scope declaration behind for a staying file (the
// `unexported-boundary` refusal class; the pattern is the S5 binder shuttle).
//
// These declarations are byte-identical to their pre-shuttle form except the
// constructor's device argument, which is the core.DeviceView seam the S6
// destination can name. They travel at the Move together with their users —
// the bundle encoder, the bundle and the bundle's validation state.

import (
	"errors"
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/resource"
)

// Unexported sentinels of render bundle validation, wrapped with the details.
var (
	errBundleDescriptor = errors.New("invalid render bundle encoder descriptor")
	errBundleCommand    = errors.New("invalid render bundle command")
	errBundleDepthWrite = errors.New("pipeline writes a depth/stencil aspect the render bundle keeps read-only")
	errBundleReadOnly   = errors.New("render pass depth/stencil is read-only but the render bundle is not")
	errBundleDevice     = errors.New("resource belongs to another device")
)

// bundleCommandKind identifies a render bundle command.
type bundleCommandKind uint8

const (
	bundleSetPipeline bundleCommandKind = iota
	bundleSetBindGroup
	bundleSetVertexBuffer
	bundleSetIndexBuffer
	bundleDraw
	bundleDrawIndexed
)

// bundleCommand is one command recorded by a RenderBundleEncoder, or one
// command of a finished RenderBundle.
// Matches Rust wgpu-core RenderCommand / ArcRenderCommand.
type bundleCommand struct {
	kind bundleCommandKind
	// slot is the bind group index or the vertex buffer slot.
	slot     uint32
	pipeline *RenderPipeline
	group    *resource.BindGroup
	// offsets is the command's own copy of the dynamic offsets.
	offsets     []uint32
	buffer      *Buffer
	format      gputypes.IndexFormat
	offset      uint64
	draw        gputypes.DrawArgs
	drawIndexed gputypes.DrawIndexedArgs
}

// renderBundleTarget is the render pass layout a bundle is recorded for:
// its attachment formats, sample count and read-only depth/stencil aspects.
type renderBundleTarget struct {
	context         core.RenderPassContext
	depthReadOnly   bool
	stencilReadOnly bool
}

// bundleRecording is what a RenderBundleEncoder records until Finish.
type bundleRecording struct {
	target   renderBundleTarget
	commands []bundleCommand
}

// newRenderBundleEncoder validates desc and returns an encoder for it.
// Matches Rust wgpu-core RenderBundleEncoder::new.
func newRenderBundleEncoder(device core.DeviceView, desc *RenderBundleEncoderDescriptor) (*RenderBundleEncoder, error) {
	target, err := renderBundleTargetOf(desc, &device.Core().Limits)
	if err != nil {
		return nil, err
	}
	return &RenderBundleEncoder{device: device, label: desc.Label, bundleRecording: &bundleRecording{target: target}}, nil
}

// renderBundleTargetOf validates the attachment formats of a render bundle
// encoder descriptor. An aspect the format does not have counts as read-only,
// and without a depth/stencil format both do.
// Matches Rust wgpu-core validate_render_bundle_encoder_descriptor.
func renderBundleTargetOf(desc *RenderBundleEncoderDescriptor, limits *gputypes.Limits) (renderBundleTarget, error) {
	target := renderBundleTarget{
		context: core.RenderPassContext{
			ColorFormats:       append([]gputypes.TextureFormat(nil), desc.ColorFormats...),
			DepthStencilFormat: desc.DepthStencilFormat,
			SampleCount:        max(desc.SampleCount, 1),
		},
		depthReadOnly:   true,
		stencilReadOnly: true,
	}
	if limits.MaxColorAttachments > 0 && uint32(len(desc.ColorFormats)) > limits.MaxColorAttachments { //nolint:gosec // slice length fits uint32
		return target, fmt.Errorf("%w: %d color formats exceed maxColorAttachments %d",
			errBundleDescriptor, len(desc.ColorFormats), limits.MaxColorAttachments)
	}
	hasAttachment := false
	for i, format := range desc.ColorFormats {
		if format == gputypes.TextureFormatUndefined {
			continue
		}
		hasAttachment = true
		if format.IsDepthStencil() || format.IsCompressed() {
			return target, fmt.Errorf("%w: color format [%d] %s is not a renderable color format",
				errBundleDescriptor, i, format)
		}
	}
	if ds := desc.DepthStencilFormat; ds != gputypes.TextureFormatUndefined {
		hasAttachment = true
		if !ds.IsDepthStencil() {
			return target, fmt.Errorf("%w: depth/stencil format %s is not a depth or stencil format", errBundleDescriptor, ds)
		}
		target.depthReadOnly = !ds.HasDepth() || desc.DepthReadOnly
		target.stencilReadOnly = !ds.HasStencil() || desc.StencilReadOnly
	}
	if !hasAttachment {
		return target, fmt.Errorf("%w: a render bundle needs at least one color or depth/stencil format", errBundleDescriptor)
	}
	return target, nil
}

// bundleContents is a finished bundle's validated contents.
type bundleContents struct {
	target renderBundleTarget
	// commands is the resolved command list replayed by ExecuteBundles.
	commands []bundleCommand
	// scope holds the usages of the bundle's resources, merged into the
	// usage scope of every pass that executes the bundle.
	scope *core.RenderBundleScope
	// refs are the bundle's references to its pipelines, bind groups and
	// buffers (and the buffers of its bind groups), held until Release.
	refs []*core.ResourceRef
	// groups and buffers are recorded on the command encoder of every pass
	// that executes the bundle, for submit-time validation.
	groups  []*resource.BindGroup
	buffers []*Buffer
}
