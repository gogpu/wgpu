//go:build js && wasm

package wgpu

import "fmt"

// NewDeviceFromHAL is not supported in the browser.
// The browser backend uses navigator.gpu (WebGPU JS API), not Go HAL.
func NewDeviceFromHAL(
	_ any,
	_ any,
	_ Features,
	_ Limits,
	_ string,
) (*Device, error) {
	return nil, fmt.Errorf("wgpu: NewDeviceFromHAL not available in browser — use RequestDevice instead")
}

// NewSurfaceFromHAL is not supported in the browser.
func NewSurfaceFromHAL(_ any, _ string) *Surface {
	return nil
}

// NewTextureFromHAL is not supported in the browser.
func NewTextureFromHAL(_ any, _ *Device, _ TextureFormat) *Texture {
	return nil
}

// NewTextureViewFromHAL is not supported in the browser.
func NewTextureViewFromHAL(_ any, _ *Device) *TextureView {
	return nil
}

// NewSamplerFromHAL is not supported in the browser.
func NewSamplerFromHAL(_ any, _ *Device) *Sampler {
	return nil
}
