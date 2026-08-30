// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package pipelinecache

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

const cacheRoot = "gogpu"

// UserCachePath returns an adapter-scoped cache file path under os.UserCacheDir().
// Example: ~/.cache/gogpu/vulkan/<adapterKey>/pipeline.cache
func UserCachePath(backend, adapterKey, fileName string) (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("pipelinecache: UserCacheDir: %w", err)
	}
	return filepath.Join(dir, cacheRoot, backend, adapterKey, fileName), nil
}

// LoadBlob reads a cache blob from disk. A missing file returns (nil, nil).
func LoadBlob(path string) ([]byte, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is constructed internally from UserCacheDir
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("pipelinecache: read %s: %w", path, err)
	}
	return data, nil
}

// SaveBlob atomically writes a cache blob to disk (write temp + rename).
func SaveBlob(path string, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("pipelinecache: mkdir %s: %w", dir, err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("pipelinecache: write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("pipelinecache: rename %s: %w", path, err)
	}
	return nil
}

// DeleteBlob removes a cache blob. Missing files are ignored.
func DeleteBlob(path string) error {
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// HexKey returns a hex-encoded SHA-256 digest of the given byte slices.
func HexKey(parts ...[]byte) string {
	h := sha256.New()
	for _, part := range parts {
		if len(part) > 0 {
			h.Write(part)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

// VulkanAdapterKey identifies a Vulkan physical device for cache scoping.
func VulkanAdapterKey(vendorID, deviceID, driverVersion uint32, uuid [16]byte) string {
	var buf [28]byte
	binary.LittleEndian.PutUint32(buf[0:4], vendorID)
	binary.LittleEndian.PutUint32(buf[4:8], deviceID)
	binary.LittleEndian.PutUint32(buf[8:12], driverVersion)
	copy(buf[12:28], uuid[:])
	return HexKey(buf[:])
}

// DX12AdapterKey identifies a DXGI adapter for cache scoping.
func DX12AdapterKey(luidLow uint32, luidHigh int32, vendorID, deviceID, revision uint32) string {
	var buf [20]byte
	binary.LittleEndian.PutUint32(buf[0:4], luidLow)
	binary.LittleEndian.PutUint32(buf[4:8], uint32(luidHigh)) //nolint:gosec // LUID high half stored as raw bits
	binary.LittleEndian.PutUint32(buf[8:12], vendorID)
	binary.LittleEndian.PutUint32(buf[12:16], deviceID)
	binary.LittleEndian.PutUint32(buf[16:20], revision)
	return HexKey(buf[:])
}
