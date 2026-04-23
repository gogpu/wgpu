//go:build windows && !(js && wasm)

package software

import (
	"syscall"
	"unsafe"
)

var (
	user32DLL = syscall.MustLoadDLL("user32.dll")
	gdi32DLL  = syscall.MustLoadDLL("gdi32.dll")

	procGetDC            = user32DLL.MustFindProc("GetDC")
	procReleaseDC        = user32DLL.MustFindProc("ReleaseDC")
	procCreateCompatDC   = gdi32DLL.MustFindProc("CreateCompatibleDC")
	procDeleteDC         = gdi32DLL.MustFindProc("DeleteDC")
	procCreateDIBSection = gdi32DLL.MustFindProc("CreateDIBSection")
	procDeleteObject     = gdi32DLL.MustFindProc("DeleteObject")
	procSelectObject     = gdi32DLL.MustFindProc("SelectObject")
	procBitBlt           = gdi32DLL.MustFindProc("BitBlt")
	procStretchDIBits    = gdi32DLL.MustFindProc("StretchDIBits")
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

// platformBlit holds Windows GDI resources for DIB section-based presentation.
// Embedded in Surface via build tags.
type platformBlit struct {
	memDC  uintptr // memory DC with DIB section selected
	bitmap uintptr // DIB section bitmap handle
	oldBmp uintptr // previous bitmap (for cleanup)
}

// createPlatformFramebuffer creates a DIB section backed by GDI.
// Returns the DIB pixel buffer as a Go []byte slice (zero-copy).
// The returned slice is backed by kernel memory, not Go heap — no GC pressure.
// On failure (headless, hwnd==0), returns nil and caller falls back to Go memory.
//
// The OLD DIB section is destroyed only AFTER the new one is created successfully.
// This prevents use-after-free: if creation fails, s.framebuffer still points
// to the old (valid) DIB section pixels and the fallback path can safely check cap().
func (s *Surface) createPlatformFramebuffer(width, height int32) []byte {
	if s.hwnd == 0 || width <= 0 || height <= 0 {
		return nil
	}

	// Get window DC to create compatible memory DC.
	windowDC, _, _ := procGetDC.Call(s.hwnd)
	if windowDC == 0 {
		return nil
	}
	defer procReleaseDC.Call(s.hwnd, windowDC) //nolint:errcheck

	// Create memory DC compatible with window.
	memDC, _, _ := procCreateCompatDC.Call(windowDC)
	if memDC == 0 {
		return nil
	}

	bmi := bitmapInfoHeader{
		biSize:     uint32(unsafe.Sizeof(bitmapInfoHeader{})),
		biWidth:    width,
		biHeight:   -height, // negative = top-down DIB
		biPlanes:   1,
		biBitCount: 32,
	}

	// Create DIB section — GDI allocates the pixel buffer.
	var pixels unsafe.Pointer
	bitmap, _, _ := procCreateDIBSection.Call(
		memDC,
		uintptr(unsafe.Pointer(&bmi)),
		uintptr(dibRGBColors),
		uintptr(unsafe.Pointer(&pixels)),
		0, 0,
	)
	if bitmap == 0 || pixels == nil {
		procDeleteDC.Call(memDC) //nolint:errcheck
		return nil
	}

	// Select DIB section into memory DC for BitBlt.
	oldBmp, _, _ := procSelectObject.Call(memDC, bitmap)

	// New DIB section created successfully — now safe to destroy the old one.
	s.destroyPlatformFramebuffer()

	s.memDC = memDC
	s.bitmap = bitmap
	s.oldBmp = oldBmp

	// Wrap kernel-allocated pixels as Go slice (zero-copy).
	size := int(width) * int(height) * 4
	return unsafe.Slice((*byte)(pixels), size)
}

// destroyPlatformFramebuffer releases DIB section GDI resources.
func (s *Surface) destroyPlatformFramebuffer() {
	if s.memDC == 0 {
		return
	}
	if s.oldBmp != 0 {
		procSelectObject.Call(s.memDC, s.oldBmp) //nolint:errcheck
	}
	if s.bitmap != 0 {
		procDeleteObject.Call(s.bitmap) //nolint:errcheck
	}
	procDeleteDC.Call(s.memDC) //nolint:errcheck
	s.memDC = 0
	s.bitmap = 0
	s.oldBmp = 0
}

// blitFramebufferToWindow copies DIB section to window via BitBlt.
// If DIB section is active (memDC != 0), uses BitBlt (enterprise pattern).
// Otherwise falls back to StretchDIBits with raw pixel data.
func (s *Surface) blitFramebufferToWindow(data []byte, width, height int32) {
	if s.hwnd == 0 || width <= 0 || height <= 0 {
		return
	}

	windowDC, _, _ := procGetDC.Call(s.hwnd)
	if windowDC == 0 {
		return
	}
	defer procReleaseDC.Call(s.hwnd, windowDC) //nolint:errcheck

	if s.memDC != 0 {
		// Enterprise path: BitBlt from DIB section memory DC.
		// DWM tracks this properly — no freeze after resize.
		procBitBlt.Call( //nolint:errcheck
			windowDC,
			0, 0, uintptr(width), uintptr(height),
			s.memDC,
			0, 0,
			uintptr(srccopy),
		)
		return
	}

	// Fallback: StretchDIBits with raw pixel data (headless/non-DIB path).
	if len(data) == 0 {
		return
	}
	bmi := bitmapInfoHeader{
		biSize:     uint32(unsafe.Sizeof(bitmapInfoHeader{})),
		biWidth:    width,
		biHeight:   -height,
		biPlanes:   1,
		biBitCount: 32,
	}
	procStretchDIBits.Call( //nolint:errcheck
		windowDC,
		0, 0, uintptr(width), uintptr(height),
		0, 0, uintptr(width), uintptr(height),
		uintptr(unsafe.Pointer(&data[0])),
		uintptr(unsafe.Pointer(&bmi)),
		uintptr(dibRGBColors),
		uintptr(srccopy),
	)
}
