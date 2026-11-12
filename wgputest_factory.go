//go:build !rust && !(js && wasm)

package wgpu

import (
	"errors"

	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/internal/wgputestbridge"
)

func init() {
	wgputestbridge.Register(newTestDeviceFactory())
}

// newTestDeviceFactory constructs the wgpu Device for package wgputest around
// a private HAL backend instance that wgputest opened. The wgpu core and its
// validation are unchanged; the recorder is attached at the public API.
//
// The root package does not import any backend for this: software stays an
// explicit import until v0.35.0, when it becomes default-registered outside
// the default mask (ADR-083).
func newTestDeviceFactory() wgputestbridge.Factory {
	return func(opts *wgputestbridge.Options) (any, any, func(), error) {
		halDevice, ok := opts.Device.(hal.Device)
		if !ok || halDevice == nil {
			return nil, nil, nil, errors.New("wgputest: no HAL device")
		}
		halQueue, ok := opts.Queue.(hal.Queue)
		if !ok || halQueue == nil {
			return nil, nil, nil, errors.New("wgputest: no HAL queue")
		}
		hooks := wgputestbridge.NewHooks(opts.Observer, opts.Faults)
		device := newDeviceFromHAL(halDevice, halQueue, opts.Features, opts.Limits, opts.Label)
		device.test = hooks
		device.queue.test = hooks
		return device, device.queue, func() { device.Release() }, nil
	}
}
