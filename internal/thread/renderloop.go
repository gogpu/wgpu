// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package thread

import (
	"sync/atomic"
)

// RenderLoop manages the separation between UI and render threads.
// Based on Ebiten's architecture for professional responsiveness.
//
// Key pattern: All GPU operations (including vkDeviceWaitIdle) happen
// on the render thread, never blocking the UI thread.
type RenderLoop struct {
	renderThread *Thread

	// Pending resize (set from UI thread, applied on render thread)
	pendingWidth  atomic.Uint32
	pendingHeight atomic.Uint32
	resizePending atomic.Bool

	// Frame synchronization
	frameReady   chan struct{}
	frameDone    chan struct{}
	renderPaused atomic.Bool
}

// NewRenderLoop creates a new render loop with a dedicated render thread.
func NewRenderLoop() *RenderLoop {
	return &RenderLoop{
		renderThread: New(),
		frameReady:   make(chan struct{}, 1),
		frameDone:    make(chan struct{}, 1),
	}
}

// Stop stops the render loop and its thread.
func (rl *RenderLoop) Stop() {
	rl.renderThread.Stop()
}

// RequestResize queues a resize to be applied on the render thread.
// This is called from the UI thread (WM_SIZE handler).
// The actual swapchain recreation happens in ApplyPendingResize.
func (rl *RenderLoop) RequestResize(width, height uint32) {
	if width == 0 || height == 0 {
		return
	}

	rl.pendingWidth.Store(width)
	rl.pendingHeight.Store(height)
	rl.resizePending.Store(true)
}

// HasPendingResize returns true if a resize is pending.
func (rl *RenderLoop) HasPendingResize() bool {
	return rl.resizePending.Load()
}

// ConsumePendingResize returns the pending resize dimensions and clears the flag.
// Returns (0, 0, false) if no resize is pending.
func (rl *RenderLoop) ConsumePendingResize() (width, height uint32, ok bool) {
	if !rl.resizePending.Swap(false) {
		return 0, 0, false
	}
	return rl.pendingWidth.Load(), rl.pendingHeight.Load(), true
}

// RunOnRenderThread executes f on the render thread and waits for completion.
// Use for GPU operations that need synchronous results.
func (rl *RenderLoop) RunOnRenderThread(f func() any) any {
	return rl.renderThread.Call(f)
}

// RunOnRenderThreadVoid executes f on the render thread and waits for completion.
// Use for GPU operations without return values.
func (rl *RenderLoop) RunOnRenderThreadVoid(f func()) {
	rl.renderThread.CallVoid(f)
}

// RunOnRenderThreadAsync executes f on the render thread without waiting.
// Use for fire-and-forget GPU operations.
func (rl *RenderLoop) RunOnRenderThreadAsync(f func()) {
	rl.renderThread.CallAsync(f)
}

// PauseRendering pauses render operations (during modal resize).
func (rl *RenderLoop) PauseRendering() {
	rl.renderPaused.Store(true)
}

// ResumeRendering resumes render operations.
func (rl *RenderLoop) ResumeRendering() {
	rl.renderPaused.Store(false)
}

// IsRenderingPaused returns true if rendering is paused.
func (rl *RenderLoop) IsRenderingPaused() bool {
	return rl.renderPaused.Load()
}
