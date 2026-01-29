package hal_test

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	_ "github.com/gogpu/wgpu/hal/noop" // Import for side effect of registering noop backend
)

// mockBackend is a simple test backend implementation.
type mockBackend struct {
	variant gputypes.Backend
}

func (m *mockBackend) Variant() gputypes.Backend {
	return m.variant
}

func (m *mockBackend) CreateInstance(_ *hal.InstanceDescriptor) (hal.Instance, error) {
	return &mockInstance{}, nil
}

// mockInstance is a minimal instance implementation for testing.
type mockInstance struct{}

func (m *mockInstance) CreateSurface(_, _ uintptr) (hal.Surface, error) {
	return &mockSurface{}, nil
}
func (m *mockInstance) EnumerateAdapters(_ hal.Surface) []hal.ExposedAdapter {
	return nil
}
func (m *mockInstance) Destroy() {}

// mockSurface is a minimal surface implementation for testing.
type mockSurface struct{}

func (m *mockSurface) Configure(_ hal.Device, _ *hal.SurfaceConfiguration) error { return nil }
func (m *mockSurface) Unconfigure(_ hal.Device)                                  {}
func (m *mockSurface) AcquireTexture(_ hal.Fence) (*hal.AcquiredSurfaceTexture, error) {
	return &hal.AcquiredSurfaceTexture{
		Texture:    &mockSurfaceTexture{},
		Suboptimal: false,
	}, nil
}
func (m *mockSurface) DiscardTexture(_ hal.SurfaceTexture) {}
func (m *mockSurface) Destroy()                            {}

// mockSurfaceTexture is a minimal surface texture implementation for testing.
type mockSurfaceTexture struct{}

func (m *mockSurfaceTexture) Destroy()              {}
func (m *mockSurfaceTexture) NativeHandle() uintptr { return 0 }

func TestRegisterBackend(t *testing.T) {
	// Register a custom backend
	mock := &mockBackend{variant: gputypes.BackendVulkan}
	hal.RegisterBackend(mock)

	// Verify it was registered
	backend, ok := hal.GetBackend(gputypes.BackendVulkan)
	if !ok {
		t.Fatal("expected backend to be registered")
	}
	if backend.Variant() != gputypes.BackendVulkan {
		t.Errorf("expected variant %v, got %v", gputypes.BackendVulkan, backend.Variant())
	}
}

func TestRegisterBackend_Replacement(t *testing.T) {
	// Register initial backend
	mock1 := &mockBackend{variant: gputypes.BackendMetal}
	hal.RegisterBackend(mock1)

	// Replace with another backend of same type
	mock2 := &mockBackend{variant: gputypes.BackendMetal}
	hal.RegisterBackend(mock2)

	// Verify the replacement
	backend, ok := hal.GetBackend(gputypes.BackendMetal)
	if !ok {
		t.Fatal("expected backend to be registered")
	}

	// Both backends have same variant, but should be the second instance
	// In real scenario, you might have different internal state
	if backend.Variant() != gputypes.BackendMetal {
		t.Errorf("expected variant %v, got %v", gputypes.BackendMetal, backend.Variant())
	}
}

func TestGetBackend(t *testing.T) {
	tests := []struct {
		name    string
		variant gputypes.Backend
		wantOk  bool
	}{
		{
			name:    "noop backend (registered by init)",
			variant: gputypes.BackendEmpty,
			wantOk:  true,
		},
		{
			name:    "unregistered backend",
			variant: gputypes.BackendDX12,
			wantOk:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend, ok := hal.GetBackend(tt.variant)
			if ok != tt.wantOk {
				t.Errorf("GetBackend() ok = %v, want %v", ok, tt.wantOk)
			}
			if ok && backend == nil {
				t.Error("GetBackend() returned ok=true but backend is nil")
			}
			if ok && backend.Variant() != tt.variant {
				t.Errorf("backend.Variant() = %v, want %v", backend.Variant(), tt.variant)
			}
		})
	}
}

func TestGetBackend_NotRegistered(t *testing.T) {
	// Try to get a backend that definitely doesn't exist
	backend, ok := hal.GetBackend(gputypes.BackendGL)
	if ok {
		t.Error("expected GetBackend to return false for unregistered backend")
	}
	if backend != nil {
		t.Error("expected nil backend for unregistered backend")
	}
}

func TestAvailableBackends(t *testing.T) {
	// Get available backends
	backends := hal.AvailableBackends()

	// Should have at least noop backend
	if len(backends) == 0 {
		t.Fatal("expected at least one backend (noop)")
	}

	// Check that noop backend is present
	found := false
	for _, b := range backends {
		if b == gputypes.BackendEmpty {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected BackendEmpty (noop) to be in available backends")
	}
}

func TestAvailableBackends_AfterRegistration(t *testing.T) {
	// Get initial count
	initialBackends := hal.AvailableBackends()
	initialCount := len(initialBackends)

	// Register a new backend (using Vulkan as test backend)
	mock := &mockBackend{variant: gputypes.BackendVulkan}
	hal.RegisterBackend(mock)

	// Get updated list
	updatedBackends := hal.AvailableBackends()
	updatedCount := len(updatedBackends)

	// Should have same or more backends (replacement case)
	if updatedCount < initialCount {
		t.Errorf("expected at least %d backends after registration, got %d", initialCount, updatedCount)
	}

	// Verify the new backend is in the list
	found := false
	for _, b := range updatedBackends {
		if b == gputypes.BackendVulkan {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected newly registered backend to be in available backends")
	}
}

func TestConcurrentAccess(t *testing.T) {
	// Test concurrent registration and retrieval
	done := make(chan bool, 2)

	// Goroutine 1: Register backends
	go func() {
		for i := 0; i < 100; i++ {
			mock := &mockBackend{variant: gputypes.Backend(i % 8)}
			hal.RegisterBackend(mock)
		}
		done <- true
	}()

	// Goroutine 2: Get backends
	go func() {
		for i := 0; i < 100; i++ {
			_ = hal.AvailableBackends()
			_, _ = hal.GetBackend(gputypes.Backend(i % 8))
		}
		done <- true
	}()

	// Wait for completion
	<-done
	<-done
}

func TestNoopBackendRegistered(t *testing.T) {
	// Verify that the noop backend is automatically registered via init()
	backend, ok := hal.GetBackend(gputypes.BackendEmpty)
	if !ok {
		t.Fatal("noop backend should be registered automatically")
	}

	if backend.Variant() != gputypes.BackendEmpty {
		t.Errorf("expected variant BackendEmpty, got %v", backend.Variant())
	}

	// Verify it can create an instance (behavior test instead of type assertion)
	instance, err := backend.CreateInstance(nil)
	if err != nil {
		t.Errorf("expected CreateInstance to succeed for noop backend, got error: %v", err)
	}
	if instance != nil {
		instance.Destroy()
	}
}
