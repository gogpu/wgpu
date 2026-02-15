// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build linux

package gl

import (
	"unsafe"

	"github.com/go-webgpu/goffi/ffi"
	"github.com/go-webgpu/goffi/types"
)

// Common CallInterface signatures (reused across multiple GL functions)
var (
	cifVoid          types.CallInterface // void fn(void)
	cifUInt32        types.CallInterface // uint32 fn(void)
	cifUInt321       types.CallInterface // uint32 fn(uint32)
	cifInt322        types.CallInterface // int32 fn(uint32, void*)
	cifVoid1         types.CallInterface // void fn(uint32)
	cifVoid2         types.CallInterface // void fn(uint32, void*)
	cifVoid2UU       types.CallInterface // void fn(uint32, uint32)
	cifVoid3         types.CallInterface // void fn(uint32, uint32, uint32)
	cifVoid4         types.CallInterface // void fn(uint32, uint32, uint32, uint32)
	cifVoid4Float    types.CallInterface // void fn(float, float, float, float)
	cifVoid4Shader   types.CallInterface // void fn(uint32, int32, void*, void*)
	cifVoid3Shader   types.CallInterface // void fn(uint32, uint32, void*)
	cifVoid4Log      types.CallInterface // void fn(uint32, uint32, void*, void*)
	cifVoid4Buffer   types.CallInterface // void fn(uint32, uintptr, void*, uint32)
	cifVoid4SubBuf   types.CallInterface // void fn(uint32, uintptr, uintptr, void*)
	cifVoid6Attrib   types.CallInterface // void fn(uint32, int32, uint32, uint8, int32, uintptr)
	cifVoid5FBO      types.CallInterface // void fn(uint32, uint32, uint32, uint32, int32)
	cifVoid9TexImg   types.CallInterface // void fn(uint32, int32, int32, int32, int32, int32, uint32, uint32, void*)
	cifVoid4Draw     types.CallInterface // void fn(uint32, int32, int32, int32)
	cifVoid5DrawElem types.CallInterface // void fn(uint32, int32, uint32, void*, int32)
	cifPtr1          types.CallInterface // void* fn(uint32)
	cifPtr2UU        types.CallInterface // void* fn(uint32, uint32)
	cifInitialized   bool
)

// initCommonCallInterfaces prepares reusable CallInterface signatures.
//
//nolint:maintidx // FFI initialization requires many CallInterface setups
func initCommonCallInterfaces() error {
	if cifInitialized {
		return nil
	}

	var err error

	// void fn(void)
	err = ffi.PrepareCallInterface(&cifVoid, types.DefaultCall,
		types.VoidTypeDescriptor, []*types.TypeDescriptor{})
	if err != nil {
		return err
	}

	// uint32 fn(void)
	err = ffi.PrepareCallInterface(&cifUInt32, types.DefaultCall,
		types.UInt32TypeDescriptor, []*types.TypeDescriptor{})
	if err != nil {
		return err
	}

	// uint32 fn(uint32)
	err = ffi.PrepareCallInterface(&cifUInt321, types.DefaultCall,
		types.UInt32TypeDescriptor,
		[]*types.TypeDescriptor{types.UInt32TypeDescriptor})
	if err != nil {
		return err
	}

	// void fn(uint32)
	err = ffi.PrepareCallInterface(&cifVoid1, types.DefaultCall,
		types.VoidTypeDescriptor,
		[]*types.TypeDescriptor{types.UInt32TypeDescriptor})
	if err != nil {
		return err
	}

	// void fn(uint32, uint32, uint32, uint32)
	err = ffi.PrepareCallInterface(&cifVoid4, types.DefaultCall,
		types.VoidTypeDescriptor,
		[]*types.TypeDescriptor{
			types.UInt32TypeDescriptor,
			types.UInt32TypeDescriptor,
			types.UInt32TypeDescriptor,
			types.UInt32TypeDescriptor,
		})
	if err != nil {
		return err
	}

	// void fn(float, float, float, float)
	err = ffi.PrepareCallInterface(&cifVoid4Float, types.DefaultCall,
		types.VoidTypeDescriptor,
		[]*types.TypeDescriptor{
			types.FloatTypeDescriptor,
			types.FloatTypeDescriptor,
			types.FloatTypeDescriptor,
			types.FloatTypeDescriptor,
		})
	if err != nil {
		return err
	}

	// void* fn(uint32)
	err = ffi.PrepareCallInterface(&cifPtr1, types.DefaultCall,
		types.PointerTypeDescriptor,
		[]*types.TypeDescriptor{types.UInt32TypeDescriptor})
	if err != nil {
		return err
	}

	// void* fn(uint32, uint32) - MapBuffer
	err = ffi.PrepareCallInterface(&cifPtr2UU, types.DefaultCall,
		types.PointerTypeDescriptor,
		[]*types.TypeDescriptor{
			types.UInt32TypeDescriptor,
			types.UInt32TypeDescriptor,
		})
	if err != nil {
		return err
	}

	// void fn(uint32, void*)
	err = ffi.PrepareCallInterface(&cifVoid2, types.DefaultCall,
		types.VoidTypeDescriptor,
		[]*types.TypeDescriptor{
			types.UInt32TypeDescriptor,
			types.PointerTypeDescriptor,
		})
	if err != nil {
		return err
	}

	// void fn(uint32, uint32)
	err = ffi.PrepareCallInterface(&cifVoid2UU, types.DefaultCall,
		types.VoidTypeDescriptor,
		[]*types.TypeDescriptor{
			types.UInt32TypeDescriptor,
			types.UInt32TypeDescriptor,
		})
	if err != nil {
		return err
	}

	// void fn(uint32, uint32, uint32)
	err = ffi.PrepareCallInterface(&cifVoid3, types.DefaultCall,
		types.VoidTypeDescriptor,
		[]*types.TypeDescriptor{
			types.UInt32TypeDescriptor,
			types.UInt32TypeDescriptor,
			types.UInt32TypeDescriptor,
		})
	if err != nil {
		return err
	}

	// int32 fn(uint32, void*)
	err = ffi.PrepareCallInterface(&cifInt322, types.DefaultCall,
		types.SInt32TypeDescriptor,
		[]*types.TypeDescriptor{
			types.UInt32TypeDescriptor,
			types.PointerTypeDescriptor,
		})
	if err != nil {
		return err
	}

	// void fn(uint32, int32, void*, void*) - ShaderSource
	err = ffi.PrepareCallInterface(&cifVoid4Shader, types.DefaultCall,
		types.VoidTypeDescriptor,
		[]*types.TypeDescriptor{
			types.UInt32TypeDescriptor,
			types.SInt32TypeDescriptor,
			types.PointerTypeDescriptor,
			types.PointerTypeDescriptor,
		})
	if err != nil {
		return err
	}

	// void fn(uint32, uint32, void*) - GetShaderiv
	err = ffi.PrepareCallInterface(&cifVoid3Shader, types.DefaultCall,
		types.VoidTypeDescriptor,
		[]*types.TypeDescriptor{
			types.UInt32TypeDescriptor,
			types.UInt32TypeDescriptor,
			types.PointerTypeDescriptor,
		})
	if err != nil {
		return err
	}

	// void fn(uint32, uint32, void*, void*) - GetShaderInfoLog
	err = ffi.PrepareCallInterface(&cifVoid4Log, types.DefaultCall,
		types.VoidTypeDescriptor,
		[]*types.TypeDescriptor{
			types.UInt32TypeDescriptor,
			types.UInt32TypeDescriptor,
			types.PointerTypeDescriptor,
			types.PointerTypeDescriptor,
		})
	if err != nil {
		return err
	}

	// void fn(uint32, uintptr, void*, uint32) - BufferData
	err = ffi.PrepareCallInterface(&cifVoid4Buffer, types.DefaultCall,
		types.VoidTypeDescriptor,
		[]*types.TypeDescriptor{
			types.UInt32TypeDescriptor,
			types.PointerTypeDescriptor, // uintptr = size
			types.PointerTypeDescriptor, // void* = data
			types.UInt32TypeDescriptor,  // usage
		})
	if err != nil {
		return err
	}

	// void fn(uint32, uintptr, uintptr, void*) - BufferSubData
	err = ffi.PrepareCallInterface(&cifVoid4SubBuf, types.DefaultCall,
		types.VoidTypeDescriptor,
		[]*types.TypeDescriptor{
			types.UInt32TypeDescriptor,  // target
			types.PointerTypeDescriptor, // offset
			types.PointerTypeDescriptor, // size
			types.PointerTypeDescriptor, // data
		})
	if err != nil {
		return err
	}

	// void fn(uint32, int32, uint32, uint8, int32, uintptr) - VertexAttribPointer
	err = ffi.PrepareCallInterface(&cifVoid6Attrib, types.DefaultCall,
		types.VoidTypeDescriptor,
		[]*types.TypeDescriptor{
			types.UInt32TypeDescriptor,  // index
			types.SInt32TypeDescriptor,  // size
			types.UInt32TypeDescriptor,  // type
			types.UInt8TypeDescriptor,   // normalized (bool as uint8)
			types.SInt32TypeDescriptor,  // stride
			types.PointerTypeDescriptor, // offset
		})
	if err != nil {
		return err
	}

	// void fn(uint32, uint32, uint32, uint32, int32) - FramebufferTexture2D
	err = ffi.PrepareCallInterface(&cifVoid5FBO, types.DefaultCall,
		types.VoidTypeDescriptor,
		[]*types.TypeDescriptor{
			types.UInt32TypeDescriptor,
			types.UInt32TypeDescriptor,
			types.UInt32TypeDescriptor,
			types.UInt32TypeDescriptor,
			types.SInt32TypeDescriptor,
		})
	if err != nil {
		return err
	}

	// void fn(uint32, int32, int32, int32, int32, int32, uint32, uint32, void*) - TexImage2D
	err = ffi.PrepareCallInterface(&cifVoid9TexImg, types.DefaultCall,
		types.VoidTypeDescriptor,
		[]*types.TypeDescriptor{
			types.UInt32TypeDescriptor,
			types.SInt32TypeDescriptor,
			types.SInt32TypeDescriptor,
			types.SInt32TypeDescriptor,
			types.SInt32TypeDescriptor,
			types.SInt32TypeDescriptor,
			types.UInt32TypeDescriptor,
			types.UInt32TypeDescriptor,
			types.PointerTypeDescriptor,
		})
	if err != nil {
		return err
	}

	// void fn(uint32, int32, int32, int32) - DrawArraysInstanced
	err = ffi.PrepareCallInterface(&cifVoid4Draw, types.DefaultCall,
		types.VoidTypeDescriptor,
		[]*types.TypeDescriptor{
			types.UInt32TypeDescriptor,
			types.SInt32TypeDescriptor,
			types.SInt32TypeDescriptor,
			types.SInt32TypeDescriptor,
		})
	if err != nil {
		return err
	}

	// void fn(uint32, int32, uint32, void*, int32) - DrawElementsInstanced
	err = ffi.PrepareCallInterface(&cifVoid5DrawElem, types.DefaultCall,
		types.VoidTypeDescriptor,
		[]*types.TypeDescriptor{
			types.UInt32TypeDescriptor,
			types.SInt32TypeDescriptor,
			types.UInt32TypeDescriptor,
			types.PointerTypeDescriptor,
			types.SInt32TypeDescriptor,
		})
	if err != nil {
		return err
	}

	cifInitialized = true
	return nil
}

// Context holds OpenGL function pointers loaded at runtime via goffi.
// Functions are loaded via eglGetProcAddress for all OpenGL functions.
type Context struct {
	// Core GL 1.1
	glGetError     unsafe.Pointer
	glGetString    unsafe.Pointer
	glGetIntegerv  unsafe.Pointer
	glEnable       unsafe.Pointer
	glDisable      unsafe.Pointer
	glClear        unsafe.Pointer
	glClearColor   unsafe.Pointer
	glClearDepth   unsafe.Pointer
	glViewport     unsafe.Pointer
	glScissor      unsafe.Pointer
	glDrawArrays   unsafe.Pointer
	glDrawElements unsafe.Pointer
	glFlush        unsafe.Pointer
	glFinish       unsafe.Pointer

	// Shaders (GL 2.0+)
	glCreateShader       unsafe.Pointer
	glDeleteShader       unsafe.Pointer
	glShaderSource       unsafe.Pointer
	glCompileShader      unsafe.Pointer
	glGetShaderiv        unsafe.Pointer
	glGetShaderInfoLog   unsafe.Pointer
	glCreateProgram      unsafe.Pointer
	glDeleteProgram      unsafe.Pointer
	glAttachShader       unsafe.Pointer
	glDetachShader       unsafe.Pointer
	glLinkProgram        unsafe.Pointer
	glUseProgram         unsafe.Pointer
	glGetProgramiv       unsafe.Pointer
	glGetProgramInfoLog  unsafe.Pointer
	glGetUniformLocation unsafe.Pointer
	glGetAttribLocation  unsafe.Pointer

	// Uniforms (GL 2.0+)
	glUniform1i        unsafe.Pointer
	glUniform1f        unsafe.Pointer
	glUniform2f        unsafe.Pointer
	glUniform3f        unsafe.Pointer
	glUniform4f        unsafe.Pointer
	glUniform1iv       unsafe.Pointer
	glUniform1fv       unsafe.Pointer
	glUniform2fv       unsafe.Pointer
	glUniform3fv       unsafe.Pointer
	glUniform4fv       unsafe.Pointer
	glUniformMatrix4fv unsafe.Pointer

	// Buffers (GL 1.5+)
	glGenBuffers    unsafe.Pointer
	glDeleteBuffers unsafe.Pointer
	glBindBuffer    unsafe.Pointer
	glBufferData    unsafe.Pointer
	glBufferSubData unsafe.Pointer
	glMapBuffer     unsafe.Pointer
	glUnmapBuffer   unsafe.Pointer

	// VAO (GL 3.0+)
	glGenVertexArrays    unsafe.Pointer
	glDeleteVertexArrays unsafe.Pointer
	glBindVertexArray    unsafe.Pointer

	// Vertex attributes (GL 2.0+)
	glEnableVertexAttribArray  unsafe.Pointer
	glDisableVertexAttribArray unsafe.Pointer
	glVertexAttribPointer      unsafe.Pointer

	// Textures (GL 1.1+)
	glGenTextures    unsafe.Pointer
	glDeleteTextures unsafe.Pointer
	glBindTexture    unsafe.Pointer
	glActiveTexture  unsafe.Pointer
	glTexImage2D     unsafe.Pointer
	glTexSubImage2D  unsafe.Pointer
	glTexParameteri  unsafe.Pointer
	glGenerateMipmap unsafe.Pointer

	// Framebuffers (GL 3.0+)
	glGenFramebuffers        unsafe.Pointer
	glDeleteFramebuffers     unsafe.Pointer
	glBindFramebuffer        unsafe.Pointer
	glFramebufferTexture2D   unsafe.Pointer
	glCheckFramebufferStatus unsafe.Pointer
	glDrawBuffers            unsafe.Pointer

	// Renderbuffers (GL 3.0+)
	glGenRenderbuffers        unsafe.Pointer
	glDeleteRenderbuffers     unsafe.Pointer
	glBindRenderbuffer        unsafe.Pointer
	glRenderbufferStorage     unsafe.Pointer
	glFramebufferRenderbuffer unsafe.Pointer

	// Blending (GL 1.4+)
	glBlendFunc             unsafe.Pointer
	glBlendFuncSeparate     unsafe.Pointer
	glBlendEquation         unsafe.Pointer
	glBlendEquationSeparate unsafe.Pointer
	glBlendColor            unsafe.Pointer

	// Depth/Stencil
	glDepthFunc   unsafe.Pointer
	glDepthMask   unsafe.Pointer
	glDepthRange  unsafe.Pointer
	glStencilFunc unsafe.Pointer
	glStencilOp   unsafe.Pointer
	glStencilMask unsafe.Pointer

	// Face culling
	glCullFace  unsafe.Pointer
	glFrontFace unsafe.Pointer

	// Sync (GL 3.2+)
	glFenceSync      unsafe.Pointer
	glDeleteSync     unsafe.Pointer
	glClientWaitSync unsafe.Pointer
	glWaitSync       unsafe.Pointer

	// UBO (GL 3.1+)
	glBindBufferBase       unsafe.Pointer
	glBindBufferRange      unsafe.Pointer
	glGetUniformBlockIndex unsafe.Pointer
	glUniformBlockBinding  unsafe.Pointer

	// Instancing (GL 3.1+)
	glDrawArraysInstanced   unsafe.Pointer
	glDrawElementsInstanced unsafe.Pointer
	glVertexAttribDivisor   unsafe.Pointer

	// Compute shaders (GL 4.3+ / ES 3.1+)
	glDispatchCompute         unsafe.Pointer
	glDispatchComputeIndirect unsafe.Pointer
	glMemoryBarrier           unsafe.Pointer
}

// ProcAddressFunc is a function that returns the address of an OpenGL function.
type ProcAddressFunc func(name string) unsafe.Pointer

// Load loads all OpenGL function pointers using the provided loader.
func (c *Context) Load(getProcAddr ProcAddressFunc) error {
	// Initialize common CallInterfaces
	if err := initCommonCallInterfaces(); err != nil {
		return err
	}

	// Core GL 1.1
	c.glGetError = getProcAddr("glGetError")
	c.glGetString = getProcAddr("glGetString")
	c.glGetIntegerv = getProcAddr("glGetIntegerv")
	c.glEnable = getProcAddr("glEnable")
	c.glDisable = getProcAddr("glDisable")
	c.glClear = getProcAddr("glClear")
	c.glClearColor = getProcAddr("glClearColor")
	c.glClearDepth = getProcAddr("glClearDepth")
	c.glViewport = getProcAddr("glViewport")
	c.glScissor = getProcAddr("glScissor")
	c.glDrawArrays = getProcAddr("glDrawArrays")
	c.glDrawElements = getProcAddr("glDrawElements")
	c.glFlush = getProcAddr("glFlush")
	c.glFinish = getProcAddr("glFinish")

	// Shaders
	c.glCreateShader = getProcAddr("glCreateShader")
	c.glDeleteShader = getProcAddr("glDeleteShader")
	c.glShaderSource = getProcAddr("glShaderSource")
	c.glCompileShader = getProcAddr("glCompileShader")
	c.glGetShaderiv = getProcAddr("glGetShaderiv")
	c.glGetShaderInfoLog = getProcAddr("glGetShaderInfoLog")
	c.glCreateProgram = getProcAddr("glCreateProgram")
	c.glDeleteProgram = getProcAddr("glDeleteProgram")
	c.glAttachShader = getProcAddr("glAttachShader")
	c.glDetachShader = getProcAddr("glDetachShader")
	c.glLinkProgram = getProcAddr("glLinkProgram")
	c.glUseProgram = getProcAddr("glUseProgram")
	c.glGetProgramiv = getProcAddr("glGetProgramiv")
	c.glGetProgramInfoLog = getProcAddr("glGetProgramInfoLog")
	c.glGetUniformLocation = getProcAddr("glGetUniformLocation")
	c.glGetAttribLocation = getProcAddr("glGetAttribLocation")

	// Uniforms
	c.glUniform1i = getProcAddr("glUniform1i")
	c.glUniform1f = getProcAddr("glUniform1f")
	c.glUniform2f = getProcAddr("glUniform2f")
	c.glUniform3f = getProcAddr("glUniform3f")
	c.glUniform4f = getProcAddr("glUniform4f")
	c.glUniform1iv = getProcAddr("glUniform1iv")
	c.glUniform1fv = getProcAddr("glUniform1fv")
	c.glUniform2fv = getProcAddr("glUniform2fv")
	c.glUniform3fv = getProcAddr("glUniform3fv")
	c.glUniform4fv = getProcAddr("glUniform4fv")
	c.glUniformMatrix4fv = getProcAddr("glUniformMatrix4fv")

	// Buffers
	c.glGenBuffers = getProcAddr("glGenBuffers")
	c.glDeleteBuffers = getProcAddr("glDeleteBuffers")
	c.glBindBuffer = getProcAddr("glBindBuffer")
	c.glBufferData = getProcAddr("glBufferData")
	c.glBufferSubData = getProcAddr("glBufferSubData")
	c.glMapBuffer = getProcAddr("glMapBuffer")
	c.glUnmapBuffer = getProcAddr("glUnmapBuffer")

	// VAO
	c.glGenVertexArrays = getProcAddr("glGenVertexArrays")
	c.glDeleteVertexArrays = getProcAddr("glDeleteVertexArrays")
	c.glBindVertexArray = getProcAddr("glBindVertexArray")

	// Vertex attributes
	c.glEnableVertexAttribArray = getProcAddr("glEnableVertexAttribArray")
	c.glDisableVertexAttribArray = getProcAddr("glDisableVertexAttribArray")
	c.glVertexAttribPointer = getProcAddr("glVertexAttribPointer")

	// Textures
	c.glGenTextures = getProcAddr("glGenTextures")
	c.glDeleteTextures = getProcAddr("glDeleteTextures")
	c.glBindTexture = getProcAddr("glBindTexture")
	c.glActiveTexture = getProcAddr("glActiveTexture")
	c.glTexImage2D = getProcAddr("glTexImage2D")
	c.glTexSubImage2D = getProcAddr("glTexSubImage2D")
	c.glTexParameteri = getProcAddr("glTexParameteri")
	c.glGenerateMipmap = getProcAddr("glGenerateMipmap")

	// Framebuffers
	c.glGenFramebuffers = getProcAddr("glGenFramebuffers")
	c.glDeleteFramebuffers = getProcAddr("glDeleteFramebuffers")
	c.glBindFramebuffer = getProcAddr("glBindFramebuffer")
	c.glFramebufferTexture2D = getProcAddr("glFramebufferTexture2D")
	c.glCheckFramebufferStatus = getProcAddr("glCheckFramebufferStatus")
	c.glDrawBuffers = getProcAddr("glDrawBuffers")

	// Renderbuffers
	c.glGenRenderbuffers = getProcAddr("glGenRenderbuffers")
	c.glDeleteRenderbuffers = getProcAddr("glDeleteRenderbuffers")
	c.glBindRenderbuffer = getProcAddr("glBindRenderbuffer")
	c.glRenderbufferStorage = getProcAddr("glRenderbufferStorage")
	c.glFramebufferRenderbuffer = getProcAddr("glFramebufferRenderbuffer")

	// Blending
	c.glBlendFunc = getProcAddr("glBlendFunc")
	c.glBlendFuncSeparate = getProcAddr("glBlendFuncSeparate")
	c.glBlendEquation = getProcAddr("glBlendEquation")
	c.glBlendEquationSeparate = getProcAddr("glBlendEquationSeparate")
	c.glBlendColor = getProcAddr("glBlendColor")

	// Depth/Stencil
	c.glDepthFunc = getProcAddr("glDepthFunc")
	c.glDepthMask = getProcAddr("glDepthMask")
	c.glDepthRange = getProcAddr("glDepthRange")
	c.glStencilFunc = getProcAddr("glStencilFunc")
	c.glStencilOp = getProcAddr("glStencilOp")
	c.glStencilMask = getProcAddr("glStencilMask")

	// Face culling
	c.glCullFace = getProcAddr("glCullFace")
	c.glFrontFace = getProcAddr("glFrontFace")

	// Sync
	c.glFenceSync = getProcAddr("glFenceSync")
	c.glDeleteSync = getProcAddr("glDeleteSync")
	c.glClientWaitSync = getProcAddr("glClientWaitSync")
	c.glWaitSync = getProcAddr("glWaitSync")

	// UBO
	c.glBindBufferBase = getProcAddr("glBindBufferBase")
	c.glBindBufferRange = getProcAddr("glBindBufferRange")
	c.glGetUniformBlockIndex = getProcAddr("glGetUniformBlockIndex")
	c.glUniformBlockBinding = getProcAddr("glUniformBlockBinding")

	// Instancing
	c.glDrawArraysInstanced = getProcAddr("glDrawArraysInstanced")
	c.glDrawElementsInstanced = getProcAddr("glDrawElementsInstanced")
	c.glVertexAttribDivisor = getProcAddr("glVertexAttribDivisor")

	// Compute shaders (optional - may be nil on older GL versions)
	c.glDispatchCompute = getProcAddr("glDispatchCompute")
	c.glDispatchComputeIndirect = getProcAddr("glDispatchComputeIndirect")
	c.glMemoryBarrier = getProcAddr("glMemoryBarrier")

	return nil
}

// --- GL Function Wrappers ---
// These use goffi CallFunction to call the loaded function pointers

func (c *Context) GetError() uint32 {
	var result uint32
	_ = ffi.CallFunction(&cifUInt32, c.glGetError, unsafe.Pointer(&result), nil)
	return result
}

func (c *Context) GetString(name uint32) string {
	var ptr uintptr
	args := [1]unsafe.Pointer{unsafe.Pointer(&name)}
	_ = ffi.CallFunction(&cifPtr1, c.glGetString, unsafe.Pointer(&ptr), args[:])
	if ptr == 0 {
		return ""
	}
	return goString(ptr)
}

func (c *Context) GetIntegerv(pname uint32, data *int32) {
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&pname),
		unsafe.Pointer(data),
	}
	_ = ffi.CallFunction(&cifVoid2, c.glGetIntegerv, nil, args[:])
}

func (c *Context) Enable(capability uint32) {
	args := [1]unsafe.Pointer{unsafe.Pointer(&capability)}
	_ = ffi.CallFunction(&cifVoid1, c.glEnable, nil, args[:])
}

func (c *Context) Disable(capability uint32) {
	args := [1]unsafe.Pointer{unsafe.Pointer(&capability)}
	_ = ffi.CallFunction(&cifVoid1, c.glDisable, nil, args[:])
}

func (c *Context) Clear(mask uint32) {
	args := [1]unsafe.Pointer{unsafe.Pointer(&mask)}
	_ = ffi.CallFunction(&cifVoid1, c.glClear, nil, args[:])
}

func (c *Context) ClearColor(r, g, b, a float32) {
	args := [4]unsafe.Pointer{
		unsafe.Pointer(&r),
		unsafe.Pointer(&g),
		unsafe.Pointer(&b),
		unsafe.Pointer(&a),
	}
	_ = ffi.CallFunction(&cifVoid4Float, c.glClearColor, nil, args[:])
}

func (c *Context) Viewport(x, y, width, height int32) {
	// Convert int32 to uint32 for API compatibility
	ux, uy, uw, uh := uint32(x), uint32(y), uint32(width), uint32(height)
	args := [4]unsafe.Pointer{
		unsafe.Pointer(&ux),
		unsafe.Pointer(&uy),
		unsafe.Pointer(&uw),
		unsafe.Pointer(&uh),
	}
	_ = ffi.CallFunction(&cifVoid4, c.glViewport, nil, args[:])
}

func (c *Context) Scissor(x, y, width, height int32) {
	// Convert int32 to uint32 for API compatibility
	ux, uy, uw, uh := uint32(x), uint32(y), uint32(width), uint32(height)
	args := [4]unsafe.Pointer{
		unsafe.Pointer(&ux),
		unsafe.Pointer(&uy),
		unsafe.Pointer(&uw),
		unsafe.Pointer(&uh),
	}
	_ = ffi.CallFunction(&cifVoid4, c.glScissor, nil, args[:])
}

func (c *Context) DrawArrays(mode uint32, first, count int32) {
	ufirst, ucount := uint32(first), uint32(count)
	args := [3]unsafe.Pointer{
		unsafe.Pointer(&mode),
		unsafe.Pointer(&ufirst),
		unsafe.Pointer(&ucount),
	}
	// Need a 3-arg void signature
	var cif3 types.CallInterface
	_ = ffi.PrepareCallInterface(&cif3, types.DefaultCall,
		types.VoidTypeDescriptor,
		[]*types.TypeDescriptor{
			types.UInt32TypeDescriptor,
			types.UInt32TypeDescriptor,
			types.UInt32TypeDescriptor,
		})
	_ = ffi.CallFunction(&cif3, c.glDrawArrays, nil, args[:])
}

func (c *Context) DrawElements(mode uint32, count int32, typ uint32, indices uintptr) {
	ucount := uint32(count)
	args := [4]unsafe.Pointer{
		unsafe.Pointer(&mode),
		unsafe.Pointer(&ucount),
		unsafe.Pointer(&typ),
		unsafe.Pointer(&indices),
	}
	_ = ffi.CallFunction(&cifVoid4, c.glDrawElements, nil, args[:])
}

func (c *Context) Flush() {
	_ = ffi.CallFunction(&cifVoid, c.glFlush, nil, nil)
}

func (c *Context) Finish() {
	_ = ffi.CallFunction(&cifVoid, c.glFinish, nil, nil)
}

// --- Shaders ---

func (c *Context) CreateShader(shaderType uint32) uint32 {
	var result uint32
	args := [1]unsafe.Pointer{unsafe.Pointer(&shaderType)}
	_ = ffi.CallFunction(&cifUInt321, c.glCreateShader, unsafe.Pointer(&result), args[:])
	return result
}

func (c *Context) DeleteShader(shader uint32) {
	args := [1]unsafe.Pointer{unsafe.Pointer(&shader)}
	_ = ffi.CallFunction(&cifVoid1, c.glDeleteShader, nil, args[:])
}

func (c *Context) ShaderSource(shader uint32, source string) {
	csource, free := cString(source)
	defer free()
	count := int32(1)
	length := int32(len(source))
	args := [4]unsafe.Pointer{
		unsafe.Pointer(&shader),
		unsafe.Pointer(&count),
		unsafe.Pointer(&csource),
		unsafe.Pointer(&length),
	}
	_ = ffi.CallFunction(&cifVoid4Shader, c.glShaderSource, nil, args[:])
}

func (c *Context) CompileShader(shader uint32) {
	args := [1]unsafe.Pointer{unsafe.Pointer(&shader)}
	_ = ffi.CallFunction(&cifVoid1, c.glCompileShader, nil, args[:])
}

func (c *Context) GetShaderiv(shader uint32, pname uint32, params *int32) {
	args := [3]unsafe.Pointer{
		unsafe.Pointer(&shader),
		unsafe.Pointer(&pname),
		unsafe.Pointer(params),
	}
	_ = ffi.CallFunction(&cifVoid3Shader, c.glGetShaderiv, nil, args[:])
}

func (c *Context) GetShaderInfoLog(shader uint32) string {
	var length int32
	c.GetShaderiv(shader, INFO_LOG_LENGTH, &length)
	if length == 0 {
		return ""
	}
	buf := make([]byte, length)
	maxLen := uint32(length)
	args := [4]unsafe.Pointer{
		unsafe.Pointer(&shader),
		unsafe.Pointer(&maxLen),
		unsafe.Pointer(&length),
		unsafe.Pointer(&buf[0]),
	}
	_ = ffi.CallFunction(&cifVoid4Log, c.glGetShaderInfoLog, nil, args[:])
	return string(buf[:length])
}

func (c *Context) CreateProgram() uint32 {
	var result uint32
	_ = ffi.CallFunction(&cifUInt32, c.glCreateProgram, unsafe.Pointer(&result), nil)
	return result
}

func (c *Context) DeleteProgram(program uint32) {
	args := [1]unsafe.Pointer{unsafe.Pointer(&program)}
	_ = ffi.CallFunction(&cifVoid1, c.glDeleteProgram, nil, args[:])
}

func (c *Context) AttachShader(program, shader uint32) {
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&program),
		unsafe.Pointer(&shader),
	}
	_ = ffi.CallFunction(&cifVoid2UU, c.glAttachShader, nil, args[:])
}

func (c *Context) LinkProgram(program uint32) {
	args := [1]unsafe.Pointer{unsafe.Pointer(&program)}
	_ = ffi.CallFunction(&cifVoid1, c.glLinkProgram, nil, args[:])
}

func (c *Context) UseProgram(program uint32) {
	args := [1]unsafe.Pointer{unsafe.Pointer(&program)}
	_ = ffi.CallFunction(&cifVoid1, c.glUseProgram, nil, args[:])
}

func (c *Context) GetProgramiv(program uint32, pname uint32, params *int32) {
	args := [3]unsafe.Pointer{
		unsafe.Pointer(&program),
		unsafe.Pointer(&pname),
		unsafe.Pointer(params),
	}
	_ = ffi.CallFunction(&cifVoid3Shader, c.glGetProgramiv, nil, args[:])
}

func (c *Context) GetProgramInfoLog(program uint32) string {
	var length int32
	c.GetProgramiv(program, INFO_LOG_LENGTH, &length)
	if length == 0 {
		return ""
	}
	buf := make([]byte, length)
	maxLen := uint32(length)
	args := [4]unsafe.Pointer{
		unsafe.Pointer(&program),
		unsafe.Pointer(&maxLen),
		unsafe.Pointer(&length),
		unsafe.Pointer(&buf[0]),
	}
	_ = ffi.CallFunction(&cifVoid4Log, c.glGetProgramInfoLog, nil, args[:])
	return string(buf[:length])
}

func (c *Context) GetUniformLocation(program uint32, name string) int32 {
	cname, free := cString(name)
	defer free()
	var result int32
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&program),
		unsafe.Pointer(cname),
	}
	_ = ffi.CallFunction(&cifInt322, c.glGetUniformLocation, unsafe.Pointer(&result), args[:])
	return result
}

func (c *Context) GetAttribLocation(program uint32, name string) int32 {
	cname, free := cString(name)
	defer free()
	var result int32
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&program),
		unsafe.Pointer(cname),
	}
	_ = ffi.CallFunction(&cifInt322, c.glGetAttribLocation, unsafe.Pointer(&result), args[:])
	return result
}

// --- Buffers ---

func (c *Context) GenBuffers(n int32) uint32 {
	var buffer uint32
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&n),
		unsafe.Pointer(&buffer),
	}
	_ = ffi.CallFunction(&cifVoid2, c.glGenBuffers, nil, args[:])
	return buffer
}

func (c *Context) DeleteBuffers(buffers ...uint32) {
	n := int32(len(buffers))
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&n),
		unsafe.Pointer(&buffers[0]),
	}
	_ = ffi.CallFunction(&cifVoid2, c.glDeleteBuffers, nil, args[:])
}

func (c *Context) BindBuffer(target, buffer uint32) {
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&target),
		unsafe.Pointer(&buffer),
	}
	_ = ffi.CallFunction(&cifVoid2UU, c.glBindBuffer, nil, args[:])
}

func (c *Context) BufferData(target uint32, size int, data uintptr, usage uint32) {
	sizePtr := uintptr(size)
	args := [4]unsafe.Pointer{
		unsafe.Pointer(&target),
		unsafe.Pointer(&sizePtr),
		unsafe.Pointer(data), //nolint:govet // FFI requires uintptr-to-pointer conversion
		unsafe.Pointer(&usage),
	}
	_ = ffi.CallFunction(&cifVoid4Buffer, c.glBufferData, nil, args[:])
}

func (c *Context) BufferSubData(target uint32, offset, size int, data uintptr) {
	offsetPtr := uintptr(offset)
	sizePtr := uintptr(size)
	args := [4]unsafe.Pointer{
		unsafe.Pointer(&target),
		unsafe.Pointer(&offsetPtr),
		unsafe.Pointer(&sizePtr),
		unsafe.Pointer(data), //nolint:govet // FFI requires uintptr-to-pointer conversion
	}
	_ = ffi.CallFunction(&cifVoid4SubBuf, c.glBufferSubData, nil, args[:])
}

// MapBuffer maps a buffer object's data store into the client's address space.
// target: GL_ARRAY_BUFFER, GL_COPY_READ_BUFFER, etc.
// access: GL_READ_ONLY, GL_WRITE_ONLY, GL_READ_WRITE.
// Returns the mapped pointer, or 0 if glMapBuffer is not available or the call fails.
func (c *Context) MapBuffer(target, access uint32) uintptr {
	if c.glMapBuffer == nil {
		return 0
	}
	var ptr uintptr
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&target),
		unsafe.Pointer(&access),
	}
	_ = ffi.CallFunction(&cifPtr2UU, c.glMapBuffer, unsafe.Pointer(&ptr), args[:])
	return ptr
}

// UnmapBuffer releases the mapping of a buffer object's data store.
// Returns true if the buffer was successfully unmapped, false if the buffer contents
// became corrupt during the mapping (GL_FALSE from driver) or glUnmapBuffer is unavailable.
func (c *Context) UnmapBuffer(target uint32) bool {
	if c.glUnmapBuffer == nil {
		return false
	}
	var result uint32
	args := [1]unsafe.Pointer{unsafe.Pointer(&target)}
	_ = ffi.CallFunction(&cifUInt321, c.glUnmapBuffer, unsafe.Pointer(&result), args[:])
	return result != 0
}

// --- VAO ---

func (c *Context) GenVertexArrays(n int32) uint32 {
	var vao uint32
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&n),
		unsafe.Pointer(&vao),
	}
	_ = ffi.CallFunction(&cifVoid2, c.glGenVertexArrays, nil, args[:])
	return vao
}

func (c *Context) DeleteVertexArrays(arrays ...uint32) {
	n := int32(len(arrays))
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&n),
		unsafe.Pointer(&arrays[0]),
	}
	_ = ffi.CallFunction(&cifVoid2, c.glDeleteVertexArrays, nil, args[:])
}

func (c *Context) BindVertexArray(array uint32) {
	args := [1]unsafe.Pointer{unsafe.Pointer(&array)}
	_ = ffi.CallFunction(&cifVoid1, c.glBindVertexArray, nil, args[:])
}

// --- Vertex Attributes ---

func (c *Context) EnableVertexAttribArray(index uint32) {
	args := [1]unsafe.Pointer{unsafe.Pointer(&index)}
	_ = ffi.CallFunction(&cifVoid1, c.glEnableVertexAttribArray, nil, args[:])
}

func (c *Context) DisableVertexAttribArray(index uint32) {
	args := [1]unsafe.Pointer{unsafe.Pointer(&index)}
	_ = ffi.CallFunction(&cifVoid1, c.glDisableVertexAttribArray, nil, args[:])
}

func (c *Context) VertexAttribPointer(index uint32, size int32, typ uint32, normalized bool, stride int32, offset uintptr) {
	var norm uint8
	if normalized {
		norm = 1
	}
	args := [6]unsafe.Pointer{
		unsafe.Pointer(&index),
		unsafe.Pointer(&size),
		unsafe.Pointer(&typ),
		unsafe.Pointer(&norm),
		unsafe.Pointer(&stride),
		unsafe.Pointer(&offset),
	}
	_ = ffi.CallFunction(&cifVoid6Attrib, c.glVertexAttribPointer, nil, args[:])
}

// --- Textures ---

func (c *Context) GenTextures(n int32) uint32 {
	var tex uint32
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&n),
		unsafe.Pointer(&tex),
	}
	_ = ffi.CallFunction(&cifVoid2, c.glGenTextures, nil, args[:])
	return tex
}

func (c *Context) DeleteTextures(textures ...uint32) {
	n := int32(len(textures))
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&n),
		unsafe.Pointer(&textures[0]),
	}
	_ = ffi.CallFunction(&cifVoid2, c.glDeleteTextures, nil, args[:])
}

func (c *Context) BindTexture(target, texture uint32) {
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&target),
		unsafe.Pointer(&texture),
	}
	_ = ffi.CallFunction(&cifVoid2UU, c.glBindTexture, nil, args[:])
}

func (c *Context) ActiveTexture(texture uint32) {
	args := [1]unsafe.Pointer{unsafe.Pointer(&texture)}
	_ = ffi.CallFunction(&cifVoid1, c.glActiveTexture, nil, args[:])
}

func (c *Context) TexParameteri(target, pname uint32, param int32) {
	args := [3]unsafe.Pointer{
		unsafe.Pointer(&target),
		unsafe.Pointer(&pname),
		unsafe.Pointer(&param),
	}
	_ = ffi.CallFunction(&cifVoid3, c.glTexParameteri, nil, args[:])
}

func (c *Context) TexImage2D(target uint32, level int32, internalformat int32, width, height int32, border int32, format, typ uint32, pixels uintptr) {
	args := [9]unsafe.Pointer{
		unsafe.Pointer(&target),
		unsafe.Pointer(&level),
		unsafe.Pointer(&internalformat),
		unsafe.Pointer(&width),
		unsafe.Pointer(&height),
		unsafe.Pointer(&border),
		unsafe.Pointer(&format),
		unsafe.Pointer(&typ),
		unsafe.Pointer(pixels), //nolint:govet // FFI requires uintptr-to-pointer conversion
	}
	_ = ffi.CallFunction(&cifVoid9TexImg, c.glTexImage2D, nil, args[:])
}

func (c *Context) GenerateMipmap(target uint32) {
	args := [1]unsafe.Pointer{unsafe.Pointer(&target)}
	_ = ffi.CallFunction(&cifVoid1, c.glGenerateMipmap, nil, args[:])
}

// --- Framebuffers ---

func (c *Context) GenFramebuffers(n int32) uint32 {
	var fbo uint32
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&n),
		unsafe.Pointer(&fbo),
	}
	_ = ffi.CallFunction(&cifVoid2, c.glGenFramebuffers, nil, args[:])
	return fbo
}

func (c *Context) DeleteFramebuffers(framebuffers ...uint32) {
	n := int32(len(framebuffers))
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&n),
		unsafe.Pointer(&framebuffers[0]),
	}
	_ = ffi.CallFunction(&cifVoid2, c.glDeleteFramebuffers, nil, args[:])
}

func (c *Context) BindFramebuffer(target, framebuffer uint32) {
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&target),
		unsafe.Pointer(&framebuffer),
	}
	_ = ffi.CallFunction(&cifVoid2UU, c.glBindFramebuffer, nil, args[:])
}

func (c *Context) FramebufferTexture2D(target, attachment, textarget, texture uint32, level int32) {
	args := [5]unsafe.Pointer{
		unsafe.Pointer(&target),
		unsafe.Pointer(&attachment),
		unsafe.Pointer(&textarget),
		unsafe.Pointer(&texture),
		unsafe.Pointer(&level),
	}
	_ = ffi.CallFunction(&cifVoid5FBO, c.glFramebufferTexture2D, nil, args[:])
}

func (c *Context) CheckFramebufferStatus(target uint32) uint32 {
	var result uint32
	args := [1]unsafe.Pointer{unsafe.Pointer(&target)}
	_ = ffi.CallFunction(&cifUInt321, c.glCheckFramebufferStatus, unsafe.Pointer(&result), args[:])
	return result
}

// --- Blending ---

func (c *Context) BlendFunc(sfactor, dfactor uint32) {
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&sfactor),
		unsafe.Pointer(&dfactor),
	}
	_ = ffi.CallFunction(&cifVoid2UU, c.glBlendFunc, nil, args[:])
}

func (c *Context) BlendFuncSeparate(srcRGB, dstRGB, srcAlpha, dstAlpha uint32) {
	args := [4]unsafe.Pointer{
		unsafe.Pointer(&srcRGB),
		unsafe.Pointer(&dstRGB),
		unsafe.Pointer(&srcAlpha),
		unsafe.Pointer(&dstAlpha),
	}
	_ = ffi.CallFunction(&cifVoid4, c.glBlendFuncSeparate, nil, args[:])
}

func (c *Context) BlendEquation(mode uint32) {
	args := [1]unsafe.Pointer{unsafe.Pointer(&mode)}
	_ = ffi.CallFunction(&cifVoid1, c.glBlendEquation, nil, args[:])
}

// BlendEquationSeparate sets separate blend equations for RGB and alpha.
func (c *Context) BlendEquationSeparate(modeRGB, modeAlpha uint32) {
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&modeRGB),
		unsafe.Pointer(&modeAlpha),
	}
	_ = ffi.CallFunction(&cifVoid2UU, c.glBlendEquationSeparate, nil, args[:])
}

// BlendColor sets the constant blend color.
func (c *Context) BlendColor(r, g, b, a float32) {
	args := [4]unsafe.Pointer{
		unsafe.Pointer(&r),
		unsafe.Pointer(&g),
		unsafe.Pointer(&b),
		unsafe.Pointer(&a),
	}
	_ = ffi.CallFunction(&cifVoid4Float, c.glBlendColor, nil, args[:])
}

// --- UBO ---

// BindBufferBase binds a buffer to an indexed binding point.
func (c *Context) BindBufferBase(target, index, buffer uint32) {
	args := [3]unsafe.Pointer{
		unsafe.Pointer(&target),
		unsafe.Pointer(&index),
		unsafe.Pointer(&buffer),
	}
	_ = ffi.CallFunction(&cifVoid3, c.glBindBufferBase, nil, args[:])
}

// BindBufferRange binds a range of a buffer to an indexed binding point.
func (c *Context) BindBufferRange(target, index, buffer uint32, offset, size int) {
	o := int32(offset)
	s := int32(size)
	args := [5]unsafe.Pointer{
		unsafe.Pointer(&target),
		unsafe.Pointer(&index),
		unsafe.Pointer(&buffer),
		unsafe.Pointer(&o),
		unsafe.Pointer(&s),
	}
	_ = ffi.CallFunction(&cifVoid5FBO, c.glBindBufferRange, nil, args[:])
}

// GetUniformBlockIndex returns the index of a named uniform block.
func (c *Context) GetUniformBlockIndex(program uint32, name string) uint32 {
	cname, free := cString(name)
	defer free()
	var result int32
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&program),
		unsafe.Pointer(cname),
	}
	_ = ffi.CallFunction(&cifInt322, c.glGetUniformBlockIndex, unsafe.Pointer(&result), args[:])
	return uint32(result)
}

// UniformBlockBinding assigns a uniform block to a binding point.
func (c *Context) UniformBlockBinding(program, blockIndex, blockBinding uint32) {
	args := [3]unsafe.Pointer{
		unsafe.Pointer(&program),
		unsafe.Pointer(&blockIndex),
		unsafe.Pointer(&blockBinding),
	}
	_ = ffi.CallFunction(&cifVoid3, c.glUniformBlockBinding, nil, args[:])
}

// --- Uniforms ---

// Uniform1i sets an integer uniform value.
func (c *Context) Uniform1i(location, value int32) {
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&location),
		unsafe.Pointer(&value),
	}
	_ = ffi.CallFunction(&cifVoid2UU, c.glUniform1i, nil, args[:])
}

// --- Depth/Stencil ---

func (c *Context) DepthFunc(fn uint32) {
	args := [1]unsafe.Pointer{unsafe.Pointer(&fn)}
	_ = ffi.CallFunction(&cifVoid1, c.glDepthFunc, nil, args[:])
}

func (c *Context) DepthMask(flag bool) {
	var f uint32
	if flag {
		f = 1
	}
	args := [1]unsafe.Pointer{unsafe.Pointer(&f)}
	_ = ffi.CallFunction(&cifVoid1, c.glDepthMask, nil, args[:])
}

// --- Face Culling ---

func (c *Context) CullFace(mode uint32) {
	args := [1]unsafe.Pointer{unsafe.Pointer(&mode)}
	_ = ffi.CallFunction(&cifVoid1, c.glCullFace, nil, args[:])
}

func (c *Context) FrontFace(mode uint32) {
	args := [1]unsafe.Pointer{unsafe.Pointer(&mode)}
	_ = ffi.CallFunction(&cifVoid1, c.glFrontFace, nil, args[:])
}

// --- Instancing ---

func (c *Context) DrawArraysInstanced(mode uint32, first, count, instanceCount int32) {
	args := [4]unsafe.Pointer{
		unsafe.Pointer(&mode),
		unsafe.Pointer(&first),
		unsafe.Pointer(&count),
		unsafe.Pointer(&instanceCount),
	}
	_ = ffi.CallFunction(&cifVoid4Draw, c.glDrawArraysInstanced, nil, args[:])
}

func (c *Context) DrawElementsInstanced(mode uint32, count int32, typ uint32, indices uintptr, instanceCount int32) {
	args := [5]unsafe.Pointer{
		unsafe.Pointer(&mode),
		unsafe.Pointer(&count),
		unsafe.Pointer(&typ),
		unsafe.Pointer(indices), //nolint:govet // FFI requires uintptr-to-pointer conversion
		unsafe.Pointer(&instanceCount),
	}
	_ = ffi.CallFunction(&cifVoid5DrawElem, c.glDrawElementsInstanced, nil, args[:])
}

// --- Compute Shaders ---

// DispatchCompute launches compute shader workgroups.
// Requires OpenGL 4.3+ or OpenGL ES 3.1+.
// No-op if compute shaders are not supported.
func (c *Context) DispatchCompute(numGroupsX, numGroupsY, numGroupsZ uint32) {
	if c.glDispatchCompute == nil {
		return
	}
	args := [3]unsafe.Pointer{
		unsafe.Pointer(&numGroupsX),
		unsafe.Pointer(&numGroupsY),
		unsafe.Pointer(&numGroupsZ),
	}
	_ = ffi.CallFunction(&cifVoid3, c.glDispatchCompute, nil, args[:])
}

// DispatchComputeIndirect launches compute workgroups with parameters from a buffer.
// The indirect parameter is an offset into the currently bound GL_DISPATCH_INDIRECT_BUFFER.
// Requires OpenGL 4.3+ or OpenGL ES 3.1+.
// No-op if compute shaders are not supported.
func (c *Context) DispatchComputeIndirect(indirect uintptr) {
	if c.glDispatchComputeIndirect == nil {
		return
	}
	args := [1]unsafe.Pointer{
		unsafe.Pointer(&indirect),
	}
	_ = ffi.CallFunction(&cifVoid1, c.glDispatchComputeIndirect, nil, args[:])
}

// MemoryBarrier inserts a memory barrier for specified access types.
// barriers is a bitwise OR of GL_*_BARRIER_BIT constants.
// Requires OpenGL 4.2+ or OpenGL ES 3.1+.
// No-op if memory barriers are not supported.
func (c *Context) MemoryBarrier(barriers uint32) {
	if c.glMemoryBarrier == nil {
		return
	}
	args := [1]unsafe.Pointer{
		unsafe.Pointer(&barriers),
	}
	_ = ffi.CallFunction(&cifVoid1, c.glMemoryBarrier, nil, args[:])
}

// SupportsCompute returns true if compute shaders are supported.
func (c *Context) SupportsCompute() bool {
	return c.glDispatchCompute != nil
}

// --- Helpers ---
// Note: floatToUint32 and unsafePointer helpers removed - goffi handles type conversions

// goString converts a null-terminated C string pointer to Go string.
func goString(cstr uintptr) string {
	if cstr == 0 {
		return ""
	}
	// Find string length (max 4096 to prevent infinite loops)
	length := 0
	ptr := (*byte)(unsafe.Pointer(cstr)) //nolint:govet // FFI requires uintptr-to-pointer conversion
	for i := 0; i < 4096; i++ {
		b := unsafe.Slice(ptr, i+1)
		if b[i] == 0 {
			length = i
			break
		}
	}
	if length == 0 {
		return ""
	}
	// Create slice and return string
	result := unsafe.Slice(ptr, length)
	return string(result)
}

// cString converts a Go string to a null-terminated C string.
// Returns the pointer and a function to free it.
func cString(s string) (*byte, func()) {
	buf := make([]byte, len(s)+1)
	copy(buf, s)
	return &buf[0], func() {} // No-op free since Go manages memory
}
