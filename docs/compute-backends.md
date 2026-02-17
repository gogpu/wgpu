# Compute Shader Backend Differences

This document describes compute shader support across wgpu's HAL backends.

## Support Matrix

| Feature | Vulkan | DX12 | Metal | GLES | Software | Noop |
|---------|:------:|:----:|:-----:|:----:|:--------:|:----:|
| Compute shaders | Yes | Yes | Yes | Partial | No | No |
| Storage buffers | Yes | Yes | Yes | Yes | No | No |
| Timestamp queries | Yes | Stub | Stub | Stub | No | No |
| Indirect dispatch | Yes | Yes | Yes | Yes | No | No |
| ReadBuffer (GPU->CPU) | Yes | Yes | Yes | Yes | No | No |
| Max workgroup size X | 1024+ | 1024 | 1024 | 1024 | N/A | N/A |
| Max workgroup invocations | 1024+ | 1024 | 1024 | 1024 | N/A | N/A |

## Vulkan

**Status:** Full compute support.

Vulkan provides the most complete compute shader implementation:

- **Shader compilation:** WGSL -> SPIR-V (via `gogpu/naga`).
- **Timestamp queries:** Fully implemented using `vkCmdWriteTimestamp` at the beginning and end of compute passes. Results are copied to a buffer via `vkCmdCopyQueryPoolResults`.
- **Memory barriers:** Automatic global memory barrier inserted at compute pass end (`End()`) to ensure shader writes are visible to subsequent commands.
- **Workgroup size limits:** Depends on the physical device. Typical minimum guaranteed by Vulkan spec: 128 invocations per workgroup. Most desktop GPUs support 1024.
- **Synchronization:** Use `Fence` + `Device.Wait()` to synchronize CPU with GPU completion.
- **Timestamp period:** `Queue.GetTimestampPeriod()` returns the nanoseconds-per-tick value for the GPU's timestamp counter.

### Vulkan-Specific Notes

- Query pools are reset automatically before each timestamp write.
- The `ResolveQuerySet` method inserts a pipeline barrier before copying results to ensure timestamps are finalized.
- `VK_QUERY_RESULT_64_BIT | VK_QUERY_RESULT_WAIT_BIT` flags ensure results are complete 64-bit values.

## DirectX 12

**Status:** Compute shaders work. Timestamp queries are stubbed.

- **Shader compilation:** WGSL -> HLSL (via `gogpu/naga`) -> DXBC (via `d3dcompiler_47.dll`).
- **Timestamp queries:** `CreateQuerySet` currently returns `ErrTimestampsNotSupported`. The underlying D3D12 API supports timestamp queries via `ID3D12Device::CreateQueryHeap` with `D3D12_QUERY_TYPE_TIMESTAMP` and `ID3D12GraphicsCommandList::EndQuery` + `ResolveQueryData`. Implementation is planned.
- **Workgroup size limits:** Maximum 1024 invocations per workgroup (D3D12 spec).
- **Root signature:** Compute pipelines use a separate root signature from graphics pipelines. Descriptor heaps are bound lazily on first `SetBindGroup` call.

### DX12-Specific Notes

- Descriptor heaps must be bound before setting descriptor tables. The `ComputePassEncoder` tracks this with `descriptorHeapsSet`.
- No explicit compute pass begin/end is needed at the D3D12 API level.

## Metal

**Status:** Compute shaders work. Timestamp queries are stubbed.

- **Shader compilation:** WGSL -> MSL (via `gogpu/naga`).
- **Timestamp queries:** `CreateQuerySet` currently returns `ErrTimestampsNotSupported`. Metal supports counter sample buffers (`MTLCounterSampleBuffer`) for GPU timestamps on Apple Silicon. Implementation is planned.
- **Workgroup size limits:** Maximum 1024 invocations per workgroup on Apple Silicon. Older Intel Macs may have lower limits.
- **Compute encoder:** Metal uses `MTLComputeCommandEncoder` (created from `MTLCommandBuffer`). The encoder is `Retain`-ed to survive autorelease pool drains.

### Metal-Specific Notes

- `GetTimestampPeriod()` returns 1.0 (Metal timestamps are already in nanoseconds).
- Bind group resources are bound directly via `setBuffer:offset:atIndex:` on the compute encoder.

## OpenGL ES (GLES)

**Status:** Partial compute support. Requires OpenGL ES 3.1+.

- **Shader compilation:** WGSL -> GLSL (via `gogpu/naga`).
- **Timestamp queries:** `CreateQuerySet` currently returns `ErrTimestampsNotSupported`. GLES supports timer queries via `GL_EXT_disjoint_timer_query` extension (not universally available). Implementation is planned.
- **Compute support flag:** Check `DownlevelFlagsComputeShaders` in adapter capabilities.
- **GLES 3.0:** No compute shader support (only GLES 3.1+).
- **GLES 3.1+:** Compute shaders supported via `glDispatchCompute`.

### GLES-Specific Notes

- Compute commands are deferred: `BeginComputePass` records commands into a list that is replayed on `Submit`.
- The `glMemoryBarrier` call is needed after dispatch to ensure writes are visible.
- Some drivers (especially Mesa llvmpipe on CI) may have limited compute support.

## Software Backend

**Status:** Compute shaders are **not supported**.

The software backend is a CPU rasterizer designed for headless rendering and testing. It does not execute compute shaders.

- `CreateQuerySet` returns an error.
- `BeginComputePass` returns a valid encoder, but `Dispatch` is a no-op.
- Use the software backend only for render pipeline testing or as a fallback when no GPU is available.

### Future Plans

A CPU-based compute shader interpreter may be added in the future (tracked as CS-013).

## Noop Backend

**Status:** Compute shaders are **not supported**.

The noop backend is a testing backend that accepts all API calls without executing anything. All compute pass operations are no-ops.

- `CreateQuerySet` returns `ErrTimestampsNotSupported`.
- Useful for unit testing command encoder state machines without requiring a GPU.

## Workgroup Size Recommendations

| Data Size | Recommended `@workgroup_size` | Notes |
|-----------|-------------------------------|-------|
| < 1K elements | 64 | Small batches, low overhead |
| 1K - 100K | 64 or 128 | Good balance for most GPUs |
| 100K - 1M | 256 | Better occupancy on discrete GPUs |
| > 1M | 256 | Consider multiple dispatches |

The optimal workgroup size depends on the GPU architecture, register usage, and shared memory requirements. When in doubt, use 64 -- it works well across all backends.

## Memory Considerations

### Buffer Alignment

- Uniform buffers: 256-byte alignment required on most backends.
- Storage buffers: No strict alignment, but 16-byte alignment is recommended.
- Copy operations: Source/destination offsets may need alignment (check `Alignments.BufferCopyOffset`).

### GPU Memory Limits

Large compute workloads may exhaust GPU memory. Monitor for `ErrDeviceOutOfMemory` errors from buffer and pipeline creation.

### Staging Buffers

To read compute results back to the CPU:
1. Create the output buffer with `BufferUsageStorage | BufferUsageCopySrc`.
2. Create a staging (readback) buffer with `BufferUsageMapRead | BufferUsageCopyDst`.
3. Use `CopyBufferToBuffer` to copy from output to staging.
4. Use `Queue.ReadBuffer` to read the staging buffer.

This two-step process is required because GPU-optimal memory is typically not directly accessible by the CPU.
