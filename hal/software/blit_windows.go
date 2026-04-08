//go:build windows

package software

import (
	"syscall"
	"unsafe"
)

var (
	user32DLL = syscall.MustLoadDLL("user32.dll")
	gdi32DLL  = syscall.MustLoadDLL("gdi32.dll")

	procGetDC         = user32DLL.MustFindProc("GetDC")
	procReleaseDC     = user32DLL.MustFindProc("ReleaseDC")
	procStretchDIBits = gdi32DLL.MustFindProc("StretchDIBits")
)

const (
	srccopy      = 0x00CC0020
	dibRGBColors = 0
)

type bitmapInfoHeader struct {
	biSize          uint32
	biWidth         int32
	biHeight        int32
	biPlanes        uint16
	biBitCount      uint16
	biCompression   uint32
	biSizeImage     uint32
	biXPelsPerMeter int32
	biYPelsPerMeter int32
	biClrUsed       uint32
	biClrImportant  uint32
}

// blitFramebufferToWindow blits BGRA framebuffer to window via GDI StretchDIBits.
// GDI BI_RGB 32-bit expects BGRA byte order — no conversion needed.
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
		biSize:     uint32(unsafe.Sizeof(bitmapInfoHeader{})),
		biWidth:    width,
		biHeight:   -height, // negative = top-down DIB
		biPlanes:   1,
		biBitCount: 32,
	}

	procStretchDIBits.Call( //nolint:errcheck
		hdc,
		0, 0, uintptr(width), uintptr(height),
		0, 0, uintptr(width), uintptr(height),
		uintptr(unsafe.Pointer(&bgra[0])),
		uintptr(unsafe.Pointer(&bmi)),
		uintptr(dibRGBColors),
		uintptr(srccopy),
	)
}
