//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu_test

import (
	"testing"

	"github.com/gogpu/wgpu"
	"github.com/gogpu/wgpu/wgputest"
)

// benchmarkFinishBatch is how many command encoders BenchmarkEncoderFinish
// finishes per refill. An encoder is finished at most once, so every
// iteration needs its own; the batch keeps that setup bounded in memory while
// staying outside the timer.
const benchmarkFinishBatch = 1024

// benchFinishCommandBuffer keeps the produced command buffer live so the
// compiler cannot discard the measured Finish.
var benchFinishCommandBuffer *wgpu.CommandBuffer

// BenchmarkEncoderFinish measures one public CommandEncoder.Finish call: the
// encoder's state transition, the recorded command stream's finalisation and
// the command buffer's construction. It completes the committed coverage of
// ADR-070 §15.7's three hot paths — Submit (BenchmarkQueueSubmit) and Present
// (BenchmarkSurfacePresent) already existed; Finish did not, and the S4
// record's "Finish" A/B could only have been QueueSubmit. The gap is recorded
// honestly in the S13 record: this benchmark is the first committed number
// for the path, so it has no pre-move side and is non-gating.
//
// Hermetic by construction: the device is the noop backend, so no GPU, driver
// or window is needed and the run is the same on any machine. Encoding is
// setup: the batch is filled with the timer stopped, so neither its time nor
// its allocations reach ns/op or allocs/op.
func BenchmarkEncoderFinish(b *testing.B) {
	b.ReportAllocs()

	device, _, cleanup, err := wgputest.NewNoopDevice()
	if err != nil {
		b.Fatalf("wgputest.NewNoopDevice: %v", err)
	}
	defer cleanup()

	batch := make([]*wgpu.CommandEncoder, benchmarkFinishBatch)
	next := len(batch) // forces a refill on the first iteration

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if next == len(batch) {
			b.StopTimer()
			fillFinishBatch(b, device, batch)
			next = 0
			b.StartTimer()
		}
		cmd, err := batch[next].Finish()
		if err != nil {
			b.Fatalf("Finish: %v", err)
		}
		benchFinishCommandBuffer = cmd
		batch[next] = nil // finished encoders are spent; drop them eagerly
		next++
	}
}

// fillFinishBatch replaces every entry of batch with a freshly created
// command encoder. The finished encoders the batch held are already dropped,
// so at most one batch is live at a time.
func fillFinishBatch(b *testing.B, device *wgpu.Device, batch []*wgpu.CommandEncoder) {
	b.Helper()
	for i := range batch {
		enc, err := device.CreateCommandEncoder(nil)
		if err != nil {
			b.Fatalf("CreateCommandEncoder: %v", err)
		}
		batch[i] = enc
	}
}
