//go:build !rust && !(js && wasm)

package wgpu

import "testing"

// TestSurfaceDeviceHostQueueIsUntypedNil pins the surface seam's untyped-nil
// contract on the root adapter, the same property the encoder and queue seams
// already pin: a nil device gives an untyped nil seam value, and a device
// without a queue gives an untyped nil from HALQueue. The destination's "not
// configured" and "queue not available" checks depend on both.
func TestSurfaceDeviceHostQueueIsUntypedNil(t *testing.T) {
	if host := newSurfaceDeviceHost(nil); host != nil {
		t.Fatalf("a nil device must give an untyped nil seam value, got %T", host)
	}

	host := newSurfaceDeviceHost(&Device{})
	if q := host.HALQueue(); q != nil {
		t.Fatalf("a device without a queue must give an untyped nil, got %T", q)
	}
}
