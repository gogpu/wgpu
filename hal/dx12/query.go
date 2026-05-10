// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows && !(js && wasm)

package dx12

import (
	"fmt"

	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/dx12/d3d12"
)

// QuerySet implements hal.QuerySet for DirectX 12.
// Maps to ID3D12QueryHeap + D3D12_QUERY_TYPE.
// Rust wgpu-hal reference: dx12/mod.rs QuerySet struct.
type QuerySet struct {
	raw   *d3d12.ID3D12QueryHeap
	rawTy d3d12.D3D12_QUERY_TYPE
	count uint32
}

// Destroy releases the DX12 query heap.
func (q *QuerySet) Destroy() {
	if q.raw != nil {
		q.raw.Release()
		q.raw = nil
	}
}

// CreateQuerySet creates a DX12 query heap.
// Maps hal.QueryType to the corresponding D3D12 heap and query types.
// Rust wgpu-hal reference: dx12/device.rs create_query_set.
func (d *Device) CreateQuerySet(desc *hal.QuerySetDescriptor) (hal.QuerySet, error) {
	if desc == nil {
		return nil, fmt.Errorf("dx12: query set descriptor is nil")
	}
	if desc.Count == 0 {
		return nil, fmt.Errorf("dx12: query set count is 0")
	}

	var heapTy d3d12.D3D12_QUERY_HEAP_TYPE
	var rawTy d3d12.D3D12_QUERY_TYPE
	switch desc.Type {
	case hal.QueryTypeTimestamp:
		heapTy = d3d12.D3D12_QUERY_HEAP_TYPE_TIMESTAMP
		rawTy = d3d12.D3D12_QUERY_TYPE_TIMESTAMP
	case hal.QueryTypeOcclusion:
		heapTy = d3d12.D3D12_QUERY_HEAP_TYPE_OCCLUSION
		rawTy = d3d12.D3D12_QUERY_TYPE_BINARY_OCCLUSION
	default:
		return nil, fmt.Errorf("dx12: unsupported query type: %d", desc.Type)
	}

	heapDesc := &d3d12.D3D12_QUERY_HEAP_DESC{
		Type:     heapTy,
		Count:    desc.Count,
		NodeMask: 0,
	}

	heap, err := d.raw.CreateQueryHeap(heapDesc)
	if err != nil {
		return nil, fmt.Errorf("dx12: CreateQueryHeap failed: %w", err)
	}

	return &QuerySet{
		raw:   heap,
		rawTy: rawTy,
		count: desc.Count,
	}, nil
}

// DestroyQuerySet destroys a DX12 query set.
func (d *Device) DestroyQuerySet(querySet hal.QuerySet) {
	if qs, ok := querySet.(*QuerySet); ok && qs != nil {
		qs.Destroy()
	}
}
