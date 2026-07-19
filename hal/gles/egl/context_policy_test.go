//go:build linux && !(js && wasm)

package egl

import "testing"

func TestSelectWindowKindHonorsExplicitTarget(t *testing.T) {
	x11 := WindowKindX11
	wayland := WindowKindWayland
	tests := []struct {
		name      string
		requested *WindowKind
		detected  WindowKind
		want      WindowKind
		wantCalls int
	}{
		{name: "explicit X11 under Wayland", requested: &x11, detected: WindowKindWayland, want: WindowKindX11, wantCalls: 0},
		{name: "explicit Wayland under X11", requested: &wayland, detected: WindowKindX11, want: WindowKindWayland, wantCalls: 0},
		{name: "nil uses automatic selection", detected: WindowKindSurfaceless, want: WindowKindSurfaceless, wantCalls: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			got := selectWindowKind(test.requested, func() WindowKind {
				calls++
				return test.detected
			})
			if got != test.want {
				t.Fatalf("selectWindowKind() = %v, want %v", got, test.want)
			}
			if calls != test.wantCalls {
				t.Fatalf("detector calls = %d, want %d", calls, test.wantCalls)
			}
		})
	}
}
