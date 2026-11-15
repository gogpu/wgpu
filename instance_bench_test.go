//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu_test

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// BenchmarkCreateInstanceDefault measures the startup cost of a default
// instance: the platform hardware backends of the build are initialised and
// their adapters enumerated.
func BenchmarkCreateInstanceDefault(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		inst, err := wgpu.CreateInstance(nil)
		if err != nil {
			b.Fatal(err)
		}
		inst.Release()
	}
}

// BenchmarkCreateInstanceAllHardware measures a CreateInstance over every
// hardware backend of the build, GL included — the default mask ADR-083 §2.1
// sets for v0.35.0. The difference against the default is the GLES
// enumeration's startup cost, recorded in ADR-083 for the lead's condition.
func BenchmarkCreateInstanceAllHardware(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		inst, err := wgpu.CreateInstance(&wgpu.InstanceDescriptor{Backends: gputypes.BackendsAll})
		if err != nil {
			b.Fatal(err)
		}
		inst.Release()
	}
}
