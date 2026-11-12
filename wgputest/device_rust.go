//go:build rust

package wgputest

import (
	"fmt"

	"github.com/gogpu/wgpu"
)

// NewSoftwareDevice returns ErrUnavailable: the Pure Go backends are not
// compiled with -tags rust.
func NewSoftwareDevice(_ ...Option) (*wgpu.Device, *wgpu.Queue, func(), error) {
	return nil, nil, nil, fmt.Errorf("wgputest: software backend: %w", ErrUnavailable)
}

// NewNoopDevice returns ErrUnavailable: the Pure Go backends are not compiled
// with -tags rust.
func NewNoopDevice(_ ...Option) (*wgpu.Device, *wgpu.Queue, func(), error) {
	return nil, nil, nil, fmt.Errorf("wgputest: noop backend: %w", ErrUnavailable)
}
