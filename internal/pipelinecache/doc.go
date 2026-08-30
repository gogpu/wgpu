// Package pipelinecache provides shared disk persistence helpers for GPU driver
// pipeline caches (Vulkan VkPipelineCache and DX12 cached PSO blobs).
//
// This package is internal to wgpu. Backends own cache lifecycle; this package
// only handles adapter-scoped paths and atomic blob I/O.
package pipelinecache
