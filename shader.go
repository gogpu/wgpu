package wgpu

import "github.com/gogpu/wgpu/hal"

// ShaderModule represents a compiled shader module.
type ShaderModule struct {
	hal      hal.ShaderModule
	device   *Device
	released bool
}

// Release destroys the shader module.
func (m *ShaderModule) Release() {
	if m.released {
		return
	}
	m.released = true
	halDevice := m.device.halDevice()
	if halDevice != nil {
		halDevice.DestroyShaderModule(m.hal)
	}
}
