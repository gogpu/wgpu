//go:build !rust && !(js && wasm)

package wgputest_test

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
	"github.com/gogpu/wgpu/wgputest"
)

// TestRecorderCounts pins the totals: they start at zero, follow the accepted
// calls, always agree with the Trace lengths, and Reset clears them.
func TestRecorderCounts(t *testing.T) {
	recorder := wgputest.NewRecorder()
	device, queue, cleanup, err := wgputest.NewNoopDevice(wgputest.WithRecorder(recorder))
	if err != nil {
		t.Fatalf("NewNoopDevice: %v", err)
	}
	defer cleanup()

	if got := recorder.Counts(); got != (wgputest.Counts{}) {
		t.Fatalf("Counts before any call = %+v, want zero", got)
	}

	buffer, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "counts",
		Size:  16,
		Usage: gputypes.BufferUsageCopySrc,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer buffer.Release()

	encoder, err := device.CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{Label: "counts"})
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	commandBuffer, err := encoder.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if _, err := queue.Submit(commandBuffer); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	want := wgputest.Counts{Encoders: 1, Buffers: 1, Submits: 1}
	if got := recorder.Counts(); got != want {
		t.Fatalf("Counts = %+v, want %+v", got, want)
	}

	// Counts is a view of the same data as Trace: they can never disagree.
	trace := recorder.Trace()
	got := recorder.Counts()
	if got.Encoders != len(trace.Encoders) ||
		got.Buffers != len(trace.Buffers) ||
		got.Textures != len(trace.Textures) ||
		got.TextureViews != len(trace.TextureViews) ||
		got.VertexBuffers != len(trace.VertexBuffers) ||
		got.IndexBuffers != len(trace.IndexBuffers) ||
		got.BindGroups != len(trace.BindGroups) ||
		got.Passes != len(trace.Passes) ||
		got.Draws != len(trace.Draws) ||
		got.DrawsIndexed != len(trace.DrawsIndexed) ||
		got.Submits != len(trace.Submits) {
		t.Fatalf("Counts %+v disagrees with Trace lengths %+v", got, trace)
	}

	recorder.Reset()
	if got := recorder.Counts(); got != (wgputest.Counts{}) {
		t.Fatalf("Counts after Reset = %+v, want zero", got)
	}
}
