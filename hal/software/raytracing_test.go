//go:build !(js && wasm)

package software

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// -------------------------------------------------------------------------
// BVH construction tests
// -------------------------------------------------------------------------

func TestBuildBVH_Empty(t *testing.T) {
	root := BuildBVH(nil)
	if root != nil {
		t.Fatal("expected nil BVH for empty input")
	}
}

func TestBuildBVH_SingleTriangle(t *testing.T) {
	tris := []Triangle{
		{
			V0: [3]float32{0, 0, 0},
			V1: [3]float32{1, 0, 0},
			V2: [3]float32{0, 1, 0},
		},
	}

	root := BuildBVH(tris)
	if root == nil {
		t.Fatal("expected non-nil BVH")
	}

	if root.NodeCount() != 1 {
		t.Errorf("single triangle BVH should have 1 node, got %d", root.NodeCount())
	}

	if len(root.Triangles) != 1 {
		t.Errorf("leaf should have 1 triangle, got %d", len(root.Triangles))
	}

	// Leaf node should have nil children.
	if root.Left != nil || root.Right != nil {
		t.Error("leaf node should have nil children")
	}
}

func TestBuildBVH_MultipleTriangles(t *testing.T) {
	// Create 8 triangles spread along the X axis to force BVH splitting.
	tris := make([]Triangle, 8)
	for i := range tris {
		x := float32(i) * 10.0
		tris[i] = Triangle{
			V0:             [3]float32{x, 0, 0},
			V1:             [3]float32{x + 1, 0, 0},
			V2:             [3]float32{x, 1, 0},
			GeometryIndex:  0,
			PrimitiveIndex: uint32(i),
		}
	}

	root := BuildBVH(tris)
	if root == nil {
		t.Fatal("expected non-nil BVH")
	}

	nodeCount := root.NodeCount()
	if nodeCount < 3 {
		t.Errorf("8 triangles should produce at least 3 BVH nodes, got %d", nodeCount)
	}

	// Root bounding box should enclose all triangles.
	if root.BBox.Min[0] > 0 {
		t.Errorf("root BBox min X should be <= 0, got %f", root.BBox.Min[0])
	}
	if root.BBox.Max[0] < 70 {
		t.Errorf("root BBox max X should be >= 70, got %f", root.BBox.Max[0])
	}

	t.Logf("BVH: %d nodes from %d triangles", nodeCount, len(tris))
}

func TestBuildBVH_MaxLeafSize(t *testing.T) {
	// Exactly maxLeafTriangles triangles should produce a single leaf.
	tris := make([]Triangle, maxLeafTriangles)
	for i := range tris {
		tris[i] = Triangle{
			V0: [3]float32{float32(i), 0, 0},
			V1: [3]float32{float32(i) + 1, 0, 0},
			V2: [3]float32{float32(i), 1, 0},
		}
	}

	root := BuildBVH(tris)
	if root == nil {
		t.Fatal("expected non-nil BVH")
	}
	if root.NodeCount() != 1 {
		t.Errorf("maxLeafTriangles (%d) should produce 1 leaf node, got %d nodes",
			maxLeafTriangles, root.NodeCount())
	}
}

func TestBuildBVHFromAABBs(t *testing.T) {
	aabbs := []AABBPrimitive{
		{Min: [3]float32{0, 0, 0}, Max: [3]float32{1, 1, 1}, GeometryIndex: 0, PrimitiveIndex: 0},
		{Min: [3]float32{2, 0, 0}, Max: [3]float32{3, 1, 1}, GeometryIndex: 0, PrimitiveIndex: 1},
	}

	root := BuildBVHFromAABBs(aabbs)
	if root == nil {
		t.Fatal("expected non-nil BVH")
	}
	if root.NodeCount() < 1 {
		t.Error("expected at least 1 node")
	}
}

// -------------------------------------------------------------------------
// Ray-triangle intersection tests
// -------------------------------------------------------------------------

func TestRayTriangleIntersect_Hit(t *testing.T) {
	tri := &Triangle{
		V0: [3]float32{-1, -1, 0},
		V1: [3]float32{1, -1, 0},
		V2: [3]float32{0, 1, 0},
	}

	origin := [3]float32{0, 0, -5}
	dir := [3]float32{0, 0, 1}

	hit, tVal := RayTriangleIntersect(origin, dir, tri)
	if !hit {
		t.Fatal("expected hit")
	}
	if math.Abs(float64(tVal-5.0)) > 0.001 {
		t.Errorf("expected t=5.0, got %f", tVal)
	}
}

func TestRayTriangleIntersect_Miss(t *testing.T) {
	tri := &Triangle{
		V0: [3]float32{-1, -1, 0},
		V1: [3]float32{1, -1, 0},
		V2: [3]float32{0, 1, 0},
	}

	// Ray pointing away from the triangle.
	origin := [3]float32{0, 0, -5}
	dir := [3]float32{0, 0, -1}

	hit, _ := RayTriangleIntersect(origin, dir, tri)
	if hit {
		t.Error("expected miss (ray pointing away)")
	}
}

func TestRayTriangleIntersect_MissParallel(t *testing.T) {
	tri := &Triangle{
		V0: [3]float32{0, 0, 0},
		V1: [3]float32{1, 0, 0},
		V2: [3]float32{0, 1, 0},
	}

	// Ray parallel to the triangle plane (in XY plane, direction along X).
	origin := [3]float32{-1, 0.5, 0}
	dir := [3]float32{1, 0, 0}

	hit, _ := RayTriangleIntersect(origin, dir, tri)
	if hit {
		t.Error("expected miss (ray parallel to triangle)")
	}
}

func TestRayTriangleIntersect_MissOutside(t *testing.T) {
	tri := &Triangle{
		V0: [3]float32{0, 0, 0},
		V1: [3]float32{1, 0, 0},
		V2: [3]float32{0, 1, 0},
	}

	// Ray aimed at a point outside the triangle (u+v > 1).
	origin := [3]float32{2, 2, -1}
	dir := [3]float32{0, 0, 1}

	hit, _ := RayTriangleIntersect(origin, dir, tri)
	if hit {
		t.Error("expected miss (ray hits plane outside triangle)")
	}
}

func TestRayTriangleIntersect_BackFace(t *testing.T) {
	tri := &Triangle{
		V0: [3]float32{-1, -1, 0},
		V1: [3]float32{1, -1, 0},
		V2: [3]float32{0, 1, 0},
	}

	// Ray from the back side of the triangle.
	origin := [3]float32{0, 0, 5}
	dir := [3]float32{0, 0, -1}

	hit, tVal := RayTriangleIntersect(origin, dir, tri)
	if !hit {
		t.Fatal("expected back-face hit (Moller-Trumbore is double-sided)")
	}
	if math.Abs(float64(tVal-5.0)) > 0.001 {
		t.Errorf("expected t=5.0, got %f", tVal)
	}
}

// -------------------------------------------------------------------------
// Ray-AABB intersection tests
// -------------------------------------------------------------------------

func TestRayAABBIntersect_Hit(t *testing.T) {
	box := &AABB{
		Min: [3]float32{-1, -1, -1},
		Max: [3]float32{1, 1, 1},
	}

	origin := [3]float32{0, 0, -5}
	dir := [3]float32{0, 0, 1}

	hit, tMin, tMax := RayAABBIntersect(origin, dir, box)
	if !hit {
		t.Fatal("expected hit")
	}
	if math.Abs(float64(tMin-4.0)) > 0.001 {
		t.Errorf("expected tMin=4.0, got %f", tMin)
	}
	if math.Abs(float64(tMax-6.0)) > 0.001 {
		t.Errorf("expected tMax=6.0, got %f", tMax)
	}
}

func TestRayAABBIntersect_Miss(t *testing.T) {
	box := &AABB{
		Min: [3]float32{-1, -1, -1},
		Max: [3]float32{1, 1, 1},
	}

	// Ray aimed past the box.
	origin := [3]float32{5, 5, -5}
	dir := [3]float32{0, 0, 1}

	hit, _, _ := RayAABBIntersect(origin, dir, box)
	if hit {
		t.Error("expected miss (ray passes the box)")
	}
}

func TestRayAABBIntersect_InsideBox(t *testing.T) {
	box := &AABB{
		Min: [3]float32{-1, -1, -1},
		Max: [3]float32{1, 1, 1},
	}

	// Origin inside the box.
	origin := [3]float32{0, 0, 0}
	dir := [3]float32{0, 0, 1}

	hit, tMin, tMax := RayAABBIntersect(origin, dir, box)
	if !hit {
		t.Fatal("expected hit (origin inside box)")
	}
	if tMin > 0.001 {
		t.Errorf("origin inside box should have tMin near 0, got %f", tMin)
	}
	if tMax < 0.999 {
		t.Errorf("expected tMax=1.0, got %f", tMax)
	}
}

// -------------------------------------------------------------------------
// BVH traversal tests
// -------------------------------------------------------------------------

func TestTraverseBVH_Hit(t *testing.T) {
	tris := []Triangle{
		{
			V0:             [3]float32{-1, -1, 5},
			V1:             [3]float32{1, -1, 5},
			V2:             [3]float32{0, 1, 5},
			GeometryIndex:  0,
			PrimitiveIndex: 0,
		},
	}

	root := BuildBVH(tris)

	origin := [3]float32{0, 0, 0}
	dir := [3]float32{0, 0, 1}

	hit, tVal, geomIdx, primIdx := TraverseBVH(root, origin, dir, float32(math.MaxFloat32))
	if !hit {
		t.Fatal("expected BVH traversal hit")
	}
	if math.Abs(float64(tVal-5.0)) > 0.001 {
		t.Errorf("expected t=5.0, got %f", tVal)
	}
	if geomIdx != 0 {
		t.Errorf("expected geomIdx=0, got %d", geomIdx)
	}
	if primIdx != 0 {
		t.Errorf("expected primIdx=0, got %d", primIdx)
	}
}

func TestTraverseBVH_ClosestHit(t *testing.T) {
	// Two triangles at different distances; traversal should return the closer one.
	tris := []Triangle{
		{
			V0:             [3]float32{-1, -1, 10},
			V1:             [3]float32{1, -1, 10},
			V2:             [3]float32{0, 1, 10},
			GeometryIndex:  0,
			PrimitiveIndex: 0,
		},
		{
			V0:             [3]float32{-1, -1, 3},
			V1:             [3]float32{1, -1, 3},
			V2:             [3]float32{0, 1, 3},
			GeometryIndex:  1,
			PrimitiveIndex: 1,
		},
	}

	root := BuildBVH(tris)

	origin := [3]float32{0, 0, 0}
	dir := [3]float32{0, 0, 1}

	hit, tVal, geomIdx, primIdx := TraverseBVH(root, origin, dir, float32(math.MaxFloat32))
	if !hit {
		t.Fatal("expected BVH traversal hit")
	}
	if math.Abs(float64(tVal-3.0)) > 0.001 {
		t.Errorf("expected t=3.0 (closer triangle), got %f", tVal)
	}
	if geomIdx != 1 {
		t.Errorf("expected geomIdx=1 (closer triangle), got %d", geomIdx)
	}
	if primIdx != 1 {
		t.Errorf("expected primIdx=1, got %d", primIdx)
	}
}

func TestTraverseBVH_Miss(t *testing.T) {
	tris := []Triangle{
		{
			V0: [3]float32{-1, -1, 5},
			V1: [3]float32{1, -1, 5},
			V2: [3]float32{0, 1, 5},
		},
	}

	root := BuildBVH(tris)

	// Ray pointing away.
	origin := [3]float32{0, 0, 0}
	dir := [3]float32{0, 0, -1}

	if traverseDidHit(root, origin, dir, float32(math.MaxFloat32)) {
		t.Error("expected BVH traversal miss")
	}
}

func TestTraverseBVH_NilRoot(t *testing.T) {
	if traverseDidHit(nil, [3]float32{0, 0, 0}, [3]float32{0, 0, 1}, float32(math.MaxFloat32)) {
		t.Error("nil root should not hit")
	}
}

func TestTraverseBVH_TMaxLimits(t *testing.T) {
	tris := []Triangle{
		{
			V0:             [3]float32{-1, -1, 5},
			V1:             [3]float32{1, -1, 5},
			V2:             [3]float32{0, 1, 5},
			GeometryIndex:  0,
			PrimitiveIndex: 0,
		},
	}
	root := BuildBVH(tris)

	origin := [3]float32{0, 0, 0}
	dir := [3]float32{0, 0, 1}

	// tMax = 2.0 is less than the intersection distance (5.0), so should miss.
	if traverseDidHit(root, origin, dir, 2.0) {
		t.Error("expected miss when tMax < intersection distance")
	}
}

// -------------------------------------------------------------------------
// TlasInstanceToBytes test
// -------------------------------------------------------------------------

func TestTlasInstanceToBytes(t *testing.T) {
	instance := hal.TlasInstance{
		Transform: [12]float32{
			1, 0, 0, 10,
			0, 1, 0, 20,
			0, 0, 1, 30,
		},
		CustomData:                     42,
		Mask:                           0xFF,
		BlasAddress:                    0xDEADBEEF12345678,
		ShaderBindingTableRecordOffset: 7,
	}

	buf := packTLASInstance(instance)
	if len(buf) != 64 {
		t.Fatalf("expected 64 bytes, got %d", len(buf))
	}

	// Verify transform (first 48 bytes: 12 floats).
	for i, expected := range instance.Transform {
		got := math.Float32frombits(binary.LittleEndian.Uint32(buf[i*4:]))
		if got != expected {
			t.Errorf("transform[%d]: expected %f, got %f", i, expected, got)
		}
	}

	// Verify customData (bits 0-23) and mask (bits 24-31) at bytes 48-51.
	customAndMask := binary.LittleEndian.Uint32(buf[48:])
	gotCustom := customAndMask & 0x00FFFFFF
	gotMask := (customAndMask >> 24) & 0xFF
	if gotCustom != 42 {
		t.Errorf("customData: expected 42, got %d", gotCustom)
	}
	if gotMask != 0xFF {
		t.Errorf("mask: expected 0xFF, got 0x%02X", gotMask)
	}

	// Verify SBT offset at bytes 52-55.
	sbtAndFlags := binary.LittleEndian.Uint32(buf[52:])
	gotSBT := sbtAndFlags & 0x00FFFFFF
	if gotSBT != 7 {
		t.Errorf("SBT offset: expected 7, got %d", gotSBT)
	}

	// Verify BLAS address at bytes 56-63.
	gotAddr := binary.LittleEndian.Uint64(buf[56:])
	if gotAddr != 0xDEADBEEF12345678 {
		t.Errorf("BLAS address: expected 0xDEADBEEF12345678, got 0x%X", gotAddr)
	}
}

// -------------------------------------------------------------------------
// Device method integration tests
// -------------------------------------------------------------------------

func TestDevice_CreateAccelerationStructure(t *testing.T) {
	d := &Device{}

	as, err := d.CreateAccelerationStructure(&hal.AccelerationStructureDescriptor{
		Label:  "test BLAS",
		Size:   1024,
		Format: hal.AccelerationStructureFormatBottomLevel,
	})
	if err != nil {
		t.Fatalf("CreateAccelerationStructure failed: %v", err)
	}
	if as == nil {
		t.Fatal("expected non-nil acceleration structure")
	}

	swAS := as.(*AccelerationStructure)
	if swAS.format != hal.AccelerationStructureFormatBottomLevel {
		t.Error("format mismatch")
	}
	if swAS.size != 1024 {
		t.Errorf("size: expected 1024, got %d", swAS.size)
	}
	if swAS.NativeHandle() == 0 {
		t.Error("NativeHandle should be non-zero")
	}
}

func TestDevice_CreateAccelerationStructure_NilDesc(t *testing.T) {
	d := &Device{}
	_, err := d.CreateAccelerationStructure(nil)
	if err == nil {
		t.Error("expected error for nil descriptor")
	}
}

func TestDevice_GetAccelerationStructureBuildSizes(t *testing.T) {
	d := &Device{}
	sizes := d.GetAccelerationStructureBuildSizes(&hal.GetAccelerationStructureBuildSizesDescriptor{
		Entries: &hal.AccelerationStructureEntries{
			Triangles: []hal.AccelerationStructureTriangles{
				{VertexCount: 300},
			},
		},
	})

	if sizes.AccelerationStructureSize == 0 {
		t.Error("AS size should be non-zero")
	}
	if sizes.BuildScratchSize == 0 {
		t.Error("build scratch size should be non-zero")
	}
}

func TestDevice_GetAccelerationStructureDeviceAddress(t *testing.T) {
	d := &Device{}

	as, err := d.CreateAccelerationStructure(&hal.AccelerationStructureDescriptor{
		Size:   512,
		Format: hal.AccelerationStructureFormatBottomLevel,
	})
	if err != nil {
		t.Fatal(err)
	}

	addr := d.GetAccelerationStructureDeviceAddress(as)
	if addr == 0 {
		t.Error("device address should be non-zero")
	}
}

func TestDevice_DestroyAccelerationStructure(t *testing.T) {
	d := &Device{}

	as, err := d.CreateAccelerationStructure(&hal.AccelerationStructureDescriptor{
		Size:   512,
		Format: hal.AccelerationStructureFormatBottomLevel,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Should not panic.
	d.DestroyAccelerationStructure(as)

	swAS := as.(*AccelerationStructure)
	if swAS.bvh != nil {
		t.Error("BVH should be nil after destroy")
	}
}

func TestDevice_TlasInstanceToBytes(t *testing.T) {
	d := &Device{}
	buf := d.TlasInstanceToBytes(hal.TlasInstance{
		Transform: [12]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0},
		Mask:      0xFF,
	})
	if len(buf) != 64 {
		t.Fatalf("expected 64 bytes, got %d", len(buf))
	}
}

// -------------------------------------------------------------------------
// CommandEncoder integration tests
// -------------------------------------------------------------------------

func TestCommandEncoder_BuildAccelerationStructures_BLAS(t *testing.T) {
	d := &Device{}
	enc := &CommandEncoder{device: d}

	// Create a vertex buffer with 1 triangle (3 vertices x 3 floats x 4 bytes = 36 bytes).
	vertexData := make([]byte, 36)
	putFloat3(vertexData, 0, 0, 0, 0)  // V0
	putFloat3(vertexData, 12, 1, 0, 0) // V1
	putFloat3(vertexData, 24, 0, 1, 0) // V2

	buf := &Buffer{
		id:   nextResourceID.Add(1),
		data: vertexData,
		size: 36,
	}

	as, err := d.CreateAccelerationStructure(&hal.AccelerationStructureDescriptor{
		Size:   4096,
		Format: hal.AccelerationStructureFormatBottomLevel,
	})
	if err != nil {
		t.Fatal(err)
	}

	enc.BuildAccelerationStructures([]hal.BuildAccelerationStructureDescriptor{
		{
			Entries: &hal.AccelerationStructureEntries{
				Triangles: []hal.AccelerationStructureTriangles{
					{
						VertexBuffer: buf,
						VertexCount:  3,
						VertexStride: 12,
					},
				},
			},
			DestinationAccelerationStructure: as,
		},
	})

	swAS := as.(*AccelerationStructure)
	if swAS.bvh == nil {
		t.Fatal("BVH should be built after BuildAccelerationStructures")
	}
	if swAS.bvh.NodeCount() != 1 {
		t.Errorf("expected 1 BVH node for 1 triangle, got %d", swAS.bvh.NodeCount())
	}
	if len(swAS.bvh.Triangles) != 1 {
		t.Errorf("expected 1 triangle in leaf, got %d", len(swAS.bvh.Triangles))
	}
}

func TestCommandEncoder_CopyAccelerationStructure(t *testing.T) {
	d := &Device{}
	enc := &CommandEncoder{device: d}

	// Build a source AS with a triangle.
	srcAS := &AccelerationStructure{
		id:     nextResourceID.Add(1),
		format: hal.AccelerationStructureFormatBottomLevel,
		size:   1024,
	}
	srcAS.bvh = BuildBVH([]Triangle{
		{V0: [3]float32{0, 0, 0}, V1: [3]float32{1, 0, 0}, V2: [3]float32{0, 1, 0}},
	})

	dstAS := &AccelerationStructure{
		id:   nextResourceID.Add(1),
		size: 1024,
	}

	enc.CopyAccelerationStructure(srcAS, dstAS, gputypes.AccelerationStructureCopyModeClone)

	if dstAS.bvh == nil {
		t.Fatal("destination BVH should not be nil after copy")
	}
	if dstAS.bvh == srcAS.bvh {
		t.Error("copy should produce a different BVH pointer (deep copy)")
	}
	if dstAS.bvh.NodeCount() != srcAS.bvh.NodeCount() {
		t.Error("copied BVH should have same node count")
	}
}

func TestCommandEncoder_ReadAccelerationStructureCompactSize(t *testing.T) {
	d := &Device{}
	enc := &CommandEncoder{device: d}

	as := &AccelerationStructure{
		id:   nextResourceID.Add(1),
		size: 4096,
	}

	buf := &Buffer{
		id:   nextResourceID.Add(1),
		data: make([]byte, 64),
		size: 64,
	}

	enc.ReadAccelerationStructureCompactSize(as, buf, 0)

	gotSize := binary.LittleEndian.Uint64(buf.data[0:])
	if gotSize != 4096 {
		t.Errorf("expected compact size 4096, got %d", gotSize)
	}
}

func TestCommandEncoder_ReadAccelerationStructureCompactSize_WithOffset(t *testing.T) {
	d := &Device{}
	enc := &CommandEncoder{device: d}

	as := &AccelerationStructure{
		id:   nextResourceID.Add(1),
		size: 2048,
	}

	buf := &Buffer{
		id:   nextResourceID.Add(1),
		data: make([]byte, 64),
		size: 64,
	}

	enc.ReadAccelerationStructureCompactSize(as, buf, 16)

	gotSize := binary.LittleEndian.Uint64(buf.data[16:])
	if gotSize != 2048 {
		t.Errorf("expected compact size 2048 at offset 16, got %d", gotSize)
	}
}

func TestCommandEncoder_PlaceAccelerationStructureBarrier(t *testing.T) {
	enc := &CommandEncoder{}
	// Should not panic (no-op).
	enc.PlaceAccelerationStructureBarrier(hal.AccelerationStructureBarrier{})
}

// -------------------------------------------------------------------------
// Deep copy verification
// -------------------------------------------------------------------------

func TestDeepCopyBVH(t *testing.T) {
	tris := []Triangle{
		{V0: [3]float32{0, 0, 0}, V1: [3]float32{1, 0, 0}, V2: [3]float32{0, 1, 0}, GeometryIndex: 0, PrimitiveIndex: 0},
		{V0: [3]float32{2, 0, 0}, V1: [3]float32{3, 0, 0}, V2: [3]float32{2, 1, 0}, GeometryIndex: 1, PrimitiveIndex: 1},
		{V0: [3]float32{4, 0, 0}, V1: [3]float32{5, 0, 0}, V2: [3]float32{4, 1, 0}, GeometryIndex: 2, PrimitiveIndex: 2},
		{V0: [3]float32{6, 0, 0}, V1: [3]float32{7, 0, 0}, V2: [3]float32{6, 1, 0}, GeometryIndex: 3, PrimitiveIndex: 3},
		{V0: [3]float32{8, 0, 0}, V1: [3]float32{9, 0, 0}, V2: [3]float32{8, 1, 0}, GeometryIndex: 4, PrimitiveIndex: 4},
	}

	original := BuildBVH(tris)
	copied := deepCopyBVH(original)

	if copied == original {
		t.Error("deep copy should produce different root pointer")
	}
	if copied.NodeCount() != original.NodeCount() {
		t.Errorf("node count mismatch: original=%d, copy=%d", original.NodeCount(), copied.NodeCount())
	}

	// Modify the copy and verify original is unaffected.
	if copied.Left != nil && len(copied.Left.Triangles) > 0 {
		copied.Left.Triangles[0].V0[0] = 999
	}
	// Original should still have the original value (not 999).
	if original.Left != nil && len(original.Left.Triangles) > 0 {
		if original.Left.Triangles[0].V0[0] == 999 {
			t.Error("modifying copy affected original — deep copy is not deep enough")
		}
	}
}

func TestDeepCopyBVH_Nil(t *testing.T) {
	if deepCopyBVH(nil) != nil {
		t.Error("deep copy of nil should return nil")
	}
}

// -------------------------------------------------------------------------
// Build size estimation test
// -------------------------------------------------------------------------

func TestEstimateBuildSize_Triangles(t *testing.T) {
	sizes := estimateBuildSize(&hal.AccelerationStructureEntries{
		Triangles: []hal.AccelerationStructureTriangles{
			{VertexCount: 300},
			{VertexCount: 600},
		},
	})

	if sizes.AccelerationStructureSize == 0 {
		t.Error("AS size should be non-zero for triangle input")
	}
	if sizes.BuildScratchSize == 0 {
		t.Error("scratch size should be non-zero")
	}
}

func TestEstimateBuildSize_Instances(t *testing.T) {
	sizes := estimateBuildSize(&hal.AccelerationStructureEntries{
		Instances: &hal.AccelerationStructureInstances{Count: 10},
	})
	if sizes.AccelerationStructureSize == 0 {
		t.Error("AS size should be non-zero for instance input")
	}
}

func TestEstimateBuildSize_Nil(t *testing.T) {
	sizes := estimateBuildSize(nil)
	if sizes.AccelerationStructureSize != 0 {
		t.Error("nil entries should produce zero size")
	}
}

// -------------------------------------------------------------------------
// Helpers
// -------------------------------------------------------------------------

// putFloat3 writes 3 float32 values to buf starting at offset.
func putFloat3(buf []byte, offset int, x, y, z float32) {
	binary.LittleEndian.PutUint32(buf[offset:], math.Float32bits(x))
	binary.LittleEndian.PutUint32(buf[offset+4:], math.Float32bits(y))
	binary.LittleEndian.PutUint32(buf[offset+8:], math.Float32bits(z))
}

// traverseDidHit wraps TraverseBVH returning only the hit boolean.
// Avoids 3+ blank identifiers that trigger the dogsled linter.
func traverseDidHit(root *BVHNode, origin, direction [3]float32, tMax float32) bool {
	hit, _, _, _ := TraverseBVH(root, origin, direction, tMax) //nolint:dogsled // test helper
	return hit
}
