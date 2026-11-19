// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

// Package resource holds the concrete GPU resource wrappers of the native,
// rust and browser builds: Buffer, Texture, TextureView, Sampler,
// ShaderModule, QuerySet, Fence, bind-group layout, bind group, pipeline
// layout, the pooled MappedRange and MapPending handles. The public root
// package projects them through thin wrappers; core and the encoders record
// against them. The pipeline wrappers (RenderPipeline, ComputePipeline) are
// their own package, internal/pipeline, and follow the same contract.
//
// # Concurrency
//
// Instance, Adapter and Device are the concurrency boundary: every resource
// here is created on one of them, and their concurrent method calls are safe.
// Their own Release — and Surface's and Queue's — is not yet compare-and-swap
// guarded (the P1-CORE-044 remainder, which also covers the internal snatch
// guards), so one of those types must not be released from two goroutines at
// once. Release of the resource wrappers below is safe to call from any
// number of goroutines at once, with itself and with the resource's other
// methods. The released flag of every destroyable type is a compare-and-swap
// guard allocated by its constructor, so exactly one caller wins and at most
// one destroy is scheduled: the winner drops the application's reference
// (ADR-056, refcount-driven destruction; the HAL object dies when the last
// reference drops, on GPU completion for recorded work) and every other
// caller returns immediately. MappedRange and MapPending are pooled: their
// guard is re-armed by the pool acquire, and it admits exactly one return of
// the handle to its pool, so one object is never handed to two owners. A
// zero-value resource never ran a constructor: it has no guard, and its
// Release returns without touching a HAL object. That internal nil-safety is
// not a public zero-value promise: the public wrappers of Sampler,
// BindGroupLayout, PipelineLayout, ShaderModule and Fence keep their pinned
// pre-move panic on a zero value (they have no implementation to release), as
// the root zero-value tests record.
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
//     a time, and a mapped range is invalid after Unmap or Release. Release
//     checks its guard before touching the range's fields, so it does not
//     reorder against Unmap; Bytes, Flush, Status and Wait stay
//     caller-serialized with Unmap and with Release.
//   - A surface texture is valid only for the acquisition it was created in;
//     acquire and present are serialized by the render loop.
//   - TextureView.SetInternalUsage is a post-construction write: call it while
//     the view is still private to its creator, never concurrently with
//     readers.
package resource
