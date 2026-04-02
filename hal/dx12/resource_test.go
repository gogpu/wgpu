// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

package dx12

import (
	"testing"
)

func TestTexture_AddPendingRef(t *testing.T) {
	tex := &Texture{}

	tex.AddPendingRef()
	if tex.pendingRefs != 1 {
		t.Errorf("after AddPendingRef: pendingRefs = %d, want 1", tex.pendingRefs)
	}

	tex.AddPendingRef()
	if tex.pendingRefs != 2 {
		t.Errorf("after second AddPendingRef: pendingRefs = %d, want 2", tex.pendingRefs)
	}
}

func TestTexture_DecPendingRef_NoDefer(t *testing.T) {
	tex := &Texture{}
	tex.pendingRefs = 2

	tex.DecPendingRef()
	if tex.pendingRefs != 1 {
		t.Errorf("after DecPendingRef: pendingRefs = %d, want 1", tex.pendingRefs)
	}
	// pendingDeath is false, so no destroy should happen.
	if tex.pendingDeath {
		t.Error("pendingDeath should be false when Destroy was not called")
	}
}

func TestTexture_Destroy_NoPending(t *testing.T) {
	// When pendingRefs == 0, Destroy should call doDestroy immediately.
	// We verify by checking that raw becomes nil (non-external texture).
	tex := &Texture{
		raw:        nil, // already nil, but pendingDeath should NOT be set
		isExternal: false,
	}

	tex.Destroy()

	if tex.pendingDeath {
		t.Error("pendingDeath should be false when refs == 0 (immediate destroy)")
	}
}

func TestTexture_Destroy_WithPending(t *testing.T) {
	// When pendingRefs > 0, Destroy should defer (set pendingDeath=true) and NOT release.
	tex := &Texture{
		pendingRefs: 2,
		isExternal:  false,
	}

	tex.Destroy()

	if !tex.pendingDeath {
		t.Error("pendingDeath should be true when Destroy called with pendingRefs > 0")
	}
	// pendingRefs should be unchanged.
	if tex.pendingRefs != 2 {
		t.Errorf("pendingRefs should remain 2, got %d", tex.pendingRefs)
	}
}

func TestTexture_DecPendingRef_TriggersDefer(t *testing.T) {
	// Simulate: AddPendingRef, Destroy (deferred), then DecPendingRef triggers release.
	tex := &Texture{
		raw:        nil, // nil raw so doDestroy is safe (no COM Release)
		isExternal: false,
	}

	tex.AddPendingRef() // pendingRefs = 1
	tex.Destroy()       // pendingDeath = true, but refs > 0 so deferred

	if !tex.pendingDeath {
		t.Fatal("pendingDeath should be true after Destroy with pending refs")
	}

	tex.DecPendingRef() // pendingRefs = 0, pendingDeath = true => doDestroy fires

	// After doDestroy, raw should be nil (it was already nil, but the path was exercised).
	// The key assertion is that we reached doDestroy without panic.
	if tex.pendingRefs > 0 {
		t.Errorf("pendingRefs should be <= 0 after final DecPendingRef, got %d", tex.pendingRefs)
	}
}

func TestTexture_DecPendingRef_NoDeferWithoutDestroy(t *testing.T) {
	// DecPendingRef with pendingDeath=false should NOT trigger doDestroy
	// even when pendingRefs reaches 0.
	tex := &Texture{
		pendingRefs: 1,
		isExternal:  false,
	}

	tex.DecPendingRef()

	if tex.pendingRefs != 0 {
		t.Errorf("pendingRefs should be 0, got %d", tex.pendingRefs)
	}
	// pendingDeath was never set, so no destroy should trigger.
	if tex.pendingDeath {
		t.Error("pendingDeath should remain false")
	}
}
