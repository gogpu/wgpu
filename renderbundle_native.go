//go:build !rust && !(js && wasm)

package wgpu

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/encoder"
)

// Render bundles exist in the native context only, exactly as before the
// ADR-070 S6 move: the browser and rust contexts have no bundle types.
//
// The implementation lives in internal/encoder; these declarations are the
// public surface. Both types stay comparable, so a value can be a map key
// (the root test of the comparable public types pins it).

// RenderBundle is a reusable sequence of render commands.
type RenderBundle struct {
	impl *encoder.RenderBundle
}

// Release drops the bundle's references to its resources. Submissions that
// executed the bundle hold references of their own until the GPU completes
// them. It is safe to call Release more than once.
func (b *RenderBundle) Release() {
	if b == nil {
		return
	}
	b.impl.Release()
}

// RenderBundleEncoder records reusable render commands. The commands are
// validated by Finish, which reports the first invalid one.
//
// NOT thread-safe.
type RenderBundleEncoder struct {
	impl *encoder.RenderBundleEncoder
}

// SetPipeline sets the render pipeline for the following draws.
func (e *RenderBundleEncoder) SetPipeline(pipeline *RenderPipeline) {
	e.impl.SetPipeline(encoderPipeline(pipeline))
}

// SetBindGroup sets a bind group for the given index. It may come before
// SetPipeline; the bundle binds it at the next draw whose pipeline uses it.
// The offsets are copied.
func (e *RenderBundleEncoder) SetBindGroup(index uint32, group *BindGroup, offsets []uint32) {
	e.impl.SetBindGroup(index, encoderBindGroup(group), offsets)
}

// SetVertexBuffer sets a vertex buffer for the given slot.
func (e *RenderBundleEncoder) SetVertexBuffer(slot uint32, buffer *Buffer, offset uint64) {
	e.impl.SetVertexBuffer(slot, encoderBuffer(buffer), offset)
}

// SetIndexBuffer sets the index buffer.
func (e *RenderBundleEncoder) SetIndexBuffer(buffer *Buffer, format gputypes.IndexFormat, offset uint64) {
	e.impl.SetIndexBuffer(encoderBuffer(buffer), format, offset)
}

// Draw draws primitives.
func (e *RenderBundleEncoder) Draw(args gputypes.DrawArgs) {
	e.impl.Draw(args)
}

// DrawIndexed draws indexed primitives.
func (e *RenderBundleEncoder) DrawIndexed(args gputypes.DrawIndexedArgs) {
	e.impl.DrawIndexed(args)
}

// Finish validates the recorded commands and returns the reusable render
// bundle, or the first validation error. The encoder cannot be used again.
func (e *RenderBundleEncoder) Finish(desc *RenderBundleDescriptor) (*RenderBundle, error) {
	var impl *encoder.RenderBundleEncoder
	if e != nil {
		impl = e.impl
	}
	bundle, err := impl.Finish(encoderRenderBundleDescriptor(desc))
	if err != nil {
		return nil, err
	}
	return &RenderBundle{impl: bundle}, nil
}

// encoderRenderBundleDescriptor converts a public bundle descriptor.
func encoderRenderBundleDescriptor(desc *RenderBundleDescriptor) *encoder.RenderBundleDescriptor {
	if desc == nil {
		return nil
	}
	return &encoder.RenderBundleDescriptor{Label: desc.Label}
}

// encoderRenderBundleEncoderDescriptor converts a public bundle encoder
// descriptor. The target validation copies the color formats (as it did before
// the move), so sharing the slice is not observable.
func encoderRenderBundleEncoderDescriptor(desc *RenderBundleEncoderDescriptor) *encoder.RenderBundleEncoderDescriptor {
	return &encoder.RenderBundleEncoderDescriptor{
		Label:              desc.Label,
		ColorFormats:       desc.ColorFormats,
		DepthStencilFormat: desc.DepthStencilFormat,
		SampleCount:        desc.SampleCount,
		DepthReadOnly:      desc.DepthReadOnly,
		StencilReadOnly:    desc.StencilReadOnly,
	}
}
