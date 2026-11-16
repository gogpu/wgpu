//go:build js && wasm

package wgpu

import (
	"errors"
	"syscall/js"
	"testing"

	"github.com/gogpu/wgpu/internal/browser"
	"github.com/gogpu/wgpu/internal/encoder"
	"github.com/gogpu/wgpu/internal/pipeline"
	"github.com/gogpu/wgpu/internal/resource"
)

// The zero-value contract of the browser ComputePassEncoder (ADR-070 §6, S6).
// The pre-move zero value answered a nil argument by returning before it
// touched the nil browser pass, and panicked on any call that reached the
// pass. The argument guards read the argument's implementation (they live in
// the destination now), so a zero-value wrapper argument is a no-op on any
// receiver, and a live argument on a zero-value receiver panics. The
// declaration layer keeps both: its delegations run into the implementation's
// own guards.
//
// The file is executed by the wasm-test gate (the root suite under node), not
// only compiled. It also carries the negative control's subject: the
// destination's End releases exactly once.

// liveBrowserComputeWrappers returns a pipeline, a bind group and a buffer
// whose implementations are non-nil browser objects: the argument shape that
// passes every guard and reaches the receiver's browser pass.
func liveBrowserComputeWrappers() (*ComputePipeline, *BindGroup, *Buffer) {
	raw := func() js.Value { return js.Global().Get("Object").New() }
	rawBuffer := raw()
	rawBuffer.Set("size", float64(16))
	rawBuffer.Set("usage", 0)
	return &ComputePipeline{impl: pipeline.NewComputePipeline(browser.NewComputePipeline(raw()))},
		&BindGroup{impl: resource.NewBindGroup(browser.NewBindGroup(raw()))},
		&Buffer{impl: resource.NewBufferBrowser(browser.NewBuffer(rawBuffer), 16, 0, resource.MapStateUnmapped)}
}

func TestBrowserComputePassEncoderZeroValueNilArgumentsAreNoops(t *testing.T) {
	pass := &ComputePassEncoder{}
	pass.SetPipeline(nil)
	pass.SetBindGroup(0, nil, nil)
	pass.DispatchIndirect(nil, 0)

	// A zero-value wrapper argument has a nil implementation: the pre-move
	// guards answered it exactly like nil, on every receiver.
	pass.SetPipeline(&ComputePipeline{})
	pass.SetBindGroup(0, &BindGroup{}, nil)
	pass.DispatchIndirect(&Buffer{}, 0)
}

func TestBrowserComputePassEncoderZeroValueRealCallsPanic(t *testing.T) {
	livePipeline, liveGroup, liveBuffer := liveBrowserComputeWrappers()
	tests := []struct {
		name string
		call func(p *ComputePassEncoder)
	}{
		{"SetPipeline", func(p *ComputePassEncoder) { p.SetPipeline(livePipeline) }},
		{"SetBindGroup", func(p *ComputePassEncoder) { p.SetBindGroup(0, liveGroup, nil) }},
		{"Dispatch", func(p *ComputePassEncoder) { p.Dispatch(1, 1, 1) }},
		{"DispatchIndirect", func(p *ComputePassEncoder) { p.DispatchIndirect(liveBuffer, 0) }},
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

// TestBrowserComputePassEncoderEndReleasesOnce pins the pre-move released
// flag: the first End reaches the browser pass, the second reports ErrReleased
// without touching it. The public ErrReleased is the shared core value, which
// internal/resource's browser sentinel binds; the destination returns that
// sentinel, so errors.Is matches across the boundary.
func TestBrowserComputePassEncoderEndReleasesOnce(t *testing.T) {
	ref := js.Global().Get("Object").New()
	ref.Set("end", js.FuncOf(func(js.Value, []js.Value) any { return nil }))
	pass := &ComputePassEncoder{impl: encoder.NewComputePassEncoder(browser.NewComputePassEncoder(ref))}

	if err := pass.End(); err != nil {
		t.Fatalf("first End() error = %v", err)
	}
	if err := pass.End(); !errors.Is(err, ErrReleased) {
		t.Fatalf("second End() error = %v, want ErrReleased", err)
	}
}
