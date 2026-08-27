package raytracing

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// --- RequestCompaction tests ---

func TestRequestCompactionHappyPath(t *testing.T) {
	blas := &Blas{
		Flags:      gputypes.ASFlagAllowCompaction,
		Compaction: CompactionIdle,
	}

	if err := RequestCompaction(blas); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if blas.Compaction != CompactionWaiting {
		t.Errorf("state = %v, want Waiting", blas.Compaction)
	}
}

func TestRequestCompactionNoFlag(t *testing.T) {
	blas := &Blas{
		Flags:      gputypes.ASFlagPreferFastTrace,
		Compaction: CompactionIdle,
	}

	err := RequestCompaction(blas)
	if err == nil {
		t.Fatal("expected error for missing ASFlagAllowCompaction")
	}

	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *ValidationError, got %T", err)
	}

	if ve.Op != opCompact {
		t.Errorf("Op = %q, want %q", ve.Op, opCompact)
	}
}

func TestRequestCompactionWrongState(t *testing.T) {
	states := []CompactionState{
		CompactionWaiting,
		CompactionReady,
		CompactionCompacted,
	}

	for _, s := range states {
		blas := &Blas{
			Flags:      gputypes.ASFlagAllowCompaction,
			Compaction: s,
		}

		err := RequestCompaction(blas)
		if err == nil {
			t.Errorf("expected error for state %v", s)
		}
	}
}

// --- SetCompactedSize tests ---

func TestSetCompactedSizeHappyPath(t *testing.T) {
	blas := &Blas{Compaction: CompactionWaiting}

	if err := SetCompactedSize(blas, 2048); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if blas.Compaction != CompactionReady {
		t.Errorf("state = %v, want Ready", blas.Compaction)
	}

	if blas.CompactedSize != 2048 {
		t.Errorf("CompactedSize = %d, want 2048", blas.CompactedSize)
	}
}

func TestSetCompactedSizeWrongState(t *testing.T) {
	states := []CompactionState{
		CompactionIdle,
		CompactionReady,
		CompactionCompacted,
	}

	for _, s := range states {
		blas := &Blas{Compaction: s}

		err := SetCompactedSize(blas, 1024)
		if err == nil {
			t.Errorf("expected error for state %v", s)
		}
	}
}

// --- CompleteCompaction tests ---

func TestCompleteCompactionHappyPath(t *testing.T) {
	blas := &Blas{Compaction: CompactionReady}

	if err := CompleteCompaction(blas); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if blas.Compaction != CompactionCompacted {
		t.Errorf("state = %v, want Compacted", blas.Compaction)
	}
}

func TestCompleteCompactionWrongState(t *testing.T) {
	states := []CompactionState{
		CompactionIdle,
		CompactionWaiting,
		CompactionCompacted,
	}

	for _, s := range states {
		blas := &Blas{Compaction: s}

		err := CompleteCompaction(blas)
		if err == nil {
			t.Errorf("expected error for state %v", s)
		}
	}
}

// --- CompactionSavings tests ---

func TestCompactionSavingsReady(t *testing.T) {
	blas := &Blas{
		SizeInfo:      hal.AccelerationStructureBuildSizes{AccelerationStructureSize: 4096},
		Compaction:    CompactionReady,
		CompactedSize: 2048,
	}

	savings := CompactionSavings(blas)
	if savings != 2048 {
		t.Errorf("savings = %d, want 2048", savings)
	}
}

func TestCompactionSavingsCompacted(t *testing.T) {
	blas := &Blas{
		SizeInfo:      hal.AccelerationStructureBuildSizes{AccelerationStructureSize: 4096},
		Compaction:    CompactionCompacted,
		CompactedSize: 3000,
	}

	savings := CompactionSavings(blas)
	if savings != 1096 {
		t.Errorf("savings = %d, want 1096", savings)
	}
}

func TestCompactionSavingsNoSavings(t *testing.T) {
	blas := &Blas{
		SizeInfo:      hal.AccelerationStructureBuildSizes{AccelerationStructureSize: 2048},
		Compaction:    CompactionReady,
		CompactedSize: 2048,
	}

	savings := CompactionSavings(blas)
	if savings != 0 {
		t.Errorf("savings = %d, want 0 (no savings)", savings)
	}
}

func TestCompactionSavingsLargerCompacted(t *testing.T) {
	// Edge case: compacted size >= original (pathological geometry).
	blas := &Blas{
		SizeInfo:      hal.AccelerationStructureBuildSizes{AccelerationStructureSize: 2048},
		Compaction:    CompactionReady,
		CompactedSize: 3000,
	}

	savings := CompactionSavings(blas)
	if savings != 0 {
		t.Errorf("savings = %d, want 0 (compacted larger)", savings)
	}
}

func TestCompactionSavingsWrongState(t *testing.T) {
	blas := &Blas{
		SizeInfo:      hal.AccelerationStructureBuildSizes{AccelerationStructureSize: 4096},
		Compaction:    CompactionIdle,
		CompactedSize: 2048,
	}

	savings := CompactionSavings(blas)
	if savings != 0 {
		t.Errorf("savings = %d, want 0 (Idle state)", savings)
	}

	blas.Compaction = CompactionWaiting
	savings = CompactionSavings(blas)
	if savings != 0 {
		t.Errorf("savings = %d, want 0 (Waiting state)", savings)
	}
}

// --- Full lifecycle test ---

func TestCompactionFullLifecycle(t *testing.T) {
	blas := &Blas{
		Flags:    gputypes.ASFlagAllowCompaction,
		SizeInfo: hal.AccelerationStructureBuildSizes{AccelerationStructureSize: 8192},
	}

	// Idle -> Waiting.
	if err := RequestCompaction(blas); err != nil {
		t.Fatalf("RequestCompaction: %v", err)
	}

	if blas.Compaction != CompactionWaiting {
		t.Fatalf("expected Waiting, got %v", blas.Compaction)
	}

	// Waiting -> Ready.
	if err := SetCompactedSize(blas, 5000); err != nil {
		t.Fatalf("SetCompactedSize: %v", err)
	}

	if blas.Compaction != CompactionReady {
		t.Fatalf("expected Ready, got %v", blas.Compaction)
	}

	savings := CompactionSavings(blas)
	if savings != 3192 {
		t.Errorf("savings = %d, want 3192", savings)
	}

	// Ready -> Compacted.
	if err := CompleteCompaction(blas); err != nil {
		t.Fatalf("CompleteCompaction: %v", err)
	}

	if blas.Compaction != CompactionCompacted {
		t.Fatalf("expected Compacted, got %v", blas.Compaction)
	}

	// Cannot request compaction again.
	err := RequestCompaction(blas)
	if err == nil {
		t.Fatal("expected error: double compaction")
	}
}
