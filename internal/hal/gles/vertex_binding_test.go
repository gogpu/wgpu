// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build (windows || linux) && !(js && wasm)

package gles

import (
	"testing"

	"github.com/gogpu/gputypes"
)

// vertexSetups returns the SetVertexBufferCommands recorded so far, by slot.
func vertexSetups(e *RenderPassEncoder) map[uint32]*SetVertexBufferCommand {
	out := map[uint32]*SetVertexBufferCommand{}
	for _, c := range e.encoder.commands {
		if s, ok := c.(*SetVertexBufferCommand); ok {
			out[s.slot] = s
		}
	}
	return out
}

// TestVertexBufferBindingOrder checks that a vertex buffer bound before the
// pipeline, which WebGPU allows, still gets the pipeline's layout: the
// attributes are set up at the draw, not at SetVertexBuffer.
func TestVertexBufferBindingOrder(t *testing.T) {
	layouts := []gputypes.VertexBufferLayout{
		{ArrayStride: 8, Attributes: []gputypes.VertexAttribute{{Format: gputypes.VertexFormatFloat32x2, ShaderLocation: 0}}},
		{ArrayStride: 16, Attributes: []gputypes.VertexAttribute{{Format: gputypes.VertexFormatFloat32x4, ShaderLocation: 1}}},
	}
	for _, buffersFirst := range []bool{false, true} {
		e := &RenderPassEncoder{encoder: &CommandEncoder{}}
		p := &RenderPipeline{vertexBuffers: layouts}
		buf0, buf1 := &Buffer{id: 1}, &Buffer{id: 2}
		if !buffersFirst {
			e.SetPipeline(p)
		}
		e.SetVertexBuffer(0, buf0, 4)
		e.SetVertexBuffer(1, buf1, 0)
		if buffersFirst {
			e.SetPipeline(p)
		}
		if n := len(vertexSetups(e)); n != 0 {
			t.Errorf("buffersFirst=%v: %d slots set up before the draw, want 0", buffersFirst, n)
		}
		e.Draw(gputypes.DrawArgs{VertexCount: 3, InstanceCount: 1})
		got := vertexSetups(e)
		for slot, want := range []*Buffer{buf0, buf1} {
			s := got[uint32(slot)]
			if s == nil || s.buffer != want || s.layout != &p.vertexBuffers[slot] {
				t.Errorf("buffersFirst=%v: slot %d set up as %+v, want buffer %d with the pipeline's layout", buffersFirst, slot, s, want.id)
			}
		}
		if s := got[0]; s != nil && s.offset != 4 {
			t.Errorf("buffersFirst=%v: slot 0 offset %d, want 4", buffersFirst, s.offset)
		}
		if e.dirtyVertexBuffers != 0 {
			t.Errorf("buffersFirst=%v: dirty mask %#b after the draw, want 0", buffersFirst, e.dirtyVertexBuffers)
		}
	}
}

// TestVertexAttribsDisabledOnPipelineChange checks that switching pipelines
// disables the attributes the old one enabled and sets the bound buffers up
// again with the new layout.
func TestVertexAttribsDisabledOnPipelineChange(t *testing.T) {
	e := &RenderPassEncoder{encoder: &CommandEncoder{}}
	first := &RenderPipeline{vertexBuffers: []gputypes.VertexBufferLayout{
		{ArrayStride: 16, Attributes: []gputypes.VertexAttribute{{Format: gputypes.VertexFormatFloat32x4, ShaderLocation: 3}}},
	}}
	second := &RenderPipeline{vertexBuffers: []gputypes.VertexBufferLayout{
		{ArrayStride: 8, Attributes: []gputypes.VertexAttribute{{Format: gputypes.VertexFormatFloat32x2, ShaderLocation: 0}}},
	}}
	e.SetPipeline(first)
	e.SetVertexBuffer(0, &Buffer{id: 1}, 0)
	e.Draw(gputypes.DrawArgs{VertexCount: 3, InstanceCount: 1})
	mark := len(e.encoder.commands)
	e.SetPipeline(second)
	e.Draw(gputypes.DrawArgs{VertexCount: 3, InstanceCount: 1})

	var disabled []uint32
	var setup *SetVertexBufferCommand
	for _, c := range e.encoder.commands[mark:] {
		switch c := c.(type) {
		case *DisableVertexAttribsCommand:
			disabled = append(disabled, c.locations...)
		case *SetVertexBufferCommand:
			setup = c
		}
	}
	if len(disabled) != 1 || disabled[0] != 3 {
		t.Errorf("disabled locations %v, want [3]", disabled)
	}
	if setup == nil || setup.layout != &second.vertexBuffers[0] {
		t.Errorf("slot 0 not set up again with the second pipeline's layout: %+v", setup)
	}
}
