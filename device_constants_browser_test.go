//go:build js && wasm

package wgpu

import (
	"errors"
	"math"
	"testing"

	"github.com/gogpu/wgpu/internal/pipeconst"
)

// TestBrowserPipelineConstants checks that a bad pipeline constant key or
// value is an error on every stage before it reaches JS, where a NaN or
// Infinity would throw a TypeError.
func TestBrowserPipelineConstants(t *testing.T) {
	type consts = map[string]float64
	tests := []struct {
		name     string
		vertex   consts
		fragment consts
		compute  consts
		wantIs   error // nil = accepted
	}{
		{name: "none"},
		{name: "set", vertex: consts{"a": 1}, fragment: consts{"1000": 0.5}, compute: consts{"b": -3}},
		{name: "vertex NaN", vertex: consts{"a": math.NaN()}, wantIs: pipeconst.ErrNotFinite},
		{name: "fragment -Inf", fragment: consts{"a": math.Inf(-1)}, wantIs: pipeconst.ErrNotFinite},
		{name: "compute Inf", compute: consts{"a": math.Inf(1)}, wantIs: pipeconst.ErrNotFinite},
		{name: "fragment empty key", fragment: consts{"": 1}, wantIs: pipeconst.ErrEmptyKey},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			render := &RenderPipelineDescriptor{
				Vertex:   VertexState{Constants: tt.vertex},
				Fragment: &FragmentState{Constants: tt.fragment},
			}
			err := errors.Join(checkBrowserRenderConstants(render), checkBrowserStageConstants("compute", tt.compute))
			if tt.wantIs == nil {
				if err != nil {
					t.Fatalf("error = %v, want none", err)
				}
				return
			}
			if !errors.Is(err, tt.wantIs) {
				t.Fatalf("error = %v, want %v", err, tt.wantIs)
			}
		})
	}
}
