package core

import (
	"sync/atomic"
	"testing"
)

func TestDestroyQueue_TrackAndTriage(t *testing.T) {
	var destroyed atomic.Bool
	q := NewDestroyQueue()

	q.Defer(5, "buffer-A", func() { destroyed.Store(true) })

	// Triage with completedIndex < submissionIndex: nothing destroyed.
	q.Triage(4)
	if destroyed.Load() {
		t.Fatal("resource destroyed before its submission completed")
	}
	if q.Len() != 1 {
		t.Fatalf("want 1 pending, got %d", q.Len())
	}

	// Triage with completedIndex >= submissionIndex: destroyed.
	q.Triage(5)
	if !destroyed.Load() {
		t.Fatal("resource was not destroyed after submission completed")
	}
	if q.Len() != 0 {
		t.Fatalf("want 0 pending, got %d", q.Len())
	}
}

func TestDestroyQueue_TriagePartial(t *testing.T) {
	var destroyedA, destroyedB, destroyedC atomic.Bool
	q := NewDestroyQueue()

	q.Defer(1, "A", func() { destroyedA.Store(true) })
	q.Defer(3, "B", func() { destroyedB.Store(true) })
	q.Defer(5, "C", func() { destroyedC.Store(true) })

	// Only submission 1 and 3 completed.
	q.Triage(3)

	if !destroyedA.Load() {
		t.Error("A (index=1) should be destroyed")
	}
	if !destroyedB.Load() {
		t.Error("B (index=3) should be destroyed")
	}
	if destroyedC.Load() {
		t.Error("C (index=5) should NOT be destroyed yet")
	}
	if q.Len() != 1 {
		t.Fatalf("want 1 pending (C), got %d", q.Len())
	}

	// Now complete submission 5.
	q.Triage(5)
	if !destroyedC.Load() {
		t.Error("C should now be destroyed")
	}
	if q.Len() != 0 {
		t.Fatalf("want 0 pending, got %d", q.Len())
	}
}

func TestDestroyQueue_FlushAll(t *testing.T) {
	var count atomic.Int32
	q := NewDestroyQueue()

	q.Defer(10, "A", func() { count.Add(1) })
	q.Defer(20, "B", func() { count.Add(1) })
	q.Defer(30, "C", func() { count.Add(1) })

	q.FlushAll()

	if got := count.Load(); got != 3 {
		t.Fatalf("FlushAll: want 3 destroyed, got %d", got)
	}
	if q.Len() != 0 {
		t.Fatalf("want 0 pending after FlushAll, got %d", q.Len())
	}
}

func TestDestroyQueue_EmptyTriage(t *testing.T) {
	q := NewDestroyQueue()

	// Should not panic on empty queue.
	q.Triage(100)
	q.FlushAll()

	if q.Len() != 0 {
		t.Fatalf("want 0, got %d", q.Len())
	}
}

func TestDestroyQueue_MultipleSubmissions(t *testing.T) {
	order := make([]string, 0, 5)
	q := NewDestroyQueue()

	q.Defer(1, "first", func() { order = append(order, "first") })
	q.Defer(2, "second", func() { order = append(order, "second") })
	q.Defer(3, "third", func() { order = append(order, "third") })
	q.Defer(4, "fourth", func() { order = append(order, "fourth") })
	q.Defer(5, "fifth", func() { order = append(order, "fifth") })

	// Triage incrementally.
	q.Triage(2) // destroys first, second
	if len(order) != 2 || order[0] != "first" || order[1] != "second" {
		t.Fatalf("after Triage(2): want [first second], got %v", order)
	}

	q.Triage(4) // destroys third, fourth
	if len(order) != 4 || order[2] != "third" || order[3] != "fourth" {
		t.Fatalf("after Triage(4): want [... third fourth], got %v", order)
	}

	q.Triage(5) // destroys fifth
	if len(order) != 5 || order[4] != "fifth" {
		t.Fatalf("after Triage(5): want [... fifth], got %v", order)
	}

	if q.Len() != 0 {
		t.Fatalf("want 0 pending, got %d", q.Len())
	}
}

func TestDestroyQueue_DoubleFlush(t *testing.T) {
	var count atomic.Int32
	q := NewDestroyQueue()

	q.Defer(1, "buf", func() { count.Add(1) })
	q.FlushAll()
	q.FlushAll() // second flush should be no-op

	if got := count.Load(); got != 1 {
		t.Fatalf("want destroy called once, got %d", got)
	}
}

func TestDestroyQueue_DeferAfterTriage(t *testing.T) {
	var destroyedA, destroyedB atomic.Bool
	q := NewDestroyQueue()

	q.Defer(1, "A", func() { destroyedA.Store(true) })
	q.Triage(1) // destroys A

	q.Defer(2, "B", func() { destroyedB.Store(true) })
	q.Triage(2) // destroys B

	if !destroyedA.Load() || !destroyedB.Load() {
		t.Fatal("both resources should be destroyed")
	}
}
