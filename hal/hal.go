// Package hal is the interop surface of wgpu's hardware abstraction layer: the
// native objects of the backend that serves a wgpu instance, adapter, device or
// surface, for applications that run their own Vulkan, Direct3D 12, Metal or
// OpenGL code on the same GPU device.
//
// # What this package is
//
// wgpu creates one HAL implementation per backend (Vulkan, Direct3D 12, Metal,
// OpenGL ES, the CPU rasterizer and the no-op test backend), and this package
// names the native objects each of them owns. The types are the graphics APIs'
// own — VkInstance, ID3D12Device, MTLDevice, EGLDisplay — whose ABI belongs to
// Khronos, Microsoft and Apple and cannot change under us. That is why this
// package is small, needs no implementation of its own, and freezes at v1.0
// together with the rest of the wgpu API.
//
// The package holds declarations only: the backend kinds and the handle
// structs. It imports nothing; every value is filled by wgpu's implementation.
//
// # Getting the handles
//
//	handles := device.HAL()
//	switch handles.Kind {
//	case hal.KindVulkan:
//		// handles.Vulkan.Device is the VkDevice, handles.Vulkan.Queue the VkQueue.
//	case hal.KindDX12:
//		// handles.DX12.Device is the ID3D12Device*.
//	}
//
// Handles is a snapshot filled at the moment of the call. Take a fresh value
// after a surface is reconfigured or a surface is recreated, and never cache
// one across a wgpu call that recreates the object it names.
//
// wgpu creates its instance, adapter and device itself and never adopts a
// caller's: these handles are the only native objects behind a wgpu object.
//
// # Ownership
//
// Every handle is borrowed. wgpu created the object, holds its reference count
// or its Vulkan/EGL ownership, and destroys it when the wgpu object it belongs
// to is released. A caller must not destroy, free, release or terminate a
// handle it receives here, and must not use one after that wgpu object has been
// released. A handle is a plain address: the garbage collector neither keeps
// the object alive nor notices the access.
//
// # Safety
//
// Using these objects bypasses wgpu's validation, its resource state tracking
// and its synchronization. wgpu and your code share one native device, and
// mistakes are use-after-free, GPU hangs or corrupted frames, not Go panics.
// The rules that hold for every handle:
//
//   - Do not destroy, free or release a handle, and do not call a function
//     that would (vkDestroyDevice, ID3D12Device::Release, -[NSObject release],
//     eglDestroyContext, wglDeleteContext).
//   - Do not wait for or reset the device or the queue in a way that races with
//     wgpu's submissions. Submit your own work only between wgpu's submits,
//     from the thread that drives wgpu's render loop.
//   - Do not use wgpu-owned resources in your commands: wgpu's trackers do not
//     see them. Create your own objects through the handles and keep them
//     yours.
//   - A handle does not keep a wgpu object alive. Stop using the handles before
//     releasing the wgpu object they came from.
//
// Each handle's own documentation adds what is specific to it.
package hal

// BackendKind identifies the graphics API whose native objects a Handles value
// carries.
//
// BackendKind is a closed enumeration. Kind values are explicit and never
// renumbered, so a value keeps its meaning in every release; a later release
// may add a value, which is not a breaking change. Code that switches on Kind
// must handle the values it knows and ignore the rest.
//
// The mapping to gputypes.Backend is by meaning, and the values are never
// converted by a numeric cast. KindVulkan, KindMetal, KindDX12, KindGLES and
// KindBrowser correspond to gputypes.BackendVulkan, gputypes.BackendMetal,
// gputypes.BackendDX12, gputypes.BackendGL and gputypes.BackendBrowserWebGPU;
// KindGLES is gputypes' GL, "OpenGL/OpenGL ES". KindSoftware corresponds to
// gputypes.BackendSoftware where that value exists. KindNoop and KindWebGPU
// have no gputypes counterpart — the no-op and software backends report
// gputypes.BackendEmpty there — and KindWebGPU exists only in a build with the
// "rust" tag. KindUnknown is the zero value where gputypes uses BackendEmpty
// for a no-op adapter.
type BackendKind uint8

const (
	// KindUnknown means no HAL object at all: the wgpu object has been
	// released, or was built without a HAL implementation. It is the zero
	// value, so the zero Handles is valid and names nothing.
	//
	// It does not mean "a backend without native objects": a software, no-op
	// or browser object reports its own Kind with zero handle fields.
	KindUnknown BackendKind = 0

	// KindVulkan is Vulkan: Vk* objects, in Handles.Vulkan.
	KindVulkan BackendKind = 1

	// KindMetal is Metal on macOS and iOS: Objective-C objects, in
	// Handles.Metal.
	KindMetal BackendKind = 2

	// KindDX12 is Direct3D 12 on Windows: COM interface pointers, in
	// Handles.DX12.
	KindDX12 BackendKind = 3

	// KindGLES is OpenGL ES or OpenGL 3.3+, through EGL or WGL: EGL and WGL
	// handles, in Handles.GLES. It is gputypes' GL ("OpenGL/OpenGL ES").
	KindGLES BackendKind = 4

	// KindBrowser is the browser's own WebGPU implementation, in a
	// GOOS=js GOARCH=wasm build. It has no native objects: the only handle is
	// a JavaScript GPUDevice, which this package does not expose, so Handles
	// carries the Kind only.
	//
	// It is not KindWebGPU: that one is wgpu-native, the "rust"-tagged build.
	KindBrowser BackendKind = 5

	// KindSoftware is the CPU rasterizer. It has no native objects: Handles
	// carries the kind only.
	KindSoftware BackendKind = 6

	// KindNoop is the no-op test backend. It has no native objects: Handles
	// carries the kind only.
	KindNoop BackendKind = 7

	// KindWebGPU is a build with the "rust" tag, where wgpu is a binding to
	// wgpu-native: the WGPU* handles of webgpu.h, in Handles.WebGPU. The
	// graphics API below them (Vulkan, Metal or Direct3D 12) is the one
	// wgpu-native selected; these are not that API's native objects.
	//
	// It is not KindBrowser: that one is the browser's WebGPU in a wasm build.
	KindWebGPU BackendKind = 8
)

// Handles carries the native objects of the backend that serves a wgpu
// resource. Obtain it from Instance.HAL, Adapter.HAL, Device.HAL or
// Surface.HAL; wgpu fills it, the caller reads it.
//
// Exactly one of the backend structs is populated — the one matching Kind; the
// others are zero. Every field is a borrowed handle: see the package
// documentation for the ownership, lifetime and threading rules.
//
// The zero Handles is valid and names nothing.
//
// Handles is comparable with == and allocates nothing. Construct it only with
// keyed fields: wgpu returns it by value, and later releases may add fields and
// whole backend structs, which only keyed literals survive (go vet's
// composites check reports an unkeyed literal of an imported struct).
type Handles struct {
	// Kind names the backend whose objects this value carries. It is set even
	// when the backend has no native objects (KindSoftware, KindNoop,
	// KindBrowser).
	Kind BackendKind

	// Vulkan carries the Vulkan objects. Set when Kind is KindVulkan.
	Vulkan VulkanHandles

	// DX12 carries the Direct3D 12 objects. Set when Kind is KindDX12.
	DX12 DX12Handles

	// Metal carries the Metal objects. Set when Kind is KindMetal.
	Metal MetalHandles

	// GLES carries the OpenGL objects. Set when Kind is KindGLES.
	GLES GLESHandles

	// WebGPU carries the wgpu-native objects of a "rust"-tagged build. Set when
	// Kind is KindWebGPU.
	WebGPU WebGPUHandles
}

// VulkanHandles carries the Vulkan objects behind a wgpu resource. wgpu created
// every one of them; none may be destroyed by the caller.
type VulkanHandles struct {
	// Instance is the VkInstance, as uintptr. Valid until the wgpu Instance
	// that produced it is released. It can be 0 in a Handles value taken from
	// an object that cannot name it.
	Instance uintptr

	// PhysicalDevice is the VkPhysicalDevice behind a wgpu adapter, device or
	// surface: the adapter's own, or the one the device was opened on.
	PhysicalDevice uintptr

	// Device is the VkDevice behind a wgpu device. Valid until that wgpu
	// Device is released.
	//
	// Vulkan requires host access to a VkDevice to be externally synchronized:
	// do not create or destroy objects on it concurrently with wgpu's own
	// creation and destruction on the same device. wgpu holds no device-wide
	// lock for you; the safe pattern is to do your device-level calls from the
	// thread that drives wgpu.
	Device uintptr

	// Queue is the VkQueue wgpu submits and presents on: the device's graphics
	// queue, and the queue wgpu's command buffers target.
	//
	// A VkQueue is not thread-safe, and wgpu serializes its own Submit and
	// Present with a mutex you cannot take. Do your own vkQueueSubmit calls
	// only between wgpu's submissions, from wgpu's render thread.
	Queue uintptr

	// QueueFamilyIndex is the family Queue belongs to. Create your own
	// VkCommandPool for it, and use it for VkImageSharingMode and the layout
	// barriers you record yourself. Valid whenever Queue is.
	QueueFamilyIndex uint32

	// GetInstanceProcAddr is wgpu's PFN_vkGetInstanceProcAddr, the loader
	// function it loaded: `PFN_vkVoidFunction (*)(VkInstance, const char*)`.
	// Pass 0 as the instance for global commands. A pure-Go caller needs an FFI
	// trampoline (goffi: CIF with uint64 + pointer, pointer return) to call it.
	GetInstanceProcAddr uintptr

	// GetDeviceProcAddr is wgpu's PFN_vkGetDeviceProcAddr,
	// `PFN_vkVoidFunction (*)(VkDevice, const char*)`. It returns nil for a
	// command of an extension or API version wgpu did not enable on the device,
	// which makes it the oracle for what may be called on Device.
	GetDeviceProcAddr uintptr

	// Surface is the VkSurfaceKHR behind a wgpu surface. Set only by
	// Surface.HAL. It stays valid while the surface is configured; take a fresh
	// Handles after a reconfigure.
	Surface uintptr
}

// DX12Handles carries the Direct3D 12 objects behind a wgpu resource. Every
// field is a raw COM interface pointer that wgpu holds a reference to. The
// caller must not call Release on one and must not use one after the wgpu
// object it came from is released. To keep one longer, call AddRef through its
// vtable and Release it yourself later.
//
// wgpu publishes no Direct3D binding package: the caller brings its own vtable
// (IUnknown's QueryInterface, AddRef and Release are the first three slots, in
// that order) and calls through it, or uses QueryInterface to reach any
// interface version not named here. See the Direct3D 12 documentation for the
// vtable layout of each interface.
type DX12Handles struct {
	// Factory is the IDXGIFactory6 behind a wgpu instance: create your own
	// swapchains, or query a newer factory through QueryInterface.
	Factory uintptr

	// Adapter is the IDXGIAdapter4 behind a wgpu adapter or device, e.g. for
	// its LUID (GetDesc3) or memory budget.
	Adapter uintptr

	// Device is the ID3D12Device behind a wgpu device. Direct3D 12 objects are
	// free-threaded, so calls on it from any goroutine are legal.
	Device uintptr

	// Queue is the ID3D12CommandQueue of type D3D12_COMMAND_LIST_TYPE_DIRECT
	// that wgpu executes and presents on. ExecuteCommandLists from your own
	// thread is legal; commands run in the order in which the calls reach the
	// runtime.
	Queue uintptr

	// SwapChain is the IDXGISwapChain4 of a wgpu surface. Set only by
	// Surface.HAL, and only while the surface is configured: wgpu may recreate
	// it on resize or reconfigure, so take a fresh Handles afterwards.
	SwapChain uintptr
}

// MetalHandles carries the Metal objects behind a wgpu resource. The fields are
// Objective-C object pointers (id) as uintptr. wgpu holds a reference to each;
// the caller must not release one and must not use one after the wgpu object it
// came from is released. To keep one longer, send it -retain and -release
// yourself. wgpu publishes no Objective-C bridge: a caller that sends messages
// brings its own (libobjc's objc_msgSend and sel_registerName).
type MetalHandles struct {
	// Device is the id<MTLDevice> behind a wgpu adapter, device or surface.
	// Metal has no separate instance or physical-device object.
	Device uintptr

	// CommandQueue is the id<MTLCommandQueue> wgpu commits on. It is
	// thread-safe, but command buffers from it run in the order they are
	// created, not committed: create yours after wgpu's for the frame if they
	// must run after them, or synchronize with your own MTLSharedEvent.
	CommandQueue uintptr

	// Layer is the CAMetalLayer of a wgpu surface, set only by Surface.HAL.
	// It is the layer the caller passed to CreateSurface, so the value only
	// confirms identity.
	Layer uintptr
}

// GLESHandles carries the OpenGL objects behind a wgpu resource.
//
// The GL context is wgpu's and lives on its render thread: wgpu makes it
// current around every GL call and keeps a cache of bound GL objects that no
// external call updates. A hand here therefore names the objects; it is not a
// license to issue GL calls (v1 has no public way to make the context current
// safely and no state-cache reset). Do not make the context current, do not
// make another context current on wgpu's thread, and do not destroy either.
//
// On Windows wgpu uses WGL, so the EGL fields are zero and HGLRC/HDC are set.
// Elsewhere the EGL fields are set and HGLRC/HDC are zero.
type GLESHandles struct {
	// EGLDisplay is the EGLDisplay wgpu initialized, as uintptr. Shared with
	// every other EGL user of the same native display in the process: wgpu
	// terminates it only when the last of its own users lets go.
	EGLDisplay uintptr

	// EGLContext is the EGLContext wgpu created.
	EGLContext uintptr

	// EGLConfig is the EGLConfig the context was created with. A second context
	// that must share objects with wgpu's needs it at eglCreateContext time.
	EGLConfig uintptr

	// EGLSurface is the EGLSurface a wgpu surface presents to. Set only by
	// Surface.HAL, and only while the surface is configured: it is recreated on
	// resize or reconfigure.
	EGLSurface uintptr

	// HGLRC is the OpenGL rendering context wgpu uses on Windows.
	HGLRC uintptr

	// HDC is the device context HGLRC is current on for device work on Windows:
	// the hidden window's DC, not the window the caller renders to.
	HDC uintptr
}

// WebGPUHandles carries the wgpu-native objects behind a wgpu resource in a
// build with the "rust" tag, as the uintptr handles of webgpu.h. wgpu-native
// owns them; the caller must not release one and must not use one after the
// wgpu object it came from is released.
type WebGPUHandles struct {
	// Instance is the WGPUInstance.
	Instance uintptr
	// Adapter is the WGPUAdapter.
	Adapter uintptr
	// Device is the WGPUDevice.
	Device uintptr
	// Queue is the WGPUQueue.
	Queue uintptr
}
