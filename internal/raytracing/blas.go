package raytracing

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// Blas represents a bottom-level acceleration structure.
//
// A BLAS contains geometry data (triangles or AABBs) and is referenced by
// TLAS instances. It owns a HAL AccelerationStructure handle and tracks
// compaction state for memory optimization.
//
// Matches Rust wgpu-core resource.rs Blas struct (lines 3238-3252).
type Blas struct {
	// Label is the debug label from the creation descriptor.
	Label string

	// Raw is the underlying HAL acceleration structure handle.
	// Nil if the BLAS is in an invalid state (e.g., creation failed).
	Raw hal.AccelerationStructure

	// SizeInfo contains the build/update/structure sizes returned by
	// GetAccelerationStructureBuildSizes at creation time.
	SizeInfo hal.AccelerationStructureBuildSizes

	// Flags are the acceleration structure creation flags.
	Flags gputypes.AccelerationStructureFlags

	// UpdateMode determines whether this BLAS supports incremental updates.
	UpdateMode gputypes.AccelerationStructureUpdateMode

	// Compaction tracks the compaction lifecycle state.
	// Matches Rust wgpu-core Blas.compacted_state (Mutex<BlasCompactState>).
	Compaction CompactionState

	// CompactedSize holds the compacted size in bytes, valid only when
	// Compaction == CompactionReady.
	CompactedSize uint64

	// BuiltIndex is a monotonically increasing index assigned when this BLAS
	// is built. Used for dependency ordering: a TLAS can only reference BLASes
	// that were built before or in the same build command.
	// Zero means not yet built.
	// Matches Rust wgpu-core Blas.built_index (RwLock<Option<NonZeroU64>>).
	BuiltIndex uint64

	// GeometryCount is the number of geometries in this BLAS, stored at
	// creation time for validation during build commands.
	GeometryCount uint32

	// Handle is the device address of this BLAS, used by TLAS instances
	// to reference this structure. Set after the first successful build.
	// Matches Rust wgpu-core Blas.handle (u64).
	Handle uint64
}

// IsBuilt returns true if this BLAS has been built at least once.
func (b *Blas) IsBuilt() bool {
	return b.BuiltIndex > 0
}

// AllowsCompaction returns true if this BLAS was created with the
// ASFlagAllowCompaction flag.
func (b *Blas) AllowsCompaction() bool {
	return b.Flags.Contains(gputypes.ASFlagAllowCompaction)
}

// AllowsUpdate returns true if this BLAS was created with the
// ASFlagAllowUpdate flag.
func (b *Blas) AllowsUpdate() bool {
	return b.Flags.Contains(gputypes.ASFlagAllowUpdate)
}
