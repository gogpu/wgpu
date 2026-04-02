package wgpu

import (
	"fmt"
	"sync"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// pendingWrites accumulates WriteBuffer operations into a
// single HAL command encoder. The accumulated commands are flushed as a single
// batch when the user calls Queue.Submit(), prepended before user command
// buffers. This eliminates per-write GPU submits and matches Rust wgpu-core's
// PendingWrites architecture.
//
// Encoder lifecycle (matching Rust wgpu-core CommandAllocator):
// Encoders are acquired from the encoder pool and returned after GPU completion.
// While one encoder records new copy commands, the previous one may be in-flight
// on the GPU. When the GPU completes, the inflight encoder is reset via ResetAll
// and returned to the pool. This avoids creating new DX12 command allocators or
// Vulkan command pools every frame, enabling maxFramesInFlight=2 on DX12.
//
// For GLES and Software backends (which use direct API calls, not command
// encoders for writes), pendingWrites delegates directly to the HAL queue.
type pendingWrites struct {
	mu sync.Mutex

	// halDevice is used to create staging buffers.
	halDevice hal.Device

	// halQueue is the underlying HAL queue for direct-write backends.
	halQueue hal.Queue

	// pool manages reusable command encoders. nil for non-batching backends.
	pool *encoderPool

	// encoder is the shared command encoder for recording copy commands.
	// nil until the first write in a batch. Acquired from pool by activate().
	encoder hal.CommandEncoder

	// isRecording is true when encoder has had BeginEncoding called.
	isRecording bool

	// staging holds staging buffers that must remain alive until the GPU
	// completes the submission that references them. Moved to inflight on flush.
	staging []hal.Buffer

	// dstBuffers tracks buffers that have pending writes, keyed by HAL buffer
	// with their creation usage flags as values. Usage is passed from the wgpu
	// public API level (matching Rust wgpu-core where usage lives on core::Buffer,
	// not on hal::Buffer). Used by flush() to compute COPY_DST → read-state barriers.
	dstBuffers map[hal.Buffer]gputypes.BufferUsage

	// dstTextures tracks textures that have pending writes.
	dstTextures map[hal.Texture]struct{}

	// inflight tracks staging buffers and encoders from previous submissions,
	// keyed by submission index. Cleaned up when PollCompleted advances past them.
	inflight []inflightSubmission

	// usesBatching is true for DX12/Vulkan/Metal (command-buffer-based
	// copy backends). false for GLES/Software (direct API writes).
	// Set once at creation, never changes.
	usesBatching bool

	// deferredBindGroups accumulates HAL BindGroups whose public Release()
	// was called but whose descriptor heap slots must not be freed until
	// the GPU completes any submission that may reference them.
	// Moved to inflightSubmission on the next Submit (BUG-DX12-007).
	deferredBindGroups []hal.BindGroup

	// deferredTextureViews accumulates HAL TextureViews whose public Release()
	// was called but whose descriptor heap slots must not be freed until
	// the GPU completes any submission that may reference them (BUG-DX12-007).
	deferredTextureViews []hal.TextureView
}

// inflightSubmission tracks resources from a single submission that must
// remain alive until the GPU completes that submission.
type inflightSubmission struct {
	submissionIndex uint64
	staging         []hal.Buffer
	cmdBuf          hal.CommandBuffer  // pending writes command buffer (consumed by ResetAll)
	encoder         hal.CommandEncoder // encoder to reset+release after completion (nil if not pooled)
	// dstTextures and dstBuffers hold references to destination resources used by
	// CopyBufferToTexture/CopyBufferToBuffer in this submission. This prevents
	// premature Release() from destroying D3D12 resources while the GPU is still
	// executing commands that reference them. Root cause of DX12 TDR (BUG-DX12-006).
	dstTextures []hal.Texture
	dstBuffers  []hal.Buffer
	// deferredBindGroups and deferredTextureViews hold HAL resources whose public
	// Release() was called during this submission's lifetime, but whose descriptor
	// heap slots must not be freed until the GPU completes the submission.
	// This prevents descriptor use-after-free — the ROOT CAUSE of DX12 TDR with
	// maxFramesInFlight=2 (BUG-DX12-007). Matches Rust wgpu's LifetimeTracker
	// pattern where BindGroup/TextureView Drop only fires after triage_submissions.
	deferredBindGroups   []hal.BindGroup
	deferredTextureViews []hal.TextureView
}

// newPendingWrites creates a pendingWrites for the given HAL device and queue.
func newPendingWrites(halDevice hal.Device, halQueue hal.Queue) *pendingWrites {
	pw := &pendingWrites{
		halDevice:    halDevice,
		halQueue:     halQueue,
		dstBuffers:   make(map[hal.Buffer]gputypes.BufferUsage),
		dstTextures:  make(map[hal.Texture]struct{}),
		usesBatching: halQueue.SupportsCommandBufferCopies(),
	}
	if pw.usesBatching {
		pw.pool = newEncoderPool(halDevice)
	}
	return pw
}

// writeBuffer records a buffer write. For batching backends, creates a staging
// buffer, copies data via CPU, and records CopyBufferToBuffer into the shared
// encoder. For direct-write backends, delegates to halQueue.WriteBuffer.
//
// usage is the buffer's WebGPU creation usage flags, passed from the wgpu public
// API level (wgpu.Buffer → Queue.WriteBuffer → here). This matches Rust wgpu-core
// where usage lives on core::Buffer, not hal::Buffer. Used by flush() to compute
// correct COPY_DST → read-state transition barriers (BUG-DX12-010).
func (pw *pendingWrites) writeBuffer(buffer hal.Buffer, usage gputypes.BufferUsage, offset uint64, data []byte) error {
	pw.mu.Lock()
	defer pw.mu.Unlock()

	if !pw.usesBatching {
		return pw.halQueue.WriteBuffer(buffer, offset, data)
	}

	dataLen := uint64(len(data))
	if dataLen == 0 {
		return nil
	}

	// Create staging buffer (upload heap, mapped at creation).
	stagingBuf, err := pw.halDevice.CreateBuffer(&hal.BufferDescriptor{
		Label:            "(wgpu internal) staging",
		Size:             dataLen,
		Usage:            gputypes.BufferUsageCopySrc | gputypes.BufferUsageMapWrite,
		MappedAtCreation: true,
	})
	if err != nil {
		return fmt.Errorf("wgpu: pending writes: create staging buffer: %w", err)
	}

	// CPU copy via HAL WriteBuffer on the staging buffer. Since the staging
	// buffer is always upload-heap/host-visible, this is a direct memcpy on
	// all backends (no command encoder, no submit).
	if err := pw.halQueue.WriteBuffer(stagingBuf, 0, data); err != nil {
		pw.halDevice.DestroyBuffer(stagingBuf)
		return fmt.Errorf("wgpu: pending writes: write to staging buffer: %w", err)
	}

	// Activate encoder (lazy creation + BeginEncoding).
	enc, err := pw.activate()
	if err != nil {
		pw.halDevice.DestroyBuffer(stagingBuf)
		return fmt.Errorf("wgpu: pending writes: activate encoder: %w", err)
	}

	// Record copy command.
	// NOTE: Buffer barriers (current→COPY_DST) are NOT added here because we
	// cannot know the buffer's current state without a DeviceTracker. Specifying
	// the wrong "from" state (e.g., COMMON when the buffer is actually in
	// VERTEX_AND_CONSTANT_BUFFER from the previous frame) causes immediate TDR.
	// maxFramesInFlight=1 prevents inter-frame data races instead.
	enc.CopyBufferToBuffer(stagingBuf, buffer, []hal.BufferCopy{
		{
			SrcOffset: 0,
			DstOffset: offset,
			Size:      dataLen,
		},
	})

	// Track resources.
	pw.staging = append(pw.staging, stagingBuf)
	pw.dstBuffers[buffer] = usage

	return nil
}

// pendingWritesRowPitchAlignment is the row pitch alignment used for staging
// buffer-to-texture copies. 256 bytes is safe for all batching backends:
// DX12 requires D3D12_TEXTURE_DATA_PITCH_ALIGNMENT (256), Vulkan benefits
// from optimalBufferCopyRowPitchAlignment (typically 256), and Metal has no
// requirement but 256 is recommended.
const pendingWritesRowPitchAlignment = 256

// writeTexture records a texture write. Creates a staging buffer with proper
// row pitch alignment, copies data via CPU, and records CopyBufferToTexture
// into the shared encoder with correct resource barriers.
//
// Barrier strategy (matching Rust wgpu-core queue.rs:935-947):
// - Before copy: transition texture from CurrentUsage() → CopyDst
// - After copy: transition texture CopyDst → TextureBinding (SHADER_RESOURCE)
//
// CurrentUsage() returns the DX12 texture's tracked currentState mapped to
// gputypes.TextureUsage. For newly created textures this is 0 (COMMON),
// for previously-written textures this is TextureBinding (SHADER_RESOURCE).
// On non-DX12 backends, CurrentUsage() returns 0 and TransitionTextures is a no-op.
func (pw *pendingWrites) writeTexture(
	dst *hal.ImageCopyTexture,
	data []byte,
	layout *hal.ImageDataLayout,
	size *hal.Extent3D,
) error {
	pw.mu.Lock()
	defer pw.mu.Unlock()

	if !pw.usesBatching {
		return pw.halQueue.WriteTexture(dst, data, layout, size)
	}

	if len(data) == 0 {
		return nil
	}

	// Calculate aligned row pitch.
	bytesPerRow := layout.BytesPerRow
	if bytesPerRow == 0 {
		bytesPerRow = uint32(len(data)) //nolint:gosec // data length fits uint32 for textures
	}

	alignedBytesPerRow := alignUp(bytesPerRow, pendingWritesRowPitchAlignment)

	rowsPerImage := layout.RowsPerImage
	if rowsPerImage == 0 {
		rowsPerImage = size.Height
	}

	depthOrLayers := size.DepthOrArrayLayers
	if depthOrLayers == 0 {
		depthOrLayers = 1
	}

	stagingSize := uint64(alignedBytesPerRow) * uint64(rowsPerImage) * uint64(depthOrLayers)

	// Create staging buffer.
	stagingBuf, err := pw.halDevice.CreateBuffer(&hal.BufferDescriptor{
		Label:            "(wgpu internal) staging texture",
		Size:             stagingSize,
		Usage:            gputypes.BufferUsageCopySrc | gputypes.BufferUsageMapWrite,
		MappedAtCreation: true,
	})
	if err != nil {
		return fmt.Errorf("wgpu: pending writes: create texture staging buffer: %w", err)
	}

	// CPU copy with row pitch alignment.
	stagingData := copyTextureDataAligned(data, layout.Offset, bytesPerRow, alignedBytesPerRow, rowsPerImage, depthOrLayers, stagingSize)

	if err := pw.halQueue.WriteBuffer(stagingBuf, 0, stagingData); err != nil {
		pw.halDevice.DestroyBuffer(stagingBuf)
		return fmt.Errorf("wgpu: pending writes: write to texture staging buffer: %w", err)
	}

	// Activate encoder.
	enc, err := pw.activate()
	if err != nil {
		pw.halDevice.DestroyBuffer(stagingBuf)
		return fmt.Errorf("wgpu: pending writes: activate encoder: %w", err)
	}

	// Transition texture to COPY_DST using its actual tracked state.
	currentUsage := dst.Texture.CurrentUsage()
	if currentUsage != gputypes.TextureUsageCopyDst {
		enc.TransitionTextures([]hal.TextureBarrier{
			{
				Texture: dst.Texture,
				Range: hal.TextureRange{
					Aspect:          gputypes.TextureAspectAll,
					BaseMipLevel:    dst.MipLevel,
					MipLevelCount:   1,
					BaseArrayLayer:  0,
					ArrayLayerCount: 1,
				},
				Usage: hal.TextureUsageTransition{
					OldUsage: currentUsage,
					NewUsage: gputypes.TextureUsageCopyDst,
				},
			},
		})
	}

	// Record copy command.
	enc.CopyBufferToTexture(stagingBuf, dst.Texture, []hal.BufferTextureCopy{
		{
			BufferLayout: hal.ImageDataLayout{
				Offset:       0,
				BytesPerRow:  alignedBytesPerRow,
				RowsPerImage: rowsPerImage,
			},
			TextureBase: *dst,
			Size:        *size,
		},
	})

	// Transition texture to SHADER_RESOURCE for rendering.
	// Unlike Rust wgpu-core (which defers this to submit-time via DeviceTextureTracker),
	// we do it eagerly because we lack a centralized tracker. This is correct but slightly
	// suboptimal — an extra barrier if the next usage is also COPY_DST.
	enc.TransitionTextures([]hal.TextureBarrier{
		{
			Texture: dst.Texture,
			Range: hal.TextureRange{
				Aspect:          gputypes.TextureAspectAll,
				BaseMipLevel:    dst.MipLevel,
				MipLevelCount:   1,
				BaseArrayLayer:  0,
				ArrayLayerCount: 1,
			},
			Usage: hal.TextureUsageTransition{
				OldUsage: gputypes.TextureUsageCopyDst,
				NewUsage: gputypes.TextureUsageTextureBinding,
			},
		},
	})

	// Track resources. AddPendingRef prevents premature Destroy (BUG-DX12-006).
	pw.staging = append(pw.staging, stagingBuf)
	if _, already := pw.dstTextures[dst.Texture]; !already {
		dst.Texture.AddPendingRef()
	}
	pw.dstTextures[dst.Texture] = struct{}{}

	return nil
}

// copyTextureDataAligned copies texture data with row pitch alignment padding.
// If no padding is needed (alignedBytesPerRow == bytesPerRow), returns data directly.
func copyTextureDataAligned(data []byte, srcOffset uint64, bytesPerRow, alignedBytesPerRow, rowsPerImage, depthOrLayers uint32, stagingSize uint64) []byte {
	if alignedBytesPerRow == bytesPerRow {
		return data
	}
	aligned := make([]byte, stagingSize)
	for layer := uint32(0); layer < depthOrLayers; layer++ {
		for row := uint32(0); row < rowsPerImage; row++ {
			dstRowStart := uint64(layer)*uint64(rowsPerImage)*uint64(alignedBytesPerRow) +
				uint64(row)*uint64(alignedBytesPerRow)
			srcRowStart := srcOffset + uint64(layer)*uint64(rowsPerImage)*uint64(bytesPerRow) +
				uint64(row)*uint64(bytesPerRow)
			if srcRowStart+uint64(bytesPerRow) > uint64(len(data)) {
				break
			}
			copy(aligned[dstRowStart:dstRowStart+uint64(bytesPerRow)],
				data[srcRowStart:srcRowStart+uint64(bytesPerRow)])
		}
	}
	return aligned
}

// alignUp rounds n up to the nearest multiple of alignment.
func alignUp(n, alignment uint32) uint32 {
	if alignment == 0 {
		return n
	}
	return (n + alignment - 1) / alignment * alignment
}

// bufferReadUsage extracts the read-state usage from a buffer's creation flags.
// Strips write/transfer usages (CopyDst, CopySrc, MapWrite, MapRead, Storage)
// to get the usage the buffer will be in during render (VERTEX, INDEX, UNIFORM).
// Returns 0 if no read usage is set (buffer only used for copies/storage).
func bufferReadUsage(usage gputypes.BufferUsage) gputypes.BufferUsage {
	// Keep only read-state flags relevant for DX12 transition barriers.
	// These map to DX12 read states: VERTEX_AND_CONSTANT_BUFFER, INDEX_BUFFER,
	// INDIRECT_ARGUMENT. Combined read states are valid in DX12.
	readMask := gputypes.BufferUsageVertex | gputypes.BufferUsageIndex |
		gputypes.BufferUsageUniform | gputypes.BufferUsageIndirect
	return usage & readMask
}

// activate lazily begins encoding on the shared command encoder.
// Acquires an encoder from the pool if none exists. Must be called with pw.mu held.
func (pw *pendingWrites) activate() (hal.CommandEncoder, error) {
	if pw.isRecording {
		return pw.encoder, nil
	}

	if pw.encoder == nil {
		enc, err := pw.pool.acquire()
		if err != nil {
			return nil, fmt.Errorf("acquire encoder from pool: %w", err)
		}
		pw.encoder = enc
	}

	if err := pw.encoder.BeginEncoding("(wgpu internal) pending writes"); err != nil {
		return nil, fmt.Errorf("begin encoding: %w", err)
	}

	pw.isRecording = true
	return pw.encoder, nil
}

// flush closes the pending command encoder and returns a command buffer to
// prepend before user command buffers. Returns nil if no writes were recorded.
// The encoder is detached for inflight tracking — after GPU completion,
// maintain() calls ResetAll and returns it to the pool.
// Must be called with pw.mu held.
func (pw *pendingWrites) flush() (hal.CommandBuffer, hal.CommandEncoder, []hal.Buffer, []hal.Texture, []hal.Buffer, error) {
	if !pw.isRecording {
		return nil, nil, nil, nil, nil, nil
	}

	// Transition destination buffers from COPY_DEST to their primary read usage
	// before closing the encoder. Without this barrier, buffers remain in COPY_DEST
	// state when the render pass tries to use them as VERTEX/INDEX/UNIFORM —
	// undefined behavior per DX12 spec (BUG-DX12-010).
	//
	// Microsoft docs: "if promoted from COMMON to COPY_DEST, a barrier is still
	// required to transition from COPY_DEST to RENDER_TARGET."
	//
	// Usage comes from the wgpu public API level (core.Buffer.usage), matching
	// Rust wgpu-core where DeviceTracker computes barriers using core::Buffer.usage.
	// We extract read-only flags (VERTEX|INDEX|UNIFORM|INDIRECT) from the creation
	// usage to determine the target state.
	if len(pw.dstBuffers) > 0 {
		barriers := make([]hal.BufferBarrier, 0, len(pw.dstBuffers))
		for buf, usage := range pw.dstBuffers {
			readUsage := bufferReadUsage(usage)
			if readUsage != 0 {
				barriers = append(barriers, hal.BufferBarrier{
					Buffer: buf,
					Usage: hal.BufferUsageTransition{
						OldUsage: gputypes.BufferUsageCopyDst,
						NewUsage: readUsage,
					},
				})
			}
		}
		if len(barriers) > 0 {
			pw.encoder.TransitionBuffers(barriers)
		}
	}

	cmdBuf, err := pw.encoder.EndEncoding()
	if err != nil {
		pw.encoder.DiscardEncoding()
		// Clean up staging buffers — they won't be used.
		for _, buf := range pw.staging {
			pw.halDevice.DestroyBuffer(buf)
		}
		pw.staging = nil
		pw.isRecording = false
		pw.encoder = nil
		pw.dstBuffers = make(map[hal.Buffer]gputypes.BufferUsage)
		pw.dstTextures = make(map[hal.Texture]struct{})
		return nil, nil, nil, nil, nil, fmt.Errorf("wgpu: pending writes: end encoding: %w", err)
	}

	// Move resources out for inflight tracking.
	// dstTextures/dstBuffers hold references to prevent premature Release (BUG-DX12-006).
	flushedStaging := pw.staging
	flushedEncoder := pw.encoder
	var flushedDstTextures []hal.Texture
	for tex := range pw.dstTextures {
		flushedDstTextures = append(flushedDstTextures, tex)
	}
	var flushedDstBuffers []hal.Buffer
	for buf := range pw.dstBuffers {
		flushedDstBuffers = append(flushedDstBuffers, buf)
	}
	pw.staging = nil
	pw.isRecording = false
	pw.encoder = nil
	pw.dstBuffers = make(map[hal.Buffer]gputypes.BufferUsage)
	pw.dstTextures = make(map[hal.Texture]struct{})

	return cmdBuf, flushedEncoder, flushedStaging, flushedDstTextures, flushedDstBuffers, nil
}

// maintain frees staging buffers and returns encoders to the pool from
// completed submissions. Must be called with pw.mu held.
func (pw *pendingWrites) maintain(completedIndex uint64) {
	// Find the cutoff point — all submissions with index <= completedIndex are done.
	cutoff := 0
	for i, sub := range pw.inflight {
		if sub.submissionIndex > completedIndex {
			break
		}
		cutoff = i + 1
		// Destroy staging buffers from completed submission.
		for _, buf := range sub.staging {
			pw.halDevice.DestroyBuffer(buf)
		}
		// Release pending refs on destination textures/buffers (BUG-DX12-006).
		// This allows deferred Destroy to proceed now that GPU is done.
		for _, tex := range sub.dstTextures {
			tex.DecPendingRef()
		}
		// Destroy deferred BindGroups whose descriptor heap slots are now safe
		// to free — GPU has completed this submission (BUG-DX12-007).
		for _, bg := range sub.deferredBindGroups {
			pw.halDevice.DestroyBindGroup(bg)
		}
		// Destroy deferred TextureViews (BUG-DX12-007).
		for _, tv := range sub.deferredTextureViews {
			pw.halDevice.DestroyTextureView(tv)
		}
		// Reset the encoder and return it to the pool.
		if sub.encoder != nil && sub.cmdBuf != nil {
			sub.encoder.ResetAll([]hal.CommandBuffer{sub.cmdBuf})
			pw.pool.release(sub.encoder)
		} else if sub.cmdBuf != nil {
			pw.halDevice.FreeCommandBuffer(sub.cmdBuf)
		}
	}

	if cutoff > 0 {
		pw.inflight = pw.inflight[cutoff:]
	}
}

// deferBindGroupDestroy adds a HAL BindGroup to the deferred destruction list.
// The BindGroup's descriptor heap slots will be freed only after the GPU completes
// the submission that is active at the time of the next Queue.Submit call.
// Must be called instead of halDevice.DestroyBindGroup for any BindGroup that
// may have been referenced by GPU commands (BUG-DX12-007).
func (pw *pendingWrites) deferBindGroupDestroy(bg hal.BindGroup) {
	pw.mu.Lock()
	pw.deferredBindGroups = append(pw.deferredBindGroups, bg)
	pw.mu.Unlock()
}

// deferTextureViewDestroy adds a HAL TextureView to the deferred destruction list.
// Same pattern as deferBindGroupDestroy (BUG-DX12-007).
func (pw *pendingWrites) deferTextureViewDestroy(tv hal.TextureView) {
	pw.mu.Lock()
	pw.deferredTextureViews = append(pw.deferredTextureViews, tv)
	pw.mu.Unlock()
}

// drainDeferred moves accumulated deferred resources out for inflight tracking.
// Must be called with pw.mu held.
func (pw *pendingWrites) drainDeferred() ([]hal.BindGroup, []hal.TextureView) {
	bg := pw.deferredBindGroups
	tv := pw.deferredTextureViews
	pw.deferredBindGroups = nil
	pw.deferredTextureViews = nil
	return bg, tv
}

// HasPendingWork returns true if there are buffered writes waiting to be flushed.
func (pw *pendingWrites) HasPendingWork() bool {
	pw.mu.Lock()
	defer pw.mu.Unlock()
	return pw.isRecording
}

// destroy releases all resources held by pendingWrites.
func (pw *pendingWrites) destroy() {
	pw.mu.Lock()
	defer pw.mu.Unlock()

	// Discard any in-progress encoding.
	if pw.isRecording && pw.encoder != nil {
		pw.encoder.DiscardEncoding()
	}
	// Destroy the current encoder if it wasn't flushed.
	if pw.encoder != nil {
		pw.encoder.Destroy()
	}
	pw.isRecording = false
	pw.encoder = nil

	// Destroy pending staging buffers.
	for _, buf := range pw.staging {
		pw.halDevice.DestroyBuffer(buf)
	}
	pw.staging = nil

	// Destroy inflight staging buffers, command buffers, encoders, and deferred resources.
	for _, sub := range pw.inflight {
		for _, buf := range sub.staging {
			pw.halDevice.DestroyBuffer(buf)
		}
		for _, bg := range sub.deferredBindGroups {
			pw.halDevice.DestroyBindGroup(bg)
		}
		for _, tv := range sub.deferredTextureViews {
			pw.halDevice.DestroyTextureView(tv)
		}
		if sub.encoder != nil {
			// Encoder owns the command buffer's resources. Destroy releases both.
			sub.encoder.Destroy()
		} else if sub.cmdBuf != nil {
			pw.halDevice.FreeCommandBuffer(sub.cmdBuf)
		}
	}
	pw.inflight = nil

	// Destroy any deferred resources that were never submitted.
	for _, bg := range pw.deferredBindGroups {
		pw.halDevice.DestroyBindGroup(bg)
	}
	pw.deferredBindGroups = nil
	for _, tv := range pw.deferredTextureViews {
		pw.halDevice.DestroyTextureView(tv)
	}
	pw.deferredTextureViews = nil

	// Destroy the encoder pool (releases all pooled encoders).
	if pw.pool != nil {
		pw.pool.destroy()
	}

	pw.dstBuffers = nil
	pw.dstTextures = nil
}
