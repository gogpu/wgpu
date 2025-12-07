package noop

import "github.com/gogpu/wgpu/hal"

// init registers the noop backend with the HAL registry.
func init() {
	hal.RegisterBackend(API{})
}
