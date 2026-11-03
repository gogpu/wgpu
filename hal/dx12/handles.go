// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows && !(js && wasm)

package dx12

import (
	"fmt"

	"github.com/gogpu/wgpu/internal/handletable"
)

// resourceHandles maps the NativeHandle values of a device's buffers,
// texture views and samplers back to their Go structs.
//
// Bind group creation needs the Go struct (GPU virtual address, size, the
// staging descriptor handles), not a native D3D12 object, so these resources
// cannot hand out a COM pointer as their handle; and handing out the struct's
// own address is invalid Go (see package handletable). Each resource is
// registered when created and removed when destroyed. Textures and
// acceleration structures keep their ID3D12Resource pointer as the handle:
// that is C memory, not a Go object.
type resourceHandles struct {
	buffers  handletable.Table[Buffer]
	views    handletable.Table[TextureView]
	samplers handletable.Table[Sampler]
}

// buffer resolves a gputypes.BufferBinding handle for the given binding.
func (h *resourceHandles) buffer(binding uint32, handle uintptr) (*Buffer, error) {
	if b, ok := h.buffers.Get(handle); ok {
		return b, nil
	}
	return nil, fmt.Errorf("dx12: binding %d: unknown buffer handle %#x", binding, handle)
}

// textureView resolves a gputypes.TextureViewBinding handle for the given binding.
func (h *resourceHandles) textureView(binding uint32, handle uintptr) (*TextureView, error) {
	if v, ok := h.views.Get(handle); ok {
		return v, nil
	}
	return nil, fmt.Errorf("dx12: binding %d: unknown texture view handle %#x", binding, handle)
}

// sampler resolves a gputypes.SamplerBinding handle for the given binding.
func (h *resourceHandles) sampler(binding uint32, handle uintptr) (*Sampler, error) {
	if s, ok := h.samplers.Get(handle); ok {
		return s, nil
	}
	return nil, fmt.Errorf("dx12: binding %d: unknown sampler handle %#x", binding, handle)
}
