package raster

// InterpolateFloat32 interpolates a single float32 attribute with perspective correction.
// b0, b1, b2 are barycentric coordinates.
// w0, w1, w2 are 1/w values from vertices (for perspective correction).
//
// The formula for perspective-correct interpolation:
//
//	result = (v0*b0*w0 + v1*b1*w1 + v2*b2*w2) / (b0*w0 + b1*w1 + b2*w2)
//
// Where w0, w1, w2 store 1/w from clip space division.
func InterpolateFloat32(
	v0, v1, v2 float32, // Attribute values at vertices
	b0, b1, b2 float32, // Barycentric coordinates
	w0, w1, w2 float32, // 1/w values
) float32 {
	oneOverW := b0*w0 + b1*w1 + b2*w2
	if oneOverW == 0 {
		// Fallback to linear interpolation (orthographic case)
		return v0*b0 + v1*b1 + v2*b2
	}
	return (v0*b0*w0 + v1*b1*w1 + v2*b2*w2) / oneOverW
}

// InterpolateFloat32Linear performs linear interpolation without perspective correction.
// This is faster but incorrect for perspective projections.
func InterpolateFloat32Linear(
	v0, v1, v2 float32,
	b0, b1, b2 float32,
) float32 {
	return v0*b0 + v1*b1 + v2*b2
}

// InterpolateVec2 interpolates a 2D vector (e.g., UV coordinates) with perspective correction.
func InterpolateVec2(
	v0, v1, v2 [2]float32,
	b0, b1, b2, w0, w1, w2 float32,
) [2]float32 {
	oneOverW := b0*w0 + b1*w1 + b2*w2
	if oneOverW == 0 {
		// Fallback to linear interpolation
		return [2]float32{
			v0[0]*b0 + v1[0]*b1 + v2[0]*b2,
			v0[1]*b0 + v1[1]*b1 + v2[1]*b2,
		}
	}
	invW := 1.0 / oneOverW
	return [2]float32{
		(v0[0]*b0*w0 + v1[0]*b1*w1 + v2[0]*b2*w2) * invW,
		(v0[1]*b0*w0 + v1[1]*b1*w1 + v2[1]*b2*w2) * invW,
	}
}

// InterpolateVec2Linear performs linear interpolation of a 2D vector.
func InterpolateVec2Linear(
	v0, v1, v2 [2]float32,
	b0, b1, b2 float32,
) [2]float32 {
	return [2]float32{
		v0[0]*b0 + v1[0]*b1 + v2[0]*b2,
		v0[1]*b0 + v1[1]*b1 + v2[1]*b2,
	}
}

// InterpolateVec3 interpolates a 3D vector (e.g., normals, RGB) with perspective correction.
func InterpolateVec3(
	v0, v1, v2 [3]float32,
	b0, b1, b2, w0, w1, w2 float32,
) [3]float32 {
	oneOverW := b0*w0 + b1*w1 + b2*w2
	if oneOverW == 0 {
		// Fallback to linear interpolation
		return [3]float32{
			v0[0]*b0 + v1[0]*b1 + v2[0]*b2,
			v0[1]*b0 + v1[1]*b1 + v2[1]*b2,
			v0[2]*b0 + v1[2]*b1 + v2[2]*b2,
		}
	}
	invW := 1.0 / oneOverW
	return [3]float32{
		(v0[0]*b0*w0 + v1[0]*b1*w1 + v2[0]*b2*w2) * invW,
		(v0[1]*b0*w0 + v1[1]*b1*w1 + v2[1]*b2*w2) * invW,
		(v0[2]*b0*w0 + v1[2]*b1*w1 + v2[2]*b2*w2) * invW,
	}
}

// InterpolateVec3Linear performs linear interpolation of a 3D vector.
func InterpolateVec3Linear(
	v0, v1, v2 [3]float32,
	b0, b1, b2 float32,
) [3]float32 {
	return [3]float32{
		v0[0]*b0 + v1[0]*b1 + v2[0]*b2,
		v0[1]*b0 + v1[1]*b1 + v2[1]*b2,
		v0[2]*b0 + v1[2]*b1 + v2[2]*b2,
	}
}

// InterpolateVec4 interpolates a 4D vector (e.g., RGBA colors) with perspective correction.
func InterpolateVec4(
	v0, v1, v2 [4]float32,
	b0, b1, b2, w0, w1, w2 float32,
) [4]float32 {
	oneOverW := b0*w0 + b1*w1 + b2*w2
	if oneOverW == 0 {
		// Fallback to linear interpolation
		return [4]float32{
			v0[0]*b0 + v1[0]*b1 + v2[0]*b2,
			v0[1]*b0 + v1[1]*b1 + v2[1]*b2,
			v0[2]*b0 + v1[2]*b1 + v2[2]*b2,
			v0[3]*b0 + v1[3]*b1 + v2[3]*b2,
		}
	}
	invW := 1.0 / oneOverW
	return [4]float32{
		(v0[0]*b0*w0 + v1[0]*b1*w1 + v2[0]*b2*w2) * invW,
		(v0[1]*b0*w0 + v1[1]*b1*w1 + v2[1]*b2*w2) * invW,
		(v0[2]*b0*w0 + v1[2]*b1*w1 + v2[2]*b2*w2) * invW,
		(v0[3]*b0*w0 + v1[3]*b1*w1 + v2[3]*b2*w2) * invW,
	}
}

// InterpolateVec4Linear performs linear interpolation of a 4D vector.
func InterpolateVec4Linear(
	v0, v1, v2 [4]float32,
	b0, b1, b2 float32,
) [4]float32 {
	return [4]float32{
		v0[0]*b0 + v1[0]*b1 + v2[0]*b2,
		v0[1]*b0 + v1[1]*b1 + v2[1]*b2,
		v0[2]*b0 + v1[2]*b1 + v2[2]*b2,
		v0[3]*b0 + v1[3]*b1 + v2[3]*b2,
	}
}

// InterpolateAttributes interpolates all attributes for a fragment with perspective correction.
// Returns nil if any input slice has different length than the others.
func InterpolateAttributes(
	attrs0, attrs1, attrs2 []float32,
	b0, b1, b2, w0, w1, w2 float32,
) []float32 {
	n := len(attrs0)
	if len(attrs1) != n || len(attrs2) != n {
		return nil
	}
	if n == 0 {
		return nil
	}

	result := make([]float32, n)
	oneOverW := b0*w0 + b1*w1 + b2*w2

	if oneOverW == 0 {
		// Fallback to linear interpolation
		for i := 0; i < n; i++ {
			result[i] = attrs0[i]*b0 + attrs1[i]*b1 + attrs2[i]*b2
		}
		return result
	}

	invW := 1.0 / oneOverW
	for i := 0; i < n; i++ {
		result[i] = (attrs0[i]*b0*w0 + attrs1[i]*b1*w1 + attrs2[i]*b2*w2) * invW
	}
	return result
}

// InterpolateAttributesLinear interpolates all attributes without perspective correction.
// Returns nil if any input slice has different length than the others.
func InterpolateAttributesLinear(
	attrs0, attrs1, attrs2 []float32,
	b0, b1, b2 float32,
) []float32 {
	n := len(attrs0)
	if len(attrs1) != n || len(attrs2) != n {
		return nil
	}
	if n == 0 {
		return nil
	}

	result := make([]float32, n)
	for i := 0; i < n; i++ {
		result[i] = attrs0[i]*b0 + attrs1[i]*b1 + attrs2[i]*b2
	}
	return result
}

// InterpolateDepth interpolates depth with perspective correction.
// Depth is stored in Z, and W contains 1/w for perspective correction.
func InterpolateDepth(
	z0, z1, z2 float32, // Depth values at vertices
	b0, b1, b2 float32, // Barycentric coordinates
	w0, w1, w2 float32, // 1/w values
) float32 {
	oneOverW := b0*w0 + b1*w1 + b2*w2
	if oneOverW == 0 {
		return z0*b0 + z1*b1 + z2*b2
	}
	return (z0*b0*w0 + z1*b1*w1 + z2*b2*w2) / oneOverW
}
