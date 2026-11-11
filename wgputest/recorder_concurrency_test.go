package wgputest

import (
	"sync"
	"testing"
)

// TestRecorderConcurrentUse exercises the documented concurrency safety: the
// recorder and Trace can be read while records are appended. CI runs it under
// -race.
func TestRecorderConcurrentUse(t *testing.T) {
	recorder := NewRecorder()
	const goroutines = 8
	const iterations = 100

	var wg sync.WaitGroup
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range iterations {
				recorder.record(func(trace *Trace) {
					trace.Draws = append(trace.Draws, Draw{Seq: uint64(j)})
				})
				_ = recorder.Trace()
			}
		}()
	}
	wg.Wait()

	if got, want := len(recorder.Trace().Draws), goroutines*iterations; got != want {
		t.Errorf("draws = %d, want %d", got, want)
	}
}
