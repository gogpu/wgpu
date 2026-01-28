package core

import (
	"testing"

	"github.com/gogpu/gputypes"
)

func TestGetAdapterInfo(t *testing.T) {
	GetGlobal().Clear()

	instance := NewInstance(nil)
	adapters := instance.EnumerateAdapters()
	if len(adapters) == 0 {
		t.Fatal("no adapters available")
	}

	adapterID := adapters[0]
	info, err := GetAdapterInfo(adapterID)
	if err != nil {
		t.Fatalf("GetAdapterInfo() error: %v", err)
	}

	// Verify mock adapter info
	if info.Name == "" {
		t.Error("adapter name is empty")
	}
	if info.Vendor == "" {
		t.Error("adapter vendor is empty")
	}
	if info.Backend == gputypes.BackendEmpty {
		t.Error("adapter backend is empty")
	}
}

func TestGetAdapterInfoInvalid(t *testing.T) {
	GetGlobal().Clear()

	// Create an invalid adapter ID
	invalidID := AdapterID{}
	_, err := GetAdapterInfo(invalidID)
	if err == nil {
		t.Error("GetAdapterInfo() should fail for invalid ID")
	}
}

func TestGetAdapterFeatures(t *testing.T) {
	GetGlobal().Clear()

	instance := NewInstance(nil)
	adapters := instance.EnumerateAdapters()
	if len(adapters) == 0 {
		t.Fatal("no adapters available")
	}

	adapterID := adapters[0]
	features, err := GetAdapterFeatures(adapterID)
	if err != nil {
		t.Fatalf("GetAdapterFeatures() error: %v", err)
	}

	// Mock adapter has no special features, so this should be 0
	if features != 0 {
		t.Logf("adapter features: %v", features)
	}
}

func TestGetAdapterFeaturesInvalid(t *testing.T) {
	GetGlobal().Clear()

	invalidID := AdapterID{}
	_, err := GetAdapterFeatures(invalidID)
	if err == nil {
		t.Error("GetAdapterFeatures() should fail for invalid ID")
	}
}

func TestGetAdapterLimits(t *testing.T) {
	GetGlobal().Clear()

	instance := NewInstance(nil)
	adapters := instance.EnumerateAdapters()
	if len(adapters) == 0 {
		t.Fatal("no adapters available")
	}

	adapterID := adapters[0]
	limits, err := GetAdapterLimits(adapterID)
	if err != nil {
		t.Fatalf("GetAdapterLimits() error: %v", err)
	}

	// Verify some basic limits
	if limits.MaxTextureDimension2D == 0 {
		t.Error("MaxTextureDimension2D should not be zero")
	}
	if limits.MaxBindGroups == 0 {
		t.Error("MaxBindGroups should not be zero")
	}

	// Verify limits match defaults
	defaultLimits := gputypes.DefaultLimits()
	if limits.MaxTextureDimension2D != defaultLimits.MaxTextureDimension2D {
		t.Errorf("MaxTextureDimension2D = %d, want %d",
			limits.MaxTextureDimension2D, defaultLimits.MaxTextureDimension2D)
	}
}

func TestGetAdapterLimitsInvalid(t *testing.T) {
	GetGlobal().Clear()

	invalidID := AdapterID{}
	_, err := GetAdapterLimits(invalidID)
	if err == nil {
		t.Error("GetAdapterLimits() should fail for invalid ID")
	}
}

func TestRequestDevice(t *testing.T) {
	tests := []struct {
		name    string
		desc    *gputypes.DeviceDescriptor
		wantErr bool
	}{
		{
			name:    "nil descriptor uses defaults",
			desc:    nil,
			wantErr: false,
		},
		{
			name: "custom descriptor",
			desc: &gputypes.DeviceDescriptor{
				Label:            "Test Device",
				RequiredFeatures: []gputypes.Feature{},
				RequiredLimits:   gputypes.DefaultLimits(),
			},
			wantErr: false,
		},
		{
			name: "with required features",
			desc: &gputypes.DeviceDescriptor{
				Label: "Feature Test Device",
				// Mock adapter has no features, so this should fail
				RequiredFeatures: []gputypes.Feature{gputypes.FeatureDepthClipControl},
				RequiredLimits:   gputypes.DefaultLimits(),
			},
			wantErr: true, // Mock adapter doesn't support this feature
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			GetGlobal().Clear()

			instance := NewInstance(nil)
			adapters := instance.EnumerateAdapters()
			if len(adapters) == 0 {
				t.Fatal("no adapters available")
			}

			adapterID := adapters[0]
			deviceID, err := RequestDevice(adapterID, tt.desc)

			if tt.wantErr {
				if err == nil {
					t.Error("RequestDevice() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("RequestDevice() unexpected error: %v", err)
				return
			}

			if deviceID.IsZero() {
				t.Error("RequestDevice() returned zero ID")
			}

			// Verify the device exists
			hub := GetGlobal().Hub()
			_, err = hub.GetDevice(deviceID)
			if err != nil {
				t.Errorf("returned device ID is invalid: %v", err)
			}
		})
	}
}

func TestRequestDeviceInvalidAdapter(t *testing.T) {
	GetGlobal().Clear()

	invalidID := AdapterID{}
	_, err := RequestDevice(invalidID, nil)
	if err == nil {
		t.Error("RequestDevice() should fail for invalid adapter ID")
	}
}

func TestAdapterDrop(t *testing.T) {
	GetGlobal().Clear()

	instance := NewInstance(nil)
	adapters := instance.EnumerateAdapters()
	if len(adapters) == 0 {
		t.Fatal("no adapters available")
	}

	adapterID := adapters[0]

	// Verify adapter exists before drop
	hub := GetGlobal().Hub()
	_, err := hub.GetAdapter(adapterID)
	if err != nil {
		t.Fatalf("adapter should exist before drop: %v", err)
	}

	// Drop the adapter
	err = AdapterDrop(adapterID)
	if err != nil {
		t.Errorf("AdapterDrop() error: %v", err)
	}

	// Verify adapter is gone
	_, err = hub.GetAdapter(adapterID)
	if err == nil {
		t.Error("adapter should not exist after drop")
	}
}

func TestAdapterDropInvalid(t *testing.T) {
	GetGlobal().Clear()

	invalidID := AdapterID{}
	err := AdapterDrop(invalidID)
	if err == nil {
		t.Error("AdapterDrop() should fail for invalid ID")
	}
}

func TestAdapterLifecycle(t *testing.T) {
	GetGlobal().Clear()

	// 1. Create instance
	instance := NewInstance(nil)

	// 2. Request adapter
	adapterID, err := instance.RequestAdapter(nil)
	if err != nil {
		t.Fatalf("RequestAdapter() error: %v", err)
	}

	// 3. Get adapter info
	info, err := GetAdapterInfo(adapterID)
	if err != nil {
		t.Fatalf("GetAdapterInfo() error: %v", err)
	}
	t.Logf("Adapter: %s (%s)", info.Name, info.Backend)

	// 4. Get adapter features
	features, err := GetAdapterFeatures(adapterID)
	if err != nil {
		t.Fatalf("GetAdapterFeatures() error: %v", err)
	}
	t.Logf("Features: %v", features)

	// 5. Get adapter limits
	limits, err := GetAdapterLimits(adapterID)
	if err != nil {
		t.Fatalf("GetAdapterLimits() error: %v", err)
	}
	t.Logf("Max Texture 2D: %d", limits.MaxTextureDimension2D)

	// 6. Request device
	deviceID, err := RequestDevice(adapterID, nil)
	if err != nil {
		t.Fatalf("RequestDevice() error: %v", err)
	}
	if deviceID.IsZero() {
		t.Fatal("device ID is zero")
	}

	// 7. Drop adapter
	err = AdapterDrop(adapterID)
	if err != nil {
		t.Fatalf("AdapterDrop() error: %v", err)
	}

	// 8. Verify adapter is gone
	_, err = GetAdapterInfo(adapterID)
	if err == nil {
		t.Error("adapter should not exist after drop")
	}
}

func TestAdapterConcurrentAccess(t *testing.T) {
	GetGlobal().Clear()

	instance := NewInstance(nil)
	adapterID, err := instance.RequestAdapter(nil)
	if err != nil {
		t.Fatalf("RequestAdapter() error: %v", err)
	}

	// Test concurrent reads of adapter properties
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			_, _ = GetAdapterInfo(adapterID)
			_, _ = GetAdapterFeatures(adapterID)
			_, _ = GetAdapterLimits(adapterID)
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestRequestDeviceFeatureValidation(t *testing.T) {
	GetGlobal().Clear()

	instance := NewInstance(nil)
	adapterID, err := instance.RequestAdapter(nil)
	if err != nil {
		t.Fatalf("RequestAdapter() error: %v", err)
	}

	// Get adapter's supported features
	adapterFeatures, err := GetAdapterFeatures(adapterID)
	if err != nil {
		t.Fatalf("GetAdapterFeatures() error: %v", err)
	}

	// Request a feature the adapter doesn't support
	unsupportedFeature := gputypes.FeatureDepthClipControl
	if adapterFeatures.Contains(unsupportedFeature) {
		t.Skip("Mock adapter unexpectedly supports this feature")
	}

	desc := gputypes.DeviceDescriptor{
		RequiredFeatures: []gputypes.Feature{unsupportedFeature},
	}

	_, err = RequestDevice(adapterID, &desc)
	if err == nil {
		t.Error("RequestDevice() should fail when requesting unsupported features")
	}
}
