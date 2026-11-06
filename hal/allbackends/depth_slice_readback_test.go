// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"fmt"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// TestRenderToDepthSlice renders into two depth slices of a 3D texture, at
// mip 0 and at mip 1: slice 0 is cleared red, the last slice of the level
// blue, and both get a green left half. Each slice must hold only its own
// pass (WebGPU GPURenderPassColorAttachment.depthSlice).
func TestRenderToDepthSlice(t *testing.T) {
	forEachOverrideBackend(t, 0, func(t *testing.T, device *wgpu.Device) {
		for _, mip := range []uint32{0, 1} {
			t.Run(fmt.Sprintf("mip%d", mip), func(t *testing.T) {
				r := newTargetRig(t, device, gputypes.TextureDimension3D, sliceRigDepth)
				last := max(uint32(sliceRigDepth)>>mip, 1) - 1
				zero := uint32(0)
				r.render(t, mip, &zero, sliceRed)
				r.render(t, mip, &last, sliceBlue)
				r.check(t, mip, 0, sliceRed)
				r.check(t, mip, last, sliceBlue)
			})
		}
	})
}

// TestDepthSliceViewRelease renders into two depth slices through one 3D
// view, releases the view and waits for its deferred destruction, then
// renders a slice again through a new view. On Vulkan the 2D views of the
// slices live with the 3D view and must go with it, framebuffers included:
// the validation layer reports a leaked or stale view.
func TestDepthSliceViewRelease(t *testing.T) {
	forEachOverrideBackend(t, 0, func(t *testing.T, device *wgpu.Device) {
		r := newTargetRig(t, device, gputypes.TextureDimension3D, sliceRigDepth)
		one, two := uint32(1), uint32(2)
		first := r.newView(t, 0)
		r.renderView(t, first, &one, sliceRed)
		r.renderView(t, first, &two, sliceRed)
		first.Release()
		if err := device.WaitIdle(); err != nil {
			t.Fatalf("WaitIdle: %v", err)
		}
		device.Poll(wgpu.PollWait)

		second := r.newView(t, 0)
		t.Cleanup(second.Release)
		r.renderView(t, second, &one, sliceBlue)
		r.check(t, 0, 1, sliceBlue)
		r.check(t, 0, 2, sliceRed)
	})
}
