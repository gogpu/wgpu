// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin

package metal

import (
	"fmt"
	"runtime"
	"sync"
	"unsafe"

	"github.com/go-webgpu/goffi/ffi"
	"github.com/go-webgpu/goffi/types"
)

// Objective-C runtime library handle and function symbols.
var (
	objcLib unsafe.Pointer

	symObjcMsgSend      unsafe.Pointer
	symObjcMsgSendFpret unsafe.Pointer
	symObjcMsgSendStret unsafe.Pointer
	symObjcGetClass     unsafe.Pointer
	symSelRegisterName  unsafe.Pointer

	cifGetClass    types.CallInterface
	cifSelRegister types.CallInterface
)

// selectorCache caches registered selectors for performance.
var selectorCache sync.Map

type objcArg struct {
	typ       *types.TypeDescriptor
	ptr       unsafe.Pointer
	keepAlive any
}

var (
	cgSizeType = &types.TypeDescriptor{
		Kind: types.StructType,
		Members: []*types.TypeDescriptor{
			types.DoubleTypeDescriptor,
			types.DoubleTypeDescriptor,
		},
	}
	mtlClearColorType = &types.TypeDescriptor{
		Kind: types.StructType,
		Members: []*types.TypeDescriptor{
			types.DoubleTypeDescriptor,
			types.DoubleTypeDescriptor,
			types.DoubleTypeDescriptor,
			types.DoubleTypeDescriptor,
		},
	}
	mtlViewportType = &types.TypeDescriptor{
		Kind: types.StructType,
		Members: []*types.TypeDescriptor{
			types.DoubleTypeDescriptor,
			types.DoubleTypeDescriptor,
			types.DoubleTypeDescriptor,
			types.DoubleTypeDescriptor,
			types.DoubleTypeDescriptor,
			types.DoubleTypeDescriptor,
		},
	}
	mtlScissorRectType = &types.TypeDescriptor{
		Kind: types.StructType,
		Members: []*types.TypeDescriptor{
			types.UInt64TypeDescriptor,
			types.UInt64TypeDescriptor,
			types.UInt64TypeDescriptor,
			types.UInt64TypeDescriptor,
		},
	}
	mtlOriginType = &types.TypeDescriptor{
		Kind: types.StructType,
		Members: []*types.TypeDescriptor{
			types.UInt64TypeDescriptor,
			types.UInt64TypeDescriptor,
			types.UInt64TypeDescriptor,
		},
	}
	mtlSizeType = &types.TypeDescriptor{
		Kind: types.StructType,
		Members: []*types.TypeDescriptor{
			types.UInt64TypeDescriptor,
			types.UInt64TypeDescriptor,
			types.UInt64TypeDescriptor,
		},
	}
	nsRangeType = &types.TypeDescriptor{
		Kind: types.StructType,
		Members: []*types.TypeDescriptor{
			types.UInt64TypeDescriptor,
			types.UInt64TypeDescriptor,
		},
	}
)

// initObjCRuntime initializes the Objective-C runtime.
func initObjCRuntime() error {
	var err error

	objcLib, err = ffi.LoadLibrary("/usr/lib/libobjc.A.dylib")
	if err != nil {
		return fmt.Errorf("metal: failed to load libobjc: %w", err)
	}

	if symObjcMsgSend, err = ffi.GetSymbol(objcLib, "objc_msgSend"); err != nil {
		return fmt.Errorf("metal: objc_msgSend not found: %w", err)
	}
	if symObjcMsgSendFpret, err = ffi.GetSymbol(objcLib, "objc_msgSend_fpret"); err != nil {
		symObjcMsgSendFpret = nil
	}
	if symObjcMsgSendStret, err = ffi.GetSymbol(objcLib, "objc_msgSend_stret"); err != nil {
		symObjcMsgSendStret = nil
	}
	if symObjcGetClass, err = ffi.GetSymbol(objcLib, "objc_getClass"); err != nil {
		return fmt.Errorf("metal: objc_getClass not found: %w", err)
	}
	if symSelRegisterName, err = ffi.GetSymbol(objcLib, "sel_registerName"); err != nil {
		return fmt.Errorf("metal: sel_registerName not found: %w", err)
	}

	return prepareObjCCallInterfaces()
}

func prepareObjCCallInterfaces() error {
	var err error

	err = ffi.PrepareCallInterface(&cifGetClass, types.DefaultCall,
		types.PointerTypeDescriptor,
		[]*types.TypeDescriptor{types.PointerTypeDescriptor})
	if err != nil {
		return fmt.Errorf("metal: failed to prepare objc_getClass: %w", err)
	}

	err = ffi.PrepareCallInterface(&cifSelRegister, types.DefaultCall,
		types.PointerTypeDescriptor,
		[]*types.TypeDescriptor{types.PointerTypeDescriptor})
	if err != nil {
		return fmt.Errorf("metal: failed to prepare sel_registerName: %w", err)
	}

	return nil
}

// GetClass returns the Class for a given name.
func GetClass(name string) Class {
	cname := append([]byte(name), 0)
	// goffi API requires pointer TO pointer value (avalue is slice of pointers to argument values)
	ptr := uintptr(unsafe.Pointer(&cname[0]))
	var result Class
	args := [1]unsafe.Pointer{unsafe.Pointer(&ptr)}
	_ = ffi.CallFunction(&cifGetClass, symObjcGetClass, unsafe.Pointer(&result), args[:])
	return result
}

// RegisterSelector registers and returns a selector for the given name.
func RegisterSelector(name string) SEL {
	if cached, ok := selectorCache.Load(name); ok {
		return cached.(SEL)
	}

	cname := append([]byte(name), 0)
	// goffi API requires pointer TO pointer value (avalue is slice of pointers to argument values)
	ptr := uintptr(unsafe.Pointer(&cname[0]))
	var result SEL
	args := [1]unsafe.Pointer{unsafe.Pointer(&ptr)}
	_ = ffi.CallFunction(&cifSelRegister, symSelRegisterName, unsafe.Pointer(&result), args[:])

	selectorCache.Store(name, result)
	return result
}

// Sel is a convenience alias for RegisterSelector.
func Sel(name string) SEL {
	return RegisterSelector(name)
}

func argPointer(val uintptr) objcArg {
	v := val
	return objcArg{typ: types.PointerTypeDescriptor, ptr: unsafe.Pointer(&v), keepAlive: &v}
}

func argUint64(val uint64) objcArg {
	v := val
	return objcArg{typ: types.UInt64TypeDescriptor, ptr: unsafe.Pointer(&v), keepAlive: &v}
}

func argInt64(val int64) objcArg {
	v := val
	return objcArg{typ: types.SInt64TypeDescriptor, ptr: unsafe.Pointer(&v), keepAlive: &v}
}

func argBool(val bool) objcArg {
	var v uint8
	if val {
		v = 1
	}
	return objcArg{typ: types.UInt8TypeDescriptor, ptr: unsafe.Pointer(&v), keepAlive: &v}
}

func argFloat32(val float32) objcArg {
	v := val
	return objcArg{typ: types.FloatTypeDescriptor, ptr: unsafe.Pointer(&v), keepAlive: &v}
}

func argFloat64(val float64) objcArg {
	v := val
	return objcArg{typ: types.DoubleTypeDescriptor, ptr: unsafe.Pointer(&v), keepAlive: &v}
}

func argStruct[T any](val T, td *types.TypeDescriptor) objcArg {
	v := val
	return objcArg{typ: td, ptr: unsafe.Pointer(&v), keepAlive: &v}
}

func pointerArgs(args []uintptr) []objcArg {
	out := make([]objcArg, len(args))
	for i, arg := range args {
		out[i] = argPointer(arg)
	}
	return out
}

func msgSend(obj ID, sel SEL, retType *types.TypeDescriptor, retPtr unsafe.Pointer, args ...objcArg) error {
	if obj == 0 || sel == 0 {
		return nil
	}

	argTypes := make([]*types.TypeDescriptor, 2+len(args))
	argTypes[0] = types.PointerTypeDescriptor
	argTypes[1] = types.PointerTypeDescriptor
	for i, arg := range args {
		argTypes[2+i] = arg.typ
	}

	cif := &types.CallInterface{}
	if err := ffi.PrepareCallInterface(cif, types.DefaultCall, retType, argTypes); err != nil {
		return err
	}

	self := uintptr(obj)
	cmd := uintptr(sel)
	argPtrs := make([]unsafe.Pointer, 2+len(args))
	argPtrs[0] = unsafe.Pointer(&self)
	argPtrs[1] = unsafe.Pointer(&cmd)
	for i, arg := range args {
		argPtrs[2+i] = arg.ptr
	}

	fn := objcMsgSendSymbol(retType)
	err := ffi.CallFunction(cif, fn, retPtr, argPtrs)
	runtime.KeepAlive(args)
	return err
}

func msgSendVoid(obj ID, sel SEL, args ...objcArg) {
	_ = msgSend(obj, sel, types.VoidTypeDescriptor, nil, args...)
}

func msgSendID(obj ID, sel SEL, args ...objcArg) ID {
	var result ID
	_ = msgSend(obj, sel, types.PointerTypeDescriptor, unsafe.Pointer(&result), args...)
	return result
}

func msgSendUint(obj ID, sel SEL, args ...objcArg) uint {
	var result uint64
	_ = msgSend(obj, sel, types.UInt64TypeDescriptor, unsafe.Pointer(&result), args...)
	return uint(result)
}

func msgSendBool(obj ID, sel SEL, args ...objcArg) bool {
	var result uint8
	_ = msgSend(obj, sel, types.UInt8TypeDescriptor, unsafe.Pointer(&result), args...)
	return result != 0
}

func objcMsgSendSymbol(retType *types.TypeDescriptor) unsafe.Pointer {
	if retType != nil && retType.Kind == types.StructType && runtime.GOARCH == "amd64" {
		if symObjcMsgSendStret != nil && typeSize(retType) > 16 {
			return symObjcMsgSendStret
		}
	}
	if retType != nil && (retType.Kind == types.FloatType || retType.Kind == types.DoubleType) && runtime.GOARCH == "amd64" {
		if symObjcMsgSendFpret != nil {
			return symObjcMsgSendFpret
		}
	}
	return symObjcMsgSend
}

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

// MsgSend calls an Objective-C method on an object.
func MsgSend(obj ID, sel SEL, args ...uintptr) ID {
	return msgSendID(obj, sel, pointerArgs(args)...)
}

// MsgSendUint calls a method and returns a uint result.
func MsgSendUint(obj ID, sel SEL, args ...uintptr) uint {
	return msgSendUint(obj, sel, pointerArgs(args)...)
}

// MsgSendBool calls a method and returns a bool result.
func MsgSendBool(obj ID, sel SEL, args ...uintptr) bool {
	return msgSendBool(obj, sel, pointerArgs(args)...)
}

// Retain increments the reference count of an object.
func Retain(obj ID) ID {
	if obj == 0 {
		return 0
	}
	return MsgSend(obj, Sel("retain"))
}

// Release decrements the reference count of an object.
func Release(obj ID) {
	if obj == 0 {
		return
	}
	_ = MsgSend(obj, Sel("release"))
}

// AutoreleasePool manages an Objective-C autorelease pool.
type AutoreleasePool struct {
	pool ID
}

// NewAutoreleasePool creates a new autorelease pool.
func NewAutoreleasePool() *AutoreleasePool {
	poolClass := GetClass("NSAutoreleasePool")
	pool := MsgSend(ID(poolClass), Sel("alloc"))
	pool = MsgSend(pool, Sel("init"))
	return &AutoreleasePool{pool: pool}
}

// Drain drains the autorelease pool.
func (p *AutoreleasePool) Drain() {
	if p.pool != 0 {
		_ = MsgSend(p.pool, Sel("drain"))
		p.pool = 0
	}
}

// NSString creates an NSString from a Go string.
// Returns a +1 retained object that the caller must Release().
// Uses alloc/initWithUTF8String: instead of stringWithUTF8String:
// to return a retained object (not autoreleased) for explicit ownership.
func NSString(s string) ID {
	nsStringClass := ID(GetClass("NSString"))
	if len(s) == 0 {
		// Use alloc/init for empty string to get +1 retained object
		obj := MsgSend(nsStringClass, Sel("alloc"))
		return MsgSend(obj, Sel("init"))
	}
	cstr := append([]byte(s), 0)
	obj := MsgSend(nsStringClass, Sel("alloc"))
	return MsgSend(
		obj,
		Sel("initWithUTF8String:"),
		uintptr(unsafe.Pointer(&cstr[0])),
	)
}

// GoString converts an NSString to a Go string.
func GoString(nsstr ID) string {
	if nsstr == 0 {
		return ""
	}
	cstr := MsgSend(nsstr, Sel("UTF8String"))
	if cstr == 0 {
		return ""
	}
	return goStringFromCStr(uintptr(cstr))
}

func goStringFromCStr(cstr uintptr) string {
	if cstr == 0 {
		return ""
	}
	length := 0
	ptr := (*byte)(unsafe.Pointer(cstr)) //nolint:govet // Required for FFI
	for i := 0; i < 4096; i++ {
		b := unsafe.Slice(ptr, i+1)
		if b[i] == 0 {
			length = i
			break
		}
	}
	if length == 0 {
		return ""
	}
	result := unsafe.Slice(ptr, length)
	return string(result)
}
