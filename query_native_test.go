//go:build !rust && !(js && wasm)

package wgpu

import "testing"

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
	qs := &QuerySet{device: NewBareDeviceForTest()}
	qs.Release()
	if !qs.released {
		t.Fatal("Release should mark query set released")
	}
}
