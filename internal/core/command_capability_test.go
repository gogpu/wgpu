//go:build !(js && wasm)

package core

import (
	"errors"
	"fmt"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core/track"
	"github.com/gogpu/wgpu/internal/hal"
)

// transitionsTestAdapter is an adapter whose backend records transitions
// (Vulkan, DX12, GLES), so a pooled encoder may split its recording into a
// pass command buffer and a transit command buffer.
func transitionsTestAdapter() *Adapter {
	return &Adapter{halCapabilities: &hal.Capabilities{RecordsTransitions: true}}
}

// noTransitionsTestAdapter is an adapter whose backend records no transitions
// (Metal, software, noop): the core must keep one recording per encoder
// (P1-MTL-008, issue #428).
func noTransitionsTestAdapter() *Adapter {
	return &Adapter{halCapabilities: &hal.Capabilities{RecordsTransitions: false}}
}

// countingEncoder is a HAL command encoder that counts BeginEncoding and
// EndEncoding calls. When err is set, the begin whose number is failBegin
// returns it wrapped, as a backend that ran out of its command-buffer budget
// does (hal.ErrCommandBufferBudget).
type countingEncoder struct {
	*recordingEncoder
	begins    int
	ends      int
	failBegin int
	err       error
}

func newCountingEncoder() *countingEncoder {
	return &countingEncoder{recordingEncoder: newRecordingEncoder()}
}

func (c *countingEncoder) BeginEncoding(label string) error {
	c.begins++
	if c.err != nil && c.begins == c.failBegin {
		return fmt.Errorf("begin %q: %w", label, c.err)
	}
	return c.recordingEncoder.BeginEncoding(label)
}

func (c *countingEncoder) EndEncoding() (hal.CommandBuffer, error) {
	c.ends++
	return c.recordingEncoder.EndEncoding()
}

// encoderWith begins the encoder's first recording and wraps rec in a core
// encoder, the way the device's encoder pool does (the path a render pass may
// split on a transitions backend).
func encoderWith(t *testing.T, adapter *Adapter, rec *countingEncoder, label string) (*CoreCommandEncoder, *Device) {
	t.Helper()
	device := NewDevice(&mockHALDevice{}, adapter, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")
	if err := rec.BeginEncoding(label); err != nil {
		t.Fatalf("BeginEncoding: %v", err)
	}
	enc, err := device.CreateCommandEncoderWithHAL(rec, label)
	if err != nil {
		t.Fatalf("CreateCommandEncoderWithHAL: %v", err)
	}
	return enc, device
}

// recordBarrierNeedingPasses records n compute-then-render-pass pairs. Every
// render pass samples a texture the preceding dispatch writes, so every pass
// needs a transition barrier ahead of it.
func recordBarrierNeedingPasses(t *testing.T, enc *CoreCommandEncoder, storage *Texture, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		dispatchOnce(t, enc, textureUses(storage, track.TextureUsesStorageWrite))
		pass, err := enc.BeginRenderPass(&RenderPassDescriptor{})
		if err != nil {
			t.Fatalf("pass %d: BeginRenderPass: %v", i, err)
		}
		if err := pass.MergeBindGroup(textureUses(storage, track.TextureUsesResource)); err != nil {
			t.Fatalf("pass %d: MergeBindGroup: %v", i, err)
		}
		if err := pass.End(); err != nil {
			t.Fatalf("pass %d: End: %v", i, err)
		}
	}
}

// onlyBarriers reports whether a recording holds barrier commands only.
func onlyBarriers(events []string) bool {
	if len(events) == 0 {
		return false
	}
	for _, ev := range events {
		if ev != "texture barrier" && ev != "buffer barrier" {
			return false
		}
	}
	return true
}

// transitRecordings counts the recordings that hold barriers only.
func transitRecordings(rec *countingEncoder) int {
	n := 0
	for _, events := range rec.log {
		if onlyBarriers(events) {
			n++
		}
	}
	return n
}

// A backend whose encoder records no transitions keeps ONE recording for the
// whole encoder: 40 barrier-needing render passes split nothing and leave no
// transit recording, so a backend that turns each recording into an
// uncompleted command buffer (Metal's queue) cannot fill its budget and
// deadlock (P1-MTL-008, issue #428).
func TestCommandEncoder_NoTransitionsBackendKeepsOneRecording(t *testing.T) {
	t.Parallel()

	rec := newCountingEncoder()
	enc, _ := encoderWith(t, noTransitionsTestAdapter(), rec, "no-transitions")
	storage := newPassUsageTestTexture(enc.device, "storage")

	recordBarrierNeedingPasses(t, enc, storage, 40)

	cb, err := enc.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if got := len(cb.HalBufferList()); got != 1 {
		t.Errorf("command buffers = %d, want 1", got)
	}
	if rec.begins != 1 || rec.ends != 1 {
		t.Errorf("BeginEncoding/EndEncoding = %d/%d, want 1/1", rec.begins, rec.ends)
	}
	if got := transitRecordings(rec); got != 0 {
		t.Errorf("transit recordings = %d, want 0", got)
	}
}

// A backend whose encoder records transitions keeps the split: every
// barrier-needing render pass gets its own command buffer with the transit
// command buffer holding the barriers right before it.
func TestCommandEncoder_TransitionsBackendSplitsWithTransitOrdering(t *testing.T) {
	t.Parallel()

	rec := newCountingEncoder()
	enc, _ := encoderWith(t, transitionsTestAdapter(), rec, "transitions")
	storage := newPassUsageTestTexture(enc.device, "storage")

	recordBarrierNeedingPasses(t, enc, storage, 40)

	cb, err := enc.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if rec.begins != rec.ends {
		t.Fatalf("BeginEncoding/EndEncoding = %d/%d, want them equal", rec.begins, rec.ends)
	}
	if rec.begins <= 1 {
		t.Fatalf("recording was never split: BeginEncoding calls = %d", rec.begins)
	}
	if got := transitRecordings(rec); got != 40 {
		t.Errorf("transit recordings = %d, want 40", got)
	}

	ids := commandBufferIDs(t, cb)
	passes := 0
	for i, id := range ids {
		events := rec.log[id]
		if len(events) != 1 || events[0] != "render pass" {
			continue
		}
		passes++
		if i == 0 {
			t.Fatalf("pass command buffer %d is first in the list, no transit before it", id)
		}
		if prev := rec.log[ids[i-1]]; !onlyBarriers(prev) {
			t.Errorf("command buffer before pass %d = %v, want only barriers", id, prev)
		}
	}
	if passes != 40 {
		t.Errorf("pass command buffers = %d, want 40", passes)
	}
}

// A HAL encoder that cannot begin another recording because its command-buffer
// budget is exhausted (hal.ErrCommandBufferBudget) fails the encoder without
// losing the device: the error surfaces from Finish, and an Internal error
// scope captures it as an internal-class error (P1-MTL-008, issue #428).
func TestCommandEncoder_BudgetErrorIsInternalClass(t *testing.T) {
	t.Parallel()

	rec := newCountingEncoder()
	rec.err = hal.ErrCommandBufferBudget
	rec.failBegin = 2 // the first is the caller's, the second the split's reopen
	enc, device := encoderWith(t, transitionsTestAdapter(), rec, "budget")
	if err := device.PushErrorScope(ErrorFilterInternal); err != nil {
		t.Fatalf("PushErrorScope: %v", err)
	}
	storage := newPassUsageTestTexture(device, "storage")

	dispatchOnce(t, enc, textureUses(storage, track.TextureUsesStorageWrite))
	if _, err := enc.BeginRenderPass(&RenderPassDescriptor{}); !errors.Is(err, hal.ErrCommandBufferBudget) {
		t.Fatalf("BeginRenderPass = %v, want the budget error", err)
	}

	if _, err := enc.Finish(); !errors.Is(err, hal.ErrCommandBufferBudget) {
		t.Fatalf("Finish = %v, want the budget error", err)
	}

	gpuErr, popErr := device.PopErrorScope()
	if popErr != nil {
		t.Fatalf("PopErrorScope: %v", popErr)
	}
	if gpuErr == nil {
		t.Fatal("Internal scope captured nothing, want the budget error")
	}
	if gpuErr.Type != ErrorFilterInternal {
		t.Errorf("captured Type = %v, want %v", gpuErr.Type, ErrorFilterInternal)
	}
	if !errors.Is(gpuErr, ErrInternal) {
		t.Errorf("errors.Is(gpuErr, ErrInternal) = false; the class identity must reach ErrInternal")
	}
}

// A Validation scope must not capture the budget error: it is not a
// validation failure, and the device is healthy.
func TestCommandEncoder_BudgetErrorSkipsValidationScope(t *testing.T) {
	t.Parallel()

	rec := newCountingEncoder()
	rec.err = hal.ErrCommandBufferBudget
	rec.failBegin = 2
	enc, device := encoderWith(t, transitionsTestAdapter(), rec, "budget-validation")
	if err := device.PushErrorScope(ErrorFilterValidation); err != nil {
		t.Fatalf("PushErrorScope: %v", err)
	}
	storage := newPassUsageTestTexture(device, "storage")

	dispatchOnce(t, enc, textureUses(storage, track.TextureUsesStorageWrite))
	if _, err := enc.BeginRenderPass(&RenderPassDescriptor{}); !errors.Is(err, hal.ErrCommandBufferBudget) {
		t.Fatalf("BeginRenderPass = %v, want the budget error", err)
	}

	gpuErr, popErr := device.PopErrorScope()
	if popErr != nil {
		t.Fatalf("PopErrorScope: %v", popErr)
	}
	if gpuErr != nil {
		t.Errorf("Validation scope captured %v, want nothing", gpuErr)
	}
}
