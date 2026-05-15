//go:build js && wasm

package wgpu

import "context"

// MapPending is a handle to an in-flight Buffer.MapAsync on the browser.
//
// The browser backend uses a goroutine + channel pattern: MapAsync spawns
// a goroutine that calls AwaitPromise on the GPUBuffer.mapAsync JS Promise.
// The result (nil on success, error on rejection) is sent to the done channel.
//
// This differs from the native backend (which uses core.MapWaiter + Device.Poll)
// because browser WebGPU resolves promises via the JS event loop, not via
// explicit GPU polling.
type MapPending struct {
	buf      *Buffer
	done     chan error
	resolved bool
	err      error
}

// Status returns the current state of the pending map without blocking.
//
//   - (true, nil)    -- mapping is ready; call Buffer.MappedRange.
//   - (false, nil)   -- still pending; the JS event loop has not resolved
//     the Promise yet.
//   - (true, err)    -- mapping failed; the buffer is back in the Unmapped
//     state and err describes why.
func (p *MapPending) Status() (ready bool, err error) {
	if p == nil || p.done == nil {
		return true, ErrMapCanceled
	}
	if p.resolved {
		return true, p.err
	}
	// Non-blocking check on the channel.
	select {
	case err := <-p.done:
		p.resolved = true
		p.err = err
		return true, p.err
	default:
		return false, nil
	}
}

// Wait blocks until the pending map resolves or ctx is canceled.
//
// If ctx is canceled before the JS Promise resolves, Wait returns
// ctx.Err(). The mapping remains Pending -- the caller should normally
// follow up with Buffer.Unmap to cancel it.
func (p *MapPending) Wait(ctx context.Context) error {
	if p == nil || p.done == nil {
		return ErrMapCanceled
	}
	if p.resolved {
		return p.err
	}
	select {
	case err := <-p.done:
		p.resolved = true
		p.err = err
		return p.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Release discards the MapPending handle. Calling this while the map is
// still in flight is allowed; the JS Promise continues to resolve but its
// completion is not observable through this handle anymore.
func (p *MapPending) Release() {
	if p == nil {
		return
	}
	p.buf = nil
	p.done = nil
	p.resolved = false
	p.err = nil
}
