//go:build !rust && !(js && wasm)

package wgpu

import (
	"errors"
	"fmt"
	"testing"

	"github.com/gogpu/wgpu/hal"
)

// TestErrContextReleasedIsDeviceLost checks that the error RequestDevice
// returns for a GLES adapter whose context is gone matches both the precise
// sentinel and ErrDeviceLost, through the wrapping RequestDevice adds: WebGPU's
// requestDevice on an expired adapter yields a lost device.
func TestErrContextReleasedIsDeviceLost(t *testing.T) {
	if !errors.Is(ErrContextReleased, ErrDeviceLost) {
		t.Fatal("ErrContextReleased does not wrap ErrDeviceLost")
	}
	// What gles' Adapter.Open returns, as RequestDevice wraps it
	// (adapter_native.go).
	open := fmt.Errorf("%w: request an adapter with a live surface", hal.ErrContextReleased)
	err := fmt.Errorf("wgpu: failed to open device: %w", open)
	if !errors.Is(err, ErrContextReleased) {
		t.Errorf("errors.Is(%v, ErrContextReleased) = false", err)
	}
	if !errors.Is(err, ErrDeviceLost) {
		t.Errorf("errors.Is(%v, ErrDeviceLost) = false", err)
	}
	if errors.Is(ErrDeviceLost, ErrContextReleased) {
		t.Error("ErrDeviceLost matches ErrContextReleased; only the reverse may hold")
	}
}
