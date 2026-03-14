package core

import (
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// =============================================================================
// Texture accessors and Destroy
// =============================================================================

// Format returns the texture's pixel format.
func (t *Texture) Format() gputypes.TextureFormat {
	return t.format
}

// Dimension returns the texture's dimension (1D, 2D, 3D).
func (t *Texture) Dimension() gputypes.TextureDimension {
	return t.dimension
}

// Usage returns the texture's usage flags.
func (t *Texture) Usage() gputypes.TextureUsage {
	return t.usage
}

// Size returns the texture's dimensions.
func (t *Texture) Size() gputypes.Extent3D {
	return t.size
}

// MipLevelCount returns the number of mip levels.
func (t *Texture) MipLevelCount() uint32 {
	return t.mipLevelCount
}

// SampleCount returns the number of samples per pixel.
func (t *Texture) SampleCount() uint32 {
	return t.sampleCount
}

// Label returns the texture's debug label.
func (t *Texture) Label() string {
	return t.label
}

// Destroy releases the HAL texture.
//
// This method is idempotent - calling it multiple times is safe.
// After calling Destroy(), Raw() returns nil.
func (t *Texture) Destroy() {
	untrackResource(uintptr(unsafe.Pointer(t))) //nolint:gosec // debug tracking uses pointer as unique ID

	if t.device == nil || t.device.SnatchLock() == nil || t.raw == nil {
		return
	}

	readGuard := t.device.SnatchLock().Read()
	halDevice := t.device.Raw(readGuard)
	readGuard.Release()

	if halDevice == nil {
		return
	}

	exclusiveGuard := t.device.SnatchLock().Write()
	defer exclusiveGuard.Release()

	halTexture := t.raw.Snatch(exclusiveGuard)
	if halTexture == nil {
		return
	}

	halDevice.DestroyTexture(*halTexture)
}

// IsDestroyed returns true if the texture has been destroyed.
func (t *Texture) IsDestroyed() bool {
	if t.raw == nil {
		return true
	}
	return t.raw.IsSnatched()
}

// =============================================================================
// Sampler accessors and Destroy
// =============================================================================

// Label returns the sampler's debug label.
func (s *Sampler) Label() string {
	return s.label
}

// Destroy releases the HAL sampler.
//
// This method is idempotent - calling it multiple times is safe.
// After calling Destroy(), Raw() returns nil.
func (s *Sampler) Destroy() {
	untrackResource(uintptr(unsafe.Pointer(s))) //nolint:gosec // debug tracking uses pointer as unique ID

	if s.device == nil || s.device.SnatchLock() == nil || s.raw == nil {
		return
	}

	readGuard := s.device.SnatchLock().Read()
	halDevice := s.device.Raw(readGuard)
	readGuard.Release()

	if halDevice == nil {
		return
	}

	exclusiveGuard := s.device.SnatchLock().Write()
	defer exclusiveGuard.Release()

	halSampler := s.raw.Snatch(exclusiveGuard)
	if halSampler == nil {
		return
	}

	halDevice.DestroySampler(*halSampler)
}

// IsDestroyed returns true if the sampler has been destroyed.
func (s *Sampler) IsDestroyed() bool {
	if s.raw == nil {
		return true
	}
	return s.raw.IsSnatched()
}

// =============================================================================
// BindGroupLayout accessors and Destroy
// =============================================================================

// Entries returns the binding entries in this layout.
//
// The returned slice is a direct reference to the internal entries.
// Callers must not modify the returned slice.
func (bgl *BindGroupLayout) Entries() []gputypes.BindGroupLayoutEntry {
	return bgl.entries
}

// EntryCount returns the number of binding entries in this layout.
func (bgl *BindGroupLayout) EntryCount() int {
	return len(bgl.entries)
}

// Label returns the bind group layout's debug label.
func (bgl *BindGroupLayout) Label() string {
	return bgl.label
}

// Destroy releases the HAL bind group layout.
//
// This method is idempotent - calling it multiple times is safe.
// After calling Destroy(), Raw() returns nil.
func (bgl *BindGroupLayout) Destroy() {
	untrackResource(uintptr(unsafe.Pointer(bgl))) //nolint:gosec // debug tracking uses pointer as unique ID

	if bgl.device == nil || bgl.device.SnatchLock() == nil || bgl.raw == nil {
		return
	}

	readGuard := bgl.device.SnatchLock().Read()
	halDevice := bgl.device.Raw(readGuard)
	readGuard.Release()

	if halDevice == nil {
		return
	}

	exclusiveGuard := bgl.device.SnatchLock().Write()
	defer exclusiveGuard.Release()

	halLayout := bgl.raw.Snatch(exclusiveGuard)
	if halLayout == nil {
		return
	}

	halDevice.DestroyBindGroupLayout(*halLayout)
}

// IsDestroyed returns true if the bind group layout has been destroyed.
func (bgl *BindGroupLayout) IsDestroyed() bool {
	if bgl.raw == nil {
		return true
	}
	return bgl.raw.IsSnatched()
}

// =============================================================================
// PipelineLayout accessors and Destroy
// =============================================================================

// BindGroupLayoutCount returns the number of bind group layouts in this pipeline layout.
func (pl *PipelineLayout) BindGroupLayoutCount() int {
	return pl.bindGroupLayoutCount
}

// Label returns the pipeline layout's debug label.
func (pl *PipelineLayout) Label() string {
	return pl.label
}

// Destroy releases the HAL pipeline layout.
//
// This method is idempotent - calling it multiple times is safe.
// After calling Destroy(), Raw() returns nil.
func (pl *PipelineLayout) Destroy() {
	untrackResource(uintptr(unsafe.Pointer(pl))) //nolint:gosec // debug tracking uses pointer as unique ID

	if pl.device == nil || pl.device.SnatchLock() == nil || pl.raw == nil {
		return
	}

	readGuard := pl.device.SnatchLock().Read()
	halDevice := pl.device.Raw(readGuard)
	readGuard.Release()

	if halDevice == nil {
		return
	}

	exclusiveGuard := pl.device.SnatchLock().Write()
	defer exclusiveGuard.Release()

	halLayout := pl.raw.Snatch(exclusiveGuard)
	if halLayout == nil {
		return
	}

	halDevice.DestroyPipelineLayout(*halLayout)
}

// IsDestroyed returns true if the pipeline layout has been destroyed.
func (pl *PipelineLayout) IsDestroyed() bool {
	if pl.raw == nil {
		return true
	}
	return pl.raw.IsSnatched()
}

// =============================================================================
// BindGroup accessors and Destroy
// =============================================================================

// Label returns the bind group's debug label.
func (bg *BindGroup) Label() string {
	return bg.label
}

// Destroy releases the HAL bind group.
//
// This method is idempotent - calling it multiple times is safe.
// After calling Destroy(), Raw() returns nil.
func (bg *BindGroup) Destroy() {
	untrackResource(uintptr(unsafe.Pointer(bg))) //nolint:gosec // debug tracking uses pointer as unique ID

	if bg.device == nil || bg.device.SnatchLock() == nil || bg.raw == nil {
		return
	}

	readGuard := bg.device.SnatchLock().Read()
	halDevice := bg.device.Raw(readGuard)
	readGuard.Release()

	if halDevice == nil {
		return
	}

	exclusiveGuard := bg.device.SnatchLock().Write()
	defer exclusiveGuard.Release()

	halGroup := bg.raw.Snatch(exclusiveGuard)
	if halGroup == nil {
		return
	}

	halDevice.DestroyBindGroup(*halGroup)
}

// IsDestroyed returns true if the bind group has been destroyed.
func (bg *BindGroup) IsDestroyed() bool {
	if bg.raw == nil {
		return true
	}
	return bg.raw.IsSnatched()
}

// =============================================================================
// ShaderModule accessors and Destroy
// =============================================================================

// Label returns the shader module's debug label.
func (sm *ShaderModule) Label() string {
	return sm.label
}

// Destroy releases the HAL shader module.
//
// This method is idempotent - calling it multiple times is safe.
// After calling Destroy(), Raw() returns nil.
func (sm *ShaderModule) Destroy() {
	untrackResource(uintptr(unsafe.Pointer(sm))) //nolint:gosec // debug tracking uses pointer as unique ID

	if sm.device == nil || sm.device.SnatchLock() == nil || sm.raw == nil {
		return
	}

	readGuard := sm.device.SnatchLock().Read()
	halDevice := sm.device.Raw(readGuard)
	readGuard.Release()

	if halDevice == nil {
		return
	}

	exclusiveGuard := sm.device.SnatchLock().Write()
	defer exclusiveGuard.Release()

	halModule := sm.raw.Snatch(exclusiveGuard)
	if halModule == nil {
		return
	}

	halDevice.DestroyShaderModule(*halModule)
}

// IsDestroyed returns true if the shader module has been destroyed.
func (sm *ShaderModule) IsDestroyed() bool {
	if sm.raw == nil {
		return true
	}
	return sm.raw.IsSnatched()
}

// =============================================================================
// RenderPipeline accessors and Destroy
// =============================================================================

// Label returns the render pipeline's debug label.
func (rp *RenderPipeline) Label() string {
	return rp.label
}

// Destroy releases the HAL render pipeline.
//
// This method is idempotent - calling it multiple times is safe.
// After calling Destroy(), Raw() returns nil.
func (rp *RenderPipeline) Destroy() {
	untrackResource(uintptr(unsafe.Pointer(rp))) //nolint:gosec // debug tracking uses pointer as unique ID

	if rp.device == nil || rp.device.SnatchLock() == nil || rp.raw == nil {
		return
	}

	readGuard := rp.device.SnatchLock().Read()
	halDevice := rp.device.Raw(readGuard)
	readGuard.Release()

	if halDevice == nil {
		return
	}

	exclusiveGuard := rp.device.SnatchLock().Write()
	defer exclusiveGuard.Release()

	halPipeline := rp.raw.Snatch(exclusiveGuard)
	if halPipeline == nil {
		return
	}

	halDevice.DestroyRenderPipeline(*halPipeline)
}

// IsDestroyed returns true if the render pipeline has been destroyed.
func (rp *RenderPipeline) IsDestroyed() bool {
	if rp.raw == nil {
		return true
	}
	return rp.raw.IsSnatched()
}

// =============================================================================
// ComputePipeline accessors and Destroy
// =============================================================================

// Label returns the compute pipeline's debug label.
func (cp *ComputePipeline) Label() string {
	return cp.label
}

// Destroy releases the HAL compute pipeline.
//
// This method is idempotent - calling it multiple times is safe.
// After calling Destroy(), Raw() returns nil.
func (cp *ComputePipeline) Destroy() {
	untrackResource(uintptr(unsafe.Pointer(cp))) //nolint:gosec // debug tracking uses pointer as unique ID

	if cp.device == nil || cp.device.SnatchLock() == nil || cp.raw == nil {
		return
	}

	readGuard := cp.device.SnatchLock().Read()
	halDevice := cp.device.Raw(readGuard)
	readGuard.Release()

	if halDevice == nil {
		return
	}

	exclusiveGuard := cp.device.SnatchLock().Write()
	defer exclusiveGuard.Release()

	halPipeline := cp.raw.Snatch(exclusiveGuard)
	if halPipeline == nil {
		return
	}

	halDevice.DestroyComputePipeline(*halPipeline)
}

// IsDestroyed returns true if the compute pipeline has been destroyed.
func (cp *ComputePipeline) IsDestroyed() bool {
	if cp.raw == nil {
		return true
	}
	return cp.raw.IsSnatched()
}

// =============================================================================
// QuerySet accessors and Destroy
// =============================================================================

// QueryType returns the type of queries in this set.
func (qs *QuerySet) QueryType() hal.QueryType {
	return qs.queryType
}

// Count returns the number of queries in the set.
func (qs *QuerySet) Count() uint32 {
	return qs.count
}

// Label returns the query set's debug label.
func (qs *QuerySet) Label() string {
	return qs.label
}

// Destroy releases the HAL query set.
//
// This method is idempotent - calling it multiple times is safe.
// After calling Destroy(), Raw() returns nil.
func (qs *QuerySet) Destroy() {
	untrackResource(uintptr(unsafe.Pointer(qs))) //nolint:gosec // debug tracking uses pointer as unique ID

	if qs.device == nil || qs.device.SnatchLock() == nil || qs.raw == nil {
		return
	}

	readGuard := qs.device.SnatchLock().Read()
	halDevice := qs.device.Raw(readGuard)
	readGuard.Release()

	if halDevice == nil {
		return
	}

	exclusiveGuard := qs.device.SnatchLock().Write()
	defer exclusiveGuard.Release()

	halQuerySet := qs.raw.Snatch(exclusiveGuard)
	if halQuerySet == nil {
		return
	}

	halDevice.DestroyQuerySet(*halQuerySet)
}

// IsDestroyed returns true if the query set has been destroyed.
func (qs *QuerySet) IsDestroyed() bool {
	if qs.raw == nil {
		return true
	}
	return qs.raw.IsSnatched()
}
