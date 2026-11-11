//go:build !rust && !(js && wasm) && !wgpu_nosoftware

package wgputest_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/noop"
	"github.com/gogpu/wgpu/wgputest"
)

// TestImportDoesNotRegisterNoop guards the factory's private noop instance:
// importing wgputest must not put the noop backend into the global HAL
// registry, where it would replace the software backend under BackendEmpty.
func TestImportDoesNotRegisterNoop(t *testing.T) {
	backend, ok := hal.GetBackend(gputypes.BackendEmpty)
	if !ok {
		t.Skip("no BackendEmpty backend registered in this build")
	}
	if _, isNoop := backend.(noop.Backend); isNoop {
		t.Fatal("importing wgputest registered the noop backend over software")
	}
}

// TestSoftwareDeviceRoundTrip renders nothing but proves the software device
// is a real wgpu device: a buffer written through the queue maps back with the
// same bytes. It also checks the device/queue pairing and CheckHealth.
func TestSoftwareDeviceRoundTrip(t *testing.T) {
	device, queue, cleanup, err := wgputest.NewSoftwareDevice()
	if err != nil {
		t.Fatalf("NewSoftwareDevice: %v", err)
	}
	defer cleanup()
	if device == nil || queue == nil {
		t.Fatal("NewSoftwareDevice returned a nil device or queue")
	}
	if device.Queue() != queue {
		t.Error("device.Queue() is not the returned queue")
	}
	if err := device.CheckHealth(); err != nil {
		t.Errorf("CheckHealth: %v", err)
	}

	const size = 64
	buffer, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "round-trip",
		Size:  size,
		Usage: wgpu.BufferUsageMapRead | wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer buffer.Release()

	want := make([]byte, size)
	for i := range want {
		want[i] = byte(i*3 + 1)
	}
	if err := queue.WriteBuffer(buffer, 0, want); err != nil {
		t.Fatalf("WriteBuffer: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := buffer.Map(ctx, wgpu.MapModeRead, 0, size); err != nil {
		t.Fatalf("Map: %v", err)
	}
	rng, err := buffer.MappedRange(0, size)
	if err != nil {
		_ = buffer.Unmap()
		t.Fatalf("MappedRange: %v", err)
	}
	got := rng.Bytes()
	if len(got) != size {
		t.Fatalf("mapped bytes: got %d, want %d", len(got), size)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("byte %d: got %#x, want %#x", i, got[i], want[i])
		}
	}
	if err := buffer.Unmap(); err != nil {
		t.Fatalf("Unmap: %v", err)
	}
}

// TestNoopDeviceRecorder proves the public seam records the calls the
// consumers assert on, with cross-kind order, encoder identity and per-pass
// load ops.
func TestNoopDeviceRecorder(t *testing.T) {
	recorder := wgputest.NewRecorder()
	device, queue, cleanup, err := wgputest.NewNoopDevice(wgputest.WithRecorder(recorder))
	if err != nil {
		t.Fatalf("NewNoopDevice: %v", err)
	}
	defer cleanup()
	if err := device.CheckHealth(); err != nil {
		t.Errorf("CheckHealth: %v", err)
	}

	buffer, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "vertex",
		Size:  64,
		Usage: wgpu.BufferUsageVertex,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer buffer.Release()

	texture, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "target",
		Size:          wgpu.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         gputypes.TextureUsageRenderAttachment,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	defer texture.Release()
	view, err := device.CreateTextureView(texture, nil)
	if err != nil {
		t.Fatalf("CreateTextureView: %v", err)
	}
	defer view.Release()

	encoder, err := device.CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{Label: "frame"})
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	pass, err := encoder.BeginRenderPass(&wgpu.RenderPassDescriptor{
		Label: "main",
		ColorAttachments: []wgpu.RenderPassColorAttachment{{
			View:       view,
			LoadOp:     gputypes.LoadOpClear,
			StoreOp:    gputypes.StoreOpStore,
			ClearValue: gputypes.Color{R: 1, A: 1},
		}},
	})
	if err != nil {
		t.Fatalf("BeginRenderPass: %v", err)
	}
	// Draw needs a pipeline (core validation); the bridge test covers the
	// draw records white-box. Here the public wiring is what is proved.
	pass.SetVertexBuffer(0, buffer, 8)
	pass.End()
	commandBuffer, err := encoder.Finish()
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if _, err := queue.Submit(commandBuffer); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	trace := recorder.Trace()

	bufferRecord := findBuffer(t, &trace, "vertex")
	if bufferRecord.Size != 64 {
		t.Errorf("buffer size = %d, want 64", bufferRecord.Size)
	}
	// Recording is on the public API: exactly the caller's encoder appears,
	// with the caller's label; core's pooled encoders never do.
	if len(trace.Encoders) != 1 {
		t.Fatalf("encoders = %+v, want one user encoder", trace.Encoders)
	}
	encoderRecord := findEncoder(t, &trace, "frame")
	passRecord := findPass(t, &trace, "main")
	if passRecord.Encoder != encoderRecord.Index {
		t.Errorf("pass encoder = %d, want %d", passRecord.Encoder, encoderRecord.Index)
	}
	if passRecord.Compute {
		t.Error("render pass recorded as compute")
	}
	if len(passRecord.LoadOps) != 1 || passRecord.LoadOps[0] != gputypes.LoadOpClear {
		t.Errorf("load ops = %v, want [Clear]", passRecord.LoadOps)
	}
	if len(passRecord.Clears) != 1 || passRecord.Clears[0].R != 1 || passRecord.Clears[0].A != 1 {
		t.Errorf("clears = %+v, want one red opaque", passRecord.Clears)
	}
	if len(trace.VertexBuffers) != 1 {
		t.Fatalf("vertex binds = %d, want 1", len(trace.VertexBuffers))
	}
	if bind := trace.VertexBuffers[0]; bind.Pass != passRecord.Index || bind.Slot != 0 ||
		bind.Label != "vertex" || bind.Offset != 8 || bind.Size != 64 {
		t.Errorf("vertex bind = %+v, want pass %d slot 0 label vertex offset 8 size 64", bind, passRecord.Index)
	}
	if len(trace.Draws) != 0 {
		t.Errorf("draws = %+v, want none (no pipeline set)", trace.Draws)
	}
	// One record per public Submit with the caller's command-buffer count;
	// the core's internal pending-write flush is not counted.
	if len(trace.Submits) != 1 || trace.Submits[0].CommandBuffers != 1 {
		t.Errorf("submits = %+v, want one submit of one command buffer", trace.Submits)
	}
	if bufferRecord.Seq >= passRecord.Seq {
		t.Errorf("seq order: buffer %d, pass %d", bufferRecord.Seq, passRecord.Seq)
	}
}

func findBuffer(t *testing.T, trace *wgputest.Trace, label string) wgputest.BufferCreate {
	t.Helper()
	for _, record := range trace.Buffers {
		if record.Label == label {
			return record
		}
	}
	t.Fatalf("buffer %q not recorded: %+v", label, trace.Buffers)
	return wgputest.BufferCreate{}
}

func findEncoder(t *testing.T, trace *wgputest.Trace, label string) wgputest.EncoderCreate {
	t.Helper()
	for _, record := range trace.Encoders {
		if record.Label == label {
			return record
		}
	}
	t.Fatalf("encoder %q not recorded: %+v", label, trace.Encoders)
	return wgputest.EncoderCreate{}
}

func findPass(t *testing.T, trace *wgputest.Trace, label string) wgputest.PassBegin {
	t.Helper()
	for _, record := range trace.Passes {
		if record.Label == label {
			return record
		}
	}
	t.Fatalf("pass %q not recorded: %+v", label, trace.Passes)
	return wgputest.PassBegin{}
}

// TestNoopDeviceFaults proves both injection points and that they are
// independent.
func TestNoopDeviceFaults(t *testing.T) {
	encoderErr := errors.New("encoder refused")
	writeErr := errors.New("write refused")
	device, queue, cleanup, err := wgputest.NewNoopDevice(wgputest.WithFaults(wgputest.Faults{
		CreateCommandEncoder: encoderErr,
		WriteBuffer:          writeErr,
	}))
	if err != nil {
		t.Fatalf("NewNoopDevice: %v", err)
	}
	defer cleanup()

	if _, err := device.CreateCommandEncoder(nil); !errors.Is(err, encoderErr) {
		t.Errorf("CreateCommandEncoder error = %v, want %v", err, encoderErr)
	}

	buffer, err := device.CreateBuffer(&wgpu.BufferDescriptor{Label: "fault", Size: 4, Usage: wgpu.BufferUsageCopyDst})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer buffer.Release()
	if err := queue.WriteBuffer(buffer, 0, []byte{1, 2, 3, 4}); !errors.Is(err, writeErr) {
		t.Errorf("WriteBuffer error = %v, want %v", err, writeErr)
	}
}
