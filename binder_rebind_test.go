//go:build !rust && !(js && wasm)

package wgpu

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/hal/noop"
	"github.com/gogpu/wgpu/internal/resource"
)

// rebindLayout returns a layout with one uniform buffer entry; layouts made
// with the same dynamic flag are compatible, entry by entry.
func rebindLayout(dynamic bool) *resource.BindGroupLayout {
	return resource.NewBindGroupLayout(nil, nil, []gputypes.BindGroupLayoutEntry{{
		Binding:    0,
		Visibility: gputypes.ShaderStageCompute,
		Buffer:     &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform, HasDynamicOffset: dynamic},
	}})
}

func rebindGroup(layout *resource.BindGroupLayout) *resource.BindGroup {
	return resource.NewBindGroup(&resource.BindGroupInit{HAL: &testHALResource{}, Layout: layout})
}

// TestExecuteBundlesResetsPassState checks that ExecuteBundles clears the
// pipeline and the bind groups, as WebGPU's executeBundles and Rust wgpu-core
// State::reset_bundle do: a draw after it needs SetPipeline again.
//
// The pass state itself lives in internal/encoder after the S5 Move; the
// assertions go through the declaration layer's test hooks, which expose
// exactly the three fields the reset clears (the binder white-box tests
// themselves moved into the destination with the cluster).
func TestExecuteBundlesResetsPassState(t *testing.T) {
	pass := &captureRenderPass{RenderPassEncoder: &noop.RenderPassEncoder{}}
	command := &captureCommandEncoder{CommandEncoder: &noop.CommandEncoder{}, renderPass: pass}
	device := newCaptureDevice(&captureDevice{Device: &noop.Device{}, commandEncoder: command})
	encoder, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder() error = %v", err)
	}
	renderPass, err := encoder.BeginRenderPass(&RenderPassDescriptor{})
	if err != nil {
		t.Fatalf("BeginRenderPass() error = %v", err)
	}
	l := rebindLayout(false)
	renderPass.SetPassStateForTest(l, rebindGroup(l))

	renderPass.ExecuteBundles(newEmptyBundle(device))
	if set, group, assigned := renderPass.PassStateForTest(); set || group != nil || assigned != nil {
		t.Fatal("ExecuteBundles kept the pipeline or the bind groups")
	}
	renderPass.Draw(gputypes.DrawArgs{VertexCount: 3, InstanceCount: 1})
	_ = renderPass.End()
	if _, err := encoder.Finish(); !errors.Is(err, ErrDrawMissingPipeline) {
		t.Fatalf("Finish() error = %v, want ErrDrawMissingPipeline", err)
	}
}
