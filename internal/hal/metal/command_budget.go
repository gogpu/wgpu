// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package metal

import (
	"sync"
	"sync/atomic"
)

// commandBudget counts the MTLCommandBuffers created through
// CommandEncoder.BeginEncoding that have neither been submitted nor dropped.
//
// Apple limits a command queue to maxCommandBufferCount *uncompleted* command
// buffers; the documented default for a queue created with -newCommandQueue is
// 64. Once the limit is reached, [MTLCommandQueue commandBuffer] does not fail:
// it blocks the calling CPU thread until one of the outstanding buffers
// completes. A core that records every render pass in its own command buffer
// and commits them only at Submit therefore deadlocks on the 65th — nothing can
// complete before a submit that cannot be reached (issue #428). The device
// opens its queue with MAX_COMMAND_BUFFERS (Rust wgpu-hal parity,
// wgpu-hal/src/metal/adapter.rs MAX_COMMAND_BUFFERS) and this counter refuses
// the command buffer that would block.
//
// The counter counts CREATED-NOT-SUBMITTED buffers, a subset of Apple's
// uncompleted count: a buffer submitted in an earlier call is still uncompleted
// (and still counts for Apple) while it no longer counts here. Refusing at this
// limit is therefore a deliberate lower-bound fail-fast: it fires no later than
// Metal would block, and it never stands in for the submitted-pending
// backpressure that the frame semaphore handles.
//
// The type is pure accounting — no Objective-C, no platform dependency — so it
// is built on every platform and its unit tests run without Metal hardware.
type commandBudget struct {
	limit       int64
	outstanding atomic.Int64
}

// newCommandBudget returns a budget that admits limit command buffers that are
// created and not yet submitted or dropped at the same time.
func newCommandBudget(limit int) *commandBudget {
	return &commandBudget{limit: int64(limit)}
}

// TryAcquire reserves one command buffer of the budget. It reports ok=false,
// without consuming budget, when the limit is already reached; outstanding is
// then the current count, for the caller's diagnostic.
//
// A refused attempt stays uncounted on purpose: the failure is operation-level
// (the encoder cannot start another recording), not a device loss, so
// submitting or dropping an outstanding command buffer must free the budget
// for the next attempt.
func (b *commandBudget) TryAcquire() (outstanding int64, ok bool) {
	for {
		cur := b.outstanding.Load()
		if cur >= b.limit {
			return cur, false
		}
		if b.outstanding.CompareAndSwap(cur, cur+1) {
			return cur + 1, true
		}
	}
}

// Release returns one command buffer to the budget. It is a no-op on an
// already-empty budget: a release without a matching acquire is a caller bug,
// and saturating at zero keeps the count from underflowing — an underflowed
// count would silently admit more than limit buffers.
func (b *commandBudget) Release() {
	for {
		cur := b.outstanding.Load()
		if cur <= 0 {
			return
		}
		if b.outstanding.CompareAndSwap(cur, cur-1) {
			return
		}
	}
}

// Outstanding returns the number of command buffers created and not yet
// submitted or dropped. It serves the guard's WARN and the unit tests.
func (b *commandBudget) Outstanding() int64 {
	return b.outstanding.Load()
}

// commandBudgetUnit is the accounting unit of one created command buffer: it
// holds the buffer's place in the command budget and returns that place
// exactly once, whichever comes first — the commit of a Submit batch or the
// drop of the buffer without committing. A second return would under-count the
// outstanding buffers and let more than the limit through, which is why the
// unit, and not the call sites, owns the sync.Once.
//
// The zero value (a nil budget) is a unit whose release is a no-op; device-free
// tests construct command buffers without a Metal device.
type commandBudgetUnit struct {
	budget *commandBudget
	once   sync.Once
}

// newCommandBudgetUnit returns the unit of a command buffer created against b.
func newCommandBudgetUnit(b *commandBudget) commandBudgetUnit {
	return commandBudgetUnit{budget: b}
}

// release returns the unit's place to the budget, once.
func (u *commandBudgetUnit) release() {
	if u == nil || u.budget == nil {
		return
	}
	u.once.Do(u.budget.Release)
}

// budgetedSubmit is one command buffer of a Submit batch as the accounting
// loop sees it: the raw handle the hooks pass to Objective-C, and the unit the
// commit returns. A zero value (unit == nil) marks a slot of the caller's list
// that held no Metal command buffer; the loop skips it, and a last slot that is
// zero receives no completion registration, as before.
type budgetedSubmit struct {
	raw  uintptr
	unit *commandBudgetUnit
}

// submitHooks carries the Objective-C effects of a submission, so the
// accounting loop is platform-neutral and unit-testable on every platform:
// onLast registers the batch's completion tracking on its last command buffer,
// and commit sends -commit to one buffer.
type submitHooks struct {
	onLast func(raw uintptr, subIdx uint64)
	commit func(raw uintptr)
}

// runSubmit runs the per-buffer part of Queue.Submit: it registers completion
// tracking on the batch's last Metal command buffer, commits every buffer, and
// returns each buffer's unit to the command budget at that instant.
//
// This is the decrement the budget depends on: a unit returned here (or by
// commandBudgetUnit.release from a drop path) frees the place for the next
// recording. It lives outside the darwin-only files so the accounting is
// pinned by tests that run on every platform.
func runSubmit(batch []budgetedSubmit, hooks submitHooks, subIdx uint64) {
	lastIdx := len(batch) - 1
	for i := range batch {
		if batch[i].unit == nil {
			continue
		}
		if i == lastIdx {
			hooks.onLast(batch[i].raw, subIdx)
		}
		hooks.commit(batch[i].raw)
		batch[i].unit.release()
	}
}
