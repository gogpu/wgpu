// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build linux && !(js && wasm)

package gles

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"unsafe"

	"github.com/go-webgpu/goffi/ffi"
	"github.com/go-webgpu/goffi/types"
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// The Wayland lifetime tests (ADR-077, #379) need a Wayland compositor. They
// skip without one, and fail on a runner whose WGPU_REQUIRE lists wayland.
// Each case runs in a re-executed test binary, so a segfault in EGL teardown
// (the #379 crash) is a failed case with its output, not a dead test run.

// waylandLifetimeCaseEnv selects the case a re-executed child runs.
const waylandLifetimeCaseEnv = "WGPU_GLES_WAYLAND_LIFETIME_CASE"

func requireWayland(tb testing.TB, reason string) {
	tb.Helper()
	for _, item := range strings.Split(os.Getenv("WGPU_REQUIRE"), ",") {
		if strings.TrimSpace(item) == "wayland" {
			tb.Fatalf("%s, but WGPU_REQUIRE=%q requires Wayland", reason, os.Getenv("WGPU_REQUIRE"))
		}
	}
	tb.Skip(reason)
}

// waylandClient is the app side of a Wayland connection: libwayland-client
// loaded by the test, as an app loads it, separate from the EGL loader.
type waylandClient struct {
	connect, disconnect       unsafe.Pointer
	cifConnect, cifDisconnect types.CallInterface
}

func loadWaylandClient() (*waylandClient, error) {
	lib, err := ffi.LoadLibrary("libwayland-client.so.0")
	if err != nil {
		return nil, err
	}
	c := &waylandClient{}
	if c.connect, err = ffi.GetSymbol(lib, "wl_display_connect"); err != nil {
		return nil, err
	}
	if c.disconnect, err = ffi.GetSymbol(lib, "wl_display_disconnect"); err != nil {
		return nil, err
	}
	ptr := []*types.TypeDescriptor{types.PointerTypeDescriptor}
	if err := ffi.PrepareCallInterface(&c.cifConnect, types.DefaultCall, types.PointerTypeDescriptor, ptr); err != nil {
		return nil, err
	}
	if err := ffi.PrepareCallInterface(&c.cifDisconnect, types.DefaultCall, types.VoidTypeDescriptor, ptr); err != nil {
		return nil, err
	}
	return c, nil
}

// connectDisplay opens a connection to the compositor named by WAYLAND_DISPLAY.
func (c *waylandClient) connectDisplay() uintptr {
	var display uintptr
	var name uintptr // NULL: WAYLAND_DISPLAY
	args := [1]unsafe.Pointer{unsafe.Pointer(&name)}
	_, _ = ffi.CallFunction(&c.cifConnect, c.connect, unsafe.Pointer(&display), args[:])
	return display
}

func (c *waylandClient) disconnectDisplay(display uintptr) {
	args := [1]unsafe.Pointer{unsafe.Pointer(&display)}
	_, _ = ffi.CallFunction(&c.cifDisconnect, c.disconnect, nil, args[:])
}

// waylandCase is one shutdown order, run in a child process.
type waylandCase struct {
	name string
	run  func(t *testing.T, c *waylandClient)
}

var waylandLifetimeCases = []waylandCase{
	{"surface then disconnect then instance", waylandSurfaceDisconnectInstance},
	{"device then surface then disconnect", waylandDeviceSurfaceDisconnect},
	{"instance display", waylandInstanceDisplay},
	{"instance display rejects a foreign surface", waylandInstanceDisplayForeignSurface},
	{"reopen window", waylandReopenWindow},
	{"adapter never released", waylandAdapterNeverReleased},
	{"foreign device rejected", waylandForeignDeviceRejected},
}

func TestWaylandLifetime(t *testing.T) {
	if os.Getenv(waylandLifetimeCaseEnv) != "" {
		t.Skip("running as a child case")
	}
	if os.Getenv("WAYLAND_DISPLAY") == "" {
		requireWayland(t, "WAYLAND_DISPLAY is not set")
	}
	client, err := loadWaylandClient()
	if err != nil {
		requireWayland(t, "libwayland-client is not available: "+err.Error())
	}
	display := client.connectDisplay()
	if display == 0 {
		requireWayland(t, "no Wayland compositor answers on WAYLAND_DISPLAY")
	}
	client.disconnectDisplay(display)

	for _, c := range waylandLifetimeCases {
		t.Run(c.name, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestWaylandLifetimeChild$", "-test.v")
			cmd.Env = append(os.Environ(), waylandLifetimeCaseEnv+"="+c.name)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("case %q failed (%v):\n%s", c.name, err, out)
			}
			if !strings.Contains(string(out), "--- PASS: TestWaylandLifetimeChild") {
				t.Fatalf("case %q did not pass:\n%s", c.name, out)
			}
		})
	}
}

// TestWaylandLifetimeChild runs one case in the re-executed binary.
func TestWaylandLifetimeChild(t *testing.T) {
	name := os.Getenv(waylandLifetimeCaseEnv)
	if name == "" {
		t.Skip("only runs as a child of TestWaylandLifetime")
	}
	client, err := loadWaylandClient()
	if err != nil {
		t.Fatalf("load libwayland-client: %v", err)
	}
	for _, c := range waylandLifetimeCases {
		if c.name == name {
			c.run(t, client)
			return
		}
	}
	t.Fatalf("unknown case %q", name)
}

func mustConnect(t *testing.T, c *waylandClient) uintptr {
	t.Helper()
	display := c.connectDisplay()
	if display == 0 {
		t.Fatal("wl_display_connect failed")
	}
	return display
}

func mustInstance(t *testing.T, desc *hal.InstanceDescriptor) *Instance {
	t.Helper()
	inst, err := Backend{}.CreateInstance(desc)
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	return inst.(*Instance)
}

// mustWaylandSurface creates a Path B surface on the app's display. Path B
// needs only the display; the window is never configured.
func mustWaylandSurface(t *testing.T, inst *Instance, display uintptr) *Surface {
	t.Helper()
	surf, err := inst.CreateSurface(hal.SurfaceTarget{Kind: hal.SurfaceTargetWaylandSurface, DisplayHandle: display})
	if err != nil {
		t.Fatalf("CreateSurface: %v", err)
	}
	return surf.(*Surface)
}

func mustAdapter(t *testing.T, inst *Instance, surf *Surface) hal.Adapter {
	t.Helper()
	adapters := inst.EnumerateAdapters(surf)
	if len(adapters) == 0 {
		t.Fatal("no GLES adapter for the surface")
	}
	return adapters[0].Adapter
}

func mustOpen(t *testing.T, adapter hal.Adapter) hal.OpenDevice {
	t.Helper()
	open, err := adapter.Open(0, gputypes.DefaultLimits())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return open
}

// waylandSurfaceDisconnectInstance is gogpu's order and the #379 crash (Auto
// with another backend selected): the GLES surface and its adapter are
// created, the window closes, the app disconnects, then the instance goes.
func waylandSurfaceDisconnectInstance(t *testing.T, c *waylandClient) {
	display := mustConnect(t, c)
	inst := mustInstance(t, nil)
	surf := mustWaylandSurface(t, inst, display)
	adapter := mustAdapter(t, inst, surf)
	surf.Destroy()
	c.disconnectDisplay(display)
	adapter.Destroy()
	inst.Destroy()
}

// waylandDeviceSurfaceDisconnect is the same order with GLES selected: the
// device is released with the window, before the disconnect.
func waylandDeviceSurfaceDisconnect(t *testing.T, c *waylandClient) {
	display := mustConnect(t, c)
	inst := mustInstance(t, nil)
	surf := mustWaylandSurface(t, inst, display)
	open := mustOpen(t, mustAdapter(t, inst, surf))
	open.Device.Destroy()
	surf.Destroy()
	c.disconnectDisplay(display)
	inst.Destroy()
}

// waylandInstanceDisplay builds the context at instance creation on the
// display the caller passes, which then outlives the Instance (D).
func waylandInstanceDisplay(t *testing.T, c *waylandClient) {
	display := mustConnect(t, c)
	inst := mustInstance(t, &hal.InstanceDescriptor{
		Display: hal.DisplayHandle{Kind: hal.DisplayKindWayland, Display: display},
	})
	ctx := inst.context()
	if ctx == nil {
		t.Fatal("no instance context on the instance display")
	}
	surf := mustWaylandSurface(t, inst, display)
	if surf.ctx != ctx {
		t.Fatal("the surface did not share the context built on the instance display")
	}
	open := mustOpen(t, mustAdapter(t, inst, surf))
	open.Device.Destroy()
	surf.Destroy()
	if !ctx.Alive() {
		t.Fatal("the instance context died with a surface and device of it")
	}
	inst.Destroy()
	if ctx.Alive() {
		t.Fatal("the instance context outlived its Instance, surfaces and devices")
	}
	c.disconnectDisplay(display)
}

// waylandInstanceDisplayForeignSurface gives a surface of another
// connection its own context: the instance context serves its own display
// only (the root package rejects such a target before HAL sees it).
func waylandInstanceDisplayForeignSurface(t *testing.T, c *waylandClient) {
	display, other := mustConnect(t, c), mustConnect(t, c)
	inst := mustInstance(t, &hal.InstanceDescriptor{
		Display: hal.DisplayHandle{Kind: hal.DisplayKindWayland, Display: display},
	})
	surf := mustWaylandSurface(t, inst, other)
	if surf.ctx == inst.context() {
		t.Fatal("a surface of another wl_display shared the instance display's context")
	}
	surf.Destroy()
	inst.Destroy()
	c.disconnectDisplay(other)
	c.disconnectDisplay(display)
}

// waylandReopenWindow closes a window and opens a new one in the same
// Instance: the cached adapter of the first is requalified for the second,
// and the new Device works.
func waylandReopenWindow(t *testing.T, c *waylandClient) {
	display := mustConnect(t, c)
	inst := mustInstance(t, nil)

	first := mustWaylandSurface(t, inst, display)
	cached := mustAdapter(t, inst, first)
	open := mustOpen(t, cached)
	open.Device.Destroy()
	first.Destroy()
	if _, err := cached.Open(0, gputypes.DefaultLimits()); !errors.Is(err, ErrContextReleased) {
		t.Fatalf("Open on the closed window's adapter = %v, want ErrContextReleased", err)
	}

	second := mustWaylandSurface(t, inst, display)
	qualified, err := cached.(*Adapter).QualifySurface(second)
	if err != nil {
		t.Fatalf("QualifySurface for the reopened window: %v", err)
	}
	open = mustOpen(t, qualified)
	buf, err := open.Device.CreateBuffer(&hal.BufferDescriptor{Size: 64, Usage: gputypes.BufferUsageCopyDst})
	if err != nil {
		t.Fatalf("CreateBuffer on the reopened window's device: %v", err)
	}
	open.Device.DestroyBuffer(buf)
	open.Device.Destroy()
	second.Destroy()
	c.disconnectDisplay(display)
	inst.Destroy()
}

// waylandAdapterNeverReleased keeps an adapter the app never releases; the
// display closes before the Instance, and the process exits cleanly.
func waylandAdapterNeverReleased(t *testing.T, c *waylandClient) {
	display := mustConnect(t, c)
	inst := mustInstance(t, nil)
	surf := mustWaylandSurface(t, inst, display)
	_ = mustAdapter(t, inst, surf)
	surf.Destroy()
	c.disconnectDisplay(display)
	inst.Destroy()
}

// waylandForeignDeviceRejected configures a surface of one connection with a
// Device of another: the GLES backend refuses instead of libwayland aborting
// with "proxy and queue point to different wl_displays".
func waylandForeignDeviceRejected(t *testing.T, c *waylandClient) {
	first, second := mustConnect(t, c), mustConnect(t, c)
	inst := mustInstance(t, nil)
	surfA := mustWaylandSurface(t, inst, first)
	surfB := mustWaylandSurface(t, inst, second)
	if surfA.ctx == surfB.ctx {
		t.Fatal("surfaces of two connections share one context")
	}
	open := mustOpen(t, mustAdapter(t, inst, surfA))
	err := surfB.Configure(open.Device, &hal.SurfaceConfiguration{Width: 64, Height: 64})
	if !errors.Is(err, hal.ErrSurfaceDeviceMismatch) {
		t.Fatalf("Configure with a foreign Device = %v, want hal.ErrSurfaceDeviceMismatch", err)
	}
	open.Device.Destroy()
	surfA.Destroy()
	surfB.Destroy()
	c.disconnectDisplay(first)
	c.disconnectDisplay(second)
	inst.Destroy()
}
