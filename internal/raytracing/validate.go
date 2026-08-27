package raytracing

import (
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// ValidateBlasCreate validates parameters for BLAS creation.
//
// Checks:
//   - FeatureRayQuery must be enabled.
//   - Geometry count must not exceed MaxBlasGeometryCount.
//   - AABB stride must be >= AABBGeometryMinStride.
//   - Update mode PreferUpdate requires ASFlagAllowUpdate.
//
// Reference: Rust wgpu-core device/resource.rs create_blas.
func ValidateBlasCreate(
	ctx DeviceContext,
	desc *gputypes.CreateBlasDescriptor,
	sizes *gputypes.BlasGeometrySizeDescriptors,
) error {
	if err := requireRayQuery(ctx); err != nil {
		return err
	}

	if err := validateBlasGeometrySizes(ctx, sizes); err != nil {
		return err
	}

	return validateUpdateFlags(opCreateBlas, desc.Flags, desc.UpdateMode)
}

// ValidateTlasCreate validates parameters for TLAS creation.
//
// Checks:
//   - FeatureRayQuery must be enabled.
//   - MaxInstances must not exceed MaxTlasInstanceCount.
//   - MaxInstances must be > 0.
//   - Update mode PreferUpdate requires ASFlagAllowUpdate.
//
// Reference: Rust wgpu-core device/resource.rs create_tlas.
func ValidateTlasCreate(
	ctx DeviceContext,
	desc *gputypes.CreateTlasDescriptor,
) error {
	if err := requireRayQuery(ctx); err != nil {
		return err
	}

	if err := validateTlasMaxInstances(ctx, desc.MaxInstances); err != nil {
		return err
	}

	return validateUpdateFlags(opCreateTlas, desc.Flags, desc.UpdateMode)
}

// ValidateBlasBuild validates a BLAS build entry against device limits.
//
// Checks:
//   - FeatureRayQuery must be enabled.
//   - Geometry count must not exceed MaxBlasGeometryCount.
//   - AABB geometries: stride >= AABBGeometryMinStride.
//   - Triangle geometries: transform buffer alignment.
//
// Reference: Rust wgpu-core command/ray_tracing.rs iter_blas.
func ValidateBlasBuild(ctx DeviceContext, entry *BlasBuildEntry) error {
	if err := requireRayQuery(ctx); err != nil {
		return err
	}

	if entry.Blas == nil {
		return &ValidationError{Op: opBuildBlas, Message: "nil BLAS"}
	}

	if entry.Geometries == nil {
		return &ValidationError{Op: opBuildBlas, Message: "nil geometry entries"}
	}

	if err := validateBuildGeometryCount(ctx, entry); err != nil {
		return err
	}

	return validateBuildGeometryDetails(entry)
}

// ValidateTlasBuild validates a TLAS build entry.
//
// Checks:
//   - FeatureRayQuery must be enabled.
//   - Instance count must not exceed MaxTlasInstanceCount.
//   - Instance count must not exceed TLAS MaxInstances.
//   - All referenced BLASes in blasMap must be built (BuiltIndex > 0).
//
// Reference: Rust wgpu-core command/ray_tracing.rs tlas validation +
// ValidateAsActionsError (ray_tracing.rs:254-276).
func ValidateTlasBuild(
	ctx DeviceContext,
	entry *TlasBuildEntry,
	blasMap map[uint64]*Blas,
) error {
	if err := requireRayQuery(ctx); err != nil {
		return err
	}

	if entry.Tlas == nil {
		return &ValidationError{Op: opBuildTlas, Message: "nil TLAS"}
	}

	if err := validateTlasBuildInstances(ctx, entry); err != nil {
		return err
	}

	return validateTlasBlasDependencies(entry, blasMap)
}

// --- Internal validation helpers ---

// requireRayQuery checks that FeatureRayQuery is enabled on the device.
func requireRayQuery(ctx DeviceContext) error {
	if !ctx.DeviceFeatures().Contains(gputypes.FeatureRayQuery) {
		return &ValidationError{
			Op:      opRayTracing,
			Message: "FeatureRayQuery is not enabled",
		}
	}

	return nil
}

// validateBlasGeometrySizes validates geometry size descriptors at creation time.
func validateBlasGeometrySizes(ctx DeviceContext, sizes *gputypes.BlasGeometrySizeDescriptors) error {
	if sizes == nil {
		return &ValidationError{Op: opCreateBlas, Message: "nil geometry size descriptors"}
	}

	limits := ctx.DeviceLimits()
	geomCount := blasGeometrySizeCount(sizes)

	if geomCount > limits.MaxBlasGeometryCount {
		return &ValidationError{
			Op: opCreateBlas,
			Message: fmt.Sprintf(
				"geometry count %d exceeds MaxBlasGeometryCount %d",
				geomCount, limits.MaxBlasGeometryCount,
			),
		}
	}

	return validateAABBStrides(sizes)
}

// validateAABBStrides checks that AABB geometry descriptors are valid.
// Currently validates that AABBs exist and are not empty when specified.
// AABB stride is validated at build time (not creation time) since the
// stride is provided in the build entry, not the size descriptor.
func validateAABBStrides(sizes *gputypes.BlasGeometrySizeDescriptors) error {
	if sizes.Triangles != nil && sizes.AABBs != nil {
		return &ValidationError{
			Op:      opCreateBlas,
			Message: "both Triangles and AABBs specified (must be exactly one)",
		}
	}

	if sizes.Triangles == nil && sizes.AABBs == nil {
		return &ValidationError{
			Op:      opCreateBlas,
			Message: "neither Triangles nor AABBs specified (must be exactly one)",
		}
	}

	return nil
}

// validateTlasMaxInstances checks TLAS instance count limits.
func validateTlasMaxInstances(ctx DeviceContext, maxInstances uint32) error {
	if maxInstances == 0 {
		return &ValidationError{
			Op:      opCreateTlas,
			Message: "MaxInstances must be > 0",
		}
	}

	limits := ctx.DeviceLimits()
	if maxInstances > limits.MaxTlasInstanceCount {
		return &ValidationError{
			Op: opCreateTlas,
			Message: fmt.Sprintf(
				"MaxInstances %d exceeds MaxTlasInstanceCount %d",
				maxInstances, limits.MaxTlasInstanceCount,
			),
		}
	}

	return nil
}

// validateUpdateFlags checks that ASFlagAllowUpdate is set when update
// mode is PreferUpdate.
func validateUpdateFlags(
	op string,
	flags gputypes.AccelerationStructureFlags,
	mode gputypes.AccelerationStructureUpdateMode,
) error {
	if mode != gputypes.AccelerationStructureUpdateModePreferUpdate {
		return nil
	}

	if !flags.Contains(gputypes.ASFlagAllowUpdate) {
		return &ValidationError{
			Op:      op,
			Message: "UpdateMode is PreferUpdate but ASFlagAllowUpdate is not set",
		}
	}

	return nil
}

// validateBuildGeometryCount checks build-time geometry count limits.
func validateBuildGeometryCount(ctx DeviceContext, entry *BlasBuildEntry) error {
	limits := ctx.DeviceLimits()
	geomCount := blasEntryGeometryCount(entry.Geometries)

	if geomCount > limits.MaxBlasGeometryCount {
		return &ValidationError{
			Op: opBuildBlas,
			Message: fmt.Sprintf(
				"geometry count %d exceeds MaxBlasGeometryCount %d",
				geomCount, limits.MaxBlasGeometryCount,
			),
		}
	}

	return nil
}

// validateBuildGeometryDetails validates per-geometry build parameters.
func validateBuildGeometryDetails(entry *BlasBuildEntry) error {
	if entry.Geometries.AABBs != nil {
		return validateAABBBuildEntries(entry.Geometries.AABBs)
	}

	if entry.Geometries.Triangles != nil {
		return validateTriangleBuildEntries(entry.Geometries.Triangles)
	}

	return nil
}

// validateAABBBuildEntries validates AABB geometry build parameters.
func validateAABBBuildEntries(aabbs []hal.AccelerationStructureAABBs) error {
	for i := range aabbs {
		if aabbs[i].Stride < gputypes.AABBGeometryMinStride {
			return &ValidationError{
				Op: opBuildBlas,
				Message: fmt.Sprintf(
					"AABB geometry %d: stride %d < AABBGeometryMinStride %d",
					i, aabbs[i].Stride, gputypes.AABBGeometryMinStride,
				),
			}
		}
	}

	return nil
}

// validateTriangleBuildEntries validates triangle geometry build parameters.
func validateTriangleBuildEntries(triangles []hal.AccelerationStructureTriangles) error {
	for i := range triangles {
		if err := validateTriangleTransform(i, &triangles[i]); err != nil {
			return err
		}
	}

	return nil
}

// validateTriangleTransform checks transform buffer alignment.
func validateTriangleTransform(index int, tri *hal.AccelerationStructureTriangles) error {
	if tri.Transform == nil {
		return nil
	}

	offset := uint64(tri.Transform.Offset)
	if offset%gputypes.TransformBufferAlignment != 0 {
		return &ValidationError{
			Op: opBuildBlas,
			Message: fmt.Sprintf(
				"triangle geometry %d: transform buffer offset %d not aligned to %d",
				index, offset, gputypes.TransformBufferAlignment,
			),
		}
	}

	return nil
}

// validateTlasBuildInstances checks TLAS build instance count limits.
func validateTlasBuildInstances(ctx DeviceContext, entry *TlasBuildEntry) error {
	limits := ctx.DeviceLimits()

	if entry.InstanceCount > limits.MaxTlasInstanceCount {
		return &ValidationError{
			Op: opBuildTlas,
			Message: fmt.Sprintf(
				"instance count %d exceeds MaxTlasInstanceCount %d",
				entry.InstanceCount, limits.MaxTlasInstanceCount,
			),
		}
	}

	if entry.InstanceCount > entry.Tlas.MaxInstances {
		return &ValidationError{
			Op: opBuildTlas,
			Message: fmt.Sprintf(
				"instance count %d exceeds TLAS MaxInstances %d",
				entry.InstanceCount, entry.Tlas.MaxInstances,
			),
		}
	}

	return nil
}

// validateTlasBlasDependencies checks that all BLAS dependencies
// referenced by this TLAS have been built.
func validateTlasBlasDependencies(entry *TlasBuildEntry, blasMap map[uint64]*Blas) error {
	for addr, blas := range blasMap {
		if blas.BuiltIndex == 0 {
			return &ValidationError{
				Op: opBuildTlas,
				Message: fmt.Sprintf(
					"BLAS at address 0x%X referenced by TLAS %q is not built",
					addr, entry.Tlas.Label,
				),
			}
		}
	}

	return nil
}

// blasGeometrySizeCount returns the total geometry count from size descriptors.
func blasGeometrySizeCount(sizes *gputypes.BlasGeometrySizeDescriptors) uint32 {
	if sizes.Triangles != nil {
		return safeIntToUint32(len(sizes.Triangles))
	}

	if sizes.AABBs != nil {
		return safeIntToUint32(len(sizes.AABBs))
	}

	return 0
}
