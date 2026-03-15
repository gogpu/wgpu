package wgpu

import (
	"log/slog"

	"github.com/gogpu/wgpu/hal"
)

// SetLogger configures the logger for the entire wgpu stack (public API,
// core validation layer, and HAL backends: Vulkan, Metal, DX12, GLES).
//
// By default, wgpu produces no log output. Call SetLogger to enable logging
// for deep debugging across the full GPU pipeline.
//
// SetLogger is safe for concurrent use.
// Pass nil to disable logging (restore default silent behavior).
func SetLogger(l *slog.Logger) {
	hal.SetLogger(l)
}

// Logger returns the current logger used by the wgpu stack.
func Logger() *slog.Logger {
	return hal.Logger()
}
