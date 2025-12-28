// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

// Command dx12-test is an integration test for the DX12 backend.
package main

import (
	"fmt"
	"os"

	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/dx12"
)

func main() {
	if err := run(); err != nil {
		fmt.Printf("FAILED: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("SUCCESS: DX12 backend works!")
}

func run() error {
	fmt.Println("=== DX12 Backend Integration Test ===")
	fmt.Println()

	// Step 1: Create backend
	fmt.Print("1. Creating DX12 backend... ")
	backend := dx12.Backend{}
	fmt.Println("OK")

	// Step 2: Create instance
	fmt.Print("2. Creating DX12 instance... ")
	instance, err := backend.CreateInstance(&hal.InstanceDescriptor{})
	if err != nil {
		return fmt.Errorf("create instance: %w", err)
	}
	defer instance.Destroy()
	fmt.Println("OK")

	// Step 3: Enumerate adapters
	fmt.Print("3. Enumerating adapters... ")
	adapters := instance.EnumerateAdapters(nil)
	if len(adapters) == 0 {
		return fmt.Errorf("no adapters found")
	}
	fmt.Printf("OK (found %d)\n", len(adapters))

	// Print adapter info
	for i := range adapters {
		exposed := &adapters[i]
		fmt.Printf("   - Adapter %d: %s (%s)\n",
			i, exposed.Info.Name, exposed.Info.DriverInfo)
	}

	// Step 4: Open device
	fmt.Print("4. Opening device... ")
	openDev, err := adapters[0].Adapter.Open(0, adapters[0].Capabilities.Limits)
	if err != nil {
		return fmt.Errorf("open device: %w", err)
	}
	device := openDev.Device
	defer device.Destroy()
	fmt.Println("OK")

	// Step 5: Create empty pipeline layout directly after device
	fmt.Print("5. Creating empty pipeline layout... ")
	pipelineLayout, err := device.CreatePipelineLayout(&hal.PipelineLayoutDescriptor{
		Label:            "Empty Pipeline Layout",
		BindGroupLayouts: nil,
	})
	if err != nil {
		return fmt.Errorf("create pipeline layout: %w", err)
	}
	device.DestroyPipelineLayout(pipelineLayout)
	fmt.Println("OK")

	fmt.Println()
	fmt.Println("=== DX12 Backend Test PASSED ===")

	return nil
}
