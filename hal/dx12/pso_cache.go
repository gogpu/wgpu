// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows && !(js && wasm)

package dx12

import (
	"path/filepath"
	"runtime"
	"unsafe"

	"github.com/gogpu/wgpu/hal/dx12/d3d12"
	"github.com/gogpu/wgpu/internal/pipelinecache"
)

// PSOBlobStore persists driver-compiled PSO blobs keyed by descriptor hash.
type PSOBlobStore struct {
	dir string
}

// createGraphicsPSO creates a graphics PSO, loading/saving cached driver blobs.
func (d *Device) createGraphicsPSO(
	psoDesc *d3d12.D3D12_GRAPHICS_PIPELINE_STATE_DESC,
	cacheKey string,
	cachedBlob []byte,
) (*d3d12.ID3D12PipelineState, error) {
	if len(cachedBlob) > 0 {
		psoDesc.CachedPSO = d3d12.D3D12_CACHED_PIPELINE_STATE{
			CachedBlob:            unsafe.Pointer(&cachedBlob[0]),
			CachedBlobSizeInBytes: uintptr(len(cachedBlob)),
		}
		pso, err := d.raw.CreateGraphicsPipelineState(psoDesc)
		runtime.KeepAlive(cachedBlob)
		psoDesc.CachedPSO = d3d12.D3D12_CACHED_PIPELINE_STATE{}
		if err == nil {
			return pso, nil
		}
		if isInvalidCachedPSOError(err) {
			_ = d.psoCache.Delete(cacheKey)
		} else {
			return nil, err
		}
	}
	pso, err := d.raw.CreateGraphicsPipelineState(psoDesc)
	if err != nil {
		return nil, err
	}
	if d.psoCache != nil && cacheKey != "" {
		if blob, blobErr := pso.GetCachedBlob(); blobErr == nil {
			_ = d.psoCache.Save(cacheKey, blob)
		}
	}
	return pso, nil
}

// createComputePSO creates a compute PSO, loading/saving cached driver blobs.
func (d *Device) createComputePSO(
	psoDesc *d3d12.D3D12_COMPUTE_PIPELINE_STATE_DESC,
	cacheKey string,
	cachedBlob []byte,
) (*d3d12.ID3D12PipelineState, error) {
	if len(cachedBlob) > 0 {
		psoDesc.CachedPSO = d3d12.D3D12_CACHED_PIPELINE_STATE{
			CachedBlob:            unsafe.Pointer(&cachedBlob[0]),
			CachedBlobSizeInBytes: uintptr(len(cachedBlob)),
		}
		pso, err := d.raw.CreateComputePipelineState(psoDesc)
		runtime.KeepAlive(cachedBlob)
		psoDesc.CachedPSO = d3d12.D3D12_CACHED_PIPELINE_STATE{}
		if err == nil {
			return pso, nil
		}
		if isInvalidCachedPSOError(err) {
			_ = d.psoCache.Delete(cacheKey)
		} else {
			return nil, err
		}
	}
	pso, err := d.raw.CreateComputePipelineState(psoDesc)
	if err != nil {
		return nil, err
	}
	if d.psoCache != nil && cacheKey != "" {
		if blob, blobErr := pso.GetCachedBlob(); blobErr == nil {
			_ = d.psoCache.Save(cacheKey, blob)
		}
	}
	return pso, nil
}

func isInvalidCachedPSOError(err error) bool {
	if err == nil {
		return false
	}
	if hr, ok := err.(d3d12.HRESULTError); ok {
		return hr == d3d12.E_INVALIDARG
	}
	return false
}

// NewPSOBlobStore creates a store at UserCacheDir()/gogpu/dx12/<adapterKey>/.
func NewPSOBlobStore(adapterKey string) (*PSOBlobStore, error) {
	dir, err := pipelinecache.UserCachePath("dx12", adapterKey, "")
	if err != nil {
		return nil, err
	}
	return &PSOBlobStore{dir: dir}, nil
}

func (s *PSOBlobStore) blobPath(key string) string {
	return filepath.Join(s.dir, key+".pso")
}

// Load returns a cached PSO blob. Missing files return (nil, nil).
func (s *PSOBlobStore) Load(key string) ([]byte, error) {
	if s == nil || key == "" {
		return nil, nil
	}
	return pipelinecache.LoadBlob(s.blobPath(key))
}

// Save stores a PSO blob atomically.
func (s *PSOBlobStore) Save(key string, blob []byte) error {
	if s == nil || key == "" || len(blob) == 0 {
		return nil
	}
	return pipelinecache.SaveBlob(s.blobPath(key), blob)
}

// Delete removes a stale PSO blob.
func (s *PSOBlobStore) Delete(key string) error {
	if s == nil || key == "" {
		return nil
	}
	return pipelinecache.DeleteBlob(s.blobPath(key))
}
