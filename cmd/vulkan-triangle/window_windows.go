// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

package main

import (
	"fmt"
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procRegisterClassExW   = user32.NewProc("RegisterClassExW")
	procCreateWindowExW    = user32.NewProc("CreateWindowExW")
	procDefWindowProcW     = user32.NewProc("DefWindowProcW")
	procDestroyWindow      = user32.NewProc("DestroyWindow")
	procShowWindow         = user32.NewProc("ShowWindow")
	procUpdateWindow       = user32.NewProc("UpdateWindow")
	procGetMessageW        = user32.NewProc("GetMessageW")
	procPeekMessageW       = user32.NewProc("PeekMessageW")
	procTranslateMessage   = user32.NewProc("TranslateMessage")
	procDispatchMessageW   = user32.NewProc("DispatchMessageW")
	procGetModuleHandleW   = kernel32.NewProc("GetModuleHandleW")
	procPostQuitMessage    = user32.NewProc("PostQuitMessage")
	procGetClientRect      = user32.NewProc("GetClientRect")
	procAdjustWindowRectEx = user32.NewProc("AdjustWindowRectEx")
	procSetWindowLongPtrW  = user32.NewProc("SetWindowLongPtrW")
	procLoadCursorW        = user32.NewProc("LoadCursorW")
	procSetCursor          = user32.NewProc("SetCursor")
)

const (
	csOwnDC = 0x0020

	// Window styles
	wsOverlappedWindow = 0x00CF0000 // Standard overlapped window with all buttons
	wsVisible          = 0x10000000

	swShow = 5

	// Window messages
	wmDestroy       = 0x0002
	wmSize          = 0x0005
	wmClose         = 0x0010
	wmQuit          = 0x0012
	wmSetCursor     = 0x0020
	wmEnterSizeMove = 0x0231
	wmExitSizeMove  = 0x0232

	pmRemove = 0x0001

	// Cursor constants
	idcArrow = 32512

	// WM_SETCURSOR hit test codes
	htClient = 1
)

type wndClassExW struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSm     uintptr
}

type msg struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
}

type point struct {
	X int32
	Y int32
}

type rect struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

// Window represents a platform window with professional event handling.
// Uses hybrid GetMessage/PeekMessage pattern from Gio for responsiveness.
type Window struct {
	hwnd      uintptr
	hInstance uintptr
	cursor    uintptr // Default arrow cursor
	width     int32
	height    int32
	running   bool

	// Resize handling (professional pattern from Gio/wgpu)
	inSizeMove  atomic.Bool // True during modal resize/move loop
	needsResize atomic.Bool // Resize pending after size move ends
	pendingW    atomic.Int32
	pendingH    atomic.Int32

	// Animation state for hybrid event loop
	animating atomic.Bool
}

// Global window pointer for wndProc callback
var globalWindow *Window

// NewWindow creates a new window with the given title and size.
func NewWindow(title string, width, height int32) (*Window, error) {
	hInstance, _, _ := procGetModuleHandleW.Call(0)

	className, err := windows.UTF16PtrFromString("VulkanTriangleWindow")
	if err != nil {
		return nil, fmt.Errorf("failed to create class name: %w", err)
	}
	windowTitle, err := windows.UTF16PtrFromString(title)
	if err != nil {
		return nil, fmt.Errorf("failed to create window title: %w", err)
	}

	// Load default arrow cursor
	cursor, _, _ := procLoadCursorW.Call(0, uintptr(idcArrow))

	wc := wndClassExW{
		Size:      uint32(unsafe.Sizeof(wndClassExW{})),
		Style:     csOwnDC,
		WndProc:   windows.NewCallback(wndProc),
		Instance:  hInstance,
		Cursor:    cursor,
		ClassName: className,
	}

	ret, _, callErr := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))) //nolint:gosec // G103: Win32 API
	if ret == 0 {
		return nil, fmt.Errorf("RegisterClassExW failed: %w", callErr)
	}

	style := uint32(wsOverlappedWindow)

	// Adjust window size to account for borders
	var rc rect
	rc.Right = width
	rc.Bottom = height
	procAdjustWindowRectEx.Call( //nolint:errcheck,gosec // G103: Win32 API
		uintptr(unsafe.Pointer(&rc)), //nolint:gosec // G103: Win32 API
		uintptr(style),
		0, // no menu
		0, // no extended style
	)

	hwnd, _, callErr := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),   //nolint:gosec // G103: Win32 API
		uintptr(unsafe.Pointer(windowTitle)), //nolint:gosec // G103: Win32 API
		uintptr(style),
		100, 100, // x, y
		uintptr(rc.Right-rc.Left),
		uintptr(rc.Bottom-rc.Top),
		0, 0, hInstance, 0,
	)
	if hwnd == 0 {
		return nil, fmt.Errorf("CreateWindowExW failed: %w", callErr)
	}

	w := &Window{
		hwnd:      hwnd,
		hInstance: hInstance,
		cursor:    cursor,
		width:     width,
		height:    height,
		running:   true,
	}
	w.animating.Store(true) // Start in animating mode for games

	// Store window pointer for wndProc
	globalWindow = w
	// Store window pointer for wndProc (gwlpUserData = -21)
	procSetWindowLongPtrW.Call(hwnd, ^uintptr(20), uintptr(unsafe.Pointer(w))) //nolint:errcheck,gosec

	// Show window
	procShowWindow.Call(hwnd, uintptr(swShow)) //nolint:errcheck,gosec // Win32 API
	procUpdateWindow.Call(hwnd)                //nolint:errcheck,gosec // Win32 API

	return w, nil
}

// Destroy destroys the window.
func (w *Window) Destroy() {
	if w.hwnd != 0 {
		_, _, _ = procDestroyWindow.Call(w.hwnd)
		w.hwnd = 0
	}
	if globalWindow == w {
		globalWindow = nil
	}
}

// Handle returns the native window handle (HWND).
func (w *Window) Handle() uintptr {
	return w.hwnd
}

// Size returns the client area size of the window.
func (w *Window) Size() (width, height int32) {
	var rc rect
	procGetClientRect.Call(w.hwnd, uintptr(unsafe.Pointer(&rc))) //nolint:errcheck,gosec // Win32 API
	return rc.Right - rc.Left, rc.Bottom - rc.Top
}

// SetAnimating sets whether the window should use continuous rendering mode.
// When true, uses PeekMessage (non-blocking) for maximum FPS.
// When false, uses GetMessage (blocking) for lower CPU usage.
func (w *Window) SetAnimating(animating bool) {
	w.animating.Store(animating)
}

// NeedsResize returns true if a resize event occurred and clears the flag.
func (w *Window) NeedsResize() bool {
	return w.needsResize.Swap(false)
}

// InSizeMove returns true if the window is currently being resized/moved.
// During this time, rendering should continue but swapchain recreation should be deferred.
func (w *Window) InSizeMove() bool {
	return w.inSizeMove.Load()
}

// PollEvents processes pending window events using hybrid GetMessage/PeekMessage.
// This is the professional pattern from Gio that prevents "Not Responding".
// Returns false when the window should close.
//
//nolint:nestif // Hybrid event loop requires different paths for animating/idle modes
func (w *Window) PollEvents() bool {
	var m msg

	// Hybrid event loop pattern from Gio:
	// - When animating: use PeekMessage (non-blocking) for max FPS
	// - When idle: would use GetMessage (blocking) for CPU efficiency
	// For games/realtime apps, we always use PeekMessage

	if w.animating.Load() {
		// Non-blocking: process all pending messages, then return for rendering
		for {
			ret, _, _ := procPeekMessageW.Call(
				uintptr(unsafe.Pointer(&m)), //nolint:gosec // G103: Win32 API
				0,
				0,
				0,
				uintptr(pmRemove),
			)
			if ret == 0 {
				break // No more messages
			}

			if m.Message == wmQuit {
				w.running = false
				return false
			}

			_, _, _ = procTranslateMessage.Call(uintptr(unsafe.Pointer(&m))) //nolint:gosec // G103: Win32 API
			_, _, _ = procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m))) //nolint:gosec // G103: Win32 API
		}
	} else {
		// Blocking: wait for message (for GUI apps that don't need continuous render)
		ret, _, _ := procGetMessageW.Call(
			uintptr(unsafe.Pointer(&m)), //nolint:gosec // G103: Win32 API
			0,
			0,
			0,
		)
		if ret == 0 || m.Message == wmQuit {
			w.running = false
			return false
		}

		_, _, _ = procTranslateMessage.Call(uintptr(unsafe.Pointer(&m))) //nolint:gosec // G103: Win32 API
		_, _, _ = procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m))) //nolint:gosec // G103: Win32 API
	}

	return w.running
}

// wndProc is the window procedure callback.
// Handles resize events professionally like Gio/wgpu.
func wndProc(hwnd, message, wParam, lParam uintptr) uintptr {
	// Get window pointer
	w := globalWindow
	if w == nil || w.hwnd != hwnd {
		ret, _, _ := procDefWindowProcW.Call(hwnd, message, wParam, lParam)
		return ret
	}

	switch message {
	case wmDestroy, wmClose:
		_, _, _ = procPostQuitMessage.Call(0)
		return 0

	case wmEnterSizeMove:
		// User started resizing/moving - enter modal loop
		// Windows blocks the message pump during resize, so we track this
		w.inSizeMove.Store(true)
		return 0

	case wmExitSizeMove:
		// User finished resizing/moving - safe to recreate swapchain now
		w.inSizeMove.Store(false)
		// Signal that resize handling is needed
		if w.pendingW.Load() > 0 && w.pendingH.Load() > 0 {
			w.needsResize.Store(true)
		}
		return 0

	case wmSize:
		// Window size changed
		width := int32(lParam & 0xFFFF)
		height := int32((lParam >> 16) & 0xFFFF)

		if width > 0 && height > 0 {
			w.width = width
			w.height = height
			w.pendingW.Store(width)
			w.pendingH.Store(height)

			// If not in modal resize loop, signal resize immediately
			if !w.inSizeMove.Load() {
				w.needsResize.Store(true)
			}
		}
		return 0

	case wmSetCursor:
		// Restore cursor to arrow when in client area
		// This fixes the resize cursor staying after resize ends
		hitTest := lParam & 0xFFFF
		if hitTest == htClient {
			_, _, _ = procSetCursor.Call(w.cursor)
			return 1 // Cursor was set
		}
		// Let Windows handle non-client area cursors (resize handles, etc.)
		ret, _, _ := procDefWindowProcW.Call(hwnd, message, wParam, lParam)
		return ret

	default:
		ret, _, _ := procDefWindowProcW.Call(hwnd, message, wParam, lParam)
		return ret
	}
}
