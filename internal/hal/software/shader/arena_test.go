//go:build !(js && wasm)

package shader

import (
	"runtime"
	"testing"
)

// TestInvocationArenaDistinctBackings pins the invariant the composite
// scratch rests on: within one invocation every composite backing, SubPointer
// and SampledImageValue the arena hands out is distinct, so building a later
// one never overwrites a value still reachable from an earlier one. Only a new
// invocation (beginInvocation, i.e. a new entry-point execution) reuses the
// buffers, at which point the previous invocation has ended.
func TestInvocationArenaDistinctBackings(t *testing.T) {
	interp := &interpreter{}
	interp.beginInvocation()

	// Two composite backings in one invocation must not alias.
	a := interp.allocArray(2)
	a[0], a[1] = ValFloat(1), ValFloat(2)
	va := ValArray(a)
	b := interp.allocArray(2)
	b[0], b[1] = ValFloat(3), ValFloat(4)
	if &a[0] == &b[0] {
		t.Fatal("two composite backings share memory within one invocation")
	}
	if got := va.AsArray(); len(got) != 2 || got[0].AsFloat32() != 1 {
		t.Fatalf("first composite changed after the second was built: %v", got)
	}

	// Two SubPointers in one invocation must not alias, and extending a path
	// into a new one must not disturb the one it extends.
	parent := &Pointer{Val: ValUint(0)}
	sp1 := interp.allocSubPointer(parent, 1)
	sp1.Indexes[0] = 7
	sp2 := interp.allocSubPointer(parent, 2)
	if sp1 == sp2 {
		t.Fatal("two SubPointers are the same object within one invocation")
	}
	sp2.Indexes[0], sp2.Indexes[1] = 8, 9
	if sp1.Indexes[0] != 7 {
		t.Fatal("second SubPointer overwrote the first one's index path")
	}

	// Two sampled images in one invocation must not alias.
	si1 := interp.allocSampledImage(ValUint(1), ValUint(2))
	si2 := interp.allocSampledImage(ValUint(3), ValUint(4))
	if si1.Ref == si2.Ref {
		t.Fatal("two sampled images share a SampledImageValue within one invocation")
	}
	if si1.AsSampledImage().Image.AsUint32() != 1 {
		t.Fatal("second sampled image overwrote the first one's image")
	}

	// A new invocation reuses the buffers: the reset is what makes the arena
	// bounded, and it is safe only because the previous invocation ended.
	interp.beginInvocation()
	c := interp.allocArray(2)
	if &c[0] != &a[0] {
		t.Fatal("a new invocation did not reuse the arena buffers")
	}
}

// TestArenaLetsTemporariesAllocateNothing pins the point of the arena: with
// room in it, a composite backing, a SubPointer and a SampledImageValue are
// taken from the buffers without allocating. The base code allocated on every
// one of these (make for the index path, a heap SubPointer, a heap
// SampledImageValue, and make plus a slice-header box for a composite).
func TestArenaLetsTemporariesAllocateNothing(t *testing.T) {
	interp := &interpreter{}
	interp.beginInvocation()
	parent := &Pointer{Val: ValUint(0)}
	inv := interp.inv
	n := testing.AllocsPerRun(200, func() {
		// Reset the bump indices so each run stays inside the arena instead of
		// spilling to the heap.
		inv.arrayIdx, inv.subIdx, inv.sampledIdx = 0, 0, 0
		_ = ValArray(interp.allocArray(4))
		_ = interp.allocSubPointer(parent, 4)
		_ = interp.allocSampledImage(ValUint(0), ValUint(0))
	})
	if n != 0 {
		t.Fatalf("arena temporaries allocated %.1f times, want 0", n)
	}
}

// TestValArrayKeepsBackingAlive pins the ValArray encoding: Ref holds the data
// pointer and U[0] the length, so a Value keeps its backing array reachable
// after the slice header is gone. A dangling Ref would show garbage or crash
// once the collector runs.
func TestValArrayKeepsBackingAlive(t *testing.T) {
	v := buildArrayValue()
	runtime.GC()
	runtime.GC()
	got := v.AsArray()
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	for i, want := range []float32{11, 22, 33} {
		if got[i].AsFloat32() != want {
			t.Fatalf("element %d = %v, want %v", i, got[i].AsFloat32(), want)
		}
	}
}

// buildArrayValue returns a Value built from a slice that lives only for the
// duration of the call.
func buildArrayValue() Value {
	return ValArray([]Value{ValFloat(11), ValFloat(22), ValFloat(33)})
}

// TestArenaHeapFallbackPastTheCaps pins the fallback branches (#419 lever 2
// review, gap 2): a loop that builds more composites, SubPointers, index paths
// or sampled images than the arena holds must keep every earlier value intact
// and take fresh memory for the excess. A regression here corrupts values
// silently rather than showing up as an allocation count.
func TestArenaHeapFallbackPastTheCaps(t *testing.T) {
	interp := &interpreter{}
	interp.beginInvocation()
	parent := &Pointer{Val: ValUint(0)}

	// Arrays: fill the arena exactly, then take one past it.
	first := interp.allocArray(2)
	first[0], first[1] = ValFloat(1), ValFloat(2)
	firstVal := ValArray(first)
	for i := 0; i < invArrayCap-2; i++ {
		interp.allocArray(1)
	}
	over := interp.allocArray(3)
	over[0], over[1], over[2] = ValFloat(9), ValFloat(9), ValFloat(9)
	if got := firstVal.AsArray(); got[0].AsFloat32() != 1 || got[1].AsFloat32() != 2 {
		t.Fatalf("earlier composite corrupted by a heap fallback: %v", got)
	}
	if &over[0] == &first[0] {
		t.Fatal("past the cap the backing must be fresh memory, not the arena")
	}

	// SubPointers: fill the arena, then one past it.
	sp := interp.allocSubPointer(parent, 1)
	sp.Indexes[0] = 7
	for i := 0; i < invSubCap-1; i++ {
		interp.allocSubPointer(parent, 1)
	}
	spOver := interp.allocSubPointer(parent, 1)
	spOver.Indexes[0] = 9
	if sp.Indexes[0] != 7 {
		t.Fatal("heap-fallback SubPointer overwrote an arena one")
	}

	// An index path longer than the inline capacity uses the heap and keeps
	// its values.
	long := interp.allocSubPointer(parent, subPointerInline+2)
	for i := range long.Indexes {
		long.Indexes[i] = uint32(i + 1)
	}
	for i, got := range long.Indexes {
		if got != uint32(i+1) {
			t.Fatalf("long index path[%d] = %d, want %d", i, got, i+1)
		}
	}

	// Sampled images: fill the arena, then one past it.
	si := interp.allocSampledImage(ValUint(1), ValUint(2))
	for i := 0; i < invSampledImageCap-1; i++ {
		interp.allocSampledImage(ValUint(0), ValUint(0))
	}
	siOver := interp.allocSampledImage(ValUint(3), ValUint(4))
	if si.Ref == siOver.Ref {
		t.Fatal("past the cap the sampled image must be fresh, not an arena slot")
	}
	if si.AsSampledImage().Image.AsUint32() != 1 || siOver.AsSampledImage().Image.AsUint32() != 3 {
		t.Fatal("heap-fallback sampled image corrupted an arena one")
	}
}
