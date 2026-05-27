//go:build rust

package wgpu

import rwgpu "github.com/go-webgpu/webgpu/wgpu"

// Texture represents a GPU texture.
// On Rust backend, this wraps go-webgpu/webgpu Texture.
type Texture struct {
	r        *rwgpu.Texture
	device   *Device
	format   TextureFormat
	released bool
}

// Format returns the texture format.
func (t *Texture) Format() TextureFormat { return t.format }

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
