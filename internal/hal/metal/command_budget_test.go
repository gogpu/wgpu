// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package metal

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
)

func TestCommandBudgetAcquireReleaseSymmetry(t *testing.T) {
	const limit = 4
	b := newCommandBudget(limit)

	for i := 0; i < limit; i++ {
		if got, ok := b.TryAcquire(); !ok || got != int64(i+1) {
			t.Fatalf("TryAcquire #%d = (%d, %v), want (%d, true)", i+1, got, ok, i+1)
		}
	}
	if got := b.Outstanding(); got != limit {
		t.Fatalf("Outstanding after %d acquires = %d, want %d", limit, got, limit)
	}

	for i := 0; i < limit; i++ {
		b.Release()
	}
	if got := b.Outstanding(); got != 0 {
		t.Fatalf("Outstanding after %d releases = %d, want 0", limit, got)
	}

	// The whole budget must be usable again.
	for i := 0; i < limit; i++ {
		if _, ok := b.TryAcquire(); !ok {
			t.Fatalf("TryAcquire #%d after full release failed", i+1)
		}
	}
}

// TestCommandBudgetRefusesAtLimit pins the boundary: the budget admits exactly
// limit acquisitions, and every further attempt is refused with the current
// count reported for the WARN.
func TestCommandBudgetRefusesAtLimit(t *testing.T) {
	const limit = 3
	b := newCommandBudget(limit)

	for i := 0; i < limit; i++ {
		if _, ok := b.TryAcquire(); !ok {
			t.Fatalf("TryAcquire #%d = false, want true", i+1)
		}
	}

	for i := 0; i < 2; i++ {
		got, ok := b.TryAcquire()
		if ok {
			t.Fatalf("TryAcquire beyond the limit = (%d, true), want (_, false)", got)
		}
		if got != limit {
			t.Fatalf("refused TryAcquire reported outstanding = %d, want %d", got, limit)
		}
	}
	if got := b.Outstanding(); got != limit {
		t.Fatalf("Outstanding after refused attempts = %d, want %d", got, limit)
	}
}

// TestCommandBudgetRefusalDoesNotConsume checks the operation-level semantics
// of the guard: a refused attempt must not inflate the count, and releasing an
// outstanding buffer frees exactly one slot for the next attempt.
func TestCommandBudgetRefusalDoesNotConsume(t *testing.T) {
	b := newCommandBudget(1)

	if _, ok := b.TryAcquire(); !ok {
		t.Fatal("first TryAcquire = false, want true")
	}
	if _, ok := b.TryAcquire(); ok {
		t.Fatal("second TryAcquire = true at the limit, want false")
	}
	if got := b.Outstanding(); got != 1 {
		t.Fatalf("Outstanding after refusal = %d, want 1", got)
	}

	b.Release()
	if _, ok := b.TryAcquire(); !ok {
		t.Fatal("TryAcquire after release = false, want true")
	}
	if got := b.Outstanding(); got != 1 {
		t.Fatalf("Outstanding = %d, want 1", got)
	}
}

// TestCommandBudgetNoNegative checks the saturating release: an unbalanced
// release must not underflow the count, because an underflowed count would
// silently admit more than limit command buffers.
func TestCommandBudgetNoNegative(t *testing.T) {
	b := newCommandBudget(2)

	b.Release()
	if got := b.Outstanding(); got != 0 {
		t.Fatalf("Outstanding after release on an empty budget = %d, want 0", got)
	}

	if _, ok := b.TryAcquire(); !ok {
		t.Fatal("TryAcquire = false, want true")
	}
	b.Release()
	b.Release()
	if got := b.Outstanding(); got != 0 {
		t.Fatalf("Outstanding after unbalanced releases = %d, want 0", got)
	}
}

// TestCommandBudgetConcurrentAcquireRelease exercises the counter under
// concurrent use: every acquired unit is released again, and the count ends at
// zero. Run with -race to check the atomic protocol itself.
func TestCommandBudgetConcurrentAcquireRelease(t *testing.T) {
	const (
		limit      = 64
		goroutines = 16
		iterations = 2000
	)
	b := newCommandBudget(limit)

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				if _, ok := b.TryAcquire(); !ok {
					runtime.Gosched()
					continue
				}
				b.Release()
			}
		}()
	}
	wg.Wait()

	if got := b.Outstanding(); got != 0 {
		t.Fatalf("Outstanding after concurrent acquire/release = %d, want 0", got)
	}
}

// TestCommandBudgetConcurrentNeverExceedsLimit holds units concurrently and
// checks that the counter never admits more than limit holders at once — the
// property the guard exists for.
func TestCommandBudgetConcurrentNeverExceedsLimit(t *testing.T) {
	const (
		limit      = 4
		goroutines = 8
		iterations = 500
	)
	b := newCommandBudget(limit)

	var held, maxHeld atomic.Int64
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				if _, ok := b.TryAcquire(); !ok {
					runtime.Gosched()
					continue
				}
				cur := held.Add(1)
				for {
					prev := maxHeld.Load()
					if cur <= prev || maxHeld.CompareAndSwap(prev, cur) {
						break
					}
				}
				if cur > limit {
					t.Errorf("%d command buffers held at once, limit is %d", cur, limit)
				}
				runtime.Gosched()
				held.Add(-1)
				b.Release()
			}
		}()
	}
	wg.Wait()

	if got := b.Outstanding(); got != 0 {
		t.Fatalf("Outstanding after concurrent hold/release = %d, want 0", got)
	}
	if got := maxHeld.Load(); got > limit {
		t.Fatalf("maximum concurrent holders = %d, want at most %d", got, limit)
	}
	// The budget must still be fully usable after the contention.
	for i := 0; i < limit; i++ {
		if _, ok := b.TryAcquire(); !ok {
			t.Fatalf("TryAcquire #%d after the concurrent run failed", i+1)
		}
	}
}

// TestCommandBudgetUnitReleasesOnce pins the exactly-once release of a command
// buffer's unit, whether it is the commit path (Submit) or a drop path
// (Destroy/discard) that calls it first.
func TestCommandBudgetUnitReleasesOnce(t *testing.T) {
	b := newCommandBudget(2)
	if _, ok := b.TryAcquire(); !ok {
		t.Fatal("TryAcquire = false, want true")
	}
	u := newCommandBudgetUnit(b)
	u.release()
	u.release()
	if got := b.Outstanding(); got != 0 {
		t.Fatalf("Outstanding after a double release = %d, want 0", got)
	}

	// The zero unit (a command buffer of a device-free test) is a no-op.
	var zero commandBudgetUnit
	zero.release()
	if got := b.Outstanding(); got != 0 {
		t.Fatalf("Outstanding after releasing a zero unit = %d, want 0", got)
	}
}

// TestSubmitLoopReleasesEveryUnit pins the Submit decrement: runSubmit must
// commit every command buffer and return each unit to the budget, and it must
// register completion tracking on the batch's last command buffer only. This
// test goes RED if the unit release is removed from the loop — the regression
// that leaves the budget leaking over a session.
func TestSubmitLoopReleasesEveryUnit(t *testing.T) {
	const buffers = 3
	b := newCommandBudget(buffers)
	batch := make([]budgetedSubmit, buffers)
	for i := range batch {
		if _, ok := b.TryAcquire(); !ok {
			t.Fatalf("TryAcquire #%d = false, want true", i+1)
		}
		unit := newCommandBudgetUnit(b)
		batch[i] = budgetedSubmit{raw: uintptr(100 + i), unit: &unit}
	}

	var commits, lastCalls int
	var lastRaw uintptr
	runSubmit(batch, submitHooks{
		onLast: func(raw uintptr, subIdx uint64) {
			lastCalls++
			lastRaw = raw
			if subIdx != 7 {
				t.Errorf("onLast subIdx = %d, want 7", subIdx)
			}
		},
		commit: func(raw uintptr) {
			commits++
		},
	}, 7)

	if commits != buffers {
		t.Fatalf("commits = %d, want %d", commits, buffers)
	}
	if lastCalls != 1 || lastRaw != 102 {
		t.Fatalf("completion registration = (%d calls, raw %d), want (1 call, raw 102)", lastCalls, lastRaw)
	}
	if got := b.Outstanding(); got != 0 {
		t.Fatalf("Outstanding after Submit = %d, want 0 (a unit was not returned)", got)
	}
}

// TestSubmitLoopSkipsEmptySlots checks that slots of the caller's list that
// held no Metal command buffer are skipped, and that completion registration
// follows the original last slot: a zero last slot registers nothing, as
// Queue.Submit's loop did before the batch split.
func TestSubmitLoopSkipsEmptySlots(t *testing.T) {
	b := newCommandBudget(2)
	var commits, lastCalls int

	// Valid slot before a zero last slot: commit happens, no registration.
	if _, ok := b.TryAcquire(); !ok {
		t.Fatal("TryAcquire = false, want true")
	}
	unit := newCommandBudgetUnit(b)
	runSubmit([]budgetedSubmit{{raw: 5, unit: &unit}, {}}, submitHooks{
		onLast: func(uintptr, uint64) { lastCalls++ },
		commit: func(uintptr) { commits++ },
	}, 1)
	if commits != 1 || lastCalls != 0 {
		t.Fatalf("commit/last calls = %d/%d, want 1/0 for a zero last slot", commits, lastCalls)
	}
	if got := b.Outstanding(); got != 0 {
		t.Fatalf("Outstanding = %d, want 0", got)
	}

	// Zero slot before a valid last slot: commit and registration both happen.
	commits, lastCalls = 0, 0
	if _, ok := b.TryAcquire(); !ok {
		t.Fatal("TryAcquire = false, want true")
	}
	unit = newCommandBudgetUnit(b)
	runSubmit([]budgetedSubmit{{}, {raw: 6, unit: &unit}}, submitHooks{
		onLast: func(uintptr, uint64) { lastCalls++ },
		commit: func(uintptr) { commits++ },
	}, 1)
	if commits != 1 || lastCalls != 1 {
		t.Fatalf("commit/last calls = %d/%d, want 1/1", commits, lastCalls)
	}
	if got := b.Outstanding(); got != 0 {
		t.Fatalf("Outstanding = %d, want 0", got)
	}
}
