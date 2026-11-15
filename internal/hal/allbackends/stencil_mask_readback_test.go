// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// TestStencilMasksZeroAreLiteral: the stencil read and write masks are taken
// literally. The stencil test runs whenever a face is not "compare always, ops
// keep", whatever the masks; with a zero read mask it compares 0 with 0
// (index.bs "stencil test": the reference and the stored value are both
// ANDed with stencilReadMask). Vulkan enabled the test only for a non-zero
// mask, as Rust wgpu does, and drew every fragment.
func TestStencilMasksZeroAreLiteral(t *testing.T) {
	forEachOverrideBackend(t, 0, func(t *testing.T, device *wgpu.Device) {
		t.Run("both-zero-not-equal-fails", func(t *testing.T) {
			// (1 & 0) != (0 & 0) is false: every fragment fails the test.
			drawZeroDefaults(t, device, stencilMaskCase(gputypes.CompareFunctionNotEqual, 0, 0, true))
		})
		t.Run("read-zero-equal-passes", func(t *testing.T) {
			// (1 & 0) == (0 & 0): every fragment passes, although the
			// reference 1 differs from the cleared stencil 0.
			drawZeroDefaults(t, device, stencilMaskCase(gputypes.CompareFunctionEqual, 0, 0xFF, false))
		})
		t.Run("read-full-equal-fails", func(t *testing.T) {
			// Control: with the full read mask, 1 == 0 fails.
			drawZeroDefaults(t, device, stencilMaskCase(gputypes.CompareFunctionEqual, 0xFF, 0xFF, true))
		})
	})
}

// stencilMaskCase draws with reference 1 into a stencil cleared to 0, depth
// test off, both faces using compare and keeping the stencil value.
func stencilMaskCase(compare gputypes.CompareFunction, readMask, writeMask uint32, wantCleared bool) zeroDefaultsCase {
	face := wgpu.StencilFaceState{
		Compare: compare,
		FailOp:  wgpu.StencilOperationKeep, DepthFailOp: wgpu.StencilOperationKeep, PassOp: wgpu.StencilOperationKeep,
	}
	return zeroDefaultsCase{
		depthFormat: gputypes.TextureFormatDepth24PlusStencil8,
		depthStencil: &wgpu.DepthStencilState{
			Format:          gputypes.TextureFormatDepth24PlusStencil8,
			DepthCompare:    gputypes.CompareFunctionAlways,
			StencilFront:    face,
			StencilBack:     face,
			StencilReadMask: readMask, StencilWriteMask: writeMask,
		},
		depthClear:  1,
		stencilRef:  1,
		wantCleared: wantCleared,
	}
}
