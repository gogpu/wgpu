//go:build !(js && wasm)

package shader

import (
	"math"
	"testing"
)

func newGLSLInterp(vals map[uint32]any) *interpreter {
	return &interpreter{
		module: &Module{
			Types:          map[uint32]*TypeInfo{},
			Constants:      map[uint32]Value{},
			ExtInstImports: map[uint32]string{1: "GLSL.std.450"},
		},
		values: testMakeValues(vals),
	}
}

func TestGLSLRadiansDegrees(t *testing.T) {
	interp := newGLSLInterp(map[uint32]any{10: ValFloat(180), 11: ValFloat(math.Pi)})
	got := interp.executeGLSLExtInst(GLSLRadians, []uint32{10})
	if math.Abs(float64(got.AsFloat32())-math.Pi) > 1e-5 {
		t.Errorf("Radians(180) = %v, want π", got.AsFloat32())
	}
	got = interp.executeGLSLExtInst(GLSLDegrees, []uint32{11})
	if math.Abs(float64(got.AsFloat32())-180) > 1e-4 {
		t.Errorf("Degrees(π) = %v, want 180", got.AsFloat32())
	}
}

func TestGLSLFma(t *testing.T) {
	interp := newGLSLInterp(map[uint32]any{
		10: ValFloat(2), 11: ValFloat(3), 12: ValFloat(4),
	})
	got := interp.executeGLSLExtInst(GLSLFma, []uint32{10, 11, 12})
	if got.AsFloat32() != 10 {
		t.Errorf("Fma(2,3,4) = %v, want 10", got.AsFloat32())
	}
}

func TestGLSLFaceForwardRefract(t *testing.T) {
	interp := newGLSLInterp(map[uint32]any{
		10: ValVec3(0, 1, 0),
		11: ValVec3(0, -1, 0),
		12: ValVec3(0, 1, 0),
		13: ValFloat(1),
	})
	got := interp.executeGLSLExtInst(GLSLFaceForward, []uint32{10, 11, 12})
	gv := got.AsVec3()
	if gv != (Vec3{0, 1, 0}) {
		t.Errorf("FaceForward = %v, want (0,1,0)", gv)
	}

	// Refract along normal with eta=1.
	got = interp.executeGLSLExtInst(GLSLRefract, []uint32{11, 10, 13})
	if got.Tag != TagVec3 {
		t.Fatalf("Refract tag = %v", got.Tag)
	}

	// Total internal reflection: steep exit with eta>1 → zero vector.
	interp.values[11] = ValVec3(0.8, -0.6, 0)
	interp.values[13] = ValFloat(1.5)
	got = interp.executeGLSLExtInst(GLSLRefract, []uint32{11, 10, 13})
	gv = got.AsVec3()
	if gv != (Vec3{0, 0, 0}) {
		t.Errorf("Refract TIR = %v, want zero", gv)
	}
}

func TestGLSLNMinNMaxNClamp(t *testing.T) {
	nan := float32(math.NaN())
	interp := newGLSLInterp(map[uint32]any{
		10: ValFloat(1), 11: ValFloat(2), 12: ValFloat(nan),
		13: ValFloat(0), 14: ValFloat(5),
	})
	if g := interp.executeGLSLExtInst(GLSLNMin, []uint32{10, 11}).AsFloat32(); g != 1 {
		t.Errorf("NMin(1,2)=%v", g)
	}
	if g := interp.executeGLSLExtInst(GLSLNMin, []uint32{10, 12}).AsFloat32(); g != 1 {
		t.Errorf("NMin(1,NaN)=%v", g)
	}
	if g := interp.executeGLSLExtInst(GLSLNMax, []uint32{12, 11}).AsFloat32(); g != 2 {
		t.Errorf("NMax(NaN,2)=%v", g)
	}
	if g := interp.executeGLSLExtInst(GLSLNClamp, []uint32{11, 13, 14}).AsFloat32(); g != 2 {
		t.Errorf("NClamp(2,0,5)=%v", g)
	}
}

func TestGLSLFindBits(t *testing.T) {
	interp := newGLSLInterp(map[uint32]any{
		10: ValUint(0),
		11: ValUint(0b1000),
		12: ValInt(-1),
		13: ValInt(0b0100),
		14: ValUint(0x80000000),
	})
	if g := interp.executeGLSLExtInst(GLSLFindILsb, []uint32{10}).AsInt32(); g != -1 {
		t.Errorf("FindILsb(0)=%d", g)
	}
	if g := interp.executeGLSLExtInst(GLSLFindILsb, []uint32{11}).AsInt32(); g != 3 {
		t.Errorf("FindILsb(8)=%d", g)
	}
	if g := interp.executeGLSLExtInst(GLSLFindSMsb, []uint32{12}).AsInt32(); g != -1 {
		t.Errorf("FindSMsb(-1)=%d", g)
	}
	if g := interp.executeGLSLExtInst(GLSLFindSMsb, []uint32{13}).AsInt32(); g != 2 {
		t.Errorf("FindSMsb(4)=%d", g)
	}
	if g := interp.executeGLSLExtInst(GLSLFindUMsb, []uint32{14}).AsInt32(); g != 31 {
		t.Errorf("FindUMsb(0x80000000)=%d", g)
	}
}

func TestGLSLInterpolateIdentity(t *testing.T) {
	interp := newGLSLInterp(map[uint32]any{10: ValFloat(42)})
	got := interp.executeGLSLExtInst(GLSLInterpolateAtCentroid, []uint32{10})
	if got.AsFloat32() != 42 {
		t.Errorf("InterpolateAtCentroid = %v", got.AsFloat32())
	}
}

func TestGLSLUnknownOpcodeWarnsAndZeros(t *testing.T) {
	interp := newGLSLInterp(nil)
	got := interp.executeGLSLExtInst(9999, nil)
	if got.Tag != TagUint32 || got.AsUint32() != 0 {
		t.Errorf("unknown opcode = %v, want ValUint(0)", got)
	}
}

func TestGLSLIMixAlias(t *testing.T) {
	interp := newGLSLInterp(map[uint32]any{
		10: ValFloat(0), 11: ValFloat(10), 12: ValFloat(0.5),
	})
	got := interp.executeGLSLExtInst(GLSLIMix, []uint32{10, 11, 12})
	if math.Abs(float64(got.AsFloat32())-5) > 1e-5 {
		t.Errorf("IMix = %v, want 5", got.AsFloat32())
	}
}
