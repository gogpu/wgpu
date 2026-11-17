//go:build !rust && !(js && wasm)

package queue

// The zero-value contract of the queue domain (ADR-070 S9): the destination's
// methods behave on a zero value exactly as the pre-move public Queue did, and
// a nil receiver is guarded instead of panicking. The map's §8 table is the
// source of the expected values and errors.

import (
	"errors"
	"testing"

	"github.com/gogpu/wgpu/internal/core"
)

func TestZeroValueContract(t *testing.T) {
	receivers := []struct {
		name string
		q    *Queue
	}{
		{"nil receiver", nil},
		{"zero struct", &Queue{}},
	}

	tests := []struct {
		name string
		run  func(t *testing.T, q *Queue)
	}{
		{
			name: "Submit returns the queue-not-available error",
			run: func(t *testing.T, q *Queue) {
				t.Helper()
				_, err := q.Submit(nil)
				if err == nil || err.Error() != "wgpu: queue not available" {
					t.Fatalf("Submit error = %v, want \"wgpu: queue not available\"", err)
				}
			},
		},
		{
			name: "Poll returns 0",
			run: func(t *testing.T, q *Queue) {
				t.Helper()
				if got := q.Poll(); got != 0 {
					t.Fatalf("Poll = %d, want 0", got)
				}
			},
		},
		{
			name: "WriteBuffer reports the nil queue or buffer",
			run: func(t *testing.T, q *Queue) {
				t.Helper()
				err := q.WriteBuffer(nil, 0, nil)
				if err == nil || err.Error() != "wgpu: WriteBuffer: queue or buffer is nil" {
					t.Fatalf("WriteBuffer error = %v, want \"wgpu: WriteBuffer: queue or buffer is nil\"", err)
				}
			},
		},
		{
			name: "WriteTexture reports the nil queue or destination",
			run: func(t *testing.T, q *Queue) {
				t.Helper()
				err := q.WriteTexture(nil, nil, nil, nil)
				if err == nil || err.Error() != "wgpu: WriteTexture: queue or destination is nil" {
					t.Fatalf("WriteTexture error = %v, want \"wgpu: WriteTexture: queue or destination is nil\"", err)
				}
			},
		},
		{
			name: "SetSwapchainSuppressed is a no-op",
			run: func(t *testing.T, q *Queue) {
				t.Helper()
				q.SetSwapchainSuppressed(true)
				q.SetSwapchainSuppressed(false)
			},
		},
		{
			name: "LastSubmissionIndex returns 0",
			run: func(t *testing.T, q *Queue) {
				t.Helper()
				if got := q.LastSubmissionIndex(); got != 0 {
					t.Fatalf("LastSubmissionIndex = %d, want 0", got)
				}
			},
		},
		{
			name: "GetTimestampPeriod returns 1.0",
			run: func(t *testing.T, q *Queue) {
				t.Helper()
				if got := q.GetTimestampPeriod(); got != 1.0 {
					t.Fatalf("GetTimestampPeriod = %v, want 1.0", got)
				}
			},
		},
	}

	for _, r := range receivers {
		t.Run(r.name, func(t *testing.T) {
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) { tt.run(t, r.q) })
			}
		})
	}
}

// TestWriteTextureReleasedTextureIsErrReleased pins the sentinel identity of
// the released-texture path: the error must be core.ErrReleased itself, not a
// copy with the same text, so a caller's errors.Is keeps working across the
// move (ADR-070 §3.1).
func TestWriteTextureReleasedTextureIsErrReleased(t *testing.T) {
	host, device := newTestHost(t, false)
	q := New(&testHALQueue{}, nil)
	q.SetDevice(host)

	tex := newTestTrackedTexture(t, device, host, "released upload target")
	tex.Release()

	dst := &ImageCopyTexture{Texture: tex}
	err := q.WriteTexture(dst, nil, nil, nil)
	if !errors.Is(err, core.ErrReleased) {
		t.Fatalf("WriteTexture on a released texture = %v, want core.ErrReleased", err)
	}
}

// TestReadOnlyPathsDoNotAllocate keeps the lock-free and no-op accessors free
// of allocations: they sit on maintenance and callback paths (ADR-070 §15.7).
func TestReadOnlyPathsDoNotAllocate(t *testing.T) {
	host, _ := newTestHost(t, false)
	q := New(&testHALQueue{}, nil)
	q.SetDevice(host)

	if got := testing.AllocsPerRun(100, func() { q.Poll() }); got != 0 {
		t.Fatalf("Poll allocates %v times per run, want 0", got)
	}
	if got := testing.AllocsPerRun(100, func() { q.LastSubmissionIndex() }); got != 0 {
		t.Fatalf("LastSubmissionIndex allocates %v times per run, want 0", got)
	}

	var zero Queue
	if got := testing.AllocsPerRun(100, func() { zero.SetSwapchainSuppressed(true) }); got != 0 {
		t.Fatalf("SetSwapchainSuppressed on a zero value allocates %v times per run, want 0", got)
	}
}
