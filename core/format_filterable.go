//go:build !(js && wasm)

package core

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// isUnfilterable32BitFloatFormat reports 32-bit float formats that require
// FeatureFloat32Filterable for linear/mipmap filtering (WebGPU spec).
func isUnfilterable32BitFloatFormat(format gputypes.TextureFormat) bool {
	switch format {
	case gputypes.TextureFormatR32Float,
		gputypes.TextureFormatRG32Float,
		gputypes.TextureFormatRGBA32Float:
		return true
	default:
		return false
	}
}

// samplerUsesFiltering reports whether a sampler applies anything other than
// nearest-neighbor filtering (mag, min, or mipmap).
func samplerUsesFiltering(desc *hal.SamplerDescriptor) bool {
	if desc == nil {
		return false
	}
	return desc.MagFilter == gputypes.FilterModeLinear ||
		desc.MinFilter == gputypes.FilterModeLinear ||
		desc.MipmapFilter == gputypes.FilterModeLinear
}

// BindGroupSamplerInfo carries sampler metadata for bind-group validation.
type BindGroupSamplerInfo struct {
	Binding uint32
	Desc    *hal.SamplerDescriptor
}

// BindGroupTextureInfo carries texture metadata for bind-group validation.
type BindGroupTextureInfo struct {
	Binding uint32
	Format  gputypes.TextureFormat
}

// validateFloat32FilterableBindings checks VAL-C21: filtering unfilterable
// 32-bit float textures requires FeatureFloat32Filterable.
func validateFloat32FilterableBindings(
	layoutEntries []gputypes.BindGroupLayoutEntry,
	samplerInfos []BindGroupSamplerInfo,
	textureInfos []BindGroupTextureInfo,
	features gputypes.Features,
) error {
	if len(textureInfos) == 0 {
		return nil
	}

	layoutByBinding := make(map[uint32]*gputypes.BindGroupLayoutEntry, len(layoutEntries))
	for i := range layoutEntries {
		layoutByBinding[layoutEntries[i].Binding] = &layoutEntries[i]
	}

	filteringSampler := false
	for _, info := range samplerInfos {
		if samplerUsesFiltering(info.Desc) {
			filteringSampler = true
			break
		}
	}

	for _, tex := range textureInfos {
		if !isUnfilterable32BitFloatFormat(tex.Format) {
			continue
		}
		layoutEntry := layoutByBinding[tex.Binding]
		if layoutEntry == nil || layoutEntry.Texture == nil {
			continue
		}
		sampleType := layoutEntry.Texture.SampleType
		switch sampleType {
		case gputypes.TextureSampleTypeFloat:
			if err := RequireFeature(features, gputypes.FeatureFloat32Filterable, "CreateBindGroup"); err != nil {
				return err
			}
		case gputypes.TextureSampleTypeUnfilterableFloat:
			if filteringSampler {
				if err := RequireFeature(features, gputypes.FeatureFloat32Filterable, "CreateBindGroup"); err != nil {
					return err
				}
			}
		}
	}

	return nil
}
