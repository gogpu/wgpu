//go:build windows

package dx12

import "github.com/gogpu/wgpu/hal"

// init registers the DX12 backend with the HAL registry.
func init() {
	hal.RegisterBackend(Backend{})
}
