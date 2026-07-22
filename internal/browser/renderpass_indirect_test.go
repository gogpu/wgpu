//go:build js && wasm

package browser

import (
	"syscall/js"
	"testing"
)

func TestDrawIndexedIndirectOverflowDelegatesOneValidationCall(t *testing.T) {
	var offsets []float64
	draw := js.FuncOf(func(_ js.Value, args []js.Value) any {
		offsets = append(offsets, args[1].Float())
		return nil
	})
	defer draw.Release()

	pass := &RenderPassEncoder{fnDrawIndexedIndirect: draw.Value}
	pass.DrawIndexedIndirect(js.Undefined(), ^uint64(0)-3)

	if len(offsets) != 1 {
		t.Fatalf("drawIndexedIndirect calls = %d, want one delegated validation call", len(offsets))
	}
}

func TestDrawIndexedIndirectCountOnePreservesOffset(t *testing.T) {
	var offsets []float64
	draw := js.FuncOf(func(_ js.Value, args []js.Value) any {
		offsets = append(offsets, args[1].Float())
		return nil
	})
	defer draw.Release()

	pass := &RenderPassEncoder{fnDrawIndexedIndirect: draw.Value}
	pass.DrawIndexedIndirect(js.Undefined(), 8)

	if len(offsets) != 1 || offsets[0] != 8 {
		t.Fatalf("drawIndexedIndirect offsets = %v, want [8]", offsets)
	}
}

func TestDrawIndexedIndirectSingleRecordCallsOnce(t *testing.T) {
	var calls int
	draw := js.FuncOf(func(_ js.Value, _ []js.Value) any {
		calls++
		return nil
	})
	defer draw.Release()

	pass := &RenderPassEncoder{fnDrawIndexedIndirect: draw.Value}
	pass.DrawIndexedIndirect(js.Undefined(), 8)

	if calls != 1 {
		t.Fatalf("drawIndexedIndirect calls = %d, want one", calls)
	}
}
