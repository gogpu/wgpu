package core

import (
	"sync"
	"sync/atomic"
	"testing"
)

func TestResourceRef_NewHasRefCount1(t *testing.T) {
	ref := NewResourceRef("test", nil)
	if got := ref.RefCount(); got != 1 {
		t.Errorf("NewResourceRef: want refCount=1, got %d", got)
	}
}

func TestResourceRef_CloneDrop(t *testing.T) {
	ref := NewResourceRef("test", nil)

	ref.Clone()
	if got := ref.RefCount(); got != 2 {
		t.Errorf("after Clone: want refCount=2, got %d", got)
	}

	ref.Drop()
	if got := ref.RefCount(); got != 1 {
		t.Errorf("after Drop: want refCount=1, got %d", got)
	}
}

func TestResourceRef_OnZeroCalledWhenLastRefDropped(t *testing.T) {
	var called atomic.Bool
	ref := NewResourceRef("test-buffer", func() {
		called.Store(true)
	})

	ref.Drop() // refCount: 1 -> 0

	if !called.Load() {
		t.Error("onZero was not called when last ref was dropped")
	}
	if got := ref.RefCount(); got != 0 {
		t.Errorf("after final Drop: want refCount=0, got %d", got)
	}
}

func TestResourceRef_OnZeroNotCalledWithRemainingRefs(t *testing.T) {
	var called atomic.Bool
	ref := NewResourceRef("test-buffer", func() {
		called.Store(true)
	})

	ref.Clone() // refCount: 1 -> 2
	ref.Drop()  // refCount: 2 -> 1

	if called.Load() {
		t.Error("onZero should not be called when refs remain")
	}
	if got := ref.RefCount(); got != 1 {
		t.Errorf("want refCount=1, got %d", got)
	}
}

func TestResourceRef_DropIdempotent(t *testing.T) {
	var callCount atomic.Int32
	ref := NewResourceRef("test", func() {
		callCount.Add(1)
	})

	ref.Drop() // 1 -> 0: calls onZero
	ref.Drop() // 0 -> -1: does NOT call onZero again

	if got := callCount.Load(); got != 1 {
		t.Errorf("onZero should be called exactly once, got %d calls", got)
	}
}

func TestResourceRef_Label(t *testing.T) {
	ref := NewResourceRef("my-buffer", nil)
	if got := ref.Label(); got != "my-buffer" {
		t.Errorf("want label='my-buffer', got %q", got)
	}
}

func TestResourceRef_NilOnZero(t *testing.T) {
	// Ensure nil onZero doesn't panic.
	ref := NewResourceRef("test", nil)
	ref.Drop() // should not panic
}

func TestResourceRef_ConcurrentCloneDrop(t *testing.T) {
	var destroyCount atomic.Int32
	ref := NewResourceRef("concurrent", func() {
		destroyCount.Add(1)
	})

	const goroutines = 100
	var wg sync.WaitGroup

	// Clone goroutines references first.
	for i := 0; i < goroutines; i++ {
		ref.Clone()
	}

	// Drop them all concurrently (plus the original).
	wg.Add(goroutines + 1)
	for i := 0; i < goroutines+1; i++ {
		go func() {
			defer wg.Done()
			ref.Drop()
		}()
	}
	wg.Wait()

	if got := destroyCount.Load(); got != 1 {
		t.Errorf("onZero should be called exactly once, got %d", got)
	}
	if got := ref.RefCount(); got != 0 {
		t.Errorf("want refCount=0, got %d", got)
	}
}
