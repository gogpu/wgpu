//go:build !rust && !(js && wasm)

package wgpu

import (
	"errors"
	"testing"

	"github.com/gogpu/wgpu/hal"
)

type fixedSurfaceTargetProvider struct {
	target SurfaceTargetUnsafe
}

func (p fixedSurfaceTargetProvider) SurfaceTarget() (SurfaceTargetUnsafe, error) {
	return p.target, nil
}

func TestSurfaceTargetUnsafeMapsToTypedHALTarget(t *testing.T) {
	tests := []struct {
		name   string
		target SurfaceTargetUnsafe
		want   hal.SurfaceTarget
	}{
		{
			name:   "Win32",
			target: SurfaceTargetFromWindowsHWND(1, 2),
			want:   hal.SurfaceTarget{Kind: hal.SurfaceTargetWindowsHWND, DisplayHandle: 1, WindowHandle: 2},
		},
		{
			name:   "Xlib",
			target: SurfaceTargetFromXlibWindow(3, 4),
			want:   hal.SurfaceTarget{Kind: hal.SurfaceTargetXlibWindow, DisplayHandle: 3, WindowHandle: 4},
		},
		{
			name:   "Wayland",
			target: SurfaceTargetFromWaylandSurface(5, 6),
			want:   hal.SurfaceTarget{Kind: hal.SurfaceTargetWaylandSurface, DisplayHandle: 5, WindowHandle: 6},
		},
		{
			name:   "Android",
			target: SurfaceTargetFromAndroidNativeWindow(7),
			want:   hal.SurfaceTarget{Kind: hal.SurfaceTargetAndroidNativeWindow, WindowHandle: 7},
		},
		{
			name:   "Metal",
			target: SurfaceTargetFromMetalLayer(8),
			want:   hal.SurfaceTarget{Kind: hal.SurfaceTargetMetalLayer, WindowHandle: 8},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.target.halTarget()
			if err != nil {
				t.Fatalf("halTarget: %v", err)
			}
			if got != test.want {
				t.Fatalf("halTarget = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestWebSurfaceTargetIsUnsupportedByNativeHAL(t *testing.T) {
	_, err := SurfaceTargetFromWebCanvasID(1).halTarget()
	if !errors.Is(err, ErrUnsupportedSurfaceTarget) {
		t.Fatalf("halTarget error = %v, want ErrUnsupportedSurfaceTarget", err)
	}

	_, err = (SurfaceTargetUnsafe{kind: surfaceTargetKind(255)}).halTarget()
	if !errors.Is(err, ErrInvalidSurfaceTarget) {
		t.Fatalf("unknown halTarget error = %v, want ErrInvalidSurfaceTarget", err)
	}
}

func TestSurfaceTargetUnsafeValidationAcceptsValidTargets(t *testing.T) {
	targets := []SurfaceTargetUnsafe{
		SurfaceTargetFromWindowsHWND(0, 1),
		SurfaceTargetFromXlibWindow(1, 2),
		SurfaceTargetFromWaylandSurface(3, 4),
		SurfaceTargetFromAndroidNativeWindow(5),
		SurfaceTargetFromMetalLayer(6),
		SurfaceTargetFromWebCanvasID(0),
	}

	for _, target := range targets {
		if err := target.validate(); err != nil {
			t.Fatalf("validate(%+v): %v", target, err)
		}
	}
}

func TestSurfaceTargetUnsafeValidationRejectsUnknownKind(t *testing.T) {
	target := SurfaceTargetUnsafe{kind: surfaceTargetKind(255), windowHandle: 1}
	if err := target.validate(); !errors.Is(err, ErrInvalidSurfaceTarget) {
		t.Fatalf("validate unknown target = %v, want ErrInvalidSurfaceTarget", err)
	}
}

func TestResolveSurfaceTargetValidatesProviderResult(t *testing.T) {
	want := SurfaceTargetFromAndroidNativeWindow(7)
	got, err := resolveSurfaceTarget(fixedSurfaceTargetProvider{target: want})
	if err != nil {
		t.Fatalf("resolveSurfaceTarget(valid): %v", err)
	}
	if got != want {
		t.Fatalf("resolved target = %+v, want %+v", got, want)
	}

	_, err = resolveSurfaceTarget(fixedSurfaceTargetProvider{})
	if !errors.Is(err, ErrInvalidSurfaceTarget) {
		t.Fatalf("resolveSurfaceTarget(invalid) = %v, want ErrInvalidSurfaceTarget", err)
	}

	_, err = resolveSurfaceTarget(nil)
	if !errors.Is(err, ErrInvalidSurfaceTarget) {
		t.Fatalf("resolveSurfaceTarget(nil) = %v, want ErrInvalidSurfaceTarget", err)
	}
}
