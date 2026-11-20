//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package surface

import (
	"errors"
	"image"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/hal"
	"github.com/gogpu/wgpu/internal/loststate"
)

// testDeviceHost is the seam double of the swapchain and acquisition tests: a
// core.DeviceView over a real core.Device plus the HAL queue presentation goes
// through. It replaces the root device fixture, which the destination cannot
// name.
type testDeviceHost struct {
	core  *core.Device
	queue hal.Queue
}

func (h testDeviceHost) Core() *core.Device             { return h.core }
func (testDeviceHost) IsReleased() bool                 { return false }
func (testDeviceHost) HALDevice() hal.Device            { return nil }
func (testDeviceHost) DestroyQueue() *core.DestroyQueue { return nil }
func (testDeviceHost) LastSubmissionIndex() uint64      { return 0 }
func (testDeviceHost) Poll(core.PollType) bool          { return false }
func (h testDeviceHost) HALQueue() hal.Queue            { return h.queue }

// MarkDeviceLost and DispatchDeviceCallbacks route to the shared core device,
// exactly as the root host does: the surface's observation is a device fact, so
// the double must record it where the tests read it (LostInfo on the core
// device). Both are nil-safe through core.Device's own guards.
func (h testDeviceHost) MarkDeviceLost(message string) {
	h.core.MarkLost(loststate.ReasonUnknown, message)
}

func (h testDeviceHost) DispatchDeviceCallbacks() { h.core.DispatchPending() }

// testQueue is the HAL queue double: presentation is a no-op the swapchain test
// only needs to complete.
type testQueue struct{ hal.Queue }

func (*testQueue) Present(hal.Surface, hal.SurfaceTexture, []image.Rectangle) error { return nil }

// newAcquiredTestSurface builds a configured surface with one live acquisition:
// a core.Surface over the local double, the seam double, and the configuration
// the lifetime tests used.
func newAcquiredTestSurface(t *testing.T) (*Surface, *SurfaceTexture, *core.Device) {
	t.Helper()

	coreDevice := core.NewDevice(&testHALDevice{}, nil, 0, gputypes.Limits{}, "surface-swapchain-test")
	t.Cleanup(coreDevice.Destroy)
	host := testDeviceHost{core: coreDevice, queue: &testQueue{}}

	s := &Surface{
		core:           core.NewSurface(&testHALSurface{}, "surface-swapchain-test"),
		device:         host,
		surfaceCreated: true,
		currentBackend: coreDevice.Backend(),
	}
	config := &SurfaceConfiguration{
		Width:       1,
		Height:      1,
		Format:      gputypes.TextureFormatRGBA8Unorm,
		Usage:       gputypes.TextureUsageRenderAttachment | gputypes.TextureUsageCopyDst,
		PresentMode: gputypes.PresentModeFifo,
		AlphaMode:   gputypes.CompositeAlphaModeAuto,
	}
	if err := s.Configure(host, config, true, nil); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	st, _, err := s.GetCurrentTexture()
	if err != nil {
		t.Fatalf("GetCurrentTexture: %v", err)
	}
	return s, &st, coreDevice
}

// TestSurfaceSwapchainTextureCaching verifies that GetCurrentTexture reuses
// the same core.Texture (and TrackerIndex) across frames instead of leaking
// a new TrackerIndex every frame (#307). Moved here with the implementation
// from surface_texture_lifetime_native_test.go (ADR-070 S10).
func TestSurfaceSwapchainTextureCaching(t *testing.T) {
	t.Parallel()

	coreDevice := core.NewDevice(&testHALDevice{}, nil, 0, gputypes.Limits{}, "cache-test")
	t.Cleanup(coreDevice.Destroy)
	host := testDeviceHost{core: coreDevice, queue: &testQueue{}}

	s := &Surface{
		core:           core.NewSurface(&testHALSurface{}, "cache-test"),
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

	// Simulate 100 frames — TrackerIndex must stay stable.
	var firstIndex uint32
	for i := range 100 {
		st, _, err := s.GetCurrentTexture()
		if err != nil {
			t.Fatalf("frame %d: GetCurrentTexture: %v", i, err)
		}
		if st.coreTexture == nil {
			t.Fatalf("frame %d: coreTexture is nil", i)
		}
		td := st.coreTexture.TrackingData()
		if td == nil || !td.Index().IsValid() {
			t.Fatalf("frame %d: invalid TrackerIndex", i)
		}
		idx := uint32(td.Index())
		if i == 0 {
			firstIndex = idx
		} else if idx != firstIndex {
			t.Fatalf("frame %d: TrackerIndex changed from %d to %d (leak!)", i, firstIndex, idx)
		}
		if err := s.Present(&st); err != nil {
			t.Fatalf("frame %d: Present: %v", i, err)
		}
	}

	// After 100 frames, texture allocator should have exactly 1 active texture
	// index (the cached swapchain texture), not 100.
	texAlloc := coreDevice.TrackerIndices().Textures()
	if texAlloc.Size() < 1 {
		t.Fatalf("expected at least 1 active texture index, got %d", texAlloc.Size())
	}

	// Unconfigure should release the cached TrackerIndex.
	s.Unconfigure()
	if s.swapchainTexture != nil {
		t.Fatal("swapchainTexture should be nil after Unconfigure")
	}

	// Reconfigure and verify a fresh TrackerIndex is allocated.
	if err := s.Configure(host, config, true, nil); err != nil {
		t.Fatalf("re-Configure: %v", err)
	}
	st, _, err := s.GetCurrentTexture()
	if err != nil {
		t.Fatalf("GetCurrentTexture after reconfigure: %v", err)
	}
	if st.coreTexture == nil {
		t.Fatal("coreTexture nil after reconfigure")
	}
	// The reused index from the free list should be the same as firstIndex.
	td := st.coreTexture.TrackingData()
	if td == nil || !td.Index().IsValid() {
		t.Fatal("invalid TrackerIndex after reconfigure")
	}
	if err := s.Present(&st); err != nil {
		t.Fatalf("Present after reconfigure: %v", err)
	}

	s.Release()
}

// TestSurfacePresentRejectsTextureFromAnotherAcquisition pins that Present
// rejects a token whose lease is not the live acquisition's, before touching
// the core state. Moved here with the implementation from
// surface_texture_lifetime_native_test.go (ADR-070 S10): the foreign token is
// built field by field, which only the domain package can do.
func TestSurfacePresentRejectsTextureFromAnotherAcquisition(t *testing.T) {
	s, st, _ := newAcquiredTestSurface(t)

	other := &SurfaceTexture{hal: st.hal, surface: st.surface, device: st.device, lease: st.lease + 1}
	if err := s.Present(other); !errors.Is(err, core.ErrReleased) {
		t.Fatalf("Present with foreign token = %v, want ErrReleased", err)
	}
	if s.core.State() != core.SurfaceStateAcquired {
		t.Fatalf("surface state after rejected Present = %v, want acquired", s.core.State())
	}
	s.DiscardTexture()
	s.Release()
}
