//go:build rust

package resource

import (
	"sync/atomic"

	rwgpu "github.com/go-webgpu/webgpu/wgpu"
	"github.com/gogpu/gputypes"
)

// Texture represents a GPU texture.
// On Rust backend, this wraps go-webgpu/webgpu Texture.
type Texture struct {
	r      *rwgpu.Texture
	format gputypes.TextureFormat
	// released is the CAS guard that admits exactly one destroy: concurrent
	// Release calls see CompareAndSwap succeed for one caller only. It is nil
	// only on an unguarded zero value, whose Release is a no-op.
	released *atomic.Bool

	// Pub caches the public wrapper (wgpu.Texture) built over this texture; see
	// the native Texture.Pub for the identity and allocation contract.
	Pub any
}

// NewTexture builds a texture over a wgpu-native texture object.
func NewTexture(r *rwgpu.Texture, format gputypes.TextureFormat) *Texture {
	return &Texture{r: r, format: format, released: new(atomic.Bool)}
}

// NewDetachedTexture builds a texture that carries only the descriptor's
// format: no backend object. It backs wgpu's detached-texture factory.
func NewDetachedTexture(format gputypes.TextureFormat) *Texture {
	return &Texture{format: format, released: new(atomic.Bool)}
}

// Rust returns the wgpu-native texture object, or nil.
func (t *Texture) Rust() *rwgpu.Texture {
	if t == nil {
		return nil
	}
	return t.r
}

// Format returns the texture format.
func (t *Texture) Format() gputypes.TextureFormat { return t.format }

// Release destroys the texture.
func (t *Texture) Release() {
	if t.released == nil || !t.released.CompareAndSwap(false, true) {
		return
	}
	if t.r != nil {
		t.r.Release()
	}
}

// TextureView represents a view into a texture.
// On Rust backend, this wraps go-webgpu/webgpu TextureView.
type TextureView struct {
	r       *rwgpu.TextureView
	texture *Texture
	// released is the CAS guard that admits exactly one destroy: concurrent
	// Release calls see CompareAndSwap succeed for one caller only. It is nil
	// only on an unguarded zero value, whose Release is a no-op.
	released *atomic.Bool
}

// NewTextureView builds a texture view over a wgpu-native texture view object.
func NewTextureView(v *rwgpu.TextureView, parent *Texture) *TextureView {
	return &TextureView{r: v, texture: parent, released: new(atomic.Bool)}
}

// Rust returns the wgpu-native texture view object, or nil.
func (v *TextureView) Rust() *rwgpu.TextureView {
	if v == nil {
		return nil
	}
	return v.r
}

// Parent returns the parent Texture this view was created from, without the
// released guard (the rust build reads it directly, as before the move).
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
	if v.released == nil || !v.released.CompareAndSwap(false, true) {
		return
	}
	if v.r != nil {
		v.r.Release()
	}
}
