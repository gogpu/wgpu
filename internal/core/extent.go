package core

import "github.com/gogpu/gputypes"

// IsEmptyExtent reports a copy extent with a zero dimension.
//
// Rust ignores such a copy after its validation and before touching the
// tracker (wgpu-core command/transfer.rs "Ignoring copy_* of size 0").
// The helper lives in core because both the encoder and the queue drop
// zero-size regions and neither may import the other (ADR-069).
func IsEmptyExtent(size gputypes.Extent3D) bool {
	return size.Width == 0 || size.Height == 0 || size.DepthOrArrayLayers == 0
}
