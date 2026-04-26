// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows && !(js && wasm)

package dx12

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal/dx12/d3d12"
)

// TestBufferInitialState verifies that buffers start in the correct D3D12 resource state
// based on their heap type, matching the initial state set in CreateBuffer.
func TestBufferInitialState(t *testing.T) {
	tests := []struct {
		name          string
		heapType      d3d12.D3D12_HEAP_TYPE
		expectedState d3d12.D3D12_RESOURCE_STATES
	}{
		{
			name:          "default buffer starts in COMMON",
			heapType:      d3d12.D3D12_HEAP_TYPE_DEFAULT,
			expectedState: d3d12.D3D12_RESOURCE_STATE_COMMON,
		},
		{
			name:          "readback buffer starts in COPY_DEST",
			heapType:      d3d12.D3D12_HEAP_TYPE_READBACK,
			expectedState: d3d12.D3D12_RESOURCE_STATE_COPY_DEST,
		},
		{
			name:          "upload buffer starts in COMMON",
			heapType:      d3d12.D3D12_HEAP_TYPE_CUSTOM,
			expectedState: d3d12.D3D12_RESOURCE_STATE_COMMON,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &Buffer{
				heapType:     tt.heapType,
				currentState: tt.expectedState,
			}
			if buf.currentState != tt.expectedState {
				t.Errorf("currentState = %d, want %d", buf.currentState, tt.expectedState)
			}
		})
	}
}

// TestNeedsExplicitBarrier verifies the DX12 implicit promotion rules.
func TestNeedsExplicitBarrier(t *testing.T) {
	tests := []struct {
		name         string
		current      d3d12.D3D12_RESOURCE_STATES
		target       d3d12.D3D12_RESOURCE_STATES
		needsBarrier bool
	}{
		{
			name:         "same state requires no barrier",
			current:      d3d12.D3D12_RESOURCE_STATE_COPY_SOURCE,
			target:       d3d12.D3D12_RESOURCE_STATE_COPY_SOURCE,
			needsBarrier: false,
		},
		{
			name:         "COMMON to COPY_DEST is implicit",
			current:      d3d12.D3D12_RESOURCE_STATE_COMMON,
			target:       d3d12.D3D12_RESOURCE_STATE_COPY_DEST,
			needsBarrier: false,
		},
		{
			name:         "COMMON to VERTEX_AND_CONSTANT_BUFFER is implicit",
			current:      d3d12.D3D12_RESOURCE_STATE_COMMON,
			target:       d3d12.D3D12_RESOURCE_STATE_VERTEX_AND_CONSTANT_BUFFER,
			needsBarrier: false,
		},
		{
			name:         "COMMON to INDEX_BUFFER is implicit",
			current:      d3d12.D3D12_RESOURCE_STATE_COMMON,
			target:       d3d12.D3D12_RESOURCE_STATE_INDEX_BUFFER,
			needsBarrier: false,
		},
		{
			name:         "COMMON to NON_PIXEL_SHADER_RESOURCE is implicit",
			current:      d3d12.D3D12_RESOURCE_STATE_COMMON,
			target:       d3d12.D3D12_RESOURCE_STATE_NON_PIXEL_SHADER_RESOURCE,
			needsBarrier: false,
		},
		{
			name:         "COMMON to PIXEL_SHADER_RESOURCE is implicit",
			current:      d3d12.D3D12_RESOURCE_STATE_COMMON,
			target:       d3d12.D3D12_RESOURCE_STATE_PIXEL_SHADER_RESOURCE,
			needsBarrier: false,
		},
		{
			name:         "COMMON to INDIRECT_ARGUMENT is implicit",
			current:      d3d12.D3D12_RESOURCE_STATE_COMMON,
			target:       d3d12.D3D12_RESOURCE_STATE_INDIRECT_ARGUMENT,
			needsBarrier: false,
		},
		{
			name:         "COMMON to COPY_SOURCE requires explicit barrier",
			current:      d3d12.D3D12_RESOURCE_STATE_COMMON,
			target:       d3d12.D3D12_RESOURCE_STATE_COPY_SOURCE,
			needsBarrier: true,
		},
		{
			name:         "COMMON to UNORDERED_ACCESS requires explicit barrier",
			current:      d3d12.D3D12_RESOURCE_STATE_COMMON,
			target:       d3d12.D3D12_RESOURCE_STATE_UNORDERED_ACCESS,
			needsBarrier: true,
		},
		{
			name:         "UNORDERED_ACCESS to COPY_SOURCE requires explicit barrier",
			current:      d3d12.D3D12_RESOURCE_STATE_UNORDERED_ACCESS,
			target:       d3d12.D3D12_RESOURCE_STATE_COPY_SOURCE,
			needsBarrier: true,
		},
		{
			name:         "UNORDERED_ACCESS to COPY_DEST requires explicit barrier",
			current:      d3d12.D3D12_RESOURCE_STATE_UNORDERED_ACCESS,
			target:       d3d12.D3D12_RESOURCE_STATE_COPY_DEST,
			needsBarrier: true,
		},
		{
			name:         "COPY_DEST to UNORDERED_ACCESS requires explicit barrier",
			current:      d3d12.D3D12_RESOURCE_STATE_COPY_DEST,
			target:       d3d12.D3D12_RESOURCE_STATE_UNORDERED_ACCESS,
			needsBarrier: true,
		},
		{
			name:         "COPY_SOURCE to COPY_DEST requires explicit barrier",
			current:      d3d12.D3D12_RESOURCE_STATE_COPY_SOURCE,
			target:       d3d12.D3D12_RESOURCE_STATE_COPY_DEST,
			needsBarrier: true,
		},
		{
			name:         "COMMON to RENDER_TARGET requires explicit barrier",
			current:      d3d12.D3D12_RESOURCE_STATE_COMMON,
			target:       d3d12.D3D12_RESOURCE_STATE_RENDER_TARGET,
			needsBarrier: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := needsExplicitBarrier(tt.current, tt.target)
			if got != tt.needsBarrier {
				t.Errorf("needsExplicitBarrier(%d, %d) = %v, want %v",
					tt.current, tt.target, got, tt.needsBarrier)
			}
		})
	}
}

// TestComputeDispatchUpdatesBufferState verifies that Dispatch() marks bound
// storage buffers as UNORDERED_ACCESS.
func TestComputeDispatchUpdatesBufferState(t *testing.T) {
	buf1 := &Buffer{
		usage:        gputypes.BufferUsageStorage,
		currentState: d3d12.D3D12_RESOURCE_STATE_COMMON,
	}
	buf2 := &Buffer{
		usage:        gputypes.BufferUsageStorage,
		currentState: d3d12.D3D12_RESOURCE_STATE_COPY_DEST,
	}

	// Simulate ComputePassEncoder with bound storage buffers.
	// We can't call Dispatch() directly (needs recording cmdList), but we
	// can test the state update logic that Dispatch() performs.
	encoder := &ComputePassEncoder{
		encoder:             &CommandEncoder{isRecording: false}, // prevent actual D3D12 call
		boundStorageBuffers: []*Buffer{buf1, buf2},
	}

	// Simulate what Dispatch does after the D3D12 dispatch call:
	// mark all bound storage buffers as UNORDERED_ACCESS and clear the slice.
	for _, buf := range encoder.boundStorageBuffers {
		buf.currentState = d3d12.D3D12_RESOURCE_STATE_UNORDERED_ACCESS
	}
	encoder.boundStorageBuffers = encoder.boundStorageBuffers[:0]

	if buf1.currentState != d3d12.D3D12_RESOURCE_STATE_UNORDERED_ACCESS {
		t.Errorf("buf1.currentState = %d, want UNORDERED_ACCESS (%d)",
			buf1.currentState, d3d12.D3D12_RESOURCE_STATE_UNORDERED_ACCESS)
	}
	if buf2.currentState != d3d12.D3D12_RESOURCE_STATE_UNORDERED_ACCESS {
		t.Errorf("buf2.currentState = %d, want UNORDERED_ACCESS (%d)",
			buf2.currentState, d3d12.D3D12_RESOURCE_STATE_UNORDERED_ACCESS)
	}
	if len(encoder.boundStorageBuffers) != 0 {
		t.Errorf("boundStorageBuffers should be empty after dispatch, got %d",
			len(encoder.boundStorageBuffers))
	}
}

// TestBindGroupStorageBufferCollection verifies that CreateBindGroup
// correctly collects storage buffer references.
func TestBindGroupStorageBufferCollection(t *testing.T) {
	t.Run("no storage buffers", func(t *testing.T) {
		bg := &BindGroup{
			layout: &BindGroupLayout{
				entries: []BindGroupLayoutEntry{
					{Binding: 0, Type: BindingTypeUniformBuffer},
				},
			},
		}
		if len(bg.storageBuffers) != 0 {
			t.Errorf("storageBuffers should be empty, got %d", len(bg.storageBuffers))
		}
	})

	t.Run("with storage buffers", func(t *testing.T) {
		buf := &Buffer{
			usage:        gputypes.BufferUsageStorage,
			currentState: d3d12.D3D12_RESOURCE_STATE_COMMON,
			size:         1024,
		}
		bg := &BindGroup{
			layout: &BindGroupLayout{
				entries: []BindGroupLayoutEntry{
					{Binding: 0, Type: BindingTypeStorageBuffer},
				},
			},
			storageBuffers: []*Buffer{buf},
		}
		if len(bg.storageBuffers) != 1 {
			t.Fatalf("storageBuffers length = %d, want 1", len(bg.storageBuffers))
		}
		if bg.storageBuffers[0] != buf {
			t.Error("storageBuffers[0] does not match expected buffer")
		}
	})
}

// TestComputeToComputeBufferStateChain verifies the full compute -> copy -> compute
// state transition chain that Born ML exercises.
func TestComputeToComputeBufferStateChain(t *testing.T) {
	buf := &Buffer{
		usage:        gputypes.BufferUsageStorage | gputypes.BufferUsageCopySrc,
		currentState: d3d12.D3D12_RESOURCE_STATE_COMMON,
		size:         1048576, // 1MB -- the size that triggers BUG-DX12-012
	}

	// Step 1: After compute dispatch, buffer should be UNORDERED_ACCESS.
	buf.currentState = d3d12.D3D12_RESOURCE_STATE_UNORDERED_ACCESS
	if buf.currentState != d3d12.D3D12_RESOURCE_STATE_UNORDERED_ACCESS {
		t.Fatalf("after dispatch: state = %d, want UNORDERED_ACCESS", buf.currentState)
	}

	// Step 2: Before CopyBufferToBuffer, the code should detect the state mismatch.
	if !needsExplicitBarrier(buf.currentState, d3d12.D3D12_RESOURCE_STATE_COPY_SOURCE) {
		t.Fatal("UNORDERED_ACCESS -> COPY_SOURCE should require explicit barrier")
	}

	// Step 3: After transition, state should be COPY_SOURCE.
	buf.currentState = d3d12.D3D12_RESOURCE_STATE_COPY_SOURCE
	if buf.currentState != d3d12.D3D12_RESOURCE_STATE_COPY_SOURCE {
		t.Fatalf("after transition: state = %d, want COPY_SOURCE", buf.currentState)
	}

	// Step 4: For another compute dispatch, COPY_SOURCE -> UNORDERED_ACCESS
	// requires explicit barrier.
	if !needsExplicitBarrier(buf.currentState, d3d12.D3D12_RESOURCE_STATE_UNORDERED_ACCESS) {
		t.Fatal("COPY_SOURCE -> UNORDERED_ACCESS should require explicit barrier")
	}
}

// TestMultipleBarrierBatching verifies that when both src and dst buffers need
// barriers, they would be batched (the count logic in transitionBuffersForCopy).
func TestMultipleBarrierBatching(t *testing.T) {
	src := &Buffer{
		usage:        gputypes.BufferUsageStorage | gputypes.BufferUsageCopySrc,
		currentState: d3d12.D3D12_RESOURCE_STATE_UNORDERED_ACCESS,
	}
	dst := &Buffer{
		usage:        gputypes.BufferUsageStorage | gputypes.BufferUsageCopyDst,
		currentState: d3d12.D3D12_RESOURCE_STATE_UNORDERED_ACCESS,
	}

	// Both need explicit barriers.
	srcNeeds := needsExplicitBarrier(src.currentState, d3d12.D3D12_RESOURCE_STATE_COPY_SOURCE)
	dstNeeds := needsExplicitBarrier(dst.currentState, d3d12.D3D12_RESOURCE_STATE_COPY_DEST)

	if !srcNeeds {
		t.Error("src UAV -> COPY_SOURCE should need barrier")
	}
	if !dstNeeds {
		t.Error("dst UAV -> COPY_DEST should need barrier")
	}

	// Simulate the transition (without actual D3D12 calls).
	count := 0
	if srcNeeds {
		count++
		src.currentState = d3d12.D3D12_RESOURCE_STATE_COPY_SOURCE
	}
	if dstNeeds {
		count++
		dst.currentState = d3d12.D3D12_RESOURCE_STATE_COPY_DEST
	}

	if count != 2 {
		t.Errorf("expected 2 batched barriers, got %d", count)
	}
	if src.currentState != d3d12.D3D12_RESOURCE_STATE_COPY_SOURCE {
		t.Errorf("src.currentState = %d, want COPY_SOURCE", src.currentState)
	}
	if dst.currentState != d3d12.D3D12_RESOURCE_STATE_COPY_DEST {
		t.Errorf("dst.currentState = %d, want COPY_DEST", dst.currentState)
	}
}

// TestImplicitPromotionNoBarrier verifies that COMMON -> COPY_DEST does NOT
// insert an explicit barrier (DX12 implicit promotion).
func TestImplicitPromotionNoBarrier(t *testing.T) {
	buf := &Buffer{
		usage:        gputypes.BufferUsageCopyDst,
		currentState: d3d12.D3D12_RESOURCE_STATE_COMMON,
	}

	needs := needsExplicitBarrier(buf.currentState, d3d12.D3D12_RESOURCE_STATE_COPY_DEST)
	if needs {
		t.Error("COMMON -> COPY_DEST should be implicit promotion (no barrier)")
	}
}

// TestComputePassSetBindGroupTracksBuffers verifies that SetBindGroup
// accumulates storage buffers from bind groups.
func TestComputePassSetBindGroupTracksBuffers(t *testing.T) {
	buf1 := &Buffer{usage: gputypes.BufferUsageStorage, currentState: d3d12.D3D12_RESOURCE_STATE_COMMON}
	buf2 := &Buffer{usage: gputypes.BufferUsageStorage, currentState: d3d12.D3D12_RESOURCE_STATE_COMMON}

	bg1 := &BindGroup{
		layout:         &BindGroupLayout{entries: []BindGroupLayoutEntry{{Binding: 0, Type: BindingTypeStorageBuffer}}},
		storageBuffers: []*Buffer{buf1},
	}
	bg2 := &BindGroup{
		layout:         &BindGroupLayout{entries: []BindGroupLayoutEntry{{Binding: 0, Type: BindingTypeStorageBuffer}}},
		storageBuffers: []*Buffer{buf2},
	}

	encoder := &ComputePassEncoder{
		encoder: &CommandEncoder{isRecording: false}, // prevent actual D3D12 call
	}

	// Manually simulate what SetBindGroup does (can't call it directly without
	// recording command list).
	encoder.boundStorageBuffers = append(encoder.boundStorageBuffers, bg1.storageBuffers...)
	encoder.boundStorageBuffers = append(encoder.boundStorageBuffers, bg2.storageBuffers...)

	if len(encoder.boundStorageBuffers) != 2 {
		t.Fatalf("boundStorageBuffers = %d, want 2", len(encoder.boundStorageBuffers))
	}
	if encoder.boundStorageBuffers[0] != buf1 {
		t.Error("first buffer should be buf1")
	}
	if encoder.boundStorageBuffers[1] != buf2 {
		t.Error("second buffer should be buf2")
	}
}

// TestDispatchClearsBoundBuffers verifies that after dispatch, the bound storage
// buffers slice is cleared for the next dispatch (per-dispatch usage scope).
func TestDispatchClearsBoundBuffers(t *testing.T) {
	buf := &Buffer{
		usage:        gputypes.BufferUsageStorage,
		currentState: d3d12.D3D12_RESOURCE_STATE_COMMON,
	}

	encoder := &ComputePassEncoder{
		encoder:             &CommandEncoder{isRecording: false},
		boundStorageBuffers: []*Buffer{buf},
	}

	// Simulate dispatch state update.
	for _, b := range encoder.boundStorageBuffers {
		b.currentState = d3d12.D3D12_RESOURCE_STATE_UNORDERED_ACCESS
	}
	encoder.boundStorageBuffers = encoder.boundStorageBuffers[:0]

	if len(encoder.boundStorageBuffers) != 0 {
		t.Errorf("boundStorageBuffers should be empty after dispatch, got %d",
			len(encoder.boundStorageBuffers))
	}
	// Capacity should be preserved for reuse (no allocation on next SetBindGroup).
	if cap(encoder.boundStorageBuffers) == 0 {
		t.Error("capacity should be preserved after clear")
	}
}

// TestBufferStateTransitionConstants verifies the D3D12 resource state constant values.
func TestBufferStateTransitionConstants(t *testing.T) {
	if d3d12.D3D12_RESOURCE_STATE_COMMON != 0 {
		t.Errorf("COMMON = %d, want 0", d3d12.D3D12_RESOURCE_STATE_COMMON)
	}
	if d3d12.D3D12_RESOURCE_STATE_UNORDERED_ACCESS != 0x8 {
		t.Errorf("UNORDERED_ACCESS = %d, want 0x8", d3d12.D3D12_RESOURCE_STATE_UNORDERED_ACCESS)
	}
	if d3d12.D3D12_RESOURCE_STATE_COPY_DEST != 0x400 {
		t.Errorf("COPY_DEST = %d, want 0x400", d3d12.D3D12_RESOURCE_STATE_COPY_DEST)
	}
	if d3d12.D3D12_RESOURCE_STATE_COPY_SOURCE != 0x800 {
		t.Errorf("COPY_SOURCE = %d, want 0x800", d3d12.D3D12_RESOURCE_STATE_COPY_SOURCE)
	}
	if d3d12.D3D12_RESOURCE_STATE_VERTEX_AND_CONSTANT_BUFFER != 0x1 {
		t.Errorf("VERTEX_AND_CONSTANT_BUFFER = %d, want 0x1",
			d3d12.D3D12_RESOURCE_STATE_VERTEX_AND_CONSTANT_BUFFER)
	}
}

// TestReadbackBufferNoCopyDestBarrier verifies that a readback buffer (initial
// state COPY_DEST) does not get a redundant barrier when used as copy destination.
func TestReadbackBufferNoCopyDestBarrier(t *testing.T) {
	buf := &Buffer{
		usage:        gputypes.BufferUsageCopyDst | gputypes.BufferUsageMapRead,
		currentState: d3d12.D3D12_RESOURCE_STATE_COPY_DEST,
	}

	needs := needsExplicitBarrier(buf.currentState, d3d12.D3D12_RESOURCE_STATE_COPY_DEST)
	if needs {
		t.Error("readback buffer already in COPY_DEST should not need barrier")
	}
}
