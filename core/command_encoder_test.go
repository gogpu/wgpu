package core

import "testing"

// =============================================================================
// CommandEncoder State Machine Tests
// =============================================================================

func TestCommandEncoderPassState_NewIsRecording(t *testing.T) {
	enc := NewCommandEncoder(nil, nil, "test")
	if enc.PassState() != CommandEncoderPassStateRecording {
		t.Errorf("new encoder state = %v, want Recording", enc.PassState())
	}
}

func TestCommandEncoderPassState_RenderPassLifecycle(t *testing.T) {
	enc := NewCommandEncoder(nil, nil, "test")

	if err := enc.BeginRenderPass(); err != nil {
		t.Fatalf("BeginRenderPass() error = %v", err)
	}
	if enc.PassState() != CommandEncoderPassStateInRenderPass {
		t.Errorf("state after BeginRenderPass = %v, want InRenderPass", enc.PassState())
	}

	if err := enc.EndRenderPass(); err != nil {
		t.Fatalf("EndRenderPass() error = %v", err)
	}
	if enc.PassState() != CommandEncoderPassStateRecording {
		t.Errorf("state after EndRenderPass = %v, want Recording", enc.PassState())
	}
}

func TestCommandEncoderPassState_ComputePassLifecycle(t *testing.T) {
	enc := NewCommandEncoder(nil, nil, "test")

	if err := enc.BeginComputePass(); err != nil {
		t.Fatalf("BeginComputePass() error = %v", err)
	}
	if enc.PassState() != CommandEncoderPassStateInComputePass {
		t.Errorf("state after BeginComputePass = %v, want InComputePass", enc.PassState())
	}

	if err := enc.EndComputePass(); err != nil {
		t.Fatalf("EndComputePass() error = %v", err)
	}
	if enc.PassState() != CommandEncoderPassStateRecording {
		t.Errorf("state after EndComputePass = %v, want Recording", enc.PassState())
	}
}

func TestCommandEncoderPassState_Finish(t *testing.T) {
	enc := NewCommandEncoder(nil, nil, "test")

	if err := enc.Finish(); err != nil {
		t.Fatalf("Finish() error = %v", err)
	}
	if enc.PassState() != CommandEncoderPassStateFinished {
		t.Errorf("state after Finish = %v, want Finished", enc.PassState())
	}
}

func TestCommandEncoderPassState_DoubleBeginRenderPass(t *testing.T) {
	enc := NewCommandEncoder(nil, nil, "test")

	if err := enc.BeginRenderPass(); err != nil {
		t.Fatalf("first BeginRenderPass() error = %v", err)
	}

	err := enc.BeginRenderPass()
	if err == nil {
		t.Fatal("second BeginRenderPass() should return error")
	}
}

func TestCommandEncoderPassState_EndWithoutBegin(t *testing.T) {
	enc := NewCommandEncoder(nil, nil, "test")

	err := enc.EndRenderPass()
	if err == nil {
		t.Fatal("EndRenderPass() without Begin should return error")
	}

	err = enc.EndComputePass()
	if err == nil {
		t.Fatal("EndComputePass() without Begin should return error")
	}
}

func TestCommandEncoderPassState_FinishWhileInPass(t *testing.T) {
	enc := NewCommandEncoder(nil, nil, "test")
	_ = enc.BeginRenderPass()

	err := enc.Finish()
	if err == nil {
		t.Fatal("Finish() while in pass should return error")
	}
}

func TestCommandEncoderPassState_FinishAfterError(t *testing.T) {
	enc := NewCommandEncoder(nil, nil, "test")
	enc.RecordError("something went wrong")

	err := enc.Finish()
	if err == nil {
		t.Fatal("Finish() after error should return error")
	}
}

func TestCommandEncoderRecordError(t *testing.T) {
	enc := NewCommandEncoder(nil, nil, "test")

	enc.RecordError("first error")
	if enc.PassState() != CommandEncoderPassStateError {
		t.Errorf("state after RecordError = %v, want Error", enc.PassState())
	}
	if enc.ErrorMessage() != "first error" {
		t.Errorf("error message = %q, want %q", enc.ErrorMessage(), "first error")
	}

	// Second error should be ignored
	enc.RecordError("second error")
	if enc.ErrorMessage() != "first error" {
		t.Errorf("error message after second RecordError = %q, want %q", enc.ErrorMessage(), "first error")
	}
}

func TestCommandEncoderOperationAfterFinish(t *testing.T) {
	enc := NewCommandEncoder(nil, nil, "test")
	_ = enc.Finish()

	err := enc.BeginRenderPass()
	if err == nil {
		t.Fatal("BeginRenderPass() after Finish should return error")
	}

	err = enc.BeginComputePass()
	if err == nil {
		t.Fatal("BeginComputePass() after Finish should return error")
	}
}

// =============================================================================
// CommandBuffer State Tests
// =============================================================================

func TestCommandBufferMarkSubmitted(t *testing.T) {
	// Create CommandBuffer directly to avoid nil device panic in NewCommandBuffer.
	cb := &CommandBuffer{
		label:       "test",
		submitState: CommandBufferSubmitStateAvailable,
	}
	if cb.IsSubmitted() {
		t.Fatal("new command buffer should not be submitted")
	}

	err := cb.MarkSubmitted()
	if err != nil {
		t.Fatalf("MarkSubmitted() error = %v", err)
	}
	if !cb.IsSubmitted() {
		t.Fatal("command buffer should be submitted after MarkSubmitted()")
	}
}

func TestCommandBufferDoubleSubmit(t *testing.T) {
	cb := &CommandBuffer{
		label:       "test",
		submitState: CommandBufferSubmitStateAvailable,
	}
	_ = cb.MarkSubmitted()

	err := cb.MarkSubmitted()
	if err == nil {
		t.Fatal("second MarkSubmitted() should return error")
	}
}

// =============================================================================
// CommandEncoderPassState String Tests
// =============================================================================

func TestCommandEncoderPassState_String(t *testing.T) {
	tests := []struct {
		state    CommandEncoderPassState
		expected string
	}{
		{CommandEncoderPassStateRecording, "Recording"},
		{CommandEncoderPassStateInRenderPass, "InRenderPass"},
		{CommandEncoderPassStateInComputePass, "InComputePass"},
		{CommandEncoderPassStateFinished, "Finished"},
		{CommandEncoderPassStateError, "Error"},
		{CommandEncoderPassState(99), "Unknown(99)"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			got := tt.state.String()
			if got != tt.expected {
				t.Errorf("String() = %q, want %q", got, tt.expected)
			}
		})
	}
}
