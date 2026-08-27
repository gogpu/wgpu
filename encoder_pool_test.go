//go:build !rust && !(js && wasm)

package wgpu

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/noop"
)

func TestEncoderPool_AcquireCreatesNew(t *testing.T) {
	dev, _, cleanup := createNoopDeviceForTest(t)
	defer cleanup()

	pool := newEncoderPool(dev)
	defer pool.destroy()

	enc, err := pool.acquire()
	if err != nil {
		t.Fatalf("acquire failed: %v", err)
	}
	if enc == nil {
		t.Fatal("acquired encoder is nil")
	}
}

func TestEncoderPool_ReleaseAndReacquire(t *testing.T) {
	dev, _, cleanup := createNoopDeviceForTest(t)
	defer cleanup()

	pool := newEncoderPool(dev)
	defer pool.destroy()

	// Acquire, use, release.
	enc1, err := pool.acquire()
	if err != nil {
		t.Fatalf("acquire failed: %v", err)
	}

	if err := enc1.BeginEncoding("test"); err != nil {
		t.Fatalf("BeginEncoding failed: %v", err)
	}
	cmdBuf, err := enc1.EndEncoding()
	if err != nil {
		t.Fatalf("EndEncoding failed: %v", err)
	}

	// Simulate GPU completion: ResetAll + release.
	enc1.ResetAll([]hal.CommandBuffer{cmdBuf})
	pool.release(enc1)

	// Reacquire should return the same encoder (from pool, not creating new).
	enc2, err := pool.acquire()
	if err != nil {
		t.Fatalf("re-acquire failed: %v", err)
	}
	if enc2 == nil {
		t.Fatal("re-acquired encoder is nil")
	}

	// The pool should be empty now.
	pool.mu.Lock()
	freeCount := len(pool.free)
	pool.mu.Unlock()
	if freeCount != 0 {
		t.Errorf("expected 0 free encoders after acquire, got %d", freeCount)
	}
}

func TestEncoderPool_MultipleEncoders(t *testing.T) {
	dev, _, cleanup := createNoopDeviceForTest(t)
	defer cleanup()

	pool := newEncoderPool(dev)
	defer pool.destroy()

	// Acquire multiple encoders (simulates maxFramesInFlight=2).
	enc1, err := pool.acquire()
	if err != nil {
		t.Fatalf("acquire 1 failed: %v", err)
	}
	enc2, err := pool.acquire()
	if err != nil {
		t.Fatalf("acquire 2 failed: %v", err)
	}

	// Release both.
	pool.release(enc1)
	pool.release(enc2)

	pool.mu.Lock()
	freeCount := len(pool.free)
	pool.mu.Unlock()
	if freeCount != 2 {
		t.Errorf("expected 2 free encoders, got %d", freeCount)
	}
}

func TestEncoderPool_DestroyReleasesAll(t *testing.T) {
	dev, _, cleanup := createNoopDeviceForTest(t)
	defer cleanup()

	pool := newEncoderPool(dev)

	enc1, err := pool.acquire()
	if err != nil {
		t.Fatalf("acquire failed: %v", err)
	}
	pool.release(enc1)

	// Destroy should not panic.
	pool.destroy()

	pool.mu.Lock()
	freeCount := len(pool.free)
	pool.mu.Unlock()
	if freeCount != 0 {
		t.Errorf("expected 0 free encoders after destroy, got %d", freeCount)
	}
}

// createNoopDeviceForTest creates a noop HAL device for testing.
func createNoopDeviceForTest(t *testing.T) (hal.Device, hal.Queue, func()) {
	t.Helper()
	api := noop.NewBackend()
	inst, err := api.CreateInstance(&hal.InstanceDescriptor{})
	if err != nil {
		t.Fatalf("noop.CreateInstance failed: %v", err)
	}
	adapters := inst.EnumerateAdapters(nil)
	if len(adapters) == 0 {
		t.Fatal("no noop adapters")
	}
	open, err := adapters[0].Adapter.Open(gputypes.Features(0), gputypes.DefaultLimits())
	if err != nil {
		t.Fatalf("adapter.Open failed: %v", err)
	}
	return open.Device, open.Queue, func() {
		open.Device.Destroy()
	}
}

// TestCommandBufferReleaseDropsUsedSets verifies that Release() drops the
// encode-time validation sets. A CommandBuffer that is Finish()'d and then
// Released without being submitted (the hal.Submit failure path) must not keep
// pinning every resource the frame touched.
func TestCommandBufferReleaseDropsUsedSets(t *testing.T) {
	cb := &CommandBuffer{
		usedBuffers:    map[*Buffer]struct{}{{}: {}},
		usedTextures:   map[*Texture]struct{}{{}: {}},
		usedBindGroups: map[*BindGroup]struct{}{{}: {}},
	}

	cb.Release()

	if cb.usedBuffers != nil || cb.usedTextures != nil || cb.usedBindGroups != nil {
		t.Errorf("Release left validation sets populated: buffers=%v textures=%v bindGroups=%v",
			cb.usedBuffers, cb.usedTextures, cb.usedBindGroups)
	}
}

// TestPostSubmitDropsSetsWithoutDestroyQueue verifies that a command buffer is
// marked submitted and has its validation sets dropped even when the device has
// no DestroyQueue. postSubmit returns early in that case; the bookkeeping must
// happen before the early return, since the HAL submit has already succeeded.
func TestPostSubmitDropsSetsWithoutDestroyQueue(t *testing.T) {
	q := &Queue{} // nil device — destroyQueue() returns nil
	if q.destroyQueue() != nil {
		t.Fatal("test precondition: expected nil destroy queue")
	}

	cb := &CommandBuffer{
		usedBuffers:    map[*Buffer]struct{}{{}: {}},
		usedTextures:   map[*Texture]struct{}{{}: {}},
		usedBindGroups: map[*BindGroup]struct{}{{}: {}},
	}

	q.postSubmit(1, []*CommandBuffer{cb})

	if !cb.submitted {
		t.Error("postSubmit left cb.submitted false — a double submit would not be caught")
	}
	if cb.usedBuffers != nil || cb.usedTextures != nil || cb.usedBindGroups != nil {
		t.Errorf("postSubmit left validation sets populated: buffers=%v textures=%v bindGroups=%v",
			cb.usedBuffers, cb.usedTextures, cb.usedBindGroups)
	}
}
