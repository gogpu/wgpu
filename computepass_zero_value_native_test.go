//go:build !rust && !(js && wasm)

package wgpu

import (
	"testing"
)

// The zero-value contract of the public ComputePassEncoder (ADR-070 §6, S6).
// The pre-move pass was a root struct whose zero value had no core pass and no
// encoder: every method panicked — SetPipeline(nil) through the nil encoder's
// error sink, the rest through the nil core pass. The declaration layer keeps
// that behavior: each method delegates through the nil implementation, or, for
// a nil pipeline, through the nil encoder, and panics the same way. No guard
// was added.

func TestComputePassEncoderZeroValuePanics(t *testing.T) {
	tests := []struct {
		name string
		call func(p *ComputePassEncoder)
	}{
		{"SetPipeline(nil)", func(p *ComputePassEncoder) { p.SetPipeline(nil) }},
		{"SetPipeline(zero pipeline)", func(p *ComputePassEncoder) { p.SetPipeline(&ComputePipeline{}) }},
		{"SetBindGroup", func(p *ComputePassEncoder) { p.SetBindGroup(0, nil, nil) }},
		{"Dispatch", func(p *ComputePassEncoder) { p.Dispatch(1, 1, 1) }},
		{"DispatchIndirect", func(p *ComputePassEncoder) { p.DispatchIndirect(nil, 0) }},
		{"End", func(p *ComputePassEncoder) { _ = p.End() }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatalf("%s on a zero-value ComputePassEncoder did not panic", tt.name)
				}
			}()
			tt.call(&ComputePassEncoder{})
		})
	}
}
