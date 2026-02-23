package vulkan

import (
	"fmt"

	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

// QuerySet implements hal.QuerySet for Vulkan.
type QuerySet struct {
	pool      vk.QueryPool
	device    *Device
	queryType hal.QueryType
	count     uint32
}

// Destroy releases the Vulkan query pool.
func (q *QuerySet) Destroy() {
	if q.pool != 0 && q.device != nil {
		q.device.cmds.DestroyQueryPool(q.device.handle, q.pool, nil)
		q.pool = 0
	}
}

// CreateQuerySet creates a Vulkan query pool.
func (d *Device) CreateQuerySet(desc *hal.QuerySetDescriptor) (hal.QuerySet, error) {
	if desc == nil {
		return nil, fmt.Errorf("vulkan: query set descriptor is nil")
	}

	if desc.Count == 0 {
		return nil, fmt.Errorf("vulkan: query set count must be > 0")
	}

	var vkQueryType vk.QueryType
	switch desc.Type {
	case hal.QueryTypeTimestamp:
		vkQueryType = vk.QueryTypeTimestamp
	case hal.QueryTypeOcclusion:
		vkQueryType = vk.QueryTypeOcclusion
	default:
		return nil, fmt.Errorf("vulkan: unsupported query type: %d", desc.Type)
	}

	createInfo := vk.QueryPoolCreateInfo{
		SType:      vk.StructureTypeQueryPoolCreateInfo,
		QueryType:  vkQueryType,
		QueryCount: desc.Count,
	}

	var pool vk.QueryPool
	result := d.cmds.CreateQueryPool(d.handle, &createInfo, nil, &pool)
	if result != vk.Success {
		return nil, fmt.Errorf("vulkan: vkCreateQueryPool failed: %d", result)
	}

	// Reset the query pool so it can be used immediately.
	d.cmds.ResetQueryPool(d.handle, pool, 0, desc.Count)

	qs := &QuerySet{
		pool:      pool,
		device:    d,
		queryType: desc.Type,
		count:     desc.Count,
	}
	if desc.Label != "" {
		d.setObjectName(vk.ObjectTypeQueryPool, uint64(pool), desc.Label)
	} else {
		d.setObjectName(vk.ObjectTypeQueryPool, uint64(pool), "QueryPool")
	}
	return qs, nil
}

// DestroyQuerySet destroys a Vulkan query set.
func (d *Device) DestroyQuerySet(querySet hal.QuerySet) {
	if qs, ok := querySet.(*QuerySet); ok {
		qs.Destroy()
	}
}
