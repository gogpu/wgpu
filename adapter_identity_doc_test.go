//go:build !rust && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package wgpu_test

import (
	"os"
	"strings"
	"testing"
)

// TestIdentityDocTableConsistent pins that the per-identifier contract table in
// the Identity() doc comment is identical across the three build-variant
// declaration files. Each file compiles alone, so the table is necessarily
// repeated there; an edit to one copy must reach the others (implementation
// gate P3-2, the maintenance risk of the triplication).
func TestIdentityDocTableConsistent(t *testing.T) {
	files := []string{"adapter_native.go", "adapter_rust.go", "adapter_browser.go"}
	tables := make([][]string, 0, len(files))
	for _, name := range files {
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		var table []string
		for _, line := range strings.Split(string(src), "\n") {
			if strings.HasPrefix(line, "// |") {
				table = append(table, line)
			}
		}
		if len(table) < 5 {
			t.Fatalf("%s: %d table lines, want the full per-identifier table (>=5)", name, len(table))
		}
		tables = append(tables, table)
	}
	for i := 1; i < len(tables); i++ {
		if strings.Join(tables[i], "\n") != strings.Join(tables[0], "\n") {
			t.Fatalf("the Identity doc table differs between %s and %s; edit all three copies", files[0], files[i])
		}
	}
}
