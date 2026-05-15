//go:build js && wasm

package wgpu

import (
	"context"
	"fmt"

	"github.com/gogpu/wgpu/internal/browser"
)

// Buffer represents a GPU buffer.
type Buffer struct {
	browser  *browser.Buffer
	size     uint64
	usage    BufferUsage
	released bool

	// mapState tracks the current mapping state. Updated by MapAsync/Unmap.
	mapState MapState
}

// Size returns the buffer size in bytes.
func (b *Buffer) Size() uint64 {
	return b.size
}

// Usage returns the buffer's usage flags.
func (b *Buffer) Usage() BufferUsage {
	return b.usage
}

// Label returns the buffer's debug label.
// Browser WebGPU does not expose label on the object; returns empty string.
func (b *Buffer) Label() string {
	return ""
}

// Release destroys the buffer.
func (b *Buffer) Release() {
	if b.released {
		return
	}
	b.released = true
	b.mapState = MapStateDestroyed
	if b.browser != nil {
		b.browser.Destroy()
	}
}

// MapState returns the current mapping state of the buffer.
func (b *Buffer) MapState() MapState {
	if b == nil || b.released {
		return MapStateUnmapped
	}
	return b.mapState
}

// mapModeToJS converts a MapMode to the WebGPU GPUMapMode flags.
// Our MapMode values already match WebGPU spec: MapModeRead=1, MapModeWrite=2.
// This function validates and converts explicitly for clarity.
func mapModeToJS(mode MapMode) uint32 {
	var jsMode uint32
	if mode&MapModeRead != 0 {
		jsMode |= 1 // GPUMapMode.READ
	}
	if mode&MapModeWrite != 0 {
		jsMode |= 2 // GPUMapMode.WRITE
	}
	return jsMode
}

// Map blocks until a CPU-visible mapping is established for the given
// byte range, or until ctx is canceled.
//
// The buffer must have been created with BufferUsageMapRead or
// BufferUsageMapWrite matching mode. offset must be a multiple of 8 and
// size must be a multiple of 4 (WebGPU MAP_ALIGNMENT).
//
// After Map succeeds, call MappedRange to obtain a byte view and Unmap
// when finished:
//
//	if err := buf.Map(ctx, wgpu.MapModeRead, 0, size); err != nil {
//	    return err
//	}
//	defer buf.Unmap()
//	rng, _ := buf.MappedRange(0, size)
//	data := rng.Bytes()
//
// On the browser, Map spawns a goroutine that calls GPUBuffer.mapAsync
// (a JS Promise) via AwaitPromise. The ctx parameter allows cancellation.
func (b *Buffer) Map(ctx context.Context, mode MapMode, offset, size uint64) error {
	if b == nil || b.browser == nil {
		return ErrReleased
	}
	if b.released {
		return ErrBufferDestroyed
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateMapArgs(b, mode, offset, size); err != nil {
		return err
	}

	// MapAsync sets the state to pending, then blocks on the JS Promise.
	// We use a channel + goroutine to support context cancellation.
	pending, err := b.MapAsync(mode, offset, size)
	if err != nil {
		return err
	}
	return pending.Wait(ctx)
}

// MapAsync initiates a buffer map without blocking the caller.
//
// Returns a *MapPending handle that resolves once the browser's
// GPUBuffer.mapAsync Promise settles. The caller can poll with
// Status() or block with Wait(ctx).
//
// Validation errors (alignment, usage mismatch, range overflow,
// already-mapped state) surface synchronously.
func (b *Buffer) MapAsync(mode MapMode, offset, size uint64) (*MapPending, error) {
	if b == nil || b.browser == nil {
		return nil, ErrReleased
	}
	if b.released {
		return nil, ErrBufferDestroyed
	}
	if err := validateMapArgs(b, mode, offset, size); err != nil {
		return nil, err
	}

	b.mapState = MapStatePending
	jsMode := mapModeToJS(mode)

	// Create a MapPending that will resolve asynchronously.
	p := &MapPending{
		buf:  b,
		done: make(chan error, 1),
	}

	// Launch a goroutine to await the JS Promise. AwaitPromise blocks the
	// goroutine (not the main thread) until the Promise settles.
	go func() {
		err := b.browser.MapAsync(jsMode, offset, size)
		if err != nil {
			b.mapState = MapStateUnmapped
			p.done <- fmt.Errorf("mapAsync: %w", err)
		} else {
			b.mapState = MapStateMapped
			p.done <- nil
		}
	}()

	return p, nil
}

// MappedRange returns a safe view over the mapped region [offset, offset+size).
//
// The buffer must be in the Mapped state (Map or MapAsync resolved, or the
// buffer was created with MappedAtCreation: true). The returned MappedRange
// is invalidated by Unmap.
func (b *Buffer) MappedRange(offset, size uint64) (*MappedRange, error) {
	if b == nil || b.browser == nil {
		return nil, ErrReleased
	}
	if b.released {
		return nil, ErrBufferDestroyed
	}
	if b.mapState != MapStateMapped {
		return nil, ErrMapNotMapped
	}
	if offset%8 != 0 || size%4 != 0 {
		return nil, ErrMapAlignment
	}
	if offset+size > b.size {
		return nil, ErrMapRangeOverflow
	}

	return &MappedRange{
		buf:    b,
		offset: offset,
		size:   size,
		valid:  true,
	}, nil
}

// Unmap releases the current mapping and invalidates all outstanding
// MappedRange handles. Safe to call multiple times; a second call returns
// ErrMapNotMapped.
func (b *Buffer) Unmap() error {
	if b == nil || b.browser == nil {
		return ErrReleased
	}
	if b.released {
		return ErrBufferDestroyed
	}
	if b.mapState != MapStateMapped && b.mapState != MapStatePending {
		return ErrMapNotMapped
	}
	b.browser.Unmap()
	b.mapState = MapStateUnmapped
	return nil
}

// validateMapArgs performs synchronous validation of mapping parameters.
func validateMapArgs(b *Buffer, mode MapMode, offset, size uint64) error {
	if b.mapState == MapStatePending {
		return ErrMapAlreadyPending
	}
	if b.mapState == MapStateMapped {
		return ErrMapAlreadyMapped
	}
	if b.mapState == MapStateDestroyed {
		return ErrBufferDestroyed
	}

	// Check usage flags.
	if mode&MapModeRead != 0 && b.usage&BufferUsageMapRead == 0 {
		return ErrMapInvalidMode
	}
	if mode&MapModeWrite != 0 && b.usage&BufferUsageMapWrite == 0 {
		return ErrMapInvalidMode
	}

	// Alignment: offset must be multiple of 8, size must be multiple of 4.
	if offset%8 != 0 || size%4 != 0 {
		return ErrMapAlignment
	}

	// Range overflow.
	if offset+size > b.size {
		return ErrMapRangeOverflow
	}

	return nil
}
