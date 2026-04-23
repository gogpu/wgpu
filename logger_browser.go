//go:build js && wasm

package wgpu

import "log/slog"

// SetLogger configures the logger for the wgpu stack.
func SetLogger(l *slog.Logger) {
	// Browser: logging not yet implemented
}

// Logger returns the current logger used by the wgpu stack.
func Logger() *slog.Logger {
	return nil
}
