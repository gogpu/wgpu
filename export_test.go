//go:build !rust && !(js && wasm)

package wgpu

import (
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/hal"
	"github.com/gogpu/wgpu/internal/resource"
)

// SetTestRequiredVertexBuffers sets the requiredVertexBuffers field for testing.
// This method is only available in test builds.
func (p *RenderPipeline) SetTestRequiredVertexBuffers(count uint32) {
	p.requiredVertexBuffers = count
}

// TestRef returns the ResourceRef for a RenderPipeline (testing only).
func (p *RenderPipeline) TestRef() *core.ResourceRef { return p.ref }

// TestRef returns the ResourceRef for a ComputePipeline (testing only).
func (p *ComputePipeline) TestRef() *core.ResourceRef { return p.ref }

// TestRef returns the ResourceRef for a BindGroup (testing only).
func (g *BindGroup) TestRef() *core.ResourceRef { return g.ref }

// TestTrackedRefs returns the tracked refs of a CommandBuffer (testing only).
func (cb *CommandBuffer) TestTrackedRefs() []*core.ResourceRef { return cb.trackedRefs }

// TestTrackedRefs returns the tracked refs of a CommandEncoder (testing only).
func (e *CommandEncoder) TestTrackedRefs() []*core.ResourceRef { return e.trackedRefs }

// TestHALEncoder returns the HAL encoder reference on a CommandBuffer (testing only).
func (cb *CommandBuffer) TestHALEncoder() interface{} { return cb.halEncoder }

// TestCmdEncoderPoolSize returns the number of free encoders in the device's pool (testing only).
// Returns -1 if no pool is configured.
func (d *Device) TestCmdEncoderPoolSize() int {
	if d.cmdEncoderPool == nil {
		return -1
	}
	d.cmdEncoderPool.mu.Lock()
	defer d.cmdEncoderPool.mu.Unlock()
	return len(d.cmdEncoderPool.free)
}

// BufferTestReleased reports whether the buffer has been released (testing
// only). It is a free function rather than a Buffer method: a test-only method
// of a type that moves to an extraction package cannot stay behind, while a
// function over the type can (ADR-070 §11 grain note; the implementation's own
// white-box test carries the same assertion once it moves).
func BufferTestReleased(b *Buffer) bool {
	if b == nil || b.impl == nil {
		return false
	}
	return b.impl.IsReleased()
}

// NewTestBufferFromCore wraps an existing core buffer in a Buffer (testing only).
// HAL is whatever the core buffer carries; sufficient for validation unit tests
// that read Size/Usage/Label or inspect the core state.
func NewTestBufferFromCore(coreBuf *core.Buffer) *Buffer {
	return &Buffer{impl: resource.NewBufferFromCore(coreBuf)}
}

// TestDestroyQueue returns the device's DestroyQueue (testing only).
func (d *Device) TestDestroyQueue() *core.DestroyQueue {
	return d.destroyQueue()
}

// TestResourceCounts returns resource counts from the global hub (testing only).
func (d *Device) TestResourceCounts() map[string]uint64 {
	return core.GetGlobal().Stats()
}

// TestBindGroupReleased returns true if the bind group has been released (testing only).
func (g *BindGroup) TestBindGroupReleased() bool {
	if g.released == nil {
		return false
	}
	return g.released.Load()
}

// SetTestBindGroupLayouts sets the bindGroupLayouts field on a RenderPipeline for testing.
func (p *RenderPipeline) SetTestBindGroupLayouts(layouts []*BindGroupLayout) {
	p.bindGroupLayouts = layouts
	p.bindGroupCount = uint32(len(layouts))
}

// SetTestBindGroupLayouts sets the bindGroupLayouts field on a ComputePipeline for testing.
func (p *ComputePipeline) SetTestBindGroupLayouts(layouts []*BindGroupLayout) {
	p.bindGroupLayouts = layouts
	p.bindGroupCount = uint32(len(layouts))
}

// SetTestEntries sets the entries field on a BindGroupLayout for testing.
func (l *BindGroupLayout) SetTestEntries(entries []gputypes.BindGroupLayoutEntry) {
	l.entries = entries
}

// SetTestLayout sets the layout field on a BindGroup for testing.
func (g *BindGroup) SetTestLayout(layout *BindGroupLayout) {
	g.layout = layout
}

// SetTestStripIndexFormat sets the stripIndexFormat field on a RenderPipeline for testing.
// Pass nil for non-strip topologies, or a pointer to the expected IndexFormat for strip topologies.
func (p *RenderPipeline) SetTestStripIndexFormat(format *gputypes.IndexFormat) {
	p.stripIndexFormat = format
}

// TestMaintainAfterIdle exposes the private maintainAfterIdle for targeted coverage tests.
func (d *Device) TestMaintainAfterIdle() { d.maintainAfterIdle() }

// NewBareDeviceForTest returns a Device with no queue or HAL state (testing only).
// Used to exercise nil-queue guard clauses in maintainAfterIdle.
func NewBareDeviceForTest() *Device { return &Device{} }

// NewTestBuffer returns a Buffer backed by core state only (testing only).
// HAL is nil; sufficient for validation unit tests that read Size/Usage/Label.
func NewTestBuffer(size uint64, usage gputypes.BufferUsage, label string) *Buffer {
	coreBuf := core.NewBuffer(nil, nil, usage, size, label)
	coreBuf.Ref = core.NewResourceRef("Buffer:"+label, nil)
	return NewTestBufferFromCore(coreBuf)
}

// NewTestDeviceWithFeatures returns a Device with core state only (testing only).
func NewTestDeviceWithFeatures(features gputypes.Features) *Device {
	return &Device{
		core: core.NewDevice(nil, nil, features, gputypes.DefaultLimits(), "test-device"),
	}
}

// NewTestCommandEncoderForDevice returns a minimal command encoder for validation tests.
func NewTestCommandEncoderForDevice(device *Device) *CommandEncoder {
	return &CommandEncoder{
		device: device,
		core:   &core.CoreCommandEncoder{},
	}
}

// The HAL wrapping constructors and the HAL device handle are not public since
// v0.35.0 (ADR-070 §15: no internal type in a public signature). Package
// wgpu_test builds objects around HAL doubles through these hooks; the
// wgputest factory uses newDeviceFromHAL directly.

// NewDeviceFromHALForTest wraps HAL device and queue objects in a Device; both
// must be non-nil, as the removed public constructor required.
func NewDeviceFromHALForTest(
	halDevice hal.Device,
	halQueue hal.Queue,
	features gputypes.Features,
	limits gputypes.Limits,
	label string,
) (*Device, error) {
	if halDevice == nil {
		return nil, fmt.Errorf("wgpu: halDevice is nil")
	}
	if halQueue == nil {
		return nil, fmt.Errorf("wgpu: halQueue is nil")
	}
	return newDeviceFromHAL(halDevice, halQueue, features, limits, label), nil
}

// NewSurfaceFromHALForTest wraps a HAL surface in a Surface.
func NewSurfaceFromHALForTest(halSurface hal.Surface, label string) *Surface {
	return &Surface{core: core.NewSurface(halSurface, label)}
}

// NewTextureFromHALForTest wraps a HAL texture in a Texture.
func NewTextureFromHALForTest(halTexture hal.Texture, device *Device, format gputypes.TextureFormat) *Texture {
	return &Texture{hal: halTexture, device: device, format: format}
}

// NewTextureViewFromHALForTest wraps a HAL texture view in a TextureView.
func NewTextureViewFromHALForTest(halView hal.TextureView, device *Device) *TextureView {
	return &TextureView{hal: halView, device: device}
}

// NewSamplerFromHALForTest wraps a HAL sampler in a Sampler.
func NewSamplerFromHALForTest(halSampler hal.Sampler, device *Device) *Sampler {
	return &Sampler{hal: halSampler, device: device}
}

// DeviceHalDeviceForTest returns the HAL device behind d.
func DeviceHalDeviceForTest(d *Device) hal.Device { return d.halDevice() }
