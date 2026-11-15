//go:build !(js && wasm)

package shader

import "math"

// matrixDeterminant returns the determinant of a square matrix stored as a
// column-major TagArray of column vectors (GLSL.std.450 Determinant = 33).
func matrixDeterminant(mat Value) float32 {
	if mat.Tag != TagArray {
		return 0
	}
	cols := mat.AsArray()
	switch len(cols) {
	case 2:
		return det2(cols)
	case 3:
		return det3(cols)
	case 4:
		return det4(cols)
	default:
		return 0
	}
}

// matrixInverse returns the inverse of a square matrix (GLSL.std.450 MatrixInverse = 34).
// For singular matrices the result is a zero matrix (SPIR-V: undefined / poison).
func matrixInverse(mat Value) Value {
	if mat.Tag != TagArray {
		return mat
	}
	cols := mat.AsArray()
	switch len(cols) {
	case 2:
		return inverse2(cols)
	case 3:
		return inverse3(cols)
	case 4:
		return inverse4(cols)
	default:
		return mat
	}
}

func matElem(cols []Value, row, col int) float32 {
	if col < 0 || col >= len(cols) {
		return 0
	}
	return toFloat32(indexComposite(cols[col], uint32(row)))
}

func det2(cols []Value) float32 {
	a := matElem(cols, 0, 0)
	b := matElem(cols, 0, 1)
	c := matElem(cols, 1, 0)
	d := matElem(cols, 1, 1)
	return a*d - b*c
}

func det3(cols []Value) float32 {
	a00, a01, a02 := matElem(cols, 0, 0), matElem(cols, 0, 1), matElem(cols, 0, 2)
	a10, a11, a12 := matElem(cols, 1, 0), matElem(cols, 1, 1), matElem(cols, 1, 2)
	a20, a21, a22 := matElem(cols, 2, 0), matElem(cols, 2, 1), matElem(cols, 2, 2)
	return a00*(a11*a22-a12*a21) - a01*(a10*a22-a12*a20) + a02*(a10*a21-a11*a20)
}

func det4(cols []Value) float32 {
	// Cofactor expansion along the first row.
	var det float32
	sign := float32(1)
	for j := 0; j < 4; j++ {
		det += sign * matElem(cols, 0, j) * minor3(cols, 0, j)
		sign = -sign
	}
	return det
}

// minor3 returns the determinant of the 3x3 minor excluding row r and column c.
func minor3(cols []Value, r, c int) float32 {
	var m [3][3]float32
	ri := 0
	for row := 0; row < 4; row++ {
		if row == r {
			continue
		}
		ci := 0
		for col := 0; col < 4; col++ {
			if col == c {
				continue
			}
			m[ri][ci] = matElem(cols, row, col)
			ci++
		}
		ri++
	}
	return m[0][0]*(m[1][1]*m[2][2]-m[1][2]*m[2][1]) -
		m[0][1]*(m[1][0]*m[2][2]-m[1][2]*m[2][0]) +
		m[0][2]*(m[1][0]*m[2][1]-m[1][1]*m[2][0])
}

func inverse2(cols []Value) Value {
	det := det2(cols)
	if det == 0 || math.IsNaN(float64(det)) {
		return zeroMatrix(2, 2)
	}
	inv := 1 / det
	a := matElem(cols, 0, 0)
	b := matElem(cols, 0, 1)
	c := matElem(cols, 1, 0)
	d := matElem(cols, 1, 1)
	// adjugate transpose / det for column-major result columns.
	return ValArray([]Value{
		ValVec2(d*inv, -c*inv),
		ValVec2(-b*inv, a*inv),
	})
}

func inverse3(cols []Value) Value {
	det := det3(cols)
	if det == 0 || math.IsNaN(float64(det)) {
		return zeroMatrix(3, 3)
	}
	inv := 1 / det
	// Cofactor matrix (row,col), then transpose into column-major columns.
	var cof [3][3]float32
	for r := 0; r < 3; r++ {
		for c := 0; c < 3; c++ {
			sign := float32(1)
			if (r+c)%2 != 0 {
				sign = -1
			}
			cof[r][c] = sign * minor2From3(cols, r, c)
		}
	}
	// Adjugate is the transpose of the cofactor matrix; emit column-major columns.
	out := make([]Value, 3)
	for c := 0; c < 3; c++ {
		out[c] = ValVec3(cof[c][0]*inv, cof[c][1]*inv, cof[c][2]*inv)
	}
	return ValArray(out)
}

func minor2From3(cols []Value, r, c int) float32 {
	var m [2][2]float32
	ri := 0
	for row := 0; row < 3; row++ {
		if row == r {
			continue
		}
		ci := 0
		for col := 0; col < 3; col++ {
			if col == c {
				continue
			}
			m[ri][ci] = matElem(cols, row, col)
			ci++
		}
		ri++
	}
	return m[0][0]*m[1][1] - m[0][1]*m[1][0]
}

func inverse4(cols []Value) Value {
	det := det4(cols)
	if det == 0 || math.IsNaN(float64(det)) {
		return zeroMatrix(4, 4)
	}
	inv := 1 / det
	var cof [4][4]float32
	for r := 0; r < 4; r++ {
		for c := 0; c < 4; c++ {
			sign := float32(1)
			if (r+c)%2 != 0 {
				sign = -1
			}
			cof[r][c] = sign * minor3(cols, r, c)
		}
	}
	// Adjugate is the transpose of the cofactor matrix; emit column-major columns.
	out := make([]Value, 4)
	for c := 0; c < 4; c++ {
		out[c] = ValVec4(cof[c][0]*inv, cof[c][1]*inv, cof[c][2]*inv, cof[c][3]*inv)
	}
	return ValArray(out)
}

func zeroMatrix(rows, cols int) Value {
	out := make([]Value, cols)
	for c := 0; c < cols; c++ {
		switch rows {
		case 2:
			out[c] = ValVec2(0, 0)
		case 3:
			out[c] = ValVec3(0, 0, 0)
		default:
			out[c] = ValVec4(0, 0, 0, 0)
		}
	}
	return ValArray(out)
}
