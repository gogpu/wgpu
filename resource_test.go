//go:build !(js && wasm)

package wgpu_test

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// =============================================================================
// Buffer accessor tests — Size(), Usage(), MapState(), Label()
// Covers buffer.go missed lines
// =============================================================================

func TestBufferAccessorsTableDriven(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	tests := []struct {
		name  string
		label string
		size  uint64
		usage wgpu.BufferUsage
	}{
		{"small_vertex", "small-vertex-buf", 64, wgpu.BufferUsageVertex | wgpu.BufferUsageCopyDst},
		{"large_storage", "large-storage-buf", 4096, wgpu.BufferUsageStorage | wgpu.BufferUsageCopyDst},
		{"uniform_buffer", "uniform-buf", 256, wgpu.BufferUsageUniform | wgpu.BufferUsageCopyDst},
		{"minimal_size", "min-buf", 4, wgpu.BufferUsageCopyDst},
		{"copysrc_copydst", "copy-buf", 128, wgpu.BufferUsageCopySrc | wgpu.BufferUsageCopyDst},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
				Label: tt.label,
				Size:  tt.size,
				Usage: tt.usage,
			})
			if err != nil {
				t.Fatalf("CreateBuffer: %v", err)
			}
			defer buf.Release()

			if got := buf.Size(); got != tt.size {
				t.Errorf("Size() = %d, want %d", got, tt.size)
			}
			if got := buf.Usage(); got != tt.usage {
				t.Errorf("Usage() = %v, want %v", got, tt.usage)
			}
			if got := buf.Label(); got != tt.label {
				t.Errorf("Label() = %q, want %q", got, tt.label)
			}
		})
	}
}

func TestBufferMapStateUnmapped(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "mapstate-unmapped",
		Size:  64,
		Usage: wgpu.BufferUsageVertex,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer buf.Release()

	state := buf.MapState()
	if state != wgpu.MapStateUnmapped {
		t.Errorf("MapState() = %v, want MapStateUnmapped", state)
	}
}

func TestBufferMapStateMappedAtCreation(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label:            "mapstate-mapped",
		Size:             64,
		Usage:            wgpu.BufferUsageMapWrite | wgpu.BufferUsageCopySrc,
		MappedAtCreation: true,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer buf.Release()

	state := buf.MapState()
	if state != wgpu.MapStateMapped {
		t.Errorf("MapState() for MappedAtCreation = %v, want MapStateMapped", state)
	}
}

func TestBufferMapStateOnNilBuffer(t *testing.T) {
	// MapState on nil buffer should return Unmapped without panic.
	var buf *wgpu.Buffer
	state := buf.MapState()
	if state != wgpu.MapStateUnmapped {
		t.Errorf("MapState() on nil buffer = %v, want MapStateUnmapped", state)
	}
}

func TestBufferReleaseTrackesReleasedFlag(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "release-tracked",
		Size:  64,
		Usage: wgpu.BufferUsageVertex,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}

	if buf.TestReleased() {
		t.Error("buffer should not be released before Release()")
	}

	buf.Release()
	if !buf.TestReleased() {
		t.Error("buffer should be released after Release()")
	}

	// Double release should not change state or panic.
	buf.Release()
	if !buf.TestReleased() {
		t.Error("buffer should remain released after double Release()")
	}
}

// =============================================================================
// Texture accessor tests
// Covers texture_native.go missed lines
// =============================================================================

func TestTextureFormat(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	tests := []struct {
		name   string
		format wgpu.TextureFormat
	}{
		{"RGBA8Unorm", wgpu.TextureFormatRGBA8Unorm},
		{"BGRA8Unorm", wgpu.TextureFormatBGRA8Unorm},
		{"R8Unorm", gputypes.TextureFormatR8Unorm},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
				Label:         "fmt-" + tt.name,
				Size:          wgpu.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1},
				MipLevelCount: 1,
				SampleCount:   1,
				Dimension:     wgpu.TextureDimension2D,
				Format:        tt.format,
				Usage:         wgpu.TextureUsageTextureBinding,
			})
			if err != nil {
				t.Fatalf("CreateTexture: %v", err)
			}
			defer tex.Release()

			if got := tex.Format(); got != tt.format {
				t.Errorf("Format() = %v, want %v", got, tt.format)
			}
		})
	}
}

func TestTextureHalTexture(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "hal-texture",
		Size:          wgpu.Extent3D{Width: 8, Height: 8, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     wgpu.TextureDimension2D,
		Format:        wgpu.TextureFormatRGBA8Unorm,
		Usage:         wgpu.TextureUsageTextureBinding,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}

	// Before release, HalTexture should be non-nil.
	if tex.HalTexture() == nil {
		t.Error("HalTexture() is nil on live texture")
	}

	tex.Release()

	// After release, HalTexture should be nil.
	if tex.HalTexture() != nil {
		t.Error("HalTexture() should be nil after Release")
	}
}

func TestTextureReleaseIdempotent(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "release-tex",
		Size:          wgpu.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     wgpu.TextureDimension2D,
		Format:        wgpu.TextureFormatRGBA8Unorm,
		Usage:         wgpu.TextureUsageTextureBinding,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}

	tex.Release()
	tex.Release() // idempotent — should not panic
}

// =============================================================================
// TextureView accessor tests
// Covers texture_native.go TextureView missed lines
// =============================================================================

func TestTextureViewHalTextureView(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "tv-hal-texture",
		Size:          wgpu.Extent3D{Width: 8, Height: 8, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     wgpu.TextureDimension2D,
		Format:        wgpu.TextureFormatRGBA8Unorm,
		Usage:         wgpu.TextureUsageTextureBinding,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	defer tex.Release()

	view, err := device.CreateTextureView(tex, nil)
	if err != nil {
		t.Fatalf("CreateTextureView: %v", err)
	}

	if view.HalTextureView() == nil {
		t.Error("HalTextureView() is nil on live view")
	}

	view.Release()

	if view.HalTextureView() != nil {
		t.Error("HalTextureView() should be nil after Release")
	}
}

func TestTextureViewReleaseIdempotent(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "tv-release-tex",
		Size:          wgpu.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     wgpu.TextureDimension2D,
		Format:        wgpu.TextureFormatRGBA8Unorm,
		Usage:         wgpu.TextureUsageTextureBinding,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	defer tex.Release()

	view, err := device.CreateTextureView(tex, nil)
	if err != nil {
		t.Fatalf("CreateTextureView: %v", err)
	}

	view.Release()
	view.Release() // idempotent
}

// =============================================================================
// Sampler tests
// Covers sampler_native.go missed lines
// =============================================================================

func TestSamplerReleaseIdempotent(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	sampler, err := device.CreateSampler(&wgpu.SamplerDescriptor{
		Label: "release-sampler",
	})
	if err != nil {
		t.Fatalf("CreateSampler: %v", err)
	}

	sampler.Release()
	sampler.Release() // idempotent
}

func TestSamplerWithFullDescriptor(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	sampler, err := device.CreateSampler(&wgpu.SamplerDescriptor{
		Label:        "full-sampler",
		AddressModeU: gputypes.AddressModeRepeat,
		AddressModeV: gputypes.AddressModeRepeat,
		AddressModeW: gputypes.AddressModeRepeat,
		MagFilter:    gputypes.FilterModeLinear,
		MinFilter:    gputypes.FilterModeLinear,
		MipmapFilter: gputypes.FilterModeLinear,
		LodMinClamp:  0.0,
		LodMaxClamp:  32.0,
	})
	if err != nil {
		t.Fatalf("CreateSampler: %v", err)
	}
	defer sampler.Release()

	if sampler == nil {
		t.Fatal("CreateSampler returned nil for full descriptor")
	}
}

// =============================================================================
// ShaderModule tests
// Covers shader_native.go missed lines
// =============================================================================

func TestShaderModuleReleaseIdempotent(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	mod, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label: "release-shader",
		WGSL:  "@vertex fn vs_main() -> @builtin(position) vec4f { return vec4f(0.0); }",
	})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}

	mod.Release()
	mod.Release() // idempotent
}

func TestShaderModuleEmptyWGSL(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()

	// Empty WGSL with no SPIRV should fail validation.
	_, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label: "empty-shader",
		WGSL:  "",
	})
	if err == nil {
		t.Fatal("CreateShaderModule with empty WGSL should fail")
	}
}

// =============================================================================
// RenderPipeline tests
// Covers pipeline_native.go missed lines
// =============================================================================

func TestRenderPipelineReleaseIdempotent(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	mod, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label: "rp-shader",
		WGSL:  "@vertex fn vs_main() -> @builtin(position) vec4f { return vec4f(0.0); }",
	})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	defer mod.Release()

	pipeline, err := device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label:  "release-rp",
		Vertex: wgpu.VertexState{Module: mod, EntryPoint: "vs_main"},
	})
	if err != nil {
		t.Fatalf("CreateRenderPipeline: %v", err)
	}

	pipeline.Release()
	pipeline.Release() // idempotent
}

// =============================================================================
// ComputePipeline tests
// Covers pipeline_native.go missed lines
// =============================================================================

func TestComputePipelineReleaseIdempotent(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	mod, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label: "cp-shader",
		WGSL:  "@compute @workgroup_size(1) fn main() {}",
	})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	defer mod.Release()

	pipeline, err := device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
		Label:      "release-cp",
		Module:     mod,
		EntryPoint: "main",
	})
	if err != nil {
		// Software backend does not support compute pipelines.
		t.Skipf("CreateComputePipeline not supported by this backend: %v", err)
	}

	pipeline.Release()
	pipeline.Release() // idempotent
}

// =============================================================================
// BindGroupLayout tests
// Covers bind_native.go missed lines
// =============================================================================

func TestBindGroupLayoutReleaseIdempotent(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	layout, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label:   "release-bgl",
		Entries: []wgpu.BindGroupLayoutEntry{},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout: %v", err)
	}

	layout.Release()
	layout.Release() // idempotent
}

// =============================================================================
// PipelineLayout tests
// Covers bind_native.go missed lines
// =============================================================================

func TestPipelineLayoutReleaseIdempotent(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	bgl, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label:   "pl-bgl",
		Entries: []wgpu.BindGroupLayoutEntry{},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout: %v", err)
	}
	defer bgl.Release()

	layout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{
		Label:            "release-pl",
		BindGroupLayouts: []*wgpu.BindGroupLayout{bgl},
	})
	if err != nil {
		t.Fatalf("CreatePipelineLayout: %v", err)
	}

	layout.Release()
	layout.Release() // idempotent
}

// =============================================================================
// BindGroup with buffer and texture resource collection
// Covers bind_native.go collectBindGroupResources + boundBuffers/boundTextures
// =============================================================================

func TestBindGroupWithBufferResource(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "bg-buffer",
		Size:  64,
		Usage: wgpu.BufferUsageUniform | wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer buf.Release()

	bufLayoutEntry := wgpu.BindGroupLayoutEntry{
		Binding:    0,
		Visibility: wgpu.ShaderStageVertex | wgpu.ShaderStageFragment,
		Buffer: &gputypes.BufferBindingLayout{
			Type:           gputypes.BufferBindingTypeUniform,
			MinBindingSize: 64,
		},
	}

	layout, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label:   "bg-layout",
		Entries: []wgpu.BindGroupLayoutEntry{bufLayoutEntry},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout: %v", err)
	}
	defer layout.Release()

	bg, err := device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Label:  "bg-with-buffer",
		Layout: layout,
		Entries: []wgpu.BindGroupEntry{
			{Binding: 0, Buffer: buf, Offset: 0, Size: 64},
		},
	})
	if err != nil {
		t.Fatalf("CreateBindGroup: %v", err)
	}
	defer bg.Release()

	if bg.TestBindGroupReleased() {
		t.Error("bind group should not be released immediately after creation")
	}
}

func TestBindGroupReleaseTrackesReleasedFlag(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	layout, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label:   "bg-release-layout",
		Entries: []wgpu.BindGroupLayoutEntry{},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout: %v", err)
	}
	defer layout.Release()

	bg, err := device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Label:   "bg-release-test",
		Layout:  layout,
		Entries: []wgpu.BindGroupEntry{},
	})
	if err != nil {
		t.Fatalf("CreateBindGroup: %v", err)
	}

	if bg.TestBindGroupReleased() {
		t.Error("bind group should not be released before Release()")
	}

	bg.Release()

	if !bg.TestBindGroupReleased() {
		t.Error("bind group should be released after Release()")
	}

	// Double release should not panic.
	bg.Release()
}
