//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu_test

import (
	"runtime"
	"testing"

	"github.com/gogpu/wgpu"
	"github.com/gogpu/wgpu/wgputest"
)

// benchmarkSubmitBatch is how many empty command buffers BenchmarkQueueSubmit
// encodes per refill. A command buffer is submitted at most once, so every
// iteration needs its own; the batch keeps that setup bounded in memory while
// staying outside the timer.
const benchmarkSubmitBatch = 1024

// benchSubmitIndex keeps the submission index live so the compiler cannot
// discard the measured Submit.
var benchSubmitIndex uint64

// BenchmarkQueueSubmit measures one public Queue.Submit call: the queue lock,
// the pending-write flush, submission assembly, the HAL submit, and the
// post-submit bookkeeping (resource tracking, encoder recycling, destroy-queue
// triage). It is the baseline of ADR-070 §15.7's perf protocol: Phase B puts
// the queue behind a consumer-defined interface, and this number must come
// back within the protocol's tolerance (ns ±5%, allocs delta 0) against the
// baseline recorded before B's first move.
//
// Hermetic by construction: the device is the noop backend, so no GPU, driver
// or window is needed and the run is the same on any machine. The numbers are
// the frontend's own cost with no driver work in them, which is exactly the
// code Phase B moves. The earlier "Submit allocs unchanged (13/13)" reading
// (v0.34.15 review) was a one-off on a real adapter, not a committed
// benchmark, so it is a different measurement, not a number to match.
//
// Each iteration submits its own empty command buffer. A command buffer is
// submitted at most once — the WebGPU spec requires it and
// validateCommandBufferForSubmit enforces it (cb.submitted) — so one buffer
// cannot be reused the way the HAL-level BenchmarkHALSubmitOverhead reuses
// one. Encoding is setup: the batch is filled with the timer stopped, so
// neither its time nor its allocations reach ns/op or allocs/op.
func BenchmarkQueueSubmit(b *testing.B) {
	b.ReportAllocs()

	device, _, cleanup, err := wgputest.NewNoopDevice()
	if err != nil {
		b.Fatalf("wgputest.NewNoopDevice: %v", err)
	}
	defer cleanup()

	queue := device.Queue()
	if queue == nil {
		b.Fatal("device.Queue() returned nil")
	}

	batch := make([]*wgpu.CommandBuffer, benchmarkSubmitBatch)
	next := len(batch) // forces a refill on the first iteration

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if next == len(batch) {
			b.StopTimer()
			fillSubmitBatch(b, device, batch)
			next = 0
			b.StartTimer()
		}
		index, err := queue.Submit(batch[next])
		if err != nil {
			b.Fatalf("Submit: %v", err)
		}
		benchSubmitIndex = index
		next++
	}

	runtime.KeepAlive(batch)
}

// fillSubmitBatch replaces every entry of batch with a freshly encoded empty
// command buffer. The spent buffers the batch held are dropped here, so at
// most one batch is live at a time.
func fillSubmitBatch(b *testing.B, device *wgpu.Device, batch []*wgpu.CommandBuffer) {
	b.Helper()
	for i := range batch {
		batch[i] = nil
	}
	for i := range batch {
		enc, err := device.CreateCommandEncoder(nil)
		if err != nil {
			b.Fatalf("CreateCommandEncoder: %v", err)
		}
		cmd, err := enc.Finish()
		if err != nil {
			b.Fatalf("Finish: %v", err)
		}
		batch[i] = cmd
	}
}
