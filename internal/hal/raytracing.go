//go:build !(js && wasm)

package hal

import "github.com/gogpu/gputypes"

// AccelerationStructure represents a GPU acceleration structure (BLAS or TLAS).
// Matches Rust wgpu-hal's AccelerationStructure trait.
type AccelerationStructure interface {
	Resource
	NativeHandle
}

// AccelerationStructureFormat distinguishes BLAS from TLAS at the HAL level.
type AccelerationStructureFormat uint8

const (
	AccelerationStructureFormatTopLevel AccelerationStructureFormat = iota
	AccelerationStructureFormatBottomLevel
)

// AccelerationStructureBuildMode selects full build vs incremental update.
type AccelerationStructureBuildMode uint8

const (
	AccelerationStructureBuildModeBuild AccelerationStructureBuildMode = iota
	AccelerationStructureBuildModeUpdate
)

// AccelerationStructureUses is a bitflag for AS usage state transitions.
type AccelerationStructureUses uint8

const (
	AccelerationStructureUsesBuildInput  AccelerationStructureUses = 0x01
	AccelerationStructureUsesBuildOutput AccelerationStructureUses = 0x02
	AccelerationStructureUsesShaderInput AccelerationStructureUses = 0x04
	AccelerationStructureUsesQueryInput  AccelerationStructureUses = 0x08
	AccelerationStructureUsesCopySrc     AccelerationStructureUses = 0x10
	AccelerationStructureUsesCopyDst     AccelerationStructureUses = 0x20
)

// AccelerationStructureDescriptor describes an acceleration structure to create.
type AccelerationStructureDescriptor struct {
	Label           string
	Size            uint64
	Format          AccelerationStructureFormat
	AllowCompaction bool
}

// AccelerationStructureBuildSizes contains the sizes needed for AS build.
type AccelerationStructureBuildSizes struct {
	AccelerationStructureSize uint64
	UpdateScratchSize         uint64
	BuildScratchSize          uint64
}

// AccelerationStructureEntries is a discriminated union of build input geometry.
// Exactly one field must be set.
type AccelerationStructureEntries struct {
	Instances *AccelerationStructureInstances
	Triangles []AccelerationStructureTriangles
	AABBs     []AccelerationStructureAABBs
}

// AccelerationStructureTriangles describes triangle geometry for BLAS build.
type AccelerationStructureTriangles struct {
	VertexBuffer Buffer
	VertexFormat gputypes.VertexFormat
	FirstVertex  uint32
	VertexCount  uint32
	VertexStride uint64
	Indices      *AccelerationStructureTriangleIndices
	Transform    *AccelerationStructureTriangleTransform
	Flags        gputypes.AccelerationStructureGeometryFlags
}

// AccelerationStructureAABBs describes AABB geometry for BLAS build.
type AccelerationStructureAABBs struct {
	Buffer Buffer
	Offset uint32
	Count  uint32
	Stride uint64
	Flags  gputypes.AccelerationStructureGeometryFlags
}

// AccelerationStructureInstances describes instance data for TLAS build.
type AccelerationStructureInstances struct {
	Buffer Buffer
	Offset uint32
	Count  uint32
}

// AccelerationStructureTriangleIndices describes index data for triangle geometry.
type AccelerationStructureTriangleIndices struct {
	Format gputypes.IndexFormat
	Buffer Buffer
	Offset uint32
	Count  uint32
}

// AccelerationStructureTriangleTransform describes transform data for triangle geometry.
type AccelerationStructureTriangleTransform struct {
	Buffer Buffer
	Offset uint32
}

// BuildAccelerationStructureDescriptor describes a single AS build operation.
type BuildAccelerationStructureDescriptor struct {
	Entries                          *AccelerationStructureEntries
	Mode                             AccelerationStructureBuildMode
	Flags                            gputypes.AccelerationStructureFlags
	SourceAccelerationStructure      AccelerationStructure
	DestinationAccelerationStructure AccelerationStructure
	ScratchBuffer                    Buffer
	ScratchBufferOffset              uint64
}

// GetAccelerationStructureBuildSizesDescriptor describes a build size query.
type GetAccelerationStructureBuildSizesDescriptor struct {
	Entries *AccelerationStructureEntries
	Flags   gputypes.AccelerationStructureFlags
}

// AccelerationStructureBarrier defines an AS state transition.
type AccelerationStructureBarrier struct {
	Usage AccelerationStructureUsageTransition
}

// AccelerationStructureUsageTransition defines an AS usage state transition.
type AccelerationStructureUsageTransition struct {
	OldUsage AccelerationStructureUses
	NewUsage AccelerationStructureUses
}

// TlasInstance describes a single TLAS instance referencing a BLAS.
// This is the CPU-side representation; TlasInstanceToBytes converts it
// to the backend-specific 64-byte packed format.
type TlasInstance struct {
	Transform                      [12]float32
	CustomData                     uint32
	Mask                           uint8
	BlasAddress                    uint64
	ShaderBindingTableRecordOffset uint32
}
