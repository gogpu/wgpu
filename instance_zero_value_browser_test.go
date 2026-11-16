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
//   - DECLARATION-LAYER STATE: the released flag is the public zero-value
//     contract that cannot live in a nil implementation. Release is its single
//     writer; the rows below flip it only through Release and then read HAL.
//
// Recorded, not invoked: a nil Instance's RequestAdapter and Release and a nil
// Adapter's Release read the receiver and panicked pre-move; a zero-value
// Instance that was never released reaches a nil implementation and panicked
// through the nil browser field pre-move. Each is the same class here, so no row
// invokes it.

import (
	"errors"
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
