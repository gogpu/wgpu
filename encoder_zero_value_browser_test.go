//go:build js && wasm

package wgpu

import "testing"

// The zero-value contract of the wrapped encoder types on the browser context
// (ADR-070 §6, S4): a zero-value CommandEncoder panics on Finish (the pre-move
// type panicked on the nil browser encoder), and CommandBuffer.Release is a
// no-op.
//
// The file is wasm-tagged, so it is compiled by the js/wasm vet pass and
// executed by no runner on this machine — the same repo-wide gap the browser
// pipeline zero-value test records (P2-INFRA-014).

func TestBrowserCommandEncoderZeroValueFinishPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("Finish on a zero-value CommandEncoder did not panic")
		}
	}()
	var zero CommandEncoder
	_, _ = zero.Finish()
}

func TestBrowserCommandBufferZeroValueReleaseIsNoop(t *testing.T) {
	var zero CommandBuffer
	zero.Release()
}
