// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows && !(js && wasm)

// DirectComposition COM bindings for per-pixel alpha transparency on DX12.
//
// DirectComposition (dcomp.dll) is required when a swap chain needs
// DXGI_ALPHA_MODE_PREMULTIPLIED — the standard CreateSwapChainForHwnd path
// does not support per-pixel alpha. Instead, we create the swap chain via
// CreateSwapChainForComposition and attach it to an IDCompositionVisual,
// which is rooted on an IDCompositionTarget bound to the HWND.
//
// Rust wgpu reference: wgpu-hal/src/dx12/dcomp.rs

package dx12

import (
	"fmt"
	"sync"
	"syscall"
	"unsafe"

	"github.com/gogpu/wgpu/hal/dx12/d3d12"
	"github.com/gogpu/wgpu/hal/dx12/dxgi"
)

// ---------------------------------------------------------------------------
// dcomp.dll lazy loading
// ---------------------------------------------------------------------------

var (
	dcompOnce          sync.Once
	dcompDLL           *syscall.LazyDLL
	dcompCreateDevice2 *syscall.LazyProc
	errDCompLoad       error
)

func loadDComp() error {
	dcompOnce.Do(func() {
		dcompDLL = syscall.NewLazyDLL("dcomp.dll")
		dcompCreateDevice2 = dcompDLL.NewProc("DCompositionCreateDevice2")
		errDCompLoad = dcompCreateDevice2.Find()
	})
	return errDCompLoad
}

// dcompAvailable reports whether dcomp.dll can be loaded on this system.
// Windows 8+ ships dcomp.dll; older systems or Server Core may not.
func dcompAvailable() bool {
	return loadDComp() == nil
}

// ---------------------------------------------------------------------------
// IID for IDCompositionDevice
// ---------------------------------------------------------------------------

// iidIDCompositionDevice is the COM interface ID for IDCompositionDevice.
// {C37EA93A-E7AA-450D-B16F-9746CB0407F3}
var iidIDCompositionDevice = d3d12.GUID{
	Data1: 0xC37EA93A,
	Data2: 0xE7AA,
	Data3: 0x450D,
	Data4: [8]byte{0xB1, 0x6F, 0x97, 0x46, 0xCB, 0x04, 0x07, 0xF3},
}

// ---------------------------------------------------------------------------
// COM interface vtbl structs
// ---------------------------------------------------------------------------

// idcompositionDeviceVtbl is the vtable layout for IDCompositionDevice.
// Inherits IUnknown (QueryInterface, AddRef, Release).
type idcompositionDeviceVtbl struct {
	// IUnknown
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr

	// IDCompositionDevice
	Commit                  uintptr // vtbl index 3
	WaitForCommitCompletion uintptr // vtbl index 4
	GetFrameStatistics      uintptr // vtbl index 5
	CreateTargetForHwnd     uintptr // vtbl index 6
	CreateVisual            uintptr // vtbl index 7
}

// idcompositionDevice wraps a raw COM pointer to IDCompositionDevice.
type idcompositionDevice struct {
	vtbl *idcompositionDeviceVtbl
}

// Release decrements the reference count. Safe to call on nil.
func (d *idcompositionDevice) Release() {
	if d == nil {
		return
	}
	//nolint:errcheck // COM Release returns remaining refcount, not an error.
	syscall.SyscallN(d.vtbl.Release, uintptr(unsafe.Pointer(d)))
}

// Commit commits all pending DirectComposition commands.
func (d *idcompositionDevice) Commit() error {
	ret, _, _ := syscall.SyscallN(d.vtbl.Commit, uintptr(unsafe.Pointer(d)))
	if ret != 0 {
		return d3d12.HRESULTError(ret)
	}
	return nil
}

// CreateTargetForHwnd creates a composition target for the specified window.
// topmost controls the z-order: false places the visual tree behind the HWND
// children, true places it in front.
func (d *idcompositionDevice) CreateTargetForHwnd(hwnd uintptr, topmost bool) (*idcompositionTarget, error) {
	var target *idcompositionTarget
	var topmostInt uintptr
	if topmost {
		topmostInt = 1
	}

	ret, _, _ := syscall.SyscallN(
		d.vtbl.CreateTargetForHwnd,
		uintptr(unsafe.Pointer(d)),
		hwnd,
		topmostInt,
		uintptr(unsafe.Pointer(&target)),
	)

	if ret != 0 {
		return nil, d3d12.HRESULTError(ret)
	}
	return target, nil
}

// CreateVisual creates a new composition visual.
func (d *idcompositionDevice) CreateVisual() (*idcompositionVisual, error) {
	var visual *idcompositionVisual

	ret, _, _ := syscall.SyscallN(
		d.vtbl.CreateVisual,
		uintptr(unsafe.Pointer(d)),
		uintptr(unsafe.Pointer(&visual)),
	)

	if ret != 0 {
		return nil, d3d12.HRESULTError(ret)
	}
	return visual, nil
}

// ---------------------------------------------------------------------------
// IDCompositionTarget
// ---------------------------------------------------------------------------

// idcompositionTargetVtbl is the vtable layout for IDCompositionTarget.
// Inherits IUnknown (QueryInterface, AddRef, Release).
type idcompositionTargetVtbl struct {
	// IUnknown
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr

	// IDCompositionTarget
	SetRoot uintptr // vtbl index 3
}

// idcompositionTarget wraps a raw COM pointer to IDCompositionTarget.
type idcompositionTarget struct {
	vtbl *idcompositionTargetVtbl
}

// Release decrements the reference count. Safe to call on nil.
func (t *idcompositionTarget) Release() {
	if t == nil {
		return
	}
	//nolint:errcheck // COM Release returns remaining refcount, not an error.
	syscall.SyscallN(t.vtbl.Release, uintptr(unsafe.Pointer(t)))
}

// SetRoot sets the root visual for this composition target.
func (t *idcompositionTarget) SetRoot(visual *idcompositionVisual) error {
	ret, _, _ := syscall.SyscallN(
		t.vtbl.SetRoot,
		uintptr(unsafe.Pointer(t)),
		uintptr(unsafe.Pointer(visual)),
	)

	if ret != 0 {
		return d3d12.HRESULTError(ret)
	}
	return nil
}

// ---------------------------------------------------------------------------
// IDCompositionVisual
// ---------------------------------------------------------------------------

// idcompositionVisualVtbl is the vtable layout for IDCompositionVisual.
// Inherits IUnknown (QueryInterface, AddRef, Release).
//
// COM overloaded methods occupy separate vtbl slots. Field names use the
// overload suffix (e.g., SetOffsetXAnimation vs SetOffsetXFloat) rather than
// underscores to satisfy Go naming conventions. We only call SetContent (index 15);
// the remaining slots are reserved for correct vtbl layout.
type idcompositionVisualVtbl struct {
	// IUnknown
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr

	// IDCompositionVisual
	SetOffsetXAnimation    uintptr // vtbl index 3  — SetOffsetX(IDCompositionAnimation*)
	SetOffsetXFloat        uintptr // vtbl index 4  — SetOffsetX(float)
	SetOffsetYAnimation    uintptr // vtbl index 5  — SetOffsetY(IDCompositionAnimation*)
	SetOffsetYFloat        uintptr // vtbl index 6  — SetOffsetY(float)
	SetTransformTransform  uintptr // vtbl index 7  — SetTransform(IDCompositionTransform*)
	SetTransformMatrix     uintptr // vtbl index 8  — SetTransform(D2D_MATRIX_3X2_F)
	SetTransformParent     uintptr // vtbl index 9
	SetEffect              uintptr // vtbl index 10
	SetBitmapInterpolation uintptr // vtbl index 11
	SetBorderMode          uintptr // vtbl index 12
	SetClipObject          uintptr // vtbl index 13 — SetClip(IDCompositionClip*)
	SetClipRect            uintptr // vtbl index 14 — SetClip(D2D_RECT_F)
	SetContent             uintptr // vtbl index 15
	AddVisual              uintptr // vtbl index 16
	RemoveVisual           uintptr // vtbl index 17
	RemoveAllVisuals       uintptr // vtbl index 18
	SetCompositeMode       uintptr // vtbl index 19
}

// idcompositionVisual wraps a raw COM pointer to IDCompositionVisual.
type idcompositionVisual struct {
	vtbl *idcompositionVisualVtbl
}

// Release decrements the reference count. Safe to call on nil.
func (v *idcompositionVisual) Release() {
	if v == nil {
		return
	}
	//nolint:errcheck // COM Release returns remaining refcount, not an error.
	syscall.SyscallN(v.vtbl.Release, uintptr(unsafe.Pointer(v)))
}

// setContent sets the content (typically a swap chain) for this visual.
// The content parameter is an IUnknown pointer — in our case it will be
// the IDXGISwapChain1 from CreateSwapChainForComposition.
func (v *idcompositionVisual) setContent(content unsafe.Pointer) error {
	ret, _, _ := syscall.SyscallN(
		v.vtbl.SetContent,
		uintptr(unsafe.Pointer(v)),
		uintptr(content),
	)

	if ret != 0 {
		return d3d12.HRESULTError(ret)
	}
	return nil
}

// ---------------------------------------------------------------------------
// dcompState — per-Surface lifecycle
// ---------------------------------------------------------------------------

// dcompState manages the DirectComposition visual tree for a single Surface.
// It owns three COM objects that must be released in reverse init order.
type dcompState struct {
	device *idcompositionDevice
	target *idcompositionTarget // must outlive visual
	visual *idcompositionVisual
}

// init creates the DComp device and visual tree for an HWND.
// Matches Rust wgpu InnerState::init: create device, create target, create
// visual, set root, ready for bindSwapChain + Commit.
func (s *dcompState) init(hwnd uintptr) error {
	if err := loadDComp(); err != nil {
		return fmt.Errorf("dcomp: %w", err)
	}

	// DCompositionCreateDevice2(NULL, IID_IDCompositionDevice, &device)
	// NULL renderingDevice = software composition device (sufficient for
	// our use — we only need the visual tree, DX12 does the actual rendering).
	var device *idcompositionDevice
	ret, _, _ := dcompCreateDevice2.Call(
		0, // renderingDevice = NULL
		uintptr(unsafe.Pointer(&iidIDCompositionDevice)),
		uintptr(unsafe.Pointer(&device)),
	)
	if ret != 0 {
		return fmt.Errorf("DCompositionCreateDevice2: %w", d3d12.HRESULTError(ret))
	}
	s.device = device

	// CreateTargetForHwnd — topmost=false places behind HWND children
	// (matches Rust wgpu: topmost=false).
	target, err := device.CreateTargetForHwnd(hwnd, false)
	if err != nil {
		s.release()
		return fmt.Errorf("IDCompositionDevice::CreateTargetForHwnd: %w", err)
	}
	s.target = target

	visual, err := device.CreateVisual()
	if err != nil {
		s.release()
		return fmt.Errorf("IDCompositionDevice::CreateVisual: %w", err)
	}
	s.visual = visual

	// Connect the visual to the target's root.
	if err := target.SetRoot(visual); err != nil {
		s.release()
		return fmt.Errorf("IDCompositionTarget::SetRoot: %w", err)
	}

	return nil
}

// bindSwapChain associates a swap chain with the DComp visual and commits
// the composition. The swap chain must have been created with
// CreateSwapChainForComposition (not CreateSwapChainForHwnd).
func (s *dcompState) bindSwapChain(swapchain *dxgi.IDXGISwapChain1) error {
	if s.visual == nil {
		return fmt.Errorf("dcomp: visual tree not initialized")
	}

	if err := s.visual.setContent(unsafe.Pointer(swapchain)); err != nil {
		return fmt.Errorf("IDCompositionVisual::SetContent: %w", err)
	}

	if err := s.device.Commit(); err != nil {
		return fmt.Errorf("IDCompositionDevice::Commit: %w", err)
	}

	return nil
}

// release tears down DComp objects in reverse init order: visual, target, device.
// Safe to call multiple times; safe to call on a zero-value dcompState.
func (s *dcompState) release() {
	// Reverse order: visual depends on target, target depends on device.
	s.visual.Release()
	s.visual = nil

	s.target.Release()
	s.target = nil

	s.device.Release()
	s.device = nil
}
