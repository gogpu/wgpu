//go:build !(js && wasm)

// CPU-based BVH ray tracing for CI/testing without GPU hardware.
//
// This file provides acceleration structure construction and ray-primitive
// intersection for the software backend. It implements the same hal.Device
// and hal.CommandEncoder methods as Vulkan/DX12/Metal, but executes entirely
// on the CPU using a binary BVH tree.
//
// BVH construction uses midpoint split along the longest AABB axis, recursing
// until each leaf contains at most maxLeafTriangles primitives. This is
// intentionally simple (not SAH) because the software backend targets CI
// correctness, not production performance.
//
// Ray-triangle intersection uses the Moller-Trumbore algorithm.
// Ray-AABB intersection uses the slab method.
// BVH traversal finds the closest hit via recursive front-to-back walk.

package software

import (
	"encoding/binary"
	"math"

	"github.com/gogpu/wgpu/hal"
)

// maxLeafTriangles is the maximum number of triangles in a BVH leaf node.
// Kept small to maintain reasonable tree depth for CI workloads.
const maxLeafTriangles = 4

// AccelerationStructure implements hal.AccelerationStructure for the software
// backend. It holds a CPU-side BVH tree built from triangle or AABB geometry.
type AccelerationStructure struct {
	Resource
	id     uint64
	format hal.AccelerationStructureFormat
	bvh    *BVHNode
	size   uint64

	// TLAS-specific: instances referencing BLAS acceleration structures.
	instances []TLASInstanceData
}

// NativeHandle returns a unique identifier for this acceleration structure.
func (a *AccelerationStructure) NativeHandle() uintptr { return uintptr(a.id) }

// Compile-time interface assertion.
var _ hal.AccelerationStructure = (*AccelerationStructure)(nil)

// BVHNode is a node in a binary bounding volume hierarchy.
// Internal nodes have non-nil Left and Right children with empty primitive
// slices. Leaf nodes have nil children with non-empty Triangles or AABBs.
type BVHNode struct {
	BBox      AABB
	Left      *BVHNode
	Right     *BVHNode
	Triangles []Triangle
	AABBs     []AABBPrimitive
}

// NodeCount returns the total number of nodes in the subtree rooted at n.
func (n *BVHNode) NodeCount() int {
	if n == nil {
		return 0
	}
	return 1 + n.Left.NodeCount() + n.Right.NodeCount()
}

// AABB is an axis-aligned bounding box defined by minimum and maximum corners.
type AABB struct {
	Min [3]float32
	Max [3]float32
}

// Triangle holds a single triangle primitive with geometry and primitive
// indices for hit identification during traversal.
type Triangle struct {
	V0, V1, V2     [3]float32
	GeometryIndex  uint32
	PrimitiveIndex uint32
}

// AABBPrimitive holds an axis-aligned bounding box primitive with indices
// for procedural geometry hit testing.
type AABBPrimitive struct {
	Min            [3]float32
	Max            [3]float32
	GeometryIndex  uint32
	PrimitiveIndex uint32
}

// TLASInstanceData represents a TLAS instance referencing a BLAS.
type TLASInstanceData struct {
	Transform  [12]float32
	CustomData uint32
	Mask       uint8
	BLASRef    *AccelerationStructure
}

// -------------------------------------------------------------------------
// AABB helpers
// -------------------------------------------------------------------------

// emptyAABB returns an AABB with inverted extents, suitable as an identity
// for union operations.
func emptyAABB() AABB {
	inf := float32(math.MaxFloat32)
	return AABB{
		Min: [3]float32{inf, inf, inf},
		Max: [3]float32{-inf, -inf, -inf},
	}
}

// union expands box a to also contain box b.
func (a AABB) union(b AABB) AABB {
	return AABB{
		Min: [3]float32{
			minF(a.Min[0], b.Min[0]),
			minF(a.Min[1], b.Min[1]),
			minF(a.Min[2], b.Min[2]),
		},
		Max: [3]float32{
			maxF(a.Max[0], b.Max[0]),
			maxF(a.Max[1], b.Max[1]),
			maxF(a.Max[2], b.Max[2]),
		},
	}
}

// longestAxis returns the axis index (0=X, 1=Y, 2=Z) with the largest extent.
func (a AABB) longestAxis() int {
	dx := a.Max[0] - a.Min[0]
	dy := a.Max[1] - a.Min[1]
	dz := a.Max[2] - a.Min[2]
	if dx >= dy && dx >= dz {
		return 0
	}
	if dy >= dz {
		return 1
	}
	return 2
}

// triangleBBox returns the AABB enclosing a single triangle.
func triangleBBox(tri *Triangle) AABB {
	return AABB{
		Min: [3]float32{
			minF(tri.V0[0], minF(tri.V1[0], tri.V2[0])),
			minF(tri.V0[1], minF(tri.V1[1], tri.V2[1])),
			minF(tri.V0[2], minF(tri.V1[2], tri.V2[2])),
		},
		Max: [3]float32{
			maxF(tri.V0[0], maxF(tri.V1[0], tri.V2[0])),
			maxF(tri.V0[1], maxF(tri.V1[1], tri.V2[1])),
			maxF(tri.V0[2], maxF(tri.V1[2], tri.V2[2])),
		},
	}
}

// aabbPrimBBox returns the AABB enclosing a single AABB primitive.
func aabbPrimBBox(prim *AABBPrimitive) AABB {
	return AABB{Min: prim.Min, Max: prim.Max}
}

// triangleCentroid returns the centroid of a triangle along a given axis.
func triangleCentroid(tri *Triangle, axis int) float32 {
	return (tri.V0[axis] + tri.V1[axis] + tri.V2[axis]) / 3.0
}

// -------------------------------------------------------------------------
// BVH construction
// -------------------------------------------------------------------------

// BuildBVH constructs a binary BVH from a slice of triangles using midpoint
// split along the longest axis. Leaf nodes contain at most maxLeafTriangles
// primitives. Returns nil for empty input.
func BuildBVH(triangles []Triangle) *BVHNode {
	if len(triangles) == 0 {
		return nil
	}
	return buildBVHRecursive(triangles)
}

// BuildBVHFromAABBs constructs a binary BVH from a slice of AABB primitives.
func BuildBVHFromAABBs(aabbs []AABBPrimitive) *BVHNode {
	if len(aabbs) == 0 {
		return nil
	}
	return buildBVHAABBRecursive(aabbs)
}

func buildBVHRecursive(tris []Triangle) *BVHNode {
	// Compute the bounding box of all triangles.
	bbox := emptyAABB()
	for i := range tris {
		bbox = bbox.union(triangleBBox(&tris[i]))
	}

	// Base case: create a leaf node.
	if len(tris) <= maxLeafTriangles {
		leaf := make([]Triangle, len(tris))
		copy(leaf, tris)
		return &BVHNode{
			BBox:      bbox,
			Triangles: leaf,
		}
	}

	// Split along the longest axis at the centroid midpoint.
	axis := bbox.longestAxis()
	mid := (bbox.Min[axis] + bbox.Max[axis]) / 2.0

	// Partition triangles: those with centroid < mid go left, rest go right.
	// In-place partitioning to avoid extra allocations.
	i := 0
	for j := range tris {
		if triangleCentroid(&tris[j], axis) < mid {
			tris[i], tris[j] = tris[j], tris[i]
			i++
		}
	}

	// Fallback: if all triangles ended up on one side, split evenly.
	if i == 0 || i == len(tris) {
		i = len(tris) / 2
	}

	return &BVHNode{
		BBox:  bbox,
		Left:  buildBVHRecursive(tris[:i]),
		Right: buildBVHRecursive(tris[i:]),
	}
}

func buildBVHAABBRecursive(aabbs []AABBPrimitive) *BVHNode {
	bbox := emptyAABB()
	for i := range aabbs {
		bbox = bbox.union(aabbPrimBBox(&aabbs[i]))
	}

	if len(aabbs) <= maxLeafTriangles {
		leaf := make([]AABBPrimitive, len(aabbs))
		copy(leaf, aabbs)
		return &BVHNode{
			BBox:  bbox,
			AABBs: leaf,
		}
	}

	axis := bbox.longestAxis()
	mid := (bbox.Min[axis] + bbox.Max[axis]) / 2.0

	i := 0
	for j := range aabbs {
		centroid := (aabbs[j].Min[axis] + aabbs[j].Max[axis]) / 2.0
		if centroid < mid {
			aabbs[i], aabbs[j] = aabbs[j], aabbs[i]
			i++
		}
	}

	if i == 0 || i == len(aabbs) {
		i = len(aabbs) / 2
	}

	return &BVHNode{
		BBox:  bbox,
		Left:  buildBVHAABBRecursive(aabbs[:i]),
		Right: buildBVHAABBRecursive(aabbs[i:]),
	}
}

// -------------------------------------------------------------------------
// Ray intersection
// -------------------------------------------------------------------------

// RayTriangleIntersect tests a ray against a triangle using the
// Moller-Trumbore algorithm. Returns whether the ray hits the triangle
// and the parametric distance t along the ray. Only front-face and
// back-face hits with t > 0 are reported.
func RayTriangleIntersect(origin, direction [3]float32, tri *Triangle) (hit bool, t float32) {
	const epsilon = 1e-8

	edge1 := sub3(tri.V1, tri.V0)
	edge2 := sub3(tri.V2, tri.V0)

	h := cross3(direction, edge2)
	det := dot3(edge1, h)

	// Ray is parallel to the triangle plane.
	if det > -epsilon && det < epsilon {
		return false, 0
	}

	invDet := 1.0 / det
	s := sub3(origin, tri.V0)
	u := dot3(s, h) * invDet
	if u < 0.0 || u > 1.0 {
		return false, 0
	}

	q := cross3(s, edge1)
	v := dot3(direction, q) * invDet
	if v < 0.0 || u+v > 1.0 {
		return false, 0
	}

	tHit := dot3(edge2, q) * invDet
	if tHit > epsilon {
		return true, tHit
	}
	return false, 0
}

// RayAABBIntersect tests a ray against an axis-aligned bounding box using
// the slab method. Returns whether the ray intersects the box and the
// parametric entry/exit distances (tMin, tMax).
func RayAABBIntersect(origin, direction [3]float32, box *AABB) (hit bool, tMin, tMax float32) {
	tMin = 0
	tMax = float32(math.MaxFloat32)

	for axis := 0; axis < 3; axis++ {
		invD := 1.0 / direction[axis]
		t0 := (box.Min[axis] - origin[axis]) * invD
		t1 := (box.Max[axis] - origin[axis]) * invD

		if invD < 0 {
			t0, t1 = t1, t0
		}

		if t0 > tMin {
			tMin = t0
		}
		if t1 < tMax {
			tMax = t1
		}

		if tMax < tMin {
			return false, 0, 0
		}
	}

	return true, tMin, tMax
}

// TraverseBVH finds the closest ray intersection in the BVH. Returns the
// hit status, parametric distance, geometry index, and primitive index.
// tMaxIn limits the search distance (use math.MaxFloat32 for unbounded).
func TraverseBVH(root *BVHNode, origin, direction [3]float32, tMaxIn float32) (hit bool, t float32, geomIdx, primIdx uint32) {
	if root == nil {
		return false, 0, 0, 0
	}
	return traverseRecursive(root, origin, direction, tMaxIn)
}

func traverseRecursive(node *BVHNode, origin, direction [3]float32, tMax float32) (hit bool, t float32, geomIdx, primIdx uint32) {
	// Test ray against node bounding box.
	boxHit, boxTMin, _ := RayAABBIntersect(origin, direction, &node.BBox)
	if !boxHit || boxTMin > tMax {
		return false, 0, 0, 0
	}

	// Leaf node: test all primitives.
	if node.Left == nil && node.Right == nil {
		return intersectLeaf(node, origin, direction, tMax)
	}

	// Internal node: recurse into both children, closest first.
	lHit, lT, lGeom, lPrim := traverseRecursive(node.Left, origin, direction, tMax)
	if lHit {
		tMax = lT
	}
	rHit, rT, rGeom, rPrim := traverseRecursive(node.Right, origin, direction, tMax)

	if rHit {
		return true, rT, rGeom, rPrim
	}
	if lHit {
		return true, lT, lGeom, lPrim
	}
	return false, 0, 0, 0
}

func intersectLeaf(node *BVHNode, origin, direction [3]float32, tMax float32) (hit bool, t float32, geomIdx, primIdx uint32) {
	closestT := tMax
	found := false

	for i := range node.Triangles {
		triHit, triT := RayTriangleIntersect(origin, direction, &node.Triangles[i])
		if triHit && triT < closestT {
			closestT = triT
			geomIdx = node.Triangles[i].GeometryIndex
			primIdx = node.Triangles[i].PrimitiveIndex
			found = true
		}
	}

	for i := range node.AABBs {
		aabb := &node.AABBs[i]
		box := AABB{Min: aabb.Min, Max: aabb.Max}
		aabbHit, aabbTMin, _ := RayAABBIntersect(origin, direction, &box)
		if aabbHit && aabbTMin < closestT {
			closestT = aabbTMin
			geomIdx = aabb.GeometryIndex
			primIdx = aabb.PrimitiveIndex
			found = true
		}
	}

	return found, closestT, geomIdx, primIdx
}

// -------------------------------------------------------------------------
// TlasInstance packing (64-byte Vulkan/DX12 compatible format)
// -------------------------------------------------------------------------

// packTLASInstance packs a TlasInstance into the 64-byte format used by
// Vulkan (VkAccelerationStructureInstanceKHR) and DX12
// (D3D12_RAYTRACING_INSTANCE_DESC). This maintains consistency across
// all backends for the same test data.
//
// Layout:
//
//	Bytes  0-47: 3x4 row-major transform (12 x float32)
//	Bytes 48-51: instanceCustomIndex (24 bits) | mask (8 bits)
//	Bytes 52-55: SBT offset (24 bits) | flags (8 bits)
//	Bytes 56-63: accelerationStructureReference (uint64)
func packTLASInstance(instance hal.TlasInstance) []byte {
	const maxU24 = (1 << 24) - 1

	buf := make([]byte, 64)

	// Transform: 12 x float32 = 48 bytes.
	for i, v := range instance.Transform {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}

	// instanceCustomIndex (bits 0-23) | mask (bits 24-31).
	customAndMask := (instance.CustomData & maxU24) | (uint32(instance.Mask) << 24)
	binary.LittleEndian.PutUint32(buf[48:], customAndMask)

	// SBT offset (bits 0-23) | flags (bits 24-31, always 0).
	sbtAndFlags := instance.ShaderBindingTableRecordOffset & maxU24
	binary.LittleEndian.PutUint32(buf[52:], sbtAndFlags)

	// accelerationStructureReference.
	binary.LittleEndian.PutUint64(buf[56:], instance.BlasAddress)

	return buf
}

// -------------------------------------------------------------------------
// BVH deep copy (for CopyAccelerationStructure)
// -------------------------------------------------------------------------

// deepCopyBVH returns a deep copy of a BVH tree.
func deepCopyBVH(node *BVHNode) *BVHNode {
	if node == nil {
		return nil
	}

	n := &BVHNode{BBox: node.BBox}

	if len(node.Triangles) > 0 {
		n.Triangles = make([]Triangle, len(node.Triangles))
		copy(n.Triangles, node.Triangles)
	}
	if len(node.AABBs) > 0 {
		n.AABBs = make([]AABBPrimitive, len(node.AABBs))
		copy(n.AABBs, node.AABBs)
	}

	n.Left = deepCopyBVH(node.Left)
	n.Right = deepCopyBVH(node.Right)
	return n
}

// -------------------------------------------------------------------------
// Build size estimation
// -------------------------------------------------------------------------

// estimateBuildSize returns a conservative size estimate for an acceleration
// structure based on primitive counts. The actual memory layout is a Go
// struct graph managed by GC, so this is purely informational (matching the
// HAL interface contract that size queries return non-zero values).
func estimateBuildSize(entries *hal.AccelerationStructureEntries) hal.AccelerationStructureBuildSizes {
	if entries == nil {
		return hal.AccelerationStructureBuildSizes{}
	}

	var primCount uint64
	switch {
	case entries.Instances != nil:
		primCount = uint64(entries.Instances.Count)
	case len(entries.Triangles) > 0:
		for _, t := range entries.Triangles {
			if t.Indices != nil {
				primCount += uint64(t.Indices.Count / 3)
			} else {
				primCount += uint64(t.VertexCount / 3)
			}
		}
	case len(entries.AABBs) > 0:
		for _, a := range entries.AABBs {
			primCount += uint64(a.Count)
		}
	}

	// Each triangle is ~48 bytes, each BVH node ~80 bytes. Roughly 2N-1
	// nodes for N leaf primitives with maxLeafTriangles per leaf.
	const bytesPerTriangle = 48
	const bytesPerNode = 80
	nodeCount := primCount*2/maxLeafTriangles + 1
	asSize := primCount*bytesPerTriangle + nodeCount*bytesPerNode

	// Scratch is not needed for CPU builds, but return a token non-zero
	// value so callers that allocate scratch buffers based on this don't
	// get a zero-size allocation.
	const scratchSize = 256

	return hal.AccelerationStructureBuildSizes{
		AccelerationStructureSize: asSize,
		UpdateScratchSize:         scratchSize,
		BuildScratchSize:          scratchSize,
	}
}

// -------------------------------------------------------------------------
// Vertex buffer extraction helpers
// -------------------------------------------------------------------------

// extractTrianglesFromBuffer reads triangle vertices from a software buffer.
// It supports float32x3 vertex data (the most common format for RT geometry).
// stride is in bytes; firstVertex is the starting vertex index.
func extractTrianglesFromBuffer(
	buf *Buffer,
	firstVertex, vertexCount uint32,
	stride uint64,
	geomIndex uint32,
) []Triangle {
	if buf == nil || len(buf.data) == 0 || vertexCount < 3 {
		return nil
	}

	buf.mu.RLock()
	defer buf.mu.RUnlock()

	// If stride is 0, assume tightly packed float32x3 (12 bytes per vertex).
	if stride == 0 {
		stride = 12
	}

	triCount := vertexCount / 3
	triangles := make([]Triangle, 0, triCount)

	for i := uint32(0); i < triCount; i++ {
		vi0 := firstVertex + i*3
		vi1 := firstVertex + i*3 + 1
		vi2 := firstVertex + i*3 + 2

		v0 := readFloat3(buf.data, uint64(vi0)*stride)
		v1 := readFloat3(buf.data, uint64(vi1)*stride)
		v2 := readFloat3(buf.data, uint64(vi2)*stride)

		if v0 == nil || v1 == nil || v2 == nil {
			continue
		}

		triangles = append(triangles, Triangle{
			V0:             *v0,
			V1:             *v1,
			V2:             *v2,
			GeometryIndex:  geomIndex,
			PrimitiveIndex: i,
		})
	}
	return triangles
}

// readFloat3 reads 3 consecutive float32 values from data starting at the
// given byte offset. Returns nil if the read would be out of bounds.
func readFloat3(data []byte, offset uint64) *[3]float32 {
	end := offset + 12
	if end > uint64(len(data)) {
		return nil
	}
	return &[3]float32{
		math.Float32frombits(binary.LittleEndian.Uint32(data[offset:])),
		math.Float32frombits(binary.LittleEndian.Uint32(data[offset+4:])),
		math.Float32frombits(binary.LittleEndian.Uint32(data[offset+8:])),
	}
}

// -------------------------------------------------------------------------
// float32 vector math (minimal, inlined by compiler)
// -------------------------------------------------------------------------

func sub3(a, b [3]float32) [3]float32 {
	return [3]float32{a[0] - b[0], a[1] - b[1], a[2] - b[2]}
}

func cross3(a, b [3]float32) [3]float32 {
	return [3]float32{
		a[1]*b[2] - a[2]*b[1],
		a[2]*b[0] - a[0]*b[2],
		a[0]*b[1] - a[1]*b[0],
	}
}

func dot3(a, b [3]float32) float32 {
	return a[0]*b[0] + a[1]*b[1] + a[2]*b[2]
}

func minF(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

func maxF(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}
