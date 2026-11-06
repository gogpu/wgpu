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

// TestRenderToArrayLayer renders into array layer k of a 2D texture with 4
// layers, through a 2d-array view and a 2d view whose baseArrayLayer is k,
// at mip 0 and mip 1. Every layer is first rendered over a blue clear; the
// pass on layer k clears red. Layer k must hold the red pass and every
// other layer the blue one: the pass targets the view's layer, not layer 0,
// and touches no other layer.
func TestRenderToArrayLayer(t *testing.T) {
	const layers = 4
	forEachOverrideBackend(t, 0, func(t *testing.T, device *wgpu.Device) {
		for _, dim := range []gputypes.TextureViewDimension{
			gputypes.TextureViewDimension2DArray, gputypes.TextureViewDimension2D,
		} {
			for _, mip := range []uint32{0, 1} {
				for _, k := range []uint32{0, 2, layers - 1} {
					t.Run(fmt.Sprintf("%s/mip%d/layer%d", dim, mip, k), func(t *testing.T) {
						r := newTargetRig(t, device, gputypes.TextureDimension2D, layers)
						for j := uint32(0); j < layers; j++ {
							r.renderView(t, r.newLayerView(t, dim, mip, j), nil, sliceBlue)
						}
						r.renderView(t, r.newLayerView(t, dim, mip, k), nil, sliceRed)
						for j := uint32(0); j < layers; j++ {
							want := sliceBlue
							if j == k {
								want = sliceRed
							}
							r.check(t, mip, j, want)
						}
					})
				}
			}
		}
	})
}
