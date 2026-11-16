//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package device

// The contract table's doubles (ADR-070 S11, commit 3). internal/device may not
// import internal/hal/noop or internal/hal/software: a destination test is a
// graph edge, and neither package is covered by a golden row. The create and
// teardown paths therefore run on a local HAL double, the shape the root keeps
// for the noop-driven fixtures it cannot move (device_release_test.go).

import (
	"errors"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/hal"
	"github.com/gogpu/wgpu/internal/queue"
	"github.com/gogpu/wgpu/internal/surface"
)

// errTestNoShaderModules is the double's shader-module answer: the core
// device's indirect-dispatch validation disables itself on it, which is how a
// device over the double comes up with no HAL resources.
var errTestNoShaderModules = errors.New("test double: no shader modules")

// testHALDevice is the local HAL device double. It embeds hal.Device, so only
// the methods the device domain actually calls are implemented; anything else
// panics on the nil interface, which names the exact HAL surface these paths
// need. events records the wait-idle/destroy ordering Release requires.
type testHALDevice struct {
	hal.Device
	events *[]string
}

// CreateShaderModule fails: the only HAL resource core.NewDevice builds
// eagerly is the indirect-dispatch validation pipeline, and it disables itself
// when this call fails.
func (d *testHALDevice) CreateShaderModule(*hal.ShaderModuleDescriptor) (hal.ShaderModule, error) {
	return nil, errTestNoShaderModules
}

// WaitIdle records the drain Release performs before the HAL device is
// destroyed.
func (d *testHALDevice) WaitIdle() error {
	d.record("wait-idle")
	return nil
}

// Destroy records the teardown that must follow WaitIdle.
func (d *testHALDevice) Destroy() {
	d.record("destroy")
}

func (d *testHALDevice) record(event string) {
	if d.events != nil {
		*d.events = append(*d.events, event)
	}
}

// testHALQueue is the local HAL queue double: the teardown path asks only for
// the completion index.
type testHALQueue struct {
	hal.Queue
}

// PollCompleted reports nothing completed, which is true for a device that
// never submitted.
func (q *testHALQueue) PollCompleted() uint64 { return 0 }

// testInstanceHost is the instance seam double: it records that the release
// scan asked it for surfaces and unregistered the device.
type testInstanceHost struct {
	surfacesCalls int
	unregistered  bool
}

// SurfacesForDevice returns no surfaces: a surface list is outside this table.
func (h *testInstanceHost) SurfacesForDevice(core.DeviceView) []*surface.Surface {
	h.surfacesCalls++
	return nil
}

// UnregisterDevice records the end of the release.
func (h *testInstanceHost) UnregisterDevice(core.DeviceView) { h.unregistered = true }

// newTestDevice builds the destination over the doubles: a core device whose
// indirect-dispatch validation disables itself on the shader-module error, a
// queue with no pending-write batch and no encoder pool. events may be nil.
func newTestDevice(events *[]string) *Device {
	coreDevice := core.NewDevice(
		&testHALDevice{events: events}, nil, 0, gputypes.DefaultLimits(), "test-device",
	)
	q := queue.New(&testHALQueue{}, nil)
	d := New(coreDevice, q, nil)
	q.SetDevice(d)
	return d
}
