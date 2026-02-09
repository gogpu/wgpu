package core

import (
	"sync"
	"testing"
)

func TestErrorScopePushPop(t *testing.T) {
	tests := []struct {
		name        string
		filter      ErrorFilter
		reportErr   bool
		errFilter   ErrorFilter
		errMessage  string
		wantGPUErr  bool
		wantMessage string
	}{
		{
			name:        "validation error captured",
			filter:      ErrorFilterValidation,
			reportErr:   true,
			errFilter:   ErrorFilterValidation,
			errMessage:  "buffer size must be greater than 0",
			wantGPUErr:  true,
			wantMessage: "buffer size must be greater than 0",
		},
		{
			name:        "out-of-memory error captured",
			filter:      ErrorFilterOutOfMemory,
			reportErr:   true,
			errFilter:   ErrorFilterOutOfMemory,
			errMessage:  "failed to allocate 1GB buffer",
			wantGPUErr:  true,
			wantMessage: "failed to allocate 1GB buffer",
		},
		{
			name:        "internal error captured",
			filter:      ErrorFilterInternal,
			reportErr:   true,
			errFilter:   ErrorFilterInternal,
			errMessage:  "unexpected shader compilation failure",
			wantGPUErr:  true,
			wantMessage: "unexpected shader compilation failure",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := NewErrorScopeManager()

			mgr.PushErrorScope(tt.filter)

			if tt.reportErr {
				captured := mgr.ReportError(tt.errFilter, tt.errMessage)
				if !captured {
					t.Fatal("ReportError returned false, expected error to be captured")
				}
			}

			gpuErr, err := mgr.PopErrorScope()
			if err != nil {
				t.Fatalf("PopErrorScope returned unexpected error: %v", err)
			}

			if tt.wantGPUErr && gpuErr == nil {
				t.Fatal("PopErrorScope returned nil, expected GPUError")
			} else if !tt.wantGPUErr && gpuErr != nil {
				t.Fatalf("PopErrorScope returned %v, want nil", gpuErr)
			}
			if tt.wantGPUErr && gpuErr != nil {
				if gpuErr.Type != tt.errFilter {
					t.Errorf("GPUError.Type = %v, want %v", gpuErr.Type, tt.errFilter)
				}
				if gpuErr.Message != tt.wantMessage {
					t.Errorf("GPUError.Message = %q, want %q", gpuErr.Message, tt.wantMessage)
				}
			}
		})
	}
}

func TestErrorScopeNoError(t *testing.T) {
	mgr := NewErrorScopeManager()

	mgr.PushErrorScope(ErrorFilterValidation)

	// No error reported within the scope

	gpuErr, err := mgr.PopErrorScope()
	if err != nil {
		t.Fatalf("PopErrorScope returned unexpected error: %v", err)
	}
	if gpuErr != nil {
		t.Errorf("PopErrorScope returned %v, want nil (no error was reported)", gpuErr)
	}
}

func TestErrorScopeNested(t *testing.T) {
	mgr := NewErrorScopeManager()

	// Push outer scope for OOM errors
	mgr.PushErrorScope(ErrorFilterOutOfMemory)

	// Push inner scope for validation errors
	mgr.PushErrorScope(ErrorFilterValidation)

	// Report a validation error -- should be caught by the inner scope
	captured := mgr.ReportError(ErrorFilterValidation, "invalid descriptor")
	if !captured {
		t.Fatal("ReportError returned false for validation error")
	}

	// Pop inner scope -- should have the validation error
	innerErr, err := mgr.PopErrorScope()
	if err != nil {
		t.Fatalf("PopErrorScope (inner) error: %v", err)
	}
	if innerErr == nil {
		t.Fatal("inner scope should have captured the validation error")
	}
	if innerErr.Type != ErrorFilterValidation {
		t.Errorf("inner scope error type = %v, want Validation", innerErr.Type)
	}
	if innerErr.Message != "invalid descriptor" {
		t.Errorf("inner scope error message = %q, want %q", innerErr.Message, "invalid descriptor")
	}

	// Pop outer scope -- should have no error (validation error was caught by inner)
	outerErr, err := mgr.PopErrorScope()
	if err != nil {
		t.Fatalf("PopErrorScope (outer) error: %v", err)
	}
	if outerErr != nil {
		t.Errorf("outer scope should have no error, got: %v", outerErr)
	}
}

func TestErrorScopeNestedInnerCatchesFirst(t *testing.T) {
	mgr := NewErrorScopeManager()

	// Both scopes listen for validation errors
	mgr.PushErrorScope(ErrorFilterValidation) // outer
	mgr.PushErrorScope(ErrorFilterValidation) // inner

	// Report a validation error -- inner (top of stack) should catch it
	captured := mgr.ReportError(ErrorFilterValidation, "caught by inner")
	if !captured {
		t.Fatal("ReportError returned false")
	}

	// Pop inner -- should have the error
	innerErr, err := mgr.PopErrorScope()
	if err != nil {
		t.Fatalf("PopErrorScope (inner) error: %v", err)
	}
	if innerErr == nil {
		t.Fatal("inner scope should have caught the error")
	}
	if innerErr.Message != "caught by inner" {
		t.Errorf("inner error message = %q, want %q", innerErr.Message, "caught by inner")
	}

	// Pop outer -- should be clean (inner already consumed the error)
	outerErr, err := mgr.PopErrorScope()
	if err != nil {
		t.Fatalf("PopErrorScope (outer) error: %v", err)
	}
	if outerErr != nil {
		t.Errorf("outer scope should have no error, got: %v", outerErr)
	}
}

func TestErrorScopeFilterMismatch(t *testing.T) {
	mgr := NewErrorScopeManager()

	// Push scope for OOM errors only
	mgr.PushErrorScope(ErrorFilterOutOfMemory)

	// Report a validation error -- should NOT be captured by OOM scope
	captured := mgr.ReportError(ErrorFilterValidation, "validation failure")
	if captured {
		t.Fatal("ReportError should return false when no matching scope exists")
	}

	// Pop scope -- should have no error (filter didn't match)
	gpuErr, err := mgr.PopErrorScope()
	if err != nil {
		t.Fatalf("PopErrorScope error: %v", err)
	}
	if gpuErr != nil {
		t.Errorf("scope should have no error (filter mismatch), got: %v", gpuErr)
	}
}

func TestErrorScopeMultipleErrors(t *testing.T) {
	mgr := NewErrorScopeManager()

	mgr.PushErrorScope(ErrorFilterValidation)

	// Report multiple validation errors -- only the first should be captured
	captured1 := mgr.ReportError(ErrorFilterValidation, "first error")
	if !captured1 {
		t.Fatal("first ReportError should return true")
	}

	captured2 := mgr.ReportError(ErrorFilterValidation, "second error")
	if !captured2 {
		t.Fatal("second ReportError should return true (scope still matches)")
	}

	captured3 := mgr.ReportError(ErrorFilterValidation, "third error")
	if !captured3 {
		t.Fatal("third ReportError should return true (scope still matches)")
	}

	gpuErr, err := mgr.PopErrorScope()
	if err != nil {
		t.Fatalf("PopErrorScope error: %v", err)
	}
	if gpuErr == nil {
		t.Fatal("scope should have captured the first error")
	}
	if gpuErr.Message != "first error" {
		t.Errorf("GPUError.Message = %q, want %q (only first error should be captured)",
			gpuErr.Message, "first error")
	}
}

func TestErrorScopePopEmptyStack(t *testing.T) {
	mgr := NewErrorScopeManager()

	// Pop without push should return an error
	gpuErr, err := mgr.PopErrorScope()
	if err == nil {
		t.Fatal("PopErrorScope on empty stack should return an error")
	}
	if gpuErr != nil {
		t.Errorf("PopErrorScope on empty stack should return nil GPUError, got: %v", gpuErr)
	}
}

func TestErrorScopeReportNoScopes(t *testing.T) {
	mgr := NewErrorScopeManager()

	// Report error with no scopes pushed -- should be uncaptured
	captured := mgr.ReportError(ErrorFilterValidation, "uncaptured error")
	if captured {
		t.Fatal("ReportError with no scopes should return false")
	}
}

func TestErrorScopeDepth(t *testing.T) {
	mgr := NewErrorScopeManager()

	if mgr.ScopeDepth() != 0 {
		t.Errorf("ScopeDepth = %d, want 0", mgr.ScopeDepth())
	}

	mgr.PushErrorScope(ErrorFilterValidation)
	if mgr.ScopeDepth() != 1 {
		t.Errorf("ScopeDepth = %d, want 1", mgr.ScopeDepth())
	}

	mgr.PushErrorScope(ErrorFilterOutOfMemory)
	if mgr.ScopeDepth() != 2 {
		t.Errorf("ScopeDepth = %d, want 2", mgr.ScopeDepth())
	}

	_, _ = mgr.PopErrorScope()
	if mgr.ScopeDepth() != 1 {
		t.Errorf("ScopeDepth = %d, want 1", mgr.ScopeDepth())
	}

	_, _ = mgr.PopErrorScope()
	if mgr.ScopeDepth() != 0 {
		t.Errorf("ScopeDepth = %d, want 0", mgr.ScopeDepth())
	}
}

func TestErrorScopeNestedFilterPropagation(t *testing.T) {
	mgr := NewErrorScopeManager()

	// Push outer scope for validation
	mgr.PushErrorScope(ErrorFilterValidation)
	// Push inner scope for OOM
	mgr.PushErrorScope(ErrorFilterOutOfMemory)

	// Report a validation error -- inner scope (OOM) doesn't match,
	// so it should propagate to outer scope (Validation)
	captured := mgr.ReportError(ErrorFilterValidation, "found by outer")
	if !captured {
		t.Fatal("validation error should be captured by outer scope")
	}

	// Pop inner (OOM) -- should be clean
	innerErr, err := mgr.PopErrorScope()
	if err != nil {
		t.Fatalf("PopErrorScope (inner) error: %v", err)
	}
	if innerErr != nil {
		t.Errorf("inner OOM scope should have no error, got: %v", innerErr)
	}

	// Pop outer (Validation) -- should have the error
	outerErr, err := mgr.PopErrorScope()
	if err != nil {
		t.Fatalf("PopErrorScope (outer) error: %v", err)
	}
	if outerErr == nil {
		t.Fatal("outer validation scope should have caught the error")
	}
	if outerErr.Message != "found by outer" {
		t.Errorf("outer error message = %q, want %q", outerErr.Message, "found by outer")
	}
}

func TestErrorScopeConcurrentAccess(t *testing.T) {
	mgr := NewErrorScopeManager()

	const numGoroutines = 20
	var wg sync.WaitGroup

	// Push scopes from multiple goroutines
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mgr.PushErrorScope(ErrorFilterValidation)
		}()
	}
	wg.Wait()

	if mgr.ScopeDepth() != numGoroutines {
		t.Errorf("ScopeDepth = %d, want %d", mgr.ScopeDepth(), numGoroutines)
	}

	// Report errors from multiple goroutines
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mgr.ReportError(ErrorFilterValidation, "concurrent error")
		}()
	}
	wg.Wait()

	// Pop all scopes
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = mgr.PopErrorScope()
		}()
	}
	wg.Wait()

	if mgr.ScopeDepth() != 0 {
		t.Errorf("ScopeDepth after pop all = %d, want 0", mgr.ScopeDepth())
	}
}

func TestGPUErrorInterface(t *testing.T) {
	gpuErr := &GPUError{
		Type:    ErrorFilterValidation,
		Message: "test validation error",
	}

	// Test Error() method
	errStr := gpuErr.Error()
	want := "GPU Validation error: test validation error"
	if errStr != want {
		t.Errorf("GPUError.Error() = %q, want %q", errStr, want)
	}

	// Verify it implements the error interface
	var err error = gpuErr
	if err.Error() != want {
		t.Errorf("error interface Error() = %q, want %q", err.Error(), want)
	}
}

func TestErrorFilterString(t *testing.T) {
	tests := []struct {
		filter ErrorFilter
		want   string
	}{
		{ErrorFilterValidation, "Validation"},
		{ErrorFilterOutOfMemory, "OutOfMemory"},
		{ErrorFilterInternal, "Internal"},
		{ErrorFilter(99), "ErrorFilter(99)"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.filter.String()
			if got != tt.want {
				t.Errorf("ErrorFilter.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDeviceErrorScopes(t *testing.T) {
	// Create a minimal device (ID-based, no HAL)
	device := &Device{
		Label: "test device",
	}

	// Push a validation scope
	device.PushErrorScope(ErrorFilterValidation)

	// Report a validation error
	captured := device.reportError(ErrorFilterValidation, "device validation error")
	if !captured {
		t.Fatal("device.reportError should return true")
	}

	// Pop the scope
	gpuErr := device.PopErrorScope()
	if gpuErr == nil {
		t.Fatal("device.PopErrorScope should return the captured error")
	}
	if gpuErr.Message != "device validation error" {
		t.Errorf("GPUError.Message = %q, want %q", gpuErr.Message, "device validation error")
	}
}

func TestDevicePopErrorScopeEmptyPanics(t *testing.T) {
	device := &Device{
		Label: "test device",
	}

	// PopErrorScope on empty stack should panic
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("device.PopErrorScope on empty stack should panic")
		}
	}()

	device.PopErrorScope()
}

func TestDeviceErrorScopeLazyInit(t *testing.T) {
	device := &Device{
		Label: "test device",
	}

	// Error scope manager should be nil before first use
	if device.errorScopeManager != nil {
		t.Fatal("errorScopeManager should be nil before first use")
	}

	// First use triggers lazy initialization
	device.PushErrorScope(ErrorFilterValidation)

	if device.errorScopeManager == nil {
		t.Fatal("errorScopeManager should be initialized after first use")
	}

	// Clean up
	_, _ = device.errorScopes().PopErrorScope()
}
