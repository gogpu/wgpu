// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin

package metal

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
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

// --------------------------------------------------------------------------
// ObjC Block ABI — Pure Go implementation
// --------------------------------------------------------------------------
//
// Objective-C blocks (closures) follow a documented ABI layout:
//
//	struct Block_literal {
//	    void *isa;           // &_NSConcreteStackBlock or &_NSConcreteGlobalBlock
//	    int  flags;          // Block flags (see blockHasCopyDispose, etc.)
//	    int  reserved;       // Always 0
//	    void *invoke;        // Function pointer: (block_ptr, args...) -> ret
//	    struct Block_descriptor *descriptor;
//	    // Captured variables follow (we embed a block ID here)
//	    uint64 blockID;      // Index into blockRegistry for Go-side state
//	};
//
// The invoke function receives the block pointer as its first argument,
// allowing us to read blockID and look up the associated Go channel.
//
// Reference: https://clang.llvm.org/docs/Block-ABI-Apple.html

// blockLiteral is the Go representation of an ObjC Block_literal struct.
// It matches the C ABI layout expected by the Objective-C runtime.
//
//nolint:unused // Fields are accessed via unsafe pointer arithmetic from C callbacks
type blockLiteral struct {
	isa        uintptr // Class pointer: _NSConcreteStackBlock
	flags      int32   // Block flags
	reserved   int32   // Reserved, always 0
	invoke     uintptr // C function pointer for the block body
	descriptor uintptr // Pointer to blockDescriptor
	blockID    uint64  // Index into blockRegistry
}

// blockDescriptor is the descriptor referenced by blockLiteral.
//
//nolint:unused // Fields are read by the ObjC runtime
type blockDescriptor struct {
	reserved uint64 // Always 0
	size     uint64 // sizeof(blockLiteral)
}

// blockRegistryEntry holds Go-side state for an active ObjC block.
type blockRegistryEntry struct {
	done chan struct{} // Signaled when the block is invoked
}

// blockRegistry maps block IDs to their Go-side state.
// Entries are added when a block is created and removed after it fires or times out.
var blockRegistry sync.Map // map[uint64]*blockRegistryEntry

// blockIDCounter is the next block ID to assign. Atomically incremented.
var blockIDCounter uint64

// symNSConcreteStackBlock is the address of _NSConcreteStackBlock from libobjc.
// Loaded during initObjCRuntime.
var symNSConcreteStackBlock uintptr

// sharedEventBlockInvoke is the ffi.NewCallback trampoline for
// the MTLSharedEvent notification block.
// Signature: void(^)(id<MTLSharedEvent> event, uint64_t value)
// Block invoke: void(block_ptr, event, value)
//
// Initialized lazily via sync.Once to avoid calling ffi.NewCallback at init time.
var (
	sharedEventBlockInvokeOnce sync.Once
	sharedEventBlockInvokePtr  uintptr
)

// sharedEventBlockDescriptor is allocated once and shared by all notification blocks.
var sharedEventBlockDescriptor *blockDescriptor

func initBlockSupport() {
	// Load _NSConcreteStackBlock symbol from libobjc
	if objcLib != nil {
		sym, err := ffi.GetSymbol(objcLib, "_NSConcreteStackBlock")
		if err == nil && sym != nil {
			// The symbol is a pointer to the class — we need the address of the global,
			// which is what the pointer value itself represents.
			symNSConcreteStackBlock = *(*uintptr)(sym)
		}
	}

	// Create shared descriptor
	sharedEventBlockDescriptor = &blockDescriptor{
		reserved: 0,
		size:     uint64(unsafe.Sizeof(blockLiteral{})),
	}
}

// getSharedEventBlockInvoke returns the C function pointer for notification block invocations.
// The function is created once via ffi.NewCallback and reused for all blocks.
func getSharedEventBlockInvoke() uintptr {
	sharedEventBlockInvokeOnce.Do(func() {
		// Block invoke signature: void (block_ptr uintptr, event uintptr, value uint64)
		// On arm64 macOS, the block pointer is the first argument (x0),
		// event is x1, value is x2.
		sharedEventBlockInvokePtr = ffi.NewCallback(func(blockPtr, event uintptr, value uint64) {
			if blockPtr == 0 {
				return
			}
			// Read blockID from the block literal at the fixed offset.
			// Offset: isa(8) + flags(4) + reserved(4) + invoke(8) + descriptor(8) = 32 bytes
			blockID := *(*uint64)(unsafe.Pointer(blockPtr + 32)) //nolint:govet // Required for ObjC block ABI access

			if entry, ok := blockRegistry.Load(blockID); ok {
				e := entry.(*blockRegistryEntry)
				select {
				case e.done <- struct{}{}:
				default:
					// Channel already has a value or is closed
				}
			}
		})
	})
	return sharedEventBlockInvokePtr
}

// newSharedEventNotificationBlock creates an ObjC block for MTLSharedEvent notifications.
// The block signals the returned channel when invoked by the GPU event system.
//
// The caller must call releaseBlock(id) when the block is no longer needed
// (either after the channel is signaled or after a timeout).
//
// Returns (block pointer, block ID, done channel) or (0, 0, nil) on failure.
func newSharedEventNotificationBlock() (uintptr, uint64, chan struct{}) {
	if symNSConcreteStackBlock == 0 {
		return 0, 0, nil
	}

	invokePtr := getSharedEventBlockInvoke()
	if invokePtr == 0 {
		return 0, 0, nil
	}

	// Allocate block ID and registry entry
	id := nextBlockID()
	done := make(chan struct{}, 1)
	blockRegistry.Store(id, &blockRegistryEntry{done: done})

	// Allocate block on Go heap — must remain alive until the callback fires.
	block := &blockLiteral{
		isa:        symNSConcreteStackBlock,
		flags:      0,
		reserved:   0,
		invoke:     invokePtr,
		descriptor: uintptr(unsafe.Pointer(sharedEventBlockDescriptor)),
		blockID:    id,
	}

	return uintptr(unsafe.Pointer(block)), id, done
}

// releaseBlock removes the block entry from the registry.
// Must be called after the block fires or times out to prevent memory leaks.
func releaseBlock(id uint64) {
	blockRegistry.Delete(id)
}

// nextBlockID atomically increments the block ID counter and returns the new value.
func nextBlockID() uint64 {
	return atomic.AddUint64(&blockIDCounter, 1)
}

// --------------------------------------------------------------------------
// Completed Handler Block — async staging buffer release for WriteTexture
// --------------------------------------------------------------------------
//
// addCompletedHandler: expects a block with signature:
//   void (^)(id<MTLCommandBuffer> commandBuffer)
// Block invoke: void(block_ptr, cmdBuffer) — 2 pointer-sized args.
//
// When the GPU finishes executing the command buffer, Metal invokes the
// block. We look up the block ID and release the associated staging buffer.

// completedHandlerRegistry maps block IDs to staging buffer IDs that
// should be released when the completion handler fires.
var completedHandlerRegistry sync.Map // map[uint64]ID

// completedHandlerBlockInvoke is the ffi.NewCallback trampoline for
// MTLCommandBuffer completion handler blocks.
// Initialized lazily via sync.Once.
var (
	completedHandlerBlockInvokeOnce sync.Once
	completedHandlerBlockInvokePtr  uintptr
)

// getCompletedHandlerBlockInvoke returns the C function pointer for completion
// handler block invocations. Created once via ffi.NewCallback and reused.
func getCompletedHandlerBlockInvoke() uintptr {
	completedHandlerBlockInvokeOnce.Do(func() {
		// Block invoke signature: void (block_ptr uintptr, cmdBuffer uintptr)
		// On arm64: block_ptr in x0, cmdBuffer in x1.
		// ffi.NewCallback requires uintptr return.
		completedHandlerBlockInvokePtr = ffi.NewCallback(func(blockPtr, _ uintptr) uintptr {
			if blockPtr == 0 {
				return 0
			}
			// Read blockID from the block literal at the fixed offset.
			// Offset: isa(8) + flags(4) + reserved(4) + invoke(8) + descriptor(8) = 32 bytes
			blockID := *(*uint64)(unsafe.Pointer(blockPtr + 32)) //nolint:govet // Required for ObjC block ABI access

			if val, ok := completedHandlerRegistry.LoadAndDelete(blockID); ok {
				stagingBuf := val.(ID)
				if stagingBuf != 0 {
					Release(stagingBuf)
				}
			}
			return 0
		})
	})
	return completedHandlerBlockInvokePtr
}

// newCompletedHandlerBlock creates an ObjC block for MTLCommandBuffer
// completion handler that releases the given staging buffer when invoked.
//
// The block is suitable for passing to [MTLCommandBuffer addCompletedHandler:].
// It uses the same blockLiteral layout and blockID mechanism as the shared
// event notification blocks.
//
// Returns (block pointer, block ID) or (0, 0) on failure.
// The caller must keep the returned block pointer alive (via runtime.KeepAlive)
// until after addCompletedHandler: and commit have been called.
//
// If the block will not be used (e.g., fallback to sync), call
// cancelCompletedHandlerBlock(blockID) to remove the registry entry and
// avoid leaking the staging buffer reference.
func newCompletedHandlerBlock(stagingBuffer ID) (uintptr, uint64) {
	if symNSConcreteStackBlock == 0 {
		return 0, 0
	}

	invokePtr := getCompletedHandlerBlockInvoke()
	if invokePtr == 0 {
		return 0, 0
	}

	// Allocate block ID and register the staging buffer
	id := nextBlockID()
	completedHandlerRegistry.Store(id, stagingBuffer)

	// Allocate block on Go heap — must remain alive until addCompletedHandler
	// has consumed it (Metal copies the block internally).
	block := &blockLiteral{
		isa:        symNSConcreteStackBlock,
		flags:      0,
		reserved:   0,
		invoke:     invokePtr,
		descriptor: uintptr(unsafe.Pointer(sharedEventBlockDescriptor)),
		blockID:    id,
	}

	return uintptr(unsafe.Pointer(block)), id
}

// cancelCompletedHandlerBlock removes a completed handler block entry from
// the registry without releasing the staging buffer. Returns the staging
// buffer ID so the caller can release it in the synchronous fallback path.
func cancelCompletedHandlerBlock(id uint64) ID {
	if val, ok := completedHandlerRegistry.LoadAndDelete(id); ok {
		return val.(ID)
	}
	return 0
}

// --------------------------------------------------------------------------
// Frame Completion Block — frame semaphore signaling for Submit throttling
// --------------------------------------------------------------------------
//
// addCompletedHandler: expects a block with signature:
//
//	void (^)(id<MTLCommandBuffer> commandBuffer)
//
// Block invoke: void(block_ptr, cmdBuffer) — 2 pointer-sized args.
//
// When the GPU finishes executing the last command buffer of a Submit batch,
// Metal invokes the block. We look up the block ID and signal the frame
// semaphore channel, releasing a slot for the next frame.

// frameCompletionRegistry maps block IDs to frame semaphore channels.
// Entries are added in newFrameCompletionBlock and removed when the GPU
// invokes the completion handler.
var frameCompletionRegistry sync.Map // map[uint64]chan<- struct{}

// frameCompletionBlockInvoke is the ffi.NewCallback trampoline for
// frame completion handler blocks.
// Initialized lazily via sync.Once.
var (
	frameCompletionBlockInvokeOnce sync.Once
	frameCompletionBlockInvokePtr  uintptr
)

// getFrameCompletionBlockInvoke returns the C function pointer for frame
// completion handler block invocations. Created once and reused.
func getFrameCompletionBlockInvoke() uintptr {
	frameCompletionBlockInvokeOnce.Do(func() {
		// Block invoke signature: void (block_ptr uintptr, cmdBuffer uintptr)
		frameCompletionBlockInvokePtr = ffi.NewCallback(func(blockPtr, _ uintptr) uintptr {
			if blockPtr == 0 {
				return 0
			}
			// Read blockID from the block literal at the fixed offset.
			// Offset: isa(8) + flags(4) + reserved(4) + invoke(8) + descriptor(8) = 32 bytes
			blockID := *(*uint64)(unsafe.Pointer(blockPtr + 32)) //nolint:govet // Required for ObjC block ABI access

			if val, ok := frameCompletionRegistry.LoadAndDelete(blockID); ok {
				ch := val.(chan<- struct{})
				if ch != nil {
					// Signal that the GPU finished this frame — release a semaphore slot.
					ch <- struct{}{}
				}
			}
			return 0
		})
	})
	return frameCompletionBlockInvokePtr
}

// newFrameCompletionBlock creates an ObjC block for MTLCommandBuffer
// addCompletedHandler: that signals the given frame semaphore channel
// when the GPU finishes executing the command buffer.
//
// Returns a block pointer suitable for passing to addCompletedHandler:,
// or 0 if block support is unavailable.
//
// The caller must keep the returned pointer alive (via runtime.KeepAlive)
// until after addCompletedHandler: has been called. Metal copies the block
// internally, so the Go-side literal can be collected after that point.
func newFrameCompletionBlock(frameSemaphore chan struct{}) uintptr {
	if symNSConcreteStackBlock == 0 || frameSemaphore == nil {
		return 0
	}

	invokePtr := getFrameCompletionBlockInvoke()
	if invokePtr == 0 {
		return 0
	}

	// Allocate block ID and register the semaphore channel.
	id := nextBlockID()
	// Store as chan<- struct{} (send-only) to match the registry type.
	frameCompletionRegistry.Store(id, (chan<- struct{})(frameSemaphore))

	// Allocate block on Go heap — must remain alive until addCompletedHandler
	// has consumed it (Metal copies the block internally).
	block := &blockLiteral{
		isa:        symNSConcreteStackBlock,
		flags:      0,
		reserved:   0,
		invoke:     invokePtr,
		descriptor: uintptr(unsafe.Pointer(sharedEventBlockDescriptor)),
		blockID:    id,
	}

	return uintptr(unsafe.Pointer(block))
}
