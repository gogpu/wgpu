package wgpu

import (
	"testing"
)

func TestBufferDescriptorToHAL(t *testing.T) {
	tests := []struct {
		name  string
		desc  BufferDescriptor
		check func(t *testing.T, desc *BufferDescriptor)
	}{
		{
			name: "basic fields",
			desc: BufferDescriptor{
				Label: "test-buf",
				Size:  1024,
				Usage: BufferUsageVertex | BufferUsageCopyDst,
			},
			check: func(t *testing.T, desc *BufferDescriptor) {
				halDesc := desc.toHAL()
				if halDesc.Label != desc.Label {
					t.Errorf("Label = %q, want %q", halDesc.Label, desc.Label)
				}
				if halDesc.Size != desc.Size {
					t.Errorf("Size = %d, want %d", halDesc.Size, desc.Size)
				}
				if halDesc.Usage != desc.Usage {
					t.Errorf("Usage = %v, want %v", halDesc.Usage, desc.Usage)
				}
				if halDesc.MappedAtCreation != desc.MappedAtCreation {
					t.Errorf("MappedAtCreation = %v, want %v", halDesc.MappedAtCreation, desc.MappedAtCreation)
				}
			},
		},
		{
			name: "mapped at creation",
			desc: BufferDescriptor{
				Label:            "mapped",
				Size:             256,
				Usage:            BufferUsageMapRead | BufferUsageCopyDst,
				MappedAtCreation: true,
			},
			check: func(t *testing.T, desc *BufferDescriptor) {
				halDesc := desc.toHAL()
				if !halDesc.MappedAtCreation {
					t.Error("MappedAtCreation should be true")
				}
			},
		},
		{
			name: "zero values",
			desc: BufferDescriptor{},
			check: func(t *testing.T, desc *BufferDescriptor) {
				halDesc := desc.toHAL()
				if halDesc.Label != "" {
					t.Errorf("Label = %q, want empty", halDesc.Label)
				}
				if halDesc.Size != 0 {
					t.Errorf("Size = %d, want 0", halDesc.Size)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.check(t, &tt.desc)
		})
	}
}

func TestTextureDescriptorToHAL(t *testing.T) {
	desc := TextureDescriptor{
		Label:         "test-tex",
		Size:          Extent3D{Width: 128, Height: 64, DepthOrArrayLayers: 1},
		MipLevelCount: 4,
		SampleCount:   1,
		Format:        TextureFormatRGBA8Unorm,
		Usage:         TextureUsageTextureBinding | TextureUsageCopyDst,
		ViewFormats:   []TextureFormat{TextureFormatRGBA8Unorm},
	}

	halDesc := desc.toHAL()

	if halDesc.Label != desc.Label {
		t.Errorf("Label = %q, want %q", halDesc.Label, desc.Label)
	}
	if halDesc.Size.Width != desc.Size.Width {
		t.Errorf("Size.Width = %d, want %d", halDesc.Size.Width, desc.Size.Width)
	}
	if halDesc.Size.Height != desc.Size.Height {
		t.Errorf("Size.Height = %d, want %d", halDesc.Size.Height, desc.Size.Height)
	}
	if halDesc.Size.DepthOrArrayLayers != desc.Size.DepthOrArrayLayers {
		t.Errorf("Size.DepthOrArrayLayers = %d, want %d", halDesc.Size.DepthOrArrayLayers, desc.Size.DepthOrArrayLayers)
	}
	if halDesc.MipLevelCount != desc.MipLevelCount {
		t.Errorf("MipLevelCount = %d, want %d", halDesc.MipLevelCount, desc.MipLevelCount)
	}
	if halDesc.SampleCount != desc.SampleCount {
		t.Errorf("SampleCount = %d, want %d", halDesc.SampleCount, desc.SampleCount)
	}
	if halDesc.Format != desc.Format {
		t.Errorf("Format = %v, want %v", halDesc.Format, desc.Format)
	}
	if halDesc.Usage != desc.Usage {
		t.Errorf("Usage = %v, want %v", halDesc.Usage, desc.Usage)
	}
	if len(halDesc.ViewFormats) != len(desc.ViewFormats) {
		t.Errorf("ViewFormats length = %d, want %d", len(halDesc.ViewFormats), len(desc.ViewFormats))
	}
}

func TestSamplerDescriptorToHAL(t *testing.T) {
	desc := SamplerDescriptor{
		Label:       "test-sampler",
		LodMinClamp: 0.0,
		LodMaxClamp: 32.0,
		Anisotropy:  16,
	}

	halDesc := desc.toHAL()

	if halDesc.Label != desc.Label {
		t.Errorf("Label = %q, want %q", halDesc.Label, desc.Label)
	}
	if halDesc.LodMinClamp != desc.LodMinClamp {
		t.Errorf("LodMinClamp = %f, want %f", halDesc.LodMinClamp, desc.LodMinClamp)
	}
	if halDesc.LodMaxClamp != desc.LodMaxClamp {
		t.Errorf("LodMaxClamp = %f, want %f", halDesc.LodMaxClamp, desc.LodMaxClamp)
	}
	if halDesc.Anisotropy != desc.Anisotropy {
		t.Errorf("Anisotropy = %d, want %d", halDesc.Anisotropy, desc.Anisotropy)
	}
}

func TestShaderModuleDescriptorToHAL(t *testing.T) {
	t.Run("WGSL source", func(t *testing.T) {
		desc := ShaderModuleDescriptor{
			Label: "wgsl-shader",
			WGSL:  "@vertex fn main() -> @builtin(position) vec4f { return vec4f(0.0); }",
		}
		halDesc := desc.toHAL()
		if halDesc.Label != desc.Label {
			t.Errorf("Label = %q, want %q", halDesc.Label, desc.Label)
		}
		if halDesc.Source.WGSL != desc.WGSL {
			t.Errorf("Source.WGSL = %q, want %q", halDesc.Source.WGSL, desc.WGSL)
		}
	})

	t.Run("SPIRV source", func(t *testing.T) {
		spirv := []uint32{0x07230203, 0x00010000}
		desc := ShaderModuleDescriptor{
			Label: "spirv-shader",
			SPIRV: spirv,
		}
		halDesc := desc.toHAL()
		if len(halDesc.Source.SPIRV) != len(spirv) {
			t.Errorf("Source.SPIRV length = %d, want %d", len(halDesc.Source.SPIRV), len(spirv))
		}
	})
}

func TestCommandEncoderDescriptorToHAL(t *testing.T) {
	desc := CommandEncoderDescriptor{Label: "test-encoder"}
	halDesc := desc.toHAL()
	if halDesc.Label != desc.Label {
		t.Errorf("Label = %q, want %q", halDesc.Label, desc.Label)
	}
}

func TestBindGroupLayoutDescriptorToHAL(t *testing.T) {
	desc := BindGroupLayoutDescriptor{
		Label:   "test-bgl",
		Entries: []BindGroupLayoutEntry{},
	}
	halDesc := desc.toHAL()
	if halDesc.Label != desc.Label {
		t.Errorf("Label = %q, want %q", halDesc.Label, desc.Label)
	}
	if len(halDesc.Entries) != 0 {
		t.Errorf("Entries length = %d, want 0", len(halDesc.Entries))
	}
}

func TestTextureViewDescriptorToHAL(t *testing.T) {
	desc := TextureViewDescriptor{
		Label:           "test-view",
		Format:          TextureFormatRGBA8Unorm,
		BaseMipLevel:    1,
		MipLevelCount:   3,
		BaseArrayLayer:  0,
		ArrayLayerCount: 1,
	}
	halDesc := desc.toHAL()
	if halDesc.Label != desc.Label {
		t.Errorf("Label = %q, want %q", halDesc.Label, desc.Label)
	}
	if halDesc.Format != desc.Format {
		t.Errorf("Format = %v, want %v", halDesc.Format, desc.Format)
	}
	if halDesc.BaseMipLevel != desc.BaseMipLevel {
		t.Errorf("BaseMipLevel = %d, want %d", halDesc.BaseMipLevel, desc.BaseMipLevel)
	}
	if halDesc.MipLevelCount != desc.MipLevelCount {
		t.Errorf("MipLevelCount = %d, want %d", halDesc.MipLevelCount, desc.MipLevelCount)
	}
}

func TestComputePipelineDescriptorToHAL(t *testing.T) {
	desc := ComputePipelineDescriptor{
		Label:      "compute-pipe",
		EntryPoint: "main",
		// Module is nil -- toHAL should handle this gracefully.
	}
	halDesc := desc.toHAL()
	if halDesc.Label != desc.Label {
		t.Errorf("Label = %q, want %q", halDesc.Label, desc.Label)
	}
}

func TestRenderPipelineDescriptorToHAL(t *testing.T) {
	desc := RenderPipelineDescriptor{
		Label: "render-pipe",
		Vertex: VertexState{
			EntryPoint: "vs_main",
			// Module is nil -- toHAL should handle this.
		},
	}
	halDesc := desc.toHAL()
	if halDesc.Label != desc.Label {
		t.Errorf("Label = %q, want %q", halDesc.Label, desc.Label)
	}
}

func TestComputePassDescriptorToHAL(t *testing.T) {
	desc := ComputePassDescriptor{Label: "compute-pass"}
	halDesc := desc.toHAL()
	if halDesc.Label != desc.Label {
		t.Errorf("Label = %q, want %q", halDesc.Label, desc.Label)
	}
}

func TestSurfaceConfigurationToHAL(t *testing.T) {
	desc := SurfaceConfiguration{
		Width:       800,
		Height:      600,
		Format:      TextureFormatBGRA8Unorm,
		Usage:       TextureUsageRenderAttachment,
		PresentMode: PresentModeFifo,
	}
	halDesc := desc.toHAL()
	if halDesc.Width != desc.Width {
		t.Errorf("Width = %d, want %d", halDesc.Width, desc.Width)
	}
	if halDesc.Height != desc.Height {
		t.Errorf("Height = %d, want %d", halDesc.Height, desc.Height)
	}
	if halDesc.Format != desc.Format {
		t.Errorf("Format = %v, want %v", halDesc.Format, desc.Format)
	}
	if halDesc.Usage != desc.Usage {
		t.Errorf("Usage = %v, want %v", halDesc.Usage, desc.Usage)
	}
	if halDesc.PresentMode != desc.PresentMode {
		t.Errorf("PresentMode = %v, want %v", halDesc.PresentMode, desc.PresentMode)
	}
}

func TestRenderPassDescriptorToHAL(t *testing.T) {
	desc := RenderPassDescriptor{
		Label: "render-pass",
		ColorAttachments: []RenderPassColorAttachment{
			{
				ClearValue: Color{R: 1, G: 0, B: 0, A: 1},
			},
		},
		DepthStencilAttachment: &RenderPassDepthStencilAttachment{
			DepthClearValue:   1.0,
			StencilClearValue: 0,
		},
	}
	halDesc := desc.toHAL()
	if halDesc.Label != desc.Label {
		t.Errorf("Label = %q, want %q", halDesc.Label, desc.Label)
	}
	if len(halDesc.ColorAttachments) != 1 {
		t.Errorf("ColorAttachments length = %d, want 1", len(halDesc.ColorAttachments))
	}
	if halDesc.DepthStencilAttachment == nil {
		t.Error("DepthStencilAttachment should not be nil")
	} else if halDesc.DepthStencilAttachment.DepthClearValue != 1.0 {
		t.Errorf("DepthClearValue = %f, want 1.0", halDesc.DepthStencilAttachment.DepthClearValue)
	}
}
