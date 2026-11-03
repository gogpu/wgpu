//go:build !rust && !(js && wasm)

package wgpu

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/core"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/noop"
)

// copyRecordingDevice hands out encoders that count the copy commands that
// reach the backend.
type copyRecordingDevice struct {
	noop.Device
	encoder *copyRecordingEncoder
}

func (d *copyRecordingDevice) CreateCommandEncoder(_ *hal.CommandEncoderDescriptor) (hal.CommandEncoder, error) {
	d.encoder = &copyRecordingEncoder{}
	return d.encoder, nil
}

type copyRecordingEncoder struct {
	noop.CommandEncoder
	bufferToBuffer, bufferToTexture, textureToBuffer, textureToTexture int
	bufferToBufferRegions, bufferToTextureRegions                      int
	textureToBufferRegions, textureToTextureRegions                    int
	lastBufferToTexture, lastTextureToBuffer                           []hal.BufferTextureCopy
}

func (e *copyRecordingEncoder) CopyBufferToBuffer(_, _ hal.Buffer, regions []hal.BufferCopy) {
	e.bufferToBuffer++
	e.bufferToBufferRegions += len(regions)
}

func (e *copyRecordingEncoder) CopyBufferToTexture(_ hal.Buffer, _ hal.Texture, regions []hal.BufferTextureCopy) {
	e.bufferToTexture++
	e.bufferToTextureRegions += len(regions)
	e.lastBufferToTexture = regions
}

func (e *copyRecordingEncoder) CopyTextureToBuffer(_ hal.Texture, _ hal.Buffer, regions []hal.BufferTextureCopy) {
	e.textureToBuffer++
	e.textureToBufferRegions += len(regions)
	e.lastTextureToBuffer = regions
}

func (e *copyRecordingEncoder) CopyTextureToTexture(_, _ hal.Texture, regions []hal.TextureCopy) {
	e.textureToTexture++
	e.textureToTextureRegions += len(regions)
}

func newCopyRecordingDevice(t *testing.T) (*Device, *copyRecordingDevice) {
	t.Helper()
	raw := &copyRecordingDevice{}
	adapter := &core.Adapter{DownlevelCapabilities: gputypes.DefaultDownlevelCapabilities()}
	coreDevice := core.NewDevice(raw, adapter, 0, gputypes.DefaultLimits(), "copy-recording-test")
	queue := &Queue{hal: &noop.Queue{}, halDevice: raw}
	device := &Device{core: coreDevice, queue: queue}
	queue.device = device
	t.Cleanup(device.Release)
	return device, raw
}

type zeroSizeFixture struct {
	device         *Device
	raw            *copyRecordingDevice
	srcBuf, dstBuf *Buffer
	srcTex, dstTex *Texture
}

func newZeroSizeFixture(t *testing.T) *zeroSizeFixture {
	t.Helper()
	device, raw := newCopyRecordingDevice(t)
	f := &zeroSizeFixture{device: device, raw: raw}
	newBuffer := func() *Buffer {
		buf, err := device.CreateBuffer(&BufferDescriptor{Size: 4096, Usage: BufferUsageCopySrc | BufferUsageCopyDst})
		if err != nil {
			t.Fatalf("CreateBuffer: %v", err)
		}
		t.Cleanup(buf.Release)
		return buf
	}
	newTexture := func() *Texture {
		tex, err := device.CreateTexture(&TextureDescriptor{
			Size:          Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 2},
			MipLevelCount: 1,
			SampleCount:   1,
			Dimension:     TextureDimension2D,
			Format:        gputypes.TextureFormatRGBA8Unorm,
			Usage:         TextureUsageCopySrc | TextureUsageCopyDst,
		})
		if err != nil {
			t.Fatalf("CreateTexture: %v", err)
		}
		t.Cleanup(tex.Release)
		return tex
	}
	f.srcBuf, f.dstBuf, f.srcTex, f.dstTex = newBuffer(), newBuffer(), newTexture(), newTexture()
	return f
}

func (f *zeroSizeFixture) encoder(t *testing.T) *CommandEncoder {
	t.Helper()
	enc, err := f.device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	t.Cleanup(enc.DiscardEncoding)
	return enc
}

func bufferTextureRegion(w, h, d uint32) BufferTextureCopy {
	return BufferTextureCopy{
		BufferLayout: ImageDataLayout{BytesPerRow: 256, RowsPerImage: 4},
		Size:         Extent3D{Width: w, Height: h, DepthOrArrayLayers: d},
	}
}

func textureRegion(src, dst *Texture, w, h, d uint32) TextureCopy {
	return TextureCopy{
		Source:      ImageCopyTexture{Texture: src},
		Destination: ImageCopyTexture{Texture: dst},
		Size:        Extent3D{Width: w, Height: h, DepthOrArrayLayers: d},
	}
}

// requireNoSideEffects: a skipped copy leaves no tracking behind (Rust returns
// before the tracker is touched) and records no error.
func requireNoSideEffects(t *testing.T, enc *CommandEncoder) {
	t.Helper()
	if len(enc.trackedRefs) != 0 {
		t.Errorf("tracked refs = %d, want 0", len(enc.trackedRefs))
	}
	if len(enc.usedBuffers) != 0 {
		t.Errorf("used buffers = %d, want 0", len(enc.usedBuffers))
	}
	if _, err := enc.Finish(); err != nil {
		t.Errorf("Finish: %v", err)
	}
}

// TestZeroSizeCopiesAreSkipped: a copy of size zero is validated and then
// dropped before the backend, as Rust wgpu-core does ("Ignoring copy_* of size
// 0", transfer.rs); Vulkan rejects a zero extent.
func TestZeroSizeCopiesAreSkipped(t *testing.T) {
	t.Run("buffer to buffer", func(t *testing.T) {
		f := newZeroSizeFixture(t)
		enc := f.encoder(t)
		enc.CopyBufferToBuffer(f.srcBuf, 0, f.dstBuf, 0, 0)
		requireNoSideEffects(t, enc)
		if n := f.raw.encoder.bufferToBuffer; n != 0 {
			t.Errorf("backend CopyBufferToBuffer calls = %d, want 0", n)
		}
	})
	extents := []struct {
		name    string
		w, h, d uint32
	}{{"zero width", 0, 4, 1}, {"zero height", 4, 0, 1}, {"zero depth", 4, 4, 0}}
	for _, ext := range extents {
		t.Run("buffer to texture, "+ext.name, func(t *testing.T) {
			f := newZeroSizeFixture(t)
			enc := f.encoder(t)
			enc.CopyBufferToTexture(f.srcBuf, f.dstTex, []BufferTextureCopy{bufferTextureRegion(ext.w, ext.h, ext.d)})
			requireNoSideEffects(t, enc)
			if n := f.raw.encoder.bufferToTexture; n != 0 {
				t.Errorf("backend CopyBufferToTexture calls = %d, want 0", n)
			}
		})
		t.Run("texture to buffer, "+ext.name, func(t *testing.T) {
			f := newZeroSizeFixture(t)
			enc := f.encoder(t)
			region := bufferTextureRegion(ext.w, ext.h, ext.d)
			region.TextureBase.Texture = f.srcTex
			enc.CopyTextureToBuffer(f.srcTex, f.dstBuf, []BufferTextureCopy{region})
			requireNoSideEffects(t, enc)
			if n := f.raw.encoder.textureToBuffer; n != 0 {
				t.Errorf("backend CopyTextureToBuffer calls = %d, want 0", n)
			}
		})
		t.Run("texture to texture, "+ext.name, func(t *testing.T) {
			f := newZeroSizeFixture(t)
			enc := f.encoder(t)
			enc.CopyTextureToTexture(f.srcTex, f.dstTex, []TextureCopy{textureRegion(f.srcTex, f.dstTex, ext.w, ext.h, ext.d)})
			requireNoSideEffects(t, enc)
			if n := f.raw.encoder.textureToTexture; n != 0 {
				t.Errorf("backend CopyTextureToTexture calls = %d, want 0", n)
			}
		})
	}
}

// TestZeroSizeRegionsAreDroppedFromMixedCopies: only the empty regions of a
// multi-region copy are dropped; the others still reach the backend.
func TestZeroSizeRegionsAreDroppedFromMixedCopies(t *testing.T) {
	t.Run("buffer to texture", func(t *testing.T) {
		f := newZeroSizeFixture(t)
		enc := f.encoder(t)
		enc.CopyBufferToTexture(f.srcBuf, f.dstTex, []BufferTextureCopy{bufferTextureRegion(0, 4, 1), bufferTextureRegion(4, 4, 1)})
		if _, err := enc.Finish(); err != nil {
			t.Fatalf("Finish: %v", err)
		}
		if got := f.raw.encoder.bufferToTextureRegions; got != 1 {
			t.Errorf("backend regions = %d, want 1", got)
		}
	})
	t.Run("texture to buffer", func(t *testing.T) {
		f := newZeroSizeFixture(t)
		enc := f.encoder(t)
		empty, full := bufferTextureRegion(4, 0, 1), bufferTextureRegion(4, 4, 1)
		empty.TextureBase.Texture, full.TextureBase.Texture = f.srcTex, f.srcTex
		enc.CopyTextureToBuffer(f.srcTex, f.dstBuf, []BufferTextureCopy{empty, full})
		if _, err := enc.Finish(); err != nil {
			t.Fatalf("Finish: %v", err)
		}
		if got := f.raw.encoder.textureToBufferRegions; got != 1 {
			t.Errorf("backend regions = %d, want 1", got)
		}
	})
	t.Run("texture to texture", func(t *testing.T) {
		f := newZeroSizeFixture(t)
		enc := f.encoder(t)
		enc.CopyTextureToTexture(f.srcTex, f.dstTex, []TextureCopy{
			textureRegion(f.srcTex, f.dstTex, 4, 4, 0), textureRegion(f.srcTex, f.dstTex, 4, 4, 1),
		})
		if _, err := enc.Finish(); err != nil {
			t.Fatalf("Finish: %v", err)
		}
		if got := f.raw.encoder.textureToTextureRegions; got != 1 {
			t.Errorf("backend regions = %d, want 1", got)
		}
	})
}

// TestZeroSizeCopyStillValidated: the skip comes after validation, so an
// invalid zero-size copy is still an error.
func TestZeroSizeCopyStillValidated(t *testing.T) {
	f := newZeroSizeFixture(t)
	enc := f.encoder(t)
	enc.CopyBufferToBuffer(f.srcBuf, 2, f.dstBuf, 0, 0) // unaligned offset
	if _, err := enc.Finish(); err == nil {
		t.Fatal("Finish succeeded for an unaligned zero-size copy")
	}
	if n := f.raw.encoder.bufferToBuffer; n != 0 {
		t.Errorf("backend CopyBufferToBuffer calls = %d, want 0", n)
	}
}
