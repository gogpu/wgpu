package hal_test

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	_ "github.com/gogpu/wgpu/hal/noop" // Import for side effect of registering noop backend
)

// Use non-standard backend variant numbers to avoid interfering with
// registry_test.go which checks that specific standard variants are not registered.
const (
	testFactoryVariant1 = gputypes.Backend(200) // unique test variant
	testFactoryVariant2 = gputypes.Backend(201) // unique test variant
	testFactoryVariant3 = gputypes.Backend(202) // unique test variant
)

// factoryTestBackend implements hal.Backend for factory tests.
type factoryTestBackend struct {
	variant gputypes.Backend
}

func (b *factoryTestBackend) Variant() gputypes.Backend { return b.variant }
func (b *factoryTestBackend) CreateInstance(_ *hal.InstanceDescriptor) (hal.Instance, error) {
	return &factoryTestInstance{}, nil
}

// factoryTestInstance implements hal.Instance for factory tests.
type factoryTestInstance struct{}

func (i *factoryTestInstance) CreateSurface(_, _ uintptr) (hal.Surface, error) { return nil, nil } //nolint:nilnil
func (i *factoryTestInstance) EnumerateAdapters(_ hal.Surface) []hal.ExposedAdapter {
	return nil
}
func (i *factoryTestInstance) Destroy() {}

// TestRegisterBackendFactory tests factory registration.
func TestRegisterBackendFactory(t *testing.T) {
	callCount := 0
	factory := func() (hal.Backend, error) {
		callCount++
		return &factoryTestBackend{variant: testFactoryVariant1}, nil
	}

	hal.RegisterBackendFactory(testFactoryVariant1, factory)

	// Factory should not be called until CreateBackend
	if callCount != 0 {
		t.Errorf("factory called during registration, want lazy")
	}
}

// TestCreateBackend tests lazy backend creation.
func TestCreateBackend(t *testing.T) {
	hal.RegisterBackendFactory(testFactoryVariant1, func() (hal.Backend, error) {
		return &factoryTestBackend{variant: testFactoryVariant1}, nil
	})

	backend, err := hal.CreateBackend(testFactoryVariant1)
	if err != nil {
		t.Fatalf("CreateBackend failed: %v", err)
	}
	if backend == nil {
		t.Fatal("CreateBackend returned nil backend")
	}
	if backend.Variant() != testFactoryVariant1 {
		t.Errorf("variant = %v, want %v", backend.Variant(), testFactoryVariant1)
	}
}

// TestCreateBackendNotRegistered tests CreateBackend with unregistered variant.
func TestCreateBackendNotRegistered(t *testing.T) {
	_, err := hal.CreateBackend(gputypes.Backend(99))
	if !errors.Is(err, hal.ErrBackendNotFound) {
		t.Errorf("expected ErrBackendNotFound, got %v", err)
	}
}

// TestCreateBackendFactoryError tests CreateBackend when factory returns error.
func TestCreateBackendFactoryError(t *testing.T) {
	factoryErr := errors.New("init failed")
	hal.RegisterBackendFactory(testFactoryVariant2, func() (hal.Backend, error) {
		return nil, factoryErr
	})

	_, err := hal.CreateBackend(testFactoryVariant2)
	if !errors.Is(err, factoryErr) {
		t.Errorf("expected factory error, got %v", err)
	}
}

// TestProbeBackendRegistered tests ProbeBackend with an already-registered backend.
func TestProbeBackendRegistered(t *testing.T) {
	// noop is registered via init()
	info, err := hal.ProbeBackend(gputypes.BackendEmpty)
	if err != nil {
		t.Fatalf("ProbeBackend for noop failed: %v", err)
	}
	if info == nil {
		t.Fatal("ProbeBackend returned nil info")
	}
	if info.Variant != gputypes.BackendEmpty {
		t.Errorf("variant = %v, want BackendEmpty", info.Variant)
	}
}

// TestProbeBackendViaFactory tests ProbeBackend with a factory.
func TestProbeBackendViaFactory(t *testing.T) {
	hal.RegisterBackendFactory(testFactoryVariant3, func() (hal.Backend, error) {
		return &factoryTestBackend{variant: testFactoryVariant3}, nil
	})

	info, err := hal.ProbeBackend(testFactoryVariant3)
	if err != nil {
		t.Fatalf("ProbeBackend via factory failed: %v", err)
	}
	if info == nil {
		t.Fatal("ProbeBackend returned nil info")
	}
	if info.Variant != testFactoryVariant3 {
		t.Errorf("variant = %v, want %v", info.Variant, testFactoryVariant3)
	}
}

// TestProbeBackendNotFound tests ProbeBackend with unknown backend.
func TestProbeBackendNotFound(t *testing.T) {
	_, err := hal.ProbeBackend(gputypes.Backend(77))
	if !errors.Is(err, hal.ErrBackendNotFound) {
		t.Errorf("expected ErrBackendNotFound, got %v", err)
	}
}

// TestSelectBestBackend tests backend selection priority.
func TestSelectBestBackend(t *testing.T) {
	// With noop registered, SelectBestBackend should return something
	backend, err := hal.SelectBestBackend()
	if err != nil {
		t.Fatalf("SelectBestBackend failed: %v", err)
	}
	if backend == nil {
		t.Fatal("SelectBestBackend returned nil")
	}
}

// TestBackendInfo tests BackendInfo struct fields.
func TestBackendInfo(t *testing.T) {
	info := hal.BackendInfo{
		Variant: gputypes.BackendVulkan,
		Name:    "Vulkan",
		Version: "1.3.0",
		Features: hal.BackendFeatures{
			SupportsCompute:    true,
			SupportsMultiQueue: true,
			MaxTextureSize:     16384,
			MaxBufferSize:      1 << 30,
		},
		Limitations: hal.BackendLimitations{
			NoAsyncCompute: false,
		},
	}

	if info.Variant != gputypes.BackendVulkan {
		t.Errorf("Variant = %v, want BackendVulkan", info.Variant)
	}
	if !info.Features.SupportsCompute {
		t.Error("SupportsCompute should be true")
	}
	if info.Features.MaxTextureSize != 16384 {
		t.Errorf("MaxTextureSize = %d, want 16384", info.Features.MaxTextureSize)
	}
}
