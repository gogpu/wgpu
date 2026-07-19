//go:build !rust && !(js && wasm)

package wgpu

import (
	"errors"
	"slices"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/core"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/noop"
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

func TestSurfaceReleaseRetainsTargetSourceThroughHALDestroy(t *testing.T) {
	events := []string{}
	targetSource := &instanceLifecycleTargetSource{}
	firstRawSurface := &instanceLifecycleTargetSourceSurface{events: &events}
	secondRawSurface := &instanceLifecycleTargetSourceSurface{events: &events}
	surface := &Surface{
		core:           core.NewSurface(firstRawSurface, "target-source-lifecycle-test"),
		targetSource:   targetSource,
		currentBackend: gputypes.BackendVulkan,
		surfaceCreated: true,
		halSurfaces: map[gputypes.Backend]hal.Surface{
			gputypes.BackendVulkan: firstRawSurface,
			gputypes.BackendGL:     secondRawSurface,
		},
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
	instance := &Instance{core: core.NewInstanceWithMock(nil)}
	rawDevice := &instanceLifecycleDevice{events: &events}
	queue := &Queue{hal: &noop.Queue{}, halDevice: rawDevice}
	device := &Device{
		core:  core.NewDevice(rawDevice, nil, 0, gputypes.DefaultLimits(), "lifecycle-test"),
		queue: queue,
	}
	queue.device = device
	if err := instance.adoptDevice(device); err != nil {
		t.Fatalf("adopt device: %v", err)
	}
	surface := &Surface{core: core.NewSurface(&instanceLifecycleSurface{events: &events}, "lifecycle-test")}
	if err := instance.adoptSurface(surface); err != nil {
		t.Fatalf("adopt surface: %v", err)
	}

	instance.Release()
	want := []string{"device-destroy", "surface-destroy"}
	if !slices.Equal(events, want) {
		t.Fatalf("release events = %v, want %v", events, want)
	}
	if surface.HAL() != nil {
		t.Fatal("retained surface exposed HAL after instance release")
	}

	instance.Release()
	surface.Release()
	device.Release()
	if !slices.Equal(events, want) {
		t.Fatalf("idempotent releases changed events to %v", events)
	}
}

func TestInstanceReleaseRetiresAcquisitionBeforeDeviceAndPlatformSurface(t *testing.T) {
	events := []string{}
	instance := &Instance{core: core.NewInstanceWithMock(nil)}
	rawDevice := &instanceLifecycleDevice{events: &events}
	queue := &Queue{hal: &noop.Queue{}, halDevice: rawDevice}
	device := &Device{
		core:  core.NewDevice(rawDevice, nil, 0, gputypes.DefaultLimits(), "configured-lifecycle-test"),
		queue: queue,
	}
	queue.device = device
	if err := instance.adoptDevice(device); err != nil {
		t.Fatalf("adopt device: %v", err)
	}

	rawSurface := &instanceLifecycleConfiguredSurface{events: &events}
	surface := &Surface{
		core:           core.NewSurface(rawSurface, "configured-lifecycle-test"),
		surfaceCreated: true,
		currentBackend: gputypes.BackendEmpty,
	}
	if err := instance.adoptSurface(surface); err != nil {
		t.Fatalf("adopt surface: %v", err)
	}
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

	instance.Release()
	want := []string{"surface-discard", "device-destroy", "surface-destroy"}
	if !slices.Equal(events, want) {
		t.Fatalf("release events = %v, want %v", events, want)
	}
	if surface.core != nil {
		t.Fatal("surface retained core state after instance release")
	}
}

func TestSurfaceConfigureRejectsDeviceFromAnotherInstance(t *testing.T) {
	first := &Instance{core: core.NewInstanceWithMock(nil)}
	second := &Instance{core: core.NewInstanceWithMock(nil)}
	defer first.Release()
	defer second.Release()

	rawDevice := &noop.Device{}
	queue := &Queue{hal: &noop.Queue{}, halDevice: rawDevice}
	device := &Device{
		core:  core.NewDevice(rawDevice, nil, 0, gputypes.DefaultLimits(), "cross-instance-test"),
		queue: queue,
	}
	queue.device = device
	if err := first.adoptDevice(device); err != nil {
		t.Fatalf("adopt device: %v", err)
	}

	surface := &Surface{
		core:           core.NewSurface(&noop.Surface{}, "cross-instance-test"),
		surfaceCreated: true,
		currentBackend: gputypes.BackendEmpty,
	}
	if err := second.adoptSurface(surface); err != nil {
		t.Fatalf("adopt surface: %v", err)
	}
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
	if surface.core.State() != core.SurfaceStateUnconfigured {
		t.Fatalf("surface state = %v, want unconfigured", surface.core.State())
	}
}

func TestReleasedInstanceRejectsLateOwnership(t *testing.T) {
	instance := &Instance{core: core.NewInstanceWithMock(nil)}
	instance.Release()
	instance.Release()

	if !instance.isReleased() {
		t.Fatal("released instance did not report released state")
	}
	var nilInstance *Instance
	if !nilInstance.isReleased() {
		t.Fatal("nil instance did not report released state")
	}

	rawDevice := &noop.Device{}
	queue := &Queue{hal: &noop.Queue{}, halDevice: rawDevice}
	device := &Device{
		core:  core.NewDevice(rawDevice, nil, 0, gputypes.DefaultLimits(), "late-device"),
		queue: queue,
	}
	queue.device = device
	if err := instance.adoptDevice(device); !errors.Is(err, ErrReleased) {
		t.Fatalf("adoptDevice after release = %v, want ErrReleased", err)
	}
	if err := instance.adoptDevice(nil); err == nil {
		t.Fatal("adoptDevice accepted nil")
	}

	surface := &Surface{core: core.NewSurface(&noop.Surface{}, "late-surface")}
	if err := instance.adoptSurface(surface); !errors.Is(err, ErrReleased) {
		t.Fatalf("adoptSurface after release = %v, want ErrReleased", err)
	}
	if err := instance.adoptSurface(nil); err == nil {
		t.Fatal("adoptSurface accepted nil")
	}

	var nilAdapter *Adapter
	if _, err := nilAdapter.RequestDevice(nil); !errors.Is(err, ErrReleased) {
		t.Fatalf("nil Adapter.RequestDevice = %v, want ErrReleased", err)
	}
	detached := &Adapter{instance: instance}
	if _, err := detached.RequestDevice(nil); !errors.Is(err, ErrReleased) {
		t.Fatalf("released-instance Adapter.RequestDevice = %v, want ErrReleased", err)
	}

	device.Release()
	surface.Release()
}
