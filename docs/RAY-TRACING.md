# Ray Tracing Extensions (Experimental)

> **Status:** Experimental (not in W3C WebGPU spec, Milestone 4+)
> **Since:** v0.32.0
> **Feature gate:** `FeatureRayQuery`

## Overview

wgpu supports inline ray queries matching Rust wgpu's `EXPERIMENTAL_RAY_QUERY`.
Ray tracing enables acceleration structure-based ray-scene intersection testing
in compute and fragment shaders.

## Backend Support

| Backend | Platform | API | Min Hardware |
|---------|----------|-----|-------------|
| Vulkan | Windows, Linux | VK_KHR_acceleration_structure + VK_KHR_ray_query | RTX 20+, RDNA 2+, Arc |
| DX12 | Windows | DXR Tier 1.1, SM 6.5 | Same GPUs |
| Metal | macOS 15.0+, iOS 18.0+ | MTLAccelerationStructure | A13+, Apple Silicon |
| Software | All platforms | CPU BVH (Moller-Trumbore) | Any CPU |
| GLES | — | Not supported | — |

## Architecture

Ray tracing is implemented as a three-layer architecture (ADR-062):

### 1. `internal/raytracing/` — Build Orchestration & Validation

Isolated in `internal/` per ADR-069 (struct ownership + callback interface pattern).
Core holds pointers and calls directly.

- **types.go** — CompactionState (Idle -> Waiting -> Ready -> Compacted), BlasAction enums
- **blas.go, tlas.go** — BLAS/TLAS resource structs with helpers (IsBuilt, AllowsCompaction)
- **build.go** — BuildContext: scratch buffer alignment, BLAS -> TLAS dependency tracking (built_index)
- **compaction.go** — per-state handler functions
- **validate.go** — 9 validation checks: feature gate, geometry/instance counts, build ordering, alignment, compact state
- **errors.go** — typed ValidationError with operation constants
- **context.go** — DeviceContext callback interface (core.Device implements without import cycle)

~1,300 LOC, 73 tests, 96.8% coverage.

### 2. `hal/raytracing.go` — HAL Interface

Defines the backend-agnostic RT interface:

- `AccelerationStructure` resource type
- 12 descriptor types for BLAS/TLAS geometry, instances, and build parameters
- `TlasInstance` for positioning geometry in the scene
- 5 Device methods (create, build, compact acceleration structures)
- 4 CommandEncoder methods (build BLAS/TLAS, write instances, compact)

### 3. Per-Backend Implementations

All 4 GPU backends implement real API calls:

- **Vulkan** — VK_KHR_acceleration_structure + VK_KHR_ray_query (FFI calls via goffi)
- **DX12** — DXR Tier 1.1, COM bindings for ID3D12Device5/CommandList4
- **Metal** — MTLAccelerationStructure (macOS 15.0+)
- **Software** — CPU BVH build + Moller-Trumbore ray-triangle intersection (for CI/testing without GPU)
- **GLES/Noop** — return ErrUnsupported

## Example

See `examples/raytracing-headless/` for a complete working example that:
1. Creates triangle geometry
2. Builds a BLAS (bottom-level acceleration structure)
3. Traces rays against the BVH
4. Renders result to PNG

No GPU required — runs on the software backend.

## Limitations

- Inline ray queries only (no ray tracing pipelines yet)
- Experimental — API may change before v1.0
- Not in W3C WebGPU spec (Milestone 4+, blocked on bindless in spec)
- Software backend uses simple midpoint-split BVH (not SAH) — for CI correctness, not production performance
