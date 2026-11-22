//go:build ((windows && (386 || amd64 || arm64)) || (((linux && !android) || darwin) && (amd64 || arm64)) || (android && arm64)) && !rust && !(js && wasm) && !wgpu_nosoftware

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu_test

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// TestEnumerateAdaptersPublicPath pins the #360 contract end to end on the one
// backend CI can always provide: the software adapter enumerates with its
// metadata, the handle opens a device (the selection half Born asked for),
// Release is wrapper-only (g3d's concern), and a released instance enumerates
// nothing.
func TestEnumerateAdaptersPublicPath(t *testing.T) {
	inst, err := wgpu.CreateInstance(&wgpu.InstanceDescriptor{Backends: gputypes.BackendsSoftware})
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}

	adapters := inst.EnumerateAdapters()
	if len(adapters) != 1 {
		inst.Release()
		t.Fatalf("EnumerateAdapters = %d adapters, want the one software adapter", len(adapters))
	}
	adapter := adapters[0]
	info := adapter.Info()
	if info.Backend != gputypes.BackendSoftware || info.DeviceType != gputypes.DeviceTypeCPU {
		inst.Release()
		t.Fatalf("enumerated adapter = %+v, want the software CPU adapter", info)
	}
	if info.Name == "" {
		inst.Release()
		t.Fatal("enumerated adapter has no name")
	}

	// The handle is directly usable for explicit selection.
	device, err := adapter.RequestDevice(nil)
	if err != nil {
		inst.Release()
		t.Fatalf("RequestDevice on an enumerated adapter: %v", err)
	}
	device.Release()

	// Release marks the wrapper only: the released handle refuses use with an
	// error, and the instance still serves a fresh adapter.
	adapter.Release()
	if _, err := adapter.RequestDevice(nil); !errors.Is(err, wgpu.ErrReleased) {
		inst.Release()
		t.Fatalf("RequestDevice after Release = %v, want ErrReleased", err)
	}
	fresh, err := inst.RequestAdapter(nil)
	if err != nil {
		inst.Release()
		t.Fatalf("RequestAdapter after releasing an enumerated handle: %v", err)
	}
	fresh.Release()

	inst.Release()
	if got := inst.EnumerateAdapters(); got != nil {
		t.Fatalf("EnumerateAdapters on a released instance = %d adapters, want nil", len(got))
	}
}
