//go:build darwin && !(js && wasm)

package software

import (
	"fmt"
	"image"
	"log/slog"
	"runtime"
	"sync"
	"unsafe"

	"github.com/go-webgpu/goffi/ffi"
	"github.com/go-webgpu/goffi/types"
)

// platformBlit is a no-op on platforms without native blit support.
// Windows has GDI (blit_windows.go), Linux has X11 (blit_linux.go).
type platformBlit struct {
	once           sync.Once
	objc           *objcReflect
	cg             *coreGraphics
	colorSpace     cgColorSpace
	isIinitialized bool
}

func (p *platformBlit) init() (err error) {
	p.objc = new(objcReflect)
	if err := p.objc.Open(); err != nil {
		return err
	}

	p.cg = new(coreGraphics)
	if err := p.cg.Open(p.objc); err != nil {
		return err
	}

	p.colorSpace, err = p.cg.ColorSpaceCreateDeviceRGB()
	if err != nil {
		return err
	}

	p.isIinitialized = true

	return nil
}

func (p *platformBlit) onceInit() bool {
	p.once.Do(func() {
		if err := p.init(); err != nil {
			slog.Debug("software: failed to initialize objc binding", slog.Any("error", err))
		}
	})

	return p.isIinitialized
}

// createPlatformFramebuffer returns nil — use Go heap memory.
func (s *Surface) createPlatformFramebuffer(_, _ int32) []byte { return nil }

// destroyPlatformFramebuffer is a no-op.
func (s *Surface) destroyPlatformFramebuffer() {}

// blitFramebufferToWindow is a no-op on unsupported platforms.
// TODO: implement CGImage+CALayer blit for macOS (Phase 2).
func (s *Surface) blitFramebufferToWindow(data []byte, width, height int32) {
	if s.hwnd == 0 {
		return
	}

	if !s.platformBlit.onceInit() {
		return
	}

	objc, cg, colorSpace := s.platformBlit.objc, s.platformBlit.cg, s.platformBlit.colorSpace

	dataProvider, err := cg.DataProviderCreateWithData(objcAnyOpaqueNil, objcAnyOpaqueFromPointer(unsafe.Pointer(unsafe.SliceData(data))), uintptr(len(data)), objcAnyOpaqueNil)
	if err != nil {
		slog.Debug("software: failed to create CGDataProvider", slog.Any("error", err))
		return
	}
	defer cg.DataProviderRelease(dataProvider)

	binfo := cgBitmapInfoByteOrder32Little.
		WithImageAlphaInfo(cgImageAlphaPremultipliedFirst)

	img, err := cg.ImageCreate(uintptr(width), uintptr(height), 8, 32, 4*uintptr(width), colorSpace, binfo, dataProvider, objcAnyOpaqueNil, false, cgColorRenderingIntentDefault)
	if err != nil {
		slog.Debug("software: failed to create CGImage", slog.Any("error", err))
		return
	}

	defer cg.ImageRelease(img)

	l := caLayer{objcAnyOpaqueFromPointer(unsafe.Pointer(s.hwnd))}

	if err := l.SetNeedsDisplay(objc); err != nil {
		slog.Debug("software: failed to set NeedsDisplay", slog.Any("error", err))
		return
	}

	if err := l.SetContents(objc, img.objcAnyOpaque); err != nil {
		slog.Debug("software: failed to [caLayer setContents: img]", slog.Any("error", err), slog.Any("img", img))
		return
	}
}

// blitDamageRectsToWindow is a no-op on unsupported platforms.
func (s *Surface) blitDamageRectsToWindow(_ []byte, _, _ int32, _ []image.Rectangle) {

}

// --- BEGIN OBJC BINDING ---

type caLayer struct{ objcAnyOpaque }

// ref: https://developer.apple.com/documentation/quartzcore/calayer/setneedsdisplay()?language=objc
func (c *caLayer) SetNeedsDisplay(objc *objcReflect) error {
	sel, err := objc.SelRegisterName("setNeedsDisplay")
	if err != nil {
		return err
	}

	ret := objcAnyOpaqueFromPointer(unsafe.Pointer(new(uintptr)))
	if err := objc.MsgSend(c.objcAnyOpaque, sel.objcAnyOpaque, ret); err != nil {
		return err
	}

	return nil
}

func (c *caLayer) SetContents(objc *objcReflect, obj objcAnyOpaque) error {
	sel, err := objc.SelRegisterName("setContents:")
	if err != nil {
		return err
	}

	ret := objcAnyOpaqueFromPointer(unsafe.Pointer(new(uintptr)))
	if err := objc.MsgSend(c.objcAnyOpaque, sel.objcAnyOpaque, ret, obj); err != nil {
		return err
	}

	return nil
}

type cgImageAlphaInfo uint32

// ref: https://learn.microsoft.com/ja-jp/dotnet/api/coregraphics.cgimagealphainfo
const (
	cgImageAlphaPremultipliedFirst cgImageAlphaInfo = 2
)

type cgBitmapInfo uint32

// ref: https://learn.microsoft.com/ja-jp/dotnet/api/coregraphics.cgbitmapflags
const (
	cgBitmapInfoByteOrder32Little cgBitmapInfo = 8192
)

func (c cgBitmapInfo) ImageAlphaInfo() cgImageAlphaInfo {
	return cgImageAlphaInfo(c & cgBitmapInfoAlphaInfoMask)
}

func (c cgBitmapInfo) WithImageAlphaInfo(a cgImageAlphaInfo) cgBitmapInfo {
	return c ^ (c & cgBitmapInfoAlphaInfoMask) | (cgBitmapInfo(a) & cgBitmapInfoAlphaInfoMask)
}

const (
	// ref: https://developer.apple.com/documentation/coregraphics/cgbitmapinfo/kcgbitmapalphainfomask?language=objc
	// ref: https://learn.microsoft.com/en-us/dotnet/api/coregraphics.cgbitmapinfo
	cgBitmapInfoAlphaInfoMask cgBitmapInfo = 31
)

// ref: https://developer.apple.com/documentation/coregraphics/cgcolorrenderingintent?language=objc
// ref: https://learn.microsoft.com/ja-jp/dotnet/api/coregraphics.cgcolorrenderingintent
type cgColorRenderingIntent uint32

func (c cgColorRenderingIntent) String() string {
	const typname = "CGColorRenderingIntent"

	switch c {
	case cgColorRenderingIntentDefault:
		return fmt.Sprintf(typname+"(Default: %d)", c)

	case cgColorRenderingIntentAbsoluteColorimetric:
		return fmt.Sprintf(typname+"(AbsoluteColorimetric: %d)", c)

	case cgColorRenderingIntentRelativeColorimetric:
		return fmt.Sprintf(typname+"(RelativeColorimetric: %d)", c)

	case cgColorRenderingIntentPerceptual:
		return fmt.Sprintf(typname+"(Perceptual: %d)", c)

	case cgColorRenderingIntentSaturation:
		return fmt.Sprintf(typname+"(Saturation: %d)", c)

	default:
		return fmt.Sprintf(typname+"(<UNK>: %d)", c)
	}
}

const (
	cgColorRenderingIntentDefault              cgColorRenderingIntent = 0
	cgColorRenderingIntentAbsoluteColorimetric cgColorRenderingIntent = 1
	cgColorRenderingIntentRelativeColorimetric cgColorRenderingIntent = 2
	cgColorRenderingIntentPerceptual           cgColorRenderingIntent = 3
	cgColorRenderingIntentSaturation           cgColorRenderingIntent = 4
)

type cgDataProvider struct{ objcAnyOpaque }
type cgColorSpace struct{ objcAnyOpaque }
type cgImage struct{ objcAnyOpaque }

var (
	cifCoreGraphicsImageCreate = &types.CallInterface{
		ArgCount: 11,
		ArgTypes: []*types.TypeDescriptor{
			// size_t width
			types.UInt64TypeDescriptor,
			// size_t height
			types.UInt64TypeDescriptor,
			// size_t bitsPerComponent
			types.UInt64TypeDescriptor,
			// size_t bitsPerPixel
			types.UInt64TypeDescriptor,
			// size_t bytesPerRow
			types.UInt64TypeDescriptor,
			// CGColorSpaceRef space
			types.PointerTypeDescriptor,
			// (CGBitMapInfo: uint32_t) bitmapInfo
			// ref: https://developer.apple.com/documentation/coregraphics/cgbitmapinfo?language=objc
			types.UInt32TypeDescriptor,
			// CGDataProviderRef provider
			types.PointerTypeDescriptor,
			// const (CGFloat: double)*
			// ref: https://developer.apple.com/documentation/CoreFoundation/CGFloat-c.typealias?language=objc
			types.PointerTypeDescriptor,
			// bool shouldInterpolate
			types.UInt8TypeDescriptor,
			// (CGRenderingIntent: uint32_t) intent
			// ref: https://developer.apple.com/documentation/coregraphics/cgcolorrenderingintent?language=objc
			types.UInt32TypeDescriptor,
		},
		ReturnType: types.PointerTypeDescriptor,
	}

	cifCoreGraphicsDataProviderCreateWithData = &types.CallInterface{
		ArgCount: 4,
		ArgTypes: []*types.TypeDescriptor{
			// void* info
			types.PointerTypeDescriptor,
			// const void* data
			types.PointerTypeDescriptor,
			// size_t size
			types.PointerTypeDescriptor,
			// CGDataProviderReleaseDataCallback releaseData
			types.PointerTypeDescriptor,
		},
		ReturnType: types.PointerTypeDescriptor,
	}

	cifCoreGraphicsImageRelease = &types.CallInterface{
		ArgCount: 1,
		ArgTypes: []*types.TypeDescriptor{
			types.PointerTypeDescriptor,
		},
		ReturnType: types.VoidTypeDescriptor,
	}

	cifCoreGraphicsDataProviderRelease = cifCoreGraphicsImageRelease

	cifCoreGraphicsColorSpaceCreateDeviceRGB = &types.CallInterface{
		ArgCount:   0,
		ReturnType: types.PointerTypeDescriptor,
	}

	// cifCoreGraphicsColorSpaceRelease = cifCoreGraphicsImageRelease
)

const coreGraphicsLibraryLocation = "/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics"

type coreGraphics struct {
	objc *objcReflect

	lib                             unsafe.Pointer
	symCGImageCreate                unsafe.Pointer
	symCGDataProviderCreateWithData unsafe.Pointer
	symCGImageRelease               unsafe.Pointer
	symCGDataProviderRelease        unsafe.Pointer
	symCGColorSpaceCreateDeviceRGB  unsafe.Pointer
	symCGColorSpaceRelease          unsafe.Pointer
}

func (c *coreGraphics) Open(objc *objcReflect) (err error) {
	if c.lib, err = ffi.LoadLibrary(coreGraphicsLibraryLocation); err != nil {
		return c.errorf("failed to load CoreGraphics: %w", err)
	}

	if c.symCGImageCreate, err = ffi.GetSymbol(c.lib, "CGImageCreate"); err != nil {
		return c.errorf("CGImageCreate not found: %w", err)
	}

	if c.symCGDataProviderCreateWithData, err = ffi.GetSymbol(c.lib, "CGDataProviderCreateWithData"); err != nil {
		return c.errorf("CGDataProviderCreateWithData not found: %w", err)
	}

	if c.symCGImageRelease, err = ffi.GetSymbol(c.lib, "CGImageRelease"); err != nil {
		return c.errorf("CGImageRelease not found: %w", err)
	}

	if c.symCGDataProviderRelease, err = ffi.GetSymbol(c.lib, "CGDataProviderRelease"); err != nil {
		return c.errorf("CGDataProviderRelease not found: %w", err)
	}

	if c.symCGColorSpaceCreateDeviceRGB, err = ffi.GetSymbol(c.lib, "CGColorSpaceCreateDeviceRGB"); err != nil {
		return c.errorf("CGColorSpaceCreateDeviceRGB not found: %w", err)
	}

	if c.symCGColorSpaceRelease, err = ffi.GetSymbol(c.lib, "CGColorSpaceRelease"); err != nil {
		return c.errorf("CGColorSpaceRelease not found: %w", err)
	}

	return nil
}

func (c *coreGraphics) ImageCreate(
	width, height uintptr,
	bitsPerComponent, bitsPerPixel, bytesPerRow uintptr,
	space cgColorSpace,
	bitmapInfo cgBitmapInfo, provider cgDataProvider,
	decode objcAnyOpaque,
	shouldInterpolate bool,
	intent cgColorRenderingIntent,
) (cgimg cgImage, err error) {
	cgimg.typ = types.PointerTypeDescriptor

	// CGImageRef: (struct CGImage)*
	cgimg.ptr = unsafe.Pointer(nil)

	err = ffi.CallFunction(cifCoreGraphicsImageCreate, c.symCGImageCreate, unsafe.Pointer(&cgimg.ptr), []unsafe.Pointer{
		unsafe.Pointer(&width),
		unsafe.Pointer(&height),
		unsafe.Pointer(&bitsPerComponent),
		unsafe.Pointer(&bitsPerPixel),
		unsafe.Pointer(&bytesPerRow),
		unsafe.Pointer(space.Pointer()),
		unsafe.Pointer(&bitmapInfo),
		unsafe.Pointer(provider.Pointer()),
		unsafe.Pointer(decode.Pointer()),
		unsafe.Pointer(&shouldInterpolate),
		unsafe.Pointer(&intent),
	})
	if err != nil {
		return cgImage{objcAnyOpaqueNil}, c.errorf("failed to CGImageCreate: %w", err)
	}

	return cgimg, nil
}

func (c *coreGraphics) DataProviderCreateWithData(
	info objcAnyOpaque,
	data objcAnyOpaque,
	size uintptr,
	releaseDataCallback objcAnyOpaque,
) (cgprovider cgDataProvider, err error) {
	cgprovider.typ = types.PointerTypeDescriptor

	// CGDataProviderRef: (struct CGDataProvider)*
	cgprovider.ptr = unsafe.Pointer(nil)

	err = ffi.CallFunction(cifCoreGraphicsDataProviderCreateWithData, c.symCGDataProviderCreateWithData, unsafe.Pointer(&cgprovider.ptr), []unsafe.Pointer{
		unsafe.Pointer(info.Pointer()),
		unsafe.Pointer(data.Pointer()),
		unsafe.Pointer(&size),
		unsafe.Pointer(releaseDataCallback.Pointer()),
	})
	if err != nil {
		return cgDataProvider{objcAnyOpaqueNil}, c.errorf("failed to CGDataProviderCreateWithData: %w", err)
	}

	return cgprovider, nil
}

func (c *coreGraphics) ImageRelease(cgimage cgImage) error {
	err := ffi.CallFunction(cifCoreGraphicsImageRelease, c.symCGImageRelease, nil, []unsafe.Pointer{unsafe.Pointer(cgimage.Pointer())})
	if err != nil {
		return c.errorf("failed to CGImageRelease: %w", err)
	}

	return nil
}

func (c *coreGraphics) DataProviderRelease(cgprovider cgDataProvider) error {
	err := ffi.CallFunction(cifCoreGraphicsDataProviderRelease, c.symCGDataProviderRelease, nil, []unsafe.Pointer{unsafe.Pointer(cgprovider.Pointer())})
	if err != nil {
		return c.errorf("failed to CGDataProviderRelease: %w", err)
	}

	return nil
}

func (c *coreGraphics) ColorSpaceCreateDeviceRGB() (cgspace cgColorSpace, err error) {
	cgspace.typ = types.PointerTypeDescriptor
	cgspace.ptr = unsafe.Pointer(nil)

	err = ffi.CallFunction(cifCoreGraphicsColorSpaceCreateDeviceRGB, c.symCGColorSpaceCreateDeviceRGB, unsafe.Pointer(&cgspace.ptr), []unsafe.Pointer{})
	if err != nil {
		return cgColorSpace{objcAnyOpaqueNil}, c.errorf("failed to CGColorSpaceCreateDeviceRGB: %w", err)
	}

	return cgspace, nil
}

// [*objcReflect.errorf]
func (c *coreGraphics) errorf(f string, vals ...any) error {
	return fmt.Errorf("hal/software.coreGraphics: "+f, vals...)
}

const objcRuntimeLibraryLocation = "/usr/lib/libobjc.A.dylib"

type objcSEL struct{ objcAnyOpaque }

var (
	cifOBJCSelRegisterName = &types.CallInterface{
		ArgCount: 1,
		ArgTypes: []*types.TypeDescriptor{
			types.PointerTypeDescriptor,
		},
		ReturnType: types.PointerTypeDescriptor,
	}
)

type objcReflect struct {
	lib                unsafe.Pointer
	symMsgSend         unsafe.Pointer
	symMsgSendStRet    unsafe.Pointer
	symMsgSendFpRet    unsafe.Pointer
	symSelRegisterName unsafe.Pointer

	selCaches sync.Map
}

func (o *objcReflect) Open() (err error) {
	o.lib, err = ffi.LoadLibrary(objcRuntimeLibraryLocation)
	if err != nil {
		return o.errorf("failed to load libobjc: %w", err)
	}

	if o.symMsgSend, err = ffi.GetSymbol(o.lib, "objc_msgSend"); err != nil {
		return o.errorf("objc_msgSend not found: %w", err)
	}

	if o.symMsgSendFpRet, err = ffi.GetSymbol(o.lib, "objc_msgSend_fpret"); err != nil {
		o.symMsgSendFpRet = nil
	}

	if o.symMsgSendStRet, err = ffi.GetSymbol(o.lib, "objc_msgSend_stret"); err != nil {
		o.symMsgSendStRet = nil
	}

	if o.symSelRegisterName, err = ffi.GetSymbol(o.lib, "sel_registerName"); err != nil {
		return o.errorf("sel_registerName not found: %w", err)
	}

	return nil
}

func (o *objcReflect) MsgSend(self objcAnyOpaque, op objcAnyOpaque, ret objcAnyOpaque, args ...objcAnyOpaque) error {
	if self.Pointer() == nil || op.Pointer() == nil {
		return nil
	}

	argTypes := make([]*types.TypeDescriptor, 2+len(args))
	argTypes[0] = types.PointerTypeDescriptor
	argTypes[1] = types.PointerTypeDescriptor
	for i, arg := range args {
		argTypes[2+i] = arg.typ
	}

	cif := &types.CallInterface{}
	if err := ffi.PrepareCallInterface(cif, types.DefaultCall, ret.Type(), argTypes); err != nil {
		return err
	}

	argPtrs := make([]unsafe.Pointer, 2+len(args))
	argPtrs[0] = unsafe.Pointer(self.Pointer())
	argPtrs[1] = unsafe.Pointer(op.Pointer())
	for i, arg := range args {
		argPtrs[2+i] = unsafe.Pointer(arg.Pointer())
	}

	fn := o.objcMsgSendSymbol(ret.Type())
	err := ffi.CallFunction(cif, fn, unsafe.Pointer(ret.Pointer()), argPtrs)
	runtime.KeepAlive(args)

	return err
}

func (o *objcReflect) SelRegisterName(name string) (sel objcSEL, err error) {
	if cached, ok := o.selCaches.Load(name); ok {
		return cached.(objcSEL), nil
	}

	sel.typ = types.PointerTypeDescriptor
	sel.ptr = unsafe.Pointer(nil)

	cname := append([]byte(name), 0)
	selname := unsafe.SliceData(cname)

	err = ffi.CallFunction(cifOBJCSelRegisterName, o.symSelRegisterName, unsafe.Pointer(&sel.ptr), []unsafe.Pointer{
		unsafe.Pointer(&selname),
	})
	if err != nil {
		return objcSEL{objcAnyOpaqueNil}, err
	}

	if sel.ptr == nil {
		return objcSEL{objcAnyOpaqueNil}, o.errorf("failed to sel_registerName: name=%s, selname_ptr=%v, selname_str=%s", name, selname, unsafe.String((*byte)(selname), len(name)))
	}

	o.selCaches.Store(name, sel)

	return sel, nil
}

func (o *objcReflect) objcMsgSendSymbol(retType *types.TypeDescriptor) unsafe.Pointer {
	if retType != nil && retType.Kind == types.StructType && runtime.GOARCH == "amd64" {
		if o.symMsgSendStRet != nil && typeSize(retType) > 16 {
			return o.symMsgSendStRet
		}
	}

	if retType != nil && (retType.Kind == types.FloatType || retType.Kind == types.DoubleType) && runtime.GOARCH == "amd64" {
		if o.symMsgSendFpRet != nil {
			return o.symMsgSendFpRet
		}
	}

	return o.symMsgSend
}

// attach module name for traceability when it's paniced.
func (o *objcReflect) errorf(f string, vals ...any) error {
	return fmt.Errorf("hal/software.objcReflect: "+f, vals...)
}

type objcAnyOpaque struct {
	typ *types.TypeDescriptor
	ptr unsafe.Pointer
}

func (o objcAnyOpaque) Type() *types.TypeDescriptor {
	return o.typ
}

func (o objcAnyOpaque) Pointer() *unsafe.Pointer {
	return &o.ptr
}

func objcAnyOpaqueFromPointer(ptr unsafe.Pointer) objcAnyOpaque {
	return objcAnyOpaque{typ: types.PointerTypeDescriptor, ptr: ptr}
}

var objcAnyOpaqueNil = objcAnyOpaque{typ: types.PointerTypeDescriptor, ptr: unsafe.Pointer(nil)}

func typeSize(td *types.TypeDescriptor) uintptr {
	if td == nil {
		return 0
	}
	if td.Size != 0 {
		return td.Size
	}
	if td.Kind != types.StructType {
		return 0
	}
	var size uintptr
	var maxAlign uintptr
	for _, member := range td.Members {
		align := typeAlign(member)
		size = alignUp(size, align)
		size += typeSize(member)
		if align > maxAlign {
			maxAlign = align
		}
	}
	return alignUp(size, maxAlign)
}

func typeAlign(td *types.TypeDescriptor) uintptr {
	if td == nil {
		return 1
	}
	if td.Alignment != 0 {
		return td.Alignment
	}
	if td.Kind != types.StructType {
		return 1
	}
	var maxAlign uintptr
	for _, member := range td.Members {
		if align := typeAlign(member); align > maxAlign {
			maxAlign = align
		}
	}
	if maxAlign == 0 {
		return 1
	}
	return maxAlign
}

func alignUp(val, align uintptr) uintptr {
	if align == 0 {
		return val
	}
	rem := val % align
	if rem == 0 {
		return val
	}
	return val + (align - rem)
}

// --- END OBJC BINDING ---
