package core

import (
	"fmt"
)

// ComputePipelineDescriptor describes a compute pipeline.
// This is the Core layer descriptor that uses resource IDs.
type ComputePipelineDescriptor struct {
	// Label is a debug label for the pipeline.
	Label string

	// Layout is the pipeline layout.
	// If zero, an automatic layout will be derived from the shader.
	Layout PipelineLayoutID

	// Compute describes the compute shader stage.
	Compute ProgrammableStage
}

// ProgrammableStage describes a programmable shader stage.
type ProgrammableStage struct {
	// Module is the shader module containing the entry point.
	Module ShaderModuleID

	// EntryPoint is the name of the entry point function in the shader.
	EntryPoint string

	// Constants are pipeline-overridable constants.
	// The keys are the constant names and values are their overridden values.
	Constants map[string]float64
}

// DeviceCreateComputePipeline creates a compute pipeline on this device.
//
// The pipeline combines a compute shader with a pipeline layout to define
// how resources are bound and the shader is executed.
//
// Returns a compute pipeline ID that can be used to access the pipeline,
// or an error if pipeline creation fails.
func DeviceCreateComputePipeline(deviceID DeviceID, desc *ComputePipelineDescriptor) (ComputePipelineID, error) {
	hub := GetGlobal().Hub()

	// Verify the device exists
	_, err := hub.GetDevice(deviceID)
	if err != nil {
		return ComputePipelineID{}, fmt.Errorf("invalid device: %w", err)
	}

	if desc == nil {
		return ComputePipelineID{}, fmt.Errorf("compute pipeline descriptor is required")
	}

	// Validate shader module
	if desc.Compute.Module.IsZero() {
		return ComputePipelineID{}, fmt.Errorf("compute shader module is required")
	}

	_, err = hub.GetShaderModule(desc.Compute.Module)
	if err != nil {
		return ComputePipelineID{}, fmt.Errorf("invalid shader module: %w", err)
	}

	// Validate entry point
	if desc.Compute.EntryPoint == "" {
		return ComputePipelineID{}, fmt.Errorf("compute entry point is required")
	}

	// Validate pipeline layout if provided
	if !desc.Layout.IsZero() {
		_, err = hub.GetPipelineLayout(desc.Layout)
		if err != nil {
			return ComputePipelineID{}, fmt.Errorf("invalid pipeline layout: %w", err)
		}
	}

	// TODO: Create actual compute pipeline via HAL backend
	// For now, create a placeholder pipeline

	pipeline := ComputePipeline{}
	pipelineID := hub.RegisterComputePipeline(pipeline)

	return pipelineID, nil
}

// DeviceDestroyComputePipeline destroys a compute pipeline.
//
// After calling this function, the compute pipeline ID becomes invalid
// and must not be used.
//
// Returns an error if the pipeline ID is invalid.
func DeviceDestroyComputePipeline(pipelineID ComputePipelineID) error {
	hub := GetGlobal().Hub()

	_, err := hub.UnregisterComputePipeline(pipelineID)
	if err != nil {
		return fmt.Errorf("failed to destroy compute pipeline: %w", err)
	}

	return nil
}

// GetComputePipeline retrieves compute pipeline data.
// Returns an error if the pipeline ID is invalid.
func GetComputePipeline(id ComputePipelineID) (*ComputePipeline, error) {
	hub := GetGlobal().Hub()
	pipeline, err := hub.GetComputePipeline(id)
	if err != nil {
		return nil, fmt.Errorf("failed to get compute pipeline: %w", err)
	}
	return &pipeline, nil
}
