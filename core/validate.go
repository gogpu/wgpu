//go:build !(js && wasm)

package core

import (
	"fmt"
	"math/bits"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// ValidateTextureDescriptor validates a texture descriptor against device limits.
// Returns nil if valid, or a *CreateTextureError describing the first validation failure.
func ValidateTextureDescriptor(desc *hal.TextureDescriptor, limits gputypes.Limits) error {
	label := desc.Label
	w := desc.Size.Width
	h := desc.Size.Height
	d := desc.Size.DepthOrArrayLayers

	// T17: Dimension must not be Undefined.
	if desc.Dimension == gputypes.TextureDimensionUndefined {
		return &CreateTextureError{
			Kind:  CreateTextureErrorInvalidDimension,
			Label: label,
		}
	}

	// T16: Format must not be Undefined.
	if desc.Format == gputypes.TextureFormatUndefined {
		return &CreateTextureError{
			Kind:  CreateTextureErrorInvalidFormat,
			Label: label,
		}
	}

	// T1-T3: All dimensions must be > 0.
	if w == 0 || h == 0 || d == 0 {
		return &CreateTextureError{
			Kind:            CreateTextureErrorZeroDimension,
			Label:           label,
			RequestedWidth:  w,
			RequestedHeight: h,
			RequestedDepth:  d,
		}
	}

	// T4-T7: Dimension limits depend on texture type.
	if err := validateTextureDimLimits(desc, label, limits); err != nil {
		return err
	}

	// T15: Usage must not be empty.
	if desc.Usage == gputypes.TextureUsageNone {
		return &CreateTextureError{
			Kind:  CreateTextureErrorEmptyUsage,
			Label: label,
		}
	}

	// Usage must not contain unknown bits.
	if desc.Usage.ContainsUnknownBits() {
		return &CreateTextureError{
			Kind:  CreateTextureErrorInvalidUsage,
			Label: label,
		}
	}

	// T8-T9: Mip level count validation.
	if desc.MipLevelCount == 0 {
		return &CreateTextureError{
			Kind:          CreateTextureErrorInvalidMipLevelCount,
			Label:         label,
			RequestedMips: 0,
			MaxMips:       maxMips(desc.Dimension, w, h, d),
		}
	}
	mMax := maxMips(desc.Dimension, w, h, d)
	if desc.MipLevelCount > mMax {
		return &CreateTextureError{
			Kind:          CreateTextureErrorInvalidMipLevelCount,
			Label:         label,
			RequestedMips: desc.MipLevelCount,
			MaxMips:       mMax,
		}
	}

	// T10: Sample count must be 1 or 4.
	if desc.SampleCount != 1 && desc.SampleCount != 4 {
		return &CreateTextureError{
			Kind:             CreateTextureErrorInvalidSampleCount,
			Label:            label,
			RequestedSamples: desc.SampleCount,
		}
	}

	// T11-T14: Multisample constraints.
	if desc.SampleCount > 1 {
		if err := validateTextureMultisample(desc, label); err != nil {
			return err
		}
	}

	return nil
}

// validateTextureDimLimits checks T4-T7 dimension limit constraints.
func validateTextureDimLimits(desc *hal.TextureDescriptor, label string, limits gputypes.Limits) error {
	w := desc.Size.Width
	h := desc.Size.Height
	d := desc.Size.DepthOrArrayLayers

	switch desc.Dimension {
	case gputypes.TextureDimension1D:
		// T4: Width <= maxTextureDimension1D
		if w > limits.MaxTextureDimension1D {
			return &CreateTextureError{
				Kind:           CreateTextureErrorMaxDimension,
				Label:          label,
				RequestedWidth: w,
				MaxDimension:   limits.MaxTextureDimension1D,
			}
		}
		// T7: ArrayLayers <= maxTextureArrayLayers
		if d > limits.MaxTextureArrayLayers {
			return &CreateTextureError{
				Kind:           CreateTextureErrorMaxArrayLayers,
				Label:          label,
				RequestedDepth: d,
				MaxDimension:   limits.MaxTextureArrayLayers,
			}
		}
	case gputypes.TextureDimension2D:
		// T5: Width, Height <= maxTextureDimension2D
		if w > limits.MaxTextureDimension2D || h > limits.MaxTextureDimension2D {
			return &CreateTextureError{
				Kind:            CreateTextureErrorMaxDimension,
				Label:           label,
				RequestedWidth:  w,
				RequestedHeight: h,
				MaxDimension:    limits.MaxTextureDimension2D,
			}
		}
		// T7: ArrayLayers <= maxTextureArrayLayers
		if d > limits.MaxTextureArrayLayers {
			return &CreateTextureError{
				Kind:           CreateTextureErrorMaxArrayLayers,
				Label:          label,
				RequestedDepth: d,
				MaxDimension:   limits.MaxTextureArrayLayers,
			}
		}
	case gputypes.TextureDimension3D:
		// T6: Width, Height, Depth <= maxTextureDimension3D
		if w > limits.MaxTextureDimension3D || h > limits.MaxTextureDimension3D || d > limits.MaxTextureDimension3D {
			return &CreateTextureError{
				Kind:            CreateTextureErrorMaxDimension,
				Label:           label,
				RequestedWidth:  w,
				RequestedHeight: h,
				RequestedDepth:  d,
				MaxDimension:    limits.MaxTextureDimension3D,
			}
		}
	}

	return nil
}

// validateTextureMultisample checks T11-T14 multisample constraints.
func validateTextureMultisample(desc *hal.TextureDescriptor, label string) error {
	// T11: MipLevelCount must be 1.
	if desc.MipLevelCount != 1 {
		return &CreateTextureError{
			Kind:          CreateTextureErrorMultisampleMipLevel,
			Label:         label,
			RequestedMips: desc.MipLevelCount,
		}
	}
	// T12: Dimension must be 2D.
	if desc.Dimension != gputypes.TextureDimension2D {
		return &CreateTextureError{
			Kind:  CreateTextureErrorMultisampleDimension,
			Label: label,
		}
	}
	// T13: DepthOrArrayLayers must be 1.
	if desc.Size.DepthOrArrayLayers != 1 {
		return &CreateTextureError{
			Kind:           CreateTextureErrorMultisampleArrayLayers,
			Label:          label,
			RequestedDepth: desc.Size.DepthOrArrayLayers,
		}
	}
	// T14: Usage must not include StorageBinding.
	if desc.Usage.Contains(gputypes.TextureUsageStorageBinding) {
		return &CreateTextureError{
			Kind:  CreateTextureErrorMultisampleStorageBinding,
			Label: label,
		}
	}
	return nil
}

// ValidateSamplerDescriptor validates a sampler descriptor.
// Returns nil if valid, or a *CreateSamplerError describing the first validation failure.
func ValidateSamplerDescriptor(desc *hal.SamplerDescriptor) error {
	label := desc.Label

	// S1: LodMinClamp >= 0.
	if desc.LodMinClamp < 0 {
		return &CreateSamplerError{
			Kind:        CreateSamplerErrorInvalidLodMinClamp,
			Label:       label,
			LodMinClamp: desc.LodMinClamp,
		}
	}

	// S2: LodMaxClamp >= LodMinClamp.
	if desc.LodMaxClamp < desc.LodMinClamp {
		return &CreateSamplerError{
			Kind:        CreateSamplerErrorInvalidLodMaxClamp,
			Label:       label,
			LodMinClamp: desc.LodMinClamp,
			LodMaxClamp: desc.LodMaxClamp,
		}
	}

	// S3-S6: Anisotropy validation. Treat 0 as 1 (default).
	anisotropy := desc.Anisotropy
	if anisotropy > 1 {
		// S4-S6: Anisotropy > 1 requires linear filtering for mag, min, and mipmap.
		if desc.MagFilter != gputypes.FilterModeLinear ||
			desc.MinFilter != gputypes.FilterModeLinear ||
			desc.MipmapFilter != gputypes.FilterModeLinear {
			return &CreateSamplerError{
				Kind:       CreateSamplerErrorAnisotropyRequiresLinearFiltering,
				Label:      label,
				Anisotropy: anisotropy,
			}
		}
	}

	return nil
}

// ValidateShaderModuleDescriptor validates a shader module descriptor.
// Returns nil if valid, or a *CreateShaderModuleError describing the first validation failure.
func ValidateShaderModuleDescriptor(desc *hal.ShaderModuleDescriptor) error {
	label := desc.Label
	hasWGSL := desc.Source.WGSL != ""
	hasSPIRV := len(desc.Source.SPIRV) > 0

	// SM1: Must have at least one source.
	if !hasWGSL && !hasSPIRV {
		return &CreateShaderModuleError{
			Kind:  CreateShaderModuleErrorNoSource,
			Label: label,
		}
	}

	// SM2: Must not have both.
	if hasWGSL && hasSPIRV {
		return &CreateShaderModuleError{
			Kind:  CreateShaderModuleErrorDualSource,
			Label: label,
		}
	}

	return nil
}

// ValidatePipelineLayoutDescriptor validates a pipeline layout descriptor against device limits.
// Returns nil if valid, or a *CreatePipelineLayoutError describing the first validation failure.
//
// Checks: bind group layout count <= maxBindGroups (typically 4).
// Rust: wgpu-core device/resource.rs:3562-3568.
func ValidatePipelineLayoutDescriptor(desc *hal.PipelineLayoutDescriptor, limits gputypes.Limits) error {
	label := desc.Label

	// PL1: Bind group layout count must not exceed maxBindGroups.
	if len(desc.BindGroupLayouts) > int(limits.MaxBindGroups) {
		return &CreatePipelineLayoutError{
			Kind:      CreatePipelineLayoutErrorTooManyGroups,
			Label:     label,
			Count:     len(desc.BindGroupLayouts),
			MaxGroups: limits.MaxBindGroups,
		}
	}

	return nil
}

// isDepthStencilFormat returns true if the format is a depth and/or stencil format.
// These formats do not have a color aspect and cannot be used as color targets.
//
// Matches Rust wgpu-types TextureFormat::is_depth_stencil_format().
// See: https://gpuweb.github.io/gpuweb/#depth-formats
func isDepthStencilFormat(f gputypes.TextureFormat) bool {
	switch f {
	case gputypes.TextureFormatStencil8,
		gputypes.TextureFormatDepth16Unorm,
		gputypes.TextureFormatDepth24Plus,
		gputypes.TextureFormatDepth24PlusStencil8,
		gputypes.TextureFormatDepth32Float,
		gputypes.TextureFormatDepth32FloatStencil8:
		return true
	}
	return false
}

// hasDepthAspect returns true if the format has a depth aspect.
// Stencil8 is stencil-only and does NOT have a depth aspect.
//
// Matches Rust hal::FormatAspects::from(format).contains(DEPTH).
func hasDepthAspect(f gputypes.TextureFormat) bool {
	switch f {
	case gputypes.TextureFormatDepth16Unorm,
		gputypes.TextureFormatDepth24Plus,
		gputypes.TextureFormatDepth24PlusStencil8,
		gputypes.TextureFormatDepth32Float,
		gputypes.TextureFormatDepth32FloatStencil8:
		return true
	}
	return false
}

// hasStencilAspect returns true if the format has a stencil aspect.
// Depth16Unorm, Depth24Plus, Depth32Float are depth-only and do NOT have a stencil aspect.
//
// Matches Rust hal::FormatAspects::from(format).contains(STENCIL).
func hasStencilAspect(f gputypes.TextureFormat) bool {
	switch f {
	case gputypes.TextureFormatStencil8,
		gputypes.TextureFormatDepth24PlusStencil8,
		gputypes.TextureFormatDepth32FloatStencil8:
		return true
	}
	return false
}

// isDepthEnabled returns true if depth testing is enabled on the depth/stencil state.
// Matches Rust wgpu-types DepthStencilState::is_depth_enabled():
//
//	depth_compare != Always || depth_write_enabled
func isDepthEnabled(ds *hal.DepthStencilState) bool {
	return ds.DepthCompare != gputypes.CompareFunctionAlways || ds.DepthWriteEnabled
}

// stencilFaceIgnored returns true if the stencil face state is the "ignore" state
// (compare=Always, all ops=Keep). Matches Rust StencilFaceState::IGNORE.
func stencilFaceIgnored(face hal.StencilFaceState) bool {
	return face.Compare == gputypes.CompareFunctionAlways &&
		face.FailOp == hal.StencilOperationKeep &&
		face.DepthFailOp == hal.StencilOperationKeep &&
		face.PassOp == hal.StencilOperationKeep
}

// isStencilEnabled returns true if stencil testing is enabled on the depth/stencil state.
// Matches Rust wgpu-types StencilState::is_enabled():
//
//	(front != IGNORE || back != IGNORE) && (read_mask != 0 || write_mask != 0)
func isStencilEnabled(ds *hal.DepthStencilState) bool {
	frontIgnored := stencilFaceIgnored(ds.StencilFront)
	backIgnored := stencilFaceIgnored(ds.StencilBack)
	if frontIgnored && backIgnored {
		return false
	}
	return ds.StencilReadMask != 0 || ds.StencilWriteMask != 0
}

// ValidateRenderPipelineDescriptor validates a render pipeline descriptor against device limits.
// Returns nil if valid, or a *CreateRenderPipelineError describing the first validation failure.
func ValidateRenderPipelineDescriptor(desc *hal.RenderPipelineDescriptor, limits gputypes.Limits) error {
	label := desc.Label

	// RP1: Vertex module must not be nil.
	if desc.Vertex.Module == nil {
		return &CreateRenderPipelineError{
			Kind:  CreateRenderPipelineErrorMissingVertexModule,
			Label: label,
		}
	}

	// RP2: Vertex entry point must not be empty.
	if desc.Vertex.EntryPoint == "" {
		return &CreateRenderPipelineError{
			Kind:  CreateRenderPipelineErrorMissingVertexEntryPoint,
			Label: label,
		}
	}

	// RP3-RP6: Fragment stage validation (if present).
	if desc.Fragment != nil {
		if err := validateFragmentStage(desc.Fragment, label, limits); err != nil {
			return err
		}
	}

	// RP8: Color target formats must be color formats (not depth/stencil).
	// Rust: resource.rs:4083-4085 — FormatNotColor
	if desc.Fragment != nil {
		for i, ct := range desc.Fragment.Targets {
			if ct.Format != gputypes.TextureFormatUndefined && isDepthStencilFormat(ct.Format) {
				return &CreateRenderPipelineError{
					Kind:        CreateRenderPipelineErrorColorTargetDepthFormat,
					Label:       label,
					TargetIndex: uint32(i),
					Format:      ct.Format.String(),
				}
			}
		}
	}

	// RP9: Depth/stencil format must be a depth/stencil format (not color).
	// Rust: resource.rs:4144-4165 — FormatNotDepth / FormatNotStencil
	if desc.DepthStencil != nil {
		if desc.DepthStencil.Format != gputypes.TextureFormatUndefined && !isDepthStencilFormat(desc.DepthStencil.Format) {
			return &CreateRenderPipelineError{
				Kind:   CreateRenderPipelineErrorDepthStencilColorFormat,
				Label:  label,
				Format: desc.DepthStencil.Format.String(),
			}
		}

		// RP9b: If depth operations are enabled, format must have a depth aspect.
		// Rust: resource.rs:4158 — ds.is_depth_enabled() && !aspect.contains(DEPTH)
		// is_depth_enabled: depth_compare != Always || depth_write_enabled
		if isDepthEnabled(desc.DepthStencil) && !hasDepthAspect(desc.DepthStencil.Format) {
			return &CreateRenderPipelineError{
				Kind:   CreateRenderPipelineErrorDepthFormatNoDepthAspect,
				Label:  label,
				Format: desc.DepthStencil.Format.String(),
			}
		}

		// RP9c: If stencil operations are enabled, format must have a stencil aspect.
		// Rust: resource.rs:4161 — ds.stencil.is_enabled() && !aspect.contains(STENCIL)
		if isStencilEnabled(desc.DepthStencil) && !hasStencilAspect(desc.DepthStencil.Format) {
			return &CreateRenderPipelineError{
				Kind:   CreateRenderPipelineErrorDepthFormatNoStencilAspect,
				Label:  label,
				Format: desc.DepthStencil.Format.String(),
			}
		}
	}

	// RP7: SampleCount must be 1 or 4.
	if desc.Multisample.Count != 0 && desc.Multisample.Count != 1 && desc.Multisample.Count != 4 {
		return &CreateRenderPipelineError{
			Kind:        CreateRenderPipelineErrorInvalidSampleCount,
			Label:       label,
			SampleCount: desc.Multisample.Count,
		}
	}

	return nil
}

// validateFragmentStage checks RP3-RP6 fragment stage constraints.
func validateFragmentStage(frag *hal.FragmentState, label string, limits gputypes.Limits) error {
	// RP3: Fragment module must not be nil.
	if frag.Module == nil {
		return &CreateRenderPipelineError{
			Kind:  CreateRenderPipelineErrorMissingFragmentModule,
			Label: label,
		}
	}
	// RP4: Fragment entry point must not be empty.
	if frag.EntryPoint == "" {
		return &CreateRenderPipelineError{
			Kind:  CreateRenderPipelineErrorMissingFragmentEntryPoint,
			Label: label,
		}
	}
	// RP5: Must have at least 1 target.
	if len(frag.Targets) == 0 {
		return &CreateRenderPipelineError{
			Kind:  CreateRenderPipelineErrorNoFragmentTargets,
			Label: label,
		}
	}
	// RP6: Color targets count <= maxColorAttachments.
	targetCount := uint32(len(frag.Targets)) //nolint:gosec // len bounded by MaxColorAttachments check
	if targetCount > limits.MaxColorAttachments {
		return &CreateRenderPipelineError{
			Kind:        CreateRenderPipelineErrorTooManyColorTargets,
			Label:       label,
			TargetCount: targetCount,
			MaxTargets:  limits.MaxColorAttachments,
		}
	}
	return nil
}

// ValidateComputePipelineDescriptor validates a compute pipeline descriptor.
// Returns nil if valid, or a *CreateComputePipelineError describing the first validation failure.
func ValidateComputePipelineDescriptor(desc *hal.ComputePipelineDescriptor) error {
	label := desc.Label

	// CP1: Module must not be nil.
	if desc.Compute.Module == nil {
		return &CreateComputePipelineError{
			Kind:  CreateComputePipelineErrorMissingModule,
			Label: label,
		}
	}

	// CP2: EntryPoint must not be empty.
	if desc.Compute.EntryPoint == "" {
		return &CreateComputePipelineError{
			Kind:  CreateComputePipelineErrorMissingEntryPoint,
			Label: label,
		}
	}

	return nil
}

// ValidateBindGroupLayoutDescriptor validates a bind group layout descriptor against device limits.
// Returns nil if valid, or a *CreateBindGroupLayoutError describing the first validation failure.
func ValidateBindGroupLayoutDescriptor(desc *hal.BindGroupLayoutDescriptor, limits gputypes.Limits) error {
	label := desc.Label

	// BGL2: Number of entries <= maxBindingsPerBindGroup.
	entryCount := uint32(len(desc.Entries)) //nolint:gosec // len bounded by MaxBindingsPerBindGroup check
	if entryCount > limits.MaxBindingsPerBindGroup {
		return &CreateBindGroupLayoutError{
			Kind:         CreateBindGroupLayoutErrorTooManyBindings,
			Label:        label,
			BindingCount: entryCount,
			MaxBindings:  limits.MaxBindingsPerBindGroup,
		}
	}

	// BGL1: Entry binding numbers must be unique.
	// Also count per-stage resource usage for limit validation.
	seen := make(map[uint32]struct{}, len(desc.Entries))
	var storageBuffers, uniformBuffers, samplers, sampledTextures, storageTextures uint32
	for _, entry := range desc.Entries {
		if _, ok := seen[entry.Binding]; ok {
			return &CreateBindGroupLayoutError{
				Kind:             CreateBindGroupLayoutErrorDuplicateBinding,
				Label:            label,
				DuplicateBinding: entry.Binding,
			}
		}
		seen[entry.Binding] = struct{}{}

		// Count resources by type for per-stage limit checks.
		if entry.Buffer != nil {
			switch entry.Buffer.Type {
			case gputypes.BufferBindingTypeStorage, gputypes.BufferBindingTypeReadOnlyStorage:
				storageBuffers++
			case gputypes.BufferBindingTypeUniform:
				uniformBuffers++
			}
		}
		if entry.Sampler != nil {
			samplers++
		}
		if entry.Texture != nil {
			sampledTextures++
		}
		if entry.StorageTexture != nil {
			storageTextures++
		}
	}

	// BGL3: Per-stage resource limits.
	if limits.MaxStorageBuffersPerShaderStage > 0 && storageBuffers > limits.MaxStorageBuffersPerShaderStage {
		return fmt.Errorf("bind group layout %q: %d storage buffers exceeds limit %d",
			label, storageBuffers, limits.MaxStorageBuffersPerShaderStage)
	}
	if limits.MaxUniformBuffersPerShaderStage > 0 && uniformBuffers > limits.MaxUniformBuffersPerShaderStage {
		return fmt.Errorf("bind group layout %q: %d uniform buffers exceeds limit %d",
			label, uniformBuffers, limits.MaxUniformBuffersPerShaderStage)
	}
	if limits.MaxSamplersPerShaderStage > 0 && samplers > limits.MaxSamplersPerShaderStage {
		return fmt.Errorf("bind group layout %q: %d samplers exceeds limit %d",
			label, samplers, limits.MaxSamplersPerShaderStage)
	}
	if limits.MaxSampledTexturesPerShaderStage > 0 && sampledTextures > limits.MaxSampledTexturesPerShaderStage {
		return fmt.Errorf("bind group layout %q: %d sampled textures exceeds limit %d",
			label, sampledTextures, limits.MaxSampledTexturesPerShaderStage)
	}
	if limits.MaxStorageTexturesPerShaderStage > 0 && storageTextures > limits.MaxStorageTexturesPerShaderStage {
		return fmt.Errorf("bind group layout %q: %d storage textures exceeds limit %d",
			label, storageTextures, limits.MaxStorageTexturesPerShaderStage)
	}

	return nil
}

// BindGroupBufferInfo carries buffer metadata needed for bind group validation.
// The core validation layer cannot access buffer objects directly (they are uintptr
// handles in gputypes.BindGroupEntry), so the caller extracts this info from the
// public API's typed *Buffer objects and passes it alongside the descriptor.
type BindGroupBufferInfo struct {
	// Binding is the binding number this info corresponds to.
	Binding uint32
	// Usage is the buffer's usage flags (from Buffer.Usage()).
	Usage gputypes.BufferUsage
	// BufferSize is the total size of the buffer in bytes (from Buffer.Size()).
	BufferSize uint64
	// Offset is the byte offset into the buffer for this binding.
	Offset uint64
	// Size is the requested binding size (0 means "rest of buffer from offset").
	Size uint64
}

// ValidateBindGroupDescriptor validates a bind group descriptor.
// layoutEntries are the entries from the bind group layout — passed separately because
// the hal.BindGroupLayout interface does not expose entries (they live in core.BindGroupLayout).
// bufferInfos carries buffer metadata for entries that bind buffers (may be nil if no buffers).
// limits are the device limits for alignment and size checks.
// Returns nil if valid, or a *CreateBindGroupError describing the first validation failure.
func ValidateBindGroupDescriptor(
	desc *hal.BindGroupDescriptor,
	layoutEntries []gputypes.BindGroupLayoutEntry,
	bufferInfos []BindGroupBufferInfo,
	limits gputypes.Limits,
) error {
	// BG1: Layout must not be nil.
	if desc.Layout == nil {
		return &CreateBindGroupError{
			Kind:  CreateBindGroupErrorMissingLayout,
			Label: desc.Label,
		}
	}

	// BG2: Entry count must match layout entry count.
	// Rust: wgpu-core device/resource.rs:3106-3111 — BindingsNumMismatch
	if len(desc.Entries) != len(layoutEntries) {
		return &CreateBindGroupError{
			Kind:     CreateBindGroupErrorBindingsNumMismatch,
			Label:    desc.Label,
			Expected: len(layoutEntries),
			Actual:   len(desc.Entries),
		}
	}

	// BG3: Each entry binding must exist in layout, and no duplicates.
	// Rust: wgpu-core device/resource.rs:3135-3138 — MissingBindingDeclaration
	// Rust: wgpu-core device/resource.rs:3275-3278 — DuplicateBinding
	//
	// Build a map of layout entries by binding number for O(1) lookup.
	layoutByBinding := make(map[uint32]*gputypes.BindGroupLayoutEntry, len(layoutEntries))
	for i := range layoutEntries {
		layoutByBinding[layoutEntries[i].Binding] = &layoutEntries[i]
	}

	seen := make(map[uint32]bool, len(desc.Entries))
	for _, entry := range desc.Entries {
		// Check duplicate binding numbers in descriptor entries.
		if seen[entry.Binding] {
			return &CreateBindGroupError{
				Kind:    CreateBindGroupErrorDuplicateBinding,
				Label:   desc.Label,
				Binding: entry.Binding,
			}
		}
		seen[entry.Binding] = true

		// Check that each entry binding has a corresponding declaration in the layout.
		if _, ok := layoutByBinding[entry.Binding]; !ok {
			return &CreateBindGroupError{
				Kind:    CreateBindGroupErrorMissingBindingDeclaration,
				Label:   desc.Label,
				Binding: entry.Binding,
			}
		}
	}

	// BG4-BG9: Buffer binding validation.
	// Rust: wgpu-core device/resource.rs:2747-2834
	return validateBindGroupBufferEntries(desc.Label, layoutByBinding, bufferInfos, limits)
}

// validateBindGroupBufferEntries validates each buffer binding entry against its
// layout declaration and device limits. Checks usage compatibility, offset alignment,
// binding size limits, bounds overflow, zero-size, and storage 4-byte alignment.
//
// Rust reference: wgpu-core device/resource.rs:2747-2834
func validateBindGroupBufferEntries(
	label string,
	layoutByBinding map[uint32]*gputypes.BindGroupLayoutEntry,
	bufferInfos []BindGroupBufferInfo,
	limits gputypes.Limits,
) error {
	for _, info := range bufferInfos {
		layoutEntry, ok := layoutByBinding[info.Binding]
		if !ok || layoutEntry.Buffer == nil {
			// No buffer layout declaration for this binding -- skip.
			// (This would be caught by resource type mismatch validation, not buffer validation.)
			continue
		}

		if err := validateSingleBufferEntry(label, layoutEntry.Buffer, info, limits); err != nil {
			return err
		}
	}

	return nil
}

// bufferBindingParams holds the resolved parameters for a buffer binding type.
type bufferBindingParams struct {
	requiredUsage    gputypes.BufferUsage
	maxBindingSize   uint64
	offsetAlignment  uint32
	isStorageBinding bool
}

// resolveBufferBindingParams returns the required usage, max size, alignment, and
// whether it is a storage binding for the given buffer binding type.
// Returns ok=false for unknown binding types.
func resolveBufferBindingParams(bindingType gputypes.BufferBindingType, limits gputypes.Limits) (bufferBindingParams, bool) {
	switch bindingType {
	case gputypes.BufferBindingTypeUniform:
		return bufferBindingParams{
			requiredUsage:    gputypes.BufferUsageUniform,
			maxBindingSize:   limits.MaxUniformBufferBindingSize,
			offsetAlignment:  limits.MinUniformBufferOffsetAlignment,
			isStorageBinding: false,
		}, true
	case gputypes.BufferBindingTypeStorage, gputypes.BufferBindingTypeReadOnlyStorage:
		return bufferBindingParams{
			requiredUsage:    gputypes.BufferUsageStorage,
			maxBindingSize:   limits.MaxStorageBufferBindingSize,
			offsetAlignment:  limits.MinStorageBufferOffsetAlignment,
			isStorageBinding: true,
		}, true
	default:
		return bufferBindingParams{}, false
	}
}

// validateSingleBufferEntry validates one buffer binding entry against its layout
// declaration and device limits.
func validateSingleBufferEntry(
	label string,
	bufLayout *gputypes.BufferBindingLayout,
	info BindGroupBufferInfo,
	limits gputypes.Limits,
) error {
	params, ok := resolveBufferBindingParams(bufLayout.Type, limits)
	if !ok {
		return nil // Unknown binding type -- skip.
	}

	// BG4: Buffer usage must include the required flag.
	if !info.Usage.Contains(params.requiredUsage) {
		return &CreateBindGroupError{
			Kind:          CreateBindGroupErrorBufferUsageMismatch,
			Label:         label,
			Binding:       info.Binding,
			ExpectedUsage: uint64(params.requiredUsage),
			ActualUsage:   uint64(info.Usage),
		}
	}

	// BG5: Buffer offset must be aligned.
	if params.offsetAlignment > 0 && info.Offset%uint64(params.offsetAlignment) != 0 {
		return &CreateBindGroupError{
			Kind:      CreateBindGroupErrorBufferOffsetAlignment,
			Label:     label,
			Binding:   info.Binding,
			Offset:    info.Offset,
			Alignment: uint64(params.offsetAlignment),
		}
	}

	// Resolve effective binding size: 0 means "rest of buffer from offset".
	effectiveSize := info.Size
	if effectiveSize == 0 && info.Offset <= info.BufferSize {
		effectiveSize = info.BufferSize - info.Offset
	}

	// BG9: Effective binding size must not be zero.
	// Rust: wgpu-core device/resource.rs:2828 -- BindingZeroSize
	if effectiveSize == 0 {
		return &CreateBindGroupError{
			Kind:    CreateBindGroupErrorBufferBindingZeroSize,
			Label:   label,
			Binding: info.Binding,
		}
	}

	// BG7: offset + bindingSize must not exceed buffer size.
	// Rust: wgpu-core resource.rs:517-535 -- BindingRangeTooLarge / BindingOffsetTooLarge
	if err := validateBufferBounds(label, info); err != nil {
		return err
	}

	// BG8: Storage buffer binding size must be a multiple of 4.
	// Rust: wgpu-core device/resource.rs:2784-2793
	if params.isStorageBinding && effectiveSize%4 != 0 {
		return &CreateBindGroupError{
			Kind:    CreateBindGroupErrorStorageBufferSizeAlignment,
			Label:   label,
			Binding: info.Binding,
			Size:    effectiveSize,
		}
	}

	// BG6: Effective binding size must not exceed the maximum for its type.
	// Rust: wgpu-core device/resource.rs:2798-2804 -- BufferRangeTooLarge
	if effectiveSize > params.maxBindingSize {
		return &CreateBindGroupError{
			Kind:    CreateBindGroupErrorBufferBindingSizeTooLarge,
			Label:   label,
			Binding: info.Binding,
			Size:    effectiveSize,
			MaxSize: params.maxBindingSize,
		}
	}

	// BG10: If MinBindingSize is set, effective binding size must be >= MinBindingSize.
	// Rust: wgpu-core device/resource.rs:2817-2826 -- BindingSizeTooSmall
	if bufLayout.MinBindingSize > 0 && effectiveSize < bufLayout.MinBindingSize {
		return &CreateBindGroupError{
			Kind:           CreateBindGroupErrorMinBindingSizeMismatch,
			Label:          label,
			Binding:        info.Binding,
			Size:           effectiveSize,
			MinBindingSize: bufLayout.MinBindingSize,
		}
	}

	return nil
}

// validateBufferBounds checks that offset + size does not exceed the buffer size.
func validateBufferBounds(label string, info BindGroupBufferInfo) error {
	if info.Size != 0 {
		// Explicit size: check offset + size <= bufferSize (with overflow protection).
		end, overflow := addUint64(info.Offset, info.Size)
		if overflow || end > info.BufferSize {
			return &CreateBindGroupError{
				Kind:       CreateBindGroupErrorBufferBoundsOverflow,
				Label:      label,
				Binding:    info.Binding,
				Offset:     info.Offset,
				Size:       info.Size,
				BufferSize: info.BufferSize,
			}
		}
	} else if info.Offset > info.BufferSize {
		// Implicit size (0 = rest of buffer): offset must not exceed buffer size.
		return &CreateBindGroupError{
			Kind:       CreateBindGroupErrorBufferBoundsOverflow,
			Label:      label,
			Binding:    info.Binding,
			Offset:     info.Offset,
			Size:       0,
			BufferSize: info.BufferSize,
		}
	}
	return nil
}

// addUint64 adds a and b, returning the result and whether it overflowed.
func addUint64(a, b uint64) (uint64, bool) {
	sum := a + b
	return sum, sum < a
}

// maxMips calculates the maximum number of mip levels for a texture.
func maxMips(dimension gputypes.TextureDimension, width, height, depth uint32) uint32 {
	var maxDim uint32
	switch dimension {
	case gputypes.TextureDimension1D:
		maxDim = width
	case gputypes.TextureDimension2D:
		maxDim = max(width, height)
	case gputypes.TextureDimension3D:
		maxDim = max(width, max(height, depth))
	}
	if maxDim == 0 {
		return 0
	}
	return uint32(bits.Len32(maxDim)) //nolint:gosec // bits.Len32 returns 0..32, always fits uint32
}
