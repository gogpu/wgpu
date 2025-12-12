// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build linux

package egl

import (
	"fmt"
	"unsafe"

	"github.com/go-webgpu/goffi/ffi"
	"github.com/go-webgpu/goffi/types"
)

var (
	// eglLib is the handle to the loaded libEGL.so library.
	eglLib unsafe.Pointer

	// EGL 1.0+ core function symbols
	symEglGetError             unsafe.Pointer
	symEglGetDisplay           unsafe.Pointer
	symEglInitialize           unsafe.Pointer
	symEglTerminate            unsafe.Pointer
	symEglQueryString          unsafe.Pointer
	symEglChooseConfig         unsafe.Pointer
	symEglGetConfigAttrib      unsafe.Pointer
	symEglCreateWindowSurface  unsafe.Pointer
	symEglCreatePbufferSurface unsafe.Pointer
	symEglDestroySurface       unsafe.Pointer
	symEglBindAPI              unsafe.Pointer
	symEglSwapInterval         unsafe.Pointer
	symEglCreateContext        unsafe.Pointer
	symEglDestroyContext       unsafe.Pointer
	symEglMakeCurrent          unsafe.Pointer
	symEglGetCurrentContext    unsafe.Pointer
	symEglGetCurrentDisplay    unsafe.Pointer
	symEglSwapBuffers          unsafe.Pointer
	symEglGetProcAddress       unsafe.Pointer
	symEglGetPlatformDisplay   unsafe.Pointer // EGL 1.5, may be nil

	// CallInterfaces for each function signature
	cifEglGetError             types.CallInterface
	cifEglGetDisplay           types.CallInterface
	cifEglInitialize           types.CallInterface
	cifEglTerminate            types.CallInterface
	cifEglQueryString          types.CallInterface
	cifEglChooseConfig         types.CallInterface
	cifEglGetConfigAttrib      types.CallInterface
	cifEglCreateWindowSurface  types.CallInterface
	cifEglCreatePbufferSurface types.CallInterface
	cifEglDestroySurface       types.CallInterface
	cifEglBindAPI              types.CallInterface
	cifEglSwapInterval         types.CallInterface
	cifEglCreateContext        types.CallInterface
	cifEglDestroyContext       types.CallInterface
	cifEglMakeCurrent          types.CallInterface
	cifEglGetCurrentContext    types.CallInterface
	cifEglGetCurrentDisplay    types.CallInterface
	cifEglSwapBuffers          types.CallInterface
	cifEglGetProcAddress       types.CallInterface
	cifEglGetPlatformDisplay   types.CallInterface
)

// Init loads the EGL library and initializes function pointers.
func Init() error {
	var err error

	// Try loading libEGL.so.1 first, then libEGL.so
	eglLib, err = ffi.LoadLibrary("libEGL.so.1")
	if err != nil {
		eglLib, err = ffi.LoadLibrary("libEGL.so")
		if err != nil {
			return fmt.Errorf("failed to load libEGL.so: %w", err)
		}
	}

	// Load symbols and prepare CallInterfaces
	if err := loadEGLSymbols(); err != nil {
		return err
	}

	return prepareEGLCallInterfaces()
}

// loadEGLSymbols loads all required EGL function symbols.
func loadEGLSymbols() error {
	var err error

	// Core functions
	if symEglGetError, err = ffi.GetSymbol(eglLib, "eglGetError"); err != nil {
		return fmt.Errorf("eglGetError not found: %w", err)
	}
	if symEglGetDisplay, err = ffi.GetSymbol(eglLib, "eglGetDisplay"); err != nil {
		return fmt.Errorf("eglGetDisplay not found: %w", err)
	}
	if symEglInitialize, err = ffi.GetSymbol(eglLib, "eglInitialize"); err != nil {
		return fmt.Errorf("eglInitialize not found: %w", err)
	}
	if symEglTerminate, err = ffi.GetSymbol(eglLib, "eglTerminate"); err != nil {
		return fmt.Errorf("eglTerminate not found: %w", err)
	}
	if symEglQueryString, err = ffi.GetSymbol(eglLib, "eglQueryString"); err != nil {
		return fmt.Errorf("eglQueryString not found: %w", err)
	}
	if symEglChooseConfig, err = ffi.GetSymbol(eglLib, "eglChooseConfig"); err != nil {
		return fmt.Errorf("eglChooseConfig not found: %w", err)
	}
	if symEglGetConfigAttrib, err = ffi.GetSymbol(eglLib, "eglGetConfigAttrib"); err != nil {
		return fmt.Errorf("eglGetConfigAttrib not found: %w", err)
	}
	if symEglCreateWindowSurface, err = ffi.GetSymbol(eglLib, "eglCreateWindowSurface"); err != nil {
		return fmt.Errorf("eglCreateWindowSurface not found: %w", err)
	}
	if symEglCreatePbufferSurface, err = ffi.GetSymbol(eglLib, "eglCreatePbufferSurface"); err != nil {
		return fmt.Errorf("eglCreatePbufferSurface not found: %w", err)
	}
	if symEglDestroySurface, err = ffi.GetSymbol(eglLib, "eglDestroySurface"); err != nil {
		return fmt.Errorf("eglDestroySurface not found: %w", err)
	}
	if symEglBindAPI, err = ffi.GetSymbol(eglLib, "eglBindAPI"); err != nil {
		return fmt.Errorf("eglBindAPI not found: %w", err)
	}
	if symEglSwapInterval, err = ffi.GetSymbol(eglLib, "eglSwapInterval"); err != nil {
		return fmt.Errorf("eglSwapInterval not found: %w", err)
	}
	if symEglCreateContext, err = ffi.GetSymbol(eglLib, "eglCreateContext"); err != nil {
		return fmt.Errorf("eglCreateContext not found: %w", err)
	}
	if symEglDestroyContext, err = ffi.GetSymbol(eglLib, "eglDestroyContext"); err != nil {
		return fmt.Errorf("eglDestroyContext not found: %w", err)
	}
	if symEglMakeCurrent, err = ffi.GetSymbol(eglLib, "eglMakeCurrent"); err != nil {
		return fmt.Errorf("eglMakeCurrent not found: %w", err)
	}
	if symEglGetCurrentContext, err = ffi.GetSymbol(eglLib, "eglGetCurrentContext"); err != nil {
		return fmt.Errorf("eglGetCurrentContext not found: %w", err)
	}
	if symEglGetCurrentDisplay, err = ffi.GetSymbol(eglLib, "eglGetCurrentDisplay"); err != nil {
		return fmt.Errorf("eglGetCurrentDisplay not found: %w", err)
	}
	if symEglSwapBuffers, err = ffi.GetSymbol(eglLib, "eglSwapBuffers"); err != nil {
		return fmt.Errorf("eglSwapBuffers not found: %w", err)
	}
	if symEglGetProcAddress, err = ffi.GetSymbol(eglLib, "eglGetProcAddress"); err != nil {
		return fmt.Errorf("eglGetProcAddress not found: %w", err)
	}

	// EGL 1.5 optional
	symEglGetPlatformDisplay, _ = ffi.GetSymbol(eglLib, "eglGetPlatformDisplay") // ignore error, optional

	return nil
}

// prepareEGLCallInterfaces prepares CallInterface for each function signature.
func prepareEGLCallInterfaces() error {
	var err error

	// EGLint eglGetError(void)
	err = ffi.PrepareCallInterface(&cifEglGetError, types.DefaultCall,
		types.UInt32TypeDescriptor, // EGLint
		[]*types.TypeDescriptor{})
	if err != nil {
		return fmt.Errorf("failed to prepare eglGetError: %w", err)
	}

	// EGLDisplay eglGetDisplay(EGLNativeDisplayType)
	err = ffi.PrepareCallInterface(&cifEglGetDisplay, types.DefaultCall,
		types.PointerTypeDescriptor,                          // EGLDisplay (pointer)
		[]*types.TypeDescriptor{types.PointerTypeDescriptor}) // EGLNativeDisplayType
	if err != nil {
		return fmt.Errorf("failed to prepare eglGetDisplay: %w", err)
	}

	// EGLBoolean eglInitialize(EGLDisplay, EGLint*, EGLint*)
	err = ffi.PrepareCallInterface(&cifEglInitialize, types.DefaultCall,
		types.UInt32TypeDescriptor, // EGLBoolean
		[]*types.TypeDescriptor{
			types.PointerTypeDescriptor, // EGLDisplay
			types.PointerTypeDescriptor, // major*
			types.PointerTypeDescriptor, // minor*
		})
	if err != nil {
		return fmt.Errorf("failed to prepare eglInitialize: %w", err)
	}

	// EGLBoolean eglTerminate(EGLDisplay)
	err = ffi.PrepareCallInterface(&cifEglTerminate, types.DefaultCall,
		types.UInt32TypeDescriptor,
		[]*types.TypeDescriptor{types.PointerTypeDescriptor})
	if err != nil {
		return fmt.Errorf("failed to prepare eglTerminate: %w", err)
	}

	// const char* eglQueryString(EGLDisplay, EGLint)
	err = ffi.PrepareCallInterface(&cifEglQueryString, types.DefaultCall,
		types.PointerTypeDescriptor,
		[]*types.TypeDescriptor{
			types.PointerTypeDescriptor, // EGLDisplay
			types.UInt32TypeDescriptor,  // EGLint
		})
	if err != nil {
		return fmt.Errorf("failed to prepare eglQueryString: %w", err)
	}

	// EGLBoolean eglChooseConfig(EGLDisplay, EGLint*, EGLConfig*, EGLint, EGLint*)
	err = ffi.PrepareCallInterface(&cifEglChooseConfig, types.DefaultCall,
		types.UInt32TypeDescriptor,
		[]*types.TypeDescriptor{
			types.PointerTypeDescriptor, // EGLDisplay
			types.PointerTypeDescriptor, // attribList*
			types.PointerTypeDescriptor, // configs*
			types.UInt32TypeDescriptor,  // configSize
			types.PointerTypeDescriptor, // numConfig*
		})
	if err != nil {
		return fmt.Errorf("failed to prepare eglChooseConfig: %w", err)
	}

	// EGLBoolean eglGetConfigAttrib(EGLDisplay, EGLConfig, EGLint, EGLint*)
	err = ffi.PrepareCallInterface(&cifEglGetConfigAttrib, types.DefaultCall,
		types.UInt32TypeDescriptor,
		[]*types.TypeDescriptor{
			types.PointerTypeDescriptor, // EGLDisplay
			types.PointerTypeDescriptor, // EGLConfig
			types.UInt32TypeDescriptor,  // attribute
			types.PointerTypeDescriptor, // value*
		})
	if err != nil {
		return fmt.Errorf("failed to prepare eglGetConfigAttrib: %w", err)
	}

	// EGLSurface eglCreateWindowSurface(EGLDisplay, EGLConfig, EGLNativeWindowType, EGLint*)
	err = ffi.PrepareCallInterface(&cifEglCreateWindowSurface, types.DefaultCall,
		types.PointerTypeDescriptor,
		[]*types.TypeDescriptor{
			types.PointerTypeDescriptor, // EGLDisplay
			types.PointerTypeDescriptor, // EGLConfig
			types.PointerTypeDescriptor, // EGLNativeWindowType
			types.PointerTypeDescriptor, // attribList*
		})
	if err != nil {
		return fmt.Errorf("failed to prepare eglCreateWindowSurface: %w", err)
	}

	// EGLSurface eglCreatePbufferSurface(EGLDisplay, EGLConfig, EGLint*)
	err = ffi.PrepareCallInterface(&cifEglCreatePbufferSurface, types.DefaultCall,
		types.PointerTypeDescriptor,
		[]*types.TypeDescriptor{
			types.PointerTypeDescriptor, // EGLDisplay
			types.PointerTypeDescriptor, // EGLConfig
			types.PointerTypeDescriptor, // attribList*
		})
	if err != nil {
		return fmt.Errorf("failed to prepare eglCreatePbufferSurface: %w", err)
	}

	// EGLBoolean eglDestroySurface(EGLDisplay, EGLSurface)
	err = ffi.PrepareCallInterface(&cifEglDestroySurface, types.DefaultCall,
		types.UInt32TypeDescriptor,
		[]*types.TypeDescriptor{
			types.PointerTypeDescriptor, // EGLDisplay
			types.PointerTypeDescriptor, // EGLSurface
		})
	if err != nil {
		return fmt.Errorf("failed to prepare eglDestroySurface: %w", err)
	}

	// EGLBoolean eglBindAPI(EGLenum)
	err = ffi.PrepareCallInterface(&cifEglBindAPI, types.DefaultCall,
		types.UInt32TypeDescriptor,
		[]*types.TypeDescriptor{types.UInt32TypeDescriptor})
	if err != nil {
		return fmt.Errorf("failed to prepare eglBindAPI: %w", err)
	}

	// EGLBoolean eglSwapInterval(EGLDisplay, EGLint)
	err = ffi.PrepareCallInterface(&cifEglSwapInterval, types.DefaultCall,
		types.UInt32TypeDescriptor,
		[]*types.TypeDescriptor{
			types.PointerTypeDescriptor, // EGLDisplay
			types.UInt32TypeDescriptor,  // interval
		})
	if err != nil {
		return fmt.Errorf("failed to prepare eglSwapInterval: %w", err)
	}

	// EGLContext eglCreateContext(EGLDisplay, EGLConfig, EGLContext, EGLint*)
	err = ffi.PrepareCallInterface(&cifEglCreateContext, types.DefaultCall,
		types.PointerTypeDescriptor,
		[]*types.TypeDescriptor{
			types.PointerTypeDescriptor, // EGLDisplay
			types.PointerTypeDescriptor, // EGLConfig
			types.PointerTypeDescriptor, // shareContext
			types.PointerTypeDescriptor, // attribList*
		})
	if err != nil {
		return fmt.Errorf("failed to prepare eglCreateContext: %w", err)
	}

	// EGLBoolean eglDestroyContext(EGLDisplay, EGLContext)
	err = ffi.PrepareCallInterface(&cifEglDestroyContext, types.DefaultCall,
		types.UInt32TypeDescriptor,
		[]*types.TypeDescriptor{
			types.PointerTypeDescriptor, // EGLDisplay
			types.PointerTypeDescriptor, // EGLContext
		})
	if err != nil {
		return fmt.Errorf("failed to prepare eglDestroyContext: %w", err)
	}

	// EGLBoolean eglMakeCurrent(EGLDisplay, EGLSurface, EGLSurface, EGLContext)
	err = ffi.PrepareCallInterface(&cifEglMakeCurrent, types.DefaultCall,
		types.UInt32TypeDescriptor,
		[]*types.TypeDescriptor{
			types.PointerTypeDescriptor, // EGLDisplay
			types.PointerTypeDescriptor, // draw
			types.PointerTypeDescriptor, // read
			types.PointerTypeDescriptor, // ctx
		})
	if err != nil {
		return fmt.Errorf("failed to prepare eglMakeCurrent: %w", err)
	}

	// EGLContext eglGetCurrentContext(void)
	err = ffi.PrepareCallInterface(&cifEglGetCurrentContext, types.DefaultCall,
		types.PointerTypeDescriptor,
		[]*types.TypeDescriptor{})
	if err != nil {
		return fmt.Errorf("failed to prepare eglGetCurrentContext: %w", err)
	}

	// EGLDisplay eglGetCurrentDisplay(void)
	err = ffi.PrepareCallInterface(&cifEglGetCurrentDisplay, types.DefaultCall,
		types.PointerTypeDescriptor,
		[]*types.TypeDescriptor{})
	if err != nil {
		return fmt.Errorf("failed to prepare eglGetCurrentDisplay: %w", err)
	}

	// EGLBoolean eglSwapBuffers(EGLDisplay, EGLSurface)
	err = ffi.PrepareCallInterface(&cifEglSwapBuffers, types.DefaultCall,
		types.UInt32TypeDescriptor,
		[]*types.TypeDescriptor{
			types.PointerTypeDescriptor, // EGLDisplay
			types.PointerTypeDescriptor, // EGLSurface
		})
	if err != nil {
		return fmt.Errorf("failed to prepare eglSwapBuffers: %w", err)
	}

	// void* eglGetProcAddress(const char*)
	err = ffi.PrepareCallInterface(&cifEglGetProcAddress, types.DefaultCall,
		types.PointerTypeDescriptor,
		[]*types.TypeDescriptor{types.PointerTypeDescriptor})
	if err != nil {
		return fmt.Errorf("failed to prepare eglGetProcAddress: %w", err)
	}

	// EGL 1.5: EGLDisplay eglGetPlatformDisplay(EGLenum, void*, EGLAttrib*)
	if symEglGetPlatformDisplay != nil {
		err = ffi.PrepareCallInterface(&cifEglGetPlatformDisplay, types.DefaultCall,
			types.PointerTypeDescriptor,
			[]*types.TypeDescriptor{
				types.UInt32TypeDescriptor,  // platform
				types.PointerTypeDescriptor, // nativeDisplay
				types.PointerTypeDescriptor, // attribList*
			})
		if err != nil {
			return fmt.Errorf("failed to prepare eglGetPlatformDisplay: %w", err)
		}
	}

	return nil
}

// GetError returns the last EGL error.
func GetError() EGLInt {
	var result EGLInt
	_ = ffi.CallFunction(&cifEglGetError, symEglGetError, unsafe.Pointer(&result), nil)
	return result
}

// GetDisplay returns an EGL display connection.
func GetDisplay(displayID EGLNativeDisplayType) EGLDisplay {
	var result EGLDisplay
	args := [1]unsafe.Pointer{
		unsafe.Pointer(&displayID),
	}
	_ = ffi.CallFunction(&cifEglGetDisplay, symEglGetDisplay, unsafe.Pointer(&result), args[:])
	return result
}

// GetPlatformDisplay returns an EGL display connection for a specific platform (EGL 1.5).
// Falls back to GetDisplay if EGL 1.5 is not available.
func GetPlatformDisplay(platform EGLEnum, nativeDisplay uintptr, attribList *EGLAttrib) EGLDisplay {
	if symEglGetPlatformDisplay != nil {
		var result EGLDisplay
		args := [3]unsafe.Pointer{
			unsafe.Pointer(&platform),
			unsafe.Pointer(&nativeDisplay),
			unsafe.Pointer(&attribList),
		}
		_ = ffi.CallFunction(&cifEglGetPlatformDisplay, symEglGetPlatformDisplay, unsafe.Pointer(&result), args[:])
		return result
	}
	// Fallback to eglGetDisplay
	return GetDisplay(EGLNativeDisplayType(nativeDisplay))
}

// Initialize initializes an EGL display connection.
func Initialize(dpy EGLDisplay, major *EGLInt, minor *EGLInt) EGLBoolean {
	var result EGLBoolean
	args := [3]unsafe.Pointer{
		unsafe.Pointer(&dpy),
		unsafe.Pointer(major),
		unsafe.Pointer(minor),
	}
	_ = ffi.CallFunction(&cifEglInitialize, symEglInitialize, unsafe.Pointer(&result), args[:])
	return result
}

// Terminate terminates an EGL display connection.
func Terminate(dpy EGLDisplay) EGLBoolean {
	var result EGLBoolean
	args := [1]unsafe.Pointer{
		unsafe.Pointer(&dpy),
	}
	_ = ffi.CallFunction(&cifEglTerminate, symEglTerminate, unsafe.Pointer(&result), args[:])
	return result
}

// QueryString returns a string describing properties of the EGL client or display.
func QueryString(dpy EGLDisplay, name EGLInt) string {
	var ptr uintptr
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&dpy),
		unsafe.Pointer(&name),
	}
	_ = ffi.CallFunction(&cifEglQueryString, symEglQueryString, unsafe.Pointer(&ptr), args[:])
	if ptr == 0 {
		return ""
	}
	return goString(ptr)
}

// ChooseConfig returns EGL frame buffer configurations that match specified attributes.
func ChooseConfig(dpy EGLDisplay, attribList *EGLInt, configs *EGLConfig, configSize EGLInt, numConfig *EGLInt) EGLBoolean {
	var result EGLBoolean
	args := [5]unsafe.Pointer{
		unsafe.Pointer(&dpy),
		unsafe.Pointer(attribList),
		unsafe.Pointer(configs),
		unsafe.Pointer(&configSize),
		unsafe.Pointer(numConfig),
	}
	_ = ffi.CallFunction(&cifEglChooseConfig, symEglChooseConfig, unsafe.Pointer(&result), args[:])
	return result
}

// GetConfigAttrib returns information about an EGL frame buffer configuration.
func GetConfigAttrib(dpy EGLDisplay, config EGLConfig, attribute EGLInt, value *EGLInt) EGLBoolean {
	var result EGLBoolean
	args := [4]unsafe.Pointer{
		unsafe.Pointer(&dpy),
		unsafe.Pointer(&config),
		unsafe.Pointer(&attribute),
		unsafe.Pointer(value),
	}
	_ = ffi.CallFunction(&cifEglGetConfigAttrib, symEglGetConfigAttrib, unsafe.Pointer(&result), args[:])
	return result
}

// CreateWindowSurface creates a new EGL window surface.
func CreateWindowSurface(dpy EGLDisplay, config EGLConfig, win EGLNativeWindowType, attribList *EGLInt) EGLSurface {
	var result EGLSurface
	args := [4]unsafe.Pointer{
		unsafe.Pointer(&dpy),
		unsafe.Pointer(&config),
		unsafe.Pointer(&win),
		unsafe.Pointer(attribList),
	}
	_ = ffi.CallFunction(&cifEglCreateWindowSurface, symEglCreateWindowSurface, unsafe.Pointer(&result), args[:])
	return result
}

// CreatePbufferSurface creates a new EGL pixel buffer surface.
func CreatePbufferSurface(dpy EGLDisplay, config EGLConfig, attribList *EGLInt) EGLSurface {
	var result EGLSurface
	args := [3]unsafe.Pointer{
		unsafe.Pointer(&dpy),
		unsafe.Pointer(&config),
		unsafe.Pointer(attribList),
	}
	_ = ffi.CallFunction(&cifEglCreatePbufferSurface, symEglCreatePbufferSurface, unsafe.Pointer(&result), args[:])
	return result
}

// DestroySurface destroys an EGL surface.
func DestroySurface(dpy EGLDisplay, surface EGLSurface) EGLBoolean {
	var result EGLBoolean
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&dpy),
		unsafe.Pointer(&surface),
	}
	_ = ffi.CallFunction(&cifEglDestroySurface, symEglDestroySurface, unsafe.Pointer(&result), args[:])
	return result
}

// BindAPI sets the current rendering API.
func BindAPI(api EGLEnum) EGLBoolean {
	var result EGLBoolean
	args := [1]unsafe.Pointer{
		unsafe.Pointer(&api),
	}
	_ = ffi.CallFunction(&cifEglBindAPI, symEglBindAPI, unsafe.Pointer(&result), args[:])
	return result
}

// SwapInterval specifies the minimum number of video frames between buffer swaps.
func SwapInterval(dpy EGLDisplay, interval EGLInt) EGLBoolean {
	var result EGLBoolean
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&dpy),
		unsafe.Pointer(&interval),
	}
	_ = ffi.CallFunction(&cifEglSwapInterval, symEglSwapInterval, unsafe.Pointer(&result), args[:])
	return result
}

// CreateContext creates a new EGL rendering context.
func CreateContext(dpy EGLDisplay, config EGLConfig, shareContext EGLContext, attribList *EGLInt) EGLContext {
	var result EGLContext
	args := [4]unsafe.Pointer{
		unsafe.Pointer(&dpy),
		unsafe.Pointer(&config),
		unsafe.Pointer(&shareContext),
		unsafe.Pointer(attribList),
	}
	_ = ffi.CallFunction(&cifEglCreateContext, symEglCreateContext, unsafe.Pointer(&result), args[:])
	return result
}

// DestroyContext destroys an EGL rendering context.
func DestroyContext(dpy EGLDisplay, ctx EGLContext) EGLBoolean {
	var result EGLBoolean
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&dpy),
		unsafe.Pointer(&ctx),
	}
	_ = ffi.CallFunction(&cifEglDestroyContext, symEglDestroyContext, unsafe.Pointer(&result), args[:])
	return result
}

// MakeCurrent binds context to the current rendering thread and surfaces.
func MakeCurrent(dpy EGLDisplay, draw EGLSurface, read EGLSurface, ctx EGLContext) EGLBoolean {
	var result EGLBoolean
	args := [4]unsafe.Pointer{
		unsafe.Pointer(&dpy),
		unsafe.Pointer(&draw),
		unsafe.Pointer(&read),
		unsafe.Pointer(&ctx),
	}
	_ = ffi.CallFunction(&cifEglMakeCurrent, symEglMakeCurrent, unsafe.Pointer(&result), args[:])
	return result
}

// GetCurrentContext returns the current EGL rendering context.
func GetCurrentContext() EGLContext {
	var result EGLContext
	_ = ffi.CallFunction(&cifEglGetCurrentContext, symEglGetCurrentContext, unsafe.Pointer(&result), nil)
	return result
}

// GetCurrentDisplay returns the current EGL display connection.
func GetCurrentDisplay() EGLDisplay {
	var result EGLDisplay
	_ = ffi.CallFunction(&cifEglGetCurrentDisplay, symEglGetCurrentDisplay, unsafe.Pointer(&result), nil)
	return result
}

// SwapBuffers posts EGL surface color buffer to a native window.
func SwapBuffers(dpy EGLDisplay, surface EGLSurface) EGLBoolean {
	var result EGLBoolean
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&dpy),
		unsafe.Pointer(&surface),
	}
	_ = ffi.CallFunction(&cifEglSwapBuffers, symEglSwapBuffers, unsafe.Pointer(&result), args[:])
	return result
}

// GetProcAddress returns the address of an EGL or client API extension function.
func GetProcAddress(procname string) uintptr {
	cname := append([]byte(procname), 0)
	var result uintptr
	args := [1]unsafe.Pointer{
		unsafe.Pointer(&cname[0]),
	}
	_ = ffi.CallFunction(&cifEglGetProcAddress, symEglGetProcAddress, unsafe.Pointer(&result), args[:])
	return result
}

// goString converts a null-terminated C string pointer to Go string.
func goString(cstr uintptr) string {
	if cstr == 0 {
		return ""
	}
	// Find string length (max 4096 to prevent infinite loops)
	length := 0
	//nolint:govet // Converting uintptr (C string address) to unsafe.Pointer is required for FFI
	ptr := (*byte)(unsafe.Pointer(cstr))
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
