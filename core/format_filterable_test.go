//go:build !(js && wasm)

package core

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

func TestValidateFloat32FilterableBindings_TextureSampleTypeFloat(t *testing.T) {
	t.Parallel()
	layout := []gputypes.BindGroupLayoutEntry{
		{
			Binding: 0,
			Texture: &gputypes.TextureBindingLayout{SampleType: gputypes.TextureSampleTypeFloat},
		},
	}
	textures := []BindGroupTextureInfo{{Binding: 0, Format: gputypes.TextureFormatR32Float}}
	err := validateFloat32FilterableBindings(layout, nil, textures, gputypes.Features(0))
	if !IsFeatureError(err) {
		t.Fatalf("expected feature error, got %T: %v", err, err)
	}
}

func TestValidateFloat32FilterableBindings_FilteringSamplerWithUnfilterableFloat(t *testing.T) {
	t.Parallel()
	layout := []gputypes.BindGroupLayoutEntry{
		{Binding: 0, Texture: &gputypes.TextureBindingLayout{SampleType: gputypes.TextureSampleTypeUnfilterableFloat}},
		{Binding: 1, Sampler: &gputypes.SamplerBindingLayout{}},
	}
	samplers := []BindGroupSamplerInfo{
		{Binding: 1, Desc: &hal.SamplerDescriptor{MagFilter: gputypes.FilterModeLinear}},
	}
	textures := []BindGroupTextureInfo{{Binding: 0, Format: gputypes.TextureFormatR32Float}}
	err := validateFloat32FilterableBindings(layout, samplers, textures, gputypes.Features(0))
	if !IsFeatureError(err) {
		t.Fatalf("expected feature error, got %T: %v", err, err)
	}
}

func TestValidateFloat32FilterableBindings_WithFeature(t *testing.T) {
	t.Parallel()
	layout := []gputypes.BindGroupLayoutEntry{
		{
			Binding: 0,
			Texture: &gputypes.TextureBindingLayout{SampleType: gputypes.TextureSampleTypeFloat},
		},
	}
	textures := []BindGroupTextureInfo{{Binding: 0, Format: gputypes.TextureFormatR32Float}}
	features := gputypes.Features(gputypes.FeatureFloat32Filterable)
	if err := validateFloat32FilterableBindings(layout, nil, textures, features); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateSPIRVShaderFeatures_Float64_NoFeature(t *testing.T) {
	t.Parallel()
	spirv := buildSPIRVWithCapabilities(spirvCapFloat64, spirvCapShader)
	err := validateSPIRVShaderFeatures(spirv, gputypes.Features(0))
	if !IsFeatureError(err) {
		t.Fatalf("expected feature error, got %T: %v", err, err)
	}
}

func TestValidateSPIRVShaderFeatures_Float64_WithFeature(t *testing.T) {
	t.Parallel()
	spirv := buildSPIRVWithCapabilities(spirvCapFloat64, spirvCapShader)
	features := gputypes.Features(gputypes.FeatureShaderFloat64)
	if err := validateSPIRVShaderFeatures(spirv, features); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

const spirvCapShader = 1

func buildSPIRVWithCapabilities(caps ...uint32) []uint32 {
	words := []uint32{
		spirvMagic,
		0x00010400, // SPIR-V 1.4
		0,
		1,
		0,
	}
	for _, capID := range caps {
		words = append(words, (2<<16)|spirvOpCodeCap, capID)
	}
	return words
}
