//go:build !(js && wasm)

package software

import (
	"testing"

	"github.com/gogpu/gputypes"
)

// TestSubresourceLayout pins the mip-major storage layout (subresource.go)
// with offsets worked out by hand: level 0 starts at 0 with the layout the
// backend had before mip chains were stored, each level is its images back
// to back at the level's own pitch, and a 3D texture's depth halves per
// level (minimum 1). Non-power-of-two sizes round each level down, and
// block-compressed formats count in 4x4 blocks.
func TestSubresourceLayout(t *testing.T) {
	type sub struct{ level, z uint32 }
	tests := []struct {
		name        string
		tex         *Texture
		levelOffset []uint32 // levelOffset(l) per level
		images      []uint32 // images in each level
		subOffset   map[sub]uint64
		storage     uint64
	}{
		{
			// 7x5 (28 B rows, 140 B), 3x2 (12 B rows, 24 B), 1x1 (4 B).
			name:        "2d with mips, npot",
			tex:         &Texture{width: 7, height: 5, depth: 1, dimension: gputypes.TextureDimension2D, format: gputypes.TextureFormatRGBA8Unorm, mipLevelCount: 3},
			levelOffset: []uint32{0, 140, 164},
			images:      []uint32{1, 1, 1},
			subOffset:   map[sub]uint64{{0, 0}: 0, {1, 0}: 140, {2, 0}: 164},
			storage:     168,
		},
		{
			// 3 layers of 5x3 (60 B each), then 3 layers of 2x1 (8 B each).
			name:        "2d array",
			tex:         &Texture{width: 5, height: 3, depth: 3, dimension: gputypes.TextureDimension2D, format: gputypes.TextureFormatRGBA8Unorm, mipLevelCount: 2},
			levelOffset: []uint32{0, 180},
			images:      []uint32{3, 3},
			subOffset:   map[sub]uint64{{0, 2}: 120, {1, 0}: 180, {1, 1}: 188, {1, 2}: 196},
			storage:     204,
		},
		{
			// 6 faces of 4x4 (64 B), 2x2 (16 B), 1x1 (4 B).
			name:        "cube",
			tex:         &Texture{width: 4, height: 4, depth: 6, dimension: gputypes.TextureDimension2D, format: gputypes.TextureFormatRGBA8Unorm, mipLevelCount: 3},
			levelOffset: []uint32{0, 384, 480},
			images:      []uint32{6, 6, 6},
			subOffset:   map[sub]uint64{{0, 5}: 320, {1, 5}: 464, {2, 3}: 492},
			storage:     504,
		},
		{
			// 5 slices of 6x4 (96 B), 2 slices of 3x2 (24 B), 1 slice of 1x1.
			name:        "3d, depth shrinks per level",
			tex:         &Texture{width: 6, height: 4, depth: 5, dimension: gputypes.TextureDimension3D, format: gputypes.TextureFormatRGBA8Unorm, mipLevelCount: 3},
			levelOffset: []uint32{0, 480, 528},
			images:      []uint32{5, 2, 1},
			subOffset:   map[sub]uint64{{0, 4}: 384, {1, 1}: 504, {2, 0}: 528},
			storage:     532,
		},
		{
			// 1-byte texels: 7x5 (35 B), 3x2 (6 B).
			name:        "r8, npot",
			tex:         &Texture{width: 7, height: 5, depth: 1, dimension: gputypes.TextureDimension2D, format: gputypes.TextureFormatR8Unorm, mipLevelCount: 2},
			levelOffset: []uint32{0, 35},
			images:      []uint32{1, 1},
			subOffset:   map[sub]uint64{{1, 0}: 35},
			storage:     41,
		},
		{
			// BC1 8-byte 4x4 blocks: 10x6 is 3x2 blocks (48 B), 5x3 is 2x1 (16 B).
			name:        "bc1 blocks",
			tex:         &Texture{width: 10, height: 6, depth: 1, dimension: gputypes.TextureDimension2D, format: gputypes.TextureFormatBC1RGBAUnorm, mipLevelCount: 2},
			levelOffset: []uint32{0, 48},
			images:      []uint32{1, 1},
			subOffset:   map[sub]uint64{{1, 0}: 48},
			storage:     64,
		},
		{
			// A literal without dimension or mip count: one 2D level of 2 layers.
			name:        "literal defaults",
			tex:         &Texture{width: 3, height: 2, depth: 2, format: gputypes.TextureFormatRGBA8Unorm},
			levelOffset: []uint32{0},
			images:      []uint32{2},
			subOffset:   map[sub]uint64{{0, 1}: 24},
			storage:     48,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tex := tt.tex
			if got := tex.storageSize(); got != tt.storage {
				t.Errorf("storageSize = %d, want %d", got, tt.storage)
			}
			for l, want := range tt.levelOffset {
				if got := tex.levelOffset(uint32(l)); got != uint64(want) {
					t.Errorf("levelOffset(%d) = %d, want %d", l, got, want)
				}
				if _, _, n := tex.levelExtent(uint32(l)); n != tt.images[l] {
					t.Errorf("level %d images = %d, want %d", l, n, tt.images[l])
				}
			}
			for s, want := range tt.subOffset {
				if got := tex.subresourceOffset(s.level, s.z); got != want {
					t.Errorf("subresourceOffset(%d, %d) = %d, want %d", s.level, s.z, got, want)
				}
			}
			tex.data = make([]byte, tex.storageSize())
			last := uint32(len(tt.levelOffset)) - 1
			if _, ok := tex.image(last, tt.images[last]-1); !ok {
				t.Errorf("image(%d, %d) missing", last, tt.images[last]-1)
			}
			if _, ok := tex.image(last+1, 0); ok {
				t.Errorf("image(%d, 0) exists past the last level", last+1)
			}
			if _, ok := tex.image(last, tt.images[last]); ok {
				t.Errorf("image(%d, %d) exists past the last image", last, tt.images[last])
			}
		})
	}
}
