//go:build !(js && wasm)

package core

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

func TestValidateTextureDescriptor_CompressionBC_NoFeature(t *testing.T) {
	t.Parallel()
	desc := validTextureDesc()
	desc.Format = gputypes.TextureFormatBC1RGBAUnorm
	err := ValidateTextureDescriptor(desc, gputypes.DefaultLimits(), gputypes.Features(0))
	if !IsFeatureError(err) {
		t.Fatalf("expected feature error, got %T: %v", err, err)
	}
}

func TestValidateTextureDescriptor_CompressionBC_WithFeature(t *testing.T) {
	t.Parallel()
	desc := validTextureDesc()
	desc.Format = gputypes.TextureFormatBC1RGBAUnorm
	features := gputypes.Features(gputypes.FeatureTextureCompressionBC)
	err := ValidateTextureDescriptor(desc, gputypes.DefaultLimits(), features)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateTextureDescriptor_Depth32FloatStencil8_NoFeature(t *testing.T) {
	t.Parallel()
	desc := validTextureDesc()
	desc.Format = gputypes.TextureFormatDepth32FloatStencil8
	desc.Usage = gputypes.TextureUsageTextureBinding
	err := ValidateTextureDescriptor(desc, gputypes.DefaultLimits(), gputypes.Features(0))
	if !IsFeatureError(err) {
		t.Fatalf("expected feature error, got %T: %v", err, err)
	}
}

func TestValidateTextureDescriptor_BGRA8Storage_NoFeature(t *testing.T) {
	t.Parallel()
	desc := validTextureDesc()
	desc.Format = gputypes.TextureFormatBGRA8Unorm
	desc.Usage = gputypes.TextureUsageStorageBinding
	err := ValidateTextureDescriptor(desc, gputypes.DefaultLimits(), gputypes.Features(0))
	if !IsFeatureError(err) {
		t.Fatalf("expected feature error, got %T: %v", err, err)
	}
}

func TestValidateTextureUsageFlags_CompressedRenderAttachment(t *testing.T) {
	t.Parallel()
	err := ValidateTextureUsageFlags(
		gputypes.TextureUsageRenderAttachment,
		1,
		gputypes.TextureFormatBC1RGBAUnorm,
	)
	if err == nil {
		t.Fatal("expected invalid usage for compressed render attachment")
	}
}

func TestValidateTextureUsageFlags_StorageAndRenderAttachment(t *testing.T) {
	t.Parallel()
	err := ValidateTextureUsageFlags(
		gputypes.TextureUsageStorageBinding|gputypes.TextureUsageRenderAttachment,
		1,
		gputypes.TextureFormatRGBA8Unorm,
	)
	if err != nil {
		t.Fatalf("storage+render attachment is valid at creation: %v", err)
	}
}

func TestValidateTextureUsageFlags_DepthRenderAttachment(t *testing.T) {
	t.Parallel()
	err := ValidateTextureUsageFlags(
		gputypes.TextureUsageRenderAttachment,
		1,
		gputypes.TextureFormatDepth24Plus,
	)
	if err != nil {
		t.Fatalf("depth render attachment is valid: %v", err)
	}
}

func TestValidateRenderPipelineDescriptor_UnclippedDepth_NoFeature(t *testing.T) {
	t.Parallel()
	desc := &hal.RenderPipelineDescriptor{
		Label: "unclipped",
		Vertex: hal.VertexState{
			Module:     mockShaderModule{},
			EntryPoint: "vs_main",
		},
		Primitive:   gputypes.PrimitiveState{UnclippedDepth: true},
		Multisample: gputypes.MultisampleState{Count: 1},
	}
	err := ValidateRenderPipelineDescriptor(desc, gputypes.DefaultLimits(), gputypes.Features(0))
	if !IsFeatureError(err) {
		t.Fatalf("expected feature error, got %T: %v", err, err)
	}
}

func TestValidateShaderModuleDescriptor_F16_NoFeature(t *testing.T) {
	t.Parallel()
	desc := &hal.ShaderModuleDescriptor{
		Label: "f16",
		Source: hal.ShaderSource{
			WGSL: `@vertex fn main() -> @builtin(position) vec4<f32> {
			 var x: f16 = 1.0;
			 return vec4<f32>(f32(x));
			}`,
		},
	}
	err := ValidateShaderModuleDescriptor(desc, gputypes.Features(0))
	if !IsFeatureError(err) {
		t.Fatalf("expected feature error, got %T: %v", err, err)
	}
}

func TestValidateQuerySetDescriptor_Timestamp_NoFeature(t *testing.T) {
	t.Parallel()
	err := ValidateQuerySetDescriptor(&hal.QuerySetDescriptor{
		Type:  hal.QueryTypeTimestamp,
		Count: 2,
	}, gputypes.Features(0))
	if !IsFeatureError(err) {
		t.Fatalf("expected feature error, got %T: %v", err, err)
	}
}

func TestValidateQuerySetDescriptor_Timestamp_WithFeature(t *testing.T) {
	t.Parallel()
	err := ValidateQuerySetDescriptor(&hal.QuerySetDescriptor{
		Type:  hal.QueryTypeTimestamp,
		Count: 2,
	}, gputypes.Features(gputypes.FeatureTimestampQuery))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateTextureDescriptor_ETC2_NoFeature(t *testing.T) {
	t.Parallel()
	desc := validTextureDesc()
	desc.Format = gputypes.TextureFormatETC2RGB8Unorm
	err := ValidateTextureDescriptor(desc, gputypes.DefaultLimits(), gputypes.Features(0))
	if !IsFeatureError(err) {
		t.Fatalf("expected feature error, got %T: %v", err, err)
	}
}

func TestValidateTextureDescriptor_ASTC_NoFeature(t *testing.T) {
	t.Parallel()
	desc := validTextureDesc()
	desc.Format = gputypes.TextureFormatASTC4x4Unorm
	err := ValidateTextureDescriptor(desc, gputypes.DefaultLimits(), gputypes.Features(0))
	if !IsFeatureError(err) {
		t.Fatalf("expected feature error, got %T: %v", err, err)
	}
}

func TestValidateTextureDescriptor_RG11B10Renderable_NoFeature(t *testing.T) {
	t.Parallel()
	desc := validTextureDesc()
	desc.Format = gputypes.TextureFormatRG11B10Ufloat
	desc.Usage = gputypes.TextureUsageRenderAttachment
	err := ValidateTextureDescriptor(desc, gputypes.DefaultLimits(), gputypes.Features(0))
	if !IsFeatureError(err) {
		t.Fatalf("expected feature error, got %T: %v", err, err)
	}
}

func TestValidateTextureUsageFlags_DepthStorageBinding(t *testing.T) {
	t.Parallel()
	err := ValidateTextureUsageFlags(
		gputypes.TextureUsageStorageBinding,
		1,
		gputypes.TextureFormatDepth24Plus,
	)
	if err == nil {
		t.Fatal("expected invalid usage for depth storage binding")
	}
}

func TestValidateTextureUsageFlags_MultisampleStorage(t *testing.T) {
	t.Parallel()
	err := ValidateTextureUsageFlags(
		gputypes.TextureUsageStorageBinding,
		4,
		gputypes.TextureFormatRGBA8Unorm,
	)
	if err == nil {
		t.Fatal("expected invalid usage for multisampled storage")
	}
}

func TestValidateTextureUsageFlags_ValidCopySrc(t *testing.T) {
	t.Parallel()
	err := ValidateTextureUsageFlags(
		gputypes.TextureUsageCopySrc,
		1,
		gputypes.TextureFormatRGBA8Unorm,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
