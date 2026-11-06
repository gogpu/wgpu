package core

import (
	"errors"
	"strings"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// ignoreFace is the WebGPU default stencil face (compare always, ops keep).
var ignoreFace = hal.StencilFaceState{
	Compare: gputypes.CompareFunctionAlways,
	FailOp:  hal.StencilOperationKeep, DepthFailOp: hal.StencilOperationKeep, PassOp: hal.StencilOperationKeep,
}

func depthStencilPipeline(ds *hal.DepthStencilState) *hal.RenderPipelineDescriptor {
	return &hal.RenderPipelineDescriptor{
		Label:        "test",
		Vertex:       hal.VertexState{Module: mockShaderModule{}, EntryPoint: "vs_main"},
		DepthStencil: ds,
		Multisample:  gputypes.MultisampleState{Count: 1},
	}
}

// TestValidateRenderPipelineDescriptor_DepthCompare covers
// GPUDepthStencilState.depthCompare, which has no default (index.bs,
// "validating GPUDepthStencilState"): CompareFunctionUndefined means "not
// provided". It is required when the format has a depth aspect and either
// depth writes are on or a face's depthFailOp is not keep; otherwise an
// absent compare passes every fragment, as "always" does (Rust wgpu-core
// MissingDepthCompare, wgpu-types is_depth_enabled unwrap_or_default).
func TestValidateRenderPipelineDescriptor_DepthCompare(t *testing.T) {
	replaceOnDepthFail := ignoreFace
	replaceOnDepthFail.DepthFailOp = hal.StencilOperationReplace
	undefinedDepthFail := ignoreFace
	undefinedDepthFail.DepthFailOp = gputypes.StencilOperationUndefined

	for _, c := range []struct {
		name     string
		ds       hal.DepthStencilState
		wantKind CreateRenderPipelineErrorKind
		wantOK   bool
	}{
		{
			name:   "Stencil8 without depthCompare is not depth enabled",
			ds:     hal.DepthStencilState{Format: gputypes.TextureFormatStencil8, StencilFront: ignoreFace, StencilBack: ignoreFace},
			wantOK: true,
		},
		{
			name:   "depth format, no writes, no depthCompare",
			ds:     hal.DepthStencilState{Format: gputypes.TextureFormatDepth24Plus, StencilFront: ignoreFace, StencilBack: ignoreFace},
			wantOK: true,
		},
		{
			name:     "depth writes without depthCompare",
			ds:       hal.DepthStencilState{Format: gputypes.TextureFormatDepth24Plus, DepthWriteEnabled: true, StencilFront: ignoreFace, StencilBack: ignoreFace},
			wantKind: CreateRenderPipelineErrorMissingDepthCompare,
		},
		{
			name: "front depthFailOp replace without depthCompare",
			ds: hal.DepthStencilState{Format: gputypes.TextureFormatDepth24PlusStencil8, StencilFront: replaceOnDepthFail, StencilBack: ignoreFace,
				StencilReadMask: 0xFF, StencilWriteMask: 0xFF},
			wantKind: CreateRenderPipelineErrorMissingDepthCompare,
		},
		{
			name: "back depthFailOp replace without depthCompare",
			ds: hal.DepthStencilState{Format: gputypes.TextureFormatDepth24PlusStencil8, StencilFront: ignoreFace, StencilBack: replaceOnDepthFail,
				StencilReadMask: 0xFF, StencilWriteMask: 0xFF},
			wantKind: CreateRenderPipelineErrorMissingDepthCompare,
		},
		{
			name:   "an undefined depthFailOp is keep",
			ds:     hal.DepthStencilState{Format: gputypes.TextureFormatDepth24PlusStencil8, StencilFront: undefinedDepthFail, StencilBack: undefinedDepthFail},
			wantOK: true,
		},
		{
			name: "depth writes with an explicit compare",
			ds: hal.DepthStencilState{Format: gputypes.TextureFormatDepth24Plus, DepthWriteEnabled: true, DepthCompare: gputypes.CompareFunctionLess,
				StencilFront: ignoreFace, StencilBack: ignoreFace},
			wantOK: true,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			ds := c.ds
			err := ValidateRenderPipelineDescriptor(depthStencilPipeline(&ds), gputypes.DefaultLimits(), gputypes.Features(0))
			if c.wantOK {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			var crpe *CreateRenderPipelineError
			if !errors.As(err, &crpe) || crpe.Kind != c.wantKind {
				t.Fatalf("got %v, want kind %v", err, c.wantKind)
			}
			if crpe.Format != ds.Format.String() || !strings.Contains(crpe.Error(), "depthCompare") {
				t.Errorf("error %q, format %q", crpe.Error(), crpe.Format)
			}
		})
	}
}
