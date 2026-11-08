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

// vvlCapture is a slog.Handler that keeps the Vulkan validation errors and
// warnings the HAL's debug messenger reports (hal/vulkan/debug.go logs them at
// slog.LevelError and slog.LevelWarn with a "vulkan: " prefix). Warnings count:
// the layer reports some spec violations as warnings, and drivers that are
// strict about them fail where ours do not (P2-VK-016,
// WARNING-CoreValidation-AllocateDescriptorSets-WrongType).
type vvlCapture struct {
	mu   sync.Mutex
	msgs []string
}

func (c *vvlCapture) Enabled(_ context.Context, level slog.Level) bool {
	return level >= slog.LevelWarn
}

func (c *vvlCapture) Handle(_ context.Context, r slog.Record) error { //nolint:gocritic // hugeParam: slog.Handler takes the Record by value
	if !strings.HasPrefix(r.Message, "vulkan: ") {
		return nil
	}
	// Only the debug messenger's records carry "type": the HAL's own warnings
	// share the "vulkan: " prefix (a failed pipeline cache save when test
	// processes write the shared cache file at once) and are not validation.
	id, messenger := "", false
	r.Attrs(func(a slog.Attr) bool {
		switch a.Key {
		case "id":
			id = a.Value.String()
		case "type":
			messenger = true
		}
		return true
	})
	if !messenger {
		return nil
	}
	c.mu.Lock()
	c.msgs = append(c.msgs, id+" "+r.Message)
	c.mu.Unlock()
	return nil
}

func (c *vvlCapture) WithAttrs([]slog.Attr) slog.Handler { return c }
func (c *vvlCapture) WithGroup(string) slog.Handler      { return c }

// drain returns the messages captured so far and forgets them, so a caller
// that attributes them to one step (a corpus case) reports each message once.
// A nil capture (a backend without the layer) has none.
func (c *vvlCapture) drain() []string {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	msgs := c.msgs
	c.msgs = nil
	return msgs
}

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
	flags, _ := startVulkanValidation(t)
	return flags
}

// startVulkanValidation is enableVulkanValidation that also returns the
// capture, for a caller that drains it per step; whatever is left when the
// test's cleanups run still fails the test.
func startVulkanValidation(t *testing.T) (gputypes.InstanceFlags, *vvlCapture) {
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
			t.Errorf("Vulkan validation message: %s", m)
		}
	})
	return gputypes.InstanceFlagsDebug, capture
}
