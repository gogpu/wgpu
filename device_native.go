//go:build !rust && !(js && wasm)

package wgpu

import (
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/naga/ir"
	"github.com/gogpu/wgpu/core"
	"github.com/gogpu/wgpu/hal"
)

// Device represents a logical GPU device.
// It is the main interface for creating GPU resources.
//
// Device methods are safe for concurrent use, except Release() which
// must not be called concurrently with other methods.
type Device struct {
	core     *core.Device
	queue    *Queue
	instance *Instance
	released atomic.Bool

	// test, when non-nil, records the public calls for package wgputest.
	test testHooks

	// recordings holds the command encoders and command buffers that still
	// carry resource references; Release discards them.
	recordings recordingRegistry

	// cmdEncoderPool is the single shared encoder pool for the device.
	// Used by both CreateCommandEncoder (user command encoders) and
	// PendingWrites (internal staging encoders). Matches Rust wgpu-core's
	// single device.command_allocator for both paths (queue.rs:1373).
	//
	// Encoders are acquired from the pool instead of creating expensive
	// GPU resources (DX12 ID3D12CommandAllocator ~64KB, Vulkan VkCommandPool)
	// every frame. After GPU completion, encoders are reset via ResetAll
	// and returned to the pool for reuse.
	//
	// Lifecycle: created before PendingWrites, destroyed after PendingWrites.
	// PendingWrites.destroy() clears its pool reference but does NOT destroy
	// the pool. Device.Release() destroys the pool after all users are done.
	//
	// nil when no HAL device (e.g., core-only path).
	cmdEncoderPool *encoderPool
}

// Queue returns the device's command queue.
func (d *Device) Queue() *Queue {
	return d.queue
}

// Features returns the device's enabled features.
func (d *Device) Features() gputypes.Features {
	return d.core.Features
}

// Limits returns the device's resource limits.
func (d *Device) Limits() gputypes.Limits {
	return d.core.Limits
}

// CreateBuffer creates a GPU buffer.
func (d *Device) CreateBuffer(desc *BufferDescriptor) (*Buffer, error) {
	if d.released.Load() {
		return nil, ErrReleased
	}
	if desc == nil {
		return nil, fmt.Errorf("wgpu: buffer descriptor is nil")
	}

	gpuDesc := &gputypes.BufferDescriptor{
		Label:            desc.Label,
		Size:             desc.Size,
		Usage:            desc.Usage,
		MappedAtCreation: desc.MappedAtCreation,
	}

	coreBuffer, err := d.core.CreateBuffer(gpuDesc)
	if err != nil {
		return nil, err
	}

	// Initialize ResourceRef with onZero callback for refcount-driven destruction.
	// When the last reference drops (either from explicit Release or Phase 2
	// Triage after GPU completion), onZero fires and HAL-destroys the buffer.
	// This matches Rust wgpu's Arc<Buffer> Drop behavior.
	//
	// Clone'd during encoding (SetBindGroup, SetVertexBuffer, CopyBufferToBuffer),
	// Drop'd when GPU completes submission via DestroyQueue.Triage.
	coreBuffer.Ref = core.NewResourceRef("Buffer:"+desc.Label, func() {
		coreBuffer.Destroy()
	})

	buf := &Buffer{core: coreBuffer, device: d, released: new(atomic.Bool)}

	// Safety net: if the buffer is garbage collected without Release(),
	// schedule deferred destruction via DestroyQueue. This prevents
	// resource leaks when callers create per-frame buffers without
	// explicit lifecycle management (BUG-WGPU-RESOURCE-LIFECYCLE-001).
	buf.cleanup = registerBufferCleanup(buf, d, coreBuffer, desc.Label)

	if d.test != nil {
		d.test.BufferCreated(desc.Label, desc.Size, desc.Usage)
	}
	return buf, nil
}

// CreateTexture creates a GPU texture.
func (d *Device) CreateTexture(desc *TextureDescriptor) (*Texture, error) {
	if d.released.Load() {
		return nil, ErrReleased
	}
	if desc == nil {
		return nil, fmt.Errorf("wgpu: texture descriptor is nil")
	}
	desc = normalizeTextureDescriptor(desc)

	halDevice := d.halDevice()
	if halDevice == nil {
		return nil, ErrReleased
	}

	halDesc := desc.toHAL()

	if err := core.ValidateTextureDescriptor(halDesc, d.core.Limits, d.core.Features); err != nil {
		return nil, err
	}
	if err := d.core.RequireViewFormats(desc.Label, desc.Format, desc.ViewFormats); err != nil {
		return nil, err
	}
	// A depth/stencil texture is always created render-attachable, as Rust
	// wgpu-core's conv.rs map_texture_usage_for_texture adds DEPTH_WRITE and
	// STENCIL_WRITE: internal passes can render into it, and a backend can
	// make a view of it (a VkImage with only transfer usages cannot have one,
	// VUID-VkImageViewCreateInfo-image-04441). core.Texture keeps desc.Usage,
	// so validation sees only what the application asked for.
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
		halTexture, d.core, desc.Format, halDesc.Dimension,
		desc.Usage,
		gputypes.Extent3D{
			Width:              desc.Size.Width,
			Height:             desc.Size.Height,
			DepthOrArrayLayers: desc.Size.DepthOrArrayLayers,
		},
		halDesc.MipLevelCount, halDesc.SampleCount, desc.Label,
	)
	// The application holds the first reference; bind groups, command
	// encoders and queue writes clone it. The texture is destroyed when the
	// last one drops, as Rust wgpu-core's Arc<Texture>.
	td := coreTexture.TrackingData()
	coreTexture.Ref = core.NewResourceRef("Texture:"+desc.Label, func() {
		d.destroyTextureDeferred(halTexture, td)
	})

	if d.test != nil {
		d.test.TextureCreated(desc.Label)
	}
	return &Texture{hal: halTexture, device: d, format: desc.Format, viewFormats: copyViewFormats(desc.ViewFormats), coreTexture: coreTexture}, nil
}

// CreateTextureView creates a view into a texture.
func (d *Device) CreateTextureView(texture *Texture, desc *TextureViewDescriptor) (*TextureView, error) {
	if d.released.Load() {
		return nil, ErrReleased
	}
	if texture == nil {
		return nil, fmt.Errorf("wgpu: texture is nil")
	}
	halTexture := texture.resolveHAL()
	if halTexture == nil {
		return nil, ErrReleased
	}

	halDevice := d.halDevice()
	if halDevice == nil {
		return nil, ErrReleased
	}

	halDesc := &hal.TextureViewDescriptor{}
	if desc != nil {
		// A texture without a known format (a bare wrapper) cannot be checked.
		if texture.format != gputypes.TextureFormatUndefined {
			if err := core.ValidateTextureViewFormat(texture.format, texture.viewFormatList(), desc.Aspect, desc.Format); err != nil {
				return nil, fmt.Errorf("wgpu: CreateTextureView: %w", err)
			}
		}
		halDesc.Label = desc.Label
		halDesc.Format = desc.Format
		halDesc.Dimension = desc.Dimension
		halDesc.Aspect = desc.Aspect
		halDesc.BaseMipLevel = desc.BaseMipLevel
		halDesc.MipLevelCount = desc.MipLevelCount
		halDesc.BaseArrayLayer = desc.BaseArrayLayer
		halDesc.ArrayLayerCount = desc.ArrayLayerCount
	}
	// The HAL gets the resolved dimension and counts, as Rust wgpu-core
	// create_texture_view passes resolved_dimension and resolved_range: a
	// backend's own fallback sees only the texture's dimension and would make
	// a default view of a layered 2D texture a 2d view, and a 2d view with no
	// layer count a view of every remaining layer. Undefined and 0 stay for a
	// texture whose descriptor is not known (wrapped from a HAL handle).
	halDesc.Dimension = resolveViewDimension(texture, halDesc.Dimension)
	halDesc.ArrayLayerCount = resolveViewLayerCount(texture, halDesc.Dimension,
		halDesc.BaseArrayLayer, halDesc.ArrayLayerCount)
	halDesc.MipLevelCount = resolveViewMipCount(texture, halDesc.BaseMipLevel, halDesc.MipLevelCount)

	halView, err := halDevice.CreateTextureView(halTexture, halDesc)
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to create texture view: %w", err)
	}

	// The view is destroyed when its last reference drops: the application's,
	// a bind group's or a command encoder's. It holds its texture's reference
	// until then, as Rust wgpu-core's TextureView holds an Arc of its parent
	// (resource.rs TextureView::parent): a texture released while a view of it
	// is alive stays valid for every later use of the view. Without it the
	// texture was destroyed under a bind group created from the view afterwards
	// (VUID-vkDestroyImage-image-01000, P1-CORE-020). Surface textures are
	// borrowed and have no reference to hold.
	var textureRef *core.ResourceRef
	if texture.surface == nil {
		textureRef = texture.ref()
	}
	if textureRef != nil {
		textureRef.Clone()
	}
	if d.test != nil {
		d.test.TextureViewCreated(halDesc.Label)
	}
	return &TextureView{
		hal:          halView,
		device:       d,
		texture:      texture,
		surface:      texture.surface,
		surfaceLease: texture.surfaceLease,
		ref: core.NewResourceRef("TextureView:"+halDesc.Label, func() {
			d.destroyTextureViewDeferred(halView)
			if textureRef != nil {
				textureRef.Drop()
			}
		}),
		dimension:       halDesc.Dimension,
		baseMipLevel:    halDesc.BaseMipLevel,
		baseArrayLayer:  halDesc.BaseArrayLayer,
		arrayLayerCount: halDesc.ArrayLayerCount,
		mipLevelCount:   halDesc.MipLevelCount,
		format:          halDesc.Format,
		aspect:          halDesc.Aspect,
	}, nil
}

// CreateSampler creates a texture sampler.
func (d *Device) CreateSampler(desc *SamplerDescriptor) (*Sampler, error) {
	if d.released.Load() {
		return nil, ErrReleased
	}

	halDevice := d.halDevice()
	if halDevice == nil {
		return nil, ErrReleased
	}

	if desc == nil {
		desc = &SamplerDescriptor{}
	}
	// toHAL resolves LodMaxClamp's zero value to 32 first, so validation checks
	// LodMaxClamp >= LodMinClamp on the value the backend receives.
	halDesc := desc.toHAL()

	if err := core.ValidateSamplerDescriptor(halDesc, d.core.Features); err != nil {
		return nil, err
	}

	halSampler, err := halDevice.CreateSampler(halDesc)
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to create sampler: %w", err)
	}

	return &Sampler{
		hal:          halSampler,
		device:       d,
		ref:          newSamplerRef(d, halDevice, halSampler, desc.Label),
		magFilter:    halDesc.MagFilter,
		minFilter:    halDesc.MinFilter,
		mipmapFilter: halDesc.MipmapFilter,
	}, nil
}

// CreateShaderModule creates a shader module.
func (d *Device) CreateShaderModule(desc *ShaderModuleDescriptor) (*ShaderModule, error) {
	if d.released.Load() {
		return nil, ErrReleased
	}
	if desc == nil {
		return nil, fmt.Errorf("wgpu: shader module descriptor is nil")
	}

	halDevice := d.halDevice()
	if halDevice == nil {
		return nil, ErrReleased
	}

	halDesc := &hal.ShaderModuleDescriptor{
		Label: desc.Label,
		Source: hal.ShaderSource{
			WGSL:  desc.WGSL,
			SPIRV: desc.SPIRV,
		},
	}

	if err := core.ValidateShaderModuleDescriptor(halDesc, d.core.Features); err != nil {
		return nil, err
	}

	halModule, err := halDevice.CreateShaderModule(halDesc)
	if err != nil {
		return nil, &core.CreateShaderModuleError{Kind: core.CreateShaderModuleErrorHAL, Label: desc.Label, HALError: err}
	}

	// Keep the IR core validated for pipeline introspection, as Rust
	// wgpu-core keeps the naga module and its info on the ShaderModule for
	// Interface::check_stage. SPIR-V modules have none.
	module := &ShaderModule{hal: halModule, device: d, irModule: halDesc.IR}
	if halDesc.IR != nil {
		info := halDesc.Info
		module.irInfo = &info
	} else if len(desc.SPIRV) > 0 {
		module.spirvWorkgroups = spirvComputeWorkgroups(desc.SPIRV)
	}
	return module, nil
}

// CreateBindGroupLayout creates a bind group layout.
func (d *Device) CreateBindGroupLayout(desc *BindGroupLayoutDescriptor) (*BindGroupLayout, error) {
	if d.released.Load() {
		return nil, ErrReleased
	}
	if desc == nil {
		return nil, fmt.Errorf("wgpu: bind group layout descriptor is nil")
	}

	halDevice := d.halDevice()
	if halDevice == nil {
		return nil, ErrReleased
	}

	halDesc := &hal.BindGroupLayoutDescriptor{
		Label:   desc.Label,
		Entries: desc.Entries,
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
	entriesCopy := make([]gputypes.BindGroupLayoutEntry, len(desc.Entries))
	copy(entriesCopy, desc.Entries)

	return &BindGroupLayout{hal: halLayout, device: d, entries: entriesCopy}, nil
}

// CreatePipelineLayout creates a pipeline layout.
func (d *Device) CreatePipelineLayout(desc *PipelineLayoutDescriptor) (*PipelineLayout, error) {
	if d.released.Load() {
		return nil, ErrReleased
	}
	if desc == nil {
		return nil, fmt.Errorf("wgpu: pipeline layout descriptor is nil")
	}

	halDevice := d.halDevice()
	if halDevice == nil {
		return nil, ErrReleased
	}

	halLayouts := make([]hal.BindGroupLayout, len(desc.BindGroupLayouts))
	for i, layout := range desc.BindGroupLayouts {
		if layout == nil {
			return nil, fmt.Errorf("wgpu: bind group layout at index %d is nil", i)
		}
		// A released layout may already be queued for destruction; its HAL
		// handle must not back a new pipeline layout (P2-CORE-023).
		if layout.released {
			return nil, ErrReleased
		}
		halLayouts[i] = layout.hal
	}

	halDesc := &hal.PipelineLayoutDescriptor{
		Label:            desc.Label,
		BindGroupLayouts: halLayouts,
	}

	if err := core.ValidatePipelineLayoutDescriptor(halDesc, d.core.Limits, d.core.Features); err != nil {
		return nil, err
	}

	halLayout, err := halDevice.CreatePipelineLayout(halDesc)
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to create pipeline layout: %w", err)
	}

	// Store a copy of the bind group layouts slice for binder validation.
	bgLayouts := make([]*BindGroupLayout, len(desc.BindGroupLayouts))
	copy(bgLayouts, desc.BindGroupLayouts)

	return &PipelineLayout{
		hal:              halLayout,
		device:           d,
		bindGroupCount:   uint32(len(desc.BindGroupLayouts)), //nolint:gosec // layout count fits uint32
		bindGroupLayouts: bgLayouts,
		ref:              newPipelineLayoutRef(d, halDevice, halLayout, desc.Label),
	}, nil
}

// CreateBindGroup creates a bind group.
func (d *Device) CreateBindGroup(desc *BindGroupDescriptor) (*BindGroup, error) {
	if d.released.Load() {
		return nil, ErrReleased
	}
	if desc == nil {
		return nil, fmt.Errorf("wgpu: bind group descriptor is nil")
	}

	halDevice := d.halDevice()
	if halDevice == nil {
		return nil, ErrReleased
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
	if desc.Layout.released {
		return nil, ErrReleased
	}

	halEntries := make([]gputypes.BindGroupEntry, len(desc.Entries))
	for i, entry := range desc.Entries {
		if entry.TextureView != nil && entry.TextureView.resolveHAL() == nil {
			return nil, ErrReleased
		}
		// A released sampler may already be queued for destruction; binding
		// it would revive its reference count and name a dying sampler.
		if entry.Sampler != nil && entry.Sampler.released {
			return nil, ErrReleased
		}
		halEntries[i] = entry.toHAL()
	}

	halDesc := &hal.BindGroupDescriptor{
		Label:   desc.Label,
		Layout:  desc.Layout.hal,
		Entries: halEntries,
	}

	bufferInfos, samplerInfos, textureInfos := buildBindGroupValidationInfos(desc.Entries)

	if err := core.ValidateBindGroupDescriptor(halDesc, desc.Layout.entries, bufferInfos, samplerInfos, textureInfos, d.core.Limits, d.core.Features); err != nil {
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
	// These record the actual bound buffer size at bind group creation time,
	// to be validated against shader requirements at draw/dispatch time.
	// Matches Rust wgpu-core's BindGroup.late_buffer_binding_infos population
	// in Device::create_bind_group (binding_model.rs:1187-1189).
	lateInfos := buildLateBufferBindingInfos(desc.Layout.entries, desc.Entries)

	// Collect buffer and texture references for submit-time validation (VAL-A6).
	boundBuffers, boundTextures := collectBindGroupResources(desc.Entries)

	// Initialize ResourceRef with onZero callback for refcount-driven destruction.
	// When the last reference drops (either from explicit Release or Phase 2
	// Triage after GPU completion), onZero fires and defers HAL destruction via
	// DestroyQueue. This matches Rust wgpu's Arc<BindGroup> Drop behavior. ADR-056.
	//
	// The bind group keeps its texture views, their textures and its samplers
	// alive until its HAL bind group is destroyed, as Rust wgpu-core's
	// BindGroup holds an Arc of each: a Vulkan descriptor set must not outlive
	// the image views and samplers it names (VUID-vkDestroySampler-sampler-01082).
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

	bg := &BindGroup{
		hal:                    halGroup,
		device:                 d,
		released:               new(atomic.Bool),
		layout:                 desc.Layout,
		lateBufferBindingInfos: lateInfos,
		dynamicBindings:        buildDynamicBindings(desc.Layout.entries, desc.Entries),
		ref:                    core.NewResourceRef("BindGroup:"+desc.Label, bgOnZero),
		boundBuffers:           boundBuffers,
		boundTextures:          boundTextures,
		usages:                 buildBindGroupUsages(desc.Layout.entries, desc.Entries),
	}

	// Safety net: if the bind group is garbage collected without Release(),
	// schedule deferred destruction via DestroyQueue (BUG-WGPU-RESOURCE-LIFECYCLE-001).
	bg.cleanup = registerBindGroupCleanup(bg, d, desc.Label)

	return bg, nil
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
					MagFilter:    entry.Sampler.magFilter,
					MinFilter:    entry.Sampler.minFilter,
					MipmapFilter: entry.Sampler.mipmapFilter,
				},
			})
		}
		if entry.TextureView != nil && entry.TextureView.texture != nil {
			textureInfos = append(textureInfos, core.BindGroupTextureInfo{
				Binding: entry.Binding,
				Format:  entry.TextureView.texture.Format(),
			})
		}
	}
	return bufferInfos, samplerInfos, textureInfos
}

func buildLateBufferBindingInfos(layoutEntries []gputypes.BindGroupLayoutEntry, entries []BindGroupEntry) []LateBufferBindingInfo {
	entryMap := buildBindGroupEntryMap(entries)
	var lateInfos []LateBufferBindingInfo
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
		lateInfos = append(lateInfos, LateBufferBindingInfo{
			BindingIndex: layoutEntry.Binding,
			Size:         boundSize,
		})
	}
	return lateInfos
}

// collectBindGroupResources extracts buffer and texture references from bind
// group entries for submit-time validation (VAL-A6). Matches Rust wgpu-core
// where bind group creation stores resource references that are later checked
// via trackers.buffers/textures.used_resources() in validate_command_buffer.
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
			hold(view.ref)
		}
		if sampler := entries[i].Sampler; sampler != nil {
			hold(sampler.ref)
		}
	}
	return refs
}

func collectBindGroupResources(entries []BindGroupEntry) ([]*Buffer, []*Texture) {
	var buffers []*Buffer
	var textures []*Texture
	for i := range entries {
		if entries[i].Buffer != nil {
			buffers = append(buffers, entries[i].Buffer)
		}
		if entries[i].TextureView != nil && entries[i].TextureView.texture != nil {
			textures = append(textures, entries[i].TextureView.texture)
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

// CreateRenderPipeline creates a render pipeline.
func (d *Device) CreateRenderPipeline(desc *RenderPipelineDescriptor) (*RenderPipeline, error) {
	if d.released.Load() {
		return nil, ErrReleased
	}
	if desc == nil {
		return nil, fmt.Errorf("wgpu: render pipeline descriptor is nil")
	}
	// A released layout may already be queued for destruction; the pipeline
	// would be created from a dying HAL layout and would later revive its
	// reference count (P2-CORE-022).
	if desc.Layout != nil && desc.Layout.released {
		return nil, ErrReleased
	}

	halDevice := d.halDevice()
	if halDevice == nil {
		return nil, ErrReleased
	}

	halDesc := desc.toHAL()

	if err := core.ValidateRenderPipelineDescriptor(halDesc, d.core.Limits, d.core.Features); err != nil {
		return nil, err
	}
	resolveDepthCompare(halDesc.DepthStencil) // after validation: its absence is checked there

	if err := validateRenderStageOverrides(desc); err != nil {
		return nil, err
	}

	halPipeline, err := halDevice.CreateRenderPipeline(halDesc)
	if err != nil {
		return nil, &core.CreateRenderPipelineError{Kind: core.CreateRenderPipelineErrorHAL, Label: desc.Label, HALError: err}
	}

	var bgCount uint32
	var bgLayouts []*BindGroupLayout
	var layoutRef *core.ResourceRef
	if desc.Layout != nil {
		// The pipeline keeps its layout alive until its own HAL pipeline is
		// destroyed: every later SetBindGroup passes the layout to
		// vkCmdBindDescriptorSets. Rust wgpu-core holds an Arc<PipelineLayout>
		// in the pipeline (pipeline.rs:496).
		layoutRef = desc.Layout.ref
		if layoutRef != nil {
			layoutRef.Clone()
		}
		bgCount = desc.Layout.bindGroupCount
		bgLayouts = desc.Layout.bindGroupLayouts
	}
	// Check if any color target uses blend constant factors.
	// Matches Rust wgpu-core PipelineFlags::BLEND_CONSTANT (resource.rs:4562-4569).
	var needsBlendConstant bool
	if desc.Fragment != nil {
		for i := range desc.Fragment.Targets {
			if b := desc.Fragment.Targets[i].Blend; b != nil {
				if b.Color.UsesConstant() || b.Alpha.UsesConstant() {
					needsBlendConstant = true
					break
				}
			}
		}
	}

	// Build shader binding sizes across all stages (vertex + fragment).
	// Matches Rust wgpu-core's check_stage calls that accumulate shader_binding_sizes
	// with max across stages for the same binding (validation.rs:1126-1139).
	shaderBindingSizes := mergeShaderBindingSizes(
		desc.Vertex.Module.bindingSizes(ir.StageVertex, desc.Vertex.EntryPoint),
		fragmentBindingSizes(desc.Fragment),
	)

	lateGroups := makeLateSizedBufferGroups(shaderBindingSizes, bgLayouts)

	// Initialize ResourceRef with onZero callback for refcount-driven destruction.
	// When the last reference drops (either from explicit Release or Phase 2
	// Triage after GPU completion), onZero fires and defers HAL destruction via
	// DestroyQueue. ADR-056: unified resource lifecycle.
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

	passContext, writesDepth, writesStencil := renderPipelineTargets(desc)
	return &RenderPipeline{
		hal:                   halPipeline,
		device:                d,
		bindGroupCount:        bgCount,
		bindGroupLayouts:      bgLayouts,
		requiredVertexBuffers: uint32(len(desc.Vertex.Buffers)), //nolint:gosec // buffer count fits uint32
		blendConstantRequired: needsBlendConstant,
		stripIndexFormat:      desc.Primitive.StripIndexFormat,
		lateSizedBufferGroups: lateGroups,
		passContext:           passContext,
		writesDepth:           writesDepth,
		writesStencil:         writesStencil,
		ref:                   core.NewResourceRef("RenderPipeline:"+desc.Label, rpOnZero),
	}, nil
}

// validateRenderStageOverrides applies the pipeline constants of the vertex
// and the fragment stage to the overrides of each stage's entry point, as
// validateComputeShader does for compute: an unknown key, a value out of the
// override's range or an override the entry point uses with no value and no
// default is a *core.CreateRenderPipelineError whose Cause is naga's error.
// A module without IR (SPIR-V) is not checked.
func validateRenderStageOverrides(desc *RenderPipelineDescriptor) error {
	if _, _, _, err := desc.Vertex.Module.specializedEntryPoint(ir.StageVertex, desc.Vertex.EntryPoint, desc.Vertex.Constants); err != nil {
		return &core.CreateRenderPipelineError{
			Kind:  core.CreateRenderPipelineErrorPipelineConstants,
			Label: desc.Label,
			Stage: "vertex",
			Cause: err,
		}
	}
	fs := desc.Fragment
	if fs == nil {
		return nil
	}
	if _, _, _, err := fs.Module.specializedEntryPoint(ir.StageFragment, fs.EntryPoint, fs.Constants); err != nil {
		return &core.CreateRenderPipelineError{
			Kind:  core.CreateRenderPipelineErrorPipelineConstants,
			Label: desc.Label,
			Stage: "fragment",
			Cause: err,
		}
	}
	return nil
}

// fragmentBindingSizes returns the binding sizes of the fragment entry
// point, or nil without a fragment stage.
func fragmentBindingSizes(fs *FragmentState) map[ir.ResourceBinding]uint64 {
	if fs == nil {
		return nil
	}
	return fs.Module.bindingSizes(ir.StageFragment, fs.EntryPoint)
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

// CreateComputePipeline creates a compute pipeline.
func (d *Device) CreateComputePipeline(desc *ComputePipelineDescriptor) (*ComputePipeline, error) {
	if d.released.Load() {
		return nil, ErrReleased
	}
	if desc == nil {
		return nil, fmt.Errorf("wgpu: compute pipeline descriptor is nil")
	}
	// A released layout may already be queued for destruction; the pipeline
	// would be created from a dying HAL layout and would later revive its
	// reference count (P2-CORE-022).
	if desc.Layout != nil && desc.Layout.released {
		return nil, ErrReleased
	}

	// Validate that the device supports compute shaders (downlevel check).
	// Matches Rust wgpu-core resource.rs:4367 — first validation in create_compute_pipeline_or_error.
	if err := d.core.RequireDownlevelFlags(gputypes.DownlevelFlagsComputeShaders); err != nil {
		return nil, fmt.Errorf("wgpu: CreateComputePipeline: %w", err)
	}

	halDevice := d.halDevice()
	if halDevice == nil {
		return nil, ErrReleased
	}

	halDesc := desc.toHAL()

	if err := core.ValidateComputePipelineDescriptor(halDesc); err != nil {
		return nil, err
	}

	if err := d.validateComputeShader(desc); err != nil {
		return nil, err
	}

	halPipeline, err := halDevice.CreateComputePipeline(halDesc)
	if err != nil {
		return nil, &core.CreateComputePipelineError{Kind: core.CreateComputePipelineErrorHAL, Label: desc.Label, HALError: err}
	}

	var bgCount uint32
	var bgLayouts []*BindGroupLayout
	var layoutRef *core.ResourceRef
	if desc.Layout != nil {
		// The pipeline keeps its layout alive until its own HAL pipeline is
		// destroyed: every later SetBindGroup passes the layout to
		// vkCmdBindDescriptorSets. Rust wgpu-core holds an Arc<PipelineLayout>
		// in the pipeline (pipeline.rs:1155).
		layoutRef = desc.Layout.ref
		if layoutRef != nil {
			layoutRef.Clone()
		}
		bgCount = desc.Layout.bindGroupCount
		bgLayouts = desc.Layout.bindGroupLayouts
	}

	// Shader binding sizes of the compute entry point.
	shaderBindingSizes := desc.Module.bindingSizes(ir.StageCompute, desc.EntryPoint)

	lateGroups := makeLateSizedBufferGroups(shaderBindingSizes, bgLayouts)

	// Initialize ResourceRef with onZero callback for refcount-driven destruction.
	// ADR-056: unified resource lifecycle.
	halCP := halPipeline
	cpOnZero := func() {
		// Pipeline first, then the layout it was created from: the layout's
		// own destruction waits for the submissions in flight.
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

	return &ComputePipeline{
		hal:                   halPipeline,
		device:                d,
		bindGroupCount:        bgCount,
		bindGroupLayouts:      bgLayouts,
		lateSizedBufferGroups: lateGroups,
		ref:                   core.NewResourceRef("ComputePipeline:"+desc.Label, cpOnZero),
	}, nil
}

// validateComputeShader checks the compute entry point against the device
// limits, after the pipeline constants are applied: workgroup size
// (VAL-010, Rust wgpu-core validation.rs check_stage) and workgroup storage
// (WebGPU createComputePipeline, workgroupStorageUsed ≤
// maxComputeWorkgroupStorageSize; Dawn ShaderModule.cpp, after the
// workgroup-size checks). A module given as SPIR-V has no IR: its LocalSize
// is checked; workgroup storage is not.
func (d *Device) validateComputeShader(desc *ComputePipelineDescriptor) error {
	if desc.Module.irModule == nil {
		if size, ok := desc.Module.spirvWorkgroupSize(desc.EntryPoint); ok {
			return d.validateComputeWorkgroupSize(desc.Label, size)
		}
		return nil
	}
	irMod, info, index, err := desc.Module.specializedEntryPoint(ir.StageCompute, desc.EntryPoint, desc.Constants)
	if err != nil {
		return &core.CreateComputePipelineError{
			Kind:  core.CreateComputePipelineErrorPipelineConstants,
			Label: desc.Label,
			Cause: err,
		}
	}
	if index < 0 {
		return nil
	}
	if err := d.validateComputeWorkgroupSize(desc.Label, irMod.EntryPoints[index].Workgroup); err != nil {
		return err
	}
	storage := core.WorkgroupStorageSize(irMod, info.EntryPoints[index].GlobalUses)
	if limit := d.core.Limits.MaxComputeWorkgroupStorageSize; storage > uint64(limit) {
		return &core.CreateComputePipelineError{
			Kind:             core.CreateComputePipelineErrorWorkgroupStorageExceeded,
			Label:            desc.Label,
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

	// Check each dimension for zero.
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

	// Check each dimension against device limits.
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

	// Check total invocations (x*y*z) against MaxComputeInvocationsPerWorkgroup.
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

// CreateCommandEncoder creates a command encoder for recording GPU commands.
//
// When a device-level encoder pool is available (BUG-DX12-004), the HAL encoder
// is acquired from the pool instead of creating a new one. This avoids allocating
// expensive GPU resources (DX12 ID3D12CommandAllocator ~64KB, Vulkan VkCommandPool)
// on every frame. After GPU completion, the encoder is reset and returned to the
// pool for reuse. Matches Rust wgpu-core's CommandAllocator pattern (allocator.rs).
func (d *Device) CreateCommandEncoder(desc *CommandEncoderDescriptor) (*CommandEncoder, error) {
	if d.released.Load() {
		return nil, ErrReleased
	}
	if d.test != nil {
		if err := d.test.FaultCreateCommandEncoder(); err != nil {
			return nil, err
		}
	}

	label := ""
	if desc != nil {
		label = desc.Label
	}

	// When pool is available, acquire a recycled HAL encoder and pass it to core.
	// This bypasses core's internal CreateCommandEncoder which would create a new
	// HAL encoder, and instead uses CreateCommandEncoderWithHAL that accepts
	// a pre-existing encoder already in recording state.
	if d.cmdEncoderPool != nil {
		halEnc, err := d.cmdEncoderPool.acquire()
		if err != nil {
			return nil, fmt.Errorf("wgpu: encoder pool acquire: %w", err)
		}

		if err := halEnc.BeginEncoding(label); err != nil {
			// Failed to begin encoding — return encoder to pool for future use.
			d.cmdEncoderPool.release(halEnc)
			return nil, fmt.Errorf("wgpu: begin encoding: %w", err)
		}

		coreEncoder, err := d.core.CreateCommandEncoderWithHAL(halEnc, label)
		if err != nil {
			halEnc.DiscardEncoding()
			d.cmdEncoderPool.release(halEnc)
			return nil, err
		}

		enc := &CommandEncoder{
			core:        coreEncoder,
			device:      d,
			halEncoder:  halEnc,
			trackedRefs: make([]*core.ResourceRef, 0, 64),
		}
		d.recordings.addEncoder(enc)
		d.attachTestHooks(enc, label)
		return enc, nil
	}

	// Fallback: no pool available (e.g., non-HAL device). Use core's built-in
	// encoder creation which creates a fresh HAL encoder each time.
	coreEncoder, err := d.core.CreateCommandEncoder(label)
	if err != nil {
		return nil, err
	}

	enc := &CommandEncoder{core: coreEncoder, device: d}
	d.recordings.addEncoder(enc)
	d.attachTestHooks(enc, label)
	return enc, nil
}

// attachTestHooks binds the wgputest recorder to a user command encoder and
// records its creation. It is a no-op on a device without a recorder.
func (d *Device) attachTestHooks(enc *CommandEncoder, label string) {
	if d.test == nil {
		return
	}
	enc.test = d.test
	enc.testEncoder = d.test.EncoderCreated(label)
}

// CreateFence creates a GPU synchronization fence.
// Fences are primarily used by the HAL internally for synchronization.
// Most callers should use Queue.Submit + Queue.Poll instead.
func (d *Device) CreateFence() (*Fence, error) {
	if d.released.Load() {
		return nil, ErrReleased
	}
	halDevice := d.halDevice()
	if halDevice == nil {
		return nil, ErrReleased
	}

	halFence, err := halDevice.CreateFence()
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to create fence: %w", err)
	}

	return &Fence{hal: halFence, device: d}, nil
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
// The fence must not be in use by the GPU.
func (d *Device) ResetFence(f *Fence) error {
	if d.released.Load() {
		return ErrReleased
	}
	if f == nil || f.released {
		return ErrReleased
	}
	halDevice := d.halDevice()
	if halDevice == nil {
		return ErrReleased
	}
	return halDevice.ResetFence(f.hal)
}

// GetFenceStatus returns true if the fence is signaled (non-blocking).
// This is used for polling completion without blocking.
func (d *Device) GetFenceStatus(f *Fence) (bool, error) {
	if d.released.Load() {
		return false, ErrReleased
	}
	if f == nil || f.released {
		return false, ErrReleased
	}
	halDevice := d.halDevice()
	if halDevice == nil {
		return false, ErrReleased
	}
	return halDevice.GetFenceStatus(f.hal)
}

// WaitForFence waits for a fence to reach the specified value.
// Returns true if the fence reached the value, false if timeout expired.
func (d *Device) WaitForFence(f *Fence, value uint64, timeout time.Duration) (bool, error) {
	if d.released.Load() {
		return false, ErrReleased
	}
	if f == nil || f.released {
		return false, ErrReleased
	}
	halDevice := d.halDevice()
	if halDevice == nil {
		return false, ErrReleased
	}
	return halDevice.Wait(f.hal, value, timeout)
}

// FreeCommandBuffer returns a command buffer to the command pool.
// This must be called after the GPU has finished using the command buffer.
// The command buffer handle becomes invalid after this call.
// For multi-CB encoders, all accumulated HAL command buffers are freed.
func (d *Device) FreeCommandBuffer(cb *CommandBuffer) {
	if d.released.Load() || cb == nil {
		return
	}
	halDevice := d.halDevice()
	if halDevice == nil {
		return
	}
	for _, buf := range cb.halBufferList() {
		if buf != nil {
			halDevice.FreeCommandBuffer(buf)
		}
	}
}

// PushErrorScope pushes a new error scope onto the device's error scope stack.
// It returns an error for a filter that is not one of the three WebGPU error
// filters, and pushes nothing then: the spec throws a TypeError for that value
// (pushErrorScope step 1), and a Go API reports it as an error.
func (d *Device) PushErrorScope(filter ErrorFilter) error {
	return d.core.PushErrorScope(filter)
}

// PopErrorScope pops the most recently pushed error scope.
// Returns the captured error, or nil if no error occurred.
func (d *Device) PopErrorScope() *GPUError {
	return d.core.PopErrorScope()
}

// WaitIdle waits for all GPU work to complete.
func (d *Device) WaitIdle() error {
	if d.released.Load() {
		return ErrReleased
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
		return ErrReleased
	}
	if err := halDevice.WaitIdle(); err != nil {
		return err
	}
	d.maintainAfterIdle()
	return nil
}

// maintainAfterIdle recycles staging buffers and destroys deferred GPU resources
// after the HAL reports the device is idle. Without this, WaitIdle only blocks
// on the GPU — DestroyQueue entries scheduled before the wait are not processed
// until the next Submit, keeping old textures alive across resize boundaries.
func (d *Device) maintainAfterIdle() {
	if d.queue == nil || d.queue.hal == nil {
		return
	}
	completed := d.queue.hal.PollCompleted()
	if dq := d.destroyQueue(); dq != nil {
		dq.Triage(completed)
	}
	if d.queue.pending != nil {
		d.queue.pending.mu.Lock()
		d.queue.pending.maintain(completed)
		d.queue.pending.mu.Unlock()
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
//
// WaitIdle is required because FlushAll calls Triage(PollCompleted()),
// but PollCompleted may return a stale index if GPU hasn't finished.
// Without WaitIdle, deferred encoder recycling callbacks don't fire,
// and pool.destroy() destroys encoders whose VkCommandPool is then
// double-freed by hal.Device.Destroy() → vkDestroyCommandPool crash.
//
// Rust avoids this via Arc ownership + maintain loop. In Go we must
// be explicit: WaitIdle ensures PollCompleted returns final index.
func (d *Device) Release() {
	if d == nil || d.released.Swap(true) {
		return
	}
	var configuredSurfaces []*Surface
	if d.instance != nil {
		configuredSurfaces = d.instance.surfacesForDevice(d)
	}
	// Retire active acquisitions while the backend device is still alive.
	// Vulkan keeps the configured swapchain itself and retires it in
	// hal.Device.Destroy; other backends also get a chance to release borrowed
	// drawable state before their device disappears.
	for _, surface := range configuredSurfaces {
		surface.discardForDevice(d)
	}
	if d.instance != nil {
		defer func() {
			d.instance.unregisterDevice(d)
			d.instance = nil
		}()
	}

	// Step 1: Wait for ALL GPU work to finish. Use the internal path because
	// the public device has already been atomically marked released to close new
	// entry points. This ensures PollCompleted() returns the final submission
	// index before pending encoders or resources are destroyed.
	_ = d.waitIdle()

	// Step 1b: Command encoders never finished or discarded, and command
	// buffers never submitted or released, still hold the references they
	// took while encoding. Drop them now, so the resources they kept alive
	// are destroyed below with the device.
	d.discardRecordings()

	// Step 2: Pending writes that were never submitted can now be discarded;
	// completed inflight batches were recycled by maintainAfterIdle above.
	if d.queue != nil {
		d.queue.release()
	}

	// Step 3: Flush deferred destructions. WaitIdle above ensures PollCompleted()
	// returns the final submission index, so Triage processes all submissions
	// and deferred encoder recycling callbacks fire correctly.
	if d.core != nil && d.core.DestroyQueueRef() != nil {
		dq := d.core.DestroyQueueRef()
		// Triage with latest completion index (GPU is idle, all done).
		if d.queue != nil && d.queue.hal != nil {
			dq.Triage(d.queue.hal.PollCompleted())
		}
		dq.FlushAll()
	}

	// Step 4: Destroy encoder pool. Each encoder.Destroy() calls
	// vkDestroyCommandPool / ID3D12CommandAllocator.Release on the still-alive
	// HAL device. After this, no native encoder resources remain.
	if d.cmdEncoderPool != nil {
		d.cmdEncoderPool.destroy()
		d.cmdEncoderPool = nil
	}

	// Step 5: Destroy core + HAL device. core.Destroy() calls FlushAll again
	// (idempotent — already flushed) then halDevice.Destroy().
	d.core.Destroy()
	for _, surface := range configuredSurfaces {
		surface.retireDevice(d)
	}
}

// destroyQueue returns the device's DestroyQueue for deferred resource destruction.
// Returns nil if the device has no HAL integration or no DestroyQueue.
func (d *Device) destroyQueue() *core.DestroyQueue {
	if d.core == nil {
		return nil
	}
	return d.core.DestroyQueueRef()
}

// Poll drives the per-device pending-map triage loop.
//
// PollPoll drains any buffer mappings whose associated GPU submissions
// have already completed and returns immediately. This is the fast path
// for game loops and is called automatically from Queue.Submit, so
// beginner code paths never need to call Poll explicitly.
//
// PollWait blocks until every currently-pending map resolves: it issues
// WaitIdle on the HAL device, then drains all buckets. This is the
// primary path used by Buffer.Map and is appropriate for shutdown
// drains and short scripts that do not run a render loop.
//
// The return value reports whether Poll observed any in-flight maps —
// tests use it to assert that Submit-driven auto-polling drained
// everything without needing an explicit Poll call.
func (d *Device) Poll(pollType PollType) bool {
	if d == nil || d.core == nil {
		return false
	}
	switch pollType {
	case PollWait:
		// Wait on the HAL device so all submissions complete, then drain.
		if halDev, ok := d.core.HalDeviceHandle(); ok {
			_ = halDev.WaitIdle()
		}
		// Drain with a sentinel "all indices completed".
		return d.core.PollMaps(^uint64(0))
	default:
		completed := uint64(0)
		if d.queue != nil && d.queue.hal != nil {
			completed = d.queue.hal.PollCompleted()
		}
		return d.core.PollMaps(completed)
	}
}

// lastSubmissionIndex returns the latest submission index from the queue.
// Used by Release() methods to schedule deferred destruction.
func (d *Device) lastSubmissionIndex() uint64 {
	if d.queue == nil {
		return 0
	}
	return d.queue.LastSubmissionIndex()
}

// halDevice returns the underlying HAL device for direct resource creation.
func (d *Device) halDevice() hal.Device {
	if d.core == nil || !d.core.HasHAL() {
		return nil
	}
	guard := d.core.SnatchLock().Read()
	defer guard.Release()
	return d.core.Raw(guard)
}

// CreateRenderBundleEncoder creates an encoder for reusable render commands.
// The bundle can run in any render pass with the descriptor's attachment
// formats and sample count, on every backend: core records and replays it.
func (d *Device) CreateRenderBundleEncoder(desc *RenderBundleEncoderDescriptor) (*RenderBundleEncoder, error) {
	if desc == nil {
		return nil, fmt.Errorf("wgpu: render bundle encoder descriptor is nil")
	}

	if d.released.Load() || d.halDevice() == nil {
		return nil, ErrReleased
	}

	encoder, err := newRenderBundleEncoder(d, desc)
	if err != nil {
		return nil, fmt.Errorf("wgpu: CreateRenderBundleEncoder: %w", err)
	}
	return encoder, nil
}
