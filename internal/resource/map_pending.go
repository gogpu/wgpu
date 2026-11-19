//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package resource

import (
	"context"
	"sync/atomic"

	"github.com/gogpu/wgpu/internal/core"
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
//
// The public wrapper (wgpu.MapPending) is cached on the object itself (Pub),
// so a steady-state MapAsync cycle allocates neither the handle nor its
// wrapper.
type MapPending struct {
	buf      *Buffer
	waiter   *core.MapWaiter
	resolved bool
	err      error

	// Pub caches the public wrapper built over this pooled object, so
	// steady-state MapAsync reuses one wrapper instead of allocating one per
	// call (ADR-070 S1 F1).
	//
	// Synchronization: sync.Pool's Get/Put is the happens-before edge. The
	// slot is written once per pooled object, by the goroutine that acquires
	// it, and read back by the next owner after the pool handover; an object
	// has exactly one owner between Get and Put, so a plain field is enough
	// and no atomic is needed. release leaves it set — see the note there.
	Pub any

	// released is the CAS guard that admits exactly one return to the pool:
	// concurrent Release calls see CompareAndSwap succeed for one caller only.
	// It is nil only on an unguarded zero value, whose Release is a no-op. It
	// is re-armed by acquireMapPending.
	released *atomic.Bool
}

var mapPendingPool = newHandlePool[*MapPending](func() any {
	return &MapPending{released: new(atomic.Bool)}
})

func acquireMapPending(buf *Buffer, waiter *core.MapWaiter) *MapPending {
	p := mapPendingPool.Get()
	if p.released == nil {
		p.released = new(atomic.Bool)
	}
	p.released.Store(false)
	p.buf = buf
	p.waiter = waiter
	p.resolved = false
	p.err = nil
	return p
}

func (p *MapPending) release() {
	if p == nil || p.released == nil || !p.released.CompareAndSwap(false, true) {
		return
	}
	// Only the per-mapping fields are cleared. Pub is deliberately left set:
	// the pooled object and its cached public wrapper are reused together, so
	// zeroing the struct on the Put path would drop the wrapper and bring the
	// per-call allocation back (ADR-070 S1 F1).
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
// Status never returns the object to the pool. The handle returns to the
// pool only via Release; until then subsequent Status calls are safe and
// return the cached (true, err) pair once the mapping has resolved.
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
