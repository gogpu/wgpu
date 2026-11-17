//go:build !rust && !(js && wasm)

package wgpu

import "testing"

// TestDeviceEncoderPoolIsUntypedNil pins the queue seam's nil contract: a
// device without an encoder pool must yield an untyped nil from EncoderPool,
// so the destination's nil checks keep their meaning across the interface. The
// contract is load-bearing: injectBarriers falls through to Acquire() on a
// non-nil value, and a typed nil boxed in the interface would defeat it.
//
// The seam value is the device implementation itself since S11 deleted the root
// adapter, so the contract is asserted on the implementation.
func TestDeviceEncoderPoolIsUntypedNil(t *testing.T) {
	device := NewBareDeviceForTest()
	if pool := device.impl.EncoderPool(); pool != nil {
		t.Fatalf("a device without a pool must give an untyped nil, got %T", pool)
	}
}
