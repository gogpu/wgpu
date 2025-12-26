// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

package dxgi

import (
	"unicode/utf16"
)

// LUID represents a locally unique identifier for an adapter.
type LUID struct {
	LowPart  uint32
	HighPart int32
}

// DXGI_ADAPTER_DESC1 describes an adapter (graphics card).
type DXGI_ADAPTER_DESC1 struct {
	Description           [128]uint16
	VendorID              uint32
	DeviceID              uint32
	SubSysID              uint32
	Revision              uint32
	DedicatedVideoMemory  uint64
	DedicatedSystemMemory uint64
	SharedSystemMemory    uint64
	AdapterLuid           LUID
	Flags                 DXGI_ADAPTER_FLAG
}

// DescriptionString returns the adapter description as a Go string.
func (d *DXGI_ADAPTER_DESC1) DescriptionString() string {
	return utf16ToString(d.Description[:])
}

// DXGI_ADAPTER_DESC3 describes an adapter with extended information.
type DXGI_ADAPTER_DESC3 struct {
	Description                   [128]uint16
	VendorID                      uint32
	DeviceID                      uint32
	SubSysID                      uint32
	Revision                      uint32
	DedicatedVideoMemory          uint64
	DedicatedSystemMemory         uint64
	SharedSystemMemory            uint64
	AdapterLuid                   LUID
	Flags                         DXGI_ADAPTER_FLAG
	GraphicsPreemptionGranularity uint32
	ComputePreemptionGranularity  uint32
}

// DescriptionString returns the adapter description as a Go string.
func (d *DXGI_ADAPTER_DESC3) DescriptionString() string {
	return utf16ToString(d.Description[:])
}

// DXGI_SAMPLE_DESC describes multi-sampling parameters.
type DXGI_SAMPLE_DESC struct {
	Count   uint32
	Quality uint32
}

// DXGI_RATIONAL represents a rational number (numerator/denominator).
type DXGI_RATIONAL struct {
	Numerator   uint32
	Denominator uint32
}

// DXGI_MODE_DESC describes a display mode.
type DXGI_MODE_DESC struct {
	Width            uint32
	Height           uint32
	RefreshRate      DXGI_RATIONAL
	Format           DXGI_FORMAT
	ScanlineOrdering DXGI_MODE_SCANLINE_ORDER
	Scaling          DXGI_MODE_SCALING
}

// DXGI_MODE_DESC1 describes a display mode with stereo support.
type DXGI_MODE_DESC1 struct {
	Width            uint32
	Height           uint32
	RefreshRate      DXGI_RATIONAL
	Format           DXGI_FORMAT
	ScanlineOrdering DXGI_MODE_SCANLINE_ORDER
	Scaling          DXGI_MODE_SCALING
	Stereo           int32 // BOOL
}

// DXGI_SWAP_CHAIN_DESC describes a swap chain.
type DXGI_SWAP_CHAIN_DESC struct {
	BufferDesc   DXGI_MODE_DESC
	SampleDesc   DXGI_SAMPLE_DESC
	BufferUsage  DXGI_USAGE
	BufferCount  uint32
	OutputWindow uintptr // HWND
	Windowed     int32   // BOOL
	SwapEffect   DXGI_SWAP_EFFECT
	Flags        uint32
}

// DXGI_SWAP_CHAIN_DESC1 describes a swap chain (extended).
type DXGI_SWAP_CHAIN_DESC1 struct {
	Width       uint32
	Height      uint32
	Format      DXGI_FORMAT
	Stereo      int32 // BOOL
	SampleDesc  DXGI_SAMPLE_DESC
	BufferUsage DXGI_USAGE
	BufferCount uint32
	Scaling     DXGI_SCALING
	SwapEffect  DXGI_SWAP_EFFECT
	AlphaMode   DXGI_ALPHA_MODE
	Flags       uint32
}

// DXGI_SWAP_CHAIN_FULLSCREEN_DESC describes fullscreen swap chain settings.
type DXGI_SWAP_CHAIN_FULLSCREEN_DESC struct {
	RefreshRate      DXGI_RATIONAL
	ScanlineOrdering DXGI_MODE_SCANLINE_ORDER
	Scaling          DXGI_MODE_SCALING
	Windowed         int32 // BOOL
}

// DXGI_OUTPUT_DESC describes an adapter output (display).
type DXGI_OUTPUT_DESC struct {
	DeviceName         [32]uint16
	DesktopCoordinates RECT
	AttachedToDesktop  int32 // BOOL
	Rotation           DXGI_MODE_ROTATION
	Monitor            uintptr // HMONITOR
}

// DeviceNameString returns the device name as a Go string.
func (d *DXGI_OUTPUT_DESC) DeviceNameString() string {
	return utf16ToString(d.DeviceName[:])
}

// RECT represents a Windows RECT structure.
type RECT struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

// DXGI_FRAME_STATISTICS describes frame statistics.
type DXGI_FRAME_STATISTICS struct {
	PresentCount        uint32
	PresentRefreshCount uint32
	SyncRefreshCount    uint32
	SyncQPCTime         int64 // LARGE_INTEGER
	SyncGPUTime         int64 // LARGE_INTEGER
}

// DXGI_PRESENT_PARAMETERS describes present parameters.
type DXGI_PRESENT_PARAMETERS struct {
	DirtyRectsCount uint32
	DirtyRects      *RECT
	ScrollRect      *RECT
	ScrollOffset    *POINT
}

// POINT represents a Windows POINT structure.
type POINT struct {
	X int32
	Y int32
}

// utf16ToString converts a null-terminated UTF-16 slice to a Go string.
func utf16ToString(s []uint16) string {
	// Find null terminator
	for i, v := range s {
		if v == 0 {
			return string(utf16.Decode(s[:i]))
		}
	}
	return string(utf16.Decode(s))
}
