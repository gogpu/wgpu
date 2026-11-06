//go:build !(js && wasm)

package core

import (
	"errors"
	"math"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/internal/pipeconst"
)

// TestValidateRenderPipelineDescriptor_PipelineConstants checks RP11: the
// constants of each render stage must have non-empty keys and finite values
// (WebIDL double), reported with the stage's name.
func TestValidateRenderPipelineDescriptor_PipelineConstants(t *testing.T) {
	type consts = map[string]float64
	tests := []struct {
		name      string
		vertex    consts
		fragment  consts
		noFrag    bool
		wantStage string // "" = valid
		wantIs    error
	}{
		{name: "none"},
		{name: "finite", vertex: consts{"a": -1.5}, fragment: consts{"1000": 1e30}},
		{name: "vertex NaN", vertex: consts{"a": math.NaN()}, wantStage: "vertex", wantIs: pipeconst.ErrNotFinite},
		{name: "vertex empty key", vertex: consts{"": 0}, wantStage: "vertex", wantIs: pipeconst.ErrEmptyKey},
		{name: "fragment +Inf", fragment: consts{"a": math.Inf(1)}, wantStage: "fragment", wantIs: pipeconst.ErrNotFinite},
		{name: "fragment empty key", fragment: consts{"": 0}, wantStage: "fragment", wantIs: pipeconst.ErrEmptyKey},
		{name: "vertex checked first", vertex: consts{"": 0}, fragment: consts{"a": math.NaN()}, wantStage: "vertex", wantIs: pipeconst.ErrEmptyKey},
		{name: "no fragment stage", vertex: consts{"a": 1}, noFrag: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desc := &hal.RenderPipelineDescriptor{
				Label:       "rp",
				Vertex:      hal.VertexState{Module: mockShaderModule{}, EntryPoint: "vs", Constants: tt.vertex},
				Multisample: gputypes.MultisampleState{Count: 1},
			}
			if !tt.noFrag {
				desc.Fragment = &hal.FragmentState{
					Module: mockShaderModule{}, EntryPoint: "fs", Constants: tt.fragment,
					Targets: []gputypes.ColorTargetState{{Format: gputypes.TextureFormatRGBA8Unorm}},
				}
			} else {
				desc.DepthStencil = &hal.DepthStencilState{Format: gputypes.TextureFormatDepth32Float}
			}
			err := ValidateRenderPipelineDescriptor(desc, gputypes.DefaultLimits(), gputypes.Features(0))
			if tt.wantStage == "" {
				if err != nil {
					t.Fatalf("expected nil error, got: %v", err)
				}
				return
			}
			var crpe *CreateRenderPipelineError
			if !errors.As(err, &crpe) || crpe.Kind != CreateRenderPipelineErrorPipelineConstants {
				t.Fatalf("error %v (%T), want kind PipelineConstants", err, err)
			}
			if crpe.Stage != tt.wantStage {
				t.Errorf("stage %q, want %q", crpe.Stage, tt.wantStage)
			}
			if !errors.Is(err, tt.wantIs) {
				t.Errorf("error %v does not match %v", err, tt.wantIs)
			}
		})
	}
}

// TestValidateComputePipelineDescriptor_PipelineConstants checks CP3, the
// compute stage's share of the same check.
func TestValidateComputePipelineDescriptor_PipelineConstants(t *testing.T) {
	for _, tt := range []struct {
		name      string
		constants map[string]float64
		wantIs    error // nil = valid
	}{
		{name: "finite", constants: map[string]float64{"wg": 64}},
		{name: "NaN", constants: map[string]float64{"wg": math.NaN()}, wantIs: pipeconst.ErrNotFinite},
		{name: "-Inf", constants: map[string]float64{"wg": math.Inf(-1)}, wantIs: pipeconst.ErrNotFinite},
		{name: "empty key", constants: map[string]float64{"": 1}, wantIs: pipeconst.ErrEmptyKey},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateComputePipelineDescriptor(&hal.ComputePipelineDescriptor{
				Label:   "cp",
				Compute: hal.ComputeState{Module: mockShaderModule{}, EntryPoint: "main", Constants: tt.constants},
			})
			if tt.wantIs == nil {
				if err != nil {
					t.Fatalf("expected nil error, got: %v", err)
				}
				return
			}
			var ccpe *CreateComputePipelineError
			if !errors.As(err, &ccpe) || ccpe.Kind != CreateComputePipelineErrorPipelineConstants {
				t.Fatalf("error %v (%T), want kind PipelineConstants", err, err)
			}
			if !errors.Is(err, tt.wantIs) {
				t.Errorf("error %v does not match %v", err, tt.wantIs)
			}
		})
	}
}
