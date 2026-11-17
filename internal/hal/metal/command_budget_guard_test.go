// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin && !(js && wasm)

package metal

import (
	"errors"
	"strings"
	"testing"

	"github.com/gogpu/wgpu/internal/hal"
)

// TestBeginEncodingRefusesWhenBudgetExhausted pins the fail-fast guard: at the
// limit BeginEncoding returns an error wrapping hal.ErrCommandBufferBudget,
// starts no recording, and does not consume budget for the refused attempt. The
// test needs no Metal device — the guard runs before any Objective-C call.
func TestBeginEncodingRefusesWhenBudgetExhausted(t *testing.T) {
	budget := newCommandBudget(1)
	if _, ok := budget.TryAcquire(); !ok {
		t.Fatal("filling the budget failed")
	}
	enc := &CommandEncoder{device: &Device{cmdBudget: budget}}

	err := enc.BeginEncoding("refused")
	if err == nil {
		t.Fatal("BeginEncoding at the budget limit = nil error, want refusal")
	}
	if !errors.Is(err, hal.ErrCommandBufferBudget) {
		t.Fatalf("BeginEncoding error = %v, want it to wrap hal.ErrCommandBufferBudget", err)
	}
	if !strings.Contains(err.Error(), "command buffer budget exhausted") {
		t.Fatalf("BeginEncoding error = %q, want it to name the exhausted command buffer budget", err)
	}
	if enc.cmdBuffer != 0 {
		t.Fatalf("refused BeginEncoding left cmdBuffer = %d, want 0", enc.cmdBuffer)
	}
	if got := budget.Outstanding(); got != 1 {
		t.Fatalf("outstanding after refused BeginEncoding = %d, want 1 (a refusal must not consume budget)", got)
	}
}

// TestBeginEncodingFailureReleasesBudget checks the release on the path where
// the encoder acquired a unit but the queue produced no command buffer (a zero
// queue stands in for the failure): the budget must return to its previous
// count, so a later encoder can still record.
func TestBeginEncodingFailureReleasesBudget(t *testing.T) {
	budget := newCommandBudget(2)
	enc := &CommandEncoder{device: &Device{cmdBudget: budget}}

	if err := enc.BeginEncoding("no-queue"); err == nil {
		t.Fatal("BeginEncoding without a command queue = nil error, want failure")
	}
	if got := budget.Outstanding(); got != 0 {
		t.Fatalf("outstanding after a failed command buffer creation = %d, want 0", got)
	}

	if _, ok := budget.TryAcquire(); !ok {
		t.Fatal("budget was not released after the failed command buffer creation")
	}
}
