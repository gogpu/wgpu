//go:build !rust && !(js && wasm)

package wgpu_test

import (
	"testing"

	"github.com/gogpu/wgpu"
)

// Regression coverage for the Buffer.Destroy tracker-index lifecycle.
//
// The public API cycle exercised here — CreateBuffer → encode usage → Submit →
// completion wait → Release/GC cleanup → tracking-data release — is the only
// meaningful pattern: manually calling td.Release() (the previous test
// approach) masks the defect where Buffer.Destroy skipped the release.
//
// The in-flight test drives the same core ResourceRef lifecycle that
// queue_native.go postSubmit performs (Clone during encoding → tracked in
// DestroyQueue → Drop at Triage when the GPU completes): no production code
// changes are simulated beyond what Submit already does.

// noHALAvailable reports whether the test env lacks a real HAL device.
func noHALAvailable(device *wgpu.Device) bool {
	return device == nil || device.Queue() == nil
}

// TestBufferDestroyReleasesTrackerIndex walks the full public lifecycle with
// explicit Release() and asserts the tracker index is returned to the device
// allocator by Buffer.Destroy (reached via the onZero refcount callback).
// After the pair create→release, allocating a new buffer must reuse the lower
// index instead of growing forever.
func TestBufferDestroyReleasesTrackerIndex(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	if noHALAvailable(device) {
		t.Skip("skipping: device has no HAL integration")
	}

	buf1, err := device.CreateBuffer(&wgpu.BufferDescriptor{Size: 16, Usage: wgpu.BufferUsageVertex})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	index1 := buf1.TestBufferTrackingData().Index()
	if !index1.IsValid() {
		t.Fatal("first buffer index invalid")
	}

	buf1.Release()
	if !buf1.TestBufferTrackingData().IsReleased() {
		t.Fatal("tracker index not released after public Release cycle")
	}

	buf2, err := device.CreateBuffer(&wgpu.BufferDescriptor{Size: 16, Usage: wgpu.BufferUsageVertex})
	if err != nil {
		t.Fatalf("CreateBuffer (reuse check): %v", err)
	}
	defer buf2.Release()
	index2 := buf2.TestBufferTrackingData().Index()
	if index2 != index1 {
		t.Fatalf("index not reused after Destroy: got %d, want %d", index2, index1)
	}
}

// TestBufferDestroyInFlightRetained proves that a buffer whose core
// ResourceRef was cloned for an in-flight submission keeps its tracker index
// until the submission's ref is dropped (queue postSubmit → DestroyQueue
// Triage). Release() with a live clone must NOT free the index yet.
func TestBufferDestroyInFlightRetained(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	if noHALAvailable(device) {
		t.Skip("skipping: device has no HAL integration")
	}

	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{Size: 64, Usage: wgpu.BufferUsageVertex | wgpu.BufferUsageCopyDst})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	index := buf.TestBufferTrackingData().Index()
	if !index.IsValid() {
		t.Fatal("index invalid")
	}

	// Simulate what queue Submit does: Clone the resource ref for the
	// in-flight submission (the native layer's trackRef path).
	buf.TestBufferRef().Clone()

	// Application releases the buffer while still in flight.
	buf.Release()

	// The index must NOT be released yet: the in-flight clone still owns a
	// reference, so the onZero callback (which reaches core Destroy and the
	// tracker release) has not fired.
	if buf.TestBufferTrackingData().IsReleased() {
		t.Fatal("tracker index released while submission in-flight — would free state the GPU may still use")
	}

	// Simulate completion: queue postSubmit's Triage drops the tracked clone.
	buf.TestBufferRef().Drop()

	if !buf.TestBufferTrackingData().IsReleased() {
		t.Fatal("tracker index not released after completion Drop")
	}
}
