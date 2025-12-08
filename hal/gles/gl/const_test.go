// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

package gl

import (
	"testing"
)

func TestGLConstants(t *testing.T) {
	// Test that constants have expected values (from OpenGL spec)
	tests := []struct {
		name  string
		value uint32
		want  uint32
	}{
		// Boolean
		{"FALSE", FALSE, 0},
		{"TRUE", TRUE, 1},

		// Data types
		{"BYTE", BYTE, 0x1400},
		{"UNSIGNED_BYTE", UNSIGNED_BYTE, 0x1401},
		{"SHORT", SHORT, 0x1402},
		{"UNSIGNED_SHORT", UNSIGNED_SHORT, 0x1403},
		{"INT", INT, 0x1404},
		{"UNSIGNED_INT", UNSIGNED_INT, 0x1405},
		{"FLOAT", FLOAT, 0x1406},

		// Error codes
		{"NO_ERROR", NO_ERROR, 0},
		{"INVALID_ENUM", INVALID_ENUM, 0x0500},
		{"INVALID_VALUE", INVALID_VALUE, 0x0501},
		{"INVALID_OPERATION", INVALID_OPERATION, 0x0502},
		{"OUT_OF_MEMORY", OUT_OF_MEMORY, 0x0505},

		// Buffer targets
		{"ARRAY_BUFFER", ARRAY_BUFFER, 0x8892},
		{"ELEMENT_ARRAY_BUFFER", ELEMENT_ARRAY_BUFFER, 0x8893},
		{"UNIFORM_BUFFER", UNIFORM_BUFFER, 0x8A11},
		{"COPY_READ_BUFFER", COPY_READ_BUFFER, 0x8F36},
		{"COPY_WRITE_BUFFER", COPY_WRITE_BUFFER, 0x8F37},

		// Buffer usage
		{"STATIC_DRAW", STATIC_DRAW, 0x88E4},
		{"DYNAMIC_DRAW", DYNAMIC_DRAW, 0x88E8},
		{"STREAM_DRAW", STREAM_DRAW, 0x88E0},

		// Texture targets
		{"TEXTURE_2D", TEXTURE_2D, 0x0DE1},
		{"TEXTURE_3D", TEXTURE_3D, 0x806F},
		{"TEXTURE_2D_ARRAY", TEXTURE_2D_ARRAY, 0x8C1A},
		{"TEXTURE_CUBE_MAP", TEXTURE_CUBE_MAP, 0x8513},

		// Shader types
		{"VERTEX_SHADER", VERTEX_SHADER, 0x8B31},
		{"FRAGMENT_SHADER", FRAGMENT_SHADER, 0x8B30},
		{"COMPUTE_SHADER", COMPUTE_SHADER, 0x91B9},

		// Compare functions
		{"NEVER", NEVER, 0x0200},
		{"LESS", LESS, 0x0201},
		{"EQUAL", EQUAL, 0x0202},
		{"LEQUAL", LEQUAL, 0x0203},
		{"GREATER", GREATER, 0x0204},
		{"NOTEQUAL", NOTEQUAL, 0x0205},
		{"GEQUAL", GEQUAL, 0x0206},
		{"ALWAYS", ALWAYS, 0x0207},

		// Primitive types
		{"POINTS", POINTS, 0x0000},
		{"LINES", LINES, 0x0001},
		{"LINE_STRIP", LINE_STRIP, 0x0003},
		{"TRIANGLES", TRIANGLES, 0x0004},
		{"TRIANGLE_STRIP", TRIANGLE_STRIP, 0x0005},
		{"TRIANGLE_FAN", TRIANGLE_FAN, 0x0006},

		// Face culling
		{"FRONT", FRONT, 0x0404},
		{"BACK", BACK, 0x0405},
		{"FRONT_AND_BACK", FRONT_AND_BACK, 0x0408},

		// Front face
		{"CW", CW, 0x0900},
		{"CCW", CCW, 0x0901},

		// Clear bits
		{"COLOR_BUFFER_BIT", COLOR_BUFFER_BIT, 0x4000},
		{"DEPTH_BUFFER_BIT", DEPTH_BUFFER_BIT, 0x0100},
		{"STENCIL_BUFFER_BIT", STENCIL_BUFFER_BIT, 0x0400},

		// Capabilities
		{"CULL_FACE", CULL_FACE, 0x0B44},
		{"DEPTH_TEST", DEPTH_TEST, 0x0B71},
		{"BLEND", BLEND, 0x0BE2},
		{"SCISSOR_TEST", SCISSOR_TEST, 0x0C11},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value != tt.want {
				t.Errorf("%s = %#x, want %#x", tt.name, tt.value, tt.want)
			}
		})
	}
}

func TestTextureFormatConstants(t *testing.T) {
	// Verify texture internal formats
	tests := []struct {
		name  string
		value uint32
		want  uint32
	}{
		{"R8", R8, 0x8229},
		{"RG8", RG8, 0x822B},
		{"RGB8", RGB8, 0x8051},
		{"RGBA8", RGBA8, 0x8058},
		{"SRGB8_ALPHA8", SRGB8_ALPHA8, 0x8C43},
		{"R16F", R16F, 0x822D},
		{"RG16F", RG16F, 0x822F},
		{"RGBA16F", RGBA16F, 0x881A},
		{"R32F", R32F, 0x822E},
		{"RG32F", RG32F, 0x8230},
		{"RGBA32F", RGBA32F, 0x8814},
		{"DEPTH_COMPONENT16", DEPTH_COMPONENT16, 0x81A5},
		{"DEPTH_COMPONENT24", DEPTH_COMPONENT24, 0x81A6},
		{"DEPTH_COMPONENT32", DEPTH_COMPONENT32, 0x81A7},
		{"DEPTH24_STENCIL8", DEPTH24_STENCIL8, 0x88F0},
		{"DEPTH32F_STENCIL8", DEPTH32F_STENCIL8, 0x8CAD},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value != tt.want {
				t.Errorf("%s = %#x, want %#x", tt.name, tt.value, tt.want)
			}
		})
	}
}

func TestPixelFormatConstants(t *testing.T) {
	tests := []struct {
		name  string
		value uint32
		want  uint32
	}{
		{"RED", RED, 0x1903},
		{"RG", RG, 0x8227},
		{"RGB", RGB, 0x1907},
		{"RGBA", RGBA, 0x1908},
		{"BGRA", BGRA, 0x80E1},
		{"DEPTH_COMPONENT", DEPTH_COMPONENT, 0x1902},
		{"DEPTH_STENCIL", DEPTH_STENCIL, 0x84F9},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value != tt.want {
				t.Errorf("%s = %#x, want %#x", tt.name, tt.value, tt.want)
			}
		})
	}
}

func TestShaderAndProgramConstants(t *testing.T) {
	tests := []struct {
		name  string
		value uint32
		want  uint32
	}{
		{"COMPILE_STATUS", COMPILE_STATUS, 0x8B81},
		{"LINK_STATUS", LINK_STATUS, 0x8B82},
		{"INFO_LOG_LENGTH", INFO_LOG_LENGTH, 0x8B84},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value != tt.want {
				t.Errorf("%s = %#x, want %#x", tt.name, tt.value, tt.want)
			}
		})
	}
}
