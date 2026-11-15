//go:build !(js && wasm)

package layoutcheck

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoTargetInvertedTwins enforces ADR-070 §15.2's golden-twin rule: no pair may exist both as a target
// row and as an inverted row. A target twin would legitimize the very edge the inversion forbids
// (covers() matches target rows only) and would be unclaimable at phase: final.
func TestNoTargetInvertedTwins(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	read := func(name string) map[string]int {
		f, err := os.Open(filepath.Join(root, "scripts", "inversion-graph", name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		defer f.Close()
		pairs := map[string]int{}
		sc := bufio.NewScanner(f)
		line := 0
		for sc.Scan() {
			line++
			s := sc.Text()
			if strings.HasPrefix(s, "#") || strings.TrimSpace(s) == "" {
				continue
			}
			fields := strings.Split(s, "\t")
			if len(fields) < 3 {
				t.Fatalf("%s:%d: malformed row", name, line)
			}
			pairs[fields[0]+"\t"+fields[1]] = line
		}
		if err := sc.Err(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return pairs
	}
	target := read("target-edges.tsv")
	inverted := read("inverted-edges.tsv")
	for pair, line := range inverted {
		if tline, ok := target[pair]; ok {
			t.Errorf("inverted-edges.tsv:%d and target-edges.tsv:%d: %s exists as both a target row and an inverted row — an inverted pair is claims-only (a target twin legitimizes the forbidden edge)", line, tline, pair)
		}
	}
}
