//go:build ((windows && (386 || amd64 || arm64)) || (((linux && !android) || darwin) && (amd64 || arm64)) || (android && arm64)) && !rust && !(js && wasm) && !wgpu_nosoftware

package defaultregtest

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/hal"
)

// TestSoftwareIsRegisteredOutsideTheDefaultMask pins ADR-083 §2.1/§2.3 in a
// binary that imports nothing but the root package: software is registered by
// default, reports its own variant, and stays out of the default set.
func TestSoftwareIsRegisteredOutsideTheDefaultMask(t *testing.T) {
	backend, ok := hal.GetBackend(gputypes.BackendSoftware)
	if !ok {
		t.Fatal("the software backend is not registered in a default binary (ADR-083 §2.3)")
	}
	if got := backend.Variant(); got != gputypes.BackendSoftware {
		t.Errorf("software backend variant = %v, want BackendSoftware", got)
	}
	if gputypes.DefaultInstanceDescriptor().Backends.Contains(gputypes.BackendSoftware) {
		t.Error("the default set contains software; it must stay explicit-only")
	}
}
