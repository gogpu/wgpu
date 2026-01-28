package core

import (
	"testing"

	"github.com/gogpu/gputypes"
)

func TestNewInstance(t *testing.T) {
	tests := []struct {
		name string
		desc *gputypes.InstanceDescriptor
		want gputypes.Backends
	}{
		{
			name: "nil descriptor uses defaults",
			desc: nil,
			want: gputypes.BackendsPrimary,
		},
		{
			name: "custom backends",
			desc: &gputypes.InstanceDescriptor{
				Backends: gputypes.BackendsVulkan | gputypes.BackendsMetal,
				Flags:    gputypes.InstanceFlagsDebug,
			},
			want: gputypes.BackendsVulkan | gputypes.BackendsMetal,
		},
		{
			name: "all backends",
			desc: &gputypes.InstanceDescriptor{
				Backends: gputypes.BackendsAll,
				Flags:    0,
			},
			want: gputypes.BackendsAll,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear global state before each test
			GetGlobal().Clear()

			instance := NewInstance(tt.desc)
			if instance == nil {
				t.Fatal("NewInstance returned nil")
			}

			got := instance.Backends()
			if got != tt.want {
				t.Errorf("Backends() = %v, want %v", got, tt.want)
			}

			// Verify mock adapter was created
			adapters := instance.EnumerateAdapters()
			if len(adapters) == 0 {
				t.Error("Expected at least one mock adapter")
			}
		})
	}
}

func TestInstanceFlags(t *testing.T) {
	tests := []struct {
		name  string
		flags gputypes.InstanceFlags
	}{
		{
			name:  "no flags",
			flags: 0,
		},
		{
			name:  "debug flag",
			flags: gputypes.InstanceFlagsDebug,
		},
		{
			name:  "validation flag",
			flags: gputypes.InstanceFlagsValidation,
		},
		{
			name:  "multiple flags",
			flags: gputypes.InstanceFlagsDebug | gputypes.InstanceFlagsValidation,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			GetGlobal().Clear()

			desc := gputypes.InstanceDescriptor{
				Backends: gputypes.BackendsPrimary,
				Flags:    tt.flags,
			}
			instance := NewInstance(&desc)

			got := instance.Flags()
			if got != tt.flags {
				t.Errorf("Flags() = %v, want %v", got, tt.flags)
			}
		})
	}
}

func TestEnumerateAdapters(t *testing.T) {
	GetGlobal().Clear()

	instance := NewInstance(nil)

	adapters := instance.EnumerateAdapters()
	if len(adapters) == 0 {
		t.Fatal("EnumerateAdapters returned empty list")
	}

	// Verify we get a copy, not a reference to internal state
	adapters1 := instance.EnumerateAdapters()
	adapters2 := instance.EnumerateAdapters()

	if &adapters1[0] == &adapters2[0] {
		t.Error("EnumerateAdapters should return a copy, not a reference")
	}

	// Verify adapter IDs are valid
	for i, adapterID := range adapters {
		if adapterID.IsZero() {
			t.Errorf("adapter %d has zero ID", i)
		}
	}
}

func TestRequestAdapter(t *testing.T) {
	tests := []struct {
		name    string
		options *gputypes.RequestAdapterOptions
		wantErr bool
	}{
		{
			name:    "nil options returns first adapter",
			options: nil,
			wantErr: false,
		},
		{
			name: "no power preference",
			options: &gputypes.RequestAdapterOptions{
				PowerPreference: gputypes.PowerPreferenceNone,
			},
			wantErr: false,
		},
		{
			name: "high performance preference",
			options: &gputypes.RequestAdapterOptions{
				PowerPreference: gputypes.PowerPreferenceHighPerformance,
			},
			wantErr: false, // Mock adapter is discrete GPU
		},
		{
			name: "low power preference",
			options: &gputypes.RequestAdapterOptions{
				PowerPreference: gputypes.PowerPreferenceLowPower,
			},
			wantErr: true, // Mock adapter is discrete GPU, not integrated
		},
		{
			name: "force fallback adapter",
			options: &gputypes.RequestAdapterOptions{
				ForceFallbackAdapter: true,
			},
			wantErr: true, // Mock adapter is not CPU
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			GetGlobal().Clear()

			instance := NewInstance(nil)
			adapterID, err := instance.RequestAdapter(tt.options)

			if tt.wantErr {
				if err == nil {
					t.Error("RequestAdapter() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("RequestAdapter() unexpected error: %v", err)
				return
			}

			if adapterID.IsZero() {
				t.Error("RequestAdapter() returned zero ID")
			}

			// Verify the adapter exists
			hub := GetGlobal().Hub()
			_, err = hub.GetAdapter(adapterID)
			if err != nil {
				t.Errorf("returned adapter ID is invalid: %v", err)
			}
		})
	}
}

func TestRequestAdapterNoAdapters(t *testing.T) {
	GetGlobal().Clear()

	// Create instance but remove mock adapter
	instance := &Instance{
		backends: gputypes.BackendsPrimary,
		flags:    0,
		adapters: []AdapterID{}, // Empty adapter list
	}

	_, err := instance.RequestAdapter(nil)
	if err == nil {
		t.Error("RequestAdapter() should fail when no adapters available")
	}
}

func TestMatchesPowerPreference(t *testing.T) {
	tests := []struct {
		name       string
		deviceType gputypes.DeviceType
		preference gputypes.PowerPreference
		want       bool
	}{
		{
			name:       "integrated GPU with low power preference",
			deviceType: gputypes.DeviceTypeIntegratedGPU,
			preference: gputypes.PowerPreferenceLowPower,
			want:       true,
		},
		{
			name:       "discrete GPU with low power preference",
			deviceType: gputypes.DeviceTypeDiscreteGPU,
			preference: gputypes.PowerPreferenceLowPower,
			want:       false,
		},
		{
			name:       "discrete GPU with high performance preference",
			deviceType: gputypes.DeviceTypeDiscreteGPU,
			preference: gputypes.PowerPreferenceHighPerformance,
			want:       true,
		},
		{
			name:       "integrated GPU with high performance preference",
			deviceType: gputypes.DeviceTypeIntegratedGPU,
			preference: gputypes.PowerPreferenceHighPerformance,
			want:       false,
		},
		{
			name:       "any GPU with no preference",
			deviceType: gputypes.DeviceTypeDiscreteGPU,
			preference: gputypes.PowerPreferenceNone,
			want:       true,
		},
		{
			name:       "CPU with low power preference",
			deviceType: gputypes.DeviceTypeCPU,
			preference: gputypes.PowerPreferenceLowPower,
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesPowerPreference(tt.deviceType, tt.preference)
			if got != tt.want {
				t.Errorf("matchesPowerPreference(%v, %v) = %v, want %v",
					tt.deviceType, tt.preference, got, tt.want)
			}
		})
	}
}

func TestInstanceConcurrentAccess(t *testing.T) {
	GetGlobal().Clear()

	instance := NewInstance(nil)

	// Test concurrent reads
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			_ = instance.EnumerateAdapters()
			_ = instance.Backends()
			_ = instance.Flags()
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}
