//go:build !(js && wasm)

package core

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// fakeGLContext stands in for an EGL context that dies with the last surface
// and device built on it.
type fakeGLContext struct {
	name string
	dead bool
}

var errFakeContextDead = errors.New("fake GLES context released")

// fakeGLSurface is a GLES-like surface: it carries the context it was
// created on.
type fakeGLSurface struct {
	stubHALSurface
	ctx *fakeGLContext
}

// fakeGLAdapter is a GLES-like adapter: it points weakly at a context and
// qualifies itself for a surface by binding to that surface's context, as
// hal/gles.Adapter.QualifySurface does.
type fakeGLAdapter struct {
	stubHALAdapter
	ctx       *fakeGLContext
	qualified int // QualifySurface calls on this adapter
}

func (a *fakeGLAdapter) Open(_ gputypes.Features, _ gputypes.Limits) (hal.OpenDevice, error) {
	if a.ctx == nil || a.ctx.dead {
		return hal.OpenDevice{}, errFakeContextDead
	}
	return hal.OpenDevice{}, nil
}

func (a *fakeGLAdapter) QualifySurface(surface hal.Surface) (hal.Adapter, error) {
	a.qualified++
	s, ok := surface.(*fakeGLSurface)
	if !ok || s.ctx == nil || s.ctx.dead {
		return nil, errFakeContextDead
	}
	return &fakeGLAdapter{ctx: s.ctx}, nil
}

var _ surfaceAdapterQualifier = (*fakeGLAdapter)(nil)

// fakeGLInstance enumerates one adapter bound to the hint surface's context.
type fakeGLInstance struct {
	enumerations int
	adapters     []*fakeGLAdapter
}

func (i *fakeGLInstance) CreateSurface(hal.SurfaceTarget) (hal.Surface, error) {
	return nil, nil //nolint:nilnil
}

func (i *fakeGLInstance) EnumerateAdapters(hint hal.Surface) []hal.ExposedAdapter {
	i.enumerations++
	a := &fakeGLAdapter{}
	if s, ok := hint.(*fakeGLSurface); ok {
		a.ctx = s.ctx
	}
	i.adapters = append(i.adapters, a)
	return []hal.ExposedAdapter{{
		Adapter: a,
		Info: gputypes.AdapterInfo{
			Name:       "fake GLES",
			DeviceType: gputypes.DeviceTypeIntegratedGPU,
			Backend:    gputypes.BackendGL,
		},
		Capabilities: hal.Capabilities{Limits: gputypes.DefaultLimits()},
	}}
}

func (i *fakeGLInstance) Destroy() {}

func openRequested(t *testing.T, id AdapterID) error {
	t.Helper()
	adapter, err := GetGlobal().Hub().GetAdapter(id)
	if err != nil {
		t.Fatalf("GetAdapter: %v", err)
	}
	_, err = adapter.halAdapter.Open(0, gputypes.DefaultLimits())
	return err
}

// TestGLESAdapterRequalifiedForNewSurface is the window-closed-and-reopened
// case: GLES enumerates once, with the first surface. After that surface's
// context is gone, a request with a new surface must give an adapter bound to
// the new surface's context, not the dead cached one.
func TestGLESAdapterRequalifiedForNewSurface(t *testing.T) {
	GetGlobal().Clear()

	gles := &fakeGLInstance{}
	instance := &Instance{
		backends:     gputypes.BackendsAll,
		deferredGLES: []hal.Instance{gles},
	}
	t.Cleanup(instance.Destroy)

	first := &fakeGLSurface{ctx: &fakeGLContext{name: "window 1"}}
	firstID, err := instance.RequestAdapterWithSurface(nil, first)
	if err != nil {
		t.Fatalf("first RequestAdapterWithSurface: %v", err)
	}
	if err := openRequested(t, firstID); err != nil {
		t.Fatalf("Open on the first window's adapter: %v", err)
	}

	// The window closes: its surface and device release the context.
	first.ctx.dead = true
	if err := openRequested(t, firstID); !errors.Is(err, errFakeContextDead) {
		t.Fatalf("Open on the cached adapter of a closed window = %v, want a typed error", err)
	}

	second := &fakeGLSurface{ctx: &fakeGLContext{name: "window 2"}}
	secondID, err := instance.RequestAdapterWithSurface(nil, second)
	if err != nil {
		t.Fatalf("second RequestAdapterWithSurface: %v", err)
	}
	if gles.enumerations != 1 {
		t.Fatalf("GLES enumerated %d times, want once (the adapter is requalified, not re-enumerated)", gles.enumerations)
	}
	if gles.adapters[0].qualified != 1 {
		t.Fatalf("cached adapter qualified %d times, want 1", gles.adapters[0].qualified)
	}
	if secondID == firstID {
		t.Fatal("the new window got the cached adapter of the closed one")
	}
	if err := openRequested(t, secondID); err != nil {
		t.Fatalf("Open on the reopened window's adapter: %v", err)
	}

	// The request-local adapter is released with its request.
	instance.ReleaseSurfaceAdapter(secondID)
	if _, err := GetGlobal().Hub().GetAdapter(secondID); err == nil {
		t.Fatal("ReleaseSurfaceAdapter left the request-local adapter registered")
	}
}

// TestGLESAdapterQualifyFailureIsNoCandidate checks that a surface whose
// context cannot back an adapter yields no candidate, not the cached one.
func TestGLESAdapterQualifyFailureIsNoCandidate(t *testing.T) {
	GetGlobal().Clear()

	gles := &fakeGLInstance{}
	instance := &Instance{
		backends:     gputypes.BackendsAll,
		deferredGLES: []hal.Instance{gles},
	}
	t.Cleanup(instance.Destroy)

	first := &fakeGLSurface{ctx: &fakeGLContext{name: "window 1"}}
	if _, err := instance.RequestAdapterWithSurface(nil, first); err != nil {
		t.Fatalf("first RequestAdapterWithSurface: %v", err)
	}
	dead := &fakeGLSurface{ctx: &fakeGLContext{name: "released", dead: true}}
	if _, err := instance.RequestAdapterWithSurface(nil, dead); err == nil {
		t.Fatal("a surface without a live context got an adapter")
	}
}
