//go:build !(js && wasm)

package testutil

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/core"
	"github.com/gogpu/wgpu/hal"
)

// EnvOption configures a ValidationEnv.
type EnvOption func(*ValidationEnv)

// ValidationEnv is a per-test fixture for validation scenarios (P9).
// One instance per t.Run; safe for t.Parallel().
type ValidationEnv struct {
	T        *testing.T
	Features gputypes.Features
	Limits   gputypes.Limits
}

// NewValidationEnv creates a validation test fixture.
func NewValidationEnv(t *testing.T, opts ...EnvOption) *ValidationEnv {
	t.Helper()
	env := &ValidationEnv{
		T:      t,
		Limits: gputypes.DefaultLimits(),
	}
	for _, opt := range opts {
		opt(env)
	}
	return env
}

// WithFeatures sets the enabled feature set.
func WithFeatures(features gputypes.Features) EnvOption {
	return func(env *ValidationEnv) {
		env.Features = features
	}
}

// WithFeature enables a single feature.
func WithFeature(feature gputypes.Feature) EnvOption {
	return func(env *ValidationEnv) {
		env.Features.Insert(feature)
	}
}

// WithLimits sets device limits used by Validate* calls.
func WithLimits(limits gputypes.Limits) EnvOption {
	return func(env *ValidationEnv) {
		env.Limits = limits
	}
}

// CreateTextureWithFormat validates texture creation for the given format and usage.
func (e *ValidationEnv) CreateTextureWithFormat(format gputypes.TextureFormat, usage gputypes.TextureUsage) error {
	desc := &hal.TextureDescriptor{
		Label:         "validation-env-texture",
		Size:          hal.Extent3D{Width: 64, Height: 64, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        format,
		Usage:         usage,
	}
	return core.ValidateTextureDescriptor(desc, e.Limits, e.Features)
}

// CreateRenderPipelineUnclippedDepth validates unclipped depth pipeline creation (VAL-C5).
func (e *ValidationEnv) CreateRenderPipelineUnclippedDepth() error {
	desc := &hal.RenderPipelineDescriptor{
		Label: "validation-env-unclipped",
		Vertex: hal.VertexState{
			Module:     struct{ hal.ShaderModule }{},
			EntryPoint: "vs_main",
		},
		Primitive:   gputypes.PrimitiveState{UnclippedDepth: true},
		Multisample: gputypes.MultisampleState{Count: 1},
	}
	return core.ValidateRenderPipelineDescriptor(desc, e.Limits, e.Features)
}

// CreateQuerySetTimestamp validates timestamp query set creation (VAL-C24).
func (e *ValidationEnv) CreateQuerySetTimestamp() error {
	return core.ValidateQuerySetDescriptor(&hal.QuerySetDescriptor{
		Type:  hal.QueryTypeTimestamp,
		Count: 2,
	}, e.Features)
}

// CreateShaderModuleWithWGSL validates shader module creation for WGSL source (VAL-C12/C19/C20).
func (e *ValidationEnv) CreateShaderModuleWithWGSL(source string) error {
	return core.ValidateShaderModuleDescriptor(&hal.ShaderModuleDescriptor{
		Label:  "validation-env-shader",
		Source: hal.ShaderSource{WGSL: source},
	}, e.Features)
}

// CreateBindGroupWithR32FloatTexture validates float32-filterable bind rules (VAL-C21).
func (e *ValidationEnv) CreateBindGroupWithR32FloatTexture() error {
	layoutEntries := []gputypes.BindGroupLayoutEntry{
		{
			Binding: 0,
			Texture: &gputypes.TextureBindingLayout{SampleType: gputypes.TextureSampleTypeFloat},
		},
	}
	desc := &hal.BindGroupDescriptor{
		Label:  "validation-env-bind-group",
		Layout: struct{ hal.BindGroupLayout }{},
		Entries: []gputypes.BindGroupEntry{
			{Binding: 0},
		},
	}
	textures := []core.BindGroupTextureInfo{
		{Binding: 0, Format: gputypes.TextureFormatR32Float},
	}
	return core.ValidateBindGroupDescriptor(desc, layoutEntries, nil, nil, textures, e.Limits, e.Features)
}

// CreatePipelineLayoutWithPushConstants validates a pipeline layout descriptor
// that declares push constant ranges (VAL-C4).
func (e *ValidationEnv) CreatePipelineLayoutWithPushConstants() error {
	desc := &hal.PipelineLayoutDescriptor{
		Label: "validation-env-push-constants",
		PushConstantRanges: []hal.PushConstantRange{
			{
				Stages: gputypes.ShaderStageVertex,
				Range:  hal.Range{Start: 0, End: 4},
			},
		},
	}
	return core.ValidatePipelineLayoutDescriptor(desc, e.Limits, e.Features)
}
