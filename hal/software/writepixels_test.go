//go:build !(js && wasm)

package software

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

func TestResolveTexture(t *testing.T) {
	t.Run("regular texture", func(t *testing.T) {
		tex := &Texture{data: make([]byte, 16), format: gputypes.TextureFormatRGBA8Unorm}
		got := resolveTexture(tex)
		if got != tex {
			t.Error("resolveTexture(*Texture) should return the same pointer")
		}
	})

	t.Run("surface texture", func(t *testing.T) {
		st := &SurfaceTexture{
			Texture: Texture{data: make([]byte, 16), format: gputypes.TextureFormatBGRA8Unorm},
		}
		got := resolveTexture(st)
		if got == nil {
			t.Fatal("resolveTexture(*SurfaceTexture) returned nil — type assertion bug")
		}
		if got != &st.Texture {
			t.Error("resolveTexture(*SurfaceTexture) should return &st.Texture")
		}
	})

	t.Run("nil", func(t *testing.T) {
		got := resolveTexture(nil)
		if got != nil {
			t.Error("resolveTexture(nil) should return nil")
		}
	})
}

func TestWriteTextureToSurfaceTexture(t *testing.T) {
	q := &Queue{}
	st := &SurfaceTexture{
		Texture: Texture{
			data:   make([]byte, 16), // 2x2 RGBA
			width:  2,
			height: 2,
			format: gputypes.TextureFormatRGBA8Unorm,
		},
	}

	pixelData := []byte{
		255, 0, 0, 255, 0, 255, 0, 255, // row 0: red, green
		0, 0, 255, 255, 255, 255, 0, 255, // row 1: blue, yellow
	}

	err := q.WriteTexture(
		&hal.ImageCopyTexture{Texture: st},
		pixelData,
		&hal.ImageDataLayout{BytesPerRow: 8},
		&hal.Extent3D{Width: 2, Height: 2, DepthOrArrayLayers: 1},
	)
	if err != nil {
		t.Fatalf("WriteTexture to SurfaceTexture: %v", err)
	}

	// Verify pixel 0 (red)
	if st.data[0] != 255 || st.data[1] != 0 || st.data[2] != 0 || st.data[3] != 255 {
		t.Errorf("pixel 0: got %v, want [255 0 0 255]", st.data[0:4])
	}
}

func TestWritePixels(t *testing.T) {
	s := &Surface{
		configured:  true,
		width:       2,
		height:      2,
		format:      gputypes.TextureFormatBGRA8Unorm,
		framebuffer: make([]byte, 16),
	}

	// RGBA input: red pixel
	rgba := []byte{
		255, 0, 0, 255, 0, 255, 0, 255, // row 0: red, green
		0, 0, 255, 255, 255, 255, 255, 255, // row 1: blue, white
	}

	err := s.WritePixels(rgba, 2, 2)
	if err != nil {
		t.Fatalf("WritePixels: %v", err)
	}

	// BGRA output: red pixel → [B=0, G=0, R=255, A=255]
	if s.framebuffer[0] != 0 || s.framebuffer[1] != 0 || s.framebuffer[2] != 255 || s.framebuffer[3] != 255 {
		t.Errorf("pixel 0 (red→BGRA): got %v, want [0 0 255 255]", s.framebuffer[0:4])
	}

	// Green pixel → [B=0, G=255, R=0, A=255]
	if s.framebuffer[4] != 0 || s.framebuffer[5] != 255 || s.framebuffer[6] != 0 || s.framebuffer[7] != 255 {
		t.Errorf("pixel 1 (green→BGRA): got %v, want [0 255 0 255]", s.framebuffer[4:8])
	}

	// Blue pixel → [B=255, G=0, R=0, A=255]
	if s.framebuffer[8] != 255 || s.framebuffer[9] != 0 || s.framebuffer[10] != 0 || s.framebuffer[11] != 255 {
		t.Errorf("pixel 2 (blue→BGRA): got %v, want [255 0 0 255]", s.framebuffer[8:12])
	}

	// White pixel → [B=255, G=255, R=255, A=255]
	if s.framebuffer[12] != 255 || s.framebuffer[13] != 255 || s.framebuffer[14] != 255 || s.framebuffer[15] != 255 {
		t.Errorf("pixel 3 (white→BGRA): got %v, want [255 255 255 255]", s.framebuffer[12:16])
	}
}

func TestWritePixelsRGBAFormat(t *testing.T) {
	s := &Surface{
		configured:  true,
		width:       2,
		height:      1,
		format:      gputypes.TextureFormatRGBA8Unorm,
		framebuffer: make([]byte, 8),
	}

	rgba := []byte{10, 20, 30, 40, 50, 60, 70, 80}
	err := s.WritePixels(rgba, 2, 1)
	if err != nil {
		t.Fatalf("WritePixels RGBA: %v", err)
	}

	// No swizzle — direct copy
	for i := range rgba {
		if s.framebuffer[i] != rgba[i] {
			t.Errorf("byte %d: got %d, want %d", i, s.framebuffer[i], rgba[i])
		}
	}
}

func TestWritePixelsNotConfigured(t *testing.T) {
	s := &Surface{configured: false}
	err := s.WritePixels([]byte{0, 0, 0, 0}, 1, 1)
	if err == nil {
		t.Error("WritePixels on unconfigured surface should return error")
	}
}

func TestWritePixelsSizeMismatch(t *testing.T) {
	s := &Surface{
		configured:  true,
		width:       4,
		height:      4,
		format:      gputypes.TextureFormatBGRA8Unorm,
		framebuffer: make([]byte, 64),
	}
	err := s.WritePixels(make([]byte, 16), 2, 2)
	if err == nil {
		t.Error("WritePixels with mismatched size should return error")
	}
}
