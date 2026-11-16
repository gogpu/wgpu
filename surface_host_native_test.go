//go:build !rust && !(js && wasm)

package wgpu

import "testing"

// TestDeviceHALQueueIsUntypedNil pins the surface seam's untyped-nil contract
// on the device implementation, which the surface now receives directly: a
// device without a queue gives an untyped nil from HALQueue. The destination's
// "queue not available" checks depend on it.
func TestDeviceHALQueueIsUntypedNil(t *testing.T) {
	device := NewBareDeviceForTest()
	if q := device.impl.HALQueue(); q != nil {
		t.Fatalf("a device without a queue must give an untyped nil, got %T", q)
	}
}
