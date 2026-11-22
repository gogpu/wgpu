// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !(js && wasm)

package core

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/hal"
)

// TestInstancePlaceholderStaysInSelectionAndIsMarked pins P1-API-040 R1 at the
// core layer: a placeholder adapter (GLES without an EGL context) stays a
// normal candidate for adapter selection — that is what keeps the existing
// descriptive Open error — and carries the Placeholder mark the public
// enumeration filters on.
func TestInstancePlaceholderStaysInSelectionAndIsMarked(t *testing.T) {
	GetGlobal().Clear()
	placeholder := fakeExposed("OpenGL Adapter", gputypes.BackendGL, gputypes.DeviceTypeOther)
	placeholder.Placeholder = true
	useProviders(t, &testProvider{
		variant:   gputypes.BackendGL,
		available: true,
		instance:  &fakeHALInstance{adapters: []hal.ExposedAdapter{placeholder}},
	})

	instance := NewInstance(&gputypes.InstanceDescriptor{Backends: gputypes.BackendsGL})
	t.Cleanup(instance.Destroy)

	// The GL backend enumerates lazily: nothing is known before the first
	// request, and the request registers whatever the HAL returned.
	if adapters := instance.EnumerateAdapters(); len(adapters) != 0 {
		t.Fatalf("adapters before the first request = %d, want 0 (the GL backend defers)", len(adapters))
	}
	if _, err := instance.RequestAdapter(nil); err != nil {
		t.Fatalf("RequestAdapter: %v (selection keeps seeing placeholders so Open can fail descriptively)", err)
	}

	ids := instance.EnumerateAdapters()
	if len(ids) != 1 {
		t.Fatalf("core adapters = %d, want 1: the placeholder stays in the selection list", len(ids))
	}
	adapter, err := GetGlobal().Hub().GetAdapter(ids[0])
	if err != nil {
		t.Fatalf("GetAdapter: %v", err)
	}
	if !adapter.Placeholder {
		t.Fatal("core adapter lost the Placeholder mark; the public enumeration cannot filter it")
	}
}

// TestInstanceRealAdaptersAreNotMarked is the control: an adapter the HAL
// reports normally stays enumeration-visible.
func TestInstanceRealAdaptersAreNotMarked(t *testing.T) {
	GetGlobal().Clear()
	useProviders(t, &testProvider{
		variant:   gputypes.BackendGL,
		available: true,
		instance: &fakeHALInstance{adapters: []hal.ExposedAdapter{
			fakeExposed("GL Hardware", gputypes.BackendGL, gputypes.DeviceTypeDiscreteGPU),
		}},
	})

	instance := NewInstance(&gputypes.InstanceDescriptor{Backends: gputypes.BackendsGL})
	t.Cleanup(instance.Destroy)

	if _, err := instance.RequestAdapter(nil); err != nil {
		t.Fatalf("RequestAdapter: %v", err)
	}
	ids := instance.EnumerateAdapters()
	if len(ids) != 1 {
		t.Fatalf("core adapters = %d, want 1", len(ids))
	}
	adapter, err := GetGlobal().Hub().GetAdapter(ids[0])
	if err != nil {
		t.Fatalf("GetAdapter: %v", err)
	}
	if adapter.Placeholder {
		t.Fatal("a real adapter was marked as a placeholder")
	}
}

// TestInstanceFallbackAdaptersNeverLeakIntoEnumeration pins P1-API-040 R4 at
// the core layer: a ForceFallbackAdapter request admits the software adapter as
// a request-local candidate only, and the instance's enumeration never lists
// it (the public contract says the same: a force request adds nothing).
func TestInstanceFallbackAdaptersNeverLeakIntoEnumeration(t *testing.T) {
	GetGlobal().Clear()
	useProviders(t,
		&testProvider{
			variant:   gputypes.BackendVulkan,
			available: true,
			instance: &fakeHALInstance{adapters: []hal.ExposedAdapter{
				fakeExposed("Iris Xe", gputypes.BackendVulkan, gputypes.DeviceTypeIntegratedGPU),
			}},
		},
		&testProvider{
			variant:   gputypes.BackendSoftware,
			available: true,
			instance: &fakeHALInstance{adapters: []hal.ExposedAdapter{
				fakeExposed("Software Renderer", gputypes.BackendSoftware, gputypes.DeviceTypeCPU),
			}},
		},
	)

	// Mask 0 is the default set: hardware backends enumerate, software does not.
	instance := NewInstance(&gputypes.InstanceDescriptor{Backends: 0})
	t.Cleanup(instance.Destroy)

	if _, err := instance.RequestAdapter(&gputypes.RequestAdapterOptions{ForceFallbackAdapter: true}); err != nil {
		t.Fatalf("ForceFallbackAdapter request: %v", err)
	}

	ids := instance.EnumerateAdapters()
	if len(ids) != 1 {
		t.Fatalf("adapters = %d, want only the hardware adapter", len(ids))
	}
	adapter, err := GetGlobal().Hub().GetAdapter(ids[0])
	if err != nil {
		t.Fatalf("GetAdapter: %v", err)
	}
	if adapter.Backend == gputypes.BackendSoftware {
		t.Fatal("the lazily admitted fallback adapter leaked into enumeration (R4)")
	}
}
