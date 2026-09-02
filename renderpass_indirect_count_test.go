//go:build !rust && !(js && wasm)

package wgpu

import (
	"errors"
	"strings"
	"testing"

	"github.com/gogpu/gputypes"
)

func TestValidateIndirectCountBuffers(t *testing.T) {
	t.Parallel()
	pass := &RenderPassEncoder{}
	indirect := NewTestBuffer(64, gputypes.BufferUsageIndirect, "indirect")
	count := NewTestBuffer(4, gputypes.BufferUsageIndirect, "count")

	tests := []struct {
		name         string
		indirect     *Buffer
		indirectOff  uint64
		count        *Buffer
		countOff     uint64
		maxDrawCount uint32
		stride       uint64
		wantErr      error
		wantErrMsg   string // when set, expect any error containing this substring
	}{
		{
			name:         "valid draw indirect span",
			indirect:     indirect,
			indirectOff:  0,
			count:        count,
			countOff:     0,
			maxDrawCount: 2,
			stride:       drawIndirectRecordSize,
		},
		{
			name:         "valid indexed indirect span",
			indirect:     NewTestBuffer(80, gputypes.BufferUsageIndirect, "indexed-indirect"),
			indirectOff:  20,
			count:        count,
			countOff:     0,
			maxDrawCount: 3,
			stride:       drawIndexedIndirectRecordSize,
		},
		{
			name:         "nil indirect buffer",
			indirect:     nil,
			count:        count,
			maxDrawCount: 1,
			stride:       drawIndirectRecordSize,
			wantErrMsg:   "indirect buffer is nil",
		},
		{
			name:         "missing indirect usage",
			indirect:     NewTestBuffer(64, gputypes.BufferUsageCopyDst, "no-indirect"),
			count:        count,
			maxDrawCount: 1,
			stride:       drawIndirectRecordSize,
			wantErr:      ErrDrawIndirectBufferUsage,
		},
		{
			name:         "unaligned indirect offset",
			indirect:     indirect,
			indirectOff:  2,
			count:        count,
			maxDrawCount: 1,
			stride:       drawIndirectRecordSize,
			wantErr:      ErrDrawIndirectOffsetAlignment,
		},
		{
			name:         "count buffer overrun",
			indirect:     indirect,
			count:        NewTestBuffer(2, gputypes.BufferUsageIndirect, "short-count"),
			countOff:     0,
			maxDrawCount: 1,
			stride:       drawIndirectRecordSize,
			wantErr:      ErrDrawIndirectBufferOverrun,
		},
		{
			name:         "indirect span overrun",
			indirect:     NewTestBuffer(20, gputypes.BufferUsageIndirect, "short-indirect"),
			indirectOff:  0,
			count:        count,
			maxDrawCount: 2,
			stride:       drawIndirectRecordSize,
			wantErr:      ErrDrawIndirectBufferOverrun,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := pass.validateIndirectCountBuffers(
				tc.indirect, tc.indirectOff,
				tc.count, tc.countOff,
				tc.maxDrawCount, tc.stride,
			)
			if tc.wantErrMsg != "" {
				if err == nil {
					t.Fatalf("expected error containing %q", tc.wantErrMsg)
				}
				if !strings.Contains(err.Error(), tc.wantErrMsg) {
					t.Fatalf("error = %v, want substring %q", err, tc.wantErrMsg)
				}
				return
			}
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestValidateIndexedIndirectCountPreconditions(t *testing.T) {
	t.Parallel()
	pass := &RenderPassEncoder{}
	err := pass.validateIndexedIndirectCountPreconditions()
	if err == nil {
		t.Fatal("expected error without index buffer")
	}
	if !errors.Is(err, ErrDrawMissingIndexBuffer) {
		t.Fatalf("error = %v, want ErrDrawMissingIndexBuffer", err)
	}

	pass.indexBufferSet = true
	strip := gputypes.IndexFormatUint16
	pass.currentStripIndexFormat = &strip
	pass.indexBufferFormat = gputypes.IndexFormatUint32
	err = pass.validateIndexedIndirectCountPreconditions()
	if err == nil {
		t.Fatal("expected format mismatch error")
	}
	if !errors.Is(err, ErrDrawIndexFormatMismatch) {
		t.Fatalf("error = %v, want ErrDrawIndexFormatMismatch", err)
	}
}
