//go:build !rust && !(js && wasm)

package resource

import (
	"log/slog"
	"runtime"
	"sync/atomic"

	"github.com/gogpu/wgpu/internal/core"
	"github.com/gogpu/wgpu/internal/hal"
)

// asCleanupRef holds what a runtime.AddCleanup callback needs to drop an
// acceleration structure's ownership reference. It must not reference the
// Blas or Tlas itself.
type asCleanupRef struct {
	kind     string
	label    string
	released *atomic.Bool
	drop     func()
}

func registerASCleanup[T any](obj *T, ref asCleanupRef) runtime.Cleanup {
	return runtime.AddCleanup(obj, func(ref asCleanupRef) {
		if !ref.released.CompareAndSwap(false, true) {
			return
		}
		slog.Warn("wgpu: "+ref.kind+" released by GC (missing explicit Release)", "label", ref.label)
		ref.drop()
	}, ref)
}

// Blas is the implementation of a bottom-level acceleration structure: the
// application's ownership reference on a core BLAS. Experimental; requires
// FeatureRayQuery.
type Blas struct {
	core     *core.Blas
	released *atomic.Bool
	cleanup  runtime.Cleanup
}

// NewBlas wraps a core BLAS the device created, taking over its creation
// reference, and arms its GC cleanup.
func NewBlas(c *core.Blas) *Blas {
	b := &Blas{core: c, released: new(atomic.Bool)}
	b.cleanup = registerASCleanup(b, asCleanupRef{
		kind: "Blas", label: c.Label(), released: b.released, drop: c.Ref.Drop,
	})
	return b
}

// Core returns the core BLAS, or nil for a zero value.
func (b *Blas) Core() *core.Blas {
	if b == nil {
		return nil
	}
	return b.core
}

// IsReleased reports whether Release has run on the BLAS (or it is a zero
// value).
func (b *Blas) IsReleased() bool {
	return b == nil || b.released == nil || b.released.Load()
}

// Release drops the application's reference to the BLAS. The acceleration
// structure is destroyed once no in-flight command buffer and no TLAS
// references it anymore.
func (b *Blas) Release() {
	if b == nil || b.released == nil || !b.released.CompareAndSwap(false, true) {
		return
	}
	b.cleanup.Stop()
	b.core.Ref.Drop()
}

// Tlas is the implementation of a top-level acceleration structure: the
// application's ownership reference on a core TLAS, and the references it
// holds on the BLASes its instance slots point at (in Rust, each TlasInstance
// holds an Arc<Blas>). Experimental; requires FeatureRayQuery.
//
// Tlas is not safe for concurrent use.
type Tlas struct {
	core     *core.Tlas
	released *atomic.Bool
	cleanup  runtime.Cleanup

	// slotRefs[i] is the BLAS whose reference the TLAS holds for instance
	// slot i. The public wrapper reconciles a slot whenever it changes, and
	// every slot at a build (RetainSlot).
	slotRefs []*core.Blas
}

// NewTlas wraps a core TLAS the device created, taking over its creation
// reference, with MaxInstances empty slots, and arms its GC cleanup.
func NewTlas(c *core.Tlas) *Tlas {
	t := &Tlas{
		core:     c,
		released: new(atomic.Bool),
		slotRefs: make([]*core.Blas, c.MaxInstances()),
	}
	// The cleanup drops the slot refs too; it captures the slice, not t.
	slotRefs := t.slotRefs
	t.cleanup = registerASCleanup(t, asCleanupRef{
		kind: "Tlas", label: c.Label(), released: t.released,
		drop: func() {
			for _, held := range slotRefs {
				if held != nil {
					held.Ref.Drop()
				}
			}
			c.Ref.Drop()
		},
	})
	return t
}

// Core returns the core TLAS, or nil for a zero value.
func (t *Tlas) Core() *core.Tlas {
	if t == nil {
		return nil
	}
	return t.core
}

// HAL returns the HAL acceleration structure, or nil if it is destroyed.
func (t *Tlas) HAL() hal.AccelerationStructure {
	if t == nil {
		return nil
	}
	return t.core.HAL()
}

// IsReleased reports whether Release has run on the TLAS (or it is a zero
// value).
func (t *Tlas) IsReleased() bool {
	return t == nil || t.released == nil || t.released.Load()
}

// Release drops the application's reference to the TLAS and to the BLASes
// its instance slots hold. The acceleration structure is destroyed once no
// in-flight command buffer or bind group references it anymore.
func (t *Tlas) Release() {
	if t == nil || t.released == nil || !t.released.CompareAndSwap(false, true) {
		return
	}
	t.cleanup.Stop()
	for i, held := range t.slotRefs {
		if held != nil {
			held.Ref.Drop()
			t.slotRefs[i] = nil
		}
	}
	t.core.Ref.Drop()
}

// RetainSlot moves slot i's held BLAS reference to want (nil empties the
// slot). Reports whether want is still alive; a destroyed BLAS is not
// retained.
func (t *Tlas) RetainSlot(i int, want *Blas) bool {
	wantCore := want.Core()
	held := t.slotRefs[i]
	if held == wantCore {
		return wantCore == nil || !wantCore.IsDestroyed()
	}
	if wantCore != nil {
		if wantCore.IsDestroyed() {
			return false
		}
		wantCore.Ref.Clone()
	}
	if held != nil {
		held.Ref.Drop()
	}
	t.slotRefs[i] = wantCore
	return true
}
