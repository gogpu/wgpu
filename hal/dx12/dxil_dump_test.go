// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows && !(js && wasm)

package dx12

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gogpu/naga/ir"
)

// TestDXILDumpPattern: the dump name keeps stage and entry point but cannot
// contain a path separator or a character Windows rejects (P2-DX12-055).
func TestDXILDumpPattern(t *testing.T) {
	got := dxilDumpPattern(ir.StageFragment, `fs/../x:y*"main"`)
	// "*" is os.CreateTemp's placeholder for the random part; the rest must be clean.
	name, ok := strings.CutSuffix(got, "-*.dxil")
	if !ok {
		t.Fatalf("pattern %q does not end with -*.dxil", got)
	}
	if strings.ContainsAny(name, `/\:*?"<>|`) {
		t.Fatalf("pattern %q contains a path or reserved character", got)
	}
	if want := fmt.Sprintf("gogpu-dxil-stage%d-fs____x_y__main_", int(ir.StageFragment)); name != want {
		t.Errorf("pattern = %q, want %q-*.dxil", got, want)
	}
}

// TestDumpRejectedDXILWritesToTempDir: the blob lands in the temp directory
// (TMP/TEMP on Windows), not at a fixed path of one development machine.
func TestDumpRejectedDXILWritesToTempDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TMP", dir)
	t.Setenv("TEMP", dir)
	blob := []byte{'D', 'X', 'B', 'C', 1, 2, 3}

	dumpRejectedDXIL(ir.StageVertex, "vs_main", blob)

	matches, err := filepath.Glob(filepath.Join(dir, fmt.Sprintf("gogpu-dxil-stage%d-vs_main-*.dxil", int(ir.StageVertex))))
	if err != nil || len(matches) != 1 {
		t.Fatalf("dump files in temp dir = %v (err %v), want 1", matches, err)
	}
	data, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(data, blob) {
		t.Errorf("dump = %v, want %v", data, blob)
	}
}
