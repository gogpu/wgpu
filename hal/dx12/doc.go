// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

// Package dx12 provides a DirectX 12 backend for the HAL.
//
// # Status: WIP
//
// High-performance Windows backend using Pure Go COM vtable calls.
// Implements hal.Device, hal.Queue, hal.CommandBuffer, hal.Surface,
// hal.Buffer, hal.Texture, hal.RenderPipeline, hal.ComputePipeline,
// and all other HAL interfaces via syscall (zero CGO).
//
// # Sub-packages
//
//   - d3d12 — Low-level Direct3D 12 COM bindings
//   - dxgi  — DXGI adapter enumeration and swap chain management
//
// # References
//
//   - D3D12 API: https://learn.microsoft.com/en-us/windows/win32/api/_direct3d12/
//   - d3d12-rs  — Rust DX12 bindings (pattern reference)
package dx12
