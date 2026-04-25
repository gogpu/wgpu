//go:build !(js && wasm)

package wgpu_test

import (
	"runtime"
	"testing"
	"time"

	"github.com/gogpu/wgpu"

	_ "github.com/gogpu/wgpu/hal/noop"
)

// =============================================================================
// Buffer GC cleanup tests (BUG-WGPU-RESOURCE-LIFECYCLE-001 Phase A)
// =============================================================================

func TestBuffer_CleanupOnGC(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	dq := device.TestDestroyQueue()
	if dq == nil {
		t.Skip("device has no DestroyQueue")
	}

	pendingBefore := dq.Len()

	// Create a buffer without calling Release(), then drop the reference.
	// The GC cleanup should schedule deferred destruction via DestroyQueue.
	func() {
		buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
			Label: "gc-cleanup-buf",
			Size:  64,
			Usage: wgpu.BufferUsageVertex,
		})
		if err != nil {
			t.Fatalf("CreateBuffer: %v", err)
		}
		// Ensure the buffer is reachable up to this point.
		runtime.KeepAlive(buf)
		// buf goes out of scope here — eligible for GC.
	}()

	// Force GC to trigger the cleanup.
	runtime.GC()
	runtime.GC() // second pass to ensure finalizers/cleanups run

	// Give cleanup goroutine time to execute (runtime.AddCleanup runs
	// asynchronously on a background goroutine).
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if dq.Len() > pendingBefore {
			break
		}
		runtime.Gosched()
		time.Sleep(5 * time.Millisecond)
	}

	pendingAfter := dq.Len()
	if pendingAfter <= pendingBefore {
		t.Errorf("DestroyQueue pending count did not increase after GC: before=%d, after=%d",
			pendingBefore, pendingAfter)
	}
}

func TestBuffer_ExplicitRelease_CancelsCleanup(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	dq := device.TestDestroyQueue()
	if dq == nil {
		t.Skip("device has no DestroyQueue")
	}

	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "explicit-release-buf",
		Size:  64,
		Usage: wgpu.BufferUsageVertex,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}

	// Explicit Release should cancel the GC cleanup.
	buf.Release()

	if !buf.TestReleased() {
		t.Fatal("buffer should be marked as released after Release()")
	}

	// Record pending count after Release.
	pendingAfterRelease := dq.Len()

	// Force GC — cleanup should NOT fire because it was canceled.
	runtime.GC()
	runtime.GC()
	runtime.Gosched()
	time.Sleep(50 * time.Millisecond)

	pendingAfterGC := dq.Len()
	if pendingAfterGC != pendingAfterRelease {
		t.Errorf("GC cleanup fired after explicit Release: pending before GC=%d, after GC=%d",
			pendingAfterRelease, pendingAfterGC)
	}
}

func TestBuffer_DoubleRelease_NoPanic(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "double-release-buf",
		Size:  64,
		Usage: wgpu.BufferUsageVertex,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}

	// Should not panic.
	buf.Release()
	buf.Release()
}

// =============================================================================
// BindGroup GC cleanup tests (BUG-WGPU-RESOURCE-LIFECYCLE-001 Phase A)
// =============================================================================

func TestBindGroup_CleanupOnGC(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	dq := device.TestDestroyQueue()
	if dq == nil {
		t.Skip("device has no DestroyQueue")
	}

	layout, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label:   "gc-bgl",
		Entries: []wgpu.BindGroupLayoutEntry{},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout: %v", err)
	}
	defer layout.Release()

	pendingBefore := dq.Len()

	// Create a bind group without calling Release(), then drop the reference.
	func() {
		bg, bgErr := device.CreateBindGroup(&wgpu.BindGroupDescriptor{
			Label:  "gc-cleanup-bg",
			Layout: layout,
		})
		if bgErr != nil {
			t.Fatalf("CreateBindGroup: %v", bgErr)
		}
		runtime.KeepAlive(bg)
	}()

	runtime.GC()
	runtime.GC()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if dq.Len() > pendingBefore {
			break
		}
		runtime.Gosched()
		time.Sleep(5 * time.Millisecond)
	}

	pendingAfter := dq.Len()
	if pendingAfter <= pendingBefore {
		t.Errorf("DestroyQueue pending count did not increase after GC: before=%d, after=%d",
			pendingBefore, pendingAfter)
	}
}

func TestBindGroup_ExplicitRelease_CancelsCleanup(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	dq := device.TestDestroyQueue()
	if dq == nil {
		t.Skip("device has no DestroyQueue")
	}

	layout, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label:   "release-bgl",
		Entries: []wgpu.BindGroupLayoutEntry{},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout: %v", err)
	}
	defer layout.Release()

	bg, err := device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Label:  "explicit-release-bg",
		Layout: layout,
	})
	if err != nil {
		t.Fatalf("CreateBindGroup: %v", err)
	}

	bg.Release()

	if !bg.TestBindGroupReleased() {
		t.Fatal("bind group should be marked as released after Release()")
	}

	pendingAfterRelease := dq.Len()

	runtime.GC()
	runtime.GC()
	runtime.Gosched()
	time.Sleep(50 * time.Millisecond)

	pendingAfterGC := dq.Len()
	if pendingAfterGC != pendingAfterRelease {
		t.Errorf("GC cleanup fired after explicit Release: pending before GC=%d, after GC=%d",
			pendingAfterRelease, pendingAfterGC)
	}
}

func TestBindGroup_DoubleRelease_NoPanic(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	layout, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label:   "dbl-bgl",
		Entries: []wgpu.BindGroupLayoutEntry{},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout: %v", err)
	}
	defer layout.Release()

	bg, err := device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Label:  "double-release-bg",
		Layout: layout,
	})
	if err != nil {
		t.Fatalf("CreateBindGroup: %v", err)
	}

	bg.Release()
	bg.Release()
}

// =============================================================================
// Benchmark: CreateBuffer overhead with runtime.AddCleanup
// =============================================================================

func BenchmarkCreateBuffer(b *testing.B) {
	inst, err := wgpu.CreateInstance(nil)
	if err != nil {
		b.Fatalf("CreateInstance: %v", err)
	}
	defer inst.Release()

	adapter, err := inst.RequestAdapter(nil)
	if err != nil {
		b.Fatalf("RequestAdapter: %v", err)
	}
	defer adapter.Release()

	device, err := adapter.RequestDevice(nil)
	if err != nil {
		b.Fatalf("RequestDevice: %v", err)
	}
	defer device.Release()

	q := device.Queue()
	if q == nil {
		b.Skip("skipping: device has no HAL integration")
	}

	desc := &wgpu.BufferDescriptor{
		Label: "bench-buf",
		Size:  256,
		Usage: wgpu.BufferUsageVertex | wgpu.BufferUsageCopyDst,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		buf, bufErr := device.CreateBuffer(desc)
		if bufErr != nil {
			b.Fatalf("CreateBuffer: %v", bufErr)
		}
		buf.Release()
	}
}

func BenchmarkCreateBindGroup(b *testing.B) {
	inst, err := wgpu.CreateInstance(nil)
	if err != nil {
		b.Fatalf("CreateInstance: %v", err)
	}
	defer inst.Release()

	adapter, err := inst.RequestAdapter(nil)
	if err != nil {
		b.Fatalf("RequestAdapter: %v", err)
	}
	defer adapter.Release()

	device, err := adapter.RequestDevice(nil)
	if err != nil {
		b.Fatalf("RequestDevice: %v", err)
	}
	defer device.Release()

	q := device.Queue()
	if q == nil {
		b.Skip("skipping: device has no HAL integration")
	}

	layout, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label:   "bench-bgl",
		Entries: []wgpu.BindGroupLayoutEntry{},
	})
	if err != nil {
		b.Fatalf("CreateBindGroupLayout: %v", err)
	}
	defer layout.Release()

	desc := &wgpu.BindGroupDescriptor{
		Label:  "bench-bg",
		Layout: layout,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		bg, bgErr := device.CreateBindGroup(desc)
		if bgErr != nil {
			b.Fatalf("CreateBindGroup: %v", bgErr)
		}
		bg.Release()
	}
}
