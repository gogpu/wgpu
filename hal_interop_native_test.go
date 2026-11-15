//go:build !rust && !(js && wasm)

package wgpu

import (
	"reflect"
	"testing"

	"github.com/gogpu/gputypes"
	publichal "github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/hal/noop"
)

// TestHALAccessorSignatures pins the method set: all four accessors take no
// arguments and return hal.Handles, on every build.
func TestHALAccessorSignatures(t *testing.T) {
	want := reflect.TypeOf(publichal.Handles{})
	for _, typ := range []reflect.Type{
		reflect.TypeOf((*Instance)(nil)),
		reflect.TypeOf((*Adapter)(nil)),
		reflect.TypeOf((*Device)(nil)),
		reflect.TypeOf((*Surface)(nil)),
	} {
		method, ok := typ.MethodByName("HAL")
		if !ok {
			t.Fatalf("%s has no HAL method", typ.Elem())
		}
		if got := method.Type; got.NumIn() != 1 || got.NumOut() != 1 || got.Out(0) != want {
			t.Fatalf("%s.HAL type = %v, want func() hal.Handles", typ.Elem(), got)
		}
	}
}

// TestHALNativeNilObjects checks the nil guard of every accessor.
func TestHALNativeNilObjects(t *testing.T) {
	var (
		instance *Instance
		adapter  *Adapter
		device   *Device
		surface  *Surface
	)
	for name, got := range map[string]publichal.Handles{
		"Instance": instance.HAL(),
		"Adapter":  adapter.HAL(),
		"Device":   device.HAL(),
		"Surface":  surface.HAL(),
	} {
		if got != (publichal.Handles{}) {
			t.Errorf("nil %s.HAL() = %+v, want zero Handles", name, got)
		}
	}
}

// TestHALNativeNoopDevice proves the plumbing without a GPU: the core device
// hands the HAL device through, the no-op backend names its Kind, and a device
// with no HAL implementation yields the zero Handles.
func TestHALNativeNoopDevice(t *testing.T) {
	device := &Device{core: core.NewDevice(&noop.Device{}, nil, 0, gputypes.DefaultLimits(), "hal-test")}
	want := publichal.Handles{Kind: publichal.KindNoop}
	if got := device.HAL(); got != want {
		t.Fatalf("noop device HAL() = %+v, want %+v", got, want)
	}

	coreOnly := &Device{core: core.NewDevice(nil, nil, 0, gputypes.DefaultLimits(), "hal-test")}
	if got := coreOnly.HAL(); got != (publichal.Handles{}) {
		t.Fatalf("device without HAL HAL() = %+v, want zero Handles", got)
	}
}

// TestHALNativeAdapterAndSurface checks the adapter and surface accessors
// against HAL objects with no native handles.
func TestHALNativeAdapterAndSurface(t *testing.T) {
	adapter := &Adapter{core: &core.Adapter{}}
	if got := adapter.HAL(); got != (publichal.Handles{}) {
		t.Fatalf("adapter without HAL HAL() = %+v, want zero Handles", got)
	}

	surface := &Surface{core: core.NewSurface(&noop.Surface{}, "hal-test")}
	defer surface.Release()
	want := publichal.Handles{Kind: publichal.KindNoop}
	if got := surface.HAL(); got != want {
		t.Fatalf("noop surface HAL() = %+v, want %+v", got, want)
	}
}

// TestHALNativeInstanceWithoutHAL checks that an instance created without HAL
// instances yields the zero Handles.
func TestHALNativeInstanceWithoutHAL(t *testing.T) {
	instance := &Instance{core: core.NewInstanceWithMock(nil)}
	if got := instance.HAL(); got != (publichal.Handles{}) {
		t.Fatalf("instance without HAL HAL() = %+v, want zero Handles", got)
	}
}
