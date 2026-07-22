//go:build darwin && !(js && wasm)

package metal

import "testing"

func TestIndexedIndirectRecordOffset(t *testing.T) {
	if got, ok := indexedIndirectRecordOffset(4, 3); !ok || got != 64 {
		t.Fatalf("indexedIndirectRecordOffset(4, 3) = %d, %t; want 64, true", got, ok)
	}
	if _, ok := indexedIndirectRecordOffset(^uint64(0)-3, 1); ok {
		t.Fatal("indexedIndirectRecordOffset should reject uint64 overflow")
	}
}
