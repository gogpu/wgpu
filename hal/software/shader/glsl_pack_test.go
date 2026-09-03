//go:build !(js && wasm)

package shader

import (
	"math"
	"testing"
)

func TestPackUnpackUnorm4x8(t *testing.T) {
	v := ValVec4(0, 0.5, 1, 0.25)
	p := packUnorm4x8(v)
	got := unpackUnorm4x8(p).AsVec4()
	want := [4]float32{0, 0.5, 1, 0.25}
	for i := 0; i < 4; i++ {
		if math.Abs(float64(got[i]-want[i])) > 1.0/255+1e-5 {
			t.Errorf("component %d: got %v want ~%v (packed=%#x)", i, got[i], want[i], p)
		}
	}
}

func TestPackUnpackSnorm4x8(t *testing.T) {
	v := ValVec4(-1, 0, 1, -0.5)
	p := packSnorm4x8(v)
	got := unpackSnorm4x8(p).AsVec4()
	for i, w := range []float32{-1, 0, 1, -0.5} {
		if math.Abs(float64(got[i]-w)) > 1.0/127+1e-5 {
			t.Errorf("snorm4x8[%d]=%v want %v", i, got[i], w)
		}
	}
}

func TestPackUnpackUnorm2x16(t *testing.T) {
	v := ValVec2(0, 1)
	p := packUnorm2x16(v)
	got := unpackUnorm2x16(p).AsVec2()
	if got[0] != 0 || math.Abs(float64(got[1]-1)) > 1e-5 {
		t.Errorf("unorm2x16 = %v", got)
	}
}

func TestPackUnpackSnorm2x16(t *testing.T) {
	v := ValVec2(-1, 1)
	p := packSnorm2x16(v)
	got := unpackSnorm2x16(p).AsVec2()
	if math.Abs(float64(got[0]+1)) > 1e-4 || math.Abs(float64(got[1]-1)) > 1e-4 {
		t.Errorf("snorm2x16 = %v", got)
	}
}

func TestPackUnpackHalf2x16(t *testing.T) {
	v := ValVec2(1.5, -2)
	p := packHalf2x16(v)
	got := unpackHalf2x16(p).AsVec2()
	if math.Abs(float64(got[0]-1.5)) > 1e-3 || math.Abs(float64(got[1]+2)) > 1e-3 {
		t.Errorf("half2x16 = %v", got)
	}
}

func TestPackUnpackDouble2x32(t *testing.T) {
	f := math.Pi
	bits := math.Float64bits(f)
	lo := uint32(bits)
	hi := uint32(bits >> 32)
	packed := packDouble2x32(ValArray([]Value{ValUint(lo), ValUint(hi)}))
	if packed.Tag != TagFloat64 || math.Abs(packed.AsFloat64()-f) > 1e-12 {
		t.Fatalf("PackDouble2x32 = %v", packed.AsFloat64())
	}
	unpacked := unpackDouble2x32(packed).AsArray()
	if toUint32(unpacked[0]) != lo || toUint32(unpacked[1]) != hi {
		t.Errorf("UnpackDouble2x32 = (%#x,%#x)", toUint32(unpacked[0]), toUint32(unpacked[1]))
	}
}

func TestFloat16RoundTrip(t *testing.T) {
	cases := []float32{0, 1, -1, 0.5, 65504, float32(math.Inf(1)), float32(math.Inf(-1))}
	for _, c := range cases {
		h := float32ToFloat16bits(c)
		got := float16bitsToFloat32(h)
		if math.IsInf(float64(c), 0) {
			if !math.IsInf(float64(got), int(math.Copysign(1, float64(c)))) {
				t.Errorf("half Inf roundtrip failed for %v → %v", c, got)
			}
			continue
		}
		if math.Abs(float64(got-c)) > 1e-2 && c != 65504 {
			t.Errorf("half(%v)=%v bits=%#x", c, got, h)
		}
	}
}

func TestPackViaExtInst(t *testing.T) {
	interp := newGLSLInterp(map[uint32]any{
		10: ValVec4(1, 0, 0, 1),
	})
	got := interp.executeGLSLExtInst(GLSLPackUnorm4x8, []uint32{10})
	if got.AsUint32() == 0 {
		t.Fatal("PackUnorm4x8 returned 0")
	}
	interp2 := newGLSLInterp(map[uint32]any{11: got})
	u := interp2.executeGLSLExtInst(GLSLUnpackUnorm4x8, []uint32{11}).AsVec4()
	if math.Abs(float64(u[0]-1)) > 1e-5 || math.Abs(float64(u[3]-1)) > 1e-5 {
		t.Errorf("roundtrip unorm4x8 = %v", u)
	}
}
