package core

import (
	"errors"
	"fmt"
	"sync"
	"testing"
)

// =============================================================================
// RawID Tests
// =============================================================================

func TestRawID_Zip(t *testing.T) {
	tests := []struct {
		name  string
		index Index
		epoch Epoch
	}{
		{"zero", 0, 0},
		{"index only", 42, 0},
		{"epoch only", 0, 5},
		{"both", 123, 456},
		{"max index", 0xFFFFFFFF, 0},
		{"max epoch", 0, 0xFFFFFFFF},
		{"max both", 0xFFFFFFFF, 0xFFFFFFFF},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := Zip(tt.index, tt.epoch)
			gotIndex, gotEpoch := raw.Unzip()
			if gotIndex != tt.index {
				t.Errorf("Zip(%d, %d).Unzip() index = %d, want %d",
					tt.index, tt.epoch, gotIndex, tt.index)
			}
			if gotEpoch != tt.epoch {
				t.Errorf("Zip(%d, %d).Unzip() epoch = %d, want %d",
					tt.index, tt.epoch, gotEpoch, tt.epoch)
			}
		})
	}
}

func TestRawID_Unzip(t *testing.T) {
	tests := []struct {
		name      string
		raw       RawID
		wantIndex Index
		wantEpoch Epoch
	}{
		{"zero", 0, 0, 0},
		{"index 1", 1, 1, 0},
		{"epoch 1", 1 << 32, 0, 1},
		{"combined", (5 << 32) | 42, 42, 5},
		{"max values", 0xFFFFFFFFFFFFFFFF, 0xFFFFFFFF, 0xFFFFFFFF},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotIndex, gotEpoch := tt.raw.Unzip()
			if gotIndex != tt.wantIndex {
				t.Errorf("RawID(%d).Unzip() index = %d, want %d",
					tt.raw, gotIndex, tt.wantIndex)
			}
			if gotEpoch != tt.wantEpoch {
				t.Errorf("RawID(%d).Unzip() epoch = %d, want %d",
					tt.raw, gotEpoch, tt.wantEpoch)
			}
		})
	}
}

func TestRawID_Index(t *testing.T) {
	tests := []struct {
		name      string
		raw       RawID
		wantIndex Index
	}{
		{"zero", 0, 0},
		{"index only", 42, 42},
		{"with epoch", (5 << 32) | 42, 42},
		{"max index", 0xFFFFFFFF, 0xFFFFFFFF},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.raw.Index()
			if got != tt.wantIndex {
				t.Errorf("RawID(%d).Index() = %d, want %d",
					tt.raw, got, tt.wantIndex)
			}
		})
	}
}

func TestRawID_Epoch(t *testing.T) {
	tests := []struct {
		name      string
		raw       RawID
		wantEpoch Epoch
	}{
		{"zero", 0, 0},
		{"epoch only", 5 << 32, 5},
		{"with index", (5 << 32) | 42, 5},
		{"max epoch", 0xFFFFFFFF << 32, 0xFFFFFFFF},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.raw.Epoch()
			if got != tt.wantEpoch {
				t.Errorf("RawID(%d).Epoch() = %d, want %d",
					tt.raw, got, tt.wantEpoch)
			}
		})
	}
}

func TestRawID_IsZero(t *testing.T) {
	tests := []struct {
		name string
		raw  RawID
		want bool
	}{
		{"zero", 0, true},
		{"index only", 1, false},
		{"epoch only", 1 << 32, false},
		{"both", (1 << 32) | 1, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.raw.IsZero()
			if got != tt.want {
				t.Errorf("RawID(%d).IsZero() = %v, want %v",
					tt.raw, got, tt.want)
			}
		})
	}
}

func TestRawID_String(t *testing.T) {
	tests := []struct {
		raw  RawID
		want string
	}{
		{0, "RawID(0,0)"},
		{Zip(42, 5), "RawID(42,5)"},
		{Zip(0xFFFFFFFF, 0xFFFFFFFF), "RawID(4294967295,4294967295)"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.raw.String()
			if got != tt.want {
				t.Errorf("RawID.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

// =============================================================================
// ID[T] Tests
// =============================================================================

func TestID_NewID(t *testing.T) {
	id := NewID[deviceMarker](42, 5)
	gotIndex, gotEpoch := id.Unzip()
	if gotIndex != 42 {
		t.Errorf("NewID(42, 5).Index() = %d, want 42", gotIndex)
	}
	if gotEpoch != 5 {
		t.Errorf("NewID(42, 5).Epoch() = %d, want 5", gotEpoch)
	}
}

func TestID_FromRaw(t *testing.T) {
	raw := Zip(42, 5)
	id := FromRaw[deviceMarker](raw)
	if id.Raw() != raw {
		t.Errorf("FromRaw().Raw() = %v, want %v", id.Raw(), raw)
	}
}

func TestID_Accessors(t *testing.T) {
	id := NewID[bufferMarker](123, 456)

	if got := id.Index(); got != 123 {
		t.Errorf("ID.Index() = %d, want 123", got)
	}
	if got := id.Epoch(); got != 456 {
		t.Errorf("ID.Epoch() = %d, want 456", got)
	}

	index, epoch := id.Unzip()
	if index != 123 {
		t.Errorf("ID.Unzip() index = %d, want 123", index)
	}
	if epoch != 456 {
		t.Errorf("ID.Unzip() epoch = %d, want 456", epoch)
	}
}

func TestID_IsZero(t *testing.T) {
	zero := NewID[textureMarker](0, 0)
	nonZero := NewID[textureMarker](1, 0)

	if !zero.IsZero() {
		t.Error("zero ID.IsZero() = false, want true")
	}
	if nonZero.IsZero() {
		t.Error("non-zero ID.IsZero() = true, want false")
	}
}

func TestID_String(t *testing.T) {
	id := NewID[samplerMarker](42, 5)
	got := id.String()
	want := "ID(42,5)"
	if got != want {
		t.Errorf("ID.String() = %q, want %q", got, want)
	}
}

func TestID_TypeSafety(t *testing.T) {
	// This test verifies compile-time type safety
	// These should not compile if uncommented:
	// var deviceID DeviceID
	// var bufferID BufferID
	// deviceID = bufferID // Error: cannot use bufferID (type BufferID) as type DeviceID

	// Verify different marker types create different ID types
	deviceID := NewID[deviceMarker](1, 1)
	bufferID := NewID[bufferMarker](1, 1)

	// They have the same raw value but different types
	if deviceID.Raw() != bufferID.Raw() {
		t.Error("Same index/epoch should produce same raw value")
	}

	// Type assertion should fail (compile-time check)
	_ = deviceID
	_ = bufferID
}

// =============================================================================
// IdentityManager Tests
// =============================================================================

func TestIdentityManager_AllocRelease(t *testing.T) {
	m := NewIdentityManager[deviceMarker]()

	// First allocation should start at index 0, epoch 1
	id1 := m.Alloc()
	if idx := id1.Index(); idx != 0 {
		t.Errorf("First alloc index = %d, want 0", idx)
	}
	if epoch := id1.Epoch(); epoch != 1 {
		t.Errorf("First alloc epoch = %d, want 1", epoch)
	}

	// Second allocation should be index 1
	id2 := m.Alloc()
	if idx := id2.Index(); idx != 1 {
		t.Errorf("Second alloc index = %d, want 1", idx)
	}

	// Count should be 2
	if count := m.Count(); count != 2 {
		t.Errorf("Count after 2 allocs = %d, want 2", count)
	}

	// Release first ID
	m.Release(id1)
	if count := m.Count(); count != 1 {
		t.Errorf("Count after release = %d, want 1", count)
	}

	// Next allocation should reuse index 0 but with epoch 2
	id3 := m.Alloc()
	if idx := id3.Index(); idx != 0 {
		t.Errorf("Reused alloc index = %d, want 0", idx)
	}
	if epoch := id3.Epoch(); epoch != 2 {
		t.Errorf("Reused alloc epoch = %d, want 2", epoch)
	}
}

func TestIdentityManager_EpochIncrement(t *testing.T) {
	m := NewIdentityManager[bufferMarker]()

	// Allocate and release multiple times
	var prevEpoch Epoch
	for i := 0; i < 5; i++ {
		id := m.Alloc()
		epoch := id.Epoch()
		if epoch <= prevEpoch {
			t.Errorf("Iteration %d: epoch %d not greater than previous %d",
				i, epoch, prevEpoch)
		}
		prevEpoch = epoch
		m.Release(id)
	}
}

func TestIdentityManager_MultipleFreeSlots(t *testing.T) {
	m := NewIdentityManager[textureMarker]()

	// Allocate several IDs
	ids := make([]ID[textureMarker], 10)
	for i := range ids {
		ids[i] = m.Alloc()
	}

	// Release them all
	for _, id := range ids {
		m.Release(id)
	}

	if free := m.FreeCount(); free != 10 {
		t.Errorf("FreeCount after releasing 10 = %d, want 10", free)
	}

	// Allocate again - should reuse in LIFO order (last released first)
	newID := m.Alloc()
	// Should reuse index 9 (last released)
	if idx := newID.Index(); idx != 9 {
		t.Errorf("After releasing all, next alloc index = %d, want 9", idx)
	}
}

func TestIdentityManager_NextIndex(t *testing.T) {
	m := NewIdentityManager[samplerMarker]()

	if next := m.NextIndex(); next != 0 {
		t.Errorf("Initial NextIndex = %d, want 0", next)
	}

	m.Alloc()
	if next := m.NextIndex(); next != 1 {
		t.Errorf("After one alloc NextIndex = %d, want 1", next)
	}

	// Releasing shouldn't change NextIndex
	id := m.Alloc()
	m.Release(id)
	if next := m.NextIndex(); next != 2 {
		t.Errorf("After alloc+release NextIndex = %d, want 2", next)
	}
}

func TestIdentityManager_Concurrent(t *testing.T) {
	m := NewIdentityManager[deviceMarker]()
	const goroutines = 100
	const allocsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < allocsPerGoroutine; j++ {
				id := m.Alloc()
				m.Release(id)
			}
		}()
	}

	wg.Wait()

	// All should be released
	if count := m.Count(); count != 0 {
		t.Errorf("After concurrent alloc/release count = %d, want 0", count)
	}
}

// =============================================================================
// Storage Tests
// =============================================================================

func TestStorage_InsertGet(t *testing.T) {
	s := NewStorage[string, deviceMarker](0)

	id := NewID[deviceMarker](0, 1)
	s.Insert(id, "test-device")

	got, ok := s.Get(id)
	if !ok {
		t.Fatal("Get() returned not found")
	}
	if got != "test-device" {
		t.Errorf("Get() = %q, want %q", got, "test-device")
	}
}

func TestStorage_EpochValidation(t *testing.T) {
	s := NewStorage[string, bufferMarker](0)

	// Insert with epoch 1
	id1 := NewID[bufferMarker](0, 1)
	s.Insert(id1, "buffer-v1")

	// Try to get with epoch 2 (wrong epoch)
	id2 := NewID[bufferMarker](0, 2)
	_, ok := s.Get(id2)
	if ok {
		t.Error("Get with wrong epoch returned found, want not found")
	}

	// Original ID should still work
	got, ok := s.Get(id1)
	if !ok || got != "buffer-v1" {
		t.Errorf("Get with correct epoch = (%q, %v), want (%q, true)",
			got, ok, "buffer-v1")
	}
}

func TestStorage_Remove(t *testing.T) {
	s := NewStorage[int, textureMarker](0)

	id := NewID[textureMarker](5, 1)
	s.Insert(id, 42)

	// Remove should return the item
	removed, ok := s.Remove(id)
	if !ok {
		t.Fatal("Remove() returned not found")
	}
	if removed != 42 {
		t.Errorf("Remove() = %d, want 42", removed)
	}

	// Item should no longer exist
	if s.Contains(id) {
		t.Error("Contains() after Remove() = true, want false")
	}

	// Get should fail
	_, ok = s.Get(id)
	if ok {
		t.Error("Get() after Remove() returned found")
	}
}

func TestStorage_GetMut(t *testing.T) {
	s := NewStorage[int, samplerMarker](0)

	id := NewID[samplerMarker](0, 1)
	s.Insert(id, 100)

	// Mutate the value
	ok := s.GetMut(id, func(val *int) {
		*val = 200
	})
	if !ok {
		t.Fatal("GetMut() returned false")
	}

	// Verify mutation
	got, ok := s.Get(id)
	if !ok || got != 200 {
		t.Errorf("After GetMut, Get() = (%d, %v), want (200, true)", got, ok)
	}
}

func TestStorage_Contains(t *testing.T) {
	s := NewStorage[string, bindGroupMarker](0)

	id := NewID[bindGroupMarker](3, 1)

	if s.Contains(id) {
		t.Error("Contains() before insert = true, want false")
	}

	s.Insert(id, "bind-group")

	if !s.Contains(id) {
		t.Error("Contains() after insert = false, want true")
	}

	s.Remove(id)

	if s.Contains(id) {
		t.Error("Contains() after remove = true, want false")
	}
}

func TestStorage_Len(t *testing.T) {
	s := NewStorage[string, shaderModuleMarker](0)

	if got := s.Len(); got != 0 {
		t.Errorf("Initial Len() = %d, want 0", got)
	}

	s.Insert(NewID[shaderModuleMarker](0, 1), "shader1")
	s.Insert(NewID[shaderModuleMarker](1, 1), "shader2")
	s.Insert(NewID[shaderModuleMarker](2, 1), "shader3")

	if got := s.Len(); got != 3 {
		t.Errorf("Len() after 3 inserts = %d, want 3", got)
	}

	s.Remove(NewID[shaderModuleMarker](1, 1))

	if got := s.Len(); got != 2 {
		t.Errorf("Len() after remove = %d, want 2", got)
	}
}

func TestStorage_Capacity(t *testing.T) {
	s := NewStorage[string, renderPipelineMarker](10)

	// Initial capacity should be 0 (no slots allocated yet)
	if capacity := s.Capacity(); capacity != 0 {
		t.Errorf("Initial Capacity() = %d, want 0", capacity)
	}

	// Insert at index 50 - storage will auto-grow
	id := NewID[renderPipelineMarker](50, 1)
	s.Insert(id, "pipeline")

	// Capacity should have grown to at least 51 (index 50 needs 51 slots)
	if capacity := s.Capacity(); capacity <= 50 {
		t.Errorf("Capacity() after insert at index 50 = %d, want > 50", capacity)
	}
}

func TestStorage_ForEach(t *testing.T) {
	s := NewStorage[int, commandBufferMarker](0)

	// Insert some items
	items := map[Index]int{
		0: 100,
		2: 200,
		5: 300,
	}

	for idx, val := range items {
		s.Insert(NewID[commandBufferMarker](idx, 1), val)
	}

	// Collect via ForEach
	collected := make(map[Index]int)
	s.ForEach(func(id ID[commandBufferMarker], val int) bool {
		collected[id.Index()] = val
		return true
	})

	// Verify all items collected
	for idx, want := range items {
		got, ok := collected[idx]
		if !ok {
			t.Errorf("ForEach didn't visit index %d", idx)
			continue
		}
		if got != want {
			t.Errorf("ForEach index %d = %d, want %d", idx, got, want)
		}
	}
}

func TestStorage_ForEach_EarlyStop(t *testing.T) {
	s := NewStorage[int, querySetMarker](0)

	for i := 0; i < 10; i++ {
		s.Insert(NewID[querySetMarker](Index(i), 1), i)
	}

	count := 0
	s.ForEach(func(_ ID[querySetMarker], _ int) bool {
		count++
		return count < 5 // Stop after 5 items
	})

	if count != 5 {
		t.Errorf("ForEach with early stop visited %d items, want 5", count)
	}
}

func TestStorage_Clear(t *testing.T) {
	s := NewStorage[string, deviceMarker](0)

	s.Insert(NewID[deviceMarker](0, 1), "device1")
	s.Insert(NewID[deviceMarker](1, 1), "device2")

	s.Clear()

	if length := s.Len(); length != 0 {
		t.Errorf("Len() after Clear() = %d, want 0", length)
	}

	// Items should not be accessible
	if s.Contains(NewID[deviceMarker](0, 1)) {
		t.Error("Contains() after Clear() = true for previously inserted item")
	}
}

func TestStorage_AutoGrow(t *testing.T) {
	s := NewStorage[string, bufferMarker](0)

	// Insert at large index
	largeIndex := Index(1000)
	id := NewID[bufferMarker](largeIndex, 1)
	s.Insert(id, "large-buffer")

	// Should auto-grow and be accessible
	got, ok := s.Get(id)
	if !ok || got != "large-buffer" {
		t.Errorf("Get after auto-grow = (%q, %v), want (%q, true)",
			got, ok, "large-buffer")
	}

	// Capacity should have grown beyond the index
	if capacity := s.Capacity(); capacity <= int(largeIndex) {
		t.Errorf("Capacity after insert at %d = %d, want > %d",
			largeIndex, capacity, largeIndex)
	}
}

func TestStorage_Concurrent(t *testing.T) {
	s := NewStorage[int, textureMarker](0)
	const goroutines = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)

	// Concurrent inserts
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			id := NewID[textureMarker](Index(idx), 1)
			s.Insert(id, idx)
		}(i)
	}

	wg.Wait()

	// Verify all inserted
	if got := s.Len(); got != goroutines {
		t.Errorf("After concurrent inserts Len() = %d, want %d",
			got, goroutines)
	}

	// Concurrent reads
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			id := NewID[textureMarker](Index(idx), 1)
			val, ok := s.Get(id)
			if !ok || val != idx {
				t.Errorf("Concurrent Get(%d) = (%d, %v), want (%d, true)",
					idx, val, ok, idx)
			}
		}(i)
	}

	wg.Wait()
}

// =============================================================================
// Registry Tests
// =============================================================================

type testResource struct {
	name string
	data int
}

func TestRegistry_RegisterGet(t *testing.T) {
	r := NewRegistry[testResource, deviceMarker]()

	res := testResource{name: "test-device", data: 42}
	id := r.Register(res)

	got, err := r.Get(id)
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if got.name != res.name || got.data != res.data {
		t.Errorf("Get() = %+v, want %+v", got, res)
	}
}

func TestRegistry_GetInvalidID(t *testing.T) {
	r := NewRegistry[testResource, bufferMarker]()

	// Zero ID
	_, err := r.Get(NewID[bufferMarker](0, 0))
	if !errors.Is(err, ErrInvalidID) {
		t.Errorf("Get(zero ID) error = %v, want ErrInvalidID", err)
	}

	// Non-existent ID
	_, err = r.Get(NewID[bufferMarker](999, 1))
	if !errors.Is(err, ErrResourceNotFound) {
		t.Errorf("Get(non-existent) error = %v, want ErrResourceNotFound", err)
	}
}

func TestRegistry_GetEpochMismatch(t *testing.T) {
	r := NewRegistry[testResource, textureMarker]()

	res := testResource{name: "texture", data: 100}
	id := r.Register(res)

	// Unregister it
	_, err := r.Unregister(id)
	if err != nil {
		t.Fatalf("Unregister() error = %v", err)
	}

	// Register another resource (will reuse index with new epoch)
	newRes := testResource{name: "new-texture", data: 200}
	newID := r.Register(newRes)

	// Old ID should fail with epoch mismatch
	_, err = r.Get(id)
	if !errors.Is(err, ErrEpochMismatch) {
		t.Errorf("Get(old ID) error = %v, want ErrEpochMismatch", err)
	}

	// New ID should work
	got, err := r.Get(newID)
	if err != nil {
		t.Fatalf("Get(new ID) error = %v", err)
	}
	if got.name != newRes.name {
		t.Errorf("Get(new ID) name = %q, want %q", got.name, newRes.name)
	}
}

func TestRegistry_GetMut(t *testing.T) {
	r := NewRegistry[testResource, samplerMarker]()

	res := testResource{name: "sampler", data: 10}
	id := r.Register(res)

	// Mutate via GetMut
	err := r.GetMut(id, func(res *testResource) {
		res.data = 20
	})
	if err != nil {
		t.Fatalf("GetMut() error = %v", err)
	}

	// Verify mutation
	got, err := r.Get(id)
	if err != nil {
		t.Fatalf("Get() after GetMut error = %v", err)
	}
	if got.data != 20 {
		t.Errorf("After GetMut, data = %d, want 20", got.data)
	}
}

func TestRegistry_Unregister(t *testing.T) {
	r := NewRegistry[testResource, bindGroupLayoutMarker]()

	res := testResource{name: "layout", data: 5}
	id := r.Register(res)

	// Unregister should return the resource
	removed, err := r.Unregister(id)
	if err != nil {
		t.Fatalf("Unregister() error = %v", err)
	}
	if removed.name != res.name {
		t.Errorf("Unregister() = %+v, want %+v", removed, res)
	}

	// Count should decrease
	if count := r.Count(); count != 0 {
		t.Errorf("Count() after Unregister = %d, want 0", count)
	}

	// ID should no longer be valid
	if r.Contains(id) {
		t.Error("Contains() after Unregister = true, want false")
	}
}

func TestRegistry_Contains(t *testing.T) {
	r := NewRegistry[testResource, pipelineLayoutMarker]()

	id := r.Register(testResource{name: "pipeline", data: 1})

	if !r.Contains(id) {
		t.Error("Contains() after Register = false, want true")
	}

	r.Unregister(id)

	if r.Contains(id) {
		t.Error("Contains() after Unregister = true, want false")
	}

	// Zero ID
	if r.Contains(NewID[pipelineLayoutMarker](0, 0)) {
		t.Error("Contains(zero ID) = true, want false")
	}
}

func TestRegistry_Count(t *testing.T) {
	r := NewRegistry[testResource, shaderModuleMarker]()

	if count := r.Count(); count != 0 {
		t.Errorf("Initial Count() = %d, want 0", count)
	}

	id1 := r.Register(testResource{name: "shader1", data: 1})
	id2 := r.Register(testResource{name: "shader2", data: 2})
	id3 := r.Register(testResource{name: "shader3", data: 3})

	if count := r.Count(); count != 3 {
		t.Errorf("Count() after 3 registers = %d, want 3", count)
	}

	r.Unregister(id2)

	if count := r.Count(); count != 2 {
		t.Errorf("Count() after unregister = %d, want 2", count)
	}

	r.Unregister(id1)
	r.Unregister(id3)

	if count := r.Count(); count != 0 {
		t.Errorf("Final Count() = %d, want 0", count)
	}
}

func TestRegistry_ForEach(t *testing.T) {
	r := NewRegistry[testResource, renderPipelineMarker]()

	resources := []testResource{
		{name: "pipeline1", data: 100},
		{name: "pipeline2", data: 200},
		{name: "pipeline3", data: 300},
	}

	for _, res := range resources {
		r.Register(res)
	}

	// Collect all via ForEach
	collected := make(map[string]int)
	r.ForEach(func(_ ID[renderPipelineMarker], res testResource) bool {
		collected[res.name] = res.data
		return true
	})

	// Verify all collected
	if len(collected) != len(resources) {
		t.Errorf("ForEach collected %d items, want %d",
			len(collected), len(resources))
	}

	for _, res := range resources {
		if data, ok := collected[res.name]; !ok {
			t.Errorf("ForEach didn't visit %q", res.name)
		} else if data != res.data {
			t.Errorf("ForEach %q data = %d, want %d",
				res.name, data, res.data)
		}
	}
}

func TestRegistry_Clear(t *testing.T) {
	r := NewRegistry[testResource, computePipelineMarker]()

	id1 := r.Register(testResource{name: "compute1", data: 1})
	id2 := r.Register(testResource{name: "compute2", data: 2})

	r.Clear()

	// Note: Clear only clears storage, doesn't release IDs
	// So Count() will still reflect allocated IDs (documented behavior)
	if count := r.Count(); count != 2 {
		t.Errorf("Count() after Clear() = %d, want 2 (Clear doesn't release IDs)", count)
	}

	// But the items should not be accessible via Contains
	if r.Contains(id1) {
		t.Error("Contains(id1) after Clear() = true, want false")
	}
	if r.Contains(id2) {
		t.Error("Contains(id2) after Clear() = true, want false")
	}
}

func TestRegistry_IDReuse(t *testing.T) {
	r := NewRegistry[testResource, commandEncoderMarker]()

	// Register and unregister
	res1 := testResource{name: "encoder1", data: 1}
	id1 := r.Register(res1)
	index1 := id1.Index()
	epoch1 := id1.Epoch()

	_, err := r.Unregister(id1)
	if err != nil {
		t.Fatalf("Unregister() error = %v", err)
	}

	// Register again - should reuse index with incremented epoch
	res2 := testResource{name: "encoder2", data: 2}
	id2 := r.Register(res2)
	index2 := id2.Index()
	epoch2 := id2.Epoch()

	if index2 != index1 {
		t.Errorf("Reused ID index = %d, want %d (same as original)",
			index2, index1)
	}
	if epoch2 <= epoch1 {
		t.Errorf("Reused ID epoch = %d, want > %d", epoch2, epoch1)
	}

	// Old ID should fail
	_, err = r.Get(id1)
	if !errors.Is(err, ErrEpochMismatch) {
		t.Errorf("Get(old ID) error = %v, want ErrEpochMismatch", err)
	}

	// New ID should work
	got, err := r.Get(id2)
	if err != nil {
		t.Fatalf("Get(new ID) error = %v", err)
	}
	if got.name != res2.name {
		t.Errorf("Get(new ID) = %+v, want %+v", got, res2)
	}
}

func TestRegistry_Concurrent(t *testing.T) {
	r := NewRegistry[testResource, commandBufferMarker]()
	const goroutines = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	ids := make([]ID[commandBufferMarker], goroutines)

	// Concurrent registrations
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			res := testResource{
				name: fmt.Sprintf("buffer-%d", idx),
				data: idx,
			}
			ids[idx] = r.Register(res)
		}(i)
	}

	wg.Wait()

	// Verify all registered
	if count := r.Count(); count != goroutines {
		t.Errorf("Count after concurrent registers = %d, want %d",
			count, goroutines)
	}

	// Concurrent reads
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			got, err := r.Get(ids[idx])
			if err != nil {
				t.Errorf("Concurrent Get(%d) error = %v", idx, err)
				return
			}
			if got.data != idx {
				t.Errorf("Concurrent Get(%d) data = %d, want %d",
					idx, got.data, idx)
			}
		}(i)
	}

	wg.Wait()
}

// =============================================================================
// Error Tests
// =============================================================================

func TestValidationError(t *testing.T) {
	err := NewValidationError("Buffer", "size", "must be > 0")
	want := "Buffer.size: must be > 0"
	if got := err.Error(); got != want {
		t.Errorf("ValidationError.Error() = %q, want %q", got, want)
	}
}

func TestValidationErrorf(t *testing.T) {
	err := NewValidationErrorf("Texture", "width", "must be <= %d, got %d", 4096, 8192)
	want := "Texture.width: must be <= 4096, got 8192"
	if got := err.Error(); got != want {
		t.Errorf("ValidationErrorf.Error() = %q, want %q", got, want)
	}
}

func TestIDError(t *testing.T) {
	id := Zip(42, 5)
	err := NewIDError(id, "invalid", nil)
	want := "ID(42,5): invalid"
	if got := err.Error(); got != want {
		t.Errorf("IDError.Error() = %q, want %q", got, want)
	}
}

func TestLimitError(t *testing.T) {
	err := NewLimitError("Buffer", "size", 1024, 512)
	want := "Buffer: size exceeded (got 1024, max 512)"
	if got := err.Error(); got != want {
		t.Errorf("LimitError.Error() = %q, want %q", got, want)
	}
}

func TestFeatureError(t *testing.T) {
	err := NewFeatureError("Texture", "depth-clip-control")
	want := "Texture: requires feature 'depth-clip-control' which is not enabled"
	if got := err.Error(); got != want {
		t.Errorf("FeatureError.Error() = %q, want %q", got, want)
	}
}

func TestErrorTypeChecks(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		checkFn  func(error) bool
		wantTrue bool
	}{
		{"ValidationError is", NewValidationError("T", "f", "m"), IsValidationError, true},
		{"ValidationError not", errors.New("test"), IsValidationError, false},
		{"IDError is", NewIDError(Zip(1, 1), "test", nil), IsIDError, true},
		{"IDError not", errors.New("test"), IsIDError, false},
		{"LimitError is", NewLimitError("T", "l", 1, 2), IsLimitError, true},
		{"LimitError not", errors.New("test"), IsLimitError, false},
		{"FeatureError is", NewFeatureError("T", "f"), IsFeatureError, true},
		{"FeatureError not", errors.New("test"), IsFeatureError, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.checkFn(tt.err)
			if got != tt.wantTrue {
				t.Errorf("%s(%v) = %v, want %v",
					tt.name, tt.err, got, tt.wantTrue)
			}
		})
	}
}

func TestErrorUnwrap(t *testing.T) {
	cause := errors.New("underlying cause")

	ve := &ValidationError{Cause: cause}
	if unwrapped := errors.Unwrap(ve); !errors.Is(unwrapped, cause) {
		t.Errorf("ValidationError.Unwrap() = %v, want %v", unwrapped, cause)
	}

	ie := &IDError{Cause: cause}
	if unwrapped := errors.Unwrap(ie); !errors.Is(unwrapped, cause) {
		t.Errorf("IDError.Unwrap() = %v, want %v", unwrapped, cause)
	}
}

// =============================================================================
// Benchmarks
// =============================================================================

func BenchmarkRawID_Zip(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = Zip(Index(i), Epoch(i))
	}
}

func BenchmarkRawID_Unzip(b *testing.B) {
	id := Zip(123, 456)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = id.Unzip()
	}
}

func BenchmarkID_NewID(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = NewID[deviceMarker](Index(i), Epoch(i))
	}
}

func BenchmarkIdentityManager_Alloc(b *testing.B) {
	m := NewIdentityManager[deviceMarker]()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.Alloc()
	}
}

func BenchmarkIdentityManager_AllocRelease(b *testing.B) {
	m := NewIdentityManager[bufferMarker]()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id := m.Alloc()
		m.Release(id)
	}
}

func BenchmarkStorage_Insert(b *testing.B) {
	s := NewStorage[int, textureMarker](0)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id := NewID[textureMarker](Index(i%1000), 1)
		s.Insert(id, i)
	}
}

func BenchmarkStorage_Get(b *testing.B) {
	s := NewStorage[int, samplerMarker](0)
	id := NewID[samplerMarker](0, 1)
	s.Insert(id, 42)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = s.Get(id)
	}
}

func BenchmarkStorage_GetMut(b *testing.B) {
	s := NewStorage[int, bindGroupMarker](0)
	id := NewID[bindGroupMarker](0, 1)
	s.Insert(id, 42)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = s.GetMut(id, func(val *int) {
			*val++
		})
	}
}

func BenchmarkRegistry_Register(b *testing.B) {
	r := NewRegistry[testResource, shaderModuleMarker]()
	res := testResource{name: "shader", data: 100}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = r.Register(res)
	}
}

func BenchmarkRegistry_Get(b *testing.B) {
	r := NewRegistry[testResource, renderPipelineMarker]()
	id := r.Register(testResource{name: "pipeline", data: 42})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = r.Get(id)
	}
}

func BenchmarkRegistry_RegisterUnregister(b *testing.B) {
	r := NewRegistry[testResource, commandBufferMarker]()
	res := testResource{name: "buffer", data: 1}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id := r.Register(res)
		_, _ = r.Unregister(id)
	}
}

// Parallel benchmarks
func BenchmarkIdentityManager_Alloc_Parallel(b *testing.B) {
	m := NewIdentityManager[deviceMarker]()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = m.Alloc()
		}
	})
}

func BenchmarkStorage_Get_Parallel(b *testing.B) {
	s := NewStorage[int, textureMarker](0)
	// Pre-populate
	for i := 0; i < 100; i++ {
		id := NewID[textureMarker](Index(i), 1)
		s.Insert(id, i)
	}
	id := NewID[textureMarker](50, 1)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = s.Get(id)
		}
	})
}

func BenchmarkRegistry_Get_Parallel(b *testing.B) {
	r := NewRegistry[testResource, bufferMarker]()
	id := r.Register(testResource{name: "buffer", data: 42})
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = r.Get(id)
		}
	})
}
