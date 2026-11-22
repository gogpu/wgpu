// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !(js && wasm)

package core

// gpuexpert probe for the P1-API-040 implementation gate (wgpu PR #446),
// adopted into the owner's suite: it pins the Placeholder copy at the CREATION
// registration site (registerExposedAdapters), which the delivered suite left
// unpinned — every other placeholder test drives the deferred GLES path.
// Executed green at a4290b5 and red when the register-site copy is removed
// (gpuexpert-authored; provenance recorded here per the #419 pattern).

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/hal"
)

// TestInstancePlaceholderMarkedAtCreationRegistration pins R1 at the second
// registration site: a non-GL backend registers at instance creation via
// registerExposedAdapters (not the deferred GLES path), and the Placeholder
// mark must survive that copy too. Today only GLES produces placeholders and
// GLES is always deferred, so this site is defensive — a future non-GLES
// placeholder producer (or moving GLES off deferral) must not silently leak a
// non-openable adapter into the public enumeration.
func TestInstancePlaceholderMarkedAtCreationRegistration(t *testing.T) {
	GetGlobal().Clear()
	placeholder := fakeExposed("Placeholder Device", gputypes.BackendVulkan, gputypes.DeviceTypeOther)
	placeholder.Placeholder = true
	useProviders(t, &testProvider{
		variant:   gputypes.BackendVulkan,
		available: true,
		instance:  &fakeHALInstance{adapters: []hal.ExposedAdapter{placeholder}},
	})

	instance := NewInstance(&gputypes.InstanceDescriptor{Backends: gputypes.BackendsVulkan})
	t.Cleanup(instance.Destroy)

	// Vulkan is not deferred: the adapter is registered at creation.
	ids := instance.EnumerateAdapters()
	if len(ids) != 1 {
		t.Fatalf("adapters = %d, want 1 (creation registration)", len(ids))
	}
	adapter, err := GetGlobal().Hub().GetAdapter(ids[0])
	if err != nil {
		t.Fatalf("GetAdapter: %v", err)
	}
	if !adapter.Placeholder {
		t.Fatal("registerExposedAdapters lost the Placeholder mark; a non-GLES placeholder producer would leak into public enumeration (R1)")
	}
}
