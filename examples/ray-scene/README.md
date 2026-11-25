# ray-scene

Port of Rust wgpu's
[`examples/features/src/ray_scene`](https://github.com/gfx-rs/wgpu/tree/trunk/examples/features/src/ray_scene).
It loads an OBJ model of the Go gopher, builds a multi-geometry BLAS for it,
and places four instances in a TLAS that is rebuilt every frame. A fullscreen
fragment shader traces an inline ray query per pixel and shades each hit using
per-geometry material data.

The Rust example opens a window. This port renders one frame headlessly at
1024×768, like the Rust example's test mode. It writes the frame to a PNG and
compares it to `screenshot.png` using a mean difference threshold of 0.02.

Differences from the Rust example:

- The Rust example loads Ferris and a cube + Suzanne scene, and draws the
  second model's instances with normals as colors. This port loads only the
  gopher, so every instance is drawn with flat material colors. Each instance
  is centered on its bounding box and turned to face the camera.
- Native Go has no implicit pipeline layouts (`layout: None` plus
  `get_bind_group_layout`), so the bind group layout is written out
  explicitly.
- `obj.go` has a small OBJ/MTL loader that reproduces the grouping rules of
  the `obj` 0.10 crate the Rust example uses. Those rules decide how many BLAS
  geometries each model gets (17 for the gopher). It is tested by
  `go test ./examples/ray-scene`.

**Status:** `screenshot.png` is a Vulkan render (RTX 5080), with the
validation layers clean. It needs an adapter with `FeatureRayQuery`; the Rust FFI and browser backends return `ErrUnsupported`.

```bash
GOGPU_GRAPHICS_API=vulkan go run ./examples/ray-scene -o out.png
# GOGPU_DEBUG=1 enables validation layers and fails on validation errors.
```

`shader.wgsl` is copied unchanged from gfx-rs/wgpu (MIT OR Apache-2.0,
Copyright (c) The gfx-rs developers). `models/gopher.*` is derived from
[golang-samples/gopher-3d](https://github.com/golang-samples/gopher-3d) by
Takuya Ueda (CC BY 3.0), with the subdivision removed to reduce its detail and
file size.
