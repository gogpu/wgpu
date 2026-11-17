//go:build !(js && wasm)

package track

import (
	"fmt"
	"testing"
)

func allSingleBitTextureUses() []TextureUses {
	return []TextureUses{
		TextureUsesUninitialized,
		TextureUsesPresent,
		TextureUsesCopySrc,
		TextureUsesCopyDst,
		TextureUsesResource,
		TextureUsesColorTarget,
		TextureUsesDepthStencilRead,
		TextureUsesDepthStencilWrite,
		TextureUsesStorageRead,
		TextureUsesStorageWrite,
	}
}

func referenceCompatible(a, b TextureUses) bool {
	if a.IsEmpty() || b.IsEmpty() {
		return true
	}
	combined := a | b
	if combined.IsExclusive() {
		return isPowerOfTwo(uint32(combined))
	}
	return true
}

func pairName(a, b TextureUses) string {
	return fmt.Sprintf("%04b_%04b", a, b)
}

func TestTextureUses_IsCompatible_Exhaustive(t *testing.T) {
	t.Parallel()
	all := allSingleBitTextureUses()
	for _, a := range all {
		for _, b := range all {
			t.Run(pairName(a, b), func(t *testing.T) {
				t.Parallel()
				got := a.IsCompatible(b)
				want := referenceCompatible(a, b)
				if got != want {
					t.Errorf("(%v, %v): IsCompatible = %v, reference = %v", a, b, got, want)
				}
			})
		}
	}
}

func TestTextureUses_IsCompatible_MatchesImplementationRule(t *testing.T) {
	t.Parallel()
	// Hand-picked pairs documented against Rust wgpu-core track/texture.rs.
	tests := []struct {
		a, b TextureUses
		want bool
	}{
		{TextureUsesCopySrc, TextureUsesResource, true},
		{TextureUsesCopySrc, TextureUsesCopyDst, false},
		{TextureUsesColorTarget, TextureUsesColorTarget, true},
		{TextureUsesCopyDst, TextureUsesColorTarget, false},
		{TextureUsesStorageRead, TextureUsesStorageWrite, false},
	}
	for _, tt := range tests {
		t.Run(pairName(tt.a, tt.b), func(t *testing.T) {
			if got := tt.a.IsCompatible(tt.b); got != tt.want {
				t.Errorf("got %v want %v", got, tt.want)
			}
		})
	}
}
