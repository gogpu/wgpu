//go:build !(js && wasm)

package wgpu

import (
	"context"
	"log/slog"
	"runtime"
	"sync/atomic"
	"unsafe"

	"github.com/gogpu/wgpu/core"
	"github.com/gogpu/wgpu/hal"
)

// bufferCleanupRef holds the data needed to destroy a buffer's HAL resources
// from a runtime.AddCleanup callback. This struct must NOT reference the Buffer
// itself — runtime.AddCleanup requires the callback argument to be independent
// of the cleaned-up object to avoid preventing garbage collection.
type bufferCleanupRef struct {
	label        string
	released     *atomic.Bool
	destroyQueue *core.DestroyQueue
	lastSubIdx   func() uint64
	destroyFn    func()
}

// Buffer represents a GPU buffer.
type Buffer struct {
	core    *core.Buffer
	device  *Device
	cleanup runtime.Cleanup
	// released is heap-allocated separately so that the cleanup ref can share
	// it without holding an interior pointer into the Buffer struct. An interior
	// pointer would make the Buffer reachable from the cleanup arg, preventing
	// GC collection and causing the cleanup to never fire.
	released *atomic.Bool
}

// Size returns the buffer size in bytes.
func (b *Buffer) Size() uint64 { return b.core.Size() }

// Usage returns the buffer's usage flags.
func (b *Buffer) Usage() BufferUsage { return b.core.Usage() }

// Label returns the buffer's debug label.
func (b *Buffer) Label() string { return b.core.Label() }

// Release destroys the buffer. The underlying HAL buffer is not freed
// immediately — destruction is deferred until the GPU completes any submission
// that may reference it. This prevents use-after-free on DX12/Vulkan when a
// buffer is released while the GPU is still reading from it (BUG-DX12-TDR).
func (b *Buffer) Release() {
	if b.released == nil || !b.released.CompareAndSwap(false, true) {
		return
	}

	// Cancel the GC cleanup — we are destroying explicitly.
	b.cleanup.Stop()

	if b.device == nil {
		b.core.Destroy()
		return
	}

	dq := b.device.destroyQueue()
	if dq == nil {
		// No DestroyQueue (legacy path or no HAL) — destroy immediately.
		b.core.Destroy()
		return
	}

	// Defer destruction until GPU completes the latest known submission.
	subIdx := b.device.lastSubmissionIndex()
	label := b.core.Label()
	dq.Defer(subIdx, "Buffer:"+label, func() {
		b.core.Destroy()
	})
}

// MapState returns the current mapping state of the buffer.
//
// This is a synchronized snapshot — the state may change immediately
// after return if another goroutine calls Unmap or Destroy, but the
// value reflects the state at the moment of the call.
func (b *Buffer) MapState() MapState {
	if b == nil || b.core == nil {
		return MapStateUnmapped
	}
	return mapStateFromCore(b.core.CurrentMapState())
}

// Map blocks until a CPU-visible mapping is established for the given
// byte range, or until ctx is canceled.
//
// The buffer must have been created with BufferUsageMapRead or
// BufferUsageMapWrite matching mode. offset must be a multiple of 8 and
// size must be a multiple of 4 (WebGPU MAP_ALIGNMENT).
//
// After Map succeeds, call MappedRange to obtain a byte view and Unmap
// when finished. The primary usage pattern mirrors database/sql rows:
//
//	if err := buf.Map(ctx, wgpu.MapModeRead, 0, size); err != nil {
//	    return err
//	}
//	defer buf.Unmap()
//	rng, _ := buf.MappedRange(0, size)
//	data := rng.Bytes()
//
// Map drives Device.Poll internally; callers do not need to schedule
// polling themselves. If you need non-blocking behavior use MapAsync.
func (b *Buffer) Map(ctx context.Context, mode MapMode, offset, size uint64) error {
	if b == nil || b.core == nil {
		return ErrReleased
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	pending, err := b.MapAsync(mode, offset, size)
	if err != nil {
		return err
	}
	// Drain the device once synchronously — for backends that complete
	// instantly (software, noop) and for the common case where the
	// buffer has no in-flight submission this returns immediately with
	// the map already resolved.
	if b.device != nil {
		b.device.Poll(PollPoll)
	}
	if done, werr := pending.Status(); done {
		pending.Release()
		return werr
	}
	// Wait with the caller's context; a PollWait runs inside Device.Poll
	// concurrently with Wait so the HAL fence advances even if nobody
	// else calls Submit before the context deadline.
	if b.device != nil {
		// Kick a PollWait on a worker goroutine so Wait's select unblocks
		// as soon as the fence advances. Allocation is a single channel
		// inside MapPending.Wait; the zero-alloc path is Status-driven.
		go func() {
			b.device.Poll(PollWait)
		}()
	}
	werr := pending.Wait(ctx)
	pending.Release()
	return werr
}

// MapAsync initiates a buffer map without blocking the caller.
//
// Returns a *MapPending handle that resolves once the GPU submission
// that last wrote to the buffer completes. The caller must periodically
// drive Device.Poll(PollPoll) (or, more commonly, rely on the auto-poll
// at the tail of Queue.Submit) to let the mapping resolve.
//
// Validation errors surface synchronously — alignment, usage mismatch,
// range overflow, already-mapped state, etc. A returned MapPending is
// always valid; its Status() may resolve as failed later if the buffer
// is destroyed or the map is canceled.
func (b *Buffer) MapAsync(mode MapMode, offset, size uint64) (*MapPending, error) {
	if b == nil || b.core == nil {
		return nil, ErrReleased
	}
	cerr := b.core.BeginMap(mode.toInternal(), offset, size)
	if cerr != nil {
		return nil, coreErrToTyped(cerr)
	}
	// Register the buffer on the device's pending-map tracker so
	// Device.Poll eventually calls hal.MapBuffer. Use the latest known
	// submission index as the completion gate; if no Submit has happened
	// yet the index is 0 and a subsequent Poll resolves immediately.
	subIdx := uint64(0)
	if b.device != nil {
		subIdx = b.device.lastSubmissionIndex()
	}
	b.core.Device().RegisterPendingMap(subIdx, b.core)
	return acquireMapPending(b, b.core.Waiter()), nil
}

// MappedRange returns a safe view over the mapped region [offset, offset+size).
//
// The buffer must be in the Mapped state (Map or MapAsync resolved).
// The returned range overlaps with neither the rest of the buffer nor
// any previously-returned MappedRange that has not been Unmap'd — WebGPU
// spec §5.3.4 forbids overlapping getMappedRange calls on the same
// buffer and we enforce this synchronously.
//
// The returned slice (via MappedRange.Bytes) is invalidated by Unmap.
func (b *Buffer) MappedRange(offset, size uint64) (*MappedRange, error) {
	if b == nil || b.core == nil {
		return nil, ErrReleased
	}
	if offset%8 != 0 || size%4 != 0 {
		return nil, ErrMapAlignment
	}
	if cerr := b.core.TryRegisterMappedRange(offset, size); cerr != nil {
		return nil, coreErrToTyped(cerr)
	}
	base, pendingOffset, _, ok := b.core.MappingInfo()
	if !ok || base == nil {
		return nil, ErrMapNotMapped
	}
	// The HAL returned a pointer at pendingOffset; shift it so m.data
	// points at the user-requested offset.
	delta := offset - pendingOffset
	var data unsafe.Pointer
	if delta == 0 {
		data = base
	} else {
		data = unsafe.Add(base, uintptr(delta))
	}
	m := acquireMappedRange()
	m.buf = b
	m.offset = offset
	m.size = size
	m.gen = b.core.Generation()
	m.data = data
	return m, nil
}

// Unmap releases the current mapping and invalidates all outstanding
// MappedRange handles for this buffer. Safe to call multiple times;
// a second call returns ErrMapNotMapped.
//
// Unmap also cancels a Pending map (the associated MapPending resolves
// with ErrMapCancelled).
func (b *Buffer) Unmap() error {
	if b == nil || b.core == nil {
		return ErrReleased
	}
	if b.device == nil {
		return ErrReleased
	}
	halDev := b.device.halDevice()
	// We do not require halDev != nil — Unmap on a device-destroyed
	// buffer still clears the state machine; the HAL call is skipped.
	sl := b.core.Device().SnatchLock()
	if sl == nil {
		return ErrReleased
	}
	guard := sl.Read()
	defer guard.Release()
	if cerr := b.core.UnmapBuffer(guard, halDev); cerr != nil {
		return coreErrToTyped(cerr)
	}
	return nil
}

// coreBuffer returns the underlying core.Buffer.
func (b *Buffer) coreBuffer() *core.Buffer { return b.core }

// halBuffer returns the underlying HAL buffer.
func (b *Buffer) halBuffer() hal.Buffer {
	if b.core == nil || b.device == nil {
		return nil
	}
	if !b.core.HasHAL() {
		return nil
	}
	guard := b.device.core.SnatchLock().Read()
	defer guard.Release()
	return b.core.Raw(guard)
}

// registerBufferCleanup registers a runtime.AddCleanup handler on the buffer.
// When GC collects the buffer without an explicit Release(), the cleanup
// schedules deferred destruction via DestroyQueue — the same path as Release().
//
// The cleanup ref struct captures only the label, released flag, DestroyQueue,
// and core destroy function — NOT the Buffer pointer itself. This is a Go 1.24
// runtime.AddCleanup requirement: the callback argument must not reference the
// object being cleaned up.
func registerBufferCleanup(buf *Buffer, dev *Device, coreBuf *core.Buffer, label string) runtime.Cleanup {
	dq := dev.destroyQueue()
	if dq == nil {
		// No DestroyQueue — register cleanup that destroys immediately.
		return runtime.AddCleanup(buf, func(ref bufferCleanupRef) {
			if !ref.released.CompareAndSwap(false, true) {
				return
			}
			slog.Warn("wgpu: Buffer released by GC (missing explicit Release)", "label", ref.label)
			ref.destroyFn()
		}, bufferCleanupRef{
			label:    label,
			released: buf.released,
			destroyFn: func() {
				coreBuf.Destroy()
			},
		})
	}

	return runtime.AddCleanup(buf, func(ref bufferCleanupRef) {
		if !ref.released.CompareAndSwap(false, true) {
			return
		}
		slog.Warn("wgpu: Buffer released by GC (missing explicit Release)", "label", ref.label)
		subIdx := ref.lastSubIdx()
		ref.destroyQueue.Defer(subIdx, "Buffer(GC):"+ref.label, ref.destroyFn)
	}, bufferCleanupRef{
		label:        label,
		released:     buf.released,
		destroyQueue: dq,
		lastSubIdx:   dev.lastSubmissionIndex,
		destroyFn: func() {
			coreBuf.Destroy()
		},
	})
}
