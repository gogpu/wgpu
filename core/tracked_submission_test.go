package core

import (
	"sync/atomic"
	"testing"
)

func TestDestroyQueue_TrackSubmission(t *testing.T) {
	var dropped atomic.Int32
	q := NewDestroyQueue()

	// Create refs and Clone them (simulating what encoding does).
	ref1 := NewResourceRef("buffer-A", func() { dropped.Add(1) })
	ref2 := NewResourceRef("buffer-B", func() { dropped.Add(1) })
	ref1.Clone()
	ref2.Clone()

	// Track the cloned refs with submission index 5.
	q.TrackSubmission(5, []*ResourceRef{ref1, ref2})

	if q.TrackedLen() != 1 {
		t.Fatalf("want 1 tracked submission, got %d", q.TrackedLen())
	}

	// Triage with completedIndex < 5: refs not dropped.
	q.Triage(4)
	if ref1.RefCount() != 2 || ref2.RefCount() != 2 {
		t.Fatalf("refs should still be at 2, got ref1=%d ref2=%d", ref1.RefCount(), ref2.RefCount())
	}
	if q.TrackedLen() != 1 {
		t.Fatalf("tracked submission should still be pending")
	}

	// Triage with completedIndex >= 5: refs dropped.
	q.Triage(5)
	if ref1.RefCount() != 1 || ref2.RefCount() != 1 {
		t.Fatalf("refs should be at 1 after Drop, got ref1=%d ref2=%d", ref1.RefCount(), ref2.RefCount())
	}
	if q.TrackedLen() != 0 {
		t.Fatalf("want 0 tracked submissions after triage, got %d", q.TrackedLen())
	}

	// onZero should NOT have been called yet (original owner still holds ref).
	if dropped.Load() != 0 {
		t.Fatalf("onZero should not be called while original ref exists")
	}

	// Drop the original refs — now onZero fires.
	ref1.Drop()
	ref2.Drop()
	if dropped.Load() != 2 {
		t.Fatalf("want 2 destroyed, got %d", dropped.Load())
	}
}

func TestDestroyQueue_TrackSubmissionMultiple(t *testing.T) {
	var dropped atomic.Int32
	q := NewDestroyQueue()

	ref1 := NewResourceRef("A", func() { dropped.Add(1) })
	ref2 := NewResourceRef("B", func() { dropped.Add(1) })
	ref3 := NewResourceRef("C", func() { dropped.Add(1) })
	ref1.Clone()
	ref2.Clone()
	ref3.Clone()

	q.TrackSubmission(1, []*ResourceRef{ref1})
	q.TrackSubmission(3, []*ResourceRef{ref2})
	q.TrackSubmission(5, []*ResourceRef{ref3})

	if q.TrackedLen() != 3 {
		t.Fatalf("want 3 tracked, got %d", q.TrackedLen())
	}

	// Triage submissions 1 and 3.
	q.Triage(3)
	if ref1.RefCount() != 1 || ref2.RefCount() != 1 {
		t.Fatal("ref1/ref2 clones should be dropped")
	}
	if ref3.RefCount() != 2 {
		t.Fatal("ref3 should still have 2 refs")
	}
	if q.TrackedLen() != 1 {
		t.Fatalf("want 1 tracked, got %d", q.TrackedLen())
	}

	// Triage submission 5.
	q.Triage(5)
	if ref3.RefCount() != 1 {
		t.Fatal("ref3 clone should be dropped")
	}
	if q.TrackedLen() != 0 {
		t.Fatalf("want 0 tracked, got %d", q.TrackedLen())
	}
}

func TestDestroyQueue_TrackSubmissionFlushAll(t *testing.T) {
	var dropped atomic.Int32
	q := NewDestroyQueue()

	ref := NewResourceRef("buf", func() { dropped.Add(1) })
	ref.Clone()

	q.TrackSubmission(10, []*ResourceRef{ref})

	// FlushAll should drop the ref even though submission 10 hasn't completed.
	q.FlushAll()
	if ref.RefCount() != 1 {
		t.Fatalf("want refCount=1 after FlushAll drop, got %d", ref.RefCount())
	}
	if q.TrackedLen() != 0 {
		t.Fatalf("want 0 tracked after FlushAll, got %d", q.TrackedLen())
	}
}

func TestDestroyQueue_TrackSubmissionEmpty(t *testing.T) {
	q := NewDestroyQueue()

	// Empty refs slice should be a no-op.
	q.TrackSubmission(1, nil)
	q.TrackSubmission(2, []*ResourceRef{})

	if q.TrackedLen() != 0 {
		t.Fatalf("empty tracking should not add entries, got %d", q.TrackedLen())
	}
}

func TestDestroyQueue_MixedDeferAndTrack(t *testing.T) {
	var deferDestroyed atomic.Bool
	var trackDropped atomic.Int32
	q := NewDestroyQueue()

	// Phase 1: Defer
	q.Defer(3, "deferred-buf", func() { deferDestroyed.Store(true) })

	// Phase 2: Track
	ref := NewResourceRef("tracked-buf", func() { trackDropped.Add(1) })
	ref.Clone()
	q.TrackSubmission(5, []*ResourceRef{ref})

	// Triage at 3: deferred destroyed, tracked still pending.
	q.Triage(3)
	if !deferDestroyed.Load() {
		t.Fatal("deferred resource should be destroyed")
	}
	if ref.RefCount() != 2 {
		t.Fatal("tracked ref should still have 2 refs")
	}

	// Triage at 5: tracked dropped.
	q.Triage(5)
	if ref.RefCount() != 1 {
		t.Fatal("tracked ref clone should be dropped")
	}
}
