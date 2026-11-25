// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

// Package rtexample holds helpers shared by the ray tracing example ports
// (examples/ray-traced-triangle, examples/ray-scene).
//
// The Rust originals use glam for matrix math and the wgpu example framework
// for headless rendering and screenshot comparison. This package provides
// the minimal equivalents so the example code can stay a line-by-line port.
package rtexample

import (
	"encoding/binary"
	"math"
)

// Vec3 is a 3-component float32 vector.
type Vec3 [3]float32

// Mat4 is a 4x4 float32 matrix stored column-major, matching glam::Mat4 and
// the WGSL mat4x4<f32> memory layout. Element (row r, column c) is m[c*4+r].
type Mat4 [16]float32

// Identity returns the identity matrix.
func Identity() Mat4 {
	return Mat4{
		1, 0, 0, 0,
		0, 1, 0, 0,
		0, 0, 1, 0,
		0, 0, 0, 1,
	}
}

// At returns the element at row r, column c.
func (m Mat4) At(r, c int) float32 { return m[c*4+r] }

// Mul returns m * n.
func (m Mat4) Mul(n Mat4) Mat4 {
	var out Mat4
	for c := range 4 {
		for r := range 4 {
			var sum float32
			for k := range 4 {
				sum += m.At(r, k) * n.At(k, c)
			}
			out[c*4+r] = sum
		}
	}
	return out
}

// Transpose returns the transpose of m.
func (m Mat4) Transpose() Mat4 {
	var out Mat4
	for c := range 4 {
		for r := range 4 {
			out[r*4+c] = m[c*4+r]
		}
	}
	return out
}

// Rows3x4 returns the top three rows of m in row-major order. This is the
// affine transform layout expected by TlasInstance.Transform, and is what the
// Rust examples compute with `m.transpose().to_cols_array()[..12]`.
func (m Mat4) Rows3x4() [12]float32 {
	var out [12]float32
	for r := range 3 {
		for c := range 4 {
			out[r*4+c] = m.At(r, c)
		}
	}
	return out
}

// AppendBytes appends m to b as little-endian float32s in column-major order.
func (m Mat4) AppendBytes(b []byte) []byte {
	for _, v := range m {
		b = binary.LittleEndian.AppendUint32(b, math.Float32bits(v))
	}
	return b
}

// Inverse returns the inverse of m. The result is undefined if m is singular.
// Matches glam::Mat4::inverse.
func (m Mat4) Inverse() Mat4 {
	// Cofactor expansion in float64 for accuracy, then rounded to float32.
	var a [16]float64
	for i, v := range m {
		a[i] = float64(v)
	}
	var inv [16]float64
	inv[0] = a[5]*a[10]*a[15] - a[5]*a[11]*a[14] - a[9]*a[6]*a[15] + a[9]*a[7]*a[14] + a[13]*a[6]*a[11] - a[13]*a[7]*a[10]
	inv[4] = -a[4]*a[10]*a[15] + a[4]*a[11]*a[14] + a[8]*a[6]*a[15] - a[8]*a[7]*a[14] - a[12]*a[6]*a[11] + a[12]*a[7]*a[10]
	inv[8] = a[4]*a[9]*a[15] - a[4]*a[11]*a[13] - a[8]*a[5]*a[15] + a[8]*a[7]*a[13] + a[12]*a[5]*a[11] - a[12]*a[7]*a[9]
	inv[12] = -a[4]*a[9]*a[14] + a[4]*a[10]*a[13] + a[8]*a[5]*a[14] - a[8]*a[6]*a[13] - a[12]*a[5]*a[10] + a[12]*a[6]*a[9]
	inv[1] = -a[1]*a[10]*a[15] + a[1]*a[11]*a[14] + a[9]*a[2]*a[15] - a[9]*a[3]*a[14] - a[13]*a[2]*a[11] + a[13]*a[3]*a[10]
	inv[5] = a[0]*a[10]*a[15] - a[0]*a[11]*a[14] - a[8]*a[2]*a[15] + a[8]*a[3]*a[14] + a[12]*a[2]*a[11] - a[12]*a[3]*a[10]
	inv[9] = -a[0]*a[9]*a[15] + a[0]*a[11]*a[13] + a[8]*a[1]*a[15] - a[8]*a[3]*a[13] - a[12]*a[1]*a[11] + a[12]*a[3]*a[9]
	inv[13] = a[0]*a[9]*a[14] - a[0]*a[10]*a[13] - a[8]*a[1]*a[14] + a[8]*a[2]*a[13] + a[12]*a[1]*a[10] - a[12]*a[2]*a[9]
	inv[2] = a[1]*a[6]*a[15] - a[1]*a[7]*a[14] - a[5]*a[2]*a[15] + a[5]*a[3]*a[14] + a[13]*a[2]*a[7] - a[13]*a[3]*a[6]
	inv[6] = -a[0]*a[6]*a[15] + a[0]*a[7]*a[14] + a[4]*a[2]*a[15] - a[4]*a[3]*a[14] - a[12]*a[2]*a[7] + a[12]*a[3]*a[6]
	inv[10] = a[0]*a[5]*a[15] - a[0]*a[7]*a[13] - a[4]*a[1]*a[15] + a[4]*a[3]*a[13] + a[12]*a[1]*a[7] - a[12]*a[3]*a[5]
	inv[14] = -a[0]*a[5]*a[14] + a[0]*a[6]*a[13] + a[4]*a[1]*a[14] - a[4]*a[2]*a[13] - a[12]*a[1]*a[6] + a[12]*a[2]*a[5]
	inv[3] = -a[1]*a[6]*a[11] + a[1]*a[7]*a[10] + a[5]*a[2]*a[11] - a[5]*a[3]*a[10] - a[9]*a[2]*a[7] + a[9]*a[3]*a[6]
	inv[7] = a[0]*a[6]*a[11] - a[0]*a[7]*a[10] - a[4]*a[2]*a[11] + a[4]*a[3]*a[10] + a[8]*a[2]*a[7] - a[8]*a[3]*a[6]
	inv[11] = -a[0]*a[5]*a[11] + a[0]*a[7]*a[9] + a[4]*a[1]*a[11] - a[4]*a[3]*a[9] - a[8]*a[1]*a[7] + a[8]*a[3]*a[5]
	inv[15] = a[0]*a[5]*a[10] - a[0]*a[6]*a[9] - a[4]*a[1]*a[10] + a[4]*a[2]*a[9] + a[8]*a[1]*a[6] - a[8]*a[2]*a[5]

	det := a[0]*inv[0] + a[1]*inv[4] + a[2]*inv[8] + a[3]*inv[12]
	var out Mat4
	for i := range inv {
		out[i] = float32(inv[i] / det)
	}
	return out
}

// Translation returns a translation matrix (glam::Mat4::from_translation).
func Translation(v Vec3) Mat4 {
	m := Identity()
	m[12], m[13], m[14] = v[0], v[1], v[2]
	return m
}

// RotationX returns a rotation of angle radians about +X (glam::Mat4::from_rotation_x).
func RotationX(angle float32) Mat4 {
	s, c := sincos(angle)
	return Mat4{
		1, 0, 0, 0,
		0, c, s, 0,
		0, -s, c, 0,
		0, 0, 0, 1,
	}
}

// RotationY returns a rotation of angle radians about +Y (glam::Mat4::from_rotation_y).
func RotationY(angle float32) Mat4 {
	s, c := sincos(angle)
	return Mat4{
		c, 0, -s, 0,
		0, 1, 0, 0,
		s, 0, c, 0,
		0, 0, 0, 1,
	}
}

// RotationZ returns a rotation of angle radians about +Z (glam::Mat4::from_rotation_z).
func RotationZ(angle float32) Mat4 {
	s, c := sincos(angle)
	return Mat4{
		c, s, 0, 0,
		-s, c, 0, 0,
		0, 0, 1, 0,
		0, 0, 0, 1,
	}
}

// RotationTranslationEulerXYZ returns the equivalent of
// glam::Mat4::from_rotation_translation(Quat::from_euler(EulerRot::XYZ, a, b, c), t).
// glam composes EulerRot::XYZ as Rx(a) * Ry(b) * Rz(c).
func RotationTranslationEulerXYZ(a, b, c float32, t Vec3) Mat4 {
	m := RotationX(a).Mul(RotationY(b)).Mul(RotationZ(c))
	m[12], m[13], m[14] = t[0], t[1], t[2]
	return m
}

// LookAtRH returns a right-handed view matrix looking from eye towards center
// (glam::camera::rh::view::look_at_mat4 / glam::Mat4::look_at_rh).
func LookAtRH(eye, center, up Vec3) Mat4 {
	f := normalize(sub(center, eye))
	s := normalize(cross(f, up))
	u := cross(s, f)
	return Mat4{
		s[0], u[0], -f[0], 0,
		s[1], u[1], -f[1], 0,
		s[2], u[2], -f[2], 0,
		-dot(eye, s), -dot(eye, u), dot(eye, f), 1,
	}
}

// PerspectiveRH returns a right-handed perspective projection with a [0, 1]
// depth range (glam::camera::rh::proj::directx::perspective /
// glam::Mat4::perspective_rh).
func PerspectiveRH(fovYRadians, aspect, near, far float32) Mat4 {
	h := float32(1 / math.Tan(float64(fovYRadians)*0.5))
	w := h / aspect
	r := far / (near - far)
	return Mat4{
		w, 0, 0, 0,
		0, h, 0, 0,
		0, 0, r, -1,
		0, 0, r * near, 0,
	}
}

// Radians converts degrees to radians.
func Radians(deg float32) float32 { return deg * math.Pi / 180 }

func sincos(a float32) (float32, float32) {
	s, c := math.Sincos(float64(a))
	return float32(s), float32(c)
}

func sub(a, b Vec3) Vec3 { return Vec3{a[0] - b[0], a[1] - b[1], a[2] - b[2]} }

func dot(a, b Vec3) float32 { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }

func cross(a, b Vec3) Vec3 {
	return Vec3{
		a[1]*b[2] - a[2]*b[1],
		a[2]*b[0] - a[0]*b[2],
		a[0]*b[1] - a[1]*b[0],
	}
}

func normalize(v Vec3) Vec3 {
	l := float32(math.Sqrt(float64(dot(v, v))))
	return Vec3{v[0] / l, v[1] / l, v[2] / l}
}
