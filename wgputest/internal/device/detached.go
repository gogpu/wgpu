// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package device

import (
	"fmt"

	"github.com/gogpu/wgpu"
	"github.com/gogpu/wgpu/internal/wgputestbridge"
)

// NewDetachedTexture builds a *wgpu.Texture that carries the descriptor's
// metadata and no backend object. The error is the bridge's: a missing factory
// is a broken build, and the public constructor turns it into a panic.
func NewDetachedTexture(desc *wgpu.TextureDescriptor) (*wgpu.Texture, error) {
	value, err := wgputestbridge.NewTexture(desc)
	if err != nil {
		return nil, err
	}
	t, ok := value.(*wgpu.Texture)
	if !ok {
		return nil, fmt.Errorf("internal error: factory returned %T", value)
	}
	return t, nil
}
