// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

package gles

import (
	"testing"

	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/gputypes"
)

func TestCommandEncoder_BeginEndEncoding(t *testing.T) {
	enc := &CommandEncoder{}

	// Begin encoding
	if err := enc.BeginEncoding("test"); err != nil {
		t.Fatalf("BeginEncoding failed: %v", err)
	}

	if enc.label != "test" {
		t.Errorf("label = %q, want %q", enc.label, "test")
	}

	// End encoding
	cmdBuf, err := enc.EndEncoding()
	if err != nil {
		t.Fatalf("EndEncoding failed: %v", err)
	}

	if cmdBuf == nil {
		t.Error("EndEncoding returned nil CommandBuffer")
	}

	// Commands should be cleared after EndEncoding
	if enc.commands != nil {
		t.Error("commands should be nil after EndEncoding")
	}
}

func TestCommandEncoder_DiscardEncoding(t *testing.T) {
	enc := &CommandEncoder{}
	_ = enc.BeginEncoding("test")

	// Add some commands manually
	enc.commands = append(enc.commands, &ClearColorCommand{r: 1, g: 0, b: 0, a: 1})

	enc.DiscardEncoding()

	if enc.commands != nil {
		t.Error("commands should be nil after DiscardEncoding")
	}
}

func TestCommandBuffer_Destroy(t *testing.T) {
	cmdBuf := &CommandBuffer{
		commands: []Command{
			&ClearColorCommand{r: 1, g: 0, b: 0, a: 1},
			&ClearDepthCommand{depth: 1.0},
		},
	}

	cmdBuf.Destroy()

	if cmdBuf.commands != nil {
		t.Error("commands should be nil after Destroy")
	}
}

func TestCommandEncoder_ClearBuffer(t *testing.T) {
	enc := &CommandEncoder{}
	_ = enc.BeginEncoding("test")

	buf := &Buffer{id: 1, size: 1024}
	enc.ClearBuffer(buf, 0, 512)

	if len(enc.commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(enc.commands))
	}

	cmd, ok := enc.commands[0].(*ClearBufferCommand)
	if !ok {
		t.Fatalf("expected ClearBufferCommand, got %T", enc.commands[0])
	}

	if cmd.buffer != buf {
		t.Error("buffer mismatch")
	}
	if cmd.offset != 0 {
		t.Errorf("offset = %d, want 0", cmd.offset)
	}
	if cmd.size != 512 {
		t.Errorf("size = %d, want 512", cmd.size)
	}
}

func TestCommandEncoder_CopyBufferToBuffer(t *testing.T) {
	enc := &CommandEncoder{}
	_ = enc.BeginEncoding("test")

	src := &Buffer{id: 1}
	dst := &Buffer{id: 2}

	regions := []hal.BufferCopy{
		{SrcOffset: 0, DstOffset: 100, Size: 256},
		{SrcOffset: 512, DstOffset: 0, Size: 128},
	}

	enc.CopyBufferToBuffer(src, dst, regions)

	if len(enc.commands) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(enc.commands))
	}

	for i, r := range regions {
		cmd, ok := enc.commands[i].(*CopyBufferCommand)
		if !ok {
			t.Fatalf("command %d: expected CopyBufferCommand, got %T", i, enc.commands[i])
		}

		if cmd.srcID != 1 {
			t.Errorf("command %d: srcID = %d, want 1", i, cmd.srcID)
		}
		if cmd.dstID != 2 {
			t.Errorf("command %d: dstID = %d, want 2", i, cmd.dstID)
		}
		if cmd.srcOffset != r.SrcOffset {
			t.Errorf("command %d: srcOffset = %d, want %d", i, cmd.srcOffset, r.SrcOffset)
		}
		if cmd.dstOffset != r.DstOffset {
			t.Errorf("command %d: dstOffset = %d, want %d", i, cmd.dstOffset, r.DstOffset)
		}
		if cmd.size != r.Size {
			t.Errorf("command %d: size = %d, want %d", i, cmd.size, r.Size)
		}
	}
}

func TestRenderPassEncoder_Draw(t *testing.T) {
	enc := &CommandEncoder{}
	_ = enc.BeginEncoding("test")

	desc := &hal.RenderPassDescriptor{
		ColorAttachments: []hal.RenderPassColorAttachment{},
	}
	rpe := enc.BeginRenderPass(desc)

	rpe.Draw(3, 1, 0, 0)

	if len(enc.commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(enc.commands))
	}

	cmd, ok := enc.commands[0].(*DrawCommand)
	if !ok {
		t.Fatalf("expected DrawCommand, got %T", enc.commands[0])
	}

	if cmd.vertexCount != 3 {
		t.Errorf("vertexCount = %d, want 3", cmd.vertexCount)
	}
	if cmd.instanceCount != 1 {
		t.Errorf("instanceCount = %d, want 1", cmd.instanceCount)
	}
}

func TestRenderPassEncoder_DrawIndexed(t *testing.T) {
	enc := &CommandEncoder{}
	_ = enc.BeginEncoding("test")

	desc := &hal.RenderPassDescriptor{
		ColorAttachments: []hal.RenderPassColorAttachment{},
	}
	rpe := enc.BeginRenderPass(desc)

	// Set index format
	idxBuf := &Buffer{id: 5}
	rpe.SetIndexBuffer(idxBuf, gputypes.IndexFormatUint32, 0)

	rpe.DrawIndexed(36, 2, 0, 0, 0)

	// Should have SetIndexBufferCommand and DrawIndexedCommand
	if len(enc.commands) < 2 {
		t.Fatalf("expected at least 2 commands, got %d", len(enc.commands))
	}

	drawCmd, ok := enc.commands[len(enc.commands)-1].(*DrawIndexedCommand)
	if !ok {
		t.Fatalf("expected DrawIndexedCommand, got %T", enc.commands[len(enc.commands)-1])
	}

	if drawCmd.indexCount != 36 {
		t.Errorf("indexCount = %d, want 36", drawCmd.indexCount)
	}
	if drawCmd.instanceCount != 2 {
		t.Errorf("instanceCount = %d, want 2", drawCmd.instanceCount)
	}
	if drawCmd.indexFormat != gputypes.IndexFormatUint32 {
		t.Errorf("indexFormat = %v, want Uint32", drawCmd.indexFormat)
	}
}

func TestRenderPassEncoder_SetViewport(t *testing.T) {
	enc := &CommandEncoder{}
	_ = enc.BeginEncoding("test")

	desc := &hal.RenderPassDescriptor{
		ColorAttachments: []hal.RenderPassColorAttachment{},
	}
	rpe := enc.BeginRenderPass(desc)

	rpe.SetViewport(10, 20, 800, 600, 0.0, 1.0)

	if len(enc.commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(enc.commands))
	}

	cmd, ok := enc.commands[0].(*SetViewportCommand)
	if !ok {
		t.Fatalf("expected SetViewportCommand, got %T", enc.commands[0])
	}

	if cmd.x != 10 || cmd.y != 20 {
		t.Errorf("position = (%v, %v), want (10, 20)", cmd.x, cmd.y)
	}
	if cmd.width != 800 || cmd.height != 600 {
		t.Errorf("size = (%v, %v), want (800, 600)", cmd.width, cmd.height)
	}
}

func TestRenderPassEncoder_SetScissorRect(t *testing.T) {
	enc := &CommandEncoder{}
	_ = enc.BeginEncoding("test")

	desc := &hal.RenderPassDescriptor{
		ColorAttachments: []hal.RenderPassColorAttachment{},
	}
	rpe := enc.BeginRenderPass(desc)

	rpe.SetScissorRect(50, 50, 400, 300)

	if len(enc.commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(enc.commands))
	}

	cmd, ok := enc.commands[0].(*SetScissorCommand)
	if !ok {
		t.Fatalf("expected SetScissorCommand, got %T", enc.commands[0])
	}

	if cmd.x != 50 || cmd.y != 50 {
		t.Errorf("position = (%d, %d), want (50, 50)", cmd.x, cmd.y)
	}
	if cmd.width != 400 || cmd.height != 300 {
		t.Errorf("size = (%d, %d), want (400, 300)", cmd.width, cmd.height)
	}
}

func TestRenderPassEncoder_SetVertexBuffer(t *testing.T) {
	enc := &CommandEncoder{}
	_ = enc.BeginEncoding("test")

	desc := &hal.RenderPassDescriptor{
		ColorAttachments: []hal.RenderPassColorAttachment{},
	}
	rpe := enc.BeginRenderPass(desc).(*RenderPassEncoder)

	buf := &Buffer{id: 10}
	rpe.SetVertexBuffer(0, buf, 64)

	if len(rpe.vertexBuffers) != 1 {
		t.Errorf("vertexBuffers length = %d, want 1", len(rpe.vertexBuffers))
	}
	if rpe.vertexBuffers[0] != buf {
		t.Error("vertexBuffers[0] mismatch")
	}

	if len(enc.commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(enc.commands))
	}

	cmd, ok := enc.commands[0].(*SetVertexBufferCommand)
	if !ok {
		t.Fatalf("expected SetVertexBufferCommand, got %T", enc.commands[0])
	}

	if cmd.slot != 0 {
		t.Errorf("slot = %d, want 0", cmd.slot)
	}
	if cmd.offset != 64 {
		t.Errorf("offset = %d, want 64", cmd.offset)
	}
}

func TestRenderPassEncoder_ClearColorOnLoad(t *testing.T) {
	enc := &CommandEncoder{}
	_ = enc.BeginEncoding("test")

	desc := &hal.RenderPassDescriptor{
		ColorAttachments: []hal.RenderPassColorAttachment{
			{
				LoadOp:     gputypes.LoadOpClear,
				ClearValue: gputypes.Color{R: 1.0, G: 0.5, B: 0.25, A: 1.0},
			},
		},
	}
	_ = enc.BeginRenderPass(desc)

	if len(enc.commands) != 1 {
		t.Fatalf("expected 1 command for clear, got %d", len(enc.commands))
	}

	cmd, ok := enc.commands[0].(*ClearColorCommand)
	if !ok {
		t.Fatalf("expected ClearColorCommand, got %T", enc.commands[0])
	}

	if cmd.r != 1.0 || cmd.g != 0.5 || cmd.b != 0.25 || cmd.a != 1.0 {
		t.Errorf("clear color = (%v, %v, %v, %v), want (1.0, 0.5, 0.25, 1.0)",
			cmd.r, cmd.g, cmd.b, cmd.a)
	}
}

func TestComputePassEncoder_Dispatch(t *testing.T) {
	enc := &CommandEncoder{}
	_ = enc.BeginEncoding("test")

	cpe := enc.BeginComputePass(nil)
	cpe.Dispatch(16, 8, 4)

	if len(enc.commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(enc.commands))
	}

	cmd, ok := enc.commands[0].(*DispatchCommand)
	if !ok {
		t.Fatalf("expected DispatchCommand, got %T", enc.commands[0])
	}

	if cmd.x != 16 || cmd.y != 8 || cmd.z != 4 {
		t.Errorf("dispatch = (%d, %d, %d), want (16, 8, 4)", cmd.x, cmd.y, cmd.z)
	}
}

func TestRenderPassEncoder_SetBlendConstant(t *testing.T) {
	enc := &CommandEncoder{}
	_ = enc.BeginEncoding("test")

	desc := &hal.RenderPassDescriptor{
		ColorAttachments: []hal.RenderPassColorAttachment{},
	}
	rpe := enc.BeginRenderPass(desc)

	color := &gputypes.Color{R: 0.2, G: 0.4, B: 0.6, A: 0.8}
	rpe.SetBlendConstant(color)

	if len(enc.commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(enc.commands))
	}

	cmd, ok := enc.commands[0].(*SetBlendConstantCommand)
	if !ok {
		t.Fatalf("expected SetBlendConstantCommand, got %T", enc.commands[0])
	}

	const epsilon = 0.0001
	if cmd.r < 0.2-epsilon || cmd.r > 0.2+epsilon {
		t.Errorf("r = %v, want ~0.2", cmd.r)
	}
}

func TestRenderPassEncoder_SetStencilReference(t *testing.T) {
	enc := &CommandEncoder{}
	_ = enc.BeginEncoding("test")

	desc := &hal.RenderPassDescriptor{
		ColorAttachments: []hal.RenderPassColorAttachment{},
	}
	rpe := enc.BeginRenderPass(desc)

	rpe.SetStencilReference(128)

	if len(enc.commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(enc.commands))
	}

	cmd, ok := enc.commands[0].(*SetStencilRefCommand)
	if !ok {
		t.Fatalf("expected SetStencilRefCommand, got %T", enc.commands[0])
	}

	if cmd.ref != 128 {
		t.Errorf("ref = %d, want 128", cmd.ref)
	}
}
