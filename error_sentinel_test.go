//go:build !rust && !(js && wasm) && !android

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/hal"
)

// emptyHALInstance is a HAL instance that enumerates no adapter.
type emptyHALInstance struct{}

func (emptyHALInstance) CreateSurface(hal.SurfaceTarget) (hal.Surface, error) {
	return nil, hal.ErrUnsupportedSurfaceTarget
}

func (emptyHALInstance) EnumerateAdapters(hal.Surface) []hal.ExposedAdapter { return nil }

func (emptyHALInstance) Destroy() {}

// emptyProvider stands in for the software backend with no adapter on this
// machine: it keeps the software variant so the mask admits it.
type emptyProvider struct{}

func (emptyProvider) Variant() gputypes.Backend { return gputypes.BackendSoftware }

func (emptyProvider) CreateInstance(*hal.InstanceDescriptor) (hal.Instance, error) {
	return emptyHALInstance{}, nil
}

func (emptyProvider) IsAvailable() bool { return true }

// TestRequestAdapterNoAdapterSentinelPublicPath proves that a selection failure
// reaches the caller of Instance.RequestAdapter as ADR-083 §2.2's sentinel, so
// errors.Is works through the public API. The condition driven here is "a
// backend is registered and admitted by the mask but has no adapter on this
// machine", which names ErrNoHardwareAdapter and nothing else.
//
// The test binary links the software backend, so that provider is swapped for
// one without an adapter for the duration of the test and restored afterwards.
func TestRequestAdapterNoAdapterSentinelPublicPath(t *testing.T) {
	core.RegisterHALBackends() // fire the one-shot registration before replacing it
	saved, ok := core.GetBackendProvider(gputypes.BackendSoftware)
	if !ok {
		t.Fatal("no software backend provider registered in the test binary")
	}
	core.RegisterBackendProvider(emptyProvider{})
	t.Cleanup(func() { core.RegisterBackendProvider(saved) })

	inst, err := CreateInstance(&InstanceDescriptor{Backends: gputypes.BackendsSoftware})
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	t.Cleanup(inst.Release)

	_, err = inst.RequestAdapter(nil)
	if err == nil {
		t.Fatal("RequestAdapter: want an error; the only selectable backend has no adapter")
	}
	if !errors.Is(err, ErrNoHardwareAdapter) {
		t.Errorf("RequestAdapter error = %v, want ErrNoHardwareAdapter through errors.Is", err)
	}
	if errors.Is(err, ErrNoBackendRegistered) {
		t.Errorf("RequestAdapter error = %v, must not match ErrNoBackendRegistered", err)
	}
}
