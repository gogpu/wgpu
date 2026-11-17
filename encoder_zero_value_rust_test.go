//go:build rust

package wgpu

import "testing"

// The zero-value contract of the wrapped encoder types under -tags rust
// (ADR-070 §6, S4): a zero-value CommandEncoder panics on Finish (the pre-move
// type panicked on the nil rust encoder), and CommandBuffer.Release is a
// no-op.
//
// The file is rust-tagged: `go test -tags rust -run 'TestRustCommandBufferZeroValue' .`
// executes it on this machine (the browser sibling remains the P2-INFRA-014
// instance — compiled and vetted, executed by no runner).

func TestRustCommandEncoderZeroValueFinishPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("Finish on a zero-value CommandEncoder did not panic")
		}
	}()
	var zero CommandEncoder
	_, _ = zero.Finish()
}

func TestRustCommandBufferZeroValueReleaseIsNoop(t *testing.T) {
	var zero CommandBuffer
	zero.Release()
}
