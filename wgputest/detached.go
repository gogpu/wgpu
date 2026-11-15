// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgputest

import (
	"fmt"

	"github.com/gogpu/wgpu"
	"github.com/gogpu/wgpu/wgputest/internal/device"
)

// NewDetachedTexture returns a *wgpu.Texture that carries the descriptor's
// metadata and no backend object. Logic that only inspects the handle — an
// identity check, a map key, a format read — works unchanged. Any operation
// that needs the GPU fails with the validation error wgpu reports for a
// texture without a HAL handle (it never panics), and Release is a no-op.
// It replaces the hand-written hal.Texture stubs that consumers built through
// NewTextureFromHAL before the HAL became internal (ADR-070).
//
// desc may be nil: the handle then carries zero metadata.
func NewDetachedTexture(desc *wgpu.TextureDescriptor) *wgpu.Texture {
	t, err := device.NewDetachedTexture(desc)
	if err != nil {
		// The root package registers the factory from init, so a missing one
		// is a broken build, not a runtime condition a test can act on.
		panic(fmt.Sprintf("wgputest: NewDetachedTexture: %v", err))
	}
	return t
}
