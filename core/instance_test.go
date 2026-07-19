//go:build !(js && wasm)

package core

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

type providerBackedTestInstance struct{}

func (*providerBackedTestInstance) CreateSurface(hal.SurfaceTarget) (hal.Surface, error) {
	return nil, errors.New("test instance does not create surfaces")
}

func (*providerBackedTestInstance) EnumerateAdapters(hal.Surface) []hal.ExposedAdapter {
	return []hal.ExposedAdapter{{
		Adapter: &providerBackedTestAdapter{},
		Info: gputypes.AdapterInfo{
			Name:       "provider-backed test adapter",
			DeviceType: gputypes.DeviceTypeCPU,
			Backend:    gputypes.BackendVulkan,
		},
		Capabilities: hal.Capabilities{Limits: gputypes.DefaultLimits()},
	}}
}

func (*providerBackedTestInstance) Destroy() {}

type providerBackedTestAdapter struct{}

func (*providerBackedTestAdapter) Open(gputypes.Features, gputypes.Limits) (hal.OpenDevice, error) {
	return hal.OpenDevice{}, nil
}

func (*providerBackedTestAdapter) TextureFormatCapabilities(gputypes.TextureFormat) hal.TextureFormatCapabilities {
	return hal.TextureFormatCapabilities{}
}

func (*providerBackedTestAdapter) SurfaceCapabilities(hal.Surface) *hal.SurfaceCapabilities {
	return nil
}

func (*providerBackedTestAdapter) Destroy() {}

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

			// Adapter discovery is provider-dependent. A missing provider must not
			// fabricate a mock adapter; deterministic mock coverage uses the
			// explicit NewInstanceWithMock constructor.
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

func TestNewInstanceDoesNotFabricateAdapterWithoutProvider(t *testing.T) {
	GetGlobal().Clear()

	instance := NewInstance(&gputypes.InstanceDescriptor{})
	if adapters := instance.EnumerateAdapters(); len(adapters) != 0 {
		t.Fatalf("NewInstance fabricated %d adapter(s) without an enabled provider", len(adapters))
	}
	if instance.IsMock() {
		t.Fatal("NewInstance unexpectedly enabled mock mode")
	}
	if _, err := instance.RequestAdapter(nil); err == nil {
		t.Fatal("RequestAdapter succeeded without a provider")
	}
}

func TestNewInstanceUsesRegisteredProviderWithoutEnablingMock(t *testing.T) {
	providersMu.Lock()
	savedProviders := providers
	providers = map[gputypes.Backend]BackendProvider{
		gputypes.BackendVulkan: &testProvider{
			variant:   gputypes.BackendVulkan,
			available: true,
			instance:  &providerBackedTestInstance{},
		},
	}
	providersMu.Unlock()
	t.Cleanup(func() {
		providersMu.Lock()
		providers = savedProviders
		providersMu.Unlock()
	})
	GetGlobal().Clear()

	instance := NewInstance(&gputypes.InstanceDescriptor{Backends: gputypes.BackendsVulkan})
	t.Cleanup(instance.Destroy)
	if instance.IsMock() {
		t.Fatal("provider-backed NewInstance unexpectedly enabled mock mode")
	}
	if adapters := instance.EnumerateAdapters(); len(adapters) != 1 {
		t.Fatalf("provider-backed NewInstance returned %d adapters, want 1", len(adapters))
	}
}

func TestNewInstanceWithMockIsExplicit(t *testing.T) {
	GetGlobal().Clear()

	instance := NewInstanceWithMock(nil)
	if !instance.IsMock() {
		t.Fatal("NewInstanceWithMock did not enable mock mode")
	}
	if adapters := instance.EnumerateAdapters(); len(adapters) != 1 {
		t.Fatalf("NewInstanceWithMock returned %d adapters, want 1", len(adapters))
	}
}

func TestEnumerateAdapters(t *testing.T) {
	GetGlobal().Clear()

	instance := NewInstanceWithMock(nil)

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
			wantErr: false, // WebGPU spec: powerPreference is a hint, falls back to discrete GPU
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

			instance := NewInstanceWithMock(nil)
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

	// An instance with no registered adapters must fail explicitly.
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

	instance := NewInstanceWithMock(nil)

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

func TestRequestAdapterPrefersGPUOverCPU(t *testing.T) {
	GetGlobal().Clear()

	instance := &Instance{
		backends: gputypes.BackendsAll,
	}

	hub := GetGlobal().Hub()

	// Register CPU adapter first (like Software backend)
	cpuAdapter := &Adapter{
		Info: gputypes.AdapterInfo{
			Name:       "Software Renderer",
			DeviceType: gputypes.DeviceTypeCPU,
			Backend:    gputypes.BackendVulkan,
		},
		Limits: gputypes.DefaultLimits(),
	}
	cpuID := hub.RegisterAdapter(cpuAdapter)
	instance.adapters = append(instance.adapters, cpuID)

	// Register GPU adapter second (like GLES deferred)
	gpuAdapter := &Adapter{
		Info: gputypes.AdapterInfo{
			Name:       "GLES GPU",
			DeviceType: gputypes.DeviceTypeIntegratedGPU,
			Backend:    gputypes.BackendGL,
		},
		Limits: gputypes.DefaultLimits(),
	}
	gpuID := hub.RegisterAdapter(gpuAdapter)
	instance.adapters = append(instance.adapters, gpuID)

	// nil options should prefer GPU over CPU
	adapterID, err := instance.RequestAdapter(nil)
	if err != nil {
		t.Fatalf("RequestAdapter(nil) error: %v", err)
	}
	adapter, _ := hub.GetAdapter(adapterID)
	if adapter.Info.DeviceType == gputypes.DeviceTypeCPU {
		t.Errorf("RequestAdapter(nil) returned CPU adapter, want GPU")
	}
	if adapter.Info.Name != "GLES GPU" {
		t.Errorf("RequestAdapter(nil) returned %q, want %q", adapter.Info.Name, "GLES GPU")
	}

	// options with no preference should also prefer GPU
	adapterID2, err := instance.RequestAdapter(&gputypes.RequestAdapterOptions{})
	if err != nil {
		t.Fatalf("RequestAdapter({}) error: %v", err)
	}
	adapter2, _ := hub.GetAdapter(adapterID2)
	if adapter2.Info.DeviceType == gputypes.DeviceTypeCPU {
		t.Errorf("RequestAdapter({}) returned CPU adapter, want GPU")
	}
}

func TestRequestAdapterForceFallbackReturnsCPU(t *testing.T) {
	GetGlobal().Clear()

	instance := &Instance{
		backends: gputypes.BackendsAll,
	}

	hub := GetGlobal().Hub()

	gpuAdapter := &Adapter{
		Info: gputypes.AdapterInfo{
			Name:       "Vulkan GPU",
			DeviceType: gputypes.DeviceTypeDiscreteGPU,
		},
		Limits: gputypes.DefaultLimits(),
	}
	gpuID := hub.RegisterAdapter(gpuAdapter)
	instance.adapters = append(instance.adapters, gpuID)

	cpuAdapter := &Adapter{
		Info: gputypes.AdapterInfo{
			Name:       "Software Renderer",
			DeviceType: gputypes.DeviceTypeCPU,
		},
		Limits: gputypes.DefaultLimits(),
	}
	cpuID := hub.RegisterAdapter(cpuAdapter)
	instance.adapters = append(instance.adapters, cpuID)

	adapterID, err := instance.RequestAdapter(&gputypes.RequestAdapterOptions{
		ForceFallbackAdapter: true,
	})
	if err != nil {
		t.Fatalf("ForceFallbackAdapter error: %v", err)
	}
	adapter, _ := hub.GetAdapter(adapterID)
	if adapter.Info.DeviceType != gputypes.DeviceTypeCPU {
		t.Errorf("ForceFallbackAdapter returned %v, want CPU", adapter.Info.DeviceType)
	}
}

func TestRequestAdapterOnlyCPUAvailable(t *testing.T) {
	GetGlobal().Clear()

	instance := &Instance{
		backends: gputypes.BackendsAll,
	}

	hub := GetGlobal().Hub()

	cpuAdapter := &Adapter{
		Info: gputypes.AdapterInfo{
			Name:       "Software Renderer",
			DeviceType: gputypes.DeviceTypeCPU,
		},
		Limits: gputypes.DefaultLimits(),
	}
	cpuID := hub.RegisterAdapter(cpuAdapter)
	instance.adapters = append(instance.adapters, cpuID)

	// Should fallback to CPU when no GPU available
	adapterID, err := instance.RequestAdapter(nil)
	if err != nil {
		t.Fatalf("RequestAdapter(nil) with only CPU: %v", err)
	}
	adapter, _ := hub.GetAdapter(adapterID)
	if adapter.Info.Name != "Software Renderer" {
		t.Errorf("got %q, want %q", adapter.Info.Name, "Software Renderer")
	}
}
