//go:build rust

package wgpu

import (
	"testing"
)

// The zero-value contract of the rust ComputePassEncoder (ADR-070 §6, S6).
// The pre-move zero value answered a nil argument by returning before it
// touched the nil go-webgpu pass, and panicked on any call that reached the
// pass. The declaration layer keeps both: the destination's own argument
// guards run first, and a call that reaches the nil backend panics.
//
// The live-argument panic and the first-End success path need real wgpu-native
// objects, which a hermetic test cannot construct; the panic shape is pinned
// by the argument-free methods, and the browser context pins the guarded ones
// with live browser objects. The file compiles in the rust build and the rust
// vet pass and is executed by `go test -tags rust .`.

func TestRustComputePassEncoderZeroValueNilArgumentsAreNoops(t *testing.T) {
	pass := &ComputePassEncoder{}
	pass.SetPipeline(nil)
	pass.SetBindGroup(0, nil, nil)
	pass.DispatchIndirect(nil, 0)

	// A zero-value wrapper argument has a nil implementation: the pre-move
	// guards answered it exactly like nil, on any receiver.
	pass.SetPipeline(&ComputePipeline{})
	pass.SetBindGroup(0, &BindGroup{}, nil)
	pass.DispatchIndirect(&Buffer{}, 0)
}

func TestRustComputePassEncoderZeroValueRealCallsPanic(t *testing.T) {
	tests := []struct {
		name string
		call func(p *ComputePassEncoder)
	}{
		{"Dispatch", func(p *ComputePassEncoder) { p.Dispatch(1, 1, 1) }},
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
