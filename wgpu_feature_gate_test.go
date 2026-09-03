//go:build !rust && !(js && wasm)

package wgpu_test

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
	"github.com/gogpu/wgpu/core"
)

func newDeviceWithoutFeatures(t *testing.T) (*wgpu.Instance, *wgpu.Adapter, *wgpu.Device) {
	t.Helper()
	inst, adapter := newAdapter(t)
	device, err := adapter.RequestDevice(&wgpu.DeviceDescriptor{
		RequiredFeatures: gputypes.Features(0),
	})
	if err != nil {
		t.Fatalf("RequestDevice: %v", err)
	}
	return inst, adapter, device
}

func newDeviceWithFeatures(t *testing.T, features gputypes.Features) (*wgpu.Instance, *wgpu.Adapter, *wgpu.Device) {
	t.Helper()
	inst, adapter := newAdapter(t)
	if !adapter.Features().ContainsAll(features) {
		t.Skipf("adapter does not support required features: %v", features)
	}
	device, err := adapter.RequestDevice(&wgpu.DeviceDescriptor{
		RequiredFeatures: features,
	})
	if err != nil {
		t.Fatalf("RequestDevice: %v", err)
	}
	return inst, adapter, device
}

func newEncoderWithRenderPassForDevice(t *testing.T, device *wgpu.Device) (*wgpu.CommandEncoder, *wgpu.RenderPassEncoder) {
	t.Helper()
	requireHAL(t, device)

	encoder, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}

	pass, err := encoder.BeginRenderPass(&wgpu.RenderPassDescriptor{
		Label: "feature-gate-pass",
		ColorAttachments: []wgpu.RenderPassColorAttachment{
			{
				LoadOp:     gputypes.LoadOpClear,
				StoreOp:    gputypes.StoreOpStore,
				ClearValue: gputypes.Color{R: 0, G: 0, B: 0, A: 1},
			},
		},
	})
	if err != nil {
		t.Fatalf("BeginRenderPass: %v", err)
	}
	return encoder, pass
}

func TestFeatureGate_MultiDrawIndirect_NoFeature(t *testing.T) {
	_, _, device := newDeviceWithoutFeatures(t)
	defer device.Release()
	requireHAL(t, device)

	encoder, pass := newEncoderWithRenderPassForDevice(t, device)
	pipeline := &wgpu.RenderPipeline{}
	pipeline.SetTestRequiredVertexBuffers(0)
	pass.SetPipeline(pipeline)

	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "multi-draw-indirect",
		Size:  32,
		Usage: wgpu.BufferUsageIndirect,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer buf.Release()

	pass.MultiDrawIndirect(buf, 0, 2)
	_ = pass.End()
	_, finishErr := encoder.Finish()
	if finishErr == nil {
		t.Fatal("Finish() should fail without FeatureMultiDrawIndirect")
	}
	var fe *core.FeatureError
	if !errors.As(finishErr, &fe) {
		t.Fatalf("error = %T (%v), want *core.FeatureError", finishErr, finishErr)
	}
	if fe.Feature != gputypes.FeatureMultiDrawIndirect.String() {
		t.Errorf("Feature = %q, want %q", fe.Feature, gputypes.FeatureMultiDrawIndirect.String())
	}
}

func TestFeatureGate_MultiDrawIndirect_WithFeature(t *testing.T) {
	features := gputypes.Features(gputypes.FeatureMultiDrawIndirect)
	_, _, device := newDeviceWithFeatures(t, features)
	defer device.Release()
	requireHAL(t, device)

	encoder, pass := newEncoderWithRenderPassForDevice(t, device)
	pipeline := &wgpu.RenderPipeline{}
	pipeline.SetTestRequiredVertexBuffers(0)
	pass.SetPipeline(pipeline)

	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "multi-draw-indirect",
		Size:  32,
		Usage: wgpu.BufferUsageIndirect,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer buf.Release()

	pass.MultiDrawIndirect(buf, 0, 2)
	if err := pass.End(); err != nil {
		t.Fatalf("End: %v", err)
	}
	if _, err := encoder.Finish(); err != nil {
		t.Fatalf("Finish() unexpected error: %v", err)
	}
}

func TestFeatureGate_MultiDrawIndirect_SingleDrawAllowedWithoutFeature(t *testing.T) {
	_, _, device := newDeviceWithoutFeatures(t)
	defer device.Release()
	requireHAL(t, device)

	encoder, pass := newEncoderWithRenderPassForDevice(t, device)
	pipeline := &wgpu.RenderPipeline{}
	pipeline.SetTestRequiredVertexBuffers(0)
	pass.SetPipeline(pipeline)

	buf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "single-draw-indirect",
		Size:  16,
		Usage: wgpu.BufferUsageIndirect,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer buf.Release()

	pass.MultiDrawIndirect(buf, 0, 1)
	if err := pass.End(); err != nil {
		t.Fatalf("End: %v", err)
	}
	if _, err := encoder.Finish(); err != nil {
		t.Fatalf("single draw should succeed without feature: %v", err)
	}
}

func TestFeatureGate_MultiDrawIndirectCount_NoFeature(t *testing.T) {
	_, _, device := newDeviceWithoutFeatures(t)
	defer device.Release()
	requireHAL(t, device)

	encoder, pass := newEncoderWithRenderPassForDevice(t, device)
	pipeline := &wgpu.RenderPipeline{}
	pipeline.SetTestRequiredVertexBuffers(0)
	pass.SetPipeline(pipeline)

	indirectBuf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "indirect",
		Size:  32,
		Usage: wgpu.BufferUsageIndirect,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer indirectBuf.Release()

	countBuf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "count",
		Size:  4,
		Usage: wgpu.BufferUsageIndirect,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer countBuf.Release()

	pass.MultiDrawIndirectCount(indirectBuf, 0, countBuf, 0, 2)
	_ = pass.End()
	_, finishErr := encoder.Finish()
	if finishErr == nil {
		t.Fatal("Finish() should fail without FeatureMultiDrawIndirectCount")
	}
	var fe *core.FeatureError
	if !errors.As(finishErr, &fe) {
		t.Fatalf("error = %T (%v), want *core.FeatureError", finishErr, finishErr)
	}
	if fe.Feature != gputypes.FeatureMultiDrawIndirectCount.String() {
		t.Errorf("Feature = %q, want %q", fe.Feature, gputypes.FeatureMultiDrawIndirectCount.String())
	}
}

func TestFeatureGate_CreateQuerySet_NoFeature(t *testing.T) {
	_, _, device := newDeviceWithoutFeatures(t)
	defer device.Release()

	_, err := device.CreateQuerySet(&wgpu.QuerySetDescriptor{
		Label: "timestamp",
		Type:  wgpu.QueryTypeTimestamp,
		Count: 2,
	})
	if err == nil {
		t.Fatal("CreateQuerySet should fail without FeatureTimestampQuery")
	}
	var fe *core.FeatureError
	if !errors.As(err, &fe) {
		t.Fatalf("error = %T (%v), want *core.FeatureError", err, err)
	}
	if fe.Feature != gputypes.FeatureTimestampQuery.String() {
		t.Errorf("Feature = %q, want %q", fe.Feature, gputypes.FeatureTimestampQuery.String())
	}
}

func TestFeatureGate_CreateQuerySet_WithFeature(t *testing.T) {
	features := gputypes.Features(gputypes.FeatureTimestampQuery)
	_, _, device := newDeviceWithFeatures(t, features)
	defer device.Release()

	qs, err := device.CreateQuerySet(&wgpu.QuerySetDescriptor{
		Label: "timestamp",
		Type:  wgpu.QueryTypeTimestamp,
		Count: 2,
	})
	if err != nil {
		t.Fatalf("CreateQuerySet: %v", err)
	}
	if qs == nil {
		t.Fatal("CreateQuerySet returned nil")
	}
	defer qs.Release()
	if qs.Count() != 2 {
		t.Errorf("Count() = %d, want 2", qs.Count())
	}
	if qs.Type() != wgpu.QueryTypeTimestamp {
		t.Errorf("Type() = %v, want Timestamp", qs.Type())
	}
}

func TestFeatureGate_CreateQuerySet_NilDescriptor(t *testing.T) {
	_, _, device := newDeviceWithoutFeatures(t)
	defer device.Release()

	if _, err := device.CreateQuerySet(nil); err == nil {
		t.Fatal("expected error for nil descriptor")
	}
}

func TestFeatureGate_MultiDrawIndirectCount_WithFeature(t *testing.T) {
	features := gputypes.Features(gputypes.FeatureMultiDrawIndirectCount)
	_, _, device := newDeviceWithFeatures(t, features)
	defer device.Release()
	requireHAL(t, device)

	encoder, pass := newEncoderWithRenderPassForDevice(t, device)
	pipeline := &wgpu.RenderPipeline{}
	pipeline.SetTestRequiredVertexBuffers(0)
	pass.SetPipeline(pipeline)

	indirectBuf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "indirect",
		Size:  32,
		Usage: wgpu.BufferUsageIndirect,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer indirectBuf.Release()

	countBuf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "count",
		Size:  4,
		Usage: wgpu.BufferUsageIndirect,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer countBuf.Release()

	pass.MultiDrawIndirectCount(indirectBuf, 0, countBuf, 0, 2)
	if err := pass.End(); err != nil {
		t.Fatalf("End: %v", err)
	}
	if _, err := encoder.Finish(); err != nil {
		t.Fatalf("Finish() unexpected error: %v", err)
	}
}

func TestFeatureGate_IndirectFirstInstance_NoFeature(t *testing.T) {
	_, _, device := newDeviceWithoutFeatures(t)
	defer device.Release()
	requireHAL(t, device)

	encoder, pass := newEncoderWithRenderPassForDevice(t, device)
	pipeline := &wgpu.RenderPipeline{}
	pipeline.SetTestRequiredVertexBuffers(0)
	pass.SetPipeline(pipeline)

	pass.Draw(gputypes.DrawArgs{VertexCount: 3, InstanceCount: 1, FirstInstance: 1})
	_ = pass.End()
	_, finishErr := encoder.Finish()
	if finishErr == nil {
		t.Fatal("Finish() should fail without FeatureIndirectFirstInstance")
	}
	var fe *core.FeatureError
	if !errors.As(finishErr, &fe) {
		t.Fatalf("error = %T (%v), want *core.FeatureError", finishErr, finishErr)
	}
	if fe.Feature != gputypes.FeatureIndirectFirstInstance.String() {
		t.Errorf("Feature = %q, want %q", fe.Feature, gputypes.FeatureIndirectFirstInstance.String())
	}
}

func TestFeatureGate_IndirectFirstInstance_WithFeature(t *testing.T) {
	features := gputypes.Features(gputypes.FeatureIndirectFirstInstance)
	_, _, device := newDeviceWithFeatures(t, features)
	defer device.Release()
	requireHAL(t, device)

	encoder, pass := newEncoderWithRenderPassForDevice(t, device)
	pipeline := &wgpu.RenderPipeline{}
	pipeline.SetTestRequiredVertexBuffers(0)
	pass.SetPipeline(pipeline)

	pass.Draw(gputypes.DrawArgs{VertexCount: 3, InstanceCount: 1, FirstInstance: 1})
	if err := pass.End(); err != nil {
		t.Fatalf("End: %v", err)
	}
	if _, err := encoder.Finish(); err != nil {
		t.Fatalf("Finish() unexpected error: %v", err)
	}
}

func TestFeatureGate_IndirectFirstInstance_DrawIndexed_NoFeature(t *testing.T) {
	_, _, device := newDeviceWithoutFeatures(t)
	defer device.Release()
	requireHAL(t, device)

	encoder, pass := newEncoderWithRenderPassForDevice(t, device)
	pipeline := &wgpu.RenderPipeline{}
	pipeline.SetTestRequiredVertexBuffers(0)
	pass.SetPipeline(pipeline)

	indexBuf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Size:  16,
		Usage: wgpu.BufferUsageIndex,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer indexBuf.Release()
	pass.SetIndexBuffer(indexBuf, gputypes.IndexFormatUint32, 0)

	pass.DrawIndexed(gputypes.DrawIndexedArgs{IndexCount: 3, InstanceCount: 1, FirstInstance: 1})
	_ = pass.End()
	_, finishErr := encoder.Finish()
	if finishErr == nil {
		t.Fatal("Finish() should fail without FeatureIndirectFirstInstance")
	}
	var fe *core.FeatureError
	if !errors.As(finishErr, &fe) {
		t.Fatalf("error = %T (%v), want *core.FeatureError", finishErr, finishErr)
	}
}

func TestFeatureGate_MultiDrawIndexedIndirect_NoFeature(t *testing.T) {
	_, _, device := newDeviceWithoutFeatures(t)
	defer device.Release()
	requireHAL(t, device)

	encoder, pass := newEncoderWithRenderPassForDevice(t, device)
	pipeline := &wgpu.RenderPipeline{}
	pipeline.SetTestRequiredVertexBuffers(0)
	pass.SetPipeline(pipeline)

	indexBuf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Size:  16,
		Usage: wgpu.BufferUsageIndex,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer indexBuf.Release()
	pass.SetIndexBuffer(indexBuf, gputypes.IndexFormatUint32, 0)

	indirectBuf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Size:  40,
		Usage: wgpu.BufferUsageIndirect,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer indirectBuf.Release()

	pass.MultiDrawIndexedIndirect(indirectBuf, 0, 2)
	_ = pass.End()
	_, finishErr := encoder.Finish()
	if finishErr == nil {
		t.Fatal("Finish() should fail without FeatureMultiDrawIndirect")
	}
	var fe *core.FeatureError
	if !errors.As(finishErr, &fe) {
		t.Fatalf("error = %T (%v), want *core.FeatureError", finishErr, finishErr)
	}
}

func TestFeatureGate_MultiDrawIndexedIndirectCount_NoFeature(t *testing.T) {
	_, _, device := newDeviceWithoutFeatures(t)
	defer device.Release()
	requireHAL(t, device)

	encoder, pass := newEncoderWithRenderPassForDevice(t, device)
	pipeline := &wgpu.RenderPipeline{}
	pipeline.SetTestRequiredVertexBuffers(0)
	pass.SetPipeline(pipeline)

	indexBuf, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Size:  16,
		Usage: wgpu.BufferUsageIndex,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer indexBuf.Release()
	pass.SetIndexBuffer(indexBuf, gputypes.IndexFormatUint32, 0)

	indirectBuf, err := device.CreateBuffer(&wgpu.BufferDescriptor{Size: 40, Usage: wgpu.BufferUsageIndirect})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer indirectBuf.Release()
	countBuf, err := device.CreateBuffer(&wgpu.BufferDescriptor{Size: 4, Usage: wgpu.BufferUsageIndirect})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	defer countBuf.Release()

	pass.MultiDrawIndexedIndirectCount(indirectBuf, 0, countBuf, 0, 2)
	_ = pass.End()
	_, finishErr := encoder.Finish()
	if finishErr == nil {
		t.Fatal("Finish() should fail without FeatureMultiDrawIndirectCount")
	}
	var fe *core.FeatureError
	if !errors.As(finishErr, &fe) {
		t.Fatalf("error = %T (%v), want *core.FeatureError", finishErr, finishErr)
	}
}
