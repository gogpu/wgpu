//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgputest

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/wgputest/internal/device"
)

// observerFor returns the bridge callbacks of r, or the zero value when the
// device is not recorded.
func observerFor(r *Recorder) device.Observer {
	if r == nil {
		return device.Observer{}
	}
	return r.observer()
}

// observer returns the internal device package's callback set. Each callback is
// this Recorder's unexported method value, not a closure: a closure here would
// put the recording logic in a public package, which the public-layer rule
// forbids (ADR-070 §15.10). The methods below are that logic, one append per
// record kind.
func (r *Recorder) observer() device.Observer {
	return device.Observer{
		EncoderCreated:     r.encoderCreated,
		BufferCreated:      r.bufferCreated,
		TextureCreated:     r.textureCreated,
		TextureViewCreated: r.textureViewCreated,
		PassBegan:          r.passBegan,
		BindGroupBound:     r.bindGroupBound,
		VertexBufferBound:  r.vertexBufferBound,
		IndexBufferBound:   r.indexBufferBound,
		Drew:               r.drew,
		DrewIndexed:        r.drewIndexed,
		Submitted:          r.submitted,
	}
}

func (r *Recorder) encoderCreated(seq uint64, encoder int, label string) {
	r.mu.Lock()
	r.trace.Encoders = append(r.trace.Encoders, EncoderCreate{Seq: seq, Index: encoder, Label: label})
	r.mu.Unlock()
}

func (r *Recorder) bufferCreated(seq uint64, label string, size uint64, usage gputypes.BufferUsage) {
	r.mu.Lock()
	r.trace.Buffers = append(r.trace.Buffers, BufferCreate{Seq: seq, Label: label, Size: size, Usage: usage})
	r.mu.Unlock()
}

func (r *Recorder) textureCreated(seq uint64, label string) {
	r.mu.Lock()
	r.trace.Textures = append(r.trace.Textures, TextureCreate{Seq: seq, Label: label})
	r.mu.Unlock()
}

func (r *Recorder) textureViewCreated(seq uint64, label string) {
	r.mu.Lock()
	r.trace.TextureViews = append(r.trace.TextureViews, TextureViewCreate{Seq: seq, Label: label})
	r.mu.Unlock()
}

func (r *Recorder) passBegan(seq uint64, pass, encoder int, compute bool, label string, loadOps []gputypes.LoadOp, clears []gputypes.Color) {
	r.mu.Lock()
	r.trace.Passes = append(r.trace.Passes, PassBegin{
		Seq: seq, Index: pass, Encoder: encoder, Compute: compute, Label: label,
		LoadOps: append([]gputypes.LoadOp(nil), loadOps...),
		Clears:  append([]gputypes.Color(nil), clears...),
	})
	r.mu.Unlock()
}

func (r *Recorder) bindGroupBound(seq uint64, pass int, index uint32, offsets []uint32) {
	r.mu.Lock()
	r.trace.BindGroups = append(r.trace.BindGroups, BindGroupBind{
		Seq: seq, Pass: pass, Index: index,
		DynamicOffsets: append([]uint32(nil), offsets...),
	})
	r.mu.Unlock()
}

func (r *Recorder) vertexBufferBound(seq uint64, pass int, slot uint32, label string, offset, size uint64) {
	r.mu.Lock()
	r.trace.VertexBuffers = append(r.trace.VertexBuffers, VertexBufferBind{
		Seq: seq, Pass: pass, Slot: slot, Label: label, Offset: offset, Size: size,
	})
	r.mu.Unlock()
}

func (r *Recorder) indexBufferBound(seq uint64, pass int, label string, format gputypes.IndexFormat, offset, size uint64) {
	r.mu.Lock()
	r.trace.IndexBuffers = append(r.trace.IndexBuffers, IndexBufferBind{
		Seq: seq, Pass: pass, Label: label, Format: format, Offset: offset, Size: size,
	})
	r.mu.Unlock()
}

func (r *Recorder) drew(seq uint64, pass int, args gputypes.DrawArgs) {
	r.mu.Lock()
	r.trace.Draws = append(r.trace.Draws, Draw{
		Seq: seq, Pass: pass,
		VertexCount: args.VertexCount, InstanceCount: args.InstanceCount,
		FirstVertex: args.FirstVertex, FirstInstance: args.FirstInstance,
	})
	r.mu.Unlock()
}

func (r *Recorder) drewIndexed(seq uint64, pass int, args gputypes.DrawIndexedArgs) {
	r.mu.Lock()
	r.trace.DrawsIndexed = append(r.trace.DrawsIndexed, DrawIndexed{
		Seq: seq, Pass: pass,
		IndexCount: args.IndexCount, InstanceCount: args.InstanceCount,
		FirstIndex: args.FirstIndex, BaseVertex: args.BaseVertex, FirstInstance: args.FirstInstance,
	})
	r.mu.Unlock()
}

func (r *Recorder) submitted(seq uint64, submissionIndex uint64, commandBuffers int) {
	r.mu.Lock()
	r.trace.Submits = append(r.trace.Submits, Submit{Seq: seq, SubmissionIndex: submissionIndex, CommandBuffers: commandBuffers})
	r.mu.Unlock()
}
