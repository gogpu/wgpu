//go:build !rust && !(js && wasm)

package wgpu

import (
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/encoder"
	"github.com/gogpu/wgpu/internal/hal"
	"github.com/gogpu/wgpu/internal/pipeline"
	"github.com/gogpu/wgpu/internal/resource"
	"github.com/gogpu/wgpu/internal/stencilblit"
)

// stencilBlitParamsStride is the distance between the Params of two layers in
// the blit's uniform buffer: 256, the largest minUniformBufferOffsetAlignment
// the WebGPU spec allows, so a multiple of every device's limit.
const stencilBlitParamsStride = 256

// Debug labels of the blit's objects.
const (
	stencilBlitLabel   = "stencil blit"
	stencilUploadLabel = "stencil upload"
)

// stencilBlitEntry holds the objects of one stencilblit.Key: the shader, the
// layouts and the nine pipelines.
type stencilBlitEntry struct {
	module    *ShaderModule
	bgl       *BindGroupLayout
	layout    *PipelineLayout
	pipelines stencilblit.Pipelines[*RenderPipeline]
}

// release drops the entry's objects; a partly built entry is fine.
func (e *stencilBlitEntry) release() {
	if e.pipelines.Clear != nil {
		e.pipelines.Clear.Release()
	}
	for _, p := range e.pipelines.Bits {
		if p != nil {
			p.Release()
		}
	}
	if e.layout != nil {
		e.layout.Release()
	}
	if e.bgl != nil {
		e.bgl.Release()
	}
	if e.module != nil {
		e.module.Release()
	}
}

// stencilBlitter is the device's cache of blit pipelines, held by the core
// device (core.Device.StencilBlitResources) and disposed with it.
type stencilBlitter struct {
	cache *stencilblit.Cache[*stencilBlitEntry]
}

var _ core.StencilBlitResources = (*stencilBlitter)(nil)

// Dispose releases every cached pipeline.
func (b *stencilBlitter) Dispose() { b.cache.Dispose() }

// stencilBlitPipelines returns the pipelines of key, building them on first
// use.
func (d *Device) stencilBlitPipelines(key stencilblit.Key) (*stencilBlitEntry, error) {
	res := d.core.StencilBlitResources(func() core.StencilBlitResources {
		return &stencilBlitter{cache: stencilblit.NewCache(d.buildStencilBlitEntry, (*stencilBlitEntry).release)}
	})
	blitter, ok := res.(*stencilBlitter)
	if !ok {
		return nil, ErrReleased
	}
	return blitter.cache.Get(key)
}

// buildStencilBlitEntry creates the shader, layouts and pipelines of key.
// Every descriptor field the blit depends on is set explicitly, so it does not
// follow a change of the descriptors' zero values.
func (d *Device) buildStencilBlitEntry(key stencilblit.Key) (*stencilBlitEntry, error) {
	src, err := stencilblit.ShaderSource(key.ViewDimension)
	if err != nil {
		return nil, err
	}
	e := &stencilBlitEntry{}
	if err := d.fillStencilBlitEntry(e, key, src); err != nil {
		e.release()
		return nil, err
	}
	return e, nil
}

func (d *Device) fillStencilBlitEntry(e *stencilBlitEntry, key stencilblit.Key, src string) error {
	var err error
	if e.module, err = d.CreateShaderModule(&ShaderModuleDescriptor{Label: stencilBlitLabel, WGSL: src}); err != nil {
		return fmt.Errorf("shader module: %w", err)
	}
	e.bgl, err = d.CreateBindGroupLayout(&BindGroupLayoutDescriptor{
		Label: stencilBlitLabel,
		Entries: []gputypes.BindGroupLayoutEntry{
			{
				Binding:    stencilblit.SourceBinding,
				Visibility: gputypes.ShaderStageFragment,
				Texture: &gputypes.TextureBindingLayout{
					SampleType:    gputypes.TextureSampleTypeUint,
					ViewDimension: key.ViewDimension,
				},
			},
			{
				Binding:    stencilblit.ParamsBinding,
				Visibility: gputypes.ShaderStageFragment,
				Buffer: &gputypes.BufferBindingLayout{
					Type:           gputypes.BufferBindingTypeUniform,
					MinBindingSize: stencilblit.ParamsSize,
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("bind group layout: %w", err)
	}
	e.layout, err = d.CreatePipelineLayout(&PipelineLayoutDescriptor{
		Label:            stencilBlitLabel,
		BindGroupLayouts: []*BindGroupLayout{e.bgl},
	})
	if err != nil {
		return fmt.Errorf("pipeline layout: %w", err)
	}
	if e.pipelines.Clear, err = d.createStencilBlitPipeline(e, key.Format, stencilblit.ClearEntryPoint, stencilblit.ClearState()); err != nil {
		return err
	}
	for bit := range stencilblit.StencilBits {
		p, err := d.createStencilBlitPipeline(e, key.Format, stencilblit.BitEntryPoint(bit), stencilblit.BitState(bit))
		if err != nil {
			return err
		}
		e.pipelines.Bits[bit] = p
	}
	return nil
}

func (d *Device) createStencilBlitPipeline(e *stencilBlitEntry, format gputypes.TextureFormat, entry string, s stencilblit.StencilState) (*RenderPipeline, error) {
	face := StencilFaceState{Compare: s.Compare, FailOp: s.FailOp, DepthFailOp: s.DepthFail, PassOp: s.PassOp}
	p, err := d.CreateRenderPipeline(&RenderPipelineDescriptor{
		Label:  stencilBlitLabel + " " + entry,
		Layout: e.layout,
		Vertex: VertexState{Module: e.module, EntryPoint: stencilblit.VertexEntryPoint},
		Primitive: gputypes.PrimitiveState{
			Topology:  gputypes.PrimitiveTopologyTriangleList,
			FrontFace: gputypes.FrontFaceCCW,
			CullMode:  gputypes.CullModeNone,
		},
		DepthStencil: &DepthStencilState{
			Format:            format,
			DepthWriteEnabled: false,
			DepthCompare:      gputypes.CompareFunctionAlways,
			StencilFront:      face,
			StencilBack:       face,
			StencilReadMask:   s.ReadMask,
			StencilWriteMask:  s.WriteMask,
		},
		Multisample: gputypes.DefaultMultisampleState(),
		Fragment:    &FragmentState{Module: e.module, EntryPoint: entry},
	})
	if err != nil {
		return nil, fmt.Errorf("pipeline %s: %w", entry, err)
	}
	return p, nil
}

// impls returns the pipeline implementation handles the blit's pass recorder
// draws with. The entry itself keeps the public wrappers: they own the
// pipelines' references and are released with the cache.
func (e *stencilBlitEntry) impls() stencilblit.Pipelines[*pipeline.RenderPipeline] {
	var impls stencilblit.Pipelines[*pipeline.RenderPipeline]
	if e.pipelines.Clear != nil {
		impls.Clear = e.pipelines.Clear.impl
	}
	for bit, p := range e.pipelines.Bits {
		if p != nil {
			impls.Bits[bit] = p.impl
		}
	}
	return impls
}

// stencilUpload is one upload into the stencil aspect of a combined
// depth-stencil texture, lowered as stencilblit describes: the caller writes
// the stencil bytes into temp (an R8Uint texture of the copy extent), then
// Record draws them into dst.
//
// The device is captured at creation rather than read off the encoder: the
// payload stays in this package when the command encoder moves to
// internal/encoder (ADR-070 S4), and a root device pointer is not something the
// destination may hold.
type stencilUpload struct {
	device   *Device
	dst      *Texture
	mipLevel uint32
	origin   Origin3D
	size     Extent3D
	temp     *Texture
}

var _ encoder.Upload = (*stencilUpload)(nil)

// newStencilUpload creates the R8Uint texture an upload of size into the
// stencil of dst at mipLevel and origin goes through.
func (d *Device) newStencilUpload(dst *Texture, mipLevel uint32, origin Origin3D, size Extent3D) (*stencilUpload, error) {
	temp, err := d.CreateTexture(&TextureDescriptor{
		Label:         stencilUploadLabel,
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
	return &stencilUpload{device: d, dst: dst, mipLevel: mipLevel, origin: origin, size: size, temp: temp}, nil
}

// tempCopy returns the region of temp that holds the stencil bytes.
func (u *stencilUpload) tempCopy() ImageCopyTexture {
	return ImageCopyTexture{Texture: u.temp, Aspect: gputypes.TextureAspectAll}
}

// Staging returns the R8Uint texture the caller writes the stencil bytes into.
func (u *stencilUpload) Staging() *resource.Texture { return u.temp.impl }

// Record records into r, for each destination layer, the render pass that
// writes the layer's stencil from temp. Every pass it opens is ended. The
// resources it creates are handed to r (Recorder.Own); the caller hands it
// temp.
func (u *stencilUpload) Record(r encoder.Recorder) error {
	d := u.device
	layers := u.size.DepthOrArrayLayers
	key := stencilblit.KeyFor(u.dst.Format(), layers)
	entry, err := d.stencilBlitPipelines(key)
	if err != nil {
		return err
	}
	srcView, err := d.CreateTextureView(u.temp, &TextureViewDescriptor{
		Label:           stencilUploadLabel + " source",
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
	impls := entry.impls()
	for layer := range layers {
		if err := u.recordLayer(r, entry, &impls, srcView, params, layer); err != nil {
			return fmt.Errorf("layer %d: %w", u.origin.Z+layer, err)
		}
	}
	return nil
}

// paramsBuffer returns a uniform buffer with the Params of each layer at
// stencilBlitParamsStride. It is written through a mapping at creation, so no
// queue write is involved (Queue.WriteTexture records the blit with the queue
// lock held).
func (u *stencilUpload) paramsBuffer(layers uint32) (*Buffer, error) {
	size := uint64(layers-1)*stencilBlitParamsStride + stencilblit.ParamsSize
	buf, err := u.device.CreateBuffer(&BufferDescriptor{
		Label:            stencilUploadLabel + " params",
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
		p := stencilblit.Params{OriginX: u.origin.X, OriginY: u.origin.Y, Layer: layer}.Bytes()
		copy(data[uint64(layer)*stencilBlitParamsStride:], p[:])
	}
	rng.Release()
	if err := buf.Unmap(); err != nil {
		buf.Release()
		return nil, fmt.Errorf("params buffer: %w", err)
	}
	return buf, nil
}

func (u *stencilUpload) recordLayer(r encoder.Recorder, entry *stencilBlitEntry, impls *stencilblit.Pipelines[*pipeline.RenderPipeline], srcView *TextureView, params *Buffer, layer uint32) error {
	d := u.device
	group, err := d.CreateBindGroup(&BindGroupDescriptor{
		Label:  stencilUploadLabel,
		Layout: entry.bgl,
		Entries: []BindGroupEntry{
			{Binding: stencilblit.SourceBinding, TextureView: srcView},
			{
				Binding: stencilblit.ParamsBinding, Buffer: params,
				Offset: uint64(layer) * stencilBlitParamsStride, Size: stencilblit.ParamsSize,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("bind group: %w", err)
	}
	r.Own(group)
	dstView, err := d.CreateTextureView(u.dst, &TextureViewDescriptor{
		Label:           stencilUploadLabel + " destination",
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
	dstView.impl.SetInternalUsage(gputypes.TextureUsageRenderAttachment)

	// Depth is loaded and stored as it is; the pipelines neither test nor
	// write it. Stencil outside the scissor keeps its contents.
	pass, err := r.BeginLayerPass(stencilUploadPassDescriptor(u.dst, dstView), group.impl)
	if err != nil {
		return fmt.Errorf("render pass: %w", err)
	}
	stencilblit.RecordLayer[*pipeline.RenderPipeline](pass, impls,
		u.origin.X, u.origin.Y, u.size.Width, u.size.Height)
	if err := pass.End(); err != nil {
		return fmt.Errorf("render pass: %w", err)
	}
	return nil
}

// stencilUploadPassDescriptor builds the core descriptor of one blit layer's
// render pass: the layer's view of the destination, loading and storing depth
// and stencil. The destination texture is the authority for the attachment's
// HAL handle, like the public render pass conversion (a borrowed surface
// texture shares one core texture across swapchain images).
func stencilUploadPassDescriptor(dst *Texture, dstView *TextureView) *core.RenderPassDescriptor {
	return &core.RenderPassDescriptor{
		Label: stencilUploadLabel,
		ResolveAttachmentTexture: func(*core.TextureView) hal.Texture {
			return dst.impl.ResolveHAL()
		},
		DepthStencilAttachment: &core.RenderPassDepthStencilAttachment{
			View:           resource.CoreTextureViewFrom(dstView.impl),
			DepthLoadOp:    gputypes.LoadOpLoad,
			DepthStoreOp:   gputypes.StoreOpStore,
			StencilLoadOp:  gputypes.LoadOpLoad,
			StencilStoreOp: gputypes.StoreOpStore,
		},
	}
}
