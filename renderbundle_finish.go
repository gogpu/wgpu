//go:build !rust && !(js && wasm)

package wgpu

import (
	"errors"
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/core"
	"github.com/gogpu/wgpu/core/track"
)

// vertexBufferAlignment is the alignment of a vertex buffer offset
// (WebGPU setVertexBuffer; Rust wgpu-types VERTEX_ALIGNMENT).
const vertexBufferAlignment = 4

// bundleState validates the commands of a render bundle encoder in order
// and builds the bundle's resolved command list: SetBindGroup is held by a
// binder and emitted right before each draw, for the groups the draw's
// pipeline uses, so the bundle never binds a group without its pipeline.
// Matches Rust wgpu-core command/bundle.rs State.
type bundleState struct {
	device *Device
	target *renderBundleTarget
	binder binder

	pipeline          *RenderPipeline
	vertexBufferCount uint32
	indexBufferSet    bool
	indexFormat       gputypes.IndexFormat

	commands []bundleCommand
	scope    *core.RenderBundleScope

	refs    []*core.ResourceRef
	seen    map[*core.ResourceRef]struct{}
	groups  []*BindGroup
	buffers []*Buffer
}

func newBundleState(device *Device, target *renderBundleTarget) *bundleState {
	return &bundleState{
		device: device,
		target: target,
		scope:  core.NewRenderBundleScope(),
		seen:   make(map[*core.ResourceRef]struct{}),
	}
}

// apply validates one recorded command and updates the state.
// Matches the command loop of Rust wgpu-core RenderBundleEncoder::finish.
func (s *bundleState) apply(cmd *bundleCommand) error {
	switch cmd.kind {
	case bundleSetPipeline:
		return s.setPipeline(cmd.pipeline)
	case bundleSetBindGroup:
		return s.setBindGroup(cmd.slot, cmd.group, cmd.offsets)
	case bundleSetVertexBuffer:
		return s.setVertexBuffer(cmd)
	case bundleSetIndexBuffer:
		return s.setIndexBuffer(cmd)
	case bundleDraw:
		return s.draw(cmd.draw)
	case bundleDrawIndexed:
		return s.drawIndexed(cmd.drawIndexed)
	default:
		return fmt.Errorf("%w: unknown command %d", errBundleCommand, cmd.kind)
	}
}

// setPipeline checks the pipeline against the bundle's attachment formats,
// sample count and read-only aspects. Matches Rust bundle.rs set_pipeline.
func (s *bundleState) setPipeline(pipeline *RenderPipeline) error {
	if pipeline == nil {
		return fmt.Errorf("SetPipeline: %w: pipeline is nil", errBundleCommand)
	}
	if pipeline.released {
		return fmt.Errorf("SetPipeline: pipeline: %w", ErrReleased)
	}
	if pipeline.device != s.device {
		return fmt.Errorf("SetPipeline: pipeline: %w", errBundleDevice)
	}
	if err := s.target.context.CheckCompatible(&pipeline.passContext, refLabel(pipeline.ref)); err != nil {
		return fmt.Errorf("SetPipeline: %w", err)
	}
	if (pipeline.writesDepth && s.target.depthReadOnly) || (pipeline.writesStencil && s.target.stencilReadOnly) {
		return fmt.Errorf("SetPipeline: %w", errBundleDepthWrite)
	}
	s.commands = append(s.commands, bundleCommand{kind: bundleSetPipeline, pipeline: pipeline})
	s.pipeline = pipeline
	s.binder.updateExpectations(pipeline.bindGroupLayouts)
	s.binder.updateLateBufferBindingsFromPipeline(pipeline.lateSizedBufferGroups)
	s.keep(pipeline.ref)
	return nil
}

// setBindGroup validates the group and its dynamic offsets and assigns it to
// the binder; nothing is emitted until a draw uses it.
// Matches Rust bundle.rs set_bind_group.
func (s *bundleState) setBindGroup(index uint32, group *BindGroup, offsets []uint32) error {
	if err := validateSetBindGroup("RenderBundleEncoder", index, group, offsets, &s.device.core.Limits); err != nil {
		return err
	}
	if group.released != nil && group.released.Load() {
		return fmt.Errorf("SetBindGroup: index %d: %w", index, ErrReleased)
	}
	if group.device != s.device {
		return fmt.Errorf("SetBindGroup: index %d: %w", index, errBundleDevice)
	}
	if err := s.scope.MergeBindGroup(group.usages); err != nil {
		return fmt.Errorf("SetBindGroup: index %d: %w", index, err)
	}
	s.binder.setGroup(index, group, offsets)
	s.keepBindGroup(group)
	return nil
}

// checkBuffer validates a vertex or index buffer of the bundle.
func (s *bundleState) checkBuffer(method string, buffer *Buffer, usage gputypes.BufferUsage, offset uint64) error {
	if buffer == nil {
		return fmt.Errorf("%s: %w: buffer is nil", method, errBundleCommand)
	}
	if buffer.core == nil || (buffer.released != nil && buffer.released.Load()) {
		return fmt.Errorf("%s: buffer: %w", method, ErrReleased)
	}
	if buffer.device != s.device {
		return fmt.Errorf("%s: buffer: %w", method, errBundleDevice)
	}
	if buffer.Usage()&usage == 0 {
		return fmt.Errorf("%s: %w: buffer %q lacks usage %v", method, errBundleCommand, buffer.Label(), usage)
	}
	if offset > buffer.Size() {
		return fmt.Errorf("%s: %w: offset %d exceeds buffer size %d", method, errBundleCommand, offset, buffer.Size())
	}
	return nil
}

// setVertexBuffer matches Rust bundle.rs set_vertex_buffer.
func (s *bundleState) setVertexBuffer(cmd *bundleCommand) error {
	if limit := s.device.core.Limits.MaxVertexBuffers; limit > 0 && cmd.slot >= limit {
		return fmt.Errorf("SetVertexBuffer: %w: slot %d >= maxVertexBuffers %d", errBundleCommand, cmd.slot, limit)
	}
	if err := s.checkBuffer("SetVertexBuffer", cmd.buffer, BufferUsageVertex, cmd.offset); err != nil {
		return err
	}
	if cmd.offset%vertexBufferAlignment != 0 {
		return fmt.Errorf("SetVertexBuffer: %w: offset %d is not a multiple of %d",
			errBundleCommand, cmd.offset, vertexBufferAlignment)
	}
	if err := s.scope.MergeBuffer(cmd.buffer.core, track.BufferUsesVertex); err != nil {
		return fmt.Errorf("SetVertexBuffer: slot %d: %w", cmd.slot, err)
	}
	s.vertexBufferCount = max(s.vertexBufferCount, cmd.slot+1)
	s.commands = append(s.commands, *cmd)
	s.keepBuffer(cmd.buffer)
	return nil
}

// setIndexBuffer matches Rust bundle.rs set_index_buffer.
func (s *bundleState) setIndexBuffer(cmd *bundleCommand) error {
	if err := s.checkBuffer("SetIndexBuffer", cmd.buffer, BufferUsageIndex, cmd.offset); err != nil {
		return err
	}
	size := uint64(cmd.format.Size())
	if size == 0 {
		return fmt.Errorf("SetIndexBuffer: %w: invalid index format %v", errBundleCommand, cmd.format)
	}
	if cmd.offset%size != 0 {
		return fmt.Errorf("SetIndexBuffer: %w: offset %d is not a multiple of the index size %d",
			errBundleCommand, cmd.offset, size)
	}
	if err := s.scope.MergeBuffer(cmd.buffer.core, track.BufferUsesIndex); err != nil {
		return fmt.Errorf("SetIndexBuffer: %w", err)
	}
	s.indexBufferSet = true
	s.indexFormat = cmd.format
	s.commands = append(s.commands, *cmd)
	s.keepBuffer(cmd.buffer)
	return nil
}

// isReady validates the state a draw needs, as a render pass does.
// Matches Rust bundle.rs State::is_ready.
func (s *bundleState) isReady(method string, indexed bool) error {
	if s.pipeline == nil {
		return fmt.Errorf("%s: no pipeline set (call SetPipeline first): %w", method, ErrDrawMissingPipeline)
	}
	if err := s.binder.checkCompatibility(); err != nil {
		sentinel := ErrDrawMissingBindGroup
		if errors.Is(err, errBindGroupIncompatible) {
			sentinel = ErrDrawIncompatibleBindGroup
		}
		return fmt.Errorf("%s: %w: %w", method, sentinel, err)
	}
	if err := s.binder.checkLateBufferBindings(); err != nil {
		return fmt.Errorf("%s: %w: %w", method, ErrDrawLateBufferTooSmall, err)
	}
	if s.vertexBufferCount < s.pipeline.requiredVertexBuffers {
		return fmt.Errorf("%s: pipeline requires %d vertex buffer(s) but only %d set: %w",
			method, s.pipeline.requiredVertexBuffers, s.vertexBufferCount, ErrDrawMissingVertexBuffer)
	}
	if !indexed {
		return nil
	}
	if !s.indexBufferSet {
		return fmt.Errorf("%s: no index buffer set (call SetIndexBuffer first): %w", method, ErrDrawMissingIndexBuffer)
	}
	if strip := s.pipeline.stripIndexFormat; strip != nil && s.indexFormat != *strip {
		return fmt.Errorf("%s: index buffer format %v does not match pipeline strip index format %v: %w",
			method, s.indexFormat, *strip, ErrDrawIndexFormatMismatch)
	}
	return nil
}

// draw matches Rust bundle.rs draw: a draw with no vertices or instances is
// validated but not recorded.
func (s *bundleState) draw(args gputypes.DrawArgs) error {
	if err := s.isReady("Draw", false); err != nil {
		return err
	}
	if args.VertexCount > 0 && args.InstanceCount > 0 {
		s.flushBindings()
		s.commands = append(s.commands, bundleCommand{kind: bundleDraw, draw: args})
	}
	return nil
}

// drawIndexed matches Rust bundle.rs draw_indexed.
func (s *bundleState) drawIndexed(args gputypes.DrawIndexedArgs) error {
	if err := s.isReady("DrawIndexed", true); err != nil {
		return err
	}
	if args.IndexCount > 0 && args.InstanceCount > 0 {
		s.flushBindings()
		s.commands = append(s.commands, bundleCommand{kind: bundleDrawIndexed, drawIndexed: args})
	}
	return nil
}

// flushBindings emits SetBindGroup for every group whose binding may be
// stale and that the current pipeline uses.
// Matches Rust bundle.rs State::flush_bindings.
func (s *bundleState) flushBindings() {
	s.binder.listValid(s.binder.takeRebindStart(), func(i uint32, group *BindGroup, offsets []uint32) {
		s.commands = append(s.commands, bundleCommand{kind: bundleSetBindGroup, slot: i, group: group, offsets: offsets})
	})
}

// keep records a reference the bundle holds, once per resource.
func (s *bundleState) keep(ref *core.ResourceRef) {
	if ref == nil {
		return
	}
	if _, ok := s.seen[ref]; ok {
		return
	}
	s.seen[ref] = struct{}{}
	s.refs = append(s.refs, ref)
}

// keepBindGroup keeps a bind group and the buffers bound in it, as a pass's
// SetBindGroup does.
func (s *bundleState) keepBindGroup(group *BindGroup) {
	if _, ok := s.seen[group.ref]; !ok || group.ref == nil {
		s.groups = append(s.groups, group)
	}
	s.keep(group.ref)
	for _, buf := range group.boundBuffers {
		if buf.core != nil {
			s.keep(buf.core.Ref)
		}
	}
}

// keepBuffer keeps a vertex or index buffer.
func (s *bundleState) keepBuffer(buffer *Buffer) {
	if _, ok := s.seen[buffer.core.Ref]; !ok || buffer.core.Ref == nil {
		s.buffers = append(s.buffers, buffer)
	}
	s.keep(buffer.core.Ref)
}

// build returns the finished bundle. It takes a reference to every resource
// the bundle uses, so they live as long as the bundle (Rust: the bundle's
// commands hold an Arc of each resource).
func (s *bundleState) build(label string) *RenderBundle {
	for _, ref := range s.refs {
		ref.Clone()
	}
	return &RenderBundle{
		device:   s.device,
		label:    label,
		target:   *s.target,
		commands: s.commands,
		scope:    s.scope,
		refs:     s.refs,
		groups:   s.groups,
		buffers:  s.buffers,
	}
}
