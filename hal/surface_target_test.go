//go:build !(js && wasm)

package hal_test

import (
	"errors"
	"testing"

	"github.com/gogpu/wgpu/hal"
)

func TestSurfaceTargetCarriesExplicitPlatformKind(t *testing.T) {
	target := hal.SurfaceTarget{
		Kind:          hal.SurfaceTargetAndroidNativeWindow,
		DisplayHandle: 0x1234,
		WindowHandle:  0x5678,
	}

	if target.Kind != hal.SurfaceTargetAndroidNativeWindow {
		t.Fatalf("Kind = %v, want Android native window", target.Kind)
	}
	if target.DisplayHandle != 0x1234 || target.WindowHandle != 0x5678 {
		t.Fatalf("handles = (%#x, %#x), want (%#x, %#x)", target.DisplayHandle, target.WindowHandle, uintptr(0x1234), uintptr(0x5678))
	}
}

func TestSurfaceTargetRequireKind(t *testing.T) {
	target := hal.SurfaceTarget{Kind: hal.SurfaceTargetAndroidNativeWindow}
	if err := target.RequireKind(hal.SurfaceTargetAndroidNativeWindow); err != nil {
		t.Fatalf("RequireKind rejected matching target: %v", err)
	}
	if err := target.RequireKind(hal.SurfaceTargetXlibWindow); !errors.Is(err, hal.ErrUnsupportedSurfaceTarget) {
		t.Fatalf("RequireKind mismatch error = %v, want ErrUnsupportedSurfaceTarget", err)
	}
}
