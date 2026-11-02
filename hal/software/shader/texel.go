// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package shader

import (
	"encoding/binary"
	"math"

	"github.com/gogpu/gputypes"
)

// texelKind is how a channel of a texel is stored.
type texelKind uint8

const (
	texelUnorm texelKind = iota
	texelSnorm
	texelUint
	texelSint
	texelFloat
)

// texelLayout describes how a texture format stores one texel: the channel
// count, the bytes per channel, the channel kind and whether red and blue are
// swapped (BGRA).
type texelLayout struct {
	channels int
	bytes    int
	kind     texelKind
	bgra     bool
}

// texelLayouts lists the uncompressed color formats whose texels the
// interpreter encodes for OpImageWrite and decodes for OpImageRead: every
// WebGPU storage texture format apart from the packed rgb10a2 and rg11b10.
var texelLayouts = map[gputypes.TextureFormat]texelLayout{
	gputypes.TextureFormatR8Unorm:     {1, 1, texelUnorm, false},
	gputypes.TextureFormatR8Snorm:     {1, 1, texelSnorm, false},
	gputypes.TextureFormatR8Uint:      {1, 1, texelUint, false},
	gputypes.TextureFormatR8Sint:      {1, 1, texelSint, false},
	gputypes.TextureFormatRG8Unorm:    {2, 1, texelUnorm, false},
	gputypes.TextureFormatRG8Snorm:    {2, 1, texelSnorm, false},
	gputypes.TextureFormatRG8Uint:     {2, 1, texelUint, false},
	gputypes.TextureFormatRG8Sint:     {2, 1, texelSint, false},
	gputypes.TextureFormatRGBA8Unorm:  {4, 1, texelUnorm, false},
	gputypes.TextureFormatRGBA8Snorm:  {4, 1, texelSnorm, false},
	gputypes.TextureFormatRGBA8Uint:   {4, 1, texelUint, false},
	gputypes.TextureFormatRGBA8Sint:   {4, 1, texelSint, false},
	gputypes.TextureFormatBGRA8Unorm:  {4, 1, texelUnorm, true},
	gputypes.TextureFormatR16Unorm:    {1, 2, texelUnorm, false},
	gputypes.TextureFormatR16Snorm:    {1, 2, texelSnorm, false},
	gputypes.TextureFormatR16Uint:     {1, 2, texelUint, false},
	gputypes.TextureFormatR16Sint:     {1, 2, texelSint, false},
	gputypes.TextureFormatR16Float:    {1, 2, texelFloat, false},
	gputypes.TextureFormatRG16Unorm:   {2, 2, texelUnorm, false},
	gputypes.TextureFormatRG16Snorm:   {2, 2, texelSnorm, false},
	gputypes.TextureFormatRG16Uint:    {2, 2, texelUint, false},
	gputypes.TextureFormatRG16Sint:    {2, 2, texelSint, false},
	gputypes.TextureFormatRG16Float:   {2, 2, texelFloat, false},
	gputypes.TextureFormatRGBA16Unorm: {4, 2, texelUnorm, false},
	gputypes.TextureFormatRGBA16Snorm: {4, 2, texelSnorm, false},
	gputypes.TextureFormatRGBA16Uint:  {4, 2, texelUint, false},
	gputypes.TextureFormatRGBA16Sint:  {4, 2, texelSint, false},
	gputypes.TextureFormatRGBA16Float: {4, 2, texelFloat, false},
	gputypes.TextureFormatR32Uint:     {1, 4, texelUint, false},
	gputypes.TextureFormatR32Sint:     {1, 4, texelSint, false},
	gputypes.TextureFormatR32Float:    {1, 4, texelFloat, false},
	gputypes.TextureFormatRG32Uint:    {2, 4, texelUint, false},
	gputypes.TextureFormatRG32Sint:    {2, 4, texelSint, false},
	gputypes.TextureFormatRG32Float:   {2, 4, texelFloat, false},
	gputypes.TextureFormatRGBA32Uint:  {4, 4, texelUint, false},
	gputypes.TextureFormatRGBA32Sint:  {4, 4, texelSint, false},
	gputypes.TextureFormatRGBA32Float: {4, 4, texelFloat, false},
}

// texelOffset returns the byte offset of texel (x, y) of tex and its layout,
// or false when the format has no layout or the texel lies outside the
// texture or its data.
func texelOffset(tex *Texture2D, x, y int) (int, texelLayout, bool) {
	layout, ok := texelLayouts[gputypes.TextureFormat(tex.Format)]
	if !ok || x < 0 || y < 0 || x >= int(tex.Width) || y >= int(tex.Height) {
		return 0, layout, false
	}
	size := layout.channels * layout.bytes
	off := (y*int(tex.Width) + x) * size
	if off+size > len(tex.Data) {
		return 0, layout, false
	}
	return off, layout, true
}

// encodeTexel stores v into texel (x, y) of tex with the WebGPU conversion
// rules for the texel format: unorm and snorm values are clamped and
// rounded, integer values are converted to the channel width, and channels
// the format lacks are dropped. Out-of-bounds writes are discarded, as
// WebGPU allows for textureStore. It reports whether the texel was written.
func encodeTexel(tex *Texture2D, x, y int, v Vec4) bool {
	off, layout, ok := texelOffset(tex, x, y)
	if !ok {
		return false
	}
	if layout.bgra {
		v[0], v[2] = v[2], v[0]
	}
	for c := range layout.channels {
		putChannel(tex.Data[off+c*layout.bytes:], layout, v[c])
	}
	return true
}

// decodeTexel reads texel (x, y) of tex as a vec4, the missing channels
// filled with (0, 0, 0, 1), and reports whether the format has a layout and
// the texel lies in bounds.
func decodeTexel(tex *Texture2D, x, y int) (Vec4, bool) {
	off, layout, ok := texelOffset(tex, x, y)
	if !ok {
		return Vec4{}, false
	}
	v := Vec4{0, 0, 0, 1}
	for c := range layout.channels {
		v[c] = getChannel(tex.Data[off+c*layout.bytes:], layout)
	}
	if layout.bgra {
		v[0], v[2] = v[2], v[0]
	}
	return v, true
}

func putChannel(b []byte, layout texelLayout, f float32) {
	var bits uint32
	switch layout.kind {
	case texelUnorm:
		maxVal := float64(uint32(1)<<(8*layout.bytes) - 1)
		bits = uint32(math.Round(float64(clampFloat(f, 0, 1)) * maxVal))
	case texelSnorm:
		maxVal := float64(uint32(1)<<(8*layout.bytes-1) - 1)
		bits = uint32(int32(math.Round(float64(clampFloat(f, -1, 1)) * maxVal)))
	case texelUint:
		bits = uint32(int64(f))
	case texelSint:
		bits = uint32(int32(f))
	case texelFloat:
		if layout.bytes == 2 {
			bits = uint32(float32ToFloat16bits(f))
		} else {
			bits = math.Float32bits(f)
		}
	}
	switch layout.bytes {
	case 1:
		b[0] = byte(bits)
	case 2:
		binary.LittleEndian.PutUint16(b, uint16(bits))
	default:
		binary.LittleEndian.PutUint32(b, bits)
	}
}

func getChannel(b []byte, layout texelLayout) float32 {
	var bits uint32
	switch layout.bytes {
	case 1:
		bits = uint32(b[0])
	case 2:
		bits = uint32(binary.LittleEndian.Uint16(b))
	default:
		bits = binary.LittleEndian.Uint32(b)
	}
	width := 8 * layout.bytes
	switch layout.kind {
	case texelUnorm:
		return float32(float64(bits) / float64(uint32(1)<<width-1))
	case texelSnorm:
		s := signExtend(bits, width)
		return clampFloat(float32(float64(s)/float64(int32(1)<<(width-1)-1)), -1, 1)
	case texelUint:
		return float32(bits)
	case texelSint:
		return float32(signExtend(bits, width))
	default: // texelFloat
		if layout.bytes == 2 {
			return float16bitsToFloat32(uint16(bits))
		}
		return math.Float32frombits(bits)
	}
}

// signExtend interprets the low width bits of bits as a signed integer.
func signExtend(bits uint32, width int) int32 {
	shift := 32 - width
	return int32(bits<<shift) >> shift
}

func clampFloat(f, lo, hi float32) float32 {
	if f < lo || math.IsNaN(float64(f)) { // NaN clamps to lo
		return lo
	}
	if f > hi {
		return hi
	}
	return f
}
