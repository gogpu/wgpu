//go:build !rust && !(js && wasm)

package resource

import (
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/hal"
)

// QuerySet represents a set of GPU queries (occlusion, timestamp, etc.).
type QuerySet struct {
	core     *core.QuerySet
	device   core.DeviceView
	released bool
}

// NewQuerySet builds a query set over core state and the shared device view.
func NewQuerySet(core *core.QuerySet, device core.DeviceView) *QuerySet {
	return &QuerySet{core: core, device: device}
}

// Core returns the core query set, or nil.
func (qs *QuerySet) Core() *core.QuerySet {
	if qs == nil {
		return nil
	}
	return qs.core
}

// Device returns the shared device view the query set was created on.
func (qs *QuerySet) Device() core.DeviceView {
	if qs == nil {
		return nil
	}
	return qs.device
}

// Type returns the query set type.
func (qs *QuerySet) Type() QueryType {
	if qs == nil || qs.core == nil {
		return QueryTypeOcclusion
	}
	return QueryType(qs.core.QueryType())
}

// Count returns the number of queries in the set.
func (qs *QuerySet) Count() uint32 {
	if qs == nil || qs.core == nil {
		return 0
	}
	return qs.core.Count()
}

// HALQuerySet returns the underlying HAL query set while holding the device
// snatch lock, or nil after the query set or its device has been released.
func (qs *QuerySet) HALQuerySet() hal.QuerySet {
	if qs == nil || qs.core == nil || qs.device == nil || qs.device.Core() == nil || qs.released {
		return nil
	}

	guard := qs.device.Core().SnatchLock().Read()
	defer guard.Release()

	return qs.core.Raw(guard)
}

// LiveCore returns the core query set, or nil when the query set is nil or
// released, so core validation rejects it. Release defers the core
// destruction, so the core object alone does not show a released set.
func (qs *QuerySet) LiveCore() *core.QuerySet {
	if qs == nil || qs.released {
		return nil
	}
	return qs.core
}

// Release drops the application's reference to the query set. The HAL query
// set is destroyed once no command buffer that recorded it references it any
// more and the GPU has completed those submissions: a query set released after
// its timestamp writes or a resolve were recorded stays valid for that work
// (P1-CORE-021).
func (qs *QuerySet) Release() {
	if qs == nil || qs.released {
		return
	}
	qs.released = true
	if qs.core == nil {
		return
	}
	if qs.core.Ref != nil {
		qs.core.Ref.Drop()
		return
	}

	// No reference count: destroy after the submissions in flight.
	halDevice := qs.device.HALDevice()
	if halDevice == nil {
		qs.core.Destroy()
		return
	}

	dq := qs.device.DestroyQueue()
	if dq == nil {
		qs.core.Destroy()
		return
	}

	subIdx := qs.device.LastSubmissionIndex()
	coreQS := qs.core
	dq.Defer(subIdx, "QuerySet", func() {
		coreQS.Destroy()
	})
}
