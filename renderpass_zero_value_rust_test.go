//go:build rust

package wgpu

import (
	"testing"

	"github.com/gogpu/gputypes"
)

// The zero-value contract of the rust RenderPassEncoder (ADR-070 §6, S5).
// The pre-move zero value answered a nil argument by returning before it
// touched the nil go-webgpu pass, and panicked on any call that reached the
// pass. The argument guards read the argument's implementation, so a
// zero-value wrapper argument was a no-op on any receiver. The declaration
// layer keeps both: its own nil guards run first, and the implementation's
// guards run behind them.
//
// The live-argument panic and the first-End success path need real wgpu-native
// objects, which a hermetic test cannot construct; the panic shape is pinned
// by the argument-free methods, and the browser context pins the guarded ones
// with live browser objects. The file compiles in the rust build and the rust
// vet pass and is executed by `go test -tags rust .`.

func TestRustRenderPassEncoderZeroValueNilArgumentsAreNoops(t *testing.T) {
	pass := &RenderPassEncoder{}
	pass.SetPipeline(nil)
	pass.SetBindGroup(0, nil, nil)
	pass.SetVertexBuffer(0, nil, 0)
	pass.SetIndexBuffer(nil, gputypes.IndexFormatUint16, 0)
	pass.SetBlendConstant(nil)

	// A zero-value wrapper argument has a nil implementation: the pre-move
	// guards answered it exactly like nil, on every receiver.
	pass.SetPipeline(&RenderPipeline{})
	pass.SetBindGroup(0, &BindGroup{}, nil)
	pass.SetVertexBuffer(0, &Buffer{}, 0)
	pass.SetIndexBuffer(&Buffer{}, gputypes.IndexFormatUint16, 0)
}

func TestRustRenderPassEncoderZeroValueRealCallsPanic(t *testing.T) {
	tests := []struct {
		name string
		call func(p *RenderPassEncoder)
	}{
		{"SetViewport", func(p *RenderPassEncoder) { p.SetViewport(gputypes.Viewport{}) }},
		{"SetScissorRect", func(p *RenderPassEncoder) { p.SetScissorRect(gputypes.ScissorRect{}) }},
		{"SetBlendConstant", func(p *RenderPassEncoder) { p.SetBlendConstant(&gputypes.Color{}) }},
		{"Draw", func(p *RenderPassEncoder) { p.Draw(gputypes.DrawArgs{}) }},
		{"DrawIndexed", func(p *RenderPassEncoder) { p.DrawIndexed(gputypes.DrawIndexedArgs{}) }},
		{"End", func(p *RenderPassEncoder) { _ = p.End() }},
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
