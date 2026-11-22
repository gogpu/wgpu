//go:build ((windows && (386 || amd64 || arm64)) || (((linux && !android) || darwin) && (amd64 || arm64)) || (android && arm64)) && !rust && !(js && wasm) && !wgpu_nosoftware

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu_test

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// TestEnumeratedSoftwareAdapterIdentityIsZero pins ADR-084's per-backend table
// for the one backend CI can always provide: the software adapter reports no
// hardware identity, and Identity is a safe zero on a released or zero-value
// adapter (nil implementation) rather than a panic.
func TestEnumeratedSoftwareAdapterIdentityIsZero(t *testing.T) {
	inst, err := wgpu.CreateInstance(&wgpu.InstanceDescriptor{Backends: gputypes.BackendsSoftware})
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	defer inst.Release()

	adapters := inst.EnumerateAdapters()
	if len(adapters) != 1 {
		t.Fatalf("EnumerateAdapters = %d adapters, want the one software adapter", len(adapters))
	}
	adapter := adapters[0]

	if id := adapter.Identity(); !id.IsZero() {
		t.Fatalf("software adapter Identity = %+v, want zero on every field (ADR-084 §2.3)", id)
	}

	// A released adapter still answers the zero identity: the implementation
	// guards the released state instead of reading stale data or panicking.
	adapter.Release()
	if id := adapter.Identity(); !id.IsZero() {
		t.Fatalf("released adapter Identity = %+v, want zero", id)
	}

	// A zero-value wrapper (nil implementation) answers zero too, not a panic.
	var zero wgpu.Adapter
	if id := zero.Identity(); !id.IsZero() {
		t.Fatalf("zero-value Adapter.Identity = %+v, want zero", id)
	}
}
