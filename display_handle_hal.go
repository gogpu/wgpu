//go:build !rust && !(js && wasm)

package wgpu

import "github.com/gogpu/wgpu/internal/hal"

// halDisplay converts the public handle to its HAL mirror at the root
// boundary; no HAL type appears in the public API.
//
// It is a method of DisplayHandle, a type that stays in the root package, so it
// lives in a staying file rather than in the instance files the S12 Move
// empties: a method cannot stay behind its type, and the type is not moving.
// The file carries the native build constraint because the conversion names
// internal/hal, which the js/wasm build does not compile; the handle helpers
// every build shares stay in display_handle.go.
func (h DisplayHandle) halDisplay() hal.DisplayHandle {
	switch h.kind {
	case displayKindXlib:
		return hal.DisplayHandle{Kind: hal.DisplayKindXlib, Display: h.ptr}
	case displayKindWayland:
		return hal.DisplayHandle{Kind: hal.DisplayKindWayland, Display: h.ptr}
	default:
		return hal.DisplayHandle{}
	}
}
