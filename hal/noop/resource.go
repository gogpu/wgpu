package noop

import (
	"sync/atomic"
	"time"

	"github.com/gogpu/wgpu/hal"
)

// Resource is a placeholder implementation for most HAL resource types.
// It implements the hal.Resource interface with a no-op Destroy method.
type Resource struct{}

// Destroy is a no-op.
func (r *Resource) Destroy() {}

// Buffer implements hal.Buffer with optional data storage.
// If created with MappedAtCreation, it stores the buffer data.
type Buffer struct {
	Resource
	data []byte
}

// Texture implements hal.Texture.
type Texture struct {
	Resource
}

// Surface implements hal.Surface for the noop backend.
type Surface struct {
	Resource
	configured bool
}

// Configure marks the surface as configured.
func (s *Surface) Configure(_ hal.Device, _ *hal.SurfaceConfiguration) error {
	s.configured = true
	return nil
}

// Unconfigure marks the surface as unconfigured.
func (s *Surface) Unconfigure(_ hal.Device) {
	s.configured = false
}

// AcquireTexture returns a placeholder surface texture.
// The fence parameter is ignored.
func (s *Surface) AcquireTexture(_ hal.Fence) (*hal.AcquiredSurfaceTexture, error) {
	return &hal.AcquiredSurfaceTexture{
		Texture:    &SurfaceTexture{},
		Suboptimal: false,
	}, nil
}

// DiscardTexture is a no-op.
func (s *Surface) DiscardTexture(_ hal.SurfaceTexture) {}

// SurfaceTexture implements hal.SurfaceTexture.
type SurfaceTexture struct {
	Texture
}

// Fence implements hal.Fence with an atomic counter for synchronization.
type Fence struct {
	Resource
	value atomic.Uint64
}

// Wait simulates waiting for the fence to reach a value.
// Returns true if the fence has reached the value, false otherwise.
// The timeout parameter is ignored in the noop implementation.
func (f *Fence) Wait(value uint64, _ time.Duration) bool {
	return f.value.Load() >= value
}

// Signal sets the fence value.
func (f *Fence) Signal(value uint64) {
	f.value.Store(value)
}

// GetValue returns the current fence value.
func (f *Fence) GetValue() uint64 {
	return f.value.Load()
}
