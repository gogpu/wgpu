//go:build !rust && !(js && wasm)

package wgpu

import (
	"testing"

	"github.com/gogpu/gputypes"
)

// The zero-value contract of the public RenderPassEncoder (ADR-070 §6, S5).
// The pre-move pass was a root struct whose zero value had no core state:
// every method that touched it panicked, and ExecuteBundles with an empty
// list was the one no-op (it never reached the pass). The declaration layer
// keeps that behavior: each method delegates through the nil implementation
// — or, for a nil pipeline, through the nil encoder — and panics the same
// way.

func TestRenderPassEncoderZeroValuePanics(t *testing.T) {
	tests := []struct {
		name string
		call func(p *RenderPassEncoder)
	}{
		{"SetPipeline(nil)", func(p *RenderPassEncoder) { p.SetPipeline(nil) }},
		{"SetPipeline(zero pipeline)", func(p *RenderPassEncoder) { p.SetPipeline(&RenderPipeline{}) }},
		{"SetBindGroup", func(p *RenderPassEncoder) { p.SetBindGroup(0, nil, nil) }},
		{"SetVertexBuffer", func(p *RenderPassEncoder) { p.SetVertexBuffer(0, nil, 0) }},
		{"SetIndexBuffer", func(p *RenderPassEncoder) { p.SetIndexBuffer(nil, gputypes.IndexFormatUint16, 0) }},
		{"SetViewport", func(p *RenderPassEncoder) { p.SetViewport(gputypes.Viewport{}) }},
		{"SetScissorRect", func(p *RenderPassEncoder) { p.SetScissorRect(gputypes.ScissorRect{}) }},
		{"SetBlendConstant", func(p *RenderPassEncoder) { p.SetBlendConstant(nil) }},
		{"SetStencilReference", func(p *RenderPassEncoder) { p.SetStencilReference(0) }},
		{"Draw", func(p *RenderPassEncoder) { p.Draw(gputypes.DrawArgs{}) }},
		{"DrawIndexed", func(p *RenderPassEncoder) { p.DrawIndexed(gputypes.DrawIndexedArgs{}) }},
		{"DrawIndirect", func(p *RenderPassEncoder) { p.DrawIndirect(nil, 0) }},
		{"MultiDrawIndirect", func(p *RenderPassEncoder) { p.MultiDrawIndirect(nil, 0, 1) }},
		{"DrawIndexedIndirect", func(p *RenderPassEncoder) { p.DrawIndexedIndirect(nil, 0) }},
		{"MultiDrawIndexedIndirect", func(p *RenderPassEncoder) { p.MultiDrawIndexedIndirect(nil, 0, 1) }},
		{"MultiDrawIndirectCount", func(p *RenderPassEncoder) { p.MultiDrawIndirectCount(nil, 0, nil, 0, 1) }},
		{"MultiDrawIndexedIndirectCount", func(p *RenderPassEncoder) { p.MultiDrawIndexedIndirectCount(nil, 0, nil, 0, 1) }},
		{"End", func(p *RenderPassEncoder) { _ = p.End() }},
		{"ExecuteBundles(nil)", func(p *RenderPassEncoder) { p.ExecuteBundles(nil) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatalf("%s on a zero-value RenderPassEncoder did not panic", tt.name)
				}
			}()
			tt.call(&RenderPassEncoder{})
		})
	}
}

// TestRenderPassEncoderZeroValueExecuteBundlesEmptyIsNoop pins the one
// pre-move non-panic: an empty bundle list never reaches the pass, so the
// post-loop reset is not taken either.
func TestRenderPassEncoderZeroValueExecuteBundlesEmptyIsNoop(t *testing.T) {
	(&RenderPassEncoder{}).ExecuteBundles()
}
