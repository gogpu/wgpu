// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build integration && linux && !(js && wasm)

package gles

import (
	"runtime"
	"testing"
	"unsafe"

	"github.com/gogpu/wgpu/hal/gles/egl"
	"github.com/gogpu/wgpu/hal/gles/gl"
)

// TestCopyBufferCommand_ExecutesCopy runs CopyBufferCommand on a real EGL
// context and reads the destination back. The command used to bind the
// COPY_READ/COPY_WRITE targets without calling glCopyBufferSubData, so the
// destination stayed zero-filled (compute readback returned all zeros).
func TestCopyBufferCommand_ExecutesCopy(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := egl.Init(); err != nil {
		t.Fatalf("egl.Init() failed: %v", err)
	}
	config := egl.DefaultContextConfig()
	config.GLES = false
	eglCtx, err := egl.NewContext(config)
	if err != nil {
		t.Skipf("egl.NewContext() failed: %v", err)
	}
	defer eglCtx.Destroy()
	if err := eglCtx.MakeCurrent(); err != nil {
		t.Fatalf("MakeCurrent failed: %v", err)
	}

	glCtx := &gl.Context{}
	if err := glCtx.Load(egl.GetGLProcAddress); err != nil {
		t.Fatalf("GL load failed: %v", err)
	}

	const size = 256
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i*3 + 5)
	}

	src := glCtx.GenBuffers(1)
	dst := glCtx.GenBuffers(1)
	defer glCtx.DeleteBuffers(src, dst)

	glCtx.BindBuffer(gl.COPY_READ_BUFFER, src)
	glCtx.BufferData(gl.COPY_READ_BUFFER, size, uintptr(unsafe.Pointer(&data[0])), gl.DYNAMIC_DRAW)
	glCtx.BindBuffer(gl.COPY_READ_BUFFER, dst)
	glCtx.BufferData(gl.COPY_READ_BUFFER, size, 0, gl.DYNAMIC_READ)
	glCtx.BindBuffer(gl.COPY_READ_BUFFER, 0)
	runtime.KeepAlive(data)

	cmds := []Command{
		&CopyBufferCommand{srcID: src, dstID: dst, srcOffset: 0, dstOffset: 128, size: 128},
		&CopyBufferCommand{srcID: src, dstID: dst, srcOffset: 128, dstOffset: 0, size: 128},
	}
	for _, c := range cmds {
		c.Execute(glCtx)
	}
	if e := glCtx.GetError(); e != 0 {
		t.Fatalf("GL error 0x%x after copy", e)
	}

	glCtx.BindBuffer(gl.COPY_READ_BUFFER, dst)
	ptr := glCtx.MapBuffer(gl.COPY_READ_BUFFER, gl.READ_ONLY)
	if ptr == 0 {
		t.Skip("glMapBuffer unavailable in this context")
	}
	got := make([]byte, size)
	copy(got, unsafe.Slice(*(**byte)(unsafe.Pointer(&ptr)), size))
	glCtx.UnmapBuffer(gl.COPY_READ_BUFFER)
	glCtx.BindBuffer(gl.COPY_READ_BUFFER, 0)

	for i := 0; i < 128; i++ {
		if got[128+i] != data[i] || got[i] != data[128+i] {
			t.Fatalf("byte %d: got lo=%d hi=%d, want lo=%d hi=%d",
				i, got[i], got[128+i], data[128+i], data[i])
		}
	}
}
