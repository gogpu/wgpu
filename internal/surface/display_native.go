//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package surface

import "github.com/gogpu/wgpu/internal/hal"

// HAL converts the display mirror to its HAL form at the root boundary; no
// HAL type appears in the public API. The file carries the native build
// constraint because internal/hal does not compile on the js/wasm or rust
// builds; the rest of the display rules are context-independent (display.go).
func (h DisplayHandle) HAL() hal.DisplayHandle {
	switch h.Kind {
	case DisplayKindXlib:
		return hal.DisplayHandle{Kind: hal.DisplayKindXlib, Display: h.Ptr}
	case DisplayKindWayland:
		return hal.DisplayHandle{Kind: hal.DisplayKindWayland, Display: h.Ptr}
	default:
		return hal.DisplayHandle{}
	}
}
