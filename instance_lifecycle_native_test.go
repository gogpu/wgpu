//go:build !rust && !(js && wasm)

package wgpu

import (
	"errors"
	"slices"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/hal"
	"github.com/gogpu/wgpu/internal/hal/noop"
	"github.com/gogpu/wgpu/internal/instance"
	surfacepkg "github.com/gogpu/wgpu/internal/surface"
)

type instanceLifecycleDevice struct {
	noop.Device
	events *[]string
}

func (d *instanceLifecycleDevice) Destroy() {
	*d.events = append(*d.events, "device-destroy")
}

type instanceLifecycleSurface struct {
	noop.Surface
	events *[]string
}

func (s *instanceLifecycleSurface) Destroy() {
	*s.events = append(*s.events, "surface-destroy")
}

type instanceLifecycleConfiguredSurface struct {
	noop.Surface
	events *[]string
}

func (s *instanceLifecycleConfiguredSurface) DiscardTexture(hal.SurfaceTexture) {
	*s.events = append(*s.events, "surface-discard")
}

func (s *instanceLifecycleConfiguredSurface) Unconfigure(hal.Device) {
	*s.events = append(*s.events, "surface-unconfigure")
}

func (s *instanceLifecycleConfiguredSurface) Destroy() {
	*s.events = append(*s.events, "surface-destroy")
}

type instanceLifecycleTargetSource struct{}

func (*instanceLifecycleTargetSource) SurfaceTarget() (SurfaceTargetUnsafe, error) {
	return SurfaceTargetFromAndroidNativeWindow(1), nil
}

type instanceLifecycleTargetSourceSurface struct {
	noop.Surface
	surface *Surface
	events  *[]string
}

func (s *instanceLifecycleTargetSourceSurface) Destroy() {
	if s.surface.targetSource == nil {
		*s.events = append(*s.events, "source-cleared-before-surface-destroy")
		return
	}
	*s.events = append(*s.events, "surface-destroy-with-source")
}

// newLifecycleInstance builds the public instance declaration layer over a mock
// core instance: the shape CreateInstance produces, without a GPU.
func newLifecycleInstance() *Instance {
	return &Instance{impl: instance.New(core.NewInstanceWithMock(nil))}
}

// adoptLifecycleDevice wires device to instance the way the adapter's
// RequestDevice does: the implementation's InstanceHost seam and the registry
// first, then the root identity cache the surface cross-check reads.
func adoptLifecycleDevice(t *testing.T, inst *Instance, device *Device) {
	t.Helper()
	device.impl.SetInstance(inst.impl)
	if err := inst.impl.AdoptDevice(device.impl); err != nil {
		t.Fatalf("adopt device: %v", err)
	}
	device.instance = inst
}

// adoptLifecycleSurface registers surface with inst the way the creation glue
// does: the implementation registry first, then the root identity cache.
func adoptLifecycleSurface(t *testing.T, inst *Instance, surface *Surface) {
	t.Helper()
	if err := inst.impl.AdoptSurface(surface.impl); err != nil {
		t.Fatalf("adopt surface: %v", err)
	}
	surface.instance = inst
}

func TestSurfaceReleaseRetainsTargetSourceThroughHALDestroy(t *testing.T) {
	events := []string{}
	targetSource := &instanceLifecycleTargetSource{}
	firstRawSurface := &instanceLifecycleTargetSourceSurface{events: &events}
	secondRawSurface := &instanceLifecycleTargetSourceSurface{events: &events}
	surface := &Surface{
		impl: surfacepkg.New(surfacepkg.Init{
			Core:           core.NewSurface(firstRawSurface, "target-source-lifecycle-test"),
			Backend:        gputypes.BackendVulkan,
			SurfaceCreated: true,
			HALSurfaces: map[gputypes.Backend]hal.Surface{
				gputypes.BackendVulkan: firstRawSurface,
				gputypes.BackendGL:     secondRawSurface,
			},
		}),
		targetSource: targetSource,
	}
	firstRawSurface.surface = surface
	secondRawSurface.surface = surface

	surface.Release()

	want := []string{"surface-destroy-with-source", "surface-destroy-with-source"}
	if !slices.Equal(events, want) {
		t.Fatalf("release events = %v, want %v", events, want)
	}
	if surface.targetSource != nil {
		t.Fatal("surface retained target source after HAL destruction")
	}

	surface.Release()
	if !slices.Equal(events, want) {
		t.Fatalf("idempotent release changed events to %v", events)
	}
}

func TestInstanceReleaseDestroysDevicesBeforeSurfaces(t *testing.T) {
	events := []string{}
	inst := newLifecycleInstance()
	rawDevice := &instanceLifecycleDevice{events: &events}
	device := newTestDeviceWithQueue(core.NewDevice(rawDevice, nil, 0, gputypes.DefaultLimits(), "lifecycle-test"), nil, &noop.Queue{}, nil)
	adoptLifecycleDevice(t, inst, device)
	surface := &Surface{impl: surfacepkg.NewFromHAL(core.NewSurface(&instanceLifecycleSurface{events: &events}, "lifecycle-test"))}
	adoptLifecycleSurface(t, inst, surface)

	inst.Release()
	want := []string{"device-destroy", "surface-destroy"}
	if !slices.Equal(events, want) {
		t.Fatalf("release events = %v, want %v", events, want)
	}
	if surface.impl.Core().RawSurface() != nil {
		t.Fatal("retained surface exposed HAL after instance release")
	}

	inst.Release()
	surface.Release()
	device.Release()
	if !slices.Equal(events, want) {
		t.Fatalf("idempotent releases changed events to %v", events)
	}
}

func TestInstanceReleaseRetiresAcquisitionBeforeDeviceAndPlatformSurface(t *testing.T) {
	events := []string{}
	inst := newLifecycleInstance()
	rawDevice := &instanceLifecycleDevice{events: &events}
	device := newTestDeviceWithQueue(core.NewDevice(rawDevice, nil, 0, gputypes.DefaultLimits(), "configured-lifecycle-test"), nil, &noop.Queue{}, nil)
	adoptLifecycleDevice(t, inst, device)

	rawSurface := &instanceLifecycleConfiguredSurface{events: &events}
	surface := &Surface{
		impl: surfacepkg.New(surfacepkg.Init{
			Core:           core.NewSurface(rawSurface, "configured-lifecycle-test"),
			SurfaceCreated: true,
			Backend:        gputypes.BackendEmpty,
		}),
	}
	adoptLifecycleSurface(t, inst, surface)
	if err := surface.Configure(device, &SurfaceConfiguration{
		Width:       1,
		Height:      1,
		Format:      gputypes.TextureFormatRGBA8Unorm,
		Usage:       gputypes.TextureUsageRenderAttachment,
		PresentMode: gputypes.PresentModeFifo,
		AlphaMode:   gputypes.CompositeAlphaModeAuto,
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if _, _, err := surface.GetCurrentTexture(); err != nil {
		t.Fatalf("GetCurrentTexture: %v", err)
	}

	inst.Release()
	want := []string{"surface-discard", "device-destroy", "surface-destroy"}
	if !slices.Equal(events, want) {
		t.Fatalf("release events = %v, want %v", events, want)
	}
	if surface.impl.Core() != nil {
		t.Fatal("surface retained core state after instance release")
	}
}

func TestSurfaceConfigureRejectsDeviceFromAnotherInstance(t *testing.T) {
	first := newLifecycleInstance()
	second := newLifecycleInstance()
	defer first.Release()
	defer second.Release()

	rawDevice := &noop.Device{}
	device := newTestDeviceWithQueue(core.NewDevice(rawDevice, nil, 0, gputypes.DefaultLimits(), "cross-instance-test"), nil, &noop.Queue{}, nil)
	adoptLifecycleDevice(t, first, device)

	surface := &Surface{
		impl: surfacepkg.New(surfacepkg.Init{
			Core:           core.NewSurface(&noop.Surface{}, "cross-instance-test"),
			SurfaceCreated: true,
			Backend:        gputypes.BackendEmpty,
		}),
	}
	adoptLifecycleSurface(t, second, surface)
	err := surface.Configure(device, &SurfaceConfiguration{
		Width:       1,
		Height:      1,
		Format:      gputypes.TextureFormatRGBA8Unorm,
		Usage:       gputypes.TextureUsageRenderAttachment,
		PresentMode: gputypes.PresentModeFifo,
		AlphaMode:   gputypes.CompositeAlphaModeAuto,
	})
	if err == nil {
		t.Fatal("Configure accepted a device from another instance")
	}
	if surface.impl.Core().State() != core.SurfaceStateUnconfigured {
		t.Fatalf("surface state = %v, want unconfigured", surface.impl.Core().State())
	}
}

func TestReleasedInstanceRejectsLateOwnership(t *testing.T) {
	inst := newLifecycleInstance()
	inst.Release()
	inst.Release()
	impl := inst.impl

	if !inst.isReleased() {
		t.Fatal("released instance did not report released state")
	}
	var nilInstance *Instance
	if !nilInstance.isReleased() {
		t.Fatal("nil instance did not report released state")
	}

	rawDevice := &noop.Device{}
	device := newTestDeviceWithQueue(core.NewDevice(rawDevice, nil, 0, gputypes.DefaultLimits(), "late-device"), nil, &noop.Queue{}, nil)
	if err := impl.AdoptDevice(device.impl); !errors.Is(err, ErrReleased) {
		t.Fatalf("AdoptDevice after release = %v, want ErrReleased", err)
	}
	if err := impl.AdoptDevice(nil); err == nil {
		t.Fatal("AdoptDevice accepted nil")
	}

	surface := &Surface{impl: surfacepkg.NewFromHAL(core.NewSurface(&noop.Surface{}, "late-surface"))}
	if err := impl.AdoptSurface(surface.impl); !errors.Is(err, ErrReleased) {
		t.Fatalf("AdoptSurface after release = %v, want ErrReleased", err)
	}
	if err := impl.AdoptSurface(nil); err == nil {
		t.Fatal("AdoptSurface accepted nil")
	}

	var nilAdapter *Adapter
	if _, err := nilAdapter.RequestDevice(nil); !errors.Is(err, ErrReleased) {
		t.Fatalf("nil Adapter.RequestDevice = %v, want ErrReleased", err)
	}
	detached := &Adapter{instance: inst}
	if _, err := detached.RequestDevice(nil); !errors.Is(err, ErrReleased) {
		t.Fatalf("released-instance Adapter.RequestDevice = %v, want ErrReleased", err)
	}

	device.Release()
	surface.Release()
}
