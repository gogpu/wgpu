//go:build !(js && wasm)

package core

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// =============================================================================
// ValidationError — extended tests (basic Error/Unwrap in core_test.go)
// =============================================================================

func TestValidationError_WithoutField(t *testing.T) {
	err := &ValidationError{Resource: "Texture", Message: "invalid"}
	want := "Texture: invalid"
	if got := err.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestValidationError_UnwrapNil(t *testing.T) {
	ve := &ValidationError{Resource: "Buffer", Message: "fail"}
	if ve.Unwrap() != nil {
		t.Error("Unwrap() should return nil when no cause")
	}
}

func TestIsValidationError_Wrapped(t *testing.T) {
	ve := &ValidationError{Resource: "X", Message: "Y"}
	wrapped := fmt.Errorf("wrapped: %w", ve)

	if !IsValidationError(wrapped) {
		t.Error("IsValidationError should return true for wrapped ValidationError")
	}
}

// =============================================================================
// IDError — extended tests (basic Error/Unwrap in core_test.go)
// =============================================================================

func TestIDError_WithCause(t *testing.T) {
	id := Zip(42, 7)
	cause := fmt.Errorf("not found")
	ie := NewIDError(id, "lookup failed", cause)

	msg := ie.Error()
	if !strings.Contains(msg, "42") || !strings.Contains(msg, "7") {
		t.Errorf("IDError.Error() = %q, want index=42 and epoch=7", msg)
	}
	if !strings.Contains(msg, "lookup failed") {
		t.Errorf("IDError.Error() missing message: %q", msg)
	}
	if !errors.Is(ie.Unwrap(), cause) {
		t.Error("IDError.Unwrap() did not return cause")
	}
}

func TestIsIDError_Wrapped(t *testing.T) {
	ie := &IDError{Message: "test"}
	wrapped := fmt.Errorf("w: %w", ie)
	if !IsIDError(wrapped) {
		t.Error("IsIDError should return true for wrapped IDError")
	}
	if IsIDError(fmt.Errorf("plain")) {
		t.Error("IsIDError should return false for plain error")
	}
}

// =============================================================================
// LimitError — extended tests
// =============================================================================

func TestLimitError_FieldValues(t *testing.T) {
	le := NewLimitError("Buffer", "maxBufferSize", 9999, 4096)
	msg := le.Error()
	if !strings.Contains(msg, "9999") || !strings.Contains(msg, "4096") {
		t.Errorf("LimitError.Error() missing values: %q", msg)
	}
}

func TestIsLimitError_Wrapped(t *testing.T) {
	le := &LimitError{Limit: "x", Resource: "y"}
	wrapped := fmt.Errorf("w: %w", le)
	if !IsLimitError(wrapped) {
		t.Error("IsLimitError should return true for wrapped LimitError")
	}
	if IsLimitError(fmt.Errorf("plain")) {
		t.Error("IsLimitError should return false for plain error")
	}
}

// =============================================================================
// FeatureError — extended tests
// =============================================================================

func TestIsFeatureError_Wrapped(t *testing.T) {
	fe := &FeatureError{Feature: "x", Resource: "y"}
	wrapped := fmt.Errorf("w: %w", fe)
	if !IsFeatureError(wrapped) {
		t.Error("IsFeatureError should return true for wrapped FeatureError")
	}
	if IsFeatureError(fmt.Errorf("plain")) {
		t.Error("IsFeatureError should return false for plain error")
	}
}

// =============================================================================
// CreateBufferError
// =============================================================================

func TestCreateBufferError_AllKinds(t *testing.T) {
	halErr := fmt.Errorf("device lost")
	tests := []struct {
		name    string
		err     *CreateBufferError
		wantSub string
	}{
		{
			name:    "ZeroSize",
			err:     &CreateBufferError{Kind: CreateBufferErrorZeroSize, Label: "buf1"},
			wantSub: "size must be greater than 0",
		},
		{
			name:    "MaxBufferSize",
			err:     &CreateBufferError{Kind: CreateBufferErrorMaxBufferSize, Label: "buf2", RequestedSize: 9999, MaxSize: 4096},
			wantSub: "exceeds maximum",
		},
		{
			name:    "EmptyUsage",
			err:     &CreateBufferError{Kind: CreateBufferErrorEmptyUsage, Label: "buf3"},
			wantSub: "usage must not be empty",
		},
		{
			name:    "InvalidUsage",
			err:     &CreateBufferError{Kind: CreateBufferErrorInvalidUsage, Label: "buf4"},
			wantSub: "invalid usage flags",
		},
		{
			name:    "MapReadWriteExclusive",
			err:     &CreateBufferError{Kind: CreateBufferErrorMapReadWriteExclusive, Label: "buf5"},
			wantSub: "mutually exclusive",
		},
		{
			name:    "HAL",
			err:     &CreateBufferError{Kind: CreateBufferErrorHAL, Label: "buf6", HALError: halErr},
			wantSub: "HAL error",
		},
		{
			name:    "Unknown",
			err:     &CreateBufferError{Kind: CreateBufferErrorKind(99), Label: "buf7"},
			wantSub: "unknown error",
		},
		{
			name:    "EmptyLabel",
			err:     &CreateBufferError{Kind: CreateBufferErrorZeroSize, Label: ""},
			wantSub: unnamedLabel,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := tt.err.Error()
			if !strings.Contains(msg, tt.wantSub) {
				t.Errorf("Error() = %q, want substring %q", msg, tt.wantSub)
			}
		})
	}
}

func TestCreateBufferError_Unwrap(t *testing.T) {
	halErr := fmt.Errorf("vk error")
	e := &CreateBufferError{Kind: CreateBufferErrorHAL, HALError: halErr}
	if !errors.Is(e.Unwrap(), halErr) {
		t.Error("Unwrap() did not return HALError")
	}

	e2 := &CreateBufferError{Kind: CreateBufferErrorZeroSize}
	if e2.Unwrap() != nil {
		t.Error("Unwrap() should return nil when no HAL error")
	}
}

func TestIsCreateBufferError(t *testing.T) {
	cbe := &CreateBufferError{Kind: CreateBufferErrorZeroSize}
	wrapped := fmt.Errorf("w: %w", cbe)
	if !IsCreateBufferError(cbe) {
		t.Error("should match direct")
	}
	if !IsCreateBufferError(wrapped) {
		t.Error("should match wrapped")
	}
	if IsCreateBufferError(fmt.Errorf("plain")) {
		t.Error("should not match plain error")
	}
}

func TestCreateBufferError_ErrorsAs(t *testing.T) {
	halErr := fmt.Errorf("device lost")
	e := &CreateBufferError{Kind: CreateBufferErrorHAL, HALError: halErr}
	wrapped := fmt.Errorf("create buffer: %w", e)

	var cbe *CreateBufferError
	if !errors.As(wrapped, &cbe) {
		t.Fatal("errors.As should unwrap to CreateBufferError")
	}
	if cbe.Kind != CreateBufferErrorHAL {
		t.Errorf("Kind = %v, want HAL", cbe.Kind)
	}
}

// =============================================================================
// CreateCommandEncoderError — extended (basic Unwrap in core_command_encoder_test.go)
// =============================================================================

func TestCreateCommandEncoderError_AllKinds(t *testing.T) {
	halErr := fmt.Errorf("encoder fail")
	tests := []struct {
		name    string
		err     *CreateCommandEncoderError
		wantSub string
	}{
		{
			name:    "HAL",
			err:     &CreateCommandEncoderError{Kind: CreateCommandEncoderErrorHAL, Label: "enc1", HALError: halErr},
			wantSub: "HAL error",
		},
		{
			name:    "Unknown",
			err:     &CreateCommandEncoderError{Kind: CreateCommandEncoderErrorKind(99), Label: "enc2"},
			wantSub: "unknown error",
		},
		{
			name:    "EmptyLabel",
			err:     &CreateCommandEncoderError{Kind: CreateCommandEncoderErrorHAL, HALError: halErr},
			wantSub: unnamedLabel,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := tt.err.Error()
			if !strings.Contains(msg, tt.wantSub) {
				t.Errorf("Error() = %q, want substring %q", msg, tt.wantSub)
			}
		})
	}
}

func TestIsCreateCommandEncoderError(t *testing.T) {
	cee := &CreateCommandEncoderError{Kind: CreateCommandEncoderErrorHAL}
	if !IsCreateCommandEncoderError(cee) {
		t.Error("should match direct")
	}
	if !IsCreateCommandEncoderError(fmt.Errorf("w: %w", cee)) {
		t.Error("should match wrapped")
	}
	if IsCreateCommandEncoderError(fmt.Errorf("plain")) {
		t.Error("should not match plain error")
	}
}

// =============================================================================
// CreateTextureError
// =============================================================================

func TestCreateTextureError_AllKinds(t *testing.T) {
	halErr := fmt.Errorf("vk texture fail")
	tests := []struct {
		name    string
		err     *CreateTextureError
		wantSub string
	}{
		{
			name:    "ZeroDimension",
			err:     &CreateTextureError{Kind: CreateTextureErrorZeroDimension, Label: "tex1", RequestedWidth: 0, RequestedHeight: 256, RequestedDepth: 1},
			wantSub: "dimensions must be greater than 0",
		},
		{
			name:    "MaxDimension",
			err:     &CreateTextureError{Kind: CreateTextureErrorMaxDimension, Label: "tex2", RequestedWidth: 99999, MaxDimension: 8192},
			wantSub: "exceeds maximum",
		},
		{
			name:    "MaxArrayLayers",
			err:     &CreateTextureError{Kind: CreateTextureErrorMaxArrayLayers, Label: "tex3", RequestedDepth: 500, MaxDimension: 256},
			wantSub: "array layers",
		},
		{
			name:    "InvalidMipLevelCount",
			err:     &CreateTextureError{Kind: CreateTextureErrorInvalidMipLevelCount, Label: "tex4", RequestedMips: 20, MaxMips: 9},
			wantSub: "mip level count",
		},
		{
			name:    "InvalidSampleCount",
			err:     &CreateTextureError{Kind: CreateTextureErrorInvalidSampleCount, Label: "tex5", RequestedSamples: 8},
			wantSub: "invalid sample count",
		},
		{
			name:    "MultisampleMipLevel",
			err:     &CreateTextureError{Kind: CreateTextureErrorMultisampleMipLevel, Label: "tex6", RequestedMips: 4},
			wantSub: "multisampled texture must have mip level count of 1",
		},
		{
			name:    "MultisampleDimension",
			err:     &CreateTextureError{Kind: CreateTextureErrorMultisampleDimension, Label: "tex7"},
			wantSub: "multisampled texture must be 2D",
		},
		{
			name:    "MultisampleArrayLayers",
			err:     &CreateTextureError{Kind: CreateTextureErrorMultisampleArrayLayers, Label: "tex8", RequestedDepth: 4},
			wantSub: "multisampled texture must have 1 array layer",
		},
		{
			name:    "MultisampleStorageBinding",
			err:     &CreateTextureError{Kind: CreateTextureErrorMultisampleStorageBinding, Label: "tex9"},
			wantSub: "multisampled texture cannot have storage binding",
		},
		{
			name:    "EmptyUsage",
			err:     &CreateTextureError{Kind: CreateTextureErrorEmptyUsage, Label: "tex10"},
			wantSub: "usage must not be empty",
		},
		{
			name:    "InvalidUsage",
			err:     &CreateTextureError{Kind: CreateTextureErrorInvalidUsage, Label: "tex11"},
			wantSub: "invalid usage flags",
		},
		{
			name:    "InvalidFormat",
			err:     &CreateTextureError{Kind: CreateTextureErrorInvalidFormat, Label: "tex12"},
			wantSub: "format must not be undefined",
		},
		{
			name:    "InvalidDimension",
			err:     &CreateTextureError{Kind: CreateTextureErrorInvalidDimension, Label: "tex13"},
			wantSub: "dimension must not be undefined",
		},
		{
			name:    "HAL",
			err:     &CreateTextureError{Kind: CreateTextureErrorHAL, Label: "tex14", HALError: halErr},
			wantSub: "HAL error",
		},
		{
			name:    "Unknown",
			err:     &CreateTextureError{Kind: CreateTextureErrorKind(99), Label: "tex15"},
			wantSub: "unknown error",
		},
		{
			name:    "EmptyLabel",
			err:     &CreateTextureError{Kind: CreateTextureErrorZeroDimension},
			wantSub: unnamedLabel,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := tt.err.Error()
			if !strings.Contains(msg, tt.wantSub) {
				t.Errorf("Error() = %q, want substring %q", msg, tt.wantSub)
			}
		})
	}
}

func TestCreateTextureError_Unwrap(t *testing.T) {
	halErr := fmt.Errorf("texture hal fail")
	e := &CreateTextureError{Kind: CreateTextureErrorHAL, HALError: halErr}
	if !errors.Is(e.Unwrap(), halErr) {
		t.Error("Unwrap() did not return HALError")
	}
	e2 := &CreateTextureError{Kind: CreateTextureErrorEmptyUsage}
	if e2.Unwrap() != nil {
		t.Error("Unwrap() should return nil when no HAL error")
	}
}

func TestIsCreateTextureError(t *testing.T) {
	cte := &CreateTextureError{Kind: CreateTextureErrorZeroDimension}
	if !IsCreateTextureError(cte) {
		t.Error("should match direct")
	}
	if !IsCreateTextureError(fmt.Errorf("w: %w", cte)) {
		t.Error("should match wrapped")
	}
	if IsCreateTextureError(fmt.Errorf("plain")) {
		t.Error("should not match plain error")
	}
}

// =============================================================================
// CreateSamplerError
// =============================================================================

func TestCreateSamplerError_AllKinds(t *testing.T) {
	halErr := fmt.Errorf("sampler hal fail")
	tests := []struct {
		name    string
		err     *CreateSamplerError
		wantSub string
	}{
		{
			name:    "InvalidLodMinClamp",
			err:     &CreateSamplerError{Kind: CreateSamplerErrorInvalidLodMinClamp, Label: "s1", LodMinClamp: -1.0},
			wantSub: "LodMinClamp must be >= 0",
		},
		{
			name:    "InvalidLodMaxClamp",
			err:     &CreateSamplerError{Kind: CreateSamplerErrorInvalidLodMaxClamp, Label: "s2", LodMinClamp: 5.0, LodMaxClamp: 2.0},
			wantSub: "LodMaxClamp",
		},
		{
			name:    "InvalidAnisotropy",
			err:     &CreateSamplerError{Kind: CreateSamplerErrorInvalidAnisotropy, Label: "s3", Anisotropy: 0},
			wantSub: "anisotropy must be >= 1",
		},
		{
			name:    "AnisotropyRequiresLinearFiltering",
			err:     &CreateSamplerError{Kind: CreateSamplerErrorAnisotropyRequiresLinearFiltering, Label: "s4", Anisotropy: 16},
			wantSub: "requires linear",
		},
		{
			name:    "HAL",
			err:     &CreateSamplerError{Kind: CreateSamplerErrorHAL, Label: "s5", HALError: halErr},
			wantSub: "HAL error",
		},
		{
			name:    "Unknown",
			err:     &CreateSamplerError{Kind: CreateSamplerErrorKind(99), Label: "s6"},
			wantSub: "unknown error",
		},
		{
			name:    "EmptyLabel",
			err:     &CreateSamplerError{Kind: CreateSamplerErrorInvalidLodMinClamp},
			wantSub: unnamedLabel,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := tt.err.Error()
			if !strings.Contains(msg, tt.wantSub) {
				t.Errorf("Error() = %q, want substring %q", msg, tt.wantSub)
			}
		})
	}
}

func TestCreateSamplerError_Unwrap(t *testing.T) {
	halErr := fmt.Errorf("sampler fail")
	e := &CreateSamplerError{Kind: CreateSamplerErrorHAL, HALError: halErr}
	if !errors.Is(e.Unwrap(), halErr) {
		t.Error("Unwrap() did not return HALError")
	}
}

func TestIsCreateSamplerError(t *testing.T) {
	cse := &CreateSamplerError{Kind: CreateSamplerErrorInvalidLodMinClamp}
	if !IsCreateSamplerError(cse) {
		t.Error("should match direct")
	}
	if !IsCreateSamplerError(fmt.Errorf("w: %w", cse)) {
		t.Error("should match wrapped")
	}
	if IsCreateSamplerError(fmt.Errorf("plain")) {
		t.Error("should not match plain error")
	}
}

// =============================================================================
// CreateShaderModuleError
// =============================================================================

func TestCreateShaderModuleError_AllKinds(t *testing.T) {
	halErr := fmt.Errorf("compile fail")
	tests := []struct {
		name    string
		err     *CreateShaderModuleError
		wantSub string
	}{
		{
			name:    "NoSource",
			err:     &CreateShaderModuleError{Kind: CreateShaderModuleErrorNoSource, Label: "shader1"},
			wantSub: "must provide either WGSL or SPIRV",
		},
		{
			name:    "DualSource",
			err:     &CreateShaderModuleError{Kind: CreateShaderModuleErrorDualSource, Label: "shader2"},
			wantSub: "must not provide both",
		},
		{
			name:    "HAL",
			err:     &CreateShaderModuleError{Kind: CreateShaderModuleErrorHAL, Label: "shader3", HALError: halErr},
			wantSub: "HAL error",
		},
		{
			name:    "Unknown",
			err:     &CreateShaderModuleError{Kind: CreateShaderModuleErrorKind(99), Label: "shader4"},
			wantSub: "unknown error",
		},
		{
			name:    "EmptyLabel",
			err:     &CreateShaderModuleError{Kind: CreateShaderModuleErrorNoSource},
			wantSub: unnamedLabel,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := tt.err.Error()
			if !strings.Contains(msg, tt.wantSub) {
				t.Errorf("Error() = %q, want substring %q", msg, tt.wantSub)
			}
		})
	}
}

func TestCreateShaderModuleError_Unwrap(t *testing.T) {
	halErr := fmt.Errorf("shader compile")
	e := &CreateShaderModuleError{Kind: CreateShaderModuleErrorHAL, HALError: halErr}
	if !errors.Is(e.Unwrap(), halErr) {
		t.Error("Unwrap() did not return HALError")
	}
}

func TestIsCreateShaderModuleError(t *testing.T) {
	csme := &CreateShaderModuleError{Kind: CreateShaderModuleErrorNoSource}
	if !IsCreateShaderModuleError(csme) {
		t.Error("should match direct")
	}
	if !IsCreateShaderModuleError(fmt.Errorf("w: %w", csme)) {
		t.Error("should match wrapped")
	}
	if IsCreateShaderModuleError(fmt.Errorf("plain")) {
		t.Error("should not match plain error")
	}
}

// =============================================================================
// CreateRenderPipelineError
// =============================================================================

func TestCreateRenderPipelineError_AllKinds(t *testing.T) {
	halErr := fmt.Errorf("pipeline fail")
	tests := []struct {
		name    string
		err     *CreateRenderPipelineError
		wantSub string
	}{
		{
			name:    "MissingVertexModule",
			err:     &CreateRenderPipelineError{Kind: CreateRenderPipelineErrorMissingVertexModule, Label: "rp1"},
			wantSub: "vertex shader module must not be nil",
		},
		{
			name:    "MissingVertexEntryPoint",
			err:     &CreateRenderPipelineError{Kind: CreateRenderPipelineErrorMissingVertexEntryPoint, Label: "rp2"},
			wantSub: "vertex entry point must not be empty",
		},
		{
			name:    "MissingFragmentModule",
			err:     &CreateRenderPipelineError{Kind: CreateRenderPipelineErrorMissingFragmentModule, Label: "rp3"},
			wantSub: "fragment shader module must not be nil",
		},
		{
			name:    "MissingFragmentEntryPoint",
			err:     &CreateRenderPipelineError{Kind: CreateRenderPipelineErrorMissingFragmentEntryPoint, Label: "rp4"},
			wantSub: "fragment entry point must not be empty",
		},
		{
			name:    "NoFragmentTargets",
			err:     &CreateRenderPipelineError{Kind: CreateRenderPipelineErrorNoFragmentTargets, Label: "rp5"},
			wantSub: "must have at least one color target",
		},
		{
			name:    "TooManyColorTargets",
			err:     &CreateRenderPipelineError{Kind: CreateRenderPipelineErrorTooManyColorTargets, Label: "rp6", TargetCount: 12, MaxTargets: 8},
			wantSub: "exceeds maximum",
		},
		{
			name:    "InvalidSampleCount",
			err:     &CreateRenderPipelineError{Kind: CreateRenderPipelineErrorInvalidSampleCount, Label: "rp7", SampleCount: 3},
			wantSub: "invalid sample count",
		},
		{
			name:    "ColorTargetDepthFormat",
			err:     &CreateRenderPipelineError{Kind: CreateRenderPipelineErrorColorTargetDepthFormat, Label: "rp8", TargetIndex: 0, Format: "Depth32Float"},
			wantSub: "does not have a color aspect",
		},
		{
			name:    "DepthStencilColorFormat",
			err:     &CreateRenderPipelineError{Kind: CreateRenderPipelineErrorDepthStencilColorFormat, Label: "rp9", Format: "RGBA8Unorm"},
			wantSub: "is not a depth/stencil format",
		},
		{
			name:    "DepthFormatNoDepthAspect",
			err:     &CreateRenderPipelineError{Kind: CreateRenderPipelineErrorDepthFormatNoDepthAspect, Label: "rp10", Format: "Stencil8"},
			wantSub: "does not have a depth aspect",
		},
		{
			name:    "DepthFormatNoStencilAspect",
			err:     &CreateRenderPipelineError{Kind: CreateRenderPipelineErrorDepthFormatNoStencilAspect, Label: "rp11", Format: "Depth16Unorm"},
			wantSub: "does not have a stencil aspect",
		},
		{
			name:    "HAL",
			err:     &CreateRenderPipelineError{Kind: CreateRenderPipelineErrorHAL, Label: "rp12", HALError: halErr},
			wantSub: "HAL error",
		},
		{
			name:    "Unknown",
			err:     &CreateRenderPipelineError{Kind: CreateRenderPipelineErrorKind(99), Label: "rp13"},
			wantSub: "unknown error",
		},
		{
			name:    "EmptyLabel",
			err:     &CreateRenderPipelineError{Kind: CreateRenderPipelineErrorMissingVertexModule},
			wantSub: unnamedLabel,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := tt.err.Error()
			if !strings.Contains(msg, tt.wantSub) {
				t.Errorf("Error() = %q, want substring %q", msg, tt.wantSub)
			}
		})
	}
}

func TestCreateRenderPipelineError_Unwrap(t *testing.T) {
	halErr := fmt.Errorf("pipeline error")
	e := &CreateRenderPipelineError{Kind: CreateRenderPipelineErrorHAL, HALError: halErr}
	if !errors.Is(e.Unwrap(), halErr) {
		t.Error("Unwrap() did not return HALError")
	}
}

func TestIsCreateRenderPipelineError(t *testing.T) {
	crpe := &CreateRenderPipelineError{Kind: CreateRenderPipelineErrorMissingVertexModule}
	if !IsCreateRenderPipelineError(crpe) {
		t.Error("should match direct")
	}
	if !IsCreateRenderPipelineError(fmt.Errorf("w: %w", crpe)) {
		t.Error("should match wrapped")
	}
	if IsCreateRenderPipelineError(fmt.Errorf("plain")) {
		t.Error("should not match plain error")
	}
}

// =============================================================================
// CreateComputePipelineError
// =============================================================================

func TestCreateComputePipelineError_AllKinds(t *testing.T) {
	halErr := fmt.Errorf("compute fail")
	tests := []struct {
		name    string
		err     *CreateComputePipelineError
		wantSub string
	}{
		{
			name:    "MissingModule",
			err:     &CreateComputePipelineError{Kind: CreateComputePipelineErrorMissingModule, Label: "cp1"},
			wantSub: "compute shader module must not be nil",
		},
		{
			name:    "MissingEntryPoint",
			err:     &CreateComputePipelineError{Kind: CreateComputePipelineErrorMissingEntryPoint, Label: "cp2"},
			wantSub: "compute entry point must not be empty",
		},
		{
			name:    "HAL",
			err:     &CreateComputePipelineError{Kind: CreateComputePipelineErrorHAL, Label: "cp3", HALError: halErr},
			wantSub: "HAL error",
		},
		{
			name:    "WorkgroupSizeExceeded",
			err:     &CreateComputePipelineError{Kind: CreateComputePipelineErrorWorkgroupSizeExceeded, Label: "cp4", Dimension: "X", Size: 512, Limit: 256},
			wantSub: "exceeds device limit",
		},
		{
			name:    "WorkgroupSizeZero",
			err:     &CreateComputePipelineError{Kind: CreateComputePipelineErrorWorkgroupSizeZero, Label: "cp5", Dimension: "Y"},
			wantSub: "must not be zero",
		},
		{
			name:    "TooManyInvocations",
			err:     &CreateComputePipelineError{Kind: CreateComputePipelineErrorTooManyInvocations, Label: "cp6", TotalInvocations: 512, Limit: 256},
			wantSub: "total workgroup invocations",
		},
		{
			name:    "Unknown",
			err:     &CreateComputePipelineError{Kind: CreateComputePipelineErrorKind(99), Label: "cp7"},
			wantSub: "unknown error",
		},
		{
			name:    "EmptyLabel",
			err:     &CreateComputePipelineError{Kind: CreateComputePipelineErrorMissingModule},
			wantSub: unnamedLabel,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := tt.err.Error()
			if !strings.Contains(msg, tt.wantSub) {
				t.Errorf("Error() = %q, want substring %q", msg, tt.wantSub)
			}
		})
	}
}

func TestCreateComputePipelineError_Unwrap(t *testing.T) {
	halErr := fmt.Errorf("compute pipeline hal")
	e := &CreateComputePipelineError{Kind: CreateComputePipelineErrorHAL, HALError: halErr}
	if !errors.Is(e.Unwrap(), halErr) {
		t.Error("Unwrap() did not return HALError")
	}
}

func TestIsCreateComputePipelineError(t *testing.T) {
	ccpe := &CreateComputePipelineError{Kind: CreateComputePipelineErrorMissingModule}
	if !IsCreateComputePipelineError(ccpe) {
		t.Error("should match direct")
	}
	if !IsCreateComputePipelineError(fmt.Errorf("w: %w", ccpe)) {
		t.Error("should match wrapped")
	}
	if IsCreateComputePipelineError(fmt.Errorf("plain")) {
		t.Error("should not match plain error")
	}
}

// =============================================================================
// CreateBindGroupLayoutError
// =============================================================================

func TestCreateBindGroupLayoutError_AllKinds(t *testing.T) {
	halErr := fmt.Errorf("bgl hal fail")
	tests := []struct {
		name    string
		err     *CreateBindGroupLayoutError
		wantSub string
	}{
		{
			name:    "DuplicateBinding",
			err:     &CreateBindGroupLayoutError{Kind: CreateBindGroupLayoutErrorDuplicateBinding, Label: "bgl1", DuplicateBinding: 3},
			wantSub: "duplicate binding number 3",
		},
		{
			name:    "TooManyBindings",
			err:     &CreateBindGroupLayoutError{Kind: CreateBindGroupLayoutErrorTooManyBindings, Label: "bgl2", BindingCount: 64, MaxBindings: 32},
			wantSub: "exceeds maximum",
		},
		{
			name:    "HAL",
			err:     &CreateBindGroupLayoutError{Kind: CreateBindGroupLayoutErrorHAL, Label: "bgl3", HALError: halErr},
			wantSub: "HAL error",
		},
		{
			name:    "Unknown",
			err:     &CreateBindGroupLayoutError{Kind: CreateBindGroupLayoutErrorKind(99), Label: "bgl4"},
			wantSub: "unknown error",
		},
		{
			name:    "EmptyLabel",
			err:     &CreateBindGroupLayoutError{Kind: CreateBindGroupLayoutErrorDuplicateBinding, DuplicateBinding: 0},
			wantSub: unnamedLabel,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := tt.err.Error()
			if !strings.Contains(msg, tt.wantSub) {
				t.Errorf("Error() = %q, want substring %q", msg, tt.wantSub)
			}
		})
	}
}

func TestCreateBindGroupLayoutError_Unwrap(t *testing.T) {
	halErr := fmt.Errorf("bgl fail")
	e := &CreateBindGroupLayoutError{Kind: CreateBindGroupLayoutErrorHAL, HALError: halErr}
	if !errors.Is(e.Unwrap(), halErr) {
		t.Error("Unwrap() did not return HALError")
	}
}

func TestIsCreateBindGroupLayoutError(t *testing.T) {
	cble := &CreateBindGroupLayoutError{Kind: CreateBindGroupLayoutErrorDuplicateBinding}
	if !IsCreateBindGroupLayoutError(cble) {
		t.Error("should match direct")
	}
	if !IsCreateBindGroupLayoutError(fmt.Errorf("w: %w", cble)) {
		t.Error("should match wrapped")
	}
	if IsCreateBindGroupLayoutError(fmt.Errorf("plain")) {
		t.Error("should not match plain error")
	}
}

// =============================================================================
// CreateBindGroupError
// =============================================================================

func TestCreateBindGroupError_AllKinds(t *testing.T) {
	halErr := fmt.Errorf("bg hal fail")
	tests := []struct {
		name    string
		err     *CreateBindGroupError
		wantSub string
	}{
		{
			name:    "MissingLayout",
			err:     &CreateBindGroupError{Kind: CreateBindGroupErrorMissingLayout, Label: "bg1"},
			wantSub: "layout must not be nil",
		},
		{
			name:    "BindingsNumMismatch",
			err:     &CreateBindGroupError{Kind: CreateBindGroupErrorBindingsNumMismatch, Label: "bg2", Expected: 3, Actual: 5},
			wantSub: "does not match",
		},
		{
			name:    "MissingBindingDeclaration",
			err:     &CreateBindGroupError{Kind: CreateBindGroupErrorMissingBindingDeclaration, Label: "bg3", Binding: 7},
			wantSub: "unable to find",
		},
		{
			name:    "DuplicateBinding",
			err:     &CreateBindGroupError{Kind: CreateBindGroupErrorDuplicateBinding, Label: "bg4", Binding: 2},
			wantSub: "at least twice",
		},
		{
			name:    "BufferUsageMismatch",
			err:     &CreateBindGroupError{Kind: CreateBindGroupErrorBufferUsageMismatch, Label: "bg5", Binding: 0, ExpectedUsage: 0x40, ActualUsage: 0x20},
			wantSub: "usage mismatch",
		},
		{
			name:    "BufferOffsetAlignment",
			err:     &CreateBindGroupError{Kind: CreateBindGroupErrorBufferOffsetAlignment, Label: "bg6", Binding: 1, Offset: 13, Alignment: 16},
			wantSub: "not aligned",
		},
		{
			name:    "BufferBindingSizeTooLarge",
			err:     &CreateBindGroupError{Kind: CreateBindGroupErrorBufferBindingSizeTooLarge, Label: "bg7", Binding: 0, Size: 70000, MaxSize: 65536},
			wantSub: "exceeds maximum",
		},
		{
			name:    "BufferBoundsOverflow",
			err:     &CreateBindGroupError{Kind: CreateBindGroupErrorBufferBoundsOverflow, Label: "bg8", Binding: 0, Offset: 100, Size: 200, BufferSize: 256},
			wantSub: "overflows",
		},
		{
			name:    "BufferBindingZeroSize",
			err:     &CreateBindGroupError{Kind: CreateBindGroupErrorBufferBindingZeroSize, Label: "bg9", Binding: 0},
			wantSub: "zero effective binding size",
		},
		{
			name:    "StorageBufferSizeAlignment",
			err:     &CreateBindGroupError{Kind: CreateBindGroupErrorStorageBufferSizeAlignment, Label: "bg10", Binding: 0, Size: 13},
			wantSub: "not a multiple of 4",
		},
		{
			name:    "MinBindingSizeMismatch",
			err:     &CreateBindGroupError{Kind: CreateBindGroupErrorMinBindingSizeMismatch, Label: "bg11", Binding: 0, Size: 32, MinBindingSize: 64},
			wantSub: "less than minimum",
		},
		{
			name:    "HAL",
			err:     &CreateBindGroupError{Kind: CreateBindGroupErrorHAL, Label: "bg12", HALError: halErr},
			wantSub: "HAL error",
		},
		{
			name:    "Unknown",
			err:     &CreateBindGroupError{Kind: CreateBindGroupErrorKind(99), Label: "bg13"},
			wantSub: "unknown error",
		},
		{
			name:    "EmptyLabel",
			err:     &CreateBindGroupError{Kind: CreateBindGroupErrorMissingLayout},
			wantSub: unnamedLabel,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := tt.err.Error()
			if !strings.Contains(msg, tt.wantSub) {
				t.Errorf("Error() = %q, want substring %q", msg, tt.wantSub)
			}
		})
	}
}

func TestCreateBindGroupError_Unwrap(t *testing.T) {
	halErr := fmt.Errorf("bg fail")
	e := &CreateBindGroupError{Kind: CreateBindGroupErrorHAL, HALError: halErr}
	if !errors.Is(e.Unwrap(), halErr) {
		t.Error("Unwrap() did not return HALError")
	}
}

func TestIsCreateBindGroupError(t *testing.T) {
	cbge := &CreateBindGroupError{Kind: CreateBindGroupErrorMissingLayout}
	if !IsCreateBindGroupError(cbge) {
		t.Error("should match direct")
	}
	if !IsCreateBindGroupError(fmt.Errorf("w: %w", cbge)) {
		t.Error("should match wrapped")
	}
	if IsCreateBindGroupError(fmt.Errorf("plain")) {
		t.Error("should not match plain error")
	}
}

// =============================================================================
// CreatePipelineLayoutError
// =============================================================================

func TestCreatePipelineLayoutError_AllKinds(t *testing.T) {
	halErr := fmt.Errorf("pl hal fail")
	tests := []struct {
		name    string
		err     *CreatePipelineLayoutError
		wantSub string
	}{
		{
			name:    "TooManyGroups",
			err:     &CreatePipelineLayoutError{Kind: CreatePipelineLayoutErrorTooManyGroups, Label: "pl1", Count: 8, MaxGroups: 4},
			wantSub: "exceeds device limit",
		},
		{
			name:    "HAL",
			err:     &CreatePipelineLayoutError{Kind: CreatePipelineLayoutErrorHAL, Label: "pl2", HALError: halErr},
			wantSub: "HAL error",
		},
		{
			name:    "Unknown",
			err:     &CreatePipelineLayoutError{Kind: CreatePipelineLayoutErrorKind(99), Label: "pl3"},
			wantSub: "unknown error",
		},
		{
			name:    "EmptyLabel",
			err:     &CreatePipelineLayoutError{Kind: CreatePipelineLayoutErrorTooManyGroups, Count: 5, MaxGroups: 4},
			wantSub: unnamedLabel,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := tt.err.Error()
			if !strings.Contains(msg, tt.wantSub) {
				t.Errorf("Error() = %q, want substring %q", msg, tt.wantSub)
			}
		})
	}
}

func TestCreatePipelineLayoutError_Unwrap(t *testing.T) {
	halErr := fmt.Errorf("layout fail")
	e := &CreatePipelineLayoutError{Kind: CreatePipelineLayoutErrorHAL, HALError: halErr}
	if !errors.Is(e.Unwrap(), halErr) {
		t.Error("Unwrap() did not return HALError")
	}
}

func TestIsCreatePipelineLayoutError(t *testing.T) {
	cple := &CreatePipelineLayoutError{Kind: CreatePipelineLayoutErrorTooManyGroups}
	if !IsCreatePipelineLayoutError(cple) {
		t.Error("should match direct")
	}
	if !IsCreatePipelineLayoutError(fmt.Errorf("w: %w", cple)) {
		t.Error("should match wrapped")
	}
	if IsCreatePipelineLayoutError(fmt.Errorf("plain")) {
		t.Error("should not match plain error")
	}
}

// =============================================================================
// EncoderStateError — extended (basic test in core_command_encoder_test.go)
// =============================================================================

func TestEncoderStateError_WithCause(t *testing.T) {
	cause := fmt.Errorf("deferred validation failure")
	e := &EncoderStateError{
		Operation: "Draw",
		Status:    CommandEncoderStatusError,
		Cause:     cause,
	}
	msg := e.Error()
	if !strings.Contains(msg, "Draw") {
		t.Errorf("Error() missing operation: %q", msg)
	}
	if !strings.Contains(msg, "deferred validation failure") {
		t.Errorf("Error() missing cause: %q", msg)
	}
	if !errors.Is(e.Unwrap(), cause) {
		t.Error("Unwrap() did not return cause")
	}
}

func TestEncoderStateError_WithoutCause(t *testing.T) {
	e := &EncoderStateError{
		Operation: "Finish",
		Status:    CommandEncoderStatusLocked,
	}
	msg := e.Error()
	if !strings.Contains(msg, "Finish") {
		t.Errorf("Error() missing operation: %q", msg)
	}
	if !strings.Contains(msg, "Locked") {
		t.Errorf("Error() missing status: %q", msg)
	}
	if e.Unwrap() != nil {
		t.Error("Unwrap() should return nil when no cause")
	}
}

func TestEncoderStateError_ErrorChaining(t *testing.T) {
	// Verify errors.Is/errors.As work through the chain when
	// an EncoderStateError wraps a cause via Unwrap.
	rootCause := &CreateBufferError{Kind: CreateBufferErrorZeroSize, Label: "inner"}
	ese := &EncoderStateError{
		Operation: "CopyBuffer",
		Status:    CommandEncoderStatusError,
		Cause:     rootCause,
	}

	// errors.As through chain
	var cbe *CreateBufferError
	if !errors.As(ese, &cbe) {
		t.Fatal("errors.As should find CreateBufferError through EncoderStateError chain")
	}
	if cbe.Kind != CreateBufferErrorZeroSize {
		t.Errorf("Kind = %v, want ZeroSize", cbe.Kind)
	}
}

// =============================================================================
// Sentinel error identity tests
// =============================================================================

func TestSentinelErrorsDistinct(t *testing.T) {
	// Verify sentinel errors are distinct and can be tested with errors.Is.
	sentinels := []struct {
		name string
		err  error
	}{
		{"ErrInvalidID", ErrInvalidID},
		{"ErrResourceNotFound", ErrResourceNotFound},
		{"ErrEpochMismatch", ErrEpochMismatch},
		{"ErrRegistryFull", ErrRegistryFull},
		{"ErrResourceInUse", ErrResourceInUse},
		{"ErrAlreadyDestroyed", ErrAlreadyDestroyed},
		{"ErrDeviceLost", ErrDeviceLost},
		{"ErrDeviceDestroyed", ErrDeviceDestroyed},
		{"ErrResourceDestroyed", ErrResourceDestroyed},
	}

	for i, s := range sentinels {
		t.Run(s.name, func(t *testing.T) {
			// Each sentinel should have a non-empty message.
			if s.err.Error() == "" {
				t.Errorf("%s has empty Error()", s.name)
			}
			// Wrapped sentinel should be findable via errors.Is.
			wrapped := fmt.Errorf("context: %w", s.err)
			if !errors.Is(wrapped, s.err) {
				t.Errorf("errors.Is failed to find %s through wrapping", s.name)
			}
			// Each sentinel should be distinct from all others.
			for j, other := range sentinels {
				if i != j && errors.Is(s.err, other.err) {
					t.Errorf("%s should not match %s", s.name, other.name)
				}
			}
		})
	}
}
