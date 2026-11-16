//go:build !rust && !(js && wasm)

package wgpu

// The stencil blit's pipeline cache. It stays in the root package when the
// upload payload moves to internal/stencilblit (S9's hand-back): the cache is
// device-owned state (core.StencilBlitResources) and holds the public wrappers
// that own the pipelines' references, so moving it would drag the public
// pipeline API into the child. The payload reaches the cached state through
// stencilblit.Device.Pipelines, which returns the internal handles (Blit) and
// never a public wrapper.

import (
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/pipeline"
	"github.com/gogpu/wgpu/internal/stencilblit"
)

// Debug label of the blit's objects.
const stencilBlitLabel = "stencil blit"

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

// pipelines returns the blit pipelines of key and the layout their bind groups
// are built on, reusing the device's cache. It is unexported: the public
// Device must not grow a method naming an internal pipeline type (the API
// golden is frozen, ADR-070 §15.1), and the payload's host in this package is
// its only caller.
func (d *Device) pipelines(key stencilblit.Key) (stencilblit.Blit, error) {
	entry, err := d.stencilBlitPipelines(key)
	if err != nil {
		return stencilblit.Blit{}, err
	}
	return stencilblit.Blit{Pipelines: entry.impls(), Layout: entry.bgl.impl}, nil
}
