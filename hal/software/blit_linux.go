//go:build linux

package software

import (
	"log/slog"
	"sync"
	"unsafe"

	"github.com/go-webgpu/goffi/ffi"
	"github.com/go-webgpu/goffi/types"
)

// X11 constants (from X.h / Xlib.h).
const (
	zPixmap  = 2 // ZPixmap format for XImage
	lsbFirst = 0 // LSBFirst byte order (little-endian)
)

// ximage matches the C XImage struct layout on amd64 Linux (LP64).
//
// Reference: X11/Xlib.h _XImage struct.
// Layout verified against Skia RasterWindowContext_unix.cpp and SDL3
// SDL_x11framebuffer.c — both construct XImage on stack with these fields.
//
// The struct ends with a funcs sub-struct of 6 function pointers (48 bytes
// on amd64) that XInitImage fills in. We include them as opaque padding so
// XInitImage has room to write without corrupting adjacent memory.
type ximage struct {
	width          int32   // 0
	height         int32   // 4
	xoffset        int32   // 8
	format         int32   // 12
	data           uintptr // 16 (char*)
	byteOrder      int32   // 24
	bitmapUnit     int32   // 28
	bitmapBitOrder int32   // 32
	bitmapPad      int32   // 36
	depth          int32   // 40
	bytesPerLine   int32   // 44
	bitsPerPixel   int32   // 48
	_pad0          int32   // 52 (padding to align unsigned long)
	redMask        uint64  // 56 (unsigned long on LP64)
	greenMask      uint64  // 64
	blueMask       uint64  // 72
	obdata         uintptr // 80 (XPointer = char*)
	// funcs: 6 function pointers filled by XInitImage
	funcCreateImage  uintptr // 88
	funcDestroyImage uintptr // 96
	funcGetPixel     uintptr // 104
	funcPutPixel     uintptr // 112
	funcSubImage     uintptr // 120
	funcAddPixel     uintptr // 128
	// Total: 136 bytes
}

// x11State holds lazily-loaded libX11 symbols and call interfaces.
// Initialized once on first blit; subsequent blits reuse the state.
var (
	x11Once  sync.Once
	x11Ready bool // true if libX11 loaded successfully

	x11Lib unsafe.Pointer

	symXCreateGC  unsafe.Pointer
	symXFreeGC    unsafe.Pointer
	symXPutImage  unsafe.Pointer
	symXFlush     unsafe.Pointer
	symXInitImage unsafe.Pointer

	// XCreateGC(Display*, Drawable, unsigned long valuemask, XGCValues*) -> GC
	cifXCreateGC types.CallInterface
	// XFreeGC(Display*, GC) -> int
	cifXFreeGC types.CallInterface
	// XPutImage(Display*, Drawable, GC, XImage*, int, int, int, int, uint, uint) -> int
	cifXPutImage types.CallInterface
	// XFlush(Display*) -> int
	cifXFlush types.CallInterface
	// XInitImage(XImage*) -> Status (int)
	cifXInitImage types.CallInterface
)

// initX11 loads libX11.so and prepares call interfaces.
// Called once via sync.Once; sets x11Ready on success.
func initX11() {
	var err error

	// Load library: try .so.6 first (standard versioned SONAME), then unversioned.
	x11Lib, err = ffi.LoadLibrary("libX11.so.6")
	if err != nil {
		x11Lib, err = ffi.LoadLibrary("libX11.so")
		if err != nil {
			slog.Debug("software: X11 blit unavailable — could not load libX11", "error", err)
			return
		}
	}

	// Load symbols.
	symXCreateGC, err = ffi.GetSymbol(x11Lib, "XCreateGC")
	if err != nil {
		return
	}
	symXFreeGC, err = ffi.GetSymbol(x11Lib, "XFreeGC")
	if err != nil {
		return
	}
	symXPutImage, err = ffi.GetSymbol(x11Lib, "XPutImage")
	if err != nil {
		return
	}
	symXFlush, err = ffi.GetSymbol(x11Lib, "XFlush")
	if err != nil {
		return
	}
	symXInitImage, err = ffi.GetSymbol(x11Lib, "XInitImage")
	if err != nil {
		return
	}

	// Prepare call interfaces.

	// GC XCreateGC(Display *display, Drawable d, unsigned long valuemask, XGCValues *values)
	// On LP64: Drawable = unsigned long (8 bytes), valuemask = unsigned long (8 bytes)
	// GC = pointer. Using Pointer for Drawable since it's XID (unsigned long = pointer-sized on LP64).
	err = ffi.PrepareCallInterface(&cifXCreateGC, types.DefaultCall,
		types.PointerTypeDescriptor, // return: GC (pointer)
		[]*types.TypeDescriptor{
			types.PointerTypeDescriptor, // Display*
			types.PointerTypeDescriptor, // Drawable (unsigned long = pointer-sized)
			types.PointerTypeDescriptor, // unsigned long valuemask (pointer-sized on LP64)
			types.PointerTypeDescriptor, // XGCValues*
		})
	if err != nil {
		return
	}

	// int XFreeGC(Display *display, GC gc)
	err = ffi.PrepareCallInterface(&cifXFreeGC, types.DefaultCall,
		types.SInt32TypeDescriptor, // return: int
		[]*types.TypeDescriptor{
			types.PointerTypeDescriptor, // Display*
			types.PointerTypeDescriptor, // GC
		})
	if err != nil {
		return
	}

	// int XPutImage(Display*, Drawable, GC, XImage*, int, int, int, int, unsigned int, unsigned int)
	err = ffi.PrepareCallInterface(&cifXPutImage, types.DefaultCall,
		types.SInt32TypeDescriptor, // return: int
		[]*types.TypeDescriptor{
			types.PointerTypeDescriptor, // Display*
			types.PointerTypeDescriptor, // Drawable (unsigned long)
			types.PointerTypeDescriptor, // GC
			types.PointerTypeDescriptor, // XImage*
			types.SInt32TypeDescriptor,  // src_x
			types.SInt32TypeDescriptor,  // src_y
			types.SInt32TypeDescriptor,  // dest_x
			types.SInt32TypeDescriptor,  // dest_y
			types.UInt32TypeDescriptor,  // width
			types.UInt32TypeDescriptor,  // height
		})
	if err != nil {
		return
	}

	// int XFlush(Display *display)
	err = ffi.PrepareCallInterface(&cifXFlush, types.DefaultCall,
		types.SInt32TypeDescriptor, // return: int
		[]*types.TypeDescriptor{
			types.PointerTypeDescriptor, // Display*
		})
	if err != nil {
		return
	}

	// Status XInitImage(XImage *image)
	err = ffi.PrepareCallInterface(&cifXInitImage, types.DefaultCall,
		types.SInt32TypeDescriptor, // return: Status (int)
		[]*types.TypeDescriptor{
			types.PointerTypeDescriptor, // XImage*
		})
	if err != nil {
		return
	}

	x11Ready = true
	slog.Debug("software: X11 blit initialized — XPutImage path available")
}

// platformBlit holds X11 GC for blit operations.
// Embedded in Surface via build tags.
type platformBlit struct {
	gc uintptr // X11 GC (Graphics Context), lazy-initialized on first blit
}

// createPlatformFramebuffer returns nil on Linux — we use Go heap memory
// and blit via XPutImage (unlike Windows which uses kernel-allocated DIB section).
func (s *Surface) createPlatformFramebuffer(_, _ int32) []byte { return nil }

// destroyPlatformFramebuffer releases the X11 GC if one was created.
func (s *Surface) destroyPlatformFramebuffer() {
	if s.gc == 0 || s.displayHandle == 0 {
		return
	}

	x11Once.Do(initX11)
	if !x11Ready {
		s.gc = 0
		return
	}

	display := s.displayHandle
	gc := s.gc
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&display),
		unsafe.Pointer(&gc),
	}
	_ = ffi.CallFunction(&cifXFreeGC, symXFreeGC, nil, args[:])
	s.gc = 0
}

// blitFramebufferToWindow copies the framebuffer to the X11 window via XPutImage.
//
// Follows the Skia enterprise pattern (RasterWindowContext_unix.cpp):
// construct XImage on stack pointing to our BGRA pixel buffer, call XInitImage
// to fill function pointers, then XPutImage to send pixels to the X server.
//
// No pixel format conversion is needed: our framebuffer stores BGRA (the software
// rasterizer's executeFullscreenBlit does RGBA→BGRA swizzle), which matches X11's
// 32bpp ZPixmap LSBFirst format on little-endian systems. This is the same reason
// Skia and SDL3 set byte_order=LSBFirst and depth=24 with bits_per_pixel=32.
func (s *Surface) blitFramebufferToWindow(data []byte, width, height int32) {
	if s.hwnd == 0 || s.displayHandle == 0 || width <= 0 || height <= 0 || len(data) == 0 {
		return
	}

	// Lazy-load libX11 on first blit.
	x11Once.Do(initX11)
	if !x11Ready {
		return
	}

	display := s.displayHandle
	window := s.hwnd

	// Lazy-create GC on first blit (same pattern as SDL3: XCreateGC in CreateWindowFramebuffer).
	if s.gc == 0 {
		var valuemask uintptr // 0 = no special GC values
		var values uintptr    // NULL = default values
		var gc uintptr
		args := [4]unsafe.Pointer{
			unsafe.Pointer(&display),
			unsafe.Pointer(&window),
			unsafe.Pointer(&valuemask),
			unsafe.Pointer(&values),
		}
		_ = ffi.CallFunction(&cifXCreateGC, symXCreateGC, unsafe.Pointer(&gc), args[:])
		if gc == 0 {
			slog.Warn("software: XCreateGC failed — blit skipped")
			return
		}
		s.gc = gc
	}

	// Construct XImage on stack pointing to our pixel buffer.
	// Fields follow Skia RasterWindowContext_unix.cpp exactly.
	var image ximage
	image.width = width
	image.height = height
	image.format = zPixmap
	image.data = uintptr(unsafe.Pointer(&data[0]))
	image.byteOrder = lsbFirst
	image.bitmapUnit = 32
	image.bitmapBitOrder = lsbFirst
	image.bitmapPad = 32
	image.depth = 24
	image.bytesPerLine = 0 // XInitImage calculates this
	image.bitsPerPixel = 32

	// XInitImage fills the function pointer struct at the end of XImage.
	// Without this call, XPutImage would crash dereferencing nil func ptrs.
	imagePtr := uintptr(unsafe.Pointer(&image))
	var status int32
	initArgs := [1]unsafe.Pointer{unsafe.Pointer(&imagePtr)}
	_ = ffi.CallFunction(&cifXInitImage, symXInitImage, unsafe.Pointer(&status), initArgs[:])
	if status == 0 {
		slog.Warn("software: XInitImage failed — blit skipped")
		return
	}

	// XPutImage: copy pixel data to the X server.
	gc := s.gc
	var srcX, srcY, dstX, dstY int32
	w := uint32(width)
	h := uint32(height)
	putArgs := [10]unsafe.Pointer{
		unsafe.Pointer(&display),
		unsafe.Pointer(&window),
		unsafe.Pointer(&gc),
		unsafe.Pointer(&imagePtr),
		unsafe.Pointer(&srcX),
		unsafe.Pointer(&srcY),
		unsafe.Pointer(&dstX),
		unsafe.Pointer(&dstY),
		unsafe.Pointer(&w),
		unsafe.Pointer(&h),
	}
	_ = ffi.CallFunction(&cifXPutImage, symXPutImage, nil, putArgs[:])

	// XFlush ensures the X server processes the put request immediately.
	// Without this, pixels may not appear until the next X event.
	flushArgs := [1]unsafe.Pointer{unsafe.Pointer(&display)}
	_ = ffi.CallFunction(&cifXFlush, symXFlush, nil, flushArgs[:])
}
