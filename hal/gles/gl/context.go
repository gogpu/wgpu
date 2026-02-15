// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

package gl

import (
	"syscall"
	"unsafe"
)

// Context holds OpenGL function pointers loaded at runtime.
// Functions are loaded via wglGetProcAddress for GL 2.0+ or
// directly from opengl32.dll for GL 1.1 functions.
type Context struct {
	// Core GL 1.1 (from opengl32.dll)
	glGetError     uintptr
	glGetString    uintptr
	glGetIntegerv  uintptr
	glEnable       uintptr
	glDisable      uintptr
	glClear        uintptr
	glClearColor   uintptr
	glClearDepth   uintptr
	glViewport     uintptr
	glScissor      uintptr
	glDrawArrays   uintptr
	glDrawElements uintptr
	glFlush        uintptr
	glFinish       uintptr

	// Shaders (GL 2.0+)
	glCreateShader       uintptr
	glDeleteShader       uintptr
	glShaderSource       uintptr
	glCompileShader      uintptr
	glGetShaderiv        uintptr
	glGetShaderInfoLog   uintptr
	glCreateProgram      uintptr
	glDeleteProgram      uintptr
	glAttachShader       uintptr
	glDetachShader       uintptr
	glLinkProgram        uintptr
	glUseProgram         uintptr
	glGetProgramiv       uintptr
	glGetProgramInfoLog  uintptr
	glGetUniformLocation uintptr
	glGetAttribLocation  uintptr

	// Uniforms (GL 2.0+)
	glUniform1i        uintptr
	glUniform1f        uintptr
	glUniform2f        uintptr
	glUniform3f        uintptr
	glUniform4f        uintptr
	glUniform1iv       uintptr
	glUniform1fv       uintptr
	glUniform2fv       uintptr
	glUniform3fv       uintptr
	glUniform4fv       uintptr
	glUniformMatrix4fv uintptr

	// Buffers (GL 1.5+)
	glGenBuffers    uintptr
	glDeleteBuffers uintptr
	glBindBuffer    uintptr
	glBufferData    uintptr
	glBufferSubData uintptr
	glMapBuffer     uintptr
	glUnmapBuffer   uintptr

	// VAO (GL 3.0+)
	glGenVertexArrays    uintptr
	glDeleteVertexArrays uintptr
	glBindVertexArray    uintptr

	// Vertex attributes (GL 2.0+)
	glEnableVertexAttribArray  uintptr
	glDisableVertexAttribArray uintptr
	glVertexAttribPointer      uintptr

	// Textures (GL 1.1+)
	glGenTextures    uintptr
	glDeleteTextures uintptr
	glBindTexture    uintptr
	glActiveTexture  uintptr
	glTexImage2D     uintptr
	glTexSubImage2D  uintptr
	glTexParameteri  uintptr
	glGenerateMipmap uintptr

	// Framebuffers (GL 3.0+)
	glGenFramebuffers        uintptr
	glDeleteFramebuffers     uintptr
	glBindFramebuffer        uintptr
	glFramebufferTexture2D   uintptr
	glCheckFramebufferStatus uintptr
	glDrawBuffers            uintptr

	// Pixel read/store (GL 1.0+)
	glReadPixels  uintptr
	glPixelStorei uintptr

	// Renderbuffers (GL 3.0+)
	glGenRenderbuffers        uintptr
	glDeleteRenderbuffers     uintptr
	glBindRenderbuffer        uintptr
	glRenderbufferStorage     uintptr
	glFramebufferRenderbuffer uintptr

	// Blending (GL 1.4+)
	glBlendFunc             uintptr
	glBlendFuncSeparate     uintptr
	glBlendEquation         uintptr
	glBlendEquationSeparate uintptr
	glBlendColor            uintptr

	// Depth/Stencil
	glDepthFunc           uintptr
	glDepthMask           uintptr
	glDepthRange          uintptr
	glStencilFunc         uintptr
	glStencilOp           uintptr
	glStencilMask         uintptr
	glStencilFuncSeparate uintptr
	glStencilOpSeparate   uintptr
	glStencilMaskSeparate uintptr
	glColorMask           uintptr

	// Face culling
	glCullFace  uintptr
	glFrontFace uintptr

	// Sync (GL 3.2+)
	glFenceSync      uintptr
	glDeleteSync     uintptr
	glClientWaitSync uintptr
	glWaitSync       uintptr

	// UBO (GL 3.1+)
	glBindBufferBase       uintptr
	glBindBufferRange      uintptr
	glGetUniformBlockIndex uintptr
	glUniformBlockBinding  uintptr

	// Instancing (GL 3.1+)
	glDrawArraysInstanced   uintptr
	glDrawElementsInstanced uintptr
	glVertexAttribDivisor   uintptr

	// Compute shaders (GL 4.3+ / ES 3.1+)
	glDispatchCompute         uintptr
	glDispatchComputeIndirect uintptr
	glMemoryBarrier           uintptr

	// MSAA (GL 3.2+ / ES 3.1+)
	glTexImage2DMultisample uintptr
	glBlitFramebuffer       uintptr
}

// ProcAddressFunc is a function that returns the address of an OpenGL function.
type ProcAddressFunc func(name string) uintptr

// Load loads all OpenGL function pointers using the provided loader.
func (c *Context) Load(getProcAddr ProcAddressFunc) error {
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

	// Pixel read/store
	c.glReadPixels = getProcAddr("glReadPixels")
	c.glPixelStorei = getProcAddr("glPixelStorei")

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
	c.glStencilFuncSeparate = getProcAddr("glStencilFuncSeparate")
	c.glStencilOpSeparate = getProcAddr("glStencilOpSeparate")
	c.glStencilMaskSeparate = getProcAddr("glStencilMaskSeparate")
	c.glColorMask = getProcAddr("glColorMask")

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

	// MSAA (optional - may be nil on older GL versions)
	c.glTexImage2DMultisample = getProcAddr("glTexImage2DMultisample")
	c.glBlitFramebuffer = getProcAddr("glBlitFramebuffer")

	return nil
}

// --- GL Function Wrappers ---

func (c *Context) GetError() uint32 {
	r, _, _ := syscall.SyscallN(c.glGetError)
	return uint32(r)
}

func (c *Context) GetString(name uint32) string {
	r, _, _ := syscall.SyscallN(c.glGetString, uintptr(name))
	if r == 0 {
		return ""
	}
	return goString(r)
}

func (c *Context) GetIntegerv(pname uint32, data *int32) {
	syscall.SyscallN(c.glGetIntegerv, uintptr(pname), uintptr(unsafe.Pointer(data)))
}

func (c *Context) Enable(capability uint32) {
	syscall.SyscallN(c.glEnable, uintptr(capability))
}

func (c *Context) Disable(capability uint32) {
	syscall.SyscallN(c.glDisable, uintptr(capability))
}

func (c *Context) Clear(mask uint32) {
	syscall.SyscallN(c.glClear, uintptr(mask))
}

func (c *Context) ClearColor(r, g, b, a float32) {
	syscall.SyscallN(c.glClearColor,
		uintptr(*(*uint32)(unsafe.Pointer(&r))),
		uintptr(*(*uint32)(unsafe.Pointer(&g))),
		uintptr(*(*uint32)(unsafe.Pointer(&b))),
		uintptr(*(*uint32)(unsafe.Pointer(&a))))
}

func (c *Context) Viewport(x, y, width, height int32) {
	syscall.SyscallN(c.glViewport, uintptr(x), uintptr(y), uintptr(width), uintptr(height))
}

func (c *Context) Scissor(x, y, width, height int32) {
	syscall.SyscallN(c.glScissor, uintptr(x), uintptr(y), uintptr(width), uintptr(height))
}

func (c *Context) DrawArrays(mode uint32, first, count int32) {
	syscall.SyscallN(c.glDrawArrays, uintptr(mode), uintptr(first), uintptr(count))
}

func (c *Context) DrawElements(mode uint32, count int32, typ uint32, indices uintptr) {
	syscall.SyscallN(c.glDrawElements, uintptr(mode), uintptr(count), uintptr(typ), indices)
}

func (c *Context) Flush() {
	syscall.SyscallN(c.glFlush)
}

func (c *Context) Finish() {
	syscall.SyscallN(c.glFinish)
}

// --- Shaders ---

func (c *Context) CreateShader(shaderType uint32) uint32 {
	r, _, _ := syscall.SyscallN(c.glCreateShader, uintptr(shaderType))
	return uint32(r)
}

func (c *Context) DeleteShader(shader uint32) {
	syscall.SyscallN(c.glDeleteShader, uintptr(shader))
}

func (c *Context) ShaderSource(shader uint32, source string) {
	csource, free := cString(source)
	defer free()
	length := int32(len(source))
	syscall.SyscallN(c.glShaderSource, uintptr(shader), 1,
		uintptr(unsafe.Pointer(&csource)),
		uintptr(unsafe.Pointer(&length)))
}

func (c *Context) CompileShader(shader uint32) {
	syscall.SyscallN(c.glCompileShader, uintptr(shader))
}

func (c *Context) GetShaderiv(shader uint32, pname uint32, params *int32) {
	syscall.SyscallN(c.glGetShaderiv, uintptr(shader), uintptr(pname),
		uintptr(unsafe.Pointer(params)))
}

func (c *Context) GetShaderInfoLog(shader uint32) string {
	var length int32
	c.GetShaderiv(shader, INFO_LOG_LENGTH, &length)
	if length == 0 {
		return ""
	}
	buf := make([]byte, length)
	syscall.SyscallN(c.glGetShaderInfoLog, uintptr(shader), uintptr(length),
		uintptr(unsafe.Pointer(&length)), uintptr(unsafe.Pointer(&buf[0])))
	return string(buf[:length])
}

func (c *Context) CreateProgram() uint32 {
	r, _, _ := syscall.SyscallN(c.glCreateProgram)
	return uint32(r)
}

func (c *Context) DeleteProgram(program uint32) {
	syscall.SyscallN(c.glDeleteProgram, uintptr(program))
}

func (c *Context) AttachShader(program, shader uint32) {
	syscall.SyscallN(c.glAttachShader, uintptr(program), uintptr(shader))
}

func (c *Context) LinkProgram(program uint32) {
	syscall.SyscallN(c.glLinkProgram, uintptr(program))
}

func (c *Context) UseProgram(program uint32) {
	syscall.SyscallN(c.glUseProgram, uintptr(program))
}

func (c *Context) GetProgramiv(program uint32, pname uint32, params *int32) {
	syscall.SyscallN(c.glGetProgramiv, uintptr(program), uintptr(pname),
		uintptr(unsafe.Pointer(params)))
}

func (c *Context) GetProgramInfoLog(program uint32) string {
	var length int32
	c.GetProgramiv(program, INFO_LOG_LENGTH, &length)
	if length == 0 {
		return ""
	}
	buf := make([]byte, length)
	syscall.SyscallN(c.glGetProgramInfoLog, uintptr(program), uintptr(length),
		uintptr(unsafe.Pointer(&length)), uintptr(unsafe.Pointer(&buf[0])))
	return string(buf[:length])
}

func (c *Context) GetUniformLocation(program uint32, name string) int32 {
	cname, free := cString(name)
	defer free()
	r, _, _ := syscall.SyscallN(c.glGetUniformLocation, uintptr(program), uintptr(unsafe.Pointer(cname)))
	return int32(r)
}

func (c *Context) GetAttribLocation(program uint32, name string) int32 {
	cname, free := cString(name)
	defer free()
	r, _, _ := syscall.SyscallN(c.glGetAttribLocation, uintptr(program), uintptr(unsafe.Pointer(cname)))
	return int32(r)
}

// --- Buffers ---

func (c *Context) GenBuffers(n int32) uint32 {
	var buffer uint32
	syscall.SyscallN(c.glGenBuffers, uintptr(n), uintptr(unsafe.Pointer(&buffer)))
	return buffer
}

func (c *Context) DeleteBuffers(buffers ...uint32) {
	syscall.SyscallN(c.glDeleteBuffers, uintptr(len(buffers)),
		uintptr(unsafe.Pointer(&buffers[0])))
}

func (c *Context) BindBuffer(target, buffer uint32) {
	syscall.SyscallN(c.glBindBuffer, uintptr(target), uintptr(buffer))
}

func (c *Context) BufferData(target uint32, size int, data unsafe.Pointer, usage uint32) {
	syscall.SyscallN(c.glBufferData, uintptr(target), uintptr(size),
		uintptr(data), uintptr(usage))
}

func (c *Context) BufferSubData(target uint32, offset, size int, data unsafe.Pointer) {
	syscall.SyscallN(c.glBufferSubData, uintptr(target), uintptr(offset),
		uintptr(size), uintptr(data))
}

// MapBuffer maps a buffer object's data store into the client's address space.
// target: GL_ARRAY_BUFFER, GL_COPY_READ_BUFFER, etc.
// access: GL_READ_ONLY, GL_WRITE_ONLY, GL_READ_WRITE.
// Returns the mapped pointer, or 0 if glMapBuffer is not available or the call fails.
func (c *Context) MapBuffer(target, access uint32) uintptr {
	if c.glMapBuffer == 0 {
		return 0
	}
	r, _, _ := syscall.SyscallN(c.glMapBuffer, uintptr(target), uintptr(access))
	return r
}

// UnmapBuffer releases the mapping of a buffer object's data store.
// Returns true if the buffer was successfully unmapped, false if the buffer contents
// became corrupt during the mapping (GL_FALSE from driver) or glUnmapBuffer is unavailable.
func (c *Context) UnmapBuffer(target uint32) bool {
	if c.glUnmapBuffer == 0 {
		return false
	}
	r, _, _ := syscall.SyscallN(c.glUnmapBuffer, uintptr(target))
	return r != 0
}

// --- VAO ---

func (c *Context) GenVertexArrays(n int32) uint32 {
	var vao uint32
	syscall.SyscallN(c.glGenVertexArrays, uintptr(n), uintptr(unsafe.Pointer(&vao)))
	return vao
}

func (c *Context) DeleteVertexArrays(arrays ...uint32) {
	syscall.SyscallN(c.glDeleteVertexArrays, uintptr(len(arrays)),
		uintptr(unsafe.Pointer(&arrays[0])))
}

func (c *Context) BindVertexArray(array uint32) {
	syscall.SyscallN(c.glBindVertexArray, uintptr(array))
}

// --- Vertex Attributes ---

func (c *Context) EnableVertexAttribArray(index uint32) {
	syscall.SyscallN(c.glEnableVertexAttribArray, uintptr(index))
}

func (c *Context) DisableVertexAttribArray(index uint32) {
	syscall.SyscallN(c.glDisableVertexAttribArray, uintptr(index))
}

func (c *Context) VertexAttribPointer(index uint32, size int32, typ uint32, normalized bool, stride int32, offset uintptr) {
	var norm uintptr
	if normalized {
		norm = TRUE
	}
	syscall.SyscallN(c.glVertexAttribPointer, uintptr(index), uintptr(size),
		uintptr(typ), norm, uintptr(stride), offset)
}

// --- Textures ---

func (c *Context) GenTextures(n int32) uint32 {
	var tex uint32
	syscall.SyscallN(c.glGenTextures, uintptr(n), uintptr(unsafe.Pointer(&tex)))
	return tex
}

func (c *Context) DeleteTextures(textures ...uint32) {
	syscall.SyscallN(c.glDeleteTextures, uintptr(len(textures)),
		uintptr(unsafe.Pointer(&textures[0])))
}

func (c *Context) BindTexture(target, texture uint32) {
	syscall.SyscallN(c.glBindTexture, uintptr(target), uintptr(texture))
}

func (c *Context) ActiveTexture(texture uint32) {
	syscall.SyscallN(c.glActiveTexture, uintptr(texture))
}

func (c *Context) TexParameteri(target, pname uint32, param int32) {
	syscall.SyscallN(c.glTexParameteri, uintptr(target), uintptr(pname), uintptr(param))
}

func (c *Context) TexImage2D(target uint32, level int32, internalformat int32, width, height int32, border int32, format, typ uint32, pixels unsafe.Pointer) {
	syscall.SyscallN(c.glTexImage2D, uintptr(target), uintptr(level),
		uintptr(internalformat), uintptr(width), uintptr(height), uintptr(border),
		uintptr(format), uintptr(typ), uintptr(pixels))
}

func (c *Context) GenerateMipmap(target uint32) {
	syscall.SyscallN(c.glGenerateMipmap, uintptr(target))
}

// TexImage2DMultisample creates a multisample 2D texture image.
// Requires OpenGL 3.2+ or OpenGL ES 3.1+.
// No-op if not supported.
func (c *Context) TexImage2DMultisample(target uint32, samples int32, internalformat uint32, width, height int32, fixedsamplelocations bool) {
	if c.glTexImage2DMultisample == 0 {
		return
	}
	var fixed uintptr
	if fixedsamplelocations {
		fixed = TRUE
	}
	syscall.SyscallN(c.glTexImage2DMultisample, uintptr(target), uintptr(samples),
		uintptr(internalformat), uintptr(width), uintptr(height), fixed)
}

// BlitFramebuffer copies a block of pixels between framebuffers.
// Requires OpenGL 3.0+ or OpenGL ES 3.0+.
// No-op if not supported.
func (c *Context) BlitFramebuffer(srcX0, srcY0, srcX1, srcY1, dstX0, dstY0, dstX1, dstY1 int32, mask, filter uint32) {
	if c.glBlitFramebuffer == 0 {
		return
	}
	syscall.SyscallN(c.glBlitFramebuffer,
		uintptr(srcX0), uintptr(srcY0), uintptr(srcX1), uintptr(srcY1),
		uintptr(dstX0), uintptr(dstY0), uintptr(dstX1), uintptr(dstY1),
		uintptr(mask), uintptr(filter))
}

// --- Framebuffers ---

func (c *Context) GenFramebuffers(n int32) uint32 {
	var fbo uint32
	syscall.SyscallN(c.glGenFramebuffers, uintptr(n), uintptr(unsafe.Pointer(&fbo)))
	return fbo
}

func (c *Context) DeleteFramebuffers(framebuffers ...uint32) {
	syscall.SyscallN(c.glDeleteFramebuffers, uintptr(len(framebuffers)),
		uintptr(unsafe.Pointer(&framebuffers[0])))
}

func (c *Context) BindFramebuffer(target, framebuffer uint32) {
	syscall.SyscallN(c.glBindFramebuffer, uintptr(target), uintptr(framebuffer))
}

func (c *Context) FramebufferTexture2D(target, attachment, textarget, texture uint32, level int32) {
	syscall.SyscallN(c.glFramebufferTexture2D, uintptr(target), uintptr(attachment),
		uintptr(textarget), uintptr(texture), uintptr(level))
}

func (c *Context) CheckFramebufferStatus(target uint32) uint32 {
	r, _, _ := syscall.SyscallN(c.glCheckFramebufferStatus, uintptr(target))
	return uint32(r)
}

// --- Pixel Read/Store ---

// ReadPixels reads a block of pixels from the framebuffer.
func (c *Context) ReadPixels(x, y, width, height int32, format, type_ uint32, pixels unsafe.Pointer) {
	syscall.SyscallN(c.glReadPixels, uintptr(x), uintptr(y), uintptr(width), uintptr(height),
		uintptr(format), uintptr(type_), uintptr(pixels))
}

// PixelStorei sets pixel storage modes that affect ReadPixels and TexImage operations.
func (c *Context) PixelStorei(pname uint32, param int32) {
	syscall.SyscallN(c.glPixelStorei, uintptr(pname), uintptr(param))
}

// --- UBO ---

// BindBufferBase binds a buffer to an indexed binding point.
func (c *Context) BindBufferBase(target, index, buffer uint32) {
	syscall.SyscallN(c.glBindBufferBase, uintptr(target), uintptr(index), uintptr(buffer))
}

// BindBufferRange binds a range of a buffer to an indexed binding point.
func (c *Context) BindBufferRange(target, index, buffer uint32, offset, size int) {
	syscall.SyscallN(c.glBindBufferRange, uintptr(target), uintptr(index), uintptr(buffer),
		uintptr(offset), uintptr(size))
}

// GetUniformBlockIndex returns the index of a named uniform block.
func (c *Context) GetUniformBlockIndex(program uint32, name string) uint32 {
	cname, free := cString(name)
	defer free()
	r, _, _ := syscall.SyscallN(c.glGetUniformBlockIndex, uintptr(program), uintptr(unsafe.Pointer(cname)))
	return uint32(r)
}

// UniformBlockBinding assigns a uniform block to a binding point.
func (c *Context) UniformBlockBinding(program, blockIndex, blockBinding uint32) {
	syscall.SyscallN(c.glUniformBlockBinding, uintptr(program), uintptr(blockIndex), uintptr(blockBinding))
}

// --- Uniforms ---

// Uniform1i sets an integer uniform value.
func (c *Context) Uniform1i(location, value int32) {
	syscall.SyscallN(c.glUniform1i, uintptr(location), uintptr(value))
}

// --- Blending ---

func (c *Context) BlendFunc(sfactor, dfactor uint32) {
	syscall.SyscallN(c.glBlendFunc, uintptr(sfactor), uintptr(dfactor))
}

func (c *Context) BlendFuncSeparate(srcRGB, dstRGB, srcAlpha, dstAlpha uint32) {
	syscall.SyscallN(c.glBlendFuncSeparate, uintptr(srcRGB), uintptr(dstRGB),
		uintptr(srcAlpha), uintptr(dstAlpha))
}

func (c *Context) BlendEquation(mode uint32) {
	syscall.SyscallN(c.glBlendEquation, uintptr(mode))
}

// BlendEquationSeparate sets separate blend equations for RGB and alpha.
func (c *Context) BlendEquationSeparate(modeRGB, modeAlpha uint32) {
	syscall.SyscallN(c.glBlendEquationSeparate, uintptr(modeRGB), uintptr(modeAlpha))
}

// BlendColor sets the constant blend color.
func (c *Context) BlendColor(r, g, b, a float32) {
	syscall.SyscallN(c.glBlendColor,
		uintptr(*(*uint32)(unsafe.Pointer(&r))),
		uintptr(*(*uint32)(unsafe.Pointer(&g))),
		uintptr(*(*uint32)(unsafe.Pointer(&b))),
		uintptr(*(*uint32)(unsafe.Pointer(&a))))
}

// --- Depth/Stencil ---

func (c *Context) DepthFunc(fn uint32) {
	syscall.SyscallN(c.glDepthFunc, uintptr(fn))
}

func (c *Context) DepthMask(flag bool) {
	var f uintptr
	if flag {
		f = TRUE
	}
	syscall.SyscallN(c.glDepthMask, f)
}

// StencilFuncSeparate sets stencil test function per face.
func (c *Context) StencilFuncSeparate(face, fn uint32, ref int32, mask uint32) {
	syscall.SyscallN(c.glStencilFuncSeparate, uintptr(face), uintptr(fn), uintptr(ref), uintptr(mask))
}

// StencilOpSeparate sets stencil operations per face.
func (c *Context) StencilOpSeparate(face, sfail, dpfail, dppass uint32) {
	syscall.SyscallN(c.glStencilOpSeparate, uintptr(face), uintptr(sfail), uintptr(dpfail), uintptr(dppass))
}

// StencilMaskSeparate sets stencil write mask per face.
func (c *Context) StencilMaskSeparate(face, mask uint32) {
	syscall.SyscallN(c.glStencilMaskSeparate, uintptr(face), uintptr(mask))
}

// ColorMask enables or disables writing of color components.
func (c *Context) ColorMask(r, g, b, a bool) {
	var rv, gv, bv, av uintptr
	if r {
		rv = TRUE
	}
	if g {
		gv = TRUE
	}
	if b {
		bv = TRUE
	}
	if a {
		av = TRUE
	}
	syscall.SyscallN(c.glColorMask, rv, gv, bv, av)
}

// --- Face Culling ---

func (c *Context) CullFace(mode uint32) {
	syscall.SyscallN(c.glCullFace, uintptr(mode))
}

func (c *Context) FrontFace(mode uint32) {
	syscall.SyscallN(c.glFrontFace, uintptr(mode))
}

// --- Instancing ---

func (c *Context) DrawArraysInstanced(mode uint32, first, count, instanceCount int32) {
	syscall.SyscallN(c.glDrawArraysInstanced, uintptr(mode), uintptr(first),
		uintptr(count), uintptr(instanceCount))
}

func (c *Context) DrawElementsInstanced(mode uint32, count int32, typ uint32, indices uintptr, instanceCount int32) {
	syscall.SyscallN(c.glDrawElementsInstanced, uintptr(mode), uintptr(count),
		uintptr(typ), indices, uintptr(instanceCount))
}

// --- Compute Shaders ---

// DispatchCompute launches compute shader workgroups.
// Requires OpenGL 4.3+ or OpenGL ES 3.1+.
// No-op if compute shaders are not supported.
func (c *Context) DispatchCompute(numGroupsX, numGroupsY, numGroupsZ uint32) {
	if c.glDispatchCompute == 0 {
		return
	}
	syscall.SyscallN(c.glDispatchCompute, uintptr(numGroupsX), uintptr(numGroupsY), uintptr(numGroupsZ))
}

// DispatchComputeIndirect launches compute workgroups with parameters from a buffer.
// The indirect parameter is an offset into the currently bound GL_DISPATCH_INDIRECT_BUFFER.
// Requires OpenGL 4.3+ or OpenGL ES 3.1+.
// No-op if compute shaders are not supported.
func (c *Context) DispatchComputeIndirect(indirect uintptr) {
	if c.glDispatchComputeIndirect == 0 {
		return
	}
	syscall.SyscallN(c.glDispatchComputeIndirect, indirect)
}

// MemoryBarrier inserts a memory barrier for specified access types.
// barriers is a bitwise OR of GL_*_BARRIER_BIT constants.
// Requires OpenGL 4.2+ or OpenGL ES 3.1+.
// No-op if memory barriers are not supported.
func (c *Context) MemoryBarrier(barriers uint32) {
	if c.glMemoryBarrier == 0 {
		return
	}
	syscall.SyscallN(c.glMemoryBarrier, uintptr(barriers))
}

// SupportsCompute returns true if compute shaders are supported.
func (c *Context) SupportsCompute() bool {
	return c.glDispatchCompute != 0
}

// --- Helpers ---

// ptrFromUintptr converts a uintptr (from FFI) to *byte without triggering go vet warning.
// This uses double pointer indirection pattern from ebitengine/purego.
// Reference: https://github.com/golang/go/issues/56487
func ptrFromUintptr(ptr uintptr) *byte {
	return *(**byte)(unsafe.Pointer(&ptr))
}

// goString converts a null-terminated C string pointer to Go string.
// The pointer must be valid and point to a null-terminated string.
// This is safe because the pointer comes from OpenGL and remains valid
// for the duration of this function call.
func goString(cstr uintptr) string {
	if cstr == 0 {
		return ""
	}
	// Find string length first (max 4096 to prevent infinite loops)
	// Use double pointer indirection to satisfy go vet (pattern from ebitengine/purego)
	length := 0
	for i := 0; i < 4096; i++ {
		b := unsafe.Slice(ptrFromUintptr(cstr), i+1)
		if b[i] == 0 {
			length = i
			break
		}
	}
	if length == 0 {
		return ""
	}
	// Create slice and copy to Go-managed memory
	result := unsafe.Slice(ptrFromUintptr(cstr), length)
	return string(result)
}

// cString converts a Go string to a null-terminated C string.
// Returns the pointer and a function to free it.
func cString(s string) (*byte, func()) {
	buf := make([]byte, len(s)+1)
	copy(buf, s)
	return &buf[0], func() {} // No-op free since Go manages memory
}
