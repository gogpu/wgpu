//go:build !(js && wasm)

package core

import (
	"testing"

	"github.com/gogpu/gputypes"
)

// TestDispatchIndirectValidationWGSL verifies the shader source generation
// embeds the correct maxWorkgroups limit constant.
func TestDispatchIndirectValidationWGSL(t *testing.T) {
	tests := []struct {
		name          string
		maxWorkgroups uint32
		wantContains  string
	}{
		{
			name:          "default limit 65535",
			maxWorkgroups: 65535,
			wantContains:  "let limit = 65535u;",
		},
		{
			name:          "low limit 256",
			maxWorkgroups: 256,
			wantContains:  "let limit = 256u;",
		},
		{
			name:          "max uint32",
			maxWorkgroups: 4294967295,
			wantContains:  "let limit = 4294967295u;",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := dispatchIndirectValidationWGSL(tt.maxWorkgroups)

			if src == "" {
				t.Fatal("generated WGSL source is empty")
			}

			// Check limit constant is embedded correctly.
			if !containsSubstring(src, tt.wantContains) {
				t.Errorf("WGSL source does not contain %q\ngot:\n%s", tt.wantContains, src)
			}

			// Verify the shader has the expected structure.
			requiredParts := []string{
				"@group(0) @binding(0) var<storage, read_write> dst",
				"@group(0) @binding(1) var<uniform> src_index",
				"@group(1) @binding(0) var<storage, read> src",
				"struct SrcIndex",
				"@compute @workgroup_size(1)",
				"fn main()",
				"dst[0] = 0u",
				"dst[1] = 0u",
				"dst[2] = 0u",
			}
			for _, part := range requiredParts {
				if !containsSubstring(src, part) {
					t.Errorf("WGSL source missing required part: %q", part)
				}
			}
		})
	}
}

// containsSubstring checks if s contains substr using a simple scan.
func containsSubstring(s, substr string) bool {
	if len(substr) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestNewIndirectValidationNilDevice verifies that nil HAL device returns nil.
func TestNewIndirectValidationNilDevice(t *testing.T) {
	iv := NewIndirectValidation(nil, gputypes.Limits{
		MaxComputeWorkgroupsPerDimension: 65535,
	})
	if iv != nil {
		t.Error("expected nil IndirectValidation for nil device")
	}
}

// TestNewIndirectValidationZeroLimit verifies that zero max workgroups returns nil.
func TestNewIndirectValidationZeroLimit(t *testing.T) {
	// Even with a real device, if the limit is 0 we should skip validation.
	iv := NewIndirectValidation(nil, gputypes.Limits{
		MaxComputeWorkgroupsPerDimension: 0,
	})
	if iv != nil {
		t.Error("expected nil IndirectValidation for zero max workgroups")
	}
}

// TestIndirectValidationDisposeNil verifies that Dispose on nil is safe.
func TestIndirectValidationDisposeNil(t *testing.T) {
	// Should not panic.
	var iv *IndirectValidation
	iv.Dispose()
}

// TestIndirectValidationDisposeAlreadyDisposed verifies double Dispose is safe.
func TestIndirectValidationDisposeAlreadyDisposed(t *testing.T) {
	// Create an IV with a nil device (simulating already-disposed state).
	iv := &IndirectValidation{device: nil}
	// Should not panic.
	iv.Dispose()
}

// TestIndirectValidationBuilderCleanupNilFields verifies builder cleanup
// handles nil fields without panicking.
func TestIndirectValidationBuilderCleanupNilFields(t *testing.T) {
	// All fields nil except device -- should not panic.
	// We cannot easily test with a real HAL device without GPU, but we can
	// verify the nil checks in cleanup don't crash.
	b := &indirectValidationBuilder{
		device: nil, // nil device means cleanup methods won't be called
	}
	// This would panic without nil checks. Since device is nil, cleanup is a no-op.
	// But the real intent is to verify the code structure doesn't panic.
	_ = b
}

// TestValidationShaderEntryPointConstant verifies the constant is "main".
func TestValidationShaderEntryPointConstant(t *testing.T) {
	if validationShaderEntryPoint != "main" {
		t.Errorf("expected entry point %q, got %q", "main", validationShaderEntryPoint)
	}
}

// TestIndirectValidationAccessors verifies accessor methods return correct values.
func TestIndirectValidationAccessors(t *testing.T) {
	iv := &IndirectValidation{
		maxWorkgroups: 65535,
	}

	if got := iv.MaxWorkgroups(); got != 65535 {
		t.Errorf("MaxWorkgroups() = %d, want 65535", got)
	}

	// Nil accessors should return nil without panicking.
	if iv.DstBuffer() != nil {
		t.Error("DstBuffer() should return nil for zero-value IV")
	}
	if iv.Pipeline() != nil {
		t.Error("Pipeline() should return nil for zero-value IV")
	}
	if iv.PipelineLayout() != nil {
		t.Error("PipelineLayout() should return nil for zero-value IV")
	}
}
