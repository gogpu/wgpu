// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

package dx12

import (
	"crypto/sha256"
	"sync"

	"github.com/gogpu/naga/ir"
	"github.com/gogpu/wgpu/hal"
)

// maxShaderCacheEntries is the maximum number of entries before LRU eviction.
// Matches Rust wgpu ShaderCache threshold (wgpu-hal/src/dx12/device.rs:423).
const maxShaderCacheEntries = 200

// shaderCacheRetainWindow is the number of recent compilations to retain
// during eviction. Entries with lastUsed >= (compiled - retainWindow) survive.
// Matches Rust wgpu eviction policy (retain last 100).
const shaderCacheRetainWindow = 100

// ShaderCacheKey identifies a unique compiled shader.
// Uses SHA-256 hash of HLSL source instead of the full string to reduce memory.
// Two pipelines producing identical HLSL for the same entry point share the cache entry.
type ShaderCacheKey struct {
	sourceHash [32]byte // SHA-256 of generated HLSL source
	entryPoint string   // HLSL entry point name (naga may rename)
	stage      ir.ShaderStage
	target     string // FXC target profile: "vs_5_1", "ps_5_1", "cs_5_1"
}

// shaderCacheValue holds cached DXBC bytecode with LRU tracking.
type shaderCacheValue struct {
	lastUsed uint32 // Value of ShaderCache.compiled when last accessed
	bytecode []byte // Compiled DXBC bytecode from FXC
}

// ShaderCache caches compiled DXBC bytecode keyed by HLSL source hash,
// entry point, stage, and target profile. This avoids redundant FXC calls
// when multiple pipelines share the same shader code.
//
// Architecture matches Rust wgpu ShaderCache (wgpu-hal/src/dx12/mod.rs:1136):
// - HashMap keyed by source + entry_point + stage + shader_model
// - LRU eviction when entries exceed 200, retaining last 100
// - Per-entry-point granularity (not per-module)
type ShaderCache struct {
	mu       sync.Mutex
	entries  map[ShaderCacheKey]*shaderCacheValue
	compiled uint32 // Monotonic counter of total compilations (for LRU)
}

// NewShaderCacheKey creates a cache key from HLSL source and compilation parameters.
func NewShaderCacheKey(hlslSource string, entryPoint string, stage ir.ShaderStage, target string) ShaderCacheKey {
	return ShaderCacheKey{
		sourceHash: sha256.Sum256([]byte(hlslSource)),
		entryPoint: entryPoint,
		stage:      stage,
		target:     target,
	}
}

// Get looks up cached DXBC bytecode for the given key.
// On hit, updates the LRU counter and returns a copy of the bytecode.
// Returns nil, false on cache miss.
func (c *ShaderCache) Get(key ShaderCacheKey) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.entries == nil {
		return nil, false
	}

	entry, ok := c.entries[key]
	if !ok {
		hal.Logger().Debug("dx12: shader cache miss",
			"entryPoint", key.entryPoint,
			"stage", key.stage,
			"target", key.target,
		)
		return nil, false
	}

	// Update LRU counter — same as Rust: value.last_used = nr_of_shaders_compiled
	entry.lastUsed = c.compiled

	hal.Logger().Debug("dx12: shader cache hit",
		"entryPoint", key.entryPoint,
		"stage", key.stage,
		"target", key.target,
		"cacheSize", len(c.entries),
	)

	// Return a copy to prevent callers from mutating cached data.
	result := make([]byte, len(entry.bytecode))
	copy(result, entry.bytecode)
	return result, true
}

// Put stores compiled DXBC bytecode in the cache.
// Increments the compilation counter and performs LRU eviction if the cache
// exceeds maxShaderCacheEntries. Eviction retains entries used within the
// last shaderCacheRetainWindow compilations.
func (c *ShaderCache) Put(key ShaderCacheKey, bytecode []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.entries == nil {
		c.entries = make(map[ShaderCacheKey]*shaderCacheValue)
	}

	c.compiled++

	// Store a copy to prevent external mutation of cached data.
	stored := make([]byte, len(bytecode))
	copy(stored, bytecode)

	c.entries[key] = &shaderCacheValue{
		lastUsed: c.compiled,
		bytecode: stored,
	}

	// LRU eviction: matches Rust wgpu (device.rs:422-427).
	// When entries exceed threshold, retain only those used recently.
	if len(c.entries) > maxShaderCacheEntries {
		cutoff := c.compiled - shaderCacheRetainWindow
		for k, v := range c.entries {
			if v.lastUsed < cutoff {
				delete(c.entries, k)
			}
		}

		hal.Logger().Debug("dx12: shader cache eviction",
			"remaining", len(c.entries),
			"compiled", c.compiled,
		)
	}
}

// Len returns the number of entries in the cache. Safe for concurrent use.
func (c *ShaderCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return len(c.entries)
}

// Compiled returns the total number of shader compilations tracked.
// Safe for concurrent use.
func (c *ShaderCache) Compiled() uint32 {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.compiled
}
