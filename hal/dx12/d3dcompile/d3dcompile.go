// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

// Package d3dcompile provides Pure Go bindings to d3dcompiler_47.dll.
//
// D3DCompile compiles HLSL source code to DXBC bytecode using the
// D3DCompile function from d3dcompiler_47.dll. This DLL ships with
// Windows 10+ and requires no additional installation.
//
// Zero CGO — uses syscall.NewLazyDLL for dynamic loading.
package d3dcompile

import (
	"fmt"
	"sync"
	"syscall"
	"unsafe"
)

var (
	lib     *Lib
	libOnce sync.Once
	errLib  error
)

// Lib provides access to d3dcompiler_47.dll functions.
type Lib struct {
	dll        *syscall.LazyDLL
	d3dCompile *syscall.LazyProc
}

// Load loads d3dcompiler_47.dll. Safe to call multiple times.
func Load() (*Lib, error) {
	libOnce.Do(func() {
		lib, errLib = loadInternal()
	})
	return lib, errLib
}

func loadInternal() (*Lib, error) {
	dll := syscall.NewLazyDLL("d3dcompiler_47.dll")
	if err := dll.Load(); err != nil {
		return nil, fmt.Errorf("d3dcompile: failed to load d3dcompiler_47.dll: %w", err)
	}

	return &Lib{
		dll:        dll,
		d3dCompile: dll.NewProc("D3DCompile"),
	}, nil
}

// Shader model target profiles for D3DCompile.
const (
	TargetVS51 = "vs_5_1" // Vertex shader, Shader Model 5.1
	TargetPS51 = "ps_5_1" // Pixel (fragment) shader, Shader Model 5.1
	TargetCS51 = "cs_5_1" // Compute shader, Shader Model 5.1
)

// id3dBlobVtbl is the COM vtable for ID3DBlob.
type id3dBlobVtbl struct {
	QueryInterface   uintptr
	AddRef           uintptr
	Release          uintptr
	GetBufferPointer uintptr
	GetBufferSize    uintptr
}

// id3dBlob represents a COM ID3DBlob (ID3D10Blob) object.
type id3dBlob struct {
	vtbl *id3dBlobVtbl
}

// release decrements the reference count.
func (b *id3dBlob) release() {
	//nolint:errcheck // COM Release returns ref count, not error
	syscall.SyscallN(b.vtbl.Release, uintptr(unsafe.Pointer(b)))
}

// getBufferPointer returns a pointer to the blob data.
func (b *id3dBlob) getBufferPointer() unsafe.Pointer {
	var ptr unsafe.Pointer
	ret, _, _ := syscall.SyscallN(b.vtbl.GetBufferPointer, uintptr(unsafe.Pointer(b)))
	// Store return value via intermediate to satisfy go vet.
	// The returned pointer is valid for the lifetime of the blob.
	*(*uintptr)(unsafe.Pointer(&ptr)) = ret
	return ptr
}

// getBufferSize returns the size of the blob data in bytes.
func (b *id3dBlob) getBufferSize() int {
	ret, _, _ := syscall.SyscallN(b.vtbl.GetBufferSize, uintptr(unsafe.Pointer(b)))
	return int(ret)
}

// bytes returns the blob content as a copied byte slice.
func (b *id3dBlob) bytes() []byte {
	ptr := b.getBufferPointer()
	size := b.getBufferSize()
	if ptr == nil || size == 0 {
		return nil
	}
	result := make([]byte, size)
	copy(result, unsafe.Slice((*byte)(ptr), size))
	return result
}

// text returns the blob content as a string.
func (b *id3dBlob) text() string {
	data := b.bytes()
	if len(data) == 0 {
		return ""
	}
	return string(data)
}

// Compile compiles HLSL source code to DXBC bytecode.
//
// Parameters:
//   - source: HLSL source code
//   - entryPoint: entry point function name (e.g. "vs_main")
//   - target: shader model target (e.g. TargetVS51, TargetPS51, TargetCS51)
//
// Returns compiled DXBC bytecode or an error with the compiler error message.
func (l *Lib) Compile(source, entryPoint, target string) ([]byte, error) {
	return l.CompileWithFlags(source, entryPoint, target, 0, 0)
}

// CompileWithFlags compiles HLSL source code with explicit compiler flags.
func (l *Lib) CompileWithFlags(source, entryPoint, target string, flags1, flags2 uint32) ([]byte, error) {
	srcBytes := []byte(source)
	entryBytes := append([]byte(entryPoint), 0) // null-terminated
	targetBytes := append([]byte(target), 0)    // null-terminated

	var codeBlob *id3dBlob
	var errorBlob *id3dBlob

	// D3DCompile(pSrcData, SrcDataSize, pSourceName, pDefines, pInclude,
	//            pEntrypoint, pTarget, Flags1, Flags2, ppCode, ppErrorMsgs)
	ret, _, _ := syscall.SyscallN(
		l.d3dCompile.Addr(),
		uintptr(unsafe.Pointer(&srcBytes[0])),    // pSrcData
		uintptr(len(srcBytes)),                   // SrcDataSize
		0,                                        // pSourceName (NULL)
		0,                                        // pDefines (NULL)
		0,                                        // pInclude (NULL)
		uintptr(unsafe.Pointer(&entryBytes[0])),  // pEntrypoint
		uintptr(unsafe.Pointer(&targetBytes[0])), // pTarget
		uintptr(flags1),                          // Flags1
		uintptr(flags2),                          // Flags2
		uintptr(unsafe.Pointer(&codeBlob)),       // ppCode
		uintptr(unsafe.Pointer(&errorBlob)),      // ppErrorMsgs
	)

	// Release error blob after extracting message
	defer func() {
		if errorBlob != nil {
			errorBlob.release()
		}
	}()

	if int32(ret) < 0 {
		errMsg := "unknown error"
		if errorBlob != nil {
			if text := errorBlob.text(); text != "" {
				errMsg = text
			}
		}
		return nil, fmt.Errorf("d3dcompile: compilation failed (HRESULT 0x%08X): %s", uint32(ret), errMsg)
	}

	if codeBlob == nil {
		return nil, fmt.Errorf("d3dcompile: compilation succeeded but code blob is nil")
	}
	defer codeBlob.release()

	bytecode := codeBlob.bytes()
	if len(bytecode) == 0 {
		return nil, fmt.Errorf("d3dcompile: empty bytecode output")
	}

	return bytecode, nil
}
