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
	desc := &hal.BindGroupDescriptor{
		Label:  "test",
		Layout: mockBindGroupLayout{},
	}
	err := ValidateBindGroupDescriptor(desc)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
}

func TestValidateBindGroupDescriptor_MissingLayout(t *testing.T) {
	desc := &hal.BindGroupDescriptor{
		Label:  "test",
		Layout: nil,
	}
	err := ValidateBindGroupDescriptor(desc)
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
	err := &CreateBindGroupError{Kind: CreateBindGroupErrorMissingLayout, Label: "test"}
	msg := err.Error()
	if !strings.Contains(msg, "must not be nil") {
		t.Errorf("expected error to contain 'must not be nil', got %q", msg)
	}
}
