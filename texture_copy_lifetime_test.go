//go:build !rust && !(js && wasm)

package wgpu

import (
	"slices"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/core"
	"github.com/gogpu/wgpu/hal"
)

type delayedCompletionQueue struct {
	hal.Queue
	completed uint64
}

// TestCopyUsageConflictDoesNotCloneBufferRef checks that a copy rejected by
// its own usage scope (a buffer copied onto itself) does not retain a
// reference to the buffer.
func TestCopyUsageConflictDoesNotCloneBufferRef(t *testing.T) {
	t.Parallel()

	_, _, device := newTestDeviceWithTracker(t)
	defer device.Release()
	buffer, err := device.CreateBuffer(&BufferDescriptor{
		Size:  256,
		Usage: BufferUsageCopySrc | BufferUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer buffer.Release()
	ref := buffer.core.Ref

	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	enc.CopyBufferToBuffer(buffer, 0, buffer, 128, 64)

	if got := ref.RefCount(); got != 1 {
		t.Fatalf("failed copy changed buffer refcount to %d, want 1", got)
	}
	if got := len(enc.trackedRefs); got != 0 {
		t.Fatalf("failed copy retained %d resource refs, want 0", got)
	}
	if _, err := enc.Finish(); err == nil {
		t.Fatal("Finish succeeded after incompatible buffer usages")
	}
}

func (q *delayedCompletionQueue) PollCompleted() uint64 { return q.completed }

// TestBindGroupHoldsTextureViewRefs checks that a bind group keeps the
// texture view it binds, and the view's texture, alive after the application
// releases them, until the bind group itself is destroyed (Rust wgpu-core's
// BindGroup holds an Arc of each view).
func TestBindGroupHoldsTextureViewRefs(t *testing.T) {
	t.Parallel()

	_, _, device := newTestDeviceWithTracker(t)
	defer device.Release()
	texture := createCopyScopeTexture(t, device, "bind-group-view", TextureUsageCopySrc)
	view, err := device.CreateTextureView(texture, nil)
	if err != nil {
		t.Fatalf("CreateTextureView: %v", err)
	}
	layout, err := device.CreateBindGroupLayout(&BindGroupLayoutDescriptor{Entries: []gputypes.BindGroupLayoutEntry{{
		Binding:    0,
		Visibility: ShaderStageFragment,
		Texture: &gputypes.TextureBindingLayout{
			SampleType:    gputypes.TextureSampleTypeFloat,
			ViewDimension: gputypes.TextureViewDimension2D,
		},
	}}})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout: %v", err)
	}
	defer layout.Release()
	group, err := device.CreateBindGroup(&BindGroupDescriptor{
		Layout:  layout,
		Entries: []BindGroupEntry{{Binding: 0, TextureView: view}},
	})
	if err != nil {
		t.Fatalf("CreateBindGroup: %v", err)
	}

	refs := map[string]*core.ResourceRef{"view": view.ref, "texture": texture.ref()}
	checkRefCounts := func(when string, want int32) {
		t.Helper()
		for name, ref := range refs {
			if got := ref.RefCount(); got != want {
				t.Fatalf("%s refcount %s = %d, want %d", name, when, got, want)
			}
		}
	}
	checkRefCounts("with the bind group", 2)
	view.Release()
	texture.Release()
	checkRefCounts("after user Release", 1)
	group.Release()
	device.destroyQueue().FlushAll()
	checkRefCounts("after the bind group is destroyed", 0)
}

// TestTextureCopyRefsHeldUntilQueueRetirement checks that a buffer-texture
// copy keeps both the buffer and the texture alive until its submission is
// retired, even after the application releases them.
func TestTextureCopyRefsHeldUntilQueueRetirement(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		bufferUsage gputypes.BufferUsage
		textureUse  gputypes.TextureUsage
		record      func(*CommandEncoder, *Buffer, *Texture)
	}{
		{
			name:        "texture to buffer destination",
			bufferUsage: BufferUsageCopyDst,
			textureUse:  TextureUsageCopySrc,
			record: func(enc *CommandEncoder, buffer *Buffer, texture *Texture) {
				enc.CopyTextureToBuffer(texture, buffer, []BufferTextureCopy{{
					TextureBase:  ImageCopyTexture{Texture: texture},
					BufferLayout: ImageDataLayout{BytesPerRow: 256, RowsPerImage: 1},
					Size:         Extent3D{Width: 1, Height: 1, DepthOrArrayLayers: 1},
				}})
			},
		},
		{
			name:        "buffer source to texture",
			bufferUsage: BufferUsageCopySrc,
			textureUse:  TextureUsageCopyDst,
			record: func(enc *CommandEncoder, buffer *Buffer, texture *Texture) {
				enc.CopyBufferToTexture(buffer, texture, []BufferTextureCopy{{
					TextureBase:  ImageCopyTexture{Texture: texture},
					BufferLayout: ImageDataLayout{BytesPerRow: 256, RowsPerImage: 1},
					Size:         Extent3D{Width: 1, Height: 1, DepthOrArrayLayers: 1},
				}})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, device := newTestDeviceWithTracker(t)
			defer device.Release()

			queue := device.Queue()
			delayed := &delayedCompletionQueue{Queue: queue.hal}
			queue.hal = delayed

			buffer, err := device.CreateBuffer(&BufferDescriptor{
				Label: "texture-copy-lifetime-buffer",
				Size:  256,
				Usage: tt.bufferUsage,
			})
			if err != nil {
				t.Fatalf("CreateBuffer: %v", err)
			}
			texture := createCopyScopeTexture(t, device, "texture-copy-lifetime", tt.textureUse)
			defer texture.Release()

			refs := map[string]*core.ResourceRef{"buffer": buffer.core.Ref, "texture": texture.ref()}
			checkRefCounts := func(when string, want int32) {
				t.Helper()
				for name, ref := range refs {
					if got := ref.RefCount(); got != want {
						t.Fatalf("%s refcount %s = %d, want %d", name, when, got, want)
					}
				}
			}
			checkTracked := func(what string, tracked []*core.ResourceRef) {
				t.Helper()
				if len(tracked) != 2 || !slices.Contains(tracked, refs["buffer"]) || !slices.Contains(tracked, refs["texture"]) {
					t.Fatalf("%s tracked refs = %v, want exactly the buffer and texture refs", what, tracked)
				}
			}
			checkRefCounts("initially", 1)
			enc, err := device.CreateCommandEncoder(nil)
			if err != nil {
				t.Fatalf("CreateCommandEncoder: %v", err)
			}
			tt.record(enc, buffer, texture)
			checkRefCounts("after encoding", 2)
			checkTracked("encoder", enc.trackedRefs)

			cb, err := enc.Finish()
			if err != nil {
				t.Fatalf("Finish: %v", err)
			}
			checkTracked("command buffer", cb.trackedRefs)
			submission, err := queue.Submit(cb)
			if err != nil {
				t.Fatalf("Submit: %v", err)
			}
			checkRefCounts("after incomplete submit", 2)

			buffer.Release()
			texture.Release()
			checkRefCounts("after user Release", 1)
			device.destroyQueue().Triage(submission - 1)
			checkRefCounts("before retirement", 1)
			delayed.completed = submission
			device.destroyQueue().Triage(delayed.PollCompleted())
			checkRefCounts("after retirement", 0)
		})
	}
}
