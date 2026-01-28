package core

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
)

// =============================================================================
// CommandEncoderStatus Tests
// =============================================================================

func TestCommandEncoderStatus_String(t *testing.T) {
	tests := []struct {
		status   CommandEncoderStatus
		expected string
	}{
		{CommandEncoderStatusRecording, "Recording"},
		{CommandEncoderStatusLocked, "Locked"},
		{CommandEncoderStatusFinished, "Finished"},
		{CommandEncoderStatusError, "Error"},
		{CommandEncoderStatusConsumed, "Consumed"},
		{CommandEncoderStatus(999), "Unknown(999)"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			got := tt.status.String()
			if got != tt.expected {
				t.Errorf("Status.String() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestCommandEncoderStatus_Constants(t *testing.T) {
	// Verify constants are distinct
	statuses := []CommandEncoderStatus{
		CommandEncoderStatusRecording,
		CommandEncoderStatusLocked,
		CommandEncoderStatusFinished,
		CommandEncoderStatusError,
		CommandEncoderStatusConsumed,
	}

	seen := make(map[CommandEncoderStatus]bool)
	for _, s := range statuses {
		if seen[s] {
			t.Errorf("Duplicate status value: %v", s)
		}
		seen[s] = true
	}
}

// =============================================================================
// CoreCommandEncoder Tests
// =============================================================================

func TestDevice_CreateCommandEncoder_Success(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	encoder, err := device.CreateCommandEncoder("TestEncoder")
	if err != nil {
		t.Fatalf("CreateCommandEncoder failed: %v", err)
	}
	if encoder == nil {
		t.Fatal("CreateCommandEncoder returned nil encoder")
	}
	if encoder.Status() != CommandEncoderStatusRecording {
		t.Errorf("Expected Recording status, got %v", encoder.Status())
	}
	if encoder.Label() != "TestEncoder" {
		t.Errorf("Expected label 'TestEncoder', got '%s'", encoder.Label())
	}
	if encoder.Device() != device {
		t.Error("Encoder should reference parent device")
	}
}

func TestDevice_CreateCommandEncoder_DeviceDestroyed(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	device.Destroy()

	_, err := device.CreateCommandEncoder("TestEncoder")
	if err == nil {
		t.Fatal("Expected error for destroyed device")
	}
	if !errors.Is(err, ErrDeviceDestroyed) {
		t.Errorf("Expected ErrDeviceDestroyed, got %v", err)
	}
}

func TestCoreCommandEncoder_BeginRenderPass(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	encoder, err := device.CreateCommandEncoder("TestEncoder")
	if err != nil {
		t.Fatalf("CreateCommandEncoder failed: %v", err)
	}

	desc := &RenderPassDescriptor{
		Label: "TestPass",
		ColorAttachments: []RenderPassColorAttachment{
			{
				LoadOp:     gputypes.LoadOpClear,
				StoreOp:    gputypes.StoreOpStore,
				ClearValue: gputypes.Color{R: 0, G: 0, B: 0, A: 1},
			},
		},
	}

	pass, err := encoder.BeginRenderPass(desc)
	if err != nil {
		t.Fatalf("BeginRenderPass failed: %v", err)
	}
	if pass == nil {
		t.Fatal("BeginRenderPass returned nil pass")
	}
	if encoder.Status() != CommandEncoderStatusLocked {
		t.Errorf("Expected Locked status after BeginRenderPass, got %v", encoder.Status())
	}
}

func TestCoreCommandEncoder_BeginRenderPass_NilDescriptor(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	encoder, _ := device.CreateCommandEncoder("TestEncoder")

	_, err := encoder.BeginRenderPass(nil)
	if err == nil {
		t.Fatal("Expected error for nil descriptor")
	}
	if encoder.Status() != CommandEncoderStatusError {
		t.Errorf("Expected Error status after nil descriptor, got %v", encoder.Status())
	}
}

func TestCoreCommandEncoder_BeginRenderPass_WhileLocked(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	encoder, _ := device.CreateCommandEncoder("TestEncoder")

	desc := &RenderPassDescriptor{Label: "Pass1"}
	_, _ = encoder.BeginRenderPass(desc)

	// Try to begin another pass while locked
	_, err := encoder.BeginRenderPass(&RenderPassDescriptor{Label: "Pass2"})
	if err == nil {
		t.Fatal("Expected error when beginning pass while locked")
	}
	var stateErr *EncoderStateError
	if !errors.As(err, &stateErr) {
		t.Fatalf("Expected EncoderStateError, got %T", err)
	}
	if stateErr.Status != CommandEncoderStatusLocked {
		t.Errorf("Expected Locked status in error, got %v", stateErr.Status)
	}
}

func TestCoreRenderPassEncoder_End(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	encoder, _ := device.CreateCommandEncoder("TestEncoder")
	pass, _ := encoder.BeginRenderPass(&RenderPassDescriptor{Label: "TestPass"})

	err := pass.End()
	if err != nil {
		t.Fatalf("End failed: %v", err)
	}
	if encoder.Status() != CommandEncoderStatusRecording {
		t.Errorf("Expected Recording status after End, got %v", encoder.Status())
	}
}

func TestCoreRenderPassEncoder_End_Idempotent(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	encoder, _ := device.CreateCommandEncoder("TestEncoder")
	pass, _ := encoder.BeginRenderPass(&RenderPassDescriptor{Label: "TestPass"})

	// First End() should succeed
	err := pass.End()
	if err != nil {
		t.Fatalf("First End failed: %v", err)
	}

	// Second End() should be no-op
	err = pass.End()
	if err != nil {
		t.Errorf("Second End should be idempotent, got error: %v", err)
	}
}

func TestCoreCommandEncoder_BeginComputePass(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	encoder, _ := device.CreateCommandEncoder("TestEncoder")

	pass, err := encoder.BeginComputePass(&CoreComputePassDescriptor{Label: "TestCompute"})
	if err != nil {
		t.Fatalf("BeginComputePass failed: %v", err)
	}
	if pass == nil {
		t.Fatal("BeginComputePass returned nil pass")
	}
	if encoder.Status() != CommandEncoderStatusLocked {
		t.Errorf("Expected Locked status after BeginComputePass, got %v", encoder.Status())
	}
}

func TestCoreCommandEncoder_BeginComputePass_NilDescriptor(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	encoder, _ := device.CreateCommandEncoder("TestEncoder")

	// nil descriptor should be allowed (becomes empty descriptor)
	pass, err := encoder.BeginComputePass(nil)
	if err != nil {
		t.Fatalf("BeginComputePass with nil desc failed: %v", err)
	}
	if pass == nil {
		t.Fatal("BeginComputePass returned nil pass")
	}
}

func TestCoreComputePassEncoder_End(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	encoder, _ := device.CreateCommandEncoder("TestEncoder")
	pass, _ := encoder.BeginComputePass(&CoreComputePassDescriptor{Label: "TestCompute"})

	err := pass.End()
	if err != nil {
		t.Fatalf("End failed: %v", err)
	}
	if encoder.Status() != CommandEncoderStatusRecording {
		t.Errorf("Expected Recording status after End, got %v", encoder.Status())
	}
}

func TestCoreComputePassEncoder_Dispatch(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	encoder, _ := device.CreateCommandEncoder("TestEncoder")
	pass, _ := encoder.BeginComputePass(&CoreComputePassDescriptor{Label: "TestCompute"})

	// Should not panic
	pass.Dispatch(1, 1, 1)
	pass.Dispatch(64, 64, 1)
	pass.Dispatch(0, 0, 0) // Zero dispatch is valid
}

func TestCoreComputePassEncoder_Dispatch_AfterEnd(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	encoder, _ := device.CreateCommandEncoder("TestEncoder")
	pass, _ := encoder.BeginComputePass(&CoreComputePassDescriptor{Label: "TestCompute"})
	_ = pass.End()

	// Should not panic, just be ignored
	pass.Dispatch(1, 1, 1)
}

func TestCoreCommandEncoder_Finish(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	encoder, _ := device.CreateCommandEncoder("TestEncoder")

	cmdBuffer, err := encoder.Finish()
	if err != nil {
		t.Fatalf("Finish failed: %v", err)
	}
	if cmdBuffer == nil {
		t.Fatal("Finish returned nil command buffer")
	}
	if encoder.Status() != CommandEncoderStatusFinished {
		t.Errorf("Expected Finished status after Finish, got %v", encoder.Status())
	}
	if cmdBuffer.Device() != device {
		t.Error("CommandBuffer should reference parent device")
	}
	if cmdBuffer.Label() != "TestEncoder" {
		t.Errorf("Expected label 'TestEncoder', got '%s'", cmdBuffer.Label())
	}
}

func TestCoreCommandEncoder_Finish_WhileLocked(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	encoder, _ := device.CreateCommandEncoder("TestEncoder")
	_, _ = encoder.BeginRenderPass(&RenderPassDescriptor{Label: "TestPass"})

	// Try to finish while locked
	_, err := encoder.Finish()
	if err == nil {
		t.Fatal("Expected error when finishing while locked")
	}
	var stateErr *EncoderStateError
	if !errors.As(err, &stateErr) {
		t.Fatalf("Expected EncoderStateError, got %T", err)
	}
}

func TestCoreCommandEncoder_Finish_AfterFinish(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	encoder, _ := device.CreateCommandEncoder("TestEncoder")
	_, _ = encoder.Finish()

	// Try to finish again
	_, err := encoder.Finish()
	if err == nil {
		t.Fatal("Expected error when finishing twice")
	}
	var stateErr *EncoderStateError
	if !errors.As(err, &stateErr) {
		t.Fatalf("Expected EncoderStateError, got %T", err)
	}
	if stateErr.Status != CommandEncoderStatusFinished {
		t.Errorf("Expected Finished status in error, got %v", stateErr.Status)
	}
}

func TestCoreCommandEncoder_MarkConsumed(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	encoder, _ := device.CreateCommandEncoder("TestEncoder")
	_, _ = encoder.Finish()

	encoder.MarkConsumed()
	if encoder.Status() != CommandEncoderStatusConsumed {
		t.Errorf("Expected Consumed status after MarkConsumed, got %v", encoder.Status())
	}
}

func TestCoreCommandEncoder_Error(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	encoder, _ := device.CreateCommandEncoder("TestEncoder")

	// Initially no error
	if encoder.Error() != nil {
		t.Error("Expected nil error initially")
	}

	// Trigger error state by passing nil descriptor
	_, _ = encoder.BeginRenderPass(nil)

	// Now should have error
	if encoder.Error() == nil {
		t.Error("Expected non-nil error after failure")
	}
}

// =============================================================================
// CoreRenderPassEncoder Method Tests
// =============================================================================

func TestCoreRenderPassEncoder_SetViewport(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	encoder, _ := device.CreateCommandEncoder("TestEncoder")
	pass, _ := encoder.BeginRenderPass(&RenderPassDescriptor{Label: "TestPass"})

	// Should not panic
	pass.SetViewport(0, 0, 800, 600, 0.0, 1.0)
}

func TestCoreRenderPassEncoder_SetScissorRect(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	encoder, _ := device.CreateCommandEncoder("TestEncoder")
	pass, _ := encoder.BeginRenderPass(&RenderPassDescriptor{Label: "TestPass"})

	// Should not panic
	pass.SetScissorRect(0, 0, 800, 600)
}

func TestCoreRenderPassEncoder_SetBlendConstant(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	encoder, _ := device.CreateCommandEncoder("TestEncoder")
	pass, _ := encoder.BeginRenderPass(&RenderPassDescriptor{Label: "TestPass"})

	// Should not panic
	pass.SetBlendConstant(&gputypes.Color{R: 1, G: 1, B: 1, A: 1})
}

func TestCoreRenderPassEncoder_SetStencilReference(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	encoder, _ := device.CreateCommandEncoder("TestEncoder")
	pass, _ := encoder.BeginRenderPass(&RenderPassDescriptor{Label: "TestPass"})

	// Should not panic
	pass.SetStencilReference(1)
}

func TestCoreRenderPassEncoder_Draw(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	encoder, _ := device.CreateCommandEncoder("TestEncoder")
	pass, _ := encoder.BeginRenderPass(&RenderPassDescriptor{Label: "TestPass"})

	// Should not panic
	pass.Draw(3, 1, 0, 0)
}

func TestCoreRenderPassEncoder_DrawIndexed(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	encoder, _ := device.CreateCommandEncoder("TestEncoder")
	pass, _ := encoder.BeginRenderPass(&RenderPassDescriptor{Label: "TestPass"})

	// Should not panic
	pass.DrawIndexed(6, 1, 0, 0, 0)
}

func TestCoreRenderPassEncoder_AfterEnd(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	encoder, _ := device.CreateCommandEncoder("TestEncoder")
	pass, _ := encoder.BeginRenderPass(&RenderPassDescriptor{Label: "TestPass"})
	_ = pass.End()

	// All methods should silently return after End
	pass.SetViewport(0, 0, 800, 600, 0.0, 1.0)
	pass.SetScissorRect(0, 0, 800, 600)
	pass.SetBlendConstant(&gputypes.Color{R: 1, G: 1, B: 1, A: 1})
	pass.SetStencilReference(1)
	pass.Draw(3, 1, 0, 0)
	pass.DrawIndexed(6, 1, 0, 0, 0)
}

// =============================================================================
// Error Type Tests
// =============================================================================

func TestCreateCommandEncoderError_Error(t *testing.T) {
	err := &CreateCommandEncoderError{
		Kind:     CreateCommandEncoderErrorHAL,
		Label:    "test",
		HALError: errors.New("backend error"),
	}

	msg := err.Error()
	if msg == "" {
		t.Error("Error message should not be empty")
	}
	if !containsString(msg, "HAL error") {
		t.Errorf("Expected 'HAL error' in message, got %q", msg)
	}
}

func TestCreateCommandEncoderError_Unwrap(t *testing.T) {
	halErr := errors.New("backend error")
	err := &CreateCommandEncoderError{
		Kind:     CreateCommandEncoderErrorHAL,
		Label:    "test",
		HALError: halErr,
	}

	if !errors.Is(err.Unwrap(), halErr) {
		t.Error("Unwrap should return HAL error")
	}
}

func TestEncoderStateError_Error(t *testing.T) {
	err := &EncoderStateError{
		Operation: "finish",
		Status:    CommandEncoderStatusLocked,
	}

	msg := err.Error()
	if msg == "" {
		t.Error("Error message should not be empty")
	}
	if !containsString(msg, "finish") {
		t.Errorf("Expected 'finish' in message, got %q", msg)
	}
	if !containsString(msg, "Locked") {
		t.Errorf("Expected 'Locked' in message, got %q", msg)
	}
}

func TestIsEncoderStateError(t *testing.T) {
	stateErr := &EncoderStateError{
		Operation: "test",
		Status:    CommandEncoderStatusError,
	}

	if !IsEncoderStateError(stateErr) {
		t.Error("IsEncoderStateError should return true for EncoderStateError")
	}

	otherErr := errors.New("other error")
	if IsEncoderStateError(otherErr) {
		t.Error("IsEncoderStateError should return false for other errors")
	}
}

// =============================================================================
// BufferUses and TextureUses Tests
// =============================================================================

func TestBufferUses_Constants(t *testing.T) {
	// Verify constants are distinct (excluding None which is 0)
	uses := []BufferUses{
		BufferUsesVertex,
		BufferUsesIndex,
		BufferUsesUniform,
		BufferUsesStorage,
		BufferUsesIndirect,
		BufferUsesCopySrc,
		BufferUsesCopyDst,
	}

	seen := make(map[BufferUses]bool)
	for _, u := range uses {
		if seen[u] {
			t.Errorf("Duplicate BufferUses value: %v", u)
		}
		seen[u] = true
		if u == BufferUsesNone {
			t.Errorf("Non-None constant should not equal BufferUsesNone: %v", u)
		}
	}
}

func TestTextureUses_Constants(t *testing.T) {
	// Verify constants are distinct (excluding None which is 0)
	uses := []TextureUses{
		TextureUsesSampled,
		TextureUsesStorage,
		TextureUsesRenderAttachment,
		TextureUsesCopySrc,
		TextureUsesCopyDst,
	}

	seen := make(map[TextureUses]bool)
	for _, u := range uses {
		if seen[u] {
			t.Errorf("Duplicate TextureUses value: %v", u)
		}
		seen[u] = true
		if u == TextureUsesNone {
			t.Errorf("Non-None constant should not equal TextureUsesNone: %v", u)
		}
	}
}

// =============================================================================
// CoreCommandBuffer Tests
// =============================================================================

func TestCoreCommandBuffer_Raw(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	encoder, _ := device.CreateCommandEncoder("TestEncoder")
	cmdBuffer, _ := encoder.Finish()

	// Raw should return the HAL command buffer
	raw := cmdBuffer.Raw()
	if raw == nil {
		t.Error("Raw should return non-nil command buffer")
	}
}

// =============================================================================
// Helper Functions
// =============================================================================

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || s != "" && containsStringHelper(s, substr))
}

func containsStringHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Note: Mock HAL types are defined in device_hal_test.go
// (mockHALDevice, mockCommandEncoder, mockRenderPassEncoder, etc.)
