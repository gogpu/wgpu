// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

package gles

import (
	"fmt"
	"time"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/gles/gl"
	"github.com/gogpu/wgpu/hal/gles/wgl"
)

// Device implements hal.Device for OpenGL.
type Device struct {
	glCtx  *gl.Context
	wglCtx *wgl.Context
	hwnd   wgl.HWND
}

// CreateBuffer creates a GPU buffer.
func (d *Device) CreateBuffer(desc *BufferDescriptor) (hal.Buffer, error) {
	id := d.glCtx.GenBuffers(1)

	// Determine GL buffer target from usage
	target := uint32(gl.ARRAY_BUFFER)
	switch {
	case desc.Usage&gputypes.BufferUsageIndex != 0:
		target = gl.ELEMENT_ARRAY_BUFFER
	case desc.Usage&gputypes.BufferUsageUniform != 0:
		target = gl.UNIFORM_BUFFER
	case desc.Usage&gputypes.BufferUsageCopySrc != 0, desc.Usage&gputypes.BufferUsageCopyDst != 0:
		target = gl.COPY_READ_BUFFER
	}

	// Determine usage hint
	usage := uint32(gl.STATIC_DRAW)
	if desc.Usage&gputypes.BufferUsageMapWrite != 0 {
		usage = gl.DYNAMIC_DRAW
	} else if desc.Usage&gputypes.BufferUsageMapRead != 0 {
		usage = gl.DYNAMIC_READ
	}

	d.glCtx.BindBuffer(target, id)
	d.glCtx.BufferData(target, int(desc.Size), nil, usage)
	d.glCtx.BindBuffer(target, 0)

	buf := &Buffer{
		id:    id,
		size:  desc.Size,
		usage: desc.Usage,
		glCtx: d.glCtx,
	}

	// Handle MappedAtCreation
	if desc.MappedAtCreation {
		buf.mapped = make([]byte, desc.Size)
	}

	return buf, nil
}

// DestroyBuffer destroys a GPU buffer.
func (d *Device) DestroyBuffer(buffer hal.Buffer) {
	if b, ok := buffer.(*Buffer); ok {
		b.Destroy()
	}
}

// CreateTexture creates a GPU texture.
func (d *Device) CreateTexture(desc *TextureDescriptor) (hal.Texture, error) {
	id := d.glCtx.GenTextures(1)

	// Map dimension to GL target
	target := uint32(gl.TEXTURE_2D)
	switch desc.Dimension {
	case gputypes.TextureDimension1D:
		// GL doesn't have 1D textures in ES, use 2D with height=1
		target = gl.TEXTURE_2D
	case gputypes.TextureDimension2D:
		if desc.Size.DepthOrArrayLayers > 1 {
			target = gl.TEXTURE_2D_ARRAY
		} else {
			target = gl.TEXTURE_2D
		}
	case gputypes.TextureDimension3D:
		target = gl.TEXTURE_3D
	}

	// Handle cube maps
	if desc.ViewFormats != nil {
		for _, vf := range desc.ViewFormats {
			if vf == desc.Format {
				// Check if this should be a cube map
				if desc.Size.DepthOrArrayLayers == 6 {
					target = gl.TEXTURE_CUBE_MAP
				}
			}
		}
	}

	d.glCtx.BindTexture(target, id)

	// Get GL format info
	internalFormat, format, dataType := textureFormatToGL(desc.Format)

	// Allocate texture storage
	switch target {
	case gl.TEXTURE_2D:
		for level := uint32(0); level < desc.MipLevelCount; level++ {
			width := maxInt32(1, int32(desc.Size.Width>>level))
			height := maxInt32(1, int32(desc.Size.Height>>level))
			d.glCtx.TexImage2D(target, int32(level), int32(internalFormat),
				width, height, 0, format, dataType, nil)
		}

	case gl.TEXTURE_CUBE_MAP:
		for face := uint32(0); face < 6; face++ {
			faceTarget := gl.TEXTURE_CUBE_MAP_POSITIVE_X + face
			for level := uint32(0); level < desc.MipLevelCount; level++ {
				width := maxInt32(1, int32(desc.Size.Width>>level))
				height := maxInt32(1, int32(desc.Size.Height>>level))
				d.glCtx.TexImage2D(faceTarget, int32(level), int32(internalFormat),
					width, height, 0, format, dataType, nil)
			}
		}
	}

	// Set default texture parameters
	d.glCtx.TexParameteri(target, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
	d.glCtx.TexParameteri(target, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
	d.glCtx.TexParameteri(target, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	d.glCtx.TexParameteri(target, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)

	d.glCtx.BindTexture(target, 0)

	return &Texture{
		id:        id,
		target:    target,
		format:    desc.Format,
		dimension: desc.Dimension,
		size:      desc.Size,
		mipLevels: desc.MipLevelCount,
		glCtx:     d.glCtx,
	}, nil
}

// DestroyTexture destroys a GPU texture.
func (d *Device) DestroyTexture(texture hal.Texture) {
	if t, ok := texture.(*Texture); ok {
		t.Destroy()
	}
}

// CreateTextureView creates a view into a texture.
func (d *Device) CreateTextureView(texture hal.Texture, desc *TextureViewDescriptor) (hal.TextureView, error) {
	t, ok := texture.(*Texture)
	if !ok {
		return nil, fmt.Errorf("gles: invalid texture type")
	}

	return &TextureView{
		texture:    t,
		aspect:     desc.Aspect,
		baseMip:    desc.BaseMipLevel,
		mipCount:   desc.MipLevelCount,
		baseLayer:  desc.BaseArrayLayer,
		layerCount: desc.ArrayLayerCount,
	}, nil
}

// DestroyTextureView destroys a texture view.
func (d *Device) DestroyTextureView(view hal.TextureView) {
	// TextureViews don't hold GL resources in OpenGL
}

// CreateSampler creates a texture sampler.
func (d *Device) CreateSampler(desc *SamplerDescriptor) (hal.Sampler, error) {
	// For now, use texture-bound sampler state
	// Note: GL sampler objects (GL 3.3+) would allow independent sampler state.
	return &Sampler{
		glCtx: d.glCtx,
	}, nil
}

// DestroySampler destroys a sampler.
func (d *Device) DestroySampler(sampler hal.Sampler) {
	if s, ok := sampler.(*Sampler); ok {
		s.Destroy()
	}
}

// CreateBindGroupLayout creates a bind group layout.
func (d *Device) CreateBindGroupLayout(desc *BindGroupLayoutDescriptor) (hal.BindGroupLayout, error) {
	return &BindGroupLayout{
		entries: desc.Entries,
	}, nil
}

// DestroyBindGroupLayout destroys a bind group layout.
func (d *Device) DestroyBindGroupLayout(layout hal.BindGroupLayout) {}

// CreateBindGroup creates a bind group.
func (d *Device) CreateBindGroup(desc *BindGroupDescriptor) (hal.BindGroup, error) {
	layout, ok := desc.Layout.(*BindGroupLayout)
	if !ok {
		return nil, fmt.Errorf("gles: invalid bind group layout type")
	}

	return &BindGroup{
		layout:  layout,
		entries: desc.Entries,
	}, nil
}

// DestroyBindGroup destroys a bind group.
func (d *Device) DestroyBindGroup(group hal.BindGroup) {}

// CreatePipelineLayout creates a pipeline layout.
func (d *Device) CreatePipelineLayout(desc *PipelineLayoutDescriptor) (hal.PipelineLayout, error) {
	layouts := make([]*BindGroupLayout, len(desc.BindGroupLayouts))
	for i, l := range desc.BindGroupLayouts {
		layout, ok := l.(*BindGroupLayout)
		if !ok {
			return nil, fmt.Errorf("gles: invalid bind group layout at index %d", i)
		}
		layouts[i] = layout
	}

	return &PipelineLayout{
		bindGroupLayouts: layouts,
	}, nil
}

// DestroyPipelineLayout destroys a pipeline layout.
func (d *Device) DestroyPipelineLayout(layout hal.PipelineLayout) {}

// CreateShaderModule creates a shader module.
func (d *Device) CreateShaderModule(desc *ShaderModuleDescriptor) (hal.ShaderModule, error) {
	// For now, store the source - compilation happens at pipeline creation
	return &ShaderModule{
		source: desc.Source,
		glCtx:  d.glCtx,
	}, nil
}

// DestroyShaderModule destroys a shader module.
func (d *Device) DestroyShaderModule(module hal.ShaderModule) {
	if m, ok := module.(*ShaderModule); ok {
		m.Destroy()
	}
}

// CreateRenderPipeline creates a render pipeline.
func (d *Device) CreateRenderPipeline(desc *RenderPipelineDescriptor) (hal.RenderPipeline, error) {
	layout, ok := desc.Layout.(*PipelineLayout)
	if !ok {
		return nil, fmt.Errorf("gles: invalid pipeline layout type")
	}

	vertexModule, ok := desc.Vertex.Module.(*ShaderModule)
	if !ok {
		return nil, fmt.Errorf("gles: invalid vertex shader module type")
	}

	// Compile vertex shader
	vertexID := d.glCtx.CreateShader(gl.VERTEX_SHADER)
	d.glCtx.ShaderSource(vertexID, vertexModule.source.WGSL)
	d.glCtx.CompileShader(vertexID)

	var status int32
	d.glCtx.GetShaderiv(vertexID, gl.COMPILE_STATUS, &status)
	if status == gl.FALSE {
		log := d.glCtx.GetShaderInfoLog(vertexID)
		d.glCtx.DeleteShader(vertexID)
		return nil, fmt.Errorf("gles: vertex shader compilation failed: %s", log)
	}

	// Compile fragment shader
	var fragmentID uint32
	if desc.Fragment != nil {
		fragmentModule, ok := desc.Fragment.Module.(*ShaderModule)
		if !ok {
			d.glCtx.DeleteShader(vertexID)
			return nil, fmt.Errorf("gles: invalid fragment shader module type")
		}

		fragmentID = d.glCtx.CreateShader(gl.FRAGMENT_SHADER)
		d.glCtx.ShaderSource(fragmentID, fragmentModule.source.WGSL)
		d.glCtx.CompileShader(fragmentID)

		d.glCtx.GetShaderiv(fragmentID, gl.COMPILE_STATUS, &status)
		if status == gl.FALSE {
			log := d.glCtx.GetShaderInfoLog(fragmentID)
			d.glCtx.DeleteShader(vertexID)
			d.glCtx.DeleteShader(fragmentID)
			return nil, fmt.Errorf("gles: fragment shader compilation failed: %s", log)
		}
	}

	// Link program
	programID := d.glCtx.CreateProgram()
	d.glCtx.AttachShader(programID, vertexID)
	if fragmentID != 0 {
		d.glCtx.AttachShader(programID, fragmentID)
	}
	d.glCtx.LinkProgram(programID)

	d.glCtx.GetProgramiv(programID, gl.LINK_STATUS, &status)
	if status == gl.FALSE {
		log := d.glCtx.GetProgramInfoLog(programID)
		d.glCtx.DeleteShader(vertexID)
		if fragmentID != 0 {
			d.glCtx.DeleteShader(fragmentID)
		}
		d.glCtx.DeleteProgram(programID)
		return nil, fmt.Errorf("gles: program linking failed: %s", log)
	}

	// Shaders can be deleted after linking
	d.glCtx.DeleteShader(vertexID)
	if fragmentID != 0 {
		d.glCtx.DeleteShader(fragmentID)
	}

	return &RenderPipeline{
		programID:         programID,
		layout:            layout,
		glCtx:             d.glCtx,
		primitiveTopology: desc.Primitive.Topology,
		cullMode:          desc.Primitive.CullMode,
		frontFace:         desc.Primitive.FrontFace,
		depthStencil:      desc.DepthStencil,
		multisample:       desc.Multisample,
	}, nil
}

// DestroyRenderPipeline destroys a render pipeline.
func (d *Device) DestroyRenderPipeline(pipeline hal.RenderPipeline) {
	if p, ok := pipeline.(*RenderPipeline); ok {
		p.Destroy()
	}
}

// CreateComputePipeline creates a compute pipeline.
func (d *Device) CreateComputePipeline(desc *ComputePipelineDescriptor) (hal.ComputePipeline, error) {
	layout, ok := desc.Layout.(*PipelineLayout)
	if !ok {
		return nil, fmt.Errorf("gles: invalid pipeline layout type")
	}

	computeModule, ok := desc.Compute.Module.(*ShaderModule)
	if !ok {
		return nil, fmt.Errorf("gles: invalid compute shader module type")
	}

	// Compile compute shader
	computeID := d.glCtx.CreateShader(gl.COMPUTE_SHADER)
	d.glCtx.ShaderSource(computeID, computeModule.source.WGSL)
	d.glCtx.CompileShader(computeID)

	var status int32
	d.glCtx.GetShaderiv(computeID, gl.COMPILE_STATUS, &status)
	if status == gl.FALSE {
		log := d.glCtx.GetShaderInfoLog(computeID)
		d.glCtx.DeleteShader(computeID)
		return nil, fmt.Errorf("gles: compute shader compilation failed: %s", log)
	}

	// Link program
	programID := d.glCtx.CreateProgram()
	d.glCtx.AttachShader(programID, computeID)
	d.glCtx.LinkProgram(programID)

	d.glCtx.GetProgramiv(programID, gl.LINK_STATUS, &status)
	if status == gl.FALSE {
		log := d.glCtx.GetProgramInfoLog(programID)
		d.glCtx.DeleteShader(computeID)
		d.glCtx.DeleteProgram(programID)
		return nil, fmt.Errorf("gles: compute program linking failed: %s", log)
	}

	d.glCtx.DeleteShader(computeID)

	return &ComputePipeline{
		programID: programID,
		layout:    layout,
		glCtx:     d.glCtx,
	}, nil
}

// DestroyComputePipeline destroys a compute pipeline.
func (d *Device) DestroyComputePipeline(pipeline hal.ComputePipeline) {
	if p, ok := pipeline.(*ComputePipeline); ok {
		p.Destroy()
	}
}

// CreateCommandEncoder creates a command encoder.
func (d *Device) CreateCommandEncoder(_ *CommandEncoderDescriptor) (hal.CommandEncoder, error) {
	return &CommandEncoder{
		glCtx: d.glCtx,
	}, nil
}

// CreateFence creates a synchronization fence.
func (d *Device) CreateFence() (hal.Fence, error) {
	return NewFence(d.glCtx), nil
}

// DestroyFence destroys a fence.
func (d *Device) DestroyFence(fence hal.Fence) {
	if f, ok := fence.(*Fence); ok {
		f.Destroy()
	}
}

// Wait waits for a fence to reach the specified value.
func (d *Device) Wait(fence hal.Fence, value uint64, timeout time.Duration) (bool, error) {
	f, ok := fence.(*Fence)
	if !ok {
		return false, fmt.Errorf("gles: invalid fence type")
	}
	return f.Wait(value, timeout), nil
}

// Destroy releases the device.
func (d *Device) Destroy() {
	// Device doesn't own the GL context
}

// Type aliases for hal descriptors
type (
	BufferDescriptor          = hal.BufferDescriptor
	TextureDescriptor         = hal.TextureDescriptor
	TextureViewDescriptor     = hal.TextureViewDescriptor
	SamplerDescriptor         = hal.SamplerDescriptor
	BindGroupLayoutDescriptor = hal.BindGroupLayoutDescriptor
	BindGroupDescriptor       = hal.BindGroupDescriptor
	PipelineLayoutDescriptor  = hal.PipelineLayoutDescriptor
	ShaderModuleDescriptor    = hal.ShaderModuleDescriptor
	RenderPipelineDescriptor  = hal.RenderPipelineDescriptor
	ComputePipelineDescriptor = hal.ComputePipelineDescriptor
	CommandEncoderDescriptor  = hal.CommandEncoderDescriptor
)

// textureFormatToGL converts a WebGPU texture format to GL format info.
func textureFormatToGL(format gputypes.TextureFormat) (internalFormat, dataFormat, dataType uint32) {
	switch format {
	case gputypes.TextureFormatR8Unorm:
		return gl.R8, gl.RED, gl.UNSIGNED_BYTE
	case gputypes.TextureFormatRG8Unorm:
		return gl.RG8, gl.RG, gl.UNSIGNED_BYTE
	case gputypes.TextureFormatRGBA8Unorm:
		return gl.RGBA8, gl.RGBA, gl.UNSIGNED_BYTE
	case gputypes.TextureFormatRGBA8UnormSrgb:
		return gl.SRGB8_ALPHA8, gl.RGBA, gl.UNSIGNED_BYTE
	case gputypes.TextureFormatBGRA8Unorm:
		return gl.RGBA8, gl.BGRA, gl.UNSIGNED_BYTE
	case gputypes.TextureFormatBGRA8UnormSrgb:
		return gl.SRGB8_ALPHA8, gl.BGRA, gl.UNSIGNED_BYTE
	case gputypes.TextureFormatR16Float:
		return gl.R16F, gl.RED, gl.HALF_FLOAT
	case gputypes.TextureFormatRG16Float:
		return gl.RG16F, gl.RG, gl.HALF_FLOAT
	case gputypes.TextureFormatRGBA16Float:
		return gl.RGBA16F, gl.RGBA, gl.HALF_FLOAT
	case gputypes.TextureFormatR32Float:
		return gl.R32F, gl.RED, gl.FLOAT
	case gputypes.TextureFormatRG32Float:
		return gl.RG32F, gl.RG, gl.FLOAT
	case gputypes.TextureFormatRGBA32Float:
		return gl.RGBA32F, gl.RGBA, gl.FLOAT
	case gputypes.TextureFormatDepth16Unorm:
		return gl.DEPTH_COMPONENT16, gl.DEPTH_COMPONENT, gl.UNSIGNED_SHORT
	case gputypes.TextureFormatDepth24Plus:
		return gl.DEPTH_COMPONENT24, gl.DEPTH_COMPONENT, gl.UNSIGNED_INT
	case gputypes.TextureFormatDepth24PlusStencil8:
		return gl.DEPTH24_STENCIL8, gl.DEPTH_STENCIL, gl.UNSIGNED_INT
	case gputypes.TextureFormatDepth32Float:
		return gl.DEPTH_COMPONENT32, gl.DEPTH_COMPONENT, gl.FLOAT
	case gputypes.TextureFormatDepth32FloatStencil8:
		return gl.DEPTH32F_STENCIL8, gl.DEPTH_STENCIL, gl.FLOAT
	default:
		// Default to RGBA8
		return gl.RGBA8, gl.RGBA, gl.UNSIGNED_BYTE
	}
}

// maxInt32 returns the larger of a or b.
func maxInt32(a, b int32) int32 {
	if a > b {
		return a
	}
	return b
}

// Ensure we use unsafe for later
var _ = unsafe.Pointer(nil)
