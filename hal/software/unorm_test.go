//go:build !(js && wasm)

package software

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// TestTextureClearRoundsToUnorm checks the clear of an 8-bit unorm texture:
// the float clear value is rounded half up (D3D11 §3.2.3.6), as the GPUs do (truncation
// stored 0.5 as 127 and 100.75/255 as 100).
func TestTextureClearRoundsToUnorm(t *testing.T) {
	dev, _, cleanup := createSoftwareDevice(t)
	defer cleanup()
	for _, format := range []gputypes.TextureFormat{gputypes.TextureFormatRGBA8Unorm, gputypes.TextureFormatBGRA8Unorm} {
		tex, err := dev.CreateTexture(&hal.TextureDescriptor{
			Size:   hal.Extent3D{Width: 2, Height: 2, DepthOrArrayLayers: 1},
			Format: format,
		})
		if err != nil {
			t.Fatalf("CreateTexture: %v", err)
		}
		tex.(*Texture).Clear(gputypes.Color{R: 0.5, G: 100.75 / 255, B: 200.25 / 255, A: math.NaN()})
		want := [4]byte{128, 101, 200, 0} // R, G, B, A; NaN converts to 0
		if format == gputypes.TextureFormatBGRA8Unorm {
			want[0], want[2] = want[2], want[0]
		}
		if got := [4]byte(tex.(*Texture).data[:4]); got != want {
			t.Errorf("%v: cleared texel = %v, want %v", format, got, want)
		}
		dev.DestroyTexture(tex)
	}
}

// TestDepth16UnormWrite checks the Depth16Unorm store: clamp, scale by
// 65535, round half up (D3D11 §3.2.3.6), NaN to 0.
func TestDepth16UnormWrite(t *testing.T) {
	tex := &Texture{format: gputypes.TextureFormatDepth16Unorm, width: 1, height: 1, data: make([]byte, 2)}
	tests := []struct {
		depth float32
		want  uint16
	}{
		{0.5, 32768}, // 32767.5, the only tie: half up
		{float32(math.NaN()), 0},
		{-1, 0},
		{2, 65535},
		{1000.25 / 65535, 1000},
		{1000.75 / 65535, 1001},
	}
	for _, tt := range tests {
		writeDepthTexel(tex, 0, 0, tt.depth)
		if got := binary.LittleEndian.Uint16(tex.data); got != tt.want {
			t.Errorf("writeDepthTexel(%v) stored %d, want %d", tt.depth, got, tt.want)
		}
	}
	for k := 0; k <= 65535; k++ {
		writeDepthTexel(tex, 0, 0, float32(k)/65535)
		if got := binary.LittleEndian.Uint16(tex.data); int(got) != k {
			t.Fatalf("writeDepthTexel(%d/65535) stored %d", k, got)
		}
	}
}
