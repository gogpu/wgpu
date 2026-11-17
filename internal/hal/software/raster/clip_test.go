//go:build !(js && wasm)

package raster

import (
	"testing"
)

// =============================================================================
// ClipPlane Distance Tests
// =============================================================================

func TestClipPlaneDistance(t *testing.T) {
	tests := []struct {
		name     string
		plane    ClipPlane
		vertex   ClipSpaceVertex
		wantSign int // -1, 0, or 1
	}{
		{
			name:     "near_plane_inside",
			plane:    ClipPlaneNear,
			vertex:   ClipSpaceVertex{Position: [4]float32{0, 0, 0.5, 1}},
			wantSign: 1,
		},
		{
			name:     "near_plane_outside",
			plane:    ClipPlaneNear,
			vertex:   ClipSpaceVertex{Position: [4]float32{0, 0, -0.5, 1}},
			wantSign: -1,
		},
		{
			name:     "near_plane_on_plane",
			plane:    ClipPlaneNear,
			vertex:   ClipSpaceVertex{Position: [4]float32{0, 0, 0, 1}},
			wantSign: 0,
		},
		{
			name:     "far_plane_inside",
			plane:    ClipPlaneFar,
			vertex:   ClipSpaceVertex{Position: [4]float32{0, 0, 0.5, 1}},
			wantSign: 1,
		},
		{
			name:     "far_plane_outside",
			plane:    ClipPlaneFar,
			vertex:   ClipSpaceVertex{Position: [4]float32{0, 0, 1.5, 1}},
			wantSign: -1,
		},
		{
			name:     "left_plane_inside",
			plane:    ClipPlaneLeft,
			vertex:   ClipSpaceVertex{Position: [4]float32{0, 0, 0.5, 1}},
			wantSign: 1,
		},
		{
			name:     "left_plane_outside",
			plane:    ClipPlaneLeft,
			vertex:   ClipSpaceVertex{Position: [4]float32{-2, 0, 0.5, 1}},
			wantSign: -1,
		},
		{
			name:     "right_plane_inside",
			plane:    ClipPlaneRight,
			vertex:   ClipSpaceVertex{Position: [4]float32{0, 0, 0.5, 1}},
			wantSign: 1,
		},
		{
			name:     "right_plane_outside",
			plane:    ClipPlaneRight,
			vertex:   ClipSpaceVertex{Position: [4]float32{2, 0, 0.5, 1}},
			wantSign: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.plane.Distance(tt.vertex)
			gotSign := signFloat(got)
			if gotSign != tt.wantSign {
				t.Errorf("Distance() = %v (sign %v), want sign %v", got, gotSign, tt.wantSign)
			}
		})
	}
}

// =============================================================================
// Trivial Accept/Reject Tests
// =============================================================================

func TestIsCompletelyOutside(t *testing.T) {
	tests := []struct {
		name string
		tri  [3]ClipSpaceVertex
		want bool
	}{
		{
			name: "inside_frustum",
			tri: [3]ClipSpaceVertex{
				{Position: [4]float32{0, 0, 0.5, 1}},
				{Position: [4]float32{0.5, 0, 0.5, 1}},
				{Position: [4]float32{0.25, 0.5, 0.5, 1}},
			},
			want: false,
		},
		{
			name: "all_behind_near",
			tri: [3]ClipSpaceVertex{
				{Position: [4]float32{0, 0, -0.5, 1}},
				{Position: [4]float32{0.5, 0, -0.5, 1}},
				{Position: [4]float32{0.25, 0.5, -0.5, 1}},
			},
			want: true,
		},
		{
			name: "all_left_of_frustum",
			tri: [3]ClipSpaceVertex{
				{Position: [4]float32{-2, 0, 0.5, 1}},
				{Position: [4]float32{-2, 0.5, 0.5, 1}},
				{Position: [4]float32{-2, 0.25, 0.5, 1}},
			},
			want: true,
		},
		{
			name: "partial_crossing",
			tri: [3]ClipSpaceVertex{
				{Position: [4]float32{0, 0, 0.5, 1}},
				{Position: [4]float32{0.5, 0, 0.5, 1}},
				{Position: [4]float32{0.25, 0, -0.5, 1}}, // Behind near plane
			},
			want: false, // Not completely outside - one vertex inside
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsCompletelyOutside(tt.tri)
			if got != tt.want {
				t.Errorf("IsCompletelyOutside() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsCompletelyInside(t *testing.T) {
	tests := []struct {
		name string
		tri  [3]ClipSpaceVertex
		want bool
	}{
		{
			name: "fully_inside",
			tri: [3]ClipSpaceVertex{
				{Position: [4]float32{0, 0, 0.5, 1}},
				{Position: [4]float32{0.5, 0, 0.5, 1}},
				{Position: [4]float32{0.25, 0.5, 0.5, 1}},
			},
			want: true,
		},
		{
			name: "one_vertex_outside",
			tri: [3]ClipSpaceVertex{
				{Position: [4]float32{0, 0, 0.5, 1}},
				{Position: [4]float32{0.5, 0, 0.5, 1}},
				{Position: [4]float32{0.25, 0, -0.5, 1}}, // Behind near plane
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsCompletelyInside(tt.tri)
			if got != tt.want {
				t.Errorf("IsCompletelyInside() = %v, want %v", got, tt.want)
			}
		})
	}
}

// =============================================================================
// Outcode Tests
// =============================================================================

func TestComputeOutcode(t *testing.T) {
	tests := []struct {
		name   string
		vertex ClipSpaceVertex
		want   Outcode
	}{
		{
			name:   "inside_all",
			vertex: ClipSpaceVertex{Position: [4]float32{0, 0, 0.5, 1}},
			want:   0,
		},
		{
			name:   "outside_near",
			vertex: ClipSpaceVertex{Position: [4]float32{0, 0, -0.5, 1}},
			want:   OutcodeNear,
		},
		{
			name:   "outside_far",
			vertex: ClipSpaceVertex{Position: [4]float32{0, 0, 1.5, 1}},
			want:   OutcodeFar,
		},
		{
			name:   "outside_left",
			vertex: ClipSpaceVertex{Position: [4]float32{-1.5, 0, 0.5, 1}},
			want:   OutcodeLeft,
		},
		{
			name:   "outside_right",
			vertex: ClipSpaceVertex{Position: [4]float32{1.5, 0, 0.5, 1}},
			want:   OutcodeRight,
		},
		{
			name:   "outside_bottom",
			vertex: ClipSpaceVertex{Position: [4]float32{0, -1.5, 0.5, 1}},
			want:   OutcodeBottom,
		},
		{
			name:   "outside_top",
			vertex: ClipSpaceVertex{Position: [4]float32{0, 1.5, 0.5, 1}},
			want:   OutcodeTop,
		},
		{
			name:   "outside_multiple",
			vertex: ClipSpaceVertex{Position: [4]float32{1.5, 1.5, 0.5, 1}},
			want:   OutcodeRight | OutcodeTop,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeOutcode(tt.vertex)
			if got != tt.want {
				t.Errorf("ComputeOutcode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTriangleTrivialReject(t *testing.T) {
	tests := []struct {
		name string
		tri  [3]ClipSpaceVertex
		want bool
	}{
		{
			name: "inside_frustum",
			tri: [3]ClipSpaceVertex{
				{Position: [4]float32{0, 0, 0.5, 1}},
				{Position: [4]float32{0.5, 0, 0.5, 1}},
				{Position: [4]float32{0.25, 0.5, 0.5, 1}},
			},
			want: false,
		},
		{
			name: "all_left",
			tri: [3]ClipSpaceVertex{
				{Position: [4]float32{-2, 0, 0.5, 1}},
				{Position: [4]float32{-2, 0.5, 0.5, 1}},
				{Position: [4]float32{-2, 0.25, 0.5, 1}},
			},
			want: true,
		},
		{
			name: "mixed_outside",
			tri: [3]ClipSpaceVertex{
				{Position: [4]float32{-2, 0, 0.5, 1}}, // Left
				{Position: [4]float32{2, 0, 0.5, 1}},  // Right
				{Position: [4]float32{0, 0, 0.5, 1}},  // Inside
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TriangleTrivialReject(tt.tri)
			if got != tt.want {
				t.Errorf("TriangleTrivialReject() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTriangleTrivialAccept(t *testing.T) {
	tests := []struct {
		name string
		tri  [3]ClipSpaceVertex
		want bool
	}{
		{
			name: "fully_inside",
			tri: [3]ClipSpaceVertex{
				{Position: [4]float32{0, 0, 0.5, 1}},
				{Position: [4]float32{0.5, 0, 0.5, 1}},
				{Position: [4]float32{0.25, 0.5, 0.5, 1}},
			},
			want: true,
		},
		{
			name: "one_outside",
			tri: [3]ClipSpaceVertex{
				{Position: [4]float32{0, 0, 0.5, 1}},
				{Position: [4]float32{0.5, 0, 0.5, 1}},
				{Position: [4]float32{2, 0, 0.5, 1}}, // Outside right
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TriangleTrivialAccept(tt.tri)
			if got != tt.want {
				t.Errorf("TriangleTrivialAccept() = %v, want %v", got, tt.want)
			}
		})
	}
}

// =============================================================================
// Benchmarks
// =============================================================================

func BenchmarkComputeOutcode(b *testing.B) {
	vertex := ClipSpaceVertex{Position: [4]float32{0.5, 0.5, 0.5, 1}}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ComputeOutcode(vertex)
	}
}

// =============================================================================
// Helper Functions
// =============================================================================

func signFloat(v float32) int {
	if v > 0.001 {
		return 1
	}
	if v < -0.001 {
		return -1
	}
	return 0
}
