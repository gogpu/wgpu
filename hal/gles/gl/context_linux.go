// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build linux && !(js && wasm)

// FFI error handling follows ADR-049 three-tier strategy:
//
//	Tier 1 (creation/submit): check both FFI error and API result code
//	Tier 2 (void/hot path): infallible by GPU API contract — Vulkan §6.6, WebGPU §21.2
//	Tier 3 (platform syscalls): use errno for diagnostics (Wayland, X11, Win32)
//
// Enterprise reference: Rust wgpu-hal returns () for all draw/destroy/barrier commands.
//
// All GL commands in this file are Tier 2: GL errors are reported via glGetError(),
// not through FFI return values. The GL spec guarantees these calls never fail at
// the transport level.
package gl

import (
	"unsafe"

	"github.com/go-webgpu/goffi/ffi"
	"github.com/go-webgpu/goffi/types"
)

// Common CallInterface signatures (reused across multiple GL functions)
var (
	cifVoid              types.CallInterface // void fn(void)
	cifUInt32            types.CallInterface // uint32 fn(void)
	cifUInt321           types.CallInterface // uint32 fn(uint32)
	cifInt322            types.CallInterface // int32 fn(uint32, void*)
	cifVoid1             types.CallInterface // void fn(uint32)
	cifVoid2             types.CallInterface // void fn(uint32, void*)
	cifVoid2UU           types.CallInterface // void fn(uint32, uint32)
	cifVoid3             types.CallInterface // void fn(uint32, uint32, uint32)
	cifVoid4             types.CallInterface // void fn(uint32, uint32, uint32, uint32)
	cifVoid4Float        types.CallInterface // void fn(float, float, float, float)
	cifVoid4Shader       types.CallInterface // void fn(uint32, int32, void*, void*)
	cifVoid3Shader       types.CallInterface // void fn(uint32, uint32, void*)
	cifVoid4Log          types.CallInterface // void fn(uint32, uint32, void*, void*)
	cifVoid4Buffer       types.CallInterface // void fn(uint32, uintptr, void*, uint32)
	cifVoid4SubBuf       types.CallInterface // void fn(uint32, uintptr, uintptr, void*)
	cifVoid6Attrib       types.CallInterface // void fn(uint32, int32, uint32, uint8, int32, uintptr)
	cifVoid5FBO          types.CallInterface // void fn(uint32, uint32, uint32, uint32, int32)
	cifVoid9TexImg       types.CallInterface // void fn(uint32, int32, int32, int32, int32, int32, uint32, uint32, void*)
	cifVoid4Draw         types.CallInterface // void fn(uint32, int32, int32, int32)
	cifVoid5DrawElem     types.CallInterface // void fn(uint32, int32, uint32, void*, int32)
	cifVoid6DrawElemBV   types.CallInterface // void fn(uint32, int32, uint32, void*, int32, int32) - DrawElementsInstancedBaseVertex
	cifVoid5DrawArrBI    types.CallInterface // void fn(uint32, int32, int32, int32, uint32) - DrawArraysInstancedBaseInstance
	cifVoid7DrawElemBVBI types.CallInterface // void fn(uint32, int32, uint32, void*, int32, int32, uint32) - DrawElementsInstancedBaseVertexBaseInstance
	cifVoid4MultiArr     types.CallInterface // void fn(uint32, void*, int32, int32) - MultiDrawArraysIndirect
	cifVoid5MultiElem    types.CallInterface // void fn(uint32, uint32, void*, int32, int32) - MultiDrawElementsIndirect
	cifPtr1              types.CallInterface // void* fn(uint32)
	cifPtr2UU            types.CallInterface // void* fn(uint32, uint32)
	cifVoid7ReadPx       types.CallInterface // void fn(int32, int32, int32, int32, uint32, uint32, void*)
	cifVoid6TexMS        types.CallInterface // void fn(uint32, int32, uint32, int32, int32, uint8) - TexImage2DMultisample
	cifVoid7BindImage    types.CallInterface // void fn(uint32, uint32, int32, uint8, int32, uint32, uint32) - BindImageTexture
	cifVoid10Blit        types.CallInterface // void fn(int32*8, uint32, uint32) - BlitFramebuffer
	cifVoid3UUF          types.CallInterface // void fn(uint32, uint32, float32) - SamplerParameterf
	cifVoid1Float        types.CallInterface // void fn(float32) - ClearDepthf
	cifVoid2Float        types.CallInterface // void fn(float32, float32) - DepthRangef
	cifVoid1Double       types.CallInterface // void fn(float64) - glClearDepth
	cifVoid2Double       types.CallInterface // void fn(float64, float64) - glDepthRange
	cifVoid10TexImg3D    types.CallInterface // void fn(uint32, int32*6, uint32, uint32, void*) - TexImage3D
	cifVoid11TexSubImg3D types.CallInterface // void fn(uint32, int32*7, uint32, uint32, void*) - TexSubImage3D
	cifVoid5FBOLayer     types.CallInterface // void fn(uint32, uint32, uint32, int32, int32) - FramebufferTextureLayer

	cifVoid5TexStorage2D  types.CallInterface // void fn(uint32, int32, uint32, int32, int32) - TexStorage2D
	cifVoid6TexStorage3D  types.CallInterface // void fn(uint32, int32, uint32, int32, int32, int32) - TexStorage3D
	cifVoid9CompSubImg2D  types.CallInterface // void fn(uint32, int32*5, uint32, int32, void*) - CompressedTexSubImage2D
	cifVoid11CompSubImg3D types.CallInterface // void fn(uint32, int32*7, uint32, int32, void*) - CompressedTexSubImage3D

	cifUInt32Wait   types.CallInterface // uint32 fn(void*, uint32, uint64) - ClientWaitSync
	cifVoid5Synciv  types.CallInterface // void fn(void*, uint32, int32, void*, void*) - GetSynciv
	cifVoid5CopyBuf types.CallInterface // void fn(uint32, uint32, uintptr, uintptr, uintptr) - CopyBufferSubData
	cifVoid8CopyTex types.CallInterface // void fn(uint32, int32 x7) - CopyTexSubImage2D

	cifInitialized bool
)

// initCommonCallInterfaces prepares reusable CallInterface signatures.
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

	// void fn(float32) - glClearDepthf
	err = ffi.PrepareCallInterface(&cifVoid1Float, types.DefaultCall,
		types.VoidTypeDescriptor,
		[]*types.TypeDescriptor{types.FloatTypeDescriptor})
	if err != nil {
		return err
	}

	// void fn(float32, float32) - glDepthRangef
	err = ffi.PrepareCallInterface(&cifVoid2Float, types.DefaultCall,
		types.VoidTypeDescriptor,
		[]*types.TypeDescriptor{
			types.FloatTypeDescriptor,
			types.FloatTypeDescriptor,
		})
	if err != nil {
		return err
	}

	// void fn(float64) - glClearDepth (desktop GL without glClearDepthf)
	err = ffi.PrepareCallInterface(&cifVoid1Double, types.DefaultCall,
		types.VoidTypeDescriptor,
		[]*types.TypeDescriptor{types.DoubleTypeDescriptor})
	if err != nil {
		return err
	}

	// void fn(float64, float64) - glDepthRange (desktop GL without glDepthRangef)
	err = ffi.PrepareCallInterface(&cifVoid2Double, types.DefaultCall,
		types.VoidTypeDescriptor,
		[]*types.TypeDescriptor{
			types.DoubleTypeDescriptor,
			types.DoubleTypeDescriptor,
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

	// void fn(uint32, int32, uint32, void*, int32, int32) - DrawElementsInstancedBaseVertex
	err = ffi.PrepareCallInterface(&cifVoid6DrawElemBV, types.DefaultCall,
		types.VoidTypeDescriptor,
		[]*types.TypeDescriptor{
			types.UInt32TypeDescriptor,
			types.SInt32TypeDescriptor,
			types.UInt32TypeDescriptor,
			types.PointerTypeDescriptor,
			types.SInt32TypeDescriptor,
			types.SInt32TypeDescriptor,
		})
	if err != nil {
		return err
	}

	// void fn(uint32, int32, int32, int32, uint32) - DrawArraysInstancedBaseInstance
	err = ffi.PrepareCallInterface(&cifVoid5DrawArrBI, types.DefaultCall,
		types.VoidTypeDescriptor,
		[]*types.TypeDescriptor{
			types.UInt32TypeDescriptor,
			types.SInt32TypeDescriptor,
			types.SInt32TypeDescriptor,
			types.SInt32TypeDescriptor,
			types.UInt32TypeDescriptor,
		})
	if err != nil {
		return err
	}

	// void fn(uint32, int32, uint32, void*, int32, int32, uint32) - DrawElementsInstancedBaseVertexBaseInstance
	err = ffi.PrepareCallInterface(&cifVoid7DrawElemBVBI, types.DefaultCall,
		types.VoidTypeDescriptor,
		[]*types.TypeDescriptor{
			types.UInt32TypeDescriptor,
			types.SInt32TypeDescriptor,
			types.UInt32TypeDescriptor,
			types.PointerTypeDescriptor,
			types.SInt32TypeDescriptor,
			types.SInt32TypeDescriptor,
			types.UInt32TypeDescriptor,
		})
	if err != nil {
		return err
	}

	// void fn(uint32, void*, int32, int32) - MultiDrawArraysIndirect
	err = ffi.PrepareCallInterface(&cifVoid4MultiArr, types.DefaultCall,
		types.VoidTypeDescriptor,
		[]*types.TypeDescriptor{
			types.UInt32TypeDescriptor,
			types.PointerTypeDescriptor,
			types.SInt32TypeDescriptor,
			types.SInt32TypeDescriptor,
		})
	if err != nil {
		return err
	}

	// void fn(uint32, uint32, void*, int32, int32) - MultiDrawElementsIndirect
	err = ffi.PrepareCallInterface(&cifVoid5MultiElem, types.DefaultCall,
		types.VoidTypeDescriptor,
		[]*types.TypeDescriptor{
			types.UInt32TypeDescriptor,
			types.UInt32TypeDescriptor,
			types.PointerTypeDescriptor,
			types.SInt32TypeDescriptor,
			types.SInt32TypeDescriptor,
		})
	if err != nil {
		return err
	}

	// void fn(int32, int32, int32, int32, uint32, uint32, void*) - ReadPixels
	err = ffi.PrepareCallInterface(&cifVoid7ReadPx, types.DefaultCall,
		types.VoidTypeDescriptor,
		[]*types.TypeDescriptor{
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

	// void fn(uint32, int32, uint32, int32, int32, uint8) - TexImage2DMultisample
	err = ffi.PrepareCallInterface(&cifVoid6TexMS, types.DefaultCall,
		types.VoidTypeDescriptor,
		[]*types.TypeDescriptor{
			types.UInt32TypeDescriptor, // target
			types.SInt32TypeDescriptor, // samples
			types.UInt32TypeDescriptor, // internalformat
			types.SInt32TypeDescriptor, // width
			types.SInt32TypeDescriptor, // height
			types.UInt8TypeDescriptor,  // fixedsamplelocations
		})
	if err != nil {
		return err
	}

	err = ffi.PrepareCallInterface(&cifVoid7BindImage, types.DefaultCall,
		types.VoidTypeDescriptor,
		[]*types.TypeDescriptor{
			types.UInt32TypeDescriptor, // unit
			types.UInt32TypeDescriptor, // texture
			types.SInt32TypeDescriptor, // level
			types.UInt8TypeDescriptor,  // layered
			types.SInt32TypeDescriptor, // layer
			types.UInt32TypeDescriptor, // access
			types.UInt32TypeDescriptor, // format
		})
	if err != nil {
		return err
	}

	// void fn(int32, int32, int32, int32, int32, int32, int32, int32, uint32, uint32) - BlitFramebuffer
	err = ffi.PrepareCallInterface(&cifVoid10Blit, types.DefaultCall,
		types.VoidTypeDescriptor,
		[]*types.TypeDescriptor{
			types.SInt32TypeDescriptor, // srcX0
			types.SInt32TypeDescriptor, // srcY0
			types.SInt32TypeDescriptor, // srcX1
			types.SInt32TypeDescriptor, // srcY1
			types.SInt32TypeDescriptor, // dstX0
			types.SInt32TypeDescriptor, // dstY0
			types.SInt32TypeDescriptor, // dstX1
			types.SInt32TypeDescriptor, // dstY1
			types.UInt32TypeDescriptor, // mask
			types.UInt32TypeDescriptor, // filter
		})
	if err != nil {
		return err
	}

	// void fn(uint32, uint32, float32) - SamplerParameterf
	err = ffi.PrepareCallInterface(&cifVoid3UUF, types.DefaultCall,
		types.VoidTypeDescriptor,
		[]*types.TypeDescriptor{
			types.UInt32TypeDescriptor,
			types.UInt32TypeDescriptor,
			types.FloatTypeDescriptor,
		})
	if err != nil {
		return err
	}

	if err := initLayeredTextureCallInterfaces(); err != nil {
		return err
	}
	if err := initCompressedTextureCallInterfaces(); err != nil {
		return err
	}
	if err := initSyncCopyCallInterfaces(); err != nil {
		return err
	}

	cifInitialized = true
	return nil
}

// initSyncCopyCallInterfaces prepares the signatures of ClientWaitSync,
// GetSyncStatus, CopyBufferSubData and CopyTexSubImage2D. They used to be
// prepared on every call, including the fence-poll hot path (ClientWaitSync,
// GetSyncStatus).
func initSyncCopyCallInterfaces() error {
	err := ffi.PrepareCallInterface(&cifUInt32Wait, types.DefaultCall,
		types.UInt32TypeDescriptor,
		[]*types.TypeDescriptor{
			types.PointerTypeDescriptor, // sync
			types.UInt32TypeDescriptor,  // flags
			types.UInt64TypeDescriptor,  // timeout
		})
	if err != nil {
		return err
	}

	err = ffi.PrepareCallInterface(&cifVoid5Synciv, types.DefaultCall,
		types.VoidTypeDescriptor,
		[]*types.TypeDescriptor{
			types.PointerTypeDescriptor, // sync
			types.UInt32TypeDescriptor,  // pname
			types.SInt32TypeDescriptor,  // count
			types.PointerTypeDescriptor, // length
			types.PointerTypeDescriptor, // values
		})
	if err != nil {
		return err
	}

	err = ffi.PrepareCallInterface(&cifVoid5CopyBuf, types.DefaultCall,
		types.VoidTypeDescriptor,
		[]*types.TypeDescriptor{
			types.UInt32TypeDescriptor,  // readTarget
			types.UInt32TypeDescriptor,  // writeTarget
			types.PointerTypeDescriptor, // readOffset (GLintptr)
			types.PointerTypeDescriptor, // writeOffset (GLintptr)
			types.PointerTypeDescriptor, // size (GLsizeiptr)
		})
	if err != nil {
		return err
	}

	return ffi.PrepareCallInterface(&cifVoid8CopyTex, types.DefaultCall,
		types.VoidTypeDescriptor,
		[]*types.TypeDescriptor{
			types.UInt32TypeDescriptor, // target
			types.SInt32TypeDescriptor, // level
			types.SInt32TypeDescriptor, // xoffset
			types.SInt32TypeDescriptor, // yoffset
			types.SInt32TypeDescriptor, // x
			types.SInt32TypeDescriptor, // y
			types.SInt32TypeDescriptor, // width
			types.SInt32TypeDescriptor, // height
		})
}

// Context holds OpenGL function pointers loaded at runtime via goffi.
// Functions are loaded via eglGetProcAddress for all OpenGL functions.
type Context struct {
	isGLES bool
	// clearDepthDouble and depthRangeDouble are set when glClearDepth and
	// glDepthRange take doubles: desktop GL without the float entry points
	// (GL 4.1 / ARB_ES2_compatibility).
	clearDepthDouble bool
	depthRangeDouble bool

	// Core GL 1.1
	glGetError     unsafe.Pointer
	glGetString    unsafe.Pointer
	glGetIntegerv  unsafe.Pointer
	glEnable       unsafe.Pointer
	glDisable      unsafe.Pointer
	glClear        unsafe.Pointer
	glClearColor   unsafe.Pointer
	glClearDepth   unsafe.Pointer
	glClearStencil unsafe.Pointer
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
	glClearBufferfv          unsafe.Pointer // GL 3.0+ — per-buffer float clear
	glClearBufferiv          unsafe.Pointer // GL 3.0+ — per-buffer signed integer clear
	glClearBufferuiv         unsafe.Pointer // GL 3.0+ — per-buffer unsigned integer clear

	// Pixel read/store (GL 1.0+)
	glReadPixels  unsafe.Pointer
	glPixelStorei unsafe.Pointer

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
	glDepthFunc           unsafe.Pointer
	glDepthMask           unsafe.Pointer
	glDepthRange          unsafe.Pointer
	glStencilFunc         unsafe.Pointer
	glStencilOp           unsafe.Pointer
	glStencilMask         unsafe.Pointer
	glStencilFuncSeparate unsafe.Pointer
	glStencilOpSeparate   unsafe.Pointer
	glStencilMaskSeparate unsafe.Pointer
	glColorMask           unsafe.Pointer

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

	// Draw variants (optional): base vertex GL 3.2 / ES 3.2, indirect GL 4.0 /
	// ES 3.1, base instance GL 4.2, glUniform1ui GL 3.0 / ES 3.0.
	glDrawElementsInstancedBaseVertex             unsafe.Pointer
	glDrawArraysIndirect                          unsafe.Pointer
	glDrawElementsIndirect                        unsafe.Pointer
	glDrawArraysInstancedBaseInstance             unsafe.Pointer
	glDrawElementsInstancedBaseVertexBaseInstance unsafe.Pointer
	glUniform1ui                                  unsafe.Pointer
	glMultiDrawArraysIndirect                     unsafe.Pointer
	glMultiDrawElementsIndirect                   unsafe.Pointer

	// Compute shaders (GL 4.3+ / ES 3.1+)
	glDispatchCompute         unsafe.Pointer
	glDispatchComputeIndirect unsafe.Pointer
	glMemoryBarrier           unsafe.Pointer
	glBindImageTexture        unsafe.Pointer

	// MSAA (GL 3.2+ / ES 3.1+)
	glTexImage2DMultisample unsafe.Pointer
	glBlitFramebuffer       unsafe.Pointer

	// 3D and 2D array textures (GL 1.2 / 3.0+, ES 3.0+)
	glTexImage3D              unsafe.Pointer
	glTexSubImage3D           unsafe.Pointer
	glFramebufferTextureLayer unsafe.Pointer

	// Immutable storage and compressed uploads (GL 4.2 / ES 3.0)
	glTexStorage2D            unsafe.Pointer
	glTexStorage3D            unsafe.Pointer
	glCompressedTexSubImage2D unsafe.Pointer
	glCompressedTexSubImage3D unsafe.Pointer

	// Sampler objects (GL 3.3+ / ES 3.0+)
	glGenSamplers       unsafe.Pointer
	glDeleteSamplers    unsafe.Pointer
	glBindSampler       unsafe.Pointer
	glSamplerParameteri unsafe.Pointer
	glSamplerParameterf unsafe.Pointer

	// Query objects (GL 3.3+ / GL_EXT_disjoint_timer_query)
	glGenQueries          unsafe.Pointer
	glDeleteQueries       unsafe.Pointer
	glQueryCounter        unsafe.Pointer
	glGetQueryObjectui64v unsafe.Pointer
	glCopyBufferSubData   unsafe.Pointer
	glCopyTexSubImage2D   unsafe.Pointer
	glGetSynciv           unsafe.Pointer

	// Indexed string query (GL 3.0+ / ES 3.0+)
	glGetStringi unsafe.Pointer
}

// ProcAddressFunc is a function that returns the address of an OpenGL function.
type ProcAddressFunc func(name string) unsafe.Pointer

// Load loads all OpenGL function pointers using the provided loader.
// The context must be current. Whether it is OpenGL ES is read from
// GL_VERSION ("OpenGL ES N.M ..."), as Rust wgpu-hal does (gles/adapter.rs
// parse_version), because some entry points differ by name (glClearDepthf vs
// glClearDepth, glMapBufferRange vs glMapBuffer) and Mesa d3d12 gallium
// returns dispatch stubs for desktop GL names on ES contexts that raise
// GL_INVALID_ENUM at call time.
func (c *Context) Load(getProcAddr ProcAddressFunc) error {
	if err := initCommonCallInterfaces(); err != nil {
		return err
	}

	// Without glGetString the version stays unknown (desktop) and the
	// required-procs check at the end of Load reports it with the others.
	c.glGetString = getProcAddr("glGetString")
	gles := c.glGetString != nil && isESVersionString(c.GetString(VERSION))
	c.isGLES = gles

	// Core GL 1.1
	c.glGetError = getProcAddr("glGetError")
	c.glGetIntegerv = getProcAddr("glGetIntegerv")
	c.glEnable = getProcAddr("glEnable")
	c.glDisable = getProcAddr("glDisable")
	c.glClear = getProcAddr("glClear")
	c.glClearColor = getProcAddr("glClearColor")
	c.glClearStencil = getProcAddr("glClearStencil")
	// The float entry point, as Rust's glow calls (clear_depth_f32): GLES,
	// and desktop GL 4.1 / ARB_ES2_compatibility. Older desktop GL has only
	// glClearDepth, which takes a double.
	c.glClearDepth = getProcAddr("glClearDepthf")
	if c.glClearDepth == nil && !gles {
		c.glClearDepth = getProcAddr("glClearDepth")
		c.clearDepthDouble = true
	}
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
	if gles && c.glMapBuffer == nil {
		c.glMapBuffer = getProcAddr("glMapBufferOES")
	}
	c.glUnmapBuffer = getProcAddr("glUnmapBuffer")

	// VAO — GLES may need OES suffix on some Mesa dispatch paths.
	c.glGenVertexArrays = getProcAddr("glGenVertexArrays")
	c.glDeleteVertexArrays = getProcAddr("glDeleteVertexArrays")
	c.glBindVertexArray = getProcAddr("glBindVertexArray")
	if gles {
		if p := getProcAddr("glGenVertexArraysOES"); p != nil {
			c.glGenVertexArrays = p
		}
		if p := getProcAddr("glDeleteVertexArraysOES"); p != nil {
			c.glDeleteVertexArrays = p
		}
		if p := getProcAddr("glBindVertexArrayOES"); p != nil {
			c.glBindVertexArray = p
		}
	}

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
	c.glClearBufferfv = getProcAddr("glClearBufferfv")
	c.glClearBufferiv = getProcAddr("glClearBufferiv")
	c.glClearBufferuiv = getProcAddr("glClearBufferuiv")

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
	// As glClearDepth: glDepthRangef (glow's depth_range_f32), the double
	// glDepthRange only where the float one is missing.
	c.glDepthRange = getProcAddr("glDepthRangef")
	if c.glDepthRange == nil && !gles {
		c.glDepthRange = getProcAddr("glDepthRange")
		c.depthRangeDouble = true
	}
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
	c.glDrawElementsInstancedBaseVertex = getProcAddr("glDrawElementsInstancedBaseVertex")
	c.glDrawArraysIndirect = getProcAddr("glDrawArraysIndirect")
	c.glDrawElementsIndirect = getProcAddr("glDrawElementsIndirect")
	c.glDrawArraysInstancedBaseInstance = getProcAddr("glDrawArraysInstancedBaseInstance")
	c.glDrawElementsInstancedBaseVertexBaseInstance = getProcAddr("glDrawElementsInstancedBaseVertexBaseInstance")
	c.glUniform1ui = getProcAddr("glUniform1ui")
	c.glMultiDrawArraysIndirect = getProcAddr("glMultiDrawArraysIndirect")
	c.glMultiDrawElementsIndirect = getProcAddr("glMultiDrawElementsIndirect")

	// Compute shaders (optional - may be nil on older GL versions)
	c.glDispatchCompute = getProcAddr("glDispatchCompute")
	c.glDispatchComputeIndirect = getProcAddr("glDispatchComputeIndirect")
	c.glMemoryBarrier = getProcAddr("glMemoryBarrier")
	c.glBindImageTexture = getProcAddr("glBindImageTexture")

	// MSAA (optional - may be nil on older GL versions)
	c.glTexImage2DMultisample = getProcAddr("glTexImage2DMultisample")
	c.glBlitFramebuffer = getProcAddr("glBlitFramebuffer")

	// 3D and 2D array textures (optional - checked with HasLayeredTextures)
	c.glTexImage3D = getProcAddr("glTexImage3D")
	c.glTexSubImage3D = getProcAddr("glTexSubImage3D")
	c.glFramebufferTextureLayer = getProcAddr("glFramebufferTextureLayer")
	// Immutable storage and compressed uploads (optional - checked with HasCompressedTextures)
	c.glTexStorage2D = getProcAddr("glTexStorage2D")
	c.glTexStorage3D = getProcAddr("glTexStorage3D")
	c.glCompressedTexSubImage2D = getProcAddr("glCompressedTexSubImage2D")
	c.glCompressedTexSubImage3D = getProcAddr("glCompressedTexSubImage3D")

	// Sampler objects (optional - GL 3.3+ / ES 3.0+)
	c.glGenSamplers = getProcAddr("glGenSamplers")
	c.glDeleteSamplers = getProcAddr("glDeleteSamplers")
	c.glBindSampler = getProcAddr("glBindSampler")
	c.glSamplerParameteri = getProcAddr("glSamplerParameteri")
	c.glSamplerParameterf = getProcAddr("glSamplerParameterf")

	// Query objects (optional - GL 3.3+ / GL_EXT_disjoint_timer_query)
	c.glGenQueries = getProcAddr("glGenQueries")
	c.glDeleteQueries = getProcAddr("glDeleteQueries")
	c.glQueryCounter = getProcAddr("glQueryCounter")
	c.glGetQueryObjectui64v = getProcAddr("glGetQueryObjectui64v")
	c.glCopyBufferSubData = getProcAddr("glCopyBufferSubData")
	c.glCopyTexSubImage2D = getProcAddr("glCopyTexSubImage2D")
	c.glGetSynciv = getProcAddr("glGetSynciv")

	// Indexed string query (GL 3.0+ / ES 3.0+)
	c.glGetStringi = getProcAddr("glGetStringi")

	return checkRequiredProcs(c.requiredProcs())
}

// requiredProcs lists the entry points called without a nil check (see
// requiredProc).
func (c *Context) requiredProcs() []requiredProc[unsafe.Pointer] {
	return []requiredProc[unsafe.Pointer]{
		{"glGetError", c.glGetError},
		{"glGetString", c.glGetString},
		{"glGetIntegerv", c.glGetIntegerv},
		{"glGetStringi", c.glGetStringi},
		{"glGenVertexArrays", c.glGenVertexArrays},
		{"glBindVertexArray", c.glBindVertexArray},
		{"glDeleteVertexArrays", c.glDeleteVertexArrays},
		{"glGenBuffers", c.glGenBuffers},
		{"glBindBuffer", c.glBindBuffer},
		{"glBufferData", c.glBufferData},
		{"glBufferSubData", c.glBufferSubData},
		{"glBindBufferBase", c.glBindBufferBase},
		{"glBindBufferRange", c.glBindBufferRange},
		{"glCreateShader", c.glCreateShader},
		{"glShaderSource", c.glShaderSource},
		{"glCompileShader", c.glCompileShader},
		{"glCreateProgram", c.glCreateProgram},
		{"glAttachShader", c.glAttachShader},
		{"glLinkProgram", c.glLinkProgram},
		{"glUseProgram", c.glUseProgram},
		{"glGenFramebuffers", c.glGenFramebuffers},
		{"glBindFramebuffer", c.glBindFramebuffer},
		{"glFramebufferTexture2D", c.glFramebufferTexture2D},
		{"glDrawBuffers", c.glDrawBuffers},
		{"glClearBufferfv", c.glClearBufferfv},
		{"glClearBufferiv", c.glClearBufferiv},
		{"glClearBufferuiv", c.glClearBufferuiv},
		{"glDrawArraysInstanced", c.glDrawArraysInstanced},
		{"glDrawElementsInstanced", c.glDrawElementsInstanced},
		{"glVertexAttribDivisor", c.glVertexAttribDivisor},
	}
}

// --- GL Function Wrappers ---
// These use goffi CallFunction to call the loaded function pointers

func (c *Context) GetError() uint32 {
	var result uint32
	_, _ = ffi.CallFunction(&cifUInt32, c.glGetError, unsafe.Pointer(&result), nil)
	return result
}

func (c *Context) GetString(name uint32) string {
	var ptr uintptr
	args := [1]unsafe.Pointer{unsafe.Pointer(&name)}
	_, _ = ffi.CallFunction(&cifPtr1, c.glGetString, unsafe.Pointer(&ptr), args[:])
	if ptr == 0 {
		return ""
	}
	return goString(ptr)
}

func (c *Context) GetIntegerv(pname uint32, data *int32) {
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&pname),
		unsafe.Pointer(&data),
	}
	_, _ = ffi.CallFunction(&cifVoid2, c.glGetIntegerv, nil, args[:])
}

// GetStringi returns an indexed string from an OpenGL string array (GL 3.0+).
// Used for querying GL_EXTENSIONS one at a time (the modern way).
// Returns "" if glGetStringi is not available or the index is out of range.
func (c *Context) GetStringi(name, index uint32) string {
	if c.glGetStringi == nil {
		return ""
	}
	var ptr uintptr
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&name),
		unsafe.Pointer(&index),
	}
	_, _ = ffi.CallFunction(&cifPtr2UU, c.glGetStringi, unsafe.Pointer(&ptr), args[:])
	if ptr == 0 {
		return ""
	}
	return goString(ptr)
}

func (c *Context) Enable(capability uint32) {
	args := [1]unsafe.Pointer{unsafe.Pointer(&capability)}
	_, _ = ffi.CallFunction(&cifVoid1, c.glEnable, nil, args[:])
}

func (c *Context) Disable(capability uint32) {
	args := [1]unsafe.Pointer{unsafe.Pointer(&capability)}
	_, _ = ffi.CallFunction(&cifVoid1, c.glDisable, nil, args[:])
}

func (c *Context) Clear(mask uint32) {
	args := [1]unsafe.Pointer{unsafe.Pointer(&mask)}
	_, _ = ffi.CallFunction(&cifVoid1, c.glClear, nil, args[:])
}

func (c *Context) ClearColor(r, g, b, a float32) {
	args := [4]unsafe.Pointer{
		unsafe.Pointer(&r),
		unsafe.Pointer(&g),
		unsafe.Pointer(&b),
		unsafe.Pointer(&a),
	}
	_, _ = ffi.CallFunction(&cifVoid4Float, c.glClearColor, nil, args[:])
}

// ClearDepth sets the depth value used by glClear(GL_DEPTH_BUFFER_BIT),
// through glClearDepthf, or glClearDepth with a double where only that exists.
// The call interface must match the loaded function: a float passed to the
// double glClearDepth reaches the driver as a near-zero double, so the depth
// cleared to ~0 and every depth-tested draw failed on desktop GL contexts.
func (c *Context) ClearDepth(depth float64) {
	if c.clearDepthDouble {
		args := [1]unsafe.Pointer{unsafe.Pointer(&depth)}
		_, _ = ffi.CallFunction(&cifVoid1Double, c.glClearDepth, nil, args[:])
		return
	}
	d := float32(depth)
	args := [1]unsafe.Pointer{unsafe.Pointer(&d)}
	_, _ = ffi.CallFunction(&cifVoid1Float, c.glClearDepth, nil, args[:])
}

// ClearStencil sets the stencil value used by glClear(GL_STENCIL_BUFFER_BIT).
func (c *Context) ClearStencil(s int32) {
	args := [1]unsafe.Pointer{unsafe.Pointer(&s)}
	_, _ = ffi.CallFunction(&cifVoid1, c.glClearStencil, nil, args[:])
}

// DepthRange sets the mapping of NDC depth to window depth, through
// glDepthRangef, or glDepthRange with doubles where only that exists.
func (c *Context) DepthRange(near, far float64) {
	if c.depthRangeDouble {
		args := [2]unsafe.Pointer{unsafe.Pointer(&near), unsafe.Pointer(&far)}
		_, _ = ffi.CallFunction(&cifVoid2Double, c.glDepthRange, nil, args[:])
		return
	}
	n, f := float32(near), float32(far)
	args := [2]unsafe.Pointer{unsafe.Pointer(&n), unsafe.Pointer(&f)}
	_, _ = ffi.CallFunction(&cifVoid2Float, c.glDepthRange, nil, args[:])
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
	_, _ = ffi.CallFunction(&cifVoid4, c.glViewport, nil, args[:])
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
	_, _ = ffi.CallFunction(&cifVoid4, c.glScissor, nil, args[:])
}

func (c *Context) DrawArrays(mode uint32, first, count int32) {
	ufirst, ucount := uint32(first), uint32(count)
	args := [3]unsafe.Pointer{
		unsafe.Pointer(&mode),
		unsafe.Pointer(&ufirst),
		unsafe.Pointer(&ucount),
	}
	_, _ = ffi.CallFunction(&cifVoid3, c.glDrawArrays, nil, args[:])
}

func (c *Context) DrawElements(mode uint32, count int32, typ uint32, indices uintptr) {
	ucount := uint32(count)
	args := [4]unsafe.Pointer{
		unsafe.Pointer(&mode),
		unsafe.Pointer(&ucount),
		unsafe.Pointer(&typ),
		unsafe.Pointer(&indices),
	}
	_, _ = ffi.CallFunction(&cifVoid4, c.glDrawElements, nil, args[:])
}

func (c *Context) Flush() {
	_, _ = ffi.CallFunction(&cifVoid, c.glFlush, nil, nil)
}

func (c *Context) Finish() {
	_, _ = ffi.CallFunction(&cifVoid, c.glFinish, nil, nil)
}

// --- Shaders ---

func (c *Context) CreateShader(shaderType uint32) uint32 {
	var result uint32
	args := [1]unsafe.Pointer{unsafe.Pointer(&shaderType)}
	_, _ = ffi.CallFunction(&cifUInt321, c.glCreateShader, unsafe.Pointer(&result), args[:])
	return result
}

func (c *Context) DeleteShader(shader uint32) {
	args := [1]unsafe.Pointer{unsafe.Pointer(&shader)}
	_, _ = ffi.CallFunction(&cifVoid1, c.glDeleteShader, nil, args[:])
}

func (c *Context) ShaderSource(shader uint32, source string) {
	csource, free := cString(source)
	defer free()
	count := int32(1)
	length := int32(len(source))
	// goffi PointerType: FFI reads 8 bytes from args[i] to fill the register.
	// For a pointer argument P, args[i] must point to a location that contains P,
	// so that the FFI dereference yields P — not *P.
	stringsPtr := &csource // **byte; FFI reads csource → OpenGL gets &csource as strings
	lengthsPtr := &length  // *int32; FFI reads &length → OpenGL gets &length as lengths
	args := [4]unsafe.Pointer{
		unsafe.Pointer(&shader),
		unsafe.Pointer(&count),
		unsafe.Pointer(&stringsPtr),
		unsafe.Pointer(&lengthsPtr),
	}
	_, _ = ffi.CallFunction(&cifVoid4Shader, c.glShaderSource, nil, args[:])
}

func (c *Context) CompileShader(shader uint32) {
	args := [1]unsafe.Pointer{unsafe.Pointer(&shader)}
	_, _ = ffi.CallFunction(&cifVoid1, c.glCompileShader, nil, args[:])
}

func (c *Context) GetShaderiv(shader uint32, pname uint32, params *int32) {
	args := [3]unsafe.Pointer{
		unsafe.Pointer(&shader),
		unsafe.Pointer(&pname),
		unsafe.Pointer(&params), // FFI reads params (= &int32) → OpenGL writes output to *params
	}
	_, _ = ffi.CallFunction(&cifVoid3Shader, c.glGetShaderiv, nil, args[:])
}

func (c *Context) GetShaderInfoLog(shader uint32) string {
	var length int32
	c.GetShaderiv(shader, INFO_LOG_LENGTH, &length)
	if length == 0 {
		return ""
	}
	buf := make([]byte, length)
	maxLen := uint32(length)
	lenPtr := &length // *int32; FFI reads &length → OpenGL writes actual log length
	bufPtr := &buf[0] // *byte;  FFI reads &buf[0]  → OpenGL writes log text
	args := [4]unsafe.Pointer{
		unsafe.Pointer(&shader),
		unsafe.Pointer(&maxLen),
		unsafe.Pointer(&lenPtr),
		unsafe.Pointer(&bufPtr),
	}
	_, _ = ffi.CallFunction(&cifVoid4Log, c.glGetShaderInfoLog, nil, args[:])
	return string(buf[:length])
}

func (c *Context) CreateProgram() uint32 {
	var result uint32
	_, _ = ffi.CallFunction(&cifUInt32, c.glCreateProgram, unsafe.Pointer(&result), nil)
	return result
}

func (c *Context) DeleteProgram(program uint32) {
	args := [1]unsafe.Pointer{unsafe.Pointer(&program)}
	_, _ = ffi.CallFunction(&cifVoid1, c.glDeleteProgram, nil, args[:])
}

func (c *Context) AttachShader(program, shader uint32) {
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&program),
		unsafe.Pointer(&shader),
	}
	_, _ = ffi.CallFunction(&cifVoid2UU, c.glAttachShader, nil, args[:])
}

func (c *Context) LinkProgram(program uint32) {
	args := [1]unsafe.Pointer{unsafe.Pointer(&program)}
	_, _ = ffi.CallFunction(&cifVoid1, c.glLinkProgram, nil, args[:])
}

func (c *Context) UseProgram(program uint32) {
	args := [1]unsafe.Pointer{unsafe.Pointer(&program)}
	_, _ = ffi.CallFunction(&cifVoid1, c.glUseProgram, nil, args[:])
}

func (c *Context) GetProgramiv(program uint32, pname uint32, params *int32) {
	args := [3]unsafe.Pointer{
		unsafe.Pointer(&program),
		unsafe.Pointer(&pname),
		unsafe.Pointer(&params), // FFI reads params (= &int32) → OpenGL writes output to *params
	}
	_, _ = ffi.CallFunction(&cifVoid3Shader, c.glGetProgramiv, nil, args[:])
}

func (c *Context) GetProgramInfoLog(program uint32) string {
	var length int32
	c.GetProgramiv(program, INFO_LOG_LENGTH, &length)
	if length == 0 {
		return ""
	}
	buf := make([]byte, length)
	maxLen := uint32(length)
	lenPtr := &length // *int32; FFI reads &length → OpenGL writes actual log length
	bufPtr := &buf[0] // *byte;  FFI reads &buf[0]  → OpenGL writes log text
	args := [4]unsafe.Pointer{
		unsafe.Pointer(&program),
		unsafe.Pointer(&maxLen),
		unsafe.Pointer(&lenPtr),
		unsafe.Pointer(&bufPtr),
	}
	_, _ = ffi.CallFunction(&cifVoid4Log, c.glGetProgramInfoLog, nil, args[:])
	return string(buf[:length])
}

func (c *Context) GetUniformLocation(program uint32, name string) int32 {
	cname, free := cString(name)
	defer free()
	var result int32
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&program),
		unsafe.Pointer(&cname), // FFI reads cname → OpenGL gets char* name
	}
	_, _ = ffi.CallFunction(&cifInt322, c.glGetUniformLocation, unsafe.Pointer(&result), args[:])
	return result
}

func (c *Context) GetAttribLocation(program uint32, name string) int32 {
	cname, free := cString(name)
	defer free()
	var result int32
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&program),
		unsafe.Pointer(&cname), // FFI reads cname → OpenGL gets char* name
	}
	_, _ = ffi.CallFunction(&cifInt322, c.glGetAttribLocation, unsafe.Pointer(&result), args[:])
	return result
}

// --- Buffers ---

func (c *Context) GenBuffers(n int32) uint32 {
	var buffer uint32
	pBuffer := &buffer // *uint32; FFI reads &buffer → OpenGL writes new ID into buffer
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&n),
		unsafe.Pointer(&pBuffer),
	}
	_, _ = ffi.CallFunction(&cifVoid2, c.glGenBuffers, nil, args[:])
	return buffer
}

func (c *Context) DeleteBuffers(buffers ...uint32) {
	n := int32(len(buffers))
	pBuffers := &buffers[0] // *uint32; FFI reads &buffers[0] → OpenGL reads IDs from slice
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&n),
		unsafe.Pointer(&pBuffers),
	}
	_, _ = ffi.CallFunction(&cifVoid2, c.glDeleteBuffers, nil, args[:])
}

func (c *Context) BindBuffer(target, buffer uint32) {
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&target),
		unsafe.Pointer(&buffer),
	}
	_, _ = ffi.CallFunction(&cifVoid2UU, c.glBindBuffer, nil, args[:])
}

func (c *Context) BufferData(target uint32, size int, data unsafe.Pointer, usage uint32) {
	sizePtr := uintptr(size)
	args := [4]unsafe.Pointer{
		unsafe.Pointer(&target),
		unsafe.Pointer(&sizePtr),
		unsafe.Pointer(&data), // unsafe.Pointer keeps the source alive for the call; FFI reads data → OpenGL gets void*
		unsafe.Pointer(&usage),
	}
	_, _ = ffi.CallFunction(&cifVoid4Buffer, c.glBufferData, nil, args[:])
}

// AllocateBuffer gives the buffer bound to target size bytes of undefined
// content (glBufferData with a null pointer).
func (c *Context) AllocateBuffer(target uint32, size int, usage uint32) {
	c.BufferData(target, size, nil, usage)
}

func (c *Context) BufferSubData(target uint32, offset, size int, data unsafe.Pointer) {
	offsetPtr := uintptr(offset)
	sizePtr := uintptr(size)
	dataPtr := data // unsafe.Pointer; FFI reads dataPtr → OpenGL gets void*
	args := [4]unsafe.Pointer{
		unsafe.Pointer(&target),
		unsafe.Pointer(&offsetPtr),
		unsafe.Pointer(&sizePtr),
		unsafe.Pointer(&dataPtr),
	}
	_, _ = ffi.CallFunction(&cifVoid4SubBuf, c.glBufferSubData, nil, args[:])
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
	_, _ = ffi.CallFunction(&cifPtr2UU, c.glMapBuffer, unsafe.Pointer(&ptr), args[:])
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
	_, _ = ffi.CallFunction(&cifUInt321, c.glUnmapBuffer, unsafe.Pointer(&result), args[:])
	return result != 0
}

// --- VAO ---

func (c *Context) GenVertexArrays(n int32) uint32 {
	var vao uint32
	pVao := &vao
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&n),
		unsafe.Pointer(&pVao),
	}
	_, _ = ffi.CallFunction(&cifVoid2, c.glGenVertexArrays, nil, args[:])
	return vao
}

func (c *Context) DeleteVertexArrays(arrays ...uint32) {
	n := int32(len(arrays))
	pArrays := &arrays[0]
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&n),
		unsafe.Pointer(&pArrays),
	}
	_, _ = ffi.CallFunction(&cifVoid2, c.glDeleteVertexArrays, nil, args[:])
}

func (c *Context) BindVertexArray(array uint32) {
	args := [1]unsafe.Pointer{unsafe.Pointer(&array)}
	_, _ = ffi.CallFunction(&cifVoid1, c.glBindVertexArray, nil, args[:])
}

// --- Vertex Attributes ---

func (c *Context) EnableVertexAttribArray(index uint32) {
	args := [1]unsafe.Pointer{unsafe.Pointer(&index)}
	_, _ = ffi.CallFunction(&cifVoid1, c.glEnableVertexAttribArray, nil, args[:])
}

func (c *Context) DisableVertexAttribArray(index uint32) {
	args := [1]unsafe.Pointer{unsafe.Pointer(&index)}
	_, _ = ffi.CallFunction(&cifVoid1, c.glDisableVertexAttribArray, nil, args[:])
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
	_, _ = ffi.CallFunction(&cifVoid6Attrib, c.glVertexAttribPointer, nil, args[:])
}

// VertexAttribDivisor sets the instance divisor for a vertex attribute.
// divisor=0 means per-vertex, divisor=1 means per-instance.
// Matches Rust wgpu-hal/src/gles/queue.rs vertex_attrib_divisor call.
func (c *Context) VertexAttribDivisor(index, divisor uint32) {
	if c.glVertexAttribDivisor == nil {
		return
	}
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&index),
		unsafe.Pointer(&divisor),
	}
	_, _ = ffi.CallFunction(&cifVoid2UU, c.glVertexAttribDivisor, nil, args[:])
}

// --- Textures ---

func (c *Context) GenTextures(n int32) uint32 {
	var tex uint32
	pTex := &tex
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&n),
		unsafe.Pointer(&pTex),
	}
	_, _ = ffi.CallFunction(&cifVoid2, c.glGenTextures, nil, args[:])
	return tex
}

func (c *Context) DeleteTextures(textures ...uint32) {
	n := int32(len(textures))
	pTextures := &textures[0]
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&n),
		unsafe.Pointer(&pTextures),
	}
	_, _ = ffi.CallFunction(&cifVoid2, c.glDeleteTextures, nil, args[:])
}

func (c *Context) BindTexture(target, texture uint32) {
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&target),
		unsafe.Pointer(&texture),
	}
	_, _ = ffi.CallFunction(&cifVoid2UU, c.glBindTexture, nil, args[:])
}

func (c *Context) ActiveTexture(texture uint32) {
	args := [1]unsafe.Pointer{unsafe.Pointer(&texture)}
	_, _ = ffi.CallFunction(&cifVoid1, c.glActiveTexture, nil, args[:])
}

func (c *Context) TexParameteri(target, pname uint32, param int32) {
	args := [3]unsafe.Pointer{
		unsafe.Pointer(&target),
		unsafe.Pointer(&pname),
		unsafe.Pointer(&param),
	}
	_, _ = ffi.CallFunction(&cifVoid3, c.glTexParameteri, nil, args[:])
}

func (c *Context) TexImage2D(target uint32, level int32, internalformat int32, width, height int32, border int32, format, typ uint32, pixels unsafe.Pointer) {
	args := [9]unsafe.Pointer{
		unsafe.Pointer(&target),
		unsafe.Pointer(&level),
		unsafe.Pointer(&internalformat),
		unsafe.Pointer(&width),
		unsafe.Pointer(&height),
		unsafe.Pointer(&border),
		unsafe.Pointer(&format),
		unsafe.Pointer(&typ),
		unsafe.Pointer(&pixels),
	}
	_, _ = ffi.CallFunction(&cifVoid9TexImg, c.glTexImage2D, nil, args[:])
}

func (c *Context) TexSubImage2D(target uint32, level int32, xoffset, yoffset, width, height int32, format, typ uint32, pixels unsafe.Pointer) {
	pixelsPtr := pixels
	args := [9]unsafe.Pointer{
		unsafe.Pointer(&target),
		unsafe.Pointer(&level),
		unsafe.Pointer(&xoffset),
		unsafe.Pointer(&yoffset),
		unsafe.Pointer(&width),
		unsafe.Pointer(&height),
		unsafe.Pointer(&format),
		unsafe.Pointer(&typ),
		unsafe.Pointer(&pixelsPtr),
	}
	_, _ = ffi.CallFunction(&cifVoid9TexImg, c.glTexSubImage2D, nil, args[:])
}

func (c *Context) GenerateMipmap(target uint32) {
	args := [1]unsafe.Pointer{unsafe.Pointer(&target)}
	_, _ = ffi.CallFunction(&cifVoid1, c.glGenerateMipmap, nil, args[:])
}

// initLayeredTextureCallInterfaces prepares the signatures of the 3D and 2D
// array texture functions.
func initLayeredTextureCallInterfaces() error {
	u32, s32, ptr := types.UInt32TypeDescriptor, types.SInt32TypeDescriptor, types.PointerTypeDescriptor
	if err := ffi.PrepareCallInterface(&cifVoid10TexImg3D, types.DefaultCall, types.VoidTypeDescriptor,
		[]*types.TypeDescriptor{u32, s32, s32, s32, s32, s32, s32, u32, u32, ptr}); err != nil {
		return err
	}
	if err := ffi.PrepareCallInterface(&cifVoid11TexSubImg3D, types.DefaultCall, types.VoidTypeDescriptor,
		[]*types.TypeDescriptor{u32, s32, s32, s32, s32, s32, s32, s32, u32, u32, ptr}); err != nil {
		return err
	}
	return ffi.PrepareCallInterface(&cifVoid5FBOLayer, types.DefaultCall, types.VoidTypeDescriptor,
		[]*types.TypeDescriptor{u32, u32, u32, s32, s32})
}

// HasLayeredTextures reports whether the context has the functions 3D and
// 2D array textures need: glTexImage3D, glTexSubImage3D and
// glFramebufferTextureLayer (GL 3.0+, ES 3.0+).
func (c *Context) HasLayeredTextures() bool {
	return c.glTexImage3D != nil && c.glTexSubImage3D != nil && c.glFramebufferTextureLayer != nil
}

// TexImage3D allocates (pixels nil) or fills one mip level of a 3D or 2D
// array texture. No-op if not supported (HasLayeredTextures).
func (c *Context) TexImage3D(target uint32, level, internalformat, width, height, depth, border int32, format, typ uint32, pixels unsafe.Pointer) {
	if c.glTexImage3D == nil {
		return
	}
	args := [10]unsafe.Pointer{
		unsafe.Pointer(&target),
		unsafe.Pointer(&level),
		unsafe.Pointer(&internalformat),
		unsafe.Pointer(&width),
		unsafe.Pointer(&height),
		unsafe.Pointer(&depth),
		unsafe.Pointer(&border),
		unsafe.Pointer(&format),
		unsafe.Pointer(&typ),
		unsafe.Pointer(&pixels),
	}
	_, _ = ffi.CallFunction(&cifVoid10TexImg3D, c.glTexImage3D, nil, args[:])
}

// TexSubImage3D writes a box of one mip level of a 3D or 2D array texture.
// No-op if not supported (HasLayeredTextures).
func (c *Context) TexSubImage3D(target uint32, level, xoffset, yoffset, zoffset, width, height, depth int32, format, typ uint32, pixels unsafe.Pointer) {
	if c.glTexSubImage3D == nil {
		return
	}
	pixelsPtr := pixels
	args := [11]unsafe.Pointer{
		unsafe.Pointer(&target),
		unsafe.Pointer(&level),
		unsafe.Pointer(&xoffset),
		unsafe.Pointer(&yoffset),
		unsafe.Pointer(&zoffset),
		unsafe.Pointer(&width),
		unsafe.Pointer(&height),
		unsafe.Pointer(&depth),
		unsafe.Pointer(&format),
		unsafe.Pointer(&typ),
		unsafe.Pointer(&pixelsPtr),
	}
	_, _ = ffi.CallFunction(&cifVoid11TexSubImg3D, c.glTexSubImage3D, nil, args[:])
}

// initCompressedTextureCallInterfaces prepares the signatures of the
// immutable storage and compressed upload functions.
func initCompressedTextureCallInterfaces() error {
	u32, s32, ptr := types.UInt32TypeDescriptor, types.SInt32TypeDescriptor, types.PointerTypeDescriptor
	if err := ffi.PrepareCallInterface(&cifVoid5TexStorage2D, types.DefaultCall, types.VoidTypeDescriptor,
		[]*types.TypeDescriptor{u32, s32, u32, s32, s32}); err != nil {
		return err
	}
	if err := ffi.PrepareCallInterface(&cifVoid6TexStorage3D, types.DefaultCall, types.VoidTypeDescriptor,
		[]*types.TypeDescriptor{u32, s32, u32, s32, s32, s32}); err != nil {
		return err
	}
	if err := ffi.PrepareCallInterface(&cifVoid9CompSubImg2D, types.DefaultCall, types.VoidTypeDescriptor,
		[]*types.TypeDescriptor{u32, s32, s32, s32, s32, s32, u32, s32, ptr}); err != nil {
		return err
	}
	return ffi.PrepareCallInterface(&cifVoid11CompSubImg3D, types.DefaultCall, types.VoidTypeDescriptor,
		[]*types.TypeDescriptor{u32, s32, s32, s32, s32, s32, s32, s32, u32, s32, ptr})
}

// HasCompressedTextures reports whether the context resolved the functions
// compressed textures need: glTexStorage2D, glTexStorage3D,
// glCompressedTexSubImage2D and glCompressedTexSubImage3D (GL 4.2, ES 3.0).
// A resolved pointer is not proof of support on EGL: check the version too.
func (c *Context) HasCompressedTextures() bool {
	return c.glTexStorage2D != nil && c.glTexStorage3D != nil &&
		c.glCompressedTexSubImage2D != nil && c.glCompressedTexSubImage3D != nil
}

// TexStorage2D allocates immutable storage for every level of a 2D texture
// or a cube map. No-op if not supported (HasCompressedTextures).
func (c *Context) TexStorage2D(target uint32, levels int32, internalformat uint32, width, height int32) {
	if c.glTexStorage2D == nil {
		return
	}
	args := [5]unsafe.Pointer{
		unsafe.Pointer(&target),
		unsafe.Pointer(&levels),
		unsafe.Pointer(&internalformat),
		unsafe.Pointer(&width),
		unsafe.Pointer(&height),
	}
	_, _ = ffi.CallFunction(&cifVoid5TexStorage2D, c.glTexStorage2D, nil, args[:])
}

// TexStorage3D allocates immutable storage for every level of a 3D or 2D
// array texture. No-op if not supported (HasCompressedTextures).
func (c *Context) TexStorage3D(target uint32, levels int32, internalformat uint32, width, height, depth int32) {
	if c.glTexStorage3D == nil {
		return
	}
	args := [6]unsafe.Pointer{
		unsafe.Pointer(&target),
		unsafe.Pointer(&levels),
		unsafe.Pointer(&internalformat),
		unsafe.Pointer(&width),
		unsafe.Pointer(&height),
		unsafe.Pointer(&depth),
	}
	_, _ = ffi.CallFunction(&cifVoid6TexStorage3D, c.glTexStorage3D, nil, args[:])
}

// CompressedTexSubImage2D writes imageSize bytes of blocks into a rectangle
// of one level of a 2D image (TEXTURE_2D or a cube face). data is client
// memory, or a byte offset when a PIXEL_UNPACK_BUFFER is bound. No-op if not
// supported (HasCompressedTextures).
func (c *Context) CompressedTexSubImage2D(target uint32, level, xoffset, yoffset, width, height int32, format uint32, imageSize int32, data unsafe.Pointer) {
	if c.glCompressedTexSubImage2D == nil {
		return
	}
	dataPtr := data
	args := [9]unsafe.Pointer{
		unsafe.Pointer(&target),
		unsafe.Pointer(&level),
		unsafe.Pointer(&xoffset),
		unsafe.Pointer(&yoffset),
		unsafe.Pointer(&width),
		unsafe.Pointer(&height),
		unsafe.Pointer(&format),
		unsafe.Pointer(&imageSize),
		unsafe.Pointer(&dataPtr),
	}
	_, _ = ffi.CallFunction(&cifVoid9CompSubImg2D, c.glCompressedTexSubImage2D, nil, args[:])
}

// CompressedTexSubImage3D writes imageSize bytes of blocks into a box of one
// level of a 3D or 2D array texture. data is client memory, or a byte offset
// when a PIXEL_UNPACK_BUFFER is bound. No-op if not supported
// (HasCompressedTextures).
func (c *Context) CompressedTexSubImage3D(target uint32, level, xoffset, yoffset, zoffset, width, height, depth int32, format uint32, imageSize int32, data unsafe.Pointer) {
	if c.glCompressedTexSubImage3D == nil {
		return
	}
	dataPtr := data
	args := [11]unsafe.Pointer{
		unsafe.Pointer(&target),
		unsafe.Pointer(&level),
		unsafe.Pointer(&xoffset),
		unsafe.Pointer(&yoffset),
		unsafe.Pointer(&zoffset),
		unsafe.Pointer(&width),
		unsafe.Pointer(&height),
		unsafe.Pointer(&depth),
		unsafe.Pointer(&format),
		unsafe.Pointer(&imageSize),
		unsafe.Pointer(&dataPtr),
	}
	_, _ = ffi.CallFunction(&cifVoid11CompSubImg3D, c.glCompressedTexSubImage3D, nil, args[:])
}

// FramebufferTextureLayer attaches one layer (a depth slice of a 3D texture
// or a layer of an array) of one mip level to the bound framebuffer.
// No-op if not supported (HasLayeredTextures).
func (c *Context) FramebufferTextureLayer(target, attachment, texture uint32, level, layer int32) {
	if c.glFramebufferTextureLayer == nil {
		return
	}
	args := [5]unsafe.Pointer{
		unsafe.Pointer(&target),
		unsafe.Pointer(&attachment),
		unsafe.Pointer(&texture),
		unsafe.Pointer(&level),
		unsafe.Pointer(&layer),
	}
	_, _ = ffi.CallFunction(&cifVoid5FBOLayer, c.glFramebufferTextureLayer, nil, args[:])
}

// TexImage2DMultisample creates a multisample 2D texture image.
// Requires OpenGL 3.2+ or OpenGL ES 3.1+.
// No-op if not supported.
func (c *Context) TexImage2DMultisample(target uint32, samples int32, internalformat uint32, width, height int32, fixedsamplelocations bool) {
	if c.glTexImage2DMultisample == nil {
		return
	}
	var fixed uint8
	if fixedsamplelocations {
		fixed = 1
	}
	args := [6]unsafe.Pointer{
		unsafe.Pointer(&target),
		unsafe.Pointer(&samples),
		unsafe.Pointer(&internalformat),
		unsafe.Pointer(&width),
		unsafe.Pointer(&height),
		unsafe.Pointer(&fixed),
	}
	_, _ = ffi.CallFunction(&cifVoid6TexMS, c.glTexImage2DMultisample, nil, args[:])
}

// BlitFramebuffer copies a block of pixels between framebuffers.
// Requires OpenGL 3.0+ or OpenGL ES 3.0+.
// No-op if not supported.
func (c *Context) BlitFramebuffer(srcX0, srcY0, srcX1, srcY1, dstX0, dstY0, dstX1, dstY1 int32, mask, filter uint32) {
	if c.glBlitFramebuffer == nil {
		return
	}
	args := [10]unsafe.Pointer{
		unsafe.Pointer(&srcX0),
		unsafe.Pointer(&srcY0),
		unsafe.Pointer(&srcX1),
		unsafe.Pointer(&srcY1),
		unsafe.Pointer(&dstX0),
		unsafe.Pointer(&dstY0),
		unsafe.Pointer(&dstX1),
		unsafe.Pointer(&dstY1),
		unsafe.Pointer(&mask),
		unsafe.Pointer(&filter),
	}
	_, _ = ffi.CallFunction(&cifVoid10Blit, c.glBlitFramebuffer, nil, args[:])
}

// --- Sampler Objects ---

// GenSamplers generates sampler object names.
// Requires OpenGL 3.3+ or OpenGL ES 3.0+.
// Returns 0 if sampler objects are not supported.
func (c *Context) GenSamplers(n int32) uint32 {
	if c.glGenSamplers == nil {
		return 0
	}
	var sampler uint32
	pSampler := &sampler
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&n),
		unsafe.Pointer(&pSampler),
	}
	_, _ = ffi.CallFunction(&cifVoid2, c.glGenSamplers, nil, args[:])
	return sampler
}

// DeleteSamplers deletes sampler objects.
// No-op if sampler objects are not supported.
func (c *Context) DeleteSamplers(samplers ...uint32) {
	if c.glDeleteSamplers == nil {
		return
	}
	n := int32(len(samplers))
	pSamplers := &samplers[0]
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&n),
		unsafe.Pointer(&pSamplers),
	}
	_, _ = ffi.CallFunction(&cifVoid2, c.glDeleteSamplers, nil, args[:])
}

// BindSampler binds a sampler object to a texture unit.
// No-op if sampler objects are not supported.
func (c *Context) BindSampler(unit, sampler uint32) {
	if c.glBindSampler == nil {
		return
	}
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&unit),
		unsafe.Pointer(&sampler),
	}
	_, _ = ffi.CallFunction(&cifVoid2UU, c.glBindSampler, nil, args[:])
}

// SamplerParameteri sets an integer parameter on a sampler object.
// No-op if sampler objects are not supported.
func (c *Context) SamplerParameteri(sampler, pname uint32, param int32) {
	if c.glSamplerParameteri == nil {
		return
	}
	args := [3]unsafe.Pointer{
		unsafe.Pointer(&sampler),
		unsafe.Pointer(&pname),
		unsafe.Pointer(&param),
	}
	_, _ = ffi.CallFunction(&cifVoid3, c.glSamplerParameteri, nil, args[:])
}

// SamplerParameterf sets a float parameter on a sampler object.
// No-op if sampler objects are not supported.
func (c *Context) SamplerParameterf(sampler, pname uint32, param float32) {
	if c.glSamplerParameterf == nil {
		return
	}
	args := [3]unsafe.Pointer{
		unsafe.Pointer(&sampler),
		unsafe.Pointer(&pname),
		unsafe.Pointer(&param),
	}
	_, _ = ffi.CallFunction(&cifVoid3UUF, c.glSamplerParameterf, nil, args[:])
}

// SupportsSamplerObjects returns true if GL sampler objects are supported.
func (c *Context) SupportsSamplerObjects() bool {
	return c.glGenSamplers != nil
}

// --- Framebuffers ---

func (c *Context) GenFramebuffers(n int32) uint32 {
	var fbo uint32
	pFbo := &fbo
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&n),
		unsafe.Pointer(&pFbo),
	}
	_, _ = ffi.CallFunction(&cifVoid2, c.glGenFramebuffers, nil, args[:])
	return fbo
}

func (c *Context) DeleteFramebuffers(framebuffers ...uint32) {
	n := int32(len(framebuffers))
	pFramebuffers := &framebuffers[0]
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&n),
		unsafe.Pointer(&pFramebuffers),
	}
	_, _ = ffi.CallFunction(&cifVoid2, c.glDeleteFramebuffers, nil, args[:])
}

func (c *Context) BindFramebuffer(target, framebuffer uint32) {
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&target),
		unsafe.Pointer(&framebuffer),
	}
	_, _ = ffi.CallFunction(&cifVoid2UU, c.glBindFramebuffer, nil, args[:])
}

func (c *Context) FramebufferTexture2D(target, attachment, textarget, texture uint32, level int32) {
	args := [5]unsafe.Pointer{
		unsafe.Pointer(&target),
		unsafe.Pointer(&attachment),
		unsafe.Pointer(&textarget),
		unsafe.Pointer(&texture),
		unsafe.Pointer(&level),
	}
	_, _ = ffi.CallFunction(&cifVoid5FBO, c.glFramebufferTexture2D, nil, args[:])
}

func (c *Context) CheckFramebufferStatus(target uint32) uint32 {
	var result uint32
	args := [1]unsafe.Pointer{unsafe.Pointer(&target)}
	_, _ = ffi.CallFunction(&cifUInt321, c.glCheckFramebufferStatus, unsafe.Pointer(&result), args[:])
	return result
}

// DrawBuffers specifies a list of color buffers to be drawn into.
// Matches Rust wgpu-hal GLES SetDrawColorBuffers (queue.rs:1202-1207).
func (c *Context) DrawBuffers(bufs []uint32) {
	if len(bufs) == 0 {
		return
	}
	n := int32(len(bufs))
	pBufs := &bufs[0]
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&n),
		unsafe.Pointer(&pBufs),
	}
	_, _ = ffi.CallFunction(&cifVoid2, c.glDrawBuffers, nil, args[:])
}

// ClearBufferfv clears a specific draw buffer with float values.
// buffer must be GL_COLOR, drawBuffer is the index (0..MAX_DRAW_BUFFERS-1),
// value points to 4 float32 values (RGBA).
// Matches Rust wgpu-hal GLES ClearColorF (queue.rs:1222).
func (c *Context) ClearBufferfv(buffer uint32, drawBuffer int32, value *[4]float32) {
	db := uint32(drawBuffer)
	pValue := unsafe.Pointer(value)
	args := [3]unsafe.Pointer{
		unsafe.Pointer(&buffer),
		unsafe.Pointer(&db),
		unsafe.Pointer(&pValue),
	}
	_, _ = ffi.CallFunction(&cifVoid3Shader, c.glClearBufferfv, nil, args[:])
}

// ClearBufferiv clears a specific draw buffer of a signed integer format
// with integer values. glClearBufferfv on an integer buffer leaves its
// contents undefined (OpenGL ES 3.0 §4.2.3, Clearing the Buffers).
func (c *Context) ClearBufferiv(buffer uint32, drawBuffer int32, value *[4]int32) {
	db := uint32(drawBuffer)
	pValue := unsafe.Pointer(value)
	args := [3]unsafe.Pointer{
		unsafe.Pointer(&buffer),
		unsafe.Pointer(&db),
		unsafe.Pointer(&pValue),
	}
	_, _ = ffi.CallFunction(&cifVoid3Shader, c.glClearBufferiv, nil, args[:])
}

// ClearBufferuiv clears a specific draw buffer of an unsigned integer format
// with integer values.
func (c *Context) ClearBufferuiv(buffer uint32, drawBuffer int32, value *[4]uint32) {
	db := uint32(drawBuffer)
	pValue := unsafe.Pointer(value)
	args := [3]unsafe.Pointer{
		unsafe.Pointer(&buffer),
		unsafe.Pointer(&db),
		unsafe.Pointer(&pValue),
	}
	_, _ = ffi.CallFunction(&cifVoid3Shader, c.glClearBufferuiv, nil, args[:])
}

// --- Renderbuffers ---

// GenRenderbuffers generates a single renderbuffer object and returns its name.
func (c *Context) GenRenderbuffers(n int32) uint32 {
	var rbo uint32
	pRbo := &rbo
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&n),
		unsafe.Pointer(&pRbo),
	}
	_, _ = ffi.CallFunction(&cifVoid2, c.glGenRenderbuffers, nil, args[:])
	return rbo
}

// DeleteRenderbuffers deletes the named renderbuffer objects.
func (c *Context) DeleteRenderbuffers(renderbuffers ...uint32) {
	if len(renderbuffers) == 0 {
		return
	}
	n := int32(len(renderbuffers))
	pRenderbuffers := &renderbuffers[0]
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&n),
		unsafe.Pointer(&pRenderbuffers),
	}
	_, _ = ffi.CallFunction(&cifVoid2, c.glDeleteRenderbuffers, nil, args[:])
}

// BindRenderbuffer binds a renderbuffer object to the given target
// (typically GL_RENDERBUFFER).
func (c *Context) BindRenderbuffer(target, renderbuffer uint32) {
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&target),
		unsafe.Pointer(&renderbuffer),
	}
	_, _ = ffi.CallFunction(&cifVoid2UU, c.glBindRenderbuffer, nil, args[:])
}

// RenderbufferStorage allocates storage for the currently bound renderbuffer.
// internalFormat is a sized format like GL_RGBA8 or GL_DEPTH24_STENCIL8.
func (c *Context) RenderbufferStorage(target, internalFormat uint32, width, height int32) {
	uw, uh := uint32(width), uint32(height)
	args := [4]unsafe.Pointer{
		unsafe.Pointer(&target),
		unsafe.Pointer(&internalFormat),
		unsafe.Pointer(&uw),
		unsafe.Pointer(&uh),
	}
	_, _ = ffi.CallFunction(&cifVoid4, c.glRenderbufferStorage, nil, args[:])
}

// FramebufferRenderbuffer attaches a renderbuffer to the currently bound
// framebuffer at the specified attachment point.
func (c *Context) FramebufferRenderbuffer(target, attachment, renderbufferTarget, renderbuffer uint32) {
	args := [4]unsafe.Pointer{
		unsafe.Pointer(&target),
		unsafe.Pointer(&attachment),
		unsafe.Pointer(&renderbufferTarget),
		unsafe.Pointer(&renderbuffer),
	}
	_, _ = ffi.CallFunction(&cifVoid4, c.glFramebufferRenderbuffer, nil, args[:])
}

// --- Pixel Read/Store ---

// ReadPixels reads a block of pixels from the framebuffer.
func (c *Context) ReadPixels(x, y, width, height int32, format, dataType uint32, pixels unsafe.Pointer) {
	pixelsPtr := pixels
	args := [7]unsafe.Pointer{
		unsafe.Pointer(&x),
		unsafe.Pointer(&y),
		unsafe.Pointer(&width),
		unsafe.Pointer(&height),
		unsafe.Pointer(&format),
		unsafe.Pointer(&dataType),
		unsafe.Pointer(&pixelsPtr),
	}
	_, _ = ffi.CallFunction(&cifVoid7ReadPx, c.glReadPixels, nil, args[:])
}

// PixelStorei sets pixel storage modes that affect ReadPixels and TexImage operations.
func (c *Context) PixelStorei(pname uint32, param int32) {
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&pname),
		unsafe.Pointer(&param),
	}
	_, _ = ffi.CallFunction(&cifVoid2UU, c.glPixelStorei, nil, args[:])
}

// --- Blending ---

func (c *Context) BlendFunc(sfactor, dfactor uint32) {
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&sfactor),
		unsafe.Pointer(&dfactor),
	}
	_, _ = ffi.CallFunction(&cifVoid2UU, c.glBlendFunc, nil, args[:])
}

func (c *Context) BlendFuncSeparate(srcRGB, dstRGB, srcAlpha, dstAlpha uint32) {
	args := [4]unsafe.Pointer{
		unsafe.Pointer(&srcRGB),
		unsafe.Pointer(&dstRGB),
		unsafe.Pointer(&srcAlpha),
		unsafe.Pointer(&dstAlpha),
	}
	_, _ = ffi.CallFunction(&cifVoid4, c.glBlendFuncSeparate, nil, args[:])
}

func (c *Context) BlendEquation(mode uint32) {
	args := [1]unsafe.Pointer{unsafe.Pointer(&mode)}
	_, _ = ffi.CallFunction(&cifVoid1, c.glBlendEquation, nil, args[:])
}

// BlendEquationSeparate sets separate blend equations for RGB and alpha.
func (c *Context) BlendEquationSeparate(modeRGB, modeAlpha uint32) {
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&modeRGB),
		unsafe.Pointer(&modeAlpha),
	}
	_, _ = ffi.CallFunction(&cifVoid2UU, c.glBlendEquationSeparate, nil, args[:])
}

// BlendColor sets the constant blend color.
func (c *Context) BlendColor(r, g, b, a float32) {
	args := [4]unsafe.Pointer{
		unsafe.Pointer(&r),
		unsafe.Pointer(&g),
		unsafe.Pointer(&b),
		unsafe.Pointer(&a),
	}
	_, _ = ffi.CallFunction(&cifVoid4Float, c.glBlendColor, nil, args[:])
}

// --- UBO ---

// BindBufferBase binds a buffer to an indexed binding point.
func (c *Context) BindBufferBase(target, index, buffer uint32) {
	args := [3]unsafe.Pointer{
		unsafe.Pointer(&target),
		unsafe.Pointer(&index),
		unsafe.Pointer(&buffer),
	}
	_, _ = ffi.CallFunction(&cifVoid3, c.glBindBufferBase, nil, args[:])
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
	_, _ = ffi.CallFunction(&cifVoid5FBO, c.glBindBufferRange, nil, args[:])
}

// GetUniformBlockIndex returns the index of a named uniform block.
func (c *Context) GetUniformBlockIndex(program uint32, name string) uint32 {
	cname, free := cString(name)
	defer free()
	var result int32
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&program),
		unsafe.Pointer(&cname), // FFI reads cname → OpenGL gets char* name
	}
	_, _ = ffi.CallFunction(&cifInt322, c.glGetUniformBlockIndex, unsafe.Pointer(&result), args[:])
	return uint32(result)
}

// UniformBlockBinding assigns a uniform block to a binding point.
func (c *Context) UniformBlockBinding(program, blockIndex, blockBinding uint32) {
	args := [3]unsafe.Pointer{
		unsafe.Pointer(&program),
		unsafe.Pointer(&blockIndex),
		unsafe.Pointer(&blockBinding),
	}
	_, _ = ffi.CallFunction(&cifVoid3, c.glUniformBlockBinding, nil, args[:])
}

// --- Uniforms ---

// Uniform1i sets an integer uniform value.
func (c *Context) Uniform1i(location, value int32) {
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&location),
		unsafe.Pointer(&value),
	}
	_, _ = ffi.CallFunction(&cifVoid2UU, c.glUniform1i, nil, args[:])
}

// --- Depth/Stencil ---

func (c *Context) DepthFunc(fn uint32) {
	args := [1]unsafe.Pointer{unsafe.Pointer(&fn)}
	_, _ = ffi.CallFunction(&cifVoid1, c.glDepthFunc, nil, args[:])
}

func (c *Context) DepthMask(flag bool) {
	var f uint32
	if flag {
		f = 1
	}
	args := [1]unsafe.Pointer{unsafe.Pointer(&f)}
	_, _ = ffi.CallFunction(&cifVoid1, c.glDepthMask, nil, args[:])
}

// StencilFuncSeparate sets stencil test function per face.
func (c *Context) StencilFuncSeparate(face, fn uint32, ref int32, mask uint32) {
	args := [4]unsafe.Pointer{
		unsafe.Pointer(&face),
		unsafe.Pointer(&fn),
		unsafe.Pointer(&ref),
		unsafe.Pointer(&mask),
	}
	_, _ = ffi.CallFunction(&cifVoid4, c.glStencilFuncSeparate, nil, args[:])
}

// StencilOpSeparate sets stencil operations per face.
func (c *Context) StencilOpSeparate(face, sfail, dpfail, dppass uint32) {
	args := [4]unsafe.Pointer{
		unsafe.Pointer(&face),
		unsafe.Pointer(&sfail),
		unsafe.Pointer(&dpfail),
		unsafe.Pointer(&dppass),
	}
	_, _ = ffi.CallFunction(&cifVoid4, c.glStencilOpSeparate, nil, args[:])
}

// StencilMaskSeparate sets stencil write mask per face.
func (c *Context) StencilMaskSeparate(face, mask uint32) {
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&face),
		unsafe.Pointer(&mask),
	}
	_, _ = ffi.CallFunction(&cifVoid2UU, c.glStencilMaskSeparate, nil, args[:])
}

// ColorMask enables or disables writing of color components.
func (c *Context) ColorMask(r, g, b, a bool) {
	var rv, gv, bv, av uint32
	if r {
		rv = 1
	}
	if g {
		gv = 1
	}
	if b {
		bv = 1
	}
	if a {
		av = 1
	}
	args := [4]unsafe.Pointer{
		unsafe.Pointer(&rv),
		unsafe.Pointer(&gv),
		unsafe.Pointer(&bv),
		unsafe.Pointer(&av),
	}
	_, _ = ffi.CallFunction(&cifVoid4, c.glColorMask, nil, args[:])
}

// --- Face Culling ---

func (c *Context) CullFace(mode uint32) {
	args := [1]unsafe.Pointer{unsafe.Pointer(&mode)}
	_, _ = ffi.CallFunction(&cifVoid1, c.glCullFace, nil, args[:])
}

func (c *Context) FrontFace(mode uint32) {
	args := [1]unsafe.Pointer{unsafe.Pointer(&mode)}
	_, _ = ffi.CallFunction(&cifVoid1, c.glFrontFace, nil, args[:])
}

// --- Instancing ---

func (c *Context) DrawArraysInstanced(mode uint32, first, count, instanceCount int32) {
	args := [4]unsafe.Pointer{
		unsafe.Pointer(&mode),
		unsafe.Pointer(&first),
		unsafe.Pointer(&count),
		unsafe.Pointer(&instanceCount),
	}
	_, _ = ffi.CallFunction(&cifVoid4Draw, c.glDrawArraysInstanced, nil, args[:])
}

func (c *Context) DrawElementsInstanced(mode uint32, count int32, typ uint32, indices uintptr, instanceCount int32) {
	args := [5]unsafe.Pointer{
		unsafe.Pointer(&mode),
		unsafe.Pointer(&count),
		unsafe.Pointer(&typ),
		unsafe.Pointer(&indices), // FFI reads &indices → OpenGL gets indices value as void*
		unsafe.Pointer(&instanceCount),
	}
	_, _ = ffi.CallFunction(&cifVoid5DrawElem, c.glDrawElementsInstanced, nil, args[:])
}

// DrawElementsInstancedBaseVertex adds baseVertex to every index (GL 3.2 /
// ES 3.2). A no-op when the context lacks it.
func (c *Context) DrawElementsInstancedBaseVertex(mode uint32, count int32, typ uint32, indices uintptr, instanceCount, baseVertex int32) {
	if c.glDrawElementsInstancedBaseVertex == nil {
		return
	}
	args := [6]unsafe.Pointer{
		unsafe.Pointer(&mode),
		unsafe.Pointer(&count),
		unsafe.Pointer(&typ),
		unsafe.Pointer(&indices),
		unsafe.Pointer(&instanceCount),
		unsafe.Pointer(&baseVertex),
	}
	_, _ = ffi.CallFunction(&cifVoid6DrawElemBV, c.glDrawElementsInstancedBaseVertex, nil, args[:])
}

// DrawArraysIndirect draws with the arguments at offset in the buffer bound
// to GL_DRAW_INDIRECT_BUFFER (GL 4.0 / ES 3.1). A no-op when the context
// lacks it.
func (c *Context) DrawArraysIndirect(mode uint32, offset uintptr) {
	if c.glDrawArraysIndirect == nil {
		return
	}
	args := [2]unsafe.Pointer{unsafe.Pointer(&mode), unsafe.Pointer(&offset)}
	_, _ = ffi.CallFunction(&cifVoid2, c.glDrawArraysIndirect, nil, args[:])
}

// DrawElementsIndirect draws indexed with the arguments at offset in the
// buffer bound to GL_DRAW_INDIRECT_BUFFER (GL 4.0 / ES 3.1). A no-op when the
// context lacks it.
func (c *Context) DrawElementsIndirect(mode, typ uint32, offset uintptr) {
	if c.glDrawElementsIndirect == nil {
		return
	}
	args := [3]unsafe.Pointer{unsafe.Pointer(&mode), unsafe.Pointer(&typ), unsafe.Pointer(&offset)}
	_, _ = ffi.CallFunction(&cifVoid3Shader, c.glDrawElementsIndirect, nil, args[:])
}

// DrawArraysInstancedBaseInstance draws with a first instance (GL 4.2). A
// no-op when the context lacks it.
func (c *Context) DrawArraysInstancedBaseInstance(mode uint32, first, count, instanceCount int32, baseInstance uint32) {
	if c.glDrawArraysInstancedBaseInstance == nil {
		return
	}
	args := [5]unsafe.Pointer{
		unsafe.Pointer(&mode),
		unsafe.Pointer(&first),
		unsafe.Pointer(&count),
		unsafe.Pointer(&instanceCount),
		unsafe.Pointer(&baseInstance),
	}
	_, _ = ffi.CallFunction(&cifVoid5DrawArrBI, c.glDrawArraysInstancedBaseInstance, nil, args[:])
}

// DrawElementsInstancedBaseVertexBaseInstance draws indexed with a base
// vertex and a first instance (GL 4.2). A no-op when the context lacks it.
func (c *Context) DrawElementsInstancedBaseVertexBaseInstance(mode uint32, count int32, typ uint32, indices uintptr, instanceCount, baseVertex int32, baseInstance uint32) {
	if c.glDrawElementsInstancedBaseVertexBaseInstance == nil {
		return
	}
	args := [7]unsafe.Pointer{
		unsafe.Pointer(&mode),
		unsafe.Pointer(&count),
		unsafe.Pointer(&typ),
		unsafe.Pointer(&indices),
		unsafe.Pointer(&instanceCount),
		unsafe.Pointer(&baseVertex),
		unsafe.Pointer(&baseInstance),
	}
	_, _ = ffi.CallFunction(&cifVoid7DrawElemBVBI, c.glDrawElementsInstancedBaseVertexBaseInstance, nil, args[:])
}

// Uniform1ui sets a uint uniform of the current program (GL 3.0 / ES 3.0). A
// no-op when the context lacks it.
func (c *Context) Uniform1ui(location int32, value uint32) {
	if c.glUniform1ui == nil {
		return
	}
	args := [2]unsafe.Pointer{unsafe.Pointer(&location), unsafe.Pointer(&value)}
	_, _ = ffi.CallFunction(&cifVoid2UU, c.glUniform1ui, nil, args[:])
}

// MultiDrawArraysIndirect draws drawCount records of stride bytes from offset
// in the buffer bound to GL_DRAW_INDIRECT_BUFFER, with gl_DrawID counting
// them (GL 4.3 / GL_EXT_multi_draw_indirect). A no-op when the context lacks
// it.
func (c *Context) MultiDrawArraysIndirect(mode uint32, offset uintptr, drawCount, stride int32) {
	if c.glMultiDrawArraysIndirect == nil {
		return
	}
	args := [4]unsafe.Pointer{
		unsafe.Pointer(&mode),
		unsafe.Pointer(&offset),
		unsafe.Pointer(&drawCount),
		unsafe.Pointer(&stride),
	}
	_, _ = ffi.CallFunction(&cifVoid4MultiArr, c.glMultiDrawArraysIndirect, nil, args[:])
}

// MultiDrawElementsIndirect is the indexed MultiDrawArraysIndirect.
func (c *Context) MultiDrawElementsIndirect(mode, typ uint32, offset uintptr, drawCount, stride int32) {
	if c.glMultiDrawElementsIndirect == nil {
		return
	}
	args := [5]unsafe.Pointer{
		unsafe.Pointer(&mode),
		unsafe.Pointer(&typ),
		unsafe.Pointer(&offset),
		unsafe.Pointer(&drawCount),
		unsafe.Pointer(&stride),
	}
	_, _ = ffi.CallFunction(&cifVoid5MultiElem, c.glMultiDrawElementsIndirect, nil, args[:])
}

// HasMultiDrawIndirect reports whether the multi-draw indirect calls are
// loaded.
func (c *Context) HasMultiDrawIndirect() bool {
	return c.glMultiDrawArraysIndirect != nil && c.glMultiDrawElementsIndirect != nil
}

// HasDrawIndirect reports whether glDrawArraysIndirect and
// glDrawElementsIndirect are loaded.
func (c *Context) HasDrawIndirect() bool {
	return c.glDrawArraysIndirect != nil && c.glDrawElementsIndirect != nil
}

// HasBaseInstance reports whether the base-instance draws are loaded.
func (c *Context) HasBaseInstance() bool {
	return c.glDrawArraysInstancedBaseInstance != nil && c.glDrawElementsInstancedBaseVertexBaseInstance != nil
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
	_, _ = ffi.CallFunction(&cifVoid3, c.glDispatchCompute, nil, args[:])
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
	_, _ = ffi.CallFunction(&cifVoid1, c.glDispatchComputeIndirect, nil, args[:])
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
	_, _ = ffi.CallFunction(&cifVoid1, c.glMemoryBarrier, nil, args[:])
}

// BindImageTexture binds a level of a texture to an image unit for shader
// image load/store (storage textures). layered binds every layer of an
// array, cube or 3D texture; otherwise layer selects one. Requires OpenGL
// 4.2+ or OpenGL ES 3.1+. No-op if image load/store is not supported.
func (c *Context) BindImageTexture(unit, texture uint32, level int32, layered bool, layer int32, access, format uint32) {
	if c.glBindImageTexture == nil {
		return
	}
	var layeredArg uint8
	if layered {
		layeredArg = 1
	}
	args := [7]unsafe.Pointer{
		unsafe.Pointer(&unit),
		unsafe.Pointer(&texture),
		unsafe.Pointer(&level),
		unsafe.Pointer(&layeredArg),
		unsafe.Pointer(&layer),
		unsafe.Pointer(&access),
		unsafe.Pointer(&format),
	}
	_, _ = ffi.CallFunction(&cifVoid7BindImage, c.glBindImageTexture, nil, args[:])
}

// SupportsCompute returns true if compute shaders are supported.
func (c *Context) SupportsCompute() bool {
	return c.glDispatchCompute != nil
}

// --- Query Objects ---

// GenQueries generates query object names.
// Returns 0 if query objects are not supported.
func (c *Context) GenQueries(n int32) uint32 {
	if c.glGenQueries == nil {
		return 0
	}
	var query uint32
	pQuery := &query
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&n),
		unsafe.Pointer(&pQuery),
	}
	_, _ = ffi.CallFunction(&cifVoid2, c.glGenQueries, nil, args[:])
	return query
}

// DeleteQueries deletes query objects.
func (c *Context) DeleteQueries(n int32, queries *uint32) {
	if c.glDeleteQueries == nil {
		return
	}
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&n),
		unsafe.Pointer(&queries), // FFI reads queries (= *uint32) → OpenGL gets pointer to IDs
	}
	_, _ = ffi.CallFunction(&cifVoid2, c.glDeleteQueries, nil, args[:])
}

// QueryCounter records a timestamp into a query object.
// Requires GL_ARB_timer_query (GL 3.3+) or GL_EXT_disjoint_timer_query (ES 3.0+).
func (c *Context) QueryCounter(query, target uint32) {
	if c.glQueryCounter == nil {
		return
	}
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&query),
		unsafe.Pointer(&target),
	}
	_, _ = ffi.CallFunction(&cifVoid2UU, c.glQueryCounter, nil, args[:])
}

// GetQueryObjectui64v retrieves a 64-bit query result.
func (c *Context) GetQueryObjectui64v(query, pname uint32, result *uint64) {
	if c.glGetQueryObjectui64v == nil {
		return
	}
	args := [3]unsafe.Pointer{
		unsafe.Pointer(&query),
		unsafe.Pointer(&pname),
		unsafe.Pointer(&result),
	}
	_, _ = ffi.CallFunction(&cifVoid3Shader, c.glGetQueryObjectui64v, nil, args[:])
}

// SupportsTimestampQueries returns true if timestamp queries are supported.
func (c *Context) SupportsTimestampQueries() bool {
	return c.glQueryCounter != nil && c.glGenQueries != nil
}

// --- Sync Object Wrappers ---

// FenceSync creates a sync object and inserts it into the GL command stream.
// Returns the sync object handle, or 0 on failure.
func (c *Context) FenceSync(condition, flags uint32) uintptr {
	if c.glFenceSync == nil {
		return 0
	}
	var result uintptr
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&condition),
		unsafe.Pointer(&flags),
	}
	_, _ = ffi.CallFunction(&cifPtr2UU, c.glFenceSync, unsafe.Pointer(&result), args[:])
	return result
}

// DeleteSync deletes a sync object.
func (c *Context) DeleteSync(sync uintptr) {
	if c.glDeleteSync == nil || sync == 0 {
		return
	}
	args := [1]unsafe.Pointer{unsafe.Pointer(&sync)}
	_, _ = ffi.CallFunction(&cifVoid1, c.glDeleteSync, nil, args[:])
}

// ClientWaitSync blocks until the sync object is signaled or timeout expires.
// Returns ALREADY_SIGNALED, CONDITION_SATISFIED, TIMEOUT_EXPIRED, or WAIT_FAILED.
func (c *Context) ClientWaitSync(sync uintptr, flags uint32, timeout uint64) uint32 {
	if c.glClientWaitSync == nil || sync == 0 {
		return WAIT_FAILED
	}
	// glClientWaitSync(GLsync sync, GLbitfield flags, GLuint64 timeout)
	var result uint32
	args := [3]unsafe.Pointer{
		unsafe.Pointer(&sync),
		unsafe.Pointer(&flags),
		unsafe.Pointer(&timeout),
	}
	_, _ = ffi.CallFunction(&cifUInt32Wait, c.glClientWaitSync, unsafe.Pointer(&result), args[:])
	return result
}

// GetSyncStatus returns the signaled status of a sync object.
func (c *Context) GetSyncStatus(sync uintptr) uint32 {
	if c.glGetSynciv == nil || sync == 0 {
		return SIGNALED // assume signaled if not supported
	}
	// glGetSynciv(GLsync sync, GLenum pname, GLsizei count, GLsizei *length, GLint *values)
	pname := uint32(SYNC_STATUS)
	count := int32(1)
	var length int32
	var value int32
	pLength := &length
	pValue := &value
	args := [5]unsafe.Pointer{
		unsafe.Pointer(&sync),
		unsafe.Pointer(&pname),
		unsafe.Pointer(&count),
		unsafe.Pointer(&pLength), // FFI reads pLength (= &length) → OpenGL writes to length
		unsafe.Pointer(&pValue),  // FFI reads pValue (= &value) → OpenGL writes to value
	}
	_, _ = ffi.CallFunction(&cifVoid5Synciv, c.glGetSynciv, nil, args[:])
	return uint32(value)
}

// SupportsFenceSync returns true if GL fence sync objects are available.
func (c *Context) SupportsFenceSync() bool {
	return c.glFenceSync != nil
}

// --- Copy Operations ---

// CopyBufferSubData copies part of one buffer to another.
// Requires GL 3.1+ / ES 3.0+.
func (c *Context) CopyBufferSubData(readTarget, writeTarget uint32, readOffset, writeOffset, size int) {
	if c.glCopyBufferSubData == nil {
		return
	}
	ro := uintptr(readOffset)
	wo := uintptr(writeOffset)
	sz := uintptr(size)
	args := [5]unsafe.Pointer{
		unsafe.Pointer(&readTarget),
		unsafe.Pointer(&writeTarget),
		unsafe.Pointer(&ro),
		unsafe.Pointer(&wo),
		unsafe.Pointer(&sz),
	}
	_, _ = ffi.CallFunction(&cifVoid5CopyBuf, c.glCopyBufferSubData, nil, args[:])
}

// CopyTexSubImage2D copies pixels from the read framebuffer into a 2D texture.
// Reads from the currently bound GL_READ_FRAMEBUFFER.
func (c *Context) CopyTexSubImage2D(target uint32, level, xoffset, yoffset, x, y, width, height int32) {
	if c.glCopyTexSubImage2D == nil {
		return
	}
	args := [8]unsafe.Pointer{
		unsafe.Pointer(&target),
		unsafe.Pointer(&level),
		unsafe.Pointer(&xoffset),
		unsafe.Pointer(&yoffset),
		unsafe.Pointer(&x),
		unsafe.Pointer(&y),
		unsafe.Pointer(&width),
		unsafe.Pointer(&height),
	}
	_, _ = ffi.CallFunction(&cifVoid8CopyTex, c.glCopyTexSubImage2D, nil, args[:])
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
