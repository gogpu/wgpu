//go:build !(js && wasm)

package vulkan

import (
	"testing"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/hal/vulkan/vk"
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

	batch, err := descriptorWrites(0, entries, types)
	if err != nil {
		t.Fatal(err)
	}
	writes, bufferInfos, imageInfos := batch.writes, batch.bufferInfos, batch.imageInfos
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

// TestDescriptorWritesTextureTypeFromLayout checks that a texture view takes
// its descriptor type and image layout from the layout entry: a storage
// texture is STORAGE_IMAGE in GENERAL, a sampled one SAMPLED_IMAGE in
// SHADER_READ_ONLY_OPTIMAL (VUID-VkWriteDescriptorSet-descriptorType-00319).
func TestDescriptorWritesTextureTypeFromLayout(t *testing.T) {
	entries := []gputypes.BindGroupEntry{
		{Binding: 0, Resource: gputypes.TextureViewBinding{TextureView: 1}},
		{Binding: 1, Resource: gputypes.TextureViewBinding{TextureView: 2}},
	}
	types := map[uint32]vk.DescriptorType{0: vk.DescriptorTypeSampledImage, 1: vk.DescriptorTypeStorageImage}
	batch, err := descriptorWrites(0, entries, types)
	if err != nil {
		t.Fatal(err)
	}
	writes, imageInfos := batch.writes, batch.imageInfos
	want := []struct {
		dt     vk.DescriptorType
		layout vk.ImageLayout
	}{
		{vk.DescriptorTypeSampledImage, vk.ImageLayoutShaderReadOnlyOptimal},
		{vk.DescriptorTypeStorageImage, vk.ImageLayoutGeneral},
	}
	for i, w := range want {
		if writes[i].DescriptorType != w.dt || imageInfos[i].ImageLayout != w.layout {
			t.Errorf("binding %d: type %d layout %d, want type %d layout %d",
				i, writes[i].DescriptorType, imageInfos[i].ImageLayout, w.dt, w.layout)
		}
	}
}

// TestDescriptorWritesAccelerationStructure checks that each acceleration
// structure write chains its own VkWriteDescriptorSetAccelerationStructureKHR,
// which points at its own handle, both inside the batch's final arrays (#356),
// and that an acceleration structure bound to another layout type is refused.
func TestDescriptorWritesAccelerationStructure(t *testing.T) {
	entries := []gputypes.BindGroupEntry{
		{Binding: 0, Resource: gputypes.AccelerationStructureBinding{AccelerationStructure: 7}},
		{Binding: 1, Resource: gputypes.BufferBinding{Buffer: 1, Size: 64}},
		{Binding: 2, Resource: gputypes.AccelerationStructureBinding{AccelerationStructure: 9}},
	}
	types := map[uint32]vk.DescriptorType{
		0: vk.DescriptorTypeAccelerationStructureKhr,
		1: vk.DescriptorTypeStorageBuffer,
		2: vk.DescriptorTypeAccelerationStructureKhr,
	}
	batch, err := descriptorWrites(0, entries, types)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.asHandles) != 2 || len(batch.asWrites) != 2 {
		t.Fatalf("got %d AS handles, %d AS writes; want 2, 2", len(batch.asHandles), len(batch.asWrites))
	}
	for k, n := range []int{0, 2} {
		w := batch.writes[n]
		if w.DescriptorType != vk.DescriptorTypeAccelerationStructureKhr || w.DescriptorCount != 1 {
			t.Errorf("write %d: type %d count %d, want ACCELERATION_STRUCTURE_KHR and 1", n, w.DescriptorType, w.DescriptorCount)
		}
		if w.PNext != (*uintptr)(unsafe.Pointer(&batch.asWrites[k])) {
			t.Errorf("write %d: pNext does not point at asWrites[%d]", n, k)
		}
		if batch.asWrites[k].PAccelerationStructures != &batch.asHandles[k] {
			t.Errorf("asWrites[%d] does not point at asHandles[%d]", k, k)
		}
	}
	if batch.asHandles[0] != 7 || batch.asHandles[1] != 9 {
		t.Errorf("AS handles = %v, want [7 9]", batch.asHandles)
	}

	types[0] = vk.DescriptorTypeStorageBuffer
	if _, err := descriptorWrites(0, entries, types); err == nil {
		t.Error("acceleration structure bound to a storage buffer layout entry: want an error")
	}
}
