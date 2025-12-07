package core

import (
	"errors"
	"sync"
	"testing"
)

func TestGetGlobal(t *testing.T) {
	global := GetGlobal()
	if global == nil {
		t.Fatal("GetGlobal returned nil")
	}

	// Verify it's a singleton
	global2 := GetGlobal()
	if global != global2 {
		t.Error("GetGlobal returned different instances")
	}
}

func TestGlobalHub(t *testing.T) {
	global := GetGlobal()
	hub := global.Hub()

	if hub == nil {
		t.Fatal("Global.Hub() returned nil")
	}

	// Verify hub is accessible
	id := hub.RegisterBuffer(Buffer{})
	_, err := hub.GetBuffer(id)
	if err != nil {
		t.Errorf("Hub access failed: %v", err)
	}
}

func TestGlobalSurface(t *testing.T) {
	global := GetGlobal()
	surface := Surface{}

	// Register
	id := global.RegisterSurface(surface)
	if id.IsZero() {
		t.Fatal("RegisterSurface returned zero ID")
	}

	// Get
	got, err := global.GetSurface(id)
	if err != nil {
		t.Fatalf("GetSurface failed: %v", err)
	}
	if got != surface {
		t.Error("GetSurface returned different surface")
	}

	// Count
	count := global.SurfaceCount()
	if count != 1 {
		t.Errorf("SurfaceCount = %d, want 1", count)
	}

	// Unregister
	removed, err := global.UnregisterSurface(id)
	if err != nil {
		t.Fatalf("UnregisterSurface failed: %v", err)
	}
	if removed != surface {
		t.Error("UnregisterSurface returned different surface")
	}

	// Get after unregister should fail
	_, err = global.GetSurface(id)
	if err == nil {
		t.Error("GetSurface after unregister should fail")
	}

	// Count should be 0
	count = global.SurfaceCount()
	if count != 0 {
		t.Errorf("SurfaceCount after unregister = %d, want 0", count)
	}
}

func TestGlobalStats(t *testing.T) {
	global := GetGlobal()

	// Take initial snapshot
	initialStats := global.Stats()

	// Register resources
	surfaceID := global.RegisterSurface(Surface{})
	adapterID := global.Hub().RegisterAdapter(&Adapter{})
	deviceID := global.Hub().RegisterDevice(Device{})
	bufferID := global.Hub().RegisterBuffer(Buffer{})

	afterStats := global.Stats()

	// Check deltas (increases by 1 each)
	deltas := map[string]uint64{
		"surfaces": 1,
		"adapters": 1,
		"devices":  1,
		"buffers":  1,
	}

	for resource, expectedDelta := range deltas {
		actualDelta := afterStats[resource] - initialStats[resource]
		if actualDelta != expectedDelta {
			t.Errorf("Stats[%s] delta = %d, want %d", resource, actualDelta, expectedDelta)
		}
	}

	// Verify Stats includes all expected resource types
	expectedTypes := []string{
		"surfaces", "adapters", "devices", "queues", "buffers",
		"textures", "textureViews", "samplers",
		"bindGroupLayouts", "pipelineLayouts", "bindGroups",
		"shaderModules", "renderPipelines", "computePipelines",
		"commandEncoders", "commandBuffers", "querySets",
	}
	for _, resourceType := range expectedTypes {
		if _, ok := afterStats[resourceType]; !ok {
			t.Errorf("Stats missing %s", resourceType)
		}
	}

	// Clean up after test
	_, _ = global.UnregisterSurface(surfaceID)
	_, _ = global.Hub().UnregisterAdapter(adapterID)
	_, _ = global.Hub().UnregisterDevice(deviceID)
	_, _ = global.Hub().UnregisterBuffer(bufferID)
}

func TestGlobalClear(t *testing.T) {
	global := GetGlobal()
	global.Clear() // Start clean

	// Register resources
	surfaceID := global.RegisterSurface(Surface{})
	adapterID := global.Hub().RegisterAdapter(&Adapter{})
	deviceID := global.Hub().RegisterDevice(Device{})
	bufferID := global.Hub().RegisterBuffer(Buffer{})

	// Verify they exist
	_, err := global.GetSurface(surfaceID)
	if err != nil {
		t.Fatalf("GetSurface failed: %v", err)
	}

	// Clear removes storage but doesn't reset counts
	global.Clear()

	// Verify resources are no longer accessible
	_, err = global.GetSurface(surfaceID)
	if err == nil {
		t.Error("GetSurface should fail after Clear")
	}
	_, err = global.Hub().GetAdapter(adapterID)
	if err == nil {
		t.Error("GetAdapter should fail after Clear")
	}
	_, err = global.Hub().GetDevice(deviceID)
	if err == nil {
		t.Error("GetDevice should fail after Clear")
	}
	_, err = global.Hub().GetBuffer(bufferID)
	if err == nil {
		t.Error("GetBuffer should fail after Clear")
	}
}

func TestGlobalConcurrentSurfaceAccess(t *testing.T) {
	global := GetGlobal()

	// Track initial count
	initialCount := global.SurfaceCount()

	const goroutines = 10
	const opsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				// Register
				id := global.RegisterSurface(Surface{})

				// Get
				_, err := global.GetSurface(id)
				if err != nil {
					t.Errorf("GetSurface failed: %v", err)
				}

				// Unregister
				_, err = global.UnregisterSurface(id)
				if err != nil {
					t.Errorf("UnregisterSurface failed: %v", err)
				}
			}
		}()
	}

	wg.Wait()

	// Count should be back to initial
	finalCount := global.SurfaceCount()
	if finalCount != initialCount {
		t.Errorf("After concurrent test, surface count = %d, want %d", finalCount, initialCount)
	}
}

func TestGlobalConcurrentHubAccess(t *testing.T) {
	global := GetGlobal()

	// Track initial count
	initialStats := global.Stats()
	initialBufferCount := initialStats["buffers"]

	const goroutines = 10
	const opsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			hub := global.Hub()
			for j := 0; j < opsPerGoroutine; j++ {
				// Register
				id := hub.RegisterBuffer(Buffer{})

				// Get
				_, err := hub.GetBuffer(id)
				if err != nil {
					t.Errorf("GetBuffer failed: %v", err)
				}

				// Unregister
				_, err = hub.UnregisterBuffer(id)
				if err != nil {
					t.Errorf("UnregisterBuffer failed: %v", err)
				}
			}
		}()
	}

	wg.Wait()

	// Count should be back to initial
	finalStats := global.Stats()
	if finalStats["buffers"] != initialBufferCount {
		t.Errorf("After concurrent test, buffer count = %d, want %d", finalStats["buffers"], initialBufferCount)
	}
}

func TestGlobalMixedOperations(t *testing.T) {
	global := GetGlobal()

	// Take initial snapshot
	initialStats := global.Stats()

	// Register surfaces and hub resources in mixed order
	surfaceID1 := global.RegisterSurface(Surface{})
	adapterID := global.Hub().RegisterAdapter(&Adapter{})
	surfaceID2 := global.RegisterSurface(Surface{})
	deviceID := global.Hub().RegisterDevice(Device{})
	bufferID := global.Hub().RegisterBuffer(Buffer{})

	// Verify all are accessible
	if _, err := global.GetSurface(surfaceID1); err != nil {
		t.Errorf("GetSurface(1) failed: %v", err)
	}
	if _, err := global.GetSurface(surfaceID2); err != nil {
		t.Errorf("GetSurface(2) failed: %v", err)
	}
	if _, err := global.Hub().GetAdapter(adapterID); err != nil {
		t.Errorf("GetAdapter failed: %v", err)
	}
	if _, err := global.Hub().GetDevice(deviceID); err != nil {
		t.Errorf("GetDevice failed: %v", err)
	}
	if _, err := global.Hub().GetBuffer(bufferID); err != nil {
		t.Errorf("GetBuffer failed: %v", err)
	}

	// Verify deltas
	afterStats := global.Stats()
	if afterStats["surfaces"]-initialStats["surfaces"] != 2 {
		t.Errorf("surface count delta = %d, want 2", afterStats["surfaces"]-initialStats["surfaces"])
	}
	if afterStats["adapters"]-initialStats["adapters"] != 1 {
		t.Errorf("adapter count delta = %d, want 1", afterStats["adapters"]-initialStats["adapters"])
	}
	if afterStats["devices"]-initialStats["devices"] != 1 {
		t.Errorf("device count delta = %d, want 1", afterStats["devices"]-initialStats["devices"])
	}
	if afterStats["buffers"]-initialStats["buffers"] != 1 {
		t.Errorf("buffer count delta = %d, want 1", afterStats["buffers"]-initialStats["buffers"])
	}

	// Clean up after test
	_, _ = global.UnregisterSurface(surfaceID1)
	_, _ = global.UnregisterSurface(surfaceID2)
	_, _ = global.Hub().UnregisterAdapter(adapterID)
	_, _ = global.Hub().UnregisterDevice(deviceID)
	_, _ = global.Hub().UnregisterBuffer(bufferID)
}

func TestGlobalInvalidSurfaceID(t *testing.T) {
	global := GetGlobal()

	// Try to get with zero ID
	zeroID := SurfaceID{}
	_, err := global.GetSurface(zeroID)
	if !errors.Is(err, ErrInvalidID) {
		t.Errorf("GetSurface with zero ID: got error %v, want %v", err, ErrInvalidID)
	}

	// Try to unregister with zero ID
	_, err = global.UnregisterSurface(zeroID)
	if !errors.Is(err, ErrInvalidID) {
		t.Errorf("UnregisterSurface with zero ID: got error %v, want %v", err, ErrInvalidID)
	}
}

func TestGlobalSurfaceEpochMismatch(t *testing.T) {
	global := GetGlobal()
	global.Clear() // Start clean

	// Register and unregister to increment epoch
	id1 := global.RegisterSurface(Surface{})
	_, err := global.UnregisterSurface(id1)
	if err != nil {
		t.Fatalf("UnregisterSurface failed: %v", err)
	}

	// Try to get with old ID (epoch mismatch)
	_, err = global.GetSurface(id1)
	if !errors.Is(err, ErrEpochMismatch) {
		t.Errorf("GetSurface with old ID: got error %v, want %v", err, ErrEpochMismatch)
	}
}

func TestGlobalSingletonAcrossGoroutines(t *testing.T) {
	const goroutines = 100
	instances := make([]*Global, goroutines)

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(index int) {
			defer wg.Done()
			instances[index] = GetGlobal()
		}(i)
	}

	wg.Wait()

	// Verify all instances are the same
	first := instances[0]
	for i := 1; i < goroutines; i++ {
		if instances[i] != first {
			t.Errorf("Instance %d is different from instance 0", i)
		}
	}
}
