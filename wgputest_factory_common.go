//go:build !rust && !(js && wasm)

package wgpu

import (
	"errors"
	"fmt"

	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/noop"
	"github.com/gogpu/wgpu/internal/wgputestbridge"
)

// newTestDeviceFactory builds devices for package wgputest from a private,
// non-registered HAL backend instance. The device goes through
// newDeviceFromHAL, the same constructor as NewDeviceFromHAL, so the wgpu
// core and its validation are unchanged; the recorder is a HAL decorator
// underneath core. softwareBackend is nil when the build omits software
// (wgpu_nosoftware).
func newTestDeviceFactory(softwareBackend func() hal.Backend) wgputestbridge.Factory {
	return func(opts wgputestbridge.Options) (any, any, func(), error) {
		var (
			backend hal.Backend
			label   string
		)
		switch opts.Backend {
		case wgputestbridge.BackendSoftware:
			if softwareBackend == nil {
				return nil, nil, nil, fmt.Errorf("software backend: %w", wgputestbridge.ErrBackendUnavailable)
			}
			backend, label = softwareBackend(), "wgputest-software"
		case wgputestbridge.BackendNoop:
			backend, label = noop.NewBackend(), "wgputest-noop"
		default:
			return nil, nil, nil, fmt.Errorf("wgputestbridge: unknown backend %d", opts.Backend)
		}

		instance, err := backend.CreateInstance(&hal.InstanceDescriptor{})
		if err != nil {
			return nil, nil, nil, fmt.Errorf("create instance: %w", err)
		}
		adapters := instance.EnumerateAdapters(nil)
		if len(adapters) == 0 {
			instance.Destroy()
			return nil, nil, nil, errors.New("no adapter")
		}
		exposed := &adapters[0]
		open, err := exposed.Adapter.Open(0, exposed.Capabilities.Limits)
		if err != nil {
			exposed.Adapter.Destroy()
			instance.Destroy()
			return nil, nil, nil, fmt.Errorf("open adapter: %w", err)
		}

		hooks := wgputestbridge.NewHooks(opts.Observer, opts.Faults)
		device := newDeviceFromHAL(open.Device, open.Queue, 0, exposed.Capabilities.Limits, label)
		// Recording lives on the public API boundary, so pooled encoders and
		// pending-write flushes of the core never appear in the trace.
		device.test = hooks
		device.queue.test = hooks
		cleanup := func() {
			device.Release()
			exposed.Adapter.Destroy()
			instance.Destroy()
		}
		queue := device.Queue()
		return device, queue, cleanup, nil
	}
}
