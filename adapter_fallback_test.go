//go:build !rust && !(js && wasm)

package wgpu_test

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// TestForceFallbackAdapterSelectsSoftware pins ADR-083 §2.3 as implemented:
// the software backend is out of the default mask and Auto, and
// RequestAdapterOptions.ForceFallbackAdapter reaches it (ADR-079: software is
// preferred among CPU adapters). The test is machine-independent: a machine
// with hardware still serves the force request from software.
func TestForceFallbackAdapterSelectsSoftware(t *testing.T) {
	inst, err := wgpu.CreateInstance(nil)
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	defer inst.Release()

	adapter, err := inst.RequestAdapter(&wgpu.RequestAdapterOptions{ForceFallbackAdapter: true})
	if err != nil {
		if errors.Is(err, wgpu.ErrNoFallbackAdapter) {
			// Built with -tags wgpu_nosoftware (or on a GOOS without the
			// software backend): the sentinel is the documented contract.
			t.Skipf("software backend is not in this build: %v", err)
		}
		t.Fatalf("force RequestAdapter: %v", err)
	}
	defer adapter.Release()

	if got := adapter.Info().Backend; got != gputypes.BackendSoftware {
		t.Fatalf("ForceFallbackAdapter picked backend %v, want %v", got, gputypes.BackendSoftware)
	}
}
