// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

package main

import (
	"fmt"
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
	procPeekMessageW       = user32.NewProc("PeekMessageW")
	procTranslateMessage   = user32.NewProc("TranslateMessage")
	procDispatchMessageW   = user32.NewProc("DispatchMessageW")
	procGetModuleHandleW   = kernel32.NewProc("GetModuleHandleW")
	procPostQuitMessage    = user32.NewProc("PostQuitMessage")
	procGetClientRect      = user32.NewProc("GetClientRect")
	procAdjustWindowRectEx = user32.NewProc("AdjustWindowRectEx")
)

const (
	csOwnDC       = 0x0020
	wsOverlapped  = 0x00000000
	wsCaption     = 0x00C00000
	wsSysMenu     = 0x00080000
	wsMinimizeBox = 0x00020000
	wsMaximizeBox = 0x00010000
	wsThickFrame  = 0x00040000
	wsVisible     = 0x10000000

	swShow = 5

	wmDestroy = 0x0002
	wmClose   = 0x0010
	wmQuit    = 0x0012

	pmRemove = 0x0001
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

// Window represents a platform window.
type Window struct {
	hwnd      uintptr
	hInstance uintptr
	width     int32
	height    int32
	running   bool
}

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

	wc := wndClassExW{
		Size:      uint32(unsafe.Sizeof(wndClassExW{})),
		Style:     csOwnDC,
		WndProc:   windows.NewCallback(wndProc),
		Instance:  hInstance,
		ClassName: className,
	}

	ret, _, callErr := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))) //nolint:gosec // G103: Win32 API
	if ret == 0 {
		return nil, fmt.Errorf("RegisterClassExW failed: %w", callErr)
	}

	style := uint32(wsOverlapped | wsCaption | wsSysMenu | wsMinimizeBox | wsMaximizeBox | wsThickFrame)

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
		width:     width,
		height:    height,
		running:   true,
	}

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

// PollEvents processes pending window events.
// Returns false when the window should close.
func (w *Window) PollEvents() bool {
	var m msg
	for {
		ret, _, _ := procPeekMessageW.Call(
			uintptr(unsafe.Pointer(&m)), //nolint:gosec // G103: Win32 API
			0,
			0,
			0,
			uintptr(pmRemove),
		)
		if ret == 0 {
			break
		}

		if m.Message == wmQuit {
			w.running = false
			return false
		}

		_, _, _ = procTranslateMessage.Call(uintptr(unsafe.Pointer(&m))) //nolint:gosec // G103: Win32 API
		_, _, _ = procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m))) //nolint:gosec // G103: Win32 API
	}

	return w.running
}

// wndProc is the window procedure callback.
func wndProc(hwnd, message, wParam, lParam uintptr) uintptr {
	switch message {
	case wmDestroy, wmClose:
		_, _, _ = procPostQuitMessage.Call(0)
		return 0
	default:
		ret, _, _ := procDefWindowProcW.Call(hwnd, message, wParam, lParam)
		return ret
	}
}
