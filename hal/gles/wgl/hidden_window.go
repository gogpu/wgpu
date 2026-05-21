// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows && !(js && wasm)

package wgl

import (
	"fmt"
	"sync"
	"syscall"
	"unsafe"
)

var (
	procRegisterClassExW *syscall.Proc
	procCreateWindowExW  *syscall.Proc
	procDestroyWindow    *syscall.Proc
	procDefWindowProcW   *syscall.Proc

	hiddenClassOnce sync.Once
	hiddenClassName [64]uint16
)

// WNDCLASSEXW is the Windows extended window class structure.
type WNDCLASSEXW struct {
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

const (
	csOwnDC = 0x0020
)

// initHiddenWindowProcs loads User32 procedures needed for hidden window management.
// Must be called after wgl.Init() (which loads user32.dll).
func initHiddenWindowProcs() error {
	if user32 == nil {
		return fmt.Errorf("wgl: user32.dll not loaded — call Init() first")
	}

	var err error
	procRegisterClassExW, err = user32.FindProc("RegisterClassExW")
	if err != nil {
		return fmt.Errorf("RegisterClassExW: %w", err)
	}

	procCreateWindowExW, err = user32.FindProc("CreateWindowExW")
	if err != nil {
		return fmt.Errorf("CreateWindowExW: %w", err)
	}

	procDestroyWindow, err = user32.FindProc("DestroyWindow")
	if err != nil {
		return fmt.Errorf("DestroyWindow: %w", err)
	}

	procDefWindowProcW, err = user32.FindProc("DefWindowProcW")
	if err != nil {
		return fmt.Errorf("DefWindowProcW: %w", err)
	}

	return nil
}

// registerHiddenWindowClass registers a window class for hidden GL windows.
// Uses CS_OWNDC so each window gets a persistent DC (required for WGL).
// Follows Rust wgpu-hal wgl.rs:272-342.
func registerHiddenWindowClass() error {
	var regErr error
	hiddenClassOnce.Do(func() {
		if err := initHiddenWindowProcs(); err != nil {
			regErr = err
			return
		}

		name := "wgpu_gles_hidden"
		for i, c := range name {
			hiddenClassName[i] = uint16(c)
		}
		hiddenClassName[len(name)] = 0

		wc := WNDCLASSEXW{
			Size:      uint32(unsafe.Sizeof(WNDCLASSEXW{})),
			Style:     csOwnDC,
			WndProc:   procDefWindowProcW.Addr(),
			ClassName: &hiddenClassName[0],
		}

		r, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
		if r == 0 {
			regErr = fmt.Errorf("RegisterClassExW failed: %w", err)
		}
	})
	return regErr
}

// HiddenWindow holds a hidden 1×1 window used to host the GL context.
// Created on the calling thread (not a goroutine) because Windows HWND/HDC
// handles are thread-safe but CS_OWNDC gives a persistent DC tied to the window.
// Follows Rust wgpu-hal wgl.rs:344-446 (InstanceDevice pattern).
//
// Rust uses a dedicated OS thread because Rust threads = OS threads.
// In Go, goroutines migrate between OS threads, so we create the hidden window
// on the caller's thread instead. The window has no message pump — it exists
// solely to own a DC for the GL context.
type HiddenWindow struct {
	hwnd HWND
	hdc  HDC
}

// NewHiddenWindow creates a hidden 1×1 window on the calling thread.
// The DC returned via HiddenWindow.DC() is valid until Destroy() is called.
func NewHiddenWindow() (*HiddenWindow, error) {
	if err := registerHiddenWindowClass(); err != nil {
		return nil, fmt.Errorf("wgl: register hidden window class: %w", err)
	}

	hwnd, err := createWindowEx(
		0,                   // dwExStyle
		&hiddenClassName[0], // lpClassName
		nil,                 // lpWindowName
		0,                   // dwStyle (no WS_VISIBLE)
		0, 0, 1, 1,          // x, y, w, h
		0, 0, 0, 0, // parent, menu, instance, param
	)
	if err != nil {
		return nil, fmt.Errorf("wgl: CreateWindowExW: %w", err)
	}

	hdc := GetDC(hwnd)
	if hdc == 0 {
		destroyWindow(hwnd)
		return nil, fmt.Errorf("wgl: GetDC failed for hidden window")
	}

	return &HiddenWindow{
		hwnd: hwnd,
		hdc:  hdc,
	}, nil
}

// DC returns the device context of the hidden window.
func (hw *HiddenWindow) DC() HDC {
	return hw.hdc
}

// HWND returns the hidden window handle.
func (hw *HiddenWindow) HWND() HWND {
	return hw.hwnd
}

// Destroy releases the DC and destroys the hidden window.
func (hw *HiddenWindow) Destroy() {
	if hw.hdc != 0 {
		ReleaseDC(hw.hwnd, hw.hdc)
		hw.hdc = 0
	}
	if hw.hwnd != 0 {
		destroyWindow(hw.hwnd)
		hw.hwnd = 0
	}
}

// createWindowEx wraps CreateWindowExW.
func createWindowEx(exStyle uint32, className *uint16, windowName *uint16, style uint32, x, y, w, h int32, parent, menu, instance, param uintptr) (HWND, error) {
	r, _, err := procCreateWindowExW.Call(
		uintptr(exStyle),
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windowName)),
		uintptr(style),
		uintptr(x), uintptr(y), uintptr(w), uintptr(h),
		parent, menu, instance, param,
	)
	if r == 0 {
		return 0, fmt.Errorf("CreateWindowExW failed: %w", err)
	}
	return HWND(r), nil
}

// destroyWindow wraps DestroyWindow.
func destroyWindow(hwnd HWND) {
	if procDestroyWindow != nil {
		procDestroyWindow.Call(uintptr(hwnd)) //nolint:errcheck // best-effort cleanup
	}
}
