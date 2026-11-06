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
				skipSoftwareMip(t, device, mip)
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

// TestClearDepthSlice renders slice 1 of a 3D texture, then runs a pass on
// slice 2 that only clears it, with no draw: the clear must reach slice 2 and
// no other slice. Regression test for the software backend, whose clear wrote
// the whole texture.
func TestClearDepthSlice(t *testing.T) {
	forEachOverrideBackend(t, 0, func(t *testing.T, device *wgpu.Device) {
		r := newTargetRig(t, device, gputypes.TextureDimension3D, sliceRigDepth)
		one, two := uint32(1), uint32(2)
		r.render(t, 0, &one, sliceRed)
		view := r.newView(t, 0)
		t.Cleanup(view.Release)
		enc, err := device.CreateCommandEncoder(nil)
		if err != nil {
			t.Fatalf("CreateCommandEncoder: %v", err)
		}
		pass, err := enc.BeginRenderPass(&wgpu.RenderPassDescriptor{
			ColorAttachments: []wgpu.RenderPassColorAttachment{{
				View: view, DepthSlice: &two,
				LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore, ClearValue: sliceBlue,
			}},
		})
		if err != nil {
			t.Fatalf("BeginRenderPass: %v", err)
		}
		if err := pass.End(); err != nil {
			t.Fatalf("End: %v", err)
		}
		cmd, err := enc.Finish()
		if err != nil {
			t.Fatalf("Finish: %v", err)
		}
		if _, err := device.Queue().Submit(cmd); err != nil {
			t.Fatalf("Submit: %v", err)
		}
		r.check(t, 0, 1, sliceRed)
		r.checkSolid(t, 0, 2, sliceBlue)
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
