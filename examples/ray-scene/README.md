# ray-scene

Port of Rust wgpu's
[`examples/features/src/ray_scene`](https://github.com/gfx-rs/wgpu/tree/trunk/examples/features/src/ray_scene).
It loads two OBJ models, builds one multi-geometry BLAS per model, and places
four instances in a TLAS that is rebuilt every frame. A fullscreen fragment
shader traces an inline ray query per pixel and shades each hit using
per-geometry material data and interpolated vertex normals.

The Rust example opens a window. This port renders one frame headlessly at
1024×768, like the Rust example's test mode. It writes the frame to a PNG and
compares it to `screenshot.png`, the Rust reference image, using a mean
difference threshold of 0.02.

Differences from the Rust example:

- Native Go has no implicit pipeline layouts (`layout: None` plus
  `get_bind_group_layout`), so the bind group layout is written out
  explicitly.
- `obj.go` has a small OBJ/MTL loader that reproduces the grouping rules of
  the `obj` 0.10 crate the Rust example uses. Those rules decide how many BLAS
  geometries each model gets. It is tested by `go test ./examples/ray-scene`.

**Status:** matches `screenshot.png` on Vulkan (RTX 5080, mean difference
0.000125), with the validation layers clean. It needs an adapter with
`FeatureRayQuery`; the Rust FFI and browser backends return `ErrUnsupported`.

```bash
GOGPU_GRAPHICS_API=vulkan go run ./examples/ray-scene -o out.png
# GOGPU_DEBUG=1 enables validation layers and fails on validation errors.
```

`shader.wgsl`, `screenshot.png`, `screenshot-vulkan.png` and the files under
`models/` are copied unchanged from gfx-rs/wgpu (MIT OR Apache-2.0, Copyright
(c) The gfx-rs developers). `rustacean-3d.*` comes from that repository's
`skybox` example.
