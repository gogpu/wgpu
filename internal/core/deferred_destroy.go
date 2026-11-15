//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package core

import (
	"github.com/gogpu/wgpu/internal/core/track"
	"github.com/gogpu/wgpu/internal/hal"
)

// DestroyTextureDeferred destroys a HAL texture once the device has completed
// the last submission so far, and frees its tracking index with it.
//
// It lives in core because both sides of the resource extraction need it: the
// moving texture type calls it from Release, and the frontend device calls it
// from the reference closures it installs (ADR-070 §11, S2). It touches only
// core's own seam surface (DeviceView, DestroyQueue, track) and hal handles.
//
// ADR-060: releasing the TrackerIndex prevents monotonic growth (#307);
// surface textures handle this in Surface.destroySwapchainTexture. The index
// is freed together with the HAL texture, after the GPU is done with it, as
// Rust frees it in Drop for TrackingData once the last Arc (including
// in-flight submissions) is gone (wgpu-core resource.rs).
func DestroyTextureDeferred(view DeviceView, halTex hal.Texture, td *track.TrackingData) {
	releaseIndex := func() {
		if td != nil {
			td.Release()
		}
	}

	halDevice := view.HALDevice()
	if halDevice == nil {
		releaseIndex()
		return
	}

	dq := view.DestroyQueue()
	if dq == nil {
		halDevice.DestroyTexture(halTex)
		releaseIndex()
		return
	}

	dq.Defer(view.LastSubmissionIndex(), "Texture", func() {
		halDevice.DestroyTexture(halTex)
		releaseIndex()
	})
}

// DestroyTextureViewDeferred destroys a HAL texture view once the device has
// completed the last submission so far. It lives in core for the reason
// DestroyTextureDeferred does.
func DestroyTextureViewDeferred(view DeviceView, halView hal.TextureView) {
	halDevice := view.HALDevice()
	if halDevice == nil {
		return
	}

	dq := view.DestroyQueue()
	if dq == nil {
		halDevice.DestroyTextureView(halView)
		return
	}

	dq.Defer(view.LastSubmissionIndex(), "TextureView", func() {
		halDevice.DestroyTextureView(halView)
	})
}
