// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

// Package dx12 provides a DirectX 12 backend for the HAL.
//
// # Status: PLANNED
//
// High-performance Windows backend.
//
// # References
//
//   - d3d12-rs    - Rust DX12 bindings
//   - Gio         - Has DX11 backend (study patterns)
//   - directx-go  - Community DX bindings
//
// # Pure Go Approach
//
// Use syscall for Windows COM APIs.
// Direct DXGI/D3D12 via vtable calls.
package dx12
