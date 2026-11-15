//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vk

import "testing"

func TestPromotedAndExtnumberEnumValues(t *testing.T) {
	tests := []struct {
		name string
		got  int64
		want int64
	}{
		{"StructureTypeFormatProperties2", int64(StructureTypeFormatProperties2), 1000059002},
		{"StructureTypeExternalMemoryImageCreateInfo", int64(StructureTypeExternalMemoryImageCreateInfo), 1000072001},
		{"StructureTypeExportMemoryAllocateInfo", int64(StructureTypeExportMemoryAllocateInfo), 1000072002},
		{"StructureTypeMemoryDedicatedAllocateInfo", int64(StructureTypeMemoryDedicatedAllocateInfo), 1000127001},
		{"StructureTypeDeviceGroupPresentCapabilitiesKhr", int64(StructureTypeDeviceGroupPresentCapabilitiesKhr), 1000060007},
		{"IndexTypeNoneKhr", int64(IndexTypeNoneKhr), 1000165000},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %d, want %d", tt.name, tt.got, tt.want)
		}
	}
}
