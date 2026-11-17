//go:build !rust && !(js && wasm)

package encoder

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/hal"
	"github.com/gogpu/wgpu/internal/resource"
	"github.com/gogpu/wgpu/internal/stencilblit"
)

// The fixtures of the encoder domain's own tests. They build the internal
// objects the encoder records against — a core device, a device view, tracked
// textures and buffers — instead of the root package's public wrappers, which
// the destination cannot import.
//
// The HAL doubles are local on purpose: the encoder package's tests may not
// reach into internal/hal/noop, because a test import is a graph edge like any
// other and the golden target rows record the production edges of the domain
// (layoutcheck golden-subset). The embedded interfaces keep the full method
// set satisfied; a call the fixture does not expect panics, which is what a
// test double should do.

// testHALTexture is a HAL texture: the embedded interface satisfies the full
// method set, and the fixture never calls anything through it.
type testHALTexture struct{ hal.Texture }

// testHALBuffer is a HAL buffer.
type testHALBuffer struct{ hal.Buffer }

// testHALView is a HAL texture view.
type testHALView struct{ hal.TextureView }

// testHALDevice is the minimal HAL device the fixtures need: it creates
// command encoders and destroys the doubles.
type testHALDevice struct{ hal.Device }

func (testHALDevice) CreateCommandEncoder(*hal.CommandEncoderDescriptor) (hal.CommandEncoder, error) {
	return testHALCommandEncoder{}, nil
}
func (testHALDevice) Destroy()                           {}
func (testHALDevice) DestroyBuffer(hal.Buffer)           {}
func (testHALDevice) DestroyTexture(hal.Texture)         {}
func (testHALDevice) DestroyTextureView(hal.TextureView) {}

// testHALCommandEncoder is the minimal HAL command encoder: the encoding
// lifecycle the core encoder drives, with no recording behind it.
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

// testDeviceView is a core.DeviceView over a bare core device and the test
// HAL device: exactly what the resource constructors read.
type testDeviceView struct {
	device *core.Device
	raw    hal.Device
}

func (v testDeviceView) Core() *core.Device               { return v.device }
func (v testDeviceView) IsReleased() bool                 { return false }
func (v testDeviceView) HALDevice() hal.Device            { return v.raw }
func (v testDeviceView) DestroyQueue() *core.DestroyQueue { return nil }
func (v testDeviceView) LastSubmissionIndex() uint64      { return 0 }
func (v testDeviceView) Poll(core.PollType) bool          { return true }

var _ core.DeviceView = testDeviceView{}

// newTestCoreDevice creates a core device over the test HAL device. Zero
// limits keep the indirect-validation pipeline (which would need a real HAL
// device) out of the fixture.
func newTestCoreDevice(t *testing.T) (*core.Device, testDeviceView) {
	t.Helper()
	raw := testHALDevice{}
	device := core.NewDevice(raw, nil, 0, gputypes.Limits{}, "encoder-test")
	t.Cleanup(device.Destroy)
	return device, testDeviceView{device: device, raw: raw}
}

// newTestCoreEncoder opens a core command encoder over the test device.
func newTestCoreEncoder(t *testing.T, device *core.Device) *core.CoreCommandEncoder {
	t.Helper()
	enc, err := device.CreateCommandEncoder("encoder-test")
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	return enc
}

// newTrackedTexture builds a texture with a valid core texture and tracker
// index, the state the copy-usage helpers read.
func newTrackedTexture(t *testing.T, device *core.Device, view testDeviceView, label string, usage gputypes.TextureUsage) *resource.Texture {
	t.Helper()
	return newTrackedTextureWithMipLevels(t, device, view, label, usage, 1)
}

// newTrackedTextureWithMipLevels returns a tracked 2D texture of mipLevels
// mip levels and one array layer.
func newTrackedTextureWithMipLevels(t *testing.T, device *core.Device, view testDeviceView, label string, usage gputypes.TextureUsage, mipLevels uint32) *resource.Texture {
	t.Helper()
	size := uint32(1) << (mipLevels - 1)
	coreTex := core.NewTexture(
		testHALTexture{}, device,
		gputypes.TextureFormatRGBA8Unorm,
		gputypes.TextureDimension2D,
		usage|gputypes.TextureUsageTextureBinding|gputypes.TextureUsageRenderAttachment,
		gputypes.Extent3D{Width: size, Height: size, DepthOrArrayLayers: 1},
		mipLevels, 1, label,
	)
	t.Cleanup(coreTex.Destroy)
	return resource.NewTexture(testHALTexture{}, view, gputypes.TextureFormatRGBA8Unorm, nil, coreTex)
}

// newTrackedBuffer builds a buffer with a valid core buffer, tracker index and
// reference count, the state the copy path reads and the encoder keeps alive.
func newTrackedBuffer(t *testing.T, device *core.Device, view testDeviceView, label string) *resource.Buffer {
	t.Helper()
	coreBuf := core.NewBuffer(
		testHALBuffer{}, device,
		gputypes.BufferUsageCopySrc|gputypes.BufferUsageCopyDst,
		256, label,
	)
	coreBuf.Ref = core.NewResourceRef("Buffer:"+label, coreBuf.Destroy)
	t.Cleanup(coreBuf.Destroy)
	return resource.NewBuffer(coreBuf, view)
}

// newTestTextureView builds a texture view over the test device with a HAL
// view, so ResolveHAL answers non-nil until the view is released.
func newTestTextureView(t *testing.T, device *core.Device, view testDeviceView, parent *resource.Texture) *resource.TextureView {
	t.Helper()
	return resource.NewTextureView(&resource.TextureViewInit{
		HAL:    testHALView{},
		Device: view,
		Parent: parent,
		Format: gputypes.TextureFormatRGBA8Unorm,
		Aspect: gputypes.TextureAspectAll,
	})
}

// testDeviceHost is the DeviceHost seam the pass tests call. Only Features is
// read by the render pass; the other seam methods are never called by it.
type testDeviceHost struct{ features gputypes.Features }

var _ DeviceHost = testDeviceHost{}

func (testDeviceHost) Recordings() core.EncoderRecordings[*CommandEncoder, *CommandBuffer] {
	return nil
}
func (testDeviceHost) EncoderPool() core.HalEncoderPool   { return nil }
func (testDeviceHost) ReportSubmitValidationError(string) {}
func (testDeviceHost) NewStencilUpload(*resource.Texture, uint32, gputypes.Origin3D, gputypes.Extent3D) (*stencilblit.Upload, error) {
	return nil, errTestNoStencilUpload
}
func (h testDeviceHost) Features() gputypes.Features { return h.features }

// errTestNoStencilUpload is what the fixture host answers for a stencil
// upload: the pass tests never create one.
var errTestNoStencilUpload = errors.New("test device host: no stencil upload")

// newTestResourceBuffer builds a buffer with a valid core buffer and a
// reference count, for the pass validation fixtures.
func newTestResourceBuffer(t *testing.T, device *core.Device, view testDeviceView, size uint64, usage gputypes.BufferUsage, label string) *resource.Buffer {
	t.Helper()
	coreBuf := core.NewBuffer(testHALBuffer{}, device, usage, size, label)
	coreBuf.Ref = core.NewResourceRef("Buffer:"+label, coreBuf.Destroy)
	t.Cleanup(coreBuf.Destroy)
	return resource.NewBuffer(coreBuf, view)
}

// newTestIndirectPass returns a pass encoder over a device host with features
// and no core pass: the indirect draw validation paths read the features and
// the deferred-error sink only.
func newTestIndirectPass(features gputypes.Features) (*RenderPassEncoder, *CommandEncoder) {
	enc := &CommandEncoder{device: testDeviceHost{features: features}}
	return &RenderPassEncoder{encoder: enc, pipelineSet: true}, enc
}
