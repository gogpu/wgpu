// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin && !(js && wasm)

package metal

import (
	"errors"
	"testing"

	"github.com/gogpu/wgpu/internal/hal"
)

// These tests pin the budget release of every wiring path that lives in the
// darwin-only files. The accounting loop of Queue.Submit is covered on every
// platform by command_budget_test.go (runSubmit); here we drive the paths that
// hold or drop a command buffer: Submit's adapter, FreeCommandBuffer, ResetAll,
// DiscardEncoding, CommandEncoder.Destroy and the EndEncoding error path.
//
// No Metal device is needed. A command buffer with a zero handle exercises the
// drop paths (Destroy skips Release on a zero handle); the encoder paths use a
// retained NSString as the handle, a valid Objective-C object that Release can
// safely consume — that needs libobjc (Init), not a GPU.

// testObjCRuntime initializes the Objective-C runtime for tests that release a
// real handle. Metal.framework exists on every macOS; no device is created.
func testObjCRuntime(t *testing.T) {
	t.Helper()
	if err := Init(); err != nil {
		t.Fatalf("metal.Init: %v", err)
	}
}

// acquireTestUnit reserves one budget place and returns the unit holding it.
func acquireTestUnit(t *testing.T, b *commandBudget) commandBudgetUnit {
	t.Helper()
	if _, ok := b.TryAcquire(); !ok {
		t.Fatal("TryAcquire = false, want true")
	}
	return newCommandBudgetUnit(b)
}

// TestSubmitAdapterReleasesBudget drives the real Queue.Submit over the real
// adapter: the batch is built from *CommandBuffer values holding budget units,
// commits go to a zero handle (Objective-C calls on 0 are no-ops), and every
// unit must come back.
func TestSubmitAdapterReleasesBudget(t *testing.T) {
	b := newCommandBudget(2)
	q := &Queue{device: &Device{cmdBudget: b}}
	buffers := make([]hal.CommandBuffer, 2)
	for i := range buffers {
		buffers[i] = &CommandBuffer{budgetUnit: acquireTestUnit(t, b)}
	}

	subIdx, err := q.Submit(buffers)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if subIdx != 1 {
		t.Fatalf("Submit subIdx = %d, want 1", subIdx)
	}
	if got := b.Outstanding(); got != 0 {
		t.Fatalf("Outstanding after Submit = %d, want 0", got)
	}
}

// TestFreeCommandBufferReleasesBudget covers the core's post-submit release and
// a command buffer destroyed without ever being committed.
func TestFreeCommandBufferReleasesBudget(t *testing.T) {
	b := newCommandBudget(2)
	d := &Device{cmdBudget: b}

	// Never submitted: FreeCommandBuffer is the drop path.
	d.FreeCommandBuffer(&CommandBuffer{budgetUnit: acquireTestUnit(t, b)})
	if got := b.Outstanding(); got != 0 {
		t.Fatalf("Outstanding after FreeCommandBuffer = %d, want 0", got)
	}

	// The destroy is idempotent: a second call must not release a second unit.
	cb := &CommandBuffer{budgetUnit: acquireTestUnit(t, b)}
	d.FreeCommandBuffer(cb)
	d.FreeCommandBuffer(cb)
	if got := b.Outstanding(); got != 0 {
		t.Fatalf("Outstanding after a double FreeCommandBuffer = %d, want 0", got)
	}
}

// TestResetAllReleasesBudget covers the encoder reuse path, which destroys the
// batch it is given.
func TestResetAllReleasesBudget(t *testing.T) {
	b := newCommandBudget(2)
	enc := &CommandEncoder{device: &Device{cmdBudget: b}}

	enc.ResetAll([]hal.CommandBuffer{&CommandBuffer{budgetUnit: acquireTestUnit(t, b)}})
	if got := b.Outstanding(); got != 0 {
		t.Fatalf("Outstanding after ResetAll = %d, want 0", got)
	}
}

// TestDiscardEncodingReleasesBudget covers a recording that is discarded
// instead of ended.
func TestDiscardEncodingReleasesBudget(t *testing.T) {
	testObjCRuntime(t)
	b := newCommandBudget(1)
	d := &Device{cmdBudget: b}
	enc := &CommandEncoder{device: d, cmdBuffer: NSString("p1-mtl-008 discard")}

	if _, ok := b.TryAcquire(); !ok {
		t.Fatal("TryAcquire = false, want true")
	}
	enc.DiscardEncoding()
	if enc.cmdBuffer != 0 {
		t.Fatalf("cmdBuffer after DiscardEncoding = %d, want 0", enc.cmdBuffer)
	}
	if got := b.Outstanding(); got != 0 {
		t.Fatalf("Outstanding after DiscardEncoding = %d, want 0", got)
	}
}

// TestEncoderDestroyReleasesBudget covers an encoder destroyed while still
// recording (core abandons the encoder without ending it).
func TestEncoderDestroyReleasesBudget(t *testing.T) {
	testObjCRuntime(t)
	b := newCommandBudget(1)
	d := &Device{cmdBudget: b}
	enc := &CommandEncoder{device: d, cmdBuffer: NSString("p1-mtl-008 destroy")}

	if _, ok := b.TryAcquire(); !ok {
		t.Fatal("TryAcquire = false, want true")
	}
	enc.Destroy()
	if enc.cmdBuffer != 0 {
		t.Fatalf("cmdBuffer after Destroy = %d, want 0", enc.cmdBuffer)
	}
	if got := b.Outstanding(); got != 0 {
		t.Fatalf("Outstanding after Destroy = %d, want 0", got)
	}
}

// TestEndEncodingErrorReleasesBudget covers an encoding that fails while
// recording: EndEncoding drops the command buffer and must return the unit.
func TestEndEncodingErrorReleasesBudget(t *testing.T) {
	testObjCRuntime(t)
	b := newCommandBudget(1)
	d := &Device{cmdBudget: b}
	enc := &CommandEncoder{device: d, cmdBuffer: NSString("p1-mtl-008 recording error")}

	if _, ok := b.TryAcquire(); !ok {
		t.Fatal("TryAcquire = false, want true")
	}
	recErr := errors.New("boom")
	enc.failRecording(recErr)
	if _, err := enc.EndEncoding(); !errors.Is(err, recErr) {
		t.Fatalf("EndEncoding error = %v, want the recording error", err)
	}
	if enc.cmdBuffer != 0 {
		t.Fatalf("cmdBuffer after EndEncoding = %d, want 0", enc.cmdBuffer)
	}
	if got := b.Outstanding(); got != 0 {
		t.Fatalf("Outstanding after EndEncoding error = %d, want 0", got)
	}
}

// TestOneShotCommandBufferRefusedAtLimit checks the one-shot path's refusal
// (write staging blits, Present, WaitIdle): an error wrapping
// hal.ErrCommandBufferBudget, no budget consumed by the refusal.
func TestOneShotCommandBufferRefusedAtLimit(t *testing.T) {
	b := newCommandBudget(1)
	if _, ok := b.TryAcquire(); !ok {
		t.Fatal("TryAcquire = false, want true")
	}

	_, err := newBudgetedCommandBuffer(0, b, "Present")
	if !errors.Is(err, hal.ErrCommandBufferBudget) {
		t.Fatalf("newBudgetedCommandBuffer error = %v, want it to wrap hal.ErrCommandBufferBudget", err)
	}
	if got := b.Outstanding(); got != 1 {
		t.Fatalf("Outstanding after a refused one-shot = %d, want 1", got)
	}
}

// TestOneShotCommandBufferCreationFailureReleases checks that a one-shot whose
// command buffer could not be created (a zero queue stands in for the failure)
// returns its unit.
func TestOneShotCommandBufferCreationFailureReleases(t *testing.T) {
	b := newCommandBudget(1)

	_, err := newBudgetedCommandBuffer(0, b, "WriteTexture")
	if err == nil {
		t.Fatal("newBudgetedCommandBuffer with a zero queue = nil error, want failure")
	}
	if errors.Is(err, hal.ErrCommandBufferBudget) {
		t.Fatalf("creation failure reported as a budget refusal: %v", err)
	}
	if got := b.Outstanding(); got != 0 {
		t.Fatalf("Outstanding after a failed one-shot creation = %d, want 0", got)
	}
}
