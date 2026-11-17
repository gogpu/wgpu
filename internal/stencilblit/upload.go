//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package stencilblit

// This file is the upload payload: one prepared upload into the stencil aspect
// of a combined depth-stencil texture, the one call that creates it, and the
// seam and contract types it is built and recorded through.
//
// The payload arrived here from the root package at S9 (ADR-070 §15.1: this
// package holds the upload payload only; the queue and the encoder record the
// commands). Its contracts were declared here from the S4 Move and are back
// with it: the encoder imports this package for its recorder adapter
// (internal/encoder/stencil_recorder.go), and this package imports neither the
// root package nor internal/encoder. The pipeline cache stays in the root
// package: it is device-owned state and holds the public wrappers that own the
// pipelines' references, so the seam below hands the payload only the internal
// handles (Device, Blit).

import (
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/hal"
	"github.com/gogpu/wgpu/internal/pipeline"
	"github.com/gogpu/wgpu/internal/resource"
)

// paramsStride is the distance between the Params of two layers in the blit's
// uniform buffer: 256, the largest minUniformBufferOffsetAlignment the WebGPU
// spec allows, so a multiple of every device's limit.
const paramsStride = 256

// uploadLabel is the debug label of the payload's objects.
const uploadLabel = "stencil upload"

// Device is the upload payload's creation seam: the pipelines of a Key and the
// four resource creations the blit's commands need. The pipeline cache and the
// label of every object stay in the root package (the cache is device-owned
// state, core.StencilBlitResources, and holds the public wrappers that own the
// pipelines' references); this seam receives the internal handles and the
// stored blit state — the nine pipelines and the bind group layout their
// groups are built on.
type Device interface {
	// Pipelines returns the blit pipelines of key and the layout of their bind
	// groups, building them on first use. The cache belongs to the device and
	// is disposed with it.
	Pipelines(key Key) (Blit, error)
	// CreateTexture creates a texture. The staging texture of an upload is
	// created through it.
	CreateTexture(desc TextureDescriptor) (*resource.Texture, error)
	// CreateTextureView creates a view of texture.
	CreateTextureView(texture *resource.Texture, desc TextureViewDescriptor) (*resource.TextureView, error)
	// CreateBuffer creates a GPU buffer. A MappedAtCreation buffer is mapped
	// when the call returns, as the public creation path's is.
	CreateBuffer(desc BufferDescriptor) (*resource.Buffer, error)
	// CreateBindGroup creates a bind group over desc.Layout.
	CreateBindGroup(desc BindGroupDescriptor) (*resource.BindGroup, error)
}

// Blit is the cached state of one Key: the nine pipelines the layer draws run
// through (the clear pass plus one per stencil bit) and the bind group layout
// their groups are built on.
type Blit struct {
	Pipelines Pipelines[*pipeline.RenderPipeline]
	Layout    *resource.BindGroupLayout
}

// The descriptor mirrors of the creation seam. The public descriptors carry the
// public wrappers; the payload may name only internal types, so these mirrors
// declare the same shapes over the implementation handles (ADR-070 R9, the
// internal/encoder/descriptor.go precedent). A mirror carries exactly the
// fields the payload sets: the root host converts field by field at the call
// boundary, and a field that is absent cannot be silently dropped there.

// TextureDescriptor describes texture creation parameters.
type TextureDescriptor struct {
	Label         string
	Size          gputypes.Extent3D
	MipLevelCount uint32
	SampleCount   uint32
	Dimension     gputypes.TextureDimension
	Format        gputypes.TextureFormat
	Usage         gputypes.TextureUsage
}

// TextureViewDescriptor describes texture view creation parameters.
type TextureViewDescriptor struct {
	Label           string
	Format          gputypes.TextureFormat
	Dimension       gputypes.TextureViewDimension
	Aspect          gputypes.TextureAspect
	BaseMipLevel    uint32
	MipLevelCount   uint32
	BaseArrayLayer  uint32
	ArrayLayerCount uint32
}

// BufferDescriptor describes buffer creation parameters.
type BufferDescriptor struct {
	Label            string
	Size             uint64
	Usage            gputypes.BufferUsage
	MappedAtCreation bool
}

// BindGroupDescriptor describes bind group creation over an internal layout.
type BindGroupDescriptor struct {
	Label   string
	Layout  *resource.BindGroupLayout
	Entries []BindGroupEntry
}

// BindGroupEntry describes one binding of a bind group: a buffer range or a
// texture view.
type BindGroupEntry struct {
	Binding     uint32
	Buffer      *resource.Buffer
	Offset      uint64
	Size        uint64
	TextureView *resource.TextureView
}

// Recorder is the command encoder the blit records into, as the blit needs it.
type Recorder interface {
	// Own hands a resource the blit created to the encoder, which releases it
	// once the command buffer it belongs to is spent (submitted or released).
	Own(r Releasable)
	// BeginLayerPass opens the render pass of one destination layer, binds
	// group at bind group index 0, and returns the pass's command recorder.
	BeginLayerPass(desc *core.RenderPassDescriptor, group *resource.BindGroup) (PassRecorder, error)
}

// PassRecorder records the commands of one blit layer and ends its pass. Its
// draw verbs are the five RecordLayer drives over the internal pipeline type,
// plus End: a value of this interface type is what BeginLayerPass returns and
// what the caller ends after RecordLayer returns.
type PassRecorder interface {
	SetPipeline(pl *pipeline.RenderPipeline)
	SetStencilReference(reference uint32)
	SetScissorRect(x, y, width, height uint32)
	Draw(vertexCount, instanceCount, firstVertex, firstInstance uint32)
	// End ends the render pass.
	End() error
}

// Releasable is a resource that owns a reference and releases it on Release.
//
// It is also the encoder's own internal-resource contract: the resources the
// frontend creates for its commands (the stencil blit's staging texture,
// views, buffers and bind groups) have no application owner, so the command
// buffer releases them once it is spent. The encoder keeps its own list of
// these when it lowers a stencil copy, so the type lives here, with the
// payload that produces most of them.
type Releasable interface {
	Release()
}

// Upload is one upload into the stencil aspect of a combined depth-stencil
// texture, lowered as this package describes: the caller writes the stencil
// bytes into temp (an R8Uint texture of the copy extent), then Record draws
// them into dst.
//
// The device is captured at creation rather than read off the encoder: the
// payload records through seam values, and a device pointer is not something
// the caller's command encoder may hand it (the pre-move payload captured the
// root Device for the same reason).
type Upload struct {
	device   Device
	dst      *resource.Texture
	mipLevel uint32
	origin   gputypes.Origin3D
	size     gputypes.Extent3D
	temp     *resource.Texture
}

// NewUpload creates the R8Uint texture an upload of size into the stencil of
// dst at mipLevel and origin goes through, and returns the payload that
// records the blit from it.
func NewUpload(d Device, dst *resource.Texture, mipLevel uint32, origin gputypes.Origin3D, size gputypes.Extent3D) (*Upload, error) {
	temp, err := d.CreateTexture(TextureDescriptor{
		Label:         uploadLabel,
		Size:          size,
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatR8Uint,
		Usage:         gputypes.TextureUsageCopyDst | gputypes.TextureUsageTextureBinding,
	})
	if err != nil {
		return nil, fmt.Errorf("staging texture: %w", err)
	}
	return &Upload{device: d, dst: dst, mipLevel: mipLevel, origin: origin, size: size, temp: temp}, nil
}

// Staging returns the R8Uint texture the caller writes the stencil bytes into.
func (u *Upload) Staging() *resource.Texture { return u.temp }

// Record records into r, for each destination layer, the render pass that
// writes the layer's stencil from temp. Every pass it opens is ended. The
// resources it creates are handed to r (Recorder.Own); the caller hands it
// temp.
func (u *Upload) Record(r Recorder) error {
	d := u.device
	layers := u.size.DepthOrArrayLayers
	key := KeyFor(u.dst.Format(), layers)
	blit, err := d.Pipelines(key)
	if err != nil {
		return err
	}
	srcView, err := d.CreateTextureView(u.temp, TextureViewDescriptor{
		Label:           uploadLabel + " source",
		Format:          gputypes.TextureFormatR8Uint,
		Dimension:       key.ViewDimension,
		Aspect:          gputypes.TextureAspectAll,
		MipLevelCount:   1,
		ArrayLayerCount: layers,
	})
	if err != nil {
		return fmt.Errorf("source view: %w", err)
	}
	r.Own(srcView)
	params, err := u.paramsBuffer(layers)
	if err != nil {
		return err
	}
	r.Own(params)
	for layer := range layers {
		if err := u.recordLayer(r, &blit, srcView, params, layer); err != nil {
			return fmt.Errorf("layer %d: %w", u.origin.Z+layer, err)
		}
	}
	return nil
}

// paramsBuffer returns a uniform buffer with the Params of each layer at
// paramsStride. It is written through a mapping at creation, so no queue write
// is involved (Queue.WriteTexture records the blit with the queue lock held).
func (u *Upload) paramsBuffer(layers uint32) (*resource.Buffer, error) {
	size := uint64(layers-1)*paramsStride + ParamsSize
	buf, err := u.device.CreateBuffer(BufferDescriptor{
		Label:            uploadLabel + " params",
		Size:             size,
		Usage:            gputypes.BufferUsageUniform,
		MappedAtCreation: true,
	})
	if err != nil {
		return nil, fmt.Errorf("params buffer: %w", err)
	}
	rng, err := buf.MappedRange(0, size)
	if err != nil {
		buf.Release()
		return nil, fmt.Errorf("params buffer: %w", err)
	}
	data := rng.BytesMut()
	for layer := range layers {
		p := Params{OriginX: u.origin.X, OriginY: u.origin.Y, Layer: layer}.Bytes()
		copy(data[uint64(layer)*paramsStride:], p[:])
	}
	rng.Release()
	if err := buf.Unmap(); err != nil {
		buf.Release()
		return nil, fmt.Errorf("params buffer: %w", err)
	}
	return buf, nil
}

func (u *Upload) recordLayer(r Recorder, blit *Blit, srcView *resource.TextureView, params *resource.Buffer, layer uint32) error {
	group, err := u.device.CreateBindGroup(BindGroupDescriptor{
		Label:  uploadLabel,
		Layout: blit.Layout,
		Entries: []BindGroupEntry{
			{Binding: SourceBinding, TextureView: srcView},
			{
				Binding: ParamsBinding, Buffer: params,
				Offset: uint64(layer) * paramsStride, Size: ParamsSize,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("bind group: %w", err)
	}
	r.Own(group)
	dstView, err := u.device.CreateTextureView(u.dst, TextureViewDescriptor{
		Label:           uploadLabel + " destination",
		Format:          u.dst.Format(),
		Dimension:       gputypes.TextureViewDimension2D,
		Aspect:          gputypes.TextureAspectAll,
		BaseMipLevel:    u.mipLevel,
		MipLevelCount:   1,
		BaseArrayLayer:  u.origin.Z + layer,
		ArrayLayerCount: 1,
	})
	if err != nil {
		return fmt.Errorf("destination view: %w", err)
	}
	r.Own(dstView)
	// The destination needs only COPY_DST: rendering into it is the
	// frontend's own use (Dawn's internal usages). Every depth/stencil texture
	// is created render-attachable in the backend (Device.CreateTexture).
	dstView.SetInternalUsage(gputypes.TextureUsageRenderAttachment)

	// Depth is loaded and stored as it is; the pipelines neither test nor
	// write it. Stencil outside the scissor keeps its contents.
	pass, err := r.BeginLayerPass(uploadPassDescriptor(u.dst, dstView), group)
	if err != nil {
		return fmt.Errorf("render pass: %w", err)
	}
	RecordLayer[*pipeline.RenderPipeline](pass, &blit.Pipelines,
		u.origin.X, u.origin.Y, u.size.Width, u.size.Height)
	if err := pass.End(); err != nil {
		return fmt.Errorf("render pass: %w", err)
	}
	return nil
}

// uploadPassDescriptor builds the core descriptor of one blit layer's render
// pass: the layer's view of the destination, loading and storing depth and
// stencil. The destination texture is the authority for the attachment's HAL
// handle, like the public render pass conversion (a borrowed surface texture
// shares one core texture across swapchain images).
func uploadPassDescriptor(dst *resource.Texture, dstView *resource.TextureView) *core.RenderPassDescriptor {
	return &core.RenderPassDescriptor{
		Label: uploadLabel,
		ResolveAttachmentTexture: func(*core.TextureView) hal.Texture {
			return dst.ResolveHAL()
		},
		DepthStencilAttachment: &core.RenderPassDepthStencilAttachment{
			View:           resource.CoreTextureViewFrom(dstView),
			DepthLoadOp:    gputypes.LoadOpLoad,
			DepthStoreOp:   gputypes.StoreOpStore,
			StencilLoadOp:  gputypes.LoadOpLoad,
			StencilStoreOp: gputypes.StoreOpStore,
		},
	}
}
