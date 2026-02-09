package core

import (
	"sync"
	"testing"
)

// TestConcurrentLeakTracking verifies that trackResource and untrackResource
// are safe for concurrent calls from multiple goroutines.
func TestConcurrentLeakTracking(t *testing.T) {
	SetDebugMode(true)
	defer func() {
		SetDebugMode(false)
		ResetLeakTracker()
	}()
	ResetLeakTracker()

	const numGoroutines = 50
	var wg sync.WaitGroup

	// Concurrently track resources
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			handle := uintptr(0x1000 + id)
			trackResource(handle, "Buffer")
		}(i)
	}
	wg.Wait()

	report := ReportLeaks()
	if report == nil {
		t.Fatal("expected leak report after concurrent tracking")
	}
	if report.Count != numGoroutines {
		t.Errorf("expected %d leaks, got %d", numGoroutines, report.Count)
	}

	// Concurrently untrack resources
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			handle := uintptr(0x1000 + id)
			untrackResource(handle)
		}(i)
	}
	wg.Wait()

	report = ReportLeaks()
	if report != nil {
		t.Errorf("expected nil report after concurrent untracking, got %v", report)
	}
}

// TestConcurrentLeakTrackingMixed verifies mixed track/untrack operations
// from multiple goroutines don't cause data races.
func TestConcurrentLeakTrackingMixed(t *testing.T) {
	SetDebugMode(true)
	defer func() {
		SetDebugMode(false)
		ResetLeakTracker()
	}()
	ResetLeakTracker()

	const numGoroutines = 100
	var wg sync.WaitGroup

	// Mix of tracking and untracking
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			handle := uintptr(0x2000 + id)
			trackResource(handle, "Texture")
			// Immediately untrack
			untrackResource(handle)
		}(i)
	}
	wg.Wait()

	report := ReportLeaks()
	if report != nil {
		t.Errorf("expected nil report after track+untrack pairs, got count=%d", report.Count)
	}
}

// TestConcurrentDebugModeToggle verifies that SetDebugMode and DebugMode
// are safe for concurrent access.
func TestConcurrentDebugModeToggle(t *testing.T) {
	defer SetDebugMode(false)

	const numGoroutines = 50
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			if id%2 == 0 {
				SetDebugMode(true)
			} else {
				SetDebugMode(false)
			}
			_ = DebugMode()
		}(i)
	}
	wg.Wait()
}

// TestConcurrentReportLeaks verifies that ReportLeaks is safe for
// concurrent calls alongside track/untrack operations.
func TestConcurrentReportLeaks(t *testing.T) {
	SetDebugMode(true)
	defer func() {
		SetDebugMode(false)
		ResetLeakTracker()
	}()
	ResetLeakTracker()

	const numGoroutines = 30
	var wg sync.WaitGroup

	// Some goroutines track, others report
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			switch id % 3 {
			case 0:
				trackResource(uintptr(0x3000+id), "Buffer")
			case 1:
				_ = ReportLeaks()
			default:
				ResetLeakTracker()
			}
		}(i)
	}
	wg.Wait()
}

// TestConcurrentErrorScopes verifies that ErrorScopeManager is safe for
// concurrent push/pop/report operations.
func TestConcurrentErrorScopes(t *testing.T) {
	mgr := NewErrorScopeManager()

	const numGoroutines = 50
	var wg sync.WaitGroup

	// Phase 1: concurrent push
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			filter := ErrorFilter(id % 3) // Validation, OutOfMemory, Internal
			mgr.PushErrorScope(filter)
		}(i)
	}
	wg.Wait()

	if mgr.ScopeDepth() != numGoroutines {
		t.Errorf("ScopeDepth = %d, want %d after concurrent push", mgr.ScopeDepth(), numGoroutines)
	}

	// Phase 2: concurrent report
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			filter := ErrorFilter(id % 3)
			mgr.ReportError(filter, "concurrent error")
		}(i)
	}
	wg.Wait()

	// Phase 3: concurrent pop
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = mgr.PopErrorScope()
		}()
	}
	wg.Wait()

	if mgr.ScopeDepth() != 0 {
		t.Errorf("ScopeDepth = %d, want 0 after concurrent pop", mgr.ScopeDepth())
	}
}

// TestConcurrentErrorScopeManagerCreation verifies creating multiple
// ErrorScopeManagers concurrently is safe.
func TestConcurrentErrorScopeManagerCreation(t *testing.T) {
	const numGoroutines = 20
	managers := make([]*ErrorScopeManager, numGoroutines)
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			mgr := NewErrorScopeManager()
			mgr.PushErrorScope(ErrorFilterValidation)
			mgr.ReportError(ErrorFilterValidation, "test error")
			gpuErr, err := mgr.PopErrorScope()
			if err != nil {
				t.Errorf("goroutine %d: PopErrorScope error: %v", id, err)
			}
			if gpuErr == nil {
				t.Errorf("goroutine %d: expected error, got nil", id)
			}
			managers[id] = mgr
		}(i)
	}
	wg.Wait()
}

// TestConcurrentInstanceCreation verifies that creating multiple instances
// concurrently is safe (each with its own mock adapter).
func TestConcurrentInstanceCreation(t *testing.T) {
	SetDebugMode(true)
	defer func() {
		SetDebugMode(false)
		ResetLeakTracker()
	}()
	ResetLeakTracker()

	const numGoroutines = 10
	instances := make([]*Instance, numGoroutines)
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			inst := NewInstanceWithMock(nil)
			instances[id] = inst
		}(i)
	}
	wg.Wait()

	// Verify all instances were created
	for i, inst := range instances {
		if inst == nil {
			t.Errorf("instance %d is nil", i)
			continue
		}
		if !inst.IsMock() {
			t.Errorf("instance %d should be mock", i)
		}
	}

	// Concurrently destroy
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			instances[id].Destroy()
		}(i)
	}
	wg.Wait()
}

// TestConcurrentAdapterRequests verifies concurrent adapter requests
// on a single instance are safe.
func TestConcurrentAdapterRequests(t *testing.T) {
	inst := NewInstanceWithMock(nil)
	defer inst.Destroy()

	const numGoroutines = 20
	var wg sync.WaitGroup
	errs := make([]error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			_, err := inst.RequestAdapter(nil)
			errs[id] = err
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("adapter request %d failed: %v", i, err)
		}
	}
}

// TestConcurrentEnumerateAdapters verifies concurrent EnumerateAdapters
// calls are safe.
func TestConcurrentEnumerateAdapters(t *testing.T) {
	inst := NewInstanceWithMock(nil)
	defer inst.Destroy()

	const numGoroutines = 20
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			adapters := inst.EnumerateAdapters()
			if len(adapters) == 0 {
				t.Error("expected at least one adapter")
			}
		}()
	}
	wg.Wait()
}
