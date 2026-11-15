//go:build (windows || linux) && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package gl

import (
	"strings"
	"testing"
)

// TestLoadFailsWithoutRequiredProcs: a context whose driver returns no entry
// points for the required GL 3.3 / ES 3.0 functions (Windows' GDI generic
// OpenGL 1.1, which the windows-latest CI runner exposed) must fail Load
// instead of leaving nil pointers that the wrappers call (an access violation
// in glGenVertexArrays on Windows).
func TestLoadFailsWithoutRequiredProcs(t *testing.T) {
	var c Context
	if err := c.Load(noProcs); err == nil {
		t.Fatal("Load with no entry points succeeded, want an error")
	} else if !strings.Contains(err.Error(), "glGenVertexArrays") {
		t.Errorf("Load error %q does not name glGenVertexArrays", err)
	}
	if got, want := len(missingProcs(c.requiredProcs())), len(c.requiredProcs()); got != want {
		t.Errorf("missing = %d, want all %d", got, want)
	}
}

func TestMissingProcsNamesOnlyMissing(t *testing.T) {
	procs := []requiredProc[int]{{"a", 1}, {"b", 0}, {"c", 2}, {"d", 0}}
	got := missingProcs(procs)
	if len(got) != 2 || got[0] != "b" || got[1] != "d" {
		t.Errorf("missingProcs = %v, want [b d]", got)
	}
	if err := checkRequiredProcs(procs[:1]); err != nil {
		t.Errorf("checkRequiredProcs with every proc present = %v, want nil", err)
	}
}
