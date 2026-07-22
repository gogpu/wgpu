// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows && !(js && wasm)

package dx12

import "testing"

func TestWaitForGPUAfterDeviceCleanupIsNoop(t *testing.T) {
	if err := (&Device{}).waitForGPU(); err != nil {
		t.Fatalf("waitForGPU on cleaned device: %v", err)
	}
}
