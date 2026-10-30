// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !android && !(js && wasm) && !rust

package allbackends_test

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// vvlCapture is a slog.Handler that keeps the Vulkan validation errors the
// HAL's debug messenger reports (hal/vulkan/debug.go logs them at
// slog.LevelError with a "vulkan: " prefix).
type vvlCapture struct {
	mu   sync.Mutex
	msgs []string
}

func (c *vvlCapture) Enabled(_ context.Context, level slog.Level) bool {
	return level >= slog.LevelError
}

func (c *vvlCapture) Handle(_ context.Context, r slog.Record) error { //nolint:gocritic // hugeParam: slog.Handler takes the Record by value
	if !strings.HasPrefix(r.Message, "vulkan: ") {
		return nil
	}
	id := ""
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == "id" {
			id = a.Value.String()
		}
		return true
	})
	c.mu.Lock()
	c.msgs = append(c.msgs, id+" "+r.Message)
	c.mu.Unlock()
	return nil
}

func (c *vvlCapture) WithAttrs([]slog.Attr) slog.Handler { return c }
func (c *vvlCapture) WithGroup(string) slog.Handler      { return c }

// enableVulkanValidation returns the instance flags that turn on the Khronos
// validation layer for a Vulkan test device, with synchronization validation,
// and fails the test for every validation error reported until the test's
// cleanups have run (the device is released first, so destruction errors
// count too). P1-VK-010: the round-trip test used to pass with three VVL
// errors, because nothing looked at them.
//
// The layer is loaded only where the Vulkan SDK is installed; elsewhere the
// HAL skips it silently and the test runs without these checks. This is a
// local check: CI has no Vulkan device.
func enableVulkanValidation(t *testing.T) gputypes.InstanceFlags {
	t.Helper()
	// Read by the layer at instance creation (VkLayer_khronos_validation.json
	// setting "validate_sync").
	t.Setenv("VK_KHRONOS_VALIDATION_VALIDATE_SYNC", "1")
	capture := &vvlCapture{}
	prev := hal.Logger()
	hal.SetLogger(slog.New(capture))
	t.Cleanup(func() {
		hal.SetLogger(prev)
		capture.mu.Lock()
		defer capture.mu.Unlock()
		for _, m := range capture.msgs {
			t.Errorf("Vulkan validation error: %s", m)
		}
	})
	return gputypes.InstanceFlagsDebug
}
