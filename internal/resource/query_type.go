//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package resource

// QueryType specifies the type of queries in a query set (WebGPU
// GPUQueryType).
//
// It mirrors the public root QueryType, which cannot be aliased to an internal
// type from a public API (ADR-070 §15.1); the frontend wrapper converts at the
// boundary, as it does for MapMode and MapState. The values are the public
// type's and wgpu-types': Occlusion is 0, Timestamp 1.
type QueryType uint32

const (
	// QueryTypeOcclusion counts the number of samples that pass depth/stencil
	// tests in the drawn geometry.
	QueryTypeOcclusion QueryType = iota

	// QueryTypeTimestamp writes GPU timestamps for profiling.
	QueryTypeTimestamp
)
