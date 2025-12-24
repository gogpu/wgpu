package memory

import (
	"testing"

	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

func TestNewMemoryTypeSelector(t *testing.T) {
	props := DeviceMemoryProperties{
		MemoryTypes: []MemoryType{
			{PropertyFlags: vk.MemoryPropertyFlags(vk.MemoryPropertyDeviceLocalBit), HeapIndex: 0},
			{PropertyFlags: vk.MemoryPropertyFlags(vk.MemoryPropertyHostVisibleBit | vk.MemoryPropertyHostCoherentBit), HeapIndex: 1},
			{PropertyFlags: vk.MemoryPropertyFlags(vk.MemoryPropertyHostVisibleBit | vk.MemoryPropertyHostCachedBit), HeapIndex: 1},
		},
		MemoryHeaps: []MemoryHeap{
			{Size: 4 << 30, Flags: 0}, // 4GB device local
			{Size: 8 << 30, Flags: 0}, // 8GB host visible
		},
	}

	selector := NewMemoryTypeSelector(props)
	if selector == nil {
		t.Fatal("NewMemoryTypeSelector returned nil")
		return
	}

	// All memory types should be valid (no exotic flags)
	if selector.validTypes != 0b111 {
		t.Errorf("validTypes = %b, want %b", selector.validTypes, 0b111)
	}
}

func TestSelectMemoryType(t *testing.T) {
	props := DeviceMemoryProperties{
		MemoryTypes: []MemoryType{
			{PropertyFlags: vk.MemoryPropertyFlags(vk.MemoryPropertyDeviceLocalBit), HeapIndex: 0},
			{PropertyFlags: vk.MemoryPropertyFlags(vk.MemoryPropertyHostVisibleBit | vk.MemoryPropertyHostCoherentBit), HeapIndex: 1},
			{PropertyFlags: vk.MemoryPropertyFlags(vk.MemoryPropertyHostVisibleBit | vk.MemoryPropertyHostCachedBit), HeapIndex: 1},
		},
		MemoryHeaps: []MemoryHeap{
			{Size: 4 << 30, Flags: 0},
			{Size: 8 << 30, Flags: 0},
		},
	}

	selector := NewMemoryTypeSelector(props)

	tests := []struct {
		name      string
		req       AllocationRequest
		wantIndex uint32
		wantFound bool
	}{
		{
			name: "fast device access prefers device local",
			req: AllocationRequest{
				Size:           1024,
				Usage:          UsageFastDeviceAccess,
				MemoryTypeBits: 0b111, // All types allowed
			},
			wantIndex: 0,
			wantFound: true,
		},
		{
			name: "upload prefers host visible + coherent",
			req: AllocationRequest{
				Size:           1024,
				Usage:          UsageUpload,
				MemoryTypeBits: 0b111,
			},
			wantIndex: 1, // HOST_VISIBLE + HOST_COHERENT
			wantFound: true,
		},
		{
			name: "download prefers host visible + cached",
			req: AllocationRequest{
				Size:           1024,
				Usage:          UsageDownload,
				MemoryTypeBits: 0b111,
			},
			wantIndex: 2, // HOST_VISIBLE + HOST_CACHED
			wantFound: true,
		},
		{
			name: "host access requires host visible",
			req: AllocationRequest{
				Size:           1024,
				Usage:          UsageHostAccess,
				MemoryTypeBits: 0b111,
			},
			wantIndex: 1, // First HOST_VISIBLE type
			wantFound: true,
		},
		{
			name: "no matching type returns false",
			req: AllocationRequest{
				Size:           1024,
				Usage:          UsageHostAccess,
				MemoryTypeBits: 0b001, // Only device local allowed
			},
			wantFound: false,
		},
		{
			name: "zero memory type bits returns false",
			req: AllocationRequest{
				Size:           1024,
				Usage:          UsageFastDeviceAccess,
				MemoryTypeBits: 0,
			},
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			index, found := selector.SelectMemoryType(tt.req)
			if found != tt.wantFound {
				t.Errorf("SelectMemoryType() found = %v, want %v", found, tt.wantFound)
			}
			if found && index != tt.wantIndex {
				t.Errorf("SelectMemoryType() index = %d, want %d", index, tt.wantIndex)
			}
		})
	}
}

func TestMemoryTypeSelectorHelpers(t *testing.T) {
	props := DeviceMemoryProperties{
		MemoryTypes: []MemoryType{
			{PropertyFlags: vk.MemoryPropertyFlags(vk.MemoryPropertyDeviceLocalBit), HeapIndex: 0},
			{PropertyFlags: vk.MemoryPropertyFlags(vk.MemoryPropertyHostVisibleBit), HeapIndex: 1},
		},
		MemoryHeaps: []MemoryHeap{
			{Size: 4 << 30, Flags: 0},
			{Size: 8 << 30, Flags: 0},
		},
	}

	selector := NewMemoryTypeSelector(props)

	// Test IsDeviceLocal
	if !selector.IsDeviceLocal(0) {
		t.Error("Expected type 0 to be device local")
	}
	if selector.IsDeviceLocal(1) {
		t.Error("Expected type 1 to NOT be device local")
	}
	if selector.IsDeviceLocal(99) {
		t.Error("Expected invalid type to return false")
	}

	// Test IsHostVisible
	if selector.IsHostVisible(0) {
		t.Error("Expected type 0 to NOT be host visible")
	}
	if !selector.IsHostVisible(1) {
		t.Error("Expected type 1 to be host visible")
	}

	// Test GetHeapSize
	if size := selector.GetHeapSize(0); size != 4<<30 {
		t.Errorf("GetHeapSize(0) = %d, want %d", size, 4<<30)
	}
	if size := selector.GetHeapSize(99); size != 0 {
		t.Errorf("GetHeapSize(99) = %d, want 0", size)
	}

	// Test GetMemoryType
	mt, ok := selector.GetMemoryType(0)
	if !ok {
		t.Error("GetMemoryType(0) should return true")
	}
	if mt.HeapIndex != 0 {
		t.Errorf("GetMemoryType(0).HeapIndex = %d, want 0", mt.HeapIndex)
	}

	_, ok = selector.GetMemoryType(99)
	if ok {
		t.Error("GetMemoryType(99) should return false")
	}
}

func TestMemoryBlockHelpers(t *testing.T) {
	block := &MemoryBlock{
		Memory:          1234,
		Offset:          0,
		Size:            4096,
		memoryTypeIndex: 2,
		dedicated:       true,
	}

	if !block.IsDedicated() {
		t.Error("IsDedicated() should return true")
	}

	if block.MemoryTypeIndex() != 2 {
		t.Errorf("MemoryTypeIndex() = %d, want 2", block.MemoryTypeIndex())
	}

	// Non-dedicated block
	pooledBlock := &MemoryBlock{
		dedicated: false,
	}
	if pooledBlock.IsDedicated() {
		t.Error("IsDedicated() should return false for pooled block")
	}
}

func TestUsageFlags(t *testing.T) {
	// Test that flags are distinct
	flags := []UsageFlags{
		UsageFastDeviceAccess,
		UsageHostAccess,
		UsageUpload,
		UsageDownload,
		UsageTransient,
	}

	for i := 0; i < len(flags); i++ {
		for j := i + 1; j < len(flags); j++ {
			if flags[i]&flags[j] != 0 {
				t.Errorf("Usage flags %d and %d overlap", i, j)
			}
		}
	}

	// Test combinations
	combined := UsageHostAccess | UsageUpload
	if combined&UsageHostAccess == 0 {
		t.Error("Combined flag should include UsageHostAccess")
	}
	if combined&UsageUpload == 0 {
		t.Error("Combined flag should include UsageUpload")
	}
	if combined&UsageDownload != 0 {
		t.Error("Combined flag should NOT include UsageDownload")
	}
}
