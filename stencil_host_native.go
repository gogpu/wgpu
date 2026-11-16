//go:build !rust && !(js && wasm)

package wgpu

import (
	"github.com/gogpu/wgpu/internal/resource"
	"github.com/gogpu/wgpu/internal/stencilblit"
)

// stencilDeviceHost adapts *Device to the stencil upload payload's creation
// seam (ADR-070 §15.1: the payload only; the queue and the encoder record the
// commands). The pipeline cache stays here — root-owned device state
// (core.StencilBlitResources), because it holds the public wrappers that own
// the pipelines' references — so Pipelines returns the internal handles the
// layer passes draw with. The four creation methods convert the internal
// descriptor mirrors field by field: the wrappers are thin handles, so the
// conversion is the handle itself, never a copy of resource state.
//
// The payload still lives in this package (S9's hand-back reaches commit 4);
// this host is the seam the creation moves to, and the payload is created
// through it once it moves, so nothing calls it yet.
type stencilDeviceHost struct{ d *Device }

var _ stencilblit.Device = stencilDeviceHost{}

// newStencilDeviceHost returns the payload's view of d. Both device hosts in
// this package (the encoder's and the queue's) hand the payload the same
// adapter, so the construction lives here.
func newStencilDeviceHost(d *Device) stencilDeviceHost { return stencilDeviceHost{d: d} }

// Pipelines returns the blit pipelines of key and the layout of their bind
// groups from the device's cache, building them on first use.
func (h stencilDeviceHost) Pipelines(key stencilblit.Key) (stencilblit.Blit, error) {
	return h.d.pipelines(key)
}

// CreateTexture creates the texture desc describes through the public
// creation path (validation, defaults and the core tracker included) and
// returns the implementation handle.
func (h stencilDeviceHost) CreateTexture(desc stencilblit.TextureDescriptor) (*resource.Texture, error) {
	tex, err := h.d.CreateTexture(&TextureDescriptor{
		Label:         desc.Label,
		Size:          Extent3D(desc.Size),
		MipLevelCount: desc.MipLevelCount,
		SampleCount:   desc.SampleCount,
		Dimension:     desc.Dimension,
		Format:        desc.Format,
		Usage:         desc.Usage,
	})
	if err != nil {
		return nil, err
	}
	return tex.impl, nil
}

// CreateTextureView creates a view of texture. The view carries no public
// owner until the caller hands it to a command buffer, so a fresh thin
// wrapper is the handle.
func (h stencilDeviceHost) CreateTextureView(texture *resource.Texture, desc stencilblit.TextureViewDescriptor) (*resource.TextureView, error) {
	view, err := h.d.CreateTextureView(texturePub(texture), &TextureViewDescriptor{
		Label:           desc.Label,
		Format:          desc.Format,
		Dimension:       desc.Dimension,
		Aspect:          desc.Aspect,
		BaseMipLevel:    desc.BaseMipLevel,
		MipLevelCount:   desc.MipLevelCount,
		BaseArrayLayer:  desc.BaseArrayLayer,
		ArrayLayerCount: desc.ArrayLayerCount,
	})
	if err != nil {
		return nil, err
	}
	return view.impl, nil
}

// CreateBuffer creates a buffer through the public creation path and returns
// the implementation handle.
func (h stencilDeviceHost) CreateBuffer(desc stencilblit.BufferDescriptor) (*resource.Buffer, error) {
	buf, err := h.d.CreateBuffer(&BufferDescriptor{
		Label:            desc.Label,
		Size:             desc.Size,
		Usage:            desc.Usage,
		MappedAtCreation: desc.MappedAtCreation,
	})
	if err != nil {
		return nil, err
	}
	return buf.impl, nil
}

// CreateBindGroup creates a bind group over desc.Layout through the public
// creation path (validation and the tracking references included) and returns
// the implementation handle. The layout and the entries are thin wrappers over
// the handles the payload holds.
func (h stencilDeviceHost) CreateBindGroup(desc stencilblit.BindGroupDescriptor) (*resource.BindGroup, error) {
	entries := make([]BindGroupEntry, len(desc.Entries))
	for i, e := range desc.Entries {
		entries[i] = BindGroupEntry{
			Binding:     e.Binding,
			Buffer:      bufferPub(e.Buffer),
			Offset:      e.Offset,
			Size:        e.Size,
			TextureView: textureViewPub(e.TextureView),
		}
	}
	group, err := h.d.CreateBindGroup(&BindGroupDescriptor{
		Label:   desc.Label,
		Layout:  &BindGroupLayout{impl: desc.Layout},
		Entries: entries,
	})
	if err != nil {
		return nil, err
	}
	return group.impl, nil
}

// bufferPub returns a thin public handle over b, or nil for an absent binding.
func bufferPub(b *resource.Buffer) *Buffer {
	if b == nil {
		return nil
	}
	return &Buffer{impl: b}
}

// textureViewPub returns a thin public handle over v, or nil for an absent
// binding.
func textureViewPub(v *resource.TextureView) *TextureView {
	if v == nil {
		return nil
	}
	return &TextureView{impl: v}
}
