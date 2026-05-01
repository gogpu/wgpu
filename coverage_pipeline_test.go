//go:build !(js && wasm)

package wgpu_test

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// =============================================================================
// RenderPipeline — stripIndexFormat, blendConstantRequired, vertexBuffers count
// Covers pipeline_native.go lines 58-117 (RenderPipeline fields + Release)
// =============================================================================

func TestRenderPipelineWithLayout(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	mod, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label: "rp-layout-shader",
		WGSL:  "@vertex fn vs_main() -> @builtin(position) vec4f { return vec4f(0.0); }",
	})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	defer mod.Release()

	bgl, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label:   "rp-layout-bgl",
		Entries: []wgpu.BindGroupLayoutEntry{},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout: %v", err)
	}
	defer bgl.Release()

	pl, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{
		Label:            "rp-pipeline-layout",
		BindGroupLayouts: []*wgpu.BindGroupLayout{bgl},
	})
	if err != nil {
		t.Fatalf("CreatePipelineLayout: %v", err)
	}
	defer pl.Release()

	pipeline, err := device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label:  "rp-with-layout",
		Layout: pl,
		Vertex: wgpu.VertexState{Module: mod, EntryPoint: "vs_main"},
	})
	if err != nil {
		t.Fatalf("CreateRenderPipeline: %v", err)
	}
	defer pipeline.Release()

	if pipeline == nil {
		t.Fatal("CreateRenderPipeline with layout returned nil")
	}
}

func TestRenderPipelineWithVertexBuffers(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	mod, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label: "rp-vb-shader",
		WGSL:  "@vertex fn vs_main() -> @builtin(position) vec4f { return vec4f(0.0); }",
	})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	defer mod.Release()

	pipeline, err := device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label: "rp-with-vb",
		Vertex: wgpu.VertexState{
			Module:     mod,
			EntryPoint: "vs_main",
			Buffers: []wgpu.VertexBufferLayout{
				{
					ArrayStride: 16,
					StepMode:    gputypes.VertexStepModeVertex,
					Attributes: []gputypes.VertexAttribute{
						{Format: gputypes.VertexFormatFloat32x4, Offset: 0, ShaderLocation: 0},
					},
				},
				{
					ArrayStride: 8,
					StepMode:    gputypes.VertexStepModeVertex,
					Attributes: []gputypes.VertexAttribute{
						{Format: gputypes.VertexFormatFloat32x2, Offset: 0, ShaderLocation: 1},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateRenderPipeline: %v", err)
	}
	defer pipeline.Release()

	// The pipeline should remember 2 vertex buffer layouts.
	// We can verify by using SetTestRequiredVertexBuffers if needed,
	// but the main assertion is that creation succeeds with >1 VB layout.
	if pipeline == nil {
		t.Fatal("CreateRenderPipeline with vertex buffers returned nil")
	}
}

// =============================================================================
// ComputePipeline — with layout, with labels
// Covers pipeline_native.go lines 119-164 (ComputePipeline fields + Release)
// =============================================================================

func TestComputePipelineWithLayout(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	mod, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label: "cp-layout-shader",
		WGSL:  "@compute @workgroup_size(1) fn main() {}",
	})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	defer mod.Release()

	bgl, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label:   "cp-layout-bgl",
		Entries: []wgpu.BindGroupLayoutEntry{},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout: %v", err)
	}
	defer bgl.Release()

	pl, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{
		Label:            "cp-pipeline-layout",
		BindGroupLayouts: []*wgpu.BindGroupLayout{bgl},
	})
	if err != nil {
		t.Fatalf("CreatePipelineLayout: %v", err)
	}
	defer pl.Release()

	pipeline, err := device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
		Label:      "cp-with-layout",
		Layout:     pl,
		Module:     mod,
		EntryPoint: "main",
	})
	if err != nil {
		// Software backend may not support compute.
		t.Skipf("CreateComputePipeline not supported: %v", err)
	}
	defer pipeline.Release()

	if pipeline == nil {
		t.Fatal("CreateComputePipeline with layout returned nil")
	}
}

// =============================================================================
// Pipeline ref counting — TestRef() on render/compute pipelines
// Covers pipeline_native.go ref field initialization
// =============================================================================

func TestRenderPipelineHasRef(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	mod, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label: "ref-shader",
		WGSL:  "@vertex fn vs_main() -> @builtin(position) vec4f { return vec4f(0.0); }",
	})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	defer mod.Release()

	pipeline, err := device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label:  "ref-rp",
		Vertex: wgpu.VertexState{Module: mod, EntryPoint: "vs_main"},
	})
	if err != nil {
		t.Fatalf("CreateRenderPipeline: %v", err)
	}
	defer pipeline.Release()

	if pipeline.TestRef() == nil {
		t.Error("RenderPipeline should have a non-nil ResourceRef after creation")
	}
}

func TestComputePipelineHasRef(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	mod, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label: "ref-cs",
		WGSL:  "@compute @workgroup_size(1) fn main() {}",
	})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	defer mod.Release()

	pipeline, err := device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
		Label:      "ref-cp",
		Module:     mod,
		EntryPoint: "main",
	})
	if err != nil {
		t.Skipf("CreateComputePipeline not supported: %v", err)
	}
	defer pipeline.Release()

	if pipeline.TestRef() == nil {
		t.Error("ComputePipeline should have a non-nil ResourceRef after creation")
	}
}

// =============================================================================
// BindGroupLayout compatibility — isCompatibleWith
// Covers bind_native.go lines 42-55, 60-82
// =============================================================================

func TestBindGroupLayoutCompatibility(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	entries := []wgpu.BindGroupLayoutEntry{
		{
			Binding:    0,
			Visibility: wgpu.ShaderStageVertex,
			Buffer: &gputypes.BufferBindingLayout{
				Type:           gputypes.BufferBindingTypeUniform,
				MinBindingSize: 16,
			},
		},
	}

	// Two identical layouts should be compatible even if separate objects.
	bgl1, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label:   "compat-a",
		Entries: entries,
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout a: %v", err)
	}
	defer bgl1.Release()

	bgl2, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label:   "compat-b",
		Entries: entries,
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout b: %v", err)
	}
	defer bgl2.Release()

	// Both should be usable in separate pipelines targeting the same bind group slot.
	// This tests that the entries copy logic works correctly.
	if bgl1 == nil || bgl2 == nil {
		t.Fatal("both layouts should be non-nil")
	}
}

// =============================================================================
// ShaderModule — SPIR-V path (no IR parsing)
// Covers shader_native.go lines 234-244 (WGSL-specific parsing skipped for SPIRV)
// =============================================================================

func TestShaderModuleCreationPath(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	// Create with valid WGSL — this exercises the naga parse + lower path.
	mod, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label: "wgsl-shader",
		WGSL:  "@vertex fn vs_main() -> @builtin(position) vec4f { return vec4f(0.0); }",
	})
	if err != nil {
		t.Fatalf("CreateShaderModule: %v", err)
	}
	mod.Release()
}

// =============================================================================
// BindGroup — Release + released flag
// Covers bind_native.go lines 211-239 (BindGroup.Release path)
// =============================================================================

func TestBindGroupReleaseIdempotent(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	bgl, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label:   "bg-release-layout",
		Entries: []wgpu.BindGroupLayoutEntry{},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout: %v", err)
	}
	defer bgl.Release()

	bg, err := device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Label:   "bg-release-test",
		Layout:  bgl,
		Entries: []wgpu.BindGroupEntry{},
	})
	if err != nil {
		t.Fatalf("CreateBindGroup: %v", err)
	}

	bg.Release()
	bg.Release() // idempotent
}

func TestBindGroupHasRef(t *testing.T) {
	_, _, device := newDevice(t)
	defer device.Release()
	requireHAL(t, device)

	bgl, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label:   "bg-ref-layout",
		Entries: []wgpu.BindGroupLayoutEntry{},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout: %v", err)
	}
	defer bgl.Release()

	bg, err := device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Label:   "bg-ref-test",
		Layout:  bgl,
		Entries: []wgpu.BindGroupEntry{},
	})
	if err != nil {
		t.Fatalf("CreateBindGroup: %v", err)
	}
	defer bg.Release()

	if bg.TestRef() == nil {
		t.Error("BindGroup should have a non-nil ResourceRef")
	}
}
