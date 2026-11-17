//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu

import "github.com/gogpu/wgpu/internal/resource"

// QuerySet represents a set of GPU queries (occlusion, timestamp, etc.).
//
// The implementation lives in internal/resource (ADR-070); the public methods
// are one delegation each, and Type converts the implementation's QueryType to
// the public one at the boundary. A zero value is safe: Count reports 0, Type
// reports Occlusion, and Release is a no-op, as before the move.
type QuerySet struct {
	impl *resource.QuerySet
}

// Type returns the query set type.
func (qs *QuerySet) Type() QueryType {
	if qs == nil || qs.impl == nil {
		return QueryTypeOcclusion
	}
	return QueryType(qs.impl.Type())
}

// Count returns the number of queries in the set.
func (qs *QuerySet) Count() uint32 {
	if qs == nil || qs.impl == nil {
		return 0
	}
	return qs.impl.Count()
}

// Release drops the application's reference to the query set.
func (qs *QuerySet) Release() {
	if qs == nil || qs.impl == nil {
		return
	}
	qs.impl.Release()
}
