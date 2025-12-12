# EGL Linux Support - Next Steps

## Completed (TASK-023 Phase 1)

### Core EGL Infrastructure ✅
All core EGL support has been implemented and compiles successfully:

1. **`hal/gles/egl/types.go`** (~300 LOC) - EGL types and constants
   - All EGL 1.4/1.5 types (EGLDisplay, EGLContext, EGLConfig, etc.)
   - All constants (errors, config attributes, context attributes, etc.)
   - WindowKind enum (X11, Wayland, Surfaceless, Unknown)

2. **`hal/gles/egl/egl.go`** (~300 LOC) - EGL library loading via purego
   - Loads libEGL.so.1/libEGL.so via purego.Dlopen
   - Registers all EGL 1.0-1.4 core functions
   - Optional EGL 1.5 functions (eglGetPlatformDisplay)
   - Wrapper functions for all EGL calls

3. **`hal/gles/egl/display.go`** (~200 LOC) - Platform display support
   - DisplayOwner struct to manage native display connections
   - OpenX11Display() - loads libX11.so, calls XOpenDisplay
   - TestWaylandDisplay() - loads libwayland-client.so, tests connection
   - DetectWindowKind() - automatic window system detection
   - GetEGLDisplay() - returns EGL display for detected platform

4. **`hal/gles/egl/context.go`** (~300 LOC) - EGL context management
   - Context struct with display, config, context, pbuffer
   - NewContext() - creates EGL context with platform detection
   - chooseEGLConfig() - selects appropriate EGL configuration
   - createEGLContext() - creates context with OpenGL version/profile
   - MakeCurrent(), Destroy() - context lifecycle management
   - GetGLProcAddress() - for GL function loading

### Linux Backend Integration ✅
5. **`hal/gles/api_linux.go`** - Linux Backend and Instance
6. **`hal/gles/adapter_linux.go`** - Linux Adapter
7. **`hal/gles/resource_linux.go`** - Linux Surface
8. **`hal/gles/device_linux.go`** - Linux Device stub
9. **`hal/gles/queue_linux.go`** - Linux Queue stub
10. **`hal/gles/init_linux.go`** - Linux backend registration

### Linux GL Context ✅
11. **`hal/gles/gl/context_linux.go`** (~670 LOC) - Linux GL function loading
    - Same Context struct as Windows but using purego instead of syscall
    - All GL function wrappers adapted for purego.SyscallN
    - Helper functions (unsafePointer, floatToUint32, goString, cString)

### Dependencies ✅
12. **go.mod** - Added `github.com/ebitengine/purego v0.9.1`

## Verification ✅
- ✅ EGL package compiles: `GOOS=linux go build ./hal/gles/egl/...`
- ✅ purego dependency added successfully
- ✅ All platform-specific code properly tagged with `//go:build linux`

## Remaining Work

### Phase 2: Complete Device/Queue Implementation

The current implementation has **minimal stubs** for Device and Queue on Linux. The Windows implementation has ~500 LOC of methods that need to be adapted for Linux.

**Two approaches:**

#### Option A: Copy-Adapt Windows Implementation (Quick)
1. Copy `hal/gles/device.go` → `hal/gles/device_impl_linux.go`
2. Copy `hal/gles/queue.go` → `hal/gles/queue_impl_linux.go`
3. Copy `hal/gles/command.go` → `hal/gles/command_linux.go`
4. Change build tags from `//go:build windows` to `//go:build linux`
5. Replace `wgl.Context` with `egl.Context` throughout
6. Replace `wgl.HWND` with `displayHandle, windowHandle uintptr`
7. Test and verify

**Estimated effort:** 2-3 hours

#### Option B: Refactor for Platform Independence (Clean)
1. Create `hal/gles/device_common.go` (no build tags)
   - Move all GL-based methods (CreateBuffer, CreateTexture, CreateShaderModule, etc.)
   - Use `*gl.Context` only (platform-independent)
2. Keep `hal/gles/device_windows.go` for Windows-specific parts
   - Just the Device struct definition with `wglCtx`
3. Create `hal/gles/device_linux.go` for Linux-specific parts
   - Just the Device struct definition with `eglCtx`
4. Same refactoring for Queue and Command
5. Test both platforms

**Estimated effort:** 4-6 hours (but cleaner long-term)

### Files That Need Linux Versions
The following Windows files need Linux equivalents:

```
hal/gles/device.go      (505 LOC) → device_impl_linux.go or refactor
hal/gles/queue.go       (100 LOC) → queue_impl_linux.go or refactor
hal/gles/command.go     (~400 LOC) → command_linux.go or refactor
```

### Key Changes Needed
When adapting Windows→Linux:

1. **Context Management:**
   ```go
   // Windows
   wglCtx *wgl.Context
   hwnd wgl.HWND

   // Linux
   eglCtx *egl.Context
   displayHandle, windowHandle uintptr
   ```

2. **Making Context Current:**
   ```go
   // Windows
   d.wglCtx.MakeCurrent()

   // Linux
   d.eglCtx.MakeCurrent()
   ```

3. **Swapping Buffers:**
   ```go
   // Windows (in Queue.Present)
   q.wglCtx.SwapBuffers()

   // Linux (in Queue.Present)
   egl.SwapBuffers(q.eglCtx.Display(), q.eglCtx.Pbuffer())
   ```

### Testing Strategy
Once Device/Queue are implemented:

1. **Unit Tests:**
   ```bash
   GOOS=linux go test ./hal/gles/egl/...
   GOOS=linux go test ./hal/gles/...
   ```

2. **Cross-Compile Check:**
   ```bash
   GOOS=linux GOARCH=amd64 go build ./...
   ```

3. **Linux VM Testing:**
   - Test on actual Linux with X11
   - Test on actual Linux with Wayland
   - Test surfaceless/headless rendering

### Integration with gogpu/gogpu
Once complete, update `gogpu/gogpu` repository:
1. Update go.mod to latest wgpu version
2. Test triangle example on Linux
3. Test texture example on Linux
4. Document Linux requirements in README

## References
- **Rust Reference:** `D:\projects\gogpu\reference\wgpu-ecosystem\wgpu\wgpu-hal\src\gles\egl.rs`
- **Windows Implementation:** `hal/gles/device.go`, `hal/gles/queue.go`
- **EGL Specification:** https://registry.khronos.org/EGL/
- **purego Documentation:** https://github.com/ebitengine/purego

## Summary
✅ **Phase 1 Complete:** Core EGL infrastructure fully implemented and compiling.
⏳ **Phase 2 Remaining:** Adapt Device/Queue/Command implementations for Linux.

The hard part (EGL abstraction, platform detection, library loading) is done.
The remaining work is mostly mechanical adaptation of existing Windows code.
