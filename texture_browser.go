//go:build js && wasm

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/resource"
)

// Texture represents a GPU texture.
//
// The implementation lives in internal/resource (ADR-070). This declaration is
// the public surface: every method is one delegation.
type Texture struct {
	impl *resource.Texture
}

// Format returns the texture format.
func (t *Texture) Format() gputypes.TextureFormat {
	if t.impl == nil {
		return gputypes.TextureFormatUndefined
	}
	return t.impl.Format()
}

// Release destroys the texture.
func (t *Texture) Release() {
	if t.impl == nil {
		return
	}
	t.impl.Release()
}

// TextureView represents a view into a texture.
type TextureView struct {
	impl *resource.TextureView
}

// Texture returns the parent Texture this view was created from. The browser
// build reads it directly: a released view still returns its texture.
func (v *TextureView) Texture() *Texture {
	if v.impl == nil {
		return nil
	}
	parent := v.impl.Texture()
	if parent == nil {
		return nil
	}
	wrapper, _ := parent.Pub.(*Texture)
	return wrapper
}

// Release marks the texture view for destruction.
func (v *TextureView) Release() {
	if v.impl == nil {
		return
	}
	v.impl.Release()
}
