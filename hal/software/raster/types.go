package raster

// ClipSpaceVertex is the output of vertex shader processing.
// Position is in homogeneous clip space coordinates (x, y, z, w).
type ClipSpaceVertex struct {
	// Position in clip space. After perspective divide:
	// x/w, y/w are in [-1, 1] (NDC), z/w is depth [0, 1].
	Position [4]float32

	// Attributes are values to be interpolated across the triangle.
	// Common uses: color (RGBA), texture coordinates (UV), normals, etc.
	Attributes []float32
}

// ScreenVertex is a vertex after perspective divide and viewport transform.
// Coordinates are in screen space (pixels).
type ScreenVertex struct {
	// X is the horizontal screen coordinate in pixels.
	X float32

	// Y is the vertical screen coordinate in pixels.
	Y float32

	// Z is the depth value in range [0, 1], where 0 is near and 1 is far.
	Z float32

	// W stores 1/w from the original clip space vertex.
	// Used for perspective-correct interpolation.
	W float32

	// Attributes are values to be interpolated.
	// These are pre-divided by the original W for perspective correction.
	Attributes []float32
}

// Fragment is a candidate pixel generated during rasterization.
type Fragment struct {
	// X is the integer horizontal pixel coordinate.
	X int

	// Y is the integer vertical pixel coordinate.
	Y int

	// Depth is the interpolated depth value in [0, 1].
	Depth float32

	// Bary holds barycentric coordinates [w0, w1, w2].
	// These sum to 1.0 and indicate the fragment's position within the triangle.
	Bary [3]float32

	// Attributes are perspective-correct interpolated values.
	Attributes []float32
}

// Triangle represents three screen-space vertices forming a triangle.
type Triangle struct {
	V0 ScreenVertex
	V1 ScreenVertex
	V2 ScreenVertex
}

// Viewport defines the rectangular area of the framebuffer to render into.
type Viewport struct {
	// X is the left edge of the viewport in pixels.
	X int

	// Y is the top edge of the viewport in pixels.
	Y int

	// Width of the viewport in pixels.
	Width int

	// Height of the viewport in pixels.
	Height int

	// MinDepth is the near depth value (typically 0.0).
	MinDepth float32

	// MaxDepth is the far depth value (typically 1.0).
	MaxDepth float32
}

// CompareFunc specifies the comparison function for depth/stencil testing.
type CompareFunc uint8

const (
	// CompareNever always fails the test.
	CompareNever CompareFunc = iota

	// CompareLess passes if source < destination.
	CompareLess

	// CompareEqual passes if source == destination.
	CompareEqual

	// CompareLessEqual passes if source <= destination.
	CompareLessEqual

	// CompareGreater passes if source > destination.
	CompareGreater

	// CompareNotEqual passes if source != destination.
	CompareNotEqual

	// CompareGreaterEqual passes if source >= destination.
	CompareGreaterEqual

	// CompareAlways always passes the test.
	CompareAlways
)

// CullMode specifies which triangle faces to cull.
type CullMode uint8

const (
	// CullNone disables face culling.
	CullNone CullMode = iota

	// CullFront culls front-facing triangles.
	CullFront

	// CullBack culls back-facing triangles.
	CullBack
)

// FrontFace specifies the winding order for front-facing triangles.
type FrontFace uint8

const (
	// FrontFaceCCW treats counter-clockwise winding as front-facing.
	FrontFaceCCW FrontFace = iota

	// FrontFaceCW treats clockwise winding as front-facing.
	FrontFaceCW
)

// min2 returns the minimum of two float32 values.
func min2(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

// max2 returns the maximum of two float32 values.
func max2(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

// min3 returns the minimum of three float32 values.
func min3(a, b, c float32) float32 {
	return min2(min2(a, b), c)
}

// max3 returns the maximum of three float32 values.
func max3(a, b, c float32) float32 {
	return max2(max2(a, b), c)
}

// minInt returns the minimum of two int values.
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// maxInt returns the maximum of two int values.
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
