//go:build !(js && wasm)

package software

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// ---------------------------------------------------------------------------
// End-to-end: triangle BLAS build + ray hit test
// ---------------------------------------------------------------------------

func TestIntegration_TriangleBLAS_RayHit(t *testing.T) {
	d := &Device{}

	// Create vertex buffer: 1 triangle in the XY plane at Z=5.
	//   V0(-1,-1,5), V1(1,-1,5), V2(0,1,5)
	const vertexBytes = 3 * 3 * 4 // 3 verts x 3 floats x 4 bytes = 36
	vertexData := make([]byte, vertexBytes)
	putFloat3(vertexData, 0, -1, -1, 5)
	putFloat3(vertexData, 12, 1, -1, 5)
	putFloat3(vertexData, 24, 0, 1, 5)

	vb := &Buffer{
		id:   nextResourceID.Add(1),
		data: vertexData,
		size: vertexBytes,
	}

	// Create BLAS.
	blasAS, err := d.CreateAccelerationStructure(&hal.AccelerationStructureDescriptor{
		Label:  "integration-blas",
		Size:   4096,
		Format: hal.AccelerationStructureFormatBottomLevel,
	})
	if err != nil {
		t.Fatalf("CreateAccelerationStructure (BLAS): %v", err)
	}

	// Build BLAS via command encoder.
	enc := &CommandEncoder{device: d}
	enc.BuildAccelerationStructures([]hal.BuildAccelerationStructureDescriptor{{
		Entries: &hal.AccelerationStructureEntries{
			Triangles: []hal.AccelerationStructureTriangles{{
				VertexBuffer: vb,
				VertexCount:  3,
				VertexStride: 12,
			}},
		},
		DestinationAccelerationStructure: blasAS,
	}})

	blas := blasAS.(*AccelerationStructure)
	if blas.bvh == nil {
		t.Fatal("BVH should be built after BuildAccelerationStructures")
	}

	// Shoot ray from origin toward +Z -- should hit the triangle at Z=5.
	origin := [3]float32{0, 0, 0}
	dir := [3]float32{0, 0, 1}

	hit, tVal, geomIdx, primIdx := TraverseBVH(blas.bvh, origin, dir, float32(math.MaxFloat32))
	if !hit {
		t.Fatal("expected ray to hit the triangle")
	}
	if math.Abs(float64(tVal-5.0)) > 0.01 {
		t.Errorf("t = %f, want ~5.0", tVal)
	}
	if geomIdx != 0 {
		t.Errorf("geomIdx = %d, want 0", geomIdx)
	}
	if primIdx != 0 {
		t.Errorf("primIdx = %d, want 0", primIdx)
	}
}

// ---------------------------------------------------------------------------
// End-to-end: TLAS build with instance referencing BLAS + ray hit
// ---------------------------------------------------------------------------

func TestIntegration_TLAS_WithBLAS_RayHit(t *testing.T) {
	d := &Device{}

	// Build BLAS with a triangle at Z=5.
	vertexData := make([]byte, 36)
	putFloat3(vertexData, 0, -1, -1, 5)
	putFloat3(vertexData, 12, 1, -1, 5)
	putFloat3(vertexData, 24, 0, 1, 5)

	vb := &Buffer{
		id:   nextResourceID.Add(1),
		data: vertexData,
		size: 36,
	}

	blasAS, err := d.CreateAccelerationStructure(&hal.AccelerationStructureDescriptor{
		Label:  "tlas-integration-blas",
		Size:   4096,
		Format: hal.AccelerationStructureFormatBottomLevel,
	})
	if err != nil {
		t.Fatalf("CreateAS BLAS: %v", err)
	}

	enc := &CommandEncoder{device: d}
	enc.BuildAccelerationStructures([]hal.BuildAccelerationStructureDescriptor{{
		Entries: &hal.AccelerationStructureEntries{
			Triangles: []hal.AccelerationStructureTriangles{{
				VertexBuffer: vb,
				VertexCount:  3,
				VertexStride: 12,
			}},
		},
		DestinationAccelerationStructure: blasAS,
	}})

	blas := blasAS.(*AccelerationStructure)
	if blas.bvh == nil {
		t.Fatal("BLAS BVH must be built")
	}

	// Create TLAS with one instance that translates the BLAS by +10 on Z.
	tlasAS, err := d.CreateAccelerationStructure(&hal.AccelerationStructureDescriptor{
		Label:  "tlas-integration-tlas",
		Size:   8192,
		Format: hal.AccelerationStructureFormatTopLevel,
	})
	if err != nil {
		t.Fatalf("CreateAS TLAS: %v", err)
	}

	tlas := tlasAS.(*AccelerationStructure)

	// Manually set TLAS instances (simulating what BuildAccelerationStructures
	// would do for TLAS with instance buffer parsing).
	tlas.instances = []TLASInstanceData{{
		Transform: [12]float32{
			1, 0, 0, 0, // Row 0: identity X + translate X=0
			0, 1, 0, 0, // Row 1: identity Y + translate Y=0
			0, 0, 1, 10, // Row 2: identity Z + translate Z=10
		},
		Mask:    0xFF,
		BLASRef: blas,
	}}

	// Verify BLAS itself has a hittable triangle (at Z=5).
	origin := [3]float32{0, 0, 0}
	dir := [3]float32{0, 0, 1}

	hit, tVal, _, _ := TraverseBVH(blas.bvh, origin, dir, float32(math.MaxFloat32))
	if !hit {
		t.Fatal("expected BLAS traversal hit (triangle at Z=5)")
	}
	if math.Abs(float64(tVal-5.0)) > 0.01 {
		t.Errorf("BLAS hit t = %f, want ~5.0", tVal)
	}

	// Verify TLAS instance data.
	if len(tlas.instances) != 1 {
		t.Fatalf("TLAS instances = %d, want 1", len(tlas.instances))
	}
	inst := tlas.instances[0]
	if inst.BLASRef != blas {
		t.Error("TLAS instance should reference the built BLAS")
	}
	if inst.Mask != 0xFF {
		t.Errorf("instance mask = 0x%02X, want 0xFF", inst.Mask)
	}
	// Z translation at transform[11] (row 2, col 3).
	if inst.Transform[11] != 10.0 {
		t.Errorf("instance Z translate = %f, want 10.0", inst.Transform[11])
	}
}

// ---------------------------------------------------------------------------
// End-to-end: ray miss test
// ---------------------------------------------------------------------------

func TestIntegration_RayMiss(t *testing.T) {
	d := &Device{}

	// Triangle at Z=5.
	vertexData := make([]byte, 36)
	putFloat3(vertexData, 0, -1, -1, 5)
	putFloat3(vertexData, 12, 1, -1, 5)
	putFloat3(vertexData, 24, 0, 1, 5)

	vb := &Buffer{
		id:   nextResourceID.Add(1),
		data: vertexData,
		size: 36,
	}

	blasAS, err := d.CreateAccelerationStructure(&hal.AccelerationStructureDescriptor{
		Size:   4096,
		Format: hal.AccelerationStructureFormatBottomLevel,
	})
	if err != nil {
		t.Fatalf("CreateAS: %v", err)
	}

	enc := &CommandEncoder{device: d}
	enc.BuildAccelerationStructures([]hal.BuildAccelerationStructureDescriptor{{
		Entries: &hal.AccelerationStructureEntries{
			Triangles: []hal.AccelerationStructureTriangles{{
				VertexBuffer: vb,
				VertexCount:  3,
				VertexStride: 12,
			}},
		},
		DestinationAccelerationStructure: blasAS,
	}})

	blas := blasAS.(*AccelerationStructure)

	// Shoot ray in the opposite direction (-Z) -- should miss.
	origin := [3]float32{0, 0, 0}
	dirAway := [3]float32{0, 0, -1}

	if traverseDidHit(blas.bvh, origin, dirAway, float32(math.MaxFloat32)) {
		t.Error("expected miss (ray pointing away from triangle)")
	}

	// Shoot ray sideways -- should also miss.
	dirSide := [3]float32{1, 0, 0}
	if traverseDidHit(blas.bvh, origin, dirSide, float32(math.MaxFloat32)) {
		t.Error("expected miss (ray pointing sideways)")
	}
}

// ---------------------------------------------------------------------------
// End-to-end: compaction via CopyAccelerationStructure
// ---------------------------------------------------------------------------

func TestIntegration_Compaction(t *testing.T) {
	d := &Device{}

	// Build a BLAS with the compaction flag.
	vertexData := make([]byte, 36)
	putFloat3(vertexData, 0, 0, 0, 0)
	putFloat3(vertexData, 12, 1, 0, 0)
	putFloat3(vertexData, 24, 0, 1, 0)

	vb := &Buffer{
		id:   nextResourceID.Add(1),
		data: vertexData,
		size: 36,
	}

	srcAS, err := d.CreateAccelerationStructure(&hal.AccelerationStructureDescriptor{
		Label:           "compaction-src",
		Size:            8192,
		Format:          hal.AccelerationStructureFormatBottomLevel,
		AllowCompaction: true,
	})
	if err != nil {
		t.Fatalf("CreateAS: %v", err)
	}

	enc := &CommandEncoder{device: d}
	enc.BuildAccelerationStructures([]hal.BuildAccelerationStructureDescriptor{{
		Entries: &hal.AccelerationStructureEntries{
			Triangles: []hal.AccelerationStructureTriangles{{
				VertexBuffer: vb,
				VertexCount:  3,
				VertexStride: 12,
			}},
		},
		DestinationAccelerationStructure: srcAS,
	}})

	src := srcAS.(*AccelerationStructure)
	if src.bvh == nil {
		t.Fatal("source BVH must be built")
	}

	// Read compacted size.
	queryBuf := &Buffer{
		id:   nextResourceID.Add(1),
		data: make([]byte, 64),
		size: 64,
	}
	enc.ReadAccelerationStructureCompactSize(srcAS, queryBuf, 0)

	compactSize := binary.LittleEndian.Uint64(queryBuf.data[0:])
	if compactSize == 0 {
		t.Fatal("compacted size should be non-zero")
	}

	// Copy with Compact mode.
	dstAS, err := d.CreateAccelerationStructure(&hal.AccelerationStructureDescriptor{
		Label:  "compaction-dst",
		Size:   compactSize,
		Format: hal.AccelerationStructureFormatBottomLevel,
	})
	if err != nil {
		t.Fatalf("CreateAS dst: %v", err)
	}

	enc.CopyAccelerationStructure(srcAS, dstAS, gputypes.AccelerationStructureCopyModeCompact)

	dst := dstAS.(*AccelerationStructure)
	if dst.bvh == nil {
		t.Fatal("destination BVH should be non-nil after compact copy")
	}

	// Verify the compacted BLAS still produces correct ray hits.
	origin := [3]float32{0.25, 0.25, -5}
	dir := [3]float32{0, 0, 1}

	hit, tVal, _, _ := TraverseBVH(dst.bvh, origin, dir, float32(math.MaxFloat32))
	if !hit {
		t.Fatal("compacted BLAS should produce ray hit")
	}
	if math.Abs(float64(tVal-5.0)) > 0.01 {
		t.Errorf("t = %f, want ~5.0", tVal)
	}

	// Deep copy verification: modifying dst should not affect src.
	if dst.bvh == src.bvh {
		t.Error("compact copy should produce independent BVH tree")
	}
}

// ---------------------------------------------------------------------------
// TlasInstanceToBytes roundtrip
// ---------------------------------------------------------------------------

func TestIntegration_TlasInstanceRoundtrip(t *testing.T) {
	d := &Device{}

	instance := hal.TlasInstance{
		Transform: [12]float32{
			2, 0, 0, 100,
			0, 3, 0, 200,
			0, 0, 4, 300,
		},
		CustomData:                     0x00ABCDEF,
		Mask:                           0x42,
		BlasAddress:                    0x1234567890ABCDEF,
		ShaderBindingTableRecordOffset: 0x00000FFF,
	}

	buf := d.TlasInstanceToBytes(instance)
	if len(buf) != 64 {
		t.Fatalf("packed size = %d, want 64", len(buf))
	}

	// Verify transform (48 bytes).
	for i, want := range instance.Transform {
		got := math.Float32frombits(binary.LittleEndian.Uint32(buf[i*4:]))
		if got != want {
			t.Errorf("transform[%d] = %f, want %f", i, got, want)
		}
	}

	// Verify CustomData (bits 0-23) and Mask (bits 24-31).
	packed48 := binary.LittleEndian.Uint32(buf[48:])
	gotCustom := packed48 & 0x00FFFFFF
	gotMask := uint8((packed48 >> 24) & 0xFF)

	if gotCustom != (instance.CustomData & 0x00FFFFFF) {
		t.Errorf("customData = 0x%06X, want 0x%06X", gotCustom, instance.CustomData&0x00FFFFFF)
	}
	if gotMask != instance.Mask {
		t.Errorf("mask = 0x%02X, want 0x%02X", gotMask, instance.Mask)
	}

	// Verify SBT offset (bits 0-23).
	packed52 := binary.LittleEndian.Uint32(buf[52:])
	gotSBT := packed52 & 0x00FFFFFF
	if gotSBT != (instance.ShaderBindingTableRecordOffset & 0x00FFFFFF) {
		t.Errorf("SBT offset = 0x%06X, want 0x%06X", gotSBT, instance.ShaderBindingTableRecordOffset&0x00FFFFFF)
	}

	// Verify BLAS address (bytes 56-63).
	gotAddr := binary.LittleEndian.Uint64(buf[56:])
	if gotAddr != instance.BlasAddress {
		t.Errorf("BLAS address = 0x%016X, want 0x%016X", gotAddr, instance.BlasAddress)
	}
}

// ---------------------------------------------------------------------------
// Multi-geometry BLAS with multiple triangles
// ---------------------------------------------------------------------------

func TestIntegration_MultiGeometryBLAS(t *testing.T) {
	d := &Device{}

	// Two geometry groups: 1 triangle each at different Z depths.
	geom0Data := make([]byte, 36)
	putFloat3(geom0Data, 0, -1, -1, 3)
	putFloat3(geom0Data, 12, 1, -1, 3)
	putFloat3(geom0Data, 24, 0, 1, 3)

	geom1Data := make([]byte, 36)
	putFloat3(geom1Data, 0, -1, -1, 8)
	putFloat3(geom1Data, 12, 1, -1, 8)
	putFloat3(geom1Data, 24, 0, 1, 8)

	vb0 := &Buffer{id: nextResourceID.Add(1), data: geom0Data, size: 36}
	vb1 := &Buffer{id: nextResourceID.Add(1), data: geom1Data, size: 36}

	blasAS, err := d.CreateAccelerationStructure(&hal.AccelerationStructureDescriptor{
		Size:   8192,
		Format: hal.AccelerationStructureFormatBottomLevel,
	})
	if err != nil {
		t.Fatalf("CreateAS: %v", err)
	}

	enc := &CommandEncoder{device: d}
	enc.BuildAccelerationStructures([]hal.BuildAccelerationStructureDescriptor{{
		Entries: &hal.AccelerationStructureEntries{
			Triangles: []hal.AccelerationStructureTriangles{
				{VertexBuffer: vb0, VertexCount: 3, VertexStride: 12},
				{VertexBuffer: vb1, VertexCount: 3, VertexStride: 12},
			},
		},
		DestinationAccelerationStructure: blasAS,
	}})

	blas := blasAS.(*AccelerationStructure)
	if blas.bvh == nil {
		t.Fatal("BVH must be built")
	}

	// Ray from origin toward +Z should hit the closer triangle (Z=3).
	origin := [3]float32{0, 0, 0}
	dir := [3]float32{0, 0, 1}

	hit, tVal, geomIdx, _ := TraverseBVH(blas.bvh, origin, dir, float32(math.MaxFloat32))
	if !hit {
		t.Fatal("expected hit")
	}
	if math.Abs(float64(tVal-3.0)) > 0.01 {
		t.Errorf("t = %f, want ~3.0 (closer triangle)", tVal)
	}
	if geomIdx != 0 {
		t.Errorf("geomIdx = %d, want 0 (closer geometry)", geomIdx)
	}
}

// ---------------------------------------------------------------------------
// Device address uniqueness
// ---------------------------------------------------------------------------

func TestIntegration_DeviceAddressUniqueness(t *testing.T) {
	d := &Device{}

	as1, err := d.CreateAccelerationStructure(&hal.AccelerationStructureDescriptor{
		Size: 1024, Format: hal.AccelerationStructureFormatBottomLevel,
	})
	if err != nil {
		t.Fatal(err)
	}

	as2, err := d.CreateAccelerationStructure(&hal.AccelerationStructureDescriptor{
		Size: 2048, Format: hal.AccelerationStructureFormatBottomLevel,
	})
	if err != nil {
		t.Fatal(err)
	}

	addr1 := d.GetAccelerationStructureDeviceAddress(as1)
	addr2 := d.GetAccelerationStructureDeviceAddress(as2)

	if addr1 == addr2 {
		t.Error("two different AS should have different device addresses")
	}
	if addr1 == 0 || addr2 == 0 {
		t.Error("device addresses must be non-zero")
	}
}

// ---------------------------------------------------------------------------
// Build sizes are consistent
// ---------------------------------------------------------------------------

func TestIntegration_BuildSizesConsistency(t *testing.T) {
	d := &Device{}

	entries := &hal.AccelerationStructureEntries{
		Triangles: []hal.AccelerationStructureTriangles{
			{VertexCount: 300, VertexStride: 12},
		},
	}

	sizes := d.GetAccelerationStructureBuildSizes(&hal.GetAccelerationStructureBuildSizesDescriptor{
		Entries: entries,
	})

	if sizes.AccelerationStructureSize == 0 {
		t.Error("AS size must be non-zero for triangle geometry")
	}
	if sizes.BuildScratchSize == 0 {
		t.Error("build scratch must be non-zero")
	}
	if sizes.UpdateScratchSize == 0 {
		t.Error("update scratch must be non-zero")
	}

	// Querying the same descriptor twice should return the same sizes
	// (deterministic estimation).
	sizes2 := d.GetAccelerationStructureBuildSizes(&hal.GetAccelerationStructureBuildSizesDescriptor{
		Entries: entries,
	})
	if sizes != sizes2 {
		t.Errorf("build sizes not deterministic: %+v vs %+v", sizes, sizes2)
	}
}
