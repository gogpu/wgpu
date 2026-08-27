package raytracing

// CompactionState tracks the compaction lifecycle of a BLAS.
//
// Transitions: Idle -> Waiting -> Ready -> Compacted.
//
// Matches Rust wgpu-core BlasCompactState (resource.rs:3216-3225).
type CompactionState uint8

const (
	// CompactionIdle means the BLAS has not been compacted and no compaction
	// has been requested. This is the initial state.
	CompactionIdle CompactionState = iota

	// CompactionWaiting means a compacted-size query has been submitted to
	// the GPU but the result is not yet available.
	CompactionWaiting

	// CompactionReady means the compacted size is available and the BLAS is
	// ready to be compacted via a copy operation.
	CompactionReady

	// CompactionCompacted means the BLAS has been compacted. No further
	// compaction is allowed (double-compaction is an error).
	CompactionCompacted
)

// String returns a human-readable name for the compaction state.
func (s CompactionState) String() string {
	switch s {
	case CompactionIdle:
		return "Idle"
	case CompactionWaiting:
		return "Waiting"
	case CompactionReady:
		return "Ready"
	case CompactionCompacted:
		return "Compacted"
	default:
		return "Unknown"
	}
}

// BlasAction describes what needs to happen to a BLAS during a build command.
type BlasAction uint8

const (
	// BlasActionNone means no build action is required.
	BlasActionNone BlasAction = iota

	// BlasActionBuild means a full acceleration structure build is required.
	BlasActionBuild

	// BlasActionUpdate means an incremental update is sufficient (the BLAS
	// must have been previously built with ASFlagAllowUpdate).
	BlasActionUpdate
)

// String returns a human-readable name for the BLAS action.
func (a BlasAction) String() string {
	switch a {
	case BlasActionNone:
		return "None"
	case BlasActionBuild:
		return "Build"
	case BlasActionUpdate:
		return "Update"
	default:
		return "Unknown"
	}
}
