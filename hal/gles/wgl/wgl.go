// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

// Package wgl provides Windows OpenGL (WGL) context management.
package wgl

import (
	"fmt"
	"syscall"
	"unsafe"
)

var (
	opengl32 *syscall.DLL
	gdi32    *syscall.DLL
	user32   *syscall.DLL

	// OpenGL32.dll functions
	procWglCreateContext     *syscall.Proc
	procWglDeleteContext     *syscall.Proc
	procWglMakeCurrent       *syscall.Proc
	procWglGetProcAddress    *syscall.Proc
	procWglGetCurrentContext *syscall.Proc
	procWglGetCurrentDC      *syscall.Proc
	procWglShareLists        *syscall.Proc

	// GDI32.dll functions
	procChoosePixelFormat *syscall.Proc
	procSetPixelFormat    *syscall.Proc
	procSwapBuffers       *syscall.Proc

	// User32.dll functions
	procGetDC     *syscall.Proc
	procReleaseDC *syscall.Proc
)

// Windows types
type (
	HANDLE uintptr
	HDC    HANDLE
	HGLRC  HANDLE
	HWND   HANDLE
	BOOL   int32
)

// PIXELFORMATDESCRIPTOR flags (unexported to follow Go naming conventions)
const (
	pfdDrawToWindow  = 0x00000004
	pfdSupportOpenGL = 0x00000020
	pfdDoubleBuffer  = 0x00000001
	pfdTypeRGBA      = 0
	pfdMainPlane     = 0
)

// PIXELFORMATDESCRIPTOR is the Windows pixel format descriptor.
type PIXELFORMATDESCRIPTOR struct {
	Size           uint16
	Version        uint16
	Flags          uint32
	PixelType      byte
	ColorBits      byte
	RedBits        byte
	RedShift       byte
	GreenBits      byte
	GreenShift     byte
	BlueBits       byte
	BlueShift      byte
	AlphaBits      byte
	AlphaShift     byte
	AccumBits      byte
	AccumRedBits   byte
	AccumGreenBits byte
	AccumBlueBits  byte
	AccumAlphaBits byte
	DepthBits      byte
	StencilBits    byte
	AuxBuffers     byte
	LayerType      byte
	Reserved       byte
	LayerMask      uint32
	VisibleMask    uint32
	DamageMask     uint32
}

// Init loads the required DLLs and procedures.
func Init() error {
	var err error

	opengl32, err = syscall.LoadDLL("opengl32.dll")
	if err != nil {
		return fmt.Errorf("failed to load opengl32.dll: %w", err)
	}

	gdi32, err = syscall.LoadDLL("gdi32.dll")
	if err != nil {
		return fmt.Errorf("failed to load gdi32.dll: %w", err)
	}

	user32, err = syscall.LoadDLL("user32.dll")
	if err != nil {
		return fmt.Errorf("failed to load user32.dll: %w", err)
	}

	// Load OpenGL32 functions
	procWglCreateContext, err = opengl32.FindProc("wglCreateContext")
	if err != nil {
		return fmt.Errorf("wglCreateContext: %w", err)
	}

	procWglDeleteContext, err = opengl32.FindProc("wglDeleteContext")
	if err != nil {
		return fmt.Errorf("wglDeleteContext: %w", err)
	}

	procWglMakeCurrent, err = opengl32.FindProc("wglMakeCurrent")
	if err != nil {
		return fmt.Errorf("wglMakeCurrent: %w", err)
	}

	procWglGetProcAddress, err = opengl32.FindProc("wglGetProcAddress")
	if err != nil {
		return fmt.Errorf("wglGetProcAddress: %w", err)
	}

	procWglGetCurrentContext, err = opengl32.FindProc("wglGetCurrentContext")
	if err != nil {
		return fmt.Errorf("wglGetCurrentContext: %w", err)
	}

	procWglGetCurrentDC, err = opengl32.FindProc("wglGetCurrentDC")
	if err != nil {
		return fmt.Errorf("wglGetCurrentDC: %w", err)
	}

	procWglShareLists, err = opengl32.FindProc("wglShareLists")
	if err != nil {
		return fmt.Errorf("wglShareLists: %w", err)
	}

	// Load GDI32 functions
	procChoosePixelFormat, err = gdi32.FindProc("ChoosePixelFormat")
	if err != nil {
		return fmt.Errorf("ChoosePixelFormat: %w", err)
	}

	procSetPixelFormat, err = gdi32.FindProc("SetPixelFormat")
	if err != nil {
		return fmt.Errorf("SetPixelFormat: %w", err)
	}

	procSwapBuffers, err = gdi32.FindProc("SwapBuffers")
	if err != nil {
		return fmt.Errorf("SwapBuffers: %w", err)
	}

	// Load User32 functions
	procGetDC, err = user32.FindProc("GetDC")
	if err != nil {
		return fmt.Errorf("GetDC: %w", err)
	}

	procReleaseDC, err = user32.FindProc("ReleaseDC")
	if err != nil {
		return fmt.Errorf("ReleaseDC: %w", err)
	}

	return nil
}

// CreateContext creates a new OpenGL rendering context.
func CreateContext(hdc HDC) (HGLRC, error) {
	r, _, err := procWglCreateContext.Call(uintptr(hdc))
	if r == 0 {
		return 0, fmt.Errorf("wglCreateContext failed: %w", err)
	}
	return HGLRC(r), nil
}

// DeleteContext deletes an OpenGL rendering context.
func DeleteContext(hglrc HGLRC) error {
	r, _, err := procWglDeleteContext.Call(uintptr(hglrc))
	if r == 0 {
		return fmt.Errorf("wglDeleteContext failed: %w", err)
	}
	return nil
}

// MakeCurrent makes the specified OpenGL context current.
func MakeCurrent(hdc HDC, hglrc HGLRC) error {
	r, _, err := procWglMakeCurrent.Call(uintptr(hdc), uintptr(hglrc))
	if r == 0 {
		return fmt.Errorf("wglMakeCurrent failed: %w", err)
	}
	return nil
}

// GetProcAddress returns the address of an OpenGL extension function.
func GetProcAddress(name string) uintptr {
	cname, _ := syscall.BytePtrFromString(name)
	r, _, _ := procWglGetProcAddress.Call(uintptr(unsafe.Pointer(cname)))
	return r
}

// GetCurrentContext returns the current OpenGL rendering context.
func GetCurrentContext() HGLRC {
	r, _, _ := procWglGetCurrentContext.Call()
	return HGLRC(r)
}

// GetCurrentDC returns the device context of the current OpenGL context.
func GetCurrentDC() HDC {
	r, _, _ := procWglGetCurrentDC.Call()
	return HDC(r)
}

// ShareLists enables sharing of display lists between contexts.
func ShareLists(hglrc1, hglrc2 HGLRC) error {
	r, _, err := procWglShareLists.Call(uintptr(hglrc1), uintptr(hglrc2))
	if r == 0 {
		return fmt.Errorf("wglShareLists failed: %w", err)
	}
	return nil
}

// ChoosePixelFormat chooses a pixel format for the device context.
func ChoosePixelFormat(hdc HDC, pfd *PIXELFORMATDESCRIPTOR) (int, error) {
	r, _, err := procChoosePixelFormat.Call(uintptr(hdc), uintptr(unsafe.Pointer(pfd)))
	if r == 0 {
		return 0, fmt.Errorf("ChoosePixelFormat failed: %w", err)
	}
	return int(r), nil
}

// SetPixelFormat sets the pixel format of the device context.
func SetPixelFormat(hdc HDC, format int, pfd *PIXELFORMATDESCRIPTOR) error {
	r, _, err := procSetPixelFormat.Call(uintptr(hdc), uintptr(format), uintptr(unsafe.Pointer(pfd)))
	if r == 0 {
		return fmt.Errorf("SetPixelFormat failed: %w", err)
	}
	return nil
}

// SwapBuffers swaps the front and back buffers.
func SwapBuffers(hdc HDC) error {
	r, _, err := procSwapBuffers.Call(uintptr(hdc))
	if r == 0 {
		return fmt.Errorf("SwapBuffers failed: %w", err)
	}
	return nil
}

// GetDC retrieves a device context for the specified window.
func GetDC(hwnd HWND) HDC {
	r, _, _ := procGetDC.Call(uintptr(hwnd))
	return HDC(r)
}

// ReleaseDC releases a device context.
func ReleaseDC(hwnd HWND, hdc HDC) int {
	r, _, _ := procReleaseDC.Call(uintptr(hwnd), uintptr(hdc))
	return int(r)
}

// DefaultPixelFormat returns a sensible default pixel format descriptor.
func DefaultPixelFormat() PIXELFORMATDESCRIPTOR {
	return PIXELFORMATDESCRIPTOR{
		Size:        uint16(unsafe.Sizeof(PIXELFORMATDESCRIPTOR{})),
		Version:     1,
		Flags:       pfdDrawToWindow | pfdSupportOpenGL | pfdDoubleBuffer,
		PixelType:   pfdTypeRGBA,
		ColorBits:   32,
		DepthBits:   24,
		StencilBits: 8,
		LayerType:   pfdMainPlane,
	}
}

// GetGLProcAddress returns the address of an OpenGL function.
// For GL 1.1 functions, it loads from opengl32.dll directly.
// For GL 2.0+ functions, it uses wglGetProcAddress.
func GetGLProcAddress(name string) uintptr {
	// First try wglGetProcAddress (for GL 2.0+)
	addr := GetProcAddress(name)
	if addr != 0 {
		return addr
	}

	// Fall back to opengl32.dll (for GL 1.1)
	proc, err := opengl32.FindProc(name)
	if err != nil {
		return 0
	}
	return proc.Addr()
}

// Context wraps a WGL rendering context with its device context.
type Context struct {
	hdc   HDC
	hglrc HGLRC
}

// NewContext creates a new WGL context for the given window handle.
func NewContext(hwnd HWND) (*Context, error) {
	hdc := GetDC(hwnd)
	if hdc == 0 {
		return nil, fmt.Errorf("GetDC failed")
	}

	pfd := DefaultPixelFormat()
	format, err := ChoosePixelFormat(hdc, &pfd)
	if err != nil {
		ReleaseDC(hwnd, hdc)
		return nil, err
	}

	if err := SetPixelFormat(hdc, format, &pfd); err != nil {
		ReleaseDC(hwnd, hdc)
		return nil, err
	}

	hglrc, err := CreateContext(hdc)
	if err != nil {
		ReleaseDC(hwnd, hdc)
		return nil, err
	}

	return &Context{
		hdc:   hdc,
		hglrc: hglrc,
	}, nil
}

// MakeCurrent makes this context current.
func (c *Context) MakeCurrent() error {
	return MakeCurrent(c.hdc, c.hglrc)
}

// SwapBuffers swaps the front and back buffers.
func (c *Context) SwapBuffers() error {
	return SwapBuffers(c.hdc)
}

// Destroy releases the context and device context.
func (c *Context) Destroy(hwnd HWND) {
	if c.hglrc != 0 {
		_ = MakeCurrent(0, 0) // Unbind
		_ = DeleteContext(c.hglrc)
		c.hglrc = 0
	}
	if c.hdc != 0 {
		ReleaseDC(hwnd, c.hdc)
		c.hdc = 0
	}
}

// HDC returns the device context.
func (c *Context) HDC() HDC {
	return c.hdc
}

// HGLRC returns the rendering context.
func (c *Context) HGLRC() HGLRC {
	return c.hglrc
}
