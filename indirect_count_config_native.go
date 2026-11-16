//go:build !rust && !(js && wasm)

package wgpu

// indirectCountDrawConfig is the per-call configuration of
// RenderPassEncoder.executeIndirectCountDraw: the two indirect buffers, the
// feature-ledger names, the record stride and the callbacks that validate and
// record. The declaration lived in renderpass_indirect_count.go until the S5
// Prepare (2026-11-16) and stays here because renderpass_indirect_count_test.go
// names it: the ADR-070 planner's `unexported-boundary` class refuses a plan
// whose staying tree names an unexported declaration that moves. The moving
// file and the test reach it through handRewrite-deferred references.
//
// Move worklist (S5): the type relocates into internal/encoder with the
// render pass; the white-box test renderpass_indirect_count_test.go follows it
// and is ported onto the destination's test doubles (the S4 pattern).
type indirectCountDrawConfig struct {
	validateDrawOp  string
	featureResource string
	indirectBuffer  *Buffer
	indirectOffset  uint64
	countBuffer     *Buffer
	countOffset     uint64
	maxDrawCount    uint32
	recordStride    uint64
	preValidate     func() error
	record          func()
}
