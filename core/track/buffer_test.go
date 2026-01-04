package track

import (
	"errors"
	"testing"

	"github.com/gogpu/wgpu/types"
)

func TestBufferUses_IsReadOnly(t *testing.T) {
	tests := []struct {
		name string
		uses BufferUses
		want bool
	}{
		{"none is read-only", BufferUsesNone, true},
		{"copy src is read-only", BufferUsesCopySrc, true},
		{"index is read-only", BufferUsesIndex, true},
		{"vertex is read-only", BufferUsesVertex, true},
		{"uniform is read-only", BufferUsesUniform, true},
		{"storage read is read-only", BufferUsesStorageRead, true},
		{"indirect is read-only", BufferUsesIndirect, true},
		{"map read is read-only", BufferUsesMapRead, true},
		{"copy dst is write", BufferUsesCopyDst, false},
		{"storage write is write", BufferUsesStorageWrite, false},
		{"map write is write", BufferUsesMapWrite, false},
		{"query resolve is write", BufferUsesQueryResolve, false},
		{"combined read-only", BufferUsesCopySrc | BufferUsesVertex, true},
		{"read + write", BufferUsesCopySrc | BufferUsesCopyDst, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.uses.IsReadOnly(); got != tt.want {
				t.Errorf("BufferUses(%d).IsReadOnly() = %v, want %v", tt.uses, got, tt.want)
			}
		})
	}
}

func TestBufferUses_IsEmpty(t *testing.T) {
	if !BufferUsesNone.IsEmpty() {
		t.Error("BufferUsesNone should be empty")
	}
	if BufferUsesCopySrc.IsEmpty() {
		t.Error("BufferUsesCopySrc should not be empty")
	}
}

func TestBufferUses_Contains(t *testing.T) {
	combined := BufferUsesCopySrc | BufferUsesVertex | BufferUsesUniform

	if !combined.Contains(BufferUsesCopySrc) {
		t.Error("Combined should contain CopySrc")
	}
	if !combined.Contains(BufferUsesVertex) {
		t.Error("Combined should contain Vertex")
	}
	if !combined.Contains(BufferUsesCopySrc | BufferUsesVertex) {
		t.Error("Combined should contain CopySrc|Vertex")
	}
	if combined.Contains(BufferUsesCopyDst) {
		t.Error("Combined should not contain CopyDst")
	}
}

func TestBufferUses_IsCompatible(t *testing.T) {
	tests := []struct {
		name string
		a    BufferUses
		b    BufferUses
		want bool
	}{
		{"empty with empty", BufferUsesNone, BufferUsesNone, true},
		{"empty with read", BufferUsesNone, BufferUsesCopySrc, true},
		{"empty with write", BufferUsesNone, BufferUsesCopyDst, true},
		{"read with read", BufferUsesCopySrc, BufferUsesVertex, true},
		{"read with same read", BufferUsesVertex, BufferUsesVertex, true},
		{"write with same write", BufferUsesCopyDst, BufferUsesCopyDst, true},
		{"write with different write", BufferUsesCopyDst, BufferUsesStorageWrite, false},
		{"read with write", BufferUsesCopySrc, BufferUsesCopyDst, false},
		{"write with read", BufferUsesCopyDst, BufferUsesVertex, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.a.IsCompatible(tt.b); got != tt.want {
				t.Errorf("BufferUses(%d).IsCompatible(%d) = %v, want %v",
					tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestBufferUses_ToBufferUsage(t *testing.T) {
	tests := []struct {
		name string
		uses BufferUses
		want types.BufferUsage
	}{
		{"none", BufferUsesNone, 0},
		{"copy src", BufferUsesCopySrc, types.BufferUsageCopySrc},
		{"copy dst", BufferUsesCopyDst, types.BufferUsageCopyDst},
		{"index", BufferUsesIndex, types.BufferUsageIndex},
		{"vertex", BufferUsesVertex, types.BufferUsageVertex},
		{"uniform", BufferUsesUniform, types.BufferUsageUniform},
		{"storage read", BufferUsesStorageRead, types.BufferUsageStorage},
		{"storage write", BufferUsesStorageWrite, types.BufferUsageStorage},
		{"indirect", BufferUsesIndirect, types.BufferUsageIndirect},
		{"map read", BufferUsesMapRead, types.BufferUsageMapRead},
		{"map write", BufferUsesMapWrite, types.BufferUsageMapWrite},
		{"query resolve", BufferUsesQueryResolve, types.BufferUsageQueryResolve},
		{
			"combined",
			BufferUsesCopySrc | BufferUsesVertex | BufferUsesUniform,
			types.BufferUsageCopySrc | types.BufferUsageVertex | types.BufferUsageUniform,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.uses.ToBufferUsage(); got != tt.want {
				t.Errorf("BufferUses(%d).ToBufferUsage() = %d, want %d",
					tt.uses, got, tt.want)
			}
		})
	}
}

func TestBufferTracker_InsertSingle(t *testing.T) {
	tracker := NewBufferTracker()

	tracker.InsertSingle(TrackerIndex(0), BufferUsesVertex)
	tracker.InsertSingle(TrackerIndex(5), BufferUsesCopySrc)

	if tracker.GetUsage(TrackerIndex(0)) != BufferUsesVertex {
		t.Error("Index 0 should have Vertex usage")
	}
	if tracker.GetUsage(TrackerIndex(5)) != BufferUsesCopySrc {
		t.Error("Index 5 should have CopySrc usage")
	}
	if tracker.Size() != 2 {
		t.Errorf("Size = %d, want 2", tracker.Size())
	}
}

func TestBufferTracker_Remove(t *testing.T) {
	tracker := NewBufferTracker()

	tracker.InsertSingle(TrackerIndex(0), BufferUsesVertex)
	tracker.InsertSingle(TrackerIndex(1), BufferUsesCopySrc)

	if tracker.Size() != 2 {
		t.Errorf("Initial size = %d, want 2", tracker.Size())
	}

	tracker.Remove(TrackerIndex(0))

	if tracker.IsTracked(TrackerIndex(0)) {
		t.Error("Index 0 should not be tracked after remove")
	}
	if !tracker.IsTracked(TrackerIndex(1)) {
		t.Error("Index 1 should still be tracked")
	}
	if tracker.Size() != 1 {
		t.Errorf("Size after remove = %d, want 1", tracker.Size())
	}

	// Remove non-existent should be safe
	tracker.Remove(TrackerIndex(100))
}

func TestBufferTracker_GetUsage(t *testing.T) {
	tracker := NewBufferTracker()

	// Untracked buffer returns None
	if tracker.GetUsage(TrackerIndex(0)) != BufferUsesNone {
		t.Error("Untracked buffer should return None")
	}

	tracker.InsertSingle(TrackerIndex(0), BufferUsesVertex)
	if tracker.GetUsage(TrackerIndex(0)) != BufferUsesVertex {
		t.Error("Tracked buffer should return its usage")
	}
}

func TestBufferTracker_SetUsage(t *testing.T) {
	tracker := NewBufferTracker()

	tracker.InsertSingle(TrackerIndex(0), BufferUsesVertex)
	tracker.SetUsage(TrackerIndex(0), BufferUsesCopySrc)

	if tracker.GetUsage(TrackerIndex(0)) != BufferUsesCopySrc {
		t.Error("Usage should be updated")
	}

	// SetUsage on untracked buffer should be no-op
	tracker.SetUsage(TrackerIndex(100), BufferUsesVertex)
}

func TestBufferUsageScope_SetUsage(t *testing.T) {
	scope := NewBufferUsageScope()

	// First usage
	err := scope.SetUsage(TrackerIndex(0), BufferUsesVertex)
	if err != nil {
		t.Fatalf("First SetUsage failed: %v", err)
	}
	if scope.GetUsage(TrackerIndex(0)) != BufferUsesVertex {
		t.Error("Usage not set correctly")
	}

	// Compatible usage should merge
	err = scope.SetUsage(TrackerIndex(0), BufferUsesUniform)
	if err != nil {
		t.Fatalf("Compatible SetUsage failed: %v", err)
	}
	expected := BufferUsesVertex | BufferUsesUniform
	if scope.GetUsage(TrackerIndex(0)) != expected {
		t.Errorf("Usage = %d, want %d", scope.GetUsage(TrackerIndex(0)), expected)
	}

	// Incompatible usage should fail
	err = scope.SetUsage(TrackerIndex(0), BufferUsesCopyDst)
	if err == nil {
		t.Error("Incompatible usage should return error")
	}
	var uce *UsageConflictError
	if !errors.As(err, &uce) {
		t.Errorf("Error should be UsageConflictError, got %T", err)
	}
}

func TestBufferUsageScope_Clear(t *testing.T) {
	scope := NewBufferUsageScope()

	_ = scope.SetUsage(TrackerIndex(0), BufferUsesVertex)
	_ = scope.SetUsage(TrackerIndex(1), BufferUsesCopySrc)

	scope.Clear()

	if scope.IsUsed(TrackerIndex(0)) {
		t.Error("Index 0 should not be used after clear")
	}
	if scope.IsUsed(TrackerIndex(1)) {
		t.Error("Index 1 should not be used after clear")
	}
}

func TestBufferTracker_Merge(t *testing.T) {
	tracker := NewBufferTracker()
	scope := NewBufferUsageScope()

	// Add buffer to device tracker
	tracker.InsertSingle(TrackerIndex(0), BufferUsesVertex)

	// Use buffer in scope with different usage
	_ = scope.SetUsage(TrackerIndex(0), BufferUsesCopySrc)

	// Merge should generate transition
	transitions := tracker.Merge(scope)

	if len(transitions) != 1 {
		t.Fatalf("Expected 1 transition, got %d", len(transitions))
	}

	trans := transitions[0]
	if trans.Index != TrackerIndex(0) {
		t.Errorf("Transition index = %d, want 0", trans.Index)
	}
	if trans.Usage.From != BufferUsesVertex {
		t.Errorf("From = %d, want %d", trans.Usage.From, BufferUsesVertex)
	}
	if trans.Usage.To != BufferUsesCopySrc {
		t.Errorf("To = %d, want %d", trans.Usage.To, BufferUsesCopySrc)
	}

	// Tracker should be updated
	if tracker.GetUsage(TrackerIndex(0)) != BufferUsesCopySrc {
		t.Error("Tracker usage should be updated after merge")
	}
}

func TestBufferTracker_Merge_NewBuffer(t *testing.T) {
	tracker := NewBufferTracker()
	scope := NewBufferUsageScope()

	// Buffer only in scope, not in tracker
	_ = scope.SetUsage(TrackerIndex(5), BufferUsesUniform)

	transitions := tracker.Merge(scope)

	// No transition for new buffer
	if len(transitions) != 0 {
		t.Errorf("Expected 0 transitions for new buffer, got %d", len(transitions))
	}

	// But tracker should now have it
	if !tracker.IsTracked(TrackerIndex(5)) {
		t.Error("New buffer should be tracked after merge")
	}
	if tracker.GetUsage(TrackerIndex(5)) != BufferUsesUniform {
		t.Error("New buffer should have scope's usage")
	}
}

func TestBufferTracker_Merge_NoTransitionIfSame(t *testing.T) {
	tracker := NewBufferTracker()
	scope := NewBufferUsageScope()

	tracker.InsertSingle(TrackerIndex(0), BufferUsesVertex)
	_ = scope.SetUsage(TrackerIndex(0), BufferUsesVertex) // Same usage

	transitions := tracker.Merge(scope)

	if len(transitions) != 0 {
		t.Errorf("Expected 0 transitions for same usage, got %d", len(transitions))
	}
}

func TestStateTransition_NeedsBarrier(t *testing.T) {
	tests := []struct {
		name string
		from BufferUses
		to   BufferUses
		want bool
	}{
		{"same usage", BufferUsesVertex, BufferUsesVertex, false},
		{"read to read", BufferUsesVertex, BufferUsesUniform, false},
		{"read to write", BufferUsesVertex, BufferUsesCopyDst, true},
		{"write to read", BufferUsesCopyDst, BufferUsesVertex, true},
		{"write to write", BufferUsesCopyDst, BufferUsesStorageWrite, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trans := StateTransition{From: tt.from, To: tt.to}
			if got := trans.NeedsBarrier(); got != tt.want {
				t.Errorf("NeedsBarrier() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPendingTransition_IntoHAL(t *testing.T) {
	trans := PendingTransition{
		Index: TrackerIndex(0),
		Usage: StateTransition{
			From: BufferUsesVertex,
			To:   BufferUsesCopyDst,
		},
	}

	// Create a nil buffer (HAL conversion doesn't need actual buffer for this test)
	barrier := trans.IntoHAL(nil)

	if barrier.Usage.OldUsage != types.BufferUsageVertex {
		t.Errorf("OldUsage = %d, want %d", barrier.Usage.OldUsage, types.BufferUsageVertex)
	}
	if barrier.Usage.NewUsage != types.BufferUsageCopyDst {
		t.Errorf("NewUsage = %d, want %d", barrier.Usage.NewUsage, types.BufferUsageCopyDst)
	}
}

func TestResourceMetadata(t *testing.T) {
	m := NewResourceMetadata()

	if m.Count() != 0 {
		t.Errorf("Initial count = %d, want 0", m.Count())
	}

	m.SetOwned(TrackerIndex(0), true)
	m.SetOwned(TrackerIndex(5), true)

	if m.Count() != 2 {
		t.Errorf("Count after 2 adds = %d, want 2", m.Count())
	}
	if !m.IsOwned(TrackerIndex(0)) {
		t.Error("Index 0 should be owned")
	}
	if !m.IsOwned(TrackerIndex(5)) {
		t.Error("Index 5 should be owned")
	}
	if m.IsOwned(TrackerIndex(3)) {
		t.Error("Index 3 should not be owned")
	}

	m.SetOwned(TrackerIndex(0), false)
	if m.Count() != 1 {
		t.Errorf("Count after remove = %d, want 1", m.Count())
	}

	m.Clear()
	if m.Count() != 0 {
		t.Errorf("Count after clear = %d, want 0", m.Count())
	}
}

func TestUsageConflictError(t *testing.T) {
	err := &UsageConflictError{
		Index:    TrackerIndex(5),
		Existing: BufferUsesVertex,
		New:      BufferUsesCopyDst,
	}

	msg := err.Error()
	if msg == "" {
		t.Error("Error message should not be empty")
	}
}

func BenchmarkBufferTracker_InsertRemove(b *testing.B) {
	tracker := NewBufferTracker()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		idx := TrackerIndex(i % 1000)
		tracker.InsertSingle(idx, BufferUsesVertex)
		tracker.Remove(idx)
	}
}

func BenchmarkBufferUsageScope_SetUsage(b *testing.B) {
	scope := NewBufferUsageScope()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		idx := TrackerIndex(i % 100)
		_ = scope.SetUsage(idx, BufferUsesVertex)
	}
}

func BenchmarkBufferTracker_Merge(b *testing.B) {
	tracker := NewBufferTracker()
	scope := NewBufferUsageScope()

	// Pre-populate
	for i := 0; i < 100; i++ {
		tracker.InsertSingle(TrackerIndex(i), BufferUsesVertex)
		_ = scope.SetUsage(TrackerIndex(i), BufferUsesCopySrc)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = tracker.Merge(scope)
	}
}
