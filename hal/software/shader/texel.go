// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !(js && wasm)

package shader

import (
	"encoding/binary"
	"math"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal/software/internal/texcompress"
	"github.com/gogpu/wgpu/hal/software/raster"
)

// texelKind is how a channel of a texel is stored.
type texelKind uint8

const (
	texelUnorm texelKind = iota
	texelSnorm
	texelUint
	texelSint
	texelFloat
	// The packed formats store a texel as one little-endian 32-bit word
	// (channels 1, bytes 4), decoded as a whole by decodePacked.
	texelRGB10A2Unorm
	texelRGB10A2Uint
	texelRG11B10Ufloat
	texelRGB9E5Ufloat
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

// sampledFormat is how the interpreter reads one plane of a format: the
// texel layout of the plane, whether its red, green and blue channels are
// sRGB-encoded, the byte offset of the plane inside the stored texel, and
// the stored texel's size (0: the plane's own size).
type sampledFormat struct {
	texelLayout
	srgb   bool
	offset int
	stride int
}

// size is the bytes of the plane's channels.
func (f sampledFormat) size() int { return f.channels * f.bytes }

// texelStride is the bytes of one stored texel.
func (f sampledFormat) texelStride() int {
	if f.stride != 0 {
		return f.stride
	}
	return f.size()
}

// Combined depth-stencil storage (the software backend's own layout): a
// texel of Depth24PlusStencil8 or Depth32FloatStencil8 is 8 bytes, the depth
// as a float32 at offset 0 and the stencil as a uint8 at offset 4. Depth24Plus
// stores the float32 depth alone; Stencil8 the stencil byte alone.
const (
	// DepthStencilTexelSize is the stored texel size of a combined format.
	DepthStencilTexelSize = 8
	// StencilPlaneOffset is the byte offset of the stencil in such a texel.
	StencilPlaneOffset = 4
)

// sampledLayouts, indexed by format, lists every uncompressed format the
// interpreter reads (sampling, textureLoad and OpImageRead): the storage
// formats of texelLayouts plus the formats only sampled. A zero entry has no
// layout. The block-compressed formats have none: decodeTexel decodes them
// per block through texcompress (BC and ETC2/EAC; ASTC is not offered, so
// core refuses it). A combined
// depth-stencil format reads its depth plane here; stencilLayout gives the
// stencil plane.
var sampledLayouts = buildSampledLayouts()

func buildSampledLayouts() [gputypes.TextureFormatDepth32FloatStencil8 + 1]sampledFormat {
	var l [gputypes.TextureFormatDepth32FloatStencil8 + 1]sampledFormat
	for f, layout := range texelLayouts {
		l[f] = sampledFormat{texelLayout: layout}
	}
	l[gputypes.TextureFormatRGBA8UnormSrgb] = sampledFormat{texelLayout: texelLayout{4, 1, texelUnorm, false}, srgb: true}
	l[gputypes.TextureFormatBGRA8UnormSrgb] = sampledFormat{texelLayout: texelLayout{4, 1, texelUnorm, true}, srgb: true}
	for f, kind := range map[gputypes.TextureFormat]texelKind{
		gputypes.TextureFormatRGB10A2Unorm:  texelRGB10A2Unorm,
		gputypes.TextureFormatRGB10A2Uint:   texelRGB10A2Uint,
		gputypes.TextureFormatRG11B10Ufloat: texelRG11B10Ufloat,
		gputypes.TextureFormatRGB9E5Ufloat:  texelRGB9E5Ufloat,
	} {
		l[f] = sampledFormat{texelLayout: texelLayout{1, 4, kind, false}}
	}
	// Depth reads as (d, 0, 0, 1), stencil as (s, 0, 0, 1).
	l[gputypes.TextureFormatStencil8] = sampledFormat{texelLayout: texelLayout{1, 1, texelUint, false}}
	l[gputypes.TextureFormatDepth16Unorm] = sampledFormat{texelLayout: texelLayout{1, 2, texelUnorm, false}}
	depth := texelLayout{1, 4, texelFloat, false}
	l[gputypes.TextureFormatDepth24Plus] = sampledFormat{texelLayout: depth}
	l[gputypes.TextureFormatDepth32Float] = sampledFormat{texelLayout: depth}
	l[gputypes.TextureFormatDepth24PlusStencil8] = sampledFormat{texelLayout: depth, stride: DepthStencilTexelSize}
	l[gputypes.TextureFormatDepth32FloatStencil8] = sampledFormat{texelLayout: depth, stride: DepthStencilTexelSize}
	return l
}

// stencilLayout returns the stencil plane of format: the whole texel of
// Stencil8, the byte at StencilPlaneOffset of a combined format.
func stencilLayout(format gputypes.TextureFormat) (sampledFormat, bool) {
	stencil := texelLayout{1, 1, texelUint, false}
	switch format {
	case gputypes.TextureFormatStencil8:
		return sampledFormat{texelLayout: stencil}, true
	case gputypes.TextureFormatDepth24PlusStencil8, gputypes.TextureFormatDepth32FloatStencil8:
		return sampledFormat{texelLayout: stencil, offset: StencilPlaneOffset, stride: DepthStencilTexelSize}, true
	}
	return sampledFormat{}, false
}

// sampledLayout returns the layout tex is read with: the plane its Aspect
// selects (the stencil of a stencil-only view, the depth or color
// otherwise). A texture without a format (0, a Texture2D built by hand)
// reads as 8-bit unorm channels by BytesPerPixel: R8, RG8, otherwise RGBA8.
func sampledLayout(tex *Texture2D) (sampledFormat, bool) {
	if tex.Format == 0 {
		switch tex.BytesPerPixel {
		case 1:
			return sampledFormat{texelLayout: texelLayouts[gputypes.TextureFormatR8Unorm]}, true
		case 2:
			return sampledFormat{texelLayout: texelLayouts[gputypes.TextureFormatRG8Unorm]}, true
		}
		return sampledFormat{texelLayout: texelLayouts[gputypes.TextureFormatRGBA8Unorm]}, true
	}
	if gputypes.TextureAspect(tex.Aspect) == gputypes.TextureAspectStencilOnly {
		return stencilLayout(gputypes.TextureFormat(tex.Format))
	}
	if uint64(tex.Format) >= uint64(len(sampledLayouts)) {
		return sampledFormat{}, false
	}
	l := sampledLayouts[tex.Format]
	return l, l.channels != 0
}

// texelOffset returns the byte offset of the plane of layout in texel
// (x, y) of tex, or false when the texel lies outside the texture or its
// data. Texels are tex.texelSize() bytes apart.
func texelOffset(tex *Texture2D, x, y int, layout sampledFormat) (int, bool) {
	if x < 0 || y < 0 || x >= int(tex.Width) || y >= int(tex.Height) {
		return 0, false
	}
	off := (y*int(tex.Width)+x)*tex.texelSize() + layout.offset
	if off+layout.size() > len(tex.Data) {
		return 0, false
	}
	return off, true
}

// encodeTexel stores v into texel (x, y) of tex with the WebGPU conversion
// rules for the texel format: unorm and snorm values are clamped and
// rounded, integer values are converted to the channel width, and channels
// the format lacks are dropped. Out-of-bounds writes are discarded, as
// WebGPU allows for textureStore. It reports whether the texel was written;
// only the storage formats of texelLayouts are written.
func encodeTexel(tex *Texture2D, x, y int, v Vec4) bool {
	layout, ok := texelLayouts[gputypes.TextureFormat(tex.Format)]
	if !ok {
		return false
	}
	off, ok := texelOffset(tex, x, y, sampledFormat{texelLayout: layout})
	if !ok {
		return false
	}
	encodeAt(tex.Data[off:], sampledFormat{texelLayout: layout}, v)
	return true
}

// decodeTexel reads texel (x, y) of tex as a vec4 by its format, the missing
// channels filled with (0, 0, 0, 1), and reports whether the format has a
// layout and the texel lies in bounds. It is the one decoder of sampled
// reads, textureLoad and storage reads; an sRGB format decodes to linear
// here, before any filtering, as the WebGPU spec requires.
func decodeTexel(tex *Texture2D, x, y int) (Vec4, bool) {
	layout, ok := sampledLayout(tex)
	if !ok {
		// Compressed formats have no per-texel layout; checking them only
		// here keeps the switch off the path of every uncompressed fetch.
		if f := gputypes.TextureFormat(tex.Format); texcompress.Decodes(f) {
			return decodeCompressedTexel(tex, f, x, y)
		}
		return Vec4{}, false
	}
	off, ok := texelOffset(tex, x, y, layout)
	if !ok {
		return Vec4{}, false
	}
	return decodeAt(tex.Data[off:], layout), true
}

// decodeCompressedTexel decodes texel (x, y) of tex, whose format f is block
// compressed: it decodes that texel from its own 4x4 block, which the
// software backend stores as whole blocks, row-major, tex.texelSize() bytes
// each (P1-SW-026). The red, green and blue of an sRGB format decode to
// linear after the block decode; texcompress returns them as exact 8-bit
// codes.
func decodeCompressedTexel(tex *Texture2D, f gputypes.TextureFormat, x, y int) (Vec4, bool) {
	if x < 0 || y < 0 || x >= int(tex.Width) || y >= int(tex.Height) {
		return Vec4{}, false
	}
	size := tex.texelSize()
	off := ((y/4)*blocksAcross(tex.Width) + x/4) * size
	if off+size > len(tex.Data) {
		return Vec4{}, false
	}
	v, ok := texcompress.Decode(f, tex.Data[off:off+size], x&3, y&3)
	if ok && f.IsSrgb() {
		for c := range 3 {
			v[c] = srgbToLinear8[uint8(v[c]*255+0.5)]
		}
	}
	return v, ok
}

// blocksAcross is the number of 4x4 blocks covering n texels.
func blocksAcross(n uint32) int { return (int(n) + 3) / 4 }

// decodeAt decodes the texel plane of layout that starts at b[0].
func decodeAt(b []byte, layout sampledFormat) Vec4 {
	if layout.kind >= texelRGB10A2Unorm {
		return decodePacked(binary.LittleEndian.Uint32(b), layout.kind)
	}
	v := Vec4{0, 0, 0, 1}
	for c := range layout.channels {
		if layout.srgb && c < 3 {
			v[c] = srgbToLinear8[b[c]]
			continue
		}
		v[c] = getChannel(b[c*layout.bytes:], layout.texelLayout)
	}
	if layout.bgra {
		v[0], v[2] = v[2], v[0]
	}
	return v
}

// encodeAt encodes v into the texel plane of layout that starts at b[0],
// with the WebGPU conversion rules: unorm and snorm values are clamped and
// rounded, sRGB channels encoded from linear, integer values converted to
// the channel width, and channels the format lacks dropped.
func encodeAt(b []byte, layout sampledFormat, v Vec4) {
	if layout.kind >= texelRGB10A2Unorm {
		binary.LittleEndian.PutUint32(b, encodePacked(v, layout.kind))
		return
	}
	if layout.bgra {
		v[0], v[2] = v[2], v[0]
	}
	for c := range layout.channels {
		if layout.srgb && c < 3 {
			b[c] = linearToSrgb8(v[c])
			continue
		}
		putChannel(b[c*layout.bytes:], layout.texelLayout, v[c])
	}
}

func putChannel(b []byte, layout texelLayout, f float32) {
	var bits uint32
	switch layout.kind {
	case texelUnorm:
		bits = raster.FloatToUnorm(f, uint32(1)<<(8*layout.bytes)-1)
	case texelSnorm:
		bits = uint32(raster.FloatToSnorm(f, int32(1)<<(8*layout.bytes-1)-1))
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

// decodePacked decodes a packed 32-bit texel w of kind, red in the low bits,
// as the Vulkan spec defines the packed formats, its unsigned 11- and 10-bit
// floats and the shared exponent of rgb9e5 (bias 15, 9-bit mantissas).
func decodePacked(w uint32, kind texelKind) Vec4 {
	switch kind {
	case texelRGB10A2Unorm:
		return Vec4{
			float32(w&0x3FF) / 1023, float32(w>>10&0x3FF) / 1023,
			float32(w>>20&0x3FF) / 1023, float32(w>>30) / 3,
		}
	case texelRGB10A2Uint:
		return Vec4{float32(w & 0x3FF), float32(w >> 10 & 0x3FF), float32(w >> 20 & 0x3FF), float32(w >> 30)}
	case texelRG11B10Ufloat:
		return Vec4{unsignedSmallFloat(w&0x7FF, 6), unsignedSmallFloat(w>>11&0x7FF, 6), unsignedSmallFloat(w>>22, 5), 1}
	default: // texelRGB9E5Ufloat: mantissas scaled by 2^(exponent - 15 - 9)
		exp := int(w>>27) - 24
		return Vec4{
			float32(math.Ldexp(float64(w&0x1FF), exp)),
			float32(math.Ldexp(float64(w>>9&0x1FF), exp)),
			float32(math.Ldexp(float64(w>>18&0x1FF), exp)), 1,
		}
	}
}

// unsignedSmallFloat decodes an unsigned float with a 5-bit exponent (bias
// 15) above a mantissa of mantBits bits: the 11-bit (6) and 10-bit (5)
// channels of rg11b10ufloat.
func unsignedSmallFloat(bits uint32, mantBits int) float32 {
	exp := int(bits >> mantBits)
	mant := float64(bits & (1<<mantBits - 1))
	switch exp {
	case 0: // denormal: mant * 2^(-14 - mantBits)
		return float32(math.Ldexp(mant, -14-mantBits))
	case 31:
		if mant == 0 {
			return float32(math.Inf(1))
		}
		return float32(math.NaN())
	}
	return float32(math.Ldexp(1+mant/float64(int(1)<<mantBits), exp-15))
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
