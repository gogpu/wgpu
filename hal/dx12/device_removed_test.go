// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows && !(js && wasm)

package dx12

import (
	"errors"
	"strings"
	"testing"

	"github.com/gogpu/wgpu/hal"
)

// TestDeviceRemovedErrorWrapsBoth checks the error built on device removal:
// errors.Is matches the backend reason and hal.ErrDeviceLost, and the text
// stays one line (errors.Join would insert a newline).
func TestDeviceRemovedErrorWrapsBoth(t *testing.T) {
	reason := errors.New("DXGI_ERROR_DEVICE_REMOVED (0x887A0005)")
	err := deviceRemovedError(42, "CreateRenderPipeline(Grid)", reason)

	if !errors.Is(err, hal.ErrDeviceLost) {
		t.Errorf("errors.Is(err, hal.ErrDeviceLost) = false; err = %v", err)
	}
	if !errors.Is(err, reason) {
		t.Errorf("errors.Is(err, reason) = false; err = %v", err)
	}
	if !strings.Contains(err.Error(), "CreateRenderPipeline(Grid)") {
		t.Errorf("error does not name the operation: %v", err)
	}
	if strings.Contains(err.Error(), "\n") {
		t.Errorf("error text spans lines: %q", err.Error())
	}
}
