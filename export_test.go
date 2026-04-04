package wgpu

import "github.com/gogpu/wgpu/core"

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
