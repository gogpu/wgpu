//go:build software

package software

import "github.com/gogpu/wgpu/hal"

// init registers the software backend with the HAL registry.
func init() {
	hal.RegisterBackend(API{})
}
