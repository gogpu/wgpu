//go:build !(js && wasm)

package shader

import (
	"math"
	"testing"
)

func TestGLSLCoverageBoostEdges(t *testing.T) {
	t.Run("faceforward_negate", func(t *testing.T) {
		// dot(Nref,I) >= 0 → -N
		got := faceForward(ValVec3(0, 1, 0), ValVec3(0, 1, 0), ValVec3(0, 1, 0))
		if got.AsVec3() != (Vec3{0, -1, 0}) {
			t.Errorf("got %v", got.AsVec3())
		}
	})

	t.Run("matrix_non_array", func(t *testing.T) {
		if matrixDeterminant(ValFloat(1)) != 0 {
			t.Fatal("det non-array")
		}
		if matrixInverse(ValFloat(1)).Tag != TagFloat32 {
			t.Fatal("inv non-array")
		}
		if matrixDeterminant(ValArray([]Value{ValFloat(1)})) != 0 {
			t.Fatal("det bad size")
		}
	})

	t.Run("zero_matrix_sizes", func(t *testing.T) {
		z2 := zeroMatrix(2, 2).AsArray()
		if z2[0].Tag != TagVec2 {
			t.Fatal(z2[0].Tag)
		}
		z3 := zeroMatrix(3, 3).AsArray()
		if z3[0].Tag != TagVec3 {
			t.Fatal(z3[0].Tag)
		}
	})

	t.Run("uint32_pair_paths", func(t *testing.T) {
		lo, hi := uint32Pair(ValVec2(1, 2))
		if lo != 1 || hi != 2 {
			t.Fatalf("%d %d", lo, hi)
		}
		v := Value{Tag: TagUint32, U: [4]uint32{9, 8}}
		lo, hi = uint32Pair(v)
		if lo != 9 || hi != 8 {
			t.Fatalf("%d %d", lo, hi)
		}
	})

	t.Run("unpack_double_fallback", func(t *testing.T) {
		got := unpackDouble2x32(ValFloat(1))
		if got.Tag != TagArray {
			t.Fatal(got.Tag)
		}
	})

	t.Run("snorm_extremes", func(t *testing.T) {
		if snorm8(-128) != -1 {
			t.Fatal(snorm8(-128))
		}
		if snorm16(-32768) != -1 {
			t.Fatal(snorm16(-32768))
		}
	})

	t.Run("clamp_and_ensure", func(t *testing.T) {
		if ensureVec4(ValVec3(1, 2, 3)).AsVec4()[2] != 3 {
			t.Fatal("ensure vec3")
		}
		if ensureVec4(ValVec2(1, 2)).AsVec4()[1] != 2 {
			t.Fatal("ensure vec2")
		}
		if ensureVec4(ValFloat(7)).AsVec4()[0] != 7 {
			t.Fatal("ensure float")
		}
		if vec2Components(ValFloat(3))[0] != 3 {
			t.Fatal("vec2Components")
		}
	})

	t.Run("float16_specials", func(t *testing.T) {
		nanBits := float32ToFloat16bits(float32(math.NaN()))
		if nanBits&0x7c00 != 0x7c00 {
			t.Fatalf("nan bits %#x", nanBits)
		}
		if float32ToFloat16bits(1e10)&0x7c00 != 0x7c00 {
			t.Fatal("overflow")
		}
		_ = float32ToFloat16bits(1e-8)
		tiny := float32ToFloat16bits(float32(math.Ldexp(1, -20)))
		_ = float16bitsToFloat32(tiny)
		_ = float16bitsToFloat32(0x0001) // denorm
		_ = float16bitsToFloat32(0x7e00) // nan
		_ = float16bitsToFloat32(0)      // +0
	})

	t.Run("find_bits_edges", func(t *testing.T) {
		if findUMsb(0) != -1 {
			t.Fatal()
		}
		if findSMsb(-8) < 0 {
			t.Fatal(findSMsb(-8))
		}
		if findSMsb(0) != -1 {
			t.Fatal()
		}
	})

	t.Run("store_through_variants", func(t *testing.T) {
		interp := newGLSLInterp(nil)
		parent := &Pointer{Val: ValArray([]Value{ValFloat(0), ValFloat(0)})}
		sp := &SubPointer{Parent: parent, Indexes: []uint32{0}}
		interp.storeThrough(ValSubPointer(sp), ValFloat(9))
		if parent.Val.AsArray()[0].AsFloat32() != 9 {
			t.Fatal("subpointer store")
		}
		buf := make([]byte, 4)
		bp := &BufferPointer{Buffer: buf, Offset: 0}
		interp.storeThrough(ValBufferPointer(bp), ValFloat(1))
		interp.storeThrough(ValFloat(0), ValFloat(1)) // no-op tag
	})

	t.Run("frexp_vectors", func(t *testing.T) {
		s, e := splitFrexp(ValVec2(2, 4))
		if s.AsVec2()[0] != 0.5 {
			t.Fatal(s)
		}
		if e.AsArray()[1].AsInt32() != 3 {
			t.Fatal(e)
		}
		_, e = splitFrexp(ValVec3(2, 4, 8))
		if len(e.AsArray()) != 3 {
			t.Fatal()
		}
		_, e = splitFrexp(ValVec4(2, 4, 8, 16))
		if len(e.AsArray()) != 4 {
			t.Fatal()
		}
	})

	t.Run("exp_component", func(t *testing.T) {
		if expComponent(ValArray(nil), 0) != 0 {
			t.Fatal()
		}
		if expComponent(ValArray([]Value{ValInt(3)}), 5) != 3 {
			t.Fatal("broadcast")
		}
		if expComponent(ValArray([]Value{ValInt(1), ValInt(2)}), 1) != 2 {
			t.Fatal()
		}
		if expComponent(ValVec2(4, 5), 1) != 5 {
			t.Fatal()
		}
		if expComponent(ValUint(7), 0) != 7 {
			t.Fatal()
		}
		if expComponent(ValFloat(2), 0) != 2 {
			t.Fatal()
		}
	})

	t.Run("ldexp_vec34", func(t *testing.T) {
		interp := newGLSLInterp(map[uint32]any{
			10: ValVec3(0.5, 0.5, 0.5),
			11: ValInt(2),
			12: ValVec4(0.5, 0.5, 0.5, 0.5),
			13: ValArray([]Value{ValInt(1), ValInt(2), ValInt(3), ValInt(4)}),
		})
		got := interp.executeGLSLExtInst(GLSLLdexp, []uint32{10, 11})
		if got.AsVec3()[0] != 2 {
			t.Fatal(got)
		}
		got = interp.executeGLSLExtInst(GLSLLdexp, []uint32{12, 13})
		if got.AsVec4()[3] != 8 {
			t.Fatal(got)
		}
	})

	t.Run("empty_operands", func(t *testing.T) {
		interp := newGLSLInterp(nil)
		ops := []uint32{
			GLSLDeterminant, GLSLMatrixInverse, GLSLPackSnorm4x8, GLSLPackUnorm4x8,
			GLSLPackSnorm2x16, GLSLPackUnorm2x16, GLSLPackHalf2x16, GLSLPackDouble2x32,
			GLSLUnpackSnorm2x16, GLSLUnpackUnorm2x16, GLSLUnpackHalf2x16,
			GLSLUnpackSnorm4x8, GLSLUnpackUnorm4x8, GLSLUnpackDouble2x32,
			GLSLFaceForward, GLSLRefract, GLSLFindILsb, GLSLFindSMsb, GLSLFindUMsb,
			GLSLInterpolateAtSample, GLSLModf, GLSLModfStruct, GLSLFrexp, GLSLFrexpStruct, GLSLLdexp,
		}
		for _, op := range ops {
			_ = interp.executeGLSLExtInst(op, nil)
		}
	})

	t.Run("nmin_nmax_nan_first", func(t *testing.T) {
		nan := float32(math.NaN())
		if !math.IsNaN(float64(nMin(nan, nan))) && nMin(nan, 1) != 1 {
			t.Fatal()
		}
		if nMax(nan, 3) != 3 {
			t.Fatal()
		}
	})

	t.Run("mat_elem_oob", func(t *testing.T) {
		if matElem(nil, 0, 0) != 0 {
			t.Fatal()
		}
		if matElem([]Value{ValVec2(1, 2)}, 0, 5) != 0 {
			t.Fatal()
		}
	})

	t.Run("inverse3_singular", func(t *testing.T) {
		m := ValArray([]Value{
			ValVec3(1, 0, 0),
			ValVec3(2, 0, 0),
			ValVec3(3, 0, 0),
		})
		inv := matrixInverse(m)
		for _, c := range inv.AsArray() {
			v := c.AsVec3()
			if v[0] != 0 || v[1] != 0 || v[2] != 0 {
				t.Fatal(v)
			}
		}
	})

	t.Run("pack_ext_all", func(t *testing.T) {
		interp := newGLSLInterp(map[uint32]any{
			10: ValVec2(-1, 1),
			11: ValVec4(0, 1, 0, 1),
			12: ValArray([]Value{ValUint(0), ValUint(0x40000000)}),
		})
		_ = interp.executeGLSLExtInst(GLSLPackSnorm2x16, []uint32{10})
		_ = interp.executeGLSLExtInst(GLSLPackUnorm2x16, []uint32{10})
		_ = interp.executeGLSLExtInst(GLSLPackHalf2x16, []uint32{10})
		_ = interp.executeGLSLExtInst(GLSLPackSnorm4x8, []uint32{11})
		p := interp.executeGLSLExtInst(GLSLPackDouble2x32, []uint32{12})
		_ = interp.executeGLSLExtInst(GLSLUnpackSnorm2x16, []uint32{10})
		interp.values[20] = ValUint(packHalf2x16(ValVec2(1, 2)))
		_ = interp.executeGLSLExtInst(GLSLUnpackHalf2x16, []uint32{20})
		interp.values[21] = ValUint(packSnorm2x16(ValVec2(-1, 1)))
		_ = interp.executeGLSLExtInst(GLSLUnpackSnorm2x16, []uint32{21})
		interp.values[22] = ValUint(packUnorm2x16(ValVec2(0, 1)))
		_ = interp.executeGLSLExtInst(GLSLUnpackUnorm2x16, []uint32{22})
		interp.values[23] = ValUint(packSnorm4x8(ValVec4(-1, 0, 1, 0)))
		_ = interp.executeGLSLExtInst(GLSLUnpackSnorm4x8, []uint32{23})
		interp.values[24] = p
		_ = interp.executeGLSLExtInst(GLSLUnpackDouble2x32, []uint32{24})
	})

	t.Run("interpolate_sample_offset", func(t *testing.T) {
		interp := newGLSLInterp(map[uint32]any{10: ValFloat(1)})
		_ = interp.executeGLSLExtInst(GLSLInterpolateAtSample, []uint32{10})
		_ = interp.executeGLSLExtInst(GLSLInterpolateAtOffset, []uint32{10})
	})

	t.Run("float64_helpers", func(t *testing.T) {
		v := ValFloat64(math.E)
		if math.Abs(v.AsFloat64()-math.E) > 1e-12 {
			t.Fatal()
		}
		if ValFloat(1).AsFloat64() != 0 {
			t.Fatal()
		}
	})
}
