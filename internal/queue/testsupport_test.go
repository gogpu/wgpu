//go:build !rust && !(js && wasm)

package queue

// The fixtures of the queue domain's own tests. They build the internal
// objects the queue submits — a core device with a tracker, a device host, a
// HAL encoder pool, tracked textures and command buffers — instead of the root
// package's public wrappers, which the destination cannot import.
//
// The HAL doubles are local on purpose: the queue package's tests may not
// reach into internal/hal/noop, because a test import is a graph edge like any
// other and the golden target rows record the production edges of the domain
// (layoutcheck golden-subset). The embedded interfaces keep the full method
// set satisfied; a call the fixture does not expect panics, which is what a
// test double should do.

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/encoder"
	"github.com/gogpu/wgpu/internal/hal"
	"github.com/gogpu/wgpu/internal/resource"
	"github.com/gogpu/wgpu/internal/stencilblit"
)

// testHALTexture is a HAL texture: the embedded interface satisfies the full
// method set, and the fixture never calls anything through it.
type testHALTexture struct{ hal.Texture }

// testHALDevice is the minimal HAL device the fixtures need.
type testHALDevice struct{ hal.Device }

func (testHALDevice) CreateCommandEncoder(*hal.CommandEncoderDescriptor) (hal.CommandEncoder, error) {
	return testHALCommandEncoder{}, nil
}
func (testHALDevice) Destroy()                           {}
func (testHALDevice) DestroyBuffer(hal.Buffer)           {}
func (testHALDevice) DestroyTexture(hal.Texture)         {}
func (testHALDevice) DestroyTextureView(hal.TextureView) {}

// testHALCommandEncoder is the minimal HAL command encoder: the encoding
// lifecycle the core encoder and the barrier recorder drive, with no recording
// behind it.
type testHALCommandEncoder struct{ hal.CommandEncoder }

func (testHALCommandEncoder) BeginEncoding(string) error { return nil }
func (testHALCommandEncoder) EndEncoding() (hal.CommandBuffer, error) {
	return testHALCommandBuffer{}, nil
}
func (testHALCommandEncoder) DiscardEncoding()                        {}
func (testHALCommandEncoder) ResetAll([]hal.CommandBuffer)            {}
func (testHALCommandEncoder) Destroy()                                {}
func (testHALCommandEncoder) TransitionBuffers([]hal.BufferBarrier)   {}
func (testHALCommandEncoder) TransitionTextures([]hal.TextureBarrier) {}
func (testHALCommandEncoder) ClearBuffer(hal.Buffer, uint64, uint64)  {}

// testHALCommandBuffer is a HAL command buffer with nothing recorded.
type testHALCommandBuffer struct{ hal.CommandBuffer }

func (testHALCommandBuffer) Destroy() {}

// testHALQueue is the HAL queue double: it counts the uploads and submissions
// that reach it and reports a completion index the test controls. An upload
// error can be installed to keep the backend-error path covered (the double
// otherwise always succeeds, so nothing else would exercise the propagation).
type testHALQueue struct {
	hal.Queue
	submitCalls     int
	writeTextures   int
	completed       uint64
	writeTextureErr error
}

func (q *testHALQueue) Submit([]hal.CommandBuffer) (uint64, error) {
	q.submitCalls++
	return uint64(q.submitCalls), nil
}

func (q *testHALQueue) PollCompleted() uint64 { return q.completed }

func (q *testHALQueue) WriteTexture(*hal.ImageCopyTexture, []byte, *hal.ImageDataLayout, *hal.Extent3D) error {
	q.writeTextures++
	return q.writeTextureErr
}

// testEncoderPool is the HalEncoderPool double: Acquire hands out the minimal
// HAL encoder and the pool counts the acquisitions.
type testEncoderPool struct{ acquired int }

var _ core.HalEncoderPool = (*testEncoderPool)(nil)

func (p *testEncoderPool) Acquire() (hal.CommandEncoder, error) {
	p.acquired++
	return testHALCommandEncoder{}, nil
}

func (p *testEncoderPool) Release(hal.CommandEncoder) {}

func (p *testEncoderPool) Retire(hal.CommandEncoder) {}

// testHost is both seams at once: the queue's DeviceHost and the encoder's, so
// a command buffer built for a test can be submitted by the queue under test.
type testHost struct {
	core     *core.Device
	raw      hal.Device
	pool     core.HalEncoderPool
	features gputypes.Features
}

var (
	_ DeviceHost         = (*testHost)(nil)
	_ encoder.DeviceHost = (*testHost)(nil)
)

// errTestNoStencilUpload is what the fixture host answers for a stencil
// upload: the queue tests never create one.
var errTestNoStencilUpload = errors.New("test device host: no stencil upload")

func (h *testHost) Core() *core.Device               { return h.core }
func (h *testHost) IsReleased() bool                 { return false }
func (h *testHost) HALDevice() hal.Device            { return h.raw }
func (h *testHost) DestroyQueue() *core.DestroyQueue { return nil }
func (h *testHost) LastSubmissionIndex() uint64      { return 0 }
func (h *testHost) Poll(core.PollType) bool          { return true }

// Recordings is nil: the fixtures build command buffers through
// encoder.NewTestCommandBuffer, so the device's registry plays no part.
func (h *testHost) Recordings() core.EncoderRecordings[*encoder.CommandEncoder, *encoder.CommandBuffer] {
	return nil
}

// EncoderPool returns an untyped nil when the fixture has no pool: the queue's
// nil checks must keep their meaning across the interface. A nil field is a
// nil interface, never a typed nil, because the fixtures assign either a
// concrete pool or nothing.
func (h *testHost) EncoderPool() core.HalEncoderPool {
	return h.pool
}

// CreateCommandEncoder answers a bare encoder handle: a queue test that needs
// a recording encoder builds it through the core device instead.
func (h *testHost) CreateCommandEncoder(string) (*encoder.CommandEncoder, error) {
	return &encoder.CommandEncoder{}, nil
}

func (h *testHost) NewStencilUpload(*resource.Texture, uint32, gputypes.Origin3D, gputypes.Extent3D) (*stencilblit.Upload, error) {
	return nil, errTestNoStencilUpload
}

func (h *testHost) Features() gputypes.Features { return h.features }

// newTestHost creates a core device over the test HAL device and the host that
// adapts it. withPool installs the encoder pool the barrier injection acquires
// from.
func newTestHost(t *testing.T, withPool bool) (*testHost, *core.Device) {
	t.Helper()
	raw := testHALDevice{}
	device := core.NewDevice(raw, nil, 0, gputypes.Limits{}, "queue-test")
	t.Cleanup(device.Destroy)
	host := &testHost{core: device, raw: raw}
	if withPool {
		host.pool = &testEncoderPool{}
	}
	return host, device
}

// newTestQueue returns a queue that records against host and has no HAL
// backend: the tests that need one build it directly.
func newTestQueue(host DeviceHost) *Queue {
	q := New(nil, nil)
	q.SetDevice(host)
	return q
}

// newTestTrackedTexture builds a texture with a valid core texture and tracker
// index, the state the barrier and submit paths read.
func newTestTrackedTexture(t *testing.T, device *core.Device, view core.DeviceView, label string) *resource.Texture {
	t.Helper()
	coreTex := core.NewTexture(
		testHALTexture{}, device,
		gputypes.TextureFormatRGBA8Unorm,
		gputypes.TextureDimension2D,
		gputypes.TextureUsageTextureBinding|gputypes.TextureUsageRenderAttachment,
		gputypes.Extent3D{Width: 64, Height: 64, DepthOrArrayLayers: 1},
		1, 1, label,
	)
	t.Cleanup(coreTex.Destroy)
	tex := resource.NewTexture(testHALTexture{}, view, gputypes.TextureFormatRGBA8Unorm, nil, coreTex)
	// The root package's version of this fixture created its texture through
	// the public device and reported a creation failure; the destination builds
	// the core texture directly, so the same precondition is checked here.
	if tex.CoreTexture() == nil || !tex.CoreTexture().TrackingData().Index().IsValid() {
		t.Fatalf("texture %q has no tracked core texture", label)
	}
	return tex
}
