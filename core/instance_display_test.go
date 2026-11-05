//go:build !(js && wasm)

package core

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// TestHALInstanceDescriptorCarriesDisplay checks that the instance display
// reaches every backend's CreateInstance.
func TestHALInstanceDescriptorCarriesDisplay(t *testing.T) {
	desc := gputypes.DefaultInstanceDescriptor()
	desc.Flags = gputypes.InstanceFlagsDebug
	display := hal.DisplayHandle{Kind: hal.DisplayKindWayland, Display: 0x1000}

	got := halInstanceDescriptor(&desc, display)
	if got.Backends != desc.Backends || got.Flags != desc.Flags {
		t.Fatalf("descriptor = %+v, lost backends or flags of %+v", got, desc)
	}
	if got.Display != display {
		t.Fatalf("Display = %+v, want %+v", got.Display, display)
	}

	if got := halInstanceDescriptor(&desc, hal.DisplayHandle{}); !got.Display.IsZero() {
		t.Fatalf("Display = %+v, want zero without an instance display", got.Display)
	}
}
