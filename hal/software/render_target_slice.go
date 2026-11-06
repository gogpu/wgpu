// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package software

import (
	"sync"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// otherMipPassLogged keeps the log of render passes on a mip level above 0 to
// one line per process, like otherMipLogged for transfers.
var otherMipPassLogged sync.Once

// attachmentTarget returns the texture a render pass writes for an attachment
// view, or nil when it writes nothing. Only mip level 0 is stored (P1-SW-026),
// so a view whose base mip level is above 0 has no storage to render into:
// the pass leaves it, and mip level 0, untouched, as transfers of other mip
// levels are skipped (skipOtherMip).
func attachmentTarget(v hal.TextureView) *Texture {
	view, ok := v.(*TextureView)
	if !ok || view.texture == nil {
		return nil
	}
	if view.baseMipLevel != 0 {
		otherMipPassLogged.Do(func() {
			hal.Logger().Debug("software: render pass on a mip level above 0 skipped: only mip level 0 is stored (P1-SW-026)",
				"mipLevel", view.baseMipLevel)
		})
		return nil
	}
	return view.texture
}

// colorSliceTarget is the pass-local 2D target of a color attachment on one
// depth slice of a 3D texture (GPURenderPassColorAttachment.depthSlice). The
// rasterizer writes a whole 2D texture, so BeginRenderPass loads the slice
// into tex, every clear and draw of the pass goes to tex, and End stores it
// back into the slice, as the depth attachment is loaded and stored
// (loadDepthAttachment, storeDepthAttachment). A clear therefore reaches only
// that slice. Only mip level 0 is stored (P1-SW-026), so the slice is one of
// level 0.
type colorSliceTarget struct {
	parent *Texture
	offset int // byte offset of the slice in parent.data
	store  bool
	tex    *Texture
}

// loadColorSliceTarget copies depth slice slice of parent into a new 2D
// texture of the same width, height and format. It returns nil when the
// texture has no such slice; core validation rejects that, so it is only
// logged.
func loadColorSliceTarget(parent *Texture, slice uint32, storeOp gputypes.StoreOp) *colorSliceTarget {
	sliceSize := int(parent.width) * int(parent.height) * int(formatBytesPerPixel(parent.format))
	parent.mu.RLock()
	defer parent.mu.RUnlock()
	offset := int(slice) * sliceSize
	if slice >= parent.depth || offset+sliceSize > len(parent.data) {
		hal.Logger().Error("software: color attachment depth slice out of range",
			"depthSlice", slice, "depth", parent.depth)
		return nil
	}
	data := make([]byte, sliceSize)
	copy(data, parent.data[offset:offset+sliceSize])
	return &colorSliceTarget{
		parent: parent,
		offset: offset,
		store:  storeOp == gputypes.StoreOpStore,
		tex: &Texture{
			id:            nextResourceID.Add(1),
			data:          data,
			width:         parent.width,
			height:        parent.height,
			depth:         1,
			format:        parent.format,
			usage:         parent.usage,
			mipLevelCount: 1,
			sampleCount:   1,
		},
	}
}

// storeBack writes the pass's target back into its depth slice with
// StoreOpStore; with StoreOpDiscard the slice keeps its old contents.
func (s *colorSliceTarget) storeBack() {
	if !s.store {
		return
	}
	s.tex.mu.RLock()
	defer s.tex.mu.RUnlock()
	s.parent.mu.Lock()
	defer s.parent.mu.Unlock()
	copy(s.parent.data[s.offset:s.offset+len(s.tex.data)], s.tex.data)
}

// resolveColorTargets sets the texture each color attachment of the pass
// renders into (BeginRenderPass): the view's texture, a depth slice target,
// or nil when the pass writes nothing for that attachment.
func (r *RenderPassEncoder) resolveColorTargets() {
	attachments := r.desc.ColorAttachments
	r.colorTargets = make([]*Texture, len(attachments))
	for i, ca := range attachments {
		tex := attachmentTarget(ca.View)
		if tex == nil || ca.DepthSlice == nil {
			r.colorTargets[i] = tex
			continue
		}
		slice := loadColorSliceTarget(tex, *ca.DepthSlice, ca.StoreOp)
		if slice == nil {
			continue
		}
		r.sliceTargets = append(r.sliceTargets, slice)
		r.colorTargets[i] = slice.tex
	}
}

// storeColorSliceTargets writes every depth slice target back (End).
func (r *RenderPassEncoder) storeColorSliceTargets() {
	for _, s := range r.sliceTargets {
		s.storeBack()
	}
}
