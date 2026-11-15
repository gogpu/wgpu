//go:build rust

package wgpu

import (
	rwgpu "github.com/go-webgpu/webgpu/wgpu"
	"github.com/gogpu/gputypes"
)

// Texture represents a GPU texture.
// On Rust backend, this wraps go-webgpu/webgpu Texture.
type Texture struct {
	r        *rwgpu.Texture
	device   *Device
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
	if t.r != nil {
		t.r.Release()
	}
}

// TextureView represents a view into a texture.
// On Rust backend, this wraps go-webgpu/webgpu TextureView.
type TextureView struct {
	r        *rwgpu.TextureView
	device   *Device
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
	if v.r != nil {
		v.r.Release()
	}
}
