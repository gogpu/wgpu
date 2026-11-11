//go:build js && wasm

package wgputest

import (
	"fmt"

	"github.com/gogpu/wgpu"
)

// NewSoftwareDevice returns ErrUnavailable: the Pure Go backends do not build
// on js/wasm (the browser's WebGPU is used there).
func NewSoftwareDevice(_ ...Option) (*wgpu.Device, *wgpu.Queue, func(), error) {
	return nil, nil, nil, fmt.Errorf("wgputest: software backend: %w", ErrUnavailable)
}

// NewNoopDevice returns ErrUnavailable: the Pure Go backends do not build on
// js/wasm (the browser's WebGPU is used there).
func NewNoopDevice(_ ...Option) (*wgpu.Device, *wgpu.Queue, func(), error) {
	return nil, nil, nil, fmt.Errorf("wgputest: noop backend: %w", ErrUnavailable)
}
