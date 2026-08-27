package raytracing

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// --- ValidateBlasCreate tests ---

func TestValidateBlasCreateHappyPath(t *testing.T) {
	ctx := newMockContext()
	desc := &gputypes.CreateBlasDescriptor{
		Label: "test-blas",
		Flags: gputypes.ASFlagPreferFastTrace,
	}
	sizes := &gputypes.BlasGeometrySizeDescriptors{
		Triangles: []gputypes.BlasTriangleGeometrySizeDescriptor{
			{VertexFormat: gputypes.VertexFormatFloat32x3, VertexCount: 100},
		},
	}

	if err := ValidateBlasCreate(ctx, desc, sizes); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateBlasCreateNoRayQuery(t *testing.T) {
	ctx := newMockContext()
	ctx.features = 0 // No FeatureRayQuery.

	desc := &gputypes.CreateBlasDescriptor{}
	sizes := &gputypes.BlasGeometrySizeDescriptors{
		Triangles: []gputypes.BlasTriangleGeometrySizeDescriptor{{}},
	}

	err := ValidateBlasCreate(ctx, desc, sizes)
	assertValidationError(t, err, "RayTracing")
}

func TestValidateBlasCreateGeometryCountExceeded(t *testing.T) {
	ctx := newMockContext()
	ctx.limits.MaxBlasGeometryCount = 2

	desc := &gputypes.CreateBlasDescriptor{}
	sizes := &gputypes.BlasGeometrySizeDescriptors{
		Triangles: make([]gputypes.BlasTriangleGeometrySizeDescriptor, 5),
	}

	err := ValidateBlasCreate(ctx, desc, sizes)
	assertValidationError(t, err, "CreateBlas")
}

func TestValidateBlasCreateBothGeometryTypes(t *testing.T) {
	ctx := newMockContext()
	desc := &gputypes.CreateBlasDescriptor{}
	sizes := &gputypes.BlasGeometrySizeDescriptors{
		Triangles: []gputypes.BlasTriangleGeometrySizeDescriptor{{}},
		AABBs:     []gputypes.BlasAABBGeometrySizeDescriptor{{}},
	}

	err := ValidateBlasCreate(ctx, desc, sizes)
	assertValidationError(t, err, "CreateBlas")
}

func TestValidateBlasCreateNeitherGeometryType(t *testing.T) {
	ctx := newMockContext()
	desc := &gputypes.CreateBlasDescriptor{}
	sizes := &gputypes.BlasGeometrySizeDescriptors{}

	err := ValidateBlasCreate(ctx, desc, sizes)
	assertValidationError(t, err, "CreateBlas")
}

func TestValidateBlasCreateNilSizes(t *testing.T) {
	ctx := newMockContext()
	desc := &gputypes.CreateBlasDescriptor{}

	err := ValidateBlasCreate(ctx, desc, nil)
	assertValidationError(t, err, "CreateBlas")
}

func TestValidateBlasCreateUpdateWithoutFlag(t *testing.T) {
	ctx := newMockContext()
	desc := &gputypes.CreateBlasDescriptor{
		Flags:      gputypes.ASFlagPreferFastBuild,
		UpdateMode: gputypes.AccelerationStructureUpdateModePreferUpdate,
	}
	sizes := &gputypes.BlasGeometrySizeDescriptors{
		Triangles: []gputypes.BlasTriangleGeometrySizeDescriptor{{}},
	}

	err := ValidateBlasCreate(ctx, desc, sizes)
	assertValidationError(t, err, "CreateBlas")
}

func TestValidateBlasCreateUpdateWithFlag(t *testing.T) {
	ctx := newMockContext()
	desc := &gputypes.CreateBlasDescriptor{
		Flags:      gputypes.ASFlagAllowUpdate,
		UpdateMode: gputypes.AccelerationStructureUpdateModePreferUpdate,
	}
	sizes := &gputypes.BlasGeometrySizeDescriptors{
		Triangles: []gputypes.BlasTriangleGeometrySizeDescriptor{{}},
	}

	if err := ValidateBlasCreate(ctx, desc, sizes); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- ValidateTlasCreate tests ---

func TestValidateTlasCreateHappyPath(t *testing.T) {
	ctx := newMockContext()
	desc := &gputypes.CreateTlasDescriptor{
		Label:        "test-tlas",
		MaxInstances: 100,
	}

	if err := ValidateTlasCreate(ctx, desc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateTlasCreateNoRayQuery(t *testing.T) {
	ctx := newMockContext()
	ctx.features = 0

	desc := &gputypes.CreateTlasDescriptor{MaxInstances: 10}

	err := ValidateTlasCreate(ctx, desc)
	assertValidationError(t, err, "RayTracing")
}

func TestValidateTlasCreateZeroInstances(t *testing.T) {
	ctx := newMockContext()
	desc := &gputypes.CreateTlasDescriptor{MaxInstances: 0}

	err := ValidateTlasCreate(ctx, desc)
	assertValidationError(t, err, "CreateTlas")
}

func TestValidateTlasCreateInstanceCountExceeded(t *testing.T) {
	ctx := newMockContext()
	ctx.limits.MaxTlasInstanceCount = 50

	desc := &gputypes.CreateTlasDescriptor{MaxInstances: 100}

	err := ValidateTlasCreate(ctx, desc)
	assertValidationError(t, err, "CreateTlas")
}

func TestValidateTlasCreateUpdateWithoutFlag(t *testing.T) {
	ctx := newMockContext()
	desc := &gputypes.CreateTlasDescriptor{
		MaxInstances: 10,
		Flags:        gputypes.ASFlagPreferFastTrace,
		UpdateMode:   gputypes.AccelerationStructureUpdateModePreferUpdate,
	}

	err := ValidateTlasCreate(ctx, desc)
	assertValidationError(t, err, "CreateTlas")
}

// --- ValidateBlasBuild tests ---

func TestValidateBlasBuildHappyPath(t *testing.T) {
	ctx := newMockContext()
	entry := &BlasBuildEntry{
		Blas: &Blas{GeometryCount: 1},
		Geometries: &hal.AccelerationStructureEntries{
			Triangles: []hal.AccelerationStructureTriangles{{}},
		},
	}

	if err := ValidateBlasBuild(ctx, entry); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateBlasBuildNoRayQuery(t *testing.T) {
	ctx := newMockContext()
	ctx.features = 0

	entry := &BlasBuildEntry{
		Blas:       &Blas{},
		Geometries: &hal.AccelerationStructureEntries{Triangles: []hal.AccelerationStructureTriangles{{}}},
	}

	err := ValidateBlasBuild(ctx, entry)
	assertValidationError(t, err, "RayTracing")
}

func TestValidateBlasBuildNilBlas(t *testing.T) {
	ctx := newMockContext()
	entry := &BlasBuildEntry{
		Blas:       nil,
		Geometries: &hal.AccelerationStructureEntries{},
	}

	err := ValidateBlasBuild(ctx, entry)
	assertValidationError(t, err, "BuildBlas")
}

func TestValidateBlasBuildNilGeometries(t *testing.T) {
	ctx := newMockContext()
	entry := &BlasBuildEntry{
		Blas:       &Blas{},
		Geometries: nil,
	}

	err := ValidateBlasBuild(ctx, entry)
	assertValidationError(t, err, "BuildBlas")
}

func TestValidateBlasBuildAABBStrideTooSmall(t *testing.T) {
	ctx := newMockContext()
	entry := &BlasBuildEntry{
		Blas: &Blas{},
		Geometries: &hal.AccelerationStructureEntries{
			AABBs: []hal.AccelerationStructureAABBs{
				{Stride: 16, Count: 10}, // 16 < 24 (AABBGeometryMinStride).
			},
		},
	}

	err := ValidateBlasBuild(ctx, entry)
	assertValidationError(t, err, "BuildBlas")
}

func TestValidateBlasBuildAABBStrideValid(t *testing.T) {
	ctx := newMockContext()
	entry := &BlasBuildEntry{
		Blas: &Blas{},
		Geometries: &hal.AccelerationStructureEntries{
			AABBs: []hal.AccelerationStructureAABBs{
				{Stride: 24, Count: 10},
				{Stride: 48, Count: 5},
			},
		},
	}

	if err := ValidateBlasBuild(ctx, entry); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateBlasBuildTransformAlignment(t *testing.T) {
	ctx := newMockContext()

	// Misaligned transform offset.
	entry := &BlasBuildEntry{
		Blas: &Blas{},
		Geometries: &hal.AccelerationStructureEntries{
			Triangles: []hal.AccelerationStructureTriangles{
				{
					Transform: &hal.AccelerationStructureTriangleTransform{Offset: 7},
				},
			},
		},
	}

	err := ValidateBlasBuild(ctx, entry)
	assertValidationError(t, err, "BuildBlas")

	// Aligned transform offset.
	entry.Geometries.Triangles[0].Transform.Offset = 32
	if err := ValidateBlasBuild(ctx, entry); err != nil {
		t.Fatalf("unexpected error for aligned offset: %v", err)
	}
}

// --- ValidateTlasBuild tests ---

func TestValidateTlasBuildHappyPath(t *testing.T) {
	ctx := newMockContext()
	entry := &TlasBuildEntry{
		Tlas: &Tlas{
			Label:        "tlas",
			MaxInstances: 100,
		},
		InstanceCount: 10,
	}
	blasMap := map[uint64]*Blas{
		0xDEAD: {BuiltIndex: 1},
		0xBEEF: {BuiltIndex: 2},
	}

	if err := ValidateTlasBuild(ctx, entry, blasMap); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateTlasBuildNoRayQuery(t *testing.T) {
	ctx := newMockContext()
	ctx.features = 0

	entry := &TlasBuildEntry{
		Tlas:          &Tlas{MaxInstances: 10},
		InstanceCount: 1,
	}

	err := ValidateTlasBuild(ctx, entry, nil)
	assertValidationError(t, err, "RayTracing")
}

func TestValidateTlasBuildNilTlas(t *testing.T) {
	ctx := newMockContext()
	entry := &TlasBuildEntry{Tlas: nil, InstanceCount: 1}

	err := ValidateTlasBuild(ctx, entry, nil)
	assertValidationError(t, err, "BuildTlas")
}

func TestValidateTlasBuildInstanceCountExceeded(t *testing.T) {
	ctx := newMockContext()
	ctx.limits.MaxTlasInstanceCount = 10

	entry := &TlasBuildEntry{
		Tlas:          &Tlas{MaxInstances: 100},
		InstanceCount: 20,
	}

	err := ValidateTlasBuild(ctx, entry, nil)
	assertValidationError(t, err, "BuildTlas")
}

func TestValidateTlasBuildInstanceCountExceedsTlasMax(t *testing.T) {
	ctx := newMockContext()
	entry := &TlasBuildEntry{
		Tlas:          &Tlas{MaxInstances: 5},
		InstanceCount: 10,
	}

	err := ValidateTlasBuild(ctx, entry, nil)
	assertValidationError(t, err, "BuildTlas")
}

func TestValidateTlasBuildUnbuiltBlasDependency(t *testing.T) {
	ctx := newMockContext()
	entry := &TlasBuildEntry{
		Tlas: &Tlas{
			Label:        "tlas",
			MaxInstances: 100,
		},
		InstanceCount: 5,
	}
	blasMap := map[uint64]*Blas{
		0xDEAD: {BuiltIndex: 1},
		0xBEEF: {BuiltIndex: 0}, // Not built.
	}

	err := ValidateTlasBuild(ctx, entry, blasMap)
	assertValidationError(t, err, "BuildTlas")
}

func TestValidateTlasBuildEmptyBlasMap(t *testing.T) {
	ctx := newMockContext()
	entry := &TlasBuildEntry{
		Tlas:          &Tlas{MaxInstances: 100},
		InstanceCount: 0,
	}

	// Empty map is valid (no dependencies).
	if err := ValidateTlasBuild(ctx, entry, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- ValidationError tests ---

func TestValidationErrorFormat(t *testing.T) {
	err := &ValidationError{Op: "CreateBlas", Message: "geometry count exceeded"}
	want := "ray tracing CreateBlas: geometry count exceeded"

	if got := err.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

// --- Helper ---

func assertValidationError(t *testing.T, err error, wantOp string) {
	t.Helper()

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *ValidationError, got %T: %v", err, err)
	}

	if ve.Op != wantOp {
		t.Errorf("Op = %q, want %q", ve.Op, wantOp)
	}
}
