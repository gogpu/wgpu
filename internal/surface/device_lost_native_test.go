//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package surface

// The surface domain's device-loss evidence (P1-API-042 / #417, C3/D5): a
// present or acquire failure that wraps hal.ErrDeviceLost latches the loss and
// fires the callback, while a surface-loss failure keeps its own identity and
// never masquerades as device loss.
//
// The pre-#417 code compiled in neither test: the seam had no observation and
// the device had no latch.

import (
	"errors"
	"fmt"
	"image"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/hal"
	"github.com/gogpu/wgpu/internal/loststate"
)

// failingPresentQueue is a HAL queue whose Present fails with err.
type failingPresentQueue struct {
	hal.Queue
	err error
}

func (q *failingPresentQueue) Present(hal.Surface, hal.SurfaceTexture, []image.Rectangle) error {
	return q.err
}

// failingAcquireSurface is a HAL surface whose AcquireTexture fails with err.
type failingAcquireSurface struct {
	*testHALSurface
	err error
}

func (s *failingAcquireSurface) AcquireTexture(hal.Fence) (*hal.AcquiredSurfaceTexture, error) {
	return nil, s.err
}

// countLost registers a counting lost callback on the core device and returns
// the counter and the last delivered info.
func countLost(coreDevice *core.Device) (*int, *loststate.Info) {
	fires := new(int)
	last := new(loststate.Info)
	coreDevice.SetLostCallback(func(info loststate.Info) {
		*fires++
		*last = info
	})
	return fires, last
}

// TestPresentDeviceLossFiresTheCallback pins the present-side observation: a
// HAL present error that wraps hal.ErrDeviceLost is the device's, recorded
// first-wins, delivered once, with the sentinel still visible to the caller.
func TestPresentDeviceLossFiresTheCallback(t *testing.T) {
	s, st, coreDevice := newAcquiredTestSurface(t)
	fires, last := countLost(coreDevice)

	wrapped := fmt.Errorf("vk: present: %w", hal.ErrDeviceLost)
	s.device = testDeviceHost{core: coreDevice, queue: &failingPresentQueue{err: wrapped}}

	err := s.Present(st)
	if !errors.Is(err, hal.ErrDeviceLost) {
		t.Fatalf("Present error = %v, want the HAL device-lost sentinel preserved", err)
	}
	if *fires != 1 {
		t.Fatalf("lost callback fired %d times, want 1", *fires)
	}
	if last.Reason != loststate.ReasonUnknown {
		t.Fatalf("lost reason = %v, want Unknown for a driver loss", last.Reason)
	}
	if info, ok := coreDevice.LostInfo(); !ok || info.Reason != loststate.ReasonUnknown {
		t.Fatalf("LostInfo after the present = (%+v, %v), want the latched Unknown loss", info, ok)
	}
}

// TestPresentSurfaceLossKeepsItsIdentity pins C3's other half: a present that
// fails with a surface-loss sentinel reports no device loss — the surface is
// gone, the device is healthy — and the error reaches the caller unchanged.
func TestPresentSurfaceLossKeepsItsIdentity(t *testing.T) {
	s, st, coreDevice := newAcquiredTestSurface(t)
	fires, _ := countLost(coreDevice)

	s.device = testDeviceHost{core: coreDevice, queue: &failingPresentQueue{err: hal.ErrSurfaceLost}}

	err := s.Present(st)
	if !errors.Is(err, hal.ErrSurfaceLost) {
		t.Fatalf("Present error = %v, want the surface-loss sentinel", err)
	}
	if errors.Is(err, hal.ErrDeviceLost) {
		t.Fatalf("Present error = %v, want no device-lost class: a lost surface is not a lost device", err)
	}
	if *fires != 0 {
		t.Fatalf("lost callback fired %d times on a surface-loss error, want 0", *fires)
	}
	if _, ok := coreDevice.LostInfo(); ok {
		t.Fatal("a surface-loss present recorded a device loss")
	}
}

// TestAcquireDeviceLossFiresTheCallback pins the acquire side: an acquire that
// fails with a wrapped hal.ErrDeviceLost is observed the same way, and the
// surface stays unacquired.
func TestAcquireDeviceLossFiresTheCallback(t *testing.T) {
	coreDevice := core.NewDevice(&testHALDevice{}, nil, 0, gputypes.Limits{}, "surface-acquire-test")
	t.Cleanup(coreDevice.Destroy)
	host := testDeviceHost{core: coreDevice, queue: &testQueue{}}

	wrapped := fmt.Errorf("dx12: acquire: %w", hal.ErrDeviceLost)
	s := &Surface{
		core:           core.NewSurface(&failingAcquireSurface{testHALSurface: &testHALSurface{}, err: wrapped}, "acquire-test"),
		device:         host,
		surfaceCreated: true,
		currentBackend: coreDevice.Backend(),
	}
	config := &SurfaceConfiguration{
		Width:       1,
		Height:      1,
		Format:      gputypes.TextureFormatRGBA8Unorm,
		Usage:       gputypes.TextureUsageRenderAttachment,
		PresentMode: gputypes.PresentModeFifo,
		AlphaMode:   gputypes.CompositeAlphaModeAuto,
	}
	if err := s.Configure(host, config, true, nil); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	fires, last := countLost(coreDevice)

	_, _, err := s.GetCurrentTexture()
	if !errors.Is(err, hal.ErrDeviceLost) {
		t.Fatalf("GetCurrentTexture error = %v, want the HAL device-lost sentinel preserved", err)
	}
	if *fires != 1 {
		t.Fatalf("lost callback fired %d times, want 1", *fires)
	}
	if last.Reason != loststate.ReasonUnknown {
		t.Fatalf("lost reason = %v, want Unknown for a driver loss", last.Reason)
	}
	if s.core.State() != core.SurfaceStateConfigured {
		t.Fatalf("surface state after a failed acquire = %v, want Configured", s.core.State())
	}
}

// TestAcquireSurfaceLossKeepsItsIdentity is the acquire-side half of C3: an
// acquire that fails with a surface-loss sentinel reports no device loss — the
// swapchain is out of date, the device is healthy — and the sentinel reaches
// the caller unchanged (review P3-4: the symmetric case was untested).
func TestAcquireSurfaceLossKeepsItsIdentity(t *testing.T) {
	coreDevice := core.NewDevice(&testHALDevice{}, nil, 0, gputypes.Limits{}, "surface-acquire-loss-test")
	t.Cleanup(coreDevice.Destroy)
	host := testDeviceHost{core: coreDevice, queue: &testQueue{}}

	s := &Surface{
		core:           core.NewSurface(&failingAcquireSurface{testHALSurface: &testHALSurface{}, err: hal.ErrSurfaceOutdated}, "acquire-loss-test"),
		device:         host,
		surfaceCreated: true,
		currentBackend: coreDevice.Backend(),
	}
	config := &SurfaceConfiguration{
		Width:       1,
		Height:      1,
		Format:      gputypes.TextureFormatRGBA8Unorm,
		Usage:       gputypes.TextureUsageRenderAttachment,
		PresentMode: gputypes.PresentModeFifo,
		AlphaMode:   gputypes.CompositeAlphaModeAuto,
	}
	if err := s.Configure(host, config, true, nil); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	fires, _ := countLost(coreDevice)

	_, _, err := s.GetCurrentTexture()
	if !errors.Is(err, hal.ErrSurfaceOutdated) {
		t.Fatalf("GetCurrentTexture error = %v, want the surface-outdated sentinel", err)
	}
	if errors.Is(err, hal.ErrDeviceLost) {
		t.Fatalf("GetCurrentTexture error = %v, want no device-lost class: an outdated surface is not a lost device", err)
	}
	if *fires != 0 {
		t.Fatalf("lost callback fired %d times on a surface-loss acquire, want 0", *fires)
	}
	if _, ok := coreDevice.LostInfo(); ok {
		t.Fatal("a surface-loss acquire recorded a device loss")
	}
}
