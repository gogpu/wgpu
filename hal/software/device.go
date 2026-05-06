//go:build !(js && wasm)

package software

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"time"
	"unsafe"

	"github.com/gogpu/gputypes"
	naga "github.com/gogpu/naga"
	"github.com/gogpu/wgpu/hal"
)

// ErrComputeRequiresSPIRV indicates that a compute pipeline requires a shader
// module with SPIR-V bytecode. WGSL shaders must be compilable to SPIR-V via
// naga for the SPIR-V interpreter to execute them.
var ErrComputeRequiresSPIRV = errors.New("software: compute pipeline requires SPIR-V shader (WGSL compilation may have failed)")

// Device implements hal.Device for the software backend.
// It maintains a resource registry for resolving handle-based bind group entries
// back to typed software resources.
type Device struct {
	mu           sync.RWMutex
	textureViews map[uintptr]*TextureView // handle -> TextureView
	buffers      map[uintptr]*Buffer      // handle -> Buffer
}

// CreateBuffer creates a software buffer with real data storage.
func (d *Device) CreateBuffer(desc *hal.BufferDescriptor) (hal.Buffer, error) {
	if desc == nil {
		return nil, fmt.Errorf("BUG: buffer descriptor is nil in Software.CreateBuffer — core validation gap")
	}
	id := nextResourceID.Add(1)
	buf := &Buffer{
		id:    id,
		data:  make([]byte, desc.Size),
		size:  desc.Size,
		usage: desc.Usage,
	}
	d.registerBuffer(buf)
	return buf, nil
}

// DestroyBuffer is a no-op (Go GC handles cleanup).
func (d *Device) DestroyBuffer(_ hal.Buffer) {}

// MapBuffer returns a pointer into the buffer's backing Go slice.
// Software buffers are always host-visible and coherent.
func (d *Device) MapBuffer(buffer hal.Buffer, offset, size uint64) (hal.BufferMapping, error) {
	buf, ok := buffer.(*Buffer)
	if !ok || buf == nil || buf.data == nil {
		return hal.BufferMapping{}, hal.ErrInvalidMapRange
	}
	if offset+size > buf.size {
		return hal.BufferMapping{}, hal.ErrInvalidMapRange
	}
	// Pointer arithmetic into a Go slice is safe as long as the slice is
	// kept alive by the caller (core retains the Buffer via ResourceRef).
	return hal.BufferMapping{
		Ptr:        unsafe.Pointer(&buf.data[offset]),
		IsCoherent: true,
	}, nil
}

// UnmapBuffer is a no-op — software buffers are persistently host-visible.
func (d *Device) UnmapBuffer(_ hal.Buffer) error { return nil }

// CreateTexture creates a software texture with real pixel storage.
func (d *Device) CreateTexture(desc *hal.TextureDescriptor) (hal.Texture, error) {
	if desc == nil {
		return nil, fmt.Errorf("BUG: texture descriptor is nil in Software.CreateTexture — core validation gap")
	}
	// Calculate total size needed for texture data
	// Simple calculation: width * height * depth * bytesPerPixel
	// Assuming 4 bytes per pixel (RGBA8) for now
	bytesPerPixel := uint64(4)
	totalSize := uint64(desc.Size.Width) * uint64(desc.Size.Height) * uint64(desc.Size.DepthOrArrayLayers) * bytesPerPixel

	return &Texture{
		id:            nextResourceID.Add(1),
		data:          make([]byte, totalSize),
		width:         desc.Size.Width,
		height:        desc.Size.Height,
		depth:         desc.Size.DepthOrArrayLayers,
		format:        desc.Format,
		usage:         desc.Usage,
		mipLevelCount: desc.MipLevelCount,
		sampleCount:   desc.SampleCount,
	}, nil
}

// DestroyTexture is a no-op (Go GC handles cleanup).
func (d *Device) DestroyTexture(_ hal.Texture) {}

// CreateTextureView creates a software texture view.
func (d *Device) CreateTextureView(texture hal.Texture, _ *hal.TextureViewDescriptor) (hal.TextureView, error) {
	// Views in software backend just reference the original texture
	if tex, ok := texture.(*Texture); ok {
		view := &TextureView{
			id:      nextResourceID.Add(1),
			texture: tex,
		}
		d.registerTextureView(view)
		return view, nil
	}
	// Also handle SurfaceTexture (embeds Texture)
	if st, ok := texture.(*SurfaceTexture); ok {
		view := &TextureView{
			id:      nextResourceID.Add(1),
			texture: &st.Texture,
		}
		d.registerTextureView(view)
		return view, nil
	}
	return &Resource{}, nil
}

// DestroyTextureView is a no-op.
func (d *Device) DestroyTextureView(_ hal.TextureView) {}

// CreateSampler creates a software sampler.
func (d *Device) CreateSampler(_ *hal.SamplerDescriptor) (hal.Sampler, error) {
	return &Resource{}, nil
}

// DestroySampler is a no-op.
func (d *Device) DestroySampler(_ hal.Sampler) {}

// CreateBindGroupLayout creates a software bind group layout.
func (d *Device) CreateBindGroupLayout(_ *hal.BindGroupLayoutDescriptor) (hal.BindGroupLayout, error) {
	return &Resource{}, nil
}

// DestroyBindGroupLayout is a no-op.
func (d *Device) DestroyBindGroupLayout(_ hal.BindGroupLayout) {}

// CreateBindGroup creates a software bind group.
// It resolves handle-based entries to typed software resources using the device registry.
func (d *Device) CreateBindGroup(desc *hal.BindGroupDescriptor) (hal.BindGroup, error) {
	bg := &BindGroup{
		desc:         desc,
		textureViews: make(map[uint32]*TextureView),
		buffers:      make(map[uint32]*Buffer),
	}
	if desc != nil {
		for _, entry := range desc.Entries {
			switch res := entry.Resource.(type) {
			case gputypes.TextureViewBinding:
				if view := d.lookupTextureView(res.TextureView); view != nil {
					bg.textureViews[entry.Binding] = view
				}
			case gputypes.BufferBinding:
				if buf := d.lookupBuffer(res.Buffer); buf != nil {
					bg.buffers[entry.Binding] = buf
				}
			}
		}
	}
	return bg, nil
}

// DestroyBindGroup is a no-op.
func (d *Device) DestroyBindGroup(_ hal.BindGroup) {}

// CreatePipelineLayout creates a software pipeline layout.
func (d *Device) CreatePipelineLayout(_ *hal.PipelineLayoutDescriptor) (hal.PipelineLayout, error) {
	return &Resource{}, nil
}

// DestroyPipelineLayout is a no-op.
func (d *Device) DestroyPipelineLayout(_ hal.PipelineLayout) {}

// CreateShaderModule creates a software shader module.
// If the source is WGSL, it compiles to SPIR-V via naga for interpretation.
// If SPIR-V is provided directly, it is stored as-is.
// Compilation failure is non-fatal: the module falls back to the existing
// callback-based shader path (fullscreen blit, vertex buffer draw).
func (d *Device) CreateShaderModule(desc *hal.ShaderModuleDescriptor) (hal.ShaderModule, error) {
	sm := &ShaderModule{desc: desc}

	switch {
	case len(desc.Source.SPIRV) > 0:
		sm.spirv = desc.Source.SPIRV
	case desc.Source.WGSL != "":
		spirvBytes, err := naga.Compile(desc.Source.WGSL)
		if err == nil && len(spirvBytes)%4 == 0 {
			sm.spirv = make([]uint32, len(spirvBytes)/4)
			for i := range sm.spirv {
				sm.spirv[i] = binary.LittleEndian.Uint32(spirvBytes[i*4:])
			}
		}
		// Compilation failure is non-fatal — existing draw paths don't need SPIR-V.
	}

	return sm, nil
}

// DestroyShaderModule is a no-op.
func (d *Device) DestroyShaderModule(_ hal.ShaderModule) {}

// CreateRenderPipeline creates a software render pipeline.
func (d *Device) CreateRenderPipeline(desc *hal.RenderPipelineDescriptor) (hal.RenderPipeline, error) {
	return &RenderPipeline{desc: desc}, nil
}

// DestroyRenderPipeline is a no-op.
func (d *Device) DestroyRenderPipeline(_ hal.RenderPipeline) {}

// CreateComputePipeline creates a software compute pipeline backed by the SPIR-V
// interpreter. The shader module must contain SPIR-V bytecode (either provided
// directly or compiled from WGSL via naga).
func (d *Device) CreateComputePipeline(desc *hal.ComputePipelineDescriptor) (hal.ComputePipeline, error) {
	if desc == nil {
		return nil, fmt.Errorf("BUG: compute pipeline descriptor is nil in Software.CreateComputePipeline")
	}
	sm, ok := desc.Compute.Module.(*ShaderModule)
	if !ok || sm == nil {
		return nil, fmt.Errorf("software: compute pipeline requires a software ShaderModule")
	}
	// Verify that SPIR-V is available (ParsedModule will parse on first access).
	if sm.ParsedModule() == nil {
		return nil, ErrComputeRequiresSPIRV
	}
	return &ComputePipeline{
		desc:       desc,
		module:     sm,
		entryPoint: desc.Compute.EntryPoint,
	}, nil
}

// DestroyComputePipeline is a no-op.
func (d *Device) DestroyComputePipeline(_ hal.ComputePipeline) {}

// CreateQuerySet is not supported in the software backend.
func (d *Device) CreateQuerySet(_ *hal.QuerySetDescriptor) (hal.QuerySet, error) {
	return nil, errors.New("software: query sets not supported")
}

// DestroyQuerySet is a no-op for the software device.
func (d *Device) DestroyQuerySet(_ hal.QuerySet) {}

// CreateCommandEncoder creates a software command encoder.
// The encoder holds a device reference so compute passes can resolve bind group
// resources during dispatch.
func (d *Device) CreateCommandEncoder(_ *hal.CommandEncoderDescriptor) (hal.CommandEncoder, error) {
	return &CommandEncoder{device: d}, nil
}

// CreateFence creates a software fence with atomic counter.
func (d *Device) CreateFence() (hal.Fence, error) {
	return &Fence{}, nil
}

// DestroyFence is a no-op.
func (d *Device) DestroyFence(_ hal.Fence) {}

// Wait simulates waiting for a fence value.
// Always returns true immediately (fence reached).
func (d *Device) Wait(fence hal.Fence, value uint64, _ time.Duration) (bool, error) {
	f, ok := fence.(*Fence)
	if !ok {
		return true, nil
	}
	// Check if fence has reached the value
	return f.value.Load() >= value, nil
}

// ResetFence resets a fence to the unsignaled state.
func (d *Device) ResetFence(fence hal.Fence) error {
	f, ok := fence.(*Fence)
	if !ok {
		return nil
	}
	f.value.Store(0)
	return nil
}

// GetFenceStatus returns true if the fence is signaled (non-blocking).
func (d *Device) GetFenceStatus(fence hal.Fence) (bool, error) {
	f, ok := fence.(*Fence)
	if !ok {
		return false, nil
	}
	return f.value.Load() > 0, nil
}

// FreeCommandBuffer is a no-op for the software device.
func (d *Device) FreeCommandBuffer(_ hal.CommandBuffer) {}

// CreateRenderBundleEncoder is not supported in the software backend.
func (d *Device) CreateRenderBundleEncoder(_ *hal.RenderBundleEncoderDescriptor) (hal.RenderBundleEncoder, error) {
	return nil, errors.New("software: render bundles not supported")
}

// DestroyRenderBundle is a no-op for the software device.
func (d *Device) DestroyRenderBundle(_ hal.RenderBundle) {}

// WaitIdle is a no-op for the software device.
func (d *Device) WaitIdle() error { return nil }

// Destroy is a no-op for the software device.
func (d *Device) Destroy() {}

// initRegistry initializes the resource maps if needed.
func (d *Device) initRegistry() {
	if d.textureViews == nil {
		d.textureViews = make(map[uintptr]*TextureView)
	}
	if d.buffers == nil {
		d.buffers = make(map[uintptr]*Buffer)
	}
}

// registerTextureView adds a texture view to the device registry.
func (d *Device) registerTextureView(view *TextureView) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.initRegistry()
	d.textureViews[uintptr(view.id)] = view
}

// registerBuffer adds a buffer to the device registry.
func (d *Device) registerBuffer(buf *Buffer) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.initRegistry()
	d.buffers[uintptr(buf.id)] = buf
}

// lookupTextureView finds a texture view by its handle.
func (d *Device) lookupTextureView(handle uintptr) *TextureView {
	if handle == 0 {
		return nil
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.textureViews == nil {
		return nil
	}
	return d.textureViews[handle]
}

// lookupBuffer finds a buffer by its handle.
func (d *Device) lookupBuffer(handle uintptr) *Buffer {
	if handle == 0 {
		return nil
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.buffers == nil {
		return nil
	}
	return d.buffers[handle]
}
