//go:build !(js && wasm)

package raster

import "fmt"

// MultisampleState is how many samples a pixel takes and which of them a
// draw may cover, WebGPU's GPUMultisampleState without alpha to coverage.
//
// A pixel of one sample is covered when its center is. A pixel of 4 samples
// is covered at the standard sample positions of Vulkan and D3D, (0.375,
// 0.125), (0.875, 0.375), (0.125, 0.625) and (0.625, 0.875) inside it, each
// under the same top-left rule as a center; its fragment is still shaded
// once, at the pixel center, and Fragment.Coverage names the samples it
// writes.
type MultisampleState struct {
	// Count is the samples of a pixel, 1 or 4; 0 is 1, and a draw panics at
	// any other.
	Count int

	// Mask keeps the samples of its set bits, bit s for sample s. 0 keeps
	// every sample: the public API turns WebGPU's default of all ones into
	// that, so a state built without a mask draws.
	Mask uint32
}

// samplePositions4 are the 4 standard sample positions, each as its offset
// from the pixel center in fixed point: multiples of 1/16 pixel, exact at
// SubPixelBits.
var samplePositions4 = [4][2]int64{
	{-2 * subPixelOne / 16, -6 * subPixelOne / 16},
	{6 * subPixelOne / 16, -2 * subPixelOne / 16},
	{-6 * subPixelOne / 16, 2 * subPixelOne / 16},
	{2 * subPixelOne / 16, 6 * subPixelOne / 16},
}

// sampleCount answers the samples of a pixel, 1 or 4. It panics at any
// other Count but 0.
func (m MultisampleState) sampleCount() int {
	switch m.Count {
	case 0, 1:
		return 1
	case len(samplePositions4):
		return m.Count
	}
	panic(fmt.Sprintf("raster: a MultisampleState's Count is 0, 1 or 4, not %d", m.Count))
}

// sampleMask answers the samples a draw may cover: Mask's bits of the
// pixel's samples, all of them when Mask is 0.
func (m MultisampleState) sampleMask() uint8 {
	all := uint8(1)<<m.sampleCount() - 1
	if m.Mask == 0 {
		return all
	}
	return uint8(m.Mask) & all
}
