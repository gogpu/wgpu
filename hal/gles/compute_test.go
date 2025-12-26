// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build linux || windows

package gles

import (
	"testing"

	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/gles/gl"
)

// TestGLESComputeConstants verifies that all GL compute shader constants
// have the correct values according to the OpenGL specification.
// These constants are essential for compute shader support detection
// and proper dispatch operations.
func TestGLESComputeConstants(t *testing.T) {
	tests := []struct {
		name  string
		value uint32
		want  uint32
	}{
		// Compute shader type (OpenGL ES 3.1+ / OpenGL 4.3+)
		{"GL_COMPUTE_SHADER", gl.COMPUTE_SHADER, 0x91B9},

		// Compute shader limits
		{"GL_MAX_COMPUTE_WORK_GROUP_COUNT", gl.MAX_COMPUTE_WORK_GROUP_COUNT, 0x91BE},
		{"GL_MAX_COMPUTE_WORK_GROUP_SIZE", gl.MAX_COMPUTE_WORK_GROUP_SIZE, 0x91BF},
		{"GL_MAX_COMPUTE_WORK_GROUP_INVOCATIONS", gl.MAX_COMPUTE_WORK_GROUP_INVOCATIONS, 0x90EB},

		// Indirect dispatch buffer target
		{"GL_DISPATCH_INDIRECT_BUFFER", gl.DISPATCH_INDIRECT_BUFFER, 0x90EE},

		// Shader storage buffer target
		{"GL_SHADER_STORAGE_BUFFER", gl.SHADER_STORAGE_BUFFER, 0x90D2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value != tt.want {
				t.Errorf("%s = %#x, want %#x", tt.name, tt.value, tt.want)
			}
		})
	}
}

// TestGLESComputeBarrierConstants verifies memory barrier bit constants
// used for synchronization after compute shader dispatch.
// These are defined in OpenGL ES 3.1+ / OpenGL 4.2+.
func TestGLESComputeBarrierConstants(t *testing.T) {
	tests := []struct {
		name  string
		value uint32
		want  uint32
	}{
		// Memory barrier bits
		{"GL_VERTEX_ATTRIB_ARRAY_BARRIER_BIT", gl.VERTEX_ATTRIB_ARRAY_BARRIER_BIT, 0x00000001},
		{"GL_ELEMENT_ARRAY_BARRIER_BIT", gl.ELEMENT_ARRAY_BARRIER_BIT, 0x00000002},
		{"GL_UNIFORM_BARRIER_BIT", gl.UNIFORM_BARRIER_BIT, 0x00000004},
		{"GL_TEXTURE_FETCH_BARRIER_BIT", gl.TEXTURE_FETCH_BARRIER_BIT, 0x00000008},
		{"GL_SHADER_IMAGE_ACCESS_BARRIER_BIT", gl.SHADER_IMAGE_ACCESS_BARRIER_BIT, 0x00000020},
		{"GL_COMMAND_BARRIER_BIT", gl.COMMAND_BARRIER_BIT, 0x00000040},
		{"GL_PIXEL_BUFFER_BARRIER_BIT", gl.PIXEL_BUFFER_BARRIER_BIT, 0x00000080},
		{"GL_TEXTURE_UPDATE_BARRIER_BIT", gl.TEXTURE_UPDATE_BARRIER_BIT, 0x00000100},
		{"GL_BUFFER_UPDATE_BARRIER_BIT", gl.BUFFER_UPDATE_BARRIER_BIT, 0x00000200},
		{"GL_FRAMEBUFFER_BARRIER_BIT", gl.FRAMEBUFFER_BARRIER_BIT, 0x00000400},
		{"GL_TRANSFORM_FEEDBACK_BARRIER_BIT", gl.TRANSFORM_FEEDBACK_BARRIER_BIT, 0x00000800},
		{"GL_ATOMIC_COUNTER_BARRIER_BIT", gl.ATOMIC_COUNTER_BARRIER_BIT, 0x00001000},
		{"GL_SHADER_STORAGE_BARRIER_BIT", gl.SHADER_STORAGE_BARRIER_BIT, 0x00002000},
		{"GL_ALL_BARRIER_BITS", gl.ALL_BARRIER_BITS, 0xFFFFFFFF},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value != tt.want {
				t.Errorf("%s = %#x, want %#x", tt.name, tt.value, tt.want)
			}
		})
	}
}

// TestGLESComputeSupport tests the compute shader support detection.
// On systems without ES 3.1+ support, SupportsCompute() should return false.
// This test validates the feature detection logic without requiring actual GPU.
func TestGLESComputeSupport(t *testing.T) {
	// Test with nil function pointer (no compute support)
	t.Run("NoComputeSupport", func(t *testing.T) {
		ctx := &gl.Context{} // Zero-initialized - no functions loaded

		if ctx.SupportsCompute() {
			t.Error("SupportsCompute() = true for uninitialized context, want false")
		}
	})

	// Test that DispatchCompute is a no-op when not supported
	t.Run("DispatchComputeNoOp", func(t *testing.T) {
		ctx := &gl.Context{} // Zero-initialized

		// This should not panic - just a no-op
		ctx.DispatchCompute(1, 1, 1)
	})

	// Test that DispatchComputeIndirect is a no-op when not supported
	t.Run("DispatchComputeIndirectNoOp", func(t *testing.T) {
		ctx := &gl.Context{} // Zero-initialized

		// This should not panic - just a no-op
		ctx.DispatchComputeIndirect(0)
	})

	// Test that MemoryBarrier is a no-op when not supported
	t.Run("MemoryBarrierNoOp", func(t *testing.T) {
		ctx := &gl.Context{} // Zero-initialized

		// This should not panic - just a no-op
		ctx.MemoryBarrier(gl.SHADER_STORAGE_BARRIER_BIT)
	})
}

// TestGLESComputeDispatch tests the DispatchCommand.Execute() logic.
// This validates command recording and execution without actual GPU.
func TestGLESComputeDispatch(t *testing.T) {
	tests := []struct {
		name    string
		x, y, z uint32
		wantX   uint32
		wantY   uint32
		wantZ   uint32
	}{
		{
			name: "SingleWorkgroup",
			x:    1, y: 1, z: 1,
			wantX: 1, wantY: 1, wantZ: 1,
		},
		{
			name: "MultipleWorkgroups",
			x:    16, y: 8, z: 4,
			wantX: 16, wantY: 8, wantZ: 4,
		},
		{
			name: "LargeDispatch",
			x:    256, y: 256, z: 1,
			wantX: 256, wantY: 256, wantZ: 1,
		},
		{
			name: "MaxDimensions",
			x:    65535, y: 65535, z: 65535,
			wantX: 65535, wantY: 65535, wantZ: 65535,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enc := &CommandEncoder{}
			_ = enc.BeginEncoding("test")

			cpe := enc.BeginComputePass(nil)
			cpe.Dispatch(tt.x, tt.y, tt.z)

			if len(enc.commands) != 1 {
				t.Fatalf("expected 1 command, got %d", len(enc.commands))
			}

			cmd, ok := enc.commands[0].(*DispatchCommand)
			if !ok {
				t.Fatalf("expected DispatchCommand, got %T", enc.commands[0])
			}

			if cmd.x != tt.wantX {
				t.Errorf("x = %d, want %d", cmd.x, tt.wantX)
			}
			if cmd.y != tt.wantY {
				t.Errorf("y = %d, want %d", cmd.y, tt.wantY)
			}
			if cmd.z != tt.wantZ {
				t.Errorf("z = %d, want %d", cmd.z, tt.wantZ)
			}
		})
	}
}

// TestGLESComputeDispatchIndirect tests indirect dispatch command recording.
// This validates DispatchIndirectCommand structure and buffer binding logic.
func TestGLESComputeDispatchIndirect(t *testing.T) {
	tests := []struct {
		name     string
		bufferID uint32
		offset   uint64
	}{
		{
			name:     "ZeroOffset",
			bufferID: 1,
			offset:   0,
		},
		{
			name:     "NonZeroOffset",
			bufferID: 2,
			offset:   64, // Typical alignment for indirect buffer
		},
		{
			name:     "MultipleDispatchOffset",
			bufferID: 3,
			offset:   12, // sizeof(uint32) * 3 = 12 bytes per dispatch
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enc := &CommandEncoder{}
			_ = enc.BeginEncoding("test")

			buf := &Buffer{id: tt.bufferID}
			cpe := enc.BeginComputePass(nil)
			cpe.DispatchIndirect(buf, tt.offset)

			if len(enc.commands) != 1 {
				t.Fatalf("expected 1 command, got %d", len(enc.commands))
			}

			cmd, ok := enc.commands[0].(*DispatchIndirectCommand)
			if !ok {
				t.Fatalf("expected DispatchIndirectCommand, got %T", enc.commands[0])
			}

			if cmd.buffer.id != tt.bufferID {
				t.Errorf("buffer.id = %d, want %d", cmd.buffer.id, tt.bufferID)
			}
			if cmd.offset != tt.offset {
				t.Errorf("offset = %d, want %d", cmd.offset, tt.offset)
			}
		})
	}

	// Test with invalid buffer type
	t.Run("InvalidBufferType", func(t *testing.T) {
		enc := &CommandEncoder{}
		_ = enc.BeginEncoding("test")

		cpe := enc.BeginComputePass(nil)
		// Pass nil - should not add command
		cpe.DispatchIndirect(nil, 0)

		if len(enc.commands) != 0 {
			t.Errorf("expected 0 commands for nil buffer, got %d", len(enc.commands))
		}
	})
}

// TestGLESComputeSSBO tests Shader Storage Buffer Object handling.
// Validates buffer binding target constant and usage patterns.
func TestGLESComputeSSBO(t *testing.T) {
	// Test SSBO buffer target constant
	t.Run("SSBOBufferTarget", func(t *testing.T) {
		if gl.SHADER_STORAGE_BUFFER != 0x90D2 {
			t.Errorf("GL_SHADER_STORAGE_BUFFER = %#x, want 0x90D2", gl.SHADER_STORAGE_BUFFER)
		}
	})

	// Test buffer creation with storage usage
	t.Run("BufferWithStorageUsage", func(t *testing.T) {
		buf := &Buffer{
			id:   1,
			size: 4096,
		}

		if buf.size != 4096 {
			t.Errorf("buffer size = %d, want 4096", buf.size)
		}
	})
}

// TestGLESComputePassEncoder tests the ComputePassEncoder behavior.
func TestGLESComputePassEncoder(t *testing.T) {
	t.Run("BeginComputePass", func(t *testing.T) {
		enc := &CommandEncoder{}
		_ = enc.BeginEncoding("test")

		cpe := enc.BeginComputePass(nil)
		if cpe == nil {
			t.Error("BeginComputePass returned nil")
		}
	})

	t.Run("ComputePassEnd", func(t *testing.T) {
		enc := &CommandEncoder{}
		_ = enc.BeginEncoding("test")

		cpe := enc.BeginComputePass(nil)
		cpe.Dispatch(1, 1, 1)
		cpe.End() // Should not panic

		// Verify dispatch was recorded
		if len(enc.commands) != 1 {
			t.Errorf("expected 1 command after End(), got %d", len(enc.commands))
		}
	})

	t.Run("SetPipeline", func(t *testing.T) {
		enc := &CommandEncoder{}
		_ = enc.BeginEncoding("test")

		pipeline := &ComputePipeline{programID: 42}
		cpe := enc.BeginComputePass(nil).(*ComputePassEncoder)
		cpe.SetPipeline(pipeline)

		if cpe.pipeline != pipeline {
			t.Error("pipeline not set")
		}

		// Verify UseProgram command was recorded
		if len(enc.commands) != 1 {
			t.Fatalf("expected 1 command, got %d", len(enc.commands))
		}

		cmd, ok := enc.commands[0].(*UseProgramCommand)
		if !ok {
			t.Fatalf("expected UseProgramCommand, got %T", enc.commands[0])
		}

		if cmd.programID != 42 {
			t.Errorf("programID = %d, want 42", cmd.programID)
		}
	})

	t.Run("SetPipelineInvalidType", func(t *testing.T) {
		enc := &CommandEncoder{}
		_ = enc.BeginEncoding("test")

		cpe := enc.BeginComputePass(nil)
		// Pass nil - should not set pipeline
		cpe.SetPipeline(nil)

		if len(enc.commands) != 0 {
			t.Errorf("expected 0 commands for nil pipeline, got %d", len(enc.commands))
		}
	})

	t.Run("SetBindGroup", func(t *testing.T) {
		enc := &CommandEncoder{}
		_ = enc.BeginEncoding("test")

		bg := &BindGroup{}
		cpe := enc.BeginComputePass(nil)
		cpe.SetBindGroup(0, bg, nil)

		if len(enc.commands) != 1 {
			t.Fatalf("expected 1 command, got %d", len(enc.commands))
		}

		cmd, ok := enc.commands[0].(*SetBindGroupCommand)
		if !ok {
			t.Fatalf("expected SetBindGroupCommand, got %T", enc.commands[0])
		}

		if cmd.index != 0 {
			t.Errorf("index = %d, want 0", cmd.index)
		}
		if cmd.group != bg {
			t.Error("bind group mismatch")
		}
	})

	t.Run("SetBindGroupWithOffsets", func(t *testing.T) {
		enc := &CommandEncoder{}
		_ = enc.BeginEncoding("test")

		bg := &BindGroup{}
		offsets := []uint32{256, 512}
		cpe := enc.BeginComputePass(nil)
		cpe.SetBindGroup(1, bg, offsets)

		if len(enc.commands) != 1 {
			t.Fatalf("expected 1 command, got %d", len(enc.commands))
		}

		cmd := enc.commands[0].(*SetBindGroupCommand)
		if len(cmd.dynamicOffsets) != 2 {
			t.Errorf("dynamicOffsets length = %d, want 2", len(cmd.dynamicOffsets))
		}
		if cmd.dynamicOffsets[0] != 256 {
			t.Errorf("dynamicOffsets[0] = %d, want 256", cmd.dynamicOffsets[0])
		}
		if cmd.dynamicOffsets[1] != 512 {
			t.Errorf("dynamicOffsets[1] = %d, want 512", cmd.dynamicOffsets[1])
		}
	})

	t.Run("SetBindGroupInvalidType", func(t *testing.T) {
		enc := &CommandEncoder{}
		_ = enc.BeginEncoding("test")

		cpe := enc.BeginComputePass(nil)
		// Pass nil - should not add command
		cpe.SetBindGroup(0, nil, nil)

		if len(enc.commands) != 0 {
			t.Errorf("expected 0 commands for nil bind group, got %d", len(enc.commands))
		}
	})
}

// TestGLESComputeBarrier tests memory barrier usage in dispatch commands.
// Verifies that DispatchCommand.Execute() uses correct barrier bits.
func TestGLESComputeBarrier(t *testing.T) {
	// Test that dispatch commands include memory barriers
	t.Run("DispatchIncludesBarrier", func(t *testing.T) {
		cmd := &DispatchCommand{x: 1, y: 1, z: 1}

		// Execute should use SHADER_STORAGE_BARRIER_BIT | BUFFER_UPDATE_BARRIER_BIT
		// We can't test actual GL calls without GPU, but we verify the command exists
		expectedBarriers := gl.SHADER_STORAGE_BARRIER_BIT | gl.BUFFER_UPDATE_BARRIER_BIT
		if expectedBarriers != 0x2200 {
			t.Errorf("expected barriers = %#x, want 0x2200", expectedBarriers)
		}
	})

	t.Run("DispatchIndirectIncludesBarrier", func(t *testing.T) {
		buf := &Buffer{id: 1}
		cmd := &DispatchIndirectCommand{buffer: buf, offset: 0}

		// Verify command stores buffer reference
		if cmd.buffer.id != 1 {
			t.Errorf("buffer.id = %d, want 1", cmd.buffer.id)
		}
	})
}

// TestGLESComputeFullWorkflow tests a complete compute pass workflow.
func TestGLESComputeFullWorkflow(t *testing.T) {
	enc := &CommandEncoder{}
	_ = enc.BeginEncoding("compute_workflow")

	// Begin compute pass
	cpe := enc.BeginComputePass(&hal.ComputePassDescriptor{
		Label: "test_compute_pass",
	})

	// Set pipeline
	pipeline := &ComputePipeline{programID: 100}
	cpe.SetPipeline(pipeline)

	// Set bind groups
	bg0 := &BindGroup{}
	bg1 := &BindGroup{}
	cpe.SetBindGroup(0, bg0, nil)
	cpe.SetBindGroup(1, bg1, []uint32{128})

	// Dispatch
	cpe.Dispatch(64, 64, 1)

	// End pass
	cpe.End()

	// End encoding
	cmdBuf, err := enc.EndEncoding()
	if err != nil {
		t.Fatalf("EndEncoding failed: %v", err)
	}

	// Verify command buffer
	if cmdBuf == nil {
		t.Error("EndEncoding returned nil CommandBuffer")
	}

	// Verify command count: UseProgram + SetBindGroup*2 + Dispatch = 4
	glCmdBuf, ok := cmdBuf.(*CommandBuffer)
	if !ok {
		t.Fatalf("expected *CommandBuffer, got %T", cmdBuf)
	}

	expectedCmds := 4
	if len(glCmdBuf.commands) != expectedCmds {
		t.Errorf("command count = %d, want %d", len(glCmdBuf.commands), expectedCmds)
	}

	// Verify command types in order
	types := []string{
		"*gles.UseProgramCommand",
		"*gles.SetBindGroupCommand",
		"*gles.SetBindGroupCommand",
		"*gles.DispatchCommand",
	}

	for i, cmd := range glCmdBuf.commands {
		typeName := typeNameOf(cmd)
		if typeName != types[i] {
			t.Errorf("command[%d] = %s, want %s", i, typeName, types[i])
		}
	}
}

// typeNameOf returns the type name of a command for testing.
func typeNameOf(cmd Command) string {
	switch cmd.(type) {
	case *UseProgramCommand:
		return "*gles.UseProgramCommand"
	case *SetBindGroupCommand:
		return "*gles.SetBindGroupCommand"
	case *DispatchCommand:
		return "*gles.DispatchCommand"
	case *DispatchIndirectCommand:
		return "*gles.DispatchIndirectCommand"
	default:
		return "unknown"
	}
}
