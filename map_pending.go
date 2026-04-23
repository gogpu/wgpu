//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu

import (
	"context"
	"sync"

	"github.com/gogpu/wgpu/core"
)

// MapPending is a zero-allocation handle to an in-flight Buffer.MapAsync.
//
// It is allocated from a sync.Pool and must be released to the pool by
// calling either Wait, Status (until ready=true), or Release — failing
// to release simply wastes a pool entry until the next GC, but does not
// leak memory. The handle becomes invalid once the underlying map has
// resolved; subsequent Status/Wait calls are idempotent and return the
// cached result.
//
// MapPending is not safe to share between goroutines without external
// synchronization; the typical pattern is "create on one goroutine,
// wait on another".
type MapPending struct {
	buf      *Buffer
	waiter   *core.MapWaiter
	resolved bool
	err      error
}

var mapPendingPool = sync.Pool{
	New: func() any { return &MapPending{} },
}

func acquireMapPending(buf *Buffer, waiter *core.MapWaiter) *MapPending {
	p := mapPendingPool.Get().(*MapPending)
	p.buf = buf
	p.waiter = waiter
	p.resolved = false
	p.err = nil
	return p
}

func (p *MapPending) release() {
	if p == nil {
		return
	}
	p.buf = nil
	p.waiter = nil
	p.resolved = false
	p.err = nil
	mapPendingPool.Put(p)
}

// Status reports the current state of the pending map without blocking.
//
//   - (true, nil)    — mapping is ready; call Buffer.MappedRange.
//   - (false, nil)   — still pending; caller should continue work and
//     call Device.Poll(PollPoll) in a later tick.
//   - (true, err)    — mapping failed; the buffer is back in the Unmapped
//     state and err describes why.
//
// After Status returns ready=true the MapPending is automatically
// returned to the pool; subsequent calls are safe but always return the
// cached (true, err) pair.
func (p *MapPending) Status() (ready bool, err error) {
	if p == nil || p.waiter == nil {
		return true, ErrMapCanceled
	}
	if p.resolved {
		return true, p.err
	}
	done, cErr := p.waiter.Status()
	if !done {
		return false, nil
	}
	p.resolved = true
	p.err = coreErrToTyped(cErr)
	return true, p.err
}

// Wait blocks until the pending map resolves or ctx is canceled.
//
// If ctx is canceled before the GPU resolves the map, Wait returns
// ctx.Err() and the mapping remains Pending — the caller should normally
// follow up with Buffer.Unmap to cancel it, or retry Wait later.
func (p *MapPending) Wait(ctx context.Context) error {
	if p == nil || p.waiter == nil {
		return ErrMapCanceled
	}
	if p.resolved {
		return p.err
	}
	// Fast path — already signaled.
	if done, cErr := p.waiter.Status(); done {
		p.resolved = true
		p.err = coreErrToTyped(cErr)
		return p.err
	}
	// Start a goroutine to observe waiter.Wait() and post the result via
	// a buffered channel; select on ctx.Done. The channel is the only
	// per-Wait allocation; MapAsync's zero-alloc guarantee applies to
	// the hot path that polls Status() directly.
	doneCh := make(chan *core.BufferMapError, 1)
	go func() { doneCh <- p.waiter.Wait() }()

	select {
	case cErr := <-doneCh:
		p.resolved = true
		p.err = coreErrToTyped(cErr)
		return p.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Release returns the MapPending to the pool without waiting. Calling
// this while the map is still in flight is allowed; the pending map
// continues to resolve inside Device.Poll but its completion is not
// observable through this handle anymore.
func (p *MapPending) Release() { p.release() }
