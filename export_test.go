//go:build !rust && !(js && wasm)

package wgpu

import (
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/encoder"
	"github.com/gogpu/wgpu/internal/hal"
	"github.com/gogpu/wgpu/internal/pipeline"
	"github.com/gogpu/wgpu/internal/resource"
)

// SetTestRenderPipelineRequiredVertexBuffers sets the requiredVertexBuffers
// field of a RenderPipeline for testing. A free function rather than a method
// for the reason BindGroupTestRef is one (ADR-070 §11 grain note): a test-only
// method of a type that moves to an extraction package cannot stay behind,
// while a function over the type can. A zero-value wrapper gets a fresh
// implementation, so the hook works on the zero values the public tests build.
func SetTestRenderPipelineRequiredVertexBuffers(p *RenderPipeline, count uint32) {
	testRenderPipelineImpl(p).SetRequiredVertexBuffers(count)
}

// testRenderPipelineImpl returns the pipeline's implementation, giving a
// zero-value wrapper a fresh one: the pre-move hooks wrote the fields of a
// zero-value struct directly, and the tests build their fixtures that way.
func testRenderPipelineImpl(p *RenderPipeline) *pipeline.RenderPipeline {
	if p.impl == nil {
		p.impl = pipeline.NewRenderPipeline(&pipeline.RenderPipelineInit{})
	}
	return p.impl
}

// RenderPipelineTestRef returns the ResourceRef for a RenderPipeline (testing only).
func RenderPipelineTestRef(p *RenderPipeline) *core.ResourceRef {
	if p == nil || p.impl == nil {
		return nil
	}
	return p.impl.Ref()
}

// ComputePipelineTestRef returns the ResourceRef for a ComputePipeline (testing only).
func ComputePipelineTestRef(p *ComputePipeline) *core.ResourceRef {
	if p == nil || p.impl == nil {
		return nil
	}
	return p.impl.Ref()
}

// BindGroupTestRef returns the ResourceRef for a BindGroup (testing only).
// It is a free function rather than a method: a test-only method of a type that
// moves to an extraction package cannot stay behind, while a function over the
// type can (ADR-070 §11 grain note; the implementation's own white-box test
// carries the same assertion once it moves).
func BindGroupTestRef(g *BindGroup) *core.ResourceRef {
	if g == nil || g.impl == nil {
		return nil
	}
	return g.impl.Ref()
}

// CommandBufferTestTrackedRefs returns the tracked refs of a CommandBuffer
// (testing only). A free function rather than a method for the reason
// BindGroupTestRef is one (ADR-070 §11 grain note): a test-only method of a
// type that moves to an extraction package cannot stay behind, while a
// function over the type can.
func CommandBufferTestTrackedRefs(cb *CommandBuffer) []*core.ResourceRef {
	if cb == nil || cb.impl == nil {
		return nil
	}
	return cb.impl.TrackedRefs()
}

// CommandEncoderTestTrackedRefs returns the tracked refs of a CommandEncoder
// (testing only). A free function for the reason CommandBufferTestTrackedRefs
// is one.
func CommandEncoderTestTrackedRefs(e *CommandEncoder) []*core.ResourceRef {
	if e == nil || e.impl == nil {
		return nil
	}
	return e.impl.TrackedRefs()
}

// CommandBufferTestHALEncoder returns the HAL encoder reference on a
// CommandBuffer (testing only). A free function for the reason
// CommandBufferTestTrackedRefs is one.
func CommandBufferTestHALEncoder(cb *CommandBuffer) interface{} {
	if cb == nil || cb.impl == nil {
		return nil
	}
	return cb.impl.HALEncoder()
}

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

// BindGroupTestReleased returns true if the bind group has been released
// (testing only). A free function for the reason BindGroupTestRef is one.
func BindGroupTestReleased(g *BindGroup) bool {
	if g == nil || g.impl == nil {
		return false
	}
	return g.impl.IsReleased()
}

// testBindGroupLayoutImpl returns the layout's implementation. A zero-value
// wrapper gets a fresh implementation over no entries: the tests build empty
// layouts and only exercise their identity in the binder, and the wrapper has
// no constructor of its own.
func testBindGroupLayoutImpl(l *BindGroupLayout) *resource.BindGroupLayout {
	if l == nil {
		return nil
	}
	if l.impl == nil {
		l.impl = resource.NewBindGroupLayout(nil, nil, nil)
	}
	return l.impl
}

// SetTestRenderPipelineBindGroupLayouts sets the bindGroupLayouts field on a
// RenderPipeline for testing. A free function for the reason BindGroupTestRef is one.
func SetTestRenderPipelineBindGroupLayouts(p *RenderPipeline, layouts []*BindGroupLayout) {
	impls := make([]*resource.BindGroupLayout, len(layouts))
	for i, l := range layouts {
		impls[i] = testBindGroupLayoutImpl(l)
	}
	testRenderPipelineImpl(p).SetBindGroupLayouts(impls)
}

// testComputePipelineImpl returns the pipeline's implementation, giving a
// zero-value wrapper a fresh one for the reason testRenderPipelineImpl does.
func testComputePipelineImpl(p *ComputePipeline) *pipeline.ComputePipeline {
	if p.impl == nil {
		p.impl = pipeline.NewComputePipeline(&pipeline.ComputePipelineInit{})
	}
	return p.impl
}

// SetTestComputePipelineBindGroupLayouts sets the bindGroupLayouts field on a
// ComputePipeline for testing. A free function for the reason BindGroupTestRef is one.
func SetTestComputePipelineBindGroupLayouts(p *ComputePipeline, layouts []*BindGroupLayout) {
	impls := make([]*resource.BindGroupLayout, len(layouts))
	for i, l := range layouts {
		impls[i] = testBindGroupLayoutImpl(l)
	}
	testComputePipelineImpl(p).SetBindGroupLayouts(impls)
}

// SetTestBindGroupLayoutEntries sets the entries field on a BindGroupLayout for
// testing. A free function for the reason BindGroupTestRef is one; the wrapper
// gets a fresh implementation over the entries, so it works on a zero value.
func SetTestBindGroupLayoutEntries(l *BindGroupLayout, entries []gputypes.BindGroupLayoutEntry) {
	l.impl = resource.NewBindGroupLayout(nil, nil, entries)
}

// SetTestBindGroupLayout sets the layout field on a BindGroup for testing.
// A free function for the reason BindGroupTestRef is one; the wrapper gets a
// fresh implementation carrying the layout, so it works on a zero value.
func SetTestBindGroupLayout(g *BindGroup, layout *BindGroupLayout) {
	init := &resource.BindGroupInit{}
	if layout != nil {
		init.Layout = layout.impl
	}
	g.impl = resource.NewBindGroup(init)
}

// SetTestRenderPipelineStripIndexFormat sets the stripIndexFormat field on a
// RenderPipeline for testing. Pass nil for non-strip topologies, or a pointer to
// the expected IndexFormat for strip topologies. A free function for the reason
// BindGroupTestRef is one.
func SetTestRenderPipelineStripIndexFormat(p *RenderPipeline, format *gputypes.IndexFormat) {
	testRenderPipelineImpl(p).SetStripIndexFormat(format)
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
		impl: encoder.NewCommandEncoder(encoder.CommandEncoderInit{
			Core:   &core.CoreCommandEncoder{},
			Device: encoderDeviceHost{d: device},
		}),
		device: device,
	}
}

// NewReleasedTestCommandEncoder returns a command encoder already marked
// released, for the white-box tests of the released paths.
func NewReleasedTestCommandEncoder() *CommandEncoder {
	impl := encoder.NewCommandEncoder(encoder.CommandEncoderInit{Core: &core.CoreCommandEncoder{}})
	impl.SetReleasedForTest(true)
	return &CommandEncoder{impl: impl}
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
	impl := resource.NewTexture(halTexture, device.seamView(), format, nil, nil)
	tex := &Texture{impl: impl}
	impl.Pub = tex
	return tex
}

// NewTextureViewFromHALForTest wraps a HAL texture view in a TextureView.
func NewTextureViewFromHALForTest(halView hal.TextureView, device *Device) *TextureView {
	return &TextureView{impl: resource.NewTextureView(&resource.TextureViewInit{HAL: halView, Device: device.seamView()})}
}

// NewSamplerFromHALForTest wraps a HAL sampler in a Sampler.
func NewSamplerFromHALForTest(halSampler hal.Sampler, device *Device) *Sampler {
	return &Sampler{impl: resource.NewSampler(halSampler, device.seamView(), nil, 0, 0, 0)}
}

// DeviceHalDeviceForTest returns the HAL device behind d.
func DeviceHalDeviceForTest(d *Device) hal.Device { return d.halDevice() }

// SetPassStateForTest seeds the render pass state the ExecuteBundles test
// asserts on: a pipeline is considered set and the binder holds one group at
// slot 0 with its expected layout. The state lives in internal/encoder after
// the S5 Move; the hook is one delegation.
func (p *RenderPassEncoder) SetPassStateForTest(layout *resource.BindGroupLayout, group *resource.BindGroup) {
	p.impl.SetPassStateForTest(layout, group)
}

// PassStateForTest reports the three fields ExecuteBundles resets: the
// pipeline flag, the active group at slot 0 and the layout assigned at slot 0.
func (p *RenderPassEncoder) PassStateForTest() (pipelineSet bool, activeGroup *resource.BindGroup, assignedLayout *resource.BindGroupLayout) {
	return p.impl.PassStateForTest()
}
