//go:build js && wasm

package resource

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/browser"
)

// Buffer represents a GPU buffer.
type Buffer struct {
	browser *browser.Buffer
	size    uint64
	usage   gputypes.BufferUsage
	// released is the CAS guard that admits exactly one destroy: concurrent
	// Release calls see CompareAndSwap succeed for one caller only. It is nil
	// only on an unguarded zero value, whose Release is a no-op.
	released *atomic.Bool

	// mapState tracks the current mapping state. Updated by MapAsync/Unmap.
	mapState MapState
}

// NewBufferBrowser builds a Buffer over a browser GPUBuffer handle. size and
// usage are cached for validation and reporting; mapState seeds the mapping
// state (MapStateMapped when the buffer was created MappedAtCreation).
func NewBufferBrowser(browserBuf *browser.Buffer, size uint64, usage gputypes.BufferUsage, mapState MapState) *Buffer {
	return &Buffer{browser: browserBuf, size: size, usage: usage, mapState: mapState, released: new(atomic.Bool)}
}

// Browser returns the underlying browser GPUBuffer handle.
func (b *Buffer) Browser() *browser.Buffer {
	if b == nil {
		return nil
	}
	return b.browser
}

// Size returns the buffer size in bytes.
func (b *Buffer) Size() uint64 {
	return b.size
}

// Usage returns the buffer's usage flags.
func (b *Buffer) Usage() gputypes.BufferUsage {
	return b.usage
}

// Label returns the buffer's debug label.
// Browser WebGPU does not expose label on the object; returns empty string.
func (b *Buffer) Label() string {
	return ""
}

// Release destroys the buffer.
func (b *Buffer) Release() {
	if b.released == nil || !b.released.CompareAndSwap(false, true) {
		return
	}
	b.mapState = MapStateDestroyed
	if b.browser != nil {
		b.browser.Destroy()
	}
}

// IsReleased reports whether Release has run on the buffer.
func (b *Buffer) IsReleased() bool {
	return b != nil && b.released != nil && b.released.Load()
}

// MapState returns the current mapping state of the buffer.
func (b *Buffer) MapState() MapState {
	if b == nil || b.IsReleased() {
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
	if b.IsReleased() {
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
	if b.IsReleased() {
		return nil, ErrBufferDestroyed
	}
	if err := validateMapArgs(b, mode, offset, size); err != nil {
		return nil, err
	}

	b.mapState = MapStatePending
	jsMode := mapModeToJS(mode)

	// Create a MapPending that will resolve asynchronously.
	p := &MapPending{
		buf:      b,
		done:     make(chan error, 1),
		released: new(atomic.Bool),
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
	if b.IsReleased() {
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
		buf:      b,
		offset:   offset,
		size:     size,
		valid:    true,
		released: new(atomic.Bool),
	}, nil
}

// Unmap releases the current mapping and invalidates all outstanding
// MappedRange handles. Safe to call multiple times; a second call returns
// ErrMapNotMapped.
func (b *Buffer) Unmap() error {
	if b == nil || b.browser == nil {
		return ErrReleased
	}
	if b.IsReleased() {
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
	if mode&MapModeRead != 0 && b.usage&gputypes.BufferUsageMapRead == 0 {
		return ErrMapInvalidMode
	}
	if mode&MapModeWrite != 0 && b.usage&gputypes.BufferUsageMapWrite == 0 {
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
