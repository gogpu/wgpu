package core

import (
	"testing"
)

func TestComputePassEncoder_SetPipeline(t *testing.T) {
	// Get global state for testing
	g := GetGlobal()
	defer g.Hub().Clear()

	encoder := &ComputePassEncoder{
		raw:    nil, // No HAL encoder for unit tests
		device: nil,
		ended:  false,
	}

	// Test with invalid pipeline ID (should fail)
	invalidPipelineID := NewID[computePipelineMarker](999, 1)
	err := encoder.SetPipeline(invalidPipelineID)
	if err == nil {
		t.Error("expected error for invalid pipeline ID, got nil")
	}

	// Register a compute pipeline and test with valid ID
	hub := GetGlobal().Hub()
	pipelineID := hub.RegisterComputePipeline(ComputePipeline{})

	err = encoder.SetPipeline(pipelineID)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestComputePassEncoder_SetPipeline_AfterEnd(t *testing.T) {
	encoder := &ComputePassEncoder{
		raw:    nil,
		device: nil,
		ended:  true, // Already ended
	}

	pipelineID := NewID[computePipelineMarker](1, 1)
	err := encoder.SetPipeline(pipelineID)
	if err == nil {
		t.Error("expected error when setting pipeline after End(), got nil")
	}
}

func TestComputePassEncoder_SetBindGroup(t *testing.T) {
	// Get global state for testing
	g := GetGlobal()
	defer g.Hub().Clear()

	encoder := &ComputePassEncoder{
		raw:    nil,
		device: nil,
		ended:  false,
	}

	// Test with invalid bind group ID (should fail)
	invalidGroupID := NewID[bindGroupMarker](999, 1)
	err := encoder.SetBindGroup(0, invalidGroupID, nil)
	if err == nil {
		t.Error("expected error for invalid bind group ID, got nil")
	}

	// Register a bind group and test with valid ID
	hub := GetGlobal().Hub()
	groupID := hub.RegisterBindGroup(BindGroup{})

	err = encoder.SetBindGroup(0, groupID, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Test with dynamic offsets
	err = encoder.SetBindGroup(1, groupID, []uint32{0, 256})
	if err != nil {
		t.Errorf("unexpected error with offsets: %v", err)
	}
}

func TestComputePassEncoder_SetBindGroup_IndexValidation(t *testing.T) {
	// Get global state for testing
	g := GetGlobal()
	defer g.Hub().Clear()

	hub := GetGlobal().Hub()
	groupID := hub.RegisterBindGroup(BindGroup{})

	encoder := &ComputePassEncoder{
		raw:    nil,
		device: nil,
		ended:  false,
	}

	tests := []struct {
		name    string
		index   uint32
		wantErr bool
	}{
		{"index 0", 0, false},
		{"index 1", 1, false},
		{"index 2", 2, false},
		{"index 3", 3, false},
		{"index 4 - invalid", 4, true},
		{"index 10 - invalid", 10, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := encoder.SetBindGroup(tt.index, groupID, nil)
			if (err != nil) != tt.wantErr {
				t.Errorf("SetBindGroup(%d) error = %v, wantErr %v", tt.index, err, tt.wantErr)
			}
		})
	}
}

func TestComputePassEncoder_SetBindGroup_AfterEnd(t *testing.T) {
	encoder := &ComputePassEncoder{
		raw:    nil,
		device: nil,
		ended:  true,
	}

	groupID := NewID[bindGroupMarker](1, 1)
	err := encoder.SetBindGroup(0, groupID, nil)
	if err == nil {
		t.Error("expected error when setting bind group after End(), got nil")
	}
}

func TestComputePassEncoder_Dispatch(t *testing.T) {
	encoder := &ComputePassEncoder{
		raw:    nil,
		device: nil,
		ended:  false,
	}

	// Dispatch should not panic (no return value to check)
	encoder.Dispatch(1, 1, 1)
	encoder.Dispatch(64, 64, 1)
	encoder.Dispatch(0, 0, 0) // Zero dispatch is valid (does nothing)
}

func TestComputePassEncoder_Dispatch_AfterEnd(t *testing.T) {
	encoder := &ComputePassEncoder{
		raw:    nil,
		device: nil,
		ended:  true,
	}

	// Should not panic, just returns early
	encoder.Dispatch(1, 1, 1)
}

func TestComputePassEncoder_DispatchIndirect(t *testing.T) {
	// Get global state for testing
	g := GetGlobal()
	defer g.Hub().Clear()

	encoder := &ComputePassEncoder{
		raw:    nil,
		device: nil,
		ended:  false,
	}

	// Test with invalid buffer ID (should fail)
	invalidBufferID := NewID[bufferMarker](999, 1)
	err := encoder.DispatchIndirect(invalidBufferID, 0)
	if err == nil {
		t.Error("expected error for invalid buffer ID, got nil")
	}

	// Register a buffer and test with valid ID
	hub := GetGlobal().Hub()
	bufferID := hub.RegisterBuffer(Buffer{})

	err = encoder.DispatchIndirect(bufferID, 0)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Test with aligned offset
	err = encoder.DispatchIndirect(bufferID, 256)
	if err != nil {
		t.Errorf("unexpected error with offset: %v", err)
	}
}

func TestComputePassEncoder_DispatchIndirect_Alignment(t *testing.T) {
	// Get global state for testing
	g := GetGlobal()
	defer g.Hub().Clear()

	hub := GetGlobal().Hub()
	bufferID := hub.RegisterBuffer(Buffer{})

	encoder := &ComputePassEncoder{
		raw:    nil,
		device: nil,
		ended:  false,
	}

	tests := []struct {
		name    string
		offset  uint64
		wantErr bool
	}{
		{"offset 0", 0, false},
		{"offset 4", 4, false},
		{"offset 8", 8, false},
		{"offset 256", 256, false},
		{"offset 1 - unaligned", 1, true},
		{"offset 2 - unaligned", 2, true},
		{"offset 3 - unaligned", 3, true},
		{"offset 5 - unaligned", 5, true},
		{"offset 257 - unaligned", 257, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := encoder.DispatchIndirect(bufferID, tt.offset)
			if (err != nil) != tt.wantErr {
				t.Errorf("DispatchIndirect(offset=%d) error = %v, wantErr %v", tt.offset, err, tt.wantErr)
			}
		})
	}
}

func TestComputePassEncoder_DispatchIndirect_AfterEnd(t *testing.T) {
	encoder := &ComputePassEncoder{
		raw:    nil,
		device: nil,
		ended:  true,
	}

	bufferID := NewID[bufferMarker](1, 1)
	err := encoder.DispatchIndirect(bufferID, 0)
	if err == nil {
		t.Error("expected error when calling DispatchIndirect after End(), got nil")
	}
}

func TestComputePassEncoder_End(t *testing.T) {
	encoder := &ComputePassEncoder{
		raw:    nil,
		device: nil,
		ended:  false,
	}

	// Should not panic
	encoder.End()

	if !encoder.ended {
		t.Error("expected ended to be true after End()")
	}

	// Calling End() again should be a no-op
	encoder.End()
}

func TestCommandEncoderImpl_BeginComputePass(t *testing.T) {
	encoder := &CommandEncoderImpl{
		raw:    nil,
		device: nil,
		state:  CommandEncoderStateRecording,
		label:  "test",
	}

	// Test with nil descriptor
	pass, err := encoder.BeginComputePass(nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if pass == nil {
		t.Fatal("expected non-nil compute pass encoder")
	}
	if pass.ended {
		t.Error("expected new compute pass to not be ended")
	}
}

func TestCommandEncoderImpl_BeginComputePass_WithDescriptor(t *testing.T) {
	encoder := &CommandEncoderImpl{
		raw:    nil,
		device: nil,
		state:  CommandEncoderStateRecording,
		label:  "test",
	}

	desc := &ComputePassDescriptor{
		Label: "my compute pass",
	}

	pass, err := encoder.BeginComputePass(desc)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if pass == nil {
		t.Error("expected non-nil compute pass encoder")
	}
}

func TestCommandEncoderImpl_BeginComputePass_NotRecording(t *testing.T) {
	tests := []struct {
		name  string
		state CommandEncoderState
	}{
		{"ended state", CommandEncoderStateEnded},
		{"error state", CommandEncoderStateError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoder := &CommandEncoderImpl{
				raw:    nil,
				device: nil,
				state:  tt.state,
				label:  "test",
			}

			_, err := encoder.BeginComputePass(nil)
			if err == nil {
				t.Error("expected error when encoder is not in recording state")
			}
		})
	}
}

func TestDeviceCreateCommandEncoder(t *testing.T) {
	// Get global state for testing
	g := GetGlobal()
	defer g.Hub().Clear()

	// Test with invalid device ID
	invalidDeviceID := NewID[deviceMarker](999, 1)
	_, err := DeviceCreateCommandEncoder(invalidDeviceID, "test")
	if err == nil {
		t.Error("expected error for invalid device ID, got nil")
	}

	// Create a device and test with valid ID
	hub := GetGlobal().Hub()

	// First need an adapter
	adapter := &Adapter{}
	adapterID := hub.RegisterAdapter(adapter)

	// Create device with queue
	queue := Queue{Label: "test queue"}
	queueID := hub.RegisterQueue(queue)

	device := Device{
		Adapter: adapterID,
		Label:   "test device",
		Queue:   queueID,
	}
	deviceID := hub.RegisterDevice(device)

	encoderID, err := DeviceCreateCommandEncoder(deviceID, "my encoder")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if encoderID.IsZero() {
		t.Error("expected non-zero encoder ID")
	}
}

func TestCommandEncoderFinish(t *testing.T) {
	// Get global state for testing
	g := GetGlobal()
	defer g.Hub().Clear()

	hub := GetGlobal().Hub()

	// Test with invalid encoder ID
	invalidEncoderID := NewID[commandEncoderMarker](999, 1)
	_, err := CommandEncoderFinish(invalidEncoderID)
	if err == nil {
		t.Error("expected error for invalid encoder ID, got nil")
	}

	// Register an encoder and test finishing
	encoderID := hub.RegisterCommandEncoder(CommandEncoder{})

	cmdBufferID, err := CommandEncoderFinish(encoderID)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if cmdBufferID.IsZero() {
		t.Error("expected non-zero command buffer ID")
	}

	// Encoder should be unregistered after finish
	_, err = hub.GetCommandEncoder(encoderID)
	if err == nil {
		t.Error("expected encoder to be unregistered after finish")
	}
}

func TestComputePassTimestampWrites(t *testing.T) {
	beginIndex := uint32(0)
	endIndex := uint32(1)

	writes := &ComputePassTimestampWrites{
		QuerySet:                  NewID[querySetMarker](1, 1),
		BeginningOfPassWriteIndex: &beginIndex,
		EndOfPassWriteIndex:       &endIndex,
	}

	if writes.QuerySet.IsZero() {
		t.Error("expected non-zero QuerySet ID")
	}
	if writes.BeginningOfPassWriteIndex == nil || *writes.BeginningOfPassWriteIndex != 0 {
		t.Error("expected BeginningOfPassWriteIndex to be 0")
	}
	if writes.EndOfPassWriteIndex == nil || *writes.EndOfPassWriteIndex != 1 {
		t.Error("expected EndOfPassWriteIndex to be 1")
	}
}

func TestCommandEncoderState_Constants(t *testing.T) {
	// Verify constants are distinct
	if CommandEncoderStateRecording == CommandEncoderStateEnded {
		t.Error("Recording and Ended states should be different")
	}
	if CommandEncoderStateRecording == CommandEncoderStateError {
		t.Error("Recording and Error states should be different")
	}
	if CommandEncoderStateEnded == CommandEncoderStateError {
		t.Error("Ended and Error states should be different")
	}
}
