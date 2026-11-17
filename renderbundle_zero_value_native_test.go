//go:build !rust && !(js && wasm)

package wgpu

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/hal/noop"
)

// The zero-value contract of the public RenderBundle and RenderBundleEncoder
// (ADR-070 §6, S6). Before the move both types were root structs: Release on a
// nil or zero bundle was a no-op, Finish on a nil or zero encoder reported
// ErrReleased, and every recording method on a zero encoder was a silent
// no-op (the nil recording returned early). The moved implementations keep
// those checks; the declaration layer reproduces the nil-receiver halves it
// must (Release, and Finish's two-level nil) and delegates the rest.

func TestRenderBundleZeroValueReleaseIsNoop(t *testing.T) {
	var bundle RenderBundle
	bundle.Release()
	bundle.Release()

	var nilBundle *RenderBundle
	nilBundle.Release()
}

func TestRenderBundleEncoderZeroValueFinishIsReleased(t *testing.T) {
	var enc RenderBundleEncoder
	if bundle, err := enc.Finish(nil); bundle != nil || !errors.Is(err, ErrReleased) {
		t.Fatalf("Finish() = (%v, %v), want ErrReleased", bundle, err)
	}

	var nilEncoder *RenderBundleEncoder
	if bundle, err := nilEncoder.Finish(nil); bundle != nil || !errors.Is(err, ErrReleased) {
		t.Fatalf("nil Finish() = (%v, %v), want ErrReleased", bundle, err)
	}
}

func TestRenderBundleEncoderZeroValueRecordingIsNoop(t *testing.T) {
	tests := []struct {
		name string
		call func(e *RenderBundleEncoder)
	}{
		{"SetPipeline", func(e *RenderBundleEncoder) { e.SetPipeline(nil) }},
		{"SetBindGroup", func(e *RenderBundleEncoder) { e.SetBindGroup(0, nil, nil) }},
		{"SetVertexBuffer", func(e *RenderBundleEncoder) { e.SetVertexBuffer(0, nil, 0) }},
		{"SetIndexBuffer", func(e *RenderBundleEncoder) { e.SetIndexBuffer(nil, gputypes.IndexFormatUint16, 0) }},
		{"Draw", func(e *RenderBundleEncoder) { e.Draw(gputypes.DrawArgs{}) }},
		{"DrawIndexed", func(e *RenderBundleEncoder) { e.DrawIndexed(gputypes.DrawIndexedArgs{}) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The pre-move encoder returned early on its nil recording; a
			// recording call on the declared zero value is a silent no-op and
			// must not panic through the nil implementation (the destination's
			// record guard carries it).
			tt.call(&RenderBundleEncoder{})
		})
	}
}

// TestDeviceCreateRenderBundleEncoderNilDescriptor pins the public
// Device.CreateRenderBundleEncoder(nil) guard. The case lived in the root
// encoder test before the S6 Move; the moved encoder tests cannot reach the
// public Device, so the guard is pinned here.
func TestDeviceCreateRenderBundleEncoderNilDescriptor(t *testing.T) {
	command := &captureCommandEncoder{CommandEncoder: &noop.CommandEncoder{}, renderPass: &captureRenderPass{RenderPassEncoder: &noop.RenderPassEncoder{}}}
	device := newCaptureDevice(&captureDevice{Device: &noop.Device{}, commandEncoder: command})
	if _, err := device.CreateRenderBundleEncoder(nil); err == nil {
		t.Fatal("CreateRenderBundleEncoder(nil) succeeded")
	}
}

// TestExecuteBundlesNilBundleReportsCommandError pins the wrapper's nil
// conversion: a nil public bundle reaches the destination as a nil bundle, and
// the pre-move "render bundle is nil" command error comes back through the
// public API. The sentinel is the root alias of the destination's value, so
// errors.Is proves the identity across the boundary.
func TestExecuteBundlesNilBundleReportsCommandError(t *testing.T) {
	command := &captureCommandEncoder{CommandEncoder: &noop.CommandEncoder{}, renderPass: &captureRenderPass{RenderPassEncoder: &noop.RenderPassEncoder{}}}
	device := newCaptureDevice(&captureDevice{Device: &noop.Device{}, commandEncoder: command})
	encoder, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder() error = %v", err)
	}
	pass, err := encoder.BeginRenderPass(&RenderPassDescriptor{})
	if err != nil {
		t.Fatalf("BeginRenderPass() error = %v", err)
	}
	pass.ExecuteBundles(nil)
	_ = pass.End()
	if _, err := encoder.Finish(); !errors.Is(err, errBundleCommand) {
		t.Fatalf("Finish() error = %v, want errBundleCommand", err)
	}
}
