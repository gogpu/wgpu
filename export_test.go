package wgpu

// SetTestRequiredVertexBuffers sets the requiredVertexBuffers field for testing.
// This method is only available in test builds.
func (p *RenderPipeline) SetTestRequiredVertexBuffers(count uint32) {
	p.requiredVertexBuffers = count
}
