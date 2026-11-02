// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package handletable

import (
	"sync"
	"testing"
)

type object struct{ id int }

func TestTable(t *testing.T) {
	a, b := &object{1}, &object{2}

	tests := []struct {
		name string
		run  func(t *testing.T, tab *Table[object])
	}{
		{"zero value is empty", func(t *testing.T, tab *Table[object]) {
			if tab.Len() != 0 {
				t.Fatalf("Len = %d, want 0", tab.Len())
			}
			if _, ok := tab.Get(1); ok {
				t.Fatal("Get(1) on an empty table succeeded")
			}
			tab.Remove(1) // must not panic
		}},
		{"add then get", func(t *testing.T, tab *Table[object]) {
			ha, hb := tab.Add(a), tab.Add(b)
			if ha == 0 || hb == 0 {
				t.Fatalf("handles %d, %d; 0 is reserved", ha, hb)
			}
			if hb <= ha {
				t.Fatalf("handles not increasing: %d then %d", ha, hb)
			}
			if got, ok := tab.Get(ha); !ok || got != a {
				t.Fatalf("Get(%d) = %v, %v; want a", ha, got, ok)
			}
			if got, ok := tab.Get(hb); !ok || got != b {
				t.Fatalf("Get(%d) = %v, %v; want b", hb, got, ok)
			}
		}},
		{"zero handle never resolves", func(t *testing.T, tab *Table[object]) {
			tab.Add(a)
			if _, ok := tab.Get(0); ok {
				t.Fatal("Get(0) succeeded")
			}
		}},
		{"remove is idempotent and not reused", func(t *testing.T, tab *Table[object]) {
			h := tab.Add(a)
			tab.Remove(h)
			tab.Remove(h)
			if _, ok := tab.Get(h); ok {
				t.Fatal("Get after Remove succeeded")
			}
			if h2 := tab.Add(b); h2 == h {
				t.Fatalf("removed handle %d issued again", h)
			}
			if tab.Len() != 1 {
				t.Fatalf("Len = %d, want 1", tab.Len())
			}
		}},
		{"same object twice gets two handles", func(t *testing.T, tab *Table[object]) {
			h1, h2 := tab.Add(a), tab.Add(a)
			if h1 == h2 {
				t.Fatalf("both adds returned %d", h1)
			}
			tab.Remove(h1)
			if got, ok := tab.Get(h2); !ok || got != a {
				t.Fatal("removing one handle dropped the other")
			}
		}},
		{"counter wrap skips 0 and live handles", func(t *testing.T, tab *Table[object]) {
			live := tab.Add(a) // handle 1
			tab.last = ^uintptr(0)
			h := tab.Add(b)
			if h == 0 || h == live {
				t.Fatalf("after wrap got handle %d (live handle %d)", h, live)
			}
			if got, _ := tab.Get(live); got != a {
				t.Fatal("wrap overwrote a live handle")
			}
		}},
		{"add nil panics", func(t *testing.T, tab *Table[object]) {
			defer func() {
				if recover() == nil {
					t.Fatal("Add(nil) did not panic")
				}
			}()
			tab.Add(nil)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.run(t, &Table[object]{})
		})
	}
}

// TestTableConcurrent adds, resolves and removes from many goroutines. Run
// it under -race: the race detector reports any access to the map or the
// counter that the mutex does not cover.
func TestTableConcurrent(t *testing.T) {
	const workers, perWorker = 8, 500
	var tab Table[object]
	var wg sync.WaitGroup
	handles := make([][]uintptr, workers)

	for w := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range perWorker {
				obj := &object{w*perWorker + i}
				h := tab.Add(obj)
				if got, ok := tab.Get(h); !ok || got != obj {
					t.Errorf("Get(%d) = %v, %v right after Add", h, got, ok)
					return
				}
				handles[w] = append(handles[w], h)
				if i%2 == 1 {
					tab.Remove(h)
				}
			}
		}()
	}
	wg.Wait()

	seen := make(map[uintptr]bool)
	for _, hs := range handles {
		for _, h := range hs {
			if seen[h] {
				t.Fatalf("handle %d issued twice", h)
			}
			seen[h] = true
		}
	}
	if want := workers * perWorker / 2; tab.Len() != want {
		t.Fatalf("Len = %d, want %d", tab.Len(), want)
	}
}
