//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package pipeline

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gogpu/wgpu/internal/core"
)

// pipelineReleaseCase builds one pipeline wrapper over a reference counted
// from two owners (the application's and one recorded use), so a second Drop
// is visible: it takes the count to zero and fires the destroy callback,
// which only the last legitimate Drop may do.
type pipelineReleaseCase struct {
	name  string
	build func(ref *core.ResourceRef) func()
}

func pipelineReleaseCases() []pipelineReleaseCase {
	return []pipelineReleaseCase{
		{
			name: "RenderPipeline",
			build: func(ref *core.ResourceRef) func() {
				return NewRenderPipeline(&RenderPipelineInit{Ref: ref}).Release
			},
		},
		{
			name: "ComputePipeline",
			build: func(ref *core.ResourceRef) func() {
				return NewComputePipeline(&ComputePipelineInit{Ref: ref}).Release
			},
		},
	}
}

// raceRelease starts goroutines callers and has them all call release from a
// shared start barrier, so the calls overlap in one window.
func raceRelease(goroutines int, release func()) {
	start := make(chan struct{})
	var ready, done sync.WaitGroup
	ready.Add(goroutines)
	done.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer done.Done()
			ready.Done()
			<-start
			release()
		}()
	}
	ready.Wait()
	close(start)
	done.Wait()
}

// TestConcurrentPipelineReleaseDropsRefOnce proves that concurrent Release
// calls drop the pipeline's reference exactly once: a second Drop would take
// the count to zero and fire the destroy callback before the test drops the
// last legitimate reference itself.
func TestConcurrentPipelineReleaseDropsRefOnce(t *testing.T) {
	for _, tc := range pipelineReleaseCases() {
		t.Run(tc.name, func(t *testing.T) {
			// A wide fan-out widens the check-then-set window the guard must
			// close: with the fix the assertion is exact, without it the
			// double Drop shows up as a refcount that reaches zero early.
			for _, goroutines := range []int{2, 64} {
				for i := 0; i < 1000; i++ {
					var destroyed atomic.Int32
					ref := core.NewResourceRef(tc.name, func() { destroyed.Add(1) })
					ref.Clone() // the application's reference plus one recorded use
					release := tc.build(ref)

					raceRelease(goroutines, release)

					if got := ref.RefCount(); got != 1 {
						t.Fatalf("iteration %d: %d concurrent Release calls left refcount %d, want 1",
							i, goroutines, got)
					}
					if got := destroyed.Load(); got != 0 {
						t.Fatalf("iteration %d: the destroy callback fired %d times before the last reference dropped, want 0",
							i, got)
					}
					ref.Drop()
					if got := destroyed.Load(); got != 1 {
						t.Fatalf("iteration %d: the destroy callback fired %d times after the last Drop, want 1", i, got)
					}
				}
			}
		})
	}
}

// TestZeroValuePipelineReleaseIsNoOp covers the nil-guard contract for the
// pipeline wrappers: a zero value has no guard and Release is a no-op.
func TestZeroValuePipelineReleaseIsNoOp(t *testing.T) {
	(&RenderPipeline{}).Release()
	(&ComputePipeline{}).Release()
}
