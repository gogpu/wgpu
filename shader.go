package wgpu

import "github.com/gogpu/wgpu/hal"

// ShaderModule represents a compiled shader module.
type ShaderModule struct {
	hal      hal.ShaderModule
	device   *Device
	released bool
}

// Release destroys the shader module. Destruction is deferred until the GPU
// completes any submission that may reference this shader module.
func (m *ShaderModule) Release() {
	if m.released {
		return
	}
	m.released = true

	halDevice := m.device.halDevice()
	if halDevice == nil {
		return
	}

	dq := m.device.destroyQueue()
	if dq == nil {
		halDevice.DestroyShaderModule(m.hal)
		return
	}

	subIdx := m.device.lastSubmissionIndex()
	halModule := m.hal
	dq.Defer(subIdx, "ShaderModule", func() {
		halDevice.DestroyShaderModule(halModule)
	})
}
