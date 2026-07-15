// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows && !(js && wasm)

package dx12

import (
	"fmt"
	"image"
	"sync"
	"time"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/dx12/d3d12"
	"github.com/gogpu/wgpu/hal/dx12/dxgi"
)

// Queue implements hal.Queue for DirectX 12.
// It manages command submission and presentation to surfaces.
type Queue struct {
	device *Device
	raw    *d3d12.ID3D12CommandQueue
	state  *queueState
}

type queueState struct {
	// submitMu serializes state reconciliation with ExecuteCommandLists. The
	// public queue already serializes at the wgpu layer, but HAL callers may
	// submit directly. It also excludes terminal Device teardown from every
	// queue operation that can touch native device state.
	submitMu sync.Mutex
	closed   bool

	preambleInFlight []preambleInFlight
	preambleIdle     []preambleInFlight
	preambleOps      preambleNativeOps
	oneShotsInFlight []oneShotInFlight
}

type preambleInFlight struct {
	submission uint64
	allocator  *d3d12.ID3D12CommandAllocator
	cmdList    *d3d12.ID3D12GraphicsCommandList
	testID     uint64
}

// preambleNativeOps keeps allocator/list lifecycle effects behind a narrow
// seam. The pool owns allocator/list pairs as one unit; tests can exercise
// every failure path without constructing COM objects.
type preambleNativeOps interface {
	create(*Device) (preambleInFlight, error)
	resetAllocator(preambleInFlight) error
	resetCommandList(preambleInFlight) error
	recordAndClose(preambleInFlight, []stateBarrierPlan) error
	release(preambleInFlight)
}

type d3d12PreambleNativeOps struct{}

// oneShotInFlight owns only native objects. Keeping Device/Buffer/encoder
// wrappers here would create a finalizer cycle through Device.queueState.
type oneShotInFlight struct {
	submission  uint64
	staging     *d3d12.ID3D12Resource
	destination *d3d12.ID3D12Resource
	allocator   *d3d12.ID3D12CommandAllocator
	cmdList     *d3d12.ID3D12GraphicsCommandList
}

// oneShotWriteOwner is the single local owner while a staged write is being
// prepared. Before submission failures it releases the wrappers normally.
// After EndEncoding, detach transfers the native objects to queueState without
// retaining wrapper back-references to Device.
type oneShotWriteOwner struct {
	staging       *Buffer
	destination   *d3d12.ID3D12Resource // explicit extra COM reference
	encoder       *CommandEncoder
	commandBuffer *CommandBuffer
}

func newOneShotWriteOwner(staging *Buffer, destination *d3d12.ID3D12Resource) *oneShotWriteOwner {
	// The caller still owns its Buffer/Texture wrapper and may destroy it as
	// soon as Write returns an error. Hold a distinct native reference until
	// completion so post-Execute ambiguity cannot free the GPU destination.
	destination.AddRef()
	return &oneShotWriteOwner{staging: staging, destination: destination}
}

func (o *oneShotWriteOwner) release() {
	if o == nil {
		return
	}
	if o.commandBuffer != nil {
		o.commandBuffer.Destroy()
	}
	if o.encoder != nil {
		o.encoder.Destroy()
	}
	if o.staging != nil {
		o.staging.Destroy()
	}
	if o.destination != nil {
		o.destination.Release()
		o.destination = nil
	}
}

func (o *oneShotWriteOwner) detach(submission uint64) oneShotInFlight {
	retained := oneShotInFlight{submission: submission}
	if o.staging != nil {
		retained.staging = o.staging.raw
		o.staging.raw = nil
		o.staging.mappedPointer = nil
	}
	retained.destination = o.destination
	o.destination = nil
	if o.commandBuffer != nil {
		retained.cmdList = o.commandBuffer.cmdList
		o.commandBuffer.cmdList = nil
	}
	if o.encoder != nil {
		retained.allocator = o.encoder.allocator
		o.encoder.allocator = nil
	}
	return retained
}

// newQueue creates a new Queue wrapping the device's command queue.
func newQueue(device *Device) *Queue {
	state := device.queueState
	if state == nil {
		state = &queueState{}
		device.queueState = state
	}
	queue := &Queue{
		device: device,
		raw:    device.directQueue,
		state:  state,
	}
	return queue
}

// Submit submits command buffers to the GPU.
// Returns a monotonically increasing submission index for tracking completion.
func (q *Queue) Submit(commandBuffers []hal.CommandBuffer) (uint64, error) {
	if err := q.lockOpen(); err != nil {
		return 0, err
	}
	defer q.state.submitMu.Unlock()
	return q.submitLocked(commandBuffers)
}

func (q *Queue) submitLocked(commandBuffers []hal.CommandBuffer) (uint64, error) {
	completed := q.device.completedFrameFenceValue()
	q.releaseCompletedPreambles(completed)
	q.releaseCompletedOneShots(completed)

	if len(commandBuffers) == 0 {
		return 0, nil
	}

	// Convert command buffers and collect their command-local summaries.
	summaries := make([]commandStateSummary, 0)
	for _, cb := range commandBuffers {
		dx12CB, ok := cb.(*CommandBuffer)
		if !ok || dx12CB == nil || dx12CB.cmdList == nil {
			return 0, fmt.Errorf("dx12: command buffer is not a DX12 command buffer")
		}
		summaries = append(summaries, dx12CB.stateSummaries...)
	}

	scheduled := q.snapshotScheduledStates(summaries)
	commands := commandSummariesInOrder(commandBuffers)
	preambles, finalStates := planSubmissionState(scheduled, commands)
	nativePreambles := make([]preambleInFlight, 0)
	executionLists := make([]*d3d12.ID3D12GraphicsCommandList, 0, len(commandBuffers)+len(preambles))
	for i, cb := range commandBuffers {
		dx12CB := cb.(*CommandBuffer)
		if i < len(preambles) && len(preambles[i]) > 0 {
			preamble, err := q.buildPreamble(preambles[i])
			if err != nil {
				q.releaseBuiltPreambles(nativePreambles)
				return 0, err
			}
			nativePreambles = append(nativePreambles, preamble)
			executionLists = append(executionLists, preamble.cmdList)
			dx12CB.preambleBarriers = append(dx12CB.preambleBarriers[:0], preambles[i]...)
		}
		executionLists = append(executionLists, dx12CB.cmdList)
	}

	// Execute command lists
	submitStart := time.Now()
	q.raw.ExecuteCommandLists(uint32(len(executionLists)), &executionLists[0])

	// Check for immediate device removal after execution.
	if reason := q.device.raw.GetDeviceRemovedReason(); reason != nil {
		q.device.logDREDBreadcrumbs()
		q.retainPreamblesWithoutFence(nativePreambles)
		return 0, fmt.Errorf("dx12: device removed after ExecuteCommandLists: %w", reason)
	}
	q.commitScheduledStates(finalStates)

	// Drain debug messages after submission.
	q.device.DrainDebugMessages()

	// Signal the frame fence for per-frame allocator tracking and return its value
	// as the submission index.
	if err := q.device.signalFrameFence(); err != nil {
		// ExecuteCommandLists has already handed these lists to the GPU. A
		// failed Signal leaves no completion value with which to prove that
		// releasing their allocator/list is safe, so retain them permanently
		// rather than risking reuse while GPU work is in flight.
		q.retainPreamblesWithoutFence(nativePreambles)
		return 0, err
	}
	submission := q.device.currentFrameFenceValue()
	for i := range nativePreambles {
		nativePreambles[i].submission = submission
	}
	q.state.preambleInFlight = append(q.state.preambleInFlight, nativePreambles...)

	hal.Logger().Debug("dx12: command list submitted",
		"cmdLists", len(executionLists),
		"elapsed", time.Since(submitStart),
	)

	return submission, nil
}

func (q *Queue) releaseBuiltPreambles(preambles []preambleInFlight) {
	for _, preamble := range preambles {
		q.state.releasePreamble(preamble)
	}
}

func (q *Queue) lockOpen() error {
	if q == nil || q.state == nil {
		return fmt.Errorf("dx12: queue is unavailable")
	}
	q.state.submitMu.Lock()
	if q.state.closed {
		q.state.submitMu.Unlock()
		return fmt.Errorf("dx12: queue is closed")
	}
	return nil
}

func commandSummariesInOrder(commandBuffers []hal.CommandBuffer) []commandStateSummary {
	var summaries []commandStateSummary
	for commandIndex, commandBuffer := range commandBuffers {
		cb, ok := commandBuffer.(*CommandBuffer)
		if !ok || cb == nil {
			continue
		}
		for _, summary := range cb.stateSummaries {
			summary.commandIndex = commandIndex
			summaries = append(summaries, summary)
		}
	}
	return summaries
}

func (q *Queue) snapshotScheduledStates(summaries []commandStateSummary) map[any]map[uint32]d3d12.D3D12_RESOURCE_STATES {
	scheduled := make(map[any]map[uint32]d3d12.D3D12_RESOURCE_STATES)
	for _, summary := range summaries {
		states := scheduled[summary.resource]
		if states == nil {
			states = make(map[uint32]d3d12.D3D12_RESOURCE_STATES)
			scheduled[summary.resource] = states
		}
		switch resource := summary.resource.(type) {
		case *Buffer:
			states[0] = resource.scheduledStateSnapshot()
		case *Texture:
			for _, state := range summary.states {
				states[state.subresource] = resource.scheduledStateSnapshot(state.subresource)
			}
		}
	}
	return scheduled
}

func (q *Queue) commitScheduledStates(final map[any]map[uint32]d3d12.D3D12_RESOURCE_STATES) {
	for resource, states := range final {
		switch typed := resource.(type) {
		case *Buffer:
			if state, ok := states[0]; ok {
				typed.commitScheduledState(state)
			}
		case *Texture:
			allStates := typed.scheduledStateSnapshotAll()
			count := int(typed.subresourceCount())
			if len(allStates) < count {
				fallback := d3d12.D3D12_RESOURCE_STATE_COMMON
				if len(allStates) > 0 {
					fallback = allStates[0]
				}
				oldLen := len(allStates)
				allStates = append(allStates, make([]d3d12.D3D12_RESOURCE_STATES, count-oldLen)...)
				for i := oldLen; i < len(allStates); i++ {
					allStates[i] = fallback
				}
			}
			for subresource, state := range states {
				if int(subresource) < len(allStates) {
					allStates[subresource] = state
				}
			}
			typed.commitScheduledStates(allStates)
		}
	}
}

func (q *Queue) buildPreamble(plans []stateBarrierPlan) (preambleInFlight, error) {
	ops := q.state.nativePreambleOps()
	if count := len(q.state.preambleIdle); count > 0 {
		preamble := q.state.preambleIdle[count-1]
		q.state.preambleIdle[count-1] = preambleInFlight{}
		q.state.preambleIdle = q.state.preambleIdle[:count-1]
		preamble.submission = 0

		if err := ops.resetAllocator(preamble); err == nil {
			if err = ops.resetCommandList(preamble); err == nil {
				if err = ops.recordAndClose(preamble, plans); err == nil {
					return preamble, nil
				}
			}
		}
		// A pair with any failed reset or close is no longer reusable. Release
		// both objects before falling back to a fresh pair.
		ops.release(preamble)
	}

	preamble, err := ops.create(q.device)
	if err != nil {
		return preambleInFlight{}, err
	}
	if err := ops.recordAndClose(preamble, plans); err != nil {
		ops.release(preamble)
		return preambleInFlight{}, fmt.Errorf("dx12: close state preamble command list: %w", err)
	}
	return preamble, nil
}

func (d3d12PreambleNativeOps) create(device *Device) (preambleInFlight, error) {
	allocator, err := device.raw.CreateCommandAllocator(d3d12.D3D12_COMMAND_LIST_TYPE_DIRECT)
	if err != nil {
		return preambleInFlight{}, fmt.Errorf("dx12: create state preamble allocator: %w", err)
	}
	list, err := device.raw.CreateCommandList(0, d3d12.D3D12_COMMAND_LIST_TYPE_DIRECT, allocator, nil)
	if err != nil {
		allocator.Release()
		return preambleInFlight{}, fmt.Errorf("dx12: create state preamble command list: %w", err)
	}
	return preambleInFlight{allocator: allocator, cmdList: list}, nil
}

func (d3d12PreambleNativeOps) resetAllocator(preamble preambleInFlight) error {
	return preamble.allocator.Reset()
}

func (d3d12PreambleNativeOps) resetCommandList(preamble preambleInFlight) error {
	return preamble.cmdList.Reset(preamble.allocator, nil)
}

func (d3d12PreambleNativeOps) recordAndClose(preamble preambleInFlight, plans []stateBarrierPlan) error {
	barriers := make([]d3d12.D3D12_RESOURCE_BARRIER, 0, len(plans))
	for _, plan := range plans {
		var raw *d3d12.ID3D12Resource
		switch resource := plan.resource.(type) {
		case *Buffer:
			raw = resource.raw
		case *Texture:
			raw = resource.raw
		}
		if raw == nil || plan.before == plan.after {
			continue
		}
		barriers = append(barriers, d3d12.NewTransitionBarrier(raw, plan.before, plan.after, plan.subresource))
	}
	if len(barriers) > 0 {
		preamble.cmdList.ResourceBarrier(uint32(len(barriers)), &barriers[0])
	}
	return preamble.cmdList.Close()
}

func (d3d12PreambleNativeOps) release(preamble preambleInFlight) {
	if preamble.cmdList != nil {
		preamble.cmdList.Release()
	}
	if preamble.allocator != nil {
		preamble.allocator.Release()
	}
}

func (q *Queue) releaseCompletedPreambles(completed uint64) {
	keep := q.state.preambleInFlight[:0]
	for _, preamble := range q.state.preambleInFlight {
		if preamble.submission != 0 && preamble.submission <= completed {
			preamble.submission = 0
			if len(q.state.preambleIdle) < maxFramesInFlight {
				q.state.preambleIdle = append(q.state.preambleIdle, preamble)
			} else {
				q.state.releasePreamble(preamble)
			}
			continue
		}
		keep = append(keep, preamble)
	}
	q.state.preambleInFlight = keep
}

func (s *queueState) nativePreambleOps() preambleNativeOps {
	if s.preambleOps != nil {
		return s.preambleOps
	}
	return d3d12PreambleNativeOps{}
}

func (s *queueState) releasePreamble(preamble preambleInFlight) {
	s.nativePreambleOps().release(preamble)
}

func (q *Queue) releaseCompletedOneShots(completed uint64) {
	keep := q.state.oneShotsInFlight[:0]
	for _, oneShot := range q.state.oneShotsInFlight {
		if oneShot.submission != 0 && oneShot.submission <= completed {
			releaseOneShot(oneShot)
			continue
		}
		keep = append(keep, oneShot)
	}
	q.state.oneShotsInFlight = keep
}

func releaseOneShot(oneShot oneShotInFlight) {
	// The command list must be released before its allocator.
	if oneShot.cmdList != nil {
		oneShot.cmdList.Release()
	}
	if oneShot.allocator != nil {
		oneShot.allocator.Release()
	}
	if oneShot.staging != nil {
		oneShot.staging.Release()
	}
	if oneShot.destination != nil {
		oneShot.destination.Release()
	}
}

func (s *queueState) releaseAllOwnedLocked() {
	for _, preamble := range s.preambleInFlight {
		s.releasePreamble(preamble)
	}
	s.preambleInFlight = nil
	s.releaseIdlePreamblesLocked()
	for _, oneShot := range s.oneShotsInFlight {
		releaseOneShot(oneShot)
	}
	s.oneShotsInFlight = nil
}

func (s *queueState) releaseIdlePreamblesLocked() {
	for _, preamble := range s.preambleIdle {
		s.releasePreamble(preamble)
	}
	s.preambleIdle = nil
}

func (s *queueState) releaseTerminalOwnedLocked(waitErr error, deviceRemoved bool) {
	if shouldReleaseTerminalOwnedObjects(waitErr, deviceRemoved) {
		s.releaseAllOwnedLocked()
		return
	}
	// Idle pairs have already crossed a trustworthy fence. Keep only native
	// objects whose in-flight completion remains ambiguous.
	s.releaseIdlePreamblesLocked()
}

func (q *Queue) retainPreamblesWithoutFence(preambles []preambleInFlight) {
	for i := range preambles {
		// submission=0 is intentionally never released by
		// releaseCompletedPreambles because no fence value is trustworthy.
		preambles[i].submission = 0
	}
	q.state.preambleInFlight = append(q.state.preambleInFlight, preambles...)
}

func (q *Queue) retainOneShot(owner *oneShotWriteOwner, submission uint64) {
	q.state.oneShotsInFlight = append(q.state.oneShotsInFlight, owner.detach(submission))
}

// PollCompleted returns the highest submission index known to be completed by the GPU.
// Non-blocking.
func (q *Queue) PollCompleted() uint64 {
	if err := q.lockOpen(); err != nil {
		return 0
	}
	defer q.state.submitMu.Unlock()
	completed := q.device.completedFrameFenceValue()
	q.releaseCompletedPreambles(completed)
	q.releaseCompletedOneShots(completed)
	return completed
}

// WriteBuffer writes data to a buffer immediately.
// For upload heap buffers, data is copied directly via CPU mapping.
// For default heap buffers, a staging buffer + GPU copy command is used.
func (q *Queue) WriteBuffer(buffer hal.Buffer, offset uint64, data []byte) error {
	if err := q.lockOpen(); err != nil {
		return err
	}
	defer q.state.submitMu.Unlock()

	if buffer == nil {
		return fmt.Errorf("dx12: WriteBuffer: nil buffer")
	}
	if len(data) == 0 {
		return nil
	}

	buf, ok := buffer.(*Buffer)
	if !ok || buf.raw == nil {
		return fmt.Errorf("dx12: WriteBuffer: invalid buffer type")
	}

	// Mappable buffers (CUSTOM/UPLOAD heaps) can be written directly via CPU mapping
	if buf.isMappable() {
		return q.writeBufferDirect(buf, offset, data)
	}

	// Default heap buffers require staging buffer + GPU copy
	return q.writeBufferStaged(buf, offset, data)
}

// writeBufferDirect copies data to a mappable (upload heap) buffer.
func (q *Queue) writeBufferDirect(buf *Buffer, offset uint64, data []byte) error {
	if buf.mappedPointer != nil {
		// Already mapped — copy directly
		dst := unsafe.Slice((*byte)(unsafe.Add(buf.mappedPointer, int(offset))), len(data))
		copy(dst, data)
		return nil
	}

	// Temporarily map, copy, unmap
	readRange := &d3d12.D3D12_RANGE{Begin: 0, End: 0} // No reads
	ptr, err := buf.raw.Map(0, readRange)
	if err != nil {
		return fmt.Errorf("dx12: WriteBuffer: Map failed: %w", err)
	}
	dst := unsafe.Slice((*byte)(unsafe.Add(ptr, int(offset))), len(data))
	copy(dst, data)

	writtenRange := &d3d12.D3D12_RANGE{
		Begin: uintptr(offset),
		End:   uintptr(offset + uint64(len(data))),
	}
	buf.raw.Unmap(0, writtenRange)
	return nil
}

// writeBufferStaged copies data to a GPU-only (default heap) buffer
// via an upload heap staging buffer and CopyBufferRegion command.
func (q *Queue) writeBufferStaged(buf *Buffer, offset uint64, data []byte) error {
	owner := newOneShotWriteOwner(nil, buf.raw)
	defer func() {
		if owner != nil {
			owner.release()
		}
	}()

	// Create upload heap staging buffer (mapped at creation for immediate write)
	staging, err := q.device.CreateBuffer(&hal.BufferDescriptor{
		Label:            "write-buffer-staging",
		Size:             uint64(len(data)),
		Usage:            gputypes.BufferUsageCopySrc | gputypes.BufferUsageMapWrite,
		MappedAtCreation: true,
	})
	if err != nil {
		return fmt.Errorf("dx12: WriteBuffer: staging buffer creation failed: %w", err)
	}
	stagingBuf := staging.(*Buffer)
	owner.staging = stagingBuf

	// Copy data to mapped staging buffer
	dst := unsafe.Slice((*byte)(stagingBuf.mappedPointer), len(data))
	copy(dst, data)

	// Unmap staging buffer
	writtenRange := &d3d12.D3D12_RANGE{Begin: 0, End: uintptr(len(data))}
	stagingBuf.raw.Unmap(0, writtenRange)
	stagingBuf.mappedPointer = nil

	// Create one-shot command encoder for the copy
	cmdEncoder, err := q.device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{
		Label: "write-buffer-copy",
	})
	if err != nil {
		return fmt.Errorf("dx12: WriteBuffer: CreateCommandEncoder failed: %w", err)
	}

	encoder := cmdEncoder.(*CommandEncoder)
	owner.encoder = encoder
	if err := encoder.BeginEncoding("write-buffer-copy"); err != nil {
		return fmt.Errorf("dx12: WriteBuffer: BeginEncoding failed: %w", err)
	}

	// Route the one-shot copy through the same command-local tracker as user
	// command buffers. The destination may already be in a shader/UAV state.
	plans := make([]stateBarrierPlan, 0, 2)
	if before, needsBarrier := encoder.stateTracker.transitionBuffer(stagingBuf, d3d12.D3D12_RESOURCE_STATE_COPY_SOURCE); needsBarrier {
		plans = append(plans, stateBarrierPlan{resource: stagingBuf, subresource: d3d12.D3D12_RESOURCE_BARRIER_ALL_SUBRESOURCES, before: before, after: d3d12.D3D12_RESOURCE_STATE_COPY_SOURCE})
	}
	if before, needsBarrier := encoder.stateTracker.transitionBuffer(buf, d3d12.D3D12_RESOURCE_STATE_COPY_DEST); needsBarrier {
		plans = append(plans, stateBarrierPlan{resource: buf, subresource: d3d12.D3D12_RESOURCE_BARRIER_ALL_SUBRESOURCES, before: before, after: d3d12.D3D12_RESOURCE_STATE_COPY_DEST})
	}
	encoder.emitStateBarrierPlans(plans)
	encoder.cmdList.CopyBufferRegion(buf.raw, offset, stagingBuf.raw, 0, uint64(len(data)))

	cmdBuffer, err := encoder.EndEncoding()
	if err != nil {
		return fmt.Errorf("dx12: WriteBuffer: EndEncoding failed: %w", err)
	}
	owner.commandBuffer = cmdBuffer.(*CommandBuffer)

	// Submit and wait for GPU completion.
	submission, err := q.submitLocked([]hal.CommandBuffer{cmdBuffer})
	if err != nil {
		q.retainOneShot(owner, 0)
		owner = nil
		return fmt.Errorf("dx12: WriteBuffer: Submit failed: %w", err)
	}
	// Block until GPU finishes the copy — staging buffer must remain valid.
	if err := q.device.waitForGPU(); err != nil {
		q.retainOneShot(owner, submission)
		owner = nil
		return fmt.Errorf("dx12: WriteBuffer: WaitIdle failed: %w", err)
	}
	// A successful all-queue wait proves completion even for older retained
	// objects that had no trustworthy submission fence.
	q.state.releaseAllOwnedLocked()
	return nil
}

const (
	// D3D12 placed footprints require a 256-byte row pitch and a 512-byte
	// starting offset. Array layers use separate footprints, so their staging
	// strides must satisfy both alignments.
	d3d12TexturePitchAlignment     = 256
	d3d12TexturePlacementAlignment = 512
)

// WriteTexture writes data to a texture immediately.
// Creates an upload heap staging buffer, copies data with proper row pitch
// alignment, and uses CopyTextureRegion to transfer to the GPU texture.
func (q *Queue) WriteTexture(dst *hal.ImageCopyTexture, data []byte, layout *hal.ImageDataLayout, size *hal.Extent3D) error {
	if err := q.lockOpen(); err != nil {
		return err
	}
	defer q.state.submitMu.Unlock()

	if dst == nil || dst.Texture == nil || len(data) == 0 || layout == nil || size == nil {
		return fmt.Errorf("dx12: WriteTexture: invalid arguments")
	}

	dstTex, ok := dst.Texture.(*Texture)
	if !ok || dstTex.raw == nil {
		return fmt.Errorf("dx12: WriteTexture: invalid texture type")
	}

	// Calculate layout parameters
	bytesPerRow := layout.BytesPerRow
	if bytesPerRow == 0 {
		bytesPerRow = size.Width * 4 // Assume RGBA8 (4 bytes per pixel)
	}

	rowsPerImage := layout.RowsPerImage
	if rowsPerImage == 0 {
		rowsPerImage = size.Height
	}
	blockHeight := textureFormatBlockHeight(dstTex.format)
	sourceBlockRowsPerImage := (rowsPerImage + blockHeight - 1) / blockHeight
	copyBlockRows := (size.Height + blockHeight - 1) / blockHeight

	depthOrLayers := size.DepthOrArrayLayers
	if depthOrLayers == 0 {
		depthOrLayers = 1
	}

	// D3D12 requires RowPitch to be aligned to 256 bytes
	alignedRowPitch := (bytesPerRow + d3d12TexturePitchAlignment - 1) &^ (d3d12TexturePitchAlignment - 1)

	stagingBlockRowsPerImage := copyBlockRows
	stagingLayerStride := uint64(alignedRowPitch) * uint64(stagingBlockRowsPerImage)
	if dstTex.dimension != gputypes.TextureDimension3D {
		stagingLayerStride = (stagingLayerStride + d3d12TexturePlacementAlignment - 1) &^ uint64(d3d12TexturePlacementAlignment-1)
		stagingBlockRowsPerImage = uint32(stagingLayerStride / uint64(alignedRowPitch))
	}
	stagingRowsPerImage := stagingBlockRowsPerImage * blockHeight
	stagingSize := stagingLayerStride * uint64(depthOrLayers)
	owner := newOneShotWriteOwner(nil, dstTex.raw)
	defer func() {
		if owner != nil {
			owner.release()
		}
	}()

	// Create upload heap staging buffer
	staging, err := q.device.CreateBuffer(&hal.BufferDescriptor{
		Label:            "write-texture-staging",
		Size:             stagingSize,
		Usage:            gputypes.BufferUsageCopySrc | gputypes.BufferUsageMapWrite,
		MappedAtCreation: true,
	})
	if err != nil {
		return fmt.Errorf("dx12: WriteTexture: CreateBuffer failed: %w", err)
	}
	stagingBuf := staging.(*Buffer)
	owner.staging = stagingBuf

	// Repack only the image rows. Input RowsPerImage controls the source stride;
	// staging padding is chosen independently to satisfy D3D12 placement rules.
	srcOffset := layout.Offset
	for z := uint32(0); z < depthOrLayers; z++ {
		for row := uint32(0); row < copyBlockRows; row++ {
			srcStart := srcOffset + uint64(z)*uint64(bytesPerRow)*uint64(sourceBlockRowsPerImage) + uint64(row)*uint64(bytesPerRow)
			if srcStart > uint64(len(data)) || uint64(bytesPerRow) > uint64(len(data))-srcStart {
				return fmt.Errorf("dx12: WriteTexture: source data is too short for layer %d block row %d", z, row)
			}
			dstStart := uint64(z)*stagingLayerStride + uint64(row)*uint64(alignedRowPitch)
			src := data[srcStart : srcStart+uint64(bytesPerRow)]
			d := unsafe.Slice((*byte)(unsafe.Add(stagingBuf.mappedPointer, int(dstStart))), bytesPerRow)
			copy(d, src)
		}
	}

	// Unmap staging buffer
	writtenRange := &d3d12.D3D12_RANGE{Begin: 0, End: uintptr(stagingSize)}
	stagingBuf.raw.Unmap(0, writtenRange)
	stagingBuf.mappedPointer = nil

	// Create one-shot command encoder
	cmdEncoder, err := q.device.CreateCommandEncoder(&hal.CommandEncoderDescriptor{
		Label: "write-texture-copy",
	})
	if err != nil {
		return fmt.Errorf("dx12: WriteTexture: CreateCommandEncoder failed: %w", err)
	}

	encoder := cmdEncoder.(*CommandEncoder)
	owner.encoder = encoder
	if err := encoder.BeginEncoding("write-texture-copy"); err != nil {
		return fmt.Errorf("dx12: WriteTexture: BeginEncoding failed: %w", err)
	}

	plans := make([]stateBarrierPlan, 0, 3)
	if before, needsBarrier := encoder.stateTracker.transitionBuffer(stagingBuf, d3d12.D3D12_RESOURCE_STATE_COPY_SOURCE); needsBarrier {
		plans = append(plans, stateBarrierPlan{resource: stagingBuf, subresource: d3d12.D3D12_RESOURCE_BARRIER_ALL_SUBRESOURCES, before: before, after: d3d12.D3D12_RESOURCE_STATE_COPY_SOURCE})
	}
	stagingLayout := hal.ImageDataLayout{BytesPerRow: alignedRowPitch, RowsPerImage: stagingRowsPerImage}
	copyPlans := planBufferTextureCopies(dstTex, *dst, stagingLayout, *size)
	for _, copyPlan := range copyPlans {
		if before, needsBarrier := encoder.stateTracker.transitionTexture(dstTex, copyPlan.subresource, d3d12.D3D12_RESOURCE_STATE_COPY_DEST); needsBarrier {
			plans = append(plans, stateBarrierPlan{resource: dstTex, subresource: copyPlan.subresource, before: before, after: d3d12.D3D12_RESOURCE_STATE_COPY_DEST})
		}
	}
	encoder.emitStateBarrierPlans(plans)

	for _, copyPlan := range copyPlans {
		srcLoc := d3d12.D3D12_TEXTURE_COPY_LOCATION{
			Resource: stagingBuf.raw,
			Type:     d3d12.D3D12_TEXTURE_COPY_TYPE_PLACED_FOOTPRINT,
		}
		srcLoc.SetPlacedFootprint(d3d12.D3D12_PLACED_SUBRESOURCE_FOOTPRINT{
			Offset: copyPlan.bufferOffset,
			Footprint: d3d12.D3D12_SUBRESOURCE_FOOTPRINT{
				Format:   textureFormatToD3D12(dstTex.format),
				Width:    size.Width,
				Height:   copyPlan.footprintHeight,
				Depth:    copyPlan.footprintDepth,
				RowPitch: alignedRowPitch,
			},
		})

		dstLoc := d3d12.D3D12_TEXTURE_COPY_LOCATION{
			Resource: dstTex.raw,
			Type:     d3d12.D3D12_TEXTURE_COPY_TYPE_SUBRESOURCE_INDEX,
		}
		dstLoc.SetSubresourceIndex(copyPlan.subresource)
		srcBox := d3d12.D3D12_BOX{
			Left:   0,
			Top:    copyPlan.bufferOriginY,
			Front:  0,
			Right:  size.Width,
			Bottom: copyPlan.bufferOriginY + size.Height,
			Back:   copyPlan.footprintDepth,
		}

		encoder.cmdList.CopyTextureRegion(
			&dstLoc,
			dst.Origin.X, dst.Origin.Y, copyPlan.textureOriginZ,
			&srcLoc,
			&srcBox,
		)
	}

	// Transition the written subresources to shader resource state (ready for rendering).
	afterState := d3d12.D3D12_RESOURCE_STATE_PIXEL_SHADER_RESOURCE | d3d12.D3D12_RESOURCE_STATE_NON_PIXEL_SHADER_RESOURCE
	for _, copyPlan := range copyPlans {
		if before, needsBarrier := encoder.stateTracker.transitionTexture(dstTex, copyPlan.subresource, afterState); needsBarrier {
			encoder.emitStateBarrierPlans([]stateBarrierPlan{{resource: dstTex, subresource: copyPlan.subresource, before: before, after: afterState}})
		}
	}

	// End encoding
	cmdBuffer, err := encoder.EndEncoding()
	if err != nil {
		return fmt.Errorf("dx12: WriteTexture: EndEncoding failed: %w", err)
	}
	owner.commandBuffer = cmdBuffer.(*CommandBuffer)

	// Submit and wait for GPU completion.
	submission, err := q.submitLocked([]hal.CommandBuffer{cmdBuffer})
	if err != nil {
		q.retainOneShot(owner, 0)
		owner = nil
		return fmt.Errorf("dx12: WriteTexture: Submit failed: %w", err)
	}
	// Block until GPU finishes the copy — staging buffer must remain valid.
	if err := q.device.waitForGPU(); err != nil {
		q.retainOneShot(owner, submission)
		owner = nil
		return fmt.Errorf("dx12: WriteTexture: WaitIdle failed: %w", err)
	}
	q.state.releaseAllOwnedLocked()

	return nil
}

// Present presents a surface texture to the screen.
// The texture must have been acquired via Surface.AcquireTexture.
//
// damageRects is an optional list of rectangles (physical pixels, top-left
// origin) indicating which surface regions changed this frame. When non-empty
// and the surface was configured with EnableDamagePresent (FLIP_SEQUENTIAL),
// IDXGISwapChain1::Present1 is called with DXGI_PRESENT_PARAMETERS containing
// the dirty rects. Otherwise, the standard Present() path is used.
func (q *Queue) Present(surface hal.Surface, _ hal.SurfaceTexture, damageRects []image.Rectangle) error {
	if err := q.lockOpen(); err != nil {
		return err
	}
	defer q.state.submitMu.Unlock()

	dx12Surface, ok := surface.(*Surface)
	if !ok {
		return fmt.Errorf("dx12: surface is not a DX12 surface")
	}

	if dx12Surface.swapchain == nil {
		return fmt.Errorf("dx12: surface not configured")
	}

	// Note: Resource barriers (render target -> present) should be
	// handled in the command buffer encoding before this call.
	// The present call here just flips the swapchain.

	// Determine sync interval and flags based on present mode
	var syncInterval uint32
	var presentFlags uint32

	switch dx12Surface.presentMode {
	case hal.PresentModeImmediate:
		// No vsync, immediate presentation
		syncInterval = 0
		if dx12Surface.allowTearing {
			presentFlags |= uint32(dxgi.DXGI_PRESENT_ALLOW_TEARING)
		}
	case hal.PresentModeMailbox:
		// VSync with triple buffering (latest frame wins)
		// Mailbox is simulated with syncInterval=0 + triple buffer
		syncInterval = 0
		if dx12Surface.allowTearing {
			presentFlags |= uint32(dxgi.DXGI_PRESENT_ALLOW_TEARING)
		}
	case hal.PresentModeFifo, hal.PresentModeFifoRelaxed:
		// VSync enabled
		syncInterval = 1
	default:
		// Default to vsync
		syncInterval = 1
	}

	// Present the frame. Use Present1 with dirty rects when the surface
	// is configured for damage-aware present (FLIP_SEQUENTIAL) and the
	// caller provided damage rects. Otherwise use standard Present.
	presentStart := time.Now()

	if len(damageRects) > 0 && dx12Surface.damagePresent {
		// Convert image.Rectangle to DXGI RECT (top-left origin, same
		// coordinate system). Stack-allocate up to 8 to avoid heap alloc.
		var stackRects [8]dxgi.RECT
		rects := stackRects[:0]
		for _, r := range damageRects {
			rects = append(rects, dxgi.RECT{
				Left:   int32(r.Min.X),
				Top:    int32(r.Min.Y),
				Right:  int32(r.Max.X),
				Bottom: int32(r.Max.Y),
			})
		}
		params := dxgi.DXGI_PRESENT_PARAMETERS{
			DirtyRectsCount: uint32(len(rects)),
			DirtyRects:      &rects[0],
		}
		if err := dx12Surface.swapchain.Present1(syncInterval, presentFlags, &params); err != nil {
			return fmt.Errorf("dx12: Present1 failed: %w", err)
		}
	} else {
		if err := dx12Surface.swapchain.Present(syncInterval, presentFlags); err != nil {
			return fmt.Errorf("dx12: Present failed: %w", err)
		}
	}

	hal.Logger().Debug("dx12: present",
		"syncInterval", syncInterval,
		"damageRects", len(damageRects),
		"elapsed", time.Since(presentStart),
	)

	// Advance frame index.
	q.device.advanceFrame()

	// Drain debug messages after present.
	q.device.DrainDebugMessages()

	return nil
}

// GetTimestampPeriod returns the timestamp period in nanoseconds.
// Used to convert timestamp query results to real time.
func (q *Queue) GetTimestampPeriod() float32 {
	if err := q.lockOpen(); err != nil {
		return 1.0
	}
	defer q.state.submitMu.Unlock()

	freq, err := q.raw.GetTimestampFrequency()
	if err != nil || freq == 0 {
		// Default to 1.0 if we can't get the frequency
		return 1.0
	}

	// Convert frequency (Hz) to period (nanoseconds)
	// period = 1 / frequency (in seconds) = 1e9 / frequency (in nanoseconds)
	return float32(1e9) / float32(freq)
}

// SupportsCommandBufferCopies returns true for DX12.
// DX12 uses command lists for copy operations — PendingWrites batches them.
func (q *Queue) SupportsCommandBufferCopies() bool {
	return true
}

// SetSwapchainSuppressed is a no-op on DX12.
// DX12 does not use swapchain semaphores — presentation synchronization is
// handled by DXGI fence signaling, which is not affected by submit ordering.
// See BUG-WGPU-VK-005 (Vulkan-specific issue).
func (q *Queue) SetSwapchainSuppressed(_ bool) {}

// -----------------------------------------------------------------------------
// Compile-time interface assertions
// -----------------------------------------------------------------------------

var _ hal.Queue = (*Queue)(nil)
