// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin

package metal

import (
	"fmt"
	"runtime"
	"strings"
	"time"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/naga"
	"github.com/gogpu/naga/ir"
	"github.com/gogpu/naga/msl"
	"github.com/gogpu/wgpu/hal"
)

// Vertex buffer indices are assigned from the end of the range and count down.
// This maximizes the gap between uniform/storage buffers and vertex buffers.
const maxVertexBuffers = 31

// unknownError is the default error message when Metal returns a nil NSError.
const unknownError = "unknown error"

// Device implements hal.Device for Metal.
type Device struct {
	raw           ID // id<MTLDevice>
	commandQueue  ID // id<MTLCommandQueue>
	adapter       *Adapter
	eventListener ID     // id<MTLSharedEventListener> — created lazily, reused
	queue         *Queue // back-reference for WaitIdle semaphore draining
}

// newDevice creates a new Device from a Metal device.
func newDevice(adapter *Adapter) (*Device, error) {
	if adapter.raw == 0 {
		return nil, fmt.Errorf("metal: adapter has no device")
	}

	queue := MsgSend(adapter.raw, Sel("newCommandQueue"))
	if queue == 0 {
		return nil, fmt.Errorf("metal: failed to create command queue")
	}

	hal.Logger().Info("metal: device created",
		"name", DeviceName(adapter.raw),
	)

	return &Device{
		raw:          adapter.raw,
		commandQueue: queue,
		adapter:      adapter,
	}, nil
}

// CreateBuffer creates a GPU buffer.
func (d *Device) CreateBuffer(desc *hal.BufferDescriptor) (hal.Buffer, error) {
	if desc == nil {
		return nil, fmt.Errorf("BUG: buffer descriptor is nil in Metal.CreateBuffer — core validation gap")
	}

	var options MTLResourceOptions
	mapRead := desc.Usage&gputypes.BufferUsageMapRead != 0
	mapWrite := desc.Usage&gputypes.BufferUsageMapWrite != 0
	copyDst := desc.Usage&gputypes.BufferUsageCopyDst != 0

	if mapRead || mapWrite || copyDst || desc.MappedAtCreation {
		// CopyDst buffers need CPU-visible storage for Queue.WriteBuffer().
		// MappedAtCreation needs CPU-visible storage for the initial mapping.
		// On Apple Silicon UMA, StorageModeShared is zero-cost (same physical memory).
		// This matches the Vulkan backend which maps CopyDst/MappedAtCreation to host-visible memory.
		options = MTLResourceStorageModeShared
	} else {
		options = MTLResourceStorageModePrivate
	}

	if mapWrite && !mapRead {
		options |= MTLResourceCPUCacheModeWriteCombined
	}

	pool := NewAutoreleasePool()
	defer pool.Drain()

	raw := MsgSend(d.raw, Sel("newBufferWithLength:options:"),
		uintptr(desc.Size), uintptr(options))
	if raw == 0 {
		return nil, fmt.Errorf("metal: failed to create buffer")
	}

	if desc.Label != "" {
		label := NSString(desc.Label)
		_ = MsgSend(raw, Sel("setLabel:"), uintptr(label))
		Release(label)
	}

	return &Buffer{
		raw:     raw,
		size:    desc.Size,
		usage:   desc.Usage,
		options: options,
		device:  d,
	}, nil
}

// MapBuffer returns a CPU-visible pointer into the given Metal buffer.
//
// Buffers created with MTLStorageModeShared or MTLStorageModeManaged expose
// host-visible memory via MTLBuffer.contents(). Private buffers do not and
// return an error; core must route such reads through a staging buffer at
// a higher layer.
//
// Metal Shared memory is always coherent with CPU access, so the driver
// reports IsCoherent=true; Managed storage would need didModifyRange
// for CPU→GPU writes but the Go wgpu path currently uses only Shared
// host-visible buffers.
func (d *Device) MapBuffer(buffer hal.Buffer, offset, size uint64) (hal.BufferMapping, error) {
	buf, ok := buffer.(*Buffer)
	if !ok || buf == nil || buf.raw == 0 {
		return hal.BufferMapping{}, hal.ErrInvalidMapRange
	}
	if offset+size > buf.size {
		return hal.BufferMapping{}, hal.ErrInvalidMapRange
	}
	ptr := buf.Contents()
	if ptr == nil {
		return hal.BufferMapping{}, hal.ErrInvalidMapRange
	}
	return hal.BufferMapping{
		Ptr:        unsafe.Add(ptr, int(offset)),
		IsCoherent: true,
	}, nil
}

// UnmapBuffer is a no-op on Metal because Shared-storage buffers are
// persistently mapped and coherent with the GPU.
func (d *Device) UnmapBuffer(_ hal.Buffer) error {
	return nil
}

// DestroyBuffer destroys a GPU buffer.
func (d *Device) DestroyBuffer(buffer hal.Buffer) {
	mtlBuffer, ok := buffer.(*Buffer)
	if !ok || mtlBuffer == nil {
		return
	}
	if mtlBuffer.raw != 0 {
		Release(mtlBuffer.raw)
		mtlBuffer.raw = 0
	}
	mtlBuffer.device = nil
}

// CreateTexture creates a GPU texture.
func (d *Device) CreateTexture(desc *hal.TextureDescriptor) (hal.Texture, error) {
	if desc == nil {
		return nil, fmt.Errorf("BUG: texture descriptor is nil in Metal.CreateTexture — core validation gap")
	}

	pool := NewAutoreleasePool()
	defer pool.Drain()

	texDesc := MsgSend(ID(GetClass("MTLTextureDescriptor")), Sel("new"))
	if texDesc == 0 {
		return nil, fmt.Errorf("metal: failed to create texture descriptor")
	}
	defer Release(texDesc)

	texType := textureTypeFromDimension(desc.Dimension, desc.SampleCount, desc.Size.DepthOrArrayLayers)
	_ = MsgSend(texDesc, Sel("setTextureType:"), uintptr(texType))

	pixelFormat := d.adapter.mapTextureFormat(desc.Format)
	_ = MsgSend(texDesc, Sel("setPixelFormat:"), uintptr(pixelFormat))

	_ = MsgSend(texDesc, Sel("setWidth:"), uintptr(desc.Size.Width))
	_ = MsgSend(texDesc, Sel("setHeight:"), uintptr(desc.Size.Height))

	depth := desc.Size.DepthOrArrayLayers
	if depth == 0 {
		depth = 1
	}
	_ = MsgSend(texDesc, Sel("setDepth:"), uintptr(depth))

	mipLevels := desc.MipLevelCount
	if mipLevels == 0 {
		mipLevels = 1
	}
	_ = MsgSend(texDesc, Sel("setMipmapLevelCount:"), uintptr(mipLevels))

	sampleCount := desc.SampleCount
	if sampleCount == 0 {
		sampleCount = 1
	}
	_ = MsgSend(texDesc, Sel("setSampleCount:"), uintptr(sampleCount))

	usage := textureUsageToMTL(desc.Usage)
	_ = MsgSend(texDesc, Sel("setUsage:"), uintptr(usage))
	_ = MsgSend(texDesc, Sel("setStorageMode:"), uintptr(MTLStorageModePrivate))

	raw := MsgSend(d.raw, Sel("newTextureWithDescriptor:"), uintptr(texDesc))
	if raw == 0 {
		return nil, fmt.Errorf("metal: failed to create texture")
	}

	if desc.Label != "" {
		label := NSString(desc.Label)
		_ = MsgSend(raw, Sel("setLabel:"), uintptr(label))
		Release(label)
	}

	return &Texture{
		raw:        raw,
		format:     desc.Format,
		width:      desc.Size.Width,
		height:     desc.Size.Height,
		depth:      depth,
		mipLevels:  mipLevels,
		samples:    sampleCount,
		dimension:  desc.Dimension,
		usage:      desc.Usage,
		device:     d,
		isExternal: false,
	}, nil
}

// DestroyTexture destroys a GPU texture.
func (d *Device) DestroyTexture(texture hal.Texture) {
	mtlTexture, ok := texture.(*Texture)
	if !ok || mtlTexture == nil {
		return
	}
	if mtlTexture.raw != 0 && !mtlTexture.isExternal {
		Release(mtlTexture.raw)
		mtlTexture.raw = 0
	}
	mtlTexture.device = nil
}

// CreateTextureView creates a view into a texture.
func (d *Device) CreateTextureView(texture hal.Texture, desc *hal.TextureViewDescriptor) (hal.TextureView, error) {
	var mtlTexture *Texture
	switch t := texture.(type) {
	case *Texture:
		mtlTexture = t
	case *SurfaceTexture:
		if t != nil {
			mtlTexture = t.texture
		}
	}
	if mtlTexture == nil {
		return nil, fmt.Errorf("metal: invalid texture")
	}
	if desc == nil {
		desc = &hal.TextureViewDescriptor{}
	}

	pool := NewAutoreleasePool()
	defer pool.Drain()

	format := desc.Format
	if format == gputypes.TextureFormatUndefined {
		format = mtlTexture.format
	}
	pixelFormat := d.adapter.mapTextureFormat(format)

	baseMip := desc.BaseMipLevel
	mipCount := desc.MipLevelCount
	if mipCount == 0 {
		// 0 means "all remaining mip levels" in WebGPU spec
		mipCount = mtlTexture.mipLevels - baseMip
	}

	baseLayer := desc.BaseArrayLayer
	layerCount := desc.ArrayLayerCount
	if layerCount == 0 {
		// 0 means "all remaining array layers" in WebGPU spec
		layerCount = mtlTexture.depth - baseLayer
		if layerCount == 0 {
			layerCount = 1
		}
	}

	var viewType MTLTextureType
	if desc.Dimension == gputypes.TextureViewDimensionUndefined {
		viewType = textureTypeFromDimension(mtlTexture.dimension, mtlTexture.samples, mtlTexture.depth)
	} else {
		viewType = textureViewDimensionToMTL(desc.Dimension)
	}

	// Metal requires the texture view type to match the source texture's
	// multisample state. A 2DMultisample source cannot have a 2D view.
	// Vulkan handles this implicitly, but Metal is stricter.
	if mtlTexture.samples > 1 && viewType == MTLTextureType2D {
		viewType = MTLTextureType2DMultisample
	}

	// Metal's newTextureViewWithPixelFormat:textureType:levels:slices: expects NSRange structs
	levelRange := NSRange{
		Location: NSUInteger(baseMip),
		Length:   NSUInteger(mipCount),
	}
	sliceRange := NSRange{
		Location: NSUInteger(baseLayer),
		Length:   NSUInteger(layerCount),
	}

	raw := msgSendID(mtlTexture.raw, Sel("newTextureViewWithPixelFormat:textureType:levels:slices:"),
		argUint64(uint64(pixelFormat)),
		argUint64(uint64(viewType)),
		argStruct(levelRange, nsRangeType),
		argStruct(sliceRange, nsRangeType),
	)
	if raw == 0 {
		return nil, fmt.Errorf("metal: failed to create texture view")
	}

	return &TextureView{raw: raw, texture: mtlTexture, device: d}, nil
}

// DestroyTextureView destroys a texture view.
func (d *Device) DestroyTextureView(view hal.TextureView) {
	mtlView, ok := view.(*TextureView)
	if !ok || mtlView == nil {
		return
	}
	if mtlView.raw != 0 {
		Release(mtlView.raw)
		mtlView.raw = 0
	}
	mtlView.device = nil
}

// CreateSampler creates a texture sampler.
func (d *Device) CreateSampler(desc *hal.SamplerDescriptor) (hal.Sampler, error) {
	if desc == nil {
		return nil, fmt.Errorf("BUG: sampler descriptor is nil in Metal.CreateSampler — core validation gap")
	}

	pool := NewAutoreleasePool()
	defer pool.Drain()

	sampDesc := MsgSend(ID(GetClass("MTLSamplerDescriptor")), Sel("new"))
	if sampDesc == 0 {
		return nil, fmt.Errorf("metal: failed to create sampler descriptor")
	}
	defer Release(sampDesc)

	_ = MsgSend(sampDesc, Sel("setMinFilter:"), uintptr(filterModeToMTL(desc.MinFilter)))
	_ = MsgSend(sampDesc, Sel("setMagFilter:"), uintptr(filterModeToMTL(desc.MagFilter)))
	_ = MsgSend(sampDesc, Sel("setMipFilter:"), uintptr(mipmapFilterModeToMTL(desc.MipmapFilter)))
	_ = MsgSend(sampDesc, Sel("setSAddressMode:"), uintptr(addressModeToMTL(desc.AddressModeU)))
	_ = MsgSend(sampDesc, Sel("setTAddressMode:"), uintptr(addressModeToMTL(desc.AddressModeV)))
	_ = MsgSend(sampDesc, Sel("setRAddressMode:"), uintptr(addressModeToMTL(desc.AddressModeW)))

	if desc.Anisotropy > 1 {
		_ = MsgSend(sampDesc, Sel("setMaxAnisotropy:"), uintptr(desc.Anisotropy))
	}

	if desc.Compare != gputypes.CompareFunctionUndefined {
		_ = MsgSend(sampDesc, Sel("setCompareFunction:"), uintptr(compareFunctionToMTL(desc.Compare)))
	}

	raw := MsgSend(d.raw, Sel("newSamplerStateWithDescriptor:"), uintptr(sampDesc))
	if raw == 0 {
		return nil, fmt.Errorf("metal: failed to create sampler state")
	}

	return &Sampler{raw: raw, device: d}, nil
}

// DestroySampler destroys a sampler.
func (d *Device) DestroySampler(sampler hal.Sampler) {
	mtlSampler, ok := sampler.(*Sampler)
	if !ok || mtlSampler == nil {
		return
	}
	if mtlSampler.raw != 0 {
		Release(mtlSampler.raw)
		mtlSampler.raw = 0
	}
	mtlSampler.device = nil
}

// CreateBindGroupLayout creates a bind group layout.
func (d *Device) CreateBindGroupLayout(desc *hal.BindGroupLayoutDescriptor) (hal.BindGroupLayout, error) {
	layout := &BindGroupLayout{entries: desc.Entries, device: d}

	// Count resources by type so PipelineLayout can compute cumulative slot offsets.
	// naga MSL generates sequential [[buffer(N)]], [[texture(M)]], [[sampler(K)]]
	// indices across all bind groups in a pipeline layout.
	for _, entry := range desc.Entries {
		switch {
		case entry.Buffer != nil:
			layout.bufferCount++
		case entry.Texture != nil:
			layout.textureCount++
		case entry.Sampler != nil:
			layout.samplerCount++
		case entry.StorageTexture != nil:
			layout.textureCount++
		}
	}

	return layout, nil
}

// DestroyBindGroupLayout destroys a bind group layout.
func (d *Device) DestroyBindGroupLayout(layout hal.BindGroupLayout) {
	mtlLayout, ok := layout.(*BindGroupLayout)
	if !ok || mtlLayout == nil {
		return
	}
	mtlLayout.device = nil
}

// CreateBindGroup creates a bind group.
func (d *Device) CreateBindGroup(desc *hal.BindGroupDescriptor) (hal.BindGroup, error) {
	return &BindGroup{layout: desc.Layout.(*BindGroupLayout), entries: desc.Entries, device: d}, nil
}

// DestroyBindGroup destroys a bind group.
func (d *Device) DestroyBindGroup(group hal.BindGroup) {
	mtlGroup, ok := group.(*BindGroup)
	if !ok || mtlGroup == nil {
		return
	}
	mtlGroup.device = nil
}

// CreatePipelineLayout creates a pipeline layout.
//
// Computes cumulative per-type slot offsets for each bind group. naga MSL generates
// sequential [[buffer(N)]], [[texture(M)]], [[sampler(K)]] indices across all groups,
// so group i's starting slot = sum of resource counts from groups 0..i-1.
//
// Reference: Rust wgpu-hal metal/device.rs:718-801 (base_resource_indices).
func (d *Device) CreatePipelineLayout(desc *hal.PipelineLayoutDescriptor) (hal.PipelineLayout, error) {
	offsets := make([]GroupSlotOffsets, len(desc.BindGroupLayouts))
	var bufAccum, texAccum, samAccum int
	for i, bglIface := range desc.BindGroupLayouts {
		offsets[i] = GroupSlotOffsets{
			Buffers:  bufAccum,
			Textures: texAccum,
			Samplers: samAccum,
		}
		if bgl, ok := bglIface.(*BindGroupLayout); ok && bgl != nil {
			bufAccum += bgl.bufferCount
			texAccum += bgl.textureCount
			samAccum += bgl.samplerCount
		}
	}
	return &PipelineLayout{layouts: desc.BindGroupLayouts, device: d, groupOffsets: offsets}, nil
}

// DestroyPipelineLayout destroys a pipeline layout.
func (d *Device) DestroyPipelineLayout(layout hal.PipelineLayout) {
	mtlLayout, ok := layout.(*PipelineLayout)
	if !ok || mtlLayout == nil {
		return
	}
	mtlLayout.device = nil
}

// CreateShaderModule creates a shader module.
func (d *Device) CreateShaderModule(desc *hal.ShaderModuleDescriptor) (hal.ShaderModule, error) {
	// If WGSL source is provided, compile to MSL
	if desc.Source.WGSL != "" { //nolint:nestif // WGSL→MSL pipeline is sequential; splitting would scatter coupled logic
		start := time.Now()

		// Parse WGSL to AST
		ast, err := naga.Parse(desc.Source.WGSL)
		if err != nil {
			return nil, fmt.Errorf("metal: failed to parse WGSL: %w", err)
		}

		// Lower AST to IR
		irModule, err := naga.LowerWithSource(ast, desc.Source.WGSL)
		if err != nil {
			return nil, fmt.Errorf("metal: failed to lower WGSL to IR: %w", err)
		}

		// Extract workgroup sizes from entry points for compute shaders
		workgroupSizes := extractWorkgroupSizes(irModule)

		// Compile IR to MSL
		mslSource, _, err := msl.Compile(irModule, msl.DefaultOptions())
		if err != nil {
			return nil, fmt.Errorf("metal: failed to compile to MSL: %w", err)
		}

		hal.Logger().Debug("metal: WGSL→MSL compilation",
			"elapsed", time.Since(start),
			"mslBytes", len(mslSource),
		)

		// Create NSString from MSL source
		mslString := NSString(mslSource)
		defer Release(mslString)

		// Create MTLLibrary from source
		// MTLLibrary* newLibraryWithSource:options:error:
		var errorPtr ID
		library := MsgSend(d.raw, Sel("newLibraryWithSource:options:error:"),
			uintptr(mslString), 0, uintptr(unsafe.Pointer(&errorPtr)))

		if library == 0 {
			errMsg := unknownError
			if errorPtr != 0 {
				if details := formatNSError(errorPtr); details != "" {
					errMsg = details
				}
				// Object is autoreleased
			}
			return nil, fmt.Errorf("metal: failed to compile MSL: %s\nMSL:\n%s", errMsg, mslSource)
		}

		hal.Logger().Info("metal: shader module compiled",
			"entryPoints", len(workgroupSizes),
		)

		return &ShaderModule{
			source:         desc.Source,
			library:        library,
			device:         d,
			workgroupSizes: workgroupSizes,
		}, nil
	}

	// No WGSL source - just store the descriptor for later
	return &ShaderModule{source: desc.Source, device: d}, nil
}

func formatNSError(errObj ID) string {
	if errObj == 0 {
		return ""
	}
	parts := make([]string, 0, 4)
	if desc := GoString(MsgSend(errObj, Sel("localizedDescription"))); desc != "" {
		parts = append(parts, desc)
	}
	if reason := GoString(MsgSend(errObj, Sel("localizedFailureReason"))); reason != "" {
		parts = append(parts, reason)
	}
	if debug := GoString(MsgSend(errObj, Sel("debugDescription"))); debug != "" {
		parts = append(parts, debug)
	}
	if info := MsgSend(errObj, Sel("userInfo")); info != 0 {
		if infoDesc := GoString(MsgSend(info, Sel("description"))); infoDesc != "" {
			parts = append(parts, infoDesc)
		}
	}
	return strings.Join(parts, " | ")
}

// DestroyShaderModule destroys a shader module.
func (d *Device) DestroyShaderModule(module hal.ShaderModule) {
	mtlModule, ok := module.(*ShaderModule)
	if !ok || mtlModule == nil {
		return
	}
	if mtlModule.library != 0 {
		Release(mtlModule.library)
		mtlModule.library = 0
	}
	mtlModule.device = nil
}

// CreateRenderPipeline creates a render pipeline.
func (d *Device) CreateRenderPipeline(desc *hal.RenderPipelineDescriptor) (hal.RenderPipeline, error) {
	pool := NewAutoreleasePool()
	defer pool.Drain()

	// Get shader modules
	vertexModule, ok := desc.Vertex.Module.(*ShaderModule)
	if !ok || vertexModule == nil || vertexModule.library == 0 {
		return nil, fmt.Errorf("metal: invalid vertex shader module")
	}

	var fragmentModule *ShaderModule
	if desc.Fragment != nil {
		fragmentModule, ok = desc.Fragment.Module.(*ShaderModule)
		if !ok || fragmentModule == nil || fragmentModule.library == 0 {
			return nil, fmt.Errorf("metal: invalid fragment shader module")
		}
	}

	// Create pipeline descriptor
	pipelineDesc := MsgSend(ID(GetClass("MTLRenderPipelineDescriptor")), Sel("new"))
	if pipelineDesc == 0 {
		return nil, fmt.Errorf("metal: failed to create pipeline descriptor")
	}
	defer Release(pipelineDesc)

	// Set label if provided
	if desc.Label != "" {
		label := NSString(desc.Label)
		_ = MsgSend(pipelineDesc, Sel("setLabel:"), uintptr(label))
		Release(label)
	}

	// Get vertex function from library
	vertexFuncName := NSString(desc.Vertex.EntryPoint)
	vertexFunc := MsgSend(vertexModule.library, Sel("newFunctionWithName:"), uintptr(vertexFuncName))
	Release(vertexFuncName)
	if vertexFunc == 0 {
		return nil, fmt.Errorf("metal: vertex function '%s' not found", desc.Vertex.EntryPoint)
	}
	defer Release(vertexFunc)

	// Set vertex function
	_ = MsgSend(pipelineDesc, Sel("setVertexFunction:"), uintptr(vertexFunc))

	// Configure vertex descriptor from vertex buffer layouts
	if len(desc.Vertex.Buffers) > 0 {
		if vd, err := d.buildVertexDescriptor(desc.Vertex.Buffers); err != nil {
			return nil, err
		} else if vd != 0 {
			_ = MsgSend(pipelineDesc, Sel("setVertexDescriptor:"), uintptr(vd))
		}
	}

	// Get and set fragment function if present
	if fragmentModule != nil && desc.Fragment != nil {
		fragmentFuncName := NSString(desc.Fragment.EntryPoint)
		fragmentFunc := MsgSend(fragmentModule.library, Sel("newFunctionWithName:"), uintptr(fragmentFuncName))
		Release(fragmentFuncName)
		if fragmentFunc == 0 {
			return nil, fmt.Errorf("metal: fragment function '%s' not found", desc.Fragment.EntryPoint)
		}
		defer Release(fragmentFunc)

		_ = MsgSend(pipelineDesc, Sel("setFragmentFunction:"), uintptr(fragmentFunc))

		// Configure color attachments
		colorAttachments := MsgSend(pipelineDesc, Sel("colorAttachments"))
		for i, target := range desc.Fragment.Targets {
			attachment := MsgSend(colorAttachments, Sel("objectAtIndexedSubscript:"), uintptr(i))
			if attachment == 0 {
				continue
			}

			// Set pixel format
			pixelFormat := d.adapter.mapTextureFormat(target.Format)
			_ = MsgSend(attachment, Sel("setPixelFormat:"), uintptr(pixelFormat))

			// Set write mask
			_ = MsgSend(attachment, Sel("setWriteMask:"), uintptr(target.WriteMask))

			// Configure blending if present
			if target.Blend != nil {
				_ = MsgSend(attachment, Sel("setBlendingEnabled:"), uintptr(1))
				_ = MsgSend(attachment, Sel("setSourceRGBBlendFactor:"), uintptr(blendFactorToMTL(target.Blend.Color.SrcFactor)))
				_ = MsgSend(attachment, Sel("setDestinationRGBBlendFactor:"), uintptr(blendFactorToMTL(target.Blend.Color.DstFactor)))
				_ = MsgSend(attachment, Sel("setRgbBlendOperation:"), uintptr(blendOperationToMTL(target.Blend.Color.Operation)))
				_ = MsgSend(attachment, Sel("setSourceAlphaBlendFactor:"), uintptr(blendFactorToMTL(target.Blend.Alpha.SrcFactor)))
				_ = MsgSend(attachment, Sel("setDestinationAlphaBlendFactor:"), uintptr(blendFactorToMTL(target.Blend.Alpha.DstFactor)))
				_ = MsgSend(attachment, Sel("setAlphaBlendOperation:"), uintptr(blendOperationToMTL(target.Blend.Alpha.Operation)))
			}
		}
	}

	var depthStencilState ID
	var depthBias, depthSlopeScale, depthClamp float32
	if desc.DepthStencil != nil {
		// Set depth attachment format
		_ = MsgSend(pipelineDesc, Sel("setDepthAttachmentPixelFormat:"), uintptr(d.adapter.mapTextureFormat(desc.DepthStencil.Format)))

		// Create depth stencil state
		depthStencilDesc := MsgSend(ID(GetClass("MTLDepthStencilDescriptor")), Sel("new"))
		_ = MsgSend(depthStencilDesc, Sel("setDepthCompareFunction:"), uintptr(compareFunctionToMTL(desc.DepthStencil.DepthCompare)))
		msgSendVoid(depthStencilDesc, Sel("setDepthWriteEnabled:"), argBool(desc.DepthStencil.DepthWriteEnabled))
		depthStencilState = MsgSend(d.raw, Sel("newDepthStencilStateWithDescriptor:"), uintptr(depthStencilDesc))

		// Record bias values to set in render pass
		depthBias = float32(desc.DepthStencil.DepthBias)
		depthSlopeScale = desc.DepthStencil.DepthBiasSlopeScale
		depthClamp = desc.DepthStencil.DepthBiasClamp
	}

	// Set sample count
	sampleCount := desc.Multisample.Count
	if sampleCount == 0 {
		sampleCount = 1
	}
	_ = MsgSend(pipelineDesc, Sel("setSampleCount:"), uintptr(sampleCount))

	// Create pipeline state
	var errorPtr ID
	pipelineState := MsgSend(d.raw, Sel("newRenderPipelineStateWithDescriptor:error:"),
		uintptr(pipelineDesc), uintptr(unsafe.Pointer(&errorPtr)))

	if pipelineState == 0 {
		errMsg := unknownError
		if errorPtr != 0 {
			errDesc := MsgSend(errorPtr, Sel("localizedDescription"))
			if errDesc != 0 {
				errMsg = GoString(errDesc)
			}
			// Object is autoreleased
		}
		return nil, fmt.Errorf("metal: failed to create pipeline state: %s", errMsg)
	}

	hal.Logger().Debug("metal: render pipeline created",
		"label", desc.Label,
		"vertexEntry", desc.Vertex.EntryPoint,
		"sampleCount", sampleCount,
	)

	var pipeLayout *PipelineLayout
	if pl, ok := desc.Layout.(*PipelineLayout); ok {
		pipeLayout = pl
	}
	return &RenderPipeline{
		raw:       pipelineState,
		device:    d,
		layout:    pipeLayout,
		cullMode:  cullModeToMTL(desc.Primitive.CullMode),
		frontFace: frontFaceToMTL(desc.Primitive.FrontFace),

		depthStencil:    depthStencilState,
		depthBias:       depthBias,
		depthSlopeScale: depthSlopeScale,
		depthClamp:      depthClamp,
	}, nil
}

// buildVertexDescriptor creates an MTLVertexDescriptor from WebGPU vertex buffer layouts.
func (d *Device) buildVertexDescriptor(buffers []gputypes.VertexBufferLayout) (ID, error) {
	vertexDesc := MsgSend(ID(GetClass("MTLVertexDescriptor")), Sel("vertexDescriptor"))
	if vertexDesc == 0 {
		return 0, fmt.Errorf("metal: failed to create vertex descriptor")
	}

	attributes := MsgSend(vertexDesc, Sel("attributes"))
	layouts := MsgSend(vertexDesc, Sel("layouts"))

	for slot, buf := range buffers {
		bufIdx := maxVertexBuffers - 1 - slot
		for _, attr := range buf.Attributes {
			attrDesc := MsgSend(attributes, Sel("objectAtIndexedSubscript:"), uintptr(attr.ShaderLocation))
			if attrDesc == 0 {
				continue
			}
			_ = MsgSend(attrDesc, Sel("setFormat:"), uintptr(vertexFormatToMTL(attr.Format)))
			_ = MsgSend(attrDesc, Sel("setOffset:"), uintptr(attr.Offset))
			_ = MsgSend(attrDesc, Sel("setBufferIndex:"), uintptr(bufIdx))
		}

		layoutDesc := MsgSend(layouts, Sel("objectAtIndexedSubscript:"), uintptr(bufIdx))
		if layoutDesc != 0 {
			_ = MsgSend(layoutDesc, Sel("setStride:"), uintptr(buf.ArrayStride))
			_ = MsgSend(layoutDesc, Sel("setStepFunction:"), uintptr(vertexStepModeToMTL(buf.StepMode)))
			_ = MsgSend(layoutDesc, Sel("setStepRate:"), uintptr(1))
		}
	}

	return vertexDesc, nil
}

// DestroyRenderPipeline destroys a render pipeline.
func (d *Device) DestroyRenderPipeline(pipeline hal.RenderPipeline) {
	mtlPipeline, ok := pipeline.(*RenderPipeline)
	if !ok || mtlPipeline == nil {
		return
	}
	if mtlPipeline.raw != 0 {
		Release(mtlPipeline.raw)
		mtlPipeline.raw = 0
	}
	mtlPipeline.device = nil
}

// CreateComputePipeline creates a compute pipeline.
func (d *Device) CreateComputePipeline(desc *hal.ComputePipelineDescriptor) (hal.ComputePipeline, error) {
	pool := NewAutoreleasePool()
	defer pool.Drain()

	// Get shader module
	computeModule, ok := desc.Compute.Module.(*ShaderModule)
	if !ok || computeModule == nil || computeModule.library == 0 {
		return nil, fmt.Errorf("metal: invalid compute shader module")
	}

	// Get compute function from library
	funcName := NSString(desc.Compute.EntryPoint)
	computeFunc := MsgSend(computeModule.library, Sel("newFunctionWithName:"), uintptr(funcName))
	Release(funcName)
	if computeFunc == 0 {
		return nil, fmt.Errorf("metal: compute function '%s' not found", desc.Compute.EntryPoint)
	}
	defer Release(computeFunc)

	// Create compute pipeline state
	var errorPtr ID
	pipelineState := MsgSend(d.raw, Sel("newComputePipelineStateWithFunction:error:"),
		uintptr(computeFunc), uintptr(unsafe.Pointer(&errorPtr)))

	if pipelineState == 0 {
		errMsg := unknownError
		if errorPtr != 0 {
			errDesc := MsgSend(errorPtr, Sel("localizedDescription"))
			if errDesc != 0 {
				errMsg = GoString(errDesc)
			}
			// Object is autoreleased
		}
		return nil, fmt.Errorf("metal: failed to create compute pipeline state: %s", errMsg)
	}

	// Get workgroup size from shader module metadata
	workgroupSize := getWorkgroupSize(computeModule, desc.Compute.EntryPoint)

	hal.Logger().Debug("metal: compute pipeline created",
		"entryPoint", desc.Compute.EntryPoint,
		"workgroupSize", fmt.Sprintf("%dx%dx%d", workgroupSize.Width, workgroupSize.Height, workgroupSize.Depth),
	)

	var pipeLayout *PipelineLayout
	if pl, ok := desc.Layout.(*PipelineLayout); ok {
		pipeLayout = pl
	}
	return &ComputePipeline{
		raw:           pipelineState,
		device:        d,
		layout:        pipeLayout,
		workgroupSize: workgroupSize,
	}, nil
}

// getWorkgroupSize retrieves workgroup size for a compute entry point.
// Falls back to default {64, 1, 1} if not found.
func getWorkgroupSize(module *ShaderModule, entryPoint string) MTLSize {
	if module.workgroupSizes != nil {
		if size, ok := module.workgroupSizes[entryPoint]; ok {
			return MTLSize{
				Width:  NSUInteger(size[0]),
				Height: NSUInteger(size[1]),
				Depth:  NSUInteger(size[2]),
			}
		}
	}
	// Default fallback
	return MTLSize{Width: 64, Height: 1, Depth: 1}
}

// DestroyComputePipeline destroys a compute pipeline.
func (d *Device) DestroyComputePipeline(pipeline hal.ComputePipeline) {
	mtlPipeline, ok := pipeline.(*ComputePipeline)
	if !ok || mtlPipeline == nil {
		return
	}
	if mtlPipeline.raw != 0 {
		Release(mtlPipeline.raw)
		mtlPipeline.raw = 0
	}
	mtlPipeline.device = nil
}

// CreateCommandEncoder creates a command encoder.
//
// The Metal command buffer is NOT created here — it is deferred to BeginEncoding.
// This matches the two-step pattern used by Vulkan (allocate → vkBeginCommandBuffer)
// CreateQuerySet creates a query set.
// TODO: implement using Metal counter sample buffers for timestamp support.
func (d *Device) CreateQuerySet(_ *hal.QuerySetDescriptor) (hal.QuerySet, error) {
	return nil, hal.ErrTimestampsNotSupported
}

// DestroyQuerySet destroys a query set.
func (d *Device) DestroyQuerySet(_ hal.QuerySet) {
	// Stub: Metal query set implementation pending.
}

// CreateCommandEncoder creates a command encoder for recording GPU commands.
//
// and DX12 (create list → Reset). Creating the command buffer eagerly here would
// conflict with BeginEncoding's guard (cmdBuffer != 0 → "already recording"),
// causing every subsequent BeginEncoding call to fail and leak the pre-allocated
// command buffer and its autorelease pool.
func (d *Device) CreateCommandEncoder(desc *hal.CommandEncoderDescriptor) (hal.CommandEncoder, error) {
	label := ""
	if desc != nil {
		label = desc.Label
	}
	hal.Logger().Debug("metal: command encoder created", "label", label)
	return &CommandEncoder{device: d, label: label}, nil
}

// CreateFence creates a synchronization fence backed by MTLSharedEvent.
//
// MTLSharedEvent (unlike MTLEvent) exposes a signaledValue property readable
// from the CPU, enabling proper blocking waits and non-blocking status queries.
func (d *Device) CreateFence() (hal.Fence, error) {
	event := MsgSend(d.raw, Sel("newSharedEvent"))
	if event == 0 {
		return nil, fmt.Errorf("metal: failed to create shared event")
	}
	return &Fence{event: event, device: d}, nil
}

// DestroyFence destroys a fence.
func (d *Device) DestroyFence(fence hal.Fence) {
	mtlFence, ok := fence.(*Fence)
	if !ok || mtlFence == nil {
		return
	}
	if mtlFence.event != 0 {
		Release(mtlFence.event)
		mtlFence.event = 0
	}
	mtlFence.device = nil
}

// getOrCreateEventListener returns a lazily-created MTLSharedEventListener.
// The listener is allocated once per device and reused for all event notifications.
// It is released in Destroy().
func (d *Device) getOrCreateEventListener() ID {
	if d.eventListener != 0 {
		return d.eventListener
	}
	cls := GetClass("MTLSharedEventListener")
	if cls == 0 {
		return 0
	}
	obj := MsgSend(ID(cls), Sel("alloc"))
	if obj == 0 {
		return 0
	}
	obj = MsgSend(obj, Sel("init"))
	if obj == 0 {
		return 0
	}
	d.eventListener = obj
	return d.eventListener
}

// Wait waits for a fence to reach the specified value.
//
// Uses Metal's MTLSharedEvent.notifyListener:atValue:block: for event-driven
// notification when available. This avoids CPU polling and reduces latency
// compared to the spin-yield-sleep fallback.
//
// Falls back to polling with progressive backoff if block infrastructure
// is unavailable (e.g., _NSConcreteStackBlock symbol not loaded).
func (d *Device) Wait(fence hal.Fence, value uint64, timeout time.Duration) (bool, error) {
	mtlFence, ok := fence.(*Fence)
	if !ok || mtlFence == nil {
		return false, fmt.Errorf("metal: invalid fence")
	}

	// Fast path: already signaled.
	signaled := MsgSendUint(mtlFence.event, Sel("signaledValue"))
	if uint64(signaled) >= value {
		return true, nil
	}

	// Try event-driven path using MTLSharedEvent notification.
	if result, attempted, err := d.waitEventDriven(mtlFence, value, timeout); attempted {
		return result, err
	}

	// Fallback: poll with progressive backoff.
	return d.waitPolling(mtlFence, value, timeout)
}

// waitEventDriven attempts to wait using MTLSharedEvent.notifyListener:atValue:block:.
// Returns (result, true, nil/error) if the event-driven path was used.
// Returns (false, false, nil) if the path is unavailable and caller should fall back.
func (d *Device) waitEventDriven(mtlFence *Fence, value uint64, timeout time.Duration) (bool, bool, error) {
	hal.Logger().Debug("metal: Wait", "value", value, "timeout", timeout, "path", "event-driven")
	listener := d.getOrCreateEventListener()
	if listener == 0 {
		return false, false, nil
	}

	blockPtr, blockID, done := newSharedEventNotificationBlock()
	if blockPtr == 0 {
		return false, false, nil
	}
	defer releaseBlock(blockID)

	// Register the notification: notifyListener:atValue:block:
	// This tells Metal to invoke our block when signaledValue >= value.
	msgSendVoid(mtlFence.event, Sel("notifyListener:atValue:block:"),
		argPointer(uintptr(listener)),
		argUint64(value),
		argPointer(blockPtr),
	)

	// Block is pinned via blockPinRegistry until releaseBlock(blockID).
	// No runtime.KeepAlive needed — the deferred releaseBlock above
	// keeps the pin alive for the entire function scope.

	// Wait for the callback or timeout.
	select {
	case <-done:
		return true, true, nil
	case <-time.After(timeout):
		// Timeout — check once more in case the event fired between
		// the select evaluation and now.
		select {
		case <-done:
			return true, true, nil
		default:
			return false, true, nil
		}
	}
}

// waitPolling waits for a fence using progressive backoff polling.
// This is the fallback path when event-driven notification is unavailable.
func (d *Device) waitPolling(mtlFence *Fence, value uint64, timeout time.Duration) (bool, error) {
	hal.Logger().Debug("metal: Wait (polling)", "value", value, "timeout", timeout)
	deadline := time.Now().Add(timeout)
	spins := 0
	for {
		signaled := MsgSendUint(mtlFence.event, Sel("signaledValue"))
		if uint64(signaled) >= value {
			return true, nil
		}
		if time.Now().After(deadline) {
			return false, nil
		}

		// Progressive backoff: first 100 iterations spin, then yield, then sleep.
		spins++
		switch {
		case spins < 100:
			// Busy spin for low-latency scenarios.
		case spins < 200:
			runtime.Gosched()
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

// ResetFence resets a fence to the unsignaled state.
// Sets the MTLSharedEvent.signaledValue to 0 via Objective-C message send.
func (d *Device) ResetFence(fence hal.Fence) error {
	mtlFence, ok := fence.(*Fence)
	if !ok || mtlFence == nil {
		return fmt.Errorf("metal: invalid fence")
	}
	_ = MsgSend(mtlFence.event, Sel("setSignaledValue:"), uintptr(0))
	return nil
}

// GetFenceStatus returns true if the fence is signaled (non-blocking).
// Reads the GPU-updated signaledValue from MTLSharedEvent.
func (d *Device) GetFenceStatus(fence hal.Fence) (bool, error) {
	mtlFence, ok := fence.(*Fence)
	if !ok || mtlFence == nil {
		return false, fmt.Errorf("metal: invalid fence")
	}
	signaled := MsgSendUint(mtlFence.event, Sel("signaledValue"))
	return signaled > 0, nil
}

// FreeCommandBuffer releases a submitted command buffer.
// Autorelease pools are no longer stored in command buffers — they use scoped
// pools that drain immediately in BeginEncoding (macOS Tahoe LIFO fix).
func (d *Device) FreeCommandBuffer(cmdBuffer hal.CommandBuffer) {
	cb, ok := cmdBuffer.(*CommandBuffer)
	if !ok || cb == nil {
		return
	}
	if cb.raw != 0 {
		Release(cb.raw)
		cb.raw = 0
	}
}

// CreateRenderBundleEncoder is not supported in Metal backend.
func (d *Device) CreateRenderBundleEncoder(desc *hal.RenderBundleEncoderDescriptor) (hal.RenderBundleEncoder, error) {
	return nil, fmt.Errorf("metal: render bundles not supported")
}

// DestroyRenderBundle is not supported in Metal backend.
func (d *Device) DestroyRenderBundle(bundle hal.RenderBundle) {}

// WaitIdle waits for all GPU work to complete.
//
// Metal has no device-level wait API like Vulkan's vkDeviceWaitIdle. Instead,
// we submit an empty command buffer and call waitUntilCompleted on it. Since
// command buffers execute in order on the same queue, this guarantees all
// previously submitted work has finished.
//
// After the GPU is idle, we drain and refill the frame semaphore to ensure
// all in-flight slots are reclaimed. This prevents deadlocks when the caller
// wants to submit new work after WaitIdle returns.
func (d *Device) WaitIdle() error {
	hal.Logger().Debug("metal: WaitIdle starting")
	pool := NewAutoreleasePool()
	defer pool.Drain()

	// Submit an empty command buffer and wait for it synchronously.
	// All previously committed command buffers on this queue will complete
	// before this one starts, so waitUntilCompleted acts as a full barrier.
	cmdBuffer := MsgSend(d.commandQueue, Sel("commandBuffer"))
	if cmdBuffer != 0 {
		Retain(cmdBuffer)
		_ = MsgSend(cmdBuffer, Sel("commit"))
		_ = MsgSend(cmdBuffer, Sel("waitUntilCompleted"))
		Release(cmdBuffer)
	}

	// Drain and refill the frame semaphore. After waitUntilCompleted, all
	// in-flight completion handlers have fired and returned their tokens.
	// We drain any remaining tokens and refill to maxFramesInFlight so the
	// semaphore is in a clean state for subsequent submissions.
	if d.queue != nil && d.queue.frameSemaphore != nil {
		// Drain all available tokens (non-blocking).
		for {
			select {
			case <-d.queue.frameSemaphore:
			default:
				goto refill
			}
		}
	refill:
		// Refill to capacity.
		for i := 0; i < maxFramesInFlight; i++ {
			d.queue.frameSemaphore <- struct{}{}
		}
	}

	hal.Logger().Debug("metal: WaitIdle complete")
	return nil
}

// Destroy releases the device and associated resources.
func (d *Device) Destroy() {
	hal.Logger().Debug("metal: device destroyed")
	if d.eventListener != 0 {
		Release(d.eventListener)
		d.eventListener = 0
	}
	if d.commandQueue != 0 {
		Release(d.commandQueue)
		d.commandQueue = 0
	}
}

// extractWorkgroupSizes extracts workgroup sizes from IR module entry points.
// Returns a map from entry point name to workgroup size [x, y, z].
func extractWorkgroupSizes(module *ir.Module) map[string][3]uint32 {
	if module == nil {
		return nil
	}
	result := make(map[string][3]uint32)
	for i := range module.EntryPoints {
		ep := &module.EntryPoints[i]
		if ep.Stage == ir.StageCompute {
			result[ep.Name] = ep.Workgroup
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
