// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build linux && !(js && wasm)

package gles

import (
	"fmt"
	"runtime"
	"sync"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/gles/egl"
	"github.com/gogpu/wgpu/hal/gles/gl"
)

// vendorUnknown is the placeholder vendor name used when the actual GPU vendor
// cannot be determined (e.g., no surface available during adapter enumeration).
const vendorUnknown = "Unknown"

// driverOpenGL is the AdapterInfo.Driver string for GLES backends.
const driverOpenGL = "OpenGL"

// Backend implements hal.Backend for OpenGL ES / OpenGL 3.3+ on Linux.
type Backend struct{}

// NewBackend returns a GLES backend instance.
func NewBackend() Backend { return Backend{} }

// Variant returns the backend type identifier.
func (Backend) Variant() gputypes.Backend {
	return gputypes.BackendGL
}

// CreateInstance creates a new OpenGL instance with an optional EGL context.
// Attempts to create an EGL context at instance level (Rust wgpu-hal egl.rs:846
// parity) for adapter enumeration without a surface. Uses surfaceless/pbuffer
// context — same role as Windows hidden 1×1 HWND (v0.28.6).
//
// On Wayland, this may fail (EGL needs wl_display*) — that's OK, CreateSurface
// provides the proper context later. On X11/headless, this succeeds.
//
// With desc.Display set, the context is built on that display at init,
// Wayland included (Rust wgpu-hal egl.rs Instance::init with
// InstanceDescriptor.display). If that fails, the backend is absent for this
// Instance, as in Rust: the error is returned and logged.
func (Backend) CreateInstance(desc *hal.InstanceDescriptor) (hal.Instance, error) {
	if err := egl.Init(); err != nil {
		return nil, fmt.Errorf("gles: failed to initialize EGL: %w", err)
	}

	if desc != nil && !desc.Display.IsZero() {
		inst, err := newDisplayInstance(desc.Display)
		if err != nil {
			hal.Logger().Warn("gles: no context on the instance display, GLES backend absent", "err", err)
			return nil, err
		}
		return inst, nil
	}

	// Try to create instance-level EGL context (Rust wgpu-hal parity).
	// Skip on Wayland: surfaceless context would create GL objects (VAO, FBO) that
	// are invisible to the Surface's windowed context (GL objects not shared between
	// EGL contexts). Device/Queue must use the SAME context as the window surface.
	// On X11/headless, instance context IS the presentation context — safe to create.
	if egl.DetectWindowKind() == egl.WindowKindWayland {
		hal.Logger().Info("gles: skipping instance context on Wayland (surface provides context)")
		return &Instance{}, nil
	}

	config := egl.DefaultContextConfig()
	if forceGLES {
		config = gles30Config(config)
	}
	eglCtx, err := egl.NewContext(config)
	if err != nil {
		hal.Logger().Info("gles: instance context unavailable (expected on Wayland)", "err", err)
		return &Instance{}, nil
	}

	hal.Logger().Debug("gles: EGL context", "robustAccess", eglCtx.RobustAccess())

	glCtx, err := loadContextGL(eglCtx, config.GLES)
	if err != nil {
		hal.Logger().Warn("gles: instance context unusable", "err", err)
		return &Instance{}, nil
	}
	return &Instance{ctx: NewAdapterContext(eglCtx, glCtx), ctxKind: eglCtx.WindowKind()}, nil
}

// newDisplayInstance builds the instance context on the caller's display.
// The context is bound to that display: only surfaces of it share the
// context (Path B never runs for it), and the caller keeps it open until
// Instance.Destroy returns.
func newDisplayInstance(display hal.DisplayHandle) (*Instance, error) {
	var kind egl.WindowKind
	switch display.Kind {
	case hal.DisplayKindXlib:
		kind = egl.WindowKindX11
	case hal.DisplayKindWayland:
		kind = egl.WindowKindWayland
	default:
		return nil, fmt.Errorf("gles: unsupported instance display kind %d", display.Kind)
	}
	if display.Display == 0 {
		return nil, fmt.Errorf("gles: instance display is nil")
	}
	ctx, err := newDisplayContext(display.Display, kind)
	if err != nil {
		return nil, err
	}
	return &Instance{ctx: ctx, ctxKind: kind, ctxDisplay: display.Display}, nil
}

// loadContextGL loads the GL functions of a new EGL context. The context is
// current only while they load, on this pinned thread, and is released
// after: AdapterContext.Lock makes it current on whatever thread uses it next
// (Rust wgpu-hal src/gles/egl.rs pairs every make_current with an
// unmake_current). On failure the EGL context is destroyed.
func loadContextGL(eglCtx *egl.Context, gles bool) (*gl.Context, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := eglCtx.MakeCurrent(); err != nil {
		eglCtx.Destroy()
		return nil, fmt.Errorf("gles: failed to make context current: %w", err)
	}
	glCtx := &gl.Context{}
	if err := glCtx.Load(egl.GetGLProcAddress, gles); err != nil {
		eglCtx.Destroy()
		return nil, fmt.Errorf("gles: failed to load GL functions: %w", err)
	}
	hal.Logger().Info("gles: EGL context created",
		"version", glCtx.GetString(gl.VERSION),
		"renderer", glCtx.GetString(gl.RENDERER),
		"gles", gles,
		"windowKind", eglCtx.WindowKind())
	if err := eglCtx.ReleaseCurrent(); err != nil {
		eglCtx.Destroy()
		return nil, fmt.Errorf("gles: failed to release context: %w", err)
	}
	return glCtx, nil
}

// displayKey identifies a caller-owned native display a context is built on.
type displayKey struct {
	kind    egl.WindowKind
	display uintptr
}

// Instance implements hal.Instance for the OpenGL backend on Linux.
//
// ctx is the instance context, created at init on our own X connection, the
// surfaceless platform or the caller's instance display, and held strongly
// until Destroy. Contexts built
// in CreateSurface on a caller's display (Path B: Wayland, or a window kind
// the instance context does not serve) belong to their surfaces and devices
// (ADR-077 B). The Instance keeps only weak pointers to them, keyed by native
// display, so later surfaces of the same display share the context.
type Instance struct {
	mu         sync.Mutex
	ctx        *AdapterContext
	ctxKind    egl.WindowKind // window system of ctx
	ctxDisplay uintptr        // caller-owned native display of ctx, 0 for our own or surfaceless

	// displayContexts holds the Path B contexts weakly: no reference is taken.
	// A dead entry is dropped when a lookup of its key finds it, and every
	// dead entry when a new context is recorded.
	displayContexts map[displayKey]*AdapterContext
}

// context returns the instance AdapterContext.
func (i *Instance) context() *AdapterContext {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.ctx
}

// CreateSurface creates an OpenGL surface from window handles.
// On Linux: displayHandle and windowHandle are platform-specific.
// For X11: displayHandle is X11 Display*, windowHandle is Window.
// For Wayland: displayHandle is wl_display*, windowHandle is wl_surface*.
//
// The surface uses the first context that serves its target (ADR-077):
//  1. the instance context, by window kind. X11 XIDs are server-global, so a
//     window of the app's Display* works with a context on our connection.
//  2. a context an earlier surface built on the same native display;
//  3. a new context on the caller's display (Path B: Wayland, where EGL needs
//     the app's wl_display*, or a window kind the instance context does not
//     serve). It lives as long as the surfaces and devices that use it.
//
// The surface holds a reference to its context until Destroy.
func (i *Instance) CreateSurface(target hal.SurfaceTarget) (hal.Surface, error) {
	var targetWindowKind egl.WindowKind
	switch target.Kind {
	case hal.SurfaceTargetXlibWindow:
		targetWindowKind = egl.WindowKindX11
	case hal.SurfaceTargetWaylandSurface:
		targetWindowKind = egl.WindowKindWayland
	default:
		return nil, fmt.Errorf("gles: %w: got %s, backend requires Xlib window or Wayland surface", hal.ErrUnsupportedSurfaceTarget, target.Kind)
	}

	i.mu.Lock()
	defer i.mu.Unlock()

	ctx := i.lookupContextLocked(targetWindowKind, target.DisplayHandle)
	if ctx == nil {
		var err error
		ctx, err = newDisplayContext(target.DisplayHandle, targetWindowKind)
		if err != nil {
			return nil, err
		}
		i.rememberDisplayContextLocked(displayKey{kind: targetWindowKind, display: target.DisplayHandle}, ctx)
	}

	surf, err := newSurface(target, ctx)
	if err != nil {
		ctx.Release()
		return nil, err
	}
	return surf, nil
}

// newSurface returns a Surface holding ctx, which already carries the
// Surface's reference.
func newSurface(target hal.SurfaceTarget, ctx *AdapterContext) (*Surface, error) {
	glCtx, err := ctx.Lock()
	if err != nil {
		return nil, err
	}
	defer ctx.Unlock()
	var eglDisplay egl.EGLDisplay
	if eglCtx := ctx.EGL(); eglCtx != nil {
		eglDisplay = eglCtx.Display()
	}
	return &Surface{
		displayHandle: target.DisplayHandle,
		windowHandle:  target.WindowHandle,
		ctx:           ctx,
		eglDisplay:    eglDisplay,
		version:       glCtx.GetString(gl.VERSION),
		renderer:      glCtx.GetString(gl.RENDERER),
	}, nil
}

// lookupContext is lookupContextLocked under the Instance lock.
func (i *Instance) lookupContext(kind egl.WindowKind, display uintptr) *AdapterContext {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.lookupContextLocked(kind, display)
}

// lookupContextLocked returns an existing context for a surface of the given
// window kind and native display, with a reference taken for the caller, or
// nil when a new context must be built. The native display is compared only
// for contexts bound to a caller-owned display.
func (i *Instance) lookupContextLocked(kind egl.WindowKind, display uintptr) *AdapterContext {
	if i.ctx != nil && i.ctx.GL() != nil && i.ctxKind == kind &&
		(i.ctxDisplay == 0 || i.ctxDisplay == display) && i.ctx.Acquire() {
		hal.Logger().Info("gles: surface sharing Instance AdapterContext")
		return i.ctx
	}
	key := displayKey{kind: kind, display: display}
	if ctx, ok := i.displayContexts[key]; ok {
		if ctx.Acquire() {
			hal.Logger().Info("gles: surface sharing the context of its native display")
			return ctx
		}
		// Every surface and device of that context is gone. A new connection
		// may reuse the address, so the dead entry is never reused.
		delete(i.displayContexts, key)
	}
	return nil
}

// rememberDisplayContextLocked records a new Path B context for key and drops
// every dead entry. A lookup drops a dead entry only for its own key, so
// without this sweep an app that opens and closes displays at changing
// addresses would keep one dead context (and its GL function table) per
// address until Instance.Destroy. The map then holds at most one entry per
// live context plus the new one.
func (i *Instance) rememberDisplayContextLocked(key displayKey, ctx *AdapterContext) {
	if i.displayContexts == nil {
		i.displayContexts = make(map[displayKey]*AdapterContext)
	}
	for k, c := range i.displayContexts {
		if !c.Alive() {
			delete(i.displayContexts, k)
		}
	}
	i.displayContexts[key] = ctx
}

// newDisplayContext builds an EGL context on the caller's native display.
// It tries desktop GL first and falls back to GLES 3.0: Mesa Wayland EGL may
// only expose EGL_OPENGL_ES3_BIT configs, not EGL_OPENGL_BIT (found by
// @lkmavi, PR #215). The caller holds the returned context's first reference.
func newDisplayContext(display uintptr, kind egl.WindowKind) (*AdapterContext, error) {
	config := egl.DefaultContextConfig()
	config.NativeDisplay = display
	config.WindowKind = &kind
	var eglCtx *egl.Context
	var err error
	if !forceGLES {
		eglCtx, err = egl.NewContext(config)
	}
	if forceGLES || err != nil {
		config = gles30Config(config)
		eglCtx, err = egl.NewContext(config)
	}
	if err != nil {
		return nil, fmt.Errorf("gles: failed to create EGL context (tried desktop GL and GLES 3.0): %w", err)
	}

	hal.Logger().Debug("gles: EGL context", "robustAccess", eglCtx.RobustAccess())

	glCtx, err := loadContextGL(eglCtx, config.GLES)
	if err != nil {
		return nil, err
	}
	return NewAdapterContext(eglCtx, glCtx), nil
}

// gles30Config is config asking for an OpenGL ES 3.0 context, Rust wgpu-hal
// egl.rs's fallback when desktop GL fails (gles_context_attributes).
func gles30Config(config egl.ContextConfig) egl.ContextConfig {
	config.GLES = true
	config.GLVersionMajor = 3
	config.GLVersionMinor = 0
	config.CoreProfile = false
	return config
}

// EnumerateAdapters returns available OpenGL adapters.
// Uses surface context (preferred), instance context (X11/headless), or placeholder.
// The adapters hold their context weakly: a Device opened from one takes its
// own reference, and Open fails once the context is gone.
func (i *Instance) EnumerateAdapters(surfaceHint hal.Surface) []hal.ExposedAdapter {
	// Priority 1: surface provides the best context (has window handle)
	if surface, ok := surfaceHint.(*Surface); ok {
		adapter, ok := surface.GetAdapterInfo()
		if !ok {
			return nil // below OpenGL 3.3 / ES 3.0 (logged)
		}
		return []hal.ExposedAdapter{adapter}
	}

	// Priority 2: instance-level AdapterContext (created in CreateInstance via
	// pbuffer/surfaceless).
	if instCtx := i.context(); instCtx != nil && instCtx.GL() != nil {
		adapter, ok := makeAdapterFromContext(instCtx)
		if !ok {
			return nil // below OpenGL 3.3 / ES 3.0 (logged)
		}
		return []hal.ExposedAdapter{adapter}
	}

	// Priority 3: no context available (Wayland without surface hint)
	// Return placeholder — Open() has nil guard from PR #210.
	return []hal.ExposedAdapter{placeholderExposedAdapter("OpenGL 3.3+ / ES 3.0+ (no context — use RequestAdapterWithSurface)")}
}

// placeholderExposedAdapter returns a non-openable adapter when no EGL context exists.
func placeholderExposedAdapter(driverInfo string) hal.ExposedAdapter {
	return hal.ExposedAdapter{
		Adapter: &Adapter{},
		Info: gputypes.AdapterInfo{
			Name:       "OpenGL Adapter",
			Vendor:     vendorUnknown,
			DeviceType: gputypes.DeviceTypeOther,
			Driver:     driverOpenGL,
			DriverInfo: driverInfo,
			Backend:    gputypes.BackendGL,
		},
		Capabilities: hal.Capabilities{
			Limits: gputypes.DefaultLimits(),
			AlignmentsMask: hal.Alignments{
				BufferCopyOffset: 4,
				BufferCopyPitch:  256,
			},
		},
	}
}

// makeAdapterFromContext creates an ExposedAdapter using a live AdapterContext
// (the X11, headless and surfaceless instance context). ok is false when the
// context is below OpenGL 3.3 / ES 3.0 or already released (no adapter).
func makeAdapterFromContext(ctx *AdapterContext) (adapter hal.ExposedAdapter, ok bool) {
	glCtx, err := ctx.Lock()
	if err != nil {
		return hal.ExposedAdapter{}, false
	}
	defer ctx.Unlock()

	caps, err := queryAdapterCapabilities(glCtx)
	if err != nil {
		return hal.ExposedAdapter{}, false // below OpenGL 3.3 / ES 3.0 (logged)
	}
	return exposeAdapter(&Adapter{ctx: ctx, version: caps.Version, renderer: caps.Renderer}, &caps), true
}

// exposeAdapter fills in the probed capabilities of an adapter and returns it
// exposed. Every Linux context, instance or surface, goes through it, as every
// context goes through one expose in Rust wgpu-hal (src/gles/adapter.rs).
// Before this, the instance-context adapter skipped the probe and reported no
// features, no downlevel flags (so no compute) and default limits.
func exposeAdapter(a *Adapter, caps *AdapterCapabilities) hal.ExposedAdapter {
	a.caps = *caps
	return hal.ExposedAdapter{
		Adapter: a,
		Info: gputypes.AdapterInfo{
			Name:       caps.Renderer,
			Vendor:     caps.Vendor,
			VendorID:   caps.VendorID,
			DeviceID:   0,
			DeviceType: caps.DeviceType,
			Driver:     caps.Version,
			DriverInfo: glDriverInfo(caps),
			Backend:    gputypes.BackendGL,
		},
		Features: caps.Features,
		Capabilities: hal.Capabilities{
			Limits: caps.Limits,
			AlignmentsMask: hal.Alignments{
				BufferCopyOffset: 4,
				BufferCopyPitch:  4,
			},
			DownlevelCapabilities: gputypes.DownlevelCapabilities{
				ShaderModel: gputypes.ShaderModelSm5,
				Limits:      gputypes.DownlevelLimits{},
				Flags:       caps.DownlevelFlags,
			},
		},
	}
}

// Destroy releases the instance's own context. Contexts built on a caller's
// display are left to the surfaces and devices that hold them; the Instance
// only forgets them.
func (i *Instance) Destroy() {
	i.mu.Lock()
	ctx := i.ctx
	i.ctx = nil
	i.displayContexts = nil
	i.mu.Unlock()
	if ctx != nil {
		ctx.Release()
	}
}
