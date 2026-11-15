//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu_test

import (
	"context"
	"errors"
	"runtime"
	"runtime/debug"
	"sync"
	"testing"

	"github.com/gogpu/wgpu"
)

// ADR-070 S1 F1 — the mapping path's allocation gates as normal tests.
//
// The implementation handles (MapPending, MappedRange) are pooled, so the
// public wrappers must be cached on the pooled objects rather than built per
// call: a steady-state map cycle allocates neither the handle nor its wrapper.
// These were benchmark-only claims until F1; a benchmark cannot fail CI, so
// they are asserted here with testing.AllocsPerRun.
//
// The two benchmarks in buffer_mapping_bench_test.go stay for benchstat A/B
// measurements; these tests are the gate.

// raceDetectorEnabled reports whether this test binary was built with -race,
// which the go command records as a -race build setting.
func raceDetectorEnabled() bool {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return false
	}
	for _, s := range info.Settings {
		if s.Key == "-race" {
			return s.Value == "true"
		}
	}
	return false
}

// skipUnderRaceDetector skips a test whose assertions depend on sync.Pool
// handing the same object back. With -race, sync.Pool.Put deliberately drops
// one entry in four (sync/pool.go, runtime_randn(4) == 0) so that code relying
// on reuse shows up; reuse is not an observable the race build offers, so
// these assertions would measure the runtime rather than the wrapper
// contract. The -race run covers the slot's synchronization instead, through
// TestBufferMapPoolHandover.
func skipUnderRaceDetector(t *testing.T) {
	t.Helper()
	if !raceDetectorEnabled() {
		return
	}
	t.Skip("skip under -race: sync.Pool.Put drops one entry in four there, so " +
		"reuse-based assertions measure the runtime, not the wrapper contract")
}

// pinMappingPool makes the sync.Pool handover deterministic for one test.
//
// sync.Pool stores entries in per-P private slots and drops them at the start
// of a GC cycle. A goroutine that migrates to another P between the release
// (Put) and the next acquire (Get) misses its own entry, and a collection in
// between clears it: both would turn these contract tests into measurements of
// pool luck. One P and no GC keep the release/acquire edge — the edge the
// cached wrapper relies on — intact.
func pinMappingPool(t *testing.T) {
	t.Helper()
	prevProcs := runtime.GOMAXPROCS(1)
	prevGC := debug.SetGCPercent(-1)
	t.Cleanup(func() {
		debug.SetGCPercent(prevGC)
		runtime.GOMAXPROCS(prevProcs)
	})
}

// mapReadOnce performs one blocking Map → MappedRange → Release → Unmap
// cycle. It returns the first error instead of touching *testing.T so it can
// run inside testing.AllocsPerRun.
func mapReadOnce(ctx context.Context, buf *wgpu.Buffer, size uint64) error {
	if err := buf.Map(ctx, wgpu.MapModeRead, 0, size); err != nil {
		return err
	}
	rng, err := buf.MappedRange(0, size)
	if err != nil {
		_ = buf.Unmap()
		return err
	}
	_ = rng.Bytes()
	rng.Release()
	return buf.Unmap()
}

// mapAsyncOnce performs one MapAsync → Poll → Status → Release →
// MappedRange → Release → Unmap cycle, the escape hatch a game loop drives.
func mapAsyncOnce(buf *wgpu.Buffer, device *wgpu.Device, size uint64) error {
	pending, err := buf.MapAsync(wgpu.MapModeRead, 0, size)
	if err != nil {
		return err
	}
	device.Poll(wgpu.PollPoll)
	if ready, statusErr := pending.Status(); !ready || statusErr != nil {
		pending.Release()
		_ = buf.Unmap()
		if statusErr != nil {
			return statusErr
		}
		return errors.New("map pending after Poll")
	}
	pending.Release()
	rng, err := buf.MappedRange(0, size)
	if err != nil {
		_ = buf.Unmap()
		return err
	}
	_ = rng.Bytes()
	rng.Release()
	return buf.Unmap()
}

// assertSteadyStateAllocs asserts that runs runs of cycle allocate exactly
// want allocations per run. testing.AllocsPerRun runs the function once as a
// warm up that fills the pools, then measures; its value is always integral,
// and it is compared exactly, so a single stray allocation fails the gate.
func assertSteadyStateAllocs(t *testing.T, name string, want, runs int, cycle func() error) {
	t.Helper()
	var cycleErr error
	allocs := testing.AllocsPerRun(runs, func() {
		if err := cycle(); err != nil && cycleErr == nil {
			cycleErr = err
		}
	})
	if cycleErr != nil {
		t.Fatalf("%s: cycle failed: %v", name, cycleErr)
	}
	if allocs != float64(want) {
		t.Fatalf("%s: %v allocs/op in steady state, want %d — the pooled handle's "+
			"public wrapper is rebuilt per call (ADR-070 S1 F1)", name, allocs, want)
	}
}

// TestBufferMapReadZeroAlloc gates the blocking Map path: the MappedRange
// wrapper must come from the pooled handle, so a map cycle allocates nothing.
func TestBufferMapReadZeroAlloc(t *testing.T) {
	pinMappingPool(t)
	skipUnderRaceDetector(t)

	instance, adapter, device := createTestDevice(t)
	defer instance.Release()
	defer adapter.Release()
	defer device.Release()

	const size = 1024
	buf := createMapReadBuf(t, device, size)
	defer buf.Release()

	ctx := context.Background()
	assertSteadyStateAllocs(t, "Map+MappedRange", 0, 100, func() error {
		return mapReadOnce(ctx, buf, size)
	})
}

// TestBufferMapAsyncZeroAlloc gates the MapAsync escape hatch: both the
// MapPending and the MappedRange wrapper must come from the pooled handles.
func TestBufferMapAsyncZeroAlloc(t *testing.T) {
	pinMappingPool(t)
	skipUnderRaceDetector(t)

	instance, adapter, device := createTestDevice(t)
	defer instance.Release()
	defer adapter.Release()
	defer device.Release()

	const size = 1024
	buf := createMapReadBuf(t, device, size)
	defer buf.Release()

	assertSteadyStateAllocs(t, "MapAsync+Status+MappedRange", 0, 100, func() error {
		return mapAsyncOnce(buf, device, size)
	})
}

// TestMapPendingWrapperIdentity verifies that one pooled implementation
// handle produces one public wrapper across acquire/release cycles, and that
// the wrapper is not rebuilt behind the caller's back.
func TestMapPendingWrapperIdentity(t *testing.T) {
	pinMappingPool(t)
	skipUnderRaceDetector(t)

	instance, adapter, device := createTestDevice(t)
	defer instance.Release()
	defer adapter.Release()
	defer device.Release()

	const size = 64
	buf := createMapReadBuf(t, device, size)
	defer buf.Release()

	first, err := buf.MapAsync(wgpu.MapModeRead, 0, size)
	if err != nil {
		t.Fatalf("first MapAsync: %v", err)
	}
	device.Poll(wgpu.PollWait)
	if ready, statusErr := first.Status(); !ready || statusErr != nil {
		t.Fatalf("first Status: ready=%v err=%v", ready, statusErr)
	}
	first.Release()
	if err := buf.Unmap(); err != nil {
		t.Fatalf("first Unmap: %v", err)
	}

	for i := 0; i < 8; i++ {
		next, mapErr := buf.MapAsync(wgpu.MapModeRead, 0, size)
		if mapErr != nil {
			t.Fatalf("MapAsync cycle %d: %v", i, mapErr)
		}
		if next != first {
			t.Fatalf("MapAsync cycle %d returned a different wrapper: the pooled "+
				"handle's cached public wrapper was lost (ADR-070 S1 F1)", i)
		}
		device.Poll(wgpu.PollWait)
		if ready, statusErr := next.Status(); !ready || statusErr != nil {
			t.Fatalf("Status cycle %d: ready=%v err=%v", i, ready, statusErr)
		}
		next.Release()
		if unmapErr := buf.Unmap(); unmapErr != nil {
			t.Fatalf("Unmap cycle %d: %v", i, unmapErr)
		}
	}
}

// TestMappedRangeWrapperIdentity verifies the same contract for MappedRange
// across map/unmap cycles (a range stays registered until Unmap, so the
// cycles cannot overlap in time).
func TestMappedRangeWrapperIdentity(t *testing.T) {
	pinMappingPool(t)
	skipUnderRaceDetector(t)

	instance, adapter, device := createTestDevice(t)
	defer instance.Release()
	defer adapter.Release()
	defer device.Release()

	const size = 64
	buf := createMapReadBuf(t, device, size)
	defer buf.Release()

	ctx := context.Background()
	var first *wgpu.MappedRange
	for i := 0; i < 8; i++ {
		if err := buf.Map(ctx, wgpu.MapModeRead, 0, size); err != nil {
			t.Fatalf("Map cycle %d: %v", i, err)
		}
		rng, err := buf.MappedRange(0, size)
		if err != nil {
			_ = buf.Unmap()
			t.Fatalf("MappedRange cycle %d: %v", i, err)
		}
		_ = rng.Bytes()
		if first == nil {
			first = rng
		} else if rng != first {
			_ = buf.Unmap()
			t.Fatalf("MappedRange cycle %d returned a different wrapper: the "+
				"pooled handle's cached public wrapper was lost (ADR-070 S1 F1)", i)
		}
		rng.Release()
		if err := buf.Unmap(); err != nil {
			t.Fatalf("Unmap cycle %d: %v", i, err)
		}
	}
}

// TestMappedRangeLiveRangesDistinct checks the other side of the slot: the
// slot is per pooled object, never per buffer, so two simultaneous
// non-overlapping ranges are two pooled objects and must be two wrappers.
func TestMappedRangeLiveRangesDistinct(t *testing.T) {
	instance, adapter, device := createTestDevice(t)
	defer instance.Release()
	defer adapter.Release()
	defer device.Release()

	const size = 128
	buf := createMapReadBuf(t, device, size)
	defer buf.Release()

	ctx := context.Background()
	if err := buf.Map(ctx, wgpu.MapModeRead, 0, size); err != nil {
		t.Fatalf("Map: %v", err)
	}
	defer func() { _ = buf.Unmap() }()

	low, err := buf.MappedRange(0, size/2)
	if err != nil {
		t.Fatalf("low MappedRange: %v", err)
	}
	high, err := buf.MappedRange(size/2, size/2)
	if err != nil {
		t.Fatalf("high MappedRange: %v", err)
	}
	if low == high {
		t.Fatalf("two simultaneous non-overlapping ranges share one wrapper: " +
			"the slot must be per pooled object, never per buffer (ADR-070 S1 F1)")
	}
	if got, want := len(low.Bytes()), int(size/2); got != want {
		t.Errorf("low.Bytes() len: got %d want %d", got, want)
	}
	if got, want := len(high.Bytes()), int(size/2); got != want {
		t.Errorf("high.Bytes() len: got %d want %d", got, want)
	}
	low.Release()
	high.Release()
}

// TestBufferMapPoolHandover drives the pooled mapping path from several
// goroutines at once on ONE device. Its purpose is the -race evidence for the
// cached wrapper's synchronization: an implementation object released by one
// goroutine is re-acquired by another, and the race detector checks the slot
// access against sync.Pool's Put/Get handover — the happens-before edge the
// design relies on — while the device's lazy map tracker is initialized by the
// same goroutines (race-free since the slot became atomic; see
// TestMapTrackerConcurrentFirstUse). It asserts correctness only: under -race
// the pool may hand a different object back, so no identity assertion belongs
// here.
func TestBufferMapPoolHandover(t *testing.T) {
	instance, adapter, device := createTestDevice(t)
	defer instance.Release()
	defer adapter.Release()
	defer device.Release()

	const (
		workers = 4
		cycles  = 25
		size    = 64
	)
	bufs := make([]*wgpu.Buffer, workers)
	for i := range bufs {
		bufs[i] = createMapReadBuf(t, device, size)
	}
	defer func() {
		for _, buf := range bufs {
			buf.Release()
		}
	}()

	ctx := context.Background()
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for _, buf := range bufs {
		wg.Add(1)
		go func(buf *wgpu.Buffer) {
			defer wg.Done()
			for i := 0; i < cycles; i++ {
				if err := mapReadOnce(ctx, buf, size); err != nil {
					errs <- err
					return
				}
			}
		}(buf)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent map cycle: %v", err)
	}
}
