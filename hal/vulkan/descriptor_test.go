// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"testing"
)

// TestDescriptorCountsTotal tests the Total method.
func TestDescriptorCountsTotal(t *testing.T) {
	tests := []struct {
		name   string
		counts DescriptorCounts
		expect uint32
	}{
		{
			name:   "Empty",
			counts: DescriptorCounts{},
			expect: 0,
		},
		{
			name: "Single type",
			counts: DescriptorCounts{
				Samplers: 10,
			},
			expect: 10,
		},
		{
			name: "Multiple types",
			counts: DescriptorCounts{
				Samplers:       5,
				SampledImages:  10,
				UniformBuffers: 15,
			},
			expect: 30,
		},
		{
			name: "All types",
			counts: DescriptorCounts{
				Samplers:           1,
				SampledImages:      2,
				StorageImages:      3,
				UniformBuffers:     4,
				StorageBuffers:     5,
				UniformTexelBuffer: 6,
				StorageTexelBuffer: 7,
				InputAttachments:   8,
			},
			expect: 36,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.counts.Total()
			if got != tt.expect {
				t.Errorf("Total() = %d, want %d", got, tt.expect)
			}
		})
	}
}

// TestDescriptorCountsIsEmpty tests the IsEmpty method.
func TestDescriptorCountsIsEmpty(t *testing.T) {
	tests := []struct {
		name   string
		counts DescriptorCounts
		expect bool
	}{
		{
			name:   "Empty counts",
			counts: DescriptorCounts{},
			expect: true,
		},
		{
			name: "Non-empty counts",
			counts: DescriptorCounts{
				Samplers: 1,
			},
			expect: false,
		},
		{
			name: "Multiple non-zero",
			counts: DescriptorCounts{
				Samplers:       5,
				UniformBuffers: 10,
			},
			expect: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.counts.IsEmpty()
			if got != tt.expect {
				t.Errorf("IsEmpty() = %v, want %v", got, tt.expect)
			}
		})
	}
}

// TestDescriptorCountsMultiply tests the Multiply method.
func TestDescriptorCountsMultiply(t *testing.T) {
	counts := DescriptorCounts{
		Samplers:       2,
		SampledImages:  4,
		UniformBuffers: 6,
	}

	result := counts.Multiply(3)

	if result.Samplers != 6 {
		t.Errorf("Samplers = %d, want 6", result.Samplers)
	}
	if result.SampledImages != 12 {
		t.Errorf("SampledImages = %d, want 12", result.SampledImages)
	}
	if result.UniformBuffers != 18 {
		t.Errorf("UniformBuffers = %d, want 18", result.UniformBuffers)
	}
}

// TestDescriptorCountsMultiplyZero tests multiplying by zero.
func TestDescriptorCountsMultiplyZero(t *testing.T) {
	counts := DescriptorCounts{
		Samplers:       10,
		UniformBuffers: 20,
	}

	result := counts.Multiply(0)

	if result.Total() != 0 {
		t.Errorf("Expected all counts to be zero, got total = %d", result.Total())
	}
}

// TestDescriptorCountsMultiplyAll tests that all fields are multiplied.
func TestDescriptorCountsMultiplyAll(t *testing.T) {
	counts := DescriptorCounts{
		Samplers:           1,
		SampledImages:      2,
		StorageImages:      3,
		UniformBuffers:     4,
		StorageBuffers:     5,
		UniformTexelBuffer: 6,
		StorageTexelBuffer: 7,
		InputAttachments:   8,
	}

	result := counts.Multiply(2)

	expected := DescriptorCounts{
		Samplers:           2,
		SampledImages:      4,
		StorageImages:      6,
		UniformBuffers:     8,
		StorageBuffers:     10,
		UniformTexelBuffer: 12,
		StorageTexelBuffer: 14,
		InputAttachments:   16,
	}

	if result != expected {
		t.Errorf("Multiply(2) = %+v, want %+v", result, expected)
	}
}

// TestDefaultDescriptorAllocatorConfig tests the default configuration.
func TestDefaultDescriptorAllocatorConfig(t *testing.T) {
	config := DefaultDescriptorAllocatorConfig()

	if config.InitialPoolSize != 64 {
		t.Errorf("InitialPoolSize = %d, want 64", config.InitialPoolSize)
	}
	if config.MaxPoolSize != 4096 {
		t.Errorf("MaxPoolSize = %d, want 4096", config.MaxPoolSize)
	}
	if config.GrowthFactor != 2 {
		t.Errorf("GrowthFactor = %d, want 2", config.GrowthFactor)
	}
}

// TestNewDescriptorAllocator tests allocator creation.
func TestNewDescriptorAllocator(t *testing.T) {
	config := DescriptorAllocatorConfig{
		InitialPoolSize: 128,
		MaxPoolSize:     2048,
		GrowthFactor:    4,
	}

	allocator := NewDescriptorAllocator(0, nil, config)

	if allocator == nil {
		t.Fatal("NewDescriptorAllocator returned nil")
		return
	}

	if allocator.initialPoolSize != 128 {
		t.Errorf("initialPoolSize = %d, want 128", allocator.initialPoolSize)
	}
	if allocator.maxPoolSize != 2048 {
		t.Errorf("maxPoolSize = %d, want 2048", allocator.maxPoolSize)
	}
	if allocator.growthFactor != 4 {
		t.Errorf("growthFactor = %d, want 4", allocator.growthFactor)
	}
}

// TestNewDescriptorAllocatorDefaults tests that zero values get defaults.
func TestNewDescriptorAllocatorDefaults(t *testing.T) {
	config := DescriptorAllocatorConfig{
		// All zeros - should get defaults
	}

	allocator := NewDescriptorAllocator(0, nil, config)

	if allocator.initialPoolSize != 64 {
		t.Errorf("initialPoolSize = %d, want 64 (default)", allocator.initialPoolSize)
	}
	if allocator.maxPoolSize != 4096 {
		t.Errorf("maxPoolSize = %d, want 4096 (default)", allocator.maxPoolSize)
	}
	if allocator.growthFactor != 2 {
		t.Errorf("growthFactor = %d, want 2 (default)", allocator.growthFactor)
	}
}

// TestDescriptorAllocatorStatsInitial tests initial statistics.
func TestDescriptorAllocatorStatsInitial(t *testing.T) {
	allocator := NewDescriptorAllocator(0, nil, DefaultDescriptorAllocatorConfig())

	pools, allocated, freed := allocator.Stats()

	if pools != 0 {
		t.Errorf("Initial pools = %d, want 0", pools)
	}
	if allocated != 0 {
		t.Errorf("Initial allocated = %d, want 0", allocated)
	}
	if freed != 0 {
		t.Errorf("Initial freed = %d, want 0", freed)
	}
}

// TestDescriptorPoolAllocation tests descriptor pool structure.
func TestDescriptorPoolAllocation(t *testing.T) {
	pool := &DescriptorPool{
		handle:        1234, // Fake handle
		maxSets:       100,
		allocatedSets: 0,
	}

	if pool.handle != 1234 {
		t.Errorf("handle = %v, want 1234", pool.handle)
	}
	if pool.maxSets != 100 {
		t.Errorf("maxSets = %d, want 100", pool.maxSets)
	}
	if pool.allocatedSets != 0 {
		t.Errorf("allocatedSets = %d, want 0", pool.allocatedSets)
	}
}

// TestDescriptorPoolAllocationTracking tests allocation counting.
func TestDescriptorPoolAllocationTracking(t *testing.T) {
	pool := &DescriptorPool{
		handle:        1,
		maxSets:       10,
		allocatedSets: 0,
	}

	// Simulate allocations
	pool.allocatedSets++
	if pool.allocatedSets != 1 {
		t.Errorf("After 1 allocation: allocatedSets = %d, want 1", pool.allocatedSets)
	}

	pool.allocatedSets++
	if pool.allocatedSets != 2 {
		t.Errorf("After 2 allocations: allocatedSets = %d, want 2", pool.allocatedSets)
	}

	// Simulate free
	pool.allocatedSets--
	if pool.allocatedSets != 1 {
		t.Errorf("After 1 free: allocatedSets = %d, want 1", pool.allocatedSets)
	}
}

// TestDescriptorPoolCapacity tests capacity checking.
func TestDescriptorPoolCapacity(t *testing.T) {
	tests := []struct {
		name          string
		maxSets       uint32
		allocatedSets uint32
		expectFull    bool
	}{
		{"Empty pool", 10, 0, false},
		{"Partial pool", 10, 5, false},
		{"Almost full", 10, 9, false},
		{"Full pool", 10, 10, true},
		{"Over capacity", 10, 11, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := &DescriptorPool{
				maxSets:       tt.maxSets,
				allocatedSets: tt.allocatedSets,
			}

			isFull := pool.allocatedSets >= pool.maxSets

			if isFull != tt.expectFull {
				t.Errorf("isFull = %v, want %v", isFull, tt.expectFull)
			}
		})
	}
}

// TestDescriptorCountsFieldNames tests that all field names are correct.
func TestDescriptorCountsFieldNames(t *testing.T) {
	// This test ensures the struct has expected fields
	counts := DescriptorCounts{
		Samplers:           1,
		SampledImages:      2,
		StorageImages:      3,
		UniformBuffers:     4,
		StorageBuffers:     5,
		UniformTexelBuffer: 6,
		StorageTexelBuffer: 7,
		InputAttachments:   8,
	}

	// Verify fields are accessible
	if counts.Samplers != 1 {
		t.Errorf("Samplers field access failed")
	}
	if counts.SampledImages != 2 {
		t.Errorf("SampledImages field access failed")
	}
	if counts.StorageImages != 3 {
		t.Errorf("StorageImages field access failed")
	}
	if counts.UniformBuffers != 4 {
		t.Errorf("UniformBuffers field access failed")
	}
	if counts.StorageBuffers != 5 {
		t.Errorf("StorageBuffers field access failed")
	}
	if counts.UniformTexelBuffer != 6 {
		t.Errorf("UniformTexelBuffer field access failed")
	}
	if counts.StorageTexelBuffer != 7 {
		t.Errorf("StorageTexelBuffer field access failed")
	}
	if counts.InputAttachments != 8 {
		t.Errorf("InputAttachments field access failed")
	}
}

// TestDescriptorAllocatorConcurrency tests basic thread-safety (mutex usage).
func TestDescriptorAllocatorConcurrency(t *testing.T) {
	allocator := NewDescriptorAllocator(0, nil, DefaultDescriptorAllocatorConfig())

	// Call Stats multiple times concurrently to test mutex
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func() {
			_, _, _ = allocator.Stats()
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}

// TestDescriptorAllocatorInitialization tests that allocator is properly initialized.
func TestDescriptorAllocatorInitialization(t *testing.T) {
	config := DescriptorAllocatorConfig{
		InitialPoolSize: 100,
		MaxPoolSize:     1000,
		GrowthFactor:    3,
	}

	allocator := NewDescriptorAllocator(0, nil, config)

	if allocator.device != 0 {
		t.Errorf("device should be 0, got %v", allocator.device)
	}
	if allocator.cmds != nil {
		t.Error("cmds should be nil")
	}
	if len(allocator.pools) != 0 {
		t.Errorf("pools should be empty, got length %d", len(allocator.pools))
	}
	if allocator.totalAllocated != 0 {
		t.Errorf("totalAllocated should be 0, got %d", allocator.totalAllocated)
	}
	if allocator.totalFreed != 0 {
		t.Errorf("totalFreed should be 0, got %d", allocator.totalFreed)
	}
}
