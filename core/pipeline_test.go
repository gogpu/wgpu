package core

import (
	"sync"
	"testing"

	"github.com/gogpu/wgpu/types"
)

// createTestShaderModule creates a shader module for testing
func createTestShaderModule(t *testing.T, deviceID DeviceID) ShaderModuleID {
	t.Helper()
	moduleID, err := DeviceCreateShaderModule(deviceID, &types.ShaderModuleDescriptor{
		Label: "Test Compute Shader",
		Source: types.ShaderSourceWGSL{
			Code: "@compute @workgroup_size(64) fn main() {}",
		},
	})
	if err != nil {
		t.Fatalf("DeviceCreateShaderModule() error = %v", err)
	}
	return moduleID
}

func TestDeviceCreateComputePipeline(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T) (DeviceID, *ComputePipelineDescriptor)
		wantErr bool
	}{
		{
			name: "create pipeline with valid descriptor",
			setup: func(t *testing.T) (DeviceID, *ComputePipelineDescriptor) {
				ResetGlobal()
				adapterID := createTestAdapter(t, types.Features(0), types.DefaultLimits())
				deviceID, _ := CreateDevice(adapterID, nil)
				moduleID := createTestShaderModule(t, deviceID)
				return deviceID, &ComputePipelineDescriptor{
					Label: "Test Compute Pipeline",
					Compute: ProgrammableStage{
						Module:     moduleID,
						EntryPoint: "main",
					},
				}
			},
			wantErr: false,
		},
		{
			name: "create pipeline with constants",
			setup: func(t *testing.T) (DeviceID, *ComputePipelineDescriptor) {
				ResetGlobal()
				adapterID := createTestAdapter(t, types.Features(0), types.DefaultLimits())
				deviceID, _ := CreateDevice(adapterID, nil)
				moduleID := createTestShaderModule(t, deviceID)
				return deviceID, &ComputePipelineDescriptor{
					Label: "Pipeline with Constants",
					Compute: ProgrammableStage{
						Module:     moduleID,
						EntryPoint: "main",
						Constants: map[string]float64{
							"workgroup_size": 256,
							"threshold":      0.5,
						},
					},
				}
			},
			wantErr: false,
		},
		{
			name: "fail with nil descriptor",
			setup: func(t *testing.T) (DeviceID, *ComputePipelineDescriptor) {
				ResetGlobal()
				adapterID := createTestAdapter(t, types.Features(0), types.DefaultLimits())
				deviceID, _ := CreateDevice(adapterID, nil)
				return deviceID, nil
			},
			wantErr: true,
		},
		{
			name: "fail with invalid device",
			setup: func(t *testing.T) (DeviceID, *ComputePipelineDescriptor) {
				ResetGlobal()
				return DeviceID{}, &ComputePipelineDescriptor{
					Label: "Test",
					Compute: ProgrammableStage{
						EntryPoint: "main",
					},
				}
			},
			wantErr: true,
		},
		{
			name: "fail with missing shader module",
			setup: func(t *testing.T) (DeviceID, *ComputePipelineDescriptor) {
				ResetGlobal()
				adapterID := createTestAdapter(t, types.Features(0), types.DefaultLimits())
				deviceID, _ := CreateDevice(adapterID, nil)
				return deviceID, &ComputePipelineDescriptor{
					Label: "Missing Module",
					Compute: ProgrammableStage{
						Module:     ShaderModuleID{}, // Zero ID
						EntryPoint: "main",
					},
				}
			},
			wantErr: true,
		},
		{
			name: "fail with invalid shader module",
			setup: func(t *testing.T) (DeviceID, *ComputePipelineDescriptor) {
				ResetGlobal()
				adapterID := createTestAdapter(t, types.Features(0), types.DefaultLimits())
				deviceID, _ := CreateDevice(adapterID, nil)
				// Create a fake module ID that doesn't exist
				invalidModuleID := NewID[shaderModuleMarker](999, 1)
				return deviceID, &ComputePipelineDescriptor{
					Label: "Invalid Module",
					Compute: ProgrammableStage{
						Module:     invalidModuleID,
						EntryPoint: "main",
					},
				}
			},
			wantErr: true,
		},
		{
			name: "fail with missing entry point",
			setup: func(t *testing.T) (DeviceID, *ComputePipelineDescriptor) {
				ResetGlobal()
				adapterID := createTestAdapter(t, types.Features(0), types.DefaultLimits())
				deviceID, _ := CreateDevice(adapterID, nil)
				moduleID := createTestShaderModule(t, deviceID)
				return deviceID, &ComputePipelineDescriptor{
					Label: "Missing Entry Point",
					Compute: ProgrammableStage{
						Module:     moduleID,
						EntryPoint: "", // Empty entry point
					},
				}
			},
			wantErr: true,
		},
		{
			name: "fail with invalid pipeline layout",
			setup: func(t *testing.T) (DeviceID, *ComputePipelineDescriptor) {
				ResetGlobal()
				adapterID := createTestAdapter(t, types.Features(0), types.DefaultLimits())
				deviceID, _ := CreateDevice(adapterID, nil)
				moduleID := createTestShaderModule(t, deviceID)
				// Create a fake layout ID that doesn't exist
				invalidLayoutID := NewID[pipelineLayoutMarker](999, 1)
				return deviceID, &ComputePipelineDescriptor{
					Label:  "Invalid Layout",
					Layout: invalidLayoutID,
					Compute: ProgrammableStage{
						Module:     moduleID,
						EntryPoint: "main",
					},
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deviceID, desc := tt.setup(t)
			pipelineID, err := DeviceCreateComputePipeline(deviceID, desc)

			if (err != nil) != tt.wantErr {
				t.Errorf("DeviceCreateComputePipeline() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				// Verify pipeline was created
				hub := GetGlobal().Hub()
				_, err := hub.GetComputePipeline(pipelineID)
				if err != nil {
					t.Errorf("ComputePipeline should exist after creation")
				}
			}
		})
	}
}

func TestDeviceDestroyComputePipeline(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T) ComputePipelineID
		wantErr bool
	}{
		{
			name: "destroy valid pipeline",
			setup: func(t *testing.T) ComputePipelineID {
				ResetGlobal()
				adapterID := createTestAdapter(t, types.Features(0), types.DefaultLimits())
				deviceID, _ := CreateDevice(adapterID, nil)
				moduleID := createTestShaderModule(t, deviceID)
				pipelineID, _ := DeviceCreateComputePipeline(deviceID, &ComputePipelineDescriptor{
					Label: "Test Pipeline",
					Compute: ProgrammableStage{
						Module:     moduleID,
						EntryPoint: "main",
					},
				})
				return pipelineID
			},
			wantErr: false,
		},
		{
			name: "fail with invalid pipeline",
			setup: func(t *testing.T) ComputePipelineID {
				ResetGlobal()
				return ComputePipelineID{} // Invalid ID
			},
			wantErr: true,
		},
		{
			name: "fail with non-existent pipeline",
			setup: func(t *testing.T) ComputePipelineID {
				ResetGlobal()
				// Create a fake pipeline ID that doesn't exist
				return NewID[computePipelineMarker](999, 1)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pipelineID := tt.setup(t)
			err := DeviceDestroyComputePipeline(pipelineID)

			if (err != nil) != tt.wantErr {
				t.Errorf("DeviceDestroyComputePipeline() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				// Verify pipeline no longer exists
				_, err := GetComputePipeline(pipelineID)
				if err == nil {
					t.Errorf("GetComputePipeline() should fail after destroy")
				}
			}
		})
	}
}

func TestGetComputePipeline(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T) ComputePipelineID
		wantErr bool
	}{
		{
			name: "get valid pipeline",
			setup: func(t *testing.T) ComputePipelineID {
				ResetGlobal()
				adapterID := createTestAdapter(t, types.Features(0), types.DefaultLimits())
				deviceID, _ := CreateDevice(adapterID, nil)
				moduleID := createTestShaderModule(t, deviceID)
				pipelineID, _ := DeviceCreateComputePipeline(deviceID, &ComputePipelineDescriptor{
					Label: "Test Pipeline",
					Compute: ProgrammableStage{
						Module:     moduleID,
						EntryPoint: "main",
					},
				})
				return pipelineID
			},
			wantErr: false,
		},
		{
			name: "fail with invalid pipeline",
			setup: func(t *testing.T) ComputePipelineID {
				ResetGlobal()
				return ComputePipelineID{}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pipelineID := tt.setup(t)
			_, err := GetComputePipeline(pipelineID)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetComputePipeline() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestComputePipelineConcurrentAccess(t *testing.T) {
	ResetGlobal()

	adapterID := createTestAdapter(t, types.Features(0), types.DefaultLimits())
	deviceID, err := CreateDevice(adapterID, nil)
	if err != nil {
		t.Fatalf("CreateDevice() error = %v", err)
	}
	moduleID := createTestShaderModule(t, deviceID)

	// Create multiple pipelines concurrently
	const numPipelines = 10
	var wg sync.WaitGroup
	pipelineIDs := make([]ComputePipelineID, numPipelines)
	errors := make([]error, numPipelines)

	for i := 0; i < numPipelines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			desc := &ComputePipelineDescriptor{
				Label: "Concurrent Pipeline",
				Compute: ProgrammableStage{
					Module:     moduleID,
					EntryPoint: "main",
				},
			}
			pipelineIDs[idx], errors[idx] = DeviceCreateComputePipeline(deviceID, desc)
		}(i)
	}

	wg.Wait()

	// Verify all pipelines were created
	for i, err := range errors {
		if err != nil {
			t.Errorf("Pipeline %d creation failed: %v", i, err)
		}
	}

	// Verify all pipelines can be accessed
	for i, pipelineID := range pipelineIDs {
		_, err := GetComputePipeline(pipelineID)
		if err != nil {
			t.Errorf("Pipeline %d access failed: %v", i, err)
		}
	}

	// Destroy all pipelines concurrently
	for i := 0; i < numPipelines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			errors[idx] = DeviceDestroyComputePipeline(pipelineIDs[idx])
		}(i)
	}

	wg.Wait()

	// Verify all pipelines were destroyed
	for i, err := range errors {
		if err != nil {
			t.Errorf("Pipeline %d destruction failed: %v", i, err)
		}
	}
}
