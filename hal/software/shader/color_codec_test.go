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

// srgbEncodeRef is the IEC 61966-2-1 encode of a linear value in [0, 1].
func srgbEncodeRef(l float64) float64 {
	if l <= 0.0031308 {
		return 12.92 * l
	}
	return 1.055*math.Pow(l, 1/2.4) - 0.055
}

// TestSRGBRoundTrip: every 8-bit sRGB code decodes to linear and encodes
// back to itself, so a copy through an sRGB view and a blend with a zero
// source keep the stored bytes.
func TestSRGBRoundTrip(t *testing.T) {
	for n := range 256 {
		if got := linearToSrgb8(srgbToLinear8[n]); int(got) != n {
			t.Errorf("code %d: decode %v encodes to %d", n, srgbToLinear8[n], got)
		}
	}
}

// TestLinearToSrgb8: the threshold search gives round(encode(l) * 255) of
// the reference curve, clamps outside [0, 1] and encodes NaN as 0.
func TestLinearToSrgb8(t *testing.T) {
	for i := 0; i <= 10000; i++ {
		l := float32(i) / 10000
		want := math.Round(srgbEncodeRef(float64(l)) * 255)
		got := float64(linearToSrgb8(l))
		// A value within float32 rounding of a halfway point may land on
		// either code.
		if got != want && math.Abs(srgbEncodeRef(float64(l))*255-math.Floor(srgbEncodeRef(float64(l))*255)-0.5) > 1e-4 {
			t.Fatalf("linearToSrgb8(%v) = %v, want %v", l, got, want)
		}
	}
	for _, tt := range []struct {
		in   float32
		want byte
	}{{-1, 0}, {2, 255}, {float32(math.NaN()), 0}, {float32(math.Inf(1)), 255}, {0.5, 188}, {0.214041, 127}, {0.2158605, 128}} {
		if got := linearToSrgb8(tt.in); got != tt.want {
			t.Errorf("linearToSrgb8(%v) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

// TestColorCodecEncode checks the colour-target encode of every renderable
// layout against the stored bytes, and that Decode reads them back.
func TestColorCodecEncode(t *testing.T) {
	f16 := func(f float32) []byte { return binary.LittleEndian.AppendUint16(nil, float32ToFloat16bits(f)) }
	f32 := func(f float32) []byte { return le32(math.Float32bits(f)) }
	tests := []struct {
		format gputypes.TextureFormat
		in     [4]float32
		want   []byte
		back   [4]float32
	}{
		{gputypes.TextureFormatRGBA8Unorm, [4]float32{0.5, 0, 1, 1}, []byte{128, 0, 255, 255}, [4]float32{128.0 / 255, 0, 1, 1}},
		{gputypes.TextureFormatRGBA8UnormSrgb, [4]float32{0.5, 0, 1, 0.5}, []byte{188, 0, 255, 128}, [4]float32{srgbToLinear8[188], 0, 1, 128.0 / 255}},
		{gputypes.TextureFormatBGRA8UnormSrgb, [4]float32{0.5, 0, 1, 1}, []byte{255, 0, 188, 255}, [4]float32{srgbToLinear8[188], 0, 1, 1}},
		{gputypes.TextureFormatR8Unorm, [4]float32{0.2, 9, 9, 9}, []byte{51}, [4]float32{0.2, 0, 0, 1}},
		{gputypes.TextureFormatR8Snorm, [4]float32{-1, 0, 0, 0}, []byte{0x81}, [4]float32{-1, 0, 0, 1}},
		{gputypes.TextureFormatR16Float, [4]float32{1.5, 0, 0, 0}, f16(1.5), [4]float32{1.5, 0, 0, 1}},
		{gputypes.TextureFormatRGBA16Float, [4]float32{2, -0.5, 4, 1}, append(append(f16(2), f16(-0.5)...), append(f16(4), f16(1)...)...), [4]float32{2, -0.5, 4, 1}},
		{gputypes.TextureFormatR32Float, [4]float32{3.25, 0, 0, 0}, f32(3.25), [4]float32{3.25, 0, 0, 1}},
		{gputypes.TextureFormatRG32Float, [4]float32{-7, 8, 0, 0}, append(f32(-7), f32(8)...), [4]float32{-7, 8, 0, 1}},
		{gputypes.TextureFormatR16Unorm, [4]float32{1, 0, 0, 0}, []byte{0xFF, 0xFF}, [4]float32{1, 0, 0, 1}},
		{gputypes.TextureFormatRG16Snorm, [4]float32{-1, 1, 0, 0}, []byte{0x01, 0x80, 0xFF, 0x7F}, [4]float32{-1, 1, 0, 1}},
		{gputypes.TextureFormatR32Uint, [4]float32{70000, 0, 0, 0}, le32(70000), [4]float32{70000, 0, 0, 1}},
		{gputypes.TextureFormatRG16Sint, [4]float32{-2, 300, 0, 0}, []byte{0xFE, 0xFF, 0x2C, 0x01}, [4]float32{-2, 300, 0, 1}},
		{gputypes.TextureFormatRGBA8Uint, [4]float32{1, 2, 3, 4}, []byte{1, 2, 3, 4}, [4]float32{1, 2, 3, 4}},
		{gputypes.TextureFormatRGB10A2Unorm, [4]float32{1, 0, 1.0 / 3, 1}, le32(1023 | 341<<20 | 3<<30), [4]float32{1, 0, 341.0 / 1023, 1}},
		{gputypes.TextureFormatRGB10A2Uint, [4]float32{1023, 7, 512, 2}, le32(1023 | 7<<10 | 512<<20 | 2<<30), [4]float32{1023, 7, 512, 2}},
		{gputypes.TextureFormatRG11B10Ufloat, [4]float32{1, 2, 0.5, 1}, le32(15<<6 | (16<<6)<<11 | (14<<5)<<22), [4]float32{1, 2, 0.5, 1}},
	}
	for _, tt := range tests {
		t.Run(tt.format.String(), func(t *testing.T) {
			c, ok := NewColorCodec(tt.format)
			if !ok {
				t.Fatal("no codec")
			}
			if c.TexelSize() != len(tt.want) {
				t.Fatalf("TexelSize = %d, want %d", c.TexelSize(), len(tt.want))
			}
			b := make([]byte, len(tt.want))
			c.Encode(b, tt.in)
			if !bytes.Equal(b, tt.want) {
				t.Errorf("Encode = %v, want %v", b, tt.want)
			}
			if got := c.Decode(b); !vec4Near(got, tt.back) {
				t.Errorf("Decode = %v, want %v", got, tt.back)
			}
		})
	}
	for _, f := range []gputypes.TextureFormat{
		gputypes.TextureFormatDepth24PlusStencil8, gputypes.TextureFormatDepth32Float,
		gputypes.TextureFormatRGB9E5Ufloat, gputypes.TextureFormatBC1RGBAUnorm, gputypes.TextureFormatUndefined,
	} {
		if _, ok := NewColorCodec(f); ok {
			t.Errorf("NewColorCodec(%v) has a codec", f)
		}
	}
}

// TestEncodeUnsignedSmallFloat: the 11- and 10-bit float encodes round to
// nearest even, flush negatives to 0, keep Inf and NaN, saturate overflow,
// and round-trip every finite code.
func TestEncodeUnsignedSmallFloat(t *testing.T) {
	for _, mant := range []int{6, 5} {
		for code := uint32(0); code < 31<<mant; code++ {
			if got := encodeUnsignedSmallFloat(unsignedSmallFloat(code, mant), mant); got != code {
				t.Fatalf("mant %d: code %#x round-trips to %#x", mant, code, got)
			}
		}
		maxFinite := uint32(30<<mant | (1<<mant - 1))
		if got := encodeUnsignedSmallFloat(1e9, mant); got != maxFinite {
			t.Errorf("mant %d: 1e9 = %#x, want max finite %#x", mant, got, maxFinite)
		}
		if got := encodeUnsignedSmallFloat(-3, mant); got != 0 {
			t.Errorf("mant %d: -3 = %#x, want 0", mant, got)
		}
		if got := encodeUnsignedSmallFloat(float32(math.Inf(1)), mant); got != 31<<mant {
			t.Errorf("mant %d: +Inf = %#x", mant, got)
		}
		if got := encodeUnsignedSmallFloat(float32(math.NaN()), mant); got>>mant != 31 || got&(1<<mant-1) == 0 {
			t.Errorf("mant %d: NaN = %#x", mant, got)
		}
	}
}

// TestColorCodecEncodeMasked: the write mask keeps the stored bits of the
// masked-out channels exactly, BGRA and packed formats included.
func TestColorCodecEncodeMasked(t *testing.T) {
	tests := []struct {
		format gputypes.TextureFormat
		old    []byte
		mask   gputypes.ColorWriteMask
		want   []byte
	}{
		{gputypes.TextureFormatRGBA8Unorm, []byte{1, 2, 3, 4}, gputypes.ColorWriteMaskRed | gputypes.ColorWriteMaskAlpha, []byte{255, 2, 3, 255}},
		// BGRA: red is stored third.
		{gputypes.TextureFormatBGRA8Unorm, []byte{1, 2, 3, 4}, gputypes.ColorWriteMaskRed, []byte{1, 2, 255, 4}},
		// Snorm -128 stays -128, though it decodes to -1 and would encode as -127.
		{gputypes.TextureFormatRG8Snorm, []byte{0x80, 0}, gputypes.ColorWriteMaskGreen, []byte{0x80, 0x7F}},
		{gputypes.TextureFormatRGB10A2Unorm, le32(5 | 6<<10 | 7<<20 | 1<<30), gputypes.ColorWriteMaskGreen, le32(5 | 1023<<10 | 7<<20 | 1<<30)},
		{gputypes.TextureFormatRGBA8Unorm, []byte{1, 2, 3, 4}, 0, []byte{1, 2, 3, 4}},
	}
	for _, tt := range tests {
		c, _ := NewColorCodec(tt.format)
		b := append([]byte(nil), tt.old...)
		c.EncodeMasked(b, [4]float32{1, 1, 1, 1}, tt.mask)
		if !bytes.Equal(b, tt.want) {
			t.Errorf("%v mask %#x: %v, want %v", tt.format, tt.mask, b, tt.want)
		}
	}
}

// TestColorCodecEncodeIntMasked: integer colour writes store the shader's
// 32 bits exactly in a 32-bit channel, keep the low bits in a narrower one,
// and leave the masked-out channels untouched.
func TestColorCodecEncodeIntMasked(t *testing.T) {
	tests := []struct {
		format gputypes.TextureFormat
		old    []byte
		v      [4]uint32
		mask   gputypes.ColorWriteMask
		want   []byte
	}{
		{gputypes.TextureFormatR32Uint, le32(0), [4]uint32{0xDEADBEEF}, gputypes.ColorWriteMaskAll, le32(0xDEADBEEF)},
		{gputypes.TextureFormatRG32Sint, make([]byte, 8), [4]uint32{0xF8A432EB, 16777217}, gputypes.ColorWriteMaskAll, append(le32(0xF8A432EB), le32(16777217)...)},
		{gputypes.TextureFormatRG32Uint, append(le32(7), le32(9)...), [4]uint32{1, 2}, gputypes.ColorWriteMaskGreen, append(le32(7), le32(2)...)},
		{gputypes.TextureFormatRGBA8Uint, []byte{1, 2, 3, 4}, [4]uint32{0x1FF, 5, 6, 7}, gputypes.ColorWriteMaskRed | gputypes.ColorWriteMaskBlue, []byte{0xFF, 2, 6, 4}},
		{gputypes.TextureFormatRG16Sint, []byte{0, 0, 0, 0}, [4]uint32{0xFFFFFFFE, 300}, gputypes.ColorWriteMaskAll, []byte{0xFE, 0xFF, 0x2C, 0x01}},
		{gputypes.TextureFormatRGB10A2Uint, le32(5 | 6<<10 | 7<<20 | 1<<30), [4]uint32{1023, 1023, 1023, 3}, gputypes.ColorWriteMaskGreen, le32(5 | 1023<<10 | 7<<20 | 1<<30)},
	}
	for _, tt := range tests {
		c, ok := NewColorCodec(tt.format)
		if !ok || !c.Integer() {
			t.Fatalf("%v: no integer codec", tt.format)
		}
		b := append([]byte(nil), tt.old...)
		c.EncodeIntMasked(b, tt.v, tt.mask)
		if !bytes.Equal(b, tt.want) {
			t.Errorf("%v mask %#x: %x, want %x", tt.format, tt.mask, b, tt.want)
		}
	}
	if c, _ := NewColorCodec(gputypes.TextureFormatR32Sint); !c.Signed() {
		t.Error("r32sint is not Signed")
	}
	if c, _ := NewColorCodec(gputypes.TextureFormatRGB10A2Uint); c.Signed() {
		t.Error("rgb10a2uint is Signed")
	}
}

// TestColorCodecEncodeClear: a clear of a 32-bit integer format is exact
// beyond float32's 24-bit mantissa.
func TestColorCodecEncodeClear(t *testing.T) {
	c, _ := NewColorCodec(gputypes.TextureFormatRG32Uint)
	b := make([]byte, 8)
	c.EncodeClear(b, [4]float64{4294967295, 16777217, 0, 0})
	if got := [2]uint32{binary.LittleEndian.Uint32(b), binary.LittleEndian.Uint32(b[4:])}; got != [2]uint32{4294967295, 16777217} {
		t.Errorf("rg32uint clear = %v", got)
	}
	s, _ := NewColorCodec(gputypes.TextureFormatR32Sint)
	s.EncodeClear(b, [4]float64{-2147483648, 0, 0, 0})
	if got := int32(binary.LittleEndian.Uint32(b)); got != math.MinInt32 {
		t.Errorf("r32sint clear = %d", got)
	}
	u, _ := NewColorCodec(gputypes.TextureFormatRGBA8UnormSrgb)
	u.EncodeClear(b[:4], [4]float64{0.5, 0, 1, 0.5})
	if !bytes.Equal(b[:4], []byte{188, 0, 255, 128}) {
		t.Errorf("rgba8unorm-srgb clear = %v", b[:4])
	}
}
