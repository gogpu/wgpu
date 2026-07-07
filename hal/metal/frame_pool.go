// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin && !(js && wasm)

package metal

// RunInFramePool executes fn inside a fresh NSAutoreleasePool.
//
// Call once per rendered frame on the render thread. Metal's nextDrawable and
// present paths create autoreleased IOSurface objects; without per-frame pool
// draining on secondary threads, memory grows unbounded during live resize
// (imgui #2910, Apple MTLBestPracticesGuide Drawables).
func RunInFramePool(fn func()) {
	pool := NewAutoreleasePool()
	defer pool.Drain()
	fn()
}
