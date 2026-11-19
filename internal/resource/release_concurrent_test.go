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
	samplers atomic.Int32
	textures atomic.Int32
	views    atomic.Int32
	modules  atomic.Int32
	queries  atomic.Int32
	fences   atomic.Int32
}

func (d *destroySpy) DestroySampler(hal.Sampler)           { d.samplers.Add(1) }
func (d *destroySpy) DestroyTexture(hal.Texture)           { d.textures.Add(1) }
func (d *destroySpy) DestroyTextureView(hal.TextureView)   { d.views.Add(1) }
func (d *destroySpy) DestroyShaderModule(hal.ShaderModule) { d.modules.Add(1) }
func (d *destroySpy) DestroyQuerySet(hal.QuerySet)         { d.queries.Add(1) }
func (d *destroySpy) DestroyFence(hal.Fence)               { d.fences.Add(1) }

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
	}
}

// concurrentReleaseIterations is the number of fresh instances per goroutine
// count. Every instance is a new check-then-set window, so the count is the
// product of independent attempts to widen the window rather than one race.
const concurrentReleaseIterations = 200

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

			if got := tc.count(spy); got != 1 {
				t.Fatalf("iteration %d: %d concurrent Release calls destroyed %d times, want exactly 1",
					i, goroutines, got)
			}
		}
	}
}

// TestZeroValueReleaseIsNoOp covers the nil-guard contract: a zero-value
// resource has no guard, and Release on it must return without panicking and
// without touching a HAL object.
func TestZeroValueReleaseIsNoOp(t *testing.T) {
	releases := map[string]func(){
		"Sampler":      func() { (&Sampler{}).Release() },
		"Texture":      func() { (&Texture{}).Release() },
		"TextureView":  func() { (&TextureView{}).Release() },
		"ShaderModule": func() { (&ShaderModule{}).Release() },
		"QuerySet":     func() { (&QuerySet{}).Release() },
		"Fence":        func() { (&Fence{}).Release() },
	}
	for name, release := range releases {
		t.Run(name, func(t *testing.T) {
			release()
		})
	}
}
