// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !(js && wasm)

package software

import (
	"encoding/binary"
	"math"
	"slices"
	"testing"

	"github.com/gogpu/gputypes"
)

// TestReadVertexAttributeIntegerWords checks that integer vertex formats
// arrive as interface lanes holding the exact 32-bit word of each component:
// zero-extended for Uint, sign-extended for Sint, bit patterns that are float
// NaNs included. They were converted with float32(int) and lost every value
// above 2^24 (P1-SW-019).
func TestReadVertexAttributeIntegerWords(t *testing.T) {
	le16 := func(v ...uint16) []byte {
		b := make([]byte, 2*len(v))
		for i, x := range v {
			binary.LittleEndian.PutUint16(b[2*i:], x)
		}
		return b
	}
	le32 := func(v ...uint32) []byte {
		b := make([]byte, 4*len(v))
		for i, x := range v {
			binary.LittleEndian.PutUint32(b[4*i:], x)
		}
		return b
	}
	cases := []struct {
		format gputypes.VertexFormat
		data   []byte
		want   []uint32
	}{
		{gputypes.VertexFormatUint8x2, []byte{0xFF, 0x80}, []uint32{0xFF, 0x80}},
		{gputypes.VertexFormatUint8x4, []byte{0xFF, 0x80, 0x7F, 0x01}, []uint32{0xFF, 0x80, 0x7F, 0x01}},
		{gputypes.VertexFormatSint8x2, []byte{0xFF, 0x80}, []uint32{0xFFFFFFFF, 0xFFFFFF80}},
		{gputypes.VertexFormatSint8x4, []byte{0xFF, 0x80, 0x7F, 0x00}, []uint32{0xFFFFFFFF, 0xFFFFFF80, 0x7F, 0}},
		{gputypes.VertexFormatUint16x2, le16(0xFFFF, 0x8000), []uint32{0xFFFF, 0x8000}},
		{gputypes.VertexFormatUint16x4, le16(0xFFFF, 0x8000, 0x7FFF, 1), []uint32{0xFFFF, 0x8000, 0x7FFF, 1}},
		{gputypes.VertexFormatSint16x2, le16(0xFFFF, 0x8000), []uint32{0xFFFFFFFF, 0xFFFF8000}},
		{gputypes.VertexFormatSint16x4, le16(0xFFFF, 0x8000, 0x7FFF, 1), []uint32{0xFFFFFFFF, 0xFFFF8000, 0x7FFF, 1}},
		{gputypes.VertexFormatUint32, le32(0x7F800001), []uint32{0x7F800001}},
		{gputypes.VertexFormatUint32x2, le32(0xDEADBEEF, 16777217), []uint32{0xDEADBEEF, 16777217}},
		{gputypes.VertexFormatUint32x3, le32(0xFFFFFFFF, 0x7FC00001, 0), []uint32{0xFFFFFFFF, 0x7FC00001, 0}},
		{gputypes.VertexFormatUint32x4, le32(1, 2, 0xFF800001, 4000000000), []uint32{1, 2, 0xFF800001, 4000000000}},
		{gputypes.VertexFormatSint32, le32(0x80000000), []uint32{0x80000000}},
		{gputypes.VertexFormatSint32x2, le32(0xF8A432EB, 16777219), []uint32{0xF8A432EB, 16777219}},
		{gputypes.VertexFormatSint32x3, le32(0xFFFFFFFF, 0x7FFFFFFF, 5), []uint32{0xFFFFFFFF, 0x7FFFFFFF, 5}},
		{gputypes.VertexFormatSint32x4, le32(0x80000001, 0, 0xFFFFFFFE, 0x01000001), []uint32{0x80000001, 0, 0xFFFFFFFE, 0x01000001}},
	}
	for _, c := range cases {
		t.Run(c.format.String(), func(t *testing.T) {
			// A leading pad byte checks the offset.
			data := append([]byte{0xAA}, c.data...)
			lanes := readVertexAttribute(data, 1, c.format)
			got := make([]uint32, len(lanes))
			for i, l := range lanes {
				got[i] = math.Float32bits(l)
			}
			if !slices.Equal(got, c.want) {
				t.Errorf("lanes = %#x, want %#x", got, c.want)
			}
			// A buffer that ends inside the attribute reads nothing.
			if short := readVertexAttribute(data[:len(data)-1], 1, c.format); short != nil {
				t.Errorf("short buffer: lanes = %v, want nil", short)
			}
		})
	}
}
