package core

import (
	"sync"
)

// DeferredDestroy represents a resource destruction that is deferred until the
// GPU completes the submission that was active when the resource was released.
type DeferredDestroy struct {
	submissionIndex uint64 // GPU submission that must complete before destroy
	destroyFn       func() // HAL destroy callback
	label           string // debug label for logging
}

// TrackedSubmission holds Clone'd ResourceRefs associated with a GPU submission.
// When the GPU completes this submission (index <= completedIndex), all refs
// are Drop()'d, decrementing their reference counts and potentially triggering
// destruction when no other owners remain.
//
// This is the Phase 2 per-command-buffer tracking mechanism: resources are
// Clone'd when used during encoding, transferred here on Submit, and Drop'd
// when the GPU finishes.
type TrackedSubmission struct {
	index uint64
	refs  []*ResourceRef
}

// DestroyQueue tracks in-flight GPU submissions and defers resource destruction
// until GPU completion. This prevents use-after-free when the application
// releases a resource while the GPU is still using it in a submitted command.
//
// Matches Rust wgpu-core's LifetimeTracker pattern where resource Drop only
// fires after triage_submissions confirms the fence has passed.
//
// Usage:
//  1. On resource Release(): call Defer(lastSubmissionIndex, destroyFn)
//  2. On Queue.Submit(): call Triage(PollCompleted()) to destroy completed resources
//  3. On Device.Release(): call FlushAll() to destroy everything
//
// Phase 2 adds TrackSubmission() for per-command-buffer resource tracking:
//
//	On Queue.Submit(): call TrackSubmission(subIdx, refs) with Clone'd refs
//	On Triage(): completed submissions' refs are Drop()'d
//
// Thread-safe for concurrent use.
type DestroyQueue struct {
	mu      sync.Mutex
	pending []DeferredDestroy
	tracked []TrackedSubmission
}

// NewDestroyQueue creates a new DestroyQueue.
func NewDestroyQueue() *DestroyQueue {
	return &DestroyQueue{}
}

// Defer schedules a resource for destruction after the GPU completes the
// submission identified by index. The destroyFn callback (typically calling
// halDevice.DestroyBuffer, DestroyTexture, etc.) will be invoked by a future
// Triage() call once completedIndex >= index.
//
// Parameters:
//   - index: the submission index that must complete before destruction
//   - label: debug label for the resource (for logging)
//   - fn: callback that performs the actual HAL destruction
func (q *DestroyQueue) Defer(index uint64, label string, fn func()) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.pending = append(q.pending, DeferredDestroy{
		submissionIndex: index,
		destroyFn:       fn,
		label:           label,
	})
}

// TrackSubmission associates Clone'd ResourceRefs with a GPU submission.
// When Triage() confirms the submission has completed, all refs are Drop()'d.
//
// This is the Phase 2 per-command-buffer tracking entry point. Called by
// Queue.Submit() after HAL submit succeeds.
//
// Parameters:
//   - index: the submission index from HAL Queue.Submit()
//   - refs: Clone'd ResourceRef pointers from command encoding
func (q *DestroyQueue) TrackSubmission(index uint64, refs []*ResourceRef) {
	if len(refs) == 0 {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	q.tracked = append(q.tracked, TrackedSubmission{index: index, refs: refs})
}

// Triage checks which deferred destructions can now proceed because their
// associated GPU submissions have completed (submissionIndex <= completedIndex).
// Resources whose submissions are still in-flight are retained.
//
// Also Drop()'s ResourceRefs from completed TrackedSubmissions (Phase 2).
//
// This should be called after each Queue.Submit() with the result of
// hal.Queue.PollCompleted().
func (q *DestroyQueue) Triage(completedIndex uint64) {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Triage deferred destroys (Phase 1).
	n := 0
	for i := range q.pending {
		if q.pending[i].submissionIndex <= completedIndex {
			q.pending[i].destroyFn()
		} else {
			q.pending[n] = q.pending[i]
			n++
		}
	}
	// Clear references in the tail to avoid retaining destroy closures.
	for i := n; i < len(q.pending); i++ {
		q.pending[i] = DeferredDestroy{}
	}
	q.pending = q.pending[:n]

	// Triage tracked submissions (Phase 2).
	tn := 0
	for i := range q.tracked {
		if q.tracked[i].index <= completedIndex {
			for _, ref := range q.tracked[i].refs {
				ref.Drop()
			}
		} else {
			q.tracked[tn] = q.tracked[i]
			tn++
		}
	}
	for i := tn; i < len(q.tracked); i++ {
		q.tracked[i] = TrackedSubmission{}
	}
	q.tracked = q.tracked[:tn]
}

// FlushAll destroys all pending resources regardless of GPU completion status.
// Called during device shutdown when all GPU work is (or should be) complete.
// Also Drop()'s all tracked submission refs (Phase 2).
func (q *DestroyQueue) FlushAll() {
	q.mu.Lock()
	defer q.mu.Unlock()

	for i := range q.pending {
		q.pending[i].destroyFn()
	}
	q.pending = nil

	for i := range q.tracked {
		for _, ref := range q.tracked[i].refs {
			ref.Drop()
		}
	}
	q.tracked = nil
}

// Len returns the number of pending deferred destructions. For testing only.
func (q *DestroyQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.pending)
}

// TrackedLen returns the number of tracked submissions. For testing only.
func (q *DestroyQueue) TrackedLen() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.tracked)
}
