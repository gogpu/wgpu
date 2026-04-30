//go:build !(js && wasm)

package wgpu

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/core"
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

// TestReleased returns true if the buffer has been released (testing only).
func (b *Buffer) TestReleased() bool {
	if b.released == nil {
		return false
	}
	return b.released.Load()
}

// TestDestroyQueue returns the device's DestroyQueue (testing only).
func (d *Device) TestDestroyQueue() *core.DestroyQueue {
	return d.destroyQueue()
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
