//go:build js && wasm

package wgpu

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/browser"
)

// Texture represents a GPU texture.
type Texture struct {
	browser  *browser.Texture
	format   gputypes.TextureFormat
	released bool
}

// Format returns the texture format.
func (t *Texture) Format() gputypes.TextureFormat { return t.format }

// newDetachedTexture returns a texture that carries the descriptor's format
// and no backend object. It backs the public wgputest.NewDetachedTexture; see
// newDetachedTextureFactory. desc may be nil.
func newDetachedTexture(desc *TextureDescriptor) *Texture {
	t := &Texture{}
	if desc != nil {
		t.format = desc.Format
	}
	return t
}

// Release destroys the texture.
func (t *Texture) Release() {
	if t.released {
		return
	}
	t.released = true
	if t.browser != nil {
		t.browser.Destroy()
	}
}

// TextureView represents a view into a texture.
type TextureView struct {
	browser  *browser.TextureView
	texture  *Texture
	released bool
}

// Texture returns the parent Texture that this view was created from.
// Returns nil if the view has been released.
func (v *TextureView) Texture() *Texture { return v.texture }

// Release marks the texture view for destruction.
func (v *TextureView) Release() {
	if v.released {
		return
	}
	v.released = true
}
