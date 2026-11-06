package wgpu

import (
	"testing"

	"github.com/gogpu/gputypes"
)

// The tables below list every member of each enum: only Undefined (the Go
// zero, which no spec value has) maps to the WebGPU default; every explicit
// value passes through unchanged.

func TestNormalizeStencilFaceCompare(t *testing.T) {
	for _, c := range []struct {
		in, want gputypes.CompareFunction
	}{
		{gputypes.CompareFunctionUndefined, gputypes.CompareFunctionAlways},
		{gputypes.CompareFunctionNever, gputypes.CompareFunctionNever},
		{gputypes.CompareFunctionLess, gputypes.CompareFunctionLess},
		{gputypes.CompareFunctionEqual, gputypes.CompareFunctionEqual},
		{gputypes.CompareFunctionLessEqual, gputypes.CompareFunctionLessEqual},
		{gputypes.CompareFunctionGreater, gputypes.CompareFunctionGreater},
		{gputypes.CompareFunctionNotEqual, gputypes.CompareFunctionNotEqual},
		{gputypes.CompareFunctionGreaterEqual, gputypes.CompareFunctionGreaterEqual},
		{gputypes.CompareFunctionAlways, gputypes.CompareFunctionAlways},
	} {
		got := normalizeStencilFace(StencilFaceState{Compare: c.in}).Compare
		if got != c.want {
			t.Errorf("stencil compare %v: got %v, want %v", c.in, got, c.want)
		}
	}
}

func TestNormalizeStencilFaceOps(t *testing.T) {
	for _, c := range []struct {
		in, want gputypes.StencilOperation
	}{
		{gputypes.StencilOperationUndefined, gputypes.StencilOperationKeep},
		{gputypes.StencilOperationKeep, gputypes.StencilOperationKeep},
		{gputypes.StencilOperationZero, gputypes.StencilOperationZero},
		{gputypes.StencilOperationReplace, gputypes.StencilOperationReplace},
		{gputypes.StencilOperationInvert, gputypes.StencilOperationInvert},
		{gputypes.StencilOperationIncrementClamp, gputypes.StencilOperationIncrementClamp},
		{gputypes.StencilOperationDecrementClamp, gputypes.StencilOperationDecrementClamp},
		{gputypes.StencilOperationIncrementWrap, gputypes.StencilOperationIncrementWrap},
		{gputypes.StencilOperationDecrementWrap, gputypes.StencilOperationDecrementWrap},
	} {
		// Each op field on its own, with the others set to Zero, so a field
		// mixed up with another fails.
		z := gputypes.StencilOperationZero
		always := gputypes.CompareFunctionAlways
		if got := normalizeStencilFace(StencilFaceState{Compare: always, FailOp: c.in, DepthFailOp: z, PassOp: z}); got.FailOp != c.want || got.DepthFailOp != z || got.PassOp != z {
			t.Errorf("failOp %v: got %+v, want failOp %v", c.in, got, c.want)
		}
		if got := normalizeStencilFace(StencilFaceState{Compare: always, FailOp: z, DepthFailOp: c.in, PassOp: z}); got.DepthFailOp != c.want || got.FailOp != z || got.PassOp != z {
			t.Errorf("depthFailOp %v: got %+v, want depthFailOp %v", c.in, got, c.want)
		}
		if got := normalizeStencilFace(StencilFaceState{Compare: always, FailOp: z, DepthFailOp: z, PassOp: c.in}); got.PassOp != c.want || got.FailOp != z || got.DepthFailOp != z {
			t.Errorf("passOp %v: got %+v, want passOp %v", c.in, got, c.want)
		}
	}
}

func TestNormalizeBlendFactors(t *testing.T) {
	explicit := []gputypes.BlendFactor{
		gputypes.BlendFactorZero, gputypes.BlendFactorOne, gputypes.BlendFactorSrc,
		gputypes.BlendFactorOneMinusSrc, gputypes.BlendFactorSrcAlpha, gputypes.BlendFactorOneMinusSrcAlpha,
		gputypes.BlendFactorDst, gputypes.BlendFactorOneMinusDst, gputypes.BlendFactorDstAlpha,
		gputypes.BlendFactorOneMinusDstAlpha, gputypes.BlendFactorSrcAlphaSaturated,
		gputypes.BlendFactorConstant, gputypes.BlendFactorOneMinusConstant,
	}
	check := func(in, wantSrc, wantDst gputypes.BlendFactor) {
		t.Helper()
		op := gputypes.BlendOperationAdd
		if got := normalizeBlendComponent(gputypes.BlendComponent{SrcFactor: in, DstFactor: gputypes.BlendFactorOne, Operation: op}).SrcFactor; got != wantSrc {
			t.Errorf("srcFactor %v: got %v, want %v", in, got, wantSrc)
		}
		if got := normalizeBlendComponent(gputypes.BlendComponent{SrcFactor: gputypes.BlendFactorZero, DstFactor: in, Operation: op}).DstFactor; got != wantDst {
			t.Errorf("dstFactor %v: got %v, want %v", in, got, wantDst)
		}
	}
	// The defaults differ per field: srcFactor "one", dstFactor "zero".
	check(gputypes.BlendFactorUndefined, gputypes.BlendFactorOne, gputypes.BlendFactorZero)
	for _, f := range explicit {
		check(f, f, f)
	}
}

func TestNormalizeBlendOperation(t *testing.T) {
	for _, c := range []struct {
		in, want gputypes.BlendOperation
	}{
		{gputypes.BlendOperationUndefined, gputypes.BlendOperationAdd},
		{gputypes.BlendOperationAdd, gputypes.BlendOperationAdd},
		{gputypes.BlendOperationSubtract, gputypes.BlendOperationSubtract},
		{gputypes.BlendOperationReverseSubtract, gputypes.BlendOperationReverseSubtract},
		{gputypes.BlendOperationMin, gputypes.BlendOperationMin},
		{gputypes.BlendOperationMax, gputypes.BlendOperationMax},
	} {
		comp := gputypes.BlendComponent{SrcFactor: gputypes.BlendFactorSrcAlpha, DstFactor: gputypes.BlendFactorDst, Operation: c.in}
		if got := normalizeBlendComponent(comp).Operation; got != c.want {
			t.Errorf("operation %v: got %v, want %v", c.in, got, c.want)
		}
	}
}

// TestNormalizeDoesNotMutate checks that normalizing copies: the caller's
// descriptor, target slice and blend state keep their zero values, so a
// descriptor reused for another pipeline is unchanged.
func TestNormalizeDoesNotMutate(t *testing.T) {
	ds := &DepthStencilState{Format: gputypes.TextureFormatDepth24PlusStencil8}
	blend := &gputypes.BlendState{}
	targets := []gputypes.ColorTargetState{{Format: gputypes.TextureFormatRGBA8Unorm, Blend: blend}, {}}

	nds := normalizeDepthStencilState(ds)
	nt := normalizeColorTargets(targets)

	if ds.StencilFront != (StencilFaceState{}) || ds.StencilBack != (StencilFaceState{}) {
		t.Errorf("caller's DepthStencilState changed: %+v", ds)
	}
	if *blend != (gputypes.BlendState{}) || targets[0].Blend != blend {
		t.Errorf("caller's blend state changed: %+v", *blend)
	}
	if nds == ds || nt[0].Blend == blend {
		t.Error("normalized values alias the caller's")
	}
	if nds.StencilFront.Compare != gputypes.CompareFunctionAlways || nds.StencilBack.PassOp != gputypes.StencilOperationKeep {
		t.Errorf("normalized stencil faces %+v, %+v", nds.StencilFront, nds.StencilBack)
	}
	wantBlend := gputypes.BlendComponent{SrcFactor: gputypes.BlendFactorOne, DstFactor: gputypes.BlendFactorZero, Operation: gputypes.BlendOperationAdd}
	if nt[0].Blend.Color != wantBlend || nt[0].Blend.Alpha != wantBlend {
		t.Errorf("normalized blend %+v", *nt[0].Blend)
	}
	if nt[1].Blend != nil {
		t.Error("a nil Blend (no blending) became non-nil")
	}
	if normalizeDepthStencilState(nil) != nil || normalizeColorTargets(nil) != nil {
		t.Error("nil inputs must stay nil")
	}
}
