//go:build !(js && wasm)

package core

import (
	"errors"
	"strings"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// validTextureDesc returns a valid 2D texture descriptor for tests.
func validTextureDesc() *hal.TextureDescriptor {
	return &hal.TextureDescriptor{
		Label:         "test",
		Size:          hal.Extent3D{Width: 256, Height: 256, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         gputypes.TextureUsageTextureBinding,
	}
}

// --- ValidateTextureDescriptor tests ---

func TestValidateTextureDescriptor_Valid(t *testing.T) {
	err := ValidateTextureDescriptor(validTextureDesc(), gputypes.DefaultLimits())
	if err != nil {
		t.Fatalf("expected nil error for valid descriptor, got: %v", err)
	}
}

func TestValidateTextureDescriptor_InvalidDimension(t *testing.T) {
	desc := validTextureDesc()
	desc.Dimension = gputypes.TextureDimensionUndefined

	err := ValidateTextureDescriptor(desc, gputypes.DefaultLimits())
	if err == nil {
		t.Fatal("expected error for undefined dimension")
	}
	var cte *CreateTextureError
	if !errors.As(err, &cte) {
		t.Fatalf("expected CreateTextureError, got %T", err)
	}
	if cte.Kind != CreateTextureErrorInvalidDimension {
		t.Errorf("expected InvalidDimension, got %v", cte.Kind)
	}
}

func TestValidateTextureDescriptor_InvalidFormat(t *testing.T) {
	desc := validTextureDesc()
	desc.Format = gputypes.TextureFormatUndefined

	err := ValidateTextureDescriptor(desc, gputypes.DefaultLimits())
	if err == nil {
		t.Fatal("expected error for undefined format")
	}
	var cte *CreateTextureError
	if !errors.As(err, &cte) {
		t.Fatalf("expected CreateTextureError, got %T", err)
	}
	if cte.Kind != CreateTextureErrorInvalidFormat {
		t.Errorf("expected InvalidFormat, got %v", cte.Kind)
	}
}

func TestValidateTextureDescriptor_ZeroDimension(t *testing.T) {
	tests := []struct {
		name   string
		width  uint32
		height uint32
		depth  uint32
	}{
		{"zero width", 0, 256, 1},
		{"zero height", 256, 0, 1},
		{"zero depth", 256, 256, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desc := validTextureDesc()
			desc.Size = hal.Extent3D{Width: tt.width, Height: tt.height, DepthOrArrayLayers: tt.depth}

			err := ValidateTextureDescriptor(desc, gputypes.DefaultLimits())
			if err == nil {
				t.Fatal("expected error for zero dimension")
			}
			var cte *CreateTextureError
			if !errors.As(err, &cte) {
				t.Fatalf("expected CreateTextureError, got %T", err)
			}
			if cte.Kind != CreateTextureErrorZeroDimension {
				t.Errorf("expected ZeroDimension, got %v", cte.Kind)
			}
			if cte.RequestedWidth != tt.width || cte.RequestedHeight != tt.height || cte.RequestedDepth != tt.depth {
				t.Errorf("expected requested dims %d,%d,%d, got %d,%d,%d",
					tt.width, tt.height, tt.depth,
					cte.RequestedWidth, cte.RequestedHeight, cte.RequestedDepth)
			}
		})
	}
}

func TestValidateTextureDescriptor_MaxDimension1D(t *testing.T) {
	limits := gputypes.DefaultLimits()
	desc := validTextureDesc()
	desc.Dimension = gputypes.TextureDimension1D
	desc.Size = hal.Extent3D{Width: limits.MaxTextureDimension1D + 1, Height: 1, DepthOrArrayLayers: 1}

	err := ValidateTextureDescriptor(desc, limits)
	if err == nil {
		t.Fatal("expected error for exceeding 1D max dimension")
	}
	var cte *CreateTextureError
	if !errors.As(err, &cte) {
		t.Fatalf("expected CreateTextureError, got %T", err)
	}
	if cte.Kind != CreateTextureErrorMaxDimension {
		t.Errorf("expected MaxDimension, got %v", cte.Kind)
	}
	if cte.MaxDimension != limits.MaxTextureDimension1D {
		t.Errorf("expected MaxDimension %d, got %d", limits.MaxTextureDimension1D, cte.MaxDimension)
	}
}

func TestValidateTextureDescriptor_MaxDimension2D(t *testing.T) {
	limits := gputypes.DefaultLimits()
	desc := validTextureDesc()
	desc.Size = hal.Extent3D{Width: limits.MaxTextureDimension2D + 1, Height: 1, DepthOrArrayLayers: 1}

	err := ValidateTextureDescriptor(desc, limits)
	if err == nil {
		t.Fatal("expected error for exceeding 2D max dimension")
	}
	var cte *CreateTextureError
	if !errors.As(err, &cte) {
		t.Fatalf("expected CreateTextureError, got %T", err)
	}
	if cte.Kind != CreateTextureErrorMaxDimension {
		t.Errorf("expected MaxDimension, got %v", cte.Kind)
	}
}

func TestValidateTextureDescriptor_MaxDimension3D(t *testing.T) {
	limits := gputypes.DefaultLimits()
	desc := validTextureDesc()
	desc.Dimension = gputypes.TextureDimension3D
	desc.Size = hal.Extent3D{Width: limits.MaxTextureDimension3D + 1, Height: 1, DepthOrArrayLayers: 1}

	err := ValidateTextureDescriptor(desc, limits)
	if err == nil {
		t.Fatal("expected error for exceeding 3D max dimension")
	}
	var cte *CreateTextureError
	if !errors.As(err, &cte) {
		t.Fatalf("expected CreateTextureError, got %T", err)
	}
	if cte.Kind != CreateTextureErrorMaxDimension {
		t.Errorf("expected MaxDimension, got %v", cte.Kind)
	}
}

func TestValidateTextureDescriptor_MaxArrayLayers(t *testing.T) {
	limits := gputypes.DefaultLimits()
	desc := validTextureDesc()
	desc.Size = hal.Extent3D{Width: 64, Height: 64, DepthOrArrayLayers: limits.MaxTextureArrayLayers + 1}

	err := ValidateTextureDescriptor(desc, limits)
	if err == nil {
		t.Fatal("expected error for exceeding max array layers")
	}
	var cte *CreateTextureError
	if !errors.As(err, &cte) {
		t.Fatalf("expected CreateTextureError, got %T", err)
	}
	if cte.Kind != CreateTextureErrorMaxArrayLayers {
		t.Errorf("expected MaxArrayLayers, got %v", cte.Kind)
	}
}

func TestValidateTextureDescriptor_EmptyUsage(t *testing.T) {
	desc := validTextureDesc()
	desc.Usage = gputypes.TextureUsageNone

	err := ValidateTextureDescriptor(desc, gputypes.DefaultLimits())
	if err == nil {
		t.Fatal("expected error for empty usage")
	}
	var cte *CreateTextureError
	if !errors.As(err, &cte) {
		t.Fatalf("expected CreateTextureError, got %T", err)
	}
	if cte.Kind != CreateTextureErrorEmptyUsage {
		t.Errorf("expected EmptyUsage, got %v", cte.Kind)
	}
}

func TestValidateTextureDescriptor_InvalidUsage(t *testing.T) {
	desc := validTextureDesc()
	desc.Usage = gputypes.TextureUsage(1 << 30) // Unknown flag

	err := ValidateTextureDescriptor(desc, gputypes.DefaultLimits())
	if err == nil {
		t.Fatal("expected error for invalid usage")
	}
	var cte *CreateTextureError
	if !errors.As(err, &cte) {
		t.Fatalf("expected CreateTextureError, got %T", err)
	}
	if cte.Kind != CreateTextureErrorInvalidUsage {
		t.Errorf("expected InvalidUsage, got %v", cte.Kind)
	}
}

func TestValidateTextureDescriptor_InvalidMipLevelCount_Zero(t *testing.T) {
	desc := validTextureDesc()
	desc.MipLevelCount = 0

	err := ValidateTextureDescriptor(desc, gputypes.DefaultLimits())
	if err == nil {
		t.Fatal("expected error for zero mip level count")
	}
	var cte *CreateTextureError
	if !errors.As(err, &cte) {
		t.Fatalf("expected CreateTextureError, got %T", err)
	}
	if cte.Kind != CreateTextureErrorInvalidMipLevelCount {
		t.Errorf("expected InvalidMipLevelCount, got %v", cte.Kind)
	}
	if cte.RequestedMips != 0 {
		t.Errorf("expected RequestedMips 0, got %d", cte.RequestedMips)
	}
}

func TestValidateTextureDescriptor_InvalidMipLevelCount_TooMany(t *testing.T) {
	desc := validTextureDesc()
	desc.Size = hal.Extent3D{Width: 256, Height: 256, DepthOrArrayLayers: 1}
	desc.MipLevelCount = 100 // max for 256x256 is 9

	err := ValidateTextureDescriptor(desc, gputypes.DefaultLimits())
	if err == nil {
		t.Fatal("expected error for too many mip levels")
	}
	var cte *CreateTextureError
	if !errors.As(err, &cte) {
		t.Fatalf("expected CreateTextureError, got %T", err)
	}
	if cte.Kind != CreateTextureErrorInvalidMipLevelCount {
		t.Errorf("expected InvalidMipLevelCount, got %v", cte.Kind)
	}
	if cte.RequestedMips != 100 {
		t.Errorf("expected RequestedMips 100, got %d", cte.RequestedMips)
	}
	if cte.MaxMips != 9 {
		t.Errorf("expected MaxMips 9, got %d", cte.MaxMips)
	}
}

func TestValidateTextureDescriptor_InvalidSampleCount(t *testing.T) {
	for _, sc := range []uint32{0, 2, 3, 5, 8, 16} {
		desc := validTextureDesc()
		desc.SampleCount = sc

		err := ValidateTextureDescriptor(desc, gputypes.DefaultLimits())
		if err == nil {
			t.Fatalf("expected error for sample count %d", sc)
		}
		var cte *CreateTextureError
		if !errors.As(err, &cte) {
			t.Fatalf("expected CreateTextureError for sample count %d, got %T", sc, err)
		}
		if cte.Kind != CreateTextureErrorInvalidSampleCount {
			t.Errorf("expected InvalidSampleCount for %d, got %v", sc, cte.Kind)
		}
		if cte.RequestedSamples != sc {
			t.Errorf("expected RequestedSamples %d, got %d", sc, cte.RequestedSamples)
		}
	}
}

func TestValidateTextureDescriptor_MultisampleMipLevel(t *testing.T) {
	desc := validTextureDesc()
	desc.SampleCount = 4
	desc.MipLevelCount = 2

	err := ValidateTextureDescriptor(desc, gputypes.DefaultLimits())
	if err == nil {
		t.Fatal("expected error for multisampled texture with mip levels > 1")
	}
	var cte *CreateTextureError
	if !errors.As(err, &cte) {
		t.Fatalf("expected CreateTextureError, got %T", err)
	}
	if cte.Kind != CreateTextureErrorMultisampleMipLevel {
		t.Errorf("expected MultisampleMipLevel, got %v", cte.Kind)
	}
}

func TestValidateTextureDescriptor_MultisampleDimension(t *testing.T) {
	desc := validTextureDesc()
	desc.Dimension = gputypes.TextureDimension3D
	desc.SampleCount = 4
	desc.MipLevelCount = 1
	desc.Size = hal.Extent3D{Width: 64, Height: 64, DepthOrArrayLayers: 1}

	err := ValidateTextureDescriptor(desc, gputypes.DefaultLimits())
	if err == nil {
		t.Fatal("expected error for multisampled non-2D texture")
	}
	var cte *CreateTextureError
	if !errors.As(err, &cte) {
		t.Fatalf("expected CreateTextureError, got %T", err)
	}
	if cte.Kind != CreateTextureErrorMultisampleDimension {
		t.Errorf("expected MultisampleDimension, got %v", cte.Kind)
	}
}

func TestValidateTextureDescriptor_MultisampleArrayLayers(t *testing.T) {
	desc := validTextureDesc()
	desc.SampleCount = 4
	desc.MipLevelCount = 1
	desc.Size = hal.Extent3D{Width: 64, Height: 64, DepthOrArrayLayers: 2}

	err := ValidateTextureDescriptor(desc, gputypes.DefaultLimits())
	if err == nil {
		t.Fatal("expected error for multisampled texture with array layers > 1")
	}
	var cte *CreateTextureError
	if !errors.As(err, &cte) {
		t.Fatalf("expected CreateTextureError, got %T", err)
	}
	if cte.Kind != CreateTextureErrorMultisampleArrayLayers {
		t.Errorf("expected MultisampleArrayLayers, got %v", cte.Kind)
	}
}

func TestValidateTextureDescriptor_MultisampleStorageBinding(t *testing.T) {
	desc := validTextureDesc()
	desc.SampleCount = 4
	desc.MipLevelCount = 1
	desc.Size = hal.Extent3D{Width: 64, Height: 64, DepthOrArrayLayers: 1}
	desc.Usage = gputypes.TextureUsageTextureBinding | gputypes.TextureUsageStorageBinding

	err := ValidateTextureDescriptor(desc, gputypes.DefaultLimits())
	if err == nil {
		t.Fatal("expected error for multisampled texture with storage binding")
	}
	var cte *CreateTextureError
	if !errors.As(err, &cte) {
		t.Fatalf("expected CreateTextureError, got %T", err)
	}
	if cte.Kind != CreateTextureErrorMultisampleStorageBinding {
		t.Errorf("expected MultisampleStorageBinding, got %v", cte.Kind)
	}
}

func TestValidateTextureDescriptor_ValidMultisample(t *testing.T) {
	desc := validTextureDesc()
	desc.SampleCount = 4
	desc.MipLevelCount = 1
	desc.Size = hal.Extent3D{Width: 64, Height: 64, DepthOrArrayLayers: 1}
	desc.Usage = gputypes.TextureUsageRenderAttachment

	err := ValidateTextureDescriptor(desc, gputypes.DefaultLimits())
	if err != nil {
		t.Fatalf("expected nil error for valid multisampled texture, got: %v", err)
	}
}

func TestValidateTextureDescriptor_ValidMaxMips(t *testing.T) {
	desc := validTextureDesc()
	desc.Size = hal.Extent3D{Width: 256, Height: 256, DepthOrArrayLayers: 1}
	desc.MipLevelCount = 9 // log2(256) + 1 = 9

	err := ValidateTextureDescriptor(desc, gputypes.DefaultLimits())
	if err != nil {
		t.Fatalf("expected nil error for max valid mip count, got: %v", err)
	}
}

// --- ValidateSamplerDescriptor tests ---

func TestValidateSamplerDescriptor_Valid(t *testing.T) {
	desc := &hal.SamplerDescriptor{
		Label:        "test",
		LodMinClamp:  0,
		LodMaxClamp:  32,
		MagFilter:    gputypes.FilterModeLinear,
		MinFilter:    gputypes.FilterModeLinear,
		MipmapFilter: gputypes.FilterModeLinear,
		Anisotropy:   1,
	}
	err := ValidateSamplerDescriptor(desc)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
}

func TestValidateSamplerDescriptor_NegativeLodMinClamp(t *testing.T) {
	desc := &hal.SamplerDescriptor{
		Label:       "test",
		LodMinClamp: -1.0,
		LodMaxClamp: 32,
	}
	err := ValidateSamplerDescriptor(desc)
	if err == nil {
		t.Fatal("expected error for negative LodMinClamp")
	}
	var cse *CreateSamplerError
	if !errors.As(err, &cse) {
		t.Fatalf("expected CreateSamplerError, got %T", err)
	}
	if cse.Kind != CreateSamplerErrorInvalidLodMinClamp {
		t.Errorf("expected InvalidLodMinClamp, got %v", cse.Kind)
	}
}

func TestValidateSamplerDescriptor_LodMaxClampLessThanMin(t *testing.T) {
	desc := &hal.SamplerDescriptor{
		Label:       "test",
		LodMinClamp: 10.0,
		LodMaxClamp: 5.0,
	}
	err := ValidateSamplerDescriptor(desc)
	if err == nil {
		t.Fatal("expected error for LodMaxClamp < LodMinClamp")
	}
	var cse *CreateSamplerError
	if !errors.As(err, &cse) {
		t.Fatalf("expected CreateSamplerError, got %T", err)
	}
	if cse.Kind != CreateSamplerErrorInvalidLodMaxClamp {
		t.Errorf("expected InvalidLodMaxClamp, got %v", cse.Kind)
	}
	if cse.LodMinClamp != 10.0 || cse.LodMaxClamp != 5.0 {
		t.Errorf("expected LodMinClamp=10, LodMaxClamp=5, got %f, %f", cse.LodMinClamp, cse.LodMaxClamp)
	}
}

func TestValidateSamplerDescriptor_AnisotropyRequiresLinear(t *testing.T) {
	desc := &hal.SamplerDescriptor{
		Label:        "test",
		LodMinClamp:  0,
		LodMaxClamp:  32,
		MagFilter:    gputypes.FilterModeNearest,
		MinFilter:    gputypes.FilterModeLinear,
		MipmapFilter: gputypes.FilterModeLinear,
		Anisotropy:   4,
	}
	err := ValidateSamplerDescriptor(desc)
	if err == nil {
		t.Fatal("expected error for anisotropy with non-linear filtering")
	}
	var cse *CreateSamplerError
	if !errors.As(err, &cse) {
		t.Fatalf("expected CreateSamplerError, got %T", err)
	}
	if cse.Kind != CreateSamplerErrorAnisotropyRequiresLinearFiltering {
		t.Errorf("expected AnisotropyRequiresLinearFiltering, got %v", cse.Kind)
	}
}

func TestValidateSamplerDescriptor_ZeroAnisotropyIsValid(t *testing.T) {
	desc := &hal.SamplerDescriptor{
		Label:       "test",
		LodMinClamp: 0,
		LodMaxClamp: 32,
		MagFilter:   gputypes.FilterModeNearest,
		MinFilter:   gputypes.FilterModeNearest,
		Anisotropy:  0, // treated as 1
	}
	err := ValidateSamplerDescriptor(desc)
	if err != nil {
		t.Fatalf("expected nil error for zero anisotropy, got: %v", err)
	}
}

// --- ValidateShaderModuleDescriptor tests ---

func TestValidateShaderModuleDescriptor_ValidWGSL(t *testing.T) {
	desc := &hal.ShaderModuleDescriptor{
		Label:  "test",
		Source: hal.ShaderSource{WGSL: "@vertex fn main() -> @builtin(position) vec4f { return vec4f(); }"},
	}
	err := ValidateShaderModuleDescriptor(desc)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
}

func TestValidateShaderModuleDescriptor_ValidSPIRV(t *testing.T) {
	desc := &hal.ShaderModuleDescriptor{
		Label:  "test",
		Source: hal.ShaderSource{SPIRV: []uint32{0x07230203, 0x00010000}},
	}
	err := ValidateShaderModuleDescriptor(desc)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
}

func TestValidateShaderModuleDescriptor_NoSource(t *testing.T) {
	desc := &hal.ShaderModuleDescriptor{Label: "test"}
	err := ValidateShaderModuleDescriptor(desc)
	if err == nil {
		t.Fatal("expected error for no source")
	}
	var csme *CreateShaderModuleError
	if !errors.As(err, &csme) {
		t.Fatalf("expected CreateShaderModuleError, got %T", err)
	}
	if csme.Kind != CreateShaderModuleErrorNoSource {
		t.Errorf("expected NoSource, got %v", csme.Kind)
	}
}

func TestValidateShaderModuleDescriptor_DualSource(t *testing.T) {
	desc := &hal.ShaderModuleDescriptor{
		Label: "test",
		Source: hal.ShaderSource{
			WGSL:  "@vertex fn main() {}",
			SPIRV: []uint32{0x07230203},
		},
	}
	err := ValidateShaderModuleDescriptor(desc)
	if err == nil {
		t.Fatal("expected error for dual source")
	}
	var csme *CreateShaderModuleError
	if !errors.As(err, &csme) {
		t.Fatalf("expected CreateShaderModuleError, got %T", err)
	}
	if csme.Kind != CreateShaderModuleErrorDualSource {
		t.Errorf("expected DualSource, got %v", csme.Kind)
	}
}

// --- ValidateRenderPipelineDescriptor tests ---

func TestValidateRenderPipelineDescriptor_Valid(t *testing.T) {
	desc := &hal.RenderPipelineDescriptor{
		Label: "test",
		Vertex: hal.VertexState{
			Module:     mockShaderModule{},
			EntryPoint: "vs_main",
		},
		Fragment: &hal.FragmentState{
			Module:     mockShaderModule{},
			EntryPoint: "fs_main",
			Targets:    []gputypes.ColorTargetState{{}},
		},
		Multisample: gputypes.MultisampleState{Count: 1},
	}
	err := ValidateRenderPipelineDescriptor(desc, gputypes.DefaultLimits())
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
}

func TestValidateRenderPipelineDescriptor_MissingVertexModule(t *testing.T) {
	desc := &hal.RenderPipelineDescriptor{
		Label: "test",
		Vertex: hal.VertexState{
			Module:     nil,
			EntryPoint: "vs_main",
		},
	}
	err := ValidateRenderPipelineDescriptor(desc, gputypes.DefaultLimits())
	if err == nil {
		t.Fatal("expected error for nil vertex module")
	}
	var crpe *CreateRenderPipelineError
	if !errors.As(err, &crpe) {
		t.Fatalf("expected CreateRenderPipelineError, got %T", err)
	}
	if crpe.Kind != CreateRenderPipelineErrorMissingVertexModule {
		t.Errorf("expected MissingVertexModule, got %v", crpe.Kind)
	}
}

func TestValidateRenderPipelineDescriptor_MissingVertexEntryPoint(t *testing.T) {
	desc := &hal.RenderPipelineDescriptor{
		Label: "test",
		Vertex: hal.VertexState{
			Module:     mockShaderModule{},
			EntryPoint: "",
		},
	}
	err := ValidateRenderPipelineDescriptor(desc, gputypes.DefaultLimits())
	if err == nil {
		t.Fatal("expected error for empty vertex entry point")
	}
	var crpe *CreateRenderPipelineError
	if !errors.As(err, &crpe) {
		t.Fatalf("expected CreateRenderPipelineError, got %T", err)
	}
	if crpe.Kind != CreateRenderPipelineErrorMissingVertexEntryPoint {
		t.Errorf("expected MissingVertexEntryPoint, got %v", crpe.Kind)
	}
}

func TestValidateRenderPipelineDescriptor_MissingFragmentModule(t *testing.T) {
	desc := &hal.RenderPipelineDescriptor{
		Label: "test",
		Vertex: hal.VertexState{
			Module:     mockShaderModule{},
			EntryPoint: "vs_main",
		},
		Fragment: &hal.FragmentState{
			Module:     nil,
			EntryPoint: "fs_main",
			Targets:    []gputypes.ColorTargetState{{}},
		},
	}
	err := ValidateRenderPipelineDescriptor(desc, gputypes.DefaultLimits())
	if err == nil {
		t.Fatal("expected error for nil fragment module")
	}
	var crpe *CreateRenderPipelineError
	if !errors.As(err, &crpe) {
		t.Fatalf("expected CreateRenderPipelineError, got %T", err)
	}
	if crpe.Kind != CreateRenderPipelineErrorMissingFragmentModule {
		t.Errorf("expected MissingFragmentModule, got %v", crpe.Kind)
	}
}

func TestValidateRenderPipelineDescriptor_MissingFragmentEntryPoint(t *testing.T) {
	desc := &hal.RenderPipelineDescriptor{
		Label: "test",
		Vertex: hal.VertexState{
			Module:     mockShaderModule{},
			EntryPoint: "vs_main",
		},
		Fragment: &hal.FragmentState{
			Module:     mockShaderModule{},
			EntryPoint: "",
			Targets:    []gputypes.ColorTargetState{{}},
		},
	}
	err := ValidateRenderPipelineDescriptor(desc, gputypes.DefaultLimits())
	if err == nil {
		t.Fatal("expected error for empty fragment entry point")
	}
	var crpe *CreateRenderPipelineError
	if !errors.As(err, &crpe) {
		t.Fatalf("expected CreateRenderPipelineError, got %T", err)
	}
	if crpe.Kind != CreateRenderPipelineErrorMissingFragmentEntryPoint {
		t.Errorf("expected MissingFragmentEntryPoint, got %v", crpe.Kind)
	}
}

func TestValidateRenderPipelineDescriptor_NoFragmentTargets(t *testing.T) {
	desc := &hal.RenderPipelineDescriptor{
		Label: "test",
		Vertex: hal.VertexState{
			Module:     mockShaderModule{},
			EntryPoint: "vs_main",
		},
		Fragment: &hal.FragmentState{
			Module:     mockShaderModule{},
			EntryPoint: "fs_main",
			Targets:    []gputypes.ColorTargetState{},
		},
	}
	err := ValidateRenderPipelineDescriptor(desc, gputypes.DefaultLimits())
	if err == nil {
		t.Fatal("expected error for empty fragment targets")
	}
	var crpe *CreateRenderPipelineError
	if !errors.As(err, &crpe) {
		t.Fatalf("expected CreateRenderPipelineError, got %T", err)
	}
	if crpe.Kind != CreateRenderPipelineErrorNoFragmentTargets {
		t.Errorf("expected NoFragmentTargets, got %v", crpe.Kind)
	}
}

func TestValidateRenderPipelineDescriptor_TooManyColorTargets(t *testing.T) {
	limits := gputypes.DefaultLimits()
	targets := make([]gputypes.ColorTargetState, limits.MaxColorAttachments+1)
	desc := &hal.RenderPipelineDescriptor{
		Label: "test",
		Vertex: hal.VertexState{
			Module:     mockShaderModule{},
			EntryPoint: "vs_main",
		},
		Fragment: &hal.FragmentState{
			Module:     mockShaderModule{},
			EntryPoint: "fs_main",
			Targets:    targets,
		},
	}
	err := ValidateRenderPipelineDescriptor(desc, limits)
	if err == nil {
		t.Fatal("expected error for too many color targets")
	}
	var crpe *CreateRenderPipelineError
	if !errors.As(err, &crpe) {
		t.Fatalf("expected CreateRenderPipelineError, got %T", err)
	}
	if crpe.Kind != CreateRenderPipelineErrorTooManyColorTargets {
		t.Errorf("expected TooManyColorTargets, got %v", crpe.Kind)
	}
	if crpe.TargetCount != limits.MaxColorAttachments+1 {
		t.Errorf("expected TargetCount %d, got %d", limits.MaxColorAttachments+1, crpe.TargetCount)
	}
}

func TestValidateRenderPipelineDescriptor_InvalidSampleCount(t *testing.T) {
	desc := &hal.RenderPipelineDescriptor{
		Label: "test",
		Vertex: hal.VertexState{
			Module:     mockShaderModule{},
			EntryPoint: "vs_main",
		},
		Multisample: gputypes.MultisampleState{Count: 3},
	}
	err := ValidateRenderPipelineDescriptor(desc, gputypes.DefaultLimits())
	if err == nil {
		t.Fatal("expected error for invalid sample count")
	}
	var crpe *CreateRenderPipelineError
	if !errors.As(err, &crpe) {
		t.Fatalf("expected CreateRenderPipelineError, got %T", err)
	}
	if crpe.Kind != CreateRenderPipelineErrorInvalidSampleCount {
		t.Errorf("expected InvalidSampleCount, got %v", crpe.Kind)
	}
}

func TestValidateRenderPipelineDescriptor_NoFragment(t *testing.T) {
	desc := &hal.RenderPipelineDescriptor{
		Label: "test",
		Vertex: hal.VertexState{
			Module:     mockShaderModule{},
			EntryPoint: "vs_main",
		},
		Multisample: gputypes.MultisampleState{Count: 1},
	}
	err := ValidateRenderPipelineDescriptor(desc, gputypes.DefaultLimits())
	if err != nil {
		t.Fatalf("expected nil error for depth-only pipeline, got: %v", err)
	}
}

// --- ValidateComputePipelineDescriptor tests ---

func TestValidateComputePipelineDescriptor_Valid(t *testing.T) {
	desc := &hal.ComputePipelineDescriptor{
		Label: "test",
		Compute: hal.ComputeState{
			Module:     mockShaderModule{},
			EntryPoint: "main",
		},
	}
	err := ValidateComputePipelineDescriptor(desc)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
}

func TestValidateComputePipelineDescriptor_MissingModule(t *testing.T) {
	desc := &hal.ComputePipelineDescriptor{
		Label: "test",
		Compute: hal.ComputeState{
			Module:     nil,
			EntryPoint: "main",
		},
	}
	err := ValidateComputePipelineDescriptor(desc)
	if err == nil {
		t.Fatal("expected error for nil module")
	}
	var ccpe *CreateComputePipelineError
	if !errors.As(err, &ccpe) {
		t.Fatalf("expected CreateComputePipelineError, got %T", err)
	}
	if ccpe.Kind != CreateComputePipelineErrorMissingModule {
		t.Errorf("expected MissingModule, got %v", ccpe.Kind)
	}
}

func TestValidateComputePipelineDescriptor_MissingEntryPoint(t *testing.T) {
	desc := &hal.ComputePipelineDescriptor{
		Label: "test",
		Compute: hal.ComputeState{
			Module:     mockShaderModule{},
			EntryPoint: "",
		},
	}
	err := ValidateComputePipelineDescriptor(desc)
	if err == nil {
		t.Fatal("expected error for empty entry point")
	}
	var ccpe *CreateComputePipelineError
	if !errors.As(err, &ccpe) {
		t.Fatalf("expected CreateComputePipelineError, got %T", err)
	}
	if ccpe.Kind != CreateComputePipelineErrorMissingEntryPoint {
		t.Errorf("expected MissingEntryPoint, got %v", ccpe.Kind)
	}
}

// --- ValidateBindGroupLayoutDescriptor tests ---

func TestValidateBindGroupLayoutDescriptor_Valid(t *testing.T) {
	desc := &hal.BindGroupLayoutDescriptor{
		Label: "test",
		Entries: []gputypes.BindGroupLayoutEntry{
			{Binding: 0},
			{Binding: 1},
		},
	}
	err := ValidateBindGroupLayoutDescriptor(desc, gputypes.DefaultLimits())
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
}

func TestValidateBindGroupLayoutDescriptor_DuplicateBinding(t *testing.T) {
	desc := &hal.BindGroupLayoutDescriptor{
		Label: "test",
		Entries: []gputypes.BindGroupLayoutEntry{
			{Binding: 0},
			{Binding: 1},
			{Binding: 0}, // duplicate
		},
	}
	err := ValidateBindGroupLayoutDescriptor(desc, gputypes.DefaultLimits())
	if err == nil {
		t.Fatal("expected error for duplicate binding")
	}
	var cble *CreateBindGroupLayoutError
	if !errors.As(err, &cble) {
		t.Fatalf("expected CreateBindGroupLayoutError, got %T", err)
	}
	if cble.Kind != CreateBindGroupLayoutErrorDuplicateBinding {
		t.Errorf("expected DuplicateBinding, got %v", cble.Kind)
	}
	if cble.DuplicateBinding != 0 {
		t.Errorf("expected DuplicateBinding 0, got %d", cble.DuplicateBinding)
	}
}

func TestValidateBindGroupLayoutDescriptor_TooManyBindings(t *testing.T) {
	limits := gputypes.DefaultLimits()
	entries := make([]gputypes.BindGroupLayoutEntry, limits.MaxBindingsPerBindGroup+1)
	for i := range entries {
		entries[i].Binding = uint32(i)
	}
	desc := &hal.BindGroupLayoutDescriptor{
		Label:   "test",
		Entries: entries,
	}
	err := ValidateBindGroupLayoutDescriptor(desc, limits)
	if err == nil {
		t.Fatal("expected error for too many bindings")
	}
	var cble *CreateBindGroupLayoutError
	if !errors.As(err, &cble) {
		t.Fatalf("expected CreateBindGroupLayoutError, got %T", err)
	}
	if cble.Kind != CreateBindGroupLayoutErrorTooManyBindings {
		t.Errorf("expected TooManyBindings, got %v", cble.Kind)
	}
}

// --- ValidateBindGroupDescriptor tests ---

func TestValidateBindGroupDescriptor_Valid(t *testing.T) {
	layoutEntries := []gputypes.BindGroupLayoutEntry{
		{Binding: 0},
		{Binding: 1},
		{Binding: 2},
	}
	desc := &hal.BindGroupDescriptor{
		Label:  "test",
		Layout: mockBindGroupLayout{},
		Entries: []gputypes.BindGroupEntry{
			{Binding: 0},
			{Binding: 1},
			{Binding: 2},
		},
	}
	err := ValidateBindGroupDescriptor(desc, layoutEntries, nil, gputypes.Limits{})
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
}

func TestValidateBindGroupDescriptor_MissingLayout(t *testing.T) {
	desc := &hal.BindGroupDescriptor{
		Label:  "test",
		Layout: nil,
	}
	// layoutEntries value does not matter -- nil layout is checked first.
	err := ValidateBindGroupDescriptor(desc, nil, nil, gputypes.Limits{})
	if err == nil {
		t.Fatal("expected error for nil layout")
	}
	var cbge *CreateBindGroupError
	if !errors.As(err, &cbge) {
		t.Fatalf("expected CreateBindGroupError, got %T", err)
	}
	if cbge.Kind != CreateBindGroupErrorMissingLayout {
		t.Errorf("expected MissingLayout, got %v", cbge.Kind)
	}
}

func TestValidateBindGroupDescriptor_BindingsNumMismatch(t *testing.T) {
	layoutEntries := []gputypes.BindGroupLayoutEntry{
		{Binding: 0},
		{Binding: 1},
		{Binding: 2},
		{Binding: 3},
	}
	desc := &hal.BindGroupDescriptor{
		Label:  "test",
		Layout: mockBindGroupLayout{},
		Entries: []gputypes.BindGroupEntry{
			{Binding: 0},
			{Binding: 2},
			{Binding: 3},
		},
	}
	err := ValidateBindGroupDescriptor(desc, layoutEntries, nil, gputypes.Limits{})
	if err == nil {
		t.Fatal("expected error for entry count mismatch (3 entries vs 4 layout entries)")
	}
	var cbge *CreateBindGroupError
	if !errors.As(err, &cbge) {
		t.Fatalf("expected CreateBindGroupError, got %T", err)
	}
	if cbge.Kind != CreateBindGroupErrorBindingsNumMismatch {
		t.Errorf("expected BindingsNumMismatch, got %v", cbge.Kind)
	}
	if cbge.Expected != 4 {
		t.Errorf("expected Expected=4, got %d", cbge.Expected)
	}
	if cbge.Actual != 3 {
		t.Errorf("expected Actual=3, got %d", cbge.Actual)
	}
}

func TestValidateBindGroupDescriptor_MissingBindingDeclaration(t *testing.T) {
	layoutEntries := []gputypes.BindGroupLayoutEntry{
		{Binding: 0},
		{Binding: 1},
	}
	desc := &hal.BindGroupDescriptor{
		Label:  "test",
		Layout: mockBindGroupLayout{},
		Entries: []gputypes.BindGroupEntry{
			{Binding: 0},
			{Binding: 5}, // not declared in layout
		},
	}
	err := ValidateBindGroupDescriptor(desc, layoutEntries, nil, gputypes.Limits{})
	if err == nil {
		t.Fatal("expected error for binding 5 not declared in layout")
	}
	var cbge *CreateBindGroupError
	if !errors.As(err, &cbge) {
		t.Fatalf("expected CreateBindGroupError, got %T", err)
	}
	if cbge.Kind != CreateBindGroupErrorMissingBindingDeclaration {
		t.Errorf("expected MissingBindingDeclaration, got %v", cbge.Kind)
	}
	if cbge.Binding != 5 {
		t.Errorf("expected Binding=5, got %d", cbge.Binding)
	}
}

func TestValidateBindGroupDescriptor_DuplicateBinding(t *testing.T) {
	layoutEntries := []gputypes.BindGroupLayoutEntry{
		{Binding: 0},
		{Binding: 1},
	}
	desc := &hal.BindGroupDescriptor{
		Label:  "test",
		Layout: mockBindGroupLayout{},
		Entries: []gputypes.BindGroupEntry{
			{Binding: 0},
			{Binding: 0}, // duplicate
		},
	}
	err := ValidateBindGroupDescriptor(desc, layoutEntries, nil, gputypes.Limits{})
	if err == nil {
		t.Fatal("expected error for duplicate binding 0")
	}
	var cbge *CreateBindGroupError
	if !errors.As(err, &cbge) {
		t.Fatalf("expected CreateBindGroupError, got %T", err)
	}
	if cbge.Kind != CreateBindGroupErrorDuplicateBinding {
		t.Errorf("expected DuplicateBinding, got %v", cbge.Kind)
	}
	if cbge.Binding != 0 {
		t.Errorf("expected Binding=0, got %d", cbge.Binding)
	}
}

// --- ValidatePipelineLayoutDescriptor tests ---

func TestValidatePipelineLayoutDescriptor_Valid(t *testing.T) {
	limits := gputypes.DefaultLimits()
	layouts := make([]hal.BindGroupLayout, limits.MaxBindGroups)
	for i := range layouts {
		layouts[i] = mockBindGroupLayout{}
	}
	desc := &hal.PipelineLayoutDescriptor{
		Label:            "test",
		BindGroupLayouts: layouts,
	}
	err := ValidatePipelineLayoutDescriptor(desc, limits)
	if err != nil {
		t.Fatalf("expected nil error for valid descriptor with %d bind group layouts, got: %v",
			limits.MaxBindGroups, err)
	}
}

func TestValidatePipelineLayoutDescriptor_Empty(t *testing.T) {
	desc := &hal.PipelineLayoutDescriptor{
		Label:            "test",
		BindGroupLayouts: nil,
	}
	err := ValidatePipelineLayoutDescriptor(desc, gputypes.DefaultLimits())
	if err != nil {
		t.Fatalf("expected nil error for empty bind group layouts, got: %v", err)
	}
}

func TestValidatePipelineLayoutDescriptor_TooManyGroups(t *testing.T) {
	limits := gputypes.DefaultLimits()
	layouts := make([]hal.BindGroupLayout, limits.MaxBindGroups+1)
	for i := range layouts {
		layouts[i] = mockBindGroupLayout{}
	}
	desc := &hal.PipelineLayoutDescriptor{
		Label:            "test",
		BindGroupLayouts: layouts,
	}
	err := ValidatePipelineLayoutDescriptor(desc, limits)
	if err == nil {
		t.Fatal("expected error for too many bind group layouts")
	}
	var cple *CreatePipelineLayoutError
	if !errors.As(err, &cple) {
		t.Fatalf("expected CreatePipelineLayoutError, got %T", err)
	}
	if cple.Kind != CreatePipelineLayoutErrorTooManyGroups {
		t.Errorf("expected TooManyGroups, got %v", cple.Kind)
	}
	if cple.Count != int(limits.MaxBindGroups)+1 {
		t.Errorf("expected Count %d, got %d", limits.MaxBindGroups+1, cple.Count)
	}
	if cple.MaxGroups != limits.MaxBindGroups {
		t.Errorf("expected MaxGroups %d, got %d", limits.MaxBindGroups, cple.MaxGroups)
	}
}

// --- ValidateRenderPipelineDescriptor format type guard tests ---

func TestValidateRenderPipelineDescriptor_ColorTargetDepthFormat(t *testing.T) {
	depthFormats := []gputypes.TextureFormat{
		gputypes.TextureFormatDepth16Unorm,
		gputypes.TextureFormatDepth24Plus,
		gputypes.TextureFormatDepth24PlusStencil8,
		gputypes.TextureFormatDepth32Float,
		gputypes.TextureFormatDepth32FloatStencil8,
		gputypes.TextureFormatStencil8,
	}

	for _, f := range depthFormats {
		t.Run(f.String(), func(t *testing.T) {
			desc := &hal.RenderPipelineDescriptor{
				Label: "test",
				Vertex: hal.VertexState{
					Module:     mockShaderModule{},
					EntryPoint: "vs_main",
				},
				Fragment: &hal.FragmentState{
					Module:     mockShaderModule{},
					EntryPoint: "fs_main",
					Targets:    []gputypes.ColorTargetState{{Format: f}},
				},
				Multisample: gputypes.MultisampleState{Count: 1},
			}
			err := ValidateRenderPipelineDescriptor(desc, gputypes.DefaultLimits())
			if err == nil {
				t.Fatalf("expected error for depth/stencil format %s as color target", f)
			}
			var crpe *CreateRenderPipelineError
			if !errors.As(err, &crpe) {
				t.Fatalf("expected CreateRenderPipelineError, got %T", err)
			}
			if crpe.Kind != CreateRenderPipelineErrorColorTargetDepthFormat {
				t.Errorf("expected ColorTargetDepthFormat, got %v", crpe.Kind)
			}
			if crpe.TargetIndex != 0 {
				t.Errorf("expected TargetIndex 0, got %d", crpe.TargetIndex)
			}
			if !strings.Contains(crpe.Error(), "color aspect") {
				t.Errorf("expected error to mention 'color aspect', got %q", crpe.Error())
			}
		})
	}
}

func TestValidateRenderPipelineDescriptor_ColorTargetDepthFormat_SecondTarget(t *testing.T) {
	desc := &hal.RenderPipelineDescriptor{
		Label: "test",
		Vertex: hal.VertexState{
			Module:     mockShaderModule{},
			EntryPoint: "vs_main",
		},
		Fragment: &hal.FragmentState{
			Module:     mockShaderModule{},
			EntryPoint: "fs_main",
			Targets: []gputypes.ColorTargetState{
				{Format: gputypes.TextureFormatRGBA8Unorm},   // valid color format
				{Format: gputypes.TextureFormatDepth32Float}, // invalid: depth format
			},
		},
		Multisample: gputypes.MultisampleState{Count: 1},
	}
	err := ValidateRenderPipelineDescriptor(desc, gputypes.DefaultLimits())
	if err == nil {
		t.Fatal("expected error for depth format as second color target")
	}
	var crpe *CreateRenderPipelineError
	if !errors.As(err, &crpe) {
		t.Fatalf("expected CreateRenderPipelineError, got %T", err)
	}
	if crpe.Kind != CreateRenderPipelineErrorColorTargetDepthFormat {
		t.Errorf("expected ColorTargetDepthFormat, got %v", crpe.Kind)
	}
	if crpe.TargetIndex != 1 {
		t.Errorf("expected TargetIndex 1, got %d", crpe.TargetIndex)
	}
}

func TestValidateRenderPipelineDescriptor_DepthStencilColorFormat(t *testing.T) {
	colorFormats := []gputypes.TextureFormat{
		gputypes.TextureFormatRGBA8Unorm,
		gputypes.TextureFormatBGRA8Unorm,
		gputypes.TextureFormatR8Unorm,
		gputypes.TextureFormatRGBA16Float,
	}

	for _, f := range colorFormats {
		t.Run(f.String(), func(t *testing.T) {
			desc := &hal.RenderPipelineDescriptor{
				Label: "test",
				Vertex: hal.VertexState{
					Module:     mockShaderModule{},
					EntryPoint: "vs_main",
				},
				DepthStencil: &hal.DepthStencilState{
					Format: f, // color format used as depth/stencil — invalid
				},
				Multisample: gputypes.MultisampleState{Count: 1},
			}
			err := ValidateRenderPipelineDescriptor(desc, gputypes.DefaultLimits())
			if err == nil {
				t.Fatalf("expected error for color format %s as depth/stencil", f)
			}
			var crpe *CreateRenderPipelineError
			if !errors.As(err, &crpe) {
				t.Fatalf("expected CreateRenderPipelineError, got %T", err)
			}
			if crpe.Kind != CreateRenderPipelineErrorDepthStencilColorFormat {
				t.Errorf("expected DepthStencilColorFormat, got %v", crpe.Kind)
			}
			if !strings.Contains(crpe.Error(), "not a depth/stencil format") {
				t.Errorf("expected error to mention 'not a depth/stencil format', got %q", crpe.Error())
			}
		})
	}
}

func TestValidateRenderPipelineDescriptor_ValidDepthStencil(t *testing.T) {
	depthFormats := []gputypes.TextureFormat{
		gputypes.TextureFormatDepth16Unorm,
		gputypes.TextureFormatDepth24Plus,
		gputypes.TextureFormatDepth24PlusStencil8,
		gputypes.TextureFormatDepth32Float,
		gputypes.TextureFormatDepth32FloatStencil8,
		gputypes.TextureFormatStencil8,
	}

	for _, f := range depthFormats {
		t.Run(f.String(), func(t *testing.T) {
			desc := &hal.RenderPipelineDescriptor{
				Label: "test",
				Vertex: hal.VertexState{
					Module:     mockShaderModule{},
					EntryPoint: "vs_main",
				},
				DepthStencil: &hal.DepthStencilState{
					Format: f, // valid depth/stencil format
				},
				Multisample: gputypes.MultisampleState{Count: 1},
			}
			err := ValidateRenderPipelineDescriptor(desc, gputypes.DefaultLimits())
			if err != nil {
				t.Fatalf("expected nil error for valid depth/stencil format %s, got: %v", f, err)
			}
		})
	}
}

func TestValidateRenderPipelineDescriptor_ValidColorTargetFormats(t *testing.T) {
	colorFormats := []gputypes.TextureFormat{
		gputypes.TextureFormatRGBA8Unorm,
		gputypes.TextureFormatBGRA8Unorm,
		gputypes.TextureFormatR8Unorm,
		gputypes.TextureFormatRGBA16Float,
		gputypes.TextureFormatRGBA32Float,
	}

	for _, f := range colorFormats {
		t.Run(f.String(), func(t *testing.T) {
			desc := &hal.RenderPipelineDescriptor{
				Label: "test",
				Vertex: hal.VertexState{
					Module:     mockShaderModule{},
					EntryPoint: "vs_main",
				},
				Fragment: &hal.FragmentState{
					Module:     mockShaderModule{},
					EntryPoint: "fs_main",
					Targets:    []gputypes.ColorTargetState{{Format: f}},
				},
				Multisample: gputypes.MultisampleState{Count: 1},
			}
			err := ValidateRenderPipelineDescriptor(desc, gputypes.DefaultLimits())
			if err != nil {
				t.Fatalf("expected nil error for valid color format %s, got: %v", f, err)
			}
		})
	}
}

// --- maxMips tests ---

func TestMaxMips(t *testing.T) {
	tests := []struct {
		name      string
		dimension gputypes.TextureDimension
		w, h, d   uint32
		want      uint32
	}{
		{"1x1 2D", gputypes.TextureDimension2D, 1, 1, 1, 1},
		{"2x2 2D", gputypes.TextureDimension2D, 2, 2, 1, 2},
		{"256x256 2D", gputypes.TextureDimension2D, 256, 256, 1, 9},
		{"1024x1 1D", gputypes.TextureDimension1D, 1024, 1, 1, 11},
		{"16x16x16 3D", gputypes.TextureDimension3D, 16, 16, 16, 5},
		{"256x128 2D", gputypes.TextureDimension2D, 256, 128, 1, 9},
		{"0 dimension", gputypes.TextureDimension2D, 0, 0, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := maxMips(tt.dimension, tt.w, tt.h, tt.d)
			if got != tt.want {
				t.Errorf("maxMips(%v, %d, %d, %d) = %d, want %d",
					tt.dimension, tt.w, tt.h, tt.d, got, tt.want)
			}
		})
	}
}

// --- Error string tests ---

func TestCreateTextureError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *CreateTextureError
		contains string
	}{
		{
			name:     "zero dimension",
			err:      &CreateTextureError{Kind: CreateTextureErrorZeroDimension, Label: "test"},
			contains: "must be greater than 0",
		},
		{
			name:     "max dimension",
			err:      &CreateTextureError{Kind: CreateTextureErrorMaxDimension, Label: "test", MaxDimension: 8192},
			contains: "exceeds maximum",
		},
		{
			name:     "empty usage",
			err:      &CreateTextureError{Kind: CreateTextureErrorEmptyUsage, Label: "test"},
			contains: "must not be empty",
		},
		{
			name:     "invalid format",
			err:      &CreateTextureError{Kind: CreateTextureErrorInvalidFormat, Label: "test"},
			contains: "must not be undefined",
		},
		{
			name:     "invalid dimension",
			err:      &CreateTextureError{Kind: CreateTextureErrorInvalidDimension, Label: "test"},
			contains: "must not be undefined",
		},
		{
			name:     "invalid sample count",
			err:      &CreateTextureError{Kind: CreateTextureErrorInvalidSampleCount, Label: "test", RequestedSamples: 3},
			contains: "must be 1 or 4",
		},
		{
			name:     "multisampled mip level",
			err:      &CreateTextureError{Kind: CreateTextureErrorMultisampleMipLevel, Label: "test"},
			contains: "multisampled",
		},
		{
			name:     "HAL error",
			err:      &CreateTextureError{Kind: CreateTextureErrorHAL, Label: "test", HALError: errors.New("backend error")},
			contains: "HAL error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := tt.err.Error()
			if msg == "" {
				t.Error("Error message should not be empty")
			}
			if !strings.Contains(msg, tt.contains) {
				t.Errorf("expected error to contain %q, got %q", tt.contains, msg)
			}
		})
	}
}

func TestCreateSamplerError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *CreateSamplerError
		contains string
	}{
		{
			name:     "invalid lod min",
			err:      &CreateSamplerError{Kind: CreateSamplerErrorInvalidLodMinClamp, Label: "test", LodMinClamp: -1},
			contains: "LodMinClamp",
		},
		{
			name:     "invalid lod max",
			err:      &CreateSamplerError{Kind: CreateSamplerErrorInvalidLodMaxClamp, Label: "test"},
			contains: "LodMaxClamp",
		},
		{
			name:     "anisotropy linear",
			err:      &CreateSamplerError{Kind: CreateSamplerErrorAnisotropyRequiresLinearFiltering, Label: "test", Anisotropy: 4},
			contains: "linear",
		},
		{
			name:     "HAL error",
			err:      &CreateSamplerError{Kind: CreateSamplerErrorHAL, Label: "test", HALError: errors.New("backend")},
			contains: "HAL error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := tt.err.Error()
			if msg == "" {
				t.Error("Error message should not be empty")
			}
			if !strings.Contains(msg, tt.contains) {
				t.Errorf("expected error to contain %q, got %q", tt.contains, msg)
			}
		})
	}
}

func TestCreateShaderModuleError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *CreateShaderModuleError
		contains string
	}{
		{
			name:     "no source",
			err:      &CreateShaderModuleError{Kind: CreateShaderModuleErrorNoSource, Label: "test"},
			contains: "either WGSL or SPIRV",
		},
		{
			name:     "dual source",
			err:      &CreateShaderModuleError{Kind: CreateShaderModuleErrorDualSource, Label: "test"},
			contains: "must not provide both",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := tt.err.Error()
			if msg == "" {
				t.Error("Error message should not be empty")
			}
			if !strings.Contains(msg, tt.contains) {
				t.Errorf("expected error to contain %q, got %q", tt.contains, msg)
			}
		})
	}
}

func TestCreateRenderPipelineError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *CreateRenderPipelineError
		contains string
	}{
		{
			name:     "missing vertex module",
			err:      &CreateRenderPipelineError{Kind: CreateRenderPipelineErrorMissingVertexModule, Label: "test"},
			contains: "vertex shader module",
		},
		{
			name:     "too many targets",
			err:      &CreateRenderPipelineError{Kind: CreateRenderPipelineErrorTooManyColorTargets, Label: "test", TargetCount: 10, MaxTargets: 8},
			contains: "exceeds maximum",
		},
		{
			name:     "color target depth format",
			err:      &CreateRenderPipelineError{Kind: CreateRenderPipelineErrorColorTargetDepthFormat, Label: "test", TargetIndex: 0, Format: "Depth32Float"},
			contains: "color aspect",
		},
		{
			name:     "depth stencil color format",
			err:      &CreateRenderPipelineError{Kind: CreateRenderPipelineErrorDepthStencilColorFormat, Label: "test", Format: "RGBA8Unorm"},
			contains: "not a depth/stencil format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := tt.err.Error()
			if msg == "" {
				t.Error("Error message should not be empty")
			}
			if !strings.Contains(msg, tt.contains) {
				t.Errorf("expected error to contain %q, got %q", tt.contains, msg)
			}
		})
	}
}

func TestCreateComputePipelineError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *CreateComputePipelineError
		contains string
	}{
		{
			name:     "missing module",
			err:      &CreateComputePipelineError{Kind: CreateComputePipelineErrorMissingModule, Label: "test"},
			contains: "compute shader module",
		},
		{
			name:     "missing entry point",
			err:      &CreateComputePipelineError{Kind: CreateComputePipelineErrorMissingEntryPoint, Label: "test"},
			contains: "entry point",
		},
		{
			name: "workgroup size exceeded",
			err: &CreateComputePipelineError{
				Kind: CreateComputePipelineErrorWorkgroupSizeExceeded, Label: "test",
				Dimension: "X", Size: 512, Limit: 256,
			},
			contains: "workgroup_size X = 512 exceeds device limit 256",
		},
		{
			name: "workgroup size zero",
			err: &CreateComputePipelineError{
				Kind: CreateComputePipelineErrorWorkgroupSizeZero, Label: "test",
				Dimension: "Y",
			},
			contains: "workgroup_size Y must not be zero",
		},
		{
			name: "too many invocations",
			err: &CreateComputePipelineError{
				Kind: CreateComputePipelineErrorTooManyInvocations, Label: "test",
				TotalInvocations: 2048, Limit: 256,
			},
			contains: "total workgroup invocations 2048 exceeds device limit 256",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := tt.err.Error()
			if msg == "" {
				t.Error("Error message should not be empty")
			}
			if !strings.Contains(msg, tt.contains) {
				t.Errorf("expected error to contain %q, got %q", tt.contains, msg)
			}
		})
	}
}

func TestCreatePipelineLayoutError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *CreatePipelineLayoutError
		contains string
	}{
		{
			name:     "too many groups",
			err:      &CreatePipelineLayoutError{Kind: CreatePipelineLayoutErrorTooManyGroups, Label: "test", Count: 5, MaxGroups: 4},
			contains: "exceeds device limit",
		},
		{
			name:     "HAL error",
			err:      &CreatePipelineLayoutError{Kind: CreatePipelineLayoutErrorHAL, Label: "test", HALError: errors.New("backend")},
			contains: "HAL error",
		},
		{
			name:     "unnamed label",
			err:      &CreatePipelineLayoutError{Kind: CreatePipelineLayoutErrorTooManyGroups, Label: "", Count: 8, MaxGroups: 4},
			contains: "<unnamed>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := tt.err.Error()
			if msg == "" {
				t.Error("Error message should not be empty")
			}
			if !strings.Contains(msg, tt.contains) {
				t.Errorf("expected error to contain %q, got %q", tt.contains, msg)
			}
		})
	}
}

func TestCreateBindGroupLayoutError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *CreateBindGroupLayoutError
		contains string
	}{
		{
			name:     "duplicate binding",
			err:      &CreateBindGroupLayoutError{Kind: CreateBindGroupLayoutErrorDuplicateBinding, Label: "test", DuplicateBinding: 3},
			contains: "duplicate binding",
		},
		{
			name:     "too many bindings",
			err:      &CreateBindGroupLayoutError{Kind: CreateBindGroupLayoutErrorTooManyBindings, Label: "test", BindingCount: 2000, MaxBindings: 1000},
			contains: "exceeds maximum",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := tt.err.Error()
			if msg == "" {
				t.Error("Error message should not be empty")
			}
			if !strings.Contains(msg, tt.contains) {
				t.Errorf("expected error to contain %q, got %q", tt.contains, msg)
			}
		})
	}
}

func TestCreateBindGroupError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *CreateBindGroupError
		contains string
	}{
		{
			name:     "missing layout",
			err:      &CreateBindGroupError{Kind: CreateBindGroupErrorMissingLayout, Label: "test"},
			contains: "must not be nil",
		},
		{
			name:     "bindings num mismatch",
			err:      &CreateBindGroupError{Kind: CreateBindGroupErrorBindingsNumMismatch, Label: "test", Expected: 4, Actual: 3},
			contains: "does not match",
		},
		{
			name:     "missing binding declaration",
			err:      &CreateBindGroupError{Kind: CreateBindGroupErrorMissingBindingDeclaration, Label: "test", Binding: 5},
			contains: "binding 5",
		},
		{
			name:     "duplicate binding",
			err:      &CreateBindGroupError{Kind: CreateBindGroupErrorDuplicateBinding, Label: "test", Binding: 2},
			contains: "binding 2",
		},
		{
			name:     "buffer usage mismatch",
			err:      &CreateBindGroupError{Kind: CreateBindGroupErrorBufferUsageMismatch, Label: "test", Binding: 0, ExpectedUsage: 0x40, ActualUsage: 0x80},
			contains: "usage mismatch",
		},
		{
			name:     "buffer offset alignment",
			err:      &CreateBindGroupError{Kind: CreateBindGroupErrorBufferOffsetAlignment, Label: "test", Binding: 1, Offset: 100, Alignment: 256},
			contains: "not aligned",
		},
		{
			name:     "buffer binding size too large",
			err:      &CreateBindGroupError{Kind: CreateBindGroupErrorBufferBindingSizeTooLarge, Label: "test", Binding: 0, Size: 100000, MaxSize: 65536},
			contains: "exceeds maximum",
		},
		{
			name:     "buffer bounds overflow",
			err:      &CreateBindGroupError{Kind: CreateBindGroupErrorBufferBoundsOverflow, Label: "test", Binding: 0, Offset: 100, Size: 200, BufferSize: 256},
			contains: "overflows",
		},
		{
			name:     "buffer binding zero size",
			err:      &CreateBindGroupError{Kind: CreateBindGroupErrorBufferBindingZeroSize, Label: "test", Binding: 0},
			contains: "zero",
		},
		{
			name:     "storage buffer size alignment",
			err:      &CreateBindGroupError{Kind: CreateBindGroupErrorStorageBufferSizeAlignment, Label: "test", Binding: 0, Size: 13},
			contains: "not a multiple of 4",
		},
		{
			name:     "HAL error",
			err:      &CreateBindGroupError{Kind: CreateBindGroupErrorHAL, Label: "test", HALError: errors.New("backend")},
			contains: "HAL error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := tt.err.Error()
			if msg == "" {
				t.Error("Error message should not be empty")
			}
			if !strings.Contains(msg, tt.contains) {
				t.Errorf("expected error to contain %q, got %q", tt.contains, msg)
			}
		})
	}
}

// --- ValidateBindGroupDescriptor buffer validation tests (VAL-A2) ---

// validBindGroupBufferSetup returns a valid bind group descriptor, layout entries,
// buffer infos, and limits for buffer validation tests. The setup has a single
// uniform buffer binding at slot 0.
func validBindGroupBufferSetup() (
	*hal.BindGroupDescriptor,
	[]gputypes.BindGroupLayoutEntry,
	[]BindGroupBufferInfo,
	gputypes.Limits,
) {
	limits := gputypes.DefaultLimits()
	layoutEntries := []gputypes.BindGroupLayoutEntry{
		{
			Binding: 0,
			Buffer: &gputypes.BufferBindingLayout{
				Type: gputypes.BufferBindingTypeUniform,
			},
		},
	}
	desc := &hal.BindGroupDescriptor{
		Label:  "test",
		Layout: mockBindGroupLayout{},
		Entries: []gputypes.BindGroupEntry{
			{Binding: 0},
		},
	}
	bufferInfos := []BindGroupBufferInfo{
		{
			Binding:    0,
			Usage:      gputypes.BufferUsageUniform,
			BufferSize: 1024,
			Offset:     0,
			Size:       256,
		},
	}
	return desc, layoutEntries, bufferInfos, limits
}

func TestValidateBindGroupDescriptor_BufferValid(t *testing.T) {
	desc, layoutEntries, bufferInfos, limits := validBindGroupBufferSetup()
	err := ValidateBindGroupDescriptor(desc, layoutEntries, bufferInfos, limits)
	if err != nil {
		t.Fatalf("expected nil error for valid buffer binding, got: %v", err)
	}
}

func TestValidateBindGroupDescriptor_BufferValidImplicitSize(t *testing.T) {
	desc, layoutEntries, bufferInfos, limits := validBindGroupBufferSetup()
	// Size == 0 means "rest of buffer from offset".
	bufferInfos[0].Size = 0
	bufferInfos[0].Offset = 256
	bufferInfos[0].BufferSize = 1024
	err := ValidateBindGroupDescriptor(desc, layoutEntries, bufferInfos, limits)
	if err != nil {
		t.Fatalf("expected nil error for valid implicit-size buffer binding, got: %v", err)
	}
}

func TestValidateBindGroupDescriptor_BufferUsageMismatch_UniformAsStorage(t *testing.T) {
	desc, layoutEntries, bufferInfos, limits := validBindGroupBufferSetup()
	// Layout wants uniform, but buffer only has storage usage.
	bufferInfos[0].Usage = gputypes.BufferUsageStorage

	err := ValidateBindGroupDescriptor(desc, layoutEntries, bufferInfos, limits)
	if err == nil {
		t.Fatal("expected error for buffer usage mismatch")
	}
	var cbge *CreateBindGroupError
	if !errors.As(err, &cbge) {
		t.Fatalf("expected CreateBindGroupError, got %T", err)
	}
	if cbge.Kind != CreateBindGroupErrorBufferUsageMismatch {
		t.Errorf("expected BufferUsageMismatch, got %v", cbge.Kind)
	}
	if cbge.Binding != 0 {
		t.Errorf("expected Binding=0, got %d", cbge.Binding)
	}
}

func TestValidateBindGroupDescriptor_BufferUsageMismatch_StorageAsUniform(t *testing.T) {
	desc, layoutEntries, bufferInfos, limits := validBindGroupBufferSetup()
	// Change layout to storage binding.
	layoutEntries[0].Buffer.Type = gputypes.BufferBindingTypeStorage
	// Buffer only has uniform usage, not storage.
	bufferInfos[0].Usage = gputypes.BufferUsageUniform
	bufferInfos[0].Size = 64 // multiple of 4 to avoid storage alignment error

	err := ValidateBindGroupDescriptor(desc, layoutEntries, bufferInfos, limits)
	if err == nil {
		t.Fatal("expected error for storage binding with uniform-only buffer")
	}
	var cbge *CreateBindGroupError
	if !errors.As(err, &cbge) {
		t.Fatalf("expected CreateBindGroupError, got %T", err)
	}
	if cbge.Kind != CreateBindGroupErrorBufferUsageMismatch {
		t.Errorf("expected BufferUsageMismatch, got %v", cbge.Kind)
	}
}

func TestValidateBindGroupDescriptor_BufferOffsetMisaligned_Uniform(t *testing.T) {
	desc, layoutEntries, bufferInfos, limits := validBindGroupBufferSetup()
	// Default MinUniformBufferOffsetAlignment is 256. Use offset 100.
	bufferInfos[0].Offset = 100

	err := ValidateBindGroupDescriptor(desc, layoutEntries, bufferInfos, limits)
	if err == nil {
		t.Fatal("expected error for misaligned uniform buffer offset")
	}
	var cbge *CreateBindGroupError
	if !errors.As(err, &cbge) {
		t.Fatalf("expected CreateBindGroupError, got %T", err)
	}
	if cbge.Kind != CreateBindGroupErrorBufferOffsetAlignment {
		t.Errorf("expected BufferOffsetAlignment, got %v", cbge.Kind)
	}
	if cbge.Offset != 100 {
		t.Errorf("expected Offset=100, got %d", cbge.Offset)
	}
	if cbge.Alignment != 256 {
		t.Errorf("expected Alignment=256, got %d", cbge.Alignment)
	}
}

func TestValidateBindGroupDescriptor_BufferOffsetMisaligned_Storage(t *testing.T) {
	desc, layoutEntries, bufferInfos, limits := validBindGroupBufferSetup()
	layoutEntries[0].Buffer.Type = gputypes.BufferBindingTypeStorage
	bufferInfos[0].Usage = gputypes.BufferUsageStorage
	bufferInfos[0].Offset = 128 // 128 is not aligned to 256 (MinStorageBufferOffsetAlignment)
	bufferInfos[0].Size = 128
	bufferInfos[0].BufferSize = 1024

	err := ValidateBindGroupDescriptor(desc, layoutEntries, bufferInfos, limits)
	if err == nil {
		t.Fatal("expected error for misaligned storage buffer offset")
	}
	var cbge *CreateBindGroupError
	if !errors.As(err, &cbge) {
		t.Fatalf("expected CreateBindGroupError, got %T", err)
	}
	if cbge.Kind != CreateBindGroupErrorBufferOffsetAlignment {
		t.Errorf("expected BufferOffsetAlignment, got %v", cbge.Kind)
	}
}

func TestValidateBindGroupDescriptor_BufferOffsetAligned(t *testing.T) {
	desc, layoutEntries, bufferInfos, limits := validBindGroupBufferSetup()
	// 256 is exactly aligned to MinUniformBufferOffsetAlignment (256).
	bufferInfos[0].Offset = 256
	bufferInfos[0].Size = 256
	bufferInfos[0].BufferSize = 1024

	err := ValidateBindGroupDescriptor(desc, layoutEntries, bufferInfos, limits)
	if err != nil {
		t.Fatalf("expected nil error for aligned offset, got: %v", err)
	}
}

func TestValidateBindGroupDescriptor_BufferBindingSizeTooLarge_Uniform(t *testing.T) {
	desc, layoutEntries, bufferInfos, limits := validBindGroupBufferSetup()
	// DefaultLimits MaxUniformBufferBindingSize is 65536. Use a larger size.
	bufferInfos[0].Size = 65537
	bufferInfos[0].BufferSize = 1 << 20 // 1 MiB buffer to avoid bounds error

	err := ValidateBindGroupDescriptor(desc, layoutEntries, bufferInfos, limits)
	if err == nil {
		t.Fatal("expected error for binding size exceeding max uniform buffer binding size")
	}
	var cbge *CreateBindGroupError
	if !errors.As(err, &cbge) {
		t.Fatalf("expected CreateBindGroupError, got %T", err)
	}
	if cbge.Kind != CreateBindGroupErrorBufferBindingSizeTooLarge {
		t.Errorf("expected BufferBindingSizeTooLarge, got %v", cbge.Kind)
	}
	if cbge.MaxSize != 65536 {
		t.Errorf("expected MaxSize=65536, got %d", cbge.MaxSize)
	}
}

func TestValidateBindGroupDescriptor_BufferBindingSizeTooLarge_Storage(t *testing.T) {
	desc, layoutEntries, bufferInfos, limits := validBindGroupBufferSetup()
	layoutEntries[0].Buffer.Type = gputypes.BufferBindingTypeStorage
	bufferInfos[0].Usage = gputypes.BufferUsageStorage
	// DefaultLimits MaxStorageBufferBindingSize is 134217728 (128 MiB).
	// Use a size that exceeds it and is 4-byte aligned.
	bufferInfos[0].Size = 134217732     // 128 MiB + 4
	bufferInfos[0].BufferSize = 1 << 28 // 256 MiB buffer

	err := ValidateBindGroupDescriptor(desc, layoutEntries, bufferInfos, limits)
	if err == nil {
		t.Fatal("expected error for binding size exceeding max storage buffer binding size")
	}
	var cbge *CreateBindGroupError
	if !errors.As(err, &cbge) {
		t.Fatalf("expected CreateBindGroupError, got %T", err)
	}
	if cbge.Kind != CreateBindGroupErrorBufferBindingSizeTooLarge {
		t.Errorf("expected BufferBindingSizeTooLarge, got %v", cbge.Kind)
	}
}

func TestValidateBindGroupDescriptor_BufferBoundsOverflow_ExplicitSize(t *testing.T) {
	desc, layoutEntries, bufferInfos, limits := validBindGroupBufferSetup()
	// offset=512, size=600, buffer=1024: 512+600=1112 > 1024
	bufferInfos[0].Offset = 512
	bufferInfos[0].Size = 600
	bufferInfos[0].BufferSize = 1024

	err := ValidateBindGroupDescriptor(desc, layoutEntries, bufferInfos, limits)
	if err == nil {
		t.Fatal("expected error for buffer bounds overflow")
	}
	var cbge *CreateBindGroupError
	if !errors.As(err, &cbge) {
		t.Fatalf("expected CreateBindGroupError, got %T", err)
	}
	if cbge.Kind != CreateBindGroupErrorBufferBoundsOverflow {
		t.Errorf("expected BufferBoundsOverflow, got %v", cbge.Kind)
	}
	if cbge.Offset != 512 {
		t.Errorf("expected Offset=512, got %d", cbge.Offset)
	}
	if cbge.Size != 600 {
		t.Errorf("expected Size=600, got %d", cbge.Size)
	}
	if cbge.BufferSize != 1024 {
		t.Errorf("expected BufferSize=1024, got %d", cbge.BufferSize)
	}
}

func TestValidateBindGroupDescriptor_BufferBoundsOverflow_ImplicitSize(t *testing.T) {
	desc, layoutEntries, bufferInfos, limits := validBindGroupBufferSetup()
	// Size=0 (implicit), offset > bufferSize
	bufferInfos[0].Size = 0
	bufferInfos[0].Offset = 2048
	bufferInfos[0].BufferSize = 1024

	err := ValidateBindGroupDescriptor(desc, layoutEntries, bufferInfos, limits)
	if err == nil {
		t.Fatal("expected error for buffer offset beyond buffer end with implicit size")
	}
	var cbge *CreateBindGroupError
	if !errors.As(err, &cbge) {
		t.Fatalf("expected CreateBindGroupError, got %T", err)
	}
	// Offset > BufferSize: effectiveSize = 0, caught by zero-size check before bounds check.
	if cbge.Kind != CreateBindGroupErrorBufferBindingZeroSize {
		t.Errorf("expected BufferBindingZeroSize (offset beyond buffer), got %v", cbge.Kind)
	}
}

func TestValidateBindGroupDescriptor_BufferBindingZeroSize_ExplicitZeroBuffer(t *testing.T) {
	desc, layoutEntries, bufferInfos, limits := validBindGroupBufferSetup()
	// Buffer of size 0 with implicit size binding: effective size = 0.
	bufferInfos[0].Size = 0
	bufferInfos[0].Offset = 0
	bufferInfos[0].BufferSize = 0

	err := ValidateBindGroupDescriptor(desc, layoutEntries, bufferInfos, limits)
	if err == nil {
		t.Fatal("expected error for zero effective binding size")
	}
	var cbge *CreateBindGroupError
	if !errors.As(err, &cbge) {
		t.Fatalf("expected CreateBindGroupError, got %T", err)
	}
	if cbge.Kind != CreateBindGroupErrorBufferBindingZeroSize {
		t.Errorf("expected BufferBindingZeroSize, got %v", cbge.Kind)
	}
}

func TestValidateBindGroupDescriptor_StorageBufferSizeNotMultipleOf4(t *testing.T) {
	desc, layoutEntries, bufferInfos, limits := validBindGroupBufferSetup()
	layoutEntries[0].Buffer.Type = gputypes.BufferBindingTypeStorage
	bufferInfos[0].Usage = gputypes.BufferUsageStorage
	bufferInfos[0].Size = 13 // not a multiple of 4
	bufferInfos[0].BufferSize = 1024

	err := ValidateBindGroupDescriptor(desc, layoutEntries, bufferInfos, limits)
	if err == nil {
		t.Fatal("expected error for storage buffer size not multiple of 4")
	}
	var cbge *CreateBindGroupError
	if !errors.As(err, &cbge) {
		t.Fatalf("expected CreateBindGroupError, got %T", err)
	}
	if cbge.Kind != CreateBindGroupErrorStorageBufferSizeAlignment {
		t.Errorf("expected StorageBufferSizeAlignment, got %v", cbge.Kind)
	}
	if cbge.Size != 13 {
		t.Errorf("expected Size=13, got %d", cbge.Size)
	}
}

func TestValidateBindGroupDescriptor_StorageBufferSizeMultipleOf4_ReadOnly(t *testing.T) {
	desc, layoutEntries, bufferInfos, limits := validBindGroupBufferSetup()
	layoutEntries[0].Buffer.Type = gputypes.BufferBindingTypeReadOnlyStorage
	bufferInfos[0].Usage = gputypes.BufferUsageStorage
	bufferInfos[0].Size = 15 // not a multiple of 4
	bufferInfos[0].BufferSize = 1024

	err := ValidateBindGroupDescriptor(desc, layoutEntries, bufferInfos, limits)
	if err == nil {
		t.Fatal("expected error for read-only storage buffer size not multiple of 4")
	}
	var cbge *CreateBindGroupError
	if !errors.As(err, &cbge) {
		t.Fatalf("expected CreateBindGroupError, got %T", err)
	}
	if cbge.Kind != CreateBindGroupErrorStorageBufferSizeAlignment {
		t.Errorf("expected StorageBufferSizeAlignment, got %v", cbge.Kind)
	}
}

func TestValidateBindGroupDescriptor_StorageBufferValid(t *testing.T) {
	desc, layoutEntries, bufferInfos, limits := validBindGroupBufferSetup()
	layoutEntries[0].Buffer.Type = gputypes.BufferBindingTypeStorage
	bufferInfos[0].Usage = gputypes.BufferUsageStorage
	bufferInfos[0].Size = 256 // aligned, multiple of 4, within limits
	bufferInfos[0].Offset = 0
	bufferInfos[0].BufferSize = 1024

	err := ValidateBindGroupDescriptor(desc, layoutEntries, bufferInfos, limits)
	if err != nil {
		t.Fatalf("expected nil error for valid storage buffer binding, got: %v", err)
	}
}

func TestValidateBindGroupDescriptor_BufferNoBufferLayoutEntry(t *testing.T) {
	// Buffer info provided for a binding whose layout entry is a sampler, not a buffer.
	// The buffer validation should skip this entry gracefully.
	layoutEntries := []gputypes.BindGroupLayoutEntry{
		{
			Binding: 0,
			Sampler: &gputypes.SamplerBindingLayout{
				Type: gputypes.SamplerBindingTypeFiltering,
			},
		},
	}
	desc := &hal.BindGroupDescriptor{
		Label:  "test",
		Layout: mockBindGroupLayout{},
		Entries: []gputypes.BindGroupEntry{
			{Binding: 0},
		},
	}
	bufferInfos := []BindGroupBufferInfo{
		{
			Binding:    0,
			Usage:      gputypes.BufferUsageUniform,
			BufferSize: 1024,
			Offset:     0,
			Size:       256,
		},
	}
	limits := gputypes.DefaultLimits()

	err := ValidateBindGroupDescriptor(desc, layoutEntries, bufferInfos, limits)
	if err != nil {
		t.Fatalf("expected nil error when buffer info binding has no buffer layout, got: %v", err)
	}
}

func TestValidateBindGroupDescriptor_BufferBoundsExact(t *testing.T) {
	desc, layoutEntries, bufferInfos, limits := validBindGroupBufferSetup()
	// Exact fit: offset + size == bufferSize
	bufferInfos[0].Offset = 0
	bufferInfos[0].Size = 1024
	bufferInfos[0].BufferSize = 1024

	err := ValidateBindGroupDescriptor(desc, layoutEntries, bufferInfos, limits)
	if err != nil {
		t.Fatalf("expected nil error for exact-fit buffer binding, got: %v", err)
	}
}
