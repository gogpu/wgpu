// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

package d3d12

import (
	"syscall"
	"unsafe"
)

// -----------------------------------------------------------------------------
// DRED Enums
// -----------------------------------------------------------------------------

// D3D12DREDEnablement controls DRED feature enablement.
type D3D12DREDEnablement int32

const (
	// D3D12DREDEnablementSystemControlled lets the system decide.
	D3D12DREDEnablementSystemControlled D3D12DREDEnablement = 0
	// D3D12DREDEnablementForcedOff disables the feature.
	D3D12DREDEnablementForcedOff D3D12DREDEnablement = 1
	// D3D12DREDEnablementForcedOn enables the feature.
	D3D12DREDEnablementForcedOn D3D12DREDEnablement = 2
)

// D3D12AutoBreadcrumbOp identifies the type of GPU operation recorded
// as an auto-breadcrumb by DRED.
type D3D12AutoBreadcrumbOp int32

const (
	D3D12AutoBreadcrumbOpSetMarker                                        D3D12AutoBreadcrumbOp = 0
	D3D12AutoBreadcrumbOpBeginEvent                                       D3D12AutoBreadcrumbOp = 1
	D3D12AutoBreadcrumbOpEndEvent                                         D3D12AutoBreadcrumbOp = 2
	D3D12AutoBreadcrumbOpDrawInstanced                                    D3D12AutoBreadcrumbOp = 3
	D3D12AutoBreadcrumbOpDrawIndexedInstanced                             D3D12AutoBreadcrumbOp = 4
	D3D12AutoBreadcrumbOpExecuteIndirect                                  D3D12AutoBreadcrumbOp = 5
	D3D12AutoBreadcrumbOpDispatch                                         D3D12AutoBreadcrumbOp = 6
	D3D12AutoBreadcrumbOpCopyBufferRegion                                 D3D12AutoBreadcrumbOp = 7
	D3D12AutoBreadcrumbOpCopyTextureRegion                                D3D12AutoBreadcrumbOp = 8
	D3D12AutoBreadcrumbOpCopyResource                                     D3D12AutoBreadcrumbOp = 9
	D3D12AutoBreadcrumbOpCopyTiles                                        D3D12AutoBreadcrumbOp = 10
	D3D12AutoBreadcrumbOpResolveSubresource                               D3D12AutoBreadcrumbOp = 11
	D3D12AutoBreadcrumbOpClearRenderTargetView                            D3D12AutoBreadcrumbOp = 12
	D3D12AutoBreadcrumbOpClearUnorderedAccessView                         D3D12AutoBreadcrumbOp = 13
	D3D12AutoBreadcrumbOpClearDepthStencilView                            D3D12AutoBreadcrumbOp = 14
	D3D12AutoBreadcrumbOpResourceBarrier                                  D3D12AutoBreadcrumbOp = 15
	D3D12AutoBreadcrumbOpExecuteBundle                                    D3D12AutoBreadcrumbOp = 16
	D3D12AutoBreadcrumbOpPresent                                          D3D12AutoBreadcrumbOp = 17
	D3D12AutoBreadcrumbOpResolveQueryData                                 D3D12AutoBreadcrumbOp = 18
	D3D12AutoBreadcrumbOpBeginSubmission                                  D3D12AutoBreadcrumbOp = 19
	D3D12AutoBreadcrumbOpEndSubmission                                    D3D12AutoBreadcrumbOp = 20
	D3D12AutoBreadcrumbOpDecodeFrame                                      D3D12AutoBreadcrumbOp = 21
	D3D12AutoBreadcrumbOpProcessFrames                                    D3D12AutoBreadcrumbOp = 22
	D3D12AutoBreadcrumbOpAtomicCopyBufferUINT                             D3D12AutoBreadcrumbOp = 23
	D3D12AutoBreadcrumbOpAtomicCopyBufferUINT64                           D3D12AutoBreadcrumbOp = 24
	D3D12AutoBreadcrumbOpResolveSubresourceRegion                         D3D12AutoBreadcrumbOp = 25
	D3D12AutoBreadcrumbOpWriteBufferImmediate                             D3D12AutoBreadcrumbOp = 26
	D3D12AutoBreadcrumbOpDecodeFrame1                                     D3D12AutoBreadcrumbOp = 27
	D3D12AutoBreadcrumbOpSetProtectedResourceSession                      D3D12AutoBreadcrumbOp = 28
	D3D12AutoBreadcrumbOpDecodeFrame2                                     D3D12AutoBreadcrumbOp = 29
	D3D12AutoBreadcrumbOpProcessFrames1                                   D3D12AutoBreadcrumbOp = 30
	D3D12AutoBreadcrumbOpBuildRaytracingAccelerationStructure             D3D12AutoBreadcrumbOp = 31
	D3D12AutoBreadcrumbOpEmitRaytracingAccelerationStructurePostbuildInfo D3D12AutoBreadcrumbOp = 32
	D3D12AutoBreadcrumbOpCopyRaytracingAccelerationStructure              D3D12AutoBreadcrumbOp = 33
	D3D12AutoBreadcrumbOpDispatchRays                                     D3D12AutoBreadcrumbOp = 34
	D3D12AutoBreadcrumbOpInitializeMetaCommand                            D3D12AutoBreadcrumbOp = 35
	D3D12AutoBreadcrumbOpExecuteMetaCommand                               D3D12AutoBreadcrumbOp = 36
	D3D12AutoBreadcrumbOpEstimateMotion                                   D3D12AutoBreadcrumbOp = 37
	D3D12AutoBreadcrumbOpResolveMotionVectorHeap                          D3D12AutoBreadcrumbOp = 38
	D3D12AutoBreadcrumbOpSetPipelineState1                                D3D12AutoBreadcrumbOp = 39
	D3D12AutoBreadcrumbOpInitializeExtensionCommand                       D3D12AutoBreadcrumbOp = 40
	D3D12AutoBreadcrumbOpExecuteExtensionCommand                          D3D12AutoBreadcrumbOp = 41
	D3D12AutoBreadcrumbOpDispatchMesh                                     D3D12AutoBreadcrumbOp = 42
	D3D12AutoBreadcrumbOpEncodeFrame                                      D3D12AutoBreadcrumbOp = 43
	D3D12AutoBreadcrumbOpResolveEncoderOutputMetadata                     D3D12AutoBreadcrumbOp = 44
)

// AutoBreadcrumbOpName returns a human-readable name for the breadcrumb operation.
func AutoBreadcrumbOpName(op D3D12AutoBreadcrumbOp) string {
	switch op {
	case D3D12AutoBreadcrumbOpSetMarker:
		return "SetMarker"
	case D3D12AutoBreadcrumbOpBeginEvent:
		return "BeginEvent"
	case D3D12AutoBreadcrumbOpEndEvent:
		return "EndEvent"
	case D3D12AutoBreadcrumbOpDrawInstanced:
		return "DrawInstanced"
	case D3D12AutoBreadcrumbOpDrawIndexedInstanced:
		return "DrawIndexedInstanced"
	case D3D12AutoBreadcrumbOpExecuteIndirect:
		return "ExecuteIndirect"
	case D3D12AutoBreadcrumbOpDispatch:
		return "Dispatch"
	case D3D12AutoBreadcrumbOpCopyBufferRegion:
		return "CopyBufferRegion"
	case D3D12AutoBreadcrumbOpCopyTextureRegion:
		return "CopyTextureRegion"
	case D3D12AutoBreadcrumbOpCopyResource:
		return "CopyResource"
	case D3D12AutoBreadcrumbOpCopyTiles:
		return "CopyTiles"
	case D3D12AutoBreadcrumbOpResolveSubresource:
		return "ResolveSubresource"
	case D3D12AutoBreadcrumbOpClearRenderTargetView:
		return "ClearRenderTargetView"
	case D3D12AutoBreadcrumbOpClearUnorderedAccessView:
		return "ClearUnorderedAccessView"
	case D3D12AutoBreadcrumbOpClearDepthStencilView:
		return "ClearDepthStencilView"
	case D3D12AutoBreadcrumbOpResourceBarrier:
		return "ResourceBarrier"
	case D3D12AutoBreadcrumbOpExecuteBundle:
		return "ExecuteBundle"
	case D3D12AutoBreadcrumbOpPresent:
		return "Present"
	case D3D12AutoBreadcrumbOpResolveQueryData:
		return "ResolveQueryData"
	case D3D12AutoBreadcrumbOpBeginSubmission:
		return "BeginSubmission"
	case D3D12AutoBreadcrumbOpEndSubmission:
		return "EndSubmission"
	case D3D12AutoBreadcrumbOpDispatchRays:
		return "DispatchRays"
	case D3D12AutoBreadcrumbOpDispatchMesh:
		return "DispatchMesh"
	default:
		return "Unknown"
	}
}

// -----------------------------------------------------------------------------
// DRED Data Structures
// -----------------------------------------------------------------------------

// D3D12AutoBreadcrumbNode1 represents a single command list/queue breadcrumb record.
// This is a linked list node — pNext points to the next node.
//
// Layout matches D3D12_AUTO_BREADCRUMB_NODE1 exactly:
//
//	WCHAR*                                pCommandListDebugNameW;   // +0
//	WCHAR*                                pCommandQueueDebugNameW;  // +8
//	ID3D12GraphicsCommandList*            pCommandList;             // +16
//	ID3D12CommandQueue*                   pCommandQueue;            // +24
//	UINT                                  BreadcrumbCount;          // +32
//	const UINT*                           pLastBreadcrumbValue;     // +40
//	const D3D12_AUTO_BREADCRUMB_OP*       pCommandHistory;          // +48
//	D3D12_AUTO_BREADCRUMB_NODE1*          pNext;                    // +56
//	UINT                                  BreadcrumbContextsCount;  // +64
//	D3D12_DRED_BREADCRUMB_CONTEXT*        pBreadcrumbContexts;      // +72
type D3D12AutoBreadcrumbNode1 struct {
	CommandListDebugNameW   *uint16
	CommandQueueDebugNameW  *uint16
	CommandList             uintptr
	CommandQueue            uintptr
	BreadcrumbCount         uint32
	_                       [4]byte // padding for alignment
	LastBreadcrumbValue     *uint32
	CommandHistory          *D3D12AutoBreadcrumbOp
	Next                    *D3D12AutoBreadcrumbNode1
	BreadcrumbContextsCount uint32
	_                       [4]byte // padding for alignment
	BreadcrumbContexts      uintptr
}

// D3D12DREDAutoBreadcrumbsOutput1 is the output structure for DRED auto-breadcrumbs.
//
// Layout matches D3D12_DRED_AUTO_BREADCRUMBS_OUTPUT1:
//
//	D3D12_AUTO_BREADCRUMB_NODE1*  pHeadAutoBreadcrumbNode;  // +0
type D3D12DREDAutoBreadcrumbsOutput1 struct {
	HeadAutoBreadcrumbNode *D3D12AutoBreadcrumbNode1
}

// D3D12DREDAllocationNode1 represents a single allocation tracked by DRED for page fault reporting.
//
// Layout matches D3D12_DRED_ALLOCATION_NODE1:
//
//	const char*                          ObjectNameA;         // +0
//	const WCHAR*                         ObjectNameW;         // +8
//	D3D12_DRED_ALLOCATION_TYPE           AllocationType;      // +16
//	D3D12_DRED_ALLOCATION_NODE1*         pNext;               // +24
type D3D12DREDAllocationNode1 struct {
	ObjectNameA    *byte
	ObjectNameW    *uint16
	AllocationType int32
	_              [4]byte // padding
	Next           *D3D12DREDAllocationNode1
}

// D3D12DREDPageFaultOutput1 is the output structure for DRED page fault information.
//
// Layout matches D3D12_DRED_PAGE_FAULT_OUTPUT1:
//
//	D3D12_GPU_VIRTUAL_ADDRESS             PageFaultVA;                        // +0
//	D3D12_DRED_ALLOCATION_NODE1*          pHeadExistingAllocationNode;        // +8
//	D3D12_DRED_ALLOCATION_NODE1*          pHeadRecentFreedAllocationNode;     // +16
type D3D12DREDPageFaultOutput1 struct {
	PageFaultVA                   uint64
	HeadExistingAllocationNode    *D3D12DREDAllocationNode1
	HeadRecentFreedAllocationNode *D3D12DREDAllocationNode1
}

// -----------------------------------------------------------------------------
// ID3D12DeviceRemovedExtendedDataSettings COM interface
// -----------------------------------------------------------------------------

// ID3D12DeviceRemovedExtendedDataSettings enables DRED features before device creation.
// GUID: {82BC481C-6B9B-4030-AEDB-7EE3D1DF1E63}
type ID3D12DeviceRemovedExtendedDataSettings struct {
	vtbl *id3d12DREDSettingsVtbl
}

type id3d12DREDSettingsVtbl struct {
	// IUnknown
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr

	// ID3D12DeviceRemovedExtendedDataSettings
	SetAutoBreadcrumbsEnablement uintptr
	SetPageFaultEnablement       uintptr
	SetWatsonDumpEnablement      uintptr
}

// Release decrements the reference count.
func (s *ID3D12DeviceRemovedExtendedDataSettings) Release() uint32 {
	ret, _, _ := syscall.Syscall(
		s.vtbl.Release,
		1,
		uintptr(unsafe.Pointer(s)),
		0, 0,
	)
	return uint32(ret)
}

// SetAutoBreadcrumbsEnablement enables or disables auto-breadcrumb recording.
func (s *ID3D12DeviceRemovedExtendedDataSettings) SetAutoBreadcrumbsEnablement(enablement D3D12DREDEnablement) {
	_, _, _ = syscall.Syscall(
		s.vtbl.SetAutoBreadcrumbsEnablement,
		2,
		uintptr(unsafe.Pointer(s)),
		uintptr(enablement),
		0,
	)
}

// SetPageFaultEnablement enables or disables page fault tracking.
func (s *ID3D12DeviceRemovedExtendedDataSettings) SetPageFaultEnablement(enablement D3D12DREDEnablement) {
	_, _, _ = syscall.Syscall(
		s.vtbl.SetPageFaultEnablement,
		2,
		uintptr(unsafe.Pointer(s)),
		uintptr(enablement),
		0,
	)
}

// -----------------------------------------------------------------------------
// ID3D12DeviceRemovedExtendedData1 COM interface
// -----------------------------------------------------------------------------

// ID3D12DeviceRemovedExtendedData1 queries DRED data after device removal.
// Obtained via QueryInterface on the device.
// GUID: {8727A009-F2F4-424F-8B91-B9C9C472D8E6}
type ID3D12DeviceRemovedExtendedData1 struct {
	vtbl *id3d12DRED1Vtbl
}

type id3d12DRED1Vtbl struct {
	// IUnknown
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr

	// ID3D12DeviceRemovedExtendedData
	GetAutoBreadcrumbsOutput uintptr

	// ID3D12DeviceRemovedExtendedData1
	GetAutoBreadcrumbsOutput1     uintptr
	GetPageFaultAllocationOutput1 uintptr
}

// Release decrements the reference count.
func (d *ID3D12DeviceRemovedExtendedData1) Release() uint32 {
	ret, _, _ := syscall.Syscall(
		d.vtbl.Release,
		1,
		uintptr(unsafe.Pointer(d)),
		0, 0,
	)
	return uint32(ret)
}

// GetAutoBreadcrumbsOutput1 retrieves the DRED auto-breadcrumb data.
func (d *ID3D12DeviceRemovedExtendedData1) GetAutoBreadcrumbsOutput1(output *D3D12DREDAutoBreadcrumbsOutput1) error {
	ret, _, _ := syscall.Syscall(
		d.vtbl.GetAutoBreadcrumbsOutput1,
		2,
		uintptr(unsafe.Pointer(d)),
		uintptr(unsafe.Pointer(output)),
		0,
	)
	if ret != 0 {
		return HRESULTError(ret)
	}
	return nil
}

// GetPageFaultAllocationOutput1 retrieves page fault allocation data.
func (d *ID3D12DeviceRemovedExtendedData1) GetPageFaultAllocationOutput1(output *D3D12DREDPageFaultOutput1) error {
	ret, _, _ := syscall.Syscall(
		d.vtbl.GetPageFaultAllocationOutput1,
		2,
		uintptr(unsafe.Pointer(d)),
		uintptr(unsafe.Pointer(output)),
		0,
	)
	if ret != 0 {
		return HRESULTError(ret)
	}
	return nil
}

// -----------------------------------------------------------------------------
// ID3D12Device QueryInterface for DRED
// -----------------------------------------------------------------------------

// QueryDRED1 queries the device for the ID3D12DeviceRemovedExtendedData1 interface.
// Returns nil if DRED is not available (device created without DRED enabled,
// or Windows version too old).
func (d *ID3D12Device) QueryDRED1() *ID3D12DeviceRemovedExtendedData1 {
	var dred *ID3D12DeviceRemovedExtendedData1
	ret, _, _ := syscall.Syscall(
		d.vtbl.QueryInterface,
		3,
		uintptr(unsafe.Pointer(d)),
		uintptr(unsafe.Pointer(&IID_ID3D12DeviceRemovedExtendedData1)),
		uintptr(unsafe.Pointer(&dred)),
	)
	if ret != 0 {
		return nil
	}
	return dred
}

// -----------------------------------------------------------------------------
// Breadcrumb Operation Helpers
// -----------------------------------------------------------------------------

// BreadcrumbOps returns the command history as a Go slice.
// The returned slice references the original DRED memory and must not be
// used after the DRED interface is released.
func (n *D3D12AutoBreadcrumbNode1) BreadcrumbOps() []D3D12AutoBreadcrumbOp {
	if n.CommandHistory == nil || n.BreadcrumbCount == 0 {
		return nil
	}
	return unsafe.Slice(n.CommandHistory, n.BreadcrumbCount)
}

// LastCompleted returns the index of the last breadcrumb the GPU completed.
// Returns 0 if no breadcrumb value is available.
func (n *D3D12AutoBreadcrumbNode1) LastCompleted() uint32 {
	if n.LastBreadcrumbValue == nil {
		return 0
	}
	return *n.LastBreadcrumbValue
}
