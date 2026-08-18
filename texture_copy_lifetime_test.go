//go:build !rust && !(js && wasm)

package wgpu

import (
	"testing"

	"github.com/gogpu/wgpu/core/track"
	"github.com/gogpu/wgpu/hal"
)

type delayedCompletionQueue struct {
	hal.Queue
	completed uint64
}

func TestTextureCopyUsageConflictDoesNotCloneBufferRef(t *testing.T) {
	t.Parallel()

	_, _, device := newTestDeviceWithTracker(t)
	defer device.Release()
	texture := createCopyScopeTexture(t, device, "conflicting-copy", TextureUsageCopySrc)
	defer texture.Release()
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
	if !enc.recordCopyBufferUsages([]copyBufferUsage{{buffer: buffer.core, usage: track.BufferUsesCopySrc}}) {
		t.Fatal("failed to establish conflicting CopySrc usage")
	}
	enc.CopyTextureToBuffer(texture, buffer, nil)

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

func TestTextureCopyBufferRefHeldUntilQueueRetirement(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		bufferUsage BufferUsage
		textureUse  TextureUsage
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

			ref := buffer.core.Ref
			if got := ref.RefCount(); got != 1 {
				t.Fatalf("initial buffer refcount = %d, want 1", got)
			}
			enc, err := device.CreateCommandEncoder(nil)
			if err != nil {
				t.Fatalf("CreateCommandEncoder: %v", err)
			}
			tt.record(enc, buffer, texture)
			if got := ref.RefCount(); got != 2 {
				t.Fatalf("buffer refcount after encoding = %d, want 2", got)
			}
			if got := len(enc.trackedRefs); got != 1 || enc.trackedRefs[0] != ref {
				t.Fatalf("encoder tracked refs = %v, want exactly the copy buffer ref", enc.trackedRefs)
			}

			cb, err := enc.Finish()
			if err != nil {
				t.Fatalf("Finish: %v", err)
			}
			if got := len(cb.trackedRefs); got != 1 || cb.trackedRefs[0] != ref {
				t.Fatalf("command buffer tracked refs = %v, want exactly the copy buffer ref", cb.trackedRefs)
			}
			submission, err := queue.Submit(cb)
			if err != nil {
				t.Fatalf("Submit: %v", err)
			}
			if got := ref.RefCount(); got != 2 {
				t.Fatalf("buffer refcount after incomplete submit = %d, want 2", got)
			}

			buffer.Release()
			if got := ref.RefCount(); got != 1 {
				t.Fatalf("buffer refcount after user Release = %d, want in-flight ref 1", got)
			}
			device.destroyQueue().Triage(submission - 1)
			if got := ref.RefCount(); got != 1 {
				t.Fatalf("buffer refcount before retirement = %d, want 1", got)
			}
			delayed.completed = submission
			device.destroyQueue().Triage(delayed.PollCompleted())
			if got := ref.RefCount(); got != 0 {
				t.Fatalf("buffer refcount after retirement = %d, want 0", got)
			}
		})
	}
}
