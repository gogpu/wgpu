//go:build !rust && !(js && wasm)

package surface

// The zero-value, released-surface and present-guard contract of the surface
// implementation (ADR-070 S10, commit 3): every answer reproduces the pre-move
// public Surface, and nil receivers are guarded instead of panicking. One hole
// the pre-move code had is left as it was: DiscardTexture on a zero value (nil
// core) panicked then and panics now, so it is not invoked here.

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core"
)

func TestZeroValueContract(t *testing.T) {
	receivers := []struct {
		name string
		make func() *Surface
	}{
		{"nil receiver", func() *Surface { return nil }},
		{"zero struct", func() *Surface { return &Surface{} }},
	}

	tests := []struct {
		name string
		run  func(t *testing.T, s *Surface)
	}{
		{
			name: "Core returns nil",
			run: func(t *testing.T, s *Surface) {
				t.Helper()
				if got := s.Core(); got != nil {
					t.Fatalf("Core = %p, want nil", got)
				}
			},
		},
		{
			name: "Released is false",
			run: func(t *testing.T, s *Surface) {
				t.Helper()
				if s.Released() {
					t.Fatal("a zero value must not report released")
				}
			},
		},
		{
			name: "SameDevice is false",
			run: func(t *testing.T, s *Surface) {
				t.Helper()
				if s.SameDevice(nil) {
					t.Fatal("a zero value must not match any device")
				}
			},
		},
		{
			name: "HALSurfaceForBackend is nil",
			run: func(t *testing.T, s *Surface) {
				t.Helper()
				if got := s.HALSurfaceForBackend(gputypes.BackendVulkan); got != nil {
					t.Fatalf("HALSurfaceForBackend = %v, want nil", got)
				}
			},
		},
		{
			name: "HALSurfacesForAdapterRequest is nil",
			run: func(t *testing.T, s *Surface) {
				t.Helper()
				if got := s.HALSurfacesForAdapterRequest(); got != nil {
					t.Fatalf("HALSurfacesForAdapterRequest = %v, want nil", got)
				}
			},
		},
		{
			name: "GetCurrentTexture reports not configured",
			run: func(t *testing.T, s *Surface) {
				t.Helper()
				_, _, err := s.GetCurrentTexture()
				if err == nil || err.Error() != "wgpu: surface not configured" {
					t.Fatalf("GetCurrentTexture error = %v, want \"wgpu: surface not configured\"", err)
				}
			},
		},
		{
			name: "PresentWithDamage reports not configured",
			run: func(t *testing.T, s *Surface) {
				t.Helper()
				err := s.PresentWithDamage(nil, nil)
				if err == nil || err.Error() != "wgpu: surface not configured" {
					t.Fatalf("PresentWithDamage error = %v, want \"wgpu: surface not configured\"", err)
				}
			},
		},
		{
			name: "ActualExtent is zero",
			run: func(t *testing.T, s *Surface) {
				t.Helper()
				if w, h := s.ActualExtent(); w != 0 || h != 0 {
					t.Fatalf("ActualExtent = %dx%d, want 0x0", w, h)
				}
			},
		},
		{
			name: "DiscardForDevice is a no-op",
			run: func(t *testing.T, s *Surface) {
				t.Helper()
				s.DiscardForDevice(nil)
			},
		},
		{
			name: "SetPresentsWithTransaction is a no-op",
			run: func(t *testing.T, s *Surface) {
				t.Helper()
				s.SetPresentsWithTransaction(true)
			},
		},
		{
			name: "Release is a no-op",
			run: func(t *testing.T, s *Surface) {
				t.Helper()
				s.Release()
			},
		},
	}

	for _, recv := range receivers {
		t.Run(recv.name, func(t *testing.T) {
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) { tt.run(t, recv.make()) })
			}
		})
	}
}

// TestReleasedSurfaceContract pins the released-surface answers: the sentinel
// is core.ErrReleased by identity, the extent is zero, and Release is
// idempotent.
func TestReleasedSurfaceContract(t *testing.T) {
	s := New(Init{})
	s.Release()

	if !s.Released() {
		t.Fatal("a released surface must report released")
	}
	if _, _, err := s.GetCurrentTexture(); !errors.Is(err, core.ErrReleased) {
		t.Fatalf("GetCurrentTexture on a released surface = %v, want core.ErrReleased", err)
	}
	if err := s.PresentWithDamage(nil, nil); !errors.Is(err, core.ErrReleased) {
		t.Fatalf("PresentWithDamage on a released surface = %v, want core.ErrReleased", err)
	}
	if w, h := s.ActualExtent(); w != 0 || h != 0 {
		t.Fatalf("ActualExtent on a released surface = %dx%d, want 0x0", w, h)
	}
	s.Release() // idempotent
}

// TestPresentPathGuards pins the remaining pre-move answers of the present
// path: no HAL queue gives the queue error, and a nil texture gives the
// texture error.
func TestPresentPathGuards(t *testing.T) {
	t.Run("no HAL queue", func(t *testing.T) {
		s := New(Init{})
		s.device = testDeviceHost{}
		err := s.PresentWithDamage(nil, nil)
		if err == nil || err.Error() != "wgpu: queue not available" {
			t.Fatalf("PresentWithDamage without a HAL queue = %v, want \"wgpu: queue not available\"", err)
		}
	})

	t.Run("nil texture", func(t *testing.T) {
		s := New(Init{})
		s.device = testDeviceHost{queue: &testQueue{}}
		err := s.PresentWithDamage(nil, nil)
		if err == nil || err.Error() != "wgpu: surface texture is nil" {
			t.Fatalf("PresentWithDamage with a nil texture = %v, want \"wgpu: surface texture is nil\"", err)
		}
	})
}
