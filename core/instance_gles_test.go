//go:build !(js && wasm)

package core

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// --- minimal hal.Surface stub for surface-hint tests ---

type stubHALSurface struct{ id int }

func (s *stubHALSurface) Configure(_ hal.Device, _ *hal.SurfaceConfiguration) error { return nil }
func (s *stubHALSurface) Unconfigure(_ hal.Device)                                  {}
func (s *stubHALSurface) AcquireTexture(_ hal.Fence) (*hal.AcquiredSurfaceTexture, error) {
	return nil, nil //nolint:nilnil
}
func (s *stubHALSurface) DiscardTexture(_ hal.SurfaceTexture) {}
func (s *stubHALSurface) ActualExtent() (uint32, uint32)      { return 0, 0 }
func (s *stubHALSurface) Destroy()                            {}

// --- minimal hal.Adapter stub ---

type stubHALAdapter struct{}

func (a *stubHALAdapter) Open(_ gputypes.Features, _ gputypes.Limits) (hal.OpenDevice, error) {
	return hal.OpenDevice{}, nil
}
func (a *stubHALAdapter) TextureFormatCapabilities(_ gputypes.TextureFormat) hal.TextureFormatCapabilities {
	return hal.TextureFormatCapabilities{}
}
func (a *stubHALAdapter) SurfaceCapabilities(_ hal.Surface) *hal.SurfaceCapabilities { return nil }
func (a *stubHALAdapter) Destroy()                                                   {}

// --- trackingGLESInstance records the hint passed to EnumerateAdapters ---

type trackingGLESInstance struct {
	receivedHint hal.Surface
	halAdapter   hal.Adapter // non-nil ⇒ returns a real ExposedAdapter
}

func (i *trackingGLESInstance) CreateSurface(_ hal.SurfaceTarget) (hal.Surface, error) {
	return nil, nil //nolint:nilnil
}
func (i *trackingGLESInstance) EnumerateAdapters(hint hal.Surface) []hal.ExposedAdapter {
	i.receivedHint = hint
	if i.halAdapter == nil {
		return nil
	}
	return []hal.ExposedAdapter{
		{
			Info: gputypes.AdapterInfo{
				Name:       "GLES GPU",
				DeviceType: gputypes.DeviceTypeIntegratedGPU,
				Backend:    gputypes.BackendGL,
			},
			Adapter:      i.halAdapter,
			Capabilities: hal.Capabilities{Limits: gputypes.DefaultLimits()},
		},
	}
}
func (i *trackingGLESInstance) Destroy() {}

// --- countingGLESInstance records call count ---

type countingGLESInstance struct{ callCount int }

func (i *countingGLESInstance) CreateSurface(_ hal.SurfaceTarget) (hal.Surface, error) {
	return nil, nil //nolint:nilnil
}
func (i *countingGLESInstance) EnumerateAdapters(_ hal.Surface) []hal.ExposedAdapter {
	i.callCount++
	return nil
}
func (i *countingGLESInstance) Destroy() {}

// TestRequestAdapterWithSurface_PassesHintToEnumerateAdapters verifies that
// RequestAdapterWithSurface forwards the surface hint to enumerateDeferredGLES
// so GLES EnumerateAdapters receives the real window handle.
func TestRequestAdapterWithSurface_PassesHintToEnumerateAdapters(t *testing.T) {
	GetGlobal().Clear()

	tracker := &trackingGLESInstance{halAdapter: &stubHALAdapter{}}
	instance := &Instance{
		backends:     gputypes.BackendsAll,
		deferredGLES: []hal.Instance{tracker},
	}

	hint := hal.Surface(&stubHALSurface{id: 42})
	_, _ = instance.RequestAdapterWithSurface(nil, hint)

	if tracker.receivedHint != hint {
		t.Errorf("EnumerateAdapters received hint %v, want %v", tracker.receivedHint, hint)
	}
}

func TestRequestAdapterWithSurfacesPassesMatchingBackendHint(t *testing.T) {
	GetGlobal().Clear()

	tracker := &trackingGLESInstance{halAdapter: &stubHALAdapter{}}
	instance := &Instance{
		backends:     gputypes.BackendsAll,
		deferredGLES: []hal.Instance{tracker},
	}
	vulkanHint := hal.Surface(&stubHALSurface{id: 1})
	glHint := hal.Surface(&stubHALSurface{id: 2})

	_, _ = instance.RequestAdapterWithSurfaces(nil, map[gputypes.Backend]hal.Surface{
		gputypes.BackendVulkan: vulkanHint,
		gputypes.BackendGL:     glHint,
	})

	if tracker.receivedHint != glHint {
		t.Errorf("GLES EnumerateAdapters received hint %v, want matching GL hint %v", tracker.receivedHint, glHint)
	}
}

// TestRequestAdapterWithSurface_SelectsGLESAdapter verifies that the GLES
// adapter exposed by EnumerateAdapters(hint) is the one RequestAdapter returns.
func TestRequestAdapterWithSurface_SelectsGLESAdapter(t *testing.T) {
	GetGlobal().Clear()

	tracker := &trackingGLESInstance{halAdapter: &stubHALAdapter{}}
	instance := &Instance{
		backends:     gputypes.BackendsAll,
		deferredGLES: []hal.Instance{tracker},
	}

	adapterID, err := instance.RequestAdapterWithSurface(nil, &stubHALSurface{id: 1})
	if err != nil {
		t.Fatalf("RequestAdapterWithSurface() error: %v", err)
	}
	if adapterID.IsZero() {
		t.Fatal("RequestAdapterWithSurface() returned zero AdapterID")
	}

	hub := GetGlobal().Hub()
	adapter, err := hub.GetAdapter(adapterID)
	if err != nil {
		t.Fatalf("GetAdapter() error: %v", err)
	}
	if adapter.Info.Backend != gputypes.BackendGL {
		t.Errorf("adapter backend = %v, want BackendGL", adapter.Info.Backend)
	}
}

// TestRequestAdapterWithSurface_IdempotentEnumeration verifies the glesEnumerated
// gate prevents EnumerateAdapters from being called more than once.
func TestRequestAdapterWithSurface_IdempotentEnumeration(t *testing.T) {
	GetGlobal().Clear()

	counter := &countingGLESInstance{}
	instance := &Instance{
		backends:     gputypes.BackendsAll,
		deferredGLES: []hal.Instance{counter},
	}

	hint := hal.Surface(&stubHALSurface{id: 1})
	_, _ = instance.RequestAdapterWithSurface(nil, hint)
	_, _ = instance.RequestAdapterWithSurface(nil, hint)

	if counter.callCount != 1 {
		t.Errorf("EnumerateAdapters called %d times, want exactly 1", counter.callCount)
	}
}

// TestRequestAdapter_DirectCallPassesNilHint verifies that a direct call to
// RequestAdapter (no surface hint) triggers enumerateDeferredGLES(nil) — the
// zero-value path that returns a nil-glCtx adapter (guarded by Open()).
func TestRequestAdapter_DirectCallPassesNilHint(t *testing.T) {
	GetGlobal().Clear()

	tracker := &trackingGLESInstance{halAdapter: &stubHALAdapter{}}
	instance := &Instance{
		backends:     gputypes.BackendsAll,
		deferredGLES: []hal.Instance{tracker},
	}

	_, _ = instance.RequestAdapter(nil)

	if tracker.receivedHint != nil {
		t.Errorf("direct RequestAdapter passed non-nil hint %v, want nil", tracker.receivedHint)
	}
}

// TestRequestAdapterWithSurface_WinsOverDirectCall verifies that a
// RequestAdapterWithSurface call wins when interleaved with RequestAdapter:
// the surface-backed adapter is selected because enumerateDeferredGLES(hint)
// marks glesEnumerated=true before the nil call in RequestAdapter runs.
func TestRequestAdapterWithSurface_WinsOverDirectCall(t *testing.T) {
	GetGlobal().Clear()

	tracker := &trackingGLESInstance{halAdapter: &stubHALAdapter{}}
	instance := &Instance{
		backends:     gputypes.BackendsAll,
		deferredGLES: []hal.Instance{tracker},
	}

	hint := hal.Surface(&stubHALSurface{id: 99})
	adapterID, err := instance.RequestAdapterWithSurface(nil, hint)
	if err != nil {
		t.Fatalf("RequestAdapterWithSurface() error: %v", err)
	}

	// Subsequent direct RequestAdapter must return the same adapter (already enumerated).
	adapterID2, err := instance.RequestAdapter(nil)
	if err != nil {
		t.Fatalf("second RequestAdapter() error: %v", err)
	}

	if adapterID != adapterID2 {
		t.Errorf("RequestAdapter returned different adapter after WithSurface; got %v, want %v",
			adapterID2, adapterID)
	}

	// Hint must have been the real surface, not nil.
	if tracker.receivedHint != hint {
		t.Errorf("received hint %v, want %v", tracker.receivedHint, hint)
	}
}
