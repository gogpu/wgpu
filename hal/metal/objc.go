// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin

package metal

import (
	"fmt"
	"sync"
	"unsafe"

	"github.com/go-webgpu/goffi/ffi"
	"github.com/go-webgpu/goffi/types"
)

// Objective-C runtime library handle and function symbols.
var (
	objcLib unsafe.Pointer

	symObjcMsgSend     unsafe.Pointer
	symObjcGetClass    unsafe.Pointer
	symSelRegisterName unsafe.Pointer

	cifGetClass    types.CallInterface
	cifSelRegister types.CallInterface
)

// selectorCache caches registered selectors for performance.
var selectorCache sync.Map

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
	var result Class
	args := [1]unsafe.Pointer{unsafe.Pointer(&cname[0])}
	_ = ffi.CallFunction(&cifGetClass, symObjcGetClass, unsafe.Pointer(&result), args[:])
	return result
}

// RegisterSelector registers and returns a selector for the given name.
func RegisterSelector(name string) SEL {
	if cached, ok := selectorCache.Load(name); ok {
		return cached.(SEL)
	}

	cname := append([]byte(name), 0)
	var result SEL
	args := [1]unsafe.Pointer{unsafe.Pointer(&cname[0])}
	_ = ffi.CallFunction(&cifSelRegister, symSelRegisterName, unsafe.Pointer(&result), args[:])

	selectorCache.Store(name, result)
	return result
}

// Sel is a convenience alias for RegisterSelector.
func Sel(name string) SEL {
	return RegisterSelector(name)
}

// MsgSend calls an Objective-C method on an object.
func MsgSend(obj ID, sel SEL, args ...uintptr) ID {
	if obj == 0 {
		return 0
	}

	allArgs := make([]unsafe.Pointer, 2+len(args))
	allArgs[0] = unsafe.Pointer(&obj)
	allArgs[1] = unsafe.Pointer(&sel)
	for i, arg := range args {
		argCopy := arg
		allArgs[2+i] = unsafe.Pointer(&argCopy)
	}

	argTypes := make([]*types.TypeDescriptor, 2+len(args))
	argTypes[0] = types.PointerTypeDescriptor
	argTypes[1] = types.PointerTypeDescriptor
	for i := range args {
		argTypes[2+i] = types.PointerTypeDescriptor
	}

	var cif types.CallInterface
	err := ffi.PrepareCallInterface(&cif, types.DefaultCall, types.PointerTypeDescriptor, argTypes)
	if err != nil {
		return 0
	}

	var result ID
	_ = ffi.CallFunction(&cif, symObjcMsgSend, unsafe.Pointer(&result), allArgs)
	return result
}

// MsgSendUint calls a method and returns a uint result.
func MsgSendUint(obj ID, sel SEL, args ...uintptr) uint {
	return uint(MsgSend(obj, sel, args...))
}

// MsgSendBool calls a method and returns a bool result.
func MsgSendBool(obj ID, sel SEL, args ...uintptr) bool {
	return MsgSend(obj, sel, args...) != 0
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
func NSString(s string) ID {
	if len(s) == 0 {
		return MsgSend(ID(GetClass("NSString")), Sel("string"))
	}
	cstr := append([]byte(s), 0)
	return MsgSend(
		ID(GetClass("NSString")),
		Sel("stringWithUTF8String:"),
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
