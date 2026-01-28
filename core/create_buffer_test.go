package core

import (
	"errors"
	"strings"
	"testing"

	"github.com/gogpu/gputypes"
)

func TestDevice_CreateBuffer_Success(t *testing.T) {
	halDevice := &mockHALDevice{}
	limits := gputypes.DefaultLimits()
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), limits, "TestDevice")

	buffer, err := device.CreateBuffer(&gputypes.BufferDescriptor{
		Label: "TestBuffer",
		Size:  1024,
		Usage: gputypes.BufferUsageVertex | gputypes.BufferUsageCopyDst,
	})

	if err != nil {
		t.Fatalf("CreateBuffer failed: %v", err)
	}
	if buffer == nil {
		t.Fatal("CreateBuffer returned nil buffer")
	}
	if buffer.Label() != "TestBuffer" {
		t.Errorf("Expected label 'TestBuffer', got '%s'", buffer.Label())
	}
	if buffer.Size() != 1024 {
		t.Errorf("Expected size 1024, got %d", buffer.Size())
	}
	if buffer.Usage() != gputypes.BufferUsageVertex|gputypes.BufferUsageCopyDst {
		t.Errorf("Unexpected usage flags")
	}
	if buffer.Device() != device {
		t.Error("Buffer should reference parent device")
	}
}

func TestDevice_CreateBuffer_ZeroSize(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	_, err := device.CreateBuffer(&gputypes.BufferDescriptor{
		Label: "ZeroBuffer",
		Size:  0,
		Usage: gputypes.BufferUsageVertex,
	})

	if err == nil {
		t.Fatal("Expected error for zero size")
	}
	var cbe *CreateBufferError
	if !errors.As(err, &cbe) {
		t.Fatalf("Expected CreateBufferError, got %T", err)
	}
	if cbe.Kind != CreateBufferErrorZeroSize {
		t.Errorf("Expected CreateBufferErrorZeroSize, got %v", cbe.Kind)
	}
}

func TestDevice_CreateBuffer_MaxSize(t *testing.T) {
	halDevice := &mockHALDevice{}
	limits := gputypes.DefaultLimits()
	limits.MaxBufferSize = 1024 // Set small max for testing
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), limits, "TestDevice")

	_, err := device.CreateBuffer(&gputypes.BufferDescriptor{
		Label: "HugeBuffer",
		Size:  2048, // Exceeds max
		Usage: gputypes.BufferUsageVertex,
	})

	if err == nil {
		t.Fatal("Expected error for exceeding max size")
	}
	var cbe *CreateBufferError
	if !errors.As(err, &cbe) {
		t.Fatalf("Expected CreateBufferError, got %T", err)
	}
	if cbe.Kind != CreateBufferErrorMaxBufferSize {
		t.Errorf("Expected CreateBufferErrorMaxBufferSize, got %v", cbe.Kind)
	}
	if cbe.RequestedSize != 2048 {
		t.Errorf("Expected RequestedSize 2048, got %d", cbe.RequestedSize)
	}
	if cbe.MaxSize != 1024 {
		t.Errorf("Expected MaxSize 1024, got %d", cbe.MaxSize)
	}
}

func TestDevice_CreateBuffer_EmptyUsage(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	_, err := device.CreateBuffer(&gputypes.BufferDescriptor{
		Label: "NoUsageBuffer",
		Size:  1024,
		Usage: 0, // Empty usage
	})

	if err == nil {
		t.Fatal("Expected error for empty usage")
	}
	var cbe *CreateBufferError
	if !errors.As(err, &cbe) {
		t.Fatalf("Expected CreateBufferError, got %T", err)
	}
	if cbe.Kind != CreateBufferErrorEmptyUsage {
		t.Errorf("Expected CreateBufferErrorEmptyUsage, got %v", cbe.Kind)
	}
}

func TestDevice_CreateBuffer_InvalidUsage(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	// Use a high bit that's not a valid usage flag
	invalidUsage := gputypes.BufferUsage(1 << 30)

	_, err := device.CreateBuffer(&gputypes.BufferDescriptor{
		Label: "InvalidBuffer",
		Size:  1024,
		Usage: invalidUsage,
	})

	if err == nil {
		t.Fatal("Expected error for invalid usage")
	}
	var cbe *CreateBufferError
	if !errors.As(err, &cbe) {
		t.Fatalf("Expected CreateBufferError, got %T", err)
	}
	if cbe.Kind != CreateBufferErrorInvalidUsage {
		t.Errorf("Expected CreateBufferErrorInvalidUsage, got %v", cbe.Kind)
	}
}

func TestDevice_CreateBuffer_MapReadWriteExclusive(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	_, err := device.CreateBuffer(&gputypes.BufferDescriptor{
		Label: "MapBuffer",
		Size:  1024,
		Usage: gputypes.BufferUsageMapRead | gputypes.BufferUsageMapWrite,
	})

	if err == nil {
		t.Fatal("Expected error for MAP_READ + MAP_WRITE")
	}
	var cbe *CreateBufferError
	if !errors.As(err, &cbe) {
		t.Fatalf("Expected CreateBufferError, got %T", err)
	}
	if cbe.Kind != CreateBufferErrorMapReadWriteExclusive {
		t.Errorf("Expected CreateBufferErrorMapReadWriteExclusive, got %v", cbe.Kind)
	}
}

func TestDevice_CreateBuffer_DeviceDestroyed(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	device.Destroy()

	_, err := device.CreateBuffer(&gputypes.BufferDescriptor{
		Label: "AfterDestroy",
		Size:  1024,
		Usage: gputypes.BufferUsageVertex,
	})

	if err == nil {
		t.Fatal("Expected error for destroyed device")
	}
	if !errors.Is(err, ErrDeviceDestroyed) {
		t.Errorf("Expected ErrDeviceDestroyed, got %v", err)
	}
}

func TestDevice_CreateBuffer_NilDescriptor(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	_, err := device.CreateBuffer(nil)

	if err == nil {
		t.Fatal("Expected error for nil descriptor")
	}
}

func TestDevice_CreateBuffer_MappedAtCreation(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	buffer, err := device.CreateBuffer(&gputypes.BufferDescriptor{
		Label:            "MappedBuffer",
		Size:             1024,
		Usage:            gputypes.BufferUsageMapWrite | gputypes.BufferUsageCopySrc,
		MappedAtCreation: true,
	})

	if err != nil {
		t.Fatalf("CreateBuffer failed: %v", err)
	}
	if buffer.MapState() != BufferMapStateMapped {
		t.Error("Buffer should be mapped at creation")
	}
	if !buffer.IsInitialized(0, 1024) {
		t.Error("Buffer should be marked as initialized when mapped at creation")
	}
}

func TestDevice_CreateBuffer_SizeAlignment(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	// Request non-aligned size
	buffer, err := device.CreateBuffer(&gputypes.BufferDescriptor{
		Label: "UnalignedBuffer",
		Size:  1023, // Not aligned to 4
		Usage: gputypes.BufferUsageVertex,
	})

	if err != nil {
		t.Fatalf("CreateBuffer failed: %v", err)
	}
	// Size should be reported as requested (1023), but HAL received aligned (1024)
	if buffer.Size() != 1023 {
		t.Errorf("Expected size 1023, got %d", buffer.Size())
	}
}

func TestDevice_CreateBuffer_ValidMapReadOnly(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	// MAP_READ alone is valid
	buffer, err := device.CreateBuffer(&gputypes.BufferDescriptor{
		Label: "MapReadBuffer",
		Size:  1024,
		Usage: gputypes.BufferUsageMapRead | gputypes.BufferUsageCopyDst,
	})

	if err != nil {
		t.Fatalf("CreateBuffer failed: %v", err)
	}
	if buffer == nil {
		t.Fatal("Buffer should not be nil")
	}
}

func TestDevice_CreateBuffer_ValidMapWriteOnly(t *testing.T) {
	halDevice := &mockHALDevice{}
	device := NewDevice(halDevice, &Adapter{}, gputypes.Features(0), gputypes.DefaultLimits(), "TestDevice")

	// MAP_WRITE alone is valid
	buffer, err := device.CreateBuffer(&gputypes.BufferDescriptor{
		Label: "MapWriteBuffer",
		Size:  1024,
		Usage: gputypes.BufferUsageMapWrite | gputypes.BufferUsageCopySrc,
	})

	if err != nil {
		t.Fatalf("CreateBuffer failed: %v", err)
	}
	if buffer == nil {
		t.Fatal("Buffer should not be nil")
	}
}

func TestCreateBufferError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *CreateBufferError
		contains string
	}{
		{
			name: "zero size",
			err: &CreateBufferError{
				Kind:  CreateBufferErrorZeroSize,
				Label: "test",
			},
			contains: "size must be greater than 0",
		},
		{
			name: "max size",
			err: &CreateBufferError{
				Kind:          CreateBufferErrorMaxBufferSize,
				Label:         "test",
				RequestedSize: 2000,
				MaxSize:       1000,
			},
			contains: "exceeds maximum",
		},
		{
			name: "empty usage",
			err: &CreateBufferError{
				Kind:  CreateBufferErrorEmptyUsage,
				Label: "test",
			},
			contains: "must not be empty",
		},
		{
			name: "invalid usage",
			err: &CreateBufferError{
				Kind:  CreateBufferErrorInvalidUsage,
				Label: "test",
			},
			contains: "invalid usage",
		},
		{
			name: "map exclusive",
			err: &CreateBufferError{
				Kind:  CreateBufferErrorMapReadWriteExclusive,
				Label: "test",
			},
			contains: "mutually exclusive",
		},
		{
			name: "hal error",
			err: &CreateBufferError{
				Kind:     CreateBufferErrorHAL,
				Label:    "test",
				HALError: errors.New("backend error"),
			},
			contains: "HAL error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := tt.err.Error()
			if msg == "" {
				t.Error("Error message should not be empty")
			}
			// Check it contains expected text (basic sanity check)
			if tt.contains != "" && !strings.Contains(msg, tt.contains) {
				t.Errorf("Expected error to contain %q, got %q", tt.contains, msg)
			}
		})
	}
}
