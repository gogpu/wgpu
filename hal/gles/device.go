// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows && !(js && wasm)

package gles

import (
	"fmt"
	"time"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/naga/glsl"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/gles/gl"
)

// Device implements hal.Device for OpenGL.
// Holds a shared *AdapterContext (owned by Instance). All GL operations
// acquire the context lock, do MakeCurrent, execute, and unlock.
type Device struct {
	ctx             *AdapterContext
	vao             uint32 // persistent VAO (Core Profile requires one bound)
	maxTextureUnits int32  // GL_MAX_TEXTURE_IMAGE_UNITS (queried at init)

	// glslVersion is the target GLSL version for shader compilation, detected
	// from the adapter's GL_SHADING_LANGUAGE_VERSION at Open time.
	// Propagated to naga GLSL writer for correct #version directive and
	// version-gated features (layout(binding=N) needs GLSL >= 420).
	glslVersion glsl.Version

	// shaderBindingLayout is true when the driver supports layout(binding=N)
	// in shaders (GLSL >= 420 desktop or >= 310 ES). When false, bindings must
	// be assigned at runtime after linking via glGetUniformBlockIndex etc.
	// Mirrors Rust wgpu-hal PrivateCapabilities::SHADER_BINDING_LAYOUT.
	shaderBindingLayout bool
}

// CreateBuffer creates a GPU buffer.
func (d *Device) CreateBuffer(desc *BufferDescriptor) (hal.Buffer, error) {
	if desc == nil {
		return nil, fmt.Errorf("BUG: buffer descriptor is nil in GLES.CreateBuffer — core validation gap")
	}

	glCtx := d.ctx.Lock()
	defer d.ctx.Unlock()

	id := glCtx.GenBuffers(1)

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

	// Determine usage hint.
	// Rust wgpu-hal GLES uses DYNAMIC_DRAW for all writable buffers (device.rs:600):
	// "Some vendors take usage very literally and STATIC_DRAW will freeze us with
	// an empty buffer." On Intel, STATIC_DRAW causes glBufferSubData to be silently
	// ignored for per-frame uniform updates, resulting in invisible text (BUG-GLES-TEXT-001).
	usage := uint32(gl.DYNAMIC_DRAW)
	if desc.Usage&gputypes.BufferUsageMapRead != 0 {
		usage = gl.DYNAMIC_READ
	}

	glCtx.BindBuffer(target, id)
	glCtx.BufferData(target, int(desc.Size), nil, usage)
	glCtx.BindBuffer(target, 0)

	buf := &Buffer{
		id:     id,
		target: target,
		size:   desc.Size,
		usage:  desc.Usage,
		glCtx:  glCtx,
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

// MapBuffer establishes a CPU-visible mapping for a GL buffer.
func (d *Device) MapBuffer(buffer hal.Buffer, offset, size uint64) (hal.BufferMapping, error) {
	buf, ok := buffer.(*Buffer)
	if !ok || buf == nil {
		return hal.BufferMapping{}, hal.ErrInvalidMapRange
	}
	if offset+size > buf.size {
		return hal.BufferMapping{}, hal.ErrInvalidMapRange
	}
	if buf.mapped == nil {
		buf.mapped = make([]byte, buf.size)

		if buf.usage&gputypes.BufferUsageMapRead != 0 {
			glCtx := d.ctx.Lock()
			switch {
			case len(buf.data) == int(buf.size):
				copy(buf.mapped, buf.data)
			case buf.id != 0:
				glCtx.BindBuffer(gl.COPY_READ_BUFFER, buf.id)
				glPtr := glCtx.MapBuffer(gl.COPY_READ_BUFFER, gl.READ_ONLY)
				if glPtr != 0 {
					basePtr := *(**byte)(unsafe.Pointer(&glPtr))
					src := unsafe.Slice(basePtr, buf.size)
					copy(buf.mapped, src)
					glCtx.UnmapBuffer(gl.COPY_READ_BUFFER)
				}
				glCtx.BindBuffer(gl.COPY_READ_BUFFER, 0)
			}
			d.ctx.Unlock()
		}
	}
	return hal.BufferMapping{
		Ptr:        unsafe.Pointer(&buf.mapped[offset]),
		IsCoherent: true,
	}, nil
}

// UnmapBuffer releases a GL buffer mapping.
func (d *Device) UnmapBuffer(buffer hal.Buffer) error {
	buf, ok := buffer.(*Buffer)
	if !ok || buf == nil {
		return nil
	}
	if buf.mapped == nil {
		return nil
	}
	if buf.usage&gputypes.BufferUsageMapWrite != 0 && buf.id != 0 {
		glCtx := d.ctx.Lock()
		glCtx.BindBuffer(buf.target, buf.id)
		glCtx.BufferSubData(buf.target, 0, len(buf.mapped), unsafe.Pointer(&buf.mapped[0]))
		glCtx.BindBuffer(buf.target, 0)
		d.ctx.Unlock()
	}
	buf.mapped = nil
	return nil
}

// CreateTexture creates a GPU texture.
func (d *Device) CreateTexture(desc *TextureDescriptor) (hal.Texture, error) {
	if desc == nil {
		return nil, fmt.Errorf("BUG: texture descriptor is nil in GLES.CreateTexture — core validation gap")
	}

	glCtx := d.ctx.Lock()
	defer d.ctx.Unlock()

	id := glCtx.GenTextures(1)

	sampleCount := desc.SampleCount
	if sampleCount == 0 {
		sampleCount = 1
	}

	// Map dimension to GL target
	target := uint32(gl.TEXTURE_2D)
	switch desc.Dimension {
	case gputypes.TextureDimension1D:
		// GL doesn't have 1D textures in ES, use 2D with height=1
		target = gl.TEXTURE_2D
	case gputypes.TextureDimension2D:
		switch {
		case sampleCount > 1:
			target = gl.TEXTURE_2D_MULTISAMPLE
		case desc.Size.DepthOrArrayLayers > 1:
			target = gl.TEXTURE_2D_ARRAY
		default:
			target = gl.TEXTURE_2D
		}
	case gputypes.TextureDimension3D:
		target = gl.TEXTURE_3D
	}

	// Handle cube maps (only for single-sample textures)
	if sampleCount <= 1 && desc.ViewFormats != nil {
		for _, vf := range desc.ViewFormats {
			if vf == desc.Format {
				// Check if this should be a cube map
				if desc.Size.DepthOrArrayLayers == 6 {
					target = gl.TEXTURE_CUBE_MAP
				}
			}
		}
	}

	glCtx.BindTexture(target, id)

	// Get GL format info
	internalFormat, format, dataType := textureFormatToGL(desc.Format)

	// Allocate texture storage
	switch target {
	case gl.TEXTURE_2D_MULTISAMPLE:
		glCtx.TexImage2DMultisample(target, int32(sampleCount), internalFormat,
			int32(desc.Size.Width), int32(desc.Size.Height), true)

	case gl.TEXTURE_2D:
		for level := uint32(0); level < desc.MipLevelCount; level++ {
			width := maxInt32(1, int32(desc.Size.Width>>level))
			height := maxInt32(1, int32(desc.Size.Height>>level))
			glCtx.TexImage2D(target, int32(level), int32(internalFormat),
				width, height, 0, format, dataType, nil)
		}

	case gl.TEXTURE_CUBE_MAP:
		for face := uint32(0); face < 6; face++ {
			faceTarget := gl.TEXTURE_CUBE_MAP_POSITIVE_X + face
			for level := uint32(0); level < desc.MipLevelCount; level++ {
				width := maxInt32(1, int32(desc.Size.Width>>level))
				height := maxInt32(1, int32(desc.Size.Height>>level))
				glCtx.TexImage2D(faceTarget, int32(level), int32(internalFormat),
					width, height, 0, format, dataType, nil)
			}
		}
	}

	if target != gl.TEXTURE_2D_MULTISAMPLE {
		glCtx.TexParameteri(target, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
		glCtx.TexParameteri(target, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
		glCtx.TexParameteri(target, gl.TEXTURE_MAX_LEVEL, int32(desc.MipLevelCount-1))
		glCtx.TexParameteri(target, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
		glCtx.TexParameteri(target, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	}

	glCtx.BindTexture(target, 0)

	hal.Logger().Debug("gles: texture created",
		"label", desc.Label,
		"format", desc.Format,
		"width", desc.Size.Width,
		"height", desc.Size.Height,
	)

	return &Texture{
		id:          id,
		target:      target,
		format:      desc.Format,
		dimension:   desc.Dimension,
		size:        desc.Size,
		mipLevels:   desc.MipLevelCount,
		sampleCount: sampleCount,
		glCtx:       glCtx,
	}, nil
}

// DestroyTexture destroys a GPU texture.
func (d *Device) DestroyTexture(texture hal.Texture) {
	if t, ok := texture.(*Texture); ok {
		t.Destroy()
	}
}

// CreateTextureView creates a view into a texture.
// Accepts both *Texture and *SurfaceTexture (default framebuffer).
func (d *Device) CreateTextureView(texture hal.Texture, desc *TextureViewDescriptor) (hal.TextureView, error) {
	// Surface texture (default framebuffer) — return a view with no GL texture.
	if st, ok := texture.(*SurfaceTexture); ok {
		return &TextureView{
			isSurface:  true,
			surfaceTex: st,
		}, nil
	}

	t, ok := texture.(*Texture)
	if !ok {
		return nil, fmt.Errorf("gles: invalid texture type")
	}

	view := &TextureView{
		texture: t,
	}
	if desc != nil {
		view.aspect = desc.Aspect
		view.baseMip = desc.BaseMipLevel
		view.mipCount = desc.MipLevelCount
		view.baseLayer = desc.BaseArrayLayer
		view.layerCount = desc.ArrayLayerCount
	}
	return view, nil
}

// DestroyTextureView destroys a texture view.
func (d *Device) DestroyTextureView(view hal.TextureView) {
	// TextureViews don't hold GL resources in OpenGL
}

// CreateSampler creates a texture sampler using GL sampler objects (GL 3.3+).
func (d *Device) CreateSampler(desc *SamplerDescriptor) (hal.Sampler, error) {
	glCtx := d.ctx.Lock()
	defer d.ctx.Unlock()

	if desc == nil {
		return &Sampler{glCtx: glCtx}, nil
	}
	id := configureSampler(glCtx, desc)
	return &Sampler{
		id:    id,
		glCtx: glCtx,
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
// Computes per-type sequential binding indices following the Rust wgpu-hal pattern
// (wgpu-hal/src/gles/device.rs:1154-1221). Five resource type counters (samplers,
// textures, images, uniform buffers, storage buffers) are incremented sequentially
// across all bind group layouts, producing a flat GL slot index per binding.
func (d *Device) CreatePipelineLayout(desc *PipelineLayoutDescriptor) (hal.PipelineLayout, error) {
	layouts := make([]*BindGroupLayout, len(desc.BindGroupLayouts))
	for i, l := range desc.BindGroupLayouts {
		layout, ok := l.(*BindGroupLayout)
		if !ok {
			return nil, fmt.Errorf("gles: invalid bind group layout at index %d", i)
		}
		layouts[i] = layout
	}

	bindingMap, groupInfos := computeBindingMap(layouts)

	return &PipelineLayout{
		bindGroupLayouts: layouts,
		groupInfos:       groupInfos,
		bindingMap:       bindingMap,
	}, nil
}

// DestroyPipelineLayout destroys a pipeline layout.
func (d *Device) DestroyPipelineLayout(layout hal.PipelineLayout) {}

// CreateShaderModule creates a shader module.
func (d *Device) CreateShaderModule(desc *ShaderModuleDescriptor) (hal.ShaderModule, error) {
	if desc == nil {
		return nil, fmt.Errorf("BUG: shader module descriptor is nil in GLES.CreateShaderModule — core validation gap")
	}
	return &ShaderModule{
		source: desc.Source,
		glCtx:  d.ctx.GL(),
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
	if desc == nil {
		return nil, fmt.Errorf("BUG: render pipeline descriptor is nil in GLES.CreateRenderPipeline — core validation gap")
	}

	glCtx := d.ctx.Lock()
	defer d.ctx.Unlock()

	start := time.Now()
	// Handle nil layout (auto-layout for shaders without bindings).
	var layout *PipelineLayout
	if desc.Layout != nil {
		var ok bool
		layout, ok = desc.Layout.(*PipelineLayout)
		if !ok {
			return nil, fmt.Errorf("gles: invalid pipeline layout type")
		}
	} else {
		layout = &PipelineLayout{}
	}

	vertexModule, ok := desc.Vertex.Module.(*ShaderModule)
	if !ok {
		return nil, fmt.Errorf("gles: invalid vertex shader module type")
	}

	// Compile WGSL → GLSL for vertex stage.
	vertexGLSL, vertexTranslationInfo, err := compileWGSLToGLSL(d.glslVersion, vertexModule.source, desc.Vertex.EntryPoint, layout.bindingMap)
	if err != nil {
		return nil, fmt.Errorf("gles: vertex shader: %w", err)
	}

	vertexID := glCtx.CreateShader(gl.VERTEX_SHADER)
	glCtx.ShaderSource(vertexID, vertexGLSL)
	glCtx.CompileShader(vertexID)

	var status int32
	glCtx.GetShaderiv(vertexID, gl.COMPILE_STATUS, &status)
	if status == gl.FALSE {
		log := glCtx.GetShaderInfoLog(vertexID)
		glCtx.DeleteShader(vertexID)
		return nil, fmt.Errorf("gles: vertex shader compilation failed: %s", log)
	}
	if infoLog := glCtx.GetShaderInfoLog(vertexID); infoLog != "" {
		hal.Logger().Debug("gles: vertex shader compile info", "info", infoLog)
	}

	// Compile fragment shader
	var fragmentID uint32
	var fragmentTranslationInfo glsl.TranslationInfo
	if desc.Fragment != nil {
		var err error
		fragmentID, fragmentTranslationInfo, err = compileFragmentShader(glCtx, d.glslVersion, desc.Fragment, layout.bindingMap)
		if err != nil {
			glCtx.DeleteShader(vertexID)
			return nil, err
		}
	}

	// Link program
	programID := glCtx.CreateProgram()
	glCtx.AttachShader(programID, vertexID)
	if fragmentID != 0 {
		glCtx.AttachShader(programID, fragmentID)
	}
	glCtx.LinkProgram(programID)

	glCtx.GetProgramiv(programID, gl.LINK_STATUS, &status)
	if status == gl.FALSE {
		log := glCtx.GetProgramInfoLog(programID)
		glCtx.DeleteShader(vertexID)
		if fragmentID != 0 {
			glCtx.DeleteShader(fragmentID)
		}
		glCtx.DeleteProgram(programID)
		return nil, fmt.Errorf("gles: program linking failed: %s", log)
	}
	if infoLog := glCtx.GetProgramInfoLog(programID); infoLog != "" {
		hal.Logger().Debug("gles: program link info", "info", infoLog)
	}

	// On GL < 4.2 (GLSL < 420), layout(binding=N) is unavailable so naga
	// omits it. We must assign bindings at runtime after linking, following
	// the Rust wgpu-hal pattern (device.rs:438-461).
	if !d.shaderBindingLayout {
		if err := assignBindingsAfterLink(glCtx, programID, layout, vertexTranslationInfo, fragmentTranslationInfo); err != nil {
			glCtx.DeleteShader(vertexID)
			if fragmentID != 0 {
				glCtx.DeleteShader(fragmentID)
			}
			glCtx.DeleteProgram(programID)
			return nil, err
		}
	}

	// Shaders can be deleted after linking
	glCtx.DeleteShader(vertexID)
	if fragmentID != 0 {
		glCtx.DeleteShader(fragmentID)
	}

	hal.Logger().Debug("gles: render pipeline created",
		"programID", programID,
		"vertexEntry", desc.Vertex.EntryPoint,
		"elapsed", time.Since(start),
	)

	// Extract blend state and color write mask from the first color target.
	var blend *gputypes.BlendState
	colorWriteMask := gputypes.ColorWriteMaskAll
	if desc.Fragment != nil && len(desc.Fragment.Targets) > 0 {
		blend = desc.Fragment.Targets[0].Blend
		colorWriteMask = desc.Fragment.Targets[0].WriteMask
	}

	pipeline := &RenderPipeline{
		programID:         programID,
		layout:            layout,
		glCtx:             glCtx,
		primitiveTopology: desc.Primitive.Topology,
		cullMode:          desc.Primitive.CullMode,
		frontFace:         desc.Primitive.FrontFace,
		depthStencil:      desc.DepthStencil,
		multisample:       desc.Multisample,
		blend:             blend,
		colorWriteMask:    colorWriteMask,
		vertexBuffers:     desc.Vertex.Buffers,
	}

	// Build SamplerBindMap from TextureMappings using pre-computed BindingMap.
	// For each combined sampler2D, map texture's GL slot to sampler's GL slot.
	for i := range pipeline.samplerBindMap {
		pipeline.samplerBindMap[i] = -1 // no sampler
	}
	for _, tm := range fragmentTranslationInfo.TextureMappings {
		if tm.SamplerBinding == nil {
			continue
		}
		texKey := glsl.BindingMapKey{Group: tm.TextureBinding.Group, Binding: tm.TextureBinding.Binding}
		samplerKey := glsl.BindingMapKey{Group: tm.SamplerBinding.Group, Binding: tm.SamplerBinding.Binding}
		texUnit, texOk := layout.bindingMap[texKey]
		samplerUnit, samplerOk := layout.bindingMap[samplerKey]
		if texOk && samplerOk && texUnit < maxTextureSlots {
			pipeline.samplerBindMap[texUnit] = int8(samplerUnit)
		}
	}

	return pipeline, nil
}

// DestroyRenderPipeline destroys a render pipeline.
func (d *Device) DestroyRenderPipeline(pipeline hal.RenderPipeline) {
	if p, ok := pipeline.(*RenderPipeline); ok {
		p.Destroy()
	}
}

// CreateComputePipeline creates a compute pipeline.
//
// TODO(compute-constants): Apply desc.Compute.Constants via naga's
// pipeline_constants::process_overrides before GLSL emission. Rust wgpu-hal
// GLES calls naga::back::pipeline_constants::process_overrides() in
// create_shader (gles/device.rs:226) and passes the processed module to
// the GLSL writer.
//
// TODO(zero-init-workgroup): Pass desc.Compute.ZeroInitializeWorkgroupMemory
// to naga GLSL options. Rust wgpu-hal sets naga_options.zero_initialize_workgroup_memory
// per-stage (gles/device.rs:268) and stores it in PipelineInner for shader
// cache invalidation (gles/mod.rs:711).
func (d *Device) CreateComputePipeline(desc *ComputePipelineDescriptor) (hal.ComputePipeline, error) {
	if desc == nil {
		return nil, fmt.Errorf("BUG: compute pipeline descriptor is nil in GLES.CreateComputePipeline — core validation gap")
	}

	glCtx := d.ctx.Lock()
	defer d.ctx.Unlock()

	start := time.Now()
	layout, ok := desc.Layout.(*PipelineLayout)
	if !ok {
		return nil, fmt.Errorf("gles: invalid pipeline layout type")
	}

	computeModule, ok := desc.Compute.Module.(*ShaderModule)
	if !ok {
		return nil, fmt.Errorf("gles: invalid compute shader module type")
	}

	// Compile WGSL → GLSL for compute stage.
	computeGLSL, computeTranslationInfo, err := compileWGSLToGLSL(d.glslVersion, computeModule.source, desc.Compute.EntryPoint, layout.bindingMap)
	if err != nil {
		return nil, fmt.Errorf("gles: compute shader: %w", err)
	}

	computeID := glCtx.CreateShader(gl.COMPUTE_SHADER)
	glCtx.ShaderSource(computeID, computeGLSL)
	glCtx.CompileShader(computeID)

	var status int32
	glCtx.GetShaderiv(computeID, gl.COMPILE_STATUS, &status)
	if status == gl.FALSE {
		log := glCtx.GetShaderInfoLog(computeID)
		glCtx.DeleteShader(computeID)
		return nil, fmt.Errorf("gles: compute shader compilation failed: %s", log)
	}
	if infoLog := glCtx.GetShaderInfoLog(computeID); infoLog != "" {
		hal.Logger().Debug("gles: compute shader compile info", "info", infoLog)
	}

	// Link program
	programID := glCtx.CreateProgram()
	glCtx.AttachShader(programID, computeID)
	glCtx.LinkProgram(programID)

	glCtx.GetProgramiv(programID, gl.LINK_STATUS, &status)
	if status == gl.FALSE {
		log := glCtx.GetProgramInfoLog(programID)
		glCtx.DeleteShader(computeID)
		glCtx.DeleteProgram(programID)
		return nil, fmt.Errorf("gles: compute program linking failed: %s", log)
	}
	if infoLog := glCtx.GetProgramInfoLog(programID); infoLog != "" {
		hal.Logger().Debug("gles: compute program link info", "info", infoLog)
	}

	// Runtime binding fallback for GL < 4.2 (see CreateRenderPipeline).
	if !d.shaderBindingLayout {
		if err := assignBindingsAfterLink(glCtx, programID, layout, computeTranslationInfo); err != nil {
			glCtx.DeleteShader(computeID)
			glCtx.DeleteProgram(programID)
			return nil, err
		}
	}

	glCtx.DeleteShader(computeID)

	hal.Logger().Debug("gles: compute pipeline created",
		"programID", programID,
		"entryPoint", desc.Compute.EntryPoint,
		"elapsed", time.Since(start),
	)

	return &ComputePipeline{
		programID: programID,
		layout:    layout,
		glCtx:     glCtx,
	}, nil
}

// DestroyComputePipeline destroys a compute pipeline.
func (d *Device) DestroyComputePipeline(pipeline hal.ComputePipeline) {
	if p, ok := pipeline.(*ComputePipeline); ok {
		p.Destroy()
	}
}

// CreateQuerySet creates a query set with GL query objects.
// Supports QueryTypeTimestamp (requires GL_ARB_timer_query / GL 3.3+).
// Matches Rust wgpu-hal/src/gles/device.rs create_query_set.
func (d *Device) CreateQuerySet(desc *hal.QuerySetDescriptor) (hal.QuerySet, error) {
	if desc == nil {
		return nil, fmt.Errorf("gles: query set descriptor is nil")
	}

	glCtx := d.ctx.Lock()
	defer d.ctx.Unlock()

	// Determine GL query target.
	var target uint32
	switch desc.Type {
	case hal.QueryTypeTimestamp:
		if !glCtx.SupportsTimestampQueries() {
			return nil, hal.ErrTimestampsNotSupported
		}
		target = gl.TIMESTAMP
	default:
		return nil, fmt.Errorf("gles: unsupported query type %d", desc.Type)
	}

	// Create GL query objects.
	queries := make([]uint32, desc.Count)
	for i := uint32(0); i < desc.Count; i++ {
		q := glCtx.GenQueries(1)
		if q == 0 {
			// Clean up already-created queries on failure.
			for j := uint32(0); j < i; j++ {
				glCtx.DeleteQueries(1, &queries[j])
			}
			return nil, fmt.Errorf("gles: failed to create query object %d/%d", i, desc.Count)
		}
		queries[i] = q
	}

	return &QuerySet{
		queries: queries,
		target:  target,
		glCtx:   glCtx,
	}, nil
}

// DestroyQuerySet destroys a query set and releases GL query objects.
func (d *Device) DestroyQuerySet(qs hal.QuerySet) {
	if q, ok := qs.(*QuerySet); ok && q != nil {
		q.Destroy()
	}
}

// CreateCommandEncoder creates a command encoder.
func (d *Device) CreateCommandEncoder(_ *CommandEncoderDescriptor) (hal.CommandEncoder, error) {
	return &CommandEncoder{
		glCtx:           d.ctx.GL(),
		vao:             d.vao,
		maxTextureUnits: d.maxTextureUnits,
	}, nil
}

// CreateFence creates a synchronization fence.
func (d *Device) CreateFence() (hal.Fence, error) {
	return NewFence(d.ctx.GL()), nil
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

// ResetFence resets a fence to the unsignaled state.
func (d *Device) ResetFence(fence hal.Fence) error {
	f, ok := fence.(*Fence)
	if !ok {
		return fmt.Errorf("gles: invalid fence type")
	}
	f.Reset()
	return nil
}

// GetFenceStatus returns true if the fence is signaled (non-blocking).
func (d *Device) GetFenceStatus(fence hal.Fence) (bool, error) {
	f, ok := fence.(*Fence)
	if !ok {
		return false, fmt.Errorf("gles: invalid fence type")
	}
	return f.GetValue() > 0, nil
}

// FreeCommandBuffer is a no-op for GLES.
// GLES doesn't have Vulkan-style command pools - commands are recorded directly.
func (d *Device) FreeCommandBuffer(cmdBuffer hal.CommandBuffer) {
	// GLES command buffers don't need explicit freeing
}

// CreateRenderBundleEncoder is not supported in GLES backend.
func (d *Device) CreateRenderBundleEncoder(desc *hal.RenderBundleEncoderDescriptor) (hal.RenderBundleEncoder, error) {
	return nil, fmt.Errorf("gles: render bundles not supported")
}

// DestroyRenderBundle is not supported in GLES backend.
func (d *Device) DestroyRenderBundle(bundle hal.RenderBundle) {}

// WaitIdle waits for all GPU work to complete.
func (d *Device) WaitIdle() error {
	glCtx := d.ctx.Lock()
	glCtx.Finish()
	d.ctx.Unlock()
	return nil
}

// Destroy releases the device.
func (d *Device) Destroy() {
	if d.vao != 0 {
		glCtx := d.ctx.Lock()
		glCtx.DeleteVertexArrays(d.vao)
		d.ctx.Unlock()
		d.vao = 0
	}
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
		return gl.DEPTH24_STENCIL8, gl.DEPTH_STENCIL, gl.UNSIGNED_INT_24_8
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

// compileFragmentShader compiles a fragment shader from WGSL source via GLSL.
// Caller must hold the AdapterContext lock. glCtx is passed explicitly to avoid
// re-locking (this is called from CreateRenderPipeline which already holds the lock).
func compileFragmentShader(glCtx *gl.Context, version glsl.Version, frag *hal.FragmentState, bindingMap map[glsl.BindingMapKey]uint8) (uint32, glsl.TranslationInfo, error) {
	fragmentModule, ok := frag.Module.(*ShaderModule)
	if !ok {
		return 0, glsl.TranslationInfo{}, fmt.Errorf("gles: invalid fragment shader module type")
	}

	fragmentGLSL, translationInfo, err := compileWGSLToGLSL(version, fragmentModule.source, frag.EntryPoint, bindingMap)
	if err != nil {
		return 0, glsl.TranslationInfo{}, fmt.Errorf("gles: fragment shader: %w", err)
	}

	fragmentID := glCtx.CreateShader(gl.FRAGMENT_SHADER)
	glCtx.ShaderSource(fragmentID, fragmentGLSL)
	glCtx.CompileShader(fragmentID)

	var status int32
	glCtx.GetShaderiv(fragmentID, gl.COMPILE_STATUS, &status)
	if status == gl.FALSE {
		log := glCtx.GetShaderInfoLog(fragmentID)
		glCtx.DeleteShader(fragmentID)
		return 0, glsl.TranslationInfo{}, fmt.Errorf("gles: fragment shader compilation failed: %s", log)
	}
	if infoLog := glCtx.GetShaderInfoLog(fragmentID); infoLog != "" {
		hal.Logger().Debug("gles: fragment shader compile info", "info", infoLog)
	}

	return fragmentID, translationInfo, nil
}

// Ensure we use unsafe for later
var _ = unsafe.Pointer(nil)
