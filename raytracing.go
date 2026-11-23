package wgpu

import "github.com/gogpu/gputypes"

// TlasInstance is one instance in a TLAS: a BLAS placed with a transform.
//
// Matches Rust wgpu's TlasInstance.
type TlasInstance struct {
	// Blas is the bottom-level acceleration structure this instance places.
	Blas *Blas
	// Transform is a 3x4 row-major affine transform (the top three rows of a
	// 4x4 matrix).
	Transform [12]float32
	// CustomData is exposed to shaders as instance_custom_data. Only the low
	// 24 bits may be set; building a TLAS with larger values is an error.
	CustomData uint32
	// Mask is ANDed with a ray's cull mask; the instance is skipped when the
	// result is zero.
	Mask uint8
}

// NewTlasInstance returns a TLAS instance of blas.
//
// Matches Rust wgpu's TlasInstance::new.
func NewTlasInstance(blas *Blas, transform [12]float32, customData uint32, mask uint8) *TlasInstance {
	return &TlasInstance{Blas: blas, Transform: transform, CustomData: customData, Mask: mask}
}

// BlasTriangleGeometry is one triangle geometry of a BLAS build.
//
// Matches Rust wgpu's BlasTriangleGeometry.
type BlasTriangleGeometry struct {
	// Size must be compatible with the size descriptor at the same index
	// given to Device.CreateBlas: the same flags, vertex format and index
	// format, and no more vertices or indices.
	Size *gputypes.BlasTriangleGeometrySizeDescriptor
	// VertexBuffer holds the vertex positions. It needs BufferUsageBlasInput.
	VertexBuffer *Buffer
	// FirstVertex is the index of the first vertex in VertexBuffer.
	FirstVertex uint32
	// VertexStride is the distance in bytes between vertices.
	VertexStride uint64
	// IndexBuffer is optional. It needs BufferUsageBlasInput.
	IndexBuffer *Buffer
	// FirstIndex is the index of the first index; required iff IndexBuffer is set.
	FirstIndex *uint32
	// TransformBuffer optionally holds a 3x4 row-major transform; it requires
	// ASFlagUseTransform on the BLAS and BufferUsageBlasInput.
	TransformBuffer *Buffer
	// TransformBufferOffset is required iff TransformBuffer is set, and must
	// be a multiple of 16.
	TransformBufferOffset *uint64
}

// BlasAabbGeometry is one AABB geometry of a BLAS build.
//
// Matches Rust wgpu's BlasAabbGeometry.
type BlasAabbGeometry struct {
	// Size must be compatible with the size descriptor at the same index
	// given to Device.CreateBlas.
	Size *gputypes.BlasAABBGeometrySizeDescriptor
	// Stride is the distance in bytes between AABBs (at least 24, a multiple of 8).
	Stride uint64
	// AabbBuffer holds the AABBs (min xyz, max xyz as float32). It needs
	// BufferUsageBlasInput.
	AabbBuffer *Buffer
	// PrimitiveOffset is the byte offset of the first AABB (a multiple of 8).
	PrimitiveOffset uint32
}

// BlasGeometries is the geometry of a BLAS build. Only one of Triangles and
// AABBs may be set, matching the kind the BLAS was created with.
//
// Matches Rust wgpu's BlasGeometries enum.
type BlasGeometries struct {
	Triangles []BlasTriangleGeometry
	AABBs     []BlasAabbGeometry
}

// BlasBuildEntry pairs a BLAS with the geometry to build it from.
//
// Matches Rust wgpu's BlasBuildEntry.
type BlasBuildEntry struct {
	Blas     *Blas
	Geometry BlasGeometries
}
