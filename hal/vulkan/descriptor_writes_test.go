//go:build !(js && wasm)

package vulkan

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

// TestDescriptorWritesPointIntoFinalInfoArrays guards #356: with several
// buffer and image entries, every write must point at its info struct in the
// final backing arrays, not in an array that a later append replaced.
func TestDescriptorWritesPointIntoFinalInfoArrays(t *testing.T) {
	entries := []gputypes.BindGroupEntry{
		{Binding: 0, Resource: gputypes.BufferBinding{Buffer: 1, Size: 16}},
		{Binding: 1, Resource: gputypes.SamplerBinding{Sampler: 2}},
		{Binding: 2, Resource: gputypes.BufferBinding{Buffer: 3, Offset: 256, Size: 0}},
		{Binding: 3, Resource: gputypes.TextureViewBinding{TextureView: 4}},
		{Binding: 4, Resource: gputypes.BufferBinding{Buffer: 5, Size: 64}},
		{Binding: 5, Resource: gputypes.TextureViewBinding{TextureView: 6}},
	}
	types := map[uint32]vk.DescriptorType{0: vk.DescriptorTypeStorageBuffer, 4: vk.DescriptorTypeStorageBuffer}

	writes, bufferInfos, imageInfos, err := descriptorWrites(0, entries, types)
	if err != nil {
		t.Fatal(err)
	}
	if len(writes) != len(entries) || len(bufferInfos) != 3 || len(imageInfos) != 3 {
		t.Fatalf("got %d writes, %d buffer infos, %d image infos; want 6, 3, 3", len(writes), len(bufferInfos), len(imageInfos))
	}
	var b, i int
	for n, w := range writes {
		switch entries[n].Resource.(type) {
		case gputypes.BufferBinding:
			if w.PBufferInfo != &bufferInfos[b] {
				t.Errorf("write %d: PBufferInfo does not point at bufferInfos[%d]", n, b)
			}
			b++
		default:
			if w.PImageInfo != &imageInfos[i] {
				t.Errorf("write %d: PImageInfo does not point at imageInfos[%d]", n, i)
			}
			i++
		}
	}
	if bufferInfos[1].Range != vk.DeviceSize(vk.WholeSize) || bufferInfos[1].Offset != 256 {
		t.Errorf("size 0 buffer binding = %+v, want offset 256 and VK_WHOLE_SIZE", bufferInfos[1])
	}
}
