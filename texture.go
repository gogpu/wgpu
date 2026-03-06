package wgpu

import (
	"github.com/gogpu/wgpu/hal"
)

// Texture represents a GPU texture.
type Texture struct {
	hal      hal.Texture
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
	halDevice := t.device.halDevice()
	if halDevice != nil {
		halDevice.DestroyTexture(t.hal)
	}
}

// TextureView represents a view into a texture.
type TextureView struct {
	hal      hal.TextureView
	device   *Device
	texture  *Texture
	released bool
}

// Release destroys the texture view.
func (v *TextureView) Release() {
	if v.released {
		return
	}
	v.released = true
	halDevice := v.device.halDevice()
	if halDevice != nil {
		halDevice.DestroyTextureView(v.hal)
	}
}

// ImageCopyTexture describes a texture subresource and origin for write operations.
type ImageCopyTexture struct {
	Texture  *Texture
	MipLevel uint32
	Origin   Origin3D
	Aspect   TextureAspect
}

func (i *ImageCopyTexture) toHAL() *hal.ImageCopyTexture {
	if i == nil || i.Texture == nil {
		return nil
	}

	return &hal.ImageCopyTexture{
		Texture:  i.Texture.hal,
		MipLevel: i.MipLevel,
		Origin: hal.Origin3D{
			X: i.Origin.X,
			Y: i.Origin.Y,
			Z: i.Origin.Z,
		},
		Aspect: i.Aspect,
	}
}

func textureDataLayoutToHAL(t *TextureDataLayout) *hal.ImageDataLayout {
	if t == nil {
		return nil
	}

	return &hal.ImageDataLayout{
		Offset:       t.Offset,
		BytesPerRow:  t.BytesPerRow,
		RowsPerImage: t.RowsPerImage,
	}
}
