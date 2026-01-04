package core

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// =============================================================================
// Snatchable[T] Tests
// =============================================================================

// testResource is a simple test resource type.
type testSnatchResource struct {
	name  string
	value int
}

func TestSnatchable_NewAndGet(t *testing.T) {
	res := testSnatchResource{name: "test", value: 42}
	s := NewSnatchable(res)

	lock := NewSnatchLock()
	guard := lock.Read()
	defer guard.Release()

	got := s.Get(guard)
	if got == nil {
		t.Fatal("Get() returned nil, want non-nil")
	}
	if got.name != res.name {
		t.Errorf("Get().name = %q, want %q", got.name, res.name)
	}
	if got.value != res.value {
		t.Errorf("Get().value = %d, want %d", got.value, res.value)
	}
}

func TestSnatchable_GetAfterSnatch(t *testing.T) {
	res := testSnatchResource{name: "test", value: 42}
	s := NewSnatchable(res)

	lock := NewSnatchLock()

	// Snatch the value
	writeGuard := lock.Write()
	snatched := s.Snatch(writeGuard)
	writeGuard.Release()

	if snatched == nil {
		t.Fatal("Snatch() returned nil, want non-nil")
	}

	// Try to get after snatch
	readGuard := lock.Read()
	defer readGuard.Release()

	got := s.Get(readGuard)
	if got != nil {
		t.Errorf("Get() after Snatch() = %+v, want nil", got)
	}
}

func TestSnatchable_Snatch(t *testing.T) {
	res := testSnatchResource{name: "snatchable", value: 100}
	s := NewSnatchable(res)

	lock := NewSnatchLock()
	guard := lock.Write()
	defer guard.Release()

	got := s.Snatch(guard)
	if got == nil {
		t.Fatal("Snatch() returned nil, want non-nil")
	}
	if got.name != res.name {
		t.Errorf("Snatch().name = %q, want %q", got.name, res.name)
	}
	if got.value != res.value {
		t.Errorf("Snatch().value = %d, want %d", got.value, res.value)
	}
}

func TestSnatchable_SnatchTwice(t *testing.T) {
	res := testSnatchResource{name: "once", value: 1}
	s := NewSnatchable(res)

	lock := NewSnatchLock()

	// First snatch should succeed
	guard1 := lock.Write()
	first := s.Snatch(guard1)
	guard1.Release()

	if first == nil {
		t.Fatal("First Snatch() returned nil, want non-nil")
	}

	// Second snatch should return nil
	guard2 := lock.Write()
	second := s.Snatch(guard2)
	guard2.Release()

	if second != nil {
		t.Errorf("Second Snatch() = %+v, want nil", second)
	}
}

func TestSnatchable_IsSnatched(t *testing.T) {
	res := testSnatchResource{name: "check", value: 5}
	s := NewSnatchable(res)

	// Initially not snatched
	if s.IsSnatched() {
		t.Error("IsSnatched() before snatch = true, want false")
	}

	lock := NewSnatchLock()
	guard := lock.Write()
	_ = s.Snatch(guard)
	guard.Release()

	// After snatch
	if !s.IsSnatched() {
		t.Error("IsSnatched() after snatch = false, want true")
	}
}

func TestSnatchable_GetReturnsPointerToSameValue(t *testing.T) {
	res := testSnatchResource{name: "ptr", value: 10}
	s := NewSnatchable(res)

	lock := NewSnatchLock()

	// Get twice and verify same pointer
	guard1 := lock.Read()
	ptr1 := s.Get(guard1)
	guard1.Release()

	guard2 := lock.Read()
	ptr2 := s.Get(guard2)
	guard2.Release()

	if ptr1 != ptr2 {
		t.Error("Get() returned different pointers for same value")
	}
}

// =============================================================================
// SnatchLock Tests
// =============================================================================

func TestSnatchLock_Read(t *testing.T) {
	lock := NewSnatchLock()

	guard := lock.Read()
	if guard == nil {
		t.Fatal("Read() returned nil guard")
	}
	if guard.released {
		t.Error("New guard already released")
	}

	guard.Release()
	if !guard.released {
		t.Error("Guard not marked as released after Release()")
	}
}

func TestSnatchLock_Write(t *testing.T) {
	lock := NewSnatchLock()

	guard := lock.Write()
	if guard == nil {
		t.Fatal("Write() returned nil guard")
	}
	if guard.released {
		t.Error("New guard already released")
	}

	guard.Release()
	if !guard.released {
		t.Error("Guard not marked as released after Release()")
	}
}

func TestSnatchLock_MultipleReaders(t *testing.T) {
	lock := NewSnatchLock()
	const readers = 10

	// Acquire multiple read guards
	guards := make([]*SnatchGuard, readers)
	for i := 0; i < readers; i++ {
		guards[i] = lock.Read()
	}

	// All should be acquired (not blocked)
	for i, g := range guards {
		if g == nil {
			t.Errorf("Reader %d: guard is nil", i)
		}
	}

	// Release all
	for _, g := range guards {
		g.Release()
	}
}

// =============================================================================
// Guard Tests
// =============================================================================

func TestSnatchGuard_Release(t *testing.T) {
	lock := NewSnatchLock()
	guard := lock.Read()

	// First release
	guard.Release()
	if !guard.released {
		t.Error("Guard not released after Release()")
	}

	// Second release should be no-op (no panic)
	guard.Release() // Should not panic
	if !guard.released {
		t.Error("Guard should stay released")
	}
}

func TestExclusiveSnatchGuard_Release(t *testing.T) {
	lock := NewSnatchLock()
	guard := lock.Write()

	// First release
	guard.Release()
	if !guard.released {
		t.Error("Guard not released after Release()")
	}

	// Second release should be no-op (no panic)
	guard.Release() // Should not panic
	if !guard.released {
		t.Error("Guard should stay released")
	}
}

func TestSnatchGuard_DoubleReleaseIsSafe(t *testing.T) {
	lock := NewSnatchLock()
	guard := lock.Read()

	// Multiple releases should be safe
	guard.Release()
	guard.Release()
	guard.Release()

	// Should be able to acquire again
	newGuard := lock.Read()
	newGuard.Release()
}

func TestExclusiveSnatchGuard_DoubleReleaseIsSafe(t *testing.T) {
	lock := NewSnatchLock()
	guard := lock.Write()

	// Multiple releases should be safe
	guard.Release()
	guard.Release()
	guard.Release()

	// Should be able to acquire again
	newGuard := lock.Write()
	newGuard.Release()
}

// =============================================================================
// Concurrent Access Tests
// =============================================================================

func TestSnatchable_ConcurrentReads(t *testing.T) {
	res := testSnatchResource{name: "concurrent", value: 999}
	s := NewSnatchable(res)
	lock := NewSnatchLock()

	const goroutines = 100
	const readsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	errors := make(chan error, goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < readsPerGoroutine; j++ {
				guard := lock.Read()
				got := s.Get(guard)
				if got == nil {
					errors <- nil // Expected if snatched, but we don't snatch here
					guard.Release()
					return
				}
				if got.value != res.value {
					t.Errorf("Concurrent read got value %d, want %d", got.value, res.value)
				}
				guard.Release()
			}
		}()
	}

	wg.Wait()
	close(errors)

	// Check for any errors
	for err := range errors {
		if err != nil {
			t.Errorf("Concurrent read error: %v", err)
		}
	}
}

func TestSnatchable_ConcurrentSnatch(t *testing.T) {
	res := testSnatchResource{name: "race", value: 42}
	s := NewSnatchable(res)
	lock := NewSnatchLock()

	const goroutines = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	var successCount atomic.Int32

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			guard := lock.Write()
			result := s.Snatch(guard)
			guard.Release()

			if result != nil {
				successCount.Add(1)
			}
		}()
	}

	wg.Wait()

	// Exactly one goroutine should have succeeded
	if count := successCount.Load(); count != 1 {
		t.Errorf("Snatch succeeded %d times, want exactly 1", count)
	}

	// Verify snatched
	if !s.IsSnatched() {
		t.Error("IsSnatched() = false after concurrent snatch")
	}
}

func TestSnatchable_ConcurrentReadAndSnatch(t *testing.T) {
	res := testSnatchResource{name: "mixed", value: 123}
	s := NewSnatchable(res)
	lock := NewSnatchLock()

	const readers = 50
	const snatchers = 10

	var wg sync.WaitGroup
	wg.Add(readers + snatchers)

	var snatchSuccess atomic.Int32
	var readSuccess atomic.Int32
	var readNil atomic.Int32

	// Start readers
	for i := 0; i < readers; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				guard := lock.Read()
				result := s.Get(guard)
				if result != nil {
					readSuccess.Add(1)
				} else {
					readNil.Add(1)
				}
				guard.Release()
			}
		}()
	}

	// Start snatchers
	for i := 0; i < snatchers; i++ {
		go func() {
			defer wg.Done()
			guard := lock.Write()
			result := s.Snatch(guard)
			guard.Release()
			if result != nil {
				snatchSuccess.Add(1)
			}
		}()
	}

	wg.Wait()

	// Exactly one snatch should succeed
	if count := snatchSuccess.Load(); count != 1 {
		t.Errorf("Snatch succeeded %d times, want exactly 1", count)
	}

	// Should be snatched
	if !s.IsSnatched() {
		t.Error("IsSnatched() = false after mixed concurrent access")
	}

	t.Logf("Stats: readSuccess=%d, readNil=%d, snatchSuccess=%d",
		readSuccess.Load(), readNil.Load(), snatchSuccess.Load())
}

func TestSnatchLock_ReadWriteExclusion(t *testing.T) {
	lock := NewSnatchLock()

	// This test verifies that write lock blocks readers
	var sequence []string
	var mu sync.Mutex

	record := func(event string) {
		mu.Lock()
		sequence = append(sequence, event)
		mu.Unlock()
	}

	var wg sync.WaitGroup
	wg.Add(2)

	// Acquire write lock
	writeGuard := lock.Write()
	record("write-acquired")

	// Start a reader (will block)
	go func() {
		defer wg.Done()
		record("reader-waiting")
		readGuard := lock.Read()
		record("reader-acquired")
		readGuard.Release()
		record("reader-released")
	}()

	// Give reader time to start waiting
	// Note: This is a timing-based test, not ideal but demonstrates the pattern
	time.Sleep(10 * time.Millisecond)

	// Release write lock
	writeGuard.Release()
	record("write-released")

	// Start another operation to ensure sequence completes
	go func() {
		defer wg.Done()
		guard := lock.Read()
		guard.Release()
	}()

	wg.Wait()

	// Verify write-released happens before reader-acquired
	mu.Lock()
	defer mu.Unlock()

	writeReleasedIdx := -1
	readerAcquiredIdx := -1
	for i, event := range sequence {
		if event == "write-released" {
			writeReleasedIdx = i
		}
		if event == "reader-acquired" {
			readerAcquiredIdx = i
		}
	}

	if writeReleasedIdx == -1 {
		t.Error("write-released not recorded")
	}
	if readerAcquiredIdx == -1 {
		t.Error("reader-acquired not recorded")
	}
	if readerAcquiredIdx < writeReleasedIdx {
		t.Errorf("reader-acquired (%d) before write-released (%d)",
			readerAcquiredIdx, writeReleasedIdx)
	}
}

// =============================================================================
// Edge Cases
// =============================================================================

func TestSnatchable_ZeroValue(t *testing.T) {
	// Test with zero value struct
	var zero testSnatchResource
	s := NewSnatchable(zero)

	lock := NewSnatchLock()
	guard := lock.Read()
	defer guard.Release()

	got := s.Get(guard)
	if got == nil {
		t.Fatal("Get() returned nil for zero value")
	}
	if got.name != "" {
		t.Errorf("Get().name = %q, want empty string", got.name)
	}
	if got.value != 0 {
		t.Errorf("Get().value = %d, want 0", got.value)
	}
}

func TestSnatchable_PointerType(t *testing.T) {
	// Test with pointer type
	original := &testSnatchResource{name: "ptr", value: 777}
	s := NewSnatchable(original)

	lock := NewSnatchLock()
	guard := lock.Read()
	got := s.Get(guard)
	guard.Release()

	if got == nil {
		t.Fatal("Get() returned nil")
	}

	// got is *(*testSnatchResource) = **testSnatchResource
	// The value stored is the pointer itself
	if *got != original {
		t.Error("Pointer value mismatch")
	}
}

func TestSnatchable_LargeStruct(t *testing.T) {
	type largeStruct struct {
		data [1024]byte
		name string
	}

	large := largeStruct{name: "large"}
	for i := range large.data {
		large.data[i] = byte(i % 256)
	}

	s := NewSnatchable(large)

	lock := NewSnatchLock()

	// Read
	readGuard := lock.Read()
	got := s.Get(readGuard)
	readGuard.Release()

	if got == nil {
		t.Fatal("Get() returned nil")
	}
	if got.name != large.name {
		t.Errorf("name = %q, want %q", got.name, large.name)
	}

	// Snatch
	writeGuard := lock.Write()
	snatched := s.Snatch(writeGuard)
	writeGuard.Release()

	if snatched == nil {
		t.Fatal("Snatch() returned nil")
	}
	if snatched.name != large.name {
		t.Errorf("snatched name = %q, want %q", snatched.name, large.name)
	}
}

// =============================================================================
// Benchmarks
// =============================================================================

func BenchmarkSnatchable_Get(b *testing.B) {
	res := testSnatchResource{name: "bench", value: 42}
	s := NewSnatchable(res)
	lock := NewSnatchLock()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		guard := lock.Read()
		_ = s.Get(guard)
		guard.Release()
	}
}

func BenchmarkSnatchable_IsSnatched(b *testing.B) {
	res := testSnatchResource{name: "bench", value: 42}
	s := NewSnatchable(res)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = s.IsSnatched()
	}
}

func BenchmarkSnatchLock_ReadRelease(b *testing.B) {
	lock := NewSnatchLock()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		guard := lock.Read()
		guard.Release()
	}
}

func BenchmarkSnatchLock_WriteRelease(b *testing.B) {
	lock := NewSnatchLock()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		guard := lock.Write()
		guard.Release()
	}
}

func BenchmarkSnatchable_Get_Parallel(b *testing.B) {
	res := testSnatchResource{name: "parallel", value: 100}
	s := NewSnatchable(res)
	lock := NewSnatchLock()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			guard := lock.Read()
			_ = s.Get(guard)
			guard.Release()
		}
	})
}
