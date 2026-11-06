//go:build !rust && !(js && wasm)

package wgpu

import (
	"bytes"
	"strings"
	"testing"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/core"
	"github.com/gogpu/wgpu/core/track"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/noop"
)

// --- Mock types for batching backend tests ---

// mockBatchingQueue wraps noop.Queue but returns true for SupportsCommandBufferCopies.
type mockBatchingQueue struct {
	noop.Queue
	writeBufferCalls  int
	writeTextureCalls int
	submitCalls       int
	submitErr         error
	lastWriteBuf      hal.Buffer
	lastWriteOffset   uint64
	lastWriteData     []byte
}

type barrierCaptureEncoder struct {
	noop.CommandEncoder
	textureBarriers []hal.TextureBarrier
	bufferBarriers  []hal.BufferBarrier
	// events are "tex-barrier", "buf-barrier", "copy", "discard" and "reset"
	// in recording order.
	events []string
}

func (e *barrierCaptureEncoder) DiscardEncoding() {
	e.events = append(e.events, "discard")
}

func (e *barrierCaptureEncoder) ResetAll(commandBuffers []hal.CommandBuffer) {
	e.CommandEncoder.ResetAll(commandBuffers)
	e.events = append(e.events, "reset")
}

func (e *barrierCaptureEncoder) TransitionTextures(barriers []hal.TextureBarrier) {
	e.textureBarriers = append(e.textureBarriers, barriers...)
	e.events = append(e.events, "tex-barrier")
}

func (e *barrierCaptureEncoder) TransitionBuffers(barriers []hal.BufferBarrier) {
	e.bufferBarriers = append(e.bufferBarriers, barriers...)
	e.events = append(e.events, "buf-barrier")
}

func (e *barrierCaptureEncoder) CopyBufferToBuffer(_, _ hal.Buffer, _ []hal.BufferCopy) {
	e.events = append(e.events, "copy")
}

func (e *barrierCaptureEncoder) CopyBufferToTexture(_ hal.Buffer, _ hal.Texture, _ []hal.BufferTextureCopy) {
	e.events = append(e.events, "copy")
}

type barrierCaptureDevice struct {
	noop.Device
	encoder *barrierCaptureEncoder
}

func (d *barrierCaptureDevice) CreateCommandEncoder(*hal.CommandEncoderDescriptor) (hal.CommandEncoder, error) {
	d.encoder = &barrierCaptureEncoder{}
	return d.encoder, nil
}

func (q *mockBatchingQueue) SupportsCommandBufferCopies() bool { return true }

func (q *mockBatchingQueue) Submit(commandBuffers []hal.CommandBuffer) (uint64, error) {
	q.submitCalls++
	if q.submitErr != nil {
		return 0, q.submitErr
	}
	return q.Queue.Submit(commandBuffers)
}

func (q *mockBatchingQueue) WriteBuffer(buffer hal.Buffer, offset uint64, data []byte) error {
	q.writeBufferCalls++
	q.lastWriteBuf = buffer
	q.lastWriteOffset = offset
	q.lastWriteData = make([]byte, len(data))
	copy(q.lastWriteData, data)
	return q.Queue.WriteBuffer(buffer, offset, data)
}

func (q *mockBatchingQueue) WriteTexture(dst *hal.ImageCopyTexture, data []byte, layout *hal.ImageDataLayout, size *hal.Extent3D) error {
	q.writeTextureCalls++
	return q.Queue.WriteTexture(dst, data, layout, size)
}

// mockStagingSizeDevice wraps noop.Device and implements hal.MaxStagingBufferSizer.
// Used to verify that newPendingWrites queries the device for the max staging buffer size.
type mockStagingSizeDevice struct {
	noop.Device
	maxStaging uint64
}

func (d *mockStagingSizeDevice) MaxStagingBufferSize() uint64 {
	return d.maxStaging
}

// mockNonBatchingQueue wraps noop.Queue and tracks direct calls.
type mockNonBatchingQueue struct {
	noop.Queue
	writeBufferCalls  int
	writeTextureCalls int
	lastWriteBuf      hal.Buffer
	lastWriteOffset   uint64
	lastWriteData     []byte
}

func (q *mockNonBatchingQueue) SupportsCommandBufferCopies() bool { return false }

func (q *mockNonBatchingQueue) WriteBuffer(buffer hal.Buffer, offset uint64, data []byte) error {
	q.writeBufferCalls++
	q.lastWriteBuf = buffer
	q.lastWriteOffset = offset
	q.lastWriteData = make([]byte, len(data))
	copy(q.lastWriteData, data)
	return q.Queue.WriteBuffer(buffer, offset, data)
}

func (q *mockNonBatchingQueue) WriteTexture(dst *hal.ImageCopyTexture, data []byte, layout *hal.ImageDataLayout, size *hal.Extent3D) error {
	q.writeTextureCalls++
	return q.Queue.WriteTexture(dst, data, layout, size)
}

// --- Helpers ---

// createBatchingPW creates a pendingWrites with a batching mock queue and shared pool.
func createBatchingPW(t *testing.T) (*pendingWrites, *noop.Device, *mockBatchingQueue) {
	t.Helper()
	dev := &noop.Device{}
	q := &mockBatchingQueue{}
	pool := newEncoderPool(dev)
	pw := newPendingWrites(dev, q, pool, nil)
	t.Cleanup(func() { pool.destroy() })
	return pw, dev, q
}

// createNonBatchingPW creates a pendingWrites with a non-batching mock queue.
func createNonBatchingPW(t *testing.T) (*pendingWrites, *noop.Device, *mockNonBatchingQueue) {
	t.Helper()
	dev := &noop.Device{}
	q := &mockNonBatchingQueue{}
	pw := newPendingWrites(dev, q, nil, nil)
	return pw, dev, q
}

// flushAndDiscard calls flush and discards all return values except error.
func flushAndDiscard(pw *pendingWrites) error {
	_, _, _, _, err := pw.flush() //nolint:dogsled // test helper discards all but error
	return err
}

// --- Tests ---

func TestPendingWrites_NewNonBatching(t *testing.T) {
	pw, _, _ := createNonBatchingPW(t)
	defer pw.destroy()

	if pw.usesBatching {
		t.Error("expected usesBatching=false for non-batching queue")
	}
	if pw.pool != nil {
		t.Error("expected pool=nil for non-batching queue")
	}
	if pw.dstBuffers == nil {
		t.Error("expected dstBuffers map to be initialized")
	}
	if pw.dstTextures == nil {
		t.Error("expected dstTextures map to be initialized")
	}
}

func TestPendingWrites_NewBatching(t *testing.T) {
	pw, _, _ := createBatchingPW(t)
	defer pw.destroy()

	if !pw.usesBatching {
		t.Error("expected usesBatching=true for batching queue")
	}
	if pw.pool == nil {
		t.Error("expected pool to be created for batching queue")
	}
	if pw.belt == nil {
		t.Error("expected belt to be created for batching queue")
	}
}

func TestPendingWrites_WriteBufferNonBatching(t *testing.T) {
	pw, dev, q := createNonBatchingPW(t)
	defer pw.destroy()

	buf, err := dev.CreateBuffer(&hal.BufferDescriptor{
		Label:            "test",
		Size:             64,
		Usage:            gputypes.BufferUsageCopyDst | gputypes.BufferUsageVertex,
		MappedAtCreation: true,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}

	data := []byte{1, 2, 3, 4}
	if err := pw.writeBufferFor(nil, buf, track.InvalidTrackerIndex, 8, data); err != nil {
		t.Fatalf("writeBuffer: %v", err)
	}

	// Should delegate directly to halQueue.WriteBuffer.
	if q.writeBufferCalls != 1 {
		t.Errorf("expected 1 WriteBuffer call on queue, got %d", q.writeBufferCalls)
	}
	if q.lastWriteBuf != buf {
		t.Error("expected WriteBuffer called with the original buffer")
	}
	if q.lastWriteOffset != 8 {
		t.Errorf("expected offset=8, got %d", q.lastWriteOffset)
	}

	// Non-batching should NOT create staging or track resources.
	if len(pw.staging) != 0 {
		t.Errorf("expected no staging buffers, got %d", len(pw.staging))
	}
	if pw.isRecording {
		t.Error("expected isRecording=false for non-batching")
	}
}

func TestPendingWrites_WriteBufferBatching(t *testing.T) {
	pw, _, q := createBatchingPW(t)
	defer pw.destroy()

	// Create a destination buffer (non-mapped).
	dstBuf := &noop.Resource{}

	data := []byte{10, 20, 30, 40, 50}
	if err := pw.writeBufferFor(nil, dstBuf, track.InvalidTrackerIndex, 16, data); err != nil {
		t.Fatalf("writeBuffer: %v", err)
	}

	// Queue.WriteBuffer is called to copy data into the staging belt chunk.
	if q.writeBufferCalls != 1 {
		t.Errorf("expected 1 WriteBuffer call (staging copy), got %d", q.writeBufferCalls)
	}

	// Belt should have an active chunk (staging is managed by belt, not pw.staging).
	s := pw.belt.stats()
	activeChunks := s.ActiveChunks
	if activeChunks != 1 {
		t.Errorf("expected 1 active belt chunk, got %d", activeChunks)
	}

	// Encoder should be active.
	if !pw.isRecording {
		t.Error("expected isRecording=true after batching write")
	}
	if pw.encoder == nil {
		t.Error("expected encoder to be non-nil")
	}

	// dstBuffers keeps the destination alive until the submission completes.
	pw.mu.Lock()
	_, ok := pw.dstBuffers[dstBuf]
	pw.mu.Unlock()
	if !ok {
		t.Error("expected destination buffer to be tracked in dstBuffers")
	}
}

func TestPendingWrites_WriteBufferEmptyData(t *testing.T) {
	pw, _, q := createBatchingPW(t)
	defer pw.destroy()

	dstBuf := &noop.Resource{}
	// Empty data should be a no-op for batching.
	if err := pw.writeBufferFor(nil, dstBuf, track.InvalidTrackerIndex, 0, []byte{}); err != nil {
		t.Fatalf("writeBuffer empty: %v", err)
	}

	if q.writeBufferCalls != 0 {
		t.Errorf("expected 0 WriteBuffer calls for empty data, got %d", q.writeBufferCalls)
	}
	if pw.isRecording {
		t.Error("expected isRecording=false after empty write")
	}
}

func TestPendingWrites_WriteTextureNonBatching(t *testing.T) {
	pw, _, q := createNonBatchingPW(t)
	defer pw.destroy()

	tex := &noop.Texture{}
	dst := &hal.ImageCopyTexture{Texture: tex}
	data := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	layout := &hal.ImageDataLayout{BytesPerRow: 4, RowsPerImage: 2}
	size := &hal.Extent3D{Width: 1, Height: 2, DepthOrArrayLayers: 1}

	if err := pw.writeTexture(dst, data, layout, size); err != nil {
		t.Fatalf("writeTexture: %v", err)
	}

	if q.writeTextureCalls != 1 {
		t.Errorf("expected 1 WriteTexture call, got %d", q.writeTextureCalls)
	}
	if pw.isRecording {
		t.Error("expected isRecording=false for non-batching")
	}
}

func TestPendingWrites_WriteTextureBatching(t *testing.T) {
	pw, _, q := createBatchingPW(t)
	defer pw.destroy()

	tex := &noop.Texture{}
	dst := &hal.ImageCopyTexture{Texture: tex, MipLevel: 0}
	data := make([]byte, 128)
	for i := range data {
		data[i] = byte(i)
	}
	layout := &hal.ImageDataLayout{BytesPerRow: 64, RowsPerImage: 2}
	size := &hal.Extent3D{Width: 16, Height: 2, DepthOrArrayLayers: 1}

	if err := pw.writeTexture(dst, data, layout, size); err != nil {
		t.Fatalf("writeTexture: %v", err)
	}

	// WriteBuffer should be called for staging copy into belt chunk.
	if q.writeBufferCalls != 1 {
		t.Errorf("expected 1 WriteBuffer call (staging), got %d", q.writeBufferCalls)
	}

	// Belt should have an active chunk (staging is managed by belt, not pw.staging).
	s := pw.belt.stats()
	activeChunks := s.ActiveChunks
	if activeChunks != 1 {
		t.Errorf("expected 1 active belt chunk, got %d", activeChunks)
	}

	// Encoder active.
	if !pw.isRecording {
		t.Error("expected isRecording=true after batching texture write")
	}

	// Texture tracked.
	pw.mu.Lock()
	_, tracked := pw.dstTextures[tex]
	pw.mu.Unlock()
	if !tracked {
		t.Error("expected texture to be tracked in dstTextures")
	}
}

// trackedPendingWrites builds a batching pendingWrites wired to a real device
// tracker, with an encoder that records barriers and copies in order.
func trackedPendingWrites(t *testing.T) (*pendingWrites, *barrierCaptureDevice, *core.Device) {
	t.Helper()
	dev := &barrierCaptureDevice{}
	coreDevice := core.NewDevice(dev, &core.Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "pw-tracker")
	pool := newEncoderPool(dev)
	pw := newPendingWrites(dev, &mockBatchingQueue{}, pool, coreDevice.Tracker())
	t.Cleanup(func() {
		pw.destroy()
		pool.destroy()
	})
	return pw, dev, coreDevice
}

// writeTrackedTexture creates a 16x16x2 texture on coreDevice, leaves it in
// state prior in the device tracker and writes one texel of layer 1.
func writeTrackedTexture(t *testing.T, pw *pendingWrites, coreDevice *core.Device, usage gputypes.TextureUsage, prior track.TextureUses) (*Texture, track.TrackerIndex) {
	t.Helper()
	halTexture := &noop.Texture{}
	coreTexture := core.NewTexture(
		halTexture, coreDevice, gputypes.TextureFormatRGBA8Unorm,
		gputypes.TextureDimension2D, usage,
		gputypes.Extent3D{Width: 16, Height: 16, DepthOrArrayLayers: 2},
		1, 1, "texture",
	)
	idx := coreTexture.TrackingData().Index()
	coreDevice.Tracker().InsertTexture(idx, prior)
	texture := &Texture{hal: halTexture, device: &Device{core: coreDevice}, coreTexture: coreTexture}
	dst := &hal.ImageCopyTexture{Texture: halTexture, Origin: hal.Origin3D{Z: 1}, Aspect: gputypes.TextureAspectAll}
	size := hal.Extent3D{Width: 1, Height: 1, DepthOrArrayLayers: 1}
	layout := &hal.ImageDataLayout{BytesPerRow: 4, RowsPerImage: 1}
	if err := pw.writeTextureFor(texture, dst, make([]byte, 4), layout, &size); err != nil {
		t.Fatalf("writeTextureFor: %v", err)
	}
	return texture, idx
}

// TestPendingWrites_WriteTextureBarrierFromTrackedState is the texture half of
// P1-CORE-008: the pre-copy barrier starts from the texture's device-tracker
// state (not from a guessed UNDEFINED, which discards earlier contents), covers
// the whole texture (the tracker is whole-texture), and is recorded before the
// copy. Rust: write_texture → trackers.textures.set_single(COPY_DST).
func TestPendingWrites_WriteTextureBarrierFromTrackedState(t *testing.T) {
	pw, dev, coreDevice := trackedPendingWrites(t)
	_, idx := writeTrackedTexture(t, pw, coreDevice,
		gputypes.TextureUsageCopyDst|gputypes.TextureUsageRenderAttachment, track.TextureUsesColorTarget)

	enc := dev.encoder
	if len(enc.textureBarriers) != 1 {
		t.Fatalf("texture barriers = %d, want 1", len(enc.textureBarriers))
	}
	b := enc.textureBarriers[0]
	if b.Usage.OldUsage != gputypes.TextureUsageRenderAttachment || b.Usage.NewUsage != gputypes.TextureUsageCopyDst {
		t.Errorf("barrier = %v → %v, want RenderAttachment → CopyDst", b.Usage.OldUsage, b.Usage.NewUsage)
	}
	if b.Range != (hal.TextureRange{}) {
		t.Errorf("barrier range = %+v, want the whole texture (zero range)", b.Range)
	}
	if len(enc.events) != 2 || enc.events[0] != "tex-barrier" || enc.events[1] != "copy" {
		t.Errorf("events = %v, want [tex-barrier copy]", enc.events)
	}
	if got := coreDevice.Tracker().TextureUsage(idx); got != track.TextureUsesCopyDst {
		t.Errorf("device state after write = %d, want CopyDst", got)
	}
}

// TestPendingWrites_FlushLeavesWrittenTexturesInCopyDst: flush records no
// barrier after the writes, as Rust wgpu-core: a written texture stays in
// COPY_DST in the device tracker, and the next command buffer's submit-time
// transit moves it into the usage its first pass gives it (bind group usages
// are in the pass usage scopes, P1-CORE-010). The eager COPY_DST → RESOURCE
// barrier that used to stand in for that is gone.
func TestPendingWrites_FlushLeavesWrittenTexturesInCopyDst(t *testing.T) {
	for _, tc := range []struct {
		name      string
		usage     gputypes.TextureUsage
		wantState track.TextureUses
	}{
		{"copy-only", gputypes.TextureUsageCopyDst | gputypes.TextureUsageCopySrc, track.TextureUsesCopyDst},
		{"sampled", gputypes.TextureUsageCopyDst | gputypes.TextureUsageTextureBinding, track.TextureUsesCopyDst},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pw, dev, coreDevice := trackedPendingWrites(t)
			_, idx := writeTrackedTexture(t, pw, coreDevice, tc.usage, track.TextureUsesUninitialized)
			before := len(dev.encoder.textureBarriers)

			pw.mu.Lock()
			cmdBuf, _, _, _, err := pw.flush()
			pw.mu.Unlock()
			if err != nil || cmdBuf == nil {
				t.Fatalf("flush: cmdBuf=%v err=%v", cmdBuf, err)
			}
			if flushBarriers := dev.encoder.textureBarriers[before:]; len(flushBarriers) != 0 {
				t.Fatalf("flush texture barriers = %v, want none", flushBarriers)
			}
			if got := coreDevice.Tracker().TextureUsage(idx); got != tc.wantState {
				t.Errorf("device state after flush = %d, want %d", got, tc.wantState)
			}
		})
	}
}

// TestPendingWrites_WriteBufferBarrierBeforeCopy is the #371 regression
// (P1-CORE-008): a WriteBuffer into a storage buffer that a compute pass left
// in STORAGE_WRITE must record STORAGE_WRITE → COPY_DST before the staging
// copy. The old code recorded no barrier at all for storage/copy buffers.
// Rust: write_staging_buffer_impl → set_single(COPY_DST) + transition_buffers
// before copy_buffer_to_buffer (queue.rs:919-950).
func TestPendingWrites_WriteBufferBarrierBeforeCopy(t *testing.T) {
	pw, dev, coreDevice := trackedPendingWrites(t)
	buf := core.NewBuffer(&noop.Resource{}, coreDevice,
		gputypes.BufferUsageStorage|gputypes.BufferUsageCopyDst|gputypes.BufferUsageCopySrc, 48, "b")
	idx := buf.TrackingData().Index()
	coreDevice.Tracker().InsertBuffer(idx, track.BufferUsesStorageWrite)

	ref := core.NewResourceRef("b", nil)
	if err := pw.writeBufferFor(ref, &noop.Resource{}, idx, 0, make([]byte, 48)); err != nil {
		t.Fatalf("writeBuffer: %v", err)
	}
	enc := dev.encoder
	if len(enc.events) != 2 || enc.events[0] != "buf-barrier" || enc.events[1] != "copy" {
		t.Fatalf("events = %v, want [buf-barrier copy]", enc.events)
	}
	b := enc.bufferBarriers[0]
	if b.Usage.OldUsage != gputypes.BufferUsageStorage || b.Usage.NewUsage != gputypes.BufferUsageCopyDst {
		t.Errorf("barrier = %v → %v, want Storage → CopyDst", b.Usage.OldUsage, b.Usage.NewUsage)
	}
	if got := coreDevice.Tracker().BufferUsage(idx); got != track.BufferUsesCopyDst {
		t.Errorf("device state after write = %d, want CopyDst", got)
	}

	// No read usage (VERTEX/INDEX/UNIFORM/INDIRECT): flush adds no barrier and
	// the buffer stays COPY_DST, so the next command buffer's transit (e.g.
	// COPY_DST → COPY_SRC for a readback) starts from the real state.
	pw.mu.Lock()
	cmdBuf, _, _, flushedRefs, err := pw.flush()
	pw.mu.Unlock()
	if err != nil || cmdBuf == nil {
		t.Fatalf("flush: cmdBuf=%v err=%v", cmdBuf, err)
	}
	if len(flushedRefs) != 1 || flushedRefs[0] != ref {
		t.Errorf("flushed destination refs = %v, want the buffer's", flushedRefs)
	}
	dropRefs(flushedRefs)
	if len(enc.bufferBarriers) != 1 {
		t.Errorf("buffer barriers after flush = %d, want 1", len(enc.bufferBarriers))
	}
	if got := coreDevice.Tracker().BufferUsage(idx); got != track.BufferUsesCopyDst {
		t.Errorf("device state after flush = %d, want CopyDst", got)
	}
}

func TestPendingWrites_WriteTextureEmptyData(t *testing.T) {
	pw, _, q := createBatchingPW(t)
	defer pw.destroy()

	tex := &noop.Texture{}
	dst := &hal.ImageCopyTexture{Texture: tex}
	layout := &hal.ImageDataLayout{BytesPerRow: 4}
	size := &hal.Extent3D{Width: 1, Height: 1, DepthOrArrayLayers: 1}

	if err := pw.writeTexture(dst, []byte{}, layout, size); err != nil {
		t.Fatalf("writeTexture empty: %v", err)
	}

	if q.writeBufferCalls != 0 {
		t.Errorf("expected 0 WriteBuffer calls for empty data, got %d", q.writeBufferCalls)
	}
	if pw.isRecording {
		t.Error("expected isRecording=false after empty texture write")
	}
}

func TestPendingWrites_FlushNoWork(t *testing.T) {
	pw, _, _ := createBatchingPW(t)
	defer pw.destroy()

	pw.mu.Lock()
	cmdBuf, enc, dstTex, dstBuf, err := pw.flush()
	pw.mu.Unlock()

	if err != nil {
		t.Fatalf("flush: %v", err)
	}
	if cmdBuf != nil {
		t.Error("expected nil cmdBuf when no work pending")
	}
	if enc != nil {
		t.Error("expected nil encoder when no work pending")
	}
	if dstTex != nil {
		t.Error("expected nil dstTextures when no work pending")
	}
	if dstBuf != nil {
		t.Error("expected nil dstBuffers when no work pending")
	}
}

func TestPendingWrites_FlushWithPendingWork(t *testing.T) {
	pw, _, _ := createBatchingPW(t)
	defer pw.destroy()

	dstBuf := &noop.Resource{}
	data := []byte{1, 2, 3, 4}
	ref := core.NewResourceRef("dst", nil)
	// Two writes to one buffer hold one reference.
	for range 2 {
		if err := pw.writeBufferFor(ref, dstBuf, track.InvalidTrackerIndex, 0, data); err != nil {
			t.Fatalf("writeBuffer: %v", err)
		}
	}
	if got := ref.RefCount(); got != 2 {
		t.Errorf("buffer refcount while recorded = %d, want 2", got)
	}

	pw.mu.Lock()
	cmdBuf, enc, _, flushedRefs, err := pw.flush()
	pw.mu.Unlock()

	if err != nil {
		t.Fatalf("flush: %v", err)
	}
	if cmdBuf == nil {
		t.Error("expected non-nil cmdBuf after flush with pending work")
	}
	if enc == nil {
		t.Error("expected non-nil encoder after flush")
	}
	// staging from flush contains only oversized one-off buffers;
	// normal belt-managed writes produce no oversized buffers.
	if len(flushedRefs) != 1 || flushedRefs[0] != ref {
		t.Errorf("flushed destination refs = %v, want the buffer's", flushedRefs)
	}
	if got := ref.RefCount(); got != 2 {
		t.Errorf("buffer refcount after flush = %d, want 2 (held for the submission)", got)
	}
	if len(pw.dstRefs) != 0 {
		t.Errorf("expected dstRefs moved out by flush, got %d", len(pw.dstRefs))
	}

	// Belt chunks should have moved to closedSubmissions.
	s := pw.belt.stats()
	closedSubs := s.ClosedSubs
	if closedSubs != 1 {
		t.Errorf("expected 1 closed belt submission, got %d", closedSubs)
	}

	// After flush, state should be reset.
	if pw.isRecording {
		t.Error("expected isRecording=false after flush")
	}
	if pw.encoder != nil {
		t.Error("expected encoder=nil after flush")
	}
	if len(pw.dstBuffers) != 0 {
		t.Errorf("expected dstBuffers cleared, got %d", len(pw.dstBuffers))
	}
}

// TestPendingWrites_DestinationRefsOutliveEncoding: a queue write keeps its
// destination buffer and texture alive while the batch is recorded, and a
// discarded batch drops them only after its encoding is discarded and the
// encoder reset. Dropping the last reference destroys the resource, and an
// open D3D12 command list must not outlive a resource it records (Rust
// wgpu-core: Drop for PendingWrites discards the encoding before its
// dst_buffers and dst_textures drop).
func TestPendingWrites_DestinationRefsOutliveEncoding(t *testing.T) {
	pw, dev, coreDevice := trackedPendingWrites(t)
	var events *[]string
	dropped := func(name string) func() {
		return func() { *events = append(*events, name) }
	}

	bufferRef := core.NewResourceRef("buffer", dropped("buffer-drop"))
	if err := pw.writeBufferFor(bufferRef, &noop.Resource{}, track.InvalidTrackerIndex, 0, []byte{1, 2, 3, 4}); err != nil {
		t.Fatalf("writeBufferFor: %v", err)
	}
	halTexture := &noop.Texture{}
	coreTexture := core.NewTexture(halTexture, coreDevice, gputypes.TextureFormatRGBA8Unorm,
		gputypes.TextureDimension2D, gputypes.TextureUsageCopyDst,
		gputypes.Extent3D{Width: 1, Height: 1, DepthOrArrayLayers: 1}, 1, 1, "texture")
	coreTexture.Ref = core.NewResourceRef("texture", dropped("texture-drop"))
	texture := &Texture{hal: halTexture, device: &Device{core: coreDevice}, coreTexture: coreTexture}
	if err := pw.writeTextureFor(texture, &hal.ImageCopyTexture{Texture: halTexture, Aspect: gputypes.TextureAspectAll},
		make([]byte, 4), &hal.ImageDataLayout{BytesPerRow: 4, RowsPerImage: 1},
		&hal.Extent3D{Width: 1, Height: 1, DepthOrArrayLayers: 1}); err != nil {
		t.Fatalf("writeTextureFor: %v", err)
	}
	events = &dev.encoder.events

	// The application releases both while the write is still recorded.
	bufferRef.Drop()
	coreTexture.Ref.Drop()
	if got := bufferRef.RefCount(); got != 1 {
		t.Errorf("buffer refcount after application release = %d, want 1 (held by the write)", got)
	}
	if got := coreTexture.Ref.RefCount(); got != 1 {
		t.Errorf("texture refcount after application release = %d, want 1 (held by the write)", got)
	}

	pw.mu.Lock()
	pw.discardRecording()
	pw.mu.Unlock()
	got := strings.Join(*events, " ")
	if !strings.HasSuffix(got, "discard reset buffer-drop texture-drop") {
		t.Errorf("events = %q, want the encoding discarded and reset before the destinations drop", got)
	}
}

func TestPendingWrites_FlushWithTextureWork(t *testing.T) {
	pw, _, _ := createBatchingPW(t)
	defer pw.destroy()

	tex := &noop.Texture{}
	dst := &hal.ImageCopyTexture{Texture: tex}
	data := make([]byte, 32)
	layout := &hal.ImageDataLayout{BytesPerRow: 32, RowsPerImage: 1}
	size := &hal.Extent3D{Width: 8, Height: 1, DepthOrArrayLayers: 1}

	if err := pw.writeTexture(dst, data, layout, size); err != nil {
		t.Fatalf("writeTexture: %v", err)
	}

	pw.mu.Lock()
	cmdBuf, enc, flushedTex, _, err := pw.flush()
	pw.mu.Unlock()

	if err != nil {
		t.Fatalf("flush: %v", err)
	}
	if cmdBuf == nil {
		t.Error("expected non-nil cmdBuf")
	}
	if enc == nil {
		t.Error("expected non-nil encoder")
	}
	// staging from flush contains only oversized one-off buffers;
	// texture data fits in a belt chunk, so no oversized buffers.
	if len(flushedTex) != 1 {
		t.Errorf("expected 1 flushed texture, got %d", len(flushedTex))
	}

	// Belt chunks should have moved to closedSubmissions.
	s := pw.belt.stats()
	closedSubs := s.ClosedSubs
	if closedSubs != 1 {
		t.Errorf("expected 1 closed belt submission, got %d", closedSubs)
	}
}

func TestPendingWrites_Maintain(t *testing.T) {
	pw, _, _ := createBatchingPW(t)
	defer pw.destroy()

	// Simulate two inflight submissions with staging buffers.
	buf1 := &noop.Resource{}
	buf2 := &noop.Resource{}
	buf3 := &noop.Resource{}
	enc1 := &noop.CommandEncoder{}
	cmdBuf1 := &noop.Resource{}

	pw.inflight = []inflightSubmission{
		{
			submissionIndex: 1,
			staging:         []hal.Buffer{buf1},
			cmdBuf:          cmdBuf1,
			encoder:         enc1,
		},
		{
			submissionIndex: 2,
			staging:         []hal.Buffer{buf2, buf3},
		},
	}

	// Complete submission 1 only.
	pw.maintain(1)

	if len(pw.inflight) != 1 {
		t.Errorf("expected 1 inflight after maintain(1), got %d", len(pw.inflight))
	}
	if pw.inflight[0].submissionIndex != 2 {
		t.Errorf("expected remaining submission index=2, got %d", pw.inflight[0].submissionIndex)
	}

	// Complete all.
	pw.maintain(5)
	if len(pw.inflight) != 0 {
		t.Errorf("expected 0 inflight after maintain(5), got %d", len(pw.inflight))
	}
}

func TestPendingWrites_MaintainWithPendingTextureRefs(t *testing.T) {
	pw, _, _ := createBatchingPW(t)
	defer pw.destroy()

	tex := &noop.Texture{}

	pw.inflight = []inflightSubmission{
		{
			submissionIndex: 1,
			dstTextures:     []hal.Texture{tex},
		},
	}

	// maintain should call DecPendingRef on textures.
	// noop.Texture.DecPendingRef is a no-op, but we verify the path runs without panic.
	pw.maintain(1)

	if len(pw.inflight) != 0 {
		t.Errorf("expected 0 inflight, got %d", len(pw.inflight))
	}
}

func TestPendingWrites_HasPendingWork(t *testing.T) {
	pw, _, _ := createBatchingPW(t)
	defer pw.destroy()

	if pw.HasPendingWork() {
		t.Error("expected no pending work initially")
	}

	// Write something.
	dstBuf := &noop.Resource{}
	if err := pw.writeBufferFor(nil, dstBuf, track.InvalidTrackerIndex, 0, []byte{1, 2, 3, 4}); err != nil {
		t.Fatalf("writeBuffer: %v", err)
	}

	if !pw.HasPendingWork() {
		t.Error("expected pending work after write")
	}

	// Flush should clear pending work.
	pw.mu.Lock()
	flushErr := flushAndDiscard(pw)
	pw.mu.Unlock()
	if flushErr != nil {
		t.Fatalf("flush: %v", flushErr)
	}

	if pw.HasPendingWork() {
		t.Error("expected no pending work after flush")
	}
}

func TestContainsTextureOwnerUsesWrapperIdentity(t *testing.T) {
	first := &Texture{}
	second := &Texture{}
	owners := []*Texture{nil, first}
	if !containsTextureOwner(owners, first) {
		t.Fatal("containsTextureOwner missed the retained wrapper")
	}
	if containsTextureOwner(owners, second) {
		t.Fatal("containsTextureOwner matched a distinct wrapper")
	}
}

func TestPendingWrites_DestroyCleanup(t *testing.T) {
	pw, _, _ := createBatchingPW(t)

	// Write to create staging + encoder.
	dstBuf := &noop.Resource{}
	if err := pw.writeBufferFor(nil, dstBuf, track.InvalidTrackerIndex, 0, []byte{1}); err != nil {
		t.Fatalf("writeBuffer: %v", err)
	}

	// Add inflight submissions.
	pw.mu.Lock()
	pw.inflight = append(pw.inflight, inflightSubmission{
		submissionIndex: 1,
		staging:         []hal.Buffer{&noop.Resource{}},
	})
	pw.mu.Unlock()

	// destroy should not panic and should clean up everything.
	pw.destroy()

	if pw.encoder != nil {
		t.Error("expected encoder=nil after destroy")
	}
	if pw.staging != nil {
		t.Error("expected staging=nil after destroy")
	}
	if pw.inflight != nil {
		t.Error("expected inflight=nil after destroy")
	}
	if pw.dstBuffers != nil {
		t.Error("expected dstBuffers=nil after destroy")
	}
	if pw.dstTextures != nil {
		t.Error("expected dstTextures=nil after destroy")
	}
}

func TestPendingWrites_DestroyNonBatching(t *testing.T) {
	pw, _, _ := createNonBatchingPW(t)
	// Should not panic even with no pool.
	pw.destroy()
}

func TestPendingWrites_AlignUp(t *testing.T) {
	tests := []struct {
		name      string
		n         uint32
		alignment uint32
		want      uint32
	}{
		{"already aligned", 256, 256, 256},
		{"needs alignment", 100, 256, 256},
		{"one over", 257, 256, 512},
		{"zero value", 0, 256, 0},
		{"alignment 1", 42, 1, 42},
		{"alignment 0", 42, 0, 42},
		{"power of two", 128, 64, 128},
		{"not power of two alignment", 10, 3, 12},
		{"exact multiple", 9, 3, 9},
		{"large value", 1000, 256, 1024},
		{"small alignment", 5, 4, 8},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := alignUp(tt.n, tt.alignment)
			if got != tt.want {
				t.Errorf("alignUp(%d, %d) = %d, want %d", tt.n, tt.alignment, got, tt.want)
			}
		})
	}
}

func TestPendingWrites_CopyTextureDataAligned(t *testing.T) {
	t.Run("no padding needed", func(t *testing.T) {
		data := []byte{1, 2, 3, 4, 5, 6, 7, 8}
		result := copyTextureDataAligned(data, 0, 4, 4, 2, 1, 8)
		// When aligned == bytesPerRow, returns original slice.
		if &result[0] != &data[0] {
			t.Error("expected same slice returned when no alignment needed")
		}
	})

	t.Run("with padding", func(t *testing.T) {
		// 2 rows of 3 bytes each, aligned to 4 bytes per row.
		data := []byte{1, 2, 3, 4, 5, 6}
		result := copyTextureDataAligned(data, 0, 3, 4, 2, 1, 8)

		if len(result) != 8 {
			t.Fatalf("expected len=8, got %d", len(result))
		}
		// Row 0: bytes 0-2 = {1,2,3}, byte 3 = padding (0).
		if result[0] != 1 || result[1] != 2 || result[2] != 3 {
			t.Errorf("row 0: got %v, want [1 2 3 ...]", result[0:4])
		}
		if result[3] != 0 {
			t.Errorf("row 0 padding: got %d, want 0", result[3])
		}
		// Row 1: bytes 4-6 = {4,5,6}, byte 7 = padding (0).
		if result[4] != 4 || result[5] != 5 || result[6] != 6 {
			t.Errorf("row 1: got %v, want [4 5 6 ...]", result[4:8])
		}
		if result[7] != 0 {
			t.Errorf("row 1 padding: got %d, want 0", result[7])
		}
	})

	t.Run("with source offset", func(t *testing.T) {
		// Skip first 2 bytes, then 2 rows of 2 bytes, aligned to 4.
		data := []byte{0xFF, 0xFF, 10, 20, 30, 40}
		result := copyTextureDataAligned(data, 2, 2, 4, 2, 1, 8)

		if len(result) != 8 {
			t.Fatalf("expected len=8, got %d", len(result))
		}
		if result[0] != 10 || result[1] != 20 {
			t.Errorf("row 0: got [%d %d], want [10 20]", result[0], result[1])
		}
		if result[4] != 30 || result[5] != 40 {
			t.Errorf("row 1: got [%d %d], want [30 40]", result[4], result[5])
		}
	})

	t.Run("multiple layers", func(t *testing.T) {
		// 2 layers, 1 row each, 2 bytes per row aligned to 4.
		data := []byte{1, 2, 3, 4}
		result := copyTextureDataAligned(data, 0, 2, 4, 1, 2, 8)

		if len(result) != 8 {
			t.Fatalf("expected len=8, got %d", len(result))
		}
		// Layer 0, row 0.
		if result[0] != 1 || result[1] != 2 {
			t.Errorf("layer 0: got [%d %d], want [1 2]", result[0], result[1])
		}
		// Layer 1, row 0.
		if result[4] != 3 || result[5] != 4 {
			t.Errorf("layer 1: got [%d %d], want [3 4]", result[4], result[5])
		}
	})

	t.Run("alignment 8", func(t *testing.T) {
		// 2 rows of 3 bytes each aligned to 8: exercises alignedBytesPerRow != 4.
		data := []byte{1, 2, 3, 4, 5, 6}
		result := copyTextureDataAligned(data, 0, 3, 8, 2, 1, 16)

		if len(result) != 16 {
			t.Fatalf("expected len=16, got %d", len(result))
		}
		if result[0] != 1 || result[1] != 2 || result[2] != 3 {
			t.Errorf("row 0 bytes: got %v, want [1 2 3]", result[0:3])
		}
		if result[8] != 4 || result[9] != 5 || result[10] != 6 {
			t.Errorf("row 1 bytes: got %v, want [4 5 6]", result[8:11])
		}
	})
}

func TestPendingWrites_AlignTextureDataInto(t *testing.T) {
	t.Run("no padding - returns original slice", func(t *testing.T) {
		data := []byte{1, 2, 3, 4}
		var buf []byte
		result := alignTextureDataInto(&buf, data, 0, 4, 4, 1, 1, 4)
		if &result[0] != &data[0] {
			t.Error("expected original slice returned when no padding needed")
		}
	})

	t.Run("allocates new buffer", func(t *testing.T) {
		// 2 rows of 3 bytes aligned to 4 — buf starts nil, must be allocated.
		data := []byte{1, 2, 3, 4, 5, 6}
		var buf []byte
		result := alignTextureDataInto(&buf, data, 0, 3, 4, 2, 1, 8)
		if len(result) != 8 {
			t.Fatalf("expected len=8, got %d", len(result))
		}
		if result[0] != 1 || result[1] != 2 || result[2] != 3 {
			t.Errorf("row 0: got %v", result[0:4])
		}
	})

	t.Run("reuses pre-allocated buffer", func(t *testing.T) {
		// Pre-allocate a buffer large enough — exercises the else branch
		// (*buf = (*buf)[:stagingSize]) instead of make().
		data := []byte{10, 20, 30, 40, 50, 60}
		buf := make([]byte, 0, 32) // cap 32 > stagingSize 8
		ptr := &buf[0:1:32][0]
		result := alignTextureDataInto(&buf, data, 0, 3, 4, 2, 1, 8)
		if len(result) != 8 {
			t.Fatalf("expected len=8, got %d", len(result))
		}
		// Verify no new allocation: the backing array should be the same.
		if len(buf) > 0 && &buf[0] != ptr {
			t.Error("expected pre-allocated buffer to be reused (no new make)")
		}
		if result[0] != 10 || result[1] != 20 || result[2] != 30 {
			t.Errorf("row 0: got %v", result[0:4])
		}
	})
}

func TestPendingWrites_AlignTextureRowsIntoTruncated(t *testing.T) {
	// data is one byte short of a full second row — the inner loop hits
	// the srcRowStart+bytesPerRow > len(data) guard and breaks early.
	data := []byte{1, 2, 3} // only 3 bytes; row 1 would need bytes [3,5) which are missing
	aligned := make([]byte, 8)
	alignTextureRowsInto(aligned, data, 0, 3, 4, 2, 1)
	// Row 0 bytes should be copied.
	if aligned[0] != 1 || aligned[1] != 2 || aligned[2] != 3 {
		t.Errorf("row 0: got %v, want [1 2 3]", aligned[0:3])
	}
	// Row 1 must be zeroed (break fired, no copy happened).
	if aligned[4] != 0 || aligned[5] != 0 || aligned[6] != 0 {
		t.Errorf("row 1 should be zero after break, got %v", aligned[4:7])
	}
}

// TestPendingWrites_AlignTextureRowsShortLastRow pins the staging copy of data
// sized by WebGPU's rule: the data ends at the last byte of the last row of
// the last image, so that row is shorter than bytesPerRow and the padding
// rows of the last image are absent. Every row must still reach staging, and
// inner images keep their full rowsPerImage pitch.
func TestPendingWrites_AlignTextureRowsShortLastRow(t *testing.T) {
	const bytesPerRow, aligned, bytesInLastRow = 6, 8, 4
	// rowOf returns a row whose bytes are 10*row+1, 10*row+2, ...
	rowOf := func(row, n int) []byte {
		b := make([]byte, n)
		for i := range b {
			b[i] = byte(10*row + i + 1)
		}
		return b
	}
	tests := []struct {
		name         string
		offset       int
		rows         int // rows written per image
		rowsPerImage int
		layers       int
	}{
		{name: "one image", offset: 0, rows: 2, rowsPerImage: 2, layers: 1},
		{name: "one image with offset", offset: 3, rows: 3, rowsPerImage: 3, layers: 1},
		{name: "three images with padding rows", offset: 5, rows: 2, rowsPerImage: 3, layers: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The data holds offset bytes, then every row of the inner images
			// and the copied rows of the last one, the last of them short.
			data := bytes.Repeat([]byte{0xEE}, tt.offset)
			total := (tt.layers-1)*tt.rowsPerImage + tt.rows
			for row := 0; row < total; row++ {
				n := bytesPerRow
				if row == total-1 {
					n = bytesInLastRow
				}
				data = append(data, rowOf(row, n)...)
			}
			if want := tt.offset + bytesPerRow*tt.rowsPerImage*(tt.layers-1) + bytesPerRow*(tt.rows-1) + bytesInLastRow; len(data) != want {
				t.Fatalf("data is %d bytes, want the WebGPU minimum %d", len(data), want)
			}
			stagingSize := uint64(aligned * tt.rowsPerImage * tt.layers)
			var buf []byte
			got := alignTextureDataInto(&buf, data, uint64(tt.offset), bytesPerRow, aligned,
				uint32(tt.rowsPerImage), uint32(tt.layers), stagingSize)
			for layer := 0; layer < tt.layers; layer++ {
				for r := 0; r < tt.rows; r++ {
					row := layer*tt.rowsPerImage + r
					n := bytesPerRow
					if row == total-1 {
						n = bytesInLastRow
					}
					if g, w := got[row*aligned:row*aligned+n], rowOf(row, n); !bytes.Equal(g, w) {
						t.Errorf("layer %d row %d = %v, want %v", layer, r, g, w)
					}
				}
			}
		})
	}
}

// TestPendingWrites_StagingSourceKeepsLayout pins the copy that needs no
// padding: staging must start at the layout offset and take no more than its
// own size, however long the data is.
func TestPendingWrites_StagingSourceKeepsLayout(t *testing.T) {
	data := make([]byte, 1024)
	for i := range data {
		data[i] = byte(i)
	}
	var buf []byte
	got := alignTextureDataInto(&buf, data, 12, 256, 256, 2, 1, 512)
	if len(got) != 512 {
		t.Fatalf("staging data is %d bytes, want 512", len(got))
	}
	if !bytes.Equal(got, data[12:12+512]) {
		t.Errorf("staging data is not the 512 bytes from the layout offset 12")
	}
	if got := copyTextureDataAligned(data, 12, 256, 256, 2, 1, 512); !bytes.Equal(got, data[12:12+512]) {
		t.Errorf("copyTextureDataAligned: staging data is not the 512 bytes from the layout offset 12")
	}
	if got := alignTextureDataInto(&buf, data[:300], 12, 256, 256, 2, 1, 512); len(got) != 288 {
		t.Errorf("short data: staging data is %d bytes, want the 288 the data holds", len(got))
	}
}

func TestPendingWrites_WriteBufferEmptyBatching(t *testing.T) {
	// Zero-length write in batching mode must return nil without allocating
	// staging or activating the encoder.
	pw, _, _ := createBatchingPW(t)
	defer pw.destroy()

	dstBuf := &noop.Resource{}
	if err := pw.writeBufferFor(nil, dstBuf, track.InvalidTrackerIndex, 0, []byte{}); err != nil {
		t.Fatalf("writeBuffer(empty): %v", err)
	}
	if pw.isRecording {
		t.Error("empty write must not activate encoder")
	}
}

func TestPendingWrites_MultipleWritesThenFlush(t *testing.T) {
	pw, _, _ := createBatchingPW(t)
	defer pw.destroy()

	buf1 := &noop.Resource{}
	buf2 := &noop.Resource{}
	data := []byte{1, 2, 3, 4}

	// Multiple writes should accumulate.
	if err := pw.writeBufferFor(nil, buf1, track.InvalidTrackerIndex, 0, data); err != nil {
		t.Fatalf("writeBuffer 1: %v", err)
	}
	if err := pw.writeBufferFor(nil, buf2, track.InvalidTrackerIndex, 0, data); err != nil {
		t.Fatalf("writeBuffer 2: %v", err)
	}

	// Both writes fit in the same belt chunk (4 bytes each, chunk=256KB).
	s := pw.belt.stats()
	activeChunks := s.ActiveChunks
	if activeChunks != 1 {
		t.Errorf("expected 1 active belt chunk (both writes fit), got %d", activeChunks)
	}

	pw.mu.Lock()
	cmdBuf, enc, _, _, err := pw.flush()
	pw.mu.Unlock()

	if err != nil {
		t.Fatalf("flush: %v", err)
	}
	if cmdBuf == nil {
		t.Error("expected non-nil cmdBuf")
	}
	if enc == nil {
		t.Error("expected non-nil encoder")
	}
	// No oversized buffers — both writes fit in belt chunks.

	// Belt chunks moved to closed.
	s = pw.belt.stats()
	if s.ClosedSubs != 1 {
		t.Errorf("expected 1 closed belt submission, got %d", s.ClosedSubs)
	}

	// State reset after flush.
	if pw.isRecording {
		t.Error("expected isRecording=false after flush")
	}
}

func TestPendingWrites_ActivateReusesEncoder(t *testing.T) {
	pw, _, _ := createBatchingPW(t)
	defer pw.destroy()

	// First write activates encoder.
	if err := pw.writeBufferFor(nil, &noop.Resource{}, track.InvalidTrackerIndex, 0, []byte{1}); err != nil {
		t.Fatalf("write 1: %v", err)
	}
	firstEncoder := pw.encoder

	// Second write should reuse the same encoder.
	if err := pw.writeBufferFor(nil, &noop.Resource{}, track.InvalidTrackerIndex, 0, []byte{2}); err != nil {
		t.Fatalf("write 2: %v", err)
	}

	if pw.encoder != firstEncoder {
		t.Error("expected activate to reuse existing encoder")
	}
}

func TestPendingWrites_FlushThenWriteGetsNewEncoder(t *testing.T) {
	pw, _, _ := createBatchingPW(t)
	defer pw.destroy()

	// Write and flush.
	if err := pw.writeBufferFor(nil, &noop.Resource{}, track.InvalidTrackerIndex, 0, []byte{1}); err != nil {
		t.Fatalf("write: %v", err)
	}

	pw.mu.Lock()
	flushedCmdBuf, flushedEnc, flushedTex, flushedBufs, err := pw.flush()
	pw.mu.Unlock()
	if err != nil {
		t.Fatalf("flush: %v", err)
	}
	// Consume return values to avoid dogsled lint.
	_ = flushedCmdBuf
	_ = flushedTex
	_ = flushedBufs

	// Write again — should get a new encoder from pool.
	if err := pw.writeBufferFor(nil, &noop.Resource{}, track.InvalidTrackerIndex, 0, []byte{2}); err != nil {
		t.Fatalf("write after flush: %v", err)
	}

	if pw.encoder == nil {
		t.Error("expected new encoder after flush+write")
	}
	// The flushed encoder should be different from current (detached).
	_ = flushedEnc // flushedEnc was detached; pw.encoder is new.
	if !pw.isRecording {
		t.Error("expected isRecording=true after new write")
	}
}

func TestPendingWrites_NewWithNoopBackend(t *testing.T) {
	// Using the actual noop backend via createNoopDeviceForTest.
	dev, q, cleanup := createNoopDeviceForTest(t)
	defer cleanup()

	pw := newPendingWrites(dev, q, nil, nil)
	defer pw.destroy()

	if pw.usesBatching {
		t.Error("noop backend should not use batching")
	}
	if pw.pool != nil {
		t.Error("noop backend should not have encoder pool")
	}
}

func TestPendingWrites_WriteBufferNonBatchingVerifiesData(t *testing.T) {
	// Verify that non-batching actually writes data through to the buffer.
	dev, q, cleanup := createNoopDeviceForTest(t)
	defer cleanup()

	pw := newPendingWrites(dev, q, nil, nil)
	defer pw.destroy()

	buf, err := dev.CreateBuffer(&hal.BufferDescriptor{
		Label:            "test",
		Size:             16,
		Usage:            gputypes.BufferUsageCopyDst,
		MappedAtCreation: true,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}

	data := []byte{0xAA, 0xBB, 0xCC, 0xDD}
	if err := pw.writeBufferFor(nil, buf, track.InvalidTrackerIndex, 4, data); err != nil {
		t.Fatalf("writeBuffer: %v", err)
	}

	// Read back from noop buffer to verify data was written, using the
	// new HAL MapBuffer path (replaces legacy Queue.ReadBuffer removed
	// in FEAT-WGPU-MAPPING-001).
	mapping, mapErr := dev.MapBuffer(buf, 0, 16)
	if mapErr != nil {
		t.Fatalf("MapBuffer: %v", mapErr)
	}
	readBack := make([]byte, 4)
	src := unsafe.Slice((*byte)(unsafe.Add(mapping.Ptr, 4)), 4)
	copy(readBack, src)
	_ = dev.UnmapBuffer(buf)

	for i, b := range readBack {
		if b != data[i] {
			t.Errorf("byte %d: got 0x%02x, want 0x%02x", i, b, data[i])
		}
	}
}

func TestPendingWrites_MaxStagingBufferSizerWiring(t *testing.T) {
	// Verify that newPendingWrites queries the hal.MaxStagingBufferSizer
	// interface from the device and propagates it to the staging belt.
	// This is the core of BUG-VK-001: on Vulkan, maxMemoryAllocationSize
	// must cap the staging belt to prevent oversized allocations.
	dev := &mockStagingSizeDevice{maxStaging: 1 << 20} // 1MB
	q := &mockBatchingQueue{}
	pool := newEncoderPool(&dev.Device)
	pw := newPendingWrites(dev, q, pool, nil)
	defer pw.destroy()
	defer pool.destroy()

	if pw.belt == nil {
		t.Fatal("expected non-nil belt for batching backend")
	}

	if pw.belt.maxStagingBufferSize != 1<<20 {
		t.Errorf("belt.maxStagingBufferSize = %d, want %d", pw.belt.maxStagingBufferSize, 1<<20)
	}
}

func TestPendingWrites_MaxStagingBufferSizerDefaultWhenNotImplemented(t *testing.T) {
	// Verify that when the device does NOT implement MaxStagingBufferSizer,
	// the belt uses the default 64MB cap.
	dev := &noop.Device{} // noop.Device does NOT implement MaxStagingBufferSizer
	q := &mockBatchingQueue{}
	pool := newEncoderPool(dev)
	pw := newPendingWrites(dev, q, pool, nil)
	defer pw.destroy()
	defer pool.destroy()

	if pw.belt == nil {
		t.Fatal("expected non-nil belt for batching backend")
	}

	if pw.belt.maxStagingBufferSize != stagingBeltMaxOversizedSize {
		t.Errorf("belt.maxStagingBufferSize = %d, want default %d",
			pw.belt.maxStagingBufferSize, stagingBeltMaxOversizedSize)
	}
}
