//go:build !rust && !(js && wasm)

package encoder

import (
	"errors"
	"strings"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/indirect"
	"github.com/gogpu/wgpu/internal/resource"
)

// The indirect-count white-box tests moved here from the root package with
// the pass (S5's Move): they drive the pass's unexported validation, which no
// root file can name. The fixtures are local — a core device over the test HAL
// double and buffers built from it. No root test hook and no
// internal/hal/noop import: a test import is a graph edge like any other
// (layoutcheck golden-subset).

func TestValidateIndirectCountBuffers(t *testing.T) {
	t.Parallel()
	device, view := newTestCoreDevice(t)
	pass := &RenderPassEncoder{}
	indirectBuffer := newTestResourceBuffer(t, device, view, 64, gputypes.BufferUsageIndirect, "indirect")
	count := newTestResourceBuffer(t, device, view, 4, gputypes.BufferUsageIndirect, "count")
	indexedIndirect := newTestResourceBuffer(t, device, view, 80, gputypes.BufferUsageIndirect, "indexed-indirect")
	noIndirectUsage := newTestResourceBuffer(t, device, view, 64, gputypes.BufferUsageCopyDst, "no-indirect")
	shortCount := newTestResourceBuffer(t, device, view, 2, gputypes.BufferUsageIndirect, "short-count")
	noIndirectCountUsage := newTestResourceBuffer(t, device, view, 4, gputypes.BufferUsageCopyDst, "no-indirect-count")
	shortIndirect := newTestResourceBuffer(t, device, view, 20, gputypes.BufferUsageIndirect, "short-indirect")

	tests := []struct {
		name         string
		indirect     *resource.Buffer
		indirectOff  uint64
		count        *resource.Buffer
		countOff     uint64
		maxDrawCount uint32
		stride       uint64
		wantErr      error
		wantErrMsg   string // when set, expect any error containing this substring
	}{
		{
			name:         "valid draw indirect span",
			indirect:     indirectBuffer,
			indirectOff:  0,
			count:        count,
			countOff:     0,
			maxDrawCount: 2,
			stride:       indirect.DrawIndirectRecordSize,
		},
		{
			name:         "valid indexed indirect span",
			indirect:     indexedIndirect,
			indirectOff:  20,
			count:        count,
			countOff:     0,
			maxDrawCount: 3,
			stride:       indirect.DrawIndexedIndirectRecordSize,
		},
		{
			name:         "nil indirect buffer",
			indirect:     nil,
			count:        count,
			maxDrawCount: 1,
			stride:       indirect.DrawIndirectRecordSize,
			wantErrMsg:   "indirect buffer is nil",
		},
		{
			name:         "missing indirect usage",
			indirect:     noIndirectUsage,
			count:        count,
			maxDrawCount: 1,
			stride:       indirect.DrawIndirectRecordSize,
			wantErr:      core.ErrDrawIndirectBufferUsage,
		},
		{
			name:         "unaligned indirect offset",
			indirect:     indirectBuffer,
			indirectOff:  2,
			count:        count,
			maxDrawCount: 1,
			stride:       indirect.DrawIndirectRecordSize,
			wantErr:      core.ErrDrawIndirectOffsetAlignment,
		},
		{
			name:         "count buffer overrun",
			indirect:     indirectBuffer,
			count:        shortCount,
			countOff:     0,
			maxDrawCount: 1,
			stride:       indirect.DrawIndirectRecordSize,
			wantErr:      core.ErrDrawIndirectBufferOverrun,
		},
		{
			name:         "nil count buffer",
			indirect:     indirectBuffer,
			count:        nil,
			maxDrawCount: 1,
			stride:       indirect.DrawIndirectRecordSize,
			wantErrMsg:   "count buffer is nil",
		},
		{
			name:         "missing count usage",
			indirect:     indirectBuffer,
			count:        noIndirectCountUsage,
			maxDrawCount: 1,
			stride:       indirect.DrawIndirectRecordSize,
			wantErr:      core.ErrDrawIndirectBufferUsage,
		},
		{
			name:         "unaligned count offset",
			indirect:     indirectBuffer,
			count:        count,
			countOff:     1,
			maxDrawCount: 1,
			stride:       indirect.DrawIndirectRecordSize,
			wantErr:      core.ErrDrawIndirectOffsetAlignment,
		},
		{
			name:         "indirect span overrun",
			indirect:     shortIndirect,
			indirectOff:  0,
			count:        count,
			maxDrawCount: 2,
			stride:       indirect.DrawIndirectRecordSize,
			wantErr:      core.ErrDrawIndirectBufferOverrun,
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
	if !errors.Is(err, core.ErrDrawMissingIndexBuffer) {
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
	if !errors.Is(err, core.ErrDrawIndexFormatMismatch) {
		t.Fatalf("error = %v, want ErrDrawIndexFormatMismatch", err)
	}
}

func TestExecuteIndirectCountDraw_FeatureMissing(t *testing.T) {
	t.Parallel()
	device, view := newTestCoreDevice(t)
	pass, _ := newTestIndirectPass(0)
	indirectBuffer := newTestResourceBuffer(t, device, view, 64, gputypes.BufferUsageIndirect, "indirect")
	count := newTestResourceBuffer(t, device, view, 4, gputypes.BufferUsageIndirect, "count")

	pass.executeIndirectCountDraw(indirectCountDrawConfig{
		validateDrawOp:  "DrawIndirect",
		featureResource: "MultiDrawIndirectCount",
		indirectBuffer:  indirectBuffer,
		countBuffer:     count,
		maxDrawCount:    1,
		recordStride:    indirect.DrawIndirectRecordSize,
		record:          func() { t.Fatal("record should not run without feature") },
	})
}

func TestExecuteIndirectCountDraw_RecordsAndTracks(t *testing.T) {
	t.Parallel()
	device, view := newTestCoreDevice(t)
	pass, enc := newTestIndirectPass(gputypes.Features(gputypes.FeatureMultiDrawIndirectCount))
	indirectBuffer := newTestResourceBuffer(t, device, view, 64, gputypes.BufferUsageIndirect, "indirect")
	count := newTestResourceBuffer(t, device, view, 4, gputypes.BufferUsageIndirect, "count")

	recorded := false
	pass.executeIndirectCountDraw(indirectCountDrawConfig{
		validateDrawOp:  "DrawIndirect",
		featureResource: "MultiDrawIndirectCount",
		indirectBuffer:  indirectBuffer,
		countBuffer:     count,
		maxDrawCount:    1,
		recordStride:    indirect.DrawIndirectRecordSize,
		record:          func() { recorded = true },
	})
	if !recorded {
		t.Fatal("record callback not invoked")
	}
	if len(enc.TrackedRefs()) == 0 {
		t.Fatal("expected tracked buffer refs")
	}
	if len(enc.UsedBuffers()) == 0 {
		t.Fatal("expected usedBuffers tracking")
	}
}

func TestExecuteIndirectCountDraw_ZeroCountNoOp(t *testing.T) {
	t.Parallel()
	pass, _ := newTestIndirectPass(gputypes.Features(gputypes.FeatureMultiDrawIndirectCount))
	recorded := false
	pass.executeIndirectCountDraw(indirectCountDrawConfig{
		maxDrawCount: 0,
		record:       func() { recorded = true },
	})
	if recorded {
		t.Fatal("zero maxDrawCount should be a no-op")
	}
}

func TestExecuteIndirectCountDraw_IndexedPreValidateFails(t *testing.T) {
	t.Parallel()
	device, view := newTestCoreDevice(t)
	pass, _ := newTestIndirectPass(gputypes.Features(gputypes.FeatureMultiDrawIndirectCount))
	indirectBuffer := newTestResourceBuffer(t, device, view, 80, gputypes.BufferUsageIndirect, "indirect")
	count := newTestResourceBuffer(t, device, view, 4, gputypes.BufferUsageIndirect, "count")
	recorded := false
	pass.executeIndirectCountDraw(indirectCountDrawConfig{
		validateDrawOp:  "DrawIndexedIndirect",
		featureResource: "MultiDrawIndexedIndirectCount",
		indirectBuffer:  indirectBuffer,
		countBuffer:     count,
		maxDrawCount:    1,
		recordStride:    indirect.DrawIndexedIndirectRecordSize,
		preValidate:     pass.validateIndexedIndirectCountPreconditions,
		record:          func() { recorded = true },
	})
	if recorded {
		t.Fatal("preValidate should block record without index buffer")
	}
}

func TestExecuteIndirectCountDraw_IndexedSuccess(t *testing.T) {
	t.Parallel()
	device, view := newTestCoreDevice(t)
	pass, _ := newTestIndirectPass(gputypes.Features(gputypes.FeatureMultiDrawIndirectCount))
	pass.indexBufferSet = true
	pass.indexBufferFormat = gputypes.IndexFormatUint32
	indirectBuffer := newTestResourceBuffer(t, device, view, 80, gputypes.BufferUsageIndirect, "indirect")
	count := newTestResourceBuffer(t, device, view, 4, gputypes.BufferUsageIndirect, "count")
	recorded := false
	pass.executeIndirectCountDraw(indirectCountDrawConfig{
		validateDrawOp:  "DrawIndexedIndirect",
		featureResource: "MultiDrawIndexedIndirectCount",
		indirectBuffer:  indirectBuffer,
		countBuffer:     count,
		maxDrawCount:    1,
		recordStride:    indirect.DrawIndexedIndirectRecordSize,
		preValidate:     pass.validateIndexedIndirectCountPreconditions,
		record:          func() { recorded = true },
	})
	if !recorded {
		t.Fatal("indexed indirect count draw should record when preconditions pass")
	}
}

func TestMultiDrawIndirectCount_PublicAPI(t *testing.T) {
	t.Parallel()
	device, view := newTestCoreDevice(t)
	pass, _ := newTestIndirectPass(gputypes.Features(gputypes.FeatureMultiDrawIndirectCount))
	pass.core = &core.CoreRenderPassEncoder{}
	indirectBuffer := newTestResourceBuffer(t, device, view, 64, gputypes.BufferUsageIndirect, "indirect")
	count := newTestResourceBuffer(t, device, view, 4, gputypes.BufferUsageIndirect, "count")
	pass.MultiDrawIndirectCount(indirectBuffer, 0, count, 0, 1)
}

func TestMultiDrawIndexedIndirectCount_PublicAPI(t *testing.T) {
	t.Parallel()
	device, view := newTestCoreDevice(t)
	pass, _ := newTestIndirectPass(gputypes.Features(gputypes.FeatureMultiDrawIndirectCount))
	pass.core = &core.CoreRenderPassEncoder{}
	pass.indexBufferSet = true
	pass.indexBufferFormat = gputypes.IndexFormatUint32
	indirectBuffer := newTestResourceBuffer(t, device, view, 80, gputypes.BufferUsageIndirect, "indirect")
	count := newTestResourceBuffer(t, device, view, 4, gputypes.BufferUsageIndirect, "count")
	pass.MultiDrawIndexedIndirectCount(indirectBuffer, 0, count, 0, 1)
}
