//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package backend

import (
	"errors"
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/hal"
)

// ErrUnavailable reports that the backend an opener needs is not in this
// build. The public wgputest package maps it to its own ErrUnavailable, so a
// caller matches the public sentinel.
var ErrUnavailable = errors.New("the backend this constructor needs is not in this build")

// Opened is a private HAL backend instance opened for one test device.
// Cleanup destroys the adapter and the instance; the wgpu device is released
// by the wgpu cleanup before it runs.
type Opened struct {
	Device   hal.Device
	Queue    hal.Queue
	Features gputypes.Features
	Limits   gputypes.Limits
	Cleanup  func()
}

// Opener opens a private HAL backend instance.
type Opener func() (Opened, error)

// Open creates a private instance of the backend and opens its first adapter.
func Open(b hal.Backend) (Opened, error) {
	instance, err := b.CreateInstance(&hal.InstanceDescriptor{})
	if err != nil {
		return Opened{}, fmt.Errorf("create instance: %w", err)
	}
	adapters := instance.EnumerateAdapters(nil)
	if len(adapters) == 0 {
		instance.Destroy()
		return Opened{}, errors.New("no adapter")
	}
	exposed := &adapters[0]
	open, err := exposed.Adapter.Open(exposed.Features, exposed.Capabilities.Limits)
	if err != nil {
		exposed.Adapter.Destroy()
		instance.Destroy()
		return Opened{}, fmt.Errorf("open adapter: %w", err)
	}
	return Opened{
		Device:   open.Device,
		Queue:    open.Queue,
		Features: exposed.Features,
		Limits:   exposed.Capabilities.Limits,
		Cleanup: func() {
			exposed.Adapter.Destroy()
			instance.Destroy()
		},
	}, nil
}
