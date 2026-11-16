//go:build js && wasm

package wgpu

import (
	"syscall/js"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/browser"
	"github.com/gogpu/wgpu/internal/pipeline"
	"github.com/gogpu/wgpu/internal/resource"
)

// The zero-value contract of the browser RenderPassEncoder (ADR-070 §6, S5).
// The pre-move zero value answered a nil argument by returning before it
// touched the nil browser pass, and panicked on any call that reached the
// pass. The argument guards read the argument's implementation, so a
// zero-value wrapper argument was a no-op on any receiver, and a live
// argument on a zero-value receiver panicked. The declaration layer keeps
// both: its own nil guards run first, and the implementation's guards run
// behind them.
//
// The file is executed by the wasm-test gate (the root suite under node), not
// only compiled.

// liveBrowserWrappers returns a pipeline, a bind group and a buffer whose
// implementations are non-nil browser objects: the argument shape that passes
// every guard and reaches the receiver's browser pass.
func liveBrowserWrappers() (*RenderPipeline, *BindGroup, *Buffer) {
	raw := func() js.Value { return js.Global().Get("Object").New() }
	rawBuffer := raw()
	rawBuffer.Set("size", float64(16))
	rawBuffer.Set("usage", 0)
	return &RenderPipeline{impl: pipeline.NewRenderPipeline(browser.NewRenderPipeline(raw()))},
		&BindGroup{impl: resource.NewBindGroup(browser.NewBindGroup(raw()))},
		&Buffer{impl: resource.NewBufferBrowser(browser.NewBuffer(rawBuffer), 16, 0, resource.MapStateUnmapped)}
}

func TestBrowserRenderPassEncoderZeroValueNilArgumentsAreNoops(t *testing.T) {
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

func TestBrowserRenderPassEncoderZeroValueRealCallsPanic(t *testing.T) {
	livePipeline, liveGroup, liveBuffer := liveBrowserWrappers()
	tests := []struct {
		name string
		call func(p *RenderPassEncoder)
	}{
		{"SetPipeline", func(p *RenderPassEncoder) { p.SetPipeline(livePipeline) }},
		{"SetBindGroup", func(p *RenderPassEncoder) { p.SetBindGroup(0, liveGroup, nil) }},
		{"SetVertexBuffer", func(p *RenderPassEncoder) { p.SetVertexBuffer(0, liveBuffer, 0) }},
		{"SetIndexBuffer", func(p *RenderPassEncoder) { p.SetIndexBuffer(liveBuffer, gputypes.IndexFormatUint16, 0) }},
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
