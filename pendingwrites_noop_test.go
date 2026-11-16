//go:build !rust && !(js && wasm)

package wgpu_test

import (
	"testing"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/hal"
	"github.com/gogpu/wgpu/internal/hal/noop"
	"github.com/gogpu/wgpu/internal/pendingwrites"
)

// TestPendingWritesNonBatchingOverRealHAL keeps one end-to-end test of the
// non-batching write path over a real HAL implementation: the move to
// internal/pendingwrites ported the unit tests onto doubles, and the S8 review
// (F3) flagged that no test drove `WriteBuffer` through an actual hal.Queue
// anymore. Batching backends record into a command encoder; this path writes
// straight through the queue, which the noop backend performs for real.
func TestPendingWritesNonBatchingOverRealHAL(t *testing.T) {
	dev := &noop.Device{}
	q := &noop.Queue{}

	buf, err := dev.CreateBuffer(&hal.BufferDescriptor{
		Label:            "non-batching round trip",
		Size:             16,
		Usage:            gputypes.BufferUsageMapRead | gputypes.BufferUsageMapWrite | gputypes.BufferUsageCopyDst,
		MappedAtCreation: true,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer dev.DestroyBuffer(buf)

	writes := pendingwrites.New(dev, q, nil, nil)
	defer writes.Destroy()

	data := []byte{0xA1, 0xB2, 0xC3, 0xD4}
	if err := writes.WriteBuffer(nil, buf, 0, 4, data); err != nil {
		t.Fatalf("WriteBuffer: %v", err)
	}
	if writes.HasPendingWork() {
		t.Fatal("non-batching writes go straight to the queue: no pending work expected")
	}

	mapping, err := dev.MapBuffer(buf, 0, 16)
	if err != nil {
		t.Fatalf("MapBuffer: %v", err)
	}
	readBack := make([]byte, len(data))
	copy(readBack, unsafe.Slice((*byte)(unsafe.Add(mapping.Ptr, 4)), len(data)))
	if err := dev.UnmapBuffer(buf); err != nil {
		t.Fatalf("UnmapBuffer: %v", err)
	}
	for i, b := range readBack {
		if b != data[i] {
			t.Errorf("byte %d: got 0x%02x, want 0x%02x", i, b, data[i])
		}
	}
}
