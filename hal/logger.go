// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package hal

import (
	"context"
	"log/slog"
	"sync/atomic"
)

// nopHandler silently discards all log records.
// Enabled returns false so the caller skips message formatting entirely,
// making disabled logging effectively zero-cost.
type nopHandler struct{}

func (nopHandler) Enabled(context.Context, slog.Level) bool  { return false }
func (nopHandler) Handle(context.Context, slog.Record) error { return nil }
func (nopHandler) WithAttrs([]slog.Attr) slog.Handler        { return nopHandler{} }
func (nopHandler) WithGroup(string) slog.Handler             { return nopHandler{} }

// loggerPtr stores the active logger. Accessed atomically so that
// SetLogger can be called concurrently with logging from any goroutine.
var loggerPtr atomic.Pointer[slog.Logger]

func init() {
	l := slog.New(nopHandler{})
	loggerPtr.Store(l)
}

// SetLogger configures the logger for the wgpu HAL layer and all backends
// (Vulkan, DX12, GLES, Metal, Software).
// By default, wgpu produces no log output. Call SetLogger to enable logging.
//
// SetLogger is safe for concurrent use: it stores the new logger atomically.
// Pass nil to disable logging (restore default silent behavior).
//
// Log levels used by wgpu:
//   - [slog.LevelDebug]: internal diagnostics (buffer copies, texture uploads)
//   - [slog.LevelInfo]: important lifecycle events (debug layer attached)
//   - [slog.LevelWarn]: non-fatal issues (debug layer fallback, device errors)
//   - [slog.LevelError]: critical issues (device removed, validation errors)
//
// Example:
//
//	// Enable info-level logging to stderr:
//	hal.SetLogger(slog.Default())
//
//	// Enable debug-level logging for full diagnostics:
//	hal.SetLogger(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
//	    Level: slog.LevelDebug,
//	})))
func SetLogger(l *slog.Logger) {
	if l == nil {
		l = slog.New(nopHandler{})
	}
	loggerPtr.Store(l)
}

// Logger returns the current logger used by the wgpu HAL layer.
// Backend packages call this to share the same logger configuration
// without introducing import cycles.
//
// Logger is safe for concurrent use.
func Logger() *slog.Logger {
	return loggerPtr.Load()
}
