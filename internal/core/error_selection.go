//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package core

import (
	"errors"
	"fmt"
)

// Adapter-selection sentinel errors (ADR-083 §2.2). The root package binds
// these values so callers of the public API can match them with errors.Is;
// every selection failure below returns exactly one of them.
var (
	// ErrNoBackendRegistered is returned when the requested backend mask selects
	// no registered backend: the backend is compiled out of the build or
	// excluded from the mask.
	ErrNoBackendRegistered = errors.New("wgpu: no backend registered for the requested mask (compiled out or excluded)")

	// ErrNoHardwareAdapter is returned when backends are registered for the
	// requested mask but none of them produced an adapter on this machine.
	ErrNoHardwareAdapter = errors.New("wgpu: no hardware adapter available")

	// ErrNoFallbackAdapter is returned when ForceFallbackAdapter was requested
	// and no CPU adapter is available, the software backend included.
	ErrNoFallbackAdapter = errors.New("wgpu: no fallback adapter available")
)

// noAdapterError maps an empty adapter candidate list to ADR-083 §2.2's
// sentinel. The mask is the deciding fact: a mask that admitted no registered
// backend means that backend is compiled out or excluded, while an admitted
// backend that produced nothing means this machine has no adapter for it.
// detail, when not empty, is appended to the ErrNoHardwareAdapter error.
func (i *Instance) noAdapterError(detail string) error {
	i.mu.RLock()
	maskSelected := i.maskSelectedBackend
	i.mu.RUnlock()

	if !maskSelected {
		return ErrNoBackendRegistered
	}
	if detail == "" {
		return ErrNoHardwareAdapter
	}
	return fmt.Errorf("%w: %s", ErrNoHardwareAdapter, detail)
}
