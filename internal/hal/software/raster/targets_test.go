//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package raster

import "testing"

// TestColorBufferAllocatedOnlyWhenRead proves a pipeline allocates its color
// buffer only on the first use that reads or writes it. A draw routed to a
// ColorOutput (every software-backend draw: routeColorOutput always sets one)
// must not pay for a full-size buffer nothing reads, and the standalone path
// must still see a new pipeline's black buffer.
func TestColorBufferAllocatedOnlyWhenRead(t *testing.T) {
	const w, h = 64, 48
	p := NewPipeline(w, h)
	if p.colorBuffer != nil {
		t.Fatalf("NewPipeline allocated a %d-byte color buffer nothing had read", len(p.colorBuffer))
	}

	routed := 0
	p.SetColorOutput(func(x, y int, coverage uint8, c [4]float32) { routed++ })
	p.DrawTriangles([]Triangle{CreateScreenTriangle(0, 0, 0, w-1, 0, 0, 0, h-1, 0)}, [4]float32{1, 0, 0, 1})
	if routed == 0 {
		t.Fatal("no fragment reached the color output")
	}
	if p.colorBuffer != nil {
		t.Error("a draw routed to a color output allocated the color buffer")
	}
	if r, g, b, a := p.GetPixel(10, 10); r|g|b|a != 0 {
		t.Errorf("unallocated color buffer pixel = %d,%d,%d,%d, want 0,0,0,0", r, g, b, a)
	}

	buf := p.GetColorBuffer()
	if len(buf) != w*h*4 {
		t.Fatalf("GetColorBuffer length = %d, want %d", len(buf), w*h*4)
	}
	for i, v := range buf {
		if v != 0 {
			t.Fatalf("GetColorBuffer byte %d = %d, want 0 (a new pipeline's black)", i, v)
		}
	}
}

// TestDepthBufferAllocatedOnlyWhenNeeded proves a pipeline allocates its own
// depth buffer only when depth testing or writing needs one, and that the
// buffer it allocates starts at the far plane.
func TestDepthBufferAllocatedOnlyWhenNeeded(t *testing.T) {
	const w, h = 64, 48
	p := NewPipeline(w, h)
	if p.depthBuffer != nil {
		t.Fatal("NewPipeline allocated a depth buffer nothing tested against")
	}

	p.SetDepthTest(false, CompareLess)
	p.SetDepthWrite(false)
	if p.depthBuffer != nil {
		t.Fatal("disabling depth test and write allocated a depth buffer")
	}

	p.SetDepthWrite(true)
	if p.depthBuffer == nil {
		t.Fatal("depth write enabled but no depth buffer was allocated")
	}
	if got := p.depthBuffer.Get(w-1, h-1); got != 1.0 {
		t.Errorf("fresh depth buffer value = %v, want 1.0 (far)", got)
	}
}

// TestResetKeepsCallersDepthBuffer proves Reset drops a depth buffer given with
// SetDepthBuffer instead of clearing it: that buffer is the render pass's depth
// attachment, which every draw of the pass tests against, so its contents must
// survive a reset.
func TestResetKeepsCallersDepthBuffer(t *testing.T) {
	const w, h = 32, 32
	shared := NewDepthBuffer(w, h)
	shared.Set(5, 7, 0.25)

	p := NewPipeline(w, h)
	p.SetDepthBuffer(shared)
	p.SetDepthTest(true, CompareLess)
	if p.depthBuffer != shared {
		t.Fatal("SetDepthBuffer did not take the caller's buffer")
	}

	p.Reset(w, h)

	if p.depthBuffer != nil {
		t.Error("Reset kept a depth buffer it did not allocate")
	}
	if got := shared.Get(5, 7); got != 0.25 {
		t.Errorf("reset cleared the caller's depth buffer: value = %v, want 0.25", got)
	}
}

// TestResetGivesNewPipelineState proves a reset pipeline behaves as a newly
// created one: default render state, and buffers with a new pipeline's
// contents. A pass that reuses one pipeline for every draw must draw each draw
// exactly as its own pipeline would.
func TestResetGivesNewPipelineState(t *testing.T) {
	const w, h = 16, 16
	p := NewPipeline(w, h)
	p.SetDepthTest(true, CompareLess)
	p.SetDepthWrite(true)
	p.SetScissor(&Rect{X: 0, Y: 0, Width: w, Height: h})
	p.Clear(0, 0, 0, 1)
	p.ClearDepth(1.0)
	p.DrawTriangles([]Triangle{CreateScreenTriangle(0, 0, 0.25, w-1, 0, 0.25, 0, h-1, 0.25)}, [4]float32{1, 0, 0, 1})
	if got := p.GetDepthBuffer().Get(1, 1); got != 0.25 {
		t.Fatalf("depth at a covered pixel = %v after the draw, want 0.25", got)
	}

	p.Reset(w, h)

	if p.depthTest || p.depthWrite {
		t.Error("reset pipeline still has depth test or write enabled")
	}
	if p.colorOutput != nil || p.scissorRect != nil {
		t.Error("reset pipeline kept draw state")
	}
	if p.Width() != w || p.Height() != h {
		t.Errorf("reset pipeline is %dx%d, want %dx%d", p.Width(), p.Height(), w, h)
	}
	if got := p.GetDepthBuffer().Get(1, 1); got != 1.0 {
		t.Errorf("depth after reset = %v, want 1.0 (far)", got)
	}
	if rgba := p.GetColorBuffer()[:4]; rgba[0]|rgba[1]|rgba[2]|rgba[3] != 0 {
		t.Errorf("color after reset = %v, want transparent black", rgba)
	}

	p.Reset(2*w, 2*h)
	if p.colorBuffer != nil || p.depthBuffer != nil {
		t.Error("Reset to another extent kept the buffers of the old one")
	}
	if p.Width() != 2*w || p.Height() != 2*h {
		t.Errorf("resized pipeline is %dx%d, want %dx%d", p.Width(), p.Height(), 2*w, 2*h)
	}
}
