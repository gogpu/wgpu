//go:build !rust && !(js && wasm)

package encoder

import (
	"errors"
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/pipeline"
	"github.com/gogpu/wgpu/internal/resource"
)

// The render bundle's record model and its constructor. They were the root
// package's shuttled declarations until the S6 Move: a move may not leave an
// unexported package-scope declaration behind for a staying root file (the
// `unexported-boundary` refusal class), so they sat next to their staying root
// users, and they travel here together with their users — the bundle encoder,
// the bundle and the bundle's validation state.
//
// The sentinels are exported because the root declaration layer still names
// them (its zero-value tests match with errors.Is), and the root keeps
// unexported aliases of these values, so the identity is the same value on
// both sides (the S5 ErrDraw* precedent).

// Sentinels of render bundle validation, wrapped with the details.
var (
	// ErrBundleDescriptor is wrapped by a render bundle encoder descriptor
	// whose attachment formats are invalid.
	ErrBundleDescriptor = errors.New("invalid render bundle encoder descriptor")

	// ErrBundleCommand is wrapped by the first invalid command a bundle
	// encoder records.
	ErrBundleCommand = errors.New("invalid render bundle command")

	// ErrBundleDepthWrite is wrapped when a pipeline writes a depth/stencil
	// aspect the bundle keeps read-only.
	ErrBundleDepthWrite = errors.New("pipeline writes a depth/stencil aspect the render bundle keeps read-only")

	// ErrBundleReadOnly is wrapped when a bundle that writes a read-only
	// aspect is executed in a read-only pass.
	ErrBundleReadOnly = errors.New("render pass depth/stencil is read-only but the render bundle is not")

	// ErrBundleDevice is wrapped when a resource recorded in a bundle belongs
	// to another device.
	ErrBundleDevice = errors.New("resource belongs to another device")
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
	pipeline *pipeline.RenderPipeline
	group    *resource.BindGroup
	// offsets is the command's own copy of the dynamic offsets.
	offsets     []uint32
	buffer      *resource.Buffer
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

// NewRenderBundleEncoder validates desc and returns an encoder for it.
// Matches Rust wgpu-core RenderBundleEncoder::new.
func NewRenderBundleEncoder(device core.DeviceView, desc *RenderBundleEncoderDescriptor) (*RenderBundleEncoder, error) {
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
			ErrBundleDescriptor, len(desc.ColorFormats), limits.MaxColorAttachments)
	}
	hasAttachment := false
	for i, format := range desc.ColorFormats {
		if format == gputypes.TextureFormatUndefined {
			continue
		}
		hasAttachment = true
		if format.IsDepthStencil() || format.IsCompressed() {
			return target, fmt.Errorf("%w: color format [%d] %s is not a renderable color format",
				ErrBundleDescriptor, i, format)
		}
	}
	if ds := desc.DepthStencilFormat; ds != gputypes.TextureFormatUndefined {
		hasAttachment = true
		if !ds.IsDepthStencil() {
			return target, fmt.Errorf("%w: depth/stencil format %s is not a depth or stencil format", ErrBundleDescriptor, ds)
		}
		target.depthReadOnly = !ds.HasDepth() || desc.DepthReadOnly
		target.stencilReadOnly = !ds.HasStencil() || desc.StencilReadOnly
	}
	if !hasAttachment {
		return target, fmt.Errorf("%w: a render bundle needs at least one color or depth/stencil format", ErrBundleDescriptor)
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
	buffers []*resource.Buffer
}
