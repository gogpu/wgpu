//go:build js && wasm

package wgpu

// DeviceDescriptor configures device creation.
type DeviceDescriptor struct {
	Label            string
	RequiredFeatures Features
	RequiredLimits   Limits
}

// Adapter represents a physical GPU.
type Adapter struct {
	info     AdapterInfo
	features Features
	limits   Limits
	released bool
}

// Info returns adapter metadata.
func (a *Adapter) Info() AdapterInfo { return a.info }

// Features returns supported features.
func (a *Adapter) Features() Features { return a.features }

// Limits returns the adapter's resource limits.
func (a *Adapter) Limits() Limits { return a.limits }

// RequestDevice creates a logical device from this adapter.
func (a *Adapter) RequestDevice(desc *DeviceDescriptor) (*Device, error) {
	panic("wgpu: browser backend not yet implemented")
}

// SurfaceCapabilities describes what a surface supports on this adapter.
type SurfaceCapabilities struct {
	Formats      []TextureFormat
	PresentModes []PresentMode
	AlphaModes   []CompositeAlphaMode
}

// GetSurfaceCapabilities returns the capabilities of a surface for this adapter.
func (a *Adapter) GetSurfaceCapabilities(surface *Surface) *SurfaceCapabilities {
	panic("wgpu: browser backend not yet implemented")
}

// Release releases the adapter.
func (a *Adapter) Release() {
	if a.released {
		return
	}
	a.released = true
}
