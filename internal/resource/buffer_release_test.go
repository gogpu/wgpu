//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package resource

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core"
)

// TestBufferReleaseTracksReleasedFlag is the white-box twin of the root test
// TestBufferReleaseTrackesReleasedFlag (resource_test.go), which still runs there. The released flag is implementation state, so the
// assertion lives with the implementation once the type moves; the root keeps
// the public-path twin (Release twice, mapping refused after release).
func TestBufferReleaseTracksReleasedFlag(t *testing.T) {
	coreBuf := core.NewBuffer(nil, nil, gputypes.BufferUsageVertex, 64, "release-tracked")
	coreBuf.Ref = core.NewResourceRef("Buffer:release-tracked", nil)
	buf := NewBufferFromCore(coreBuf)

	if buf.TestReleased() {
		t.Error("buffer should not be released before Release()")
	}

	buf.Release()
	if !buf.TestReleased() {
		t.Error("buffer should be released after Release()")
	}

	// Double release must not change state or panic.
	buf.Release()
	if !buf.TestReleased() {
		t.Error("buffer should remain released after double Release()")
	}
}

// TestBufferReleaseWithoutCore covers the guard path a wrong or partial
// construction takes: no core state, nothing to drop, no panic.
func TestBufferReleaseWithoutCore(t *testing.T) {
	buf := NewBufferFromCore(nil)
	buf.Release()
	if !buf.TestReleased() {
		t.Error("buffer should be released after Release() with no core state")
	}
}
