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
	"github.com/gogpu/naga/ir"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/gles/gl"
	"github.com/gogpu/wgpu/internal/handletable"
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

	// fullInstancing is AdapterCapabilities.FullyFeaturedInstancing: draws
	// pass the first instance to GL, and every stage is compiled with naga's
	// glsl.Options.DrawParameters (gl_BaseInstanceARB, gl_DrawIDARB), as Rust
	// sets WriterFlags::DRAW_PARAMETERS from it (gles/device.rs).
	fullInstancing bool

	// cubeArrays is DownlevelFlagsCubeArrayTextures of the adapter: the
	// context can create TEXTURE_CUBE_MAP_ARRAY textures (textureTarget).
	cubeArrays bool

	// depthStencilTextureMode: the context has DEPTH_STENCIL_TEXTURE_MODE (ES
	// 3.1, GL 4.3), which selects the aspect a combined depth-stencil texture
	// samples (bindSampledTexture).
	depthStencilTextureMode bool

	// maxColorAttachments is Limits.MaxColorAttachments of the adapter: the
	// color attachment points a render pass detaches before it attaches its
	// own (resetFramebufferAttachments).
	maxColorAttachments uint32

	// isES is true for an OpenGL ES context: a multisampled texture is then a
	// renderbuffer, which ES has and multisample texture images it lacks
	// (createMultisampleRenderbuffer).
	isES bool

	// immutableStorage is AdapterCapabilities.TextureStorage: 2D and cube
	// textures are allocated with glTexStorage2D, which an ES image binding
	// (glBindImageTexture) requires.
	immutableStorage bool

	// views maps TextureView.NativeHandle values back to the views, for
	// CreateBindGroup (newBindGroup).
	views handletable.Table[TextureView]
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

// DestroyBuffer destroys a GPU buffer under the AdapterContext lock.
func (d *Device) DestroyBuffer(buffer hal.Buffer) {
	b, ok := buffer.(*Buffer)
	if !ok || d.ctx == nil {
		return
	}
	_ = d.ctx.Lock()
	defer d.ctx.Unlock()
	b.Destroy()
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
				// readGLBuffer maps with glMapBufferRange, the entry point
				// OpenGL ES has in place of glMapBuffer.
				copy(buf.mapped, readGLBuffer(glCtx, buf))
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
	// Flush the CPU-side shadow buffer to the GL buffer. This must happen for
	// ALL writable mappings, not only MapWrite buffers. MappedAtCreation works
	// with any buffer usage (Uniform, Vertex, Index, CopyDst) per the WebGPU
	// spec: "mappedAtCreation does not require MAP_WRITE usage." Without this
	// flush, data written via MappedRange is silently discarded and the GL
	// buffer remains zero-filled — uniform buffers get zero matrices, vertex
	// buffers get zero positions, etc.
	if buf.id != 0 {
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
	glFormat, err := describeTextureFormat(desc.Format)
	if err != nil {
		return nil, err
	}

	glCtx := d.ctx.Lock()
	defer d.ctx.Unlock()

	sampleCount := desc.SampleCount
	if sampleCount == 0 {
		sampleCount = 1
	}

	target := textureTarget(desc.Dimension, desc.Size, sampleCount, d.cubeArrays)

	internalFormat, format, dataType := glFormat.internal, glFormat.external, glFormat.dataType

	// A multisampled render target on an ES context is a renderbuffer: ES has
	// no glTexImage2DMultisample (desktop only) and no multisample texture
	// images before ES 3.1. See Texture.renderBuffer and
	// createMultisampleRenderbuffer. No WGL context of this backend is an ES
	// context today; the branch keeps both platform files one definition of
	// what a multisampled texture is.
	if d.isES && sampleCount > 1 {
		rb, err := createMultisampleRenderbuffer(glCtx, sampleCount, internalFormat, desc.Size)
		if err != nil {
			return nil, err
		}
		return &Texture{
			id:           rb,
			target:       target,
			format:       desc.Format,
			dimension:    desc.Dimension,
			size:         desc.Size,
			mipLevels:    desc.MipLevelCount,
			sampleCount:  sampleCount,
			glCtx:        glCtx,
			renderBuffer: true,
		}, nil
	}

	id := glCtx.GenTextures(1)
	glCtx.BindTexture(target, id)

	// Allocate texture storage
	switch {
	case desc.Format.IsCompressed():
		if err := allocateCompressedStorage(glCtx, target, internalFormat, desc.Size, desc.MipLevelCount); err != nil {
			glCtx.BindTexture(target, 0)
			glCtx.DeleteTextures(id)
			return nil, err
		}

	case target == gl.TEXTURE_2D_MULTISAMPLE:
		glCtx.TexImage2DMultisample(target, int32(sampleCount), internalFormat,
			int32(desc.Size.Width), int32(desc.Size.Height), true)

	case target == gl.TEXTURE_2D:
		if d.immutableStorage {
			// Immutable storage, as Rust wgpu-hal allocates a texture with
			// TEXTURE_STORAGE (gles/device.rs create_texture); every later
			// write goes through TexSubImage2D.
			glCtx.TexStorage2D(target, int32(max(desc.MipLevelCount, 1)), internalFormat,
				int32(desc.Size.Width), int32(desc.Size.Height))
			break
		}
		for level := uint32(0); level < desc.MipLevelCount; level++ {
			width := maxInt32(1, int32(desc.Size.Width>>level))
			height := maxInt32(1, int32(desc.Size.Height>>level))
			glCtx.TexImage2D(target, int32(level), int32(internalFormat),
				width, height, 0, format, dataType, nil)
		}

	case target == gl.TEXTURE_CUBE_MAP:
		if d.immutableStorage {
			// One glTexStorage2D allocates every face and level (Rust
			// wgpu-hal gles/device.rs create_texture).
			glCtx.TexStorage2D(target, int32(max(desc.MipLevelCount, 1)), internalFormat,
				int32(desc.Size.Width), int32(desc.Size.Height))
			break
		}
		for face := uint32(0); face < 6; face++ {
			faceTarget := gl.TEXTURE_CUBE_MAP_POSITIVE_X + face
			for level := uint32(0); level < desc.MipLevelCount; level++ {
				width := maxInt32(1, int32(desc.Size.Width>>level))
				height := maxInt32(1, int32(desc.Size.Height>>level))
				glCtx.TexImage2D(faceTarget, int32(level), int32(internalFormat),
					width, height, 0, format, dataType, nil)
			}
		}

	case isLayeredTarget(target):
		// One glTexStorage3D allocates every level when the context has
		// immutable storage; without it every level is allocated with
		// TexImage3D, as Rust wgpu-hal does (gles/device.rs create_texture).
		if !glCtx.HasLayeredTextures() {
			glCtx.BindTexture(target, 0)
			glCtx.DeleteTextures(id)
			return nil, errNoLayeredTextures
		}
		if d.immutableStorage {
			w, h, depth := layeredLevelSize(target, desc.Size, 0)
			glCtx.TexStorage3D(target, int32(max(desc.MipLevelCount, 1)), internalFormat, w, h, depth)
			break
		}
		for level := uint32(0); level < desc.MipLevelCount; level++ {
			w, h, depth := layeredLevelSize(target, desc.Size, level)
			glCtx.TexImage3D(target, int32(level), int32(internalFormat), w, h, depth, 0, format, dataType, nil)
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

// DestroyTexture destroys a GPU texture under the AdapterContext lock.
func (d *Device) DestroyTexture(texture hal.Texture) {
	t, ok := texture.(*Texture)
	if !ok || d.ctx == nil {
		return
	}
	_ = d.ctx.Lock()
	defer d.ctx.Unlock()
	t.Destroy()
}

// CreateTextureView creates a view into a texture.
// Accepts both *Texture and *SurfaceTexture (default framebuffer).
func (d *Device) CreateTextureView(texture hal.Texture, desc *TextureViewDescriptor) (hal.TextureView, error) {
	// Surface texture (default framebuffer) — return a view with no GL texture.
	if st, ok := texture.(*SurfaceTexture); ok {
		view := &TextureView{
			isSurface:  true,
			surfaceTex: st,
			device:     d,
		}
		view.nativeHandle = d.views.Add(view)
		return view, nil
	}

	t, ok := texture.(*Texture)
	if !ok {
		return nil, fmt.Errorf("gles: invalid texture type")
	}

	view := &TextureView{
		texture: t,
		device:  d,
	}
	if desc != nil {
		view.aspect = desc.Aspect
		view.baseMip = desc.BaseMipLevel
		view.mipCount = desc.MipLevelCount
		view.baseLayer = desc.BaseArrayLayer
		view.layerCount = desc.ArrayLayerCount
	}
	view.nativeHandle = d.views.Add(view)
	return view, nil
}

// DestroyTextureView destroys a texture view. It holds no GL object; this
// only drops its handle.
func (d *Device) DestroyTextureView(view hal.TextureView) {
	if v, ok := view.(*TextureView); ok && v != nil {
		v.Destroy()
	}
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

// DestroySampler destroys a sampler under the AdapterContext lock.
func (d *Device) DestroySampler(sampler hal.Sampler) {
	s, ok := sampler.(*Sampler)
	if !ok || d.ctx == nil {
		return
	}
	_ = d.ctx.Lock()
	defer d.ctx.Unlock()
	s.Destroy()
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

	return newBindGroup(&d.views, layout, desc.Entries)
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
	return newShaderModule(desc, d.ctx.GL())
}

// DestroyShaderModule destroys a shader module under the AdapterContext lock.
func (d *Device) DestroyShaderModule(module hal.ShaderModule) {
	m, ok := module.(*ShaderModule)
	if !ok || d.ctx == nil {
		return
	}
	_ = d.ctx.Lock()
	defer d.ctx.Unlock()
	m.Destroy()
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
	vertexGLSL, vertexTranslationInfo, err := compileWGSLToGLSL(d.glslVersion, d.fullInstancing, vertexModule, ir.StageVertex, desc.Vertex.EntryPoint, desc.Vertex.Constants, layout.bindingMap)
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
		fragmentID, fragmentTranslationInfo, err = compileFragmentShader(glCtx, d.glslVersion, d.fullInstancing, desc.Fragment, layout.bindingMap)
		if err != nil {
			glCtx.DeleteShader(vertexID)
			return nil, err
		}
	} else {
		fragmentID, err = compileEmptyFragmentShader(glCtx, d.glslVersion)
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

	// Extract per-target blend/write-mask configuration for MRT.
	// Matches Rust wgpu-hal GLES device.rs:1559-1570: stores ALL color targets
	// so that SetPipeline can apply per-draw-buffer blend state.
	var colorTargets []ColorTargetDesc
	if desc.Fragment != nil {
		colorTargets = make([]ColorTargetDesc, len(desc.Fragment.Targets))
		for i, ct := range desc.Fragment.Targets {
			colorTargets[i] = ColorTargetDesc{
				Blend:     ct.Blend,
				WriteMask: ct.WriteMask,
			}
		}
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
		colorTargets:      colorTargets,
		vertexBuffers:     desc.Vertex.Buffers,
		// -1 when the vertex shader reads no instance_index or uses
		// gl_BaseInstanceARB (Rust wgpu-hal gles/device.rs, FIRST_INSTANCE_BINDING).
		firstInstanceLocation: glCtx.GetUniformLocation(programID, "naga_vs_first_instance"),
	}

	// Pair each combined sampler's texture unit with its sampler slot, over
	// every stage (Rust wgpu-hal gles create_pipeline).
	pipeline.samplerBindMap = buildSamplerBindMap(layout.bindingMap, vertexTranslationInfo, fragmentTranslationInfo)

	return pipeline, nil
}

// DestroyRenderPipeline destroys a render pipeline under the AdapterContext lock.
func (d *Device) DestroyRenderPipeline(pipeline hal.RenderPipeline) {
	p, ok := pipeline.(*RenderPipeline)
	if !ok || d.ctx == nil {
		return
	}
	_ = d.ctx.Lock()
	defer d.ctx.Unlock()
	p.Destroy()
}

// CreateComputePipeline creates a compute pipeline.
//
// desc.Compute.Constants are applied by compileWGSLToGLSL
// (naga.ProcessOverrides before GLSL emission, Rust gles/device.rs:385).
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
	computeGLSL, computeTranslationInfo, err := compileWGSLToGLSL(d.glslVersion, d.fullInstancing, computeModule, ir.StageCompute, desc.Compute.EntryPoint, desc.Compute.Constants, layout.bindingMap)
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
		programID:      programID,
		layout:         layout,
		glCtx:          glCtx,
		samplerBindMap: buildSamplerBindMap(layout.bindingMap, computeTranslationInfo),
	}, nil
}

// DestroyComputePipeline destroys a compute pipeline under the AdapterContext lock.
func (d *Device) DestroyComputePipeline(pipeline hal.ComputePipeline) {
	p, ok := pipeline.(*ComputePipeline)
	if !ok || d.ctx == nil {
		return
	}
	_ = d.ctx.Lock()
	defer d.ctx.Unlock()
	p.Destroy()
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
	q, ok := qs.(*QuerySet)
	if !ok || q == nil || d.ctx == nil {
		return
	}
	_ = d.ctx.Lock()
	defer d.ctx.Unlock()
	q.Destroy()
}

// CreateCommandEncoder creates a command encoder.
func (d *Device) CreateCommandEncoder(_ *CommandEncoderDescriptor) (hal.CommandEncoder, error) {
	return &CommandEncoder{
		glCtx:                   d.ctx.GL(),
		vao:                     d.vao,
		maxTextureUnits:         d.maxTextureUnits,
		fullInstancing:          d.fullInstancing,
		depthStencilTextureMode: d.depthStencilTextureMode,
		maxColorAttachments:     d.maxColorAttachments,
	}, nil
}

// CreateFence creates a synchronization fence.
func (d *Device) CreateFence() (hal.Fence, error) {
	return NewFence(d.ctx.GL()), nil
}

// DestroyFence destroys a fence under the AdapterContext lock.
func (d *Device) DestroyFence(fence hal.Fence) {
	f, ok := fence.(*Fence)
	if !ok || d.ctx == nil {
		return
	}
	_ = d.ctx.Lock()
	defer d.ctx.Unlock()
	f.Destroy()
}

// Wait waits for a fence to reach the specified value.
func (d *Device) Wait(fence hal.Fence, value uint64, timeout time.Duration) (bool, error) {
	f, ok := fence.(*Fence)
	if !ok {
		return false, fmt.Errorf("gles: invalid fence type")
	}
	// glClientWaitSync needs the context current on this OS thread (Rust
	// wgpu-hal locks it in Device::wait, gles/device.rs).
	if d.ctx != nil {
		_ = d.ctx.Lock()
		defer d.ctx.Unlock()
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
	// GetValue polls with glGetSynciv, which needs the context current.
	if d.ctx != nil {
		_ = d.ctx.Lock()
		defer d.ctx.Unlock()
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

// CreateAccelerationStructure returns an error because the GLES backend
// does not support ray tracing. OpenGL ES has no acceleration structure API.
func (d *Device) CreateAccelerationStructure(_ *hal.AccelerationStructureDescriptor) (hal.AccelerationStructure, error) {
	return nil, fmt.Errorf("gles: ray tracing not supported")
}

// DestroyAccelerationStructure is a no-op (GLES has no ray tracing).
func (d *Device) DestroyAccelerationStructure(_ hal.AccelerationStructure) {}

// GetAccelerationStructureBuildSizes returns zero sizes (GLES has no ray tracing).
func (d *Device) GetAccelerationStructureBuildSizes(_ *hal.GetAccelerationStructureBuildSizesDescriptor) hal.AccelerationStructureBuildSizes {
	return hal.AccelerationStructureBuildSizes{}
}

// GetAccelerationStructureDeviceAddress returns 0 (GLES has no ray tracing).
func (d *Device) GetAccelerationStructureDeviceAddress(_ hal.AccelerationStructure) uint64 {
	return 0
}

// TlasInstanceToBytes returns nil (GLES has no ray tracing).
func (d *Device) TlasInstanceToBytes(_ hal.TlasInstance) []byte { return nil }

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
func compileFragmentShader(glCtx *gl.Context, version glsl.Version, drawParameters bool, frag *hal.FragmentState, bindingMap map[ir.ResourceBinding]uint8) (uint32, glsl.TranslationInfo, error) {
	fragmentModule, ok := frag.Module.(*ShaderModule)
	if !ok {
		return 0, glsl.TranslationInfo{}, fmt.Errorf("gles: invalid fragment shader module type")
	}

	fragmentGLSL, translationInfo, err := compileWGSLToGLSL(version, drawParameters, fragmentModule, ir.StageFragment, frag.EntryPoint, frag.Constants, bindingMap)
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
