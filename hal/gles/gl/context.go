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
	glDepthFunc   uintptr
	glDepthMask   uintptr
	glDepthRange  uintptr
	glStencilFunc uintptr
	glStencilOp   uintptr
	glStencilMask uintptr

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
