package core

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// mockQuerySet implements hal.QuerySet for testing.
type mockQuerySet struct{}

func (mockQuerySet) Destroy() {}

// =============================================================================
// Texture accessor tests
// =============================================================================

func TestTexture_Accessors(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	size := gputypes.Extent3D{Width: 256, Height: 128, DepthOrArrayLayers: 1}
	tex := NewTexture(
		mockTexture{}, device,
		gputypes.TextureFormatRGBA8Unorm,
		gputypes.TextureDimension2D,
		gputypes.TextureUsageRenderAttachment|gputypes.TextureUsageTextureBinding,
		size, 4, 1, "TestTexture",
	)

	if tex.Format() != gputypes.TextureFormatRGBA8Unorm {
		t.Errorf("Format() = %v, want RGBA8Unorm", tex.Format())
	}
	if tex.Dimension() != gputypes.TextureDimension2D {
		t.Errorf("Dimension() = %v, want 2D", tex.Dimension())
	}
	if tex.Usage() != gputypes.TextureUsageRenderAttachment|gputypes.TextureUsageTextureBinding {
		t.Errorf("Usage() = %v, want RenderAttachment|TextureBinding", tex.Usage())
	}
	if tex.Size() != size {
		t.Errorf("Size() = %v, want %v", tex.Size(), size)
	}
	if tex.MipLevelCount() != 4 {
		t.Errorf("MipLevelCount() = %d, want 4", tex.MipLevelCount())
	}
	if tex.SampleCount() != 1 {
		t.Errorf("SampleCount() = %d, want 1", tex.SampleCount())
	}
	if tex.Label() != "TestTexture" {
		t.Errorf("Label() = %q, want %q", tex.Label(), "TestTexture")
	}
}

func TestTexture_Destroy(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	tex := NewTexture(
		mockTexture{}, device,
		gputypes.TextureFormatRGBA8Unorm, gputypes.TextureDimension2D,
		gputypes.TextureUsageRenderAttachment,
		gputypes.Extent3D{Width: 64, Height: 64, DepthOrArrayLayers: 1},
		1, 1, "DestroyTest",
	)

	if tex.IsDestroyed() {
		t.Error("Texture should not be destroyed initially")
	}

	tex.Destroy()

	if !tex.IsDestroyed() {
		t.Error("Texture should be destroyed after Destroy()")
	}

	// Raw should return nil after destroy
	guard := device.SnatchLock().Read()
	raw := tex.Raw(guard)
	guard.Release()
	if raw != nil {
		t.Error("Raw() should return nil after Destroy()")
	}
}

func TestTexture_DestroyIdempotent(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	tex := NewTexture(
		mockTexture{}, device,
		gputypes.TextureFormatRGBA8Unorm, gputypes.TextureDimension2D,
		gputypes.TextureUsageRenderAttachment,
		gputypes.Extent3D{Width: 64, Height: 64, DepthOrArrayLayers: 1},
		1, 1, "IdempotentTest",
	)

	// Multiple destroy calls should be safe
	tex.Destroy()
	tex.Destroy()
	tex.Destroy()

	if !tex.IsDestroyed() {
		t.Error("Texture should be destroyed")
	}
}

// =============================================================================
// Sampler accessor tests
// =============================================================================

func TestSampler_Accessors(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	sampler := NewSampler(mockSampler{}, device, "TestSampler")

	if sampler.Label() != "TestSampler" {
		t.Errorf("Label() = %q, want %q", sampler.Label(), "TestSampler")
	}
}

func TestSampler_Destroy(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	sampler := NewSampler(mockSampler{}, device, "DestroyTest")

	if sampler.IsDestroyed() {
		t.Error("Sampler should not be destroyed initially")
	}

	sampler.Destroy()

	if !sampler.IsDestroyed() {
		t.Error("Sampler should be destroyed after Destroy()")
	}

	// Idempotent
	sampler.Destroy()
}

// =============================================================================
// BindGroupLayout accessor tests
// =============================================================================

func TestBindGroupLayout_Accessors(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	entries := []gputypes.BindGroupLayoutEntry{
		{Binding: 0},
		{Binding: 1},
		{Binding: 2},
	}
	bgl := NewBindGroupLayout(mockBindGroupLayout{}, device, entries, "TestBGL")

	if bgl.Label() != "TestBGL" {
		t.Errorf("Label() = %q, want %q", bgl.Label(), "TestBGL")
	}
	if bgl.EntryCount() != 3 {
		t.Errorf("EntryCount() = %d, want 3", bgl.EntryCount())
	}
	gotEntries := bgl.Entries()
	if len(gotEntries) != 3 {
		t.Fatalf("Entries() len = %d, want 3", len(gotEntries))
	}
	for i, e := range gotEntries {
		if e.Binding != uint32(i) {
			t.Errorf("Entries()[%d].Binding = %d, want %d", i, e.Binding, i)
		}
	}
}

func TestBindGroupLayout_Destroy(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	bgl := NewBindGroupLayout(mockBindGroupLayout{}, device, nil, "DestroyTest")

	if bgl.IsDestroyed() {
		t.Error("BindGroupLayout should not be destroyed initially")
	}

	bgl.Destroy()

	if !bgl.IsDestroyed() {
		t.Error("BindGroupLayout should be destroyed after Destroy()")
	}

	// Idempotent
	bgl.Destroy()
}

// =============================================================================
// PipelineLayout accessor tests
// =============================================================================

func TestPipelineLayout_Accessors(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	pl := NewPipelineLayout(mockPipelineLayout{}, device, 3, "TestPL")

	if pl.Label() != "TestPL" {
		t.Errorf("Label() = %q, want %q", pl.Label(), "TestPL")
	}
	if pl.BindGroupLayoutCount() != 3 {
		t.Errorf("BindGroupLayoutCount() = %d, want 3", pl.BindGroupLayoutCount())
	}
}

func TestPipelineLayout_Destroy(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	pl := NewPipelineLayout(mockPipelineLayout{}, device, 2, "DestroyTest")

	if pl.IsDestroyed() {
		t.Error("PipelineLayout should not be destroyed initially")
	}

	pl.Destroy()

	if !pl.IsDestroyed() {
		t.Error("PipelineLayout should be destroyed after Destroy()")
	}

	// Idempotent
	pl.Destroy()
}

// =============================================================================
// BindGroup accessor tests
// =============================================================================

func TestBindGroup_Accessors(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	bg := NewBindGroup(mockBindGroup{}, device, "TestBG")

	if bg.Label() != "TestBG" {
		t.Errorf("Label() = %q, want %q", bg.Label(), "TestBG")
	}
}

func TestBindGroup_Destroy(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	bg := NewBindGroup(mockBindGroup{}, device, "DestroyTest")

	if bg.IsDestroyed() {
		t.Error("BindGroup should not be destroyed initially")
	}

	bg.Destroy()

	if !bg.IsDestroyed() {
		t.Error("BindGroup should be destroyed after Destroy()")
	}

	// Idempotent
	bg.Destroy()
}

// =============================================================================
// ShaderModule accessor tests
// =============================================================================

func TestShaderModule_Accessors(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	sm := NewShaderModule(mockShaderModule{}, device, "TestSM")

	if sm.Label() != "TestSM" {
		t.Errorf("Label() = %q, want %q", sm.Label(), "TestSM")
	}
}

func TestShaderModule_Destroy(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	sm := NewShaderModule(mockShaderModule{}, device, "DestroyTest")

	if sm.IsDestroyed() {
		t.Error("ShaderModule should not be destroyed initially")
	}

	sm.Destroy()

	if !sm.IsDestroyed() {
		t.Error("ShaderModule should be destroyed after Destroy()")
	}

	// Idempotent
	sm.Destroy()
}

// =============================================================================
// RenderPipeline accessor tests
// =============================================================================

func TestRenderPipeline_Accessors(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	rp := NewRenderPipeline(mockRenderPipeline{}, device, "TestRP")

	if rp.Label() != "TestRP" {
		t.Errorf("Label() = %q, want %q", rp.Label(), "TestRP")
	}
}

func TestRenderPipeline_Destroy(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	rp := NewRenderPipeline(mockRenderPipeline{}, device, "DestroyTest")

	if rp.IsDestroyed() {
		t.Error("RenderPipeline should not be destroyed initially")
	}

	rp.Destroy()

	if !rp.IsDestroyed() {
		t.Error("RenderPipeline should be destroyed after Destroy()")
	}

	// Idempotent
	rp.Destroy()
}

// =============================================================================
// ComputePipeline accessor tests
// =============================================================================

func TestComputePipeline_Accessors(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	cp := NewComputePipeline(mockComputePipeline{}, device, "TestCP")

	if cp.Label() != "TestCP" {
		t.Errorf("Label() = %q, want %q", cp.Label(), "TestCP")
	}
}

func TestComputePipeline_Destroy(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	cp := NewComputePipeline(mockComputePipeline{}, device, "DestroyTest")

	if cp.IsDestroyed() {
		t.Error("ComputePipeline should not be destroyed initially")
	}

	cp.Destroy()

	if !cp.IsDestroyed() {
		t.Error("ComputePipeline should be destroyed after Destroy()")
	}

	// Idempotent
	cp.Destroy()
}

// =============================================================================
// QuerySet accessor tests
// =============================================================================

func TestQuerySet_Accessors(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	qs := NewQuerySet(mockQuerySet{}, device, hal.QueryTypeOcclusion, 16, "TestQS")

	if qs.QueryType() != hal.QueryTypeOcclusion {
		t.Errorf("QueryType() = %v, want Occlusion", qs.QueryType())
	}
	if qs.Count() != 16 {
		t.Errorf("Count() = %d, want 16", qs.Count())
	}
	if qs.Label() != "TestQS" {
		t.Errorf("Label() = %q, want %q", qs.Label(), "TestQS")
	}
}

func TestQuerySet_Destroy(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	qs := NewQuerySet(mockQuerySet{}, device, hal.QueryTypeOcclusion, 8, "DestroyTest")

	if qs.IsDestroyed() {
		t.Error("QuerySet should not be destroyed initially")
	}

	qs.Destroy()

	if !qs.IsDestroyed() {
		t.Error("QuerySet should be destroyed after Destroy()")
	}

	// Idempotent
	qs.Destroy()
}

// =============================================================================
// Cross-cutting destroy tests
// =============================================================================

func TestDestroy_AfterDeviceDestroyed(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	tex := NewTexture(
		mockTexture{}, device,
		gputypes.TextureFormatRGBA8Unorm, gputypes.TextureDimension2D,
		gputypes.TextureUsageRenderAttachment,
		gputypes.Extent3D{Width: 64, Height: 64, DepthOrArrayLayers: 1},
		1, 1, "OrphanTexture",
	)

	// Destroy device first
	device.Destroy()

	// Destroying texture after device should not panic
	tex.Destroy()
}

func TestDestroy_NilDevice(t *testing.T) {
	// Resources with nil device should not panic on Destroy
	tex := &Texture{}
	tex.Destroy()

	sampler := &Sampler{}
	sampler.Destroy()

	bgl := &BindGroupLayout{}
	bgl.Destroy()

	pl := &PipelineLayout{}
	pl.Destroy()

	bg := &BindGroup{}
	bg.Destroy()

	sm := &ShaderModule{}
	sm.Destroy()

	rp := &RenderPipeline{}
	rp.Destroy()

	cp := &ComputePipeline{}
	cp.Destroy()

	qs := &QuerySet{}
	qs.Destroy()
}
