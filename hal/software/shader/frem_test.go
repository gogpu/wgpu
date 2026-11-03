//go:build !(js && wasm)

package shader

import "testing"

// TestFRemSignOfDividend checks OpFRem against WGSL's e1 - e2*trunc(e1/e2):
// the result has the sign of the dividend. The cases are naga's readback case
// floats/fmod-sign-rt, which failed on the software backend (P1-SW-013).
func TestFRemSignOfDividend(t *testing.T) {
	for _, c := range []struct{ a, b, want float32 }{
		{-1.5, 1, -0.5},
		{1.5, -1, 0.5},
		{-7.5, 2, -1.5},
		{7.5, -2, 1.5},
		{-7.5, -2, -1.5},
		{7, 3, 1},
	} {
		if got := fRem(c.a, c.b); got != c.want {
			t.Errorf("fRem(%v, %v) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}
