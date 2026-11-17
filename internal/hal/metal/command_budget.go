// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package metal

import "sync/atomic"

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
