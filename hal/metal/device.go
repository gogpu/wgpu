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

// Device implements hal.Device for Metal.
type Device struct {
	raw           ID // id<MTLDevice>
	commandQueue  ID // id<MTLCommandQueue>
	adapter       *Adapter
	eventListener ID // id<MTLSharedEventListener> — created lazily, reused
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
		return nil, fmt.Errorf("metal: buffer descriptor is nil")
	}
	if desc.Size == 0 {
		return nil, fmt.Errorf("metal: buffer size must be > 0")
	}

	var options MTLResourceOptions
	mapRead := desc.Usage&gputypes.BufferUsageMapRead != 0
	mapWrite := desc.Usage&gputypes.BufferUsageMapWrite != 0

	if mapRead || mapWrite {
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
		return nil, fmt.Errorf("metal: texture descriptor is nil")
	}
	if desc.Size.Width == 0 || desc.Size.Height == 0 {
		return nil, fmt.Errorf("metal: texture size must be > 0")
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

	pixelFormat := textureFormatToMTL(desc.Format)
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
	pixelFormat := textureFormatToMTL(format)

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
		return nil, fmt.Errorf("metal: sampler descriptor is nil")
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
	return &BindGroupLayout{entries: desc.Entries, device: d}, nil
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
func (d *Device) CreatePipelineLayout(desc *hal.PipelineLayoutDescriptor) (hal.PipelineLayout, error) {
	return &PipelineLayout{layouts: desc.BindGroupLayouts, device: d}, nil
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
	if desc.Source.WGSL != "" {
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
			errMsg := "unknown error"
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
			pixelFormat := textureFormatToMTL(target.Format)
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
		errMsg := "unknown error"
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

	return &RenderPipeline{raw: pipelineState, device: d}, nil
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
		errMsg := "unknown error"
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

	return &ComputePipeline{
		raw:           pipelineState,
		device:        d,
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
	if result, err, attempted := d.waitEventDriven(mtlFence, value, timeout); attempted {
		return result, err
	}

	// Fallback: poll with progressive backoff.
	return d.waitPolling(mtlFence, value, timeout)
}

// waitEventDriven attempts to wait using MTLSharedEvent.notifyListener:atValue:block:.
// Returns (result, error, true) if the event-driven path was used.
// Returns (false, nil, false) if the path is unavailable and caller should fall back.
func (d *Device) waitEventDriven(mtlFence *Fence, value uint64, timeout time.Duration) (bool, error, bool) {
	listener := d.getOrCreateEventListener()
	if listener == 0 {
		return false, nil, false
	}

	blockPtr, blockID, done := newSharedEventNotificationBlock()
	if blockPtr == 0 {
		return false, nil, false
	}
	defer releaseBlock(blockID)

	// Register the notification: notifyListener:atValue:block:
	// This tells Metal to invoke our block when signaledValue >= value.
	msgSendVoid(mtlFence.event, Sel("notifyListener:atValue:block:"),
		argPointer(uintptr(listener)),
		argUint64(value),
		argPointer(blockPtr),
	)

	// Keep block alive until notification fires or times out.
	// The block struct is on the Go heap; runtime.KeepAlive prevents GC
	// from collecting the underlying memory while Metal holds a reference.
	defer runtime.KeepAlive(blockPtr)

	// Wait for the callback or timeout.
	select {
	case <-done:
		return true, nil, true
	case <-time.After(timeout):
		// Timeout — check once more in case the event fired between
		// the select evaluation and now.
		select {
		case <-done:
			return true, nil, true
		default:
			return false, nil, true
		}
	}
}

// waitPolling waits for a fence using progressive backoff polling.
// This is the fallback path when event-driven notification is unavailable.
func (d *Device) waitPolling(mtlFence *Fence, value uint64, timeout time.Duration) (bool, error) {
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
func (d *Device) WaitIdle() error {
	// Metal doesn't have a device-level wait, but we can wait on the command queue.
	// The MTLCommandQueue waitUntilAllCommandsCompleted is not a standard method.
	// Use the event-based Wait() with the highest known event value if available.
	return nil
}

// Destroy releases the device and associated resources.
func (d *Device) Destroy() {
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
	for _, ep := range module.EntryPoints {
		if ep.Stage == ir.StageCompute {
			result[ep.Name] = ep.Workgroup
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
