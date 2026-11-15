//go:build js && wasm

package resource

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/browser"
)

// Texture represents a GPU texture.
type Texture struct {
	browser  *browser.Texture
	format   gputypes.TextureFormat
	released bool

	// Pub caches the public wrapper (wgpu.Texture) built over this texture; see
	// the native Texture.Pub for the identity and allocation contract.
	Pub any
}

// NewTexture builds a texture over a browser texture object.
func NewTexture(b *browser.Texture, format gputypes.TextureFormat) *Texture {
	return &Texture{browser: b, format: format}
}

// NewDetachedTexture builds a texture that carries only the descriptor's
// format: no backend object. It backs wgpu's detached-texture factory.
func NewDetachedTexture(format gputypes.TextureFormat) *Texture {
	return &Texture{format: format}
}

// Browser returns the browser texture object, or nil.
func (t *Texture) Browser() *browser.Texture {
	if t == nil {
		return nil
	}
	return t.browser
}

// Format returns the texture format.
func (t *Texture) Format() gputypes.TextureFormat { return t.format }

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

// NewTextureView builds a texture view over a browser texture view object.
func NewTextureView(v *browser.TextureView, parent *Texture) *TextureView {
	return &TextureView{browser: v, texture: parent}
}

// Browser returns the browser texture view object, or nil.
func (v *TextureView) Browser() *browser.TextureView {
	if v == nil {
		return nil
	}
	return v.browser
}

// Parent returns the parent Texture this view was created from, without the
// released guard (the browser build reads it directly, as before the move).
func (v *TextureView) Parent() *Texture {
	if v == nil {
		return nil
	}
	return v.texture
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
