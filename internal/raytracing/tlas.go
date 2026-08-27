package raytracing

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// Tlas represents a top-level acceleration structure.
//
// A TLAS contains instances that reference BLASes, forming the top of the
// two-level acceleration structure hierarchy used for ray tracing. It owns
// a HAL AccelerationStructure handle and a persistent GPU instance buffer.
//
// Matches Rust wgpu-core resource.rs Tlas struct (lines 3474-3486).
type Tlas struct {
	// Label is the debug label from the creation descriptor.
	Label string

	// Raw is the underlying HAL acceleration structure handle.
	// Nil if the TLAS is in an invalid state.
	Raw hal.AccelerationStructure

	// SizeInfo contains the build/update/structure sizes returned by
	// GetAccelerationStructureBuildSizes at creation time.
	SizeInfo hal.AccelerationStructureBuildSizes

	// Flags are the acceleration structure creation flags.
	Flags gputypes.AccelerationStructureFlags

	// UpdateMode determines whether this TLAS supports incremental updates.
	UpdateMode gputypes.AccelerationStructureUpdateMode

	// MaxInstances is the maximum number of BLAS instances this TLAS can
	// hold, specified at creation time. Determines the instance buffer size.
	// Matches Rust wgpu-core Tlas.max_instance_count.
	MaxInstances uint32

	// BuiltIndex is a monotonically increasing index assigned when this TLAS
	// is built. Used for ordering builds within a single command.
	// Zero means not yet built.
	// Matches Rust wgpu-core Tlas.built_index (RwLock<Option<NonZeroU64>>).
	BuiltIndex uint64

	// InstanceBuffer is the persistent GPU buffer holding packed TlasInstance
	// data (64 bytes per instance). Allocated at creation time and reused
	// across rebuilds.
	// Matches Rust wgpu-core TlasState.instance_buffer.
	InstanceBuffer hal.Buffer

	// Dependencies tracks the BLAS device addresses that this TLAS currently
	// references. Updated on each build to reflect the active instance set.
	// Used for validation: all referenced BLASes must be built before or in
	// the same build command as this TLAS.
	// Matches Rust wgpu-core Tlas.dependencies (RwLock<Vec<Arc<Blas>>>).
	Dependencies []uint64
}

// IsBuilt returns true if this TLAS has been built at least once.
func (t *Tlas) IsBuilt() bool {
	return t.BuiltIndex > 0
}

// AllowsUpdate returns true if this TLAS was created with the
// ASFlagAllowUpdate flag.
func (t *Tlas) AllowsUpdate() bool {
	return t.Flags.Contains(gputypes.ASFlagAllowUpdate)
}
