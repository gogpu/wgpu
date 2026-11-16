//go:build ((windows && (386 || amd64 || arm64)) || (((linux && !android) || darwin) && (amd64 || arm64)) || (android && arm64)) && !rust && !(js && wasm) && !wgpu_nosoftware

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu_test

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// TestSoftwareIsSelectedByItsMaskAlone pins ADR-083 §2.3 on the public path:
// the software backend is registered by default but is never part of the
// default set's enumeration or Auto selection (neither mask 0 nor nil admits
// it), a BackendsSoftware mask selects it, and RequestAdapterOptions'
// ForceFallbackAdapter reaches it under the default set (ADR-079's rows; the
// dated implementation note in ADR-083 §2.4).
func TestSoftwareIsSelectedByItsMaskAlone(t *testing.T) {
	for _, desc := range []*wgpu.InstanceDescriptor{
		nil,
		{Backends: 0},
	} {
		inst, err := wgpu.CreateInstance(desc)
		if err != nil {
			t.Fatalf("CreateInstance(%v): %v", desc, err)
		}
		if adapter, err := inst.RequestAdapter(nil); err == nil {
			if got := adapter.Info().Backend; got == gputypes.BackendSoftware {
				t.Errorf("the default set selected the software adapter %q; software is explicit-only", adapter.Info().Name)
			}
			adapter.Release()
		}
		if adapter, err := inst.RequestAdapter(&wgpu.RequestAdapterOptions{ForceFallbackAdapter: true}); err != nil {
			t.Errorf("ForceFallbackAdapter on the default set: %v (ADR-083 §2.3: the force path reaches software)", err)
		} else {
			if got := adapter.Info().Backend; got != gputypes.BackendSoftware {
				t.Errorf("ForceFallbackAdapter selected %v, want the software backend (ADR-079: preferred among CPU adapters)", got)
			}
			adapter.Release()
		}
		inst.Release()
	}

	sw, err := wgpu.CreateInstance(&wgpu.InstanceDescriptor{Backends: gputypes.BackendsSoftware})
	if err != nil {
		t.Fatalf("CreateInstance(BackendsSoftware): %v", err)
	}
	defer sw.Release()

	adapter, err := sw.RequestAdapter(nil)
	if err != nil {
		t.Fatalf("RequestAdapter on a BackendsSoftware instance: %v", err)
	}
	defer adapter.Release()
	info := adapter.Info()
	if info.Backend != gputypes.BackendSoftware {
		t.Errorf("adapter backend = %v, want BackendSoftware", info.Backend)
	}
	if info.DeviceType != gputypes.DeviceTypeCPU {
		t.Errorf("adapter device type = %v, want CPU", info.DeviceType)
	}
}
