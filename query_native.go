//go:build !rust && !(js && wasm)

package wgpu

import (
	"sync/atomic"

	"github.com/gogpu/wgpu/hal"
)

// QuerySet is a collection of timestamp or occlusion queries.
type QuerySet struct {
	hal      hal.QuerySet
	device   *Device
	released atomic.Bool
}

// Release destroys the query set. It is safe to call Release more than once.
func (q *QuerySet) Release() {
	if q == nil || q.released.Swap(true) {
		return
	}
	if device := q.device.halDevice(); device != nil {
		device.DestroyQuerySet(q.hal)
	}
}
