//go:build !(js && wasm)

package hal

import (
	"testing"

	publichal "github.com/gogpu/wgpu/hal"
)

// TestPublicKindTable pins every internal backend identity to its public kind
// and checks that the table is exhaustive: a new BackendID without a row would
// otherwise reach a consumer as KindUnknown.
func TestPublicKindTable(t *testing.T) {
	tests := []struct {
		id   BackendID
		want publichal.BackendKind
	}{
		{BackendIDUnknown, publichal.KindUnknown},
		{BackendIDVulkan, publichal.KindVulkan},
		{BackendIDMetal, publichal.KindMetal},
		{BackendIDDX12, publichal.KindDX12},
		{BackendIDGLES, publichal.KindGLES},
		{BackendIDBrowser, publichal.KindBrowser},
		{BackendIDSoftware, publichal.KindSoftware},
		{BackendIDNoop, publichal.KindNoop},
		{BackendIDWebGPU, publichal.KindWebGPU},
	}
	if len(backendKinds) != len(tests) {
		t.Fatalf("backendKinds has %d rows, want %d (one per BackendID)", len(backendKinds), len(tests))
	}
	for _, test := range tests {
		if got := PublicKind(test.id); got != test.want {
			t.Errorf("PublicKind(%d) = %d, want %d", test.id, got, test.want)
		}
	}
	if got := PublicKind(BackendID(len(backendKinds))); got != publichal.KindUnknown {
		t.Errorf("PublicKind(out of range) = %d, want KindUnknown", got)
	}
}
