// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin && !(js && wasm)

package metal

import "testing"

// TestSelectorsAreNeverCachedAsZero pins the lazy runtime resolution and the
// cache invariant behind it. A selector reached through the accessors resolves
// the Objective-C runtime first and is cached once; a zero result is never
// stored. The #429 CI SIGABRT came from a poisoned cache: a Sel call before the
// first Init cached "alloc" as zero (goffi returns zeros for a nil symbol), and
// a later NSString allocation used the nil selector, leaving the Metal API
// call with a nil source.
func TestSelectorsAreNeverCachedAsZero(t *testing.T) {
	probe := "wgpuObjCLazyInitProbe:"
	first := RegisterSelector(probe)
	if first == 0 {
		t.Fatalf("RegisterSelector(%q) = 0: the accessors must resolve the runtime lazily", probe)
	}
	if again := RegisterSelector(probe); again != first {
		t.Fatalf("RegisterSelector(%q) returned %#x then %#x, want a stable selector", probe, first, again)
	}

	selectorCache.Range(func(name, sel any) bool {
		s, ok := sel.(SEL)
		if !ok || s == 0 {
			t.Errorf("selectorCache holds %v for %v: a zero selector poisons every later message using it", sel, name)
		}
		return true
	})
}
