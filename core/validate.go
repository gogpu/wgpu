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

// ValidateBindGroupDescriptor validates a bind group descriptor.
// Returns nil if valid, or a *CreateBindGroupError describing the first validation failure.
func ValidateBindGroupDescriptor(desc *hal.BindGroupDescriptor) error {
	// BG1: Layout must not be nil.
	if desc.Layout == nil {
		return &CreateBindGroupError{
			Kind:  CreateBindGroupErrorMissingLayout,
			Label: desc.Label,
		}
	}

	return nil
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
