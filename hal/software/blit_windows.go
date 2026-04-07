//go:build windows

package software

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32Blit = windows.NewLazySystemDLL("user32.dll")
	gdi32Blit  = windows.NewLazySystemDLL("gdi32.dll")

	procGetDC         = user32Blit.NewProc("GetDC")
	procReleaseDC     = user32Blit.NewProc("ReleaseDC")
	procStretchDIBits = gdi32Blit.NewProc("StretchDIBits")
)

const (
	dibRGBColors = 0
	srcCopy      = 0x00CC0020
	biRGB        = 0
)

// bitmapInfoHeader is the Windows BITMAPINFOHEADER structure.
type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

// blitFramebufferToWindow blits a BGRA framebuffer to the window using GDI StretchDIBits.
// The caller must provide pre-converted BGRA data (not RGBA).
func blitFramebufferToWindow(hwnd uintptr, bgra []byte, width, height int32) {
	if len(bgra) == 0 || width <= 0 || height <= 0 {
		return
	}

	hdc, _, _ := procGetDC.Call(hwnd)
	if hdc == 0 {
		return
	}
	defer procReleaseDC.Call(hwnd, hdc) //nolint:errcheck

	bmi := bitmapInfoHeader{
		Size:        uint32(unsafe.Sizeof(bitmapInfoHeader{})),
		Width:       width,
		Height:      -height, // negative = top-down DIB
		Planes:      1,
		BitCount:    32,
		Compression: biRGB,
	}

	procStretchDIBits.Call( //nolint:errcheck
		hdc,
		0, 0, // dest x, y
		uintptr(width),
		uintptr(height),
		0, 0, // src x, y
		uintptr(width),
		uintptr(height),
		uintptr(unsafe.Pointer(&bgra[0])),
		uintptr(unsafe.Pointer(&bmi)),
		uintptr(dibRGBColors),
		uintptr(srcCopy),
	)
}
