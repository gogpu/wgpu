//go:build !rust && !(js && wasm)

package wgpu

import (
	"fmt"

	"github.com/gogpu/wgpu/core"
	"github.com/gogpu/wgpu/hal"
)

// QuerySet represents a set of GPU queries (occlusion, timestamp, etc.).
type QuerySet struct {
	core     *core.QuerySet
	device   *Device
	released bool
}

// Type returns the query set type.
func (qs *QuerySet) Type() QueryType {
	if qs == nil || qs.core == nil {
		return QueryTypeOcclusion
	}
	return qs.core.QueryType()
}

// Count returns the number of queries in the set.
func (qs *QuerySet) Count() uint32 {
	if qs == nil || qs.core == nil {
		return 0
	}
	return qs.core.Count()
}

// halQuerySet returns the underlying HAL query set while holding the device
// snatch lock, or nil after the query set or its device has been released.
func (qs *QuerySet) halQuerySet() hal.QuerySet {
	if qs == nil || qs.core == nil || qs.device == nil || qs.device.core == nil || qs.released {
		return nil
	}

	guard := qs.device.core.SnatchLock().Read()
	defer guard.Release()

	return qs.core.Raw(guard)
}

// liveCore returns the core query set, or nil when the query set is nil or
// released, so core validation rejects it. Release defers the core
// destruction, so the core object alone does not show a released set.
func (qs *QuerySet) liveCore() *core.QuerySet {
	if qs == nil || qs.released {
		return nil
	}
	return qs.core
}

// newQuerySetRef returns the reference count of a HAL query set: when its last
// owner drops it, the set is destroyed after the submissions in flight, as a
// sampler or a bind group is. The owners are the application until Release and
// every command buffer that recorded the set's timestamp writes or a resolve
// of it (P1-CORE-021, Rust wgpu-core's QuerySet is held by an Arc).
func newQuerySetRef(d *Device, coreQS *core.QuerySet, label string) *core.ResourceRef {
	return core.NewResourceRef("QuerySet:"+label, func() {
		dq := d.destroyQueue()
		if dq == nil {
			coreQS.Destroy()
			return
		}
		dq.Defer(d.lastSubmissionIndex(), "QuerySet", coreQS.Destroy)
	})
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
	halDevice := qs.device.halDevice()
	if halDevice == nil {
		qs.core.Destroy()
		return
	}

	dq := qs.device.destroyQueue()
	if dq == nil {
		qs.core.Destroy()
		return
	}

	subIdx := qs.device.lastSubmissionIndex()
	coreQS := qs.core
	dq.Defer(subIdx, "QuerySet", func() {
		coreQS.Destroy()
	})
}

// CreateQuerySet creates a query set for occlusion or timestamp queries.
func (d *Device) CreateQuerySet(desc *QuerySetDescriptor) (*QuerySet, error) {
	if d.released.Load() {
		return nil, ErrReleased
	}
	if desc == nil {
		return nil, fmt.Errorf("wgpu: query set descriptor is nil")
	}

	halDevice := d.halDevice()
	if halDevice == nil {
		return nil, ErrReleased
	}

	halDesc := desc.toHAL()
	if err := core.ValidateQuerySetDescriptor(halDesc, d.core.Features); err != nil {
		return nil, err
	}

	halQS, err := halDevice.CreateQuerySet(halDesc)
	if err != nil {
		return nil, fmt.Errorf("wgpu: failed to create query set: %w", err)
	}

	coreQS := core.NewQuerySet(halQS, d.core, halDesc.Type, halDesc.Count, desc.Label)
	coreQS.Ref = newQuerySetRef(d, coreQS, desc.Label)
	return &QuerySet{core: coreQS, device: d}, nil
}
