//go:build !rust && !(js && wasm)

package queue

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/core/track"
	"github.com/gogpu/wgpu/internal/encoder"
	"github.com/gogpu/wgpu/internal/hal"
	"github.com/gogpu/wgpu/internal/resource"
)

// These doubles moved here with the test (ADR-070 S9). The root package's
// version embedded internal/hal/noop; a test import is a graph edge, and the
// golden has no queue -> hal/noop row, so the destination declares its own.

// labeledCB is a HAL command buffer that remembers the label its encoder was
// begun with (non-zero size, so every instance has its own identity).
type labeledCB struct {
	label string
}

func (*labeledCB) Destroy() {}

// labelingEncoder returns a labeledCB from EndEncoding.
type labelingEncoder struct {
	testHALCommandEncoder
	label string
}

func (e *labelingEncoder) BeginEncoding(label string) error {
	e.label = label
	return nil
}

func (e *labelingEncoder) EndEncoding() (hal.CommandBuffer, error) {
	return &labeledCB{label: e.label}, nil
}

// labelingDevice hands out labelingEncoders.
type labelingDevice struct {
	testHALDevice
}

func (*labelingDevice) CreateCommandEncoder(*hal.CommandEncoderDescriptor) (hal.CommandEncoder, error) {
	return &labelingEncoder{}, nil
}

// labelingPool is the encoder pool whose encoders label the command buffers
// they produce, so the test can read the submission order back.
type labelingPool struct{}

var _ core.HalEncoderPool = labelingPool{}

func (labelingPool) Acquire() (hal.CommandEncoder, error) { return &labelingEncoder{}, nil }
func (labelingPool) Release(hal.CommandEncoder)           {}
func (labelingPool) Retire(hal.CommandEncoder)            {}

// finishWithTextureUse finishes an empty command buffer whose usage tracker
// says it uses tex with use.
func finishWithTextureUse(t *testing.T, device *core.Device, host *testHost, label string, tex *resource.Texture, use track.TextureUses) *encoder.CommandBuffer {
	t.Helper()
	coreEnc, err := device.CreateCommandEncoder(label)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	coreCB, err := coreEnc.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	coreCB.TextureScope().SetSingle(tex.CoreTexture().TrackingData().Index(), use)
	out := encoder.NewTestCommandBuffer(coreCB, host)
	out.SetUsedSetsForTest(nil, map[*resource.Texture]struct{}{tex: {}}, nil)
	return out
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
	raw := &labelingDevice{}
	device := core.NewDevice(raw, nil, 0, gputypes.Limits{}, "order")
	t.Cleanup(device.Destroy)

	host := &testHost{core: device, raw: raw, pool: labelingPool{}}
	q := newTestQueue(host)

	tex := newTestTrackedTexture(t, device, host, "order")
	idx := tex.CoreTexture().TrackingData().Index()
	device.Tracker().InsertTexture(idx, track.TextureUsesResource)

	cb1 := finishWithTextureUse(t, device, host, "cb1", tex, track.TextureUsesColorTarget)
	cb2 := finishWithTextureUse(t, device, host, "cb2", tex, track.TextureUsesResource)

	pending := &labeledCB{label: "pending"}
	list, err := q.assembleSubmission(pending, []*encoder.CommandBuffer{cb1, cb2})
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
	if state := device.Tracker().TextureUsage(idx); state != track.TextureUsesResource {
		t.Errorf("device state after both command buffers = %d, want Resource (cb2's end)", state)
	}
}

var _ hal.CommandBuffer = (*labeledCB)(nil)
