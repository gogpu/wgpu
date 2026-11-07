//go:build !(js && wasm)

package software

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// submit ends enc's recording and submits the command buffer. The software
// backend runs recorded commands only at Submit, as the GPU backends do.
func submit(t testing.TB, enc hal.CommandEncoder) {
	t.Helper()
	cb, err := enc.EndEncoding()
	if err != nil {
		t.Fatalf("EndEncoding: %v", err)
	}
	if _, err := (&Queue{}).Submit([]hal.CommandBuffer{cb}); err != nil {
		t.Fatalf("Submit: %v", err)
	}
}

// newFilledBuffer returns a software buffer of len(data) bytes holding data.
func newFilledBuffer(t *testing.T, dev *Device, data []byte) *Buffer {
	t.Helper()
	b, err := dev.CreateBuffer(&hal.BufferDescriptor{
		Size:  uint64(len(data)),
		Usage: gputypes.BufferUsageCopySrc | gputypes.BufferUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	buf := b.(*Buffer)
	buf.WriteData(0, data)
	return buf
}

// TestCommandsRunAtSubmit records a copy and checks that it changes nothing
// while it is recorded, ended or waiting to be submitted, and lands when
// the command buffer is submitted.
func TestCommandsRunAtSubmit(t *testing.T) {
	dev, queue, cleanup := createSoftwareDevice(t)
	defer cleanup()

	src := newFilledBuffer(t, dev, bytes.Repeat([]byte{0xAB}, 16))
	dst := newFilledBuffer(t, dev, make([]byte, 16))

	enc, _ := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
	if err := enc.BeginEncoding("copy"); err != nil {
		t.Fatalf("BeginEncoding: %v", err)
	}
	enc.CopyBufferToBuffer(src, dst, []hal.BufferCopy{{Size: 16}})
	if got := dst.GetData(); !bytes.Equal(got, make([]byte, 16)) {
		t.Fatalf("dst after recording = %x, want zeros", got)
	}
	cb, err := enc.EndEncoding()
	if err != nil {
		t.Fatalf("EndEncoding: %v", err)
	}
	if got := dst.GetData(); !bytes.Equal(got, make([]byte, 16)) {
		t.Fatalf("dst after EndEncoding = %x, want zeros", got)
	}
	if _, err := queue.Submit([]hal.CommandBuffer{cb}); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if got := dst.GetData(); !bytes.Equal(got, src.GetData()) {
		t.Fatalf("dst after Submit = %x, want %x", got, src.GetData())
	}
}

// TestWriteBufferBeforeSubmitIsOverwritten writes dst through the queue
// after a copy into it is recorded but before it is submitted: the copy
// runs at Submit, after the write, and wins.
func TestWriteBufferBeforeSubmitIsOverwritten(t *testing.T) {
	dev, queue, cleanup := createSoftwareDevice(t)
	defer cleanup()

	src := newFilledBuffer(t, dev, bytes.Repeat([]byte{0x11}, 8))
	dst := newFilledBuffer(t, dev, make([]byte, 8))

	enc, _ := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
	_ = enc.BeginEncoding("copy")
	enc.CopyBufferToBuffer(src, dst, []hal.BufferCopy{{Size: 8}})
	cb, _ := enc.EndEncoding()
	if err := queue.WriteBuffer(dst, 0, bytes.Repeat([]byte{0x22}, 8)); err != nil {
		t.Fatalf("WriteBuffer: %v", err)
	}
	if _, err := queue.Submit([]hal.CommandBuffer{cb}); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if got := dst.GetData(); !bytes.Equal(got, src.GetData()) {
		t.Fatalf("dst = %x, want the copied %x", got, src.GetData())
	}
}

// TestDiscardEncodingDropsCommands checks that discarded commands never run,
// and that the encoder records a fresh command buffer afterwards.
func TestDiscardEncodingDropsCommands(t *testing.T) {
	dev, queue, cleanup := createSoftwareDevice(t)
	defer cleanup()

	buf := newFilledBuffer(t, dev, bytes.Repeat([]byte{0xFF}, 8))
	enc, _ := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
	_ = enc.BeginEncoding("discarded")
	enc.ClearBuffer(buf, 0, 4)
	enc.DiscardEncoding()

	_ = enc.BeginEncoding("kept")
	enc.ClearBuffer(buf, 4, 4)
	cb, _ := enc.EndEncoding()
	if _, err := queue.Submit([]hal.CommandBuffer{cb}); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	want := []byte{0xFF, 0xFF, 0xFF, 0xFF, 0, 0, 0, 0}
	if got := buf.GetData(); !bytes.Equal(got, want) {
		t.Fatalf("buffer = %x, want %x", got, want)
	}
}

// TestCommandBuffersRunInSubmitOrder submits two command buffers that write
// the same bytes: the later one in the list wins.
func TestCommandBuffersRunInSubmitOrder(t *testing.T) {
	dev, queue, cleanup := createSoftwareDevice(t)
	defer cleanup()

	first := newFilledBuffer(t, dev, bytes.Repeat([]byte{0x01}, 4))
	second := newFilledBuffer(t, dev, bytes.Repeat([]byte{0x02}, 4))
	dst := newFilledBuffer(t, dev, make([]byte, 4))

	enc, _ := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
	_ = enc.BeginEncoding("a")
	enc.CopyBufferToBuffer(second, dst, []hal.BufferCopy{{Size: 4}})
	cbSecond, _ := enc.EndEncoding()
	_ = enc.BeginEncoding("b")
	enc.CopyBufferToBuffer(first, dst, []hal.BufferCopy{{Size: 4}})
	cbFirst, _ := enc.EndEncoding()

	if _, err := queue.Submit([]hal.CommandBuffer{cbFirst, cbSecond}); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if got := dst.GetData(); !bytes.Equal(got, second.GetData()) {
		t.Fatalf("dst = %x, want %x from the last command buffer", got, second.GetData())
	}
}

// TestRenderPassRunsAtSubmit records a clearing render pass: the target
// keeps its contents until the command buffer is submitted.
func TestRenderPassRunsAtSubmit(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()

	tex, _ := dev.CreateTexture(&hal.TextureDescriptor{
		Size:   hal.Extent3D{Width: 2, Height: 2, DepthOrArrayLayers: 1},
		Format: gputypes.TextureFormatRGBA8Unorm,
	})
	view, _ := dev.CreateTextureView(tex, &hal.TextureViewDescriptor{})
	enc, _ := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
	pass := enc.BeginRenderPass(&hal.RenderPassDescriptor{
		ColorAttachments: []hal.RenderPassColorAttachment{{
			View: view, LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore,
			ClearValue: gputypes.Color{R: 1, G: 1, B: 1, A: 1},
		}},
	})
	pass.End()

	swTex := tex.(*Texture)
	if got := swTex.GetData(); !bytes.Equal(got, make([]byte, 16)) {
		t.Fatalf("target before Submit = %x, want zeros", got)
	}
	submit(t, enc)
	if got := swTex.GetData(); !bytes.Equal(got, bytes.Repeat([]byte{0xFF}, 16)) {
		t.Fatalf("target after Submit = %x, want white", got)
	}
}

// TestSubmitRejectsForeignCommandBuffer checks that Submit refuses a command
// buffer the software backend did not record, and runs none of the others.
func TestSubmitRejectsForeignCommandBuffer(t *testing.T) {
	dev, queue, cleanup := createSoftwareDevice(t)
	defer cleanup()

	buf := newFilledBuffer(t, dev, bytes.Repeat([]byte{0xFF}, 4))
	enc, _ := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
	_ = enc.BeginEncoding("valid")
	enc.ClearBuffer(buf, 0, 4)
	cb, _ := enc.EndEncoding()

	_, err := queue.Submit([]hal.CommandBuffer{cb, &Resource{}})
	if err == nil || !strings.Contains(err.Error(), "not a software command buffer") {
		t.Fatalf("Submit error = %v, want a foreign command buffer error", err)
	}
	if got := buf.GetData(); !bytes.Equal(got, bytes.Repeat([]byte{0xFF}, 4)) {
		t.Fatalf("buffer = %x, want it untouched by a refused submission", got)
	}
}

// TestRenderPassStatsBeforeSubmit checks that Stats is complete after End
// without a Submit: it is gathered at record time, and headless e2e tests
// (gogpu/ui's desktop software test) read it that way.
func TestRenderPassStatsBeforeSubmit(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()
	tex, err := dev.CreateTexture(&hal.TextureDescriptor{
		Size:   hal.Extent3D{Width: 20, Height: 10, DepthOrArrayLayers: 1},
		Format: gputypes.TextureFormatRGBA8Unorm,
		Usage:  gputypes.TextureUsageRenderAttachment,
	})
	if err != nil {
		t.Fatalf("CreateTexture: %v", err)
	}
	view, err := dev.CreateTextureView(tex, &hal.TextureViewDescriptor{})
	if err != nil {
		t.Fatalf("CreateTextureView: %v", err)
	}
	enc, err := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{})
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	pass := enc.BeginRenderPass(&hal.RenderPassDescriptor{ColorAttachments: []hal.RenderPassColorAttachment{{
		View: view, LoadOp: gputypes.LoadOpLoad, StoreOp: gputypes.StoreOpStore,
	}}})
	pass.SetScissorRect(gputypes.ScissorRect{X: 1, Y: 2, Width: 3, Height: 4})
	pass.Draw(gputypes.DrawArgs{VertexCount: 3, InstanceCount: 1})
	pass.End()

	s := pass.(*RenderPassEncoder).Stats()
	if s.DrawCount != 1 || !s.HasScissor || s.Width != 20 || s.Height != 10 || s.ScissorRect.Dx() != 3 {
		t.Fatalf("stats after End, before Submit = %+v, want 1 draw, scissor 3 wide, 20x10", s)
	}
}

// TestCloneEntriesIsDeep checks that a recorded acceleration structure build
// keeps its own geometry lists: a caller reusing its descriptors before
// Submit must not change the build.
func TestCloneEntriesIsDeep(t *testing.T) {
	src := &hal.AccelerationStructureEntries{
		Instances: &hal.AccelerationStructureInstances{Offset: 16, Count: 2},
		Triangles: []hal.AccelerationStructureTriangles{{
			FirstVertex: 1,
			Indices:     &hal.AccelerationStructureTriangleIndices{Count: 6},
		}},
		AABBs: []hal.AccelerationStructureAABBs{{Count: 3}},
	}
	got := cloneEntries(src)
	src.Instances.Count = 99
	src.Triangles[0].FirstVertex = 99
	src.Triangles[0].Indices.Count = 99
	src.AABBs[0].Count = 99
	if got.Instances.Count != 2 || got.Triangles[0].FirstVertex != 1 || got.Triangles[0].Indices.Count != 6 ||
		got.AABBs[0].Count != 3 {
		t.Fatalf("clone changed with its source: %+v %+v %+v", *got.Instances, got.Triangles[0], got.AABBs[0])
	}
	if cloneEntries(nil) != nil {
		t.Fatal("cloneEntries(nil) != nil")
	}
}
