//go:build !rust && !(js && wasm)

package encoder

import (
	"fmt"
	"sync/atomic"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/hal"
	"github.com/gogpu/wgpu/internal/pipeline"
	"github.com/gogpu/wgpu/internal/resource"
)

// Render bundles are recorded and replayed by core, not by the backends:
// the encoder records a command list, Finish validates it and resolves the
// bind groups through a binder, and ExecuteBundles replays the resolved list
// into the render pass's backend encoder. Every backend runs bundles, and a
// bundle's draws see only the state the bundle itself set.
//
// Reference: Rust wgpu-core command/bundle.rs (RenderBundleEncoder,
// RenderBundleEncoder::finish, RenderBundle::execute).
//
// The types lived in the root package until ADR-070 S6; their signatures name
// the domain's handle types (pipeline.RenderPipeline, resource.BindGroup,
// resource.Buffer), and the root declaration layer converts at the call
// boundary. The recording model and the constructor live in
// renderbundle_recording_native.go, the execution path in
// renderbundle_execute.go.

// RenderBundleEncoder records reusable render commands. The commands are
// validated by Finish, which reports the first invalid one.
//
// NOT thread-safe.
type RenderBundleEncoder struct {
	device core.DeviceView
	label  string
	// The recording lives behind a pointer so RenderBundleEncoder stays a
	// comparable type, as it was before core recorded bundles.
	*bundleRecording
	finished atomic.Bool
}

// record appends a command unless the encoder is finished.
func (e *RenderBundleEncoder) record(cmd bundleCommand) {
	if e.finished.Load() || e.bundleRecording == nil {
		return
	}
	e.commands = append(e.commands, cmd)
}

// SetPipeline sets the render pipeline for the following draws.
func (e *RenderBundleEncoder) SetPipeline(pl *pipeline.RenderPipeline) {
	e.record(bundleCommand{kind: bundleSetPipeline, pipeline: pl})
}

// SetBindGroup sets a bind group for the given index. It may come before
// SetPipeline; the bundle binds it at the next draw whose pipeline uses it.
// The offsets are copied.
func (e *RenderBundleEncoder) SetBindGroup(index uint32, group *resource.BindGroup, offsets []uint32) {
	var copied []uint32
	if len(offsets) > 0 {
		copied = append(copied, offsets...)
	}
	e.record(bundleCommand{kind: bundleSetBindGroup, slot: index, group: group, offsets: copied})
}

// SetVertexBuffer sets a vertex buffer for the given slot.
func (e *RenderBundleEncoder) SetVertexBuffer(slot uint32, buffer *resource.Buffer, offset uint64) {
	e.record(bundleCommand{kind: bundleSetVertexBuffer, slot: slot, buffer: buffer, offset: offset})
}

// SetIndexBuffer sets the index buffer.
func (e *RenderBundleEncoder) SetIndexBuffer(buffer *resource.Buffer, format gputypes.IndexFormat, offset uint64) {
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
	if e == nil || e.device == nil || e.bundleRecording == nil || e.finished.Swap(true) {
		return nil, fmt.Errorf("wgpu: RenderBundleEncoder.Finish: %w", core.ErrReleased)
	}
	commands := e.commands
	e.commands = nil
	if e.device.IsReleased() {
		return nil, fmt.Errorf("wgpu: RenderBundleEncoder.Finish: device: %w", core.ErrReleased)
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
	device core.DeviceView
	label  string
	// The contents live behind a pointer so RenderBundle stays a comparable
	// type, as it was before core recorded bundles.
	*bundleContents
	released atomic.Bool
}

// Release drops the bundle's references to its resources. Submissions that
// executed the bundle hold references of their own until the GPU completes
// them. It is safe to call Release more than once.
func (b *RenderBundle) Release() {
	if b == nil || b.released.Swap(true) {
		return
	}
	if b.bundleContents == nil {
		return
	}
	for _, ref := range b.refs {
		ref.Drop()
	}
	b.refs = nil
}

// NewEmptyBundleForTest returns a bundle with no commands for a render pass
// with no attachments. The public creation path refuses a descriptor without
// an attachment, and the root package's ExecuteBundles pass-state test needs
// exactly this compatible empty bundle. Test support only.
func NewEmptyBundleForTest(device core.DeviceView) *RenderBundle {
	return &RenderBundle{
		device: device,
		bundleContents: &bundleContents{
			target: renderBundleTarget{context: core.RenderPassContext{SampleCount: 1}, depthReadOnly: true, stencilReadOnly: true},
			scope:  core.NewRenderBundleScope(),
		},
	}
}

// replay records the bundle's resolved commands into a backend render pass.
// Finish validated them, so no state is tracked here.
// Matches Rust wgpu-core command/bundle.rs RenderBundle::execute.
func (b *RenderBundle) replay(raw hal.RenderPassEncoder) error {
	if raw == nil {
		return nil
	}
	for i := range b.commands {
		cmd := &b.commands[i]
		switch cmd.kind {
		case bundleSetPipeline:
			raw.SetPipeline(cmd.pipeline.HAL())
		case bundleSetBindGroup:
			raw.SetBindGroup(cmd.slot, cmd.group.HAL(), cmd.offsets)
		case bundleSetVertexBuffer:
			buffer := cmd.buffer.HALBuffer()
			if buffer == nil {
				return fmt.Errorf("vertex buffer %q: %w", cmd.buffer.Label(), core.ErrResourceDestroyed)
			}
			raw.SetVertexBuffer(cmd.slot, buffer, cmd.offset)
		case bundleSetIndexBuffer:
			buffer := cmd.buffer.HALBuffer()
			if buffer == nil {
				return fmt.Errorf("index buffer %q: %w", cmd.buffer.Label(), core.ErrResourceDestroyed)
			}
			raw.SetIndexBuffer(buffer, cmd.format, cmd.offset)
		case bundleDraw:
			raw.Draw(cmd.draw)
		case bundleDrawIndexed:
			raw.DrawIndexed(cmd.drawIndexed)
		}
	}
	return nil
}
