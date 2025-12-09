# Vulkan Triangle - Pure Go Integration Test

Full integration test for the Pure Go Vulkan backend. This test validates the entire rendering pipeline from window creation to triangle rendering.

## What It Tests

This integration test validates:

1. **Window Creation** - Win32 API via syscall/purego
2. **Vulkan Instance** - Pure Go Vulkan initialization
3. **Surface Creation** - Platform-specific surface for Win32
4. **Adapter Enumeration** - Physical device detection
5. **Device Creation** - Logical device and queue
6. **Surface Configuration** - Swapchain setup
7. **Shader Modules** - SPIR-V shader loading
8. **Pipeline Creation** - Full render pipeline
9. **Render Loop** - Acquire → Render → Present cycle
10. **Clean Shutdown** - Resource cleanup

## Test Output

The test should:
- Create an 800x600 window titled "Vulkan Triangle - Pure Go"
- Display a **red triangle** on a **blue background**
- Print initialization steps with OK/FAILED status
- Show FPS counter in console
- Run until window is closed

## Architecture

### Shaders

Pre-compiled SPIR-V shaders are embedded in `shaders.go`:

**Vertex Shader** (`triangle.vert.spv`):
- Hardcoded triangle positions (no vertex buffers)
- Uses `gl_VertexIndex` to select vertex

**Fragment Shader** (`triangle.frag.spv`):
- Outputs solid red color (1.0, 0.0, 0.0, 1.0)

### Render Pipeline

```
Window (Win32) → Surface (VK_KHR_win32_surface)
    → Swapchain (BGRA8Unorm, Fifo)
    → RenderPass (Blue clear color)
    → Pipeline (Vertex + Fragment shaders)
    → Draw (3 vertices, triangle list)
    → Present
```

### No CGO Required

All Vulkan calls use `syscall.SyscallN` via `hal/vulkan/vk` package.
No C compiler needed.

## Building

```bash
# Windows only (for now)
cd cmd/vulkan-triangle
GOROOT="/c/Program Files/Go" go build
```

## Running

```bash
# Requires:
# - Windows 10+
# - Vulkan driver installed
# - vulkan-1.dll in PATH (usually in System32)

./vulkan-triangle.exe
```

## Expected Output

```
=== Vulkan Triangle Integration Test ===

1. Creating window... OK
2. Creating Vulkan backend... OK
3. Creating Vulkan instance... OK
4. Creating surface... OK
5. Enumerating adapters... OK (found 1)
   - Adapter 0: NVIDIA GeForce RTX 3080 (NVIDIA Vulkan 1.3.XXX)
6. Opening device... OK
7. Configuring surface... OK
8. Creating shader modules... OK
9. Creating pipeline layout... OK
10. Creating render pipeline... OK

=== Starting Render Loop ===
Press ESC or close window to exit

Rendered 60 frames (60.0 FPS)
Rendered 120 frames (60.0 FPS)
...

=== Test Complete ===
Total frames: 360
Average FPS: 60.0
```

## Troubleshooting

### Build Errors

**Error**: `cannot find package "github.com/gogpu/wgpu/hal/vulkan"`
- Make sure you're in the `wgpu` repository
- Run `go mod tidy`

### Runtime Errors

**Error**: `vulkan-1.dll not found`
- Install Vulkan SDK or GPU driver with Vulkan support
- Add Vulkan SDK bin directory to PATH

**Error**: `vkCreateInstance failed`
- GPU driver may not support Vulkan 1.2+
- Try removing `types.InstanceFlagsDebug` from instance descriptor

**Error**: `no adapters found`
- GPU driver doesn't support the surface type
- Check GPU supports Vulkan

**Window doesn't appear**
- Check console for error messages
- Try running as administrator

**Black window / No triangle**
- Shaders may not be loading correctly
- Pipeline creation may have failed (check console)
- Check GPU supports BGRA8Unorm format

## Implementation Status

- [x] Window creation (Windows)
- [x] Vulkan initialization
- [x] Surface creation
- [x] Adapter enumeration
- [x] Device creation
- [x] Swapchain configuration
- [x] Shader modules (SPIR-V)
- [x] Pipeline creation
- [x] Render pass
- [x] Command encoding
- [x] Presentation
- [ ] Linux support (VK_KHR_xlib_surface)
- [ ] macOS support (MoltenVK)

## Files

| File | Purpose |
|------|---------|
| `main.go` | Main test logic and render loop |
| `window_windows.go` | Win32 window creation via syscall |
| `shaders.go` | Embedded SPIR-V bytecode |
| `shaders/triangle.vert` | Original GLSL vertex shader |
| `shaders/triangle.frag` | Original GLSL fragment shader |

## Design Notes

This test follows the **minimal WebGPU workflow**:

1. Instance → Surface → Adapter → Device
2. Configure surface with format and present mode
3. Create shaders → Create pipeline
4. Loop: Acquire → Encode → Submit → Present

Key differences from WebGPU:
- Vulkan requires explicit swapchain management
- Memory allocation is explicit (using `hal/vulkan/memory` allocator)
- No automatic resource tracking (manual Destroy calls)

## Future Enhancements

- [ ] Vertex buffers test (not hardcoded positions)
- [ ] Index buffers test
- [ ] Uniform buffers test
- [ ] Texture sampling test
- [ ] Depth testing test
- [ ] Multiple render targets
- [ ] Compute shader test

## Related Tests

- `cmd/vk-test` - Minimal Vulkan device test (no rendering)
- `cmd/gles-test` - OpenGL ES backend test

---

**Platform:** Windows 10+ (64-bit)
**Go Version:** 1.25+
**Vulkan Version:** 1.2+
