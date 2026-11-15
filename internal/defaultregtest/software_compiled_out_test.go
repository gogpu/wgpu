//go:build ((windows && (386 || amd64 || arm64)) || (((linux && !android) || darwin) && (amd64 || arm64)) || (android && arm64)) && !rust && !(js && wasm) && wgpu_nosoftware

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package defaultregtest

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// TestSoftwareMaskWithoutSoftwareReportsNoBackendRegistered closes ADR-083
// §2.2's last row on the public path: in a build with the software backend
// compiled out, a BackendsSoftware mask selects no registered backend. This
// package imports nothing else in the module (its own doc states the contract),
// so no other test can register a software provider into this binary.
func TestSoftwareMaskWithoutSoftwareReportsNoBackendRegistered(t *testing.T) {
	inst, err := wgpu.CreateInstance(&wgpu.InstanceDescriptor{Backends: gputypes.BackendsSoftware})
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	defer inst.Release()

	_, err = inst.RequestAdapter(nil)
	if !errors.Is(err, wgpu.ErrNoBackendRegistered) {
		t.Fatalf("RequestAdapter error = %v, want ErrNoBackendRegistered", err)
	}
	if errors.Is(err, wgpu.ErrNoHardwareAdapter) {
		t.Errorf("RequestAdapter error = %v, must not match ErrNoHardwareAdapter", err)
	}
}
