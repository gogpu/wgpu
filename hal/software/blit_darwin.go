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

func init() {
	slog.SetLogLoggerLevel(slog.LevelDebug)
}

// platformBlit is a no-op on platforms without native blit support.
// Windows has GDI (blit_windows.go), Linux has X11 (blit_linux.go).
type platformBlit struct {
	once           sync.Once
	objc           *objcReflect
	cg             *coreGraphics
	colorSpace     cgColorSpace
	mtl            *metal
	mtlDevice      mtlDevice
	mtlQueue       mtlCommandQueue
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

	p.mtl = new(metal)
	if err := p.mtl.Open(p.objc); err != nil {
		p.mtl = nil
	}

	if p.mtl != nil {
		p.mtlDevice, err = p.mtl.CreateSystemDefaultDevice()
		if err != nil {
			return err
		}

		p.mtlQueue, err = p.mtlDevice.NewCommandQueue(p.objc)
		if err != nil {
			return err
		}
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

	objc, cg, colorSpace, mtl := s.platformBlit.objc, s.platformBlit.cg, s.platformBlit.colorSpace, s.platformBlit.mtl
	l := caLayer{objcAnyOpaqueFromPointer(unsafe.Pointer(s.hwnd))}

	// true when s.hwnd is CAMetalLayer
	isMetalNeeded := false

	if mtl != nil {
		isMetalNeeded = isCAMetalLayer(objc, l)
	}

	if isMetalNeeded {
		// CAMetalLayer does not support `setContents:`
		// slog.Warn("software: blitFrameBufferToWindow does not support CAMetalLayer. (Did you try using metal backend?)")

		device, queue := s.platformBlit.mtlDevice, s.platformBlit.mtlQueue

		ml := caMetalLayer{l}

		d, err := ml.Device(objc)
		if err != nil {
			slog.Debug("software: failed to get device", slog.Any("error", err))
		} else if d.IsSamePointer(&objcAnyOpaqueNil) {
			d = device
			ml.SetDevice(objc, d)
		}

		col, err := ml.ColorSpace(objc)
		if err != nil {
			slog.Debug("software: failed to get colorspace", slog.Any("error", err))
		} else if col.IsSamePointer(&objcAnyOpaqueNil) {
			if err := ml.SetColorSpace(objc, colorSpace); err != nil {
				slog.Debug("software: failed to set colorspace", slog.Any("error", err))
			}
		}

		blitFrameBufferToWindowMetal(objc, queue, ml, uint64(width), uint64(height), data, 4*uint64(width))
	} else {
		img, release := createCGImageByFramebuffer(cg, colorSpace, data, width, height)
		defer release()

		blitFrameBufferToWindowCALayer(objc, l, img)
	}
}

// blitDamageRectsToWindow is a no-op on unsupported platforms.
func (s *Surface) blitDamageRectsToWindow(src []byte, w, h int32, rects []image.Rectangle) {
	if s.hwnd == 0 || len(rects) == 0 {
		return
	}

	if !s.platformBlit.onceInit() {
		return
	}

	objc, cg, colorSpace, mtl := s.platformBlit.objc, s.platformBlit.cg, s.platformBlit.colorSpace, s.platformBlit.mtl
	l := caLayer{objcAnyOpaqueFromPointer(unsafe.Pointer(s.hwnd))}

	isMetalNeeded := false
	if mtl != nil {
		isMetalNeeded = isCAMetalLayer(objc, l)
	}

	if isMetalNeeded {

	} else {
		img, release := createCGImageByFramebuffer(cg, colorSpace, src, w, h)
		defer release()

		blitDamageRectsToWindowCALayer(objc, l, img, w, h, rects)
	}
}

func createCGImageByFramebuffer(cg *coreGraphics, colorSpace cgColorSpace, data []byte, width, height int32) (img cgImage, release func()) {
	dataProvider, err := cg.DataProviderCreateWithData(objcAnyOpaqueNil, objcAnyOpaqueFromPointer(unsafe.Pointer(unsafe.SliceData(data))), uintptr(len(data)), objcAnyOpaqueNil)
	if err != nil {
		slog.Debug("software: failed to create CGDataProvider", slog.Any("error", err))
		return
	}

	binfo := cgBitmapInfoByteOrder32Little.
		WithImageAlphaInfo(cgImageAlphaPremultipliedFirst)

	img, err = cg.ImageCreate(uintptr(width), uintptr(height), 8, 32, 4*uintptr(width), colorSpace, binfo, dataProvider, objcAnyOpaqueNil, false, cgColorRenderingIntentDefault)
	if err != nil {
		cg.DataProviderRelease(dataProvider)

		slog.Debug("software: failed to create CGImage", slog.Any("error", err))
		return
	}

	release = func() {
		defer cg.DataProviderRelease(dataProvider)
		defer cg.ImageRelease(img)
	}

	return img, release
}

func blitDamageRectsToWindowCALayer(objc *objcReflect, l caLayer, img cgImage, w, h int32, rects []image.Rectangle) {
	if err := l.SetContents(objc, img.objcAnyOpaque); err != nil {
		slog.Debug("software: failed to [caLayer setContents: img]", slog.Any("error", err), slog.Any("img", img))
		return
	}

	bounds := image.Rect(0, 0, int(w), int(h))
	for _, rect := range rects {
		r := bounds.Intersect(rect)
		if r.Empty() {
			continue
		}

		cgr := cgRect{
			Origin: cgPoint{X: float64(r.Min.X), Y: float64(r.Min.Y)},
			Size: cgSize{
				Width:  float64(r.Dx()),
				Height: float64(r.Dy()),
			},
		}

		err := l.SetNeedsDisplayInRect(objc, cgr)
		if err != nil {
			slog.Debug("software: failed to [caLayer setNeedsDisplayInRect:cgRect]", slog.Any("error", err), slog.Any("cgRect", cgr))
			continue
		}
	}
}

func isCAMetalLayer(objc *objcReflect, l caLayer) bool {
	clsCAMetalLayer, err := objc.LoopUpClass("CAMetalLayer")
	if err != nil {
		slog.Debug("software: failed to get CAMetalLayer class", slog.Any("error", err))
		return false
	}

	lClass, err := objc.GetClass(l.objcAnyOpaque)
	if err != nil {
		slog.Debug("software: failed to get object class", slog.Any("error", err))
		return false
	}

	return lClass.IsSamePointer(&clsCAMetalLayer)
}

func blitFrameBufferToWindowCALayer(objc *objcReflect, layer caLayer, img cgImage) {
	if err := layer.SetContents(objc, img.objcAnyOpaque); err != nil {
		slog.Debug("software: failed to [caLayer setContents: img]", slog.Any("error", err), slog.Any("img", img))
		return
	}

	if err := layer.SetNeedsDisplay(objc); err != nil {
		slog.Debug("software: failed to set NeedsDisplay", slog.Any("error", err))
		return
	}
}

func blitFrameBufferToWindowMetal(objc *objcReflect, queue mtlCommandQueue, layer caMetalLayer, w, h uint64, data []byte, bytesPerRow uint64) {
	if err := layer.SetDrawableSize(objc, cgSize{
		Width:  float64(w),
		Height: float64(h),
	}); err != nil {
		slog.Debug("software: failed to [caMetalLayer setDrawableSize:size]", slog.Any("error", err))
		return
	}

	dr, err := layer.NextDrawable(objc)
	if err != nil || dr.IsSamePointer(&objcAnyOpaqueNil) {
		slog.Debug("software: failed to [caMetalLayer nextDrawable]", slog.Any("error", err), slog.Any("dr", dr))
		return
	}

	tex, err := dr.Texture(objc)
	if err != nil || tex.IsSamePointer(&objcAnyOpaqueNil) {
		slog.Debug("software: failed to [caMetalDrawable texture]", slog.Any("error", err))
		return
	}

	region := metalRegion{
		Origin: metalOrigin{
			X: 0,
			Y: 0,
			Z: 0,
		},
		Size: metalSize{
			Width:  w,
			Height: h,
			Depth:  1,
		},
	}

	err = tex.ReplaceRegion_MipmapLevel_WithBytes_BytesPerRow(objc, region, 0, data, bytesPerRow)
	if err != nil {
		slog.Debug("software: failed to [mtlTexture replaceRegion:mipmapLevel:withBytes:bytesPerRow:]", slog.Any("region", region), slog.Any("bytesPerRow", bytesPerRow))
		return
	}

	buf, err := queue.CommandBuffer(objc)
	if err != nil {
		slog.Debug("software: failed to [mtlQueue commandBuffer]", slog.Any("error", err))
		return
	}

	if err := buf.PresentDrawable(objc, dr); err != nil {
		slog.Debug("software: failed to [mtlCommandBuffer presentDrawable:nextDrawable]", slog.Any("error", err), slog.Any("nextDrawable", dr))
		return
	}

	if err := buf.Commit(objc); err != nil {
		slog.Debug("software: failed to [mtlCommandBufer commit]", slog.Any("error", err))
		return
	}

	if err := layer.SetNeedsDisplay(objc); err != nil {
		slog.Debug("software: failed set needsDisplay", slog.Any("error", err))
		return
	}
}

// --- BEGIN OBJC BINDING ---

type mtlCommandQueue struct{ objcAnyOpaque }

func (m *mtlCommandQueue) CommandBuffer(objc *objcReflect) (b mtlCommandBuffer, err error) {
	sel, err := objc.SelRegisterName("commandBuffer")
	if err != nil {
		return mtlCommandBuffer{objcAnyOpaqueNil}, err
	}

	ret := objcAnyOpaqueNil
	if err := objc.MsgSend(m.objcAnyOpaque, sel.objcAnyOpaque, &ret); err != nil {
		return mtlCommandBuffer{objcAnyOpaqueNil}, err
	}

	return mtlCommandBuffer{ret}, nil
}

type mtlCommandBuffer struct{ objcAnyOpaque }

func (m *mtlCommandBuffer) PresentDrawable(objc *objcReflect, dr caMTLDrawable) error {
	sel, err := objc.SelRegisterName("presentDrawable:")
	if err != nil {
		return err
	}

	ret := objcAnyOpaqueNil
	if err := objc.MsgSend(m.objcAnyOpaque, sel.objcAnyOpaque, &ret, &dr); err != nil {
		return err
	}

	return nil
}

func (m *mtlCommandBuffer) Commit(objc *objcReflect) error {
	sel, err := objc.SelRegisterName("commit")
	if err != nil {
		return err
	}

	ret := objcAnyOpaqueNil
	if err := objc.MsgSend(m.objcAnyOpaque, sel.objcAnyOpaque, &ret); err != nil {
		return err
	}

	return nil
}

type mtlDevice struct{ objcAnyOpaque }

func (m *mtlDevice) NewCommandQueue(objc *objcReflect) (b mtlCommandQueue, err error) {
	sel, err := objc.SelRegisterName("newCommandQueue")
	if err != nil {
		return mtlCommandQueue{objcAnyOpaqueNil}, err
	}

	ret := objcAnyOpaqueNil
	if err := objc.MsgSend(m.objcAnyOpaque, sel.objcAnyOpaque, &ret); err != nil {
		return mtlCommandQueue{objcAnyOpaqueNil}, err
	}

	return mtlCommandQueue{ret}, nil
}

type caMetalLayer struct{ caLayer }

func (c *caMetalLayer) PixelFormat(objc *objcReflect) (pixfmt uint64, err error) {
	sel, err := objc.SelRegisterName("pixelFormat")
	if err != nil {
		return 0, err
	}

	ret := objcBoxedWith(uint64(0), types.UInt64TypeDescriptor)
	if err := objc.MsgSend(c.objcAnyOpaque, sel.objcAnyOpaque, ret); err != nil {
		return 0, err
	}

	return ret.data, nil
}

func (c *caMetalLayer) SetPixelFormat(objc *objcReflect, pixfmt uint64) error {
	sel, err := objc.SelRegisterName("setPixelFormat:")
	if err != nil {
		return err
	}

	ret := objcAnyOpaqueNil
	if err := objc.MsgSend(c.objcAnyOpaque, sel.objcAnyOpaque, &ret, objcBoxedWith(pixfmt, types.UInt64TypeDescriptor)); err != nil {
		return err
	}

	return nil
}

func (c *caMetalLayer) DrawableSize(objc *objcReflect) (sz cgSize, err error) {
	sel, err := objc.SelRegisterName("drawableSize")
	if err != nil {
		return sz, err
	}

	ret := objcBoxedWith(sz, cgSizeTypeDescriptor)
	if err := objc.MsgSend(c.objcAnyOpaque, sel.objcAnyOpaque, ret); err != nil {
		return sz, err
	}

	return ret.data, nil
}

func (c *caMetalLayer) SetDrawableSize(objc *objcReflect, sz cgSize) error {
	sel, err := objc.SelRegisterName("setDrawableSize:")
	if err != nil {
		return err
	}

	ret := objcAnyOpaqueNil
	if err := objc.MsgSend(c.objcAnyOpaque, sel.objcAnyOpaque, &ret, objcBoxedWith(sz, cgSizeTypeDescriptor)); err != nil {
		return err
	}

	return nil
}

func (c *caMetalLayer) Device(objc *objcReflect) (d mtlDevice, err error) {
	sel, err := objc.SelRegisterName("device")
	if err != nil {
		return mtlDevice{objcAnyOpaqueNil}, err
	}

	d.objcAnyOpaque = objcAnyOpaqueNil

	if err := objc.MsgSend(c.objcAnyOpaque, sel.objcAnyOpaque, &d); err != nil {
		return mtlDevice{objcAnyOpaqueNil}, err
	}

	return d, nil
}

func (c *caMetalLayer) SetDevice(objc *objcReflect, device mtlDevice) error {
	sel, err := objc.SelRegisterName("setDevice:")
	if err != nil {
		return err
	}

	ret := objcAnyOpaqueNil
	if err := objc.MsgSend(c.objcAnyOpaque, sel.objcAnyOpaque, &ret, &device); err != nil {
		return err
	}

	return nil
}

func (c *caMetalLayer) ColorSpace(objc *objcReflect) (col cgColorSpace, err error) {
	sel, err := objc.SelRegisterName("colorspace")
	if err != nil {
		return cgColorSpace{objcAnyOpaqueNil}, err
	}

	col.objcAnyOpaque = objcAnyOpaqueNil
	if err := objc.MsgSend(c.objcAnyOpaque, sel.objcAnyOpaque, &col); err != nil {
		return cgColorSpace{objcAnyOpaqueNil}, err
	}

	return col, nil
}

func (c *caMetalLayer) SetColorSpace(objc *objcReflect, col cgColorSpace) error {
	sel, err := objc.SelRegisterName("setColorspace:")
	if err != nil {
		return err
	}

	ret := objcAnyOpaqueNil
	if err := objc.MsgSend(c.objcAnyOpaque, sel.objcAnyOpaque, &ret, &col.objcAnyOpaque); err != nil {
		return err
	}

	return nil
}

func (c *caMetalLayer) NextDrawable(objc *objcReflect) (dr caMTLDrawable, err error) {
	sel, err := objc.SelRegisterName("nextDrawable")
	if err != nil {
		return caMTLDrawable{objcAnyOpaqueNil}, err
	}

	dr.objcAnyOpaque = objcAnyOpaqueNil

	if err := objc.MsgSend(c.objcAnyOpaque, sel.objcAnyOpaque, &dr); err != nil {
		return caMTLDrawable{objcAnyOpaqueNil}, err
	}

	return dr, nil
}

type caMTLDrawable struct{ objcAnyOpaque }

func (c *caMTLDrawable) Texture(objc *objcReflect) (tex mtlTexture, err error) {
	sel, err := objc.SelRegisterName("texture")
	if err != nil {
		return mtlTexture{objcAnyOpaqueNil}, err
	}

	tex.objcAnyOpaque = objcAnyOpaqueNil

	if err := objc.MsgSend(c.objcAnyOpaque, sel.objcAnyOpaque, &tex.objcAnyOpaque); err != nil {
		return mtlTexture{objcAnyOpaqueNil}, err
	}

	return tex, nil
}

type mtlTexture struct{ objcAnyOpaque }

// ref: https://developer.apple.com/documentation/metal/mtltexture/replace(region:mipmaplevel:withbytes:bytesperrow:)?language=objc
func (m *mtlTexture) ReplaceRegion_MipmapLevel_WithBytes_BytesPerRow(
	objc *objcReflect,
	region metalRegion,
	mipmapLevel uint64,
	withBytes []byte,
	bytesPerRow uint64,
) error {
	sel, err := objc.SelRegisterName("replaceRegion:mipmapLevel:withBytes:bytesPerRow:")
	if err != nil {
		return err
	}

	err = objc.MsgSend(m.objcAnyOpaque, sel.objcAnyOpaque, &objcAnyOpaqueNil,
		objcBoxedWith(region, metalRegionType),
		objcBoxedWith(mipmapLevel, types.UInt64TypeDescriptor),
		objcBoxedWith(unsafe.Pointer(unsafe.SliceData(withBytes)), types.PointerTypeDescriptor),
		objcBoxedWith(bytesPerRow, types.UInt64TypeDescriptor),
	)

	if err != nil {
		return err
	}

	return nil
}

func (m *mtlTexture) Width(objc *objcReflect) (w uint64, err error) {
	sel, err := objc.SelRegisterName("width")
	if err != nil {
		return 0, err
	}

	ret := objcBoxedWith(0, types.PointerTypeDescriptor)
	err = objc.MsgSend(m.objcAnyOpaque, sel.objcAnyOpaque, ret)
	if err != nil {
		return 0, err
	}

	slog.Debug("software: width ret", slog.Any("ret", ret))

	return uint64(ret.data), nil
}

func (m *mtlTexture) Height(objc *objcReflect) (h uint64, err error) {
	sel, err := objc.SelRegisterName("height")
	if err != nil {
		return 0, err
	}

	ret := objcBoxedWith(0, types.PointerTypeDescriptor)
	err = objc.MsgSend(m.objcAnyOpaque, sel.objcAnyOpaque, ret)
	if err != nil {
		return 0, err
	}

	slog.Debug("software: width ret", slog.Any("ret", ret))

	return uint64(ret.data), nil
}

type metalRegion struct {
	Origin metalOrigin
	Size   metalSize
}

type metalOrigin struct {
	X, Y, Z uint64
}

type metalSize struct {
	Width, Height, Depth uint64
}

var (
	metalOriginType = &types.TypeDescriptor{
		Kind: types.StructType,
		Members: []*types.TypeDescriptor{
			types.UInt64TypeDescriptor,
			types.UInt64TypeDescriptor,
			types.UInt64TypeDescriptor,
		},
	}
	metalSizeType = &types.TypeDescriptor{
		Kind: types.StructType,
		Members: []*types.TypeDescriptor{
			types.UInt64TypeDescriptor,
			types.UInt64TypeDescriptor,
			types.UInt64TypeDescriptor,
		},
	}
	metalRegionType = &types.TypeDescriptor{
		Kind: types.StructType,
		Members: []*types.TypeDescriptor{
			metalOriginType,
			metalSizeType,
		},
	}
	cifMetalRegionMake2D = &types.CallInterface{
		ArgCount: 4,
		ArgTypes: []*types.TypeDescriptor{
			types.UInt64TypeDescriptor,
			types.UInt64TypeDescriptor,
			types.UInt64TypeDescriptor,
			types.UInt64TypeDescriptor,
		},
		ReturnType: metalRegionType,
	}
	cifMTLCreateSystemDefaultDevice = types.CallInterface{
		ArgCount:   0,
		ReturnType: types.PointerTypeDescriptor,
	}
)

const metalLibraryLocation = "/System/Library/Frameworks/Metal.framework/Metal"

type metal struct {
	lib unsafe.Pointer
	// symMTLRegionMake2D              unsafe.Pointer
	symMTLCreateSystemDefaultDevice unsafe.Pointer

	objc *objcReflect
}

func (m *metal) Open(objc *objcReflect) (err error) {
	m.objc = objc

	if m.lib, err = ffi.LoadLibrary(metalLibraryLocation); err != nil {
		return m.errorf("failed to load metal library: %w", err)
	}

	// if m.symMTLRegionMake2D, err = ffi.GetSymbol(m.lib, "MTLRegionMake2D"); err != nil {
	// 	return m.errorf("MTLRegionMake2D is not found: %w", err)
	// }

	if m.symMTLCreateSystemDefaultDevice, err = ffi.GetSymbol(m.lib, "MTLCreateSystemDefaultDevice"); err != nil {
		return m.errorf("MTLCreateSystemDefaultDevice is not found: %w", err)
	}

	return nil
}

// func (m *metal) RegionMake2D(x, y, w, h uint64) (region metalRegion, err error) {
// 	err = ffi.CallFunction(cifMetalRegionMake2D, m.symMTLRegionMake2D, unsafe.Pointer(&region), []unsafe.Pointer{
// 		unsafe.Pointer(&x),
// 		unsafe.Pointer(&y),
// 		unsafe.Pointer(&w),
// 		unsafe.Pointer(&h),
// 	})
// 	if err != nil {
// 		return region, m.errorf("failed to MTLRegionMake2D: %w", err)
// 	}

// 	return region, nil
// }

func (m *metal) CreateSystemDefaultDevice() (d mtlDevice, err error) {
	d.objcAnyOpaque = objcAnyOpaqueNil

	err = ffi.CallFunction(&cifMTLCreateSystemDefaultDevice, m.symMTLCreateSystemDefaultDevice, unsafe.Pointer(&d.data), []unsafe.Pointer{})
	if err != nil {
		return mtlDevice{objcAnyOpaqueNil}, m.errorf("failed to MTLRegionMake2D: %w", err)
	}

	return d, nil
}

func (m *metal) errorf(f string, args ...any) error {
	return fmt.Errorf("software.metal: "+f, args...)
}

type caLayer struct{ objcAnyOpaque }

func (c *caLayer) SetNeedsDisplayInRect(objc *objcReflect, rect cgRect) error {
	sel, err := objc.SelRegisterName("setNeedsDisplayInRect:")
	if err != nil {
		return err
	}

	ret := objcAnyOpaqueNil
	src := objcBoxedWith(rect, cgRectTypeDescriptor)
	if err := objc.MsgSend(c.objcAnyOpaque, sel.objcAnyOpaque, &ret, src); err != nil {
		return err
	}

	return nil
}

// ref: https://developer.apple.com/documentation/quartzcore/calayer/setneedsdisplay()?language=objc
func (c *caLayer) SetNeedsDisplay(objc *objcReflect) error {
	sel, err := objc.SelRegisterName("setNeedsDisplay")
	if err != nil {
		return err
	}

	ret := objcAnyOpaqueNil
	if err := objc.MsgSend(c.objcAnyOpaque, sel.objcAnyOpaque, &ret); err != nil {
		return err
	}

	return nil
}

func (c *caLayer) SetContents(objc *objcReflect, obj objcAnyOpaque) error {
	sel, err := objc.SelRegisterName("setContents:")
	if err != nil {
		return err
	}

	ret := objcAnyOpaqueNil
	if err := objc.MsgSend(c.objcAnyOpaque, sel.objcAnyOpaque, &ret, &obj); err != nil {
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
type cgSize struct {
	Width  float64
	Height float64
}

type cgRect struct {
	Origin cgPoint
	Size   cgSize
}

type cgPoint struct {
	X, Y float64
}

var (
	cgSizeTypeDescriptor = &types.TypeDescriptor{
		Kind: types.StructType,
		Members: []*types.TypeDescriptor{
			types.DoubleTypeDescriptor,
			types.DoubleTypeDescriptor,
		},
	}
	cgPointTypeDescriptor = &types.TypeDescriptor{
		Kind: types.StructType,
		Members: []*types.TypeDescriptor{
			types.DoubleTypeDescriptor,
			types.DoubleTypeDescriptor,
		},
	}
	cgRectTypeDescriptor = &types.TypeDescriptor{
		Kind: types.StructType,
		Members: []*types.TypeDescriptor{
			cgPointTypeDescriptor,
			cgSizeTypeDescriptor,
		},
	}
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
	// CGImageRef: (struct CGImage)*
	cgimg.objcAnyOpaque = objcAnyOpaqueNil

	err = ffi.CallFunction(cifCoreGraphicsImageCreate, c.symCGImageCreate, unsafe.Pointer(cgimg.Pointer()), []unsafe.Pointer{
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
	// CGDataProviderRef: (struct CGDataProvider)*
	cgprovider.objcAnyOpaque = objcAnyOpaqueNil

	err = ffi.CallFunction(cifCoreGraphicsDataProviderCreateWithData, c.symCGDataProviderCreateWithData, unsafe.Pointer(cgprovider.Pointer()), []unsafe.Pointer{
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
	cgspace.objcAnyOpaque = objcAnyOpaqueNil

	err = ffi.CallFunction(cifCoreGraphicsColorSpaceCreateDeviceRGB, c.symCGColorSpaceCreateDeviceRGB, unsafe.Pointer(&cgspace.data), []unsafe.Pointer{})
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
	cifOBJCLookUpClass = cifOBJCSelRegisterName
	cifOBJCGetClass    = cifOBJCSelRegisterName
)

type objcReflect struct {
	lib                unsafe.Pointer
	symMsgSend         unsafe.Pointer
	symMsgSendStRet    unsafe.Pointer
	symMsgSendFpRet    unsafe.Pointer
	symSelRegisterName unsafe.Pointer
	symOBJCLookUpClass unsafe.Pointer
	symObjectGetClass  unsafe.Pointer

	selCaches sync.Map
	clsCaches sync.Map
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

	if o.symOBJCLookUpClass, err = ffi.GetSymbol(o.lib, "objc_lookUpClass"); err != nil {
		return o.errorf("objc_lookUpClass not found: %w", err)
	}

	if o.symObjectGetClass, err = ffi.GetSymbol(o.lib, "object_getClass"); err != nil {
		return o.errorf("object_getClass not found: %w", err)
	}

	return nil
}

func (o *objcReflect) MsgSend(self objcAnyOpaque, op objcAnyOpaque, ret objcPointer, args ...objcPointer) error {
	if self.Pointer() == nil || op.Pointer() == nil {
		return nil
	}

	argTypes := make([]*types.TypeDescriptor, 2+len(args))
	argTypes[0] = types.PointerTypeDescriptor
	argTypes[1] = types.PointerTypeDescriptor
	for i, arg := range args {
		argTypes[2+i] = arg.Type()
	}

	cif := &types.CallInterface{}
	if err := ffi.PrepareCallInterface(cif, types.DefaultCall, ret.Type(), argTypes); err != nil {
		return err
	}

	argPtrs := make([]unsafe.Pointer, 2+len(args))
	argPtrs[0] = self.Pointer()
	argPtrs[1] = op.Pointer()
	for i, arg := range args {
		argPtrs[2+i] = arg.Pointer()
	}

	fn := o.objcMsgSendSymbol(ret.Type())
	err := ffi.CallFunction(cif, fn, ret.Pointer(), argPtrs)
	runtime.KeepAlive(args)

	return err
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

func (o *objcReflect) SelRegisterName(name string) (sel objcSEL, err error) {
	if cached, ok := o.selCaches.Load(name); ok {
		return cached.(objcSEL), nil
	}

	sel.objcAnyOpaque = objcAnyOpaqueNil

	cname := append([]byte(name), 0)
	selname := unsafe.SliceData(cname)

	err = ffi.CallFunction(cifOBJCSelRegisterName, o.symSelRegisterName, unsafe.Pointer(&sel.data), []unsafe.Pointer{
		unsafe.Pointer(&selname),
	})
	if err != nil {
		return objcSEL{objcAnyOpaqueNil}, err
	}

	if sel.IsSamePointer(&objcAnyOpaqueNil) {
		return objcSEL{objcAnyOpaqueNil}, o.errorf("failed to sel_registerName: name=%s, selname_ptr=%v, selname_str=%s", name, selname, unsafe.String((*byte)(selname), len(name)))
	}

	o.selCaches.Store(name, sel)

	return sel, nil
}

func (o *objcReflect) LoopUpClass(name string) (cls objcAnyOpaque, err error) {
	if cached, ok := o.clsCaches.Load(name); ok {
		return cached.(objcAnyOpaque), nil
	}

	cls = objcAnyOpaqueNil

	cname := append([]byte(name), 0)
	selname := unsafe.SliceData(cname)

	err = ffi.CallFunction(cifOBJCLookUpClass, o.symOBJCLookUpClass, unsafe.Pointer(&cls.data), []unsafe.Pointer{
		unsafe.Pointer(&selname),
	})
	if err != nil {
		return objcAnyOpaqueNil, err
	}

	if cls.data == nil {
		return objcAnyOpaqueNil, o.errorf("failed to objc_lookUpClass: name=%s, selname_ptr=%v, selname_str=%s", name, selname, unsafe.String((*byte)(selname), len(name)))
	}

	o.clsCaches.Store(name, cls)

	return cls, nil
}

func (o *objcReflect) GetClass(obj objcAnyOpaque) (cls objcAnyOpaque, err error) {
	cls = objcAnyOpaqueNil

	err = ffi.CallFunction(cifOBJCGetClass, o.symObjectGetClass, unsafe.Pointer(&cls.data), []unsafe.Pointer{
		unsafe.Pointer(obj.Pointer()),
	})
	if err != nil {
		return objcAnyOpaqueNil, err
	}

	return cls, nil
}

// attach module name for traceability when it's paniced.
func (o *objcReflect) errorf(f string, vals ...any) error {
	return fmt.Errorf("hal/software.objcReflect: "+f, vals...)
}

type objcPointer interface {
	Type() *types.TypeDescriptor
	Pointer() unsafe.Pointer
	IsSamePointer(objcPointer) bool
}

type objcAnyOpaque = objcBoxed[unsafe.Pointer]

type objcBoxed[T any] struct {
	typ  *types.TypeDescriptor
	data T
}

func (o *objcBoxed[T]) Type() *types.TypeDescriptor {
	return o.typ
}

func (o *objcBoxed[T]) Pointer() unsafe.Pointer {
	return unsafe.Pointer(&o.data)
}

func (o *objcBoxed[T]) IsSamePointer(y objcPointer) bool {
	switch y := y.(type) {
	case *objcBoxed[T]:
		return any(o.data) == any(y.data)

	default:
		return o.Pointer() == y.Pointer()
	}
}

func objcAnyOpaqueFromPointer(ptr unsafe.Pointer) objcAnyOpaque {
	return *objcBoxedWith(ptr, types.PointerTypeDescriptor)
}

func objcBoxedWith[T any](data T, typ *types.TypeDescriptor) *objcBoxed[T] {
	return &objcBoxed[T]{typ, data}
}

var objcAnyOpaqueNil = objcAnyOpaqueFromPointer(unsafe.Pointer(nil))

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
