//go:build js && wasm

package wgpu

// Texture represents a GPU texture.
type Texture struct {
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
}

// TextureView represents a view into a texture.
type TextureView struct {
	released bool
}

// Release marks the texture view for destruction.
func (v *TextureView) Release() {
	if v.released {
		return
	}
	v.released = true
}
