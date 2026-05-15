package browser

import (
	"testing"

	"github.com/gogpu/gputypes"
)

// TestLoadOpToJS verifies all load operation mappings.
func TestLoadOpToJS(t *testing.T) {
	tests := []struct {
		op   gputypes.LoadOp
		want string
	}{
		{gputypes.LoadOpClear, "clear"},
		{gputypes.LoadOpLoad, "load"},
		{gputypes.LoadOpUndefined, "load"}, // default fallback
	}
	for _, tc := range tests {
		got := LoadOpToJS(tc.op)
		if got != tc.want {
			t.Errorf("LoadOpToJS(%v) = %q, want %q", tc.op, got, tc.want)
		}
	}
}

// TestStoreOpToJS verifies all store operation mappings.
func TestStoreOpToJS(t *testing.T) {
	tests := []struct {
		op   gputypes.StoreOp
		want string
	}{
		{gputypes.StoreOpStore, "store"},
		{gputypes.StoreOpDiscard, "discard"},
		{gputypes.StoreOpUndefined, "store"}, // default fallback
	}
	for _, tc := range tests {
		got := StoreOpToJS(tc.op)
		if got != tc.want {
			t.Errorf("StoreOpToJS(%v) = %q, want %q", tc.op, got, tc.want)
		}
	}
}

// TestLoadOpStoreOpRoundTrip verifies that every non-undefined load/store op
// produces a non-empty string, preventing silent empty-string bugs at runtime.
func TestLoadOpStoreOpRoundTrip(t *testing.T) {
	loadOps := []gputypes.LoadOp{gputypes.LoadOpLoad, gputypes.LoadOpClear}
	for _, op := range loadOps {
		s := LoadOpToJS(op)
		if s == "" {
			t.Errorf("LoadOpToJS(%v) returned empty string — missing mapping", op)
		}
	}

	storeOps := []gputypes.StoreOp{gputypes.StoreOpStore, gputypes.StoreOpDiscard}
	for _, op := range storeOps {
		s := StoreOpToJS(op)
		if s == "" {
			t.Errorf("StoreOpToJS(%v) returned empty string — missing mapping", op)
		}
	}
}
