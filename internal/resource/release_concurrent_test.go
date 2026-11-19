//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package resource

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/naga/valid"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/hal"
)

// destroySpy is a HAL device that counts every destroy call. The embedded
// hal.Device is nil on purpose: only the destroy methods the test reaches are
// declared, so any other HAL call panics — which surfaces an unexpected path
// instead of hiding it.
type destroySpy struct {
	hal.Device
	samplers         atomic.Int32
	textures         atomic.Int32
	views            atomic.Int32
	modules          atomic.Int32
	queries          atomic.Int32
	fences           atomic.Int32
	bindGroupLayouts atomic.Int32
	pipelineLayouts  atomic.Int32
}

func (d *destroySpy) DestroySampler(hal.Sampler)                 { d.samplers.Add(1) }
func (d *destroySpy) DestroyTexture(hal.Texture)                 { d.textures.Add(1) }
func (d *destroySpy) DestroyTextureView(hal.TextureView)         { d.views.Add(1) }
func (d *destroySpy) DestroyShaderModule(hal.ShaderModule)       { d.modules.Add(1) }
func (d *destroySpy) DestroyQuerySet(hal.QuerySet)               { d.queries.Add(1) }
func (d *destroySpy) DestroyFence(hal.Fence)                     { d.fences.Add(1) }
func (d *destroySpy) DestroyBindGroupLayout(hal.BindGroupLayout) { d.bindGroupLayouts.Add(1) }
func (d *destroySpy) DestroyPipelineLayout(hal.PipelineLayout)   { d.pipelineLayouts.Add(1) }

// spyDeviceView is the core.DeviceView the resource wrappers see. It reports
// no destroy queue, so every Release destroys inline through destroySpy and
// the counters see each destroy synchronously.
type spyDeviceView struct {
	halDevice hal.Device
	coreDev   *core.Device
}

func (d *spyDeviceView) Core() *core.Device               { return d.coreDev }
func (d *spyDeviceView) IsReleased() bool                 { return false }
func (d *spyDeviceView) HALDevice() hal.Device            { return d.halDevice }
func (d *spyDeviceView) DestroyQueue() *core.DestroyQueue { return nil }
func (d *spyDeviceView) LastSubmissionIndex() uint64      { return 0 }
func (d *spyDeviceView) Poll(core.PollType) bool          { return false }

// releaseCase builds one covered resource type over a fresh spy device and
// reports how many times that type's HAL destroy ran.
type releaseCase struct {
	name  string
	count func(*destroySpy) int32
	build func(*spyDeviceView) func()
}

func releaseCases() []releaseCase {
	return []releaseCase{
		{
			name:  "Sampler",
			count: func(s *destroySpy) int32 { return s.samplers.Load() },
			build: func(d *spyDeviceView) func() {
				s := NewSampler(nil, d, nil, gputypes.FilterModeNearest, gputypes.FilterModeNearest, gputypes.FilterModeNearest)
				return s.Release
			},
		},
		{
			name:  "Texture",
			count: func(s *destroySpy) int32 { return s.textures.Load() },
			build: func(d *spyDeviceView) func() {
				t := NewTexture(nil, d, gputypes.TextureFormatRGBA8Unorm, nil, nil)
				return t.Release
			},
		},
		{
			name:  "TextureView",
			count: func(s *destroySpy) int32 { return s.views.Load() },
			build: func(d *spyDeviceView) func() {
				v := NewTextureView(&TextureViewInit{Device: d})
				return v.Release
			},
		},
		{
			name:  "ShaderModule",
			count: func(s *destroySpy) int32 { return s.modules.Load() },
			build: func(d *spyDeviceView) func() {
				m := NewShaderModule(nil, d, nil, valid.ModuleInfo{})
				return m.Release
			},
		},
		{
			name:  "QuerySet",
			count: func(s *destroySpy) int32 { return s.queries.Load() },
			build: func(d *spyDeviceView) func() {
				// A zero-limits device skips indirect validation creation, so
				// the spy sees no HAL calls at construction.
				cd := core.NewDevice(d.halDevice, nil, 0, gputypes.Limits{}, "qs")
				qs := NewQuerySet(core.NewQuerySet(nil, cd, hal.QueryTypeOcclusion, 1, "qs"), d)
				return qs.Release
			},
		},
		{
			name:  "Fence",
			count: func(s *destroySpy) int32 { return s.fences.Load() },
			build: func(d *spyDeviceView) func() {
				f := NewFence(nil, d)
				return f.Release
			},
		},
		{
			name:  "BindGroupLayout",
			count: func(s *destroySpy) int32 { return s.bindGroupLayouts.Load() },
			build: func(d *spyDeviceView) func() {
				l := NewBindGroupLayout(nil, d, nil)
				return l.Release
			},
		},
		{
			name:  "PipelineLayout",
			count: func(s *destroySpy) int32 { return s.pipelineLayouts.Load() },
			build: func(d *spyDeviceView) func() {
				l := NewPipelineLayout(nil, d, 0, nil, nil)
				return l.Release
			},
		},
	}
}

// concurrentReleaseIterations is the number of fresh instances per goroutine
// count. Every instance is a new check-then-set window, so the count is the
// product of independent attempts to widen the window rather than one race.
const concurrentReleaseIterations = 500

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

// TestConcurrentReleaseDestroysOnce proves that concurrent Release calls on
// one resource destroy the HAL object at most — and here exactly — once. Two
// callers and eight callers are both covered.
func TestConcurrentReleaseDestroysOnce(t *testing.T) {
	for _, tc := range releaseCases() {
		t.Run(tc.name, func(t *testing.T) {
			runConcurrentReleases(t, tc)
		})
	}
}

func runConcurrentReleases(t *testing.T, tc releaseCase) {
	t.Helper()
	for _, goroutines := range []int{2, 8} {
		for i := 0; i < concurrentReleaseIterations; i++ {
			spy := &destroySpy{}
			release := tc.build(&spyDeviceView{halDevice: spy})

			raceRelease(goroutines, release)

			if got := tc.count(spy); got != 1 {
				t.Fatalf("iteration %d: %d concurrent Release calls destroyed %d times, want exactly 1",
					i, goroutines, got)
			}
		}
	}
}

// TestConcurrentQuerySetReleaseDropsRefOnce proves the query set's guard by
// its reference drop: the HAL destroy itself is snatch-idempotent in core, so
// a second Release would be invisible there, while a second Drop of the
// resource' reference count takes it to zero and fires the destroy callback.
func TestConcurrentQuerySetReleaseDropsRefOnce(t *testing.T) {
	for _, goroutines := range []int{2, 64} {
		for i := 0; i < 1000; i++ {
			var destroyed atomic.Int32
			ref := core.NewResourceRef("QuerySet", func() { destroyed.Add(1) })
			ref.Clone() // the application's reference plus one recorded use

			cd := core.NewDevice(&destroySpy{}, nil, 0, gputypes.Limits{}, "qs")
			coreQS := core.NewQuerySet(nil, cd, hal.QueryTypeOcclusion, 1, "qs")
			coreQS.Ref = ref
			qs := NewQuerySet(coreQS, &spyDeviceView{halDevice: &destroySpy{}})

			raceRelease(goroutines, qs.Release)

			if got := ref.RefCount(); got != 1 {
				t.Fatalf("iteration %d: %d concurrent Release calls left refcount %d, want 1", i, goroutines, got)
			}
			if got := destroyed.Load(); got != 0 {
				t.Fatalf("iteration %d: the destroy callback fired %d times before the last reference dropped, want 0", i, got)
			}
			ref.Drop()
			if got := destroyed.Load(); got != 1 {
				t.Fatalf("iteration %d: the destroy callback fired %d times after the last Drop, want 1", i, got)
			}
		}
	}
}

// poolReleaseCases covers the pooled handles, whose Release returns the
// handle to its pool: the CAS guard must admit exactly one return per
// acquisition (a double Put would hand one object to two owners).
type poolReleaseCase struct {
	name    string
	acquire func() func()
	puts    func() int64
}

func poolReleaseCases() []poolReleaseCase {
	return []poolReleaseCase{
		{
			name:    "MappedRange",
			acquire: func() func() { return acquireMappedRange().Release },
			puts:    func() int64 { return mappedRangePool.puts.Load() },
		},
		{
			name:    "MapPending",
			acquire: func() func() { return acquireMapPending(nil, nil).Release },
			puts:    func() int64 { return mapPendingPool.puts.Load() },
		},
	}
}

// TestConcurrentPoolReleasePutsOnce proves that concurrent Release calls on a
// pooled handle return it to the pool exactly once, so no object is ever
// handed to two owners.
func TestConcurrentPoolReleasePutsOnce(t *testing.T) {
	for _, tc := range poolReleaseCases() {
		t.Run(tc.name, func(t *testing.T) {
			for _, goroutines := range []int{2, 8} {
				for i := 0; i < concurrentReleaseIterations; i++ {
					before := tc.puts()
					release := tc.acquire()
					raceRelease(goroutines, release)
					if got := tc.puts() - before; got != 1 {
						t.Fatalf("iteration %d: %d concurrent Release calls returned the handle to the pool %d times, want exactly 1",
							i, goroutines, got)
					}
				}
			}
		})
	}
}

// TestZeroValueReleaseIsNoOp covers the nil-guard contract: a zero-value
// resource has no guard, and Release on it must return without panicking and
// without touching a HAL object.
func TestZeroValueReleaseIsNoOp(t *testing.T) {
	releases := map[string]func(){
		"Sampler":         func() { (&Sampler{}).Release() },
		"Texture":         func() { (&Texture{}).Release() },
		"TextureView":     func() { (&TextureView{}).Release() },
		"ShaderModule":    func() { (&ShaderModule{}).Release() },
		"QuerySet":        func() { (&QuerySet{}).Release() },
		"Fence":           func() { (&Fence{}).Release() },
		"BindGroupLayout": func() { (&BindGroupLayout{}).Release() },
		"PipelineLayout":  func() { (&PipelineLayout{}).Release() },
		"MappedRange":     func() { (&MappedRange{}).Release() },
		"MapPending":      func() { (&MapPending{}).Release() },
	}
	for name, release := range releases {
		t.Run(name, func(t *testing.T) {
			release()
		})
	}
}
