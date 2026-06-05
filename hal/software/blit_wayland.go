//go:build linux && !(js && wasm)

// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package software

import (
	"image"
	"log/slog"
	"sync"
	"unsafe"

	"github.com/go-webgpu/goffi/ffi"
	"github.com/go-webgpu/goffi/types"
	"golang.org/x/sys/unix"
)

// Wayland SHM presentation for the software backend.
//
// On Wayland, displayHandle is wl_display* and hwnd is wl_surface*.
// The X11 blit path (XPutImage) would crash because those are not X11 handles.
//
// This file implements the Wayland present path using wl_shm shared memory
// buffers. The pattern follows our own CSD SHM implementation in
// gogpu/internal/platform/wayland/libwayland_csd.go:
//
//  1. memfd_create + mmap → shared memory pool
//  2. wl_shm_create_pool → wl_shm_pool
//  3. wl_shm_pool::create_buffer → wl_buffer
//  4. Register wl_buffer.release listener (Qt6 pattern: qwaylandbuffer.cpp)
//  5. Copy pixels, wl_surface_attach + damage_buffer + commit + flush
//
// Double-buffering uses wl_buffer.release to avoid writing into a buffer
// the compositor is still reading. If both buffers are busy, the frame is
// skipped (no corruption, no tearing — same as Qt6 qwaylandshmbackingstore.cpp).
//
// Pixel format: Software backend stores BGRA byte order. On little-endian
// (all supported Linux), BGRA bytes = uint32 0xAARRGGBB = wl_shm ARGB8888.
// No conversion needed — identical to the CSD path.

// waylandShmState holds lazily-loaded libwayland-client symbols and the
// wl_shm global object required for SHM buffer creation. Initialized once
// on first Wayland blit via waylandOnce.
var (
	waylandOnce  sync.Once
	waylandReady bool // true if Wayland SHM path is available

	wlClientLib unsafe.Pointer

	// wl_display functions
	symWlDisplayGetFD       unsafe.Pointer
	symWlDisplayGetRegistry unsafe.Pointer
	symWlDisplayRoundtrip   unsafe.Pointer
	symWlDisplayFlush       unsafe.Pointer

	// wl_registry / wl_shm / wl_shm_pool / wl_surface / wl_buffer / proxy functions
	symWlProxyMarshalConstructor          unsafe.Pointer
	symWlProxyMarshalConstructorVersioned unsafe.Pointer //nolint:unused // reserved for versioned bind
	symWlProxyAddListener                 unsafe.Pointer
	symWlProxyMarshal                     unsafe.Pointer
	symWlProxyDestroy                     unsafe.Pointer

	// wl_interface pointers (loaded from libwayland-client.so data section)
	wlRegistryInterface unsafe.Pointer
	wlShmPoolInterface  unsafe.Pointer
	wlBufferInterface   unsafe.Pointer
	wlCallbackInterface unsafe.Pointer

	// CIF for wl_display_get_fd(wl_display*) -> int
	cifWlDisplayGetFD types.CallInterface
	// CIF for wl_display_roundtrip(wl_display*) -> int
	cifWlDisplayRoundtrip types.CallInterface
	// CIF for wl_display_flush(wl_display*) -> int
	cifWlDisplayFlush types.CallInterface
	// CIF for wl_proxy_marshal_constructor(proxy*, opcode, interface*, ...) -> proxy*
	cifWlProxyMarshalConstructor types.CallInterface
	// CIF for wl_proxy_add_listener(proxy*, listener_impl**, data*) -> int
	cifWlProxyAddListener types.CallInterface
	// CIF for wl_proxy_marshal(proxy*, opcode, ...) -> void
	// We use variadic-like calls via the raw CIF, passing args as uintptr.
	cifWlProxyMarshal types.CallInterface
	// CIF for wl_proxy_destroy(proxy*) -> void
	cifWlProxyDestroy types.CallInterface
)

// waylandBlitState holds per-Surface Wayland SHM resources for double-buffered
// presentation. Embedded in platformBlit (blit_linux.go).
type waylandBlitState struct {
	isWayland bool // true if displayHandle is wl_display* (detected once)
	detected  bool // true if detection has been performed

	wlShm uintptr // bound wl_shm global (0 if not yet obtained)

	// Double-buffer state: two SHM buffers, toggle between them.
	// This avoids writing to a buffer the compositor is still reading.
	buffers  [2]waylandShmBuffer
	frontIdx int // index of the buffer last submitted to compositor
}

// waylandShmBuffer holds one SHM buffer for Wayland presentation.
type waylandShmBuffer struct {
	fd     int     // memfd file descriptor (-1 if unused)
	data   []byte  // mmap'd pixel data
	pool   uintptr // wl_shm_pool proxy
	buffer uintptr // wl_buffer proxy
	width  int32
	height int32
	busy   bool // true while compositor owns this buffer (Qt6 qwaylandbuffer.cpp pattern)
}

// Cached CIFs for per-frame marshal calls (zero alloc after init).
var (
	// wl_proxy_marshal(proxy, opcode) — commit (opcode 6), pool destroy, buffer destroy.
	cifMarshal2 types.CallInterface
	// wl_proxy_marshal(proxy, opcode, buffer, x, y) — wl_surface_attach (opcode 1).
	cifSurfaceAttach types.CallInterface
	// wl_proxy_marshal(proxy, opcode, x, y, w, h) — wl_surface_damage_buffer (opcode 9).
	cifSurfaceDamageBuffer types.CallInterface
)

// Global registry listener callback state.
// Protected by waylandOnce (only one goroutine does init).
var (
	registryListenerFuncs [2]uintptr // global, announce, remove
	registryListenersOnce sync.Once

	// Buffer release listener: wl_buffer has one event (release, opcode 0).
	// Single callback slot — all buffers share the same function.
	bufferListenerFuncs [1]uintptr
	bufferListenerOnce  sync.Once

	// pendingShmBindName stores the wl_shm global name during registry roundtrip.
	pendingShmBindMu   sync.Mutex
	pendingShmBindName uint32

	// bufferBusyMap maps wl_buffer proxy address to the waylandShmBuffer owning it.
	// Protected by bufferBusyMu. Used by the release callback to clear busy flag.
	bufferBusyMu  sync.Mutex
	bufferBusyMap = map[uintptr]*waylandShmBuffer{}
)

// isWaylandDisplay checks whether the given display handle is a Wayland
// wl_display by attempting wl_display_get_fd. A valid Wayland display
// returns fd >= 0. An X11 Display* passed to this function will either
// return a negative value or crash would be caught — but since the library
// was loaded successfully, the call is safe (wl_display_get_fd validates
// its argument before accessing members via the display magic number).
//
// This detection is conservative: if libwayland-client.so is not available,
// we assume X11.
func isWaylandDisplay(displayHandle uintptr) bool {
	waylandOnce.Do(initWayland)
	if !waylandReady {
		return false
	}

	var fd int32
	args := [1]unsafe.Pointer{unsafe.Pointer(&displayHandle)}
	_ = ffi.CallFunction(&cifWlDisplayGetFD, symWlDisplayGetFD, unsafe.Pointer(&fd), args[:])
	return fd >= 0
}

// initWayland loads libwayland-client.so and prepares CIFs for SHM presentation.
func initWayland() {
	var err error

	wlClientLib, err = ffi.LoadLibrary("libwayland-client.so.0")
	if err != nil {
		wlClientLib, err = ffi.LoadLibrary("libwayland-client.so")
		if err != nil {
			slog.Debug("software: Wayland blit unavailable — could not load libwayland-client", "error", err)
			return
		}
	}

	// Load function symbols.
	symbols := []struct {
		name string
		dst  *unsafe.Pointer
	}{
		{"wl_display_get_fd", &symWlDisplayGetFD},
		{"wl_display_get_registry", &symWlDisplayGetRegistry},
		{"wl_display_roundtrip", &symWlDisplayRoundtrip},
		{"wl_display_flush", &symWlDisplayFlush},
		{"wl_proxy_marshal_constructor", &symWlProxyMarshalConstructor},
		{"wl_proxy_add_listener", &symWlProxyAddListener},
		{"wl_proxy_marshal", &symWlProxyMarshal},
		{"wl_proxy_destroy", &symWlProxyDestroy},
	}
	for _, s := range symbols {
		*s.dst, err = ffi.GetSymbol(wlClientLib, s.name)
		if err != nil {
			slog.Debug("software: Wayland blit unavailable — missing symbol", "symbol", s.name, "error", err)
			return
		}
	}

	// Load wl_interface pointers (data symbols in libwayland-client.so).
	interfaces := []struct {
		name string
		dst  *unsafe.Pointer
	}{
		{"wl_registry_interface", &wlRegistryInterface},
		{"wl_shm_pool_interface", &wlShmPoolInterface},
		{"wl_buffer_interface", &wlBufferInterface},
		{"wl_callback_interface", &wlCallbackInterface},
	}
	for _, iface := range interfaces {
		*iface.dst, err = ffi.GetSymbol(wlClientLib, iface.name)
		if err != nil {
			slog.Debug("software: Wayland blit unavailable — missing interface", "interface", iface.name, "error", err)
			return
		}
	}

	// Prepare CIFs.

	// int wl_display_get_fd(wl_display*)
	if err = ffi.PrepareCallInterface(&cifWlDisplayGetFD, types.DefaultCall,
		types.SInt32TypeDescriptor,
		[]*types.TypeDescriptor{types.PointerTypeDescriptor}); err != nil {
		return
	}

	// wl_registry* wl_display_get_registry(wl_display*)
	// Actually wl_proxy_marshal_constructor, but we'll use the direct symbol.
	// wl_display_get_registry is: wl_proxy_marshal_constructor((wl_proxy*)display, WL_DISPLAY_GET_REGISTRY, &wl_registry_interface, NULL)
	// We'll call wl_proxy_marshal_constructor directly.

	// int wl_display_roundtrip(wl_display*)
	if err = ffi.PrepareCallInterface(&cifWlDisplayRoundtrip, types.DefaultCall,
		types.SInt32TypeDescriptor,
		[]*types.TypeDescriptor{types.PointerTypeDescriptor}); err != nil {
		return
	}

	// int wl_display_flush(wl_display*)
	if err = ffi.PrepareCallInterface(&cifWlDisplayFlush, types.DefaultCall,
		types.SInt32TypeDescriptor,
		[]*types.TypeDescriptor{types.PointerTypeDescriptor}); err != nil {
		return
	}

	// wl_proxy* wl_proxy_marshal_constructor(wl_proxy*, uint32 opcode, wl_interface*, ...)
	// Variadic, but goffi treats extra args as pointer-sized. We'll prepare for 4 args
	// (proxy, opcode, interface, NULL) which covers get_registry and create_pool.
	if err = ffi.PrepareCallInterface(&cifWlProxyMarshalConstructor, types.DefaultCall,
		types.PointerTypeDescriptor,
		[]*types.TypeDescriptor{
			types.PointerTypeDescriptor, // proxy
			types.UInt32TypeDescriptor,  // opcode
			types.PointerTypeDescriptor, // interface
			types.PointerTypeDescriptor, // first variadic (NULL or arg)
		}); err != nil {
		return
	}

	// int wl_proxy_add_listener(wl_proxy*, void(**)(void), void* data)
	if err = ffi.PrepareCallInterface(&cifWlProxyAddListener, types.DefaultCall,
		types.SInt32TypeDescriptor,
		[]*types.TypeDescriptor{
			types.PointerTypeDescriptor, // proxy
			types.PointerTypeDescriptor, // implementation
			types.PointerTypeDescriptor, // data
		}); err != nil {
		return
	}

	// void wl_proxy_marshal(wl_proxy*, uint32 opcode, ...)
	// We need multiple arities. Prepare a 7-arg version (covers create_buffer's 6 args + opcode).
	if err = ffi.PrepareCallInterface(&cifWlProxyMarshal, types.DefaultCall,
		types.VoidTypeDescriptor,
		[]*types.TypeDescriptor{
			types.PointerTypeDescriptor, // proxy
			types.UInt32TypeDescriptor,  // opcode
			types.PointerTypeDescriptor, // arg1
			types.PointerTypeDescriptor, // arg2
			types.PointerTypeDescriptor, // arg3
			types.PointerTypeDescriptor, // arg4
			types.PointerTypeDescriptor, // arg5
		}); err != nil {
		return
	}

	// void wl_proxy_destroy(wl_proxy*)
	if err = ffi.PrepareCallInterface(&cifWlProxyDestroy, types.DefaultCall,
		types.VoidTypeDescriptor,
		[]*types.TypeDescriptor{types.PointerTypeDescriptor}); err != nil {
		return
	}

	// Cached CIFs for per-frame calls (zero alloc after init).

	// marshal2: wl_proxy_marshal(proxy, opcode) — for commit, pool destroy, buffer destroy.
	if err = ffi.PrepareCallInterface(&cifMarshal2, types.DefaultCall,
		types.VoidTypeDescriptor,
		[]*types.TypeDescriptor{
			types.PointerTypeDescriptor, // proxy
			types.UInt32TypeDescriptor,  // opcode
		}); err != nil {
		return
	}

	// attach: wl_proxy_marshal(proxy, opcode, buffer, x, y) — wl_surface_attach.
	if err = ffi.PrepareCallInterface(&cifSurfaceAttach, types.DefaultCall,
		types.VoidTypeDescriptor,
		[]*types.TypeDescriptor{
			types.PointerTypeDescriptor, // proxy (surface)
			types.UInt32TypeDescriptor,  // opcode (1)
			types.PointerTypeDescriptor, // buffer
			types.SInt32TypeDescriptor,  // x
			types.SInt32TypeDescriptor,  // y
		}); err != nil {
		return
	}

	// damage_buffer: wl_proxy_marshal(proxy, opcode, x, y, w, h) — opcode 9.
	if err = ffi.PrepareCallInterface(&cifSurfaceDamageBuffer, types.DefaultCall,
		types.VoidTypeDescriptor,
		[]*types.TypeDescriptor{
			types.PointerTypeDescriptor, // proxy (surface)
			types.UInt32TypeDescriptor,  // opcode (9)
			types.SInt32TypeDescriptor,  // x
			types.SInt32TypeDescriptor,  // y
			types.SInt32TypeDescriptor,  // w
			types.SInt32TypeDescriptor,  // h
		}); err != nil {
		return
	}

	waylandReady = true
	slog.Debug("software: Wayland SHM blit initialized")
}

// obtainWlShm gets the wl_shm global by creating a wl_registry, listening
// for the wl_shm global, and doing a roundtrip. This is called once per
// surface on first Wayland blit.
func obtainWlShm(display uintptr) uintptr {
	if !waylandReady {
		return 0
	}

	// wl_display_get_registry = wl_proxy_marshal_constructor(display, 1, &wl_registry_interface, NULL)
	// WL_DISPLAY_GET_REGISTRY opcode = 1
	var opcode uint32 = 1
	var null uintptr
	ifacePtr := uintptr(wlRegistryInterface)
	args := [4]unsafe.Pointer{
		unsafe.Pointer(&display),
		unsafe.Pointer(&opcode),
		unsafe.Pointer(&ifacePtr),
		unsafe.Pointer(&null),
	}
	var registry uintptr
	_ = ffi.CallFunction(&cifWlProxyMarshalConstructor, symWlProxyMarshalConstructor, unsafe.Pointer(&registry), args[:])
	if registry == 0 {
		slog.Warn("software: wl_display_get_registry failed")
		return 0
	}

	// Add registry listener to catch wl_shm global.
	registryListenersOnce.Do(func() {
		registryListenerFuncs[0] = ffi.NewCallback(registryGlobalCb)
		registryListenerFuncs[1] = ffi.NewCallback(registryGlobalRemoveCb)
	})

	pendingShmBindMu.Lock()
	pendingShmBindName = 0
	pendingShmBindMu.Unlock()

	listenerPtr := uintptr(unsafe.Pointer(&registryListenerFuncs[0]))
	var listenerData uintptr // not used, callbacks access globals
	addArgs := [3]unsafe.Pointer{
		unsafe.Pointer(&registry),
		unsafe.Pointer(&listenerPtr),
		unsafe.Pointer(&listenerData),
	}
	var addResult int32
	_ = ffi.CallFunction(&cifWlProxyAddListener, symWlProxyAddListener, unsafe.Pointer(&addResult), addArgs[:])

	// Roundtrip to receive registry events.
	roundtripArgs := [1]unsafe.Pointer{unsafe.Pointer(&display)}
	var rtResult int32
	_ = ffi.CallFunction(&cifWlDisplayRoundtrip, symWlDisplayRoundtrip, unsafe.Pointer(&rtResult), roundtripArgs[:])

	pendingShmBindMu.Lock()
	shmName := pendingShmBindName
	pendingShmBindMu.Unlock()

	if shmName == 0 {
		slog.Warn("software: wl_shm not found in registry")
		// Destroy registry proxy.
		destroyArgs := [1]unsafe.Pointer{unsafe.Pointer(&registry)}
		_ = ffi.CallFunction(&cifWlProxyDestroy, symWlProxyDestroy, nil, destroyArgs[:])
		return 0
	}

	// Bind wl_shm: wl_registry_bind = wl_proxy_marshal_constructor_versioned
	// But simpler: use wl_proxy_marshal_constructor with opcode 0 (bind).
	// wl_registry::bind opcode = 0, signature "usun" → name, interface_name, version, new_id
	// Actually wl_registry_bind is implemented as:
	//   wl_proxy_marshal_constructor_versioned(registry, WL_REGISTRY_BIND, &wl_shm_interface, version, name, interface_name, version, NULL)
	// This is complex. Let's load the versioned variant.

	// Simpler approach: load wl_proxy_marshal_constructor_versioned.
	var symVersioned unsafe.Pointer
	symVersioned, _ = ffi.GetSymbol(wlClientLib, "wl_proxy_marshal_constructor_versioned")
	if symVersioned == nil {
		slog.Warn("software: wl_proxy_marshal_constructor_versioned not found")
		destroyArgs := [1]unsafe.Pointer{unsafe.Pointer(&registry)}
		_ = ffi.CallFunction(&cifWlProxyDestroy, symWlProxyDestroy, nil, destroyArgs[:])
		return 0
	}

	// Prepare CIF for versioned: wl_proxy*(proxy, opcode, interface, version, name, ifaceName, version, NULL)
	// The actual signature for wl_registry_bind is:
	//   wl_proxy_marshal_constructor_versioned(registry, 0, &wl_shm_interface, 1, name, "wl_shm", 1, NULL)
	var cifVersioned types.CallInterface
	if err := ffi.PrepareCallInterface(&cifVersioned, types.DefaultCall,
		types.PointerTypeDescriptor,
		[]*types.TypeDescriptor{
			types.PointerTypeDescriptor, // proxy (registry)
			types.UInt32TypeDescriptor,  // opcode (0 = bind)
			types.PointerTypeDescriptor, // interface (&wl_shm_interface)
			types.UInt32TypeDescriptor,  // version
			types.UInt32TypeDescriptor,  // name
			types.PointerTypeDescriptor, // interface_name string
			types.UInt32TypeDescriptor,  // version (repeated)
			types.PointerTypeDescriptor, // NULL terminator
		}); err != nil {
		destroyArgs := [1]unsafe.Pointer{unsafe.Pointer(&registry)}
		_ = ffi.CallFunction(&cifWlProxyDestroy, symWlProxyDestroy, nil, destroyArgs[:])
		return 0
	}

	// Get wl_shm_interface pointer.
	var symShmInterface unsafe.Pointer
	symShmInterface, _ = ffi.GetSymbol(wlClientLib, "wl_shm_interface")
	if symShmInterface == nil {
		slog.Warn("software: wl_shm_interface not found")
		destroyArgs := [1]unsafe.Pointer{unsafe.Pointer(&registry)}
		_ = ffi.CallFunction(&cifWlProxyDestroy, symWlProxyDestroy, nil, destroyArgs[:])
		return 0
	}

	shmIfacePtr := uintptr(symShmInterface)
	var bindOpcode uint32
	var bindVersion uint32 = 1
	shmNameCStr := append([]byte("wl_shm"), 0)
	shmNamePtr := uintptr(unsafe.Pointer(&shmNameCStr[0]))

	bindArgs := [8]unsafe.Pointer{
		unsafe.Pointer(&registry),
		unsafe.Pointer(&bindOpcode),
		unsafe.Pointer(&shmIfacePtr),
		unsafe.Pointer(&bindVersion),
		unsafe.Pointer(&shmName),
		unsafe.Pointer(&shmNamePtr),
		unsafe.Pointer(&bindVersion),
		unsafe.Pointer(&null),
	}
	var shm uintptr
	_ = ffi.CallFunction(&cifVersioned, symVersioned, unsafe.Pointer(&shm), bindArgs[:])

	// Roundtrip again to ensure bind completes.
	_ = ffi.CallFunction(&cifWlDisplayRoundtrip, symWlDisplayRoundtrip, unsafe.Pointer(&rtResult), roundtripArgs[:])

	// Don't destroy registry — keep it alive for the display lifetime.
	// (Destroying it is safe but unnecessary; the proxy is tiny.)

	if shm == 0 {
		slog.Warn("software: wl_registry_bind for wl_shm failed")
	} else {
		slog.Debug("software: wl_shm bound successfully", "shm", shm)
	}
	return shm
}

// registryGlobalCb: void(data, wl_registry, name, interface_name, version)
func registryGlobalCb(data, registry, name, ifaceName, version uintptr) {
	// Read interface name string.
	if ifaceName == 0 {
		return
	}
	nameStr := cString(ifaceName)
	if nameStr == "wl_shm" {
		pendingShmBindMu.Lock()
		pendingShmBindName = uint32(name)
		pendingShmBindMu.Unlock()
	}
}

// registryGlobalRemoveCb: void(data, wl_registry, name)
func registryGlobalRemoveCb(_, _, _ uintptr) {}

// bufferReleaseCb is called by the compositor when it no longer needs the
// wl_buffer contents. Matches Qt6 qwaylandbuffer.cpp:30-37 pattern.
// Signature: void(data, wl_buffer)
func bufferReleaseCb(_, wlBuffer uintptr) {
	bufferBusyMu.Lock()
	if buf, ok := bufferBusyMap[wlBuffer]; ok {
		buf.busy = false
	}
	bufferBusyMu.Unlock()
}

// cString reads a null-terminated C string from a pointer.
func cString(ptr uintptr) string {
	if ptr == 0 {
		return ""
	}
	//nolint:govet // Converting uintptr (C string address) to unsafe.Pointer is required for FFI
	p := (*byte)(unsafe.Pointer(ptr))
	length := 0
	for i := 0; i < 256; i++ {
		b := unsafe.Slice(p, i+1)
		if b[i] == 0 {
			length = i
			break
		}
	}
	if length == 0 {
		return ""
	}
	result := unsafe.Slice(p, length)
	return string(result)
}

// waylandCreateShmBuffer creates a new SHM buffer for the given dimensions.
func waylandCreateShmBuffer(shm uintptr, width, height int32) *waylandShmBuffer {
	stride := width * 4
	size := int(stride) * int(height)

	// Create memfd.
	fd, err := unix.MemfdCreate("gogpu-sw-blit", unix.MFD_CLOEXEC)
	if err != nil {
		slog.Warn("software: Wayland memfd_create failed", "error", err)
		return nil
	}
	if err := unix.Ftruncate(fd, int64(size)); err != nil {
		_ = unix.Close(fd) // Best-effort cleanup; fd is invalid after ftruncate failure.
		slog.Warn("software: Wayland ftruncate failed", "error", err)
		return nil
	}

	// mmap the shared memory.
	data, err := unix.Mmap(fd, 0, size, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		_ = unix.Close(fd) // Best-effort cleanup; mmap failed.
		slog.Warn("software: Wayland mmap failed", "error", err)
		return nil
	}

	// wl_shm_create_pool: wl_proxy_marshal_constructor(shm, 0, &wl_shm_pool_interface, NULL, fd, size)
	// opcode 0 = create_pool, signature "nhi" → new_id, fd, size
	// The fd is passed as the first variadic arg (wl_argument.h = fd).
	var poolOpcode uint32
	shmPoolIfacePtr := uintptr(wlShmPoolInterface)
	fdVal := uintptr(uint32(fd))
	sizeVal := uintptr(uint32(size))

	// wl_proxy_marshal_constructor for create_pool: proxy, opcode, interface, NULL_id, fd, size
	// We need a 6-arg CIF.
	var cifCreatePool types.CallInterface
	if err := ffi.PrepareCallInterface(&cifCreatePool, types.DefaultCall,
		types.PointerTypeDescriptor,
		[]*types.TypeDescriptor{
			types.PointerTypeDescriptor, // proxy (shm)
			types.UInt32TypeDescriptor,  // opcode
			types.PointerTypeDescriptor, // interface
			types.PointerTypeDescriptor, // new_id (NULL placeholder)
			types.PointerTypeDescriptor, // fd
			types.PointerTypeDescriptor, // size
		}); err != nil {
		_ = unix.Munmap(data)
		_ = unix.Close(fd)
		return nil
	}

	var null uintptr
	poolArgs := [6]unsafe.Pointer{
		unsafe.Pointer(&shm),
		unsafe.Pointer(&poolOpcode),
		unsafe.Pointer(&shmPoolIfacePtr),
		unsafe.Pointer(&null),
		unsafe.Pointer(&fdVal),
		unsafe.Pointer(&sizeVal),
	}
	var pool uintptr
	_ = ffi.CallFunction(&cifCreatePool, symWlProxyMarshalConstructor, unsafe.Pointer(&pool), poolArgs[:])
	if pool == 0 {
		_ = unix.Munmap(data)
		_ = unix.Close(fd)
		slog.Warn("software: wl_shm_create_pool failed")
		return nil
	}

	// wl_shm_pool::create_buffer: opcode 0, signature "niiiiu"
	// create_buffer(new_id, offset=0, width, height, stride, format=0(ARGB8888))
	bufIfacePtr := uintptr(wlBufferInterface)
	var bufOpcode uint32
	var offset uintptr
	widthArg := uintptr(uint32(width))
	heightArg := uintptr(uint32(height))
	strideArg := uintptr(uint32(stride))
	var format uintptr // 0 = ARGB8888

	var cifCreateBuffer types.CallInterface
	if err := ffi.PrepareCallInterface(&cifCreateBuffer, types.DefaultCall,
		types.PointerTypeDescriptor,
		[]*types.TypeDescriptor{
			types.PointerTypeDescriptor, // proxy (pool)
			types.UInt32TypeDescriptor,  // opcode
			types.PointerTypeDescriptor, // interface
			types.PointerTypeDescriptor, // new_id (NULL placeholder)
			types.PointerTypeDescriptor, // offset
			types.PointerTypeDescriptor, // width
			types.PointerTypeDescriptor, // height
			types.PointerTypeDescriptor, // stride
			types.PointerTypeDescriptor, // format
		}); err != nil {
		_ = unix.Munmap(data)
		_ = unix.Close(fd)
		return nil
	}

	bufArgs := [9]unsafe.Pointer{
		unsafe.Pointer(&pool),
		unsafe.Pointer(&bufOpcode),
		unsafe.Pointer(&bufIfacePtr),
		unsafe.Pointer(&null),
		unsafe.Pointer(&offset),
		unsafe.Pointer(&widthArg),
		unsafe.Pointer(&heightArg),
		unsafe.Pointer(&strideArg),
		unsafe.Pointer(&format),
	}
	var buffer uintptr
	_ = ffi.CallFunction(&cifCreateBuffer, symWlProxyMarshalConstructor, unsafe.Pointer(&buffer), bufArgs[:])
	if buffer == 0 {
		// Destroy pool: opcode 1
		destroyPool(pool)
		_ = unix.Munmap(data)
		_ = unix.Close(fd)
		slog.Warn("software: wl_shm_pool::create_buffer failed")
		return nil
	}

	// Destroy the pool immediately — the buffer keeps the fd reference alive.
	// (Same pattern as CSD code.)
	destroyPool(pool)

	buf := &waylandShmBuffer{
		fd:     fd,
		data:   data,
		pool:   0, // already destroyed
		buffer: buffer,
		width:  width,
		height: height,
	}

	// Register wl_buffer.release listener (Qt6 qwaylandbuffer.cpp pattern).
	// wl_buffer has one event: release (opcode 0). The listener array has 1 slot.
	bufferListenerOnce.Do(func() {
		bufferListenerFuncs[0] = ffi.NewCallback(bufferReleaseCb)
	})

	bufferBusyMu.Lock()
	bufferBusyMap[buffer] = buf
	bufferBusyMu.Unlock()

	listenerPtr := uintptr(unsafe.Pointer(&bufferListenerFuncs[0]))
	var listenerData uintptr
	addArgs := [3]unsafe.Pointer{
		unsafe.Pointer(&buffer),
		unsafe.Pointer(&listenerPtr),
		unsafe.Pointer(&listenerData),
	}
	var addResult int32
	_ = ffi.CallFunction(&cifWlProxyAddListener, symWlProxyAddListener, unsafe.Pointer(&addResult), addArgs[:])

	return buf
}

// destroyPool calls wl_shm_pool::destroy (opcode 1) then wl_proxy_destroy.
func destroyPool(pool uintptr) {
	var destroyOpcode uint32 = 1
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&pool),
		unsafe.Pointer(&destroyOpcode),
	}
	_ = ffi.CallFunction(&cifMarshal2, symWlProxyMarshal, nil, args[:])

	destroyArgs := [1]unsafe.Pointer{unsafe.Pointer(&pool)}
	_ = ffi.CallFunction(&cifWlProxyDestroy, symWlProxyDestroy, nil, destroyArgs[:])
}

// waylandDestroyShmBuffer releases all resources associated with a SHM buffer.
func waylandDestroyShmBuffer(buf *waylandShmBuffer) {
	if buf == nil {
		return
	}
	if buf.buffer != 0 {
		// Remove from release callback map before destroying.
		bufferBusyMu.Lock()
		delete(bufferBusyMap, buf.buffer)
		bufferBusyMu.Unlock()

		// wl_buffer::destroy opcode = 0
		var destroyOpcode uint32
		marshalArgs := [2]unsafe.Pointer{
			unsafe.Pointer(&buf.buffer),
			unsafe.Pointer(&destroyOpcode),
		}
		_ = ffi.CallFunction(&cifMarshal2, symWlProxyMarshal, nil, marshalArgs[:])

		destroyArgs := [1]unsafe.Pointer{unsafe.Pointer(&buf.buffer)}
		_ = ffi.CallFunction(&cifWlProxyDestroy, symWlProxyDestroy, nil, destroyArgs[:])
		buf.buffer = 0
	}
	if buf.data != nil {
		_ = unix.Munmap(buf.data) // Best-effort cleanup on destroy.
		buf.data = nil
	}
	if buf.fd >= 0 {
		_ = unix.Close(buf.fd) // Best-effort cleanup on destroy.
		buf.fd = -1
	}
}

// waylandPresent copies pixel data into the SHM buffer and commits the surface.
func (s *Surface) waylandPresent(data []byte, width, height int32) {
	wl := &s.wlState
	if wl.wlShm == 0 {
		wl.wlShm = obtainWlShm(s.displayHandle)
		if wl.wlShm == 0 {
			return
		}
	}

	// Pick a non-busy buffer (Qt6 qwaylandshmbackingstore.cpp:349 pattern).
	backIdx := 1 - wl.frontIdx
	buf := &wl.buffers[backIdx]
	if buf.busy {
		// Try the other buffer.
		buf = &wl.buffers[wl.frontIdx]
		if buf.busy {
			// Both busy — skip frame to avoid corruption.
			return
		}
		backIdx = wl.frontIdx
	}

	// Reallocate if dimensions changed.
	if buf.buffer == 0 || buf.width != width || buf.height != height {
		if buf.buffer != 0 {
			waylandDestroyShmBuffer(buf)
			*buf = waylandShmBuffer{fd: -1}
		}
		newBuf := waylandCreateShmBuffer(wl.wlShm, width, height)
		if newBuf == nil {
			return
		}
		*buf = *newBuf
	}

	// Copy pixels into the SHM buffer.
	n := int(width) * int(height) * 4
	if n > len(data) {
		n = len(data)
	}
	if n > len(buf.data) {
		n = len(buf.data)
	}
	copy(buf.data[:n], data[:n])

	surface := s.hwnd

	// wl_surface_attach(surface, buffer, 0, 0) — opcode 1
	waylandSurfaceAttach(surface, buf.buffer, 0, 0)

	// wl_surface_damage_buffer(surface, 0, 0, width, height) — opcode 9
	// Preferred over deprecated wl_surface_damage (opcode 2) since wl_surface v4.
	waylandSurfaceDamageBuffer(surface, 0, 0, width, height)

	// wl_surface_commit(surface) — opcode 6
	waylandSurfaceCommit(surface)

	// Mark buffer as owned by compositor until release callback fires.
	buf.busy = true

	// wl_display_flush
	flushArgs := [1]unsafe.Pointer{unsafe.Pointer(&s.displayHandle)}
	var flushResult int32
	_ = ffi.CallFunction(&cifWlDisplayFlush, symWlDisplayFlush, unsafe.Pointer(&flushResult), flushArgs[:])

	wl.frontIdx = backIdx
}

// waylandPresentDamage copies pixel data and commits with damage rects.
func (s *Surface) waylandPresentDamage(data []byte, width, height int32, rects []image.Rectangle) {
	wl := &s.wlState
	if wl.wlShm == 0 {
		wl.wlShm = obtainWlShm(s.displayHandle)
		if wl.wlShm == 0 {
			return
		}
	}

	// Pick a non-busy buffer.
	backIdx := 1 - wl.frontIdx
	buf := &wl.buffers[backIdx]
	if buf.busy {
		buf = &wl.buffers[wl.frontIdx]
		if buf.busy {
			return
		}
		backIdx = wl.frontIdx
	}

	if buf.buffer == 0 || buf.width != width || buf.height != height {
		if buf.buffer != 0 {
			waylandDestroyShmBuffer(buf)
			*buf = waylandShmBuffer{fd: -1}
		}
		newBuf := waylandCreateShmBuffer(wl.wlShm, width, height)
		if newBuf == nil {
			return
		}
		*buf = *newBuf
	}

	n := int(width) * int(height) * 4
	if n > len(data) {
		n = len(data)
	}
	if n > len(buf.data) {
		n = len(buf.data)
	}
	copy(buf.data[:n], data[:n])

	surface := s.hwnd

	waylandSurfaceAttach(surface, buf.buffer, 0, 0)

	// Issue damage_buffer for each rect (opcode 9, buffer coordinates).
	bounds := image.Rect(0, 0, int(width), int(height))
	for _, r := range rects {
		r = r.Intersect(bounds)
		if r.Empty() {
			continue
		}
		waylandSurfaceDamageBuffer(surface, int32(r.Min.X), int32(r.Min.Y), int32(r.Dx()), int32(r.Dy()))
	}

	waylandSurfaceCommit(surface)

	buf.busy = true

	flushArgs := [1]unsafe.Pointer{unsafe.Pointer(&s.displayHandle)}
	var flushResult int32
	_ = ffi.CallFunction(&cifWlDisplayFlush, symWlDisplayFlush, unsafe.Pointer(&flushResult), flushArgs[:])

	wl.frontIdx = backIdx
}

// waylandSurfaceCommit calls wl_surface_commit (opcode 6).
func waylandSurfaceCommit(surface uintptr) {
	var opcode uint32 = 6
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&surface),
		unsafe.Pointer(&opcode),
	}
	_ = ffi.CallFunction(&cifMarshal2, symWlProxyMarshal, nil, args[:])
}

// waylandSurfaceAttach calls wl_surface_attach(surface, buffer, x, y) — opcode 1.
func waylandSurfaceAttach(surface, buffer uintptr, x, y int32) {
	var opcode uint32 = 1
	args := [5]unsafe.Pointer{
		unsafe.Pointer(&surface),
		unsafe.Pointer(&opcode),
		unsafe.Pointer(&buffer),
		unsafe.Pointer(&x),
		unsafe.Pointer(&y),
	}
	_ = ffi.CallFunction(&cifSurfaceAttach, symWlProxyMarshal, nil, args[:])
}

// waylandSurfaceDamageBuffer calls wl_surface_damage_buffer(surface, x, y, w, h) — opcode 9.
// Preferred over deprecated wl_surface_damage (opcode 2) since wl_surface v4 (Wayland 1.10, 2016).
// Uses buffer coordinates instead of surface coordinates — correct on HiDPI.
func waylandSurfaceDamageBuffer(surface uintptr, x, y, w, h int32) {
	var opcode uint32 = 9
	args := [6]unsafe.Pointer{
		unsafe.Pointer(&surface),
		unsafe.Pointer(&opcode),
		unsafe.Pointer(&x),
		unsafe.Pointer(&y),
		unsafe.Pointer(&w),
		unsafe.Pointer(&h),
	}
	_ = ffi.CallFunction(&cifSurfaceDamageBuffer, symWlProxyMarshal, nil, args[:])
}

// destroyWaylandBlitState releases all Wayland SHM resources for a surface.
func (s *Surface) destroyWaylandBlitState() {
	wl := &s.wlState
	for i := range wl.buffers {
		waylandDestroyShmBuffer(&wl.buffers[i])
		wl.buffers[i] = waylandShmBuffer{fd: -1}
	}
	wl.wlShm = 0
}
