// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package rtexample

import (
	"context"
	"encoding/binary"
	"fmt"
	"image"
	"log/slog"
	"math"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// validationErrors counts backend validation errors logged since InitDevice.
var validationErrors atomic.Int64

// validationHandler prints wgpu warnings and errors to stderr and counts
// validation-layer errors (logged at Error level with a "vulkan: " prefix).
type validationHandler struct {
	slog.Handler
}

func (h validationHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= slog.LevelWarn
}

func (h validationHandler) Handle(ctx context.Context, rec slog.Record) error { //nolint:gocritic // signature fixed by slog.Handler
	if rec.Level >= slog.LevelError && strings.HasPrefix(rec.Message, "vulkan: ") {
		validationErrors.Add(1)
	}
	return h.Handler.Handle(ctx, rec)
}

// CheckValidation returns an error if any validation errors were logged
// since InitDevice. It always succeeds when GOGPU_DEBUG is unset.
func CheckValidation() error {
	if n := validationErrors.Load(); n > 0 {
		return fmt.Errorf("%d validation error(s) logged", n)
	}
	return nil
}

// InitDevice creates an instance, adapter and device with the given required
// features and limits. GOGPU_GRAPHICS_API selects the backend, as in the
// other examples. If GOGPU_DEBUG is set, validation layers are requested,
// their messages are printed to stderr, and CheckValidation reports errors.
//
// It fails with a descriptive error if the adapter does not support every
// required feature, mirroring the Rust example framework's adapter filter.
func InitDevice(features gputypes.Features, limits gputypes.Limits) (*wgpu.Device, func(), error) {
	backends := wgpu.BackendsAll
	switch os.Getenv("GOGPU_GRAPHICS_API") {
	case "dx12", "d3d12":
		backends = wgpu.BackendsDX12
	case "vulkan", "vk":
		backends = wgpu.BackendsVulkan
	case "metal":
		backends = wgpu.BackendsMetal
	case "gl", "gles":
		backends = wgpu.BackendsGL
	}
	var flags gputypes.InstanceFlags
	if os.Getenv("GOGPU_DEBUG") != "" {
		flags |= gputypes.InstanceFlagsDebug
		wgpu.SetLogger(slog.New(validationHandler{slog.NewTextHandler(os.Stderr, nil)}))
	}

	instance, err := wgpu.CreateInstance(&wgpu.InstanceDescriptor{Backends: backends, Flags: flags})
	if err != nil {
		return nil, nil, fmt.Errorf("CreateInstance: %w", err)
	}
	adapter, err := instance.RequestAdapter(nil)
	if err != nil {
		instance.Release()
		return nil, nil, fmt.Errorf("RequestAdapter: %w", err)
	}
	info := adapter.Info()
	fmt.Printf("Adapter: %s (%v)\n", info.Name, info.Backend)

	if missing := features &^ adapter.Features(); missing != 0 {
		adapter.Release()
		instance.Release()
		return nil, nil, fmt.Errorf("adapter %q is missing required features %#x", info.Name, uint64(missing))
	}

	device, err := adapter.RequestDevice(&wgpu.DeviceDescriptor{
		Label:            "rt-example",
		RequiredFeatures: features,
		RequiredLimits:   limits,
	})
	if err != nil {
		adapter.Release()
		instance.Release()
		return nil, nil, fmt.Errorf("RequestDevice: %w", err)
	}
	cleanup := func() {
		device.Release()
		adapter.Release()
		instance.Release()
	}
	return device, cleanup, nil
}

// Target is an offscreen color render target plus a readback buffer. It plays
// the role of the surface texture in the Rust example framework's test mode,
// which renders one frame into an RGBA8 texture and compares it to a
// screenshot.
type Target struct {
	Width, Height uint32
	Format        gputypes.TextureFormat
	Texture       *wgpu.Texture
	View          *wgpu.TextureView

	readback    *wgpu.Buffer
	bytesPerRow uint32
}

// NewTarget creates a render target of the given size and format. format must
// be a 4-byte-per-texel format (RGBA8Unorm or RGBA8UnormSrgb).
func NewTarget(device *wgpu.Device, width, height uint32, format gputypes.TextureFormat) (*Target, error) {
	tex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "destination",
		Size:          wgpu.Extent3D{Width: width, Height: height, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        format,
		Usage:         gputypes.TextureUsageRenderAttachment | gputypes.TextureUsageCopySrc,
	})
	if err != nil {
		return nil, fmt.Errorf("create target texture: %w", err)
	}
	view, err := device.CreateTextureView(tex, nil)
	if err != nil {
		tex.Release()
		return nil, fmt.Errorf("create target view: %w", err)
	}
	bytesPerRow := alignUp(width*4, 256)
	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "image map buffer",
		Size:  uint64(bytesPerRow) * uint64(height),
		Usage: wgpu.BufferUsageCopyDst | wgpu.BufferUsageMapRead,
	})
	if err != nil {
		view.Release()
		tex.Release()
		return nil, fmt.Errorf("create readback buffer: %w", err)
	}
	return &Target{
		Width: width, Height: height, Format: format,
		Texture: tex, View: view,
		readback: buf, bytesPerRow: bytesPerRow,
	}, nil
}

// Release frees the target's GPU resources.
func (t *Target) Release() {
	t.readback.Release()
	t.View.Release()
	t.Texture.Release()
}

// Read copies the target texture to the readback buffer, waits for the GPU,
// and returns the pixels. The bytes are returned as stored in the texture;
// for an sRGB format they are already sRGB-encoded, which is what the Rust
// framework writes to its PNGs.
func (t *Target) Read(device *wgpu.Device) (*image.NRGBA, error) {
	encoder, err := device.CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{Label: "readback"})
	if err != nil {
		return nil, fmt.Errorf("create encoder: %w", err)
	}
	encoder.CopyTextureToBuffer(t.Texture, t.readback, []wgpu.BufferTextureCopy{{
		BufferLayout: wgpu.ImageDataLayout{BytesPerRow: t.bytesPerRow, RowsPerImage: t.Height},
		TextureBase:  wgpu.ImageCopyTexture{Texture: t.Texture},
		Size:         wgpu.Extent3D{Width: t.Width, Height: t.Height, DepthOrArrayLayers: 1},
	}})
	cmd, err := encoder.Finish()
	if err != nil {
		return nil, fmt.Errorf("finish encoder: %w", err)
	}
	if _, err := device.Queue().Submit(cmd); err != nil {
		return nil, fmt.Errorf("submit: %w", err)
	}

	size := t.readback.Size()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := t.readback.Map(ctx, wgpu.MapModeRead, 0, size); err != nil {
		return nil, fmt.Errorf("map readback: %w", err)
	}
	rng, err := t.readback.MappedRange(0, size)
	if err != nil {
		_ = t.readback.Unmap()
		return nil, fmt.Errorf("mapped range: %w", err)
	}
	src := rng.Bytes()
	img := image.NewNRGBA(image.Rect(0, 0, int(t.Width), int(t.Height)))
	rowBytes := int(t.Width) * 4
	for y := range int(t.Height) {
		copy(img.Pix[y*img.Stride:y*img.Stride+rowBytes], src[y*int(t.bytesPerRow):])
	}
	if err := t.readback.Unmap(); err != nil {
		return nil, fmt.Errorf("unmap: %w", err)
	}
	return img, nil
}

func alignUp(n, a uint32) uint32 { return (n + a - 1) / a * a }

// CreateBufferInit is the equivalent of wgpu::util::DeviceExt::create_buffer_init:
// it creates a buffer mapped at creation, copies contents in, and unmaps it.
// Like Rust, it pads the size up to a multiple of 4 (COPY_BUFFER_ALIGNMENT).
func CreateBufferInit(device *wgpu.Device, label string, contents []byte, usage gputypes.BufferUsage) (*wgpu.Buffer, error) {
	size := uint64(alignUp(uint32(len(contents)), 4)) //nolint:gosec // example buffers are far below 4 GiB
	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label:            label,
		Size:             size,
		Usage:            usage,
		MappedAtCreation: true,
	})
	if err != nil {
		return nil, err
	}
	rng, err := buf.MappedRange(0, size)
	if err != nil {
		buf.Release()
		return nil, fmt.Errorf("mapped range: %w", err)
	}
	copy(rng.BytesMut(), contents)
	if err := buf.Unmap(); err != nil {
		buf.Release()
		return nil, fmt.Errorf("unmap: %w", err)
	}
	return buf, nil
}

// F32Bytes returns v as little-endian bytes (bytemuck::cast_slice).
func F32Bytes(v []float32) []byte {
	b := make([]byte, 0, 4*len(v))
	for _, f := range v {
		b = binary.LittleEndian.AppendUint32(b, math.Float32bits(f))
	}
	return b
}

// U32Bytes returns v as little-endian bytes (bytemuck::cast_slice).
func U32Bytes(v []uint32) []byte {
	b := make([]byte, 0, 4*len(v))
	for _, u := range v {
		b = binary.LittleEndian.AppendUint32(b, u)
	}
	return b
}
