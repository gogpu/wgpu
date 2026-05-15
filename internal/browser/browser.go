//go:build js && wasm

// Package browser implements the WebGPU backend for browsers using syscall/js.
//
// This package delegates all GPU operations to the browser's native WebGPU API
// (navigator.gpu) via JavaScript interop. It is only compiled on GOOS=js GOARCH=wasm.
//
// Architecture: each Go type (Instance, Adapter, Device, Queue) wraps a js.Value
// reference to the corresponding GPUInstance / GPUAdapter / GPUDevice / GPUQueue
// JavaScript object. Methods are pre-bound at construction time to avoid repeated
// property lookups on the hot path (Ebiten pattern).
//
// No core/ or hal/ packages are used — the browser validates GPU operations itself.
package browser

import "syscall/js"

// GPUAvailable reports whether the browser supports WebGPU.
// It checks for the existence of navigator.gpu.
func GPUAvailable() bool {
	navigator := js.Global().Get("navigator")
	if navigator.IsUndefined() || navigator.IsNull() {
		return false
	}
	gpu := navigator.Get("gpu")
	return !gpu.IsUndefined() && !gpu.IsNull()
}
