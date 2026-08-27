package raytracing

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// --- CompactionState tests ---

func TestCompactionStateString(t *testing.T) {
	tests := []struct {
		state CompactionState
		want  string
	}{
		{CompactionIdle, "Idle"},
		{CompactionWaiting, "Waiting"},
		{CompactionReady, "Ready"},
		{CompactionCompacted, "Compacted"},
		{CompactionState(99), "Unknown"},
	}
	for _, tt := range tests {
		if got := tt.state.String(); got != tt.want {
			t.Errorf("CompactionState(%d).String() = %q, want %q", tt.state, got, tt.want)
		}
	}
}

func TestCompactionStateValues(t *testing.T) {
	// Verify iota ordering matches the expected lifecycle.
	if CompactionIdle != 0 {
		t.Errorf("CompactionIdle = %d, want 0", CompactionIdle)
	}
	if CompactionWaiting != 1 {
		t.Errorf("CompactionWaiting = %d, want 1", CompactionWaiting)
	}
	if CompactionReady != 2 {
		t.Errorf("CompactionReady = %d, want 2", CompactionReady)
	}
	if CompactionCompacted != 3 {
		t.Errorf("CompactionCompacted = %d, want 3", CompactionCompacted)
	}
}

// --- BlasAction tests ---

func TestBlasActionString(t *testing.T) {
	tests := []struct {
		action BlasAction
		want   string
	}{
		{BlasActionNone, "None"},
		{BlasActionBuild, "Build"},
		{BlasActionUpdate, "Update"},
		{BlasAction(99), "Unknown"},
	}
	for _, tt := range tests {
		if got := tt.action.String(); got != tt.want {
			t.Errorf("BlasAction(%d).String() = %q, want %q", tt.action, got, tt.want)
		}
	}
}

func TestBlasActionValues(t *testing.T) {
	if BlasActionNone != 0 {
		t.Errorf("BlasActionNone = %d, want 0", BlasActionNone)
	}
	if BlasActionBuild != 1 {
		t.Errorf("BlasActionBuild = %d, want 1", BlasActionBuild)
	}
	if BlasActionUpdate != 2 {
		t.Errorf("BlasActionUpdate = %d, want 2", BlasActionUpdate)
	}
}

// --- Blas tests ---

func TestBlasZeroValue(t *testing.T) {
	var b Blas
	if b.IsBuilt() {
		t.Error("zero-value Blas should not be built")
	}
	if b.AllowsCompaction() {
		t.Error("zero-value Blas should not allow compaction")
	}
	if b.AllowsUpdate() {
		t.Error("zero-value Blas should not allow update")
	}
	if b.Compaction != CompactionIdle {
		t.Errorf("zero-value Blas.Compaction = %v, want CompactionIdle", b.Compaction)
	}
}

func TestBlasIsBuilt(t *testing.T) {
	b := Blas{BuiltIndex: 0}
	if b.IsBuilt() {
		t.Error("BuiltIndex=0 should not be built")
	}
	b.BuiltIndex = 1
	if !b.IsBuilt() {
		t.Error("BuiltIndex=1 should be built")
	}
	b.BuiltIndex = 42
	if !b.IsBuilt() {
		t.Error("BuiltIndex=42 should be built")
	}
}

func TestBlasAllowsCompaction(t *testing.T) {
	b := Blas{Flags: gputypes.ASFlagAllowCompaction}
	if !b.AllowsCompaction() {
		t.Error("BLAS with ASFlagAllowCompaction should allow compaction")
	}

	b.Flags = gputypes.ASFlagPreferFastTrace
	if b.AllowsCompaction() {
		t.Error("BLAS without ASFlagAllowCompaction should not allow compaction")
	}

	// Multiple flags including compaction.
	b.Flags = gputypes.ASFlagAllowCompaction | gputypes.ASFlagPreferFastTrace
	if !b.AllowsCompaction() {
		t.Error("BLAS with combined flags including ASFlagAllowCompaction should allow compaction")
	}
}

func TestBlasAllowsUpdate(t *testing.T) {
	b := Blas{Flags: gputypes.ASFlagAllowUpdate}
	if !b.AllowsUpdate() {
		t.Error("BLAS with ASFlagAllowUpdate should allow update")
	}

	b.Flags = 0
	if b.AllowsUpdate() {
		t.Error("BLAS without ASFlagAllowUpdate should not allow update")
	}
}

func TestBlasCreation(t *testing.T) {
	b := Blas{
		Label:         "test-blas",
		SizeInfo:      hal.AccelerationStructureBuildSizes{AccelerationStructureSize: 4096, BuildScratchSize: 2048, UpdateScratchSize: 1024},
		Flags:         gputypes.ASFlagAllowCompaction | gputypes.ASFlagAllowUpdate,
		UpdateMode:    gputypes.AccelerationStructureUpdateModePreferUpdate,
		GeometryCount: 3,
	}

	if b.Label != "test-blas" {
		t.Errorf("Label = %q, want %q", b.Label, "test-blas")
	}
	if b.SizeInfo.AccelerationStructureSize != 4096 {
		t.Errorf("AccelerationStructureSize = %d, want 4096", b.SizeInfo.AccelerationStructureSize)
	}
	if b.SizeInfo.BuildScratchSize != 2048 {
		t.Errorf("BuildScratchSize = %d, want 2048", b.SizeInfo.BuildScratchSize)
	}
	if b.SizeInfo.UpdateScratchSize != 1024 {
		t.Errorf("UpdateScratchSize = %d, want 1024", b.SizeInfo.UpdateScratchSize)
	}
	if b.GeometryCount != 3 {
		t.Errorf("GeometryCount = %d, want 3", b.GeometryCount)
	}
	if b.UpdateMode != gputypes.AccelerationStructureUpdateModePreferUpdate {
		t.Errorf("UpdateMode = %d, want PreferUpdate", b.UpdateMode)
	}
	if !b.AllowsCompaction() {
		t.Error("should allow compaction")
	}
	if !b.AllowsUpdate() {
		t.Error("should allow update")
	}
}

// --- Tlas tests ---

func TestTlasZeroValue(t *testing.T) {
	var tl Tlas
	if tl.IsBuilt() {
		t.Error("zero-value Tlas should not be built")
	}
	if tl.AllowsUpdate() {
		t.Error("zero-value Tlas should not allow update")
	}
	if tl.Dependencies != nil {
		t.Error("zero-value Tlas.Dependencies should be nil")
	}
}

func TestTlasIsBuilt(t *testing.T) {
	tl := Tlas{BuiltIndex: 0}
	if tl.IsBuilt() {
		t.Error("BuiltIndex=0 should not be built")
	}
	tl.BuiltIndex = 1
	if !tl.IsBuilt() {
		t.Error("BuiltIndex=1 should be built")
	}
}

func TestTlasAllowsUpdate(t *testing.T) {
	tl := Tlas{Flags: gputypes.ASFlagAllowUpdate}
	if !tl.AllowsUpdate() {
		t.Error("TLAS with ASFlagAllowUpdate should allow update")
	}

	tl.Flags = 0
	if tl.AllowsUpdate() {
		t.Error("TLAS without ASFlagAllowUpdate should not allow update")
	}
}

func TestTlasCreation(t *testing.T) {
	tl := Tlas{
		Label:        "test-tlas",
		SizeInfo:     hal.AccelerationStructureBuildSizes{AccelerationStructureSize: 8192, BuildScratchSize: 4096, UpdateScratchSize: 2048},
		Flags:        gputypes.ASFlagPreferFastTrace,
		UpdateMode:   gputypes.AccelerationStructureUpdateModeBuild,
		MaxInstances: 100,
		Dependencies: make([]uint64, 0, 8),
	}

	if tl.Label != "test-tlas" {
		t.Errorf("Label = %q, want %q", tl.Label, "test-tlas")
	}
	if tl.MaxInstances != 100 {
		t.Errorf("MaxInstances = %d, want 100", tl.MaxInstances)
	}
	if tl.SizeInfo.AccelerationStructureSize != 8192 {
		t.Errorf("AccelerationStructureSize = %d, want 8192", tl.SizeInfo.AccelerationStructureSize)
	}
	if tl.AllowsUpdate() {
		t.Error("TLAS with only PreferFastTrace should not allow update")
	}
	if len(tl.Dependencies) != 0 {
		t.Errorf("Dependencies len = %d, want 0", len(tl.Dependencies))
	}
	if cap(tl.Dependencies) != 8 {
		t.Errorf("Dependencies cap = %d, want 8", cap(tl.Dependencies))
	}
}

func TestTlasDependencies(t *testing.T) {
	tl := Tlas{
		Dependencies: make([]uint64, 0, 4),
	}

	// Simulate adding BLAS addresses.
	addresses := []uint64{0xDEAD_BEEF, 0xCAFE_BABE, 0x1234_5678}
	tl.Dependencies = append(tl.Dependencies, addresses...)

	if len(tl.Dependencies) != 3 {
		t.Fatalf("Dependencies len = %d, want 3", len(tl.Dependencies))
	}
	for i, want := range addresses {
		if tl.Dependencies[i] != want {
			t.Errorf("Dependencies[%d] = 0x%X, want 0x%X", i, tl.Dependencies[i], want)
		}
	}
}

// --- CompactionState lifecycle test ---

func TestCompactionLifecycle(t *testing.T) {
	b := Blas{
		Flags:      gputypes.ASFlagAllowCompaction,
		Compaction: CompactionIdle,
	}

	// Idle -> Waiting (compact size query submitted).
	b.Compaction = CompactionWaiting
	if b.Compaction != CompactionWaiting {
		t.Errorf("expected Waiting, got %v", b.Compaction)
	}

	// Waiting -> Ready (size available).
	b.Compaction = CompactionReady
	b.CompactedSize = 2048
	if b.Compaction != CompactionReady {
		t.Errorf("expected Ready, got %v", b.Compaction)
	}
	if b.CompactedSize != 2048 {
		t.Errorf("CompactedSize = %d, want 2048", b.CompactedSize)
	}

	// Ready -> Compacted (copy complete).
	b.Compaction = CompactionCompacted
	if b.Compaction != CompactionCompacted {
		t.Errorf("expected Compacted, got %v", b.Compaction)
	}
}
