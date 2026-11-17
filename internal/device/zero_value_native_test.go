//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package device

// The device domain's zero-value, nil-state and released-device contract
// (ADR-070 S11, commit 3). Every answer reproduces the pre-move public Device of
// device_native.go at 1d1778e, cited per case, with the two exceptions named
// below; where the pre-move code panicked, that is recorded here and the case
// is not invoked. The released answers are core.ErrReleased by identity:
// errors.Is is the callers' contract.
//
// Two classes answer differently from the pre-move root Device, both named in
// the CHANGELOG. Features and Limits on a zero-value device dereferenced d.core
// and panicked pre-move (device_native.go:74-81 at 1d1778e); the seam guards
// them now. Release on a zero-value Device is a no-op where pre-move it
// panicked through the nil device — the safe direction, the window's only
// intentional behavior change, carded as P2-CORE-034. A nil
// receiver is guarded by the destination's accessors, where pre-move only
// destroyQueue/lastSubmissionIndex/halDevice guarded their field (:1742, :1787,
// :1795) and the exported Features/Limits did not; Queue in the root
// declaration layer still has no nil guard, exactly as pre-move (:68-71). The
// widening is the seam's, and the table pins it so no later step narrows it
// back into a panic.
//
// What this table pins is the implementation's zero value. The public
// wgpu.Device caller actually holds was the other half of the contract and was
// missing here (S11 review F2): its declaration layer forwards to a nil
// *device.Device, so it used to panic where the zero struct answered. The
// fix round gives every entry point a nil-receiver guard that delegates to the
// zero struct, so the two receivers now answer identically; the root test
// TestPublicZeroValueDeviceAnswers proves the create, fence and wait rows
// through the public receiver, and the remaining rows are pinned here on both
// zero-value receivers and were verified by probe against the pre-move tree.

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/encoder"
	"github.com/gogpu/wgpu/internal/hal"
)

// deviceReceiver is one of the two receivers every zero-value answer is pinned
// on: a nil *Device and the zero struct.
type deviceReceiver struct {
	name string
	make func() *Device
}

// zeroValueReceivers returns the two receivers, each built fresh per case so a
// setter in one case cannot leak into another.
func zeroValueReceivers() []deviceReceiver {
	return []deviceReceiver{
		{"nil receiver", func() *Device { return nil }},
		{"zero struct", func() *Device { return &Device{} }},
	}
}

// runOnZeroValues runs test on both zero-value receivers.
func runOnZeroValues(t *testing.T, test func(t *testing.T, d *Device)) {
	t.Helper()
	for _, recv := range zeroValueReceivers() {
		t.Run(recv.name, func(t *testing.T) { test(t, recv.make()) })
	}
}

// TestZeroValueAccessors pins the read-only answers: no state, no panic.
func TestZeroValueAccessors(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T, d *Device)
	}{
		{
			name: "Core is nil",
			run: func(t *testing.T, d *Device) {
				t.Helper()
				if got := d.Core(); got != nil {
					t.Fatalf("Core = %p, want nil", got)
				}
			},
		},
		{
			name: "IsReleased is false",
			run: func(t *testing.T, d *Device) {
				t.Helper()
				if d.IsReleased() {
					t.Fatal("neither the nil receiver nor the zero struct is released")
				}
			},
		},
		{
			name: "HALDevice is nil",
			run: func(t *testing.T, d *Device) {
				t.Helper()
				if got := d.HALDevice(); got != nil {
					t.Fatalf("HALDevice = %v, want nil", got)
				}
			},
		},
		{
			name: "DestroyQueue is nil",
			run: func(t *testing.T, d *Device) {
				t.Helper()
				if got := d.DestroyQueue(); got != nil {
					t.Fatalf("DestroyQueue = %p, want nil", got)
				}
			},
		},
		{
			name: "LastSubmissionIndex is 0",
			run: func(t *testing.T, d *Device) {
				t.Helper()
				if got := d.LastSubmissionIndex(); got != 0 {
					t.Fatalf("LastSubmissionIndex = %d, want 0", got)
				}
			},
		},
		{
			name: "HALQueue is nil",
			run: func(t *testing.T, d *Device) {
				t.Helper()
				if got := d.HALQueue(); got != nil {
					t.Fatalf("HALQueue = %v, want nil", got)
				}
			},
		},
		{
			name: "EncoderPool is nil",
			run: func(t *testing.T, d *Device) {
				t.Helper()
				if got := d.EncoderPool(); got != nil {
					t.Fatalf("EncoderPool = %v, want an untyped nil", got)
				}
			},
		},
		{
			name: "Features is 0",
			run: func(t *testing.T, d *Device) {
				t.Helper()
				if got := d.Features(); got != 0 {
					t.Fatalf("Features = %#x, want 0", got)
				}
			},
		},
		{
			name: "Limits is zero",
			run: func(t *testing.T, d *Device) {
				t.Helper()
				if got := d.Limits(); got != (gputypes.Limits{}) {
					t.Fatalf("Limits = %+v, want the zero value", got)
				}
			},
		},
	}
	for _, tt := range tests {
		runOnZeroValues(t, tt.run)
	}
}

// TestZeroValueGuards pins the guarded no-ops: the health check and poll
// answer without state, and the seam setters either guard a nil receiver or
// write the zero struct without touching anything.
func TestZeroValueGuards(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T, d *Device)
	}{
		{
			name: "CheckHealth is nil",
			run: func(t *testing.T, d *Device) {
				t.Helper()
				if err := d.CheckHealth(); err != nil {
					t.Fatalf("CheckHealth = %v, want nil", err)
				}
			},
		},
		{
			name: "Poll reports no work",
			run: func(t *testing.T, d *Device) {
				t.Helper()
				if d.Poll(core.PollPoll) {
					t.Fatal("Poll(PollPoll) = true, want false")
				}
				if d.Poll(core.PollWait) {
					t.Fatal("Poll(PollWait) = true, want false")
				}
			},
		},
		{
			name: "SetInstance is a no-op",
			run: func(t *testing.T, d *Device) {
				t.Helper()
				d.SetInstance(nil)
				d.SetInstance(&testInstanceHost{})
			},
		},
		{
			name: "SetTestHooks is a no-op",
			run: func(t *testing.T, d *Device) {
				t.Helper()
				d.SetTestHooks(nil)
			},
		},
		{
			name: "SetPoolForTest is a no-op",
			run: func(t *testing.T, d *Device) {
				t.Helper()
				d.SetPoolForTest(nil)
			},
		},
		{
			name: "SetQueueForTest is a no-op",
			run: func(t *testing.T, d *Device) {
				t.Helper()
				d.SetQueueForTest(nil)
			},
		},
	}
	for _, tt := range tests {
		runOnZeroValues(t, tt.run)
	}
}

// TestRecordingsRegistry pins the one case where the nil receiver and the zero
// struct answer differently: Recordings takes the address of the registry
// field, so a nil receiver has none and a zero struct has an empty one.
func TestRecordingsRegistry(t *testing.T) {
	var nilDevice *Device
	if got := nilDevice.Recordings(); got != nil {
		t.Fatalf("Recordings on a nil receiver = %v, want nil", got)
	}
	zero := &Device{}
	if got := zero.Recordings(); got == nil {
		t.Fatal("Recordings on the zero struct = nil, want the empty registry")
	}
}

// TestNilReceiverReleaseIsANoOp pins the pre-move nil guard
// (device_native.go:1672-1673 at 1d1778e): Release on a nil receiver returns
// instead of panicking, and stays idempotent. The zero struct is deliberately
// not covered: Release dereferences d.core (:1734 pre-move, :1725 here), so it
// panicked then and panics now.
func TestNilReceiverReleaseIsANoOp(t *testing.T) {
	var d *Device
	d.Release()
	d.Release() // idempotent
}

// TestMaintainAfterIdleNilQueue pins the pre-move nil-queue early return
// (device_native.go:1629 at 1d1778e; the root coverage test covers the same
// guard through the public hook): maintenance on a device with no queue must
// not panic, on either zero-value receiver.
func TestMaintainAfterIdleNilQueue(t *testing.T) {
	runOnZeroValues(t, func(t *testing.T, d *Device) {
		d.MaintainAfterIdle()
	})
}

// TestRenderPipelineMissingFragmentModule pins the descriptor check that keeps
// a module-less fragment state from reaching the HAL (S11 review F1). toHAL
// drops the fragment stage when its module is nil, so the descriptor arrives
// with no fragment while HasFragment is set; the check is the last place the
// invalid descriptor is visible. Both zero-value receivers answer it, which is
// what the nil receiver's delegation guard forwards to.
func TestRenderPipelineMissingFragmentModule(t *testing.T) {
	runOnZeroValues(t, func(t *testing.T, d *Device) {
		_, err := d.CreateRenderPipeline(&hal.RenderPipelineDescriptor{Label: "frag-nil"}, RenderPipelineExtras{HasFragment: true})
		var rpe *core.CreateRenderPipelineError
		if !errors.As(err, &rpe) || rpe.Kind != core.CreateRenderPipelineErrorMissingFragmentModule {
			t.Fatalf("error %v (%T), want *core.CreateRenderPipelineError of kind MissingFragmentModule", err, err)
		}
		if rpe.Label != "frag-nil" {
			t.Fatalf("error label = %q, want the HAL descriptor's label", rpe.Label)
		}
	})
}

// TestZeroValueCreateGuards pins the create, fence, error-scope and wait
// answers on both zero-value receivers. Each string is the pre-move one
// (device_native.go:89, :136, :317, :439, :480, :516, :569, :939, :1179,
// :1466, :1809 at 1d1778e); the pre-move public Device answered them too,
// which the root test TestPublicZeroValueDeviceAnswers proves through the
// declaration layer. The nil receiver reaches these answers through the
// delegation guard in each entry point (S11 review F2); before the fix round
// every case below panicked on it.
//
// Five cases are recorded and not invoked, because the zero struct itself
// panics and the guard therefore only forwards the panic: CreateBuffer with a
// descriptor and CreateCommandEncoder reach the nil core (:201 and :1447 here;
// :99 and :1414 pre-move), PushErrorScope and PopErrorScope reach it through
// core (:1599-1605 here), and Release does (:1725 here; :1734 pre-move).
func TestZeroValueCreateGuards(t *testing.T) {
	descriptorTests := []struct {
		name string
		run  func(d *Device) error
		want string
	}{
		{"CreateBuffer", func(d *Device) error { _, err := d.CreateBuffer(nil); return err }, "wgpu: buffer descriptor is nil"},
		{"CreateTexture", func(d *Device) error { _, err := d.CreateTexture(nil); return err }, "wgpu: texture descriptor is nil"},
		{"CreateTextureView", func(d *Device) error { _, err := d.CreateTextureView(nil, nil); return err }, "wgpu: texture is nil"},
		{"CreateShaderModule", func(d *Device) error { _, err := d.CreateShaderModule(nil); return err }, "wgpu: shader module descriptor is nil"},
		{"CreateBindGroupLayout", func(d *Device) error { _, err := d.CreateBindGroupLayout(nil); return err }, "wgpu: bind group layout descriptor is nil"},
		{"CreatePipelineLayout", func(d *Device) error { _, err := d.CreatePipelineLayout(nil, nil); return err }, "wgpu: pipeline layout descriptor is nil"},
		{"CreateBindGroup", func(d *Device) error { _, err := d.CreateBindGroup(nil); return err }, "wgpu: bind group descriptor is nil"},
		{"CreateRenderPipeline", func(d *Device) error {
			_, err := d.CreateRenderPipeline(nil, RenderPipelineExtras{})
			return err
		}, "wgpu: render pipeline descriptor is nil"},
		{"CreateComputePipeline", func(d *Device) error { _, err := d.CreateComputePipeline(nil, nil, nil); return err }, "wgpu: compute pipeline descriptor is nil"},
		{"CreateQuerySet", func(d *Device) error { _, err := d.CreateQuerySet(nil); return err }, "wgpu: query set descriptor is nil"},
		{"CreateRenderBundleEncoder", func(d *Device) error { _, err := d.CreateRenderBundleEncoder(nil); return err }, "wgpu: render bundle encoder descriptor is nil"},
	}
	for _, tt := range descriptorTests {
		t.Run(tt.name, func(t *testing.T) {
			runOnZeroValues(t, func(t *testing.T, d *Device) {
				if err := tt.run(d); err == nil || err.Error() != tt.want {
					t.Fatalf("%s error = %v, want %q", tt.name, err, tt.want)
				}
			})
		})
	}

	releasedTests := []struct {
		name string
		run  func(d *Device) error
	}{
		{"CreateSampler", func(d *Device) error { _, err := d.CreateSampler(nil); return err }},
		{"CreateTexture without a HAL device", func(d *Device) error {
			_, err := d.CreateTexture(&hal.TextureDescriptor{})
			return err
		}},
		{"CreateRenderBundleEncoder without a HAL device", func(d *Device) error {
			_, err := d.CreateRenderBundleEncoder(&encoder.RenderBundleEncoderDescriptor{})
			return err
		}},
		{"CreateFence", func(d *Device) error { _, err := d.CreateFence(); return err }},
		{"WaitIdle", func(d *Device) error { return d.WaitIdle() }},
		{"ResetFence", func(d *Device) error { return d.ResetFence(nil) }},
		{"GetFenceStatus", func(d *Device) error { _, err := d.GetFenceStatus(nil); return err }},
		{"WaitForFence", func(d *Device) error { _, err := d.WaitForFence(nil, 0, 0); return err }},
	}
	for _, tt := range releasedTests {
		t.Run(tt.name, func(t *testing.T) {
			runOnZeroValues(t, func(t *testing.T, d *Device) {
				if err := tt.run(d); !errors.Is(err, core.ErrReleased) {
					t.Fatalf("%s error = %v, want core.ErrReleased", tt.name, err)
				}
			})
		})
	}

	// The void entry points answer without state on both receivers: a command
	// buffer that is nil or carries no HAL buffers has nothing to free, and
	// the zero device has no HAL device to free it on.
	runOnZeroValues(t, func(t *testing.T, d *Device) {
		d.FreeCommandBuffer(nil)
		d.FreeCommandBuffer(&encoder.CommandBuffer{})
	})
}

// TestNilCoreContract pins a device built without core state or queue: the
// accessors answer zero, the create paths reach their HAL checks and report
// core.ErrReleased, and the descriptor guard answers first.
func TestNilCoreContract(t *testing.T) {
	d := New(nil, nil, nil)

	if got := d.Core(); got != nil {
		t.Fatalf("Core = %p, want nil", got)
	}
	if d.IsReleased() {
		t.Fatal("a device without core state is not released")
	}
	if got := d.Features(); got != 0 {
		t.Fatalf("Features = %#x, want 0", got)
	}
	if got := d.Limits(); got != (gputypes.Limits{}) {
		t.Fatalf("Limits = %+v, want the zero value", got)
	}
	if got := d.HALDevice(); got != nil {
		t.Fatalf("HALDevice = %v, want nil", got)
	}
	if got := d.DestroyQueue(); got != nil {
		t.Fatalf("DestroyQueue = %p, want nil", got)
	}
	if got := d.HALQueue(); got != nil {
		t.Fatalf("HALQueue = %v, want nil", got)
	}
	if got := d.LastSubmissionIndex(); got != 0 {
		t.Fatalf("LastSubmissionIndex = %d, want 0", got)
	}
	if err := d.CheckHealth(); err != nil {
		t.Fatalf("CheckHealth = %v, want nil", err)
	}
	if d.Poll(core.PollPoll) {
		t.Fatal("Poll(PollPoll) = true, want false")
	}
	if _, err := d.CreateSampler(nil); !errors.Is(err, core.ErrReleased) {
		t.Fatalf("CreateSampler error = %v, want core.ErrReleased", err)
	}
	if err := d.WaitIdle(); !errors.Is(err, core.ErrReleased) {
		t.Fatalf("WaitIdle error = %v, want core.ErrReleased", err)
	}
	if _, err := d.CreateBuffer(nil); err == nil || err.Error() != "wgpu: buffer descriptor is nil" {
		t.Fatalf("CreateBuffer(nil) error = %v, want the descriptor error", err)
	}
	// Release on this device panics at d.core.Destroy (device_native.go:1725),
	// exactly as it did pre-move: it is recorded, not invoked.
}

// TestNilQueueContract pins the nil-queue answers of a device with core state
// but no queue: maintenance and the submission index are no-ops, and Release
// still drains and destroys the HAL device.
func TestNilQueueContract(t *testing.T) {
	events := make([]string, 0, 2)
	coreDevice := core.NewDevice(
		&testHALDevice{events: &events}, nil, 0, gputypes.DefaultLimits(), "test-nil-queue",
	)
	d := New(coreDevice, nil, nil)

	if got := d.HALQueue(); got != nil {
		t.Fatalf("HALQueue = %v, want nil", got)
	}
	if got := d.LastSubmissionIndex(); got != 0 {
		t.Fatalf("LastSubmissionIndex = %d, want 0", got)
	}
	d.MaintainAfterIdle() // the pre-move nil-queue guard, no panic
	if d.Poll(core.PollPoll) {
		t.Fatal("Poll(PollPoll) = true, want false")
	}

	d.Release()
	if !d.IsReleased() {
		t.Fatal("Release did not mark a queue-less device released")
	}
	if len(events) != 2 || events[0] != "wait-idle" || events[1] != "destroy" {
		t.Fatalf("HAL lifecycle events = %v, want [wait-idle destroy]", events)
	}
}

// TestReleasedDeviceContract pins the released answers: every create path and
// WaitIdle answer core.ErrReleased by identity (the pre-move guards at
// device_native.go:86, :133, :406, :512, :565, :944, :1184, :1362, :1602 at
// 1d1778e), CheckHealth follows, the HAL device is gone, and Release is
// idempotent.
func TestReleasedDeviceContract(t *testing.T) {
	events := make([]string, 0, 2)
	d := newTestDevice(&events)
	d.Release()

	if !d.IsReleased() {
		t.Fatal("Release did not mark the device released")
	}
	if got := d.HALDevice(); got != nil {
		t.Fatalf("HALDevice after Release = %v, want nil", got)
	}

	createTests := []struct {
		name string
		run  func() error
	}{
		{"CreateBuffer", func() error {
			_, err := d.CreateBuffer(&hal.BufferDescriptor{Label: "post-release", Size: 4, Usage: gputypes.BufferUsageVertex})
			return err
		}},
		{"CreateTexture", func() error {
			_, err := d.CreateTexture(&hal.TextureDescriptor{Label: "post-release"})
			return err
		}},
		{"CreateTextureView", func() error { _, err := d.CreateTextureView(nil, nil); return err }},
		{"CreateSampler", func() error { _, err := d.CreateSampler(nil); return err }},
		{"CreateShaderModule", func() error { _, err := d.CreateShaderModule(nil); return err }},
		{"CreateBindGroupLayout", func() error { _, err := d.CreateBindGroupLayout(nil); return err }},
		{"CreatePipelineLayout", func() error { _, err := d.CreatePipelineLayout(nil, nil); return err }},
		{"CreateBindGroup", func() error { _, err := d.CreateBindGroup(nil); return err }},
		{"CreateRenderPipeline", func() error {
			_, err := d.CreateRenderPipeline(nil, RenderPipelineExtras{})
			return err
		}},
		{"CreateComputePipeline", func() error { _, err := d.CreateComputePipeline(nil, nil, nil); return err }},
		{"CreateCommandEncoder", func() error { _, err := d.CreateCommandEncoder(""); return err }},
		{"CreateFence", func() error { _, err := d.CreateFence(); return err }},
		{"CreateQuerySet", func() error { _, err := d.CreateQuerySet(nil); return err }},
		{"CreateRenderBundleEncoder", func() error {
			_, err := d.CreateRenderBundleEncoder(&encoder.RenderBundleEncoderDescriptor{})
			return err
		}},
	}
	for _, tt := range createTests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); !errors.Is(err, core.ErrReleased) {
				t.Fatalf("%s error = %v, want core.ErrReleased", tt.name, err)
			}
		})
	}

	if err := d.WaitIdle(); !errors.Is(err, core.ErrReleased) {
		t.Fatalf("WaitIdle error = %v, want core.ErrReleased", err)
	}
	if err := d.CheckHealth(); !errors.Is(err, core.ErrReleased) {
		t.Fatalf("CheckHealth error = %v, want core.ErrReleased", err)
	}

	d.Release() // idempotent: no second wait-idle/destroy
	if len(events) != 2 || events[0] != "wait-idle" || events[1] != "destroy" {
		t.Fatalf("HAL lifecycle events = %v, want [wait-idle destroy] once", events)
	}
}

// TestReleaseUsesTheInstanceSeam pins the teardown's instance seam: Release
// asks the host for the device's surfaces once and unregisters the device
// afterwards, so the seam's two calls keep the shape the root host adapts.
func TestReleaseUsesTheInstanceSeam(t *testing.T) {
	events := make([]string, 0, 2)
	d := newTestDevice(&events)
	host := &testInstanceHost{}
	d.SetInstance(host)

	d.Release()

	if host.surfacesCalls != 1 {
		t.Errorf("SurfacesForDevice calls = %d, want 1", host.surfacesCalls)
	}
	if !host.unregistered {
		t.Error("Release did not unregister the device on its instance")
	}
	if len(events) != 2 || events[0] != "wait-idle" || events[1] != "destroy" {
		t.Fatalf("HAL lifecycle events = %v, want [wait-idle destroy]", events)
	}
}
