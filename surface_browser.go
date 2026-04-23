//go:build js && wasm

package wgpu

// Surface represents a platform rendering surface.
// On browser, this wraps a GPUCanvasContext.
type Surface struct {
	released bool
}

// Configure configures the surface for presentation.
func (s *Surface) Configure(device *Device, config *SurfaceConfiguration) error {
	panic("wgpu: browser backend not yet implemented")
}

// Unconfigure removes the surface configuration.
func (s *Surface) Unconfigure() {
	panic("wgpu: browser backend not yet implemented")
}

// GetCurrentTexture acquires the next texture for rendering.
func (s *Surface) GetCurrentTexture() (*SurfaceTexture, bool, error) {
	panic("wgpu: browser backend not yet implemented")
}

// Present presents a surface texture to the screen.
func (s *Surface) Present(texture *SurfaceTexture) error {
	panic("wgpu: browser backend not yet implemented")
}

// DiscardTexture discards the acquired surface texture without presenting it.
func (s *Surface) DiscardTexture() {
	panic("wgpu: browser backend not yet implemented")
}

// Release releases the surface.
func (s *Surface) Release() {
	if s.released {
		return
	}
	s.released = true
}

// SurfaceTexture is a texture acquired from a surface for rendering.
type SurfaceTexture struct {
	released bool
}

// CreateView creates a texture view of this surface texture.
func (st *SurfaceTexture) CreateView(desc *TextureViewDescriptor) (*TextureView, error) {
	panic("wgpu: browser backend not yet implemented")
}
