//go:build !(js && wasm)

package core

import (
	"fmt"
	"sync"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/hal"
)

// instanceEnumerateMu serializes HAL backend probing during NewInstance.
// Concurrent CreateInstance calls on Windows CI (kolkov/racedetector) can
// otherwise race inside driver init while enumerating adapters.
var instanceEnumerateMu sync.Mutex

// Instance represents a WebGPU instance for GPU discovery and initialization.
// The instance is responsible for enumerating available GPU adapters and
// creating adapters based on application requirements.
//
// An instance maintains the list of available backends and their configuration.
// It is the entry point for all WebGPU operations.
//
// Thread-safe for concurrent use.
type Instance struct {
	mu sync.RWMutex
	// surfaceRequestMu serializes surface-aware adapter requests. Deferred GLES
	// enumeration is intentionally one-shot, so the snapshot that distinguishes
	// adapters created for the current surface must be taken atomically with that
	// enumeration.
	surfaceRequestMu sync.Mutex
	backends         gputypes.Backends
	flags            gputypes.InstanceFlags

	// adapters contains the registered adapter IDs.
	adapters []AdapterID

	// surfaceAdapters contains request-local adapters created for a compatible
	// surface. They are intentionally kept out of adapters so ordinary adapter
	// enumeration and selection never retain surface-bound queue state.
	surfaceAdapters []AdapterID

	// halInstances tracks HAL instances created for each backend.
	// These are destroyed when the Instance is destroyed.
	halInstances []hal.Instance
	// halInstanceEntries preserves backend priority alongside each HAL instance.
	// Surface creation uses this ordered view to mirror Rust wgpu's attempt to
	// create a raw surface for every enabled backend.
	halInstanceEntries []HALInstanceEntry

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

	// maskSelectedBackend records whether the descriptor's backend mask admitted
	// at least one registered backend provider during enumeration. It separates
	// ADR-083 §2.2's two no-adapter sentinels: a mask that admitted nothing means
	// the requested backend is compiled out or excluded (ErrNoBackendRegistered),
	// while an admitted backend that produced no adapter means this machine has
	// none (ErrNoHardwareAdapter).
	maskSelectedBackend bool

	// instanceDesc is the descriptor the instance was built from, kept for the
	// lazy software fallback enumeration below.
	instanceDesc hal.InstanceDescriptor

	// fallbackMu guards fallbackReady and fallbackAdapters. The software
	// backend's adapters are created lazily on the first ForceFallbackAdapter
	// request under the default mask (ADR-083 §2.3, implemented 2026-11-16):
	// enumeration (adapters) and Auto selection never see them, and only a
	// force request admits them as candidates (ADR-079).
	fallbackMu       sync.Mutex
	fallbackReady    bool
	fallbackAdapters []AdapterID
}

// HALInstanceEntry associates an enabled backend with its HAL instance.
// Entries are ordered by the same backend priority used during discovery.
type HALInstanceEntry struct {
	Backend  gputypes.Backend
	Instance hal.Instance
}

// surfaceAdapterQualifier is an optional backend capability. It intentionally
// stays private to core so the stable HAL Adapter interface does not grow a
// surface-specific method. Vulkan implements the method on its concrete
// adapter and returns a request-local wrapper with queue/surface state.
type surfaceAdapterQualifier interface {
	QualifySurface(surface hal.Surface) (hal.Adapter, error)
}

// NewInstance creates a new WebGPU instance with the given descriptor.
// If desc is nil, default settings are used.
//
// The instance will enumerate available GPU adapters based on the enabled
// backends specified in the descriptor. If no provider is available, the
// instance remains empty and RequestAdapter reports the failure. Tests that
// need a deterministic adapter must opt in through NewInstanceWithMock.
func NewInstance(desc *gputypes.InstanceDescriptor) *Instance {
	return NewInstanceWithDisplay(desc, hal.DisplayHandle{})
}

// NewInstanceWithDisplay is NewInstance with the native display connection
// the caller passed at instance creation (wgpu.InstanceDescriptor.Display),
// handed to every backend's CreateInstance. A zero display is NewInstance.
func NewInstanceWithDisplay(desc *gputypes.InstanceDescriptor, display hal.DisplayHandle) *Instance {
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

	// Try to enumerate real adapters via HAL backends. The descriptor is kept
	// for the lazy software fallback enumeration (ensureFallbackAdapters).
	i.instanceDesc = *halInstanceDescriptor(desc, display)
	i.enumerateRealAdapters(&i.instanceDesc)

	trackResource(uintptr(unsafe.Pointer(i)), "Instance") //nolint:gosec // debug tracking uses pointer as unique ID
	return i
}

// halInstanceDescriptor builds the descriptor every backend's CreateInstance
// receives.
func halInstanceDescriptor(desc *gputypes.InstanceDescriptor, display hal.DisplayHandle) *hal.InstanceDescriptor {
	return &hal.InstanceDescriptor{
		Backends: desc.Backends,
		Flags:    desc.Flags,
		Display:  display,
	}
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
func (i *Instance) enumerateRealAdapters(halDesc *hal.InstanceDescriptor) {
	instanceEnumerateMu.Lock()
	defer instanceEnumerateMu.Unlock()

	// First, ensure HAL backends are registered
	RegisterHALBackends()

	// Get backend providers filtered by the enabled backends mask
	providers := FilterBackendsByMask(halDesc.Backends)

	// Remember whether the mask admitted any registered backend: RequestAdapter
	// needs it to tell "compiled out or excluded" from "no adapter on this
	// machine" (ADR-083 §2.2).
	i.mu.Lock()
	i.maskSelectedBackend = len(providers) > 0
	i.mu.Unlock()

	// Try each backend provider
	for _, provider := range providers {
		// Noop is the only BackendEmpty provider — a test double, never
		// enumerable — and the software backend reports BackendSoftware since
		// ADR-083, so this can only mean noop: skip it. The former
		// temp-instance/noop probe can no longer fire, and skipping costs
		// nothing.
		if provider.Variant() == gputypes.BackendEmpty {
			continue
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
			i.halInstanceEntries = append(i.halInstanceEntries, HALInstanceEntry{
				Backend:  provider.Variant(),
				Instance: halInstance,
			})
			i.halInstanceMap[provider.Variant()] = halInstance
			i.deferredGLES = append(i.deferredGLES, halInstance)
			continue
		}

		// Track HAL instance for cleanup
		i.halInstances = append(i.halInstances, halInstance)
		i.halInstanceEntries = append(i.halInstanceEntries, HALInstanceEntry{
			Backend:  provider.Variant(),
			Instance: halInstance,
		})
		i.halInstanceMap[provider.Variant()] = halInstance

		// Enumerate adapters from this backend
		i.adapters = append(i.adapters, i.registerExposedAdapters(halInstance)...)
	}
}

// registerExposedAdapters wraps a HAL instance's adapters in core adapters and
// registers them in the hub, returning their IDs. It leaves the instance's
// adapter lists alone: the caller decides whether they join the normal
// enumeration (adapters) or the software fallback list (fallbackAdapters,
// ADR-083 §2.3).
func (i *Instance) registerExposedAdapters(halInstance hal.Instance) []AdapterID {
	exposedAdapters := halInstance.EnumerateAdapters(nil)
	ids := make([]AdapterID, 0, len(exposedAdapters))
	hub := GetGlobal().Hub()
	for idx := range exposedAdapters {
		exposed := &exposedAdapters[idx] // Use pointer to avoid copy
		// Create core.Adapter wrapping the HAL adapter
		adapter := &Adapter{
			Info:                  exposed.Info,
			Features:              exposed.Features,
			Limits:                exposed.Capabilities.Limits,
			DownlevelCapabilities: exposed.Capabilities.DownlevelCapabilities,
			StencilCopyNeedsBlit:  exposed.Capabilities.StencilCopyNeedsBlit,
			Backend:               exposed.Info.Backend,
			halAdapter:            exposed.Adapter,
			halCapabilities:       &exposed.Capabilities,
		}
		ids = append(ids, hub.RegisterAdapter(adapter))
	}
	return ids
}

// admitsSoftwareFallback reports whether a ForceFallbackAdapter request may
// reach the software backend on this instance. ADR-083 §2.3: software leaves
// the default mask, normal enumeration and Auto; it stays reachable through
// the explicit bit, which enumerates it normally, or through
// ForceFallbackAdapter under the default set. A reduced explicit mask keeps its
// meaning (ADR-079: Backends=DX12 does not silently gain software).
func (i *Instance) admitsSoftwareFallback() bool {
	if i.useMock {
		// A mock instance is a test double over fabricated adapters; a force
		// request must not reach the real software backend through it.
		return false
	}
	if i.backends&gputypes.BackendsSoftware != 0 {
		return false // enumerated normally; Auto and force requests both see it
	}
	return i.backends == 0 || i.backends == gputypes.DefaultInstanceDescriptor().Backends
}

// ensureFallbackAdapters creates the software backend's adapters on the first
// ForceFallbackAdapter request. The boundary is "enumeration unchanged,
// candidacy extended": i.adapters stays hardware-only, and only the request
// that asked for a fallback sees these candidates.
func (i *Instance) ensureFallbackAdapters() []AdapterID {
	i.fallbackMu.Lock()
	defer i.fallbackMu.Unlock()
	if i.fallbackReady {
		return append([]AdapterID(nil), i.fallbackAdapters...)
	}
	i.fallbackReady = true
	if !i.admitsSoftwareFallback() {
		return nil
	}
	for _, provider := range GetOrderedBackendProviders() {
		if provider.Variant() != gputypes.BackendSoftware || !provider.IsAvailable() {
			continue
		}
		swInstance, err := provider.CreateInstance(&i.instanceDesc)
		if err != nil {
			return nil
		}
		ids := i.registerExposedAdapters(swInstance)
		i.mu.Lock()
		i.halInstances = append(i.halInstances, swInstance)
		i.halInstanceEntries = append(i.halInstanceEntries, HALInstanceEntry{
			Backend:  gputypes.BackendSoftware,
			Instance: swInstance,
		})
		i.halInstanceMap[gputypes.BackendSoftware] = swInstance
		i.fallbackAdapters = append(i.fallbackAdapters, ids...)
		i.mu.Unlock()
		return append([]AdapterID(nil), i.fallbackAdapters...)
	}
	return nil
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
func (i *Instance) RequestAdapter(options *gputypes.RequestAdapterOptions) (AdapterID, error) {
	// Trigger deferred GLES adapter enumeration if not yet done.
	// When called directly (without a prior RequestAdapterWithSurface call) the
	// nil surfaceHint causes EnumerateAdapters to return a zero-value Adapter
	// (nil glCtx) — Open() guards this with a descriptive error.
	// Callers that want a real GLES adapter must use RequestAdapterWithSurface.
	// Must be called BEFORE RLock to avoid deadlock (enumerateDeferredGLES
	// acquires a write lock internally).
	i.enumerateDeferredGLES(nil)

	i.mu.RLock()
	adapterIDs := append([]AdapterID(nil), i.adapters...)
	i.mu.RUnlock()

	if options != nil && options.ForceFallbackAdapter {
		// ADR-083 §2.3: under the default mask the software backend is a
		// fallback candidate only; the force request is what admits it.
		adapterIDs = append(adapterIDs, i.ensureFallbackAdapters()...)
	}

	if len(adapterIDs) == 0 {
		return AdapterID{}, i.noAdapterError("")
	}

	return selectAdapterIDs(options, adapterIDs)
}

// selectAdapterIDs applies the public adapter selection policy to an explicit
// candidate list. Keeping the policy independent from Instance state lets a
// surface request select request-local adapters without exposing unqualified
// cached adapters to the selection pass.
//
// Without ForceFallbackAdapter the order is: hardware adapters (an exact
// powerPreference match first), then CPU adapters of a GPU API such as DX12
// WARP or Vulkan lavapipe, which JIT-compile shaders, then the software
// backend, which interprets them. powerPreference is a hint, never a filter
// (W3C WebGPU spec: it "must not cause requestAdapter() to fail if there is at
// least one available adapter"), as in Rust wgpu, which ranks Cpu last.
//
// ForceFallbackAdapter considers CPU adapters only and prefers the software
// backend; another CPU adapter (WARP) is chosen only when the software
// backend is not a candidate.
//
// Ties keep the candidate order.
func selectAdapterIDs(options *gputypes.RequestAdapterOptions, adapterIDs []AdapterID) (AdapterID, error) {
	if len(adapterIDs) == 0 {
		// RequestAdapter maps an empty adapter list to ErrNoBackendRegistered
		// first when the mask admitted no backend; a direct call has no mask to
		// consult, so it reports the registered-backend-without-adapter case.
		return AdapterID{}, ErrNoHardwareAdapter
	}

	rank := func(info *gputypes.AdapterInfo) int { return autoAdapterRank(info, gputypes.PowerPreferenceNone) }
	if options != nil {
		if options.ForceFallbackAdapter {
			rank = fallbackAdapterRank
		} else {
			preference := options.PowerPreference
			rank = func(info *gputypes.AdapterInfo) int { return autoAdapterRank(info, preference) }
		}
	}

	hub := GetGlobal().Hub()
	var best AdapterID
	bestRank := rankExcluded
	for _, adapterID := range adapterIDs {
		adapter, err := hub.GetAdapter(adapterID)
		if err != nil {
			continue
		}
		if r := rank(&adapter.Info); r < bestRank {
			best, bestRank = adapterID, r
		}
	}
	if bestRank != rankExcluded {
		return best, nil
	}
	if options != nil && options.ForceFallbackAdapter {
		return AdapterID{}, ErrNoFallbackAdapter
	}
	return AdapterID{}, fmt.Errorf("%w: no adapter matches the requested options", ErrNoHardwareAdapter)
}

// Adapter ranks for selectAdapterIDs; a lower rank wins.
const (
	rankPreferred = iota // hardware adapter matching powerPreference; the software backend under ForceFallbackAdapter
	rankHardware         // hardware adapter of another type
	rankCPU              // CPU adapter of a GPU API (DX12 WARP, Vulkan lavapipe, GL llvmpipe)
	rankSoftware         // the software backend
	rankExcluded         // never selected
)

// isSoftwareBackendAdapter reports whether info is our software backend's
// adapter. Other CPU adapters belong to a GPU API backend (Backend DX12 for
// WARP), which keeps them distinguishable.
func isSoftwareBackendAdapter(info *gputypes.AdapterInfo) bool {
	return info.Backend == gputypes.BackendSoftware && info.DeviceType == gputypes.DeviceTypeCPU
}

func autoAdapterRank(info *gputypes.AdapterInfo, preference gputypes.PowerPreference) int {
	switch {
	case isSoftwareBackendAdapter(info):
		return rankSoftware
	case info.DeviceType == gputypes.DeviceTypeCPU:
		return rankCPU
	case matchesPowerPreference(info.DeviceType, preference):
		return rankPreferred
	default:
		return rankHardware
	}
}

func fallbackAdapterRank(info *gputypes.AdapterInfo) int {
	switch {
	case isSoftwareBackendAdapter(info):
		return rankPreferred
	case info.DeviceType == gputypes.DeviceTypeCPU:
		return rankCPU
	default:
		return rankExcluded
	}
}

// RequestAdapterWithSurface requests an adapter matching the given options,
// using the provided HAL surface as a hint for backends that require it.
//
// For GLES, the surface hint is mandatory: EGL needs a window handle to create
// a GL context, which is required before EnumerateAdapters can return a real
// adapter. Calling without a surface yields a zero-value Adapter (nil glCtx).
func (i *Instance) RequestAdapterWithSurface(options *gputypes.RequestAdapterOptions, surfaceHint hal.Surface) (AdapterID, error) {
	if surfaceHint == nil {
		return i.RequestAdapter(options)
	}
	return i.requestAdapterWithSurfaceResolver(options, func(gputypes.Backend) hal.Surface {
		return surfaceHint
	})
}

// admitSurfaceFallbackCandidates extends a surface request's candidate list
// with the lazily admitted fallback adapters and returns their ID set. The
// caller's surface map was built before the first force request, so it cannot
// name the software backend; the software HAL surface is created on demand at
// configure (surface_native.go), which is why a missing surface hint must not
// drop a fallback adapter. The map remains authoritative for the hardware
// backends, whose instances existed when it was built.
func (i *Instance) admitSurfaceFallbackCandidates(options *gputypes.RequestAdapterOptions, allAdapterIDs []AdapterID) ([]AdapterID, map[AdapterID]struct{}) {
	if options == nil || !options.ForceFallbackAdapter {
		return allAdapterIDs, nil
	}
	fallback := i.ensureFallbackAdapters()
	if len(fallback) == 0 {
		return allAdapterIDs, nil
	}
	ids := make(map[AdapterID]struct{}, len(fallback))
	for _, id := range fallback {
		ids[id] = struct{}{}
	}
	return append(allAdapterIDs, fallback...), ids
}

// RequestAdapterWithSurfaces requests an adapter using the surface created for
// each adapter's own backend. This mirrors Rust wgpu's per-backend surface map
// while preserving RequestAdapterWithSurface for callers that own one HAL.
func (i *Instance) RequestAdapterWithSurfaces(options *gputypes.RequestAdapterOptions, surfaces map[gputypes.Backend]hal.Surface) (AdapterID, error) {
	if len(surfaces) == 0 {
		return i.RequestAdapter(options)
	}
	return i.requestAdapterWithSurfaceResolver(options, func(backend gputypes.Backend) hal.Surface {
		return surfaces[backend]
	})
}

func (i *Instance) requestAdapterWithSurfaceResolver(options *gputypes.RequestAdapterOptions, surfaceForBackend func(gputypes.Backend) hal.Surface) (AdapterID, error) {
	i.surfaceRequestMu.Lock()
	defer i.surfaceRequestMu.Unlock()

	// Remember which adapters existed before deferred enumeration. Adapters
	// created by EnumerateAdapters(surfaceHint) are already surface-qualified by
	// their backend and can be used directly. Cached adapters are qualified via
	// the optional HAL SurfaceAdapter interface below.
	i.mu.RLock()
	before := make(map[AdapterID]struct{}, len(i.adapters))
	for _, adapterID := range i.adapters {
		before[adapterID] = struct{}{}
	}
	i.mu.RUnlock()

	// Run deferred GLES enumeration with the real surface hint before collecting
	// candidates. Once glesEnumerated is true this is a no-op on later calls.
	if glesSurface := surfaceForBackend(gputypes.BackendGL); glesSurface != nil {
		i.enumerateDeferredGLES(glesSurface)
	}

	i.mu.RLock()
	allAdapterIDs := append([]AdapterID(nil), i.adapters...)
	i.mu.RUnlock()

	// ADR-083 §2.3: the lazily admitted fallback adapters are candidates too;
	// admitSurfaceFallbackCandidates explains why a missing surface hint keeps
	// them.
	allAdapterIDs, fallbackIDs := i.admitSurfaceFallbackCandidates(options, allAdapterIDs)

	hub := GetGlobal().Hub()
	i.mu.RLock()
	usingMock := i.useMock
	i.mu.RUnlock()
	candidates := make([]AdapterID, 0, len(allAdapterIDs))
	qualifiedIDs := make([]AdapterID, 0, len(allAdapterIDs))
	for _, adapterID := range allAdapterIDs {
		adapter, err := hub.GetAdapter(adapterID)
		if err != nil {
			continue
		}
		surfaceHint := surfaceForBackend(adapter.Backend)
		if surfaceHint == nil {
			if _, isFallback := fallbackIDs[adapterID]; isFallback {
				candidates = append(candidates, adapterID)
			}
			continue
		}

		// A newly enumerated adapter was produced with the surface hint. Keep
		// that backend-owned adapter as-is; this preserves existing GLES
		// enumeration behavior and avoids a second context query.
		if _, isNew := before[adapterID]; !isNew {
			candidates = append(candidates, adapterID)
			continue
		}

		if adapter.halAdapter == nil {
			if usingMock {
				// Explicit mock instances have no native surface query. Preserve
				// their established behavior instead of turning a test adapter
				// request into an unexpected compatibility error.
				candidates = append(candidates, adapterID)
			}
			continue
		}

		if qualifier, ok := adapter.halAdapter.(surfaceAdapterQualifier); ok {
			qualified, err := qualifier.QualifySurface(surfaceHint)
			if err != nil || qualified == nil {
				continue
			}

			// Register a request-local core adapter. The cached adapter and its
			// HAL object are left untouched; only the wrapper carries the queue
			// family and checked surface snapshot into Open().
			qualifiedCore := adapter
			qualifiedCore.halAdapter = qualified
			// Capabilities belong to the cached physical adapter, so the
			// wrapper carries a value copy, never the cached pointer.
			qualifiedCore.halCapabilities = qualifiedSurfaceCapabilities(adapter.halCapabilities)
			qualifiedID := hub.RegisterAdapter(&qualifiedCore)
			i.mu.Lock()
			i.surfaceAdapters = append(i.surfaceAdapters, qualifiedID)
			i.mu.Unlock()
			candidates = append(candidates, qualifiedID)
			qualifiedIDs = append(qualifiedIDs, qualifiedID)
			continue
		}

		// Backends without a request-local qualifier may still expose a
		// checked surface capability query. Do not treat a nil result as
		// compatible, and never fall back to an unqualified Vulkan adapter.
		if adapter.halAdapter.SurfaceCapabilities(surfaceHint) != nil {
			candidates = append(candidates, adapterID)
		}
	}

	if len(candidates) == 0 {
		return AdapterID{}, i.noAdapterError("no adapters compatible with surface")
	}
	selectedID, err := selectAdapterIDs(options, candidates)
	for _, qualifiedID := range qualifiedIDs {
		if err != nil || qualifiedID != selectedID {
			i.ReleaseSurfaceAdapter(qualifiedID)
		}
	}
	return selectedID, err
}

// qualifiedSurfaceCapabilities returns the capabilities of a request-local
// surface-qualified adapter: a value copy of the cached physical adapter's,
// never the instance's pointer, so the request-local wrapper cannot mutate the
// cached state. The physical facts must survive the copy — above all
// RecordsTransitions, because a wrapper that looks like a backend recording no
// transitions makes the core drop the render pass's intra-pass barriers
// (issue #429). nil (a mock adapter without HAL capabilities) stays nil.
func qualifiedSurfaceCapabilities(cached *hal.Capabilities) *hal.Capabilities {
	if cached == nil {
		return nil
	}
	caps := *cached
	return &caps
}

// ReleaseSurfaceAdapter releases a request-local adapter created by
// RequestAdapterWithSurface. Ordinary cached adapter IDs are instance-owned and
// intentionally ignored, so public Adapter.Release can use this method without
// special-casing the selected backend.
func (i *Instance) ReleaseSurfaceAdapter(id AdapterID) {
	if i == nil {
		return
	}
	i.mu.Lock()
	defer i.mu.Unlock()

	index := -1
	for candidateIndex, candidateID := range i.surfaceAdapters {
		if candidateID == id {
			index = candidateIndex
			break
		}
	}
	if index < 0 {
		return
	}

	hub := GetGlobal().Hub()
	if adapter, err := hub.GetAdapter(id); err == nil {
		if adapter.halAdapter != nil {
			adapter.halAdapter.Destroy()
		}
		_, _ = hub.UnregisterAdapter(id)
	}
	copy(i.surfaceAdapters[index:], i.surfaceAdapters[index+1:])
	i.surfaceAdapters[len(i.surfaceAdapters)-1] = AdapterID{}
	i.surfaceAdapters = i.surfaceAdapters[:len(i.surfaceAdapters)-1]
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
				Info:                  exposed.Info,
				Features:              exposed.Features,
				Limits:                exposed.Capabilities.Limits,
				DownlevelCapabilities: exposed.Capabilities.DownlevelCapabilities,
				StencilCopyNeedsBlit:  exposed.Capabilities.StencilCopyNeedsBlit,
				Backend:               exposed.Info.Backend,
				halAdapter:            exposed.Adapter,
				halCapabilities:       &exposed.Capabilities,
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

// HALInstanceEntries returns enabled HAL instances in backend-priority order.
// The returned slice is a snapshot and may be inspected without holding the
// Instance lock.
func (i *Instance) HALInstanceEntries() []HALInstanceEntry {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return append([]HALInstanceEntry(nil), i.halInstanceEntries...)
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
	i.surfaceRequestMu.Lock()
	defer i.surfaceRequestMu.Unlock()
	// fallbackMu before i.mu: the same order ensureFallbackAdapters takes, so a
	// Release racing an in-flight force request cannot race the slice header or
	// leave a registered fallback adapter behind.
	i.fallbackMu.Lock()
	defer i.fallbackMu.Unlock()

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

	// Request-local surface adapters are separate from the ordinary adapter
	// list, but still belong to this instance and must be released here.
	for _, adapterID := range i.surfaceAdapters {
		adapter, err := hub.GetAdapter(adapterID)
		if err != nil {
			continue
		}
		if adapter.halAdapter != nil {
			adapter.halAdapter.Destroy()
		}
		_, _ = hub.UnregisterAdapter(adapterID)
	}
	i.surfaceAdapters = nil

	// The lazily created software fallback adapters (ADR-083 §2.3) belong to
	// this instance the same way.
	for _, adapterID := range i.fallbackAdapters {
		adapter, err := hub.GetAdapter(adapterID)
		if err != nil {
			continue
		}
		if adapter.halAdapter != nil {
			adapter.halAdapter.Destroy()
		}
		_, _ = hub.UnregisterAdapter(adapterID)
	}
	i.fallbackAdapters = nil

	// Destroy all HAL instances (includes deferred GLES instances).
	for _, halInstance := range i.halInstances {
		halInstance.Destroy()
	}
	i.halInstances = nil
	i.halInstanceEntries = nil
	i.deferredGLES = nil
}
