//go:build !rust && !(js && wasm)

package wgpu

import (
	"testing"

	"github.com/gogpu/wgpu/internal/hal/noop"
)

// The zero-value contract of the wrapped encoder types (ADR-070 §6, S4). The
// pre-move types were structs whose zero value had no core state: every
// recording method dereferenced the nil core and panicked, and
// CommandBuffer.Release was a no-op. The moved implementation keeps that
// contract across the declaration layer; these tests pin it.

// TestCommandEncoderZeroValuePanics checks every public recording method of a
// zero-value CommandEncoder: each panics on the missing implementation, as the
// pre-move zero value panicked on its nil core.
func TestCommandEncoderZeroValuePanics(t *testing.T) {
	tests := []struct {
		name string
		call func(e *CommandEncoder)
	}{
		{"Finish", func(e *CommandEncoder) { _, _ = e.Finish() }},
		{"DiscardEncoding", func(e *CommandEncoder) { e.DiscardEncoding() }},
		{"BeginRenderPass", func(e *CommandEncoder) { _, _ = e.BeginRenderPass(nil) }},
		{"BeginComputePass", func(e *CommandEncoder) { _, _ = e.BeginComputePass(nil) }},
		{"CopyBufferToBuffer", func(e *CommandEncoder) { e.CopyBufferToBuffer(nil, 0, nil, 0, 0) }},
		{"CopyTextureToBuffer", func(e *CommandEncoder) { e.CopyTextureToBuffer(nil, nil, nil) }},
		{"CopyTextureToTexture", func(e *CommandEncoder) { e.CopyTextureToTexture(nil, nil, nil) }},
		{"CopyBufferToTexture", func(e *CommandEncoder) { e.CopyBufferToTexture(nil, nil, nil) }},
		{"ClearBuffer", func(e *CommandEncoder) { e.ClearBuffer(nil, 0, 0) }},
		{"ResolveQuerySet", func(e *CommandEncoder) { e.ResolveQuerySet(nil, 0, 0, nil, 0) }},
		{"TransitionTextures", func(e *CommandEncoder) { e.TransitionTextures(nil) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatalf("%s on a zero-value CommandEncoder did not panic", test.name)
				}
			}()
			var zero CommandEncoder
			test.call(&zero)
		})
	}
}

// TestCommandEncoderNilReceiverPanics checks the nil receiver, which panicked
// before the move on the first field read.
func TestCommandEncoderNilReceiverPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("Finish on a nil *CommandEncoder did not panic")
		}
	}()
	var encoder *CommandEncoder
	_, _ = encoder.Finish()
}

// TestCommandBufferZeroValueReleaseIsNoop checks that a zero-value command
// buffer releases nothing and does not panic.
func TestCommandBufferZeroValueReleaseIsNoop(t *testing.T) {
	var zero CommandBuffer
	zero.Release()
}

// TestCommandBufferNilReleaseIsNoop checks the nil receiver of Release, which
// returned early before the move.
func TestCommandBufferNilReleaseIsNoop(t *testing.T) {
	var cb *CommandBuffer
	cb.Release()
}

// TestEncoderDeviceHostPoolIsUntypedNil pins the seam's nil contract: a
// device without a command encoder pool must yield an untyped nil from
// EncoderPool, so the encoder's nil checks (returnEncoderToPool,
// CommandBuffer.Release) keep their meaning across the interface. A typed nil
// boxed in the interface would compare non-nil and be called.
func TestEncoderDeviceHostPoolIsUntypedNil(t *testing.T) {
	device := NewBareDeviceForTest()
	if pool := device.impl.EncoderPool(); pool != nil {
		t.Fatalf("a device without a pool must give an untyped nil, got %T", pool)
	}
}

// TestQueueSubmitAcceptsZeroValueCommandBuffer pins the pre-move behavior of
// a zero-value command buffer handed to Submit: it carries no commands, so the
// submit path accepts and spends it instead of reporting a nil element. A nil
// *CommandBuffer stays a nil element and is reported.
func TestQueueSubmitAcceptsZeroValueCommandBuffer(t *testing.T) {
	queue := newQueue(&noop.Queue{}, nil)

	var zero CommandBuffer
	if _, err := queue.Submit(&zero); err != nil {
		t.Fatalf("Submit(zero-value CommandBuffer) = %v, want nil", err)
	}
	if _, err := queue.Submit(nil); err == nil {
		t.Fatal("Submit(nil CommandBuffer) succeeded, want a nil-element error")
	}
}
