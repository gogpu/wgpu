//go:build js && wasm

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu_test

// The public zero-receiver contract of Instance and Adapter on the browser
// build (ADR-070 S12, commit 3), the PUBLIC row class for this context. It is
// executed by the wasm-test gate (the root suite under node), not only compiled.
//
// Categories, kept apart:
//
//   - RESTORED PRE-MOVE DEFAULT: DownlevelCapabilities on a nil receiver. The
//     pre-move method was a constant that ignored the receiver
//     (adapter_browser.go:47 at 17a8c4d); the declaration layer now delegates, so
//     its nil guard answers the same constant, and this is parity restored, not a
//     behavior choice.
//   - PARITY: the zero struct's static browser surface capabilities (the answer
//     does not depend on the adapter, and the pre-move method ignored it), the
//     HAL kinds, and the released Instance's ErrReleased (pinned with the
//     sentinel identity by TestBrowserInstanceReleasedAnswersErrReleased).
//   - SAFE-DIRECTION DELTA, carded as P2-CORE-034, NOT restored parity: two
//     zero-struct rows whose pre-move answer was a panic.
//     (1) A zero-value Adapter.RequestDevice dereferenced the nil
//     *browser.Adapter (adapter_browser.go:61 at 17a8c4d, reaching
//     internal/browser's adapter ref_); the declaration layer now answers
//     wgpu.ErrReleased before any js call, pinned below.
//     (2) A zero-value Instance.CreateSurfaceFromCanvas dereferenced the nil
//     *browser.Instance (surface_browser.go:142 at 17a8c4d: browser.NewSurface(i.browser.GPU(), …));
//     the instance side now nil-checks its implementation, contributes the zero
//     implementation's undefined GPU object, and leaves the answer to the canvas
//     glue. Pinned below with a canvas whose getContext answers null: the call
//     answers a defined error where the pre-move call panicked.
//   - DECLARATION-LAYER STATE: the released flag is the public zero-value
//     contract that cannot live in a nil implementation. Release is its single
//     writer; the rows below flip it only through Release and then read HAL.
//
// Recorded, not invoked: a nil Instance's RequestAdapter and Release and a nil
// Adapter's Release read the receiver and panicked pre-move and here. A
// zero-value Instance that was never released still panics through the browser
// implementation exactly as it panicked through the nil *browser.Instance
// pre-move (internal/browser's inst.gpu dereference): that row is parity of
// panics, not a delta. The two rows above are where the direction changed from a
// panic to a defined answer, and they are declared, carded, and not claimed as
// restored parity.

import (
	"errors"
	"syscall/js"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
	publichal "github.com/gogpu/wgpu/hal"
)

// mustStaticBrowserCapabilities asserts the browser's static capability answer,
// which depends on neither the adapter nor the surface when the surface is nil.
func mustStaticBrowserCapabilities(t *testing.T, adapter *wgpu.Adapter) {
	t.Helper()

	caps := adapter.GetSurfaceCapabilities(nil)
	if caps == nil {
		t.Fatal("GetSurfaceCapabilities(nil) = nil, want the static browser capabilities")
	}
	if len(caps.Formats) != 3 ||
		caps.Formats[0] != gputypes.TextureFormatRGBA8Unorm ||
		caps.Formats[1] != gputypes.TextureFormatBGRA8Unorm ||
		caps.Formats[2] != gputypes.TextureFormatRGBA16Float {
		t.Fatalf("Formats = %v, want rgba8unorm, bgra8unorm, rgba16float", caps.Formats)
	}
	if len(caps.PresentModes) != 1 || caps.PresentModes[0] != gputypes.PresentModeFifo {
		t.Fatalf("PresentModes = %v, want [Fifo]", caps.PresentModes)
	}
	if len(caps.AlphaModes) != 1 || caps.AlphaModes[0] != gputypes.CompositeAlphaModeOpaque {
		t.Fatalf("AlphaModes = %v, want [Opaque]", caps.AlphaModes)
	}
}

// TestPublicZeroValueBrowserAdapter pins the adapter's zero-value answers: the
// nil receiver's restored DownlevelCapabilities, the static capabilities, and
// the released HAL contract of the zero struct.
func TestPublicZeroValueBrowserAdapter(t *testing.T) {
	var nilAdapter *wgpu.Adapter
	if got := nilAdapter.DownlevelCapabilities(); got != gputypes.DefaultDownlevelCapabilities() {
		t.Fatalf("nil DownlevelCapabilities = %+v, want the default capabilities", got)
	}
	mustStaticBrowserCapabilities(t, nilAdapter)
	if got := nilAdapter.HAL(); got != (publichal.Handles{}) {
		t.Fatalf("nil adapter HAL = %+v, want the zero Handles", got)
	}

	zero := &wgpu.Adapter{}
	if got := zero.DownlevelCapabilities(); got != gputypes.DefaultDownlevelCapabilities() {
		t.Fatalf("zero DownlevelCapabilities = %+v, want the default capabilities", got)
	}
	mustStaticBrowserCapabilities(t, zero)
	if got := zero.HAL(); got.Kind != publichal.KindBrowser {
		t.Fatalf("zero adapter HAL = %+v, want KindBrowser", got)
	}
	zero.Release()
	if got := zero.HAL(); got != (publichal.Handles{}) {
		t.Fatalf("released zero adapter HAL = %+v, want the zero Handles", got)
	}
	zero.Release() // idempotent; Release is the flag's single writer
}

// TestPublicZeroValueBrowserRequestDevice pins the safe-direction delta (1):
// a zero-value adapter answers a defined error instead of dereferencing the nil
// browser adapter its pre-move implementation reached.
func TestPublicZeroValueBrowserRequestDevice(t *testing.T) {
	zero := &wgpu.Adapter{}
	if _, err := zero.RequestDevice(nil); !errors.Is(err, wgpu.ErrReleased) {
		t.Fatalf("zero RequestDevice = %v, want wgpu.ErrReleased (P2-CORE-034)", err)
	}
}

// TestPublicZeroValueBrowserCreateSurfaceFromCanvas pins the safe-direction
// delta (2): a zero-value Instance reaches surface creation through its nil
// implementation without dereferencing it, and the canvas glue's refusal is a
// defined error. Pre-move the call dereferenced the nil *browser.Instance and
// panicked before any canvas was asked.
func TestPublicZeroValueBrowserCreateSurfaceFromCanvas(t *testing.T) {
	zero := &wgpu.Instance{}

	// A canvas that behaves like an element without a WebGPU context: the glue
	// asks for one and gets null, the answer a browser without WebGPU gives.
	canvas := js.Global().Get("Object").New()
	ctx := js.FuncOf(func(js.Value, []js.Value) any { return js.Null() })
	defer ctx.Release()
	canvas.Set("getContext", ctx)

	if s, err := zero.CreateSurfaceFromCanvas(canvas); err == nil {
		t.Fatalf("zero CreateSurfaceFromCanvas = (%v, nil), want a defined error (P2-CORE-034)", s)
	}

	zero.Release()
	if _, err := zero.CreateSurfaceFromCanvas(canvas); !errors.Is(err, wgpu.ErrReleased) {
		t.Fatalf("CreateSurfaceFromCanvas after zero Release = %v, want wgpu.ErrReleased", err)
	}
}

// TestPublicZeroValueBrowserInstance pins the instance's zero-value answers: a
// nil instance reports nothing, and the zero struct reports the browser kind
// until its own Release flips the declaration-layer flag, which is the public
// zero-value contract internal/instance cannot hold.
func TestPublicZeroValueBrowserInstance(t *testing.T) {
	var nilInstance *wgpu.Instance
	if got := nilInstance.HAL(); got != (publichal.Handles{}) {
		t.Fatalf("HAL on a nil instance = %+v, want the zero Handles", got)
	}

	zero := &wgpu.Instance{}
	if got := zero.HAL(); got.Kind != publichal.KindBrowser {
		t.Fatalf("HAL on the zero instance = %+v, want KindBrowser", got)
	}
	zero.Release()
	if got := zero.HAL(); got != (publichal.Handles{}) {
		t.Fatalf("HAL after Release = %+v, want the zero Handles", got)
	}
	zero.Release() // idempotent

	if _, err := zero.RequestAdapter(nil); !errors.Is(err, wgpu.ErrReleased) {
		t.Fatalf("RequestAdapter on a released instance = %v, want ErrReleased", err)
	}
}
