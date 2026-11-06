package wgpu

import "github.com/gogpu/gputypes"

// The WebGPU defaults of the render pipeline enum fields whose Go zero value
// is an Undefined member the spec does not have (ADR-080 Rule 1):
//
//   - GPUStencilFaceState: compare "always"; failOp, depthFailOp and passOp
//     "keep" (index.bs, dictionary GPUStencilFaceState).
//   - GPUBlendComponent: operation "add", srcFactor "one", dstFactor "zero"
//     (index.bs, dictionary GPUBlendComponent); Rust wgpu-types
//     BlendComponent::REPLACE.
//
// GPUCompareFunction, GPUStencilOperation, GPUBlendFactor and
// GPUBlendOperation have no "undefined" member, so the Go zero can never be
// an explicit spec value and always means "not set". Every explicit value is
// non-zero in gputypes and passes through unchanged.
//
// Every CreateRenderPipeline path (native, -tags rust, js/wasm) applies these
// in its descriptor conversion, before validation and before the value
// reaches a backend, wgpu-native or the browser. The HAL takes the values
// literally. GPUDepthStencilState.depthCompare is not here: it has no
// default, its absence is meaningful (an error when required, otherwise the
// test passes), and the native path resolves it after core validation.
const (
	stencilCompareDefault = gputypes.CompareFunctionAlways
	stencilOpDefault      = gputypes.StencilOperationKeep
	blendSrcFactorDefault = gputypes.BlendFactorOne
	blendDstFactorDefault = gputypes.BlendFactorZero
	blendOpDefault        = gputypes.BlendOperationAdd
)

// normalizeStencilFace maps the Undefined members of a stencil face state to
// the spec defaults, so a zero StencilFaceState ignores the stencil (Rust
// StencilFaceState::IGNORE).
func normalizeStencilFace(s StencilFaceState) StencilFaceState {
	if s.Compare == gputypes.CompareFunctionUndefined {
		s.Compare = stencilCompareDefault
	}
	s.FailOp = normalizeStencilOp(s.FailOp)
	s.DepthFailOp = normalizeStencilOp(s.DepthFailOp)
	s.PassOp = normalizeStencilOp(s.PassOp)
	return s
}

func normalizeStencilOp(op gputypes.StencilOperation) gputypes.StencilOperation {
	if op == gputypes.StencilOperationUndefined {
		return stencilOpDefault
	}
	return op
}

// normalizeDepthStencilState returns a copy of ds with both stencil faces
// normalized, or nil for nil. The caller's descriptor is not modified.
func normalizeDepthStencilState(ds *DepthStencilState) *DepthStencilState {
	if ds == nil {
		return nil
	}
	out := *ds
	out.StencilFront = normalizeStencilFace(ds.StencilFront)
	out.StencilBack = normalizeStencilFace(ds.StencilBack)
	return &out
}

// normalizeBlendComponent maps the Undefined members of a blend component to
// the spec defaults per field: a zero BlendComponent replaces the destination
// with the source.
func normalizeBlendComponent(c gputypes.BlendComponent) gputypes.BlendComponent {
	if c.SrcFactor == gputypes.BlendFactorUndefined {
		c.SrcFactor = blendSrcFactorDefault
	}
	if c.DstFactor == gputypes.BlendFactorUndefined {
		c.DstFactor = blendDstFactorDefault
	}
	if c.Operation == gputypes.BlendOperationUndefined {
		c.Operation = blendOpDefault
	}
	return c
}

// normalizeColorTargets returns a copy of targets whose blend states are
// normalized; a nil Blend (no blending) stays nil. The caller's slice and
// blend states are not modified.
func normalizeColorTargets(targets []gputypes.ColorTargetState) []gputypes.ColorTargetState {
	if targets == nil {
		return nil
	}
	out := make([]gputypes.ColorTargetState, len(targets))
	for i, t := range targets {
		if t.Blend != nil {
			b := gputypes.BlendState{
				Color: normalizeBlendComponent(t.Blend.Color),
				Alpha: normalizeBlendComponent(t.Blend.Alpha),
			}
			t.Blend = &b
		}
		out[i] = t
	}
	return out
}
