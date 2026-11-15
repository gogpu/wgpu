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

func TestQuerySet_ReleaseNilCore(t *testing.T) {
	t.Parallel()
	qs := &QuerySet{impl: resource.NewQuerySet(nil, NewBareDeviceForTest().seamView())}
	qs.Release()
	if qs.impl.LiveCore() != nil {
		t.Fatal("Release should mark query set released")
	}
}
