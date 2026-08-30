// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package pipelinecache

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestUserCachePath(t *testing.T) {
	path, err := UserCachePath("vulkan", "abc123", "pipeline.cache")
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(path) {
		t.Fatalf("expected absolute path, got %q", path)
	}
	if filepath.Base(path) != "pipeline.cache" {
		t.Fatalf("unexpected file name: %q", path)
	}
}

func TestSaveLoadDeleteBlob(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "cache.bin")

	data, err := LoadBlob(path)
	if err != nil {
		t.Fatal(err)
	}
	if data != nil {
		t.Fatalf("expected nil for missing file, got %d bytes", len(data))
	}

	payload := []byte("driver-isa-cache")
	if err := SaveBlob(path, payload); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadBlob(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(loaded, payload) {
		t.Fatalf("round-trip mismatch: %q vs %q", loaded, payload)
	}

	if err := DeleteBlob(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected file removed, stat err=%v", err)
	}
}

func TestSaveBlobEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.bin")
	if err := SaveBlob(path, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("empty save should be a no-op")
	}
}

func TestAdapterKeysDeterministic(t *testing.T) {
	uuid := [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	k1 := VulkanAdapterKey(0x10DE, 0x2204, 999, uuid)
	k2 := VulkanAdapterKey(0x10DE, 0x2204, 999, uuid)
	if k1 != k2 {
		t.Fatalf("VulkanAdapterKey not deterministic: %q vs %q", k1, k2)
	}

	d1 := DX12AdapterKey(1, 2, 0x1002, 0x73FF, 0xC1)
	d2 := DX12AdapterKey(1, 2, 0x1002, 0x73FF, 0xC1)
	if d1 != d2 {
		t.Fatalf("DX12AdapterKey not deterministic: %q vs %q", d1, d2)
	}
}
