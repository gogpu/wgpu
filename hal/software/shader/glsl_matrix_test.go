//go:build !(js && wasm)

package shader

import (
	"math"
	"testing"
)

func TestMatrixDeterminant2(t *testing.T) {
	// | 1 2 |
	// | 3 4 | det = -2
	m := ValArray([]Value{ValVec2(1, 3), ValVec2(2, 4)})
	if d := matrixDeterminant(m); d != -2 {
		t.Errorf("det2 = %v, want -2", d)
	}
}

func TestMatrixDeterminant3(t *testing.T) {
	// Identity mat3
	m := ValArray([]Value{
		ValVec3(1, 0, 0),
		ValVec3(0, 1, 0),
		ValVec3(0, 0, 1),
	})
	if d := matrixDeterminant(m); math.Abs(float64(d)-1) > 1e-5 {
		t.Errorf("det3(I) = %v, want 1", d)
	}
}

func TestMatrixDeterminant4(t *testing.T) {
	m := ValArray([]Value{
		ValVec4(2, 0, 0, 0),
		ValVec4(0, 3, 0, 0),
		ValVec4(0, 0, 4, 0),
		ValVec4(0, 0, 0, 5),
	})
	if d := matrixDeterminant(m); math.Abs(float64(d)-120) > 1e-3 {
		t.Errorf("det4(diag) = %v, want 120", d)
	}
}

func TestMatrixInverse2(t *testing.T) {
	m := ValArray([]Value{ValVec2(4, 2), ValVec2(3, 1)})
	inv := matrixInverse(m)
	cols := inv.AsArray()
	// Inverse of [[4,3],[2,1]] = [[-0.5,1.5],[1,-2]] column-major:
	// columns: (-0.5, 1), (1.5, -2) wait — matrix column-major:
	// col0=(4,2), col1=(3,1) → row-major [[4,3],[2,1]], det=-2
	// inv = (-1/2)*[[1,-3],[-2,4]] = [[-0.5,1.5],[1,-2]]
	// columns: col0=(-0.5,1), col1=(1.5,-2)
	c0, c1 := cols[0].AsVec2(), cols[1].AsVec2()
	if math.Abs(float64(c0[0]+0.5)) > 1e-5 || math.Abs(float64(c0[1]-1)) > 1e-5 {
		t.Errorf("inv2 col0 = %v", c0)
	}
	if math.Abs(float64(c1[0]-1.5)) > 1e-5 || math.Abs(float64(c1[1]+2)) > 1e-5 {
		t.Errorf("inv2 col1 = %v", c1)
	}
}

func TestMatrixInverseIdentity4(t *testing.T) {
	id := ValArray([]Value{
		ValVec4(1, 0, 0, 0),
		ValVec4(0, 1, 0, 0),
		ValVec4(0, 0, 1, 0),
		ValVec4(0, 0, 0, 1),
	})
	inv := matrixInverse(id)
	for i, col := range inv.AsArray() {
		want := id.AsArray()[i].AsVec4()
		got := col.AsVec4()
		for j := 0; j < 4; j++ {
			if math.Abs(float64(got[j]-want[j])) > 1e-5 {
				t.Fatalf("inv(I)[%d][%d] = %v, want %v", i, j, got[j], want[j])
			}
		}
	}
}

func TestMatrixInverseSingular(t *testing.T) {
	m := ValArray([]Value{ValVec2(1, 2), ValVec2(2, 4)})
	inv := matrixInverse(m)
	for _, col := range inv.AsArray() {
		v := col.AsVec2()
		if v[0] != 0 || v[1] != 0 {
			t.Errorf("singular inverse = %v, want zero", v)
		}
	}
}

func TestMatrixInverseViaExtInst(t *testing.T) {
	m := ValArray([]Value{
		ValVec3(2, 0, 0),
		ValVec3(0, 4, 0),
		ValVec3(0, 0, 5),
	})
	interp := newGLSLInterp(map[uint32]any{10: m})
	det := interp.executeGLSLExtInst(GLSLDeterminant, []uint32{10})
	if math.Abs(float64(det.AsFloat32())-40) > 1e-4 {
		t.Errorf("Determinant = %v, want 40", det.AsFloat32())
	}
	inv := interp.executeGLSLExtInst(GLSLMatrixInverse, []uint32{10})
	cols := inv.AsArray()
	if math.Abs(float64(cols[0].AsVec3()[0]-0.5)) > 1e-5 {
		t.Errorf("inv3[0][0] = %v, want 0.5", cols[0].AsVec3()[0])
	}
}

// mulMatVec multiplies a column-major matrix by a column vector.
func mulMatVec(cols []Value, v []float32) []float32 {
	n := len(cols)
	out := make([]float32, n)
	for r := 0; r < n; r++ {
		var sum float32
		for c := 0; c < n; c++ {
			sum += matElem(cols, r, c) * v[c]
		}
		out[r] = sum
	}
	return out
}

func assertMatMulIdentity(t *testing.T, name string, m, inv Value) {
	t.Helper()
	a := m.AsArray()
	b := inv.AsArray()
	n := len(a)
	if len(b) != n {
		t.Fatalf("%s: column count mismatch %d vs %d", name, n, len(b))
	}
	for i := 0; i < n; i++ {
		e := make([]float32, n)
		e[i] = 1
		got := mulMatVec(b, mulMatVec(a, e))
		for j := 0; j < n; j++ {
			want := float32(0)
			if j == i {
				want = 1
			}
			if math.Abs(float64(got[j]-want)) > 1e-4 {
				t.Fatalf("%s: (A*inv) e_%d = %v, want identity column", name, i, got)
			}
		}
	}
}

func TestMatrixInverse3Nonsymmetric(t *testing.T) {
	// Column-major [[1,2,3],[0,1,4],[5,6,0]] — non-symmetric so transpose bugs show up.
	m := ValArray([]Value{
		ValVec3(1, 0, 5),
		ValVec3(2, 1, 6),
		ValVec3(3, 4, 0),
	})
	inv := matrixInverse(m)
	assertMatMulIdentity(t, "mat3", m, inv)
}

func TestMatrixInverse4Nonsymmetric(t *testing.T) {
	// Non-symmetric mat4 (column-major) with det ≠ 0.
	m := ValArray([]Value{
		ValVec4(1, 0, 2, 1),
		ValVec4(2, 1, 0, 0),
		ValVec4(0, 3, 1, 2),
		ValVec4(1, 0, 0, 1),
	})
	inv := matrixInverse(m)
	assertMatMulIdentity(t, "mat4", m, inv)
}
