package raster

import (
	"math"
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
// ClipPlane Intersect Tests
// =============================================================================

func TestClipPlaneIntersect(t *testing.T) {
	tests := []struct {
		name          string
		plane         ClipPlane
		v0, v1        ClipSpaceVertex
		wantT         float32
		wantPosApprox [4]float32
	}{
		{
			name:          "near_plane_midpoint",
			plane:         ClipPlaneNear,
			v0:            ClipSpaceVertex{Position: [4]float32{0, 0, 0.5, 1}},
			v1:            ClipSpaceVertex{Position: [4]float32{0, 0, -0.5, 1}},
			wantT:         0.5,
			wantPosApprox: [4]float32{0, 0, 0, 1},
		},
		{
			name:          "far_plane_quarter",
			plane:         ClipPlaneFar,
			v0:            ClipSpaceVertex{Position: [4]float32{0, 0, 0.5, 1}},
			v1:            ClipSpaceVertex{Position: [4]float32{0, 0, 1.5, 1}},
			wantT:         0.5,
			wantPosApprox: [4]float32{0, 0, 1, 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, gotT := tt.plane.Intersect(tt.v0, tt.v1)

			if math.Abs(float64(gotT-tt.wantT)) > 0.01 {
				t.Errorf("Intersect() t = %v, want %v", gotT, tt.wantT)
			}

			for i := 0; i < 4; i++ {
				if math.Abs(float64(result.Position[i]-tt.wantPosApprox[i])) > 0.01 {
					t.Errorf("Intersect() position[%d] = %v, want approx %v",
						i, result.Position[i], tt.wantPosApprox[i])
				}
			}
		})
	}
}

func TestClipPlaneIntersectWithAttributes(t *testing.T) {
	plane := ClipPlaneNear

	v0 := ClipSpaceVertex{
		Position:   [4]float32{0, 0, 0.5, 1},
		Attributes: []float32{1, 0, 0, 1}, // Red
	}
	v1 := ClipSpaceVertex{
		Position:   [4]float32{0, 0, -0.5, 1},
		Attributes: []float32{0, 0, 1, 1}, // Blue
	}

	result, tParam := plane.Intersect(v0, v1)

	// tParam should be 0.5 for midpoint
	if math.Abs(float64(tParam-0.5)) > 0.01 {
		t.Errorf("Intersect() t = %v, want 0.5", tParam)
	}

	// Attributes should be interpolated
	if len(result.Attributes) != 4 {
		t.Errorf("Expected 4 attributes, got %d", len(result.Attributes))
		return
	}

	// At t=0.5, color should be mix of red and blue
	// R: 1*0.5 + 0*0.5 = 0.5
	// B: 0*0.5 + 1*0.5 = 0.5
	expectedR := float32(0.5)
	expectedB := float32(0.5)

	if math.Abs(float64(result.Attributes[0]-expectedR)) > 0.01 {
		t.Errorf("Interpolated R = %v, want %v", result.Attributes[0], expectedR)
	}
	if math.Abs(float64(result.Attributes[2]-expectedB)) > 0.01 {
		t.Errorf("Interpolated B = %v, want %v", result.Attributes[2], expectedB)
	}
}

// =============================================================================
// ClipTriangleAgainstPlane Tests
// =============================================================================

func TestClipTriangleAllInside(t *testing.T) {
	// Triangle completely inside the near plane
	tri := [3]ClipSpaceVertex{
		{Position: [4]float32{0, 0, 0.5, 1}},
		{Position: [4]float32{0.5, 0, 0.5, 1}},
		{Position: [4]float32{0.25, 0.5, 0.5, 1}},
	}

	result := ClipTriangleAgainstPlane(tri, ClipPlaneNear)

	if len(result) != 1 {
		t.Errorf("Expected 1 triangle, got %d", len(result))
		return
	}

	// Original triangle should be returned
	for i := 0; i < 3; i++ {
		for j := 0; j < 4; j++ {
			if result[0][i].Position[j] != tri[i].Position[j] {
				t.Errorf("Triangle modified when it should not be")
			}
		}
	}
}

func TestClipTriangleAllOutside(t *testing.T) {
	// Triangle completely outside the near plane (all z < 0)
	tri := [3]ClipSpaceVertex{
		{Position: [4]float32{0, 0, -0.5, 1}},
		{Position: [4]float32{0.5, 0, -0.5, 1}},
		{Position: [4]float32{0.25, 0.5, -0.5, 1}},
	}

	result := ClipTriangleAgainstPlane(tri, ClipPlaneNear)

	if len(result) != 0 {
		t.Errorf("Expected 0 triangles, got %d", len(result))
	}
}

func TestClipTriangleOneVertexInside(t *testing.T) {
	// One vertex inside near plane, two outside
	tri := [3]ClipSpaceVertex{
		{Position: [4]float32{0, 0, 0.5, 1}},       // Inside (z >= 0)
		{Position: [4]float32{0.5, 0, -0.5, 1}},    // Outside
		{Position: [4]float32{0.25, 0.5, -0.5, 1}}, // Outside
	}

	result := ClipTriangleAgainstPlane(tri, ClipPlaneNear)

	if len(result) != 1 {
		t.Errorf("Expected 1 triangle, got %d", len(result))
		return
	}

	// The resulting triangle should have the inside vertex and 2 intersection points
	// All z values should be >= 0 (on or inside the near plane)
	for i := 0; i < 3; i++ {
		if result[0][i].Position[2] < -0.001 {
			t.Errorf("Clipped triangle vertex[%d] has z = %v, expected >= 0",
				i, result[0][i].Position[2])
		}
	}
}

func TestClipTriangleTwoVerticesInside(t *testing.T) {
	// Two vertices inside near plane, one outside
	tri := [3]ClipSpaceVertex{
		{Position: [4]float32{0, 0, -0.5, 1}},     // Outside (z < 0)
		{Position: [4]float32{0.5, 0, 0.5, 1}},    // Inside
		{Position: [4]float32{0.25, 0.5, 0.5, 1}}, // Inside
	}

	result := ClipTriangleAgainstPlane(tri, ClipPlaneNear)

	if len(result) != 2 {
		t.Errorf("Expected 2 triangles (quad split), got %d", len(result))
		return
	}

	// All resulting vertices should have z >= 0
	for i, resTri := range result {
		for j := 0; j < 3; j++ {
			if resTri[j].Position[2] < -0.001 {
				t.Errorf("Clipped triangle[%d] vertex[%d] has z = %v, expected >= 0",
					i, j, resTri[j].Position[2])
			}
		}
	}
}

// =============================================================================
// Full Frustum Clipping Tests
// =============================================================================

func TestClipTriangleNearPlane(t *testing.T) {
	// Triangle crossing near plane only
	tri := [3]ClipSpaceVertex{
		{Position: [4]float32{0, 0, 0.5, 1}},
		{Position: [4]float32{0.5, 0, 0.5, 1}},
		{Position: [4]float32{0.25, 0, -0.5, 1}}, // This vertex is behind near plane
	}

	result := ClipTriangleNearFar(tri)

	if len(result) == 0 {
		t.Error("Expected clipped triangles, got none")
		return
	}

	// Verify all resulting vertices are in front of near plane
	for i, resTri := range result {
		for j := 0; j < 3; j++ {
			if resTri[j].Position[2] < -0.001 {
				t.Errorf("Result[%d] vertex[%d] z = %v, expected >= 0", i, j, resTri[j].Position[2])
			}
		}
	}
}

func TestClipTriangleFarPlane(t *testing.T) {
	// Triangle crossing far plane only
	tri := [3]ClipSpaceVertex{
		{Position: [4]float32{0, 0, 0.5, 1}},
		{Position: [4]float32{0.5, 0, 0.5, 1}},
		{Position: [4]float32{0.25, 0, 1.5, 1}}, // This vertex is beyond far plane (z > w)
	}

	result := ClipTriangleNearFar(tri)

	if len(result) == 0 {
		t.Error("Expected clipped triangles, got none")
		return
	}

	// Verify all resulting vertices are in front of far plane (z <= w)
	for i, resTri := range result {
		for j := 0; j < 3; j++ {
			z := resTri[j].Position[2]
			w := resTri[j].Position[3]
			if z > w+0.001 {
				t.Errorf("Result[%d] vertex[%d] z = %v > w = %v", i, j, z, w)
			}
		}
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
// ClipTriangleFast Tests
// =============================================================================

func TestClipTriangleFast(t *testing.T) {
	tests := []struct {
		name      string
		tri       [3]ClipSpaceVertex
		wantCount int // Expected number of triangles
	}{
		{
			name: "trivial_accept",
			tri: [3]ClipSpaceVertex{
				{Position: [4]float32{0, 0, 0.5, 1}},
				{Position: [4]float32{0.5, 0, 0.5, 1}},
				{Position: [4]float32{0.25, 0.5, 0.5, 1}},
			},
			wantCount: 1,
		},
		{
			name: "trivial_reject",
			tri: [3]ClipSpaceVertex{
				{Position: [4]float32{-2, 0, 0.5, 1}},
				{Position: [4]float32{-2, 0.5, 0.5, 1}},
				{Position: [4]float32{-2, 0.25, 0.5, 1}},
			},
			wantCount: 0,
		},
		{
			name: "needs_clipping",
			tri: [3]ClipSpaceVertex{
				{Position: [4]float32{0, 0, 0.5, 1}},
				{Position: [4]float32{0.5, 0, 0.5, 1}},
				{Position: [4]float32{0.25, 0, -0.5, 1}},
			},
			wantCount: 2, // One vertex behind, produces 2 triangles
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ClipTriangleFast(tt.tri)
			if len(result) != tt.wantCount {
				t.Errorf("ClipTriangleFast() returned %d triangles, want %d", len(result), tt.wantCount)
			}
		})
	}
}

// =============================================================================
// Benchmarks
// =============================================================================

func BenchmarkClipTriangle(b *testing.B) {
	tri := [3]ClipSpaceVertex{
		{Position: [4]float32{0, 0, 0.5, 1}},
		{Position: [4]float32{0.5, 0, 0.5, 1}},
		{Position: [4]float32{0.25, 0, -0.5, 1}},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ClipTriangle(tri)
	}
}

func BenchmarkClipTriangleFast(b *testing.B) {
	tri := [3]ClipSpaceVertex{
		{Position: [4]float32{0, 0, 0.5, 1}},
		{Position: [4]float32{0.5, 0, 0.5, 1}},
		{Position: [4]float32{0.25, 0, -0.5, 1}},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ClipTriangleFast(tri)
	}
}

func BenchmarkClipTriangleTrivialAccept(b *testing.B) {
	tri := [3]ClipSpaceVertex{
		{Position: [4]float32{0, 0, 0.5, 1}},
		{Position: [4]float32{0.5, 0, 0.5, 1}},
		{Position: [4]float32{0.25, 0.5, 0.5, 1}},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ClipTriangleFast(tri)
	}
}

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
