package core

import "fmt"

// =============================================================================
// CommandEncoder State Machine (CORE-003)
// =============================================================================

// PassState returns the current pass lifecycle state.
func (e *CommandEncoder) PassState() CommandEncoderPassState {
	return e.passState
}

// Label returns the encoder's debug label.
func (e *CommandEncoder) EncoderLabel() string {
	return e.label
}

// BeginRenderPass validates the encoder state and transitions to InRenderPass.
//
// The encoder must be in the Recording state. After this call, the encoder
// is locked in the InRenderPass state until EndRenderPass is called.
func (e *CommandEncoder) BeginRenderPass() error {
	if e.passState != CommandEncoderPassStateRecording {
		return fmt.Errorf("core: command encoder: cannot begin render pass in %s state", e.passState)
	}
	e.passState = CommandEncoderPassStateInRenderPass
	e.passDepth++
	return nil
}

// EndRenderPass validates the encoder state and transitions back to Recording.
//
// The encoder must be in the InRenderPass state.
func (e *CommandEncoder) EndRenderPass() error {
	if e.passState != CommandEncoderPassStateInRenderPass {
		return fmt.Errorf("core: command encoder: cannot end render pass in %s state", e.passState)
	}
	e.passState = CommandEncoderPassStateRecording
	e.passDepth--
	return nil
}

// BeginComputePass validates the encoder state and transitions to InComputePass.
//
// The encoder must be in the Recording state. After this call, the encoder
// is locked in the InComputePass state until EndComputePass is called.
func (e *CommandEncoder) BeginComputePass() error {
	if e.passState != CommandEncoderPassStateRecording {
		return fmt.Errorf("core: command encoder: cannot begin compute pass in %s state", e.passState)
	}
	e.passState = CommandEncoderPassStateInComputePass
	e.passDepth++
	return nil
}

// EndComputePass validates the encoder state and transitions back to Recording.
//
// The encoder must be in the InComputePass state.
func (e *CommandEncoder) EndComputePass() error {
	if e.passState != CommandEncoderPassStateInComputePass {
		return fmt.Errorf("core: command encoder: cannot end compute pass in %s state", e.passState)
	}
	e.passState = CommandEncoderPassStateRecording
	e.passDepth--
	return nil
}

// Finish validates the encoder state and transitions to Finished.
//
// The encoder must be in the Recording state with no open passes.
// Returns an error if the encoder is in the Error state, not in Recording
// state, or has open passes.
func (e *CommandEncoder) Finish() error {
	if e.passState == CommandEncoderPassStateError {
		return fmt.Errorf("core: command encoder: encoder in error state: %s", e.errorMessage)
	}
	if e.passState != CommandEncoderPassStateRecording {
		return fmt.Errorf("core: command encoder: cannot finish in %s state", e.passState)
	}
	if e.passDepth != 0 {
		return fmt.Errorf("core: command encoder: cannot finish with %d open passes", e.passDepth)
	}
	e.passState = CommandEncoderPassStateFinished
	return nil
}

// RecordError records the first error encountered by this encoder.
//
// The encoder transitions to the Error state. Subsequent calls to RecordError
// are ignored, preserving the first error message.
func (e *CommandEncoder) RecordError(msg string) {
	if e.passState == CommandEncoderPassStateError {
		return // Keep first error
	}
	e.errorMessage = msg
	e.passState = CommandEncoderPassStateError
}

// ErrorMessage returns the recorded error message, or empty string if none.
func (e *CommandEncoder) ErrorMessage() string {
	return e.errorMessage
}

// =============================================================================
// CommandBuffer State Methods (CORE-003)
// =============================================================================

// MarkSubmitted transitions the command buffer to the submitted state.
//
// Returns an error if the buffer has already been submitted.
func (cb *CommandBuffer) MarkSubmitted() error {
	if cb.submitState != CommandBufferSubmitStateAvailable {
		return fmt.Errorf("core: command buffer: already submitted")
	}
	cb.submitState = CommandBufferSubmitStateSubmitted
	return nil
}

// IsSubmitted returns whether the command buffer has been submitted.
func (cb *CommandBuffer) IsSubmitted() bool {
	return cb.submitState == CommandBufferSubmitStateSubmitted
}
