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
	procBeginPaint         = user32.NewProc("BeginPaint")
	procEndPaint           = user32.NewProc("EndPaint")
)

const (
	csOwnDC = 0x0020

	// Window styles
	wsOverlappedWindow = 0x00CF0000
	wsVisible          = 0x10000000

	swShow = 5

	// Window messages
	wmDestroy       = 0x0002
	wmSize          = 0x0005
	wmPaint         = 0x000F
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

// paintStruct is the Windows PAINTSTRUCT.
type paintStruct struct {
	HDC         uintptr
	FErase      int32
	RcPaintL    int32
	RcPaintT    int32
	RcPaintR    int32
	RcPaintB    int32
	FRestore    int32
	FIncUpdate  int32
	RGBReserved [32]byte
}

// Window represents a platform window for software rendering.
type Window struct {
	hwnd      uintptr
	hInstance uintptr
	cursor    uintptr
	width     int32
	height    int32
	running   bool

	// Resize handling
	inSizeMove  atomic.Bool
	needsResize atomic.Bool
	pendingW    atomic.Int32
	pendingH    atomic.Int32

	animating atomic.Bool
}

// Global window pointer for wndProc callback.
var globalWindow *Window

// NewWindow creates a new window with the given title and size.
func NewWindow(title string, width, height int32) (*Window, error) {
	hInstance, _, _ := procGetModuleHandleW.Call(0)

	className, err := windows.UTF16PtrFromString("SwTriangleWindow")
	if err != nil {
		return nil, fmt.Errorf("failed to create class name: %w", err)
	}
	windowTitle, err := windows.UTF16PtrFromString(title)
	if err != nil {
		return nil, fmt.Errorf("failed to create window title: %w", err)
	}

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

	var rc rect
	rc.Right = width
	rc.Bottom = height
	procAdjustWindowRectEx.Call( //nolint:errcheck,gosec // G103: Win32 API
		uintptr(unsafe.Pointer(&rc)), //nolint:gosec // G103: Win32 API
		uintptr(style),
		0,
		0,
	)

	hwnd, _, callErr := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),   //nolint:gosec // G103: Win32 API
		uintptr(unsafe.Pointer(windowTitle)), //nolint:gosec // G103: Win32 API
		uintptr(style),
		100, 100,
		uintptr(rc.Right-rc.Left), //nolint:gosec // G115: window dimensions always positive
		uintptr(rc.Bottom-rc.Top), //nolint:gosec // G115: window dimensions always positive
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
	w.animating.Store(true)

	globalWindow = w
	procSetWindowLongPtrW.Call(hwnd, ^uintptr(20), uintptr(unsafe.Pointer(w))) //nolint:errcheck,gosec

	procShowWindow.Call(hwnd, uintptr(swShow)) //nolint:errcheck,gosec
	procUpdateWindow.Call(hwnd)                //nolint:errcheck,gosec

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

// NeedsResize returns true if a resize event occurred and clears the flag.
func (w *Window) NeedsResize() bool {
	return w.needsResize.Swap(false)
}

// PollEvents processes pending window events.
// Returns false when the window should close.
func (w *Window) PollEvents() bool {
	var m msg

	for {
		ret, _, _ := procPeekMessageW.Call(
			uintptr(unsafe.Pointer(&m)), //nolint:gosec // G103: Win32 API
			0, 0, 0,
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
	w := globalWindow
	if w == nil || w.hwnd != hwnd {
		ret, _, _ := procDefWindowProcW.Call(hwnd, message, wParam, lParam)
		return ret
	}

	switch message {
	case wmDestroy, wmClose:
		_, _, _ = procPostQuitMessage.Call(0)
		return 0

	case wmPaint:
		// Validate the region to prevent continuous WM_PAINT.
		// Software backend renders via direct GetDC blit in Present(),
		// so WM_PAINT just needs to validate without erasing.
		var ps paintStruct
		procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps))) //nolint:errcheck,gosec
		procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))   //nolint:errcheck,gosec
		return 0

	case wmEnterSizeMove:
		w.inSizeMove.Store(true)
		return 0

	case wmExitSizeMove:
		w.inSizeMove.Store(false)
		if w.pendingW.Load() > 0 && w.pendingH.Load() > 0 {
			w.needsResize.Store(true)
		}
		return 0

	case wmSize:
		width := int32(lParam & 0xFFFF)
		height := int32((lParam >> 16) & 0xFFFF)

		if width > 0 && height > 0 {
			w.width = width
			w.height = height
			w.pendingW.Store(width)
			w.pendingH.Store(height)

			if !w.inSizeMove.Load() {
				w.needsResize.Store(true)
			}
		}
		return 0

	case wmSetCursor:
		hitTest := lParam & 0xFFFF
		if hitTest == htClient {
			_, _, _ = procSetCursor.Call(w.cursor)
			return 1
		}
		ret, _, _ := procDefWindowProcW.Call(hwnd, message, wParam, lParam)
		return ret

	default:
		ret, _, _ := procDefWindowProcW.Call(hwnd, message, wParam, lParam)
		return ret
	}
}
