// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows && !(js && wasm)

package d3d12

import (
	"syscall"
	"unsafe"
)

// GetCachedBlob returns the driver-compiled PSO blob for disk caching.
// The blob can be passed back via D3D12_CACHED_PIPELINE_STATE on next launch.
func (p *ID3D12PipelineState) GetCachedBlob() ([]byte, error) {
	var blob *ID3DBlob
	ret, _, _ := syscall.Syscall(
		p.vtbl.GetCachedBlob,
		2,
		uintptr(unsafe.Pointer(p)),
		uintptr(unsafe.Pointer(&blob)),
		0,
	)
	if ret != 0 {
		return nil, HRESULTError(ret)
	}
	if blob == nil {
		return nil, nil
	}
	defer blob.Release()

	ptr := blob.GetBufferPointer()
	size := blob.GetBufferSize()
	if ptr == nil || size == 0 {
		return nil, nil
	}
	data := make([]byte, size)
	copy(data, unsafe.Slice((*byte)(ptr), size))
	return data, nil
}
