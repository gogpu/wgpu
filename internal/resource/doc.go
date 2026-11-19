// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

// Package resource holds the concrete GPU resource wrappers of the native,
// rust and browser builds: Buffer, Texture, TextureView, Sampler,
// ShaderModule, QuerySet, Fence, bind-group layout, bind group, pipeline
// layout, the pipelines, MappedRange and MapPending. The public root package
// projects them through thin wrappers; core and the encoders record against
// them.
//
// # Concurrency
//
// Instance, Adapter and Device are safe for concurrent use, and every
// resource here is created on one of them. Release is safe to call from any
// number of goroutines at once, with itself and with the resource's other
// methods. The released flag of every destroyable type is a compare-and-swap
// guard allocated by its constructor, so exactly one caller wins and at most
// one HAL destroy is scheduled: the winner drops the application's reference
// (ADR-056, refcount-driven destruction; the HAL object dies when the last
// reference drops, on GPU completion for recorded work) and every other
// caller returns immediately. A zero-value resource never ran a constructor:
// it has no guard, and Release on it is a no-op.
//
// The rest of what a resource exposes is read after construction and is
// therefore safe for concurrent use. The exceptions come from the WebGPU
// model, not from the wrapper:
//
//   - Command recording (CommandEncoder, RenderPassEncoder,
//     ComputePassEncoder) is not thread-safe: record from one goroutine.
//   - Buffer mapping transitions (Map, MapAsync, MappedRange, Unmap) are
//     synchronized against each other by the device snatch lock, but the
//     spec's rule still holds: one mapping and one mapped range per buffer at
//     a time, and a mapped range is invalid after Unmap or Release.
//   - A surface texture is valid only for the acquisition it was created in;
//     acquire and present are serialized by the render loop.
//   - TextureView.SetInternalUsage is a post-construction write: call it while
//     the view is still private to its creator, never concurrently with
//     readers.
package resource
