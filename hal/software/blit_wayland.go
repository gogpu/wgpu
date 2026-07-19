//go:build linux && !(js && wasm)

// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

// FFI error handling follows ADR-049 three-tier strategy:
//
//	Tier 1 (creation/submit): check both FFI error and API result code
//	Tier 2 (void/hot path): infallible by GPU API contract — Vulkan §6.6, WebGPU §21.2
//	Tier 3 (platform syscalls): use errno for diagnostics (Wayland, X11, Win32)
//
// Enterprise reference: Rust wgpu-hal returns () for all draw/destroy/barrier commands.
//
// Wayland blit calls are Tier 2: wl_proxy_marshal, wl_proxy_destroy, and
// wl_display_flush are void or return status already checked inline.
// wl_display_roundtrip_queue returns -1 on error (checked via result variable).
package software

import (
	"image"
	"log/slog"
	"sync"
	"syscall"
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
// Triple-buffering (3 SHM buffers) with wl_buffer.release avoids writing
// into a buffer the compositor is still reading. If all buffers are busy,
// the frame is skipped (no corruption, no tearing). Qt6 uses up to 5
// buffers (qwaylandshmbackingstore.cpp); 3 is the minimum for skip-free
// presentation on slow compositors (e.g. pixman renderer).
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
	symWlDisplayRoundtrip      unsafe.Pointer
	symWlDisplayFlush          unsafe.Pointer
	symWlDisplayCreateQueue    unsafe.Pointer // wl_display_create_queue(display) -> queue*
	symWlDisplayDispatchQueueP unsafe.Pointer // wl_display_dispatch_queue_pending(display, queue) -> int

	// wl_registry / wl_shm / wl_shm_pool / wl_surface / wl_buffer / proxy functions
	symWlProxyMarshalConstructor          unsafe.Pointer
	symWlProxyMarshalConstructorVersioned unsafe.Pointer //nolint:unused // reserved for versioned bind
	symWlProxyAddListener                 unsafe.Pointer
	symWlProxyMarshal                     unsafe.Pointer
	symWlProxyDestroy                     unsafe.Pointer
	symWlProxySetQueue                    unsafe.Pointer // wl_proxy_set_queue(proxy, queue) -> void
	symWlProxyCreateWrapper               unsafe.Pointer // wl_proxy_create_wrapper(proxy) -> wrapper*
	symWlProxyWrapperDestroy              unsafe.Pointer // wl_proxy_wrapper_destroy(wrapper) -> void
	symWlDisplayRoundtripQueue            unsafe.Pointer // wl_display_roundtrip_queue(display, queue) -> int
	symWlEventQueueDestroy                unsafe.Pointer // wl_event_queue_destroy(queue) -> void

	// wl_interface pointers (loaded from libwayland-client.so data section)
	wlRegistryInterface unsafe.Pointer
	wlShmPoolInterface  unsafe.Pointer
	wlBufferInterface   unsafe.Pointer

	// CIF for wl_display_roundtrip(wl_display*) -> int
	cifWlDisplayRoundtrip types.CallInterface
	// CIF for wl_display_flush(wl_display*) -> int
	cifWlDisplayFlush types.CallInterface
	// CIF for wl_display_create_queue(wl_display*) -> wl_event_queue*
	cifWlDisplayCreateQueue types.CallInterface
	// CIF for wl_display_dispatch_queue_pending(wl_display*, wl_event_queue*) -> int
	cifWlDisplayDispatchQueueP types.CallInterface
	// CIF for wl_proxy_marshal_constructor(proxy*, opcode, interface*, ...) -> proxy*
	cifWlProxyMarshalConstructor types.CallInterface
	// CIF for wl_proxy_add_listener(proxy*, listener_impl**, data*) -> int
	cifWlProxyAddListener types.CallInterface
	// CIF for wl_proxy_destroy(proxy*) -> void
	cifWlProxyDestroy types.CallInterface
	// CIF for wl_proxy_set_queue(wl_proxy*, wl_event_queue*) -> void
	cifWlProxySetQueue types.CallInterface
	// CIF for wl_proxy_create_wrapper(void*) -> wl_proxy*
	cifWlProxyCreateWrapper types.CallInterface
	// CIF for wl_proxy_wrapper_destroy(void*) -> void
	cifWlProxyWrapperDestroy types.CallInterface
	// CIF for wl_display_roundtrip_queue(wl_display*, wl_event_queue*) -> int
	cifWlDisplayRoundtripQueue types.CallInterface
	// CIF for wl_event_queue_destroy(wl_event_queue*) -> void
	cifWlEventQueueDestroy types.CallInterface
)

// waylandBlitState holds per-Surface Wayland SHM resources for triple-buffered
// presentation. Embedded in platformBlit (blit_linux.go).
//
// Triple-buffering (3 buffers) guarantees a free buffer even when the
// compositor holds two (previous + current). Qt6 uses up to 5
// (qwaylandshmbackingstore.cpp); 3 is the minimum for skip-free
// presentation on slow compositors (e.g. pixman renderer).
type waylandBlitState struct {
	isWayland bool // true when SurfaceTargetKind is Wayland
	detected  bool // true if detection has been performed

	wlShm    uintptr // bound wl_shm global (on shmQueue, 0 if not yet obtained)
	registry uintptr // wl_registry proxy (on shmQueue, for proper cleanup)

	// shmQueue is a dedicated wl_event_queue for ALL software backend Wayland
	// proxies (registry, wl_shm, wl_shm_pool, wl_buffer). Created via
	// wl_display_create_queue. All proxies inherit shmQueue through the
	// wl_proxy_create_wrapper pattern (Qt6 qwaylanddisplay.cpp:335-341):
	// wrapper = wl_proxy_create_wrapper(display)
	// wl_proxy_set_queue(wrapper, shmQueue)
	// registry = wl_display_get_registry(wrapper) → on shmQueue
	// wl_shm = wl_registry_bind(registry) → on shmQueue
	// pool/buffer inherit from wl_shm → on shmQueue
	//
	// This eliminates queue mismatch with gogpu's appQueue (ADR-041):
	// gogpu dispatches ONLY appQueue, so objects on DEFAULT queue would
	// never receive events. See BUG-SW-WAYLAND-002.
	shmQueue uintptr // 0 until first buffer created

	// Triple-buffer state: three SHM buffers, pick first non-busy.
	// This avoids writing to a buffer the compositor is still reading.
	buffers  [3]waylandShmBuffer
	frontIdx int // index of the buffer last submitted to compositor
}

// waylandShmBuffer holds one SHM buffer for Wayland presentation.
type waylandShmBuffer struct {
	fd            int     // memfd file descriptor (-1 if unused)
	data          []byte  // mmap'd pixel data
	pool          uintptr // wl_shm_pool proxy
	buffer        uintptr // wl_buffer proxy
	width         int32
	height        int32
	busy          bool // true while compositor owns this buffer (Qt6 qwaylandbuffer.cpp pattern)
	needsFullCopy bool // true after allocation — first frame must be full copy
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
		{"wl_display_roundtrip", &symWlDisplayRoundtrip},
		{"wl_display_flush", &symWlDisplayFlush},
		{"wl_display_create_queue", &symWlDisplayCreateQueue},
		{"wl_display_dispatch_queue_pending", &symWlDisplayDispatchQueueP},
		{"wl_proxy_marshal_constructor", &symWlProxyMarshalConstructor},
		{"wl_proxy_add_listener", &symWlProxyAddListener},
		{"wl_proxy_marshal", &symWlProxyMarshal},
		{"wl_proxy_destroy", &symWlProxyDestroy},
		{"wl_proxy_set_queue", &symWlProxySetQueue},
		{"wl_proxy_create_wrapper", &symWlProxyCreateWrapper},
		{"wl_proxy_wrapper_destroy", &symWlProxyWrapperDestroy},
		{"wl_display_roundtrip_queue", &symWlDisplayRoundtripQueue},
		{"wl_event_queue_destroy", &symWlEventQueueDestroy},
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
	}
	for _, iface := range interfaces {
		*iface.dst, err = ffi.GetSymbol(wlClientLib, iface.name)
		if err != nil {
			slog.Debug("software: Wayland blit unavailable — missing interface", "interface", iface.name, "error", err)
			return
		}
	}

	// Prepare CIFs.

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

	// wl_event_queue* wl_display_create_queue(wl_display*)
	if err = ffi.PrepareCallInterface(&cifWlDisplayCreateQueue, types.DefaultCall,
		types.PointerTypeDescriptor,
		[]*types.TypeDescriptor{types.PointerTypeDescriptor}); err != nil {
		return
	}

	// int wl_display_dispatch_queue_pending(wl_display*, wl_event_queue*)
	if err = ffi.PrepareCallInterface(&cifWlDisplayDispatchQueueP, types.DefaultCall,
		types.SInt32TypeDescriptor,
		[]*types.TypeDescriptor{
			types.PointerTypeDescriptor, // display
			types.PointerTypeDescriptor, // queue
		}); err != nil {
		return
	}

	// wl_proxy* wl_proxy_marshal_constructor(wl_proxy*, uint32 opcode, wl_interface*, ...)
	// Variadic: 3 fixed args (proxy, opcode, interface), rest variadic.
	// nfixedargs=3 ensures correct ABI on ARM64 (Apple AAPCS64 variadic convention).
	if err = ffi.PrepareVariadicCallInterface(&cifWlProxyMarshalConstructor, types.DefaultCall,
		3, // nfixedargs: proxy, opcode, interface are fixed
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

	// void wl_proxy_destroy(wl_proxy*)
	if err = ffi.PrepareCallInterface(&cifWlProxyDestroy, types.DefaultCall,
		types.VoidTypeDescriptor,
		[]*types.TypeDescriptor{types.PointerTypeDescriptor}); err != nil {
		return
	}

	// void wl_proxy_set_queue(wl_proxy*, wl_event_queue*)
	if err = ffi.PrepareCallInterface(&cifWlProxySetQueue, types.DefaultCall,
		types.VoidTypeDescriptor,
		[]*types.TypeDescriptor{
			types.PointerTypeDescriptor, // proxy
			types.PointerTypeDescriptor, // queue
		}); err != nil {
		return
	}

	// wl_proxy* wl_proxy_create_wrapper(void* proxy)
	if err = ffi.PrepareCallInterface(&cifWlProxyCreateWrapper, types.DefaultCall,
		types.PointerTypeDescriptor,
		[]*types.TypeDescriptor{types.PointerTypeDescriptor}); err != nil {
		return
	}

	// void wl_proxy_wrapper_destroy(void* wrapper)
	if err = ffi.PrepareCallInterface(&cifWlProxyWrapperDestroy, types.DefaultCall,
		types.VoidTypeDescriptor,
		[]*types.TypeDescriptor{types.PointerTypeDescriptor}); err != nil {
		return
	}

	// int wl_display_roundtrip_queue(wl_display*, wl_event_queue*)
	if err = ffi.PrepareCallInterface(&cifWlDisplayRoundtripQueue, types.DefaultCall,
		types.SInt32TypeDescriptor,
		[]*types.TypeDescriptor{
			types.PointerTypeDescriptor, // display
			types.PointerTypeDescriptor, // queue
		}); err != nil {
		return
	}

	// void wl_event_queue_destroy(wl_event_queue*)
	if err = ffi.PrepareCallInterface(&cifWlEventQueueDestroy, types.DefaultCall,
		types.VoidTypeDescriptor,
		[]*types.TypeDescriptor{types.PointerTypeDescriptor}); err != nil {
		return
	}

	// Cached CIFs for per-frame calls (zero alloc after init).

	// marshal2: wl_proxy_marshal(proxy, opcode) — for commit, pool destroy, buffer destroy.
	// wl_proxy_marshal is variadic: nfixedargs=2 (proxy, opcode), no variadic args in this CIF.
	if err = ffi.PrepareVariadicCallInterface(&cifMarshal2, types.DefaultCall,
		2, // nfixedargs: proxy + opcode are fixed
		types.VoidTypeDescriptor,
		[]*types.TypeDescriptor{
			types.PointerTypeDescriptor, // proxy
			types.UInt32TypeDescriptor,  // opcode
		}); err != nil {
		return
	}

	// attach: wl_proxy_marshal(proxy, opcode, buffer, x, y) — wl_surface_attach.
	// wl_proxy_marshal is variadic: nfixedargs=2 (proxy, opcode), buffer/x/y are variadic.
	if err = ffi.PrepareVariadicCallInterface(&cifSurfaceAttach, types.DefaultCall,
		2, // nfixedargs: proxy + opcode are fixed
		types.VoidTypeDescriptor,
		[]*types.TypeDescriptor{
			types.PointerTypeDescriptor, // proxy (surface)
			types.UInt32TypeDescriptor,  // opcode (1)
			types.PointerTypeDescriptor, // buffer (variadic)
			types.SInt32TypeDescriptor,  // x (variadic)
			types.SInt32TypeDescriptor,  // y (variadic)
		}); err != nil {
		return
	}

	// damage_buffer: wl_proxy_marshal(proxy, opcode, x, y, w, h) — opcode 9.
	// wl_proxy_marshal is variadic: nfixedargs=2 (proxy, opcode), x/y/w/h are variadic.
	if err = ffi.PrepareVariadicCallInterface(&cifSurfaceDamageBuffer, types.DefaultCall,
		2, // nfixedargs: proxy + opcode are fixed
		types.VoidTypeDescriptor,
		[]*types.TypeDescriptor{
			types.PointerTypeDescriptor, // proxy (surface)
			types.UInt32TypeDescriptor,  // opcode (9)
			types.SInt32TypeDescriptor,  // x (variadic)
			types.SInt32TypeDescriptor,  // y (variadic)
			types.SInt32TypeDescriptor,  // w (variadic)
			types.SInt32TypeDescriptor,  // h (variadic)
		}); err != nil {
		return
	}

	waylandReady = true
	slog.Debug("software: Wayland SHM blit initialized")
}

// obtainWlShm gets the wl_shm global by creating a wl_registry, listening
// for the wl_shm global, and doing a roundtrip. Called eagerly from
// configurePlatformBlit (once per surface, on the main thread).
//
// All proxies (registry, wl_shm) are created through a display wrapper
// assigned to shmQueue — so they inherit shmQueue, NOT the default queue.
// This matches the Qt6 pattern (qwaylanddisplay.cpp:335-341):
//
//	wrapper = wl_proxy_create_wrapper(display)
//	wl_proxy_set_queue(wrapper, queue)
//	registry = wl_display_get_registry(wrapper)
//
// Without this, proxies end up on the DEFAULT queue which gogpu never
// dispatches (ADR-041), causing events to accumulate and crash.
// See BUG-SW-WAYLAND-002.
//
// Returns (wl_shm, wl_registry) — both on shmQueue. Caller stores registry
// for proper cleanup in destroyWaylandBlitState.
func obtainWlShm(display, shmQueue uintptr) (uintptr, uintptr) {
	waylandOnce.Do(initWayland)
	if !waylandReady {
		return 0, 0
	}

	wrapperArgs := [1]unsafe.Pointer{unsafe.Pointer(&display)}
	var wrapper uintptr
	_, _ = ffi.CallFunction(&cifWlProxyCreateWrapper, symWlProxyCreateWrapper, unsafe.Pointer(&wrapper), wrapperArgs[:])
	if wrapper == 0 {
		slog.Warn("software: wl_proxy_create_wrapper failed")
		return 0, 0
	}
	setQueueArgs := [2]unsafe.Pointer{
		unsafe.Pointer(&wrapper),
		unsafe.Pointer(&shmQueue),
	}
	_, _ = ffi.CallFunction(&cifWlProxySetQueue, symWlProxySetQueue, nil, setQueueArgs[:])

	// wl_display_get_registry via WRAPPER → registry inherits shmQueue.
	var opcode uint32 = 1
	var null uintptr
	ifacePtr := uintptr(wlRegistryInterface)
	args := [4]unsafe.Pointer{
		unsafe.Pointer(&wrapper), // wrapper, NOT raw display
		unsafe.Pointer(&opcode),
		unsafe.Pointer(&ifacePtr),
		unsafe.Pointer(&null),
	}
	var registry uintptr
	_, _ = ffi.CallFunction(&cifWlProxyMarshalConstructor, symWlProxyMarshalConstructor, unsafe.Pointer(&registry), args[:])

	// Wrapper no longer needed — child proxies already inherited the queue.
	destroyWrapperArgs := [1]unsafe.Pointer{unsafe.Pointer(&wrapper)}
	_, _ = ffi.CallFunction(&cifWlProxyWrapperDestroy, symWlProxyWrapperDestroy, nil, destroyWrapperArgs[:])

	if registry == 0 {
		slog.Warn("software: wl_display_get_registry failed")
		return 0, 0
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
	var listenerData uintptr
	addArgs := [3]unsafe.Pointer{
		unsafe.Pointer(&registry),
		unsafe.Pointer(&listenerPtr),
		unsafe.Pointer(&listenerData),
	}
	var addResult int32
	_, _ = ffi.CallFunction(&cifWlProxyAddListener, symWlProxyAddListener, unsafe.Pointer(&addResult), addArgs[:])

	// Roundtrip on shmQueue (not default) to receive registry events.
	roundtripArgs := [2]unsafe.Pointer{
		unsafe.Pointer(&display),
		unsafe.Pointer(&shmQueue),
	}
	var rtResult int32
	// wl_display_roundtrip_queue returns -1 on error and sets errno.
	// EPIPE/ECONNRESET = compositor dead, EPROTO = protocol error.
	// Qt6 qwaylanddisplay.cpp and SDL3 SDL_waylandevents.c check errno here.
	rtErrno, _ := ffi.CallFunction(&cifWlDisplayRoundtripQueue, symWlDisplayRoundtripQueue, unsafe.Pointer(&rtResult), roundtripArgs[:])
	if rtResult < 0 {
		slog.Warn("software: wl_display_roundtrip_queue failed (init registry)", "errno", rtErrno, "errstr", rtErrno.Error())
	}

	pendingShmBindMu.Lock()
	shmName := pendingShmBindName
	pendingShmBindMu.Unlock()

	if shmName == 0 {
		slog.Warn("software: wl_shm not found in registry")
		destroyArgs := [1]unsafe.Pointer{unsafe.Pointer(&registry)}
		_, _ = ffi.CallFunction(&cifWlProxyDestroy, symWlProxyDestroy, nil, destroyArgs[:])
		return 0, 0
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
		_, _ = ffi.CallFunction(&cifWlProxyDestroy, symWlProxyDestroy, nil, destroyArgs[:])
		return 0, 0
	}

	// Prepare CIF for versioned: wl_proxy*(proxy, opcode, interface, version, name, ifaceName, version, NULL)
	// The actual signature for wl_registry_bind is:
	//   wl_proxy_marshal_constructor_versioned(registry, 0, &wl_shm_interface, 1, name, "wl_shm", 1, NULL)
	// Variadic: nfixedargs=4 (proxy, opcode, interface, version), rest variadic.
	var cifVersioned types.CallInterface
	if err := ffi.PrepareVariadicCallInterface(&cifVersioned, types.DefaultCall,
		4, // nfixedargs: proxy, opcode, interface, version are fixed
		types.PointerTypeDescriptor,
		[]*types.TypeDescriptor{
			types.PointerTypeDescriptor, // proxy (registry)
			types.UInt32TypeDescriptor,  // opcode (0 = bind)
			types.PointerTypeDescriptor, // interface (&wl_shm_interface)
			types.UInt32TypeDescriptor,  // version
			types.UInt32TypeDescriptor,  // name (variadic)
			types.PointerTypeDescriptor, // interface_name string (variadic)
			types.UInt32TypeDescriptor,  // version (variadic, repeated)
			types.PointerTypeDescriptor, // NULL terminator (variadic)
		}); err != nil {
		destroyArgs := [1]unsafe.Pointer{unsafe.Pointer(&registry)}
		_, _ = ffi.CallFunction(&cifWlProxyDestroy, symWlProxyDestroy, nil, destroyArgs[:])
		return 0, 0
	}

	// Get wl_shm_interface pointer.
	var symShmInterface unsafe.Pointer
	symShmInterface, _ = ffi.GetSymbol(wlClientLib, "wl_shm_interface")
	if symShmInterface == nil {
		slog.Warn("software: wl_shm_interface not found")
		destroyArgs := [1]unsafe.Pointer{unsafe.Pointer(&registry)}
		_, _ = ffi.CallFunction(&cifWlProxyDestroy, symWlProxyDestroy, nil, destroyArgs[:])
		return 0, 0
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
	_, _ = ffi.CallFunction(&cifVersioned, symVersioned, unsafe.Pointer(&shm), bindArgs[:])

	// Roundtrip on shmQueue to ensure bind completes.
	rtErrno, _ = ffi.CallFunction(&cifWlDisplayRoundtripQueue, symWlDisplayRoundtripQueue, unsafe.Pointer(&rtResult), roundtripArgs[:])
	if rtResult < 0 {
		slog.Warn("software: wl_display_roundtrip_queue failed (init bind)", "errno", rtErrno, "errstr", rtErrno.Error())
	}

	if shm == 0 {
		slog.Warn("software: wl_registry_bind for wl_shm failed")
		destroyArgs := [1]unsafe.Pointer{unsafe.Pointer(&registry)}
		_, _ = ffi.CallFunction(&cifWlProxyDestroy, symWlProxyDestroy, nil, destroyArgs[:])
		return 0, 0
	}
	slog.Debug("software: wl_shm bound successfully", "shm", shm, "queue", "shmQueue")
	return shm, registry
}

// createShmQueue creates a dedicated wl_event_queue for SHM buffer proxies.
// Returns 0 if Wayland is not ready or queue creation fails.
func createShmQueue(display uintptr) uintptr {
	waylandOnce.Do(initWayland)
	if !waylandReady {
		return 0
	}
	var queue uintptr
	args := [1]unsafe.Pointer{unsafe.Pointer(&display)}
	_, _ = ffi.CallFunction(&cifWlDisplayCreateQueue, symWlDisplayCreateQueue, unsafe.Pointer(&queue), args[:])
	if queue == 0 {
		slog.Warn("software: wl_display_create_queue failed for SHM buffers")
	}
	return queue
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

// cString reads a null-terminated C string from a uintptr (C char*).
// Uses unsafe.Pointer conversion required for FFI interop with libwayland.
//
//go:nosplit
//go:nocheckptr
func cString(ptr uintptr) string {
	if ptr == 0 {
		return ""
	}
	p := *(*unsafe.Pointer)(unsafe.Pointer(&ptr))
	bp := (*byte)(p)
	length := 0
	for i := 0; i < 256; i++ {
		b := unsafe.Slice(bp, i+1)
		if b[i] == 0 {
			length = i
			break
		}
	}
	if length == 0 {
		return ""
	}
	result := unsafe.Slice(bp, length)
	return string(result)
}

// waylandCreateShmBuffer creates a new SHM buffer for the given dimensions.
// The buffer proxy inherits shmQueue from wl_shm via the display wrapper
// pattern (BUG-SW-WAYLAND-002) — no manual wl_proxy_set_queue needed.
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
	// Variadic: nfixedargs=3 (proxy, opcode, interface), rest variadic.
	var cifCreatePool types.CallInterface
	if err := ffi.PrepareVariadicCallInterface(&cifCreatePool, types.DefaultCall,
		3, // nfixedargs: proxy, opcode, interface are fixed
		types.PointerTypeDescriptor,
		[]*types.TypeDescriptor{
			types.PointerTypeDescriptor, // proxy (shm)
			types.UInt32TypeDescriptor,  // opcode
			types.PointerTypeDescriptor, // interface
			types.PointerTypeDescriptor, // new_id (variadic, NULL placeholder)
			types.PointerTypeDescriptor, // fd (variadic)
			types.PointerTypeDescriptor, // size (variadic)
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
	_, _ = ffi.CallFunction(&cifCreatePool, symWlProxyMarshalConstructor, unsafe.Pointer(&pool), poolArgs[:])
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

	// Variadic: nfixedargs=3 (proxy, opcode, interface), rest variadic.
	var cifCreateBuffer types.CallInterface
	if err := ffi.PrepareVariadicCallInterface(&cifCreateBuffer, types.DefaultCall,
		3, // nfixedargs: proxy, opcode, interface are fixed
		types.PointerTypeDescriptor,
		[]*types.TypeDescriptor{
			types.PointerTypeDescriptor, // proxy (pool)
			types.UInt32TypeDescriptor,  // opcode
			types.PointerTypeDescriptor, // interface
			types.PointerTypeDescriptor, // new_id (variadic, NULL placeholder)
			types.PointerTypeDescriptor, // offset (variadic)
			types.PointerTypeDescriptor, // width (variadic)
			types.PointerTypeDescriptor, // height (variadic)
			types.PointerTypeDescriptor, // stride (variadic)
			types.PointerTypeDescriptor, // format (variadic)
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
	_, _ = ffi.CallFunction(&cifCreateBuffer, symWlProxyMarshalConstructor, unsafe.Pointer(&buffer), bufArgs[:])
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
		fd:            fd,
		data:          data,
		pool:          0, // already destroyed
		buffer:        buffer,
		width:         width,
		height:        height,
		needsFullCopy: true, // first frame must be full copy
	}

	// Register wl_buffer.release listener (Qt6 qwaylandbuffer.cpp pattern).
	// wl_buffer has one event: release (opcode 0). The listener array has 1 slot.
	bufferListenerOnce.Do(func() {
		bufferListenerFuncs[0] = ffi.NewCallback(bufferReleaseCb)
	})

	// NOTE: do NOT register in bufferBusyMap here — the caller does
	// *buf = *newBuf (value copy into wl.buffers[idx]). The map must point
	// to &wl.buffers[idx], not to this local buf. Caller registers after copy.

	listenerPtr := uintptr(unsafe.Pointer(&bufferListenerFuncs[0]))
	var listenerData uintptr
	addArgs := [3]unsafe.Pointer{
		unsafe.Pointer(&buffer),
		unsafe.Pointer(&listenerPtr),
		unsafe.Pointer(&listenerData),
	}
	var addResult int32
	_, _ = ffi.CallFunction(&cifWlProxyAddListener, symWlProxyAddListener, unsafe.Pointer(&addResult), addArgs[:])

	// Buffer proxy inherits shmQueue from wl_shm (via pool) through the
	// display wrapper pattern (BUG-SW-WAYLAND-002). No wl_proxy_set_queue
	// needed — queue assignment is automatic via libwayland proxy_create
	// (wayland-client.c:492: proxy->queue = factory->queue).

	return buf
}

// destroyPool calls wl_shm_pool::destroy (opcode 1) then wl_proxy_destroy.
func destroyPool(pool uintptr) {
	var destroyOpcode uint32 = 1
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&pool),
		unsafe.Pointer(&destroyOpcode),
	}
	_, _ = ffi.CallFunction(&cifMarshal2, symWlProxyMarshal, nil, args[:])

	destroyArgs := [1]unsafe.Pointer{unsafe.Pointer(&pool)}
	_, _ = ffi.CallFunction(&cifWlProxyDestroy, symWlProxyDestroy, nil, destroyArgs[:])
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
		_, _ = ffi.CallFunction(&cifMarshal2, symWlProxyMarshal, nil, marshalArgs[:])

		destroyArgs := [1]unsafe.Pointer{unsafe.Pointer(&buf.buffer)}
		_, _ = ffi.CallFunction(&cifWlProxyDestroy, symWlProxyDestroy, nil, destroyArgs[:])
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
// wl_shm must be obtained eagerly (in blit_linux.go detection block) before
// this method is called. If wlShm is 0, init failed and we bail out.
func (s *Surface) waylandPresent(data []byte, width, height int32) {
	wl := &s.wlState
	if wl.wlShm == 0 {
		return
	}

	// Pick a non-busy buffer (Qt6 qwaylandshmbackingstore.cpp pattern).
	backIdx := -1
	for i := 1; i <= len(wl.buffers); i++ {
		idx := (wl.frontIdx + i) % len(wl.buffers)
		if !wl.buffers[idx].busy {
			backIdx = idx
			break
		}
	}
	if backIdx < 0 {
		return
	}
	buf := &wl.buffers[backIdx]

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
		// Register AFTER value copy — buf points to wl.buffers[backIdx],
		// so release callback sets busy=false on the actual array element.
		bufferBusyMu.Lock()
		bufferBusyMap[buf.buffer] = buf
		bufferBusyMu.Unlock()
	}

	copy(buf.data[:min(len(data), len(buf.data))], data[:min(len(data), len(buf.data))])

	surface := s.hwnd
	waylandSurfaceAttach(surface, buf.buffer, 0, 0)
	waylandSurfaceDamageBuffer(surface, 0, 0, width, height)
	waylandSurfaceCommit(surface)

	buf.busy = true

	// wl_display_flush sends the commit to the compositor.
	// Returns bytes sent or -1 with errno. EAGAIN = compositor backpressure
	// (Qt6 polls POLLOUT and retries), EPIPE = compositor dead.
	flushArgs := [1]unsafe.Pointer{unsafe.Pointer(&s.displayHandle)}
	var flushResult int32
	flushErrno, _ := ffi.CallFunction(&cifWlDisplayFlush, symWlDisplayFlush, unsafe.Pointer(&flushResult), flushArgs[:])
	if flushResult < 0 && flushErrno != syscall.EAGAIN {
		slog.Warn("software: wl_display_flush failed", "errno", flushErrno, "errstr", flushErrno.Error())
	}

	waylandDispatchShmQueue(s.displayHandle, wl.shmQueue)

	wl.frontIdx = backIdx
}

// waylandPresentDamage copies pixel data and commits with damage rects.
// wl_shm must be obtained eagerly (in blit_linux.go detection block) before
// this method is called. If wlShm is 0, init failed and we bail out.
func (s *Surface) waylandPresentDamage(data []byte, width, height int32, rects []image.Rectangle) {
	wl := &s.wlState
	if wl.wlShm == 0 {
		return
	}

	// Pick a non-busy buffer (same logic as waylandPresent).
	backIdx := -1
	for i := 1; i <= len(wl.buffers); i++ {
		idx := (wl.frontIdx + i) % len(wl.buffers)
		if !wl.buffers[idx].busy {
			backIdx = idx
			break
		}
	}
	if backIdx < 0 {
		return
	}
	buf := &wl.buffers[backIdx]

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
		bufferBusyMu.Lock()
		bufferBusyMap[buf.buffer] = buf
		bufferBusyMu.Unlock()
	}

	// Copy pixels: full copy for new buffers or when no damage rects provided,
	// partial row-based copy for damaged regions only.
	waylandCopyPixels(buf, data, width, height, rects)

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
	flushErrno, _ := ffi.CallFunction(&cifWlDisplayFlush, symWlDisplayFlush, unsafe.Pointer(&flushResult), flushArgs[:])
	if flushResult < 0 && flushErrno != syscall.EAGAIN {
		slog.Warn("software: wl_display_flush failed (damage)", "errno", flushErrno, "errstr", flushErrno.Error())
	}

	// Dispatch pending release events on the SHM queue.
	waylandDispatchShmQueue(s.displayHandle, wl.shmQueue)

	wl.frontIdx = backIdx
}

// waylandCopyPixels copies pixel data into a SHM buffer. On first use
// (needsFullCopy) or when no damage rects are provided, a full-frame copy
// is performed. Otherwise only the damaged rows are copied.
func waylandCopyPixels(buf *waylandShmBuffer, data []byte, width, height int32, rects []image.Rectangle) {
	if buf.needsFullCopy || len(rects) == 0 {
		n := int(width) * int(height) * 4
		if n > len(data) {
			n = len(data)
		}
		if n > len(buf.data) {
			n = len(buf.data)
		}
		copy(buf.data[:n], data[:n])
		buf.needsFullCopy = false
		return
	}

	// Row-based partial copy — only damaged regions.
	stride := int(width) * 4
	bounds := image.Rect(0, 0, int(width), int(height))
	for _, r := range rects {
		r = r.Intersect(bounds)
		if r.Empty() {
			continue
		}
		rowBytes := r.Dx() * 4
		for y := r.Min.Y; y < r.Max.Y; y++ {
			off := y*stride + r.Min.X*4
			end := off + rowBytes
			if end > len(data) || end > len(buf.data) {
				continue
			}
			copy(buf.data[off:end], data[off:end])
		}
	}
}

// waylandSurfaceCommit calls wl_surface_commit (opcode 6).
func waylandSurfaceCommit(surface uintptr) {
	var opcode uint32 = 6
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&surface),
		unsafe.Pointer(&opcode),
	}
	_, _ = ffi.CallFunction(&cifMarshal2, symWlProxyMarshal, nil, args[:])
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
	_, _ = ffi.CallFunction(&cifSurfaceAttach, symWlProxyMarshal, nil, args[:])
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
	_, _ = ffi.CallFunction(&cifSurfaceDamageBuffer, symWlProxyMarshal, nil, args[:])
}

// waylandDispatchShmQueue reads and dispatches events on the SHM queue.
// Uses wl_display_roundtrip_queue which does prepare_read → read_events →
// dispatch_queue — ensuring release events are actually read from the
// socket, not just dispatched from an empty queue.
//
// wl_display_dispatch_queue_pending (the previous approach) only dispatches
// already-read events. But nobody reads events into shmQueue — gogpu's
// read_events routes events to their queue, but the render thread calling
// dispatch_queue_pending may run before main thread reads. Result: empty
// queue, release callbacks never fire, all 3 buffers stuck busy=true.
func waylandDispatchShmQueue(display, queue uintptr) {
	if queue == 0 {
		return
	}
	args := [2]unsafe.Pointer{
		unsafe.Pointer(&display),
		unsafe.Pointer(&queue),
	}
	var result int32
	rtErrno, _ := ffi.CallFunction(&cifWlDisplayRoundtripQueue, symWlDisplayRoundtripQueue, unsafe.Pointer(&result), args[:])
	if result < 0 {
		slog.Warn("software: wl_display_roundtrip_queue failed (dispatch)", "errno", rtErrno, "errstr", rtErrno.Error())
	}
}

// destroyWaylandBlitState releases all Wayland SHM resources for a surface.
// Destruction order: buffers → wl_shm → registry → shmQueue.
// Proxies must be destroyed before the queue they're assigned to.
func (s *Surface) destroyWaylandBlitState() {
	wl := &s.wlState
	for i := range wl.buffers {
		waylandDestroyShmBuffer(&wl.buffers[i])
		wl.buffers[i] = waylandShmBuffer{fd: -1}
	}
	// Destroy wl_shm proxy (was leaked before BUG-SW-WAYLAND-002 fix).
	if wl.wlShm != 0 {
		args := [1]unsafe.Pointer{unsafe.Pointer(&wl.wlShm)}
		_, _ = ffi.CallFunction(&cifWlProxyDestroy, symWlProxyDestroy, nil, args[:])
		wl.wlShm = 0
	}
	// Destroy registry proxy (was leaked before BUG-SW-WAYLAND-002 fix).
	if wl.registry != 0 {
		args := [1]unsafe.Pointer{unsafe.Pointer(&wl.registry)}
		_, _ = ffi.CallFunction(&cifWlProxyDestroy, symWlProxyDestroy, nil, args[:])
		wl.registry = 0
	}
	// Destroy the SHM event queue LAST — all proxies assigned to it
	// must be destroyed first (wl_event_queue_destroy asserts this).
	if wl.shmQueue != 0 {
		args := [1]unsafe.Pointer{unsafe.Pointer(&wl.shmQueue)}
		_, _ = ffi.CallFunction(&cifWlEventQueueDestroy, symWlEventQueueDestroy, nil, args[:])
		wl.shmQueue = 0
	}
}
