//go:build rust

package wgpu

import (
	"errors"
	"math"
	"testing"

	"github.com/gogpu/wgpu/internal/pipeconst"
)

// TestRustPipelineConstants checks that pipeline constants on any stage are
// refused under -tags rust instead of being dropped (go-webgpu does not map
// them yet), and that a bad key or value gets the same error as every other
// build first.
func TestRustPipelineConstants(t *testing.T) {
	type consts = map[string]float64
	tests := []struct {
		name     string
		vertex   consts
		fragment consts
		compute  consts
		wantIs   error // nil = accepted
	}{
		{name: "none"},
		{name: "empty maps", vertex: consts{}, fragment: consts{}, compute: consts{}},
		{name: "vertex", vertex: consts{"a": 1}, wantIs: errRustConstantsUnmapped},
		{name: "fragment", fragment: consts{"a": 1}, wantIs: errRustConstantsUnmapped},
		{name: "compute", compute: consts{"a": 1}, wantIs: errRustConstantsUnmapped},
		{name: "vertex NaN", vertex: consts{"a": math.NaN()}, wantIs: pipeconst.ErrNotFinite},
		{name: "fragment Inf", fragment: consts{"a": math.Inf(1)}, wantIs: pipeconst.ErrNotFinite},
		{name: "compute empty key", compute: consts{"": 1}, wantIs: pipeconst.ErrEmptyKey},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			render := &RenderPipelineDescriptor{
				Vertex:   VertexState{Constants: tt.vertex},
				Fragment: &FragmentState{Constants: tt.fragment},
			}
			err := errors.Join(checkRustRenderConstants(render), checkRustStageConstants("compute", tt.compute))
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
	// A pipeline without a fragment stage checks only the vertex stage.
	if err := checkRustRenderConstants(&RenderPipelineDescriptor{}); err != nil {
		t.Fatalf("no fragment stage: %v", err)
	}
}
