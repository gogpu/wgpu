package raytracing

import "github.com/gogpu/gputypes"

// RequestCompaction marks a BLAS for compaction query.
//
// Transitions: Idle -> Waiting. Returns an error if the BLAS is not in
// Idle state or if it was not created with ASFlagAllowCompaction.
//
// After this call, the caller should submit a compacted-size query to the
// GPU. When the query result is available, call SetCompactedSize.
//
// Matches Rust wgpu-core Blas::prepare_compact (resource.rs:3382-3403).
func RequestCompaction(blas *Blas) error {
	if err := checkCompactionAllowed(blas); err != nil {
		return err
	}

	if blas.Compaction != CompactionIdle {
		return &ValidationError{
			Op:      opCompact,
			Message: "BLAS is not in Idle state (current: " + blas.Compaction.String() + ")",
		}
	}

	blas.Compaction = CompactionWaiting

	return nil
}

// SetCompactedSize records the post-compaction size from GPU readback.
//
// Transitions: Waiting -> Ready. Returns an error if the BLAS is not in
// Waiting state. The size is typically read from a query buffer after GPU
// execution.
//
// Matches Rust wgpu-core Blas::on_pending_compact_resolve (resource.rs:3414-3451).
func SetCompactedSize(blas *Blas, size uint64) error {
	if blas.Compaction != CompactionWaiting {
		return &ValidationError{
			Op:      opCompact,
			Message: "BLAS is not in Waiting state (current: " + blas.Compaction.String() + ")",
		}
	}

	blas.CompactedSize = size
	blas.Compaction = CompactionReady

	return nil
}

// CompleteCompaction marks compaction as done after a copy-compact operation.
//
// Transitions: Ready -> Compacted. Returns an error if the BLAS is not in
// Ready state. After this call, no further compaction is possible.
//
// The caller is responsible for performing the actual copy-compact via the
// HAL before calling this function.
func CompleteCompaction(blas *Blas) error {
	if blas.Compaction != CompactionReady {
		return &ValidationError{
			Op:      opCompact,
			Message: "BLAS is not in Ready state (current: " + blas.Compaction.String() + ")",
		}
	}

	blas.Compaction = CompactionCompacted

	return nil
}

// CompactionSavings returns the memory saved by compaction in bytes.
//
// Returns 0 if the BLAS has not reached Ready or Compacted state, or if
// the compacted size is larger than the original (unlikely but possible
// for pathological geometry).
func CompactionSavings(blas *Blas) uint64 {
	if blas.Compaction != CompactionReady && blas.Compaction != CompactionCompacted {
		return 0
	}

	original := blas.SizeInfo.AccelerationStructureSize
	if blas.CompactedSize >= original {
		return 0
	}

	return original - blas.CompactedSize
}

// checkCompactionAllowed validates that a BLAS was created with the
// ASFlagAllowCompaction flag.
func checkCompactionAllowed(blas *Blas) error {
	if !blas.Flags.Contains(gputypes.ASFlagAllowCompaction) {
		return &ValidationError{
			Op:      opCompact,
			Message: "BLAS was not created with ASFlagAllowCompaction",
		}
	}

	return nil
}
