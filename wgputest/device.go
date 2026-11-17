//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgputest

import (
	"errors"
	"fmt"

	"github.com/gogpu/wgpu"
	"github.com/gogpu/wgpu/wgputest/internal/backend"
	"github.com/gogpu/wgpu/wgputest/internal/device"
)

// NewSoftwareDevice returns a device and queue on the Pure Go software
// backend (SPIR-V interpreter + rasterizer), headless. It never uses the
// browser or the Rust backend. The cleanup releases the device, the adapter
// and the instance.
//
// The Pure Go backends build on windows 386/amd64/arm64, linux and darwin
// amd64/arm64, and android arm64. Elsewhere, under -tags rust, or under
// wgpu_nosoftware, the constructor returns ErrUnavailable.
func NewSoftwareDevice(opts ...Option) (*wgpu.Device, *wgpu.Queue, func(), error) {
	return newDevice("software", backend.OpenSoftware, opts)
}

// NewNoopDevice returns a device and queue on the noop backend, headless.
// The cleanup releases the device, the adapter and the instance.
// The noop backend builds on every native platform; under -tags rust the
// constructor returns ErrUnavailable.
func NewNoopDevice(opts ...Option) (*wgpu.Device, *wgpu.Queue, func(), error) {
	return newDevice("noop", backend.OpenNoop, opts)
}

// newDevice collects the options and delegates to the internal device package,
// which opens the backend and wraps it. Only the unavailable sentinel is
// mapped here: it is this package's public value. The option fold lives in the
// internal package (device.Apply), so this facade holds no loop (ADR-070 §15.10).
func newDevice(name string, open backend.Opener, opts []Option) (*wgpu.Device, *wgpu.Queue, func(), error) {
	var cfg config
	device.Apply(opts, &cfg)
	dev, queue, cleanup, err := device.New(device.Config{
		Name: name,
		Open: open,
		Faults: device.Faults{
			CreateCommandEncoder: cfg.faults.CreateCommandEncoder,
			WriteBuffer:          cfg.faults.WriteBuffer,
		},
		Observer: observerFor(cfg.recorder),
	})
	if errors.Is(err, backend.ErrUnavailable) || errors.Is(err, device.ErrNoFactory) {
		return nil, nil, nil, fmt.Errorf("wgputest: %s backend: %w", name, ErrUnavailable)
	}
	return dev, queue, cleanup, err
}
