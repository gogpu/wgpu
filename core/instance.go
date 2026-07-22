//go:build !(js && wasm)

package core

import (
	"fmt"
	"sync"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// Instance represents a WebGPU instance for GPU discovery and initialization.
// The instance is responsible for enumerating available GPU adapters and
// creating adapters based on application requirements.
//
// An instance maintains the list of available backends and their configuration.
// It is the entry point for all WebGPU operations.
//
// Thread-safe for concurrent use.
type Instance struct {
	mu       sync.RWMutex
	backends gputypes.Backends
	flags    gputypes.InstanceFlags

	// adapters contains the registered adapter IDs.
	adapters []AdapterID

	// halInstances tracks HAL instances created for each backend.
	// These are destroyed when the Instance is destroyed.
	halInstances []hal.Instance

	// halInstanceMap maps backend type to its HAL instance for backend-specific
	// surface creation. When a device uses a different backend than the initially
	// selected one (e.g., software adapter with ForceFallbackAdapter), the surface
	// must be re-created on the correct backend's HAL instance.
	halInstanceMap map[gputypes.Backend]hal.Instance

	// deferredGLES holds GLES HAL instances whose adapter enumeration is deferred
	// until a surface is available. OpenGL requires a GL context (obtained from a
	// surface) to query adapter capabilities. These are enumerated lazily on the
	// first RequestAdapterWithSurface call that provides a non-nil surface hint.
	deferredGLES []hal.Instance

	// glesEnumerated tracks whether deferred GLES adapters have been enumerated.
	glesEnumerated bool

	// useMock indicates whether this instance was explicitly created with mock
	// adapters through NewInstanceWithMock.
	useMock bool
}

// NewInstance creates a new WebGPU instance with the given descriptor.
// If desc is nil, default settings are used.
//
// The instance will enumerate available GPU adapters based on the enabled
// backends specified in the descriptor. If no provider is available, the
// instance remains empty and RequestAdapter reports the failure. Tests that
// need a deterministic adapter must opt in through NewInstanceWithMock.
func NewInstance(desc *gputypes.InstanceDescriptor) *Instance {
	if desc == nil {
		defaultDesc := gputypes.DefaultInstanceDescriptor()
		desc = &defaultDesc
	}

	i := &Instance{
		backends:       desc.Backends,
		flags:          desc.Flags,
		adapters:       []AdapterID{},
		halInstances:   []hal.Instance{},
		halInstanceMap: make(map[gputypes.Backend]hal.Instance),
		useMock:        false,
	}

	// Try to enumerate real adapters via HAL backends
	i.enumerateRealAdapters(desc)

	trackResource(uintptr(unsafe.Pointer(i)), "Instance") //nolint:gosec // debug tracking uses pointer as unique ID
	return i
}

// NewInstanceWithMock creates a new WebGPU instance with mock adapters.
// This is primarily for testing without requiring real GPU hardware.
func NewInstanceWithMock(desc *gputypes.InstanceDescriptor) *Instance {
	if desc == nil {
		defaultDesc := gputypes.DefaultInstanceDescriptor()
		desc = &defaultDesc
	}

	i := &Instance{
		backends:       desc.Backends,
		flags:          desc.Flags,
		adapters:       []AdapterID{},
		halInstances:   []hal.Instance{},
		halInstanceMap: make(map[gputypes.Backend]hal.Instance),
		useMock:        true,
	}

	i.createMockAdapter()
	trackResource(uintptr(unsafe.Pointer(i)), "Instance") //nolint:gosec // debug tracking uses pointer as unique ID
	return i
}

// enumerateRealAdapters attempts to enumerate real GPU adapters via HAL
// backends. If none are available, the instance remains empty.
func (i *Instance) enumerateRealAdapters(desc *gputypes.InstanceDescriptor) {
	// First, ensure HAL backends are registered
	RegisterHALBackends()

	// Get backend providers filtered by the enabled backends mask
	providers := FilterBackendsByMask(desc.Backends)

	hub := GetGlobal().Hub()

	// Create HAL descriptor
	halDesc := &hal.InstanceDescriptor{
		Backends: desc.Backends,
		Flags:    desc.Flags,
	}

	// Try each backend provider
	for _, provider := range providers {
		// Skip noop backend — it's for testing only, not real rendering.
		// Software backend (also BackendEmpty variant) is allowed through
		// because it provides real CPU-based rendering.
		if provider.Variant() == gputypes.BackendEmpty {
			halInst, err := provider.CreateInstance(halDesc)
			if err != nil {
				continue
			}
			adapters := halInst.EnumerateAdapters(nil)
			isNoop := len(adapters) > 0 && adapters[0].Info.DeviceType == gputypes.DeviceTypeOther
			if isNoop {
				halInst.Destroy()
				continue
			}
			// Not noop (software backend) — destroy temp instance and fall through
			halInst.Destroy()
		}

		// Try to create HAL instance
		halInstance, err := provider.CreateInstance(halDesc)
		if err != nil {
			// Backend not available, try next
			continue
		}

		// GLES/GL backends use lazy GL context initialization (hidden window,
		// v0.28.6+). Defer enumeration until RequestAdapter, where the first
		// Lock() call creates the GL context on the render thread via sync.Once.
		if provider.Variant() == gputypes.BackendGL {
			i.halInstances = append(i.halInstances, halInstance)
			i.halInstanceMap[provider.Variant()] = halInstance
			i.deferredGLES = append(i.deferredGLES, halInstance)
			continue
		}

		// Track HAL instance for cleanup
		i.halInstances = append(i.halInstances, halInstance)
		i.halInstanceMap[provider.Variant()] = halInstance

		// Enumerate adapters from this backend
		exposedAdapters := halInstance.EnumerateAdapters(nil)
		for idx := range exposedAdapters {
			exposed := &exposedAdapters[idx] // Use pointer to avoid copy
			// Create core.Adapter wrapping the HAL adapter
			adapter := &Adapter{
				Info:            exposed.Info,
				Features:        exposed.Features,
				Limits:          exposed.Capabilities.Limits,
				Backend:         exposed.Info.Backend,
				halAdapter:      exposed.Adapter,
				halCapabilities: &exposed.Capabilities,
			}

			// Register in the hub
			adapterID := hub.RegisterAdapter(adapter)
			i.adapters = append(i.adapters, adapterID)
		}
	}
}

// createMockAdapter creates a mock adapter for testing purposes.
// Mock adapters provide a functional Core API without requiring real GPU hardware.
func (i *Instance) createMockAdapter() {
	// Create a mock adapter with reasonable default values
	adapter := &Adapter{
		Info: gputypes.AdapterInfo{
			Name:       "Mock Adapter",
			Vendor:     "MockVendor",
			VendorID:   0x1234,
			DeviceID:   0x5678,
			DeviceType: gputypes.DeviceTypeDiscreteGPU,
			Driver:     "1.0.0",
			DriverInfo: "Mock Driver (no real GPU)",
			Backend:    gputypes.BackendVulkan,
		},
		Features: gputypes.Features(0), // No special features for mock
		Limits:   gputypes.DefaultLimits(),
		Backend:  gputypes.BackendVulkan,
		// HAL fields are nil for mock adapters
		halAdapter:      nil,
		halCapabilities: nil,
	}

	// Register the adapter in the global hub
	hub := GetGlobal().Hub()
	adapterID := hub.RegisterAdapter(adapter)
	i.adapters = append(i.adapters, adapterID)
}

// EnumerateAdapters returns a list of all available GPU adapters.
// The adapters are filtered based on the backends enabled in the instance.
//
// This method returns a snapshot of available adapters at the time of the call.
// The adapter list may change if GPUs are added or removed from the system.
func (i *Instance) EnumerateAdapters() []AdapterID {
	i.mu.RLock()
	defer i.mu.RUnlock()

	// Return a copy to prevent external modification
	result := make([]AdapterID, len(i.adapters))
	copy(result, i.adapters)
	return result
}

// RequestAdapter requests an adapter matching the given options.
// Returns the first adapter that meets the requirements, or an error if none found.
//
// Options control adapter selection:
//   - PowerPreference: prefer low-power or high-performance adapters
//   - ForceFallbackAdapter: use software rendering
//   - CompatibleSurface: adapter must support the given surface
//
// If options is nil, the first available adapter is returned.
func (i *Instance) RequestAdapter(options *gputypes.RequestAdapterOptions) (AdapterID, error) { //nolint:gocognit // adapter selection with GPU preference logic
	// Trigger deferred GLES adapter enumeration if not yet done.
	// When called directly (without a prior RequestAdapterWithSurface call) the
	// nil surfaceHint causes EnumerateAdapters to return a zero-value Adapter
	// (nil glCtx) — Open() guards this with a descriptive error.
	// Callers that want a real GLES adapter must use RequestAdapterWithSurface.
	// Must be called BEFORE RLock to avoid deadlock (enumerateDeferredGLES
	// acquires a write lock internally).
	i.enumerateDeferredGLES(nil)

	i.mu.RLock()
	defer i.mu.RUnlock()

	if len(i.adapters) == 0 {
		return AdapterID{}, fmt.Errorf("no adapters available")
	}

	// If no options specified, prefer non-CPU adapters (GPU > Software fallback).
	if options == nil {
		hub := GetGlobal().Hub()
		for _, adapterID := range i.adapters {
			adapter, err := hub.GetAdapter(adapterID)
			if err != nil {
				continue
			}
			if adapter.Info.DeviceType != gputypes.DeviceTypeCPU {
				return adapterID, nil
			}
		}
		return i.adapters[0], nil // fallback to first (Software)
	}

	hub := GetGlobal().Hub()

	// ForceFallbackAdapter: return first CPU adapter directly
	if options.ForceFallbackAdapter {
		for _, adapterID := range i.adapters {
			adapter, err := hub.GetAdapter(adapterID)
			if err != nil {
				continue
			}
			if adapter.Info.DeviceType == gputypes.DeviceTypeCPU {
				return adapterID, nil
			}
		}
		return AdapterID{}, fmt.Errorf("no software/fallback adapter available")
	}

	// Two-pass adapter selection matching WebGPU spec:
	// powerPreference is a HINT, not a filter. "must not cause requestAdapter()
	// to fail if there is at least one available adapter." (W3C WebGPU spec)
	// Matches Rust wgpu which sorts by preference, never filters.
	var cpuFallback AdapterID
	hasCPUFallback := false
	var gpuFallback AdapterID
	hasGPUFallback := false

	// Pass 1: find preferred adapter + track fallbacks.
	for _, adapterID := range i.adapters {
		adapter, err := hub.GetAdapter(adapterID)
		if err != nil {
			continue
		}

		if adapter.Info.DeviceType == gputypes.DeviceTypeCPU {
			if !hasCPUFallback {
				cpuFallback = adapterID
				hasCPUFallback = true
			}
			continue
		}

		// Track first non-CPU adapter as GPU fallback
		if !hasGPUFallback {
			gpuFallback = adapterID
			hasGPUFallback = true
		}

		// Check power preference — return immediately on exact match
		if options.PowerPreference != gputypes.PowerPreferenceNone {
			if matchesPowerPreference(adapter.Info.DeviceType, options.PowerPreference) {
				return adapterID, nil
			}
		} else {
			return adapterID, nil
		}
	}

	// Pass 2: no exact preference match — return any GPU (fallback).
	// E.g., HighPerformance requested but only IntegratedGPU available.
	if hasGPUFallback {
		return gpuFallback, nil
	}

	// Pass 3: CPU fallback (software rasterizer).
	if hasCPUFallback {
		return cpuFallback, nil
	}

	return AdapterID{}, fmt.Errorf("no adapter matches the requested options")
}

// RequestAdapterWithSurface requests an adapter matching the given options,
// using the provided HAL surface as a hint for backends that require it.
//
// For GLES, the surface hint is mandatory: EGL needs a window handle to create
// a GL context, which is required before EnumerateAdapters can return a real
// adapter. Calling without a surface yields a zero-value Adapter (nil glCtx).
func (i *Instance) RequestAdapterWithSurface(options *gputypes.RequestAdapterOptions, surfaceHint hal.Surface) (AdapterID, error) {
	// Run deferred GLES enumeration with the real surface hint BEFORE RequestAdapter
	// calls enumerateDeferredGLES(nil). Once glesEnumerated is true the nil call
	// inside RequestAdapter becomes a no-op, so the surface-backed adapters win.
	i.enumerateDeferredGLES(surfaceHint)
	return i.RequestAdapter(options)
}

// enumerateDeferredGLES enumerates adapters for deferred GLES HAL instances.
// Called at most once per instance (guarded by glesEnumerated flag).
//
// surfaceHint should be a real surface for GLES: EGL needs a display/window
// handle to build an EGL context. A nil hint produces a zero-value Adapter
// (nil glCtx); Open() on such an adapter returns a descriptive error.
//
// Must NOT be called with i.mu held (it acquires mu internally).
func (i *Instance) enumerateDeferredGLES(surfaceHint hal.Surface) {
	i.mu.Lock()
	defer i.mu.Unlock()

	if i.glesEnumerated || len(i.deferredGLES) == 0 {
		return
	}
	i.glesEnumerated = true

	hub := GetGlobal().Hub()

	for _, halInstance := range i.deferredGLES {
		exposedAdapters := halInstance.EnumerateAdapters(surfaceHint)
		for idx := range exposedAdapters {
			exposed := &exposedAdapters[idx]
			adapter := &Adapter{
				Info:            exposed.Info,
				Features:        exposed.Features,
				Limits:          exposed.Capabilities.Limits,
				Backend:         exposed.Info.Backend,
				halAdapter:      exposed.Adapter,
				halCapabilities: &exposed.Capabilities,
			}

			adapterID := hub.RegisterAdapter(adapter)
			i.adapters = append(i.adapters, adapterID)
		}
	}

	// Clear deferred list -- enumeration is done.
	i.deferredGLES = nil
}

// matchesPowerPreference checks if a device type matches the power preference.
func matchesPowerPreference(deviceType gputypes.DeviceType, preference gputypes.PowerPreference) bool {
	switch preference {
	case gputypes.PowerPreferenceLowPower:
		// Prefer integrated GPUs for low power
		return deviceType == gputypes.DeviceTypeIntegratedGPU
	case gputypes.PowerPreferenceHighPerformance:
		// Prefer discrete GPUs for high performance
		return deviceType == gputypes.DeviceTypeDiscreteGPU
	default:
		return true
	}
}

// Backends returns the enabled backends for this instance.
func (i *Instance) Backends() gputypes.Backends {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.backends
}

// Flags returns the instance flags.
func (i *Instance) Flags() gputypes.InstanceFlags {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.flags
}

// IsMock returns true if the instance is using mock adapters.
// Mock adapters are used only when the instance was explicitly created with
// NewInstanceWithMock.
func (i *Instance) IsMock() bool {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.useMock
}

// HasHALAdapters returns true if any real HAL adapters are available.
func (i *Instance) HasHALAdapters() bool {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return len(i.halInstances) > 0 && !i.useMock
}

// HALInstance returns the first available HAL instance, or nil if none.
// Used by the public API for surface creation without creating duplicate HAL instances.
func (i *Instance) HALInstance() hal.Instance {
	i.mu.RLock()
	defer i.mu.RUnlock()
	if len(i.halInstances) > 0 {
		return i.halInstances[0]
	}
	return nil
}

// HALInstanceForBackend returns the HAL instance for a specific backend type.
// Returns nil if no instance exists for the given backend.
// Used to re-create surfaces when the device's backend differs from the
// initially selected one (e.g., software adapter via ForceFallbackAdapter).
func (i *Instance) HALInstanceForBackend(backend gputypes.Backend) hal.Instance {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.halInstanceMap[backend]
}

// HALInstanceMap returns the backend-to-instance mapping.
// Used to determine which backend a given HAL instance belongs to.
func (i *Instance) HALInstanceMap() map[gputypes.Backend]hal.Instance {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.halInstanceMap
}

// Destroy releases all resources associated with this instance.
// This includes unregistering all adapters and destroying HAL instances.
// After calling Destroy, the instance should not be used.
func (i *Instance) Destroy() {
	i.mu.Lock()
	defer i.mu.Unlock()

	untrackResource(uintptr(unsafe.Pointer(i))) //nolint:gosec // debug tracking uses pointer as unique ID

	hub := GetGlobal().Hub()

	// Unregister all adapters from the hub
	for _, adapterID := range i.adapters {
		adapter, err := hub.GetAdapter(adapterID)
		if err != nil {
			continue
		}

		// Destroy the HAL adapter if present
		if adapter.halAdapter != nil {
			adapter.halAdapter.Destroy()
		}

		// Unregister from hub
		_, _ = hub.UnregisterAdapter(adapterID)
	}
	i.adapters = nil

	// Destroy all HAL instances (includes deferred GLES instances).
	for _, halInstance := range i.halInstances {
		halInstance.Destroy()
	}
	i.halInstances = nil
	i.deferredGLES = nil
}
