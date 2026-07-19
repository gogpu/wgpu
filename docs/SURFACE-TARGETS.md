# Surface targets

Surface creation follows the safe/unsafe ownership split in Rust `wgpu`
v29.0.3, pinned at
[`4cbe6232b2d7c289b6e1a38416a6ae1461a22e81`](https://github.com/gfx-rs/wgpu/tree/4cbe6232b2d7c289b6e1a38416a6ae1461a22e81).
The Go API is additive: the existing two-`uintptr` `CreateSurface` method remains
available as a compatibility adapter, while new code can name its platform
target and ownership mode explicitly.

## Exhaustive API mapping

This map covers every exported production symbol added or behaviorally changed
by this follow-up relative to PR #268. Test-only declarations are excluded.
Build-tagged implementations of the same package selector are one logical Go
symbol. The Rust references are the pinned v29.0.3 sources for
[`Instance::create_surface`](https://github.com/gfx-rs/wgpu/blob/4cbe6232b2d7c289b6e1a38416a6ae1461a22e81/wgpu/src/api/instance.rs#L175-L280),
[`SurfaceTarget` and `SurfaceTargetUnsafe`](https://github.com/gfx-rs/wgpu/blob/4cbe6232b2d7c289b6e1a38416a6ae1461a22e81/wgpu/src/api/surface.rs#L254-L458),
and [core surface creation](https://github.com/gfx-rs/wgpu/blob/4cbe6232b2d7c289b6e1a38416a6ae1461a22e81/wgpu-core/src/instance.rs#L218-L290).

### Root `wgpu` package

| Go symbol | Rust analogue or explicit Go adaptation | Ownership and error contract |
|-----------|------------------------------------------|------------------------------|
| `ErrInvalidSurfaceTarget` | `CreateSurfaceErrorKind::RawHandle`, represented as an `errors.Is` sentinel because Go has no raw-window-handle error type | Returned for an empty target, typed-nil provider, unknown kind, or missing required handle before a backend call |
| `ErrUnsupportedSurfaceTarget` | `CreateSurfaceError::FailedToCreateSurfaceForAnyBackend`, split from malformed-input errors so callers can distinguish platform support | Returned when an implementation or every enabled native backend rejects the target kind; backend detail remains wrapped |
| `SurfaceTarget` | Safe `SurfaceTarget<'window>` | A provider is sampled exactly once; the provider, not merely its raw result, is retained through backend destruction |
| `SurfaceTarget.SurfaceTarget` | `Into<SurfaceTarget>` followed by raw-handle extraction | May return an application error; its identity is preserved with wrapping; it is never called after instance release |
| `SurfaceTargetUnsafe` | `SurfaceTargetUnsafe::RawHandle` | Opaque closed value prevents callers from inventing a kind/handle mismatch; retains no ownership source |
| `SurfaceTargetFromWindowsHWND` | `RawWindowHandle::Win32` plus the optional Windows display/module handle | `HWND` is required at creation; caller owns both handles through release |
| `SurfaceTargetFromXlibWindow` | `RawDisplayHandle::Xlib` plus `RawWindowHandle::Xlib` | Both `Display*` and `Window` are required and caller-owned |
| `SurfaceTargetFromWaylandSurface` | `RawDisplayHandle::Wayland` plus `RawWindowHandle::Wayland` | Both `wl_display*` and `wl_surface*` are required and caller-owned |
| `SurfaceTargetFromAndroidNativeWindow` | `RawWindowHandle::AndroidNdk`; Rust Vulkan also ignores the display variant | `ANativeWindow*` is required; caller keeps its native reference alive through release |
| `SurfaceTargetFromMetalLayer` | Rust core's Metal-layer creation path, adapted to the repository's existing `CAMetalLayer*` convention | Layer is required; the Metal HAL takes its own Objective-C retain while the unsafe Go call retains no provider |
| `SurfaceTargetFromWebCanvasID` | `RawWindowHandle::Web` numeric DOM identifier | Zero intentionally preserves the first-canvas convention; non-browser implementations return the unsupported sentinel |
| `(*Instance).CreateSurfaceFromTarget` | Safe `Instance::create_surface` | Checks instance lifetime first, samples and validates once, retains the provider on success, and clears it only after surface teardown |
| `(*Instance).CreateSurfaceUnsafe` | `unsafe Instance::create_surface_unsafe` | Go cannot mark a method unsafe, so the name, opaque constructors, validation, and documentation carry the contract; no provider is retained |
| `(*Instance).CreateSurface` | Go-only source-compatibility adapter to the typed unsafe path | Retains no provider; Linux alone keeps legacy environment-based Xlib/Wayland selection; new code should use an explicit target |
| `(*Instance).RequestAdapter` with `CompatibleSurface` | Rust `RequestAdapterOptions::compatible_surface` | Native dispatch supplies the surface created by each candidate adapter's own backend; a missing backend surface makes that backend incompatible |
| `(*Adapter).GetSurfaceCapabilities` | `Surface::get_capabilities`, which resolves `surface.raw(adapter.backend())` | Never substitutes the currently active surface for another backend; missing same-backend state reports no capabilities |
| `(*Surface).Configure` backend selection | Rust core's `surface_per_backend` selection by device backend | Reuses the retained surface for that backend and leaves other backend surfaces owned but inactive |
| `(*Surface).Release` | Rust `Surface` drop and `_handle_source` field order | Destroys the active and inactive backend surfaces before clearing the safe provider; idempotent |
| `(*Instance).Release` documentation | Rust surfaces have independent lifetimes | Native instances retire tracked surfaces; Rust-tag and browser surfaces still require explicit release, now stated without a false cascading promise |

### Exported integration seams

These packages are advanced implementation surfaces, but they are importable Go
packages and therefore still require an explicit parity rationale. Rust's
corresponding layers are the ordered `instance_per_backend` / `surface_per_backend`
maps in core and the [`wgpu-hal::Instance::create_surface`](https://github.com/gfx-rs/wgpu/blob/4cbe6232b2d7c289b6e1a38416a6ae1461a22e81/wgpu-hal/src/lib.rs#L648-L662)
backend trait.

| Go symbol | Rust analogue or explicit Go adaptation | Ownership and error contract |
|-----------|------------------------------------------|------------------------------|
| `hal.ErrUnsupportedSurfaceTarget` | `wgpu_hal::InstanceError` for an incompatible raw-window-handle variant, made classifiable with `errors.Is` | Must be returned before a foreign target's integers are interpreted as platform pointers |
| `hal.SurfaceTargetKind` | The discriminator of `RawWindowHandle` | Closed enum used only below the public opaque target; no ownership |
| `hal.SurfaceTargetInvalid` | Go validation sentinel; no Rust payload analogue | Never reaches a platform API |
| `hal.SurfaceTargetHeadless` | Go software/noop extension | Carries no handles; accepted only by backends with an explicit headless contract |
| `hal.SurfaceTargetWindowsHWND` | `RawWindowHandle::Win32` | Selects Win32 consumers only |
| `hal.SurfaceTargetXlibWindow` | `RawWindowHandle::Xlib` | Selects Xlib consumers independently of environment variables |
| `hal.SurfaceTargetWaylandSurface` | `RawWindowHandle::Wayland` | Selects Wayland consumers independently of environment variables |
| `hal.SurfaceTargetAndroidNativeWindow` | `RawWindowHandle::AndroidNdk` | Selects `ANativeWindow*`; display is ignored |
| `hal.SurfaceTargetMetalLayer` | Rust Metal-layer surface path | Selects `CAMetalLayer*` |
| `hal.SurfaceTarget` and fields `Kind`, `DisplayHandle`, `WindowHandle` | Go representation of Rust's typed display/window-handle pair | Borrowed data only; HAL does not receive a Go ownership source and must reject a mismatched kind before pointer use |
| `hal.SurfaceTarget.RequireKind` | A Rust `match` arm on `RawWindowHandle` | Wraps `hal.ErrUnsupportedSurfaceTarget`; performs no I/O or pointer access |
| `hal.SurfaceTargetKind.String` | `Debug` formatting of raw-window-handle variants | Stable diagnostics only; unknown numeric values remain printable and unsupported |
| `hal.Instance.CreateSurface` | `wgpu_hal::Instance::create_surface` | Signature intentionally changes from two unlabelled integers to one typed borrowed target; platform failures remain backend errors |
| `hal/dx12.(*Instance).CreateSurface` | Rust DX12 Win32 surface creation | Accepts only `WindowsHWND`; stores a borrowed HWND and rejects other kinds first |
| `hal/gles.(*Instance).CreateSurface` | Rust GLES WGL/EGL surface creation | Windows accepts `WindowsHWND`; Linux accepts Xlib or Wayland and explicitly selects the matching EGL display; backend errors remain wrapped |
| `hal/metal.(*Instance).CreateSurface` | Rust Metal layer creation | Accepts only `MetalLayer`, validates nonzero, then retains/releases the Objective-C layer internally |
| `hal/noop.(*Instance).CreateSurface` | Go test-backend adaptation | Performs no native access and accepts the target without ownership; not a platform support claim |
| `hal/software.(*Instance).CreateSurface` | Go CPU-presenter adaptation | Accepts only headless or host-appropriate kinds, stores the kind with the handles, and rejects foreign kinds before deferred blit setup |
| `hal/vulkan.Backend.CreateInstance` | Rust Vulkan instance creation enables the loader-supported WSI set rather than selecting one ABI from session state | Enumerates extensions before instance creation, preserves deterministic candidate order, and returns loader/enumeration failures without acquiring window ownership |
| `hal/vulkan.(*Instance).CreateSurface` | Rust Vulkan's [raw-handle match](https://github.com/gfx-rs/wgpu/blob/4cbe6232b2d7c289b6e1a38416a6ae1461a22e81/wgpu-hal/src/vulkan/instance.rs#L879-L922) | Accepts only kinds supported by the build target; Android passes `ANativeWindow*` directly and ignores display; Vulkan errors remain wrapped |
| `core.HALInstanceEntry` and fields `Backend`, `Instance` | One ordered entry in Rust core's `instance_per_backend` | Core retains ownership; snapshots handed upward borrow instances until core destruction |
| `core.(*Instance).HALInstanceEntries` | Ordered iteration over `instance_per_backend` | Returns a copied slice in backend-priority order; copying transfers no HAL ownership |
| `core.(*Instance).RequestAdapterWithSurfaces` | Adapter selection against Rust core's backend-keyed `surface_per_backend` | Borrows the map for the call, gives each adapter only its same-backend surface, and falls back to ordinary selection only for an empty map |
| `core.(*Instance).RequestAdapterWithSurface` | Go-only compatibility wrapper for existing single-HAL callers | Borrows one surface and preserves historical single-backend behavior; root multi-backend code uses the map method |
| `egl.ContextConfig.WindowKind` | Rust EGL's display-handle-selected window-system interface | Optional pointer: `nil` means legacy detection, non-nil forces X11/Wayland/surfaceless; zero-value `ContextConfig` therefore cannot accidentally force X11 |
| `egl.DefaultContextConfig` | Default Rust EGL instance policy | Leaves `WindowKind` nil, so no native object or environment decision is captured early |
| `egl.NewContext` | Rust EGL initialization from an explicit display-handle variant | Honors an explicit kind without consulting opposing session variables; otherwise preserves detection; owns only any display connection it opens itself |
| `egl.GetEGLDisplay` | Existing Go auto-detection entry point | Remains source-compatible and delegates to the private explicit-kind mechanism; errors close any internally opened display owner |

### Canonical Rust-wrapper dependency

| Go symbol | Rust / WebGPU-native analogue | Ownership and error contract |
|-----------|--------------------------------|------------------------------|
| `go-webgpu/webgpu/wgpu.(*Instance).CreateSurfaceFromAndroidNativeWindow` | `WGPUSurfaceSourceAndroidNativeWindow` passed to `wgpuInstanceCreateSurface` | Android-only; rejects zero before FFI, keeps the wire descriptor live for the call, retains no `ANativeWindow` reference, and returns `WGPUError` for released instance or null result |

That wrapper method belongs in canonical `go-webgpu/webgpu`, not this
repository. Its PR and an immutable released revision must land before the
Rust-tag Android lane can be merged here; the WGPU follow-up does not vendor the
ABI struct or add a local `replace`.

Like Rust `wgpu`, the native implementation attempts surface creation for
every enabled backend and succeeds when at least one backend succeeds. It keeps
one raw surface per successful backend so `CompatibleSurface` qualification and
later configuration always use the surface created by that same backend.

Rust can express the target as an enum of `raw-window-handle` values and mark a
method `unsafe`. Go has neither tagged unions nor unsafe methods, so the Go
adaptation uses an opaque value with named constructors and validates it at the
API boundary. The target kind remains explicit all the way into HAL; backends
never infer Xlib versus Wayland from two unlabelled integers.

## Safe provider path

A provider converts an application-owned window object into a raw target:

```go
type AndroidWindow struct {
	nativeWindow uintptr
	// Fields that own or retain the native window belong here.
}

func (w *AndroidWindow) SurfaceTarget() (wgpu.SurfaceTargetUnsafe, error) {
	return wgpu.SurfaceTargetFromAndroidNativeWindow(w.nativeWindow), nil
}

surface, err := instance.CreateSurfaceFromTarget(window)
```

`CreateSurfaceFromTarget` calls `SurfaceTarget` exactly once. On success it
retains the provider until backend surface destruction has completed during
`Surface.Release`. The default native implementation also releases its tracked
surfaces from `Instance.Release`; Rust-tag and browser callers must release each
surface explicitly. This keeps the provider's Go ownership graph reachable.
The provider still must make the underlying native objects valid for that whole
lifetime; finalizers are not a substitute for an explicit platform lifetime
contract.

## Unsafe raw-target path

Use a raw target when the host already controls the native object's lifetime:

```go
surface, err := instance.CreateSurfaceUnsafe(
	wgpu.SurfaceTargetFromAndroidNativeWindow(nativeWindow),
)
```

No ownership source is retained. Every referenced display, window, surface, or
layer must remain valid until `Surface.Release` returns. A zero required handle
is rejected before any backend call.

Available constructors are:

| Constructor | Raw objects |
|-------------|-------------|
| `SurfaceTargetFromWindowsHWND` | optional `HINSTANCE`, required `HWND` |
| `SurfaceTargetFromXlibWindow` | required `Display*` and Xlib `Window` |
| `SurfaceTargetFromWaylandSurface` | required `wl_display*` and `wl_surface*` |
| `SurfaceTargetFromAndroidNativeWindow` | required `ANativeWindow*`; no display |
| `SurfaceTargetFromMetalLayer` | required `CAMetalLayer*` |
| `SurfaceTargetFromWebCanvasID` | numeric `data-raw-handle`; zero selects the first canvas |

Each implementation accepts only targets it can realize. A mismatched target
returns `ErrUnsupportedSurfaceTarget`; a malformed or empty target returns
`ErrInvalidSurfaceTarget`. Provider errors preserve `errors.Is` identity.
`ErrReleased` takes precedence when the instance has already been released, so
creation never evaluates a provider or target after instance teardown.

## Compatibility method

`Instance.CreateSurface(displayHandle, windowHandle)` adapts legacy handles to
the same typed implementation. It remains source-compatible, but new code
should prefer one of the explicit methods above. On Linux only, the legacy
method must still use `WAYLAND_DISPLAY` to distinguish Wayland from Xlib;
typed targets remove that ambiguity. On Android, `displayHandle` is ignored,
matching Rust's Vulkan mapping of `RawWindowHandle::AndroidNdk` directly to
`a_native_window`.

The browser-specific `CreateSurfaceFromCanvas(js.Value)` remains the preferred
path when the caller already has an `HTMLCanvasElement` or `OffscreenCanvas`.
