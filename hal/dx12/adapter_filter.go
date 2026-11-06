// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows && !(js && wasm)

package dx12

import (
	"slices"
	"strings"

	"github.com/gogpu/wgpu/hal/dx12/dxgi"
)

const (
	// warpVendorID is Microsoft's PCI vendor ID (5140), used by WARP.
	warpVendorID = 0x1414
	// warpAdapterName is the adapter description of WARP.
	warpAdapterName = "Microsoft Basic Render Driver"
	// intelVendorID is Intel's PCI vendor ID.
	intelVendorID = 0x8086
)

// haswellDeviceIDs lists the Intel Haswell iGPUs. D3D12 support was removed
// from them for a security vulnerability, and they report device loss after
// D3D12CreateDevice succeeds. List from
// https://dgpu-docs.intel.com/devices/hardware-table.html, as in Rust wgpu-hal.
var haswellDeviceIDs = []uint32{
	0x0422, 0x0426, 0x042A, 0x042B, 0x042E, 0x0C22, 0x0C26, 0x0C2A, 0x0C2B, 0x0C2E, 0x0A22,
	0x0A2A, 0x0A2B, 0x0D2A, 0x0D2B, 0x0D2E, 0x0A26, 0x0A2E, 0x0D22, 0x0D26, 0x0412, 0x0416,
	0x0D12, 0x041A, 0x041B, 0x0C12, 0x0C16, 0x0C1A, 0x0C1B, 0x0C1E, 0x0A12, 0x0A1A, 0x0A1B,
	0x0D16, 0x0D1A, 0x0D1B, 0x0D1E, 0x041E, 0x0A16, 0x0A1E, 0x0402, 0x0406, 0x040A, 0x040B,
	0x040E, 0x0C02, 0x0C06, 0x0C0A, 0x0C0B, 0x0C0E, 0x0A02, 0x0A06, 0x0A0A, 0x0A0B, 0x0A0E,
	0x0D02, 0x0D06, 0x0D0A, 0x0D0B, 0x0D0E,
}

// shouldKeepAdapter reports whether an enumerated DXGI adapter is exposed.
// It follows Rust wgpu-hal (wgpu-hal/src/auxil/dxgi/factory.rs,
// should_keep_adapter):
//
//   - Intel Haswell iGPUs are dropped (see haswellDeviceIDs).
//   - WARP flagged DXGI_ADAPTER_FLAG_SOFTWARE is kept; deviceType reports it
//     as DeviceTypeCPU.
//   - On a machine without a GPU (a CI runner), Windows also lists an
//     unflagged "Microsoft Basic Render Driver" so that applications that
//     filter software adapters still run. That twin pretends to be an
//     integrated GPU, so it is dropped: the flagged WARP stands for it.
func shouldKeepAdapter(desc *dxgi.DXGI_ADAPTER_DESC1) bool {
	if desc.VendorID == intelVendorID && slices.Contains(haswellDeviceIDs, desc.DeviceID) {
		return false
	}
	if desc.VendorID == warpVendorID && desc.Flags&dxgi.DXGI_ADAPTER_FLAG_SOFTWARE == 0 &&
		strings.Contains(utf16ToString(desc.Description[:]), warpAdapterName) {
		return false
	}
	return true
}
