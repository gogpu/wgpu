// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package shader

import (
	"bytes"
	"math"
	"testing"

	"github.com/gogpu/gputypes"
)

// TestEncodeDecodeTexel checks the storage texel encoding of each channel
// kind against the WebGPU conversion rules (unorm and snorm clamp and round,
// integers convert, half floats round-trip), and that decoding returns the
// stored value with the missing channels as (0, 0, 0, 1).
func TestEncodeDecodeTexel(t *testing.T) {
	tests := []struct {
		name   string
		format gputypes.TextureFormat
		in     Vec4
		bytes  []byte
		out    Vec4
	}{
		{"rgba8unorm clamps and rounds", gputypes.TextureFormatRGBA8Unorm, Vec4{0.25, 1.5, -1, 1},
			[]byte{64, 255, 0, 255}, Vec4{64.0 / 255, 1, 0, 1}},
		{"bgra8unorm swaps red and blue", gputypes.TextureFormatBGRA8Unorm, Vec4{1, 0, 0, 1},
			[]byte{0, 0, 255, 255}, Vec4{1, 0, 0, 1}},
		{"rgba8snorm", gputypes.TextureFormatRGBA8Snorm, Vec4{-1, 1, 0.5, -2},
			[]byte{0x81, 0x7F, 64, 0x81}, Vec4{-1, 1, 64.0 / 127, -1}},
		{"rgba8uint", gputypes.TextureFormatRGBA8Uint, Vec4{1, 2, 200, 255},
			[]byte{1, 2, 200, 255}, Vec4{1, 2, 200, 255}},
		{"rgba8sint", gputypes.TextureFormatRGBA8Sint, Vec4{-1, 2, -128, 127},
			[]byte{0xFF, 2, 0x80, 0x7F}, Vec4{-1, 2, -128, 127}},
		{"r32float", gputypes.TextureFormatR32Float, Vec4{1.5, 9, 9, 9},
			[]byte{0, 0, 0xC0, 0x3F}, Vec4{1.5, 0, 0, 1}},
		{"r32sint", gputypes.TextureFormatR32Sint, Vec4{-2, 0, 0, 0},
			[]byte{0xFE, 0xFF, 0xFF, 0xFF}, Vec4{-2, 0, 0, 1}},
		{"rg16float", gputypes.TextureFormatRG16Float, Vec4{1, -0.5, 0, 0},
			[]byte{0x00, 0x3C, 0x00, 0xB8}, Vec4{1, -0.5, 0, 1}},
		{"rg16uint", gputypes.TextureFormatRG16Uint, Vec4{65535, 7, 0, 0},
			[]byte{0xFF, 0xFF, 7, 0}, Vec4{65535, 7, 0, 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tex := &Texture2D{Width: 2, Height: 1, Format: uint32(tt.format), Data: make([]byte, 2*len(tt.bytes))}
			if !encodeTexel(tex, 1, 0, tt.in) {
				t.Fatal("encodeTexel refused an in-bounds texel")
			}
			if got := tex.Data[len(tt.bytes):]; !bytes.Equal(got, tt.bytes) {
				t.Errorf("stored %v, want %v", got, tt.bytes)
			}
			if got := tex.Data[:len(tt.bytes)]; !bytes.Equal(got, make([]byte, len(tt.bytes))) {
				t.Errorf("texel 0 changed to %v", got)
			}
			got, ok := decodeTexel(tex, 1, 0)
			if !ok || got != tt.out {
				t.Errorf("decodeTexel = %v, %v, want %v", got, ok, tt.out)
			}
		})
	}
}

// TestVectorShuffleOfIntegerArray checks that a shuffle reads the components
// of an integer vector held as an array of scalars, as global_invocation_id
// is: id.xy read (0, 0) before, so every invocation stored to texel (0, 0).
func TestVectorShuffleOfIntegerArray(t *testing.T) {
	m := &Module{Types: map[uint32]*TypeInfo{1: {Kind: TypeVector, Components: 2}}}
	id := ValArray([]Value{ValUint(3), ValUint(5), ValUint(7)})
	got := vectorShuffle(m, 1, id, id, []uint32{0, 1})
	if got.Tag != TagVec2 || got.F[0] != 3 || got.F[1] != 5 {
		t.Errorf("shuffle = %v %v, want vec2(3, 5)", got.Tag, got.F)
	}
	if x, y := imageCoord(id); x != 3 || y != 5 {
		t.Errorf("imageCoord = (%d, %d), want (3, 5)", x, y)
	}
}

// TestEncodeTexelOutOfBounds checks that an out-of-bounds store is discarded
// and an unsupported format is refused.
func TestEncodeTexelOutOfBounds(t *testing.T) {
	tex := &Texture2D{Width: 1, Height: 1, Format: uint32(gputypes.TextureFormatRGBA8Unorm), Data: make([]byte, 4)}
	for _, xy := range [][2]int{{1, 0}, {0, 1}, {-1, 0}} {
		if encodeTexel(tex, xy[0], xy[1], Vec4{1, 1, 1, 1}) {
			t.Errorf("encodeTexel(%d, %d) wrote out of bounds", xy[0], xy[1])
		}
	}
	if !bytes.Equal(tex.Data, make([]byte, 4)) {
		t.Errorf("data changed to %v", tex.Data)
	}
	packed := &Texture2D{Width: 1, Height: 1, Format: uint32(gputypes.TextureFormatRGB10A2Unorm), Data: make([]byte, 4)}
	if encodeTexel(packed, 0, 0, Vec4{1, 1, 1, 1}) {
		t.Error("encodeTexel wrote a packed format it has no layout for")
	}
}

// TestEncodeTexelNaN checks that a NaN stored to a normalized format becomes
// 0 (D3D11 §3.2.3.6/§3.2.3.7), not the low end of the range: for snorm that
// was -1 (0x81).
func TestEncodeTexelNaN(t *testing.T) {
	nan := float32(math.NaN())
	for _, tt := range []struct {
		format gputypes.TextureFormat
		size   int
	}{
		{gputypes.TextureFormatRGBA8Unorm, 4},
		{gputypes.TextureFormatRGBA8Snorm, 4},
		{gputypes.TextureFormatRGBA16Unorm, 8},
	} {
		tex := &Texture2D{Width: 1, Height: 1, Format: uint32(tt.format), Data: bytes.Repeat([]byte{0x5A}, tt.size)}
		if !encodeTexel(tex, 0, 0, Vec4{nan, nan, nan, nan}) {
			t.Fatalf("%v: encodeTexel refused an in-bounds texel", tt.format)
		}
		if !bytes.Equal(tex.Data, make([]byte, tt.size)) {
			t.Errorf("%v: NaN stored as %v, want zeros", tt.format, tex.Data)
		}
	}
}
