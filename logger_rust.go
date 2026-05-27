//go:build rust

package wgpu

import "log/slog"

// SetLogger configures the logger for the wgpu stack.
// On Rust backend, logging is handled by wgpu-native internally.
func SetLogger(_ *slog.Logger) {
	// Rust backend: wgpu-native has its own logging.
}

// Logger returns the current logger used by the wgpu stack.
// On Rust backend, returns nil.
func Logger() *slog.Logger {
	return nil
}
