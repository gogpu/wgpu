//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package device

// This file is the device domain's implementation on the native build, moved
// out of the root package by the S11 Move (ADR-070). The root package keeps a
// declaration layer over it: the public Device type, its frozen methods and
// the descriptor conversion (toHAL) stay there, and this type owns everything
// behind them.
//
// The boundary is ADR-070 shape 1 (lead ruling, S11): the root converts a
// public descriptor with toHAL and unwraps the public handles, and each Create
// here takes the HAL descriptor plus the internal handles and the few scalars
// the HAL descriptor cannot carry. No public root type appears in a signature
// below. The device-local structs for those extras are declared in
// descriptor_hal.go, beside the seam.
//
// d.seamView() of the pre-move code is this type itself: the six methods of
// core.DeviceView are implemented here, so every child that took the cached
// S1 adapter is handed the device directly. The adapter (device_view_native.go)
// is deleted with the move.

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"sync/atomic"
	"time"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/naga/ir"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/core/track"
	"github.com/gogpu/wgpu/internal/encoder"
	"github.com/gogpu/wgpu/internal/hal"
	"github.com/gogpu/wgpu/internal/pipeline"
	"github.com/gogpu/wgpu/internal/queue"
	"github.com/gogpu/wgpu/internal/resource"
	"github.com/gogpu/wgpu/internal/stencilblit"
	"github.com/gogpu/wgpu/internal/surface"
)

// Device is the device domain's implementation: the core device state, the
// queue it submits through, the command-encoder pool and recordings, and the
// instance that adopted it.
//
// It is the type every extraction child receives as its device view, and the
// five seam interfaces are satisfied directly, by the call-site assignments
// that hand this value to each child (the buffer, encoder, queue, surface and
// stencil-blit seams): ADR-070 §15.6 uses assignment instead of var _.
type Device struct {
	core  *core.Device
	queue *queue.Queue

	// instance is the device domain's seam view of the instance that adopted
	// it: the teardown scan's surface list and the unregister on release. nil
	// for a device that was never adopted (a bare HAL or core-only device).
	// Written once by SetInstance; cleared by Release after the unregister.
	instance InstanceHost

	released atomic.Bool

	// test, when non-nil, records the public calls for package wgputest.
	test core.TestHooks

	// recordings holds the command encoders and command buffers that still
	// carry resource references; Release discards them.
	recordings encoder.Recordings

	// cmdEncoderPool is the single shared encoder pool for the device.
	// Used by both CreateCommandEncoder (user command encoders) and
	// PendingWrites (internal staging encoders). Matches Rust wgpu-core's
	// single device.command_allocator for both paths (queue.rs:1373).
	//
	// nil when no HAL device (e.g., core-only path).
	cmdEncoderPool *encoder.Pool
}

// New builds a device over an already-constructed core device and queue, with
// pool as its shared encoder pool. The root declaration layer and the
// destination's own constructors are its callers: the core device, the queue
// and the pending-write batch are built by the package that owns the HAL
// objects (the root bridge for now), because internal/device must not depend
// on internal/pendingwrites (ADR-070 S11 golden, no such edge).
func New(coreDevice *core.Device, q *queue.Queue, pool *encoder.Pool) *Device {
	return &Device{core: coreDevice, queue: q, cmdEncoderPool: pool}
}

// SetInstance wires the device to the instance that adopted it. It is called
// once at construction by internal/instance's Adapter.RequestDevice; the root
// declaration layer separately records the instance identity the returned
// device carries. Nothing else writes the seam.
func (d *Device) SetInstance(h InstanceHost) {
	if d == nil || h == nil {
		return
	}
	d.instance = h
}

// SetTestHooks attaches the wgputest recorder to the device.
func (d *Device) SetTestHooks(hooks core.TestHooks) {
	if d == nil {
		return
	}
	d.test = hooks
}

// SetPoolForTest replaces the device's encoder pool. The root queue fixtures
// rebuild a device's pool and queue around a hand-built queue; nothing in the
// lifecycle calls it.
func (d *Device) SetPoolForTest(pool *encoder.Pool) {
	if d == nil {
		return
	}
	d.cmdEncoderPool = pool
}

// SetQueueForTest replaces the device's queue. Same purpose as SetPoolForTest.
func (d *Device) SetQueueForTest(q *queue.Queue) {
	if d == nil {
		return
	}
	d.queue = q
}

// orZero returns d, or the zero struct when d is nil. The root declaration
// layer forwards every public Device call to its implementation pointer, and a
// zero-value wgpu.Device has none: pre-move those calls read the zero core
// state of the root Device and answered with the errors that state produces
// (S11 review F2). Each entry point a caller can reach starts by replacing a
// nil receiver with the zero struct, so a nil receiver answers exactly what
// the contract table in zero_value_native_test.go pins for the zero struct.
func orZero(d *Device) *Device {
	if d == nil {
		return &Device{}
	}
	return d
}

// Core returns the core device state a child records against.
func (d *Device) Core() *core.Device {
	if d == nil {
		return nil
	}
	return d.core
}

// Queue returns the implementation queue the device submits through, or nil
// for a device built without one (the core-only path). The root declaration
// layer builds its public Queue wrapper over this value at construction, so the
// queue the device holds and the queue the caller submits through are the same
// object.
func (d *Device) Queue() *queue.Queue {
	if d == nil {
		return nil
	}
	return d.queue
}

// IsReleased reports whether the device has been released.
func (d *Device) IsReleased() bool {
	return d != nil && d.released.Load()
}

// HALDevice returns the HAL device for resource creation, or nil.
func (d *Device) HALDevice() hal.Device { return d.halDevice() }

// DestroyQueue returns the deferred-destroy queue, or nil.
func (d *Device) DestroyQueue() *core.DestroyQueue { return d.destroyQueue() }

// LastSubmissionIndex returns the latest submission index known to the device.
func (d *Device) LastSubmissionIndex() uint64 { return d.lastSubmissionIndex() }

// HALQueue returns the HAL queue presentation goes through, or nil.
func (d *Device) HALQueue() hal.Queue {
	if d == nil || d.queue == nil {
		return nil
	}
	return d.queue.HALQueue()
}

// EncoderPool returns the device's HAL command encoder pool, or an untyped nil
// when the device has none: the queue's and the encoder's nil checks must hold
// across the interface.
func (d *Device) EncoderPool() core.HalEncoderPool {
	if d == nil || d.cmdEncoderPool == nil {
		return nil
	}
	return d.cmdEncoderPool
}

// ReportSubmitValidationError delivers a submit-class validation error to the
// core device's error-scope sink (error-scope reshape, stage A3). It is the
// seam behind the queue's explicit class entry; nothing else routes through it.
func (d *Device) ReportSubmitValidationError(message string) {
	if d == nil || d.core == nil {
		return
	}
	d.core.ReportError(core.ErrorFilterValidation, message)
}

// Recordings returns the device's recordings registry.
func (d *Device) Recordings() core.EncoderRecordings[*encoder.CommandEncoder, *encoder.CommandBuffer] {
	if d == nil {
		return nil
	}
	return &d.recordings
}

// Features returns the device's enabled features.
func (d *Device) Features() gputypes.Features {
	if d == nil || d.core == nil {
		return 0
	}
	return d.core.Features
}

// Limits returns the device's resource limits.
func (d *Device) Limits() gputypes.Limits {
	if d == nil || d.core == nil {
		return gputypes.Limits{}
	}
	return d.core.Limits
}

// CreateBuffer creates a GPU buffer from an already-converted descriptor.
func (d *Device) CreateBuffer(halDesc *hal.BufferDescriptor) (*resource.Buffer, error) {
	d = orZero(d)
	if d.released.Load() {
		return nil, core.ErrReleased
	}
	if halDesc == nil {
		return nil, fmt.Errorf("wgpu: buffer descriptor is nil")
	}

	coreBuffer, err := d.core.CreateBuffer(&gputypes.BufferDescriptor{
		Label:            halDesc.Label,
		Size:             halDesc.Size,
		Usage:            halDesc.Usage,
		MappedAtCreation: halDesc.MappedAtCreation,
	})
	if err != nil {
		return nil, err
	}

	// Initialize ResourceRef with onZero callback for refcount-driven destruction.
	// When the last reference drops (either from explicit Release or Phase 2
	// Triage after GPU completion), onZero fires and HAL-destroys the buffer.
	// This matches Rust wgpu's Arc<Buffer> Drop behavior.
	coreBuffer.Ref = core.NewResourceRef("Buffer:"+halDesc.Label, func() {
		coreBuffer.Destroy()
	})

	buf := resource.NewBuffer(coreBuffer, d)

	if d.test != nil {
		d.test.BufferCreated(halDesc.Label, halDesc.Size, halDesc.Usage)
	}
	return buf, nil
}

// CreateTexture creates a GPU texture from an already-normalized descriptor.
func (d *Device) CreateTexture(halDesc *hal.TextureDescriptor) (*resource.Texture, error) {
	d = orZero(d)
	if d.released.Load() {
		return nil, core.ErrReleased
	}
	if halDesc == nil {
		return nil, fmt.Errorf("wgpu: texture descriptor is nil")
	}

	halDevice := d.halDevice()
	if halDevice == nil {
		return nil, core.ErrReleased
	}

	// The original usage is what core records for validation and tracking; the
	// HAL usage gains render attachment for depth/stencil formats below.
	originalUsage := halDesc.Usage

	if err := core.ValidateTextureDescriptor(halDesc, d.core.Limits, d.core.Features); err != nil {
		return nil, err
	}
	if err := d.core.RequireViewFormats(halDesc.Label, halDesc.Format, halDesc.ViewFormats); err != nil {
		return nil, err
	}
	// A depth/stencil texture is always created render-attachable, as Rust
	// wgpu-core's conv.rs map_texture_usage_for_texture adds DEPTH_WRITE and
	// STENCIL_WRITE: internal passes can render into it, and a backend can
	// make a view of it (a VkImage with only transfer usages cannot have one,
	// VUID-VkImageViewCreateInfo-image-04441). core.Texture keeps the original
	// usage, so validation sees only what the application asked for.
	if halDesc.Format.IsDepthStencil() {
		halDesc.Usage |= gputypes.TextureUsageRenderAttachment
	}

	halTexture, err := halDevice.CreateTexture(halDesc)
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to create texture: %w", err)
	}

	// Create core.Texture for TrackerIndex allocation. This enables submit-time
	// barrier injection: the TrackerIndex is used by populateTextureScope to
	// record per-texture usage in the command buffer's TextureUsageScope.
	coreTexture := core.NewTexture(
		halTexture, d.core, halDesc.Format, halDesc.Dimension,
		originalUsage,
		gputypes.Extent3D{
			Width:              halDesc.Size.Width,
			Height:             halDesc.Size.Height,
			DepthOrArrayLayers: halDesc.Size.DepthOrArrayLayers,
		},
		halDesc.MipLevelCount, halDesc.SampleCount, halDesc.Label,
	)
	// The application holds the first reference; bind groups, command
	// encoders and queue writes clone it. The texture is destroyed when the
	// last one drops, as Rust wgpu-core's Arc<Texture>.
	td := coreTexture.TrackingData()
	coreTexture.Ref = core.NewResourceRef("Texture:"+halDesc.Label, func() {
		d.destroyTextureDeferred(halTexture, td)
	})

	if d.test != nil {
		d.test.TextureCreated(halDesc.Label)
	}
	return resource.NewTexture(halTexture, d, halDesc.Format, copyViewFormats(halDesc.ViewFormats), coreTexture), nil
}

// destroyTextureDeferred destroys a HAL texture once the GPU has completed the
// last submission so far.
func (d *Device) destroyTextureDeferred(halTex hal.Texture, td *track.TrackingData) {
	core.DestroyTextureDeferred(d, halTex, td)
}

// destroyTextureViewDeferred destroys a HAL texture view once the GPU has
// completed the last submission so far.
func (d *Device) destroyTextureViewDeferred(halView hal.TextureView) {
	core.DestroyTextureViewDeferred(d, halView)
}

// copyViewFormats copies TextureDescriptor.ViewFormats, so a later change to
// the caller's slice does not change what views are allowed.
func copyViewFormats(formats []gputypes.TextureFormat) *[]gputypes.TextureFormat {
	if len(formats) == 0 {
		return nil
	}
	c := slices.Clone(formats)
	return &c
}

// resolveViewMipCount returns the number of mip levels a texture view covers:
// the requested count, or the levels from baseMipLevel to the end of the
// texture ("resolving GPUTextureViewDescriptor defaults"). 0 when the texture
// is not known.
func resolveViewMipCount(texture *resource.Texture, base, requested uint32) uint32 {
	if requested != 0 {
		return requested
	}
	if texture == nil || texture.CoreTexture() == nil {
		return 0
	}
	if mips := texture.CoreTexture().MipLevelCount(); mips > base {
		return mips - base
	}
	return 0
}

// resolveViewLayerCount returns the number of array layers a texture view
// covers: the requested count, or the default WebGPU derives ("resolving
// GPUTextureViewDescriptor defaults"). 0 when the texture is not known.
func resolveViewLayerCount(texture *resource.Texture, dim gputypes.TextureViewDimension, base, requested uint32) uint32 {
	if requested != 0 {
		return requested
	}
	switch dim {
	case gputypes.TextureViewDimension1D, gputypes.TextureViewDimension2D, gputypes.TextureViewDimension3D:
		return 1
	case gputypes.TextureViewDimensionCube:
		return 6
	}
	if texture == nil || texture.CoreTexture() == nil {
		return 0
	}
	if layers := texture.CoreTexture().Size().DepthOrArrayLayers; layers > base {
		return layers - base
	}
	return 0
}

// resolveViewDimension returns the dimension a texture view has: the requested
// one, or the default WebGPU derives from the texture.
func resolveViewDimension(texture *resource.Texture, requested gputypes.TextureViewDimension) gputypes.TextureViewDimension {
	if requested != gputypes.TextureViewDimensionUndefined {
		return requested
	}
	if texture == nil || texture.CoreTexture() == nil {
		return gputypes.TextureViewDimensionUndefined
	}
	switch texture.CoreTexture().Dimension() {
	case gputypes.TextureDimension1D:
		return gputypes.TextureViewDimension1D
	case gputypes.TextureDimension3D:
		return gputypes.TextureViewDimension3D
	default:
		if texture.CoreTexture().Size().DepthOrArrayLayers > 1 {
			return gputypes.TextureViewDimension2DArray
		}
		return gputypes.TextureViewDimension2D
	}
}

// validateTextureViewDescriptor checks an explicit view descriptor against the
// texture's format support. A texture without a known format (a bare wrapper)
// cannot be checked.
func validateTextureViewDescriptor(texture *resource.Texture, halDesc *hal.TextureViewDescriptor) error {
	if texture.Format() == gputypes.TextureFormatUndefined {
		return nil
	}
	if err := core.ValidateTextureViewFormat(texture.Format(), texture.ViewFormatList(), halDesc.Aspect, halDesc.Format); err != nil {
		return fmt.Errorf("wgpu: CreateTextureView: %w", err)
	}
	return nil
}

// CreateTextureView creates a view into texture. halDesc is nil when the
// caller passed no descriptor; the defaults are resolved here exactly as the
// pre-move body resolved them.
func (d *Device) CreateTextureView(texture *resource.Texture, halDesc *hal.TextureViewDescriptor) (*resource.TextureView, error) {
	d = orZero(d)
	if d.released.Load() {
		return nil, core.ErrReleased
	}
	if texture == nil {
		return nil, fmt.Errorf("wgpu: texture is nil")
	}
	halTexture := texture.ResolveHAL()
	if halTexture == nil {
		return nil, core.ErrReleased
	}

	halDevice := d.halDevice()
	if halDevice == nil {
		return nil, core.ErrReleased
	}

	if halDesc == nil {
		halDesc = &hal.TextureViewDescriptor{}
	} else if err := validateTextureViewDescriptor(texture, halDesc); err != nil {
		return nil, err
	}
	// The HAL gets the resolved dimension and counts, as Rust wgpu-core
	// create_texture_view passes resolved_dimension and resolved_range.
	halDesc.Dimension = resolveViewDimension(texture, halDesc.Dimension)
	halDesc.ArrayLayerCount = resolveViewLayerCount(texture, halDesc.Dimension,
		halDesc.BaseArrayLayer, halDesc.ArrayLayerCount)
	halDesc.MipLevelCount = resolveViewMipCount(texture, halDesc.BaseMipLevel, halDesc.MipLevelCount)

	halView, err := halDevice.CreateTextureView(halTexture, halDesc)
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to create texture view: %w", err)
	}

	// The view holds its texture's reference until its last owner drops it, as
	// Rust wgpu-core's TextureView holds an Arc of its parent. Surface textures
	// are borrowed and have no reference to hold.
	var textureRef *core.ResourceRef
	if texture.Surface() == nil {
		textureRef = texture.Ref()
	}
	if textureRef != nil {
		textureRef.Clone()
	}
	if d.test != nil {
		d.test.TextureViewCreated(halDesc.Label)
	}
	return resource.NewTextureView(&resource.TextureViewInit{
		HAL:          halView,
		Device:       d,
		Parent:       texture,
		Surface:      texture.Surface(),
		SurfaceLease: texture.SurfaceLease(),
		Ref: core.NewResourceRef("TextureView:"+halDesc.Label, func() {
			d.destroyTextureViewDeferred(halView)
			if textureRef != nil {
				textureRef.Drop()
			}
		}),
		Dimension:       halDesc.Dimension,
		BaseMipLevel:    halDesc.BaseMipLevel,
		BaseArrayLayer:  halDesc.BaseArrayLayer,
		ArrayLayerCount: halDesc.ArrayLayerCount,
		MipLevelCount:   halDesc.MipLevelCount,
		Format:          halDesc.Format,
		Aspect:          halDesc.Aspect,
	}), nil
}

// CreateSampler creates a texture sampler from an already-converted descriptor.
func (d *Device) CreateSampler(halDesc *hal.SamplerDescriptor) (*resource.Sampler, error) {
	d = orZero(d)
	if d.released.Load() {
		return nil, core.ErrReleased
	}

	halDevice := d.halDevice()
	if halDevice == nil {
		return nil, core.ErrReleased
	}

	if halDesc == nil {
		halDesc = &hal.SamplerDescriptor{}
	}

	if err := core.ValidateSamplerDescriptor(halDesc, d.core.Features); err != nil {
		return nil, err
	}

	halSampler, err := halDevice.CreateSampler(halDesc)
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to create sampler: %w", err)
	}

	return resource.NewSampler(halSampler, d, newSamplerRef(d, halDevice, halSampler, halDesc.Label), halDesc.MagFilter, halDesc.MinFilter, halDesc.MipmapFilter), nil
}

// CreateShaderModule creates a shader module from an already-converted
// descriptor. core.ValidateShaderModuleDescriptor fills the descriptor's IR and
// Info, which the module keeps for pipeline introspection.
func (d *Device) CreateShaderModule(halDesc *hal.ShaderModuleDescriptor) (*resource.ShaderModule, error) {
	d = orZero(d)
	if d.released.Load() {
		return nil, core.ErrReleased
	}
	if halDesc == nil {
		return nil, fmt.Errorf("wgpu: shader module descriptor is nil")
	}

	halDevice := d.halDevice()
	if halDevice == nil {
		return nil, core.ErrReleased
	}

	if err := core.ValidateShaderModuleDescriptor(halDesc, d.core.Features); err != nil {
		return nil, err
	}

	halModule, err := halDevice.CreateShaderModule(halDesc)
	if err != nil {
		return nil, &core.CreateShaderModuleError{Kind: core.CreateShaderModuleErrorHAL, Label: halDesc.Label, HALError: err}
	}

	// Keep the IR core validated for pipeline introspection, as Rust
	// wgpu-core keeps the naga module and its info on the ShaderModule for
	// Interface::check_stage. SPIR-V modules have none.
	module := resource.NewShaderModule(halModule, d, halDesc.IR, halDesc.Info)
	if halDesc.IR == nil && len(halDesc.Source.SPIRV) > 0 {
		module.ArmSPIRVWorkgroups(halDesc.Source.SPIRV)
	}
	return module, nil
}

// CreateBindGroupLayout creates a bind group layout.
func (d *Device) CreateBindGroupLayout(halDesc *hal.BindGroupLayoutDescriptor) (*resource.BindGroupLayout, error) {
	d = orZero(d)
	if d.released.Load() {
		return nil, core.ErrReleased
	}
	if halDesc == nil {
		return nil, fmt.Errorf("wgpu: bind group layout descriptor is nil")
	}

	halDevice := d.halDevice()
	if halDevice == nil {
		return nil, core.ErrReleased
	}

	if err := core.ValidateBindGroupLayoutDescriptor(halDesc, d.core.Limits); err != nil {
		return nil, err
	}

	halLayout, err := halDevice.CreateBindGroupLayout(halDesc)
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to create bind group layout: %w", err)
	}

	// Store a defensive copy of entries for entry-by-entry compatibility checks.
	// This matches Rust wgpu-core's pattern where binder compares layouts by entries.
	entriesCopy := make([]gputypes.BindGroupLayoutEntry, len(halDesc.Entries))
	copy(entriesCopy, halDesc.Entries)

	return resource.NewBindGroupLayout(halLayout, d, entriesCopy), nil
}

// CreatePipelineLayout creates a pipeline layout. The HAL layout handles and
// the internal handles are both derived from layouts, after the nil and released
// checks the pre-move body performed on the public wrappers.
func (d *Device) CreatePipelineLayout(halDesc *hal.PipelineLayoutDescriptor, layouts []*resource.BindGroupLayout) (*resource.PipelineLayout, error) {
	d = orZero(d)
	if d.released.Load() {
		return nil, core.ErrReleased
	}
	if halDesc == nil {
		return nil, fmt.Errorf("wgpu: pipeline layout descriptor is nil")
	}

	halDevice := d.halDevice()
	if halDevice == nil {
		return nil, core.ErrReleased
	}

	halLayouts := make([]hal.BindGroupLayout, len(layouts))
	for i, layout := range layouts {
		if layout == nil {
			return nil, fmt.Errorf("wgpu: bind group layout at index %d is nil", i)
		}
		// A released layout may already be queued for destruction; its HAL
		// handle must not back a new pipeline layout (P2-CORE-023).
		if layout.IsReleased() {
			return nil, core.ErrReleased
		}
		halLayouts[i] = layout.HAL()
	}
	halDesc.BindGroupLayouts = halLayouts

	if err := core.ValidatePipelineLayoutDescriptor(halDesc, d.core.Limits, d.core.Features); err != nil {
		return nil, err
	}

	halLayout, err := halDevice.CreatePipelineLayout(halDesc)
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to create pipeline layout: %w", err)
	}

	// Store a copy of the bind group layouts slice for binder validation.
	bgLayouts := make([]*resource.BindGroupLayout, len(layouts))
	copy(bgLayouts, layouts)

	// Complete the handle: the count was validated against the device limit
	// above (ValidatePipelineLayoutDescriptor), so it fits uint32.
	bindGroupCount := uint32(len(layouts)) //nolint:gosec // G115: bounded by ValidatePipelineLayoutDescriptor (MaxBindGroups, at most 4)
	return resource.NewPipelineLayout(halLayout, d, bindGroupCount, bgLayouts, newPipelineLayoutRef(d, halDevice, halLayout, halDesc.Label)), nil
}

// CreateBindGroup creates a bind group from the device-local descriptor: the
// label, the internal layout handle and the internal resource handles.
func (d *Device) CreateBindGroup(desc *BindGroupDescriptor) (*resource.BindGroup, error) {
	d = orZero(d)
	if d.released.Load() {
		return nil, core.ErrReleased
	}
	if desc == nil {
		return nil, fmt.Errorf("wgpu: bind group descriptor is nil")
	}

	halDevice := d.halDevice()
	if halDevice == nil {
		return nil, core.ErrReleased
	}

	if desc.Layout == nil {
		return nil, &core.CreateBindGroupError{
			Kind:  core.CreateBindGroupErrorMissingLayout,
			Label: desc.Label,
		}
	}
	// A released layout may already be queued for destruction; its HAL handle
	// must not back a new descriptor set (P2-CORE-023). The HAL bind group does
	// not keep the layout — Rust wgpu-hal's Vulkan BindGroup holds only the
	// descriptor set — so a group created while the layout was alive is
	// unaffected by a later Release.
	if desc.Layout.IsReleased() {
		return nil, core.ErrReleased
	}

	halEntries := make([]gputypes.BindGroupEntry, len(desc.Entries))
	for i, entry := range desc.Entries {
		if entry.TextureView != nil && entry.TextureView.ResolveHAL() == nil {
			return nil, core.ErrReleased
		}
		// A released sampler may already be queued for destruction; binding
		// it would revive its reference count and name a dying sampler.
		if entry.Sampler != nil && entry.Sampler.IsReleased() {
			return nil, core.ErrReleased
		}
		halEntries[i] = bindGroupEntryToHAL(entry)
	}

	halDesc := &hal.BindGroupDescriptor{
		Label:   desc.Label,
		Layout:  desc.Layout.HAL(),
		Entries: halEntries,
	}

	bufferInfos, samplerInfos, textureInfos := buildBindGroupValidationInfos(desc.Entries)

	if err := core.ValidateBindGroupDescriptor(halDesc, desc.Layout.Entries(), bufferInfos, samplerInfos, textureInfos, d.core.Limits, d.core.Features); err != nil {
		return nil, err
	}

	halGroup, err := halDevice.CreateBindGroup(halDesc)
	if errors.Is(err, hal.ErrTextureViewNotSampleable) {
		return nil, &core.CreateBindGroupError{
			Kind: core.CreateBindGroupErrorTextureViewNotSampleable, Label: desc.Label, HALError: err,
		}
	}
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to create bind group: %w", err)
	}

	// Build late buffer binding info for layout entries with MinBindingSize == 0.
	// Matches Rust wgpu-core's BindGroup.late_buffer_binding_infos population
	// in Device::create_bind_group (binding_model.rs:1187-1189).
	lateInfos := buildLateBufferBindingInfos(desc.Layout.Entries(), desc.Entries)

	// Collect buffer and texture references for submit-time validation (VAL-A6).
	boundBuffers, boundTextures := collectBindGroupResources(desc.Entries)

	// Initialize ResourceRef with onZero callback for refcount-driven destruction.
	// The bind group keeps its texture views, their textures and its samplers
	// alive until its HAL bind group is destroyed, as Rust wgpu-core's BindGroup
	// holds an Arc of each (VUID-vkDestroySampler-sampler-01082).
	halBG := halGroup
	heldRefs := cloneBindGroupResourceRefs(desc.Entries)
	destroy := func() {
		halDevice.DestroyBindGroup(halBG)
		for _, ref := range heldRefs {
			ref.Drop()
		}
	}
	bgOnZero := func() {
		dq := d.destroyQueue()
		if dq != nil {
			subIdx := d.lastSubmissionIndex()
			dq.Defer(subIdx, "BindGroup", destroy)
		} else {
			destroy()
		}
	}

	return resource.NewBindGroup(&resource.BindGroupInit{
		Label:                  desc.Label,
		HAL:                    halGroup,
		Device:                 d,
		Layout:                 desc.Layout,
		LateBufferBindingInfos: lateInfos,
		DynamicBindings:        buildDynamicBindings(desc.Layout.Entries(), desc.Entries),
		Ref:                    core.NewResourceRef("BindGroup:"+desc.Label, bgOnZero),
		BoundBuffers:           boundBuffers,
		BoundTextures:          boundTextures,
		Usages:                 buildBindGroupUsages(desc.Layout.Entries(), desc.Entries),
	}), nil
}

// bindGroupEntryToHAL renders one internal entry as the flat gputypes entry the
// HAL and the backends take. It is the pre-move public BindGroupEntry.toHAL,
// retyped over the internal handles.
func bindGroupEntryToHAL(e BindGroupEntry) gputypes.BindGroupEntry {
	entry := gputypes.BindGroupEntry{
		Binding: e.Binding,
	}
	var halView hal.TextureView
	if e.TextureView != nil {
		halView = e.TextureView.ResolveHAL()
	}

	switch {
	case e.Buffer != nil:
		halBuf := e.Buffer.HALBuffer()
		if halBuf != nil {
			entry.Resource = gputypes.BufferBinding{
				Buffer: halBuf.NativeHandle(),
				Offset: e.Offset,
				Size:   e.Size,
			}
		}
	case e.Sampler != nil && e.Sampler.HAL() != nil:
		entry.Resource = gputypes.SamplerBinding{
			Sampler: e.Sampler.HAL().NativeHandle(),
		}
	case halView != nil:
		entry.Resource = gputypes.TextureViewBinding{
			TextureView: halView.NativeHandle(),
		}
	}

	return entry
}

func buildBindGroupValidationInfos(entries []BindGroupEntry) (
	[]core.BindGroupBufferInfo,
	[]core.BindGroupSamplerInfo,
	[]core.BindGroupTextureInfo,
) {
	var bufferInfos []core.BindGroupBufferInfo
	var samplerInfos []core.BindGroupSamplerInfo
	var textureInfos []core.BindGroupTextureInfo
	for _, entry := range entries {
		if entry.Buffer != nil {
			bufferInfos = append(bufferInfos, core.BindGroupBufferInfo{
				Binding:    entry.Binding,
				Usage:      entry.Buffer.Usage(),
				BufferSize: entry.Buffer.Size(),
				Offset:     entry.Offset,
				Size:       entry.Size,
			})
		}
		if entry.Sampler != nil {
			samplerInfos = append(samplerInfos, core.BindGroupSamplerInfo{
				Binding: entry.Binding,
				Desc: &hal.SamplerDescriptor{
					MagFilter:    entry.Sampler.MagFilter(),
					MinFilter:    entry.Sampler.MinFilter(),
					MipmapFilter: entry.Sampler.MipmapFilter(),
				},
			})
		}
		if entry.TextureView != nil && entry.TextureView.Parent() != nil {
			textureInfos = append(textureInfos, core.BindGroupTextureInfo{
				Binding: entry.Binding,
				Format:  entry.TextureView.Parent().Format(),
			})
		}
	}
	return bufferInfos, samplerInfos, textureInfos
}

func buildLateBufferBindingInfos(layoutEntries []gputypes.BindGroupLayoutEntry, entries []BindGroupEntry) []resource.LateBufferBindingInfo {
	entryMap := buildBindGroupEntryMap(entries)
	var lateInfos []resource.LateBufferBindingInfo
	for _, layoutEntry := range layoutEntries {
		if layoutEntry.Buffer == nil || layoutEntry.Buffer.MinBindingSize != 0 {
			continue
		}
		var boundSize uint64
		if bgEntry, ok := entryMap[layoutEntry.Binding]; ok && bgEntry.Buffer != nil {
			boundSize = bgEntry.Size
			if boundSize == 0 {
				bufSize := bgEntry.Buffer.Size()
				if bgEntry.Offset < bufSize {
					boundSize = bufSize - bgEntry.Offset
				}
			}
		}
		lateInfos = append(lateInfos, resource.LateBufferBindingInfo{
			BindingIndex: layoutEntry.Binding,
			Size:         boundSize,
		})
	}
	return lateInfos
}

// cloneBindGroupResourceRefs clones the references of every texture view and
// sampler a bind group binds, and returns them, for the bind group to drop when
// it is destroyed. A view holds its texture's reference itself.
func cloneBindGroupResourceRefs(entries []BindGroupEntry) []*core.ResourceRef {
	var refs []*core.ResourceRef
	hold := func(ref *core.ResourceRef) {
		if ref != nil {
			ref.Clone()
			refs = append(refs, ref)
		}
	}
	for i := range entries {
		if view := entries[i].TextureView; view != nil {
			hold(view.Ref())
		}
		if sampler := entries[i].Sampler; sampler != nil {
			hold(sampler.Ref())
		}
	}
	return refs
}

func collectBindGroupResources(entries []BindGroupEntry) ([]*resource.Buffer, []*resource.Texture) {
	var buffers []*resource.Buffer
	var textures []*resource.Texture
	for i := range entries {
		if entries[i].Buffer != nil {
			buffers = append(buffers, entries[i].Buffer)
		}
		if entries[i].TextureView != nil && entries[i].TextureView.Parent() != nil {
			textures = append(textures, entries[i].TextureView.Parent())
		}
	}
	return buffers, textures
}

// buildBindGroupEntryMap builds a lookup map from binding index to BindGroupEntry
// for efficient access during late buffer binding info construction.
func buildBindGroupEntryMap(entries []BindGroupEntry) map[uint32]*BindGroupEntry {
	m := make(map[uint32]*BindGroupEntry, len(entries))
	for i := range entries {
		m[entries[i].Binding] = &entries[i]
	}
	return m
}

// buildBindGroupUsages derives the usage of every buffer and texture binding
// from its layout entry: uniform, read-only storage, or storage for buffers;
// sampled, or storage with the layout's access mode, for textures.
//
// Reference: wgpu-core device/resource.rs create_buffer_binding /
// create_texture_binding (the usages recorded in BindGroup.used)
func buildBindGroupUsages(layoutEntries []gputypes.BindGroupLayoutEntry, entries []BindGroupEntry) *core.BindGroupUsages {
	entryMap := buildBindGroupEntryMap(entries)
	usages := &core.BindGroupUsages{}
	for i := range layoutEntries {
		layoutEntry := &layoutEntries[i]
		entry, ok := entryMap[layoutEntry.Binding]
		if !ok {
			continue
		}
		switch {
		case layoutEntry.Buffer != nil && entry.Buffer != nil && entry.Buffer.CoreBuffer() != nil:
			usages.Buffers = append(usages.Buffers, core.BindGroupBufferUse{
				Buffer: entry.Buffer.CoreBuffer(),
				Usage:  bufferBindingUses(layoutEntry.Buffer.Type),
			})
		case layoutEntry.Texture != nil || layoutEntry.StorageTexture != nil:
			if entry.TextureView == nil || entry.TextureView.Parent() == nil || entry.TextureView.Parent().CoreTexture() == nil {
				continue
			}
			texture := entry.TextureView.Parent()
			usage := track.TextureUsesResource
			if layoutEntry.StorageTexture != nil {
				usage = storageTextureUses(layoutEntry.StorageTexture.Access)
			}
			view := entry.TextureView
			usages.Textures = append(usages.Textures, core.BindGroupTextureUse{
				Texture: texture.CoreTexture(),
				Subresources: texture.CoreTexture().Subresources(view.BaseMipLevel(), view.MipLevelCount(),
					view.BaseArrayLayer(), view.ArrayLayerCount()),
				Usage:   usage,
				Resolve: texture.ResolveHAL,
			})
		}
	}
	return usages
}

// bufferBindingUses maps a buffer binding type to its tracker usage.
func bufferBindingUses(bindingType gputypes.BufferBindingType) track.BufferUses {
	switch bindingType {
	case gputypes.BufferBindingTypeStorage:
		return track.BufferUsesStorageWrite
	case gputypes.BufferBindingTypeReadOnlyStorage:
		return track.BufferUsesStorageRead
	default:
		return track.BufferUsesUniform
	}
}

// storageTextureUses maps a storage texture access mode to its tracker usage.
func storageTextureUses(access gputypes.StorageTextureAccess) track.TextureUses {
	if access == gputypes.StorageTextureAccessReadOnly {
		return track.TextureUsesStorageRead
	}
	return track.TextureUsesStorageWrite
}

// buildDynamicBindings collects the dynamic buffer bindings of a bind group,
// sorted by binding number (Rust wgpu-core Device::create_bind_group sorts
// dynamic_binding_info by binding_idx).
func buildDynamicBindings(layoutEntries []gputypes.BindGroupLayoutEntry, entries []BindGroupEntry) []core.DynamicBinding {
	entryMap := buildBindGroupEntryMap(entries)
	var bindings []core.DynamicBinding
	for _, layoutEntry := range layoutEntries {
		if layoutEntry.Buffer == nil || !layoutEntry.Buffer.HasDynamicOffset {
			continue
		}
		bgEntry, ok := entryMap[layoutEntry.Binding]
		if !ok || bgEntry.Buffer == nil {
			continue
		}
		bufSize := bgEntry.Buffer.Size()
		end := bufSize
		if bgEntry.Size != 0 {
			end = bgEntry.Offset + bgEntry.Size
		}
		var maxOffset uint64
		if end <= bufSize {
			maxOffset = bufSize - end
		}
		bindings = append(bindings, core.DynamicBinding{
			Binding:          layoutEntry.Binding,
			BindingType:      layoutEntry.Buffer.Type,
			BufferSize:       bufSize,
			BindingEnd:       end,
			MaxDynamicOffset: maxOffset,
		})
	}
	slices.SortFunc(bindings, func(a, b core.DynamicBinding) int { return cmp.Compare(a.Binding, b.Binding) })
	return bindings
}

// newSamplerRef returns the reference count of a HAL sampler: when its last
// owner drops it, the sampler is destroyed after the submissions in flight.
func newSamplerRef(d core.DeviceView, halDevice hal.Device, halSampler hal.Sampler, label string) *core.ResourceRef {
	return core.NewResourceRef("Sampler:"+label, func() {
		dq := d.DestroyQueue()
		if dq == nil {
			halDevice.DestroySampler(halSampler)
			return
		}
		dq.Defer(d.LastSubmissionIndex(), "Sampler", func() {
			halDevice.DestroySampler(halSampler)
		})
	})
}

// newPipelineLayoutRef returns the reference count of a HAL pipeline layout:
// when its last owner drops it, the layout is destroyed after the submissions
// in flight (P2-CORE-022).
func newPipelineLayoutRef(d core.DeviceView, halDevice hal.Device, halLayout hal.PipelineLayout, label string) *core.ResourceRef {
	return core.NewResourceRef("PipelineLayout:"+label, func() {
		dq := d.DestroyQueue()
		if dq == nil {
			halDevice.DestroyPipelineLayout(halLayout)
			return
		}
		dq.Defer(d.LastSubmissionIndex(), "PipelineLayout", func() {
			halDevice.DestroyPipelineLayout(halLayout)
		})
	})
}

// CreateRenderPipeline creates a render pipeline. The extras carry the internal
// handles and the fragment description the HAL descriptor cannot.
func (d *Device) CreateRenderPipeline(halDesc *hal.RenderPipelineDescriptor, extras RenderPipelineExtras) (*pipeline.RenderPipeline, error) {
	d = orZero(d)
	if d.released.Load() {
		return nil, core.ErrReleased
	}
	if halDesc == nil {
		return nil, fmt.Errorf("wgpu: render pipeline descriptor is nil")
	}
	// A released layout may already be queued for destruction; the pipeline
	// would be created from a dying HAL layout and would later revive its
	// reference count (P2-CORE-022). It keeps its pre-move precedence over the
	// fragment check below: a descriptor that is both released and invalid
	// answered ErrReleased before the move (S11 delta review N5).
	if extras.Layout != nil && extras.Layout.IsReleased() {
		return nil, core.ErrReleased
	}
	// A fragment state without a module is an invalid descriptor: WebGPU
	// requires GPUFragmentState.module, and the pre-move body dereferenced it
	// (a panic). The HAL descriptor cannot carry the invalid stage — toHAL
	// drops it when the module is nil, so the fragment validation in
	// ValidateRenderPipelineDescriptor never sees it and the pipeline would be
	// built without a fragment stage while HasFragment still fills the pass
	// context's color formats (S11 review F1). Reject it here, the earliest
	// point that still knows both.
	if extras.HasFragment && extras.FragmentModule == nil {
		return nil, &core.CreateRenderPipelineError{
			Kind:  core.CreateRenderPipelineErrorMissingFragmentModule,
			Label: halDesc.Label,
		}
	}

	halDevice := d.halDevice()
	if halDevice == nil {
		return nil, core.ErrReleased
	}

	if err := core.ValidateRenderPipelineDescriptor(halDesc, d.core.Limits, d.core.Features); err != nil {
		return nil, err
	}
	resolveDepthCompare(halDesc.DepthStencil) // after validation: its absence is checked there

	if err := validateRenderStageOverrides(halDesc, extras); err != nil {
		return nil, err
	}

	halPipeline, err := halDevice.CreateRenderPipeline(halDesc)
	if err != nil {
		return nil, &core.CreateRenderPipelineError{Kind: core.CreateRenderPipelineErrorHAL, Label: halDesc.Label, HALError: err}
	}

	var bgCount uint32
	var bgLayouts []*resource.BindGroupLayout
	var layoutRef *core.ResourceRef
	if extras.Layout != nil {
		// The pipeline keeps its layout alive until its own HAL pipeline is
		// destroyed: every later SetBindGroup passes the layout to
		// vkCmdBindDescriptorSets. Rust wgpu-core holds an Arc<PipelineLayout>
		// in the pipeline (pipeline.rs:496).
		layoutRef = extras.Layout.Ref()
		if layoutRef != nil {
			layoutRef.Clone()
		}
		bgCount = extras.Layout.BindGroupCount()
		bgLayouts = extras.Layout.BindGroupLayouts()
	}
	// Check if any color target uses blend constant factors.
	// Matches Rust wgpu-core PipelineFlags::BLEND_CONSTANT (resource.rs:4562-4569).
	var needsBlendConstant bool
	for i := range extras.FragmentTargets {
		if b := extras.FragmentTargets[i].Blend; b != nil {
			if b.Color.UsesConstant() || b.Alpha.UsesConstant() {
				needsBlendConstant = true
				break
			}
		}
	}

	// Build shader binding sizes across all stages (vertex + fragment).
	// Matches Rust wgpu-core's check_stage calls that accumulate shader_binding_sizes
	// with max across stages for the same binding (validation.rs:1126-1139).
	shaderBindingSizes := mergeShaderBindingSizes(
		extras.VertexModule.BindingSizes(ir.StageVertex, halDesc.Vertex.EntryPoint),
		fragmentBindingSizes(halDesc, extras),
	)

	lateGroups := makeLateSizedBufferGroups(shaderBindingSizes, bgLayouts)

	// Initialize ResourceRef with onZero callback for refcount-driven destruction.
	// ADR-056: unified resource lifecycle.
	halRP := halPipeline
	rpOnZero := func() {
		// Pipeline first, then the layout it was created from: the layout's
		// own destruction waits for the submissions in flight.
		destroy := func() {
			halDevice.DestroyRenderPipeline(halRP)
			if layoutRef != nil {
				layoutRef.Drop()
			}
		}
		dq := d.destroyQueue()
		if dq != nil {
			dq.Defer(d.lastSubmissionIndex(), "RenderPipeline", destroy)
			return
		}
		destroy()
	}

	passContext, writesDepth, writesStencil := renderPipelineTargets(halDesc, extras)
	return pipeline.NewRenderPipeline(&pipeline.RenderPipelineInit{
		HAL:                   halPipeline,
		Device:                d,
		BindGroupCount:        bgCount,
		BindGroupLayouts:      bgLayouts,
		RequiredVertexBuffers: uint32(len(halDesc.Vertex.Buffers)), //nolint:gosec // buffer count fits uint32
		BlendConstantRequired: needsBlendConstant,
		StripIndexFormat:      halDesc.Primitive.StripIndexFormat,
		LateSizedBufferGroups: lateGroups,
		PassContext:           passContext,
		WritesDepth:           writesDepth,
		WritesStencil:         writesStencil,
		Ref:                   core.NewResourceRef("RenderPipeline:"+halDesc.Label, rpOnZero),
	}), nil
}

// validateRenderStageOverrides applies the pipeline constants of the vertex
// and the fragment stage to the overrides of each stage's entry point, as
// validateComputeShader does for compute.
func validateRenderStageOverrides(halDesc *hal.RenderPipelineDescriptor, extras RenderPipelineExtras) error {
	if _, _, _, err := extras.VertexModule.SpecializedEntryPoint(ir.StageVertex, halDesc.Vertex.EntryPoint, halDesc.Vertex.Constants); err != nil {
		return &core.CreateRenderPipelineError{
			Kind:  core.CreateRenderPipelineErrorPipelineConstants,
			Label: halDesc.Label,
			Stage: "vertex",
			Cause: err,
		}
	}
	if !extras.HasFragment {
		return nil
	}
	var fs hal.FragmentState
	if halDesc.Fragment != nil {
		fs = *halDesc.Fragment
	}
	if _, _, _, err := extras.FragmentModule.SpecializedEntryPoint(ir.StageFragment, fs.EntryPoint, fs.Constants); err != nil {
		return &core.CreateRenderPipelineError{
			Kind:  core.CreateRenderPipelineErrorPipelineConstants,
			Label: halDesc.Label,
			Stage: "fragment",
			Cause: err,
		}
	}
	return nil
}

// fragmentBindingSizes returns the binding sizes of the fragment entry point,
// or nil without a fragment stage.
func fragmentBindingSizes(halDesc *hal.RenderPipelineDescriptor, extras RenderPipelineExtras) map[ir.ResourceBinding]uint64 {
	if !extras.HasFragment {
		return nil
	}
	var entryPoint string
	if halDesc.Fragment != nil {
		entryPoint = halDesc.Fragment.EntryPoint
	}
	return extras.FragmentModule.BindingSizes(ir.StageFragment, entryPoint)
}

// mergeShaderBindingSizes merges the binding sizes of the vertex and fragment
// entry points, taking the max size when both stages use the same binding.
// Matches Rust wgpu-core's pattern of calling check_stage for each stage with
// the same shader_binding_sizes map, where Entry::Occupied takes the max
// (wgpu-core/src/validation.rs).
func mergeShaderBindingSizes(vertex, fragment map[ir.ResourceBinding]uint64) map[ir.ResourceBinding]uint64 {
	result := make(map[ir.ResourceBinding]uint64, len(vertex)+len(fragment))
	for rb, size := range vertex {
		result[rb] = size
	}
	for rb, size := range fragment {
		if existing, ok := result[rb]; !ok || size > existing {
			result[rb] = size
		}
	}
	return result
}

// makeLateSizedBufferGroups generates late-sized buffer group info for a pipeline
// from the shader binding sizes and the pipeline layout.
//
// Equivalent to Rust wgpu-core's Device::make_late_sized_buffer_groups (resource.rs:2413-2448).
func makeLateSizedBufferGroups(
	shaderBindingSizes map[ir.ResourceBinding]uint64,
	layouts []*resource.BindGroupLayout,
) [core.MaxBindGroups]pipeline.LateSizedBufferGroup {
	var groups [core.MaxBindGroups]pipeline.LateSizedBufferGroup

	for groupIdx, bgl := range layouts {
		if bgl == nil {
			continue
		}
		var sizes []uint64
		for _, entry := range bgl.Entries() {
			if entry.Buffer == nil || entry.Buffer.MinBindingSize != 0 {
				continue
			}
			// This is a buffer entry with MinBindingSize == 0 —
			// validation is deferred to draw/dispatch time.
			rb := ir.ResourceBinding{
				Group:   uint32(groupIdx),
				Binding: entry.Binding,
			}
			shaderSize := shaderBindingSizes[rb] // 0 if not found
			sizes = append(sizes, shaderSize)
		}
		groups[groupIdx] = pipeline.LateSizedBufferGroup{ShaderSizes: sizes}
	}

	return groups
}

// renderPipelineTargets returns the attachment configuration a render
// pipeline renders to and whether it writes depth or stencil.
// Matches Rust wgpu-core Device::create_render_pipeline (pass_context and
// the WRITES_DEPTH / WRITES_STENCIL flags).
func renderPipelineTargets(halDesc *hal.RenderPipelineDescriptor, extras RenderPipelineExtras) (ctx core.RenderPassContext, writesDepth, writesStencil bool) {
	ctx.SampleCount = max(halDesc.Multisample.Count, 1)
	if extras.HasFragment {
		ctx.ColorFormats = make([]gputypes.TextureFormat, len(extras.FragmentTargets))
		for i := range extras.FragmentTargets {
			ctx.ColorFormats[i] = extras.FragmentTargets[i].Format
		}
	}
	ds := halDesc.DepthStencil
	if ds == nil {
		return ctx, false, false
	}
	ctx.DepthStencilFormat = ds.Format
	return ctx, ds.DepthWriteEnabled, !stencilReadOnly(ds, halDesc.Primitive.CullMode)
}

// stencilReadOnly reports whether a depth/stencil state never writes the
// stencil aspect: no write mask, or every face that is not culled keeps the
// stencil value on every outcome.
// Matches Rust wgpu-types StencilState::is_read_only.
func stencilReadOnly(ds *hal.DepthStencilState, cull gputypes.CullMode) bool {
	if ds.StencilWriteMask == 0 {
		return true
	}
	front := cull == gputypes.CullModeFront || stencilFaceKeeps(ds.StencilFront)
	back := cull == gputypes.CullModeBack || stencilFaceKeeps(ds.StencilBack)
	return front && back
}

// stencilFaceKeeps reports whether the face keeps the stencil value whether the
// stencil or the depth test passes or fails. The zero operation is WebGPU's
// default, keep. Matches Rust wgpu-types StencilFaceState::is_read_only.
func stencilFaceKeeps(s hal.StencilFaceState) bool {
	keep := func(op gputypes.StencilOperation) bool {
		return op == gputypes.StencilOperationKeep || op == gputypes.StencilOperationUndefined
	}
	return keep(s.FailOp) && keep(s.DepthFailOp) && keep(s.PassOp)
}

// CreateComputePipeline creates a compute pipeline. module is the internal
// shader-module handle the stage checks read and layout the internal pipeline
// layout the refcount and group count come from.
func (d *Device) CreateComputePipeline(halDesc *hal.ComputePipelineDescriptor, module *resource.ShaderModule, layout *resource.PipelineLayout) (*pipeline.ComputePipeline, error) {
	d = orZero(d)
	if d.released.Load() {
		return nil, core.ErrReleased
	}
	if halDesc == nil {
		return nil, fmt.Errorf("wgpu: compute pipeline descriptor is nil")
	}
	// A released layout may already be queued for destruction; the pipeline
	// would be created from a dying HAL layout and would later revive its
	// reference count (P2-CORE-022).
	if layout != nil && layout.IsReleased() {
		return nil, core.ErrReleased
	}

	// Validate that the device supports compute shaders (downlevel check).
	// Matches Rust wgpu-core resource.rs:4367.
	if err := d.core.RequireDownlevelFlags(gputypes.DownlevelFlagsComputeShaders); err != nil {
		return nil, fmt.Errorf("wgpu: CreateComputePipeline: %w", err)
	}

	halDevice := d.halDevice()
	if halDevice == nil {
		return nil, core.ErrReleased
	}

	if err := core.ValidateComputePipelineDescriptor(halDesc); err != nil {
		return nil, err
	}

	if err := d.validateComputeShader(halDesc, module); err != nil {
		return nil, err
	}

	halPipeline, err := halDevice.CreateComputePipeline(halDesc)
	if err != nil {
		return nil, &core.CreateComputePipelineError{Kind: core.CreateComputePipelineErrorHAL, Label: halDesc.Label, HALError: err}
	}

	var bgCount uint32
	var bgLayouts []*resource.BindGroupLayout
	var layoutRef *core.ResourceRef
	if layout != nil {
		// The pipeline keeps its layout alive until its own HAL pipeline is
		// destroyed (pipeline.rs:1155).
		layoutRef = layout.Ref()
		if layoutRef != nil {
			layoutRef.Clone()
		}
		bgCount = layout.BindGroupCount()
		bgLayouts = layout.BindGroupLayouts()
	}

	// Shader binding sizes of the compute entry point.
	shaderBindingSizes := module.BindingSizes(ir.StageCompute, halDesc.Compute.EntryPoint)

	lateGroups := makeLateSizedBufferGroups(shaderBindingSizes, bgLayouts)

	// Initialize ResourceRef with onZero callback for refcount-driven destruction.
	halCP := halPipeline
	cpOnZero := func() {
		destroy := func() {
			halDevice.DestroyComputePipeline(halCP)
			if layoutRef != nil {
				layoutRef.Drop()
			}
		}
		dq := d.destroyQueue()
		if dq != nil {
			dq.Defer(d.lastSubmissionIndex(), "ComputePipeline", destroy)
			return
		}
		destroy()
	}

	return pipeline.NewComputePipeline(&pipeline.ComputePipelineInit{
		HAL:                   halPipeline,
		BindGroupCount:        bgCount,
		BindGroupLayouts:      bgLayouts,
		LateSizedBufferGroups: lateGroups,
		Ref:                   core.NewResourceRef("ComputePipeline:"+halDesc.Label, cpOnZero),
	}), nil
}

// validateComputeShader checks the compute entry point against the device
// limits, after the pipeline constants are applied.
func (d *Device) validateComputeShader(halDesc *hal.ComputePipelineDescriptor, module *resource.ShaderModule) error {
	entryPoint := halDesc.Compute.EntryPoint
	if module.IRModule() == nil {
		if size, ok := module.SPIRVWorkgroupSize(entryPoint); ok {
			return d.validateComputeWorkgroupSize(halDesc.Label, size)
		}
		return nil
	}
	irMod, info, index, err := module.SpecializedEntryPoint(ir.StageCompute, entryPoint, halDesc.Compute.Constants)
	if err != nil {
		return &core.CreateComputePipelineError{
			Kind:  core.CreateComputePipelineErrorPipelineConstants,
			Label: halDesc.Label,
			Cause: err,
		}
	}
	if index < 0 {
		return nil
	}
	if err := d.validateComputeWorkgroupSize(halDesc.Label, irMod.EntryPoints[index].Workgroup); err != nil {
		return err
	}
	storage := core.WorkgroupStorageSize(irMod, info.EntryPoints[index].GlobalUses)
	if limit := d.core.Limits.MaxComputeWorkgroupStorageSize; storage > uint64(limit) {
		return &core.CreateComputePipelineError{
			Kind:             core.CreateComputePipelineErrorWorkgroupStorageExceeded,
			Label:            halDesc.Label,
			WorkgroupStorage: storage,
			Limit:            limit,
		}
	}
	return nil
}

// validateComputeWorkgroupSize checks shader workgroup_size against device limits.
// VAL-010: Matches Rust wgpu-core validation.rs:1243-1264.
func (d *Device) validateComputeWorkgroupSize(label string, wg [3]uint32) error {
	limits := d.core.Limits

	dimNames := [3]string{"X", "Y", "Z"}
	for dim := 0; dim < 3; dim++ {
		if wg[dim] == 0 {
			return &core.CreateComputePipelineError{
				Kind:      core.CreateComputePipelineErrorWorkgroupSizeZero,
				Label:     label,
				Dimension: dimNames[dim],
			}
		}
	}

	dimLimits := [3]uint32{
		limits.MaxComputeWorkgroupSizeX,
		limits.MaxComputeWorkgroupSizeY,
		limits.MaxComputeWorkgroupSizeZ,
	}
	for dim := 0; dim < 3; dim++ {
		if wg[dim] > dimLimits[dim] {
			return &core.CreateComputePipelineError{
				Kind:      core.CreateComputePipelineErrorWorkgroupSizeExceeded,
				Label:     label,
				Dimension: dimNames[dim],
				Size:      wg[dim],
				Limit:     dimLimits[dim],
			}
		}
	}

	total := uint64(wg[0]) * uint64(wg[1]) * uint64(wg[2])
	if total > uint64(limits.MaxComputeInvocationsPerWorkgroup) {
		return &core.CreateComputePipelineError{
			Kind:             core.CreateComputePipelineErrorTooManyInvocations,
			Label:            label,
			TotalInvocations: total,
			Limit:            limits.MaxComputeInvocationsPerWorkgroup,
		}
	}

	return nil
}

// CreateCommandEncoder creates a user command encoder under label. It is the
// creation path both the public Device.CreateCommandEncoder and the queue's
// device seam take: the encoder joins the device's recordings and carries the
// test hooks.
//
// When a device-level encoder pool is available (BUG-DX12-004), the HAL encoder
// is acquired from the pool instead of creating a new one. Matches Rust
// wgpu-core's CommandAllocator pattern (allocator.rs).
func (d *Device) CreateCommandEncoder(label string) (*encoder.CommandEncoder, error) {
	if d.released.Load() {
		return nil, core.ErrReleased
	}
	if d.test != nil {
		if err := d.test.FaultCreateCommandEncoder(); err != nil {
			return nil, err
		}
	}

	if d.cmdEncoderPool != nil {
		halEnc, err := d.cmdEncoderPool.Acquire()
		if err != nil {
			return nil, fmt.Errorf("wgpu: encoder pool acquire: %w", err)
		}

		if err := halEnc.BeginEncoding(label); err != nil {
			// Failed to begin encoding — return encoder to pool for future use.
			d.cmdEncoderPool.Release(halEnc)
			return nil, fmt.Errorf("wgpu: begin encoding: %w", err)
		}

		coreEncoder, err := d.core.CreateCommandEncoderWithHAL(halEnc, label)
		if err != nil {
			halEnc.DiscardEncoding()
			d.cmdEncoderPool.Release(halEnc)
			return nil, err
		}

		enc := encoder.NewCommandEncoder(encoder.CommandEncoderInit{
			Core:       coreEncoder,
			Device:     d,
			HALEncoder: halEnc,
			Pooled:     true,
		})
		d.recordings.AddEncoder(enc)
		d.attachTestHooks(enc, label)
		return enc, nil
	}

	// Fallback: no pool available (e.g., non-HAL device). Use core's built-in
	// encoder creation which creates a fresh HAL encoder each time.
	coreEncoder, err := d.core.CreateCommandEncoder(label)
	if err != nil {
		return nil, err
	}

	enc := encoder.NewCommandEncoder(encoder.CommandEncoderInit{
		Core:   coreEncoder,
		Device: d,
	})
	d.recordings.AddEncoder(enc)
	d.attachTestHooks(enc, label)
	return enc, nil
}

// attachTestHooks binds the wgputest recorder to a user command encoder and
// records its creation. It is a no-op on a device without a recorder.
func (d *Device) attachTestHooks(enc *encoder.CommandEncoder, label string) {
	if d.test == nil {
		return
	}
	enc.SetTestHooks(d.test, d.test.EncoderCreated(label))
}

// NewStencilUpload creates the staging texture an upload of size into the
// stencil of dst goes through, through the payload's own seam.
func (d *Device) NewStencilUpload(dst *resource.Texture, mipLevel uint32, origin gputypes.Origin3D, size gputypes.Extent3D) (*stencilblit.Upload, error) {
	return stencilblit.NewUpload(stencilHost{d: d}, dst, mipLevel, origin, size)
}

// CreateFence creates a GPU synchronization fence.
func (d *Device) CreateFence() (*resource.Fence, error) {
	d = orZero(d)
	if d.released.Load() {
		return nil, core.ErrReleased
	}
	halDevice := d.halDevice()
	if halDevice == nil {
		return nil, core.ErrReleased
	}

	halFence, err := halDevice.CreateFence()
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to create fence: %w", err)
	}

	return resource.NewFence(halFence, d), nil
}

// CreateQuerySet creates a query set for occlusion or timestamp queries.
func (d *Device) CreateQuerySet(halDesc *hal.QuerySetDescriptor) (*resource.QuerySet, error) {
	d = orZero(d)
	if d.released.Load() {
		return nil, core.ErrReleased
	}
	if halDesc == nil {
		return nil, fmt.Errorf("wgpu: query set descriptor is nil")
	}

	halDevice := d.halDevice()
	if halDevice == nil {
		return nil, core.ErrReleased
	}

	if err := core.ValidateQuerySetDescriptor(halDesc, d.core.Features); err != nil {
		return nil, err
	}

	halQS, err := halDevice.CreateQuerySet(halDesc)
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to create query set: %w", err)
	}

	coreQS := core.NewQuerySet(halQS, d.core, halDesc.Type, halDesc.Count, halDesc.Label)
	coreQS.Ref = newQuerySetRef(d, coreQS, halDesc.Label)
	return resource.NewQuerySet(coreQS, d), nil
}

// newQuerySetRef returns the reference count of a HAL query set: when its last
// owner drops it, the set is destroyed after the submissions in flight
// (P1-CORE-021, Rust wgpu-core's QuerySet is held by an Arc).
func newQuerySetRef(d *Device, coreQS *core.QuerySet, label string) *core.ResourceRef {
	return core.NewResourceRef("QuerySet:"+label, func() {
		dq := d.destroyQueue()
		if dq == nil {
			coreQS.Destroy()
			return
		}
		dq.Defer(d.lastSubmissionIndex(), "QuerySet", coreQS.Destroy)
	})
}

// ResetFence resets a fence to the unsignaled state.
func (d *Device) ResetFence(f *resource.Fence) error {
	d = orZero(d)
	if d.released.Load() {
		return core.ErrReleased
	}
	if f == nil || f.IsReleased() {
		return core.ErrReleased
	}
	halDevice := d.halDevice()
	if halDevice == nil {
		return core.ErrReleased
	}
	return halDevice.ResetFence(f.HAL())
}

// GetFenceStatus returns true if the fence is signaled (non-blocking).
func (d *Device) GetFenceStatus(f *resource.Fence) (bool, error) {
	d = orZero(d)
	if d.released.Load() {
		return false, core.ErrReleased
	}
	if f == nil || f.IsReleased() {
		return false, core.ErrReleased
	}
	halDevice := d.halDevice()
	if halDevice == nil {
		return false, core.ErrReleased
	}
	return halDevice.GetFenceStatus(f.HAL())
}

// WaitForFence waits for a fence to reach the specified value.
func (d *Device) WaitForFence(f *resource.Fence, value uint64, timeout time.Duration) (bool, error) {
	d = orZero(d)
	if d.released.Load() {
		return false, core.ErrReleased
	}
	if f == nil || f.IsReleased() {
		return false, core.ErrReleased
	}
	halDevice := d.halDevice()
	if halDevice == nil {
		return false, core.ErrReleased
	}
	return halDevice.Wait(f.HAL(), value, timeout)
}

// FreeCommandBuffer returns a command buffer to the command pool.
// For multi-CB encoders, all accumulated HAL command buffers are freed.
func (d *Device) FreeCommandBuffer(cb *encoder.CommandBuffer) {
	d = orZero(d)
	if d.released.Load() || cb == nil {
		return
	}
	halDevice := d.halDevice()
	if halDevice == nil {
		return
	}
	for _, buf := range cb.HALBufferList() {
		if buf != nil {
			halDevice.FreeCommandBuffer(buf)
		}
	}
}

// PushErrorScope pushes a new error scope onto the device's error scope stack.
func (d *Device) PushErrorScope(filter core.ErrorFilter) error {
	return d.core.PushErrorScope(filter)
}

// PopErrorScope pops the most recently pushed error scope.
func (d *Device) PopErrorScope() (*core.GPUError, error) {
	return d.core.PopErrorScope()
}

// WaitIdle waits for all GPU work to complete.
func (d *Device) WaitIdle() error {
	d = orZero(d)
	if d.released.Load() {
		return core.ErrReleased
	}
	return d.waitIdle()
}

// waitIdle drains the HAL and deferred resource queues without consulting the
// public released bit. Release marks the device unavailable before starting
// teardown, but the native device must remain alive while its last submission
// is drained.
func (d *Device) waitIdle() error {
	halDevice := d.halDevice()
	if halDevice == nil {
		return core.ErrReleased
	}
	if err := halDevice.WaitIdle(); err != nil {
		return err
	}
	d.maintainAfterIdle()
	return nil
}

// MaintainAfterIdle recycles staging buffers and destroys deferred GPU
// resources after the HAL reports the device is idle. The root test hook reads
// it.
func (d *Device) MaintainAfterIdle() {
	d = orZero(d)
	d.maintainAfterIdle()
}

// maintainAfterIdle is the body of MaintainAfterIdle.
func (d *Device) maintainAfterIdle() {
	if d.queue == nil || d.queue.HALQueue() == nil {
		return
	}
	completed := d.queue.HALQueue().PollCompleted()
	if dq := d.destroyQueue(); dq != nil {
		dq.Triage(completed)
	}
	d.queue.Maintain(completed)
}

// discardRecordings discards every command encoder and releases every
// command buffer still in the registry. Called by Release while the backend
// device is alive, before the deferred destructions are flushed.
func (d *Device) discardRecordings() {
	encoders, buffers := d.recordings.Take()
	for _, e := range encoders {
		e.DiscardEncoding()
	}
	for _, cb := range buffers {
		cb.Release()
	}
}

// Release releases the device and all associated resources.
// Deferred resource destructions are flushed before the device is destroyed.
// Shutdown order:
//  0. Retire active surface acquisitions
//  1. WaitIdle and maintain completed work
//  2. Discard unsubmitted queue writes
//  3. Triage + FlushAll — deferred callbacks fire (encoders return to pool)
//  4. Destroy encoder pool (HAL device still alive)
//  5. Destroy core + HAL device
func (d *Device) Release() {
	if d == nil || d.released.Swap(true) {
		return
	}
	var configuredSurfaces []*surface.Surface
	host := d.instance
	if host != nil {
		configuredSurfaces = host.SurfacesForDevice(d)
	}
	// Retire active acquisitions while the backend device is still alive.
	for _, s := range configuredSurfaces {
		s.DiscardForDevice(d.core)
	}
	if host != nil {
		defer func() {
			host.UnregisterDevice(d)
			d.instance = nil
		}()
	}

	// Step 1: Wait for ALL GPU work to finish. Use the internal path because
	// the device has already been atomically marked released to close new
	// entry points.
	_ = d.waitIdle()

	// Step 1b: Command encoders never finished or discarded, and command
	// buffers never submitted or released, still hold the references they
	// took while encoding. Drop them now.
	d.discardRecordings()

	// Step 2: Pending writes that were never submitted can now be discarded.
	if d.queue != nil {
		d.queue.Release()
	}

	// Step 3: Flush deferred destructions. WaitIdle above ensures PollCompleted()
	// returns the final submission index.
	if d.core != nil && d.core.DestroyQueueRef() != nil {
		dq := d.core.DestroyQueueRef()
		if d.queue != nil && d.queue.HALQueue() != nil {
			dq.Triage(d.queue.HALQueue().PollCompleted())
		}
		dq.FlushAll()
	}

	// Step 4: Destroy encoder pool. Each encoder.Destroy() calls
	// vkDestroyCommandPool / ID3D12CommandAllocator.Release on the still-alive
	// HAL device.
	if d.cmdEncoderPool != nil {
		d.cmdEncoderPool.Destroy()
		d.cmdEncoderPool = nil
	}

	// Step 5: Destroy core + HAL device.
	d.core.Destroy()
	for _, s := range configuredSurfaces {
		s.RetireDevice(d.core)
	}
}

// destroyQueue returns the device's DestroyQueue for deferred resource destruction.
func (d *Device) destroyQueue() *core.DestroyQueue {
	if d == nil || d.core == nil {
		return nil
	}
	return d.core.DestroyQueueRef()
}

// Poll drives the per-device pending-map triage loop. It is the seam method
// core.DeviceView declares; the root declaration layer's Poll converts the
// public PollType to core.PollType and calls it.
func (d *Device) Poll(t core.PollType) bool {
	if d == nil || d.core == nil {
		return false
	}
	switch t {
	case core.PollWait:
		// Wait on the HAL device so all submissions complete, then drain.
		if halDev, ok := d.core.HalDeviceHandle(); ok {
			if err := halDev.WaitIdle(); err != nil {
				// The wait itself failed: report it to the error-scope sink and
				// claim no completion. Draining with the all-indices-completed
				// sentinel after a failed wait would resolve every pending map
				// as done on a device that may be lost. The lost-device class
				// becomes first-class with the uncaptured-error idiom
				// (P1-API-039); until then the internal class is the honest
				// one — the failure is the implementation's, not the caller's
				// validation.
				d.core.ReportError(core.ErrorFilterInternal, "Device.Poll: wait idle: "+err.Error())
				return false
			}
		}
		// Drain with a sentinel "all indices completed".
		return d.core.PollMaps(^uint64(0))
	default:
		completed := uint64(0)
		if d.queue != nil && d.queue.HALQueue() != nil {
			completed = d.queue.HALQueue().PollCompleted()
		}
		return d.core.PollMaps(completed)
	}
}

// lastSubmissionIndex returns the latest submission index from the queue.
func (d *Device) lastSubmissionIndex() uint64 {
	if d == nil || d.queue == nil {
		return 0
	}
	return d.queue.LastSubmissionIndex()
}

// halDevice returns the underlying HAL device for direct resource creation.
func (d *Device) halDevice() hal.Device {
	if d == nil || d.core == nil || !d.core.HasHAL() {
		return nil
	}
	guard := d.core.SnatchLock().Read()
	defer guard.Release()
	return d.core.Raw(guard)
}

// CreateRenderBundleEncoder creates an encoder for reusable render commands.
func (d *Device) CreateRenderBundleEncoder(desc *encoder.RenderBundleEncoderDescriptor) (*encoder.RenderBundleEncoder, error) {
	d = orZero(d)
	if desc == nil {
		return nil, fmt.Errorf("wgpu: render bundle encoder descriptor is nil")
	}

	if d.released.Load() || d.halDevice() == nil {
		return nil, core.ErrReleased
	}

	bundleEncoder, err := encoder.NewRenderBundleEncoder(d, desc)
	if err != nil {
		return nil, fmt.Errorf("wgpu: CreateRenderBundleEncoder: %w", err)
	}
	return bundleEncoder, nil
}
