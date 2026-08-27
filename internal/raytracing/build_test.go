package raytracing

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// --- mockDeviceContext ---

type mockDeviceContext struct {
	features   gputypes.Features
	limits     gputypes.Limits
	alignments hal.Alignments
}

func (m *mockDeviceContext) DeviceFeatures() gputypes.Features { return m.features }
func (m *mockDeviceContext) DeviceLimits() gputypes.Limits     { return m.limits }
func (m *mockDeviceContext) DeviceAlignments() hal.Alignments  { return m.alignments }

func (m *mockDeviceContext) CreateBuffer(_ *hal.BufferDescriptor) (hal.Buffer, error) {
	return nil, nil //nolint:nilnil // test mock — CreateBuffer never called in these tests
}

func (m *mockDeviceContext) DestroyBuffer(_ hal.Buffer) {}
func (m *mockDeviceContext) HALDevice() hal.Device      { return nil }

func newMockContext() *mockDeviceContext {
	return &mockDeviceContext{
		features: gputypes.Features(gputypes.FeatureRayQuery),
		limits: gputypes.Limits{
			MaxBlasGeometryCount: 64,
			MaxTlasInstanceCount: 1024,
		},
		alignments: hal.Alignments{
			RayTracingScratchBufferAlignment: 256,
		},
	}
}

// --- NewBuildContext tests ---

func TestNewBuildContext(t *testing.T) {
	ctx := newMockContext()
	bc := NewBuildContext(ctx, 42)

	if bc.device != ctx {
		t.Error("device mismatch")
	}

	if bc.buildIndex != 42 {
		t.Errorf("buildIndex = %d, want 42", bc.buildIndex)
	}
}

// --- PrepareBlasBuild tests ---

func TestPrepareBlasBuildEmpty(t *testing.T) {
	ctx := newMockContext()
	bc := NewBuildContext(ctx, 1)

	size, err := bc.PrepareBlasBuild(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if size != 0 {
		t.Errorf("scratch size = %d, want 0", size)
	}
}

func TestPrepareBlasBuildScratchSizeAlignment(t *testing.T) {
	ctx := newMockContext()
	bc := NewBuildContext(ctx, 1)

	entries := []*BlasBuildEntry{
		{
			Blas: &Blas{
				Label:         "blas-a",
				SizeInfo:      hal.AccelerationStructureBuildSizes{BuildScratchSize: 100},
				GeometryCount: 1,
			},
			Geometries: &hal.AccelerationStructureEntries{
				Triangles: []hal.AccelerationStructureTriangles{{}},
			},
		},
		{
			Blas: &Blas{
				Label:         "blas-b",
				SizeInfo:      hal.AccelerationStructureBuildSizes{BuildScratchSize: 300},
				GeometryCount: 2,
			},
			Geometries: &hal.AccelerationStructureEntries{
				Triangles: []hal.AccelerationStructureTriangles{{}, {}},
			},
		},
	}

	size, err := bc.PrepareBlasBuild(entries)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 100 aligned to 256 = 256, 300 aligned to 256 = 512.
	// Total = 256 + 512 = 768.
	if size != 768 {
		t.Errorf("scratch size = %d, want 768", size)
	}
}

func TestPrepareBlasBuildSetsBuiltIndex(t *testing.T) {
	ctx := newMockContext()
	bc := NewBuildContext(ctx, 7)

	blas := &Blas{
		SizeInfo:      hal.AccelerationStructureBuildSizes{BuildScratchSize: 64},
		GeometryCount: 1,
	}
	entries := []*BlasBuildEntry{
		{
			Blas:       blas,
			Geometries: &hal.AccelerationStructureEntries{Triangles: []hal.AccelerationStructureTriangles{{}}},
		},
	}

	_, err := bc.PrepareBlasBuild(entries)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if blas.BuiltIndex != 7 {
		t.Errorf("BuiltIndex = %d, want 7", blas.BuiltIndex)
	}
}

func TestPrepareBlasBuildGeometryCountExceeded(t *testing.T) {
	ctx := newMockContext()
	ctx.limits.MaxBlasGeometryCount = 2
	bc := NewBuildContext(ctx, 1)

	tris := make([]hal.AccelerationStructureTriangles, 3)
	entries := []*BlasBuildEntry{
		{
			Blas: &Blas{
				SizeInfo:      hal.AccelerationStructureBuildSizes{BuildScratchSize: 64},
				GeometryCount: 3,
			},
			Geometries: &hal.AccelerationStructureEntries{Triangles: tris},
		},
	}

	_, err := bc.PrepareBlasBuild(entries)
	if err == nil {
		t.Fatal("expected error for geometry count exceeded")
	}

	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *ValidationError, got %T", err)
	}

	if ve.Op != opBuildBlas {
		t.Errorf("Op = %q, want %q", ve.Op, opBuildBlas)
	}
}

func TestPrepareBlasBuildNilBlas(t *testing.T) {
	ctx := newMockContext()
	bc := NewBuildContext(ctx, 1)

	entries := []*BlasBuildEntry{
		{
			Blas:       nil,
			Geometries: &hal.AccelerationStructureEntries{Triangles: []hal.AccelerationStructureTriangles{{}}},
		},
	}

	_, err := bc.PrepareBlasBuild(entries)
	if err == nil {
		t.Fatal("expected error for nil BLAS")
	}
}

func TestPrepareBlasBuildNilGeometries(t *testing.T) {
	ctx := newMockContext()
	bc := NewBuildContext(ctx, 1)

	entries := []*BlasBuildEntry{
		{
			Blas:       &Blas{SizeInfo: hal.AccelerationStructureBuildSizes{BuildScratchSize: 64}},
			Geometries: nil,
		},
	}

	_, err := bc.PrepareBlasBuild(entries)
	if err == nil {
		t.Fatal("expected error for nil geometries")
	}
}

// --- PrepareTlasBuild tests ---

func TestPrepareTlasBuildEmpty(t *testing.T) {
	ctx := newMockContext()
	bc := NewBuildContext(ctx, 1)

	size, err := bc.PrepareTlasBuild(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if size != 0 {
		t.Errorf("scratch size = %d, want 0", size)
	}
}

func TestPrepareTlasBuildScratchSizeAlignment(t *testing.T) {
	ctx := newMockContext()
	bc := NewBuildContext(ctx, 1)

	entries := []*TlasBuildEntry{
		{
			Tlas: &Tlas{
				Label:        "tlas-a",
				SizeInfo:     hal.AccelerationStructureBuildSizes{BuildScratchSize: 500},
				MaxInstances: 100,
			},
			InstanceCount: 10,
		},
		{
			Tlas: &Tlas{
				Label:        "tlas-b",
				SizeInfo:     hal.AccelerationStructureBuildSizes{BuildScratchSize: 200},
				MaxInstances: 50,
			},
			InstanceCount: 5,
		},
	}

	size, err := bc.PrepareTlasBuild(entries)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 500 aligned to 256 = 512, 200 aligned to 256 = 256.
	// Total = 512 + 256 = 768.
	if size != 768 {
		t.Errorf("scratch size = %d, want 768", size)
	}
}

func TestPrepareTlasBuildSetsBuiltIndex(t *testing.T) {
	ctx := newMockContext()
	bc := NewBuildContext(ctx, 5)

	tlas := &Tlas{
		SizeInfo:     hal.AccelerationStructureBuildSizes{BuildScratchSize: 128},
		MaxInstances: 100,
	}
	entries := []*TlasBuildEntry{
		{Tlas: tlas, InstanceCount: 10},
	}

	_, err := bc.PrepareTlasBuild(entries)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tlas.BuiltIndex != 5 {
		t.Errorf("BuiltIndex = %d, want 5", tlas.BuiltIndex)
	}
}

func TestPrepareTlasBuildInstanceCountExceedsDeviceLimit(t *testing.T) {
	ctx := newMockContext()
	ctx.limits.MaxTlasInstanceCount = 10
	bc := NewBuildContext(ctx, 1)

	entries := []*TlasBuildEntry{
		{
			Tlas: &Tlas{
				SizeInfo:     hal.AccelerationStructureBuildSizes{BuildScratchSize: 64},
				MaxInstances: 100,
			},
			InstanceCount: 20,
		},
	}

	_, err := bc.PrepareTlasBuild(entries)
	if err == nil {
		t.Fatal("expected error for instance count exceeded")
	}
}

func TestPrepareTlasBuildInstanceCountExceedsTlasMax(t *testing.T) {
	ctx := newMockContext()
	bc := NewBuildContext(ctx, 1)

	entries := []*TlasBuildEntry{
		{
			Tlas: &Tlas{
				SizeInfo:     hal.AccelerationStructureBuildSizes{BuildScratchSize: 64},
				MaxInstances: 5,
			},
			InstanceCount: 10,
		},
	}

	_, err := bc.PrepareTlasBuild(entries)
	if err == nil {
		t.Fatal("expected error for instance count > TLAS MaxInstances")
	}
}

func TestPrepareTlasBuildNilTlas(t *testing.T) {
	ctx := newMockContext()
	bc := NewBuildContext(ctx, 1)

	entries := []*TlasBuildEntry{
		{Tlas: nil, InstanceCount: 1},
	}

	_, err := bc.PrepareTlasBuild(entries)
	if err == nil {
		t.Fatal("expected error for nil TLAS")
	}
}

// --- alignUp64 tests ---

func TestAlignUp64(t *testing.T) {
	tests := []struct {
		n, alignment, want uint64
	}{
		{0, 256, 0},
		{1, 256, 256},
		{255, 256, 256},
		{256, 256, 256},
		{257, 256, 512},
		{100, 0, 100},     // alignment=0 returns n
		{0, 0, 0},         // both zero
		{1024, 256, 1024}, // already aligned
	}

	for _, tt := range tests {
		got := alignUp64(tt.n, tt.alignment)
		if got != tt.want {
			t.Errorf("alignUp64(%d, %d) = %d, want %d", tt.n, tt.alignment, got, tt.want)
		}
	}
}

// --- scratchAlignment tests ---

func TestScratchAlignmentDefault(t *testing.T) {
	ctx := newMockContext()
	ctx.alignments.RayTracingScratchBufferAlignment = 0
	bc := NewBuildContext(ctx, 1)

	a := bc.scratchAlignment()
	if a != 256 {
		t.Errorf("scratchAlignment() = %d, want 256 (default)", a)
	}
}

func TestScratchAlignmentFromDevice(t *testing.T) {
	ctx := newMockContext()
	ctx.alignments.RayTracingScratchBufferAlignment = 512
	bc := NewBuildContext(ctx, 1)

	a := bc.scratchAlignment()
	if a != 512 {
		t.Errorf("scratchAlignment() = %d, want 512", a)
	}
}

// --- blasEntryGeometryCount tests ---

func TestBlasEntryGeometryCount(t *testing.T) {
	// Triangles.
	entries := &hal.AccelerationStructureEntries{
		Triangles: make([]hal.AccelerationStructureTriangles, 5),
	}
	if got := blasEntryGeometryCount(entries); got != 5 {
		t.Errorf("triangles count = %d, want 5", got)
	}

	// AABBs.
	entries = &hal.AccelerationStructureEntries{
		AABBs: make([]hal.AccelerationStructureAABBs, 3),
	}
	if got := blasEntryGeometryCount(entries); got != 3 {
		t.Errorf("aabbs count = %d, want 3", got)
	}

	// Empty (instances only).
	entries = &hal.AccelerationStructureEntries{
		Instances: &hal.AccelerationStructureInstances{},
	}
	if got := blasEntryGeometryCount(entries); got != 0 {
		t.Errorf("instances-only count = %d, want 0", got)
	}
}
