// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package software

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// attachmentSubresource returns the texture of an attachment view and the
// subresource a render pass writes in it: the view's base mip level, and
// the depth slice depthSlice of a 3D texture or the view's base array layer
// otherwise. ok is false for a view without a software texture.
func attachmentSubresource(v hal.TextureView, depthSlice *uint32) (tex *Texture, level, z uint32, ok bool) {
	view, isView := v.(*TextureView)
	if !isView || view.texture == nil {
		return nil, 0, 0, false
	}
	z = view.baseArrayLayer
	if view.texture.is3D() {
		z = 0
		if depthSlice != nil {
			z = *depthSlice
		}
	}
	return view.texture, view.baseMipLevel, z, true
}

// subresourceTarget is the pass-local 2D target of a color attachment that
// renders into one subresource of a texture with several: a mip level, an
// array layer or a depth slice (GPURenderPassColorAttachment.depthSlice).
// The rasterizer and Texture.Clear write a whole 2D texture, so
// BeginRenderPass loads the subresource's image into tex, every clear and
// draw of the pass goes to tex, and End stores it back, as the depth
// attachment is loaded and stored (loadDepthAttachment,
// storeDepthAttachment). A clear therefore reaches only that subresource. A
// texture that is a single image (the surface texture among them) is
// rendered into directly.
type subresourceTarget struct {
	parent   *Texture
	level, z uint32
	store    bool
	tex      *Texture
}

// loadSubresourceTarget copies image z of mip level level of parent into a
// new 2D texture of the level's size and the parent's format. It returns nil
// when the texture has no such image; core validation rejects that, so it
// is only logged.
func loadSubresourceTarget(parent *Texture, level, z uint32, storeOp gputypes.StoreOp) *subresourceTarget {
	parent.mu.RLock()
	defer parent.mu.RUnlock()
	img, ok := parent.image(level, z)
	if !ok {
		_, _, images := parent.levelExtent(level)
		hal.Logger().Error("software: render attachment subresource out of range",
			"mipLevel", level, "mipLevels", parent.mipLevels(), "layerOrSlice", z, "images", images)
		return nil
	}
	data := make([]byte, len(img.data))
	copy(data, img.data)
	return &subresourceTarget{
		parent: parent,
		level:  level,
		z:      z,
		store:  storeOp == gputypes.StoreOpStore,
		tex: &Texture{
			id:            nextResourceID.Add(1),
			data:          data,
			width:         img.width,
			height:        img.height,
			depth:         1,
			dimension:     gputypes.TextureDimension2D,
			format:        parent.format,
			usage:         parent.usage,
			mipLevelCount: 1,
			sampleCount:   1,
		},
	}
}

// storeBack writes the pass's target back into its subresource with
// StoreOpStore; with StoreOpDiscard the subresource keeps its old contents.
func (s *subresourceTarget) storeBack() {
	if !s.store {
		return
	}
	s.tex.mu.RLock()
	defer s.tex.mu.RUnlock()
	s.parent.mu.Lock()
	defer s.parent.mu.Unlock()
	if img, ok := s.parent.image(s.level, s.z); ok {
		copy(img.data, s.tex.data)
	}
}

// resolveColorTargets sets the texture each color attachment of the pass
// renders into (BeginRenderPass): the view's texture when it is a single
// image, a subresource target otherwise, or nil when the pass writes nothing
// for that attachment.
func (r *RenderPassEncoder) resolveColorTargets() {
	attachments := r.desc.ColorAttachments
	r.colorTargets = make([]*Texture, len(attachments))
	r.colorFormats = make([]gputypes.TextureFormat, len(attachments))
	for i, ca := range attachments {
		tex, level, z, ok := attachmentSubresource(ca.View, ca.DepthSlice)
		if !ok {
			continue
		}
		if view, isView := ca.View.(*TextureView); isView {
			r.colorFormats[i] = view.viewFormat()
		}
		if tex.isSingleSubresource() && level == 0 && z == 0 {
			r.colorTargets[i] = tex
			continue
		}
		sub := loadSubresourceTarget(tex, level, z, ca.StoreOp)
		if sub == nil {
			continue
		}
		r.subTargets = append(r.subTargets, sub)
		r.colorTargets[i] = sub.tex
	}
}

// resolveInto copies the pass's color target src into the subresource of
// the resolve target view (End). The software backend has no multisampling,
// so the resolve is a copy of an image of the same size.
func resolveInto(src *Texture, resolveView hal.TextureView) {
	dst, level, z, ok := attachmentSubresource(resolveView, nil)
	if src == nil || !ok || dst == src {
		return
	}
	src.mu.RLock()
	defer src.mu.RUnlock()
	dst.mu.Lock()
	defer dst.mu.Unlock()
	srcImg, srcOK := src.image(0, 0)
	dstImg, dstOK := dst.image(level, z)
	if srcOK && dstOK && len(srcImg.data) == len(dstImg.data) {
		copy(dstImg.data, srcImg.data)
	}
}

// storeSubresourceTargets writes every subresource target back (End).
func (r *RenderPassEncoder) storeSubresourceTargets() {
	for _, s := range r.subTargets {
		s.storeBack()
	}
}

// depthAttachmentImage returns the image of the depth/stencil attachment's
// view: its base mip level and base array layer. ok is false without one.
func depthAttachmentImage(ds *hal.RenderPassDepthStencilAttachment) (tex *Texture, level, layer uint32, ok bool) {
	if ds == nil {
		return nil, 0, 0, false
	}
	return attachmentSubresource(ds.View, nil)
}
