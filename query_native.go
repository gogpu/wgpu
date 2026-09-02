//go:build !rust && !(js && wasm)

package wgpu

import (
	"fmt"

	"github.com/gogpu/wgpu/core"
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

// Release destroys the query set. Destruction is deferred until the GPU
// completes any submission that may reference this query set.
func (qs *QuerySet) Release() {
	if qs == nil || qs.released {
		return
	}
	qs.released = true
	if qs.core == nil {
		return
	}
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
	return &QuerySet{core: coreQS, device: d}, nil
}
