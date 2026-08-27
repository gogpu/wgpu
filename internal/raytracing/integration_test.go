package raytracing

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// ---------------------------------------------------------------------------
// Full BLAS lifecycle: Create -> Validate -> PrepareBuild -> BuiltIndex
// ---------------------------------------------------------------------------

func TestBLASFullLifecycle(t *testing.T) {
	ctx := newMockContext()

	// 1. Validate creation descriptor.
	desc := &gputypes.CreateBlasDescriptor{
		Label: "lifecycle-blas",
		Flags: gputypes.ASFlagAllowCompaction | gputypes.ASFlagPreferFastTrace,
	}
	sizes := &gputypes.BlasGeometrySizeDescriptors{
		Triangles: []gputypes.BlasTriangleGeometrySizeDescriptor{
			{VertexFormat: gputypes.VertexFormatFloat32x3, VertexCount: 300},
		},
	}

	if err := ValidateBlasCreate(ctx, desc, sizes); err != nil {
		t.Fatalf("ValidateBlasCreate: %v", err)
	}

	// 2. Construct BLAS in-memory (simulating core.Device).
	blas := &Blas{
		Label:         desc.Label,
		Flags:         desc.Flags,
		GeometryCount: 1,
		SizeInfo: hal.AccelerationStructureBuildSizes{
			AccelerationStructureSize: 4096,
			BuildScratchSize:          1024,
			UpdateScratchSize:         512,
		},
	}

	if blas.IsBuilt() {
		t.Fatal("fresh BLAS should not be built")
	}

	// 3. Prepare build via BuildContext.
	bc := NewBuildContext(ctx, 1)
	entries := []*BlasBuildEntry{{
		Blas: blas,
		Geometries: &hal.AccelerationStructureEntries{
			Triangles: []hal.AccelerationStructureTriangles{{
				VertexCount:  300,
				VertexStride: 12,
			}},
		},
	}}

	scratch, err := bc.PrepareBlasBuild(entries)
	if err != nil {
		t.Fatalf("PrepareBlasBuild: %v", err)
	}
	if scratch == 0 {
		t.Error("expected non-zero scratch size")
	}

	// 4. Verify BuiltIndex was assigned.
	if !blas.IsBuilt() {
		t.Fatal("BLAS should be marked built after PrepareBlasBuild")
	}
	if blas.BuiltIndex != 1 {
		t.Errorf("BuiltIndex = %d, want 1", blas.BuiltIndex)
	}

	// 5. Verify compaction is available (flag was set).
	if !blas.AllowsCompaction() {
		t.Error("BLAS should allow compaction")
	}
}

// ---------------------------------------------------------------------------
// Full TLAS lifecycle: Create -> Validate -> PrepareBuild with BLAS deps
// ---------------------------------------------------------------------------

func TestTLASFullLifecycle(t *testing.T) {
	ctx := newMockContext()

	// 1. Validate TLAS creation.
	tlasDesc := &gputypes.CreateTlasDescriptor{
		Label:        "lifecycle-tlas",
		MaxInstances: 64,
		Flags:        gputypes.ASFlagPreferFastTrace,
	}

	if err := ValidateTlasCreate(ctx, tlasDesc); err != nil {
		t.Fatalf("ValidateTlasCreate: %v", err)
	}

	// 2. Construct TLAS in-memory.
	tlas := &Tlas{
		Label:        tlasDesc.Label,
		Flags:        tlasDesc.Flags,
		MaxInstances: tlasDesc.MaxInstances,
		SizeInfo: hal.AccelerationStructureBuildSizes{
			AccelerationStructureSize: 8192,
			BuildScratchSize:          2048,
		},
		Dependencies: make([]uint64, 0, 4),
	}

	if tlas.IsBuilt() {
		t.Fatal("fresh TLAS should not be built")
	}

	// 3. Prepare TLAS build.
	bc := NewBuildContext(ctx, 2)
	tlasEntries := []*TlasBuildEntry{{
		Tlas:          tlas,
		InstanceCount: 10,
	}}

	scratch, err := bc.PrepareTlasBuild(tlasEntries)
	if err != nil {
		t.Fatalf("PrepareTlasBuild: %v", err)
	}
	if scratch == 0 {
		t.Error("expected non-zero TLAS scratch size")
	}

	// 4. Verify BuiltIndex.
	if !tlas.IsBuilt() {
		t.Fatal("TLAS should be built after PrepareTlasBuild")
	}
	if tlas.BuiltIndex != 2 {
		t.Errorf("TLAS BuiltIndex = %d, want 2", tlas.BuiltIndex)
	}
}

// ---------------------------------------------------------------------------
// Compaction full cycle: Request -> SetSize -> Complete -> verify savings
// ---------------------------------------------------------------------------

func TestCompactionFullCycleWithSavings(t *testing.T) {
	const originalSize uint64 = 16384
	const compactedSize uint64 = 10000

	blas := &Blas{
		Flags: gputypes.ASFlagAllowCompaction,
		SizeInfo: hal.AccelerationStructureBuildSizes{
			AccelerationStructureSize: originalSize,
		},
		BuiltIndex: 1,
	}

	// Phase 1: Idle -> Waiting.
	if err := RequestCompaction(blas); err != nil {
		t.Fatalf("RequestCompaction: %v", err)
	}
	if blas.Compaction != CompactionWaiting {
		t.Fatalf("state = %v, want Waiting", blas.Compaction)
	}

	// Phase 2: Waiting -> Ready.
	if err := SetCompactedSize(blas, compactedSize); err != nil {
		t.Fatalf("SetCompactedSize: %v", err)
	}
	if blas.Compaction != CompactionReady {
		t.Fatalf("state = %v, want Ready", blas.Compaction)
	}

	// Savings available in Ready state.
	savings := CompactionSavings(blas)
	want := originalSize - compactedSize
	if savings != want {
		t.Errorf("savings = %d, want %d", savings, want)
	}

	// Phase 3: Ready -> Compacted.
	if err := CompleteCompaction(blas); err != nil {
		t.Fatalf("CompleteCompaction: %v", err)
	}
	if blas.Compaction != CompactionCompacted {
		t.Fatalf("state = %v, want Compacted", blas.Compaction)
	}

	// Savings still available after compaction.
	if CompactionSavings(blas) != want {
		t.Errorf("post-compaction savings = %d, want %d", CompactionSavings(blas), want)
	}

	// Phase 4: double compaction rejected.
	if err := RequestCompaction(blas); err == nil {
		t.Fatal("expected error: double compaction")
	}
}

// ---------------------------------------------------------------------------
// Validation rejection: missing feature
// ---------------------------------------------------------------------------

func TestValidationRejectsMissingFeature(t *testing.T) {
	ctx := newMockContext()
	ctx.features = 0 // Disable FeatureRayQuery.

	// BLAS create.
	err := ValidateBlasCreate(ctx, &gputypes.CreateBlasDescriptor{}, &gputypes.BlasGeometrySizeDescriptors{
		Triangles: []gputypes.BlasTriangleGeometrySizeDescriptor{{}},
	})
	assertOp(t, err, opRayTracing)

	// TLAS create.
	err = ValidateTlasCreate(ctx, &gputypes.CreateTlasDescriptor{MaxInstances: 10})
	assertOp(t, err, opRayTracing)

	// BLAS build.
	err = ValidateBlasBuild(ctx, &BlasBuildEntry{
		Blas:       &Blas{},
		Geometries: &hal.AccelerationStructureEntries{Triangles: []hal.AccelerationStructureTriangles{{}}},
	})
	assertOp(t, err, opRayTracing)

	// TLAS build.
	err = ValidateTlasBuild(ctx, &TlasBuildEntry{
		Tlas:          &Tlas{MaxInstances: 10},
		InstanceCount: 1,
	}, nil)
	assertOp(t, err, opRayTracing)
}

// ---------------------------------------------------------------------------
// Validation rejection: exceeded limits
// ---------------------------------------------------------------------------

func TestValidationRejectsExceededLimits(t *testing.T) {
	ctx := newMockContext()
	ctx.limits.MaxBlasGeometryCount = 2
	ctx.limits.MaxTlasInstanceCount = 5

	// BLAS geometry count exceeded at creation.
	err := ValidateBlasCreate(ctx, &gputypes.CreateBlasDescriptor{}, &gputypes.BlasGeometrySizeDescriptors{
		Triangles: make([]gputypes.BlasTriangleGeometrySizeDescriptor, 3),
	})
	assertOp(t, err, opCreateBlas)

	// TLAS instance count exceeded at creation.
	err = ValidateTlasCreate(ctx, &gputypes.CreateTlasDescriptor{MaxInstances: 10})
	assertOp(t, err, opCreateTlas)

	// TLAS instance count exceeded at build time.
	err = ValidateTlasBuild(ctx, &TlasBuildEntry{
		Tlas:          &Tlas{MaxInstances: 100},
		InstanceCount: 10,
	}, nil)
	assertOp(t, err, opBuildTlas)
}

// ---------------------------------------------------------------------------
// Validation rejection: stale BLAS reference in TLAS build
// ---------------------------------------------------------------------------

func TestValidationRejectsStaleBLAS(t *testing.T) {
	ctx := newMockContext()

	builtBlas := &Blas{BuiltIndex: 3, Handle: 0xAAAA}
	staleBlas := &Blas{BuiltIndex: 0, Handle: 0xBBBB} // Never built.

	blasMap := map[uint64]*Blas{
		builtBlas.Handle: builtBlas,
		staleBlas.Handle: staleBlas,
	}

	entry := &TlasBuildEntry{
		Tlas:          &Tlas{Label: "depends-on-stale", MaxInstances: 100},
		InstanceCount: 2,
	}

	err := ValidateTlasBuild(ctx, entry, blasMap)
	assertOp(t, err, opBuildTlas)
}

// ---------------------------------------------------------------------------
// Build ordering: BLAS BuiltIndex tracking across multiple builds
// ---------------------------------------------------------------------------

func TestBuildOrderingMultipleBatches(t *testing.T) {
	ctx := newMockContext()

	blasA := &Blas{
		SizeInfo:      hal.AccelerationStructureBuildSizes{BuildScratchSize: 256},
		GeometryCount: 1,
	}
	blasB := &Blas{
		SizeInfo:      hal.AccelerationStructureBuildSizes{BuildScratchSize: 128},
		GeometryCount: 1,
	}

	geom := &hal.AccelerationStructureEntries{
		Triangles: []hal.AccelerationStructureTriangles{{}},
	}

	// Batch 1: build index = 1.
	bc1 := NewBuildContext(ctx, 1)
	_, err := bc1.PrepareBlasBuild([]*BlasBuildEntry{
		{Blas: blasA, Geometries: geom},
	})
	if err != nil {
		t.Fatalf("batch 1: %v", err)
	}
	if blasA.BuiltIndex != 1 {
		t.Errorf("blasA BuiltIndex = %d, want 1", blasA.BuiltIndex)
	}

	// Batch 2: build index = 2.
	bc2 := NewBuildContext(ctx, 2)
	_, err = bc2.PrepareBlasBuild([]*BlasBuildEntry{
		{Blas: blasB, Geometries: geom},
	})
	if err != nil {
		t.Fatalf("batch 2: %v", err)
	}
	if blasB.BuiltIndex != 2 {
		t.Errorf("blasB BuiltIndex = %d, want 2", blasB.BuiltIndex)
	}

	// Verify ordering: A was built before B.
	if blasA.BuiltIndex >= blasB.BuiltIndex {
		t.Error("blasA should have a lower BuiltIndex than blasB")
	}
}

// ---------------------------------------------------------------------------
// BLAS + TLAS combined build in same context
// ---------------------------------------------------------------------------

func TestCombinedBLASTLASBuild(t *testing.T) {
	ctx := newMockContext()

	blas := &Blas{
		SizeInfo:      hal.AccelerationStructureBuildSizes{BuildScratchSize: 512},
		GeometryCount: 1,
	}
	tlas := &Tlas{
		SizeInfo:     hal.AccelerationStructureBuildSizes{BuildScratchSize: 1024},
		MaxInstances: 16,
	}

	bc := NewBuildContext(ctx, 3)

	// Build BLAS first.
	blasScratch, err := bc.PrepareBlasBuild([]*BlasBuildEntry{{
		Blas: blas,
		Geometries: &hal.AccelerationStructureEntries{
			Triangles: []hal.AccelerationStructureTriangles{{}},
		},
	}})
	if err != nil {
		t.Fatalf("PrepareBlasBuild: %v", err)
	}

	// Build TLAS second (same build index).
	tlasScratch, err := bc.PrepareTlasBuild([]*TlasBuildEntry{{
		Tlas:          tlas,
		InstanceCount: 4,
	}})
	if err != nil {
		t.Fatalf("PrepareTlasBuild: %v", err)
	}

	// Both should have the same build index.
	if blas.BuiltIndex != 3 {
		t.Errorf("BLAS BuiltIndex = %d, want 3", blas.BuiltIndex)
	}
	if tlas.BuiltIndex != 3 {
		t.Errorf("TLAS BuiltIndex = %d, want 3", tlas.BuiltIndex)
	}

	// Total scratch should be sum of aligned sizes.
	totalScratch := blasScratch + tlasScratch
	if totalScratch == 0 {
		t.Error("total scratch should be non-zero")
	}
}

// ---------------------------------------------------------------------------
// Compaction without AllowCompaction flag
// ---------------------------------------------------------------------------

func TestCompactionRejectedWithoutFlag(t *testing.T) {
	blas := &Blas{
		Flags:      gputypes.ASFlagPreferFastTrace, // No ASFlagAllowCompaction.
		BuiltIndex: 1,
	}

	err := RequestCompaction(blas)
	assertOp(t, err, opCompact)
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func assertOp(t *testing.T, err error, wantOp string) {
	t.Helper()

	if err == nil {
		t.Fatalf("expected error with Op=%q, got nil", wantOp)
	}

	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *ValidationError, got %T: %v", err, err)
	}
	if ve.Op != wantOp {
		t.Errorf("Op = %q, want %q (error: %v)", ve.Op, wantOp, err)
	}
}
