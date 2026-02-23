# Compute Shaders in wgpu

This guide covers how to use compute shaders with the `gogpu/wgpu` Pure Go WebGPU implementation.

## Overview

Compute shaders are GPU programs that run outside the graphics pipeline. They are used for general-purpose GPU computing (GPGPU): physics simulations, image processing, data transformations, machine learning inference, and more.

In WebGPU, compute shaders are written in WGSL and dispatched as workgroups. Each workgroup contains a fixed number of invocations (threads) defined by `@workgroup_size`.

## Writing WGSL Compute Shaders

A minimal compute shader that doubles every element in a buffer:

```wgsl
@group(0) @binding(0)
var<storage, read> input: array<f32>;

@group(0) @binding(1)
var<storage, read_write> output: array<f32>;

@compute @workgroup_size(64)
fn main(@builtin(global_invocation_id) id: vec3<u32>) {
    let i = id.x;
    if (i < arrayLength(&input)) {
        output[i] = input[i] * 2.0;
    }
}
```

Key concepts:
- `@compute` marks the function as a compute shader entry point.
- `@workgroup_size(64)` means each workgroup has 64 invocations (threads).
- `@builtin(global_invocation_id)` is the unique ID for each invocation across all workgroups.
- `var<storage, read>` declares a read-only storage buffer.
- `var<storage, read_write>` declares a read-write storage buffer.
- `arrayLength(&input)` returns the runtime size of the storage buffer array.

### Workgroup Size Guidelines

- `@workgroup_size(64)` is a safe default for most GPUs.
- `@workgroup_size(256)` may be faster for large data sets on discrete GPUs.
- Maximum workgroup size varies by backend (see [Backend Differences](compute-backends.md)).
- The total invocations per workgroup (x * y * z) must not exceed the device limit.

## Creating a Compute Pipeline

The compute pipeline binds a shader module to a pipeline layout.

### Step 1: Create a Shader Module

Compile WGSL source code (or SPIR-V bytecode) into a shader module:

```go
shaderModule, err := device.CreateShaderModule(&hal.ShaderModuleDescriptor{
    Label: "Compute Shader",
    Source: hal.ShaderSource{
        WGSL: wgslSource,
    },
})
if err != nil {
    log.Fatal("failed to create shader module:", err)
}
defer device.DestroyShaderModule(shaderModule)
```

The `naga` shader compiler translates WGSL to the backend's native format:
- Vulkan: WGSL -> SPIR-V
- DX12: WGSL -> HLSL -> DXBC
- Metal: WGSL -> MSL
- GLES: WGSL -> GLSL

### Step 2: Create a Bind Group Layout

Define the resource binding structure:

```go
bindGroupLayout, err := device.CreateBindGroupLayout(&hal.BindGroupLayoutDescriptor{
    Label: "Compute Bind Group Layout",
    Entries: []gputypes.BindGroupLayoutEntry{
        {
            Binding:    0,
            Visibility: gputypes.ShaderStageCompute,
            Buffer: &gputypes.BufferBindingLayout{
                Type: gputypes.BufferBindingTypeReadOnlyStorage,
            },
        },
        {
            Binding:    1,
            Visibility: gputypes.ShaderStageCompute,
            Buffer: &gputypes.BufferBindingLayout{
                Type: gputypes.BufferBindingTypeStorage,
            },
        },
    },
})
if err != nil {
    log.Fatal("failed to create bind group layout:", err)
}
defer device.DestroyBindGroupLayout(bindGroupLayout)
```

### Step 3: Create a Pipeline Layout

```go
pipelineLayout, err := device.CreatePipelineLayout(&hal.PipelineLayoutDescriptor{
    Label:            "Compute Pipeline Layout",
    BindGroupLayouts: []hal.BindGroupLayout{bindGroupLayout},
})
if err != nil {
    log.Fatal("failed to create pipeline layout:", err)
}
defer device.DestroyPipelineLayout(pipelineLayout)
```

### Step 4: Create the Compute Pipeline

```go
pipeline, err := device.CreateComputePipeline(&hal.ComputePipelineDescriptor{
    Label:  "Compute Pipeline",
    Layout: pipelineLayout,
    Compute: hal.ComputeState{
        Module:     shaderModule,
        EntryPoint: "main",
    },
})
if err != nil {
    log.Fatal("failed to create compute pipeline:", err)
}
defer device.DestroyComputePipeline(pipeline)
```

## Creating Buffers

Compute shaders read from and write to GPU buffers.

### Storage Buffers

Storage buffers are the primary way to pass data to and from compute shaders:

```go
// Input buffer: written by CPU, read by shader
inputBuffer, err := device.CreateBuffer(&hal.BufferDescriptor{
    Label: "Input Buffer",
    Size:  uint64(len(inputData)) * 4, // f32 = 4 bytes
    Usage: gputypes.BufferUsageStorage | gputypes.BufferUsageCopyDst,
})

// Output buffer: written by shader, read back by CPU
outputBuffer, err := device.CreateBuffer(&hal.BufferDescriptor{
    Label: "Output Buffer",
    Size:  uint64(len(inputData)) * 4,
    Usage: gputypes.BufferUsageStorage | gputypes.BufferUsageCopySrc,
})

// Readback buffer: CPU-readable staging buffer
readbackBuffer, err := device.CreateBuffer(&hal.BufferDescriptor{
    Label: "Readback Buffer",
    Size:  uint64(len(inputData)) * 4,
    Usage: gputypes.BufferUsageMapRead | gputypes.BufferUsageCopyDst,
})
```

### Uniform Buffers

For small, frequently updated data (e.g., parameters, dimensions):

```go
uniformBuffer, err := device.CreateBuffer(&hal.BufferDescriptor{
    Label: "Uniform Buffer",
    Size:  16, // e.g., vec4<f32>
    Usage: gputypes.BufferUsageUniform | gputypes.BufferUsageCopyDst,
})
```

### Writing Data to Buffers

Use `Queue.WriteBuffer` to upload data from CPU to GPU:

```go
// Convert float32 slice to bytes
data := []float32{1.0, 2.0, 3.0, 4.0}
byteData := unsafe.Slice((*byte)(unsafe.Pointer(&data[0])), len(data)*4)
queue.WriteBuffer(inputBuffer, 0, byteData)
```

## Creating Bind Groups

Bind groups connect actual GPU resources to the layout:

```go
bindGroup, err := device.CreateBindGroup(&hal.BindGroupDescriptor{
    Label:  "Compute Bind Group",
    Layout: bindGroupLayout,
    Entries: []gputypes.BindGroupEntry{
        {
            Binding: 0,
            Resource: gputypes.BufferBinding{
                Buffer: inputBuffer.NativeHandle(),
                Offset: 0,
                Size:   inputBufferSize,
            },
        },
        {
            Binding: 1,
            Resource: gputypes.BufferBinding{
                Buffer: outputBuffer.NativeHandle(),
                Offset: 0,
                Size:   outputBufferSize,
            },
        },
    },
})
if err != nil {
    log.Fatal("failed to create bind group:", err)
}
defer device.DestroyBindGroup(bindGroup)
```

## Dispatching Workgroups

### Recording Commands

```go
encoder, err := device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{
    Label: "Compute Encoder",
})
if err != nil {
    log.Fatal("failed to create encoder:", err)
}

err = encoder.BeginEncoding("Compute Commands")
if err != nil {
    log.Fatal("failed to begin encoding:", err)
}

// Begin compute pass
computePass := encoder.BeginComputePass(&hal.ComputePassDescriptor{
    Label: "Compute Pass",
})

// Bind pipeline and resources
computePass.SetPipeline(pipeline)
computePass.SetBindGroup(0, bindGroup, nil)

// Dispatch workgroups
// If data has 1024 elements and workgroup_size is 64:
// we need ceil(1024/64) = 16 workgroups
numWorkgroups := (numElements + 63) / 64
computePass.Dispatch(numWorkgroups, 1, 1)

// End compute pass
computePass.End()
```

### Indirect Dispatch

For GPU-driven workload sizes, use `DispatchIndirect`:

```go
// Buffer contains: { x: u32, y: u32, z: u32 }
computePass.DispatchIndirect(indirectBuffer, 0)
```

## Reading Back Results

After the compute shader writes to the output buffer, copy the results to a CPU-readable buffer and read them back.

### Step 1: Copy Output to Readback Buffer

```go
// Transition and copy
encoder.CopyBufferToBuffer(
    outputBuffer, readbackBuffer,
    []hal.BufferCopy{{
        SrcOffset: 0,
        DstOffset: 0,
        Size:      outputBufferSize,
    }},
)

// Finish encoding
cmdBuffer, err := encoder.EndEncoding()
if err != nil {
    log.Fatal("failed to finish encoding:", err)
}
```

### Step 2: Submit and Wait

```go
fence, err := device.CreateFence()
if err != nil {
    log.Fatal("failed to create fence:", err)
}
defer device.DestroyFence(fence)

err = queue.Submit([]hal.CommandBuffer{cmdBuffer}, fence, 1)
if err != nil {
    log.Fatal("failed to submit:", err)
}

// Wait for GPU to finish
ok, err := device.Wait(fence, 1, 5*time.Second)
if err != nil || !ok {
    log.Fatal("GPU wait failed:", err)
}
```

### Step 3: Read Back Data

```go
resultBytes := make([]byte, outputBufferSize)
err = queue.ReadBuffer(readbackBuffer, 0, resultBytes)
if err != nil {
    log.Fatal("failed to read buffer:", err)
}

// Convert bytes back to float32 slice
results := unsafe.Slice((*float32)(unsafe.Pointer(&resultBytes[0])), numElements)
```

## Timestamp Queries for Profiling

You can measure GPU execution time of compute passes using timestamp queries.

### Creating a Query Set

```go
querySet, err := device.CreateQuerySet(&hal.QuerySetDescriptor{
    Label: "Timestamp Queries",
    Type:  hal.QueryTypeTimestamp,
    Count: 2, // begin + end
})
if err != nil {
    // Backend may not support timestamps
    log.Println("timestamps not supported:", err)
}
defer device.DestroyQuerySet(querySet)
```

### Using Timestamps in a Compute Pass

```go
beginIdx := uint32(0)
endIdx := uint32(1)

computePass := encoder.BeginComputePass(&hal.ComputePassDescriptor{
    Label: "Timed Compute Pass",
    TimestampWrites: &hal.ComputePassTimestampWrites{
        QuerySet:                  querySet,
        BeginningOfPassWriteIndex: &beginIdx,
        EndOfPassWriteIndex:       &endIdx,
    },
})

// ... dispatch work ...
computePass.End()
```

### Reading Timestamp Results

```go
// Create a buffer for timestamp results (2 * uint64 = 16 bytes)
timestampBuffer, _ := device.CreateBuffer(&hal.BufferDescriptor{
    Label: "Timestamp Buffer",
    Size:  16,
    Usage: gputypes.BufferUsageCopyDst | gputypes.BufferUsageMapRead,
})

// Resolve timestamps into the buffer
encoder.ResolveQuerySet(querySet, 0, 2, timestampBuffer, 0)

// After submit + wait, read back and compute elapsed time
timestampBytes := make([]byte, 16)
queue.ReadBuffer(timestampBuffer, 0, timestampBytes)

timestamps := unsafe.Slice((*uint64)(unsafe.Pointer(&timestampBytes[0])), 2)
begin := timestamps[0]
end := timestamps[1]

// Convert to nanoseconds using the timestamp period
period := queue.GetTimestampPeriod()
elapsedNs := float64(end-begin) * float64(period)
fmt.Printf("Compute pass took %.3f ms\n", elapsedNs/1e6)
```

## Error Handling

### Backend-Specific Errors

```go
import "github.com/gogpu/wgpu/hal"

pipeline, err := device.CreateComputePipeline(desc)
if err != nil {
    // Check for specific error types
    if errors.Is(err, hal.ErrDeviceOutOfMemory) {
        // Reduce buffer sizes or batch work
    }
    if errors.Is(err, hal.ErrDriverBug) {
        // Try a different backend
    }
    log.Fatal("pipeline creation failed:", err)
}
```

### Timestamp Support

Not all backends support timestamp queries:

```go
querySet, err := device.CreateQuerySet(&hal.QuerySetDescriptor{
    Type:  hal.QueryTypeTimestamp,
    Count: 2,
})
if errors.Is(err, hal.ErrTimestampsNotSupported) {
    // Fall back to CPU timing
    log.Println("GPU timestamps not available, using CPU timing")
}
```

## Complete Example

See `examples/compute-sum/` for a working example of a compute shader that sums array elements.

## Further Reading

- [Backend Differences](compute-backends.md) -- per-backend capabilities and limits
- [WebGPU Compute Specification](https://www.w3.org/TR/webgpu/#compute-passes)
- [WGSL Specification](https://www.w3.org/TR/WGSL/)
