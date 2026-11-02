//go:build !(js && wasm)

package core

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/core/track"
	"github.com/gogpu/wgpu/hal"
)

// recordingCommandBuffer is a HAL command buffer with an identity, so tests
// can check the order of the command buffers an encoder produced.
type recordingCommandBuffer struct {
	mockCommandBuffer
	id int
}

// recordingEncoder is a HAL command encoder that logs, per recording, the
// commands that matter for pass usage tracking: barriers, passes, dispatches.
type recordingEncoder struct {
	mockCommandEncoder
	nextID  int
	current int
	open    bool
	log     map[int][]string
	texture map[int][]hal.TextureBarrier
}

func newRecordingEncoder() *recordingEncoder {
	// The creator begins the first recording (see CreateCommandEncoderWithHAL).
	return &recordingEncoder{
		nextID:  1,
		open:    true,
		log:     map[int][]string{},
		texture: map[int][]hal.TextureBarrier{},
	}
}

func (r *recordingEncoder) record(event string) {
	r.log[r.current] = append(r.log[r.current], event)
}

func (r *recordingEncoder) BeginEncoding(string) error {
	r.current = r.nextID
	r.nextID++
	r.open = true
	return nil
}

func (r *recordingEncoder) EndEncoding() (hal.CommandBuffer, error) {
	r.open = false
	return recordingCommandBuffer{id: r.current}, nil
}

func (r *recordingEncoder) TransitionTextures(barriers []hal.TextureBarrier) {
	r.record("texture barrier")
	r.texture[r.current] = append(r.texture[r.current], barriers...)
}

func (r *recordingEncoder) TransitionBuffers([]hal.BufferBarrier) {
	r.record("buffer barrier")
}

func (r *recordingEncoder) BeginRenderPass(*hal.RenderPassDescriptor) hal.RenderPassEncoder {
	r.record("render pass")
	return mockRenderPassEncoder{}
}

func (r *recordingEncoder) BeginComputePass(*hal.ComputePassDescriptor) hal.ComputePassEncoder {
	r.record("compute pass")
	return recordingComputePass{encoder: r}
}

type recordingComputePass struct {
	mockComputePassEncoder
	encoder *recordingEncoder
}

func (p recordingComputePass) Dispatch(_, _, _ uint32) { p.encoder.record("dispatch") }

// namedTexture is a HAL texture distinguishable from other mock textures.
type namedTexture struct {
	mockTexture
	name string
}

func newPassUsageTestEncoder(t *testing.T) (*CoreCommandEncoder, *recordingEncoder, *Device) {
	t.Helper()
	device := NewDevice(&mockHALDevice{}, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")
	rec := newRecordingEncoder()
	enc, err := device.CreateCommandEncoderWithHAL(rec, "pass-usage")
	if err != nil {
		t.Fatalf("CreateCommandEncoderWithHAL: %v", err)
	}
	return enc, rec, device
}

func newPassUsageTestTexture(device *Device, name string) *Texture {
	return NewTexture(
		namedTexture{name: name}, device,
		gputypes.TextureFormatRGBA8Unorm, gputypes.TextureDimension2D,
		gputypes.TextureUsageStorageBinding|gputypes.TextureUsageTextureBinding|gputypes.TextureUsageRenderAttachment,
		gputypes.Extent3D{Width: 8, Height: 8, DepthOrArrayLayers: 1},
		1, 1, name,
	)
}

func textureUses(texture *Texture, usage track.TextureUses) *BindGroupUsages {
	return &BindGroupUsages{Textures: []BindGroupTextureUse{{Texture: texture, Usage: usage}}}
}

func dispatchOnce(t *testing.T, enc *CoreCommandEncoder, groups ...*BindGroupUsages) {
	t.Helper()
	pass, err := enc.BeginComputePass(nil)
	if err != nil {
		t.Fatalf("BeginComputePass: %v", err)
	}
	if err := pass.FlushDispatch(groups, nil, track.BufferUsesNone); err != nil {
		t.Fatalf("FlushDispatch: %v", err)
	}
	pass.Dispatch(1, 1, 1)
	if err := pass.End(); err != nil {
		t.Fatalf("compute End: %v", err)
	}
}

func commandBufferIDs(t *testing.T, cb *CoreCommandBuffer) []int {
	t.Helper()
	list := cb.HalBufferList()
	ids := make([]int, 0, len(list))
	for _, raw := range list {
		rcb, ok := raw.(recordingCommandBuffer)
		if !ok {
			t.Fatalf("unexpected command buffer %T", raw)
		}
		ids = append(ids, rcb.id)
	}
	return ids
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// A storage texture written by a dispatch and sampled in a later render pass
// of the same encoder needs a barrier ahead of the pass. The pass gets its own
// command buffer and a transit command buffer holding the barrier is placed
// before it (ray-traced-triangle: compute writes, fullscreen blit samples).
func TestRenderPass_BindGroupTransitionGoesInTransitCommandBuffer(t *testing.T) {
	enc, rec, device := newPassUsageTestEncoder(t)
	storage := newPassUsageTestTexture(device, "storage")

	dispatchOnce(t, enc, textureUses(storage, track.TextureUsesStorageWrite))

	pass, err := enc.BeginRenderPass(&RenderPassDescriptor{Label: "blit"})
	if err != nil {
		t.Fatalf("BeginRenderPass: %v", err)
	}
	if err := pass.MergeBindGroup(textureUses(storage, track.TextureUsesResource)); err != nil {
		t.Fatalf("MergeBindGroup: %v", err)
	}
	pass.Draw(gputypes.DrawArgs{VertexCount: 3, InstanceCount: 1})
	if err := pass.End(); err != nil {
		t.Fatalf("render End: %v", err)
	}

	cb, err := enc.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	// Recording 0: compute; 1: render pass; 2: transit, submitted before 1.
	if got, want := commandBufferIDs(t, cb), []int{0, 2, 1}; !equalInts(got, want) {
		t.Fatalf("command buffer order = %v, want %v", got, want)
	}
	if got := rec.log[1]; len(got) != 1 || got[0] != "render pass" {
		t.Errorf("render pass command buffer = %v, want only the pass", got)
	}
	barriers := rec.texture[2]
	if len(barriers) != 1 {
		t.Fatalf("transit texture barriers = %d, want 1", len(barriers))
	}
	if barriers[0].Texture != (namedTexture{name: "storage"}) {
		t.Errorf("transit barrier texture = %v, want storage", barriers[0].Texture)
	}
	if barriers[0].Usage.OldUsage != gputypes.TextureUsageStorageBinding ||
		barriers[0].Usage.NewUsage != gputypes.TextureUsageTextureBinding {
		t.Errorf("transit barrier = %v -> %v, want StorageBinding -> TextureBinding",
			barriers[0].Usage.OldUsage, barriers[0].Usage.NewUsage)
	}
	if got := cb.TextureScope().EndUsage(storage.TrackingData().Index()); got != track.TextureUsesResource {
		t.Errorf("end usage = %v, want Resource", got)
	}
}

// A render pass at the start of an encoder needs no transit command buffer
// (the submit-time barrier covers first uses), so it stays in the one
// recording the encoder began with.
func TestRenderPass_FirstCommandKeepsSingleCommandBuffer(t *testing.T) {
	enc, rec, device := newPassUsageTestEncoder(t)
	texture := newPassUsageTestTexture(device, "sampled")

	pass, err := enc.BeginRenderPass(&RenderPassDescriptor{Label: "first"})
	if err != nil {
		t.Fatalf("BeginRenderPass: %v", err)
	}
	if err := pass.MergeBindGroup(textureUses(texture, track.TextureUsesResource)); err != nil {
		t.Fatalf("MergeBindGroup: %v", err)
	}
	if err := pass.End(); err != nil {
		t.Fatalf("render End: %v", err)
	}
	cb, err := enc.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if got := len(cb.HalBufferList()); got != 1 {
		t.Errorf("command buffers = %d, want 1", got)
	}
	if len(rec.texture) != 0 {
		t.Errorf("unexpected barriers: %v", rec.texture)
	}
	if got := cb.TextureScope().StartUsage(texture.TrackingData().Index()); got != track.TextureUsesResource {
		t.Errorf("start usage = %v, want Resource", got)
	}
}

// Commands after a transit-split render pass go into a new recording that is
// submitted after the pass.
func TestRenderPass_CommandsAfterSplitReopenRecording(t *testing.T) {
	enc, rec, device := newPassUsageTestEncoder(t)
	storage := newPassUsageTestTexture(device, "storage")

	dispatchOnce(t, enc, textureUses(storage, track.TextureUsesStorageWrite))
	pass, err := enc.BeginRenderPass(&RenderPassDescriptor{Label: "blit"})
	if err != nil {
		t.Fatalf("BeginRenderPass: %v", err)
	}
	if err := pass.MergeBindGroup(textureUses(storage, track.TextureUsesResource)); err != nil {
		t.Fatalf("MergeBindGroup: %v", err)
	}
	if err := pass.End(); err != nil {
		t.Fatalf("render End: %v", err)
	}
	if rec.open {
		t.Fatal("recording should be closed after the transit split")
	}

	// The next dispatch writes the texture again: it reopens a recording and
	// transitions the texture from the pass's sampled state.
	dispatchOnce(t, enc, textureUses(storage, track.TextureUsesStorageWrite))

	cb, err := enc.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if got, want := commandBufferIDs(t, cb), []int{0, 2, 1, 3}; !equalInts(got, want) {
		t.Fatalf("command buffer order = %v, want %v", got, want)
	}
	want := []string{"compute pass", "texture barrier", "dispatch"}
	if got := rec.log[3]; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Errorf("recording 3 = %v, want %v", got, want)
	}
}

// Barriers between dispatches go straight into the compute pass's command
// buffer, before the dispatch that needs them.
func TestComputePass_BarrierBetweenDispatches(t *testing.T) {
	enc, rec, device := newPassUsageTestEncoder(t)
	storage := newPassUsageTestTexture(device, "storage")

	pass, err := enc.BeginComputePass(nil)
	if err != nil {
		t.Fatalf("BeginComputePass: %v", err)
	}
	for _, usage := range []track.TextureUses{track.TextureUsesStorageWrite, track.TextureUsesStorageRead} {
		if err := pass.FlushDispatch([]*BindGroupUsages{textureUses(storage, usage)}, nil, track.BufferUsesNone); err != nil {
			t.Fatalf("FlushDispatch: %v", err)
		}
		pass.Dispatch(1, 1, 1)
	}
	if err := pass.End(); err != nil {
		t.Fatalf("End: %v", err)
	}

	want := []string{"compute pass", "dispatch", "texture barrier", "dispatch"}
	got := rec.log[0]
	if len(got) != len(want) {
		t.Fatalf("log = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("log = %v, want %v", got, want)
		}
	}
}

// Usages inside one dispatch must be compatible.
func TestComputePass_DispatchUsageConflict(t *testing.T) {
	enc, _, device := newPassUsageTestEncoder(t)
	storage := newPassUsageTestTexture(device, "storage")

	pass, err := enc.BeginComputePass(nil)
	if err != nil {
		t.Fatalf("BeginComputePass: %v", err)
	}
	err = pass.FlushDispatch([]*BindGroupUsages{
		textureUses(storage, track.TextureUsesStorageWrite),
		textureUses(storage, track.TextureUsesResource),
	}, nil, track.BufferUsesNone)
	if err == nil {
		t.Fatal("expected a usage conflict between storage write and sampled")
	}
}

// A texture used as an attachment and bound for sampling in the same render
// pass is a usage conflict.
func TestRenderPass_AttachmentAndBindGroupConflict(t *testing.T) {
	enc, _, device := newPassUsageTestEncoder(t)
	target := newPassUsageTestTexture(device, "target")

	pass, err := enc.BeginRenderPass(&RenderPassDescriptor{
		ColorAttachments: []RenderPassColorAttachment{{
			View:    &TextureView{HAL: mockTextureView{}, Parent: target},
			LoadOp:  gputypes.LoadOpClear,
			StoreOp: gputypes.StoreOpStore,
		}},
	})
	if err != nil {
		t.Fatalf("BeginRenderPass: %v", err)
	}
	if err := pass.MergeBindGroup(textureUses(target, track.TextureUsesResource)); err == nil {
		t.Fatal("expected a usage conflict between color attachment and sampled")
	}
}

// DiscardHAL returns the recordings a split already closed, so ResetAll can
// reclaim them.
func TestCoreCommandEncoder_DiscardHALReturnsClosedCommandBuffers(t *testing.T) {
	enc, _, device := newPassUsageTestEncoder(t)
	storage := newPassUsageTestTexture(device, "storage")

	dispatchOnce(t, enc, textureUses(storage, track.TextureUsesStorageWrite))
	pass, err := enc.BeginRenderPass(&RenderPassDescriptor{})
	if err != nil {
		t.Fatalf("BeginRenderPass: %v", err)
	}
	if err := pass.MergeBindGroup(textureUses(storage, track.TextureUsesResource)); err != nil {
		t.Fatalf("MergeBindGroup: %v", err)
	}
	if err := pass.End(); err != nil {
		t.Fatalf("End: %v", err)
	}
	if got := len(enc.DiscardHAL()); got != 3 {
		t.Errorf("closed command buffers = %d, want 3", got)
	}
}

// Standalone HAL encoders cannot begin another recording after EndEncoding,
// so render passes never split them.
func TestRenderPass_StandaloneEncoderDoesNotSplit(t *testing.T) {
	device := NewDevice(&mockHALDevice{}, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")
	enc, err := device.CreateCommandEncoder("standalone")
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	storage := newPassUsageTestTexture(device, "storage")

	dispatchOnce(t, enc, textureUses(storage, track.TextureUsesStorageWrite))
	pass, err := enc.BeginRenderPass(&RenderPassDescriptor{})
	if err != nil {
		t.Fatalf("BeginRenderPass: %v", err)
	}
	if err := pass.MergeBindGroup(textureUses(storage, track.TextureUsesResource)); err != nil {
		t.Fatalf("MergeBindGroup: %v", err)
	}
	if err := pass.End(); err != nil {
		t.Fatalf("End: %v", err)
	}
	cb, err := enc.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if got := len(cb.HalBufferList()); got != 1 {
		t.Errorf("command buffers = %d, want 1", got)
	}
}
