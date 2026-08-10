//go:build !(js && wasm)

package track

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
)

func TestTextureUses_IsReadOnly(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		uses TextureUses
		want bool
	}{
		{"none is read-only", TextureUsesNone, true},
		{"copy src is read-only", TextureUsesCopySrc, true},
		{"resource is read-only", TextureUsesResource, true},
		{"depth stencil read is read-only", TextureUsesDepthStencilRead, true},
		{"storage read is read-only", TextureUsesStorageRead, true},
		{"uninitialized is read-only (no exclusive bit)", TextureUsesUninitialized, true},
		{"copy dst is write", TextureUsesCopyDst, false},
		{"color target is write", TextureUsesColorTarget, false},
		{"depth stencil write is write", TextureUsesDepthStencilWrite, false},
		{"storage write is write", TextureUsesStorageWrite, false},
		{"present is write", TextureUsesPresent, false},
		{"combined read-only", TextureUsesCopySrc | TextureUsesResource, true},
		{"read + write", TextureUsesCopySrc | TextureUsesCopyDst, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.uses.IsReadOnly(); got != tt.want {
				t.Errorf("TextureUses(%d).IsReadOnly() = %v, want %v", tt.uses, got, tt.want)
			}
		})
	}
}

func TestTextureUses_IsEmpty(t *testing.T) {
	t.Parallel()

	if !TextureUsesNone.IsEmpty() {
		t.Error("TextureUsesNone should be empty")
	}
	if TextureUsesCopySrc.IsEmpty() {
		t.Error("TextureUsesCopySrc should not be empty")
	}
}

func TestTextureUses_Contains(t *testing.T) {
	t.Parallel()

	combined := TextureUsesCopySrc | TextureUsesResource | TextureUsesColorTarget

	if !combined.Contains(TextureUsesCopySrc) {
		t.Error("Combined should contain CopySrc")
	}
	if !combined.Contains(TextureUsesResource) {
		t.Error("Combined should contain Resource")
	}
	if !combined.Contains(TextureUsesCopySrc | TextureUsesResource) {
		t.Error("Combined should contain CopySrc|Resource")
	}
	if combined.Contains(TextureUsesCopyDst) {
		t.Error("Combined should not contain CopyDst")
	}
}

func TestTextureUses_AllOrdered(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		uses TextureUses
		want bool
	}{
		{"none is ordered", TextureUsesNone, true},
		{"copy src is ordered", TextureUsesCopySrc, true},
		{"resource is ordered", TextureUsesResource, true},
		{"color target is ordered", TextureUsesColorTarget, true},
		{"depth stencil read is ordered", TextureUsesDepthStencilRead, true},
		{"depth stencil write is ordered", TextureUsesDepthStencilWrite, true},
		{"storage read is ordered", TextureUsesStorageRead, true},
		{"copy dst is NOT ordered", TextureUsesCopyDst, false},
		{"storage write is NOT ordered", TextureUsesStorageWrite, false},
		{"present is NOT ordered", TextureUsesPresent, false},
		{"uninitialized is NOT ordered", TextureUsesUninitialized, false},
		{"ordered combo", TextureUsesCopySrc | TextureUsesResource | TextureUsesColorTarget, true},
		{"mixed ordered + unordered", TextureUsesCopySrc | TextureUsesPresent, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.uses.AllOrdered(); got != tt.want {
				t.Errorf("TextureUses(%d).AllOrdered() = %v, want %v", tt.uses, got, tt.want)
			}
		})
	}
}

func TestTextureUses_IsExclusive(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		uses TextureUses
		want bool
	}{
		{"none", TextureUsesNone, false},
		{"copy src", TextureUsesCopySrc, false},
		{"resource", TextureUsesResource, false},
		{"copy dst", TextureUsesCopyDst, true},
		{"color target", TextureUsesColorTarget, true},
		{"depth stencil write", TextureUsesDepthStencilWrite, true},
		{"storage write", TextureUsesStorageWrite, true},
		{"present", TextureUsesPresent, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.uses.IsExclusive(); got != tt.want {
				t.Errorf("TextureUses(%d).IsExclusive() = %v, want %v", tt.uses, got, tt.want)
			}
		})
	}
}

func TestTextureUses_IsCompatible(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		a    TextureUses
		b    TextureUses
		want bool
	}{
		{"empty with empty", TextureUsesNone, TextureUsesNone, true},
		{"empty with read", TextureUsesNone, TextureUsesCopySrc, true},
		{"empty with write", TextureUsesNone, TextureUsesCopyDst, true},
		{"read with read", TextureUsesCopySrc, TextureUsesResource, true},
		{"read with same read", TextureUsesResource, TextureUsesResource, true},
		{"exclusive with same exclusive", TextureUsesColorTarget, TextureUsesColorTarget, true},
		{"exclusive with different exclusive", TextureUsesCopyDst, TextureUsesColorTarget, false},
		{"read with exclusive", TextureUsesCopySrc, TextureUsesCopyDst, false},
		{"exclusive with read", TextureUsesCopyDst, TextureUsesResource, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.a.IsCompatible(tt.b); got != tt.want {
				t.Errorf("TextureUses(%d).IsCompatible(%d) = %v, want %v",
					tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestSkipBarrier(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		old  TextureUses
		new  TextureUses
		want bool
	}{
		{"same ordered state skips", TextureUsesColorTarget, TextureUsesColorTarget, true},
		{"same inclusive state skips", TextureUsesResource, TextureUsesResource, true},
		{"same unordered does NOT skip (present)", TextureUsesPresent, TextureUsesPresent, false},
		{"same unordered does NOT skip (copy dst)", TextureUsesCopyDst, TextureUsesCopyDst, false},
		{"different state never skips", TextureUsesResource, TextureUsesColorTarget, false},
		{"color target to present", TextureUsesColorTarget, TextureUsesPresent, false},
		{"none to none skips", TextureUsesNone, TextureUsesNone, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := SkipBarrier(tt.old, tt.new); got != tt.want {
				t.Errorf("SkipBarrier(%d, %d) = %v, want %v", tt.old, tt.new, got, tt.want)
			}
		})
	}
}

func TestTextureUses_ToTextureUsage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		uses TextureUses
		want gputypes.TextureUsage
	}{
		{"none", TextureUsesNone, 0},
		{"copy src", TextureUsesCopySrc, gputypes.TextureUsageCopySrc},
		{"copy dst", TextureUsesCopyDst, gputypes.TextureUsageCopyDst},
		{"resource (sampled)", TextureUsesResource, gputypes.TextureUsageTextureBinding},
		{"depth stencil read (sampled)", TextureUsesDepthStencilRead, gputypes.TextureUsageTextureBinding},
		{"storage read", TextureUsesStorageRead, gputypes.TextureUsageStorageBinding},
		{"storage write", TextureUsesStorageWrite, gputypes.TextureUsageStorageBinding},
		{"color target", TextureUsesColorTarget, gputypes.TextureUsageRenderAttachment},
		{"depth stencil write", TextureUsesDepthStencilWrite, gputypes.TextureUsageRenderAttachment},
		{
			"combined",
			TextureUsesCopySrc | TextureUsesResource | TextureUsesColorTarget,
			gputypes.TextureUsageCopySrc | gputypes.TextureUsageTextureBinding | gputypes.TextureUsageRenderAttachment,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.uses.ToTextureUsage(); got != tt.want {
				t.Errorf("TextureUses(%d).ToTextureUsage() = %d, want %d",
					tt.uses, got, tt.want)
			}
		})
	}
}

func TestTextureTracker_InsertSingle(t *testing.T) {
	t.Parallel()

	tracker := NewTextureTracker()

	tracker.InsertSingle(TrackerIndex(0), TextureUsesColorTarget)
	tracker.InsertSingle(TrackerIndex(5), TextureUsesCopySrc)

	if tracker.GetUsage(TrackerIndex(0)) != TextureUsesColorTarget {
		t.Error("Index 0 should have ColorTarget usage")
	}
	if tracker.GetUsage(TrackerIndex(5)) != TextureUsesCopySrc {
		t.Error("Index 5 should have CopySrc usage")
	}
	if tracker.Size() != 2 {
		t.Errorf("Size = %d, want 2", tracker.Size())
	}
}

func TestTextureTracker_Remove(t *testing.T) {
	t.Parallel()

	tracker := NewTextureTracker()

	tracker.InsertSingle(TrackerIndex(0), TextureUsesColorTarget)
	tracker.InsertSingle(TrackerIndex(1), TextureUsesCopySrc)

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

func TestTextureTracker_GetUsage(t *testing.T) {
	t.Parallel()

	tracker := NewTextureTracker()

	if tracker.GetUsage(TrackerIndex(0)) != TextureUsesNone {
		t.Error("Untracked texture should return None")
	}

	tracker.InsertSingle(TrackerIndex(0), TextureUsesColorTarget)
	if tracker.GetUsage(TrackerIndex(0)) != TextureUsesColorTarget {
		t.Error("Tracked texture should return its usage")
	}
}

func TestTextureTracker_SetUsage(t *testing.T) {
	t.Parallel()

	tracker := NewTextureTracker()

	tracker.InsertSingle(TrackerIndex(0), TextureUsesColorTarget)
	tracker.SetUsage(TrackerIndex(0), TextureUsesCopySrc)

	if tracker.GetUsage(TrackerIndex(0)) != TextureUsesCopySrc {
		t.Error("Usage should be updated")
	}

	// SetUsage on untracked texture should be no-op
	tracker.SetUsage(TrackerIndex(100), TextureUsesColorTarget)
}

func TestTextureUsageScope_SetUsage(t *testing.T) {
	t.Parallel()

	scope := NewTextureUsageScope()

	// First usage
	err := scope.SetUsage(TrackerIndex(0), TextureUsesResource)
	if err != nil {
		t.Fatalf("First SetUsage failed: %v", err)
	}
	if scope.GetUsage(TrackerIndex(0)) != TextureUsesResource {
		t.Error("Usage not set correctly")
	}

	// Compatible usage should merge (two inclusive usages)
	err = scope.SetUsage(TrackerIndex(0), TextureUsesCopySrc)
	if err != nil {
		t.Fatalf("Compatible SetUsage failed: %v", err)
	}
	expected := TextureUsesResource | TextureUsesCopySrc
	if scope.GetUsage(TrackerIndex(0)) != expected {
		t.Errorf("Usage = %d, want %d", scope.GetUsage(TrackerIndex(0)), expected)
	}

	// Incompatible usage should fail (inclusive + exclusive = bad)
	err = scope.SetUsage(TrackerIndex(0), TextureUsesCopyDst)
	if err == nil {
		t.Error("Incompatible usage should return error")
	}
	var tuce *TextureUsageConflictError
	if !errors.As(err, &tuce) {
		t.Errorf("Error should be TextureUsageConflictError, got %T", err)
	}
}

func TestTextureUsageScope_ExclusiveAloneOK(t *testing.T) {
	t.Parallel()

	scope := NewTextureUsageScope()

	// A single exclusive usage on its own is fine
	err := scope.SetUsage(TrackerIndex(0), TextureUsesColorTarget)
	if err != nil {
		t.Fatalf("Single exclusive usage should succeed: %v", err)
	}

	// Adding another exclusive usage to the same texture should fail
	err = scope.SetUsage(TrackerIndex(0), TextureUsesCopyDst)
	if err == nil {
		t.Error("Two exclusive usages should fail")
	}
}

func TestTextureUsageScope_Clear(t *testing.T) {
	t.Parallel()

	scope := NewTextureUsageScope()

	_ = scope.SetUsage(TrackerIndex(0), TextureUsesResource)
	_ = scope.SetUsage(TrackerIndex(1), TextureUsesCopySrc)

	scope.Clear()

	if scope.IsUsed(TrackerIndex(0)) {
		t.Error("Index 0 should not be used after clear")
	}
	if scope.IsUsed(TrackerIndex(1)) {
		t.Error("Index 1 should not be used after clear")
	}
}

func TestTextureTracker_Merge_Transition(t *testing.T) {
	t.Parallel()

	tracker := NewTextureTracker()
	scope := NewTextureUsageScope()

	// Add texture to device tracker with color target usage
	tracker.InsertSingle(TrackerIndex(0), TextureUsesColorTarget)

	// Use texture in scope with copy source usage (different state)
	_ = scope.SetUsage(TrackerIndex(0), TextureUsesCopySrc)

	// Merge should generate transition
	transitions := tracker.Merge(scope)

	if len(transitions) != 1 {
		t.Fatalf("Expected 1 transition, got %d", len(transitions))
	}

	trans := transitions[0]
	if trans.Index != TrackerIndex(0) {
		t.Errorf("Transition index = %d, want 0", trans.Index)
	}
	if trans.Usage.From != TextureUsesColorTarget {
		t.Errorf("From = %d, want %d", trans.Usage.From, TextureUsesColorTarget)
	}
	if trans.Usage.To != TextureUsesCopySrc {
		t.Errorf("To = %d, want %d", trans.Usage.To, TextureUsesCopySrc)
	}

	// Tracker should be updated
	if tracker.GetUsage(TrackerIndex(0)) != TextureUsesCopySrc {
		t.Error("Tracker usage should be updated after merge")
	}
}

func TestTextureTracker_Merge_NoTransition(t *testing.T) {
	t.Parallel()

	tracker := NewTextureTracker()
	scope := NewTextureUsageScope()

	// Same ordered state: no barrier needed (SkipBarrier returns true)
	tracker.InsertSingle(TrackerIndex(0), TextureUsesResource)
	_ = scope.SetUsage(TrackerIndex(0), TextureUsesResource)

	transitions := tracker.Merge(scope)

	if len(transitions) != 0 {
		t.Errorf("Expected 0 transitions for same ordered state, got %d", len(transitions))
	}
}

func TestTextureTracker_Merge_SameUnorderedNeedsBarrier(t *testing.T) {
	t.Parallel()

	tracker := NewTextureTracker()
	scope := NewTextureUsageScope()

	// Present is unordered — even same-to-same needs a barrier
	tracker.InsertSingle(TrackerIndex(0), TextureUsesPresent)
	_ = scope.SetUsage(TrackerIndex(0), TextureUsesPresent)

	transitions := tracker.Merge(scope)

	if len(transitions) != 1 {
		t.Errorf("Expected 1 transition for same unordered state, got %d", len(transitions))
	}
}

func TestTextureTracker_Merge_Present(t *testing.T) {
	t.Parallel()

	tracker := NewTextureTracker()
	scope := NewTextureUsageScope()

	// Swapchain texture: color target -> present
	tracker.InsertSingle(TrackerIndex(0), TextureUsesColorTarget)
	_ = scope.SetUsage(TrackerIndex(0), TextureUsesPresent)

	transitions := tracker.Merge(scope)

	if len(transitions) != 1 {
		t.Fatalf("Expected 1 transition, got %d", len(transitions))
	}

	trans := transitions[0]
	if trans.Usage.From != TextureUsesColorTarget {
		t.Errorf("From = %d, want %d", trans.Usage.From, TextureUsesColorTarget)
	}
	if trans.Usage.To != TextureUsesPresent {
		t.Errorf("To = %d, want %d", trans.Usage.To, TextureUsesPresent)
	}

	// Verify HAL conversion produces the right gputypes
	barrier := trans.IntoHAL(nil)
	if barrier.Usage.OldUsage != gputypes.TextureUsageRenderAttachment {
		t.Errorf("HAL OldUsage = %d, want RenderAttachment", barrier.Usage.OldUsage)
	}
	// Present maps to 0 in gputypes (not a public usage flag)
	if barrier.Usage.NewUsage != 0 {
		t.Errorf("HAL NewUsage = %d, want 0 (Present has no gputypes mapping)", barrier.Usage.NewUsage)
	}
}

func TestTextureTracker_Merge_NewTexture(t *testing.T) {
	t.Parallel()

	tracker := NewTextureTracker()
	scope := NewTextureUsageScope()

	// Texture only in scope, not in tracker
	_ = scope.SetUsage(TrackerIndex(5), TextureUsesResource)

	transitions := tracker.Merge(scope)

	// No transition for new texture
	if len(transitions) != 0 {
		t.Errorf("Expected 0 transitions for new texture, got %d", len(transitions))
	}

	// But tracker should now have it
	if !tracker.IsTracked(TrackerIndex(5)) {
		t.Error("New texture should be tracked after merge")
	}
	if tracker.GetUsage(TrackerIndex(5)) != TextureUsesResource {
		t.Error("New texture should have scope's usage")
	}
}

func TestTextureStateTransition_NeedsBarrier(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		from TextureUses
		to   TextureUses
		want bool
	}{
		{"same ordered", TextureUsesResource, TextureUsesResource, false},
		{"same unordered (present)", TextureUsesPresent, TextureUsesPresent, true},
		{"different", TextureUsesResource, TextureUsesColorTarget, true},
		{"color to present", TextureUsesColorTarget, TextureUsesPresent, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			trans := TextureStateTransition{From: tt.from, To: tt.to}
			if got := trans.NeedsBarrier(); got != tt.want {
				t.Errorf("NeedsBarrier() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTexturePendingTransition_IntoHAL(t *testing.T) {
	t.Parallel()

	trans := TexturePendingTransition{
		Index: TrackerIndex(0),
		Usage: TextureStateTransition{
			From: TextureUsesResource,
			To:   TextureUsesCopyDst,
		},
	}

	barrier := trans.IntoHAL(nil)

	if barrier.Usage.OldUsage != gputypes.TextureUsageTextureBinding {
		t.Errorf("OldUsage = %d, want %d", barrier.Usage.OldUsage, gputypes.TextureUsageTextureBinding)
	}
	if barrier.Usage.NewUsage != gputypes.TextureUsageCopyDst {
		t.Errorf("NewUsage = %d, want %d", barrier.Usage.NewUsage, gputypes.TextureUsageCopyDst)
	}
}

func TestTextureUsageConflictError(t *testing.T) {
	t.Parallel()

	err := &TextureUsageConflictError{
		Index:    TrackerIndex(5),
		Existing: TextureUsesResource,
		New:      TextureUsesCopyDst,
	}

	msg := err.Error()
	if msg == "" {
		t.Error("Error message should not be empty")
	}
}

func BenchmarkTextureTracker_InsertRemove(b *testing.B) {
	tracker := NewTextureTracker()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		idx := TrackerIndex(i % 1000)
		tracker.InsertSingle(idx, TextureUsesColorTarget)
		tracker.Remove(idx)
	}
}

func BenchmarkTextureUsageScope_SetUsage(b *testing.B) {
	scope := NewTextureUsageScope()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		idx := TrackerIndex(i % 100)
		_ = scope.SetUsage(idx, TextureUsesResource)
	}
}

// =============================================================================
// TextureUsageScope.IsEmpty
// =============================================================================

func TestTextureUsageScope_IsEmpty(t *testing.T) {
	t.Parallel()

	scope := NewTextureUsageScope()
	if !scope.IsEmpty() {
		t.Error("New scope should be empty")
	}

	_ = scope.SetUsage(TrackerIndex(0), TextureUsesColorTarget)
	if scope.IsEmpty() {
		t.Error("Scope should not be empty after SetUsage")
	}

	scope.Clear()
	if !scope.IsEmpty() {
		t.Error("Scope should be empty after Clear")
	}
}

func BenchmarkTextureTracker_Merge(b *testing.B) {
	tracker := NewTextureTracker()
	scope := NewTextureUsageScope()

	// Pre-populate
	for i := 0; i < 100; i++ {
		tracker.InsertSingle(TrackerIndex(i), TextureUsesResource)
		_ = scope.SetUsage(TrackerIndex(i), TextureUsesColorTarget)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = tracker.Merge(scope)
	}
}
