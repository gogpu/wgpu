//go:build windows

// Package memory provides GPU memory allocation for Vulkan backend.
//
// # Architecture
//
// The memory subsystem is organized in layers:
//
//	┌─────────────────────────────────────────────────────────┐
//	│                    GpuAllocator                         │
//	│  (High-level API: Alloc/Free, memory type selection)    │
//	├─────────────────────────────────────────────────────────┤
//	│                   MemoryTypePool                        │
//	│  (Per memory-type pools, dedicated allocation logic)    │
//	├─────────────────────────────────────────────────────────┤
//	│                   BuddyAllocator                        │
//	│  (Power-of-2 block management, O(log n) operations)     │
//	├─────────────────────────────────────────────────────────┤
//	│                   Vulkan Memory API                     │
//	│  (vkAllocateMemory, vkFreeMemory, vkMapMemory)          │
//	└─────────────────────────────────────────────────────────┘
//
// # Buddy Allocator
//
// Implements classic buddy allocation algorithm:
//   - Memory divided into power-of-2 blocks
//   - Blocks split recursively until desired size reached
//   - Adjacent "buddy" blocks merged on free
//   - O(log n) allocation and deallocation
//   - Minimal external fragmentation
//
// # Memory Type Selection
//
// Vulkan exposes multiple memory types with different properties:
//   - DEVICE_LOCAL: Fast GPU access, no CPU access
//   - HOST_VISIBLE: CPU can map and access
//   - HOST_COHERENT: No flush/invalidate needed
//   - HOST_CACHED: CPU reads are cached
//
// The allocator selects optimal memory type based on usage flags.
//
// # Allocation Strategies
//
//   - Pooled: Small/medium allocations suballocated from large blocks
//   - Dedicated: Large allocations (>32MB) get their own VkDeviceMemory
//   - External: Memory imported from outside (not managed by allocator)
//
// # Thread Safety
//
// GpuAllocator is thread-safe. Internal synchronization via mutex.
// Individual MemoryBlock handles are not thread-safe.
package memory
