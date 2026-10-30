//go:build !(js && wasm)

package core

import (
	"sync"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/core/track"
)

// submitTextureUse merges a command buffer that uses the texture index with
// usage into the device tracker, as Queue.Submit does, and returns the
// barriers recorded before that command buffer.
func submitTextureUse(dt *DeviceTracker, idx track.TrackerIndex, usage track.TextureUses) []track.TexturePendingTransition {
	enc := track.NewEncoderTextureTracker()
	enc.SetSingle(idx, usage)
	return dt.MergeTextureScope(enc)
}

func submitBufferUse(dt *DeviceTracker, idx track.TrackerIndex, usage track.BufferUses) []track.PendingTransition {
	enc := track.NewEncoderBufferTracker()
	enc.SetSingle(idx, usage)
	return dt.MergeBufferScope(enc)
}

func newTrackerTestTexture(device *Device, label string) *Texture {
	return NewTexture(&noopTexture{}, device, gputypes.TextureFormatRGBA8Unorm,
		gputypes.TextureDimension2D, gputypes.TextureUsageCopySrc|gputypes.TextureUsageCopyDst,
		gputypes.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1}, 1, 1, label)
}

// TestDeviceTracker_ReusedTextureIndexStartsUninitialized is the P1-CORE-009
// regression (#365): a texture created with the tracker index of a destroyed
// texture must not inherit the destroyed texture's device state. Before the
// fix the slot kept COPY_SRC, so the new image's first copy recorded
// COPY_SRC → COPY_DST on an image still in VK_IMAGE_LAYOUT_UNDEFINED (VVL
// VUID-VkImageMemoryBarrier-oldLayout-01197). Rust overwrites the slot at
// creation: device/resource.rs create_texture → insert_single(UNINITIALIZED).
func TestDeviceTracker_ReusedTextureIndexStartsUninitialized(t *testing.T) {
	t.Parallel()

	device := NewDevice(&mockHALDevice{}, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")
	dt := device.Tracker()

	a := newTrackerTestTexture(device, "A")
	idx := a.TrackingData().Index()
	if got := dt.TextureUsage(idx); got != track.TextureUsesUninitialized {
		t.Fatalf("new texture device state = %d, want Uninitialized", got)
	}
	submitTextureUse(dt, idx, track.TextureUsesCopySrc)
	a.Destroy()

	b := newTrackerTestTexture(device, "B")
	if b.TrackingData().Index() != idx {
		t.Fatalf("B index = %d, want reused index %d", b.TrackingData().Index(), idx)
	}
	transitions := submitTextureUse(dt, idx, track.TextureUsesCopyDst)
	if len(transitions) != 1 {
		t.Fatalf("transitions = %d, want 1", len(transitions))
	}
	if from := transitions[0].Usage.From; from != track.TextureUsesUninitialized {
		t.Errorf("B first barrier from = %d, want Uninitialized (%d)", from, track.TextureUsesUninitialized)
	}
}

// TestDeviceTracker_NewTextureFirstUseHasBarrier verifies that a fresh index
// gets an Uninitialized → first-usage barrier instead of being adopted with
// no barrier (the image would be used in UNDEFINED layout).
func TestDeviceTracker_NewTextureFirstUseHasBarrier(t *testing.T) {
	t.Parallel()

	device := NewDevice(&mockHALDevice{}, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")
	tex := newTrackerTestTexture(device, "fresh")

	transitions := submitTextureUse(device.Tracker(), tex.TrackingData().Index(), track.TextureUsesCopyDst)
	if len(transitions) != 1 || transitions[0].Usage.From != track.TextureUsesUninitialized ||
		transitions[0].Usage.To != track.TextureUsesCopyDst {
		t.Fatalf("transitions = %+v, want one Uninitialized → CopyDst", transitions)
	}
}

// TestDeviceTracker_BufferIndexReusedAfterDestroy is the buffer side of
// P1-CORE-009 (closed PR #361): Destroy must return the tracker index, and
// the buffer that reuses it starts from an empty device state.
func TestDeviceTracker_BufferIndexReusedAfterDestroy(t *testing.T) {
	t.Parallel()

	device := NewDevice(&mockHALDevice{}, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")
	dt := device.Tracker()

	a := NewBuffer(&noopHALBuffer{}, device, gputypes.BufferUsageStorage|gputypes.BufferUsageCopySrc, 64, "A")
	idx := a.TrackingData().Index()
	submitBufferUse(dt, idx, track.BufferUsesStorageWrite)
	a.Destroy()

	b := NewBuffer(&noopHALBuffer{}, device, gputypes.BufferUsageCopySrc, 64, "B")
	if b.TrackingData().Index() != idx {
		t.Fatalf("B index = %d, want reused index %d", b.TrackingData().Index(), idx)
	}
	if got := dt.BufferUsage(idx); got != track.BufferUsesNone {
		t.Errorf("B device state = %d, want None", got)
	}
	if n := device.TrackerIndices().Buffers().Size(); n != 1 {
		t.Errorf("live buffer indices = %d, want 1", n)
	}
}

// TestDeviceTracker_ConcurrentCreateDestroyAndMerge exercises the tracker
// lock: resources are created and destroyed on several goroutines while
// another merges command buffers, as Queue.Submit does. Run with -race.
func TestDeviceTracker_ConcurrentCreateDestroyAndMerge(t *testing.T) {
	t.Parallel()

	device := NewDevice(&mockHALDevice{}, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")
	dt := device.Tracker()

	const workers, iterations = 4, 200
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				tex := newTrackerTestTexture(device, "tex")
				buf := NewBuffer(&noopHALBuffer{}, device, gputypes.BufferUsageCopySrc, 16, "buf")
				tex.Destroy()
				buf.Destroy()
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < workers*iterations; i++ {
			idx := track.TrackerIndex(i % 8)
			submitTextureUse(dt, idx, track.TextureUsesCopySrc)
			submitBufferUse(dt, idx, track.BufferUsesCopySrc)
		}
	}()
	wg.Wait()
}
