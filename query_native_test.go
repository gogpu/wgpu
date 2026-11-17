//go:build !rust && !(js && wasm)

package wgpu

import (
	"testing"

	"github.com/gogpu/wgpu/internal/resource"
)

func TestQuerySet_NilSafeAccessors(t *testing.T) {
	t.Parallel()
	var qs *QuerySet
	if qs.Type() != QueryTypeOcclusion {
		t.Fatalf("nil Type() = %v, want Occlusion", qs.Type())
	}
	if qs.Count() != 0 {
		t.Fatalf("nil Count() = %d, want 0", qs.Count())
	}
	qs.Release()
}

// TestQuerySet_ReleaseNilCore pins the released flag of a query set that has
// no core: LiveCore cannot show the release here (it is nil either way), so
// the assertion reads the flag Release sets, as the pre-move test read the
// released field directly.
func TestQuerySet_ReleaseNilCore(t *testing.T) {
	t.Parallel()
	qs := &QuerySet{impl: resource.NewQuerySet(nil, testDeviceView(NewBareDeviceForTest()))}
	if qs.impl.IsReleased() {
		t.Fatal("a fresh query set must not be released")
	}
	qs.Release()
	if !qs.impl.IsReleased() {
		t.Fatal("Release should mark query set released")
	}
}
