# Ray Tracing (Experimental, HAL only)

> **Status:** experimental, not in the W3C WebGPU spec. Not usable through the public `wgpu` API yet.
> **Plan:** public API following Rust wgpu's `EXPERIMENTAL_RAY_QUERY`, starting with Vulkan
> ([#369](https://github.com/gogpu/wgpu/issues/369)).

## What exists today

The HAL (`hal/raytracing.go`) has acceleration structure resources and build commands, and each backend has code for
them. Nothing above the HAL uses it yet: the root `wgpu` package has no `Blas`, `Tlas` or acceleration structure
binding, so ray tracing is reachable only by driving `hal` directly, as `examples/raytracing-headless/` does.

| Backend | HAL code | Advertises `FeatureRayQuery` | Has run |
|---------|----------|------------------------------|---------|
| Vulkan | VK_KHR_acceleration_structure + VK_KHR_ray_query calls | No: the extensions and device features are never enabled | No |
| DX12 | DXR Tier 1.1 (ID3D12Device5 / ID3D12GraphicsCommandList4) | No | No |
| Metal | MTLAccelerationStructure (macOS 15+, iOS 18+) | No | No |
| Software | CPU BVH build and ray-triangle intersection | No: see below | BVH build and intersection: unit tests |
| GLES, Noop | none (returns an error) | No | — |

- On Vulkan, DX12 and Metal no adapter advertises `FeatureRayQuery`, so a device never has it, and the code has
  never run on a GPU. Vulkan never enables the ray tracing extensions either. Work on #369 found that the Vulkan path
  also needed missing function loading and a struct layout fix before it could run.
- The software backend builds BVHs on the CPU, but its SPIR-V interpreter does not execute ray query
  instructions, so it does not advertise `FeatureRayQuery`: a shader that uses `ray_query` cannot run there.
- `internal/raytracing/` holds build ordering, compaction and validation logic for the planned public API.

## Example

`examples/raytracing-headless/` creates a software HAL device, builds a BLAS from one triangle, and traces rays
against the resulting BVH in Go on the CPU to render a PNG. It shows the HAL build path. It does not run a shader
ray query.

## Limitations

- No public API yet; the HAL types may change before they are exposed.
- Inline ray queries only are planned; ray tracing pipelines are not.
- Not in the W3C WebGPU spec.
- The software BVH uses a simple midpoint split (not SAH): meant for tests, not performance.
