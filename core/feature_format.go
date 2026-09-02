//go:build !(js && wasm)

package core

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

func isBCFormat(format gputypes.TextureFormat) bool {
	return format >= gputypes.TextureFormatBC1RGBAUnorm &&
		format <= gputypes.TextureFormatBC7RGBAUnormSrgb
}

func isETC2Format(format gputypes.TextureFormat) bool {
	return format >= gputypes.TextureFormatETC2RGB8Unorm &&
		format <= gputypes.TextureFormatEACRG11Snorm
}

func isASTCFormat(format gputypes.TextureFormat) bool {
	return format >= gputypes.TextureFormatASTC4x4Unorm &&
		format <= gputypes.TextureFormatASTC12x12UnormSrgb
}

func is64BitVertexFormat(_ gputypes.VertexFormat) bool {
	// gputypes does not yet expose 64-bit vertex attribute formats; keep the
	// hook ready for FeatureVertexAttribute64bit (VAL-C13).
	return false
}

// validateTextureFormatFeatures checks feature gates for texture formats (VAL-C6..C11).
func validateTextureFormatFeatures(
	format gputypes.TextureFormat,
	usage gputypes.TextureUsage,
	features gputypes.Features,
) error {
	switch {
	case format == gputypes.TextureFormatDepth32FloatStencil8:
		if err := RequireFeature(features, gputypes.FeatureDepth32FloatStencil8, "CreateTexture"); err != nil {
			return err
		}
	case isBCFormat(format):
		if err := RequireFeature(features, gputypes.FeatureTextureCompressionBC, "CreateTexture"); err != nil {
			return err
		}
	case isETC2Format(format):
		if err := RequireFeature(features, gputypes.FeatureTextureCompressionETC2, "CreateTexture"); err != nil {
			return err
		}
	case isASTCFormat(format):
		if err := RequireFeature(features, gputypes.FeatureTextureCompressionASTC, "CreateTexture"); err != nil {
			return err
		}
	}

	if format == gputypes.TextureFormatRG11B10Ufloat {
		if usage.Contains(gputypes.TextureUsageRenderAttachment) {
			if err := RequireFeature(features, gputypes.FeatureRG11B10UfloatRenderable, "CreateTexture"); err != nil {
				return err
			}
		}
	}

	if format == gputypes.TextureFormatBGRA8Unorm && usage.Contains(gputypes.TextureUsageStorageBinding) {
		if err := RequireFeature(features, gputypes.FeatureBGRA8UnormStorage, "CreateTexture"); err != nil {
			return err
		}
	}

	return nil
}

// validateRenderPipelineFormatFeatures checks format-related feature gates (VAL-C5, VAL-C10, VAL-C13).
func validateRenderPipelineFormatFeatures(desc *hal.RenderPipelineDescriptor, features gputypes.Features) error {
	label := desc.Label

	// VAL-C5: Unclipped depth requires FeatureDepthClipControl.
	if desc.Primitive.UnclippedDepth {
		if err := RequireFeature(features, gputypes.FeatureDepthClipControl, "CreateRenderPipeline"); err != nil {
			return err
		}
	}

	if desc.Fragment != nil {
		for _, ct := range desc.Fragment.Targets {
			if ct.Format == gputypes.TextureFormatRG11B10Ufloat {
				if err := RequireFeature(features, gputypes.FeatureRG11B10UfloatRenderable, "CreateRenderPipeline"); err != nil {
					return err
				}
			}
		}
	}

	for _, buf := range desc.Vertex.Buffers {
		for _, attr := range buf.Attributes {
			if is64BitVertexFormat(attr.Format) {
				if err := RequireFeature(features, gputypes.FeatureVertexAttribute64bit, "CreateRenderPipeline"); err != nil {
					return err
				}
			}
		}
	}

	_ = label
	return nil
}
