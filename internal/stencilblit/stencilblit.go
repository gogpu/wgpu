// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

// Package stencilblit writes the stencil aspect of a depth/stencil texture
// with render passes, for backends that cannot copy a buffer into it.
//
// OpenGL ES has no way to upload only the stencil of a combined
// depth/stencil format (DEPTH24_STENCIL8, DEPTH32F_STENCIL8): the pixel
// transfer of glTexSubImage2D writes both aspects or, with STENCIL_INDEX,
// needs GL 4.4 / ES 3.2 and a stencil-only internal format. Dawn lowers such a
// copy in the frontend (src/dawn/native/BlitBufferToDepthStencil.cpp,
// BlitR8ToStencil), and so do we:
//
//  1. the stencil bytes are copied into an R8Uint texture of the copy extent;
//  2. for each destination array layer, one render pass on a view of that mip
//     and layer loads and stores depth and stencil, draws a full-screen
//     triangle with the clear pipeline (reference 0, Replace, write mask 0xFF:
//     every stencil bit of the copied texels becomes 0), then once per bit
//     with a pipeline whose write mask is that bit (reference 0xFF, Replace),
//     whose fragment shader discards the texels where the bit is 0.
//
// Depth is neither tested nor written (depthCompare Always, depthWriteEnabled
// false), so the depth aspect keeps its contents.
//
// This package owns the shaders, the pipeline state, the per-device cache of
// pipelines and the order of the commands. The caller (the root package)
// creates the resources through its public API and records through a Pass,
// so the rule lives in one place and this package imports nothing of the
// caller (ADR-069).
package stencilblit

import (
	"encoding/binary"
	"errors"
	"sync"

	"github.com/gogpu/gputypes"
)

// StencilBits is the number of bits of every WebGPU stencil format.
const StencilBits = 8

// ParamsSize is the size in bytes of the uniform Params of one layer.
const ParamsSize = 16

// VertexCount is the vertex count of the full-screen triangle.
const VertexCount = 3

// ErrDisposed is returned by Cache.Get after Cache.Dispose.
var ErrDisposed = errors.New("stencilblit: cache disposed")

// Key selects the pipelines of one blit: the destination format (the
// pipelines' depth/stencil state format) and the dimension of the source
// view, which the shader's texture type follows.
//
// Dawn keys its cache the same way (BlitBufferToDepthStencil.cpp,
// GetOrCreateR8ToStencilPipelines keyed by format and view dimension).
type Key struct {
	Format        gputypes.TextureFormat
	ViewDimension gputypes.TextureViewDimension
}

// KeyFor returns the key of a copy of layers array layers into format.
// The source view is 2d for one layer and 2d-array otherwise, as Dawn picks
// it for compatibility mode (one view dimension per texture binding).
func KeyFor(format gputypes.TextureFormat, layers uint32) Key {
	return Key{Format: format, ViewDimension: SourceViewDimension(layers)}
}

// SourceViewDimension returns the dimension of the R8Uint source view of a
// copy of layers array layers.
func SourceViewDimension(layers uint32) gputypes.TextureViewDimension {
	if layers > 1 {
		return gputypes.TextureViewDimension2DArray
	}
	return gputypes.TextureViewDimension2D
}

// Params are the uniform values of the pass of one layer.
type Params struct {
	// OriginX and OriginY are the copy's origin in the destination mip
	// level; the source texel of a fragment is its position minus origin.
	OriginX, OriginY uint32
	// Layer is the source array layer of the pass (2d-array view only).
	Layer uint32
}

// Bytes returns p as the WGSL struct Params (origin: vec2u, layer: u32),
// padded to ParamsSize.
func (p Params) Bytes() [ParamsSize]byte {
	var b [ParamsSize]byte
	binary.LittleEndian.PutUint32(b[0:], p.OriginX)
	binary.LittleEndian.PutUint32(b[4:], p.OriginY)
	binary.LittleEndian.PutUint32(b[8:], p.Layer)
	return b
}

// StencilState is the stencil state of one blit pipeline, the same for the
// front and back faces.
type StencilState struct {
	Compare   gputypes.CompareFunction
	FailOp    gputypes.StencilOperation
	DepthFail gputypes.StencilOperation
	PassOp    gputypes.StencilOperation
	ReadMask  uint32
	WriteMask uint32
}

// ClearState is the stencil state of the clear pipeline: every bit of each
// covered texel takes the reference 0.
func ClearState() StencilState {
	return StencilState{
		Compare:   gputypes.CompareFunctionAlways,
		FailOp:    gputypes.StencilOperationKeep,
		DepthFail: gputypes.StencilOperationKeep,
		PassOp:    gputypes.StencilOperationReplace,
		ReadMask:  0xFF,
		WriteMask: 0xFF,
	}
}

// BitState is the stencil state of the pipeline of bit: only that bit is
// written, from the reference BitReference, where the fragment is kept.
func BitState(bit int) StencilState {
	s := ClearState()
	s.WriteMask = 1 << uint(bit)
	return s
}

// ClearReference and BitReference are the stencil references of the clear
// pipeline and of the bit pipelines.
const (
	ClearReference uint32 = 0
	BitReference   uint32 = 0xFF
)

// Pipelines are the render pipelines of one Key.
type Pipelines[P any] struct {
	Clear P
	Bits  [StencilBits]P
}

// Pass records the commands of one layer into a render pass the caller
// opened on the destination view, with the layer's bind group set.
type Pass[P any] interface {
	SetPipeline(p P)
	SetStencilReference(ref uint32)
	SetScissorRect(x, y, width, height uint32)
	Draw(vertexCount, instanceCount, firstVertex, firstInstance uint32)
}

// RecordLayer records the blit of one layer: the clear draw, then one draw
// per stencil bit. x, y, width and height are the copied rectangle of the
// destination mip level; fragments outside it keep their stencil.
//
// Dawn passes the bit to one shader through firstInstance; this package bakes
// it into one fragment entry point per bit instead, because a base instance
// is not available on every OpenGL ES (it needs GL 4.2 or
// EXT_base_instance), the backend this blit exists for.
func RecordLayer[P any](pass Pass[P], pipelines *Pipelines[P], x, y, width, height uint32) {
	pass.SetScissorRect(x, y, width, height)
	pass.SetStencilReference(ClearReference)
	pass.SetPipeline(pipelines.Clear)
	pass.Draw(VertexCount, 1, 0, 0)
	pass.SetStencilReference(BitReference)
	for bit := range StencilBits {
		pass.SetPipeline(pipelines.Bits[bit])
		pass.Draw(VertexCount, 1, 0, 0)
	}
}

// Cache holds the pipelines of each Key of one device, built on first use.
// E is the caller's entry: the pipelines and whatever they need (shader
// module, layouts). It is safe for concurrent use.
type Cache[E any] struct {
	build   func(Key) (E, error)
	release func(E)

	mu       sync.Mutex
	entries  map[Key]E
	disposed bool
}

// NewCache returns an empty cache whose entries build creates and release
// destroys.
func NewCache[E any](build func(Key) (E, error), release func(E)) *Cache[E] {
	return &Cache[E]{build: build, release: release, entries: make(map[Key]E)}
}

// Get returns the entry of key, building it on the first call for key. A
// failed build is not cached: the next Get tries again.
//
// The lock is held while building, so two callers never build the same key
// twice; build must not call back into the cache.
func (c *Cache[E]) Get(key Key) (E, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var zero E
	if c.disposed {
		return zero, ErrDisposed
	}
	if e, ok := c.entries[key]; ok {
		return e, nil
	}
	e, err := c.build(key)
	if err != nil {
		return zero, err
	}
	c.entries[key] = e
	return e, nil
}

// Len returns the number of built entries.
func (c *Cache[E]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

// Dispose releases every entry. Later calls of Get return ErrDisposed, and
// later calls of Dispose do nothing.
func (c *Cache[E]) Dispose() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.disposed {
		return
	}
	c.disposed = true
	for key, e := range c.entries {
		c.release(e)
		delete(c.entries, key)
	}
}
