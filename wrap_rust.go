//go:build rust

package wgpu

import (
	"fmt"

	"github.com/gogpu/wgpu/hal"
)

// NewDeviceFromHAL is not supported with the Rust FFI backend.
// The Rust backend creates devices through wgpu-native, not through Go HAL.
func NewDeviceFromHAL(
	_ hal.Device,
	_ hal.Queue,
	_ Features,
	_ Limits,
	_ string,
) (*Device, error) {
	return nil, fmt.Errorf("wgpu: NewDeviceFromHAL not available with Rust backend — use RequestDevice instead")
}

// NewSurfaceFromHAL is not supported with the Rust FFI backend.
func NewSurfaceFromHAL(_ hal.Surface, _ string) *Surface {
	return nil
}

// NewTextureFromHAL is not supported with the Rust FFI backend.
func NewTextureFromHAL(_ hal.Texture, _ *Device, _ TextureFormat) *Texture {
	return nil
}

// NewTextureViewFromHAL is not supported with the Rust FFI backend.
func NewTextureViewFromHAL(_ hal.TextureView, _ *Device) *TextureView {
	return nil
}

// NewSamplerFromHAL is not supported with the Rust FFI backend.
func NewSamplerFromHAL(_ hal.Sampler, _ *Device) *Sampler {
	return nil
}
