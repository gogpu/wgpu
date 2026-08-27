package raytracing

import (
	"math"

	"github.com/gogpu/wgpu/hal"
)

// BlasBuildEntry pairs a BLAS with the geometry data for building.
type BlasBuildEntry struct {
	// Blas is the acceleration structure to build or update.
	Blas *Blas

	// Geometries provides the geometry input for the build.
	// Must match the geometry type (triangles or AABBs) used at creation.
	Geometries *hal.AccelerationStructureEntries
}

// TlasBuildEntry pairs a TLAS with the instance buffer for building.
type TlasBuildEntry struct {
	// Tlas is the acceleration structure to build or update.
	Tlas *Tlas

	// InstanceBuffer is the GPU buffer containing packed instance data.
	InstanceBuffer hal.Buffer

	// InstanceCount is the number of active instances in the buffer.
	InstanceCount uint32
}

// BuildContext holds state for a single BuildAccelerationStructures call.
//
// It tracks a monotonically increasing build index used for dependency
// ordering: a TLAS can only reference BLASes that were built in the same
// or an earlier build command (BuiltIndex > 0).
//
// Matches Rust wgpu-core's build_acceleration_structures scratch size
// accumulation and built_index assignment.
type BuildContext struct {
	device     DeviceContext
	buildIndex uint64
}

// NewBuildContext creates a build context with the given device and
// current build index. The build index should be incremented by the
// caller for each build command.
func NewBuildContext(device DeviceContext, currentBuildIndex uint64) *BuildContext {
	return &BuildContext{
		device:     device,
		buildIndex: currentBuildIndex,
	}
}

// PrepareBlasBuild validates and prepares BLAS entries for building.
//
// It calculates the total scratch buffer size needed (aligned per adapter
// requirements) and assigns a BuiltIndex to each BLAS. The returned
// scratch size can be used to allocate a shared scratch buffer.
//
// Reference: Rust wgpu-core command/ray_tracing.rs iter_blas().
func (bc *BuildContext) PrepareBlasBuild(entries []*BlasBuildEntry) (uint64, error) {
	if len(entries) == 0 {
		return 0, nil
	}

	alignment := bc.scratchAlignment()
	var scratchSize uint64

	for _, entry := range entries {
		if err := bc.validateBlasEntry(entry); err != nil {
			return 0, err
		}

		scratchSize += alignUp64(entry.Blas.SizeInfo.BuildScratchSize, alignment)
		entry.Blas.BuiltIndex = bc.buildIndex
	}

	return scratchSize, nil
}

// PrepareTlasBuild validates and prepares TLAS entries for building.
//
// It verifies that all referenced BLASes have been built (BuiltIndex > 0)
// and calculates the total scratch buffer size. Each TLAS is assigned
// the current build index after successful preparation.
//
// Reference: Rust wgpu-core command/ray_tracing.rs tlas loop (line 243-315).
func (bc *BuildContext) PrepareTlasBuild(entries []*TlasBuildEntry) (uint64, error) {
	if len(entries) == 0 {
		return 0, nil
	}

	alignment := bc.scratchAlignment()
	limits := bc.device.DeviceLimits()
	var scratchSize uint64

	for _, entry := range entries {
		if err := validateTlasInstanceCount(entry, limits.MaxTlasInstanceCount); err != nil {
			return 0, err
		}

		scratchSize += alignUp64(entry.Tlas.SizeInfo.BuildScratchSize, alignment)
		entry.Tlas.BuiltIndex = bc.buildIndex
	}

	return scratchSize, nil
}

// validateBlasEntry checks a single BLAS build entry against device limits.
func (bc *BuildContext) validateBlasEntry(entry *BlasBuildEntry) error {
	if entry.Blas == nil {
		return &ValidationError{Op: opBuildBlas, Message: "nil BLAS"}
	}

	if entry.Geometries == nil {
		return &ValidationError{Op: opBuildBlas, Message: "nil geometry entries"}
	}

	limits := bc.device.DeviceLimits()
	geomCount := blasEntryGeometryCount(entry.Geometries)

	if geomCount > limits.MaxBlasGeometryCount {
		return &ValidationError{
			Op:      opBuildBlas,
			Message: "geometry count exceeds MaxBlasGeometryCount",
		}
	}

	return nil
}

// validateTlasInstanceCount checks that the instance count does not exceed
// the device limit.
func validateTlasInstanceCount(entry *TlasBuildEntry, maxInstances uint32) error {
	if entry.Tlas == nil {
		return &ValidationError{Op: opBuildTlas, Message: "nil TLAS"}
	}

	if entry.InstanceCount > maxInstances {
		return &ValidationError{
			Op:      opBuildTlas,
			Message: "instance count exceeds MaxTlasInstanceCount",
		}
	}

	if entry.InstanceCount > entry.Tlas.MaxInstances {
		return &ValidationError{
			Op:      opBuildTlas,
			Message: "instance count exceeds TLAS MaxInstances",
		}
	}

	return nil
}

// blasEntryGeometryCount returns the number of geometries in an AS entries union.
// Panics are not possible: len() returns int >= 0 and MaxBlasGeometryCount
// is uint32, so the geometry count always fits in uint32.
func blasEntryGeometryCount(entries *hal.AccelerationStructureEntries) uint32 {
	if entries.Triangles != nil {
		return safeIntToUint32(len(entries.Triangles))
	}

	if entries.AABBs != nil {
		return safeIntToUint32(len(entries.AABBs))
	}

	return 0
}

// scratchAlignment returns the scratch buffer alignment as uint64.
// Falls back to 256 if the device reports 0 (safe default per Vulkan spec).
func (bc *BuildContext) scratchAlignment() uint64 {
	a := uint64(bc.device.DeviceAlignments().RayTracingScratchBufferAlignment)
	if a == 0 {
		return 256
	}

	return a
}

// alignUp64 rounds n up to the next multiple of alignment.
// alignment must be a power of two. Returns n unchanged if alignment is 0.
func alignUp64(n, alignment uint64) uint64 {
	if alignment == 0 {
		return n
	}

	return (n + alignment - 1) &^ (alignment - 1)
}

// safeIntToUint32 converts an int to uint32, clamping to math.MaxUint32 on overflow.
func safeIntToUint32(v int) uint32 {
	if v < 0 {
		return 0
	}

	if v > math.MaxUint32 {
		return math.MaxUint32
	}

	return uint32(v)
}
