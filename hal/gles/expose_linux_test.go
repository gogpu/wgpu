// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build linux && !(js && wasm)

package gles

import (
	"testing"

	"github.com/gogpu/gputypes"
)

// TestExposeAdapterCarriesCapabilities checks that an exposed Linux adapter
// reports the probed capabilities. The instance-context adapter used to skip
// the probe, so a headless Mesa llvmpipe context (GL 4.5) reported no compute.
func TestExposeAdapterCarriesCapabilities(t *testing.T) {
	limits := gputypes.DefaultLimits()
	limits.MaxComputeInvocationsPerWorkgroup = 1024
	caps := AdapterCapabilities{
		Vendor:         "Mesa",
		Renderer:       "llvmpipe (LLVM 17.0.6, 256 bits)",
		Version:        "4.5 (Core Profile) Mesa 24.0.5",
		GLMajor:        4,
		GLMinor:        5,
		DeviceType:     gputypes.DeviceTypeCPU,
		Features:       gputypes.Features(gputypes.FeatureDepthClipControl),
		Limits:         limits,
		DownlevelFlags: gputypes.DownlevelFlagsComputeShaders | gputypes.DownlevelFlagsFragmentStorage,
	}
	ctx := &AdapterContext{}
	got := exposeAdapter(&Adapter{ctx: ctx}, &caps)

	if got.Capabilities.DownlevelCapabilities.Flags != caps.DownlevelFlags {
		t.Errorf("downlevel flags = %#x, want %#x", uint32(got.Capabilities.DownlevelCapabilities.Flags), uint32(caps.DownlevelFlags))
	}
	if got.Features != caps.Features {
		t.Errorf("features = %v, want %v", got.Features, caps.Features)
	}
	if got.Capabilities.Limits != caps.Limits {
		t.Error("limits are not the probed ones")
	}
	if got.Info.Name != caps.Renderer || got.Info.DeviceType != caps.DeviceType || got.Info.Backend != gputypes.BackendGL {
		t.Errorf("info = %+v", got.Info)
	}
	a, ok := got.Adapter.(*Adapter)
	if !ok || a.ctx != ctx || a.caps.GLMajor != 4 || a.caps.GLMinor != 5 {
		t.Errorf("adapter = %+v, want the context and the probed caps", got.Adapter)
	}
	if got.Capabilities.AlignmentsMask.BufferCopyPitch != 4 || got.Capabilities.AlignmentsMask.BufferCopyOffset != 4 {
		t.Errorf("alignments = %+v, want 4 and 4 (Rust wgpu-hal gles)", got.Capabilities.AlignmentsMask)
	}
}
