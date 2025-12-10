package memory

import (
	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

// UsageFlags specifies intended memory usage.
// These flags help select the optimal memory type.
type UsageFlags uint32

const (
	// UsageFastDeviceAccess indicates memory primarily accessed by GPU.
	// Prefers DEVICE_LOCAL memory.
	UsageFastDeviceAccess UsageFlags = 1 << iota

	// UsageHostAccess indicates memory needs CPU access.
	// Requires HOST_VISIBLE memory.
	UsageHostAccess

	// UsageUpload indicates memory used for CPU->GPU transfers.
	// Prefers HOST_VISIBLE + HOST_COHERENT, avoids HOST_CACHED.
	UsageUpload

	// UsageDownload indicates memory used for GPU->CPU readback.
	// Prefers HOST_VISIBLE + HOST_CACHED.
	UsageDownload

	// UsageTransient indicates memory for short-lived allocations.
	// May use LAZILY_ALLOCATED if available.
	UsageTransient
)

// AllocationRequest describes a memory allocation request.
type AllocationRequest struct {
	// Size is the required allocation size in bytes.
	Size uint64

	// Alignment is the required alignment (must be power of 2).
	// Use 0 or 1 for no specific alignment beyond block size.
	Alignment uint64

	// Usage specifies how the memory will be used.
	Usage UsageFlags

	// MemoryTypeBits is a bitmask of allowed memory type indices.
	// Typically from VkMemoryRequirements.memoryTypeBits.
	MemoryTypeBits uint32
}

// MemoryBlock represents an allocated memory region.
type MemoryBlock struct {
	// Memory is the Vulkan device memory handle.
	Memory vk.DeviceMemory

	// Offset is the byte offset within the device memory.
	Offset uint64

	// Size is the allocated size in bytes.
	Size uint64

	// memoryTypeIndex is the Vulkan memory type used.
	memoryTypeIndex uint32

	// dedicated indicates this is a dedicated allocation.
	dedicated bool

	// buddyBlock holds buddy allocator metadata for pooled allocations.
	buddyBlock *BuddyBlock

	// MappedPtr holds the mapped pointer if memory is mapped.
	// Set by Map(), cleared by Unmap().
	MappedPtr uintptr
}

// IsDedicated returns true if this is a dedicated allocation.
func (b *MemoryBlock) IsDedicated() bool {
	return b.dedicated
}

// MemoryTypeIndex returns the Vulkan memory type index.
func (b *MemoryBlock) MemoryTypeIndex() uint32 {
	return b.memoryTypeIndex
}

// MemoryType describes a Vulkan memory type.
type MemoryType struct {
	// PropertyFlags contains VkMemoryPropertyFlags.
	PropertyFlags vk.MemoryPropertyFlags

	// HeapIndex is the index of the memory heap.
	HeapIndex uint32
}

// MemoryHeap describes a Vulkan memory heap.
type MemoryHeap struct {
	// Size is the total heap size in bytes.
	Size uint64

	// Flags contains VkMemoryHeapFlags.
	Flags vk.MemoryHeapFlags
}

// DeviceMemoryProperties holds all memory properties for a device.
type DeviceMemoryProperties struct {
	// MemoryTypes lists all available memory types.
	MemoryTypes []MemoryType

	// MemoryHeaps lists all available memory heaps.
	MemoryHeaps []MemoryHeap
}

// MemoryTypeSelector selects optimal memory types for allocations.
type MemoryTypeSelector struct {
	properties DeviceMemoryProperties

	// validTypes is a bitmask of memory types we consider safe to use.
	// Excludes exotic/vendor-specific types.
	validTypes uint32
}

// knownMemoryFlags are memory property flags we understand and can use.
const knownMemoryFlags = vk.MemoryPropertyFlags(vk.MemoryPropertyDeviceLocalBit) |
	vk.MemoryPropertyFlags(vk.MemoryPropertyHostVisibleBit) |
	vk.MemoryPropertyFlags(vk.MemoryPropertyHostCoherentBit) |
	vk.MemoryPropertyFlags(vk.MemoryPropertyHostCachedBit) |
	vk.MemoryPropertyFlags(vk.MemoryPropertyLazilyAllocatedBit)

// NewMemoryTypeSelector creates a selector from device memory properties.
func NewMemoryTypeSelector(props DeviceMemoryProperties) *MemoryTypeSelector {
	// Build bitmask of valid (known) memory types
	var validTypes uint32
	for i, mt := range props.MemoryTypes {
		// Only include types where we understand all flags
		unknownFlags := mt.PropertyFlags & ^knownMemoryFlags
		if unknownFlags == 0 {
			validTypes |= 1 << i
		}
	}

	return &MemoryTypeSelector{
		properties: props,
		validTypes: validTypes,
	}
}

// SelectMemoryType finds the best memory type for the given request.
//
// Returns the memory type index and true if found, or 0 and false if
// no suitable type exists.
func (s *MemoryTypeSelector) SelectMemoryType(req AllocationRequest) (uint32, bool) {
	// Determine required and preferred flags based on usage
	required, preferred := s.usageToFlags(req.Usage)

	// First pass: try to find type with all preferred flags
	if idx, ok := s.findMemoryType(req.MemoryTypeBits, required|preferred); ok {
		return idx, true
	}

	// Second pass: fall back to just required flags
	if idx, ok := s.findMemoryType(req.MemoryTypeBits, required); ok {
		return idx, true
	}

	return 0, false
}

// findMemoryType finds a memory type matching the requirements.
func (s *MemoryTypeSelector) findMemoryType(typeBits uint32, flags vk.MemoryPropertyFlags) (uint32, bool) {
	for i, mt := range s.properties.MemoryTypes {
		typeMask := uint32(1) << i

		// Check if type is allowed by resource requirements
		if typeBits&typeMask == 0 {
			continue
		}

		// Check if type is in our known-good list
		if s.validTypes&typeMask == 0 {
			continue
		}

		// Check if it has required properties
		if mt.PropertyFlags&flags == flags {
			return uint32(i), true
		}
	}

	return 0, false
}

// usageToFlags converts usage flags to Vulkan memory property flags.
func (s *MemoryTypeSelector) usageToFlags(usage UsageFlags) (required, preferred vk.MemoryPropertyFlags) {
	if usage&UsageHostAccess != 0 || usage&UsageUpload != 0 || usage&UsageDownload != 0 {
		// CPU access required
		required |= vk.MemoryPropertyFlags(vk.MemoryPropertyHostVisibleBit)

		if usage&UsageUpload != 0 {
			// Upload: coherent preferred to avoid flush
			preferred |= vk.MemoryPropertyFlags(vk.MemoryPropertyHostCoherentBit)
		}

		if usage&UsageDownload != 0 {
			// Download: cached preferred for read performance
			preferred |= vk.MemoryPropertyFlags(vk.MemoryPropertyHostCachedBit)
		}
	} else if usage&UsageFastDeviceAccess != 0 {
		// GPU-only: prefer device local
		preferred |= vk.MemoryPropertyFlags(vk.MemoryPropertyDeviceLocalBit)
	}

	if usage&UsageTransient != 0 {
		// Transient: lazily allocated if available
		preferred |= vk.MemoryPropertyFlags(vk.MemoryPropertyLazilyAllocatedBit)
	}

	return required, preferred
}

// GetHeapSize returns the size of the specified heap.
func (s *MemoryTypeSelector) GetHeapSize(heapIndex uint32) uint64 {
	if int(heapIndex) >= len(s.properties.MemoryHeaps) {
		return 0
	}
	return s.properties.MemoryHeaps[heapIndex].Size
}

// GetMemoryType returns the memory type at the given index.
func (s *MemoryTypeSelector) GetMemoryType(index uint32) (MemoryType, bool) {
	if int(index) >= len(s.properties.MemoryTypes) {
		return MemoryType{}, false
	}
	return s.properties.MemoryTypes[index], true
}

// IsDeviceLocal returns true if the memory type is device local.
func (s *MemoryTypeSelector) IsDeviceLocal(typeIndex uint32) bool {
	mt, ok := s.GetMemoryType(typeIndex)
	if !ok {
		return false
	}
	return mt.PropertyFlags&vk.MemoryPropertyFlags(vk.MemoryPropertyDeviceLocalBit) != 0
}

// IsHostVisible returns true if the memory type is host visible.
func (s *MemoryTypeSelector) IsHostVisible(typeIndex uint32) bool {
	mt, ok := s.GetMemoryType(typeIndex)
	if !ok {
		return false
	}
	return mt.PropertyFlags&vk.MemoryPropertyFlags(vk.MemoryPropertyHostVisibleBit) != 0
}
