package types

// Feature represents a WebGPU feature flag.
type Feature uint64

const (
	// FeatureDepthClipControl enables depth clip control.
	FeatureDepthClipControl Feature = 1 << iota
	// FeatureDepth32FloatStencil8 enables depth32float-stencil8 format.
	FeatureDepth32FloatStencil8
	// FeatureTextureCompressionBC enables BC texture compression.
	FeatureTextureCompressionBC
	// FeatureTextureCompressionETC2 enables ETC2 texture compression.
	FeatureTextureCompressionETC2
	// FeatureTextureCompressionASTC enables ASTC texture compression.
	FeatureTextureCompressionASTC
	// FeatureIndirectFirstInstance enables indirect draw first instance.
	FeatureIndirectFirstInstance
	// FeatureShaderF16 enables f16 in shaders.
	FeatureShaderF16
	// FeatureRG11B10UfloatRenderable enables RG11B10Ufloat as render target.
	FeatureRG11B10UfloatRenderable
	// FeatureBGRA8UnormStorage enables BGRA8Unorm as storage texture.
	FeatureBGRA8UnormStorage
	// FeatureFloat32Filterable enables filtering of float32 textures.
	FeatureFloat32Filterable
	// FeatureTimestampQuery enables timestamp queries.
	FeatureTimestampQuery
	// FeaturePipelineStatisticsQuery enables pipeline statistics queries.
	FeaturePipelineStatisticsQuery
	// FeatureMultiDrawIndirect enables multi-draw indirect commands.
	FeatureMultiDrawIndirect
	// FeatureMultiDrawIndirectCount enables multi-draw indirect count.
	FeatureMultiDrawIndirectCount
	// FeaturePushConstants enables push constants.
	FeaturePushConstants
	// FeatureTextureAdapterSpecificFormatFeatures enables adapter-specific formats.
	FeatureTextureAdapterSpecificFormatFeatures
	// FeatureShaderFloat64 enables f64 in shaders.
	FeatureShaderFloat64
	// FeatureVertexAttribute64bit enables 64-bit vertex attributes.
	FeatureVertexAttribute64bit
	// FeatureSubgroupOperations enables subgroup operations in shaders.
	FeatureSubgroupOperations
	// FeatureSubgroupBarrier enables subgroup barriers.
	FeatureSubgroupBarrier
)

// Features is a set of feature flags.
type Features uint64

// Contains checks if the feature set contains a specific feature.
func (f Features) Contains(feature Feature) bool {
	return f&Features(feature) != 0
}

// ContainsAll checks if the feature set contains all specified features.
func (f Features) ContainsAll(other Features) bool {
	return f&other == other
}

// Insert adds a feature to the set.
func (f *Features) Insert(feature Feature) {
	*f |= Features(feature)
}

// Remove removes a feature from the set.
func (f *Features) Remove(feature Feature) {
	*f &^= Features(feature)
}

// Intersect returns features common to both sets.
func (f Features) Intersect(other Features) Features {
	return f & other
}

// Union returns all features from both sets.
func (f Features) Union(other Features) Features {
	return f | other
}
