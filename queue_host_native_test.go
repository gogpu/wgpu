//go:build !rust && !(js && wasm)

package wgpu

import "testing"

// TestQueueDeviceHostPoolIsUntypedNil pins the queue seam's nil contract, the
// same property TestEncoderDeviceHostPoolIsUntypedNil pins for the encoder:
// a device without an encoder pool must yield an untyped nil from EncoderPool,
// so the destination's nil checks keep their meaning across the interface. The
// contract is load-bearing: injectBarriers falls through to Acquire() on a
// non-nil value, and a typed nil boxed in the interface would defeat it.
func TestQueueDeviceHostPoolIsUntypedNil(t *testing.T) {
	host := newQueueDeviceHost(&Device{})
	if pool := host.EncoderPool(); pool != nil {
		t.Fatalf("a device without a pool must give an untyped nil, got %T", pool)
	}
}
