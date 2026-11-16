//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu

// This file is the public declaration layer over internal/device on the native
// build (ADR-070 S11). The implementation moved to internal/device; what stays
// here is the frozen public Device type, its methods, the descriptor conversion
// (toHAL and the wrapper unwrapping, lead ruling 2) and the instance identity
// cache the surface cross-check reads.

import (
	"fmt"
	"time"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/device"
	"github.com/gogpu/wgpu/internal/encoder"
	"github.com/gogpu/wgpu/internal/hal"
	"github.com/gogpu/wgpu/internal/resource"
)

// Device represents a logical GPU device.
// It is the main interface for creating GPU resources.
//
// Device methods are safe for concurrent use, except Release() which
// must not be called concurrently with other methods.
type Device struct {
	impl  *device.Device
	queue *Queue

	// instance is the identity-check cache: the root Instance that adopted this
	// device, written once by setInstance at construction and cleared by
	// Release. Surface.Configure compares it to reject a cross-instance
	// device/surface pair. It is set from the same host that is handed to the
	// implementation, so the two cannot diverge, and nothing else writes it
	// (ADR-070 S11, lead ruling 7).
	instance *Instance
}

// setInstance wires the device to the instance that adopted it: it binds the
// host's device pointer, stores the root identity cache from the same host and
// hands the host to the implementation's own seam, so both describe one
// instance. It takes the concrete host rather than device.InstanceHost because
// it binds the device pointer and reads the root instance pointer, neither of
// which the seam may name; the call-site assignment is the compile check that
// the host satisfies the seam.
//
// Called once, by Instance.adoptDevice.
func (d *Device) setInstance(h *deviceInstanceHost) {
	if d == nil || h == nil {
		return
	}
	h.d = d
	d.instance = h.i
	if d.impl != nil {
		d.impl.SetInstance(h)
	}
}

// Queue returns the device's command queue.
func (d *Device) Queue() *Queue {
	return d.queue
}

// Features returns the device's enabled features.
func (d *Device) Features() gputypes.Features {
	if d == nil || d.impl == nil {
		return 0
	}
	return d.impl.Features()
}

// Limits returns the device's resource limits.
func (d *Device) Limits() gputypes.Limits {
	if d == nil || d.impl == nil {
		return gputypes.Limits{}
	}
	return d.impl.Limits()
}

// CreateBuffer creates a GPU buffer.
func (d *Device) CreateBuffer(desc *BufferDescriptor) (*Buffer, error) {
	var halDesc *hal.BufferDescriptor
	if desc != nil {
		halDesc = desc.toHAL()
	}
	impl, err := d.impl.CreateBuffer(halDesc)
	if err != nil {
		return nil, err
	}
	return &Buffer{impl: impl}, nil
}

// CreateTexture creates a GPU texture.
func (d *Device) CreateTexture(desc *TextureDescriptor) (*Texture, error) {
	var halDesc *hal.TextureDescriptor
	if desc != nil {
		desc = normalizeTextureDescriptor(desc)
		halDesc = desc.toHAL()
	}
	impl, err := d.impl.CreateTexture(halDesc)
	if err != nil {
		return nil, err
	}
	tex := &Texture{impl: impl}
	impl.Pub = tex
	return tex, nil
}

// CreateTextureView creates a view into a texture.
func (d *Device) CreateTextureView(texture *Texture, desc *TextureViewDescriptor) (*TextureView, error) {
	// A non-nil wrapper without an implementation is a zero-value handle, not
	// an absent one: it answers ErrReleased, as the pre-move implementation did
	// through the nil-safe ResolveHAL (S11 review F3).
	if texture != nil && texture.impl == nil {
		return nil, ErrReleased
	}
	var parent *resource.Texture
	if texture != nil {
		parent = texture.impl
	}
	var halDesc *hal.TextureViewDescriptor
	if desc != nil {
		halDesc = desc.toHAL()
	}
	impl, err := d.impl.CreateTextureView(parent, halDesc)
	if err != nil {
		return nil, err
	}
	return &TextureView{impl: impl}, nil
}

// CreateSampler creates a texture sampler.
func (d *Device) CreateSampler(desc *SamplerDescriptor) (*Sampler, error) {
	if desc == nil {
		desc = &SamplerDescriptor{}
	}
	impl, err := d.impl.CreateSampler(desc.toHAL())
	if err != nil {
		return nil, err
	}
	return &Sampler{impl: impl}, nil
}

// CreateShaderModule creates a shader module.
func (d *Device) CreateShaderModule(desc *ShaderModuleDescriptor) (*ShaderModule, error) {
	var halDesc *hal.ShaderModuleDescriptor
	if desc != nil {
		halDesc = &hal.ShaderModuleDescriptor{
			Label: desc.Label,
			Source: hal.ShaderSource{
				WGSL:  desc.WGSL,
				SPIRV: desc.SPIRV,
			},
		}
	}
	impl, err := d.impl.CreateShaderModule(halDesc)
	if err != nil {
		return nil, err
	}
	return &ShaderModule{impl: impl}, nil
}

// CreateBindGroupLayout creates a bind group layout.
func (d *Device) CreateBindGroupLayout(desc *BindGroupLayoutDescriptor) (*BindGroupLayout, error) {
	var halDesc *hal.BindGroupLayoutDescriptor
	if desc != nil {
		halDesc = desc.toHAL()
	}
	impl, err := d.impl.CreateBindGroupLayout(halDesc)
	if err != nil {
		return nil, err
	}
	return &BindGroupLayout{impl: impl}, nil
}

// CreatePipelineLayout creates a pipeline layout.
func (d *Device) CreatePipelineLayout(desc *PipelineLayoutDescriptor) (*PipelineLayout, error) {
	var halDesc *hal.PipelineLayoutDescriptor
	var layouts []*resource.BindGroupLayout
	if desc != nil {
		halDesc = &hal.PipelineLayoutDescriptor{Label: desc.Label}
		layouts = make([]*resource.BindGroupLayout, len(desc.BindGroupLayouts))
		for i, l := range desc.BindGroupLayouts {
			if l != nil {
				layouts[i] = l.impl
			}
		}
	}
	impl, err := d.impl.CreatePipelineLayout(halDesc, layouts)
	if err != nil {
		return nil, err
	}
	return &PipelineLayout{impl: impl}, nil
}

// CreateBindGroup creates a bind group.
func (d *Device) CreateBindGroup(desc *BindGroupDescriptor) (*BindGroup, error) {
	var bgDesc *device.BindGroupDescriptor
	if desc != nil {
		var layout *resource.BindGroupLayout
		if desc.Layout != nil {
			layout = desc.Layout.impl
		}
		entries := make([]device.BindGroupEntry, len(desc.Entries))
		for i, e := range desc.Entries {
			entry, err := bindGroupEntry(e)
			if err != nil {
				return nil, err
			}
			entries[i] = entry
		}
		bgDesc = &device.BindGroupDescriptor{Label: desc.Label, Layout: layout, Entries: entries}
	}
	impl, err := d.impl.CreateBindGroup(bgDesc)
	if err != nil {
		return nil, err
	}
	return &BindGroup{impl: impl}, nil
}

// bindGroupEntry converts one public bind group entry into the device-local
// one. The three zero-value handles keep the answers they had before the move,
// per the S11 review's identity standard:
//   - a non-nil TextureView wrapper without an implementation answered
//     ErrReleased (its nil-safe ResolveHAL guard);
//   - a non-nil Buffer wrapper without an implementation panicked in the
//     pre-move entry inspection, which read the public wrapper's Usage through
//     a nil handle (buildBindGroupValidationInfos:692);
//   - a non-nil Sampler wrapper without an implementation was accepted and
//     bound nothing.
//
// Normalising the three to defined errors is a good change that is carded for
// after this moves-only window.
func bindGroupEntry(e BindGroupEntry) (device.BindGroupEntry, error) {
	if e.TextureView != nil && e.TextureView.impl == nil {
		return device.BindGroupEntry{}, ErrReleased
	}
	if e.Buffer != nil && e.Buffer.impl == nil {
		_ = e.Buffer.Usage() // nil handle: panics, exactly as before the move
	}
	return device.BindGroupEntry{
		Binding:     e.Binding,
		Buffer:      bufferImpl(e.Buffer),
		Offset:      e.Offset,
		Size:        e.Size,
		Sampler:     samplerImpl(e.Sampler),
		TextureView: textureViewImpl(e.TextureView),
	}, nil
}

// samplerImpl returns the implementation handle of a sampler wrapper, or nil.
func samplerImpl(s *Sampler) *resource.Sampler {
	if s == nil {
		return nil
	}
	return s.impl
}

// textureViewImpl returns the implementation handle of a texture view wrapper,
// or nil.
func textureViewImpl(v *TextureView) *resource.TextureView {
	if v == nil {
		return nil
	}
	return v.impl
}

// CreateRenderPipeline creates a render pipeline.
func (d *Device) CreateRenderPipeline(desc *RenderPipelineDescriptor) (*RenderPipeline, error) {
	halDesc, extras := renderPipelineHal(desc)
	impl, err := d.impl.CreateRenderPipeline(halDesc, extras)
	if err != nil {
		return nil, err
	}
	return &RenderPipeline{impl: impl}, nil
}

// renderPipelineHal converts a public descriptor into the HAL descriptor and
// the internal handles the implementation needs (ADR-070 Shape 1).
func renderPipelineHal(desc *RenderPipelineDescriptor) (*hal.RenderPipelineDescriptor, device.RenderPipelineExtras) {
	if desc == nil {
		return nil, device.RenderPipelineExtras{}
	}
	extras := device.RenderPipelineExtras{HasFragment: desc.Fragment != nil}
	if desc.Fragment != nil {
		extras.FragmentTargets = desc.Fragment.Targets
		if desc.Fragment.Module != nil {
			extras.FragmentModule = desc.Fragment.Module.impl
		}
	}
	if desc.Layout != nil {
		extras.Layout = desc.Layout.impl
	}
	if desc.Vertex.Module != nil {
		extras.VertexModule = desc.Vertex.Module.impl
	}
	return desc.toHAL(), extras
}

// CreateComputePipeline creates a compute pipeline.
func (d *Device) CreateComputePipeline(desc *ComputePipelineDescriptor) (*ComputePipeline, error) {
	var halDesc *hal.ComputePipelineDescriptor
	var module *resource.ShaderModule
	var layout *resource.PipelineLayout
	if desc != nil {
		halDesc = desc.toHAL()
		if desc.Module != nil {
			module = desc.Module.impl
		}
		if desc.Layout != nil {
			layout = desc.Layout.impl
		}
	}
	impl, err := d.impl.CreateComputePipeline(halDesc, module, layout)
	if err != nil {
		return nil, err
	}
	return &ComputePipeline{impl: impl}, nil
}

// CreateCommandEncoder creates a command encoder for recording GPU commands.
//
// When a device-level encoder pool is available (BUG-DX12-004), the HAL encoder
// is acquired from the pool instead of creating a new one. Matches Rust
// wgpu-core's CommandAllocator pattern (allocator.rs).
func (d *Device) CreateCommandEncoder(desc *CommandEncoderDescriptor) (*CommandEncoder, error) {
	label := ""
	if desc != nil {
		label = desc.Label
	}
	impl, err := d.impl.CreateCommandEncoder(label)
	if err != nil {
		return nil, err
	}
	return &CommandEncoder{impl: impl, device: d}, nil
}

// CreateFence creates a GPU synchronization fence.
func (d *Device) CreateFence() (*Fence, error) {
	impl, err := d.impl.CreateFence()
	if err != nil {
		return nil, err
	}
	return &Fence{impl: impl}, nil
}

// CreateQuerySet creates a query set for occlusion or timestamp queries.
func (d *Device) CreateQuerySet(desc *QuerySetDescriptor) (*QuerySet, error) {
	var halDesc *hal.QuerySetDescriptor
	if desc != nil {
		halDesc = desc.toHAL()
	}
	impl, err := d.impl.CreateQuerySet(halDesc)
	if err != nil {
		return nil, err
	}
	return &QuerySet{impl: impl}, nil
}

// DestroyFence destroys a fence.
// The fence must not be in use by the GPU when destroyed.
//
// Deprecated: Use Fence.Release() instead.
func (d *Device) DestroyFence(f *Fence) {
	if f != nil {
		f.Release()
	}
}

// ResetFence resets a fence to the unsignaled state.
func (d *Device) ResetFence(f *Fence) error {
	var impl *resource.Fence
	if f != nil {
		impl = f.impl
	}
	return d.impl.ResetFence(impl)
}

// GetFenceStatus returns true if the fence is signaled (non-blocking).
func (d *Device) GetFenceStatus(f *Fence) (bool, error) {
	var impl *resource.Fence
	if f != nil {
		impl = f.impl
	}
	return d.impl.GetFenceStatus(impl)
}

// WaitForFence waits for a fence to reach the specified value.
// Returns true if the fence reached the value, false if timeout expired.
func (d *Device) WaitForFence(f *Fence, value uint64, timeout time.Duration) (bool, error) {
	var impl *resource.Fence
	if f != nil {
		impl = f.impl
	}
	return d.impl.WaitForFence(impl, value, timeout)
}

// FreeCommandBuffer returns a command buffer to the command pool.
// This must be called after the GPU has finished using the command buffer.
// For multi-CB encoders, all accumulated HAL command buffers are freed.
func (d *Device) FreeCommandBuffer(cb *CommandBuffer) {
	var impl *encoder.CommandBuffer
	if cb != nil {
		impl = cb.impl
	}
	d.impl.FreeCommandBuffer(impl)
}

// PushErrorScope pushes a new error scope onto the device's error scope stack.
func (d *Device) PushErrorScope(filter ErrorFilter) error {
	return d.impl.PushErrorScope(core.ErrorFilter(filter))
}

// PopErrorScope pops the most recently pushed error scope.
// Returns the captured error, or nil if no error occurred.
func (d *Device) PopErrorScope() *GPUError {
	e := d.impl.PopErrorScope()
	if e == nil {
		return nil
	}
	return &GPUError{Type: ErrorFilter(e.Type), Message: e.Message}
}

// WaitIdle waits for all GPU work to complete.
func (d *Device) WaitIdle() error {
	return d.impl.WaitIdle()
}

// Release releases the device and all associated resources.
func (d *Device) Release() {
	if d == nil {
		return
	}
	d.impl.Release()
	d.instance = nil
}

// Poll drives the per-device pending-map triage loop.
func (d *Device) Poll(pollType PollType) bool {
	if d == nil || d.impl == nil {
		return false
	}
	return d.impl.Poll(core.PollType(pollType))
}

// CreateRenderBundleEncoder creates an encoder for reusable render commands.
func (d *Device) CreateRenderBundleEncoder(desc *RenderBundleEncoderDescriptor) (*RenderBundleEncoder, error) {
	// The descriptor check belongs before the forwarding: a nil descriptor
	// answered this error before the move, and a nil *Device would otherwise
	// panic on d.impl first (S11 delta review M1).
	if desc == nil {
		return nil, fmt.Errorf("wgpu: render bundle encoder descriptor is nil")
	}
	var encDesc *encoder.RenderBundleEncoderDescriptor
	if desc != nil {
		encDesc = encoderRenderBundleEncoderDescriptor(desc)
	}
	impl, err := d.impl.CreateRenderBundleEncoder(encDesc)
	if err != nil {
		return nil, err
	}
	return &RenderBundleEncoder{impl: impl}, nil
}

// halDevice returns the HAL device behind the implementation, or nil.
func (d *Device) halDevice() hal.Device {
	if d == nil || d.impl == nil {
		return nil
	}
	return d.impl.HALDevice()
}

// newDetachedTexture returns a texture that carries the descriptor's metadata
// (its format and view formats) and nothing else: no device and no HAL object.
// It backs the public wgputest.NewDetachedTexture. desc may be nil: the handle
// then carries zero metadata.
func newDetachedTexture(desc *TextureDescriptor) *Texture {
	var format gputypes.TextureFormat
	var viewFormats *[]gputypes.TextureFormat
	if desc != nil {
		format = desc.Format
		viewFormats = copyViewFormats(desc.ViewFormats)
	}
	impl := resource.NewDetachedTexture(format, viewFormats)
	tex := &Texture{impl: impl}
	impl.Pub = tex
	return tex
}

// copyViewFormats copies TextureDescriptor.ViewFormats, so a later change to
// the caller's slice does not change what views are allowed.
func copyViewFormats(formats []gputypes.TextureFormat) *[]gputypes.TextureFormat {
	if len(formats) == 0 {
		return nil
	}
	c := make([]gputypes.TextureFormat, len(formats))
	copy(c, formats)
	return &c
}
