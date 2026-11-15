//go:build !rust && !(js && wasm)

package wgpu

import (
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/hal"
)

// deviceView adapts the root Device to core.DeviceView, the shared seam a
// resource child holds (ADR-070 §15.3).
//
// Why an adapter and not the Device itself: the seam's Poll takes core's
// PollType and the public Device.Poll takes the root PollType, and Go forbids
// two methods of one type from sharing a name even with different parameter
// types. The golden freezes `method (*Device) Poll(pollType PollType) bool`,
// and root PollType stays a root definition with a conversion (the way A
// treats QueryType), so Device cannot be the seam. The seam's other five
// methods would also have to be public methods on Device naming internal
// types, which §15.1 forbids.
//
// The method bodies are the compile check §15.6 places at the call site: each
// one reads a Device field or calls a Device method directly, so the adapter
// cannot drift from the host. The receiver carries no logic, no conditional
// and no copy.
//
// The adapter disappears at S11: once the device type itself moves to an
// internal package, that type owns Poll(core.PollType) and needs no bridge.
type deviceView struct{ d *Device }

// Core returns the core device state the child records against.
func (v *deviceView) Core() *core.Device { return v.d.core }

// IsReleased reports whether the host has been released.
func (v *deviceView) IsReleased() bool { return v.d.released.Load() }

// HALDevice returns the HAL device for resource creation.
func (v *deviceView) HALDevice() hal.Device { return v.d.halDevice() }

// DestroyQueue returns the deferred-destroy queue, or nil.
func (v *deviceView) DestroyQueue() *core.DestroyQueue { return v.d.destroyQueue() }

// LastSubmissionIndex returns the latest known submission index.
func (v *deviceView) LastSubmissionIndex() uint64 { return v.d.lastSubmissionIndex() }

// Poll drives the device's pending-map triage loop; the root PollType and the
// seam's core.PollType share the same underlying values.
func (v *deviceView) Poll(t core.PollType) bool { return v.d.Poll(PollType(t)) }

// seamView returns the device's cached core.DeviceView. The adapter is
// created on first use and stored once: the result is a pointer, so a child
// receives it without a new allocation per resource. The atomic swap keeps
// the lazy creation race-free for the fixtures that build a Device literal
// instead of going through a constructor; a losing racer simply returns the
// winner. A nil device is a nil interface, never a typed nil (§15.5).
func (d *Device) seamView() core.DeviceView {
	if d == nil {
		return nil
	}
	if v := d.view.Load(); v != nil {
		return v
	}
	v := &deviceView{d: d}
	if d.view.CompareAndSwap(nil, v) {
		return v
	}
	return d.view.Load()
}
