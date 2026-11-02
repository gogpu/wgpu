// Package raytracing provides core-level resource management for ray tracing
// acceleration structures (BLAS and TLAS).
//
// This package is internal to wgpu and not intended for direct use by
// applications; core/ray_tracing.go wraps it for the public API.
//
// Architecture follows Rust wgpu-core resource.rs (Blas/Tlas structs) with
// compaction state tracking and build dependency ordering.
package raytracing
