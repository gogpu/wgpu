//go:build !rust && !(js && wasm)

package wgputest

import (
	"errors"
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// openedBackend is a private HAL backend instance opened for one test device.
// The cleanup destroys the adapter and the instance; the wgpu device is
// released by the wgpu cleanup before it runs.
type openedBackend struct {
	device   hal.Device
	queue    hal.Queue
	features gputypes.Features
	limits   gputypes.Limits
	cleanup  func()
}

// opener opens a private HAL backend instance.
type opener func() (openedBackend, error)

// openBackend creates a private instance of the backend and opens its first
// adapter.
func openBackend(backend hal.Backend) (openedBackend, error) {
	instance, err := backend.CreateInstance(&hal.InstanceDescriptor{})
	if err != nil {
		return openedBackend{}, fmt.Errorf("create instance: %w", err)
	}
	adapters := instance.EnumerateAdapters(nil)
	if len(adapters) == 0 {
		instance.Destroy()
		return openedBackend{}, errors.New("no adapter")
	}
	exposed := &adapters[0]
	open, err := exposed.Adapter.Open(0, exposed.Capabilities.Limits)
	if err != nil {
		exposed.Adapter.Destroy()
		instance.Destroy()
		return openedBackend{}, fmt.Errorf("open adapter: %w", err)
	}
	return openedBackend{
		device:   open.Device,
		queue:    open.Queue,
		features: exposed.Features,
		limits:   exposed.Capabilities.Limits,
		cleanup: func() {
			exposed.Adapter.Destroy()
			instance.Destroy()
		},
	}, nil
}
