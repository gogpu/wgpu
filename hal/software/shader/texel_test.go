// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package shader

import (
	"bytes"
	"encoding/binary"
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

// le32 is v in little-endian byte order, a packed texel as stored.
func le32(v uint32) []byte { return binary.LittleEndian.AppendUint32(nil, v) }

// TestReadTexelByFormat: a sampled read (readTexel, which textureLoad and
// textureSample use) decodes the texel by its format, as storage reads do.
// It decoded by byte count alone: 2 bytes as two 8-bit unorm channels, so a
// Depth16Unorm 0x3000 read 0 and an R16Float 1.5 read (0, 62/255); 4 bytes as
// RGBA8, so a Depth32Float or R32Float read its float's bytes.
func TestReadTexelByFormat(t *testing.T) {
	f32 := func(f float32) []byte { return le32(math.Float32bits(f)) }
	tests := []struct {
		format gputypes.TextureFormat
		texel  []byte
		want   Vec4
	}{
		{gputypes.TextureFormatR8Unorm, []byte{51}, Vec4{0.2, 0, 0, 1}},
		{gputypes.TextureFormatR8Uint, []byte{200}, Vec4{200, 0, 0, 1}},
		{gputypes.TextureFormatR8Sint, []byte{0xFF}, Vec4{-1, 0, 0, 1}},
		{gputypes.TextureFormatRG8Unorm, []byte{0, 255}, Vec4{0, 1, 0, 1}},
		{gputypes.TextureFormatRGBA8Unorm, []byte{255, 0, 51, 255}, Vec4{1, 0, 0.2, 1}},
		{gputypes.TextureFormatRGBA8Snorm, []byte{0x81, 0x7F, 0, 0x7F}, Vec4{-1, 1, 0, 1}},
		{gputypes.TextureFormatRGBA8Uint, []byte{1, 2, 3, 4}, Vec4{1, 2, 3, 4}},
		{gputypes.TextureFormatBGRA8Unorm, []byte{51, 0, 255, 255}, Vec4{1, 0, 0.2, 1}},
		// The sRGB formats read their stored bytes, without the sRGB-to-linear
		// decode: P1-SW-021 adds it with the encode on writes.
		{gputypes.TextureFormatRGBA8UnormSrgb, []byte{255, 0, 51, 255}, Vec4{1, 0, 0.2, 1}},
		{gputypes.TextureFormatBGRA8UnormSrgb, []byte{51, 0, 255, 255}, Vec4{1, 0, 0.2, 1}},
		{gputypes.TextureFormatR16Unorm, []byte{0xFF, 0xFF}, Vec4{1, 0, 0, 1}},
		{gputypes.TextureFormatR16Uint, []byte{0xFF, 0xFF}, Vec4{65535, 0, 0, 1}},
		{gputypes.TextureFormatR16Sint, []byte{0xFE, 0xFF}, Vec4{-2, 0, 0, 1}},
		{gputypes.TextureFormatR16Float, []byte{0x00, 0x3E}, Vec4{1.5, 0, 0, 1}},
		{gputypes.TextureFormatRG16Unorm, []byte{0, 0, 0xFF, 0xFF}, Vec4{0, 1, 0, 1}},
		{gputypes.TextureFormatRGBA16Unorm, []byte{0xFF, 0xFF, 0, 0, 0, 0, 0xFF, 0xFF}, Vec4{1, 0, 0, 1}},
		{gputypes.TextureFormatRGBA16Float, []byte{0x00, 0x3C, 0x00, 0xB8, 0x00, 0x40, 0x00, 0x3C}, Vec4{1, -0.5, 2, 1}},
		{gputypes.TextureFormatRGBA16Sint, []byte{0xFF, 0xFF, 2, 0, 0, 0x80, 0xFF, 0x7F}, Vec4{-1, 2, -32768, 32767}},
		{gputypes.TextureFormatR32Float, f32(1.5), Vec4{1.5, 0, 0, 1}},
		{gputypes.TextureFormatR32Uint, le32(70000), Vec4{70000, 0, 0, 1}},
		{gputypes.TextureFormatR32Sint, le32(0xFFFFFFFE), Vec4{-2, 0, 0, 1}},
		{gputypes.TextureFormatRG32Float, append(f32(0.5), f32(-3)...), Vec4{0.5, -3, 0, 1}},
		{gputypes.TextureFormatRGBA32Float, append(append(f32(1), f32(2)...), append(f32(3), f32(4)...)...), Vec4{1, 2, 3, 4}},
		{gputypes.TextureFormatRGBA32Uint, append(append(le32(1), le32(2)...), append(le32(3), le32(4)...)...), Vec4{1, 2, 3, 4}},
		// Packed: r in the low bits.
		{gputypes.TextureFormatRGB10A2Unorm, le32(1023 | 0<<10 | 341<<20 | 3<<30), Vec4{1, 0, 341.0 / 1023, 1}},
		{gputypes.TextureFormatRGB10A2Uint, le32(1023 | 7<<10 | 512<<20 | 2<<30), Vec4{1023, 7, 512, 2}},
		// 11-bit floats 1.0 (exponent 15) and 2.0, 10-bit float 0.5.
		{gputypes.TextureFormatRG11B10Ufloat, le32(15<<6 | (16<<6)<<11 | (14<<5)<<22), Vec4{1, 2, 0.5, 1}},
		// Shared exponent 16: mantissas scale by 2^(16-15-9).
		{gputypes.TextureFormatRGB9E5Ufloat, le32(256 | 128<<9 | 0<<18 | 16<<27), Vec4{1, 0.5, 0, 1}},
		// Depth reads as (d, 0, 0, 1). The software backend stores Depth24Plus
		// and the depth of the combined formats as float32, 4 bytes a texel.
		{gputypes.TextureFormatDepth16Unorm, []byte{0x00, 0x30}, Vec4{float32(0x3000) / 65535, 0, 0, 1}},
		{gputypes.TextureFormatDepth32Float, f32(0.75), Vec4{0.75, 0, 0, 1}},
		{gputypes.TextureFormatDepth24Plus, f32(0.25), Vec4{0.25, 0, 0, 1}},
		{gputypes.TextureFormatDepth24PlusStencil8, f32(0.5), Vec4{0.5, 0, 0, 1}},
		{gputypes.TextureFormatDepth32FloatStencil8, f32(0.125), Vec4{0.125, 0, 0, 1}},
		{gputypes.TextureFormatStencil8, []byte{7}, Vec4{7, 0, 0, 1}},
	}
	for _, tt := range tests {
		t.Run(tt.format.String(), func(t *testing.T) {
			// Texel 0 is zero and texel 1 holds the value, so a decode of the
			// wrong texel size reads across the two.
			n := len(tt.texel)
			tex := &Texture2D{Width: 2, Height: 1, Format: uint32(tt.format), BytesPerPixel: uint32(n),
				Data: append(make([]byte, n), tt.texel...)}
			if got := readTexel(tex, 1, 0); !vec4Near(got, tt.want) {
				t.Errorf("readTexel = %v, want %v", got, tt.want)
			}
		})
	}
}

// vec4Near reports whether a and b agree to within float32 rounding of the
// decodes.
func vec4Near(a, b Vec4) bool {
	for i := range a {
		if d := a[i] - b[i]; d > 1e-6 || d < -1e-6 {
			return false
		}
	}
	return true
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
	if xyz := imageCoord3(id); xyz != [3]int{3, 5, 7} {
		t.Errorf("imageCoord3 = %v, want [3 5 7]", xyz)
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
