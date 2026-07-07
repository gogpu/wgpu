//go:build !rust && !(js && wasm)

package wgpu_test

import (
	"testing"

	"github.com/gogpu/wgpu"
	"github.com/gogpu/wgpu/hal/noop"
)

// =============================================================================
// Surface.SetPresentsWithTransaction — guard-clause coverage
// Covers surface_native.go lines 190-201
// =============================================================================

// TestSetPresentsWithTransactionReleased exercises the
// "s.released || s.core == nil" early-return branch.
// Release() sets s.core = nil; subsequent calls must be a no-op.
func TestSetPresentsWithTransactionReleased(t *testing.T) {
	// A noop.Surface has a valid Destroy(), so Release() succeeds.
	surface := wgpu.NewSurfaceFromHAL(&noop.Surface{}, "released")
	surface.Release() // sets s.core = nil
	// Must not panic (s.core == nil hits the early return).
	surface.SetPresentsWithTransaction(true)
	surface.SetPresentsWithTransaction(false)
}

// TestSetPresentsWithTransactionNilRaw exercises the "raw == nil" branch:
// the surface was created with a nil HAL surface so RawSurface() returns nil.
// Note: Release() panics when RawSurface() is nil (it calls Destroy() on nil),
// so cleanup is intentionally omitted here — no GPU resources were allocated.
func TestSetPresentsWithTransactionNilRaw(t *testing.T) {
	surface := wgpu.NewSurfaceFromHAL(nil, "nil-raw")
	// core is non-nil, but core.RawSurface() == nil — must return without panic.
	surface.SetPresentsWithTransaction(true)
	surface.SetPresentsWithTransaction(false)
}

// TestSetPresentsWithTransactionNoopBackend exercises the type-assertion false
// branch: noop.Surface does not implement SetPresentsWithTransaction, so the
// assertion fails silently and the call is a no-op.
func TestSetPresentsWithTransactionNoopBackend(t *testing.T) {
	halSurface := &noop.Surface{}
	surface := wgpu.NewSurfaceFromHAL(halSurface, "noop")
	defer surface.Release()
	// noop.Surface has no SetPresentsWithTransaction — type assertion is false.
	surface.SetPresentsWithTransaction(true)
	surface.SetPresentsWithTransaction(false)
}

// mockTransactionSurface wraps noop.Surface and adds SetPresentsWithTransaction
// so the type assertion in Surface.SetPresentsWithTransaction succeeds.
type mockTransactionSurface struct {
	noop.Surface
	lastEnabled bool
	callCount   int
}

func (m *mockTransactionSurface) SetPresentsWithTransaction(enabled bool) {
	m.lastEnabled = enabled
	m.callCount++
}

// TestSetPresentsWithTransactionImplemented exercises the type-assertion true
// branch: a surface that implements SetPresentsWithTransaction delegates to it.
func TestSetPresentsWithTransactionImplemented(t *testing.T) {
	mock := &mockTransactionSurface{}
	surface := wgpu.NewSurfaceFromHAL(mock, "mock-metal")
	defer surface.Release()

	surface.SetPresentsWithTransaction(true)
	if !mock.lastEnabled || mock.callCount != 1 {
		t.Errorf("SetPresentsWithTransaction(true): callCount=%d lastEnabled=%v", mock.callCount, mock.lastEnabled)
	}

	surface.SetPresentsWithTransaction(false)
	if mock.lastEnabled || mock.callCount != 2 {
		t.Errorf("SetPresentsWithTransaction(false): callCount=%d lastEnabled=%v", mock.callCount, mock.lastEnabled)
	}
}

// =============================================================================
// Surface.ActualExtent — unconfigured surface returns (0, 0)
// Covers surface_native.go ActualExtent guard clauses
// =============================================================================

func TestActualExtentUnconfigured(t *testing.T) {
	// nil HAL surface → RawSurface() == nil → returns (0,0).
	// Release() not called because it panics with nil RawSurface.
	surface := wgpu.NewSurfaceFromHAL(nil, "unconfigured")

	w, h := surface.ActualExtent()
	if w != 0 || h != 0 {
		t.Errorf("ActualExtent on unconfigured surface: got (%d,%d), want (0,0)", w, h)
	}
}

// =============================================================================
// Surface.DiscardTexture — unconfigured surface is a no-op
// Covers surface_native.go DiscardTexture guard clause
// =============================================================================

func TestDiscardTextureUnconfigured(t *testing.T) {
	// nil HAL surface → Release() not called (would panic on nil RawSurface).
	surface := wgpu.NewSurfaceFromHAL(nil, "unconfigured")
	// Must not panic when no texture has been acquired.
	surface.DiscardTexture()
}
