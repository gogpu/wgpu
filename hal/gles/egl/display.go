// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build linux

package egl

import (
	"fmt"
	"os"
	"strings"
	"unsafe"

	"github.com/go-webgpu/goffi/ffi"
	"github.com/go-webgpu/goffi/types"
)

var (
	// x11Lib is the handle to the loaded libX11.so library.
	x11Lib unsafe.Pointer
	// waylandClientLib is the handle to the loaded libwayland-client.so library.
	waylandClientLib unsafe.Pointer

	// X11 function symbols
	symXOpenDisplay  unsafe.Pointer
	symXCloseDisplay unsafe.Pointer

	// Wayland function symbols
	symWlDisplayConnect    unsafe.Pointer
	symWlDisplayDisconnect unsafe.Pointer

	// CallInterfaces
	cifXOpenDisplay        types.CallInterface
	cifXCloseDisplay       types.CallInterface
	cifWlDisplayConnect    types.CallInterface
	cifWlDisplayDisconnect types.CallInterface
)

// DisplayOwner manages the lifetime of a native display connection.
// It ensures the display is properly closed when the owner is destroyed.
type DisplayOwner struct {
	kind    WindowKind
	display uintptr
	lib     unsafe.Pointer
}

// OpenX11Display opens an X11 display connection.
// Returns nil if X11 libraries are not available or display cannot be opened.
func OpenX11Display() *DisplayOwner {
	var err error

	// Try loading libX11.so.6 first, then libX11.so
	x11Lib, err = ffi.LoadLibrary("libX11.so.6")
	if err != nil {
		x11Lib, err = ffi.LoadLibrary("libX11.so")
		if err != nil {
			return nil
		}
	}

	// Load symbols
	symXOpenDisplay, err = ffi.GetSymbol(x11Lib, "XOpenDisplay")
	if err != nil {
		return nil
	}
	symXCloseDisplay, err = ffi.GetSymbol(x11Lib, "XCloseDisplay")
	if err != nil {
		return nil
	}

	// Prepare CallInterfaces
	// Display* XOpenDisplay(char* display_name)
	err = ffi.PrepareCallInterface(&cifXOpenDisplay, types.DefaultCall,
		types.PointerTypeDescriptor,
		[]*types.TypeDescriptor{types.PointerTypeDescriptor})
	if err != nil {
		return nil
	}

	// int XCloseDisplay(Display* display)
	err = ffi.PrepareCallInterface(&cifXCloseDisplay, types.DefaultCall,
		types.UInt32TypeDescriptor,
		[]*types.TypeDescriptor{types.PointerTypeDescriptor})
	if err != nil {
		return nil
	}

	// Open default display (NULL = use DISPLAY environment variable)
	var display uintptr
	var displayName uintptr // NULL pointer for default display
	args := [1]unsafe.Pointer{unsafe.Pointer(&displayName)}
	_ = ffi.CallFunction(&cifXOpenDisplay, symXOpenDisplay, unsafe.Pointer(&display), args[:])
	if display == 0 {
		return nil
	}

	return &DisplayOwner{
		kind:    WindowKindX11,
		display: display,
		lib:     x11Lib,
	}
}

// TestWaylandDisplay tests if a Wayland display connection is available.
// Returns nil if Wayland libraries are not available or display cannot be opened.
// This function connects and immediately disconnects to test availability.
func TestWaylandDisplay() *DisplayOwner {
	var err error

	// Try loading libwayland-client.so.0 first, then libwayland-client.so
	waylandClientLib, err = ffi.LoadLibrary("libwayland-client.so.0")
	if err != nil {
		waylandClientLib, err = ffi.LoadLibrary("libwayland-client.so")
		if err != nil {
			return nil
		}
	}

	// Load symbols
	symWlDisplayConnect, err = ffi.GetSymbol(waylandClientLib, "wl_display_connect")
	if err != nil {
		return nil
	}
	symWlDisplayDisconnect, err = ffi.GetSymbol(waylandClientLib, "wl_display_disconnect")
	if err != nil {
		return nil
	}

	// Prepare CallInterfaces
	// wl_display* wl_display_connect(const char* name)
	err = ffi.PrepareCallInterface(&cifWlDisplayConnect, types.DefaultCall,
		types.PointerTypeDescriptor,
		[]*types.TypeDescriptor{types.PointerTypeDescriptor})
	if err != nil {
		return nil
	}

	// void wl_display_disconnect(wl_display* display)
	err = ffi.PrepareCallInterface(&cifWlDisplayDisconnect, types.DefaultCall,
		types.VoidTypeDescriptor,
		[]*types.TypeDescriptor{types.PointerTypeDescriptor})
	if err != nil {
		return nil
	}

	// Test connection (NULL = use WAYLAND_DISPLAY environment variable)
	var display uintptr
	var displayName uintptr // NULL pointer for default display
	args := [1]unsafe.Pointer{unsafe.Pointer(&displayName)}
	_ = ffi.CallFunction(&cifWlDisplayConnect, symWlDisplayConnect, unsafe.Pointer(&display), args[:])
	if display == 0 {
		return nil
	}

	// Immediately disconnect - we just wanted to test availability
	argsDisconnect := [1]unsafe.Pointer{unsafe.Pointer(&display)}
	_ = ffi.CallFunction(&cifWlDisplayDisconnect, symWlDisplayDisconnect, nil, argsDisconnect[:])

	return &DisplayOwner{
		kind:    WindowKindWayland,
		display: 0, // We don't keep the connection open
		lib:     waylandClientLib,
	}
}

// QueryClientExtensions returns EGL client extensions available without a display.
// This MUST be called with EGL_NO_DISPLAY to get client extensions.
// EGL client extensions are extensions that can be queried before display initialization.
func QueryClientExtensions() string {
	return QueryString(NoDisplay, Extensions)
}

// HasSurfacelessSupport checks if Mesa surfaceless platform is available.
func HasSurfacelessSupport() bool {
	extensions := QueryClientExtensions()
	return strings.Contains(extensions, "EGL_MESA_platform_surfaceless")
}

// DetectWindowKind detects the available window system.
// Priority order:
//  1. If no DISPLAY/WAYLAND_DISPLAY set AND surfaceless available -> Surfaceless (for CI)
//  2. Wayland (if WAYLAND_DISPLAY is set and works)
//  3. X11 (if DISPLAY is set and works)
//  4. Surfaceless (final fallback)
func DetectWindowKind() WindowKind {
	displayEnv := os.Getenv("DISPLAY")
	waylandEnv := os.Getenv("WAYLAND_DISPLAY")

	// In headless environments (no display env vars), prefer surfaceless
	// This is critical for CI environments where Xvfb may be set but doesn't work properly
	if displayEnv == "" && waylandEnv == "" {
		if HasSurfacelessSupport() {
			return WindowKindSurfaceless
		}
	}

	// Check environment variables for real displays
	if waylandEnv != "" {
		if owner := TestWaylandDisplay(); owner != nil {
			owner.Close()
			return WindowKindWayland
		}
	}

	if displayEnv != "" {
		if owner := OpenX11Display(); owner != nil {
			owner.Close()
			return WindowKindX11
		}
	}

	// Fallback to surfaceless for headless systems
	return WindowKindSurfaceless
}

// GetEGLDisplay returns an EGL display for the detected platform.
// It automatically detects the window system and uses the appropriate EGL platform.
func GetEGLDisplay() (EGLDisplay, WindowKind, error) {
	windowKind := DetectWindowKind()

	switch windowKind {
	case WindowKindX11:
		owner := OpenX11Display()
		if owner == nil {
			return NoDisplay, WindowKindUnknown, fmt.Errorf("failed to open X11 display")
		}
		defer owner.Close()

		// Try EGL 1.5 platform extension first
		display := GetPlatformDisplay(PlatformX11KHR, owner.display, nil)
		if display != NoDisplay {
			return display, WindowKindX11, nil
		}

		// Fallback to EGL 1.4
		display = GetDisplay(EGLNativeDisplayType(owner.display))
		if display == NoDisplay {
			return NoDisplay, WindowKindUnknown, fmt.Errorf("eglGetDisplay failed for X11")
		}
		return display, WindowKindX11, nil

	case WindowKindWayland:
		// For Wayland, we use the default display (NULL)
		// The actual Wayland connection will be managed by the window system
		display := GetPlatformDisplay(PlatformWaylandKHR, 0, nil)
		if display != NoDisplay {
			return display, WindowKindWayland, nil
		}

		// Fallback to EGL 1.4
		display = GetDisplay(DefaultDisplay)
		if display == NoDisplay {
			return NoDisplay, WindowKindUnknown, fmt.Errorf("eglGetDisplay failed for Wayland")
		}
		return display, WindowKindWayland, nil

	case WindowKindSurfaceless:
		// Surfaceless rendering (headless)
		display := GetPlatformDisplay(PlatformSurfacelessMesa, 0, nil)
		if display != NoDisplay {
			return display, WindowKindSurfaceless, nil
		}

		// Fallback to default display
		display = GetDisplay(DefaultDisplay)
		if display == NoDisplay {
			return NoDisplay, WindowKindUnknown, fmt.Errorf("eglGetDisplay failed for surfaceless")
		}
		return display, WindowKindSurfaceless, nil

	default:
		return NoDisplay, WindowKindUnknown, fmt.Errorf("unknown window system")
	}
}

// Kind returns the window system type.
func (d *DisplayOwner) Kind() WindowKind {
	return d.kind
}

// Display returns the native display pointer.
func (d *DisplayOwner) Display() uintptr {
	return d.display
}

// Close closes the native display connection and unloads the library.
func (d *DisplayOwner) Close() {
	if d.display == 0 {
		return
	}

	switch d.kind {
	case WindowKindX11:
		if symXCloseDisplay != nil {
			var result int32
			// goffi API requires pointer TO pointer value (avalue is slice of pointers to argument values)
			displayPtr := d.display
			args := [1]unsafe.Pointer{unsafe.Pointer(&displayPtr)}
			_ = ffi.CallFunction(&cifXCloseDisplay, symXCloseDisplay, unsafe.Pointer(&result), args[:])
		}
	case WindowKindWayland:
		// Wayland display is managed by the window system
		// We don't close it here
	}

	d.display = 0
}
