//go:build !rust && !(js && wasm)

package wgpu

import (
	"testing"

	"github.com/gogpu/wgpu/core/track"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/noop"
)

// labeledCB is a HAL command buffer that remembers the label its encoder was
// begun with (non-zero size, so every instance has its own identity).
type labeledCB struct {
	label string
}

func (*labeledCB) Destroy() {}

// labelingEncoder returns a labeledCB from EndEncoding.
type labelingEncoder struct {
	noop.CommandEncoder
	label string
}

func (e *labelingEncoder) BeginEncoding(label string) error {
	e.label = label
	return nil
}

func (e *labelingEncoder) EndEncoding() (hal.CommandBuffer, error) {
	return &labeledCB{label: e.label}, nil
}

type labelingDevice struct {
	noop.Device
}

func (*labelingDevice) CreateCommandEncoder(*hal.CommandEncoderDescriptor) (hal.CommandEncoder, error) {
	return &labelingEncoder{}, nil
}

// finishWithTextureUse finishes an empty command buffer whose usage tracker
// says it uses tex with use.
func finishWithTextureUse(t *testing.T, device *Device, label string, tex *Texture, use track.TextureUses) *CommandBuffer {
	t.Helper()
	enc, err := device.CreateCommandEncoder(&CommandEncoderDescriptor{Label: label})
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	cmd, err := enc.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	cmd.core.TextureScope().SetSingle(tex.coreTexture.TrackingData().Index(), use)
	cmd.usedTextures = map[*Texture]struct{}{tex: {}}
	return cmd
}

// TestAssembleSubmission_TransitBeforeEachCommandBuffer is the submit-order
// half of P1-CORE-008 (#371): pending writes run first, and every command
// buffer gets its own transit command buffer directly before it. The old code
// put one barrier command buffer with every command buffer's transitions in
// front of everything, so a transition out of a state left by the pending
// writes or by an earlier command buffer ran before that work.
// Rust: queue.rs submit — per baked command buffer a Transit pass +
// close_and_push_front; pending writes at index 0.
func TestAssembleSubmission_TransitBeforeEachCommandBuffer(t *testing.T) {
	device, err := NewDeviceFromHAL(&labelingDevice{}, &noop.Queue{}, 0, DefaultLimits(), "order")
	if err != nil {
		t.Fatalf("NewDeviceFromHAL: %v", err)
	}
	defer device.Release()
	q := device.Queue()

	tex, err := device.CreateTexture(&TextureDescriptor{
		Label:         "order",
		Size:          Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1},
		Dimension:     TextureDimension2D,
		Format:        TextureFormatRGBA8Unorm,
		Usage:         TextureUsageRenderAttachment | TextureUsageTextureBinding,
		MipLevelCount: 1,
		SampleCount:   1,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	defer tex.Release()
	idx := tex.coreTexture.TrackingData().Index()
	device.core.Tracker().InsertTexture(idx, track.TextureUsesResource)

	cb1 := finishWithTextureUse(t, device, "cb1", tex, track.TextureUsesColorTarget)
	cb2 := finishWithTextureUse(t, device, "cb2", tex, track.TextureUsesResource)

	pending := &labeledCB{label: "pending"}
	list, err := q.assembleSubmission(pending, []*CommandBuffer{cb1, cb2})
	if err != nil {
		t.Fatalf("assembleSubmission: %v", err)
	}
	defer q.releaseTransits()

	got := make([]string, 0, len(list))
	for _, cb := range list {
		got = append(got, cb.(*labeledCB).label)
	}
	want := []string{"pending", "barrier-inject", "cb1", "barrier-inject", "cb2"}
	if len(got) != len(want) {
		t.Fatalf("submission = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("submission = %v, want %v", got, want)
		}
	}
	if len(q.transits) != 2 {
		t.Errorf("transits kept for recycling = %d, want 2", len(q.transits))
	}
	if state := device.core.Tracker().TextureUsage(idx); state != track.TextureUsesResource {
		t.Errorf("device state after both command buffers = %d, want Resource (cb2's end)", state)
	}
}

var _ hal.CommandBuffer = (*labeledCB)(nil)
