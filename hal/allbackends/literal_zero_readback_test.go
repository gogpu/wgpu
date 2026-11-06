// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"strings"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// TestColorWriteMaskZeroWritesNothing: a zero ColorTargetState.WriteMask is
// GPUColorWrite 0, a value the spec allows, so it is taken literally and the
// draw writes no channel. The spec's default (ALL) applies only to an absent
// member, which a Go struct cannot express.
func TestColorWriteMaskZeroWritesNothing(t *testing.T) {
	none := gputypes.ColorWriteMaskNone
	forEachOverrideBackend(t, 0, func(t *testing.T, device *wgpu.Device) {
		if strings.HasSuffix(t.Name(), "/software") {
			t.Skip("P1-SW-021: the software backend ignores the color write mask until its per-format write path lands")
		}
		drawZeroDefaults(t, device, zeroDefaultsCase{writeMask: &none, wantCleared: true})
	})
}
