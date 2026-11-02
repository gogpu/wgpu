//go:build !rust && !(js && wasm)

package wgpu

import (
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/core"
)

// Render bundles are recorded and replayed by core, not by the backends:
// the encoder records a command list, Finish validates it and resolves the
// bind groups through a binder, and ExecuteBundles replays the resolved list
// into the render pass's backend encoder. Every backend runs bundles, and a
// bundle's draws see only the state the bundle itself set.
//
// Reference: Rust wgpu-core command/bundle.rs (RenderBundleEncoder,
// RenderBundleEncoder::finish, RenderBundle::execute).

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
	group    *BindGroup
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

// RenderBundleEncoder records reusable render commands. The commands are
// validated by Finish, which reports the first invalid one.
//
// NOT thread-safe.
type RenderBundleEncoder struct {
	device   *Device
	label    string
	target   renderBundleTarget
	commands []bundleCommand
	finished atomic.Bool
}

// newRenderBundleEncoder validates desc and returns an encoder for it.
// Matches Rust wgpu-core RenderBundleEncoder::new.
func newRenderBundleEncoder(device *Device, desc *RenderBundleEncoderDescriptor) (*RenderBundleEncoder, error) {
	target, err := renderBundleTargetOf(desc, &device.core.Limits)
	if err != nil {
		return nil, err
	}
	return &RenderBundleEncoder{device: device, label: desc.Label, target: target}, nil
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

// record appends a command unless the encoder is finished.
func (e *RenderBundleEncoder) record(cmd bundleCommand) {
	if e.finished.Load() {
		return
	}
	e.commands = append(e.commands, cmd)
}

// SetPipeline sets the render pipeline for the following draws.
func (e *RenderBundleEncoder) SetPipeline(pipeline *RenderPipeline) {
	e.record(bundleCommand{kind: bundleSetPipeline, pipeline: pipeline})
}

// SetBindGroup sets a bind group for the given index. It may come before
// SetPipeline; the bundle binds it at the next draw whose pipeline uses it.
// The offsets are copied.
func (e *RenderBundleEncoder) SetBindGroup(index uint32, group *BindGroup, offsets []uint32) {
	var copied []uint32
	if len(offsets) > 0 {
		copied = append(copied, offsets...)
	}
	e.record(bundleCommand{kind: bundleSetBindGroup, slot: index, group: group, offsets: copied})
}

// SetVertexBuffer sets a vertex buffer for the given slot.
func (e *RenderBundleEncoder) SetVertexBuffer(slot uint32, buffer *Buffer, offset uint64) {
	e.record(bundleCommand{kind: bundleSetVertexBuffer, slot: slot, buffer: buffer, offset: offset})
}

// SetIndexBuffer sets the index buffer.
func (e *RenderBundleEncoder) SetIndexBuffer(buffer *Buffer, format gputypes.IndexFormat, offset uint64) {
	e.record(bundleCommand{kind: bundleSetIndexBuffer, buffer: buffer, format: format, offset: offset})
}

// Draw draws primitives.
func (e *RenderBundleEncoder) Draw(args gputypes.DrawArgs) {
	e.record(bundleCommand{kind: bundleDraw, draw: args})
}

// DrawIndexed draws indexed primitives.
func (e *RenderBundleEncoder) DrawIndexed(args gputypes.DrawIndexedArgs) {
	e.record(bundleCommand{kind: bundleDrawIndexed, drawIndexed: args})
}

// Finish validates the recorded commands and returns the reusable render
// bundle, or the first validation error. The encoder cannot be used again.
func (e *RenderBundleEncoder) Finish(desc *RenderBundleDescriptor) (*RenderBundle, error) {
	if e == nil || e.device == nil || e.finished.Swap(true) {
		return nil, fmt.Errorf("wgpu: RenderBundleEncoder.Finish: %w", ErrReleased)
	}
	commands := e.commands
	e.commands = nil
	if e.device.released.Load() {
		return nil, fmt.Errorf("wgpu: RenderBundleEncoder.Finish: device: %w", ErrReleased)
	}

	label := ""
	if desc != nil {
		label = desc.Label
	}
	state := newBundleState(e.device, &e.target)
	for i := range commands {
		if err := state.apply(&commands[i]); err != nil {
			return nil, fmt.Errorf("wgpu: RenderBundleEncoder.Finish: encoder %q, command %d: %w", e.label, i, err)
		}
	}
	return state.build(label), nil
}

// RenderBundle is a reusable sequence of render commands.
type RenderBundle struct {
	device *Device
	label  string
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
	groups   []*BindGroup
	buffers  []*Buffer
	released atomic.Bool
}

// Release drops the bundle's references to its resources. Submissions that
// executed the bundle hold references of their own until the GPU completes
// them. It is safe to call Release more than once.
func (b *RenderBundle) Release() {
	if b == nil || b.released.Swap(true) {
		return
	}
	for _, ref := range b.refs {
		ref.Drop()
	}
	b.refs = nil
}
