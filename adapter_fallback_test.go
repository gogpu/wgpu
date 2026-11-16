//go:build ((windows && (386 || amd64 || arm64)) || (((linux && !android) || darwin) && (amd64 || arm64)) || (android && arm64)) && !rust && !(js && wasm) && !wgpu_nosoftware

package wgpu_test

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// TestForceFallbackAdapterSelectsSoftware pins ADR-083 §2.3 as implemented:
// the software backend is out of the default mask and Auto, and
// RequestAdapterOptions.ForceFallbackAdapter reaches it (ADR-079: software is
// preferred among CPU adapters). The build tag guarantees the software backend
// is in the binary, so — unlike a test that skips on ErrNoFallbackAdapter —
// this one fails on the pre-fix code instead of hiding it.
func TestForceFallbackAdapterSelectsSoftware(t *testing.T) {
	inst, err := wgpu.CreateInstance(nil)
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	defer inst.Release()

	adapter, err := inst.RequestAdapter(&wgpu.RequestAdapterOptions{ForceFallbackAdapter: true})
	if err != nil {
		t.Fatalf("force RequestAdapter: %v (ADR-083 §2.3: the force path reaches software; ErrNoFallbackAdapter here means the fallback candidacy regressed)", err)
	}
	defer adapter.Release()

	if got := adapter.Info().Backend; got != gputypes.BackendSoftware {
		t.Fatalf("ForceFallbackAdapter picked backend %v, want %v", got, gputypes.BackendSoftware)
	}
}
