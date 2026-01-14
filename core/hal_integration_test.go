// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build integration

package core_test

import (
	"testing"

	"github.com/gogpu/wgpu/core"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/types"

	// Import all backends for side-effect registration.
	// This enables real GPU adapter enumeration.
	_ "github.com/gogpu/wgpu/hal/allbackends"
)

// TestCoreHALIntegration verifies that Core properly integrates with HAL backends.
// This test requires a real GPU and is skipped in regular CI.
//
// Run with: go test -tags=integration -v ./core/...
func TestCoreHALIntegration(t *testing.T) {
	// Check if any HAL backends are registered
	backends := hal.AvailableBackends()
	t.Logf("Available HAL backends: %v", backends)

	if len(backends) == 0 {
		t.Skip("No HAL backends available - skipping integration test")
	}

	// Clear global state
	core.GetGlobal().Clear()

	// Create instance - should enumerate real adapters
	instance := core.NewInstance(&types.InstanceDescriptor{
		Backends: types.BackendsPrimary,
		Flags:    0,
	})

	if instance == nil {
		t.Fatal("NewInstance returned nil")
	}
	defer instance.Destroy()

	// Check if we're using real adapters or mock
	if instance.IsMock() {
		t.Log("Instance is using mock adapters (no GPU available)")
	} else {
		t.Log("Instance is using real HAL adapters")
	}

	// Enumerate adapters
	adapterIDs := instance.EnumerateAdapters()
	t.Logf("Found %d adapters", len(adapterIDs))

	if len(adapterIDs) == 0 {
		t.Fatal("No adapters found")
	}

	// Get adapter info for each
	hub := core.GetGlobal().Hub()
	for i, adapterID := range adapterIDs {
		adapter, err := hub.GetAdapter(adapterID)
		if err != nil {
			t.Errorf("Failed to get adapter %d: %v", i, err)
			continue
		}

		t.Logf("Adapter %d: %s (%s, %s)",
			i,
			adapter.Info.Name,
			adapter.Info.Vendor,
			adapter.Info.Backend.String(),
		)

		// Verify adapter has valid info
		if adapter.Info.Name == "" {
			t.Errorf("Adapter %d has empty name", i)
		}

		// Check if adapter has HAL integration
		if adapter.HasHAL() {
			t.Logf("  - Has HAL integration: yes")
		} else {
			t.Logf("  - Has HAL integration: no (mock)")
		}
	}
}

// TestCoreDeviceCreation tests creating a device via Core API with HAL.
// This test requires a real GPU.
func TestCoreDeviceCreation(t *testing.T) {
	backends := hal.AvailableBackends()
	if len(backends) == 0 {
		t.Skip("No HAL backends available")
	}

	// Clear and create instance
	core.GetGlobal().Clear()
	instance := core.NewInstance(nil)
	if instance.IsMock() {
		t.Skip("Mock adapters only - skipping device creation test")
	}
	defer instance.Destroy()

	// Request adapter
	adapterID, err := instance.RequestAdapter(&types.RequestAdapterOptions{
		PowerPreference: types.PowerPreferenceHighPerformance,
	})
	if err != nil {
		t.Fatalf("RequestAdapter failed: %v", err)
	}

	hub := core.GetGlobal().Hub()
	adapter, err := hub.GetAdapter(adapterID)
	if err != nil {
		t.Fatalf("GetAdapter failed: %v", err)
	}

	if !adapter.HasHAL() {
		t.Skip("Adapter has no HAL integration")
	}

	t.Logf("Selected adapter: %s (%s)", adapter.Info.Name, adapter.Info.Backend.String())

	// Open device via HAL
	halAdapter := adapter.HALAdapter()
	if halAdapter == nil {
		t.Fatal("HALAdapter() returned nil")
	}

	openDev, err := halAdapter.Open(0, adapter.Limits)
	if err != nil {
		t.Fatalf("Adapter.Open failed: %v", err)
	}
	defer openDev.Device.Destroy()

	t.Log("Device created successfully via HAL")

	// Create a buffer to verify device works
	halBuffer, err := openDev.Device.CreateBuffer(&hal.BufferDescriptor{
		Label: "Test Buffer",
		Size:  1024,
		Usage: types.BufferUsageVertex | types.BufferUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateBuffer failed: %v", err)
	}
	openDev.Device.DestroyBuffer(halBuffer)

	t.Log("Buffer created and destroyed successfully")
}

// TestCoreBufferCreationViaDevice tests creating a buffer via core.Device.
func TestCoreBufferCreationViaDevice(t *testing.T) {
	backends := hal.AvailableBackends()
	if len(backends) == 0 {
		t.Skip("No HAL backends available")
	}

	// Clear and create instance
	core.GetGlobal().Clear()
	instance := core.NewInstance(nil)
	if instance.IsMock() {
		t.Skip("Mock adapters only")
	}
	defer instance.Destroy()

	// Request adapter
	adapterID, err := instance.RequestAdapter(nil)
	if err != nil {
		t.Fatalf("RequestAdapter failed: %v", err)
	}

	hub := core.GetGlobal().Hub()
	adapter, err := hub.GetAdapter(adapterID)
	if err != nil {
		t.Fatalf("GetAdapter failed: %v", err)
	}

	if !adapter.HasHAL() {
		t.Skip("Adapter has no HAL integration")
	}

	// Open HAL device
	halAdapter := adapter.HALAdapter()
	openDev, err := halAdapter.Open(0, adapter.Limits)
	if err != nil {
		t.Fatalf("Adapter.Open failed: %v", err)
	}
	defer openDev.Device.Destroy()

	// Wrap in core.Device
	device := core.NewDevice(
		openDev.Device,
		&adapter,
		0,
		adapter.Limits,
		"Integration Test Device",
	)
	defer device.Destroy()

	if !device.HasHAL() {
		t.Fatal("Device should have HAL integration")
	}

	// Create buffer via core.Device
	buffer, err := device.CreateBuffer(&types.BufferDescriptor{
		Label:            "Core API Buffer",
		Size:             2048,
		Usage:            types.BufferUsageStorage | types.BufferUsageCopyDst,
		MappedAtCreation: false,
	})
	if err != nil {
		t.Fatalf("Device.CreateBuffer failed: %v", err)
	}

	t.Logf("Created buffer: size=%d, usage=%v", buffer.Size(), buffer.Usage())

	// Verify buffer properties
	if buffer.Size() != 2048 {
		t.Errorf("Buffer size = %d, want 2048", buffer.Size())
	}
	if !buffer.HasHAL() {
		t.Error("Buffer should have HAL integration")
	}
	if buffer.IsDestroyed() {
		t.Error("Buffer should not be destroyed")
	}

	// Destroy buffer
	buffer.Destroy()
	if !buffer.IsDestroyed() {
		t.Error("Buffer should be destroyed")
	}

	t.Log("Buffer lifecycle test passed")
}
