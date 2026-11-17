//go:build !(js && wasm)

package shader

import (
	"math"
	"testing"
)

func TestGLSLModfStruct(t *testing.T) {
	interp := newGLSLInterp(map[uint32]any{10: ValFloat(-3.75)})
	got := interp.executeGLSLExtInst(GLSLModfStruct, []uint32{10})
	arr := got.AsArray()
	if len(arr) != 2 {
		t.Fatalf("ModfStruct len=%d", len(arr))
	}
	if math.Abs(float64(arr[0].AsFloat32()+0.75)) > 1e-5 {
		t.Errorf("fract=%v", arr[0].AsFloat32())
	}
	if math.Abs(float64(arr[1].AsFloat32()+3)) > 1e-5 {
		t.Errorf("whole=%v", arr[1].AsFloat32())
	}
}

func TestGLSLModfPointer(t *testing.T) {
	ptr := &Pointer{}
	interp := newGLSLInterp(map[uint32]any{
		10: ValFloat(2.25),
		11: ValPointer(ptr),
	})
	fract := interp.executeGLSLExtInst(GLSLModf, []uint32{10, 11})
	if math.Abs(float64(fract.AsFloat32()-0.25)) > 1e-5 {
		t.Errorf("fract=%v", fract.AsFloat32())
	}
	if math.Abs(float64(ptr.Val.AsFloat32()-2)) > 1e-5 {
		t.Errorf("stored whole=%v", ptr.Val.AsFloat32())
	}
}

func TestGLSLFrexpStruct(t *testing.T) {
	interp := newGLSLInterp(map[uint32]any{10: ValFloat(8)})
	got := interp.executeGLSLExtInst(GLSLFrexpStruct, []uint32{10})
	arr := got.AsArray()
	if math.Abs(float64(arr[0].AsFloat32()-0.5)) > 1e-5 {
		t.Errorf("sig=%v", arr[0].AsFloat32())
	}
	if arr[1].AsInt32() != 4 {
		t.Errorf("exp=%v", arr[1].AsInt32())
	}
}

func TestGLSLFrexpPointer(t *testing.T) {
	ptr := &Pointer{}
	interp := newGLSLInterp(map[uint32]any{
		10: ValFloat(3),
		11: ValPointer(ptr),
	})
	sig := interp.executeGLSLExtInst(GLSLFrexp, []uint32{10, 11})
	if math.Abs(float64(sig.AsFloat32())-0.75) > 1e-5 {
		t.Errorf("sig=%v", sig.AsFloat32())
	}
	if ptr.Val.AsInt32() != 2 {
		t.Errorf("exp=%v", ptr.Val.AsInt32())
	}
}

func TestGLSLLdexp(t *testing.T) {
	interp := newGLSLInterp(map[uint32]any{
		10: ValFloat(0.75),
		11: ValInt(2),
	})
	got := interp.executeGLSLExtInst(GLSLLdexp, []uint32{10, 11})
	if math.Abs(float64(got.AsFloat32())-3) > 1e-5 {
		t.Errorf("Ldexp(0.75,2)=%v", got.AsFloat32())
	}

	interp = newGLSLInterp(map[uint32]any{
		10: ValVec2(0.5, 0.75),
		11: ValInt(3),
	})
	got = interp.executeGLSLExtInst(GLSLLdexp, []uint32{10, 11})
	v := got.AsVec2()
	if math.Abs(float64(v[0]-4)) > 1e-5 || math.Abs(float64(v[1]-6)) > 1e-5 {
		t.Errorf("Ldexp vec = %v", v)
	}
}
