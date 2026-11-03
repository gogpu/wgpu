// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

// Package handletable maps opaque integer handles to Go objects.
//
// HAL resources expose a NativeHandle() uintptr, and bind group entries carry
// those handles back into the backend (gputypes.BufferBinding.Buffer and so
// on). For Vulkan, Metal and the DX12 Texture that handle is a native object
// (a VkHandle, an Objective-C id, a COM pointer). Where a backend needs its
// own Go struct back instead (DX12 buffers, texture views and samplers, GLES
// texture views), the handle must not be the struct's address: converting a
// Go pointer to uintptr and back breaks rule 6 of the unsafe.Pointer rules
// (the uintptr does not keep the object alive and the GC may move or free
// it), and checkptr, which go test -race enables, reports it as a fatal
// error.
//
// A Table hands out a fresh integer per object and resolves it through a map.
// The map holds the object alive until Remove, so a resource must Remove its
// handle when it is destroyed.
package handletable

import "sync"

// Table maps handles to *T. The zero value is an empty table ready to use.
// A Table must not be copied after first use.
type Table[T any] struct {
	mu      sync.RWMutex
	last    uintptr
	objects map[uintptr]*T
}

// Add registers v and returns its handle. Handles are never 0 (0 means "no
// resource" in the HAL binding types) and increase monotonically, so a
// removed handle is not handed out again until the counter wraps.
func (t *Table[T]) Add(v *T) uintptr {
	if v == nil {
		panic("handletable: Add(nil)")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.objects == nil {
		t.objects = make(map[uintptr]*T)
	}
	for {
		t.last++
		if _, used := t.objects[t.last]; t.last != 0 && !used {
			break
		}
	}
	t.objects[t.last] = v
	return t.last
}

// Get returns the object registered under h. It reports false for 0, for a
// handle this table never issued and for a removed handle.
func (t *Table[T]) Get(h uintptr) (*T, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	v, ok := t.objects[h]
	return v, ok
}

// Remove drops h. Removing 0, an unknown or an already removed handle does
// nothing, so a resource's Destroy may call it more than once.
func (t *Table[T]) Remove(h uintptr) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.objects, h)
}

// Len returns the number of registered objects.
func (t *Table[T]) Len() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.objects)
}
