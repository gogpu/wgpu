//go:build js && wasm

package wgpu

// CheckHealth returns nil: the browser exposes no health check. The method
// exists so the public API is identical in every build.
func (d *Device) CheckHealth() error { return nil }
