//go:build !rust && !(js && wasm)

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
// the public surface: every method is one delegation. The file is per context,
// like the implementation it fronts; its siblings (browser, rust) declare the
// same surface so a later step can let them diverge without moving the type.
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

// Release drops the application's reference to the texture; the HAL texture is
// destroyed once nothing references it any more. A zero-value Texture has no
// implementation and Release is a no-op, as it was before the move.
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

// Texture returns the parent Texture this view was created from, the same
// public wrapper the device handed out. Returns nil if the view has been
// released (native behavior; the browser and rust builds read it directly).
func (v *TextureView) Texture() *Texture {
	if v == nil || v.impl == nil {
		return nil
	}
	parent := v.impl.Texture()
	if parent == nil {
		return nil
	}
	wrapper, _ := parent.Pub.(*Texture)
	return wrapper
}

// Release drops the application's reference to the texture view.
func (v *TextureView) Release() {
	if v.impl == nil {
		return
	}
	v.impl.Release()
}
