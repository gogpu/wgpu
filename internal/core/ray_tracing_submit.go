//go:build !(js && wasm)

package core

import (
	"fmt"
	"sync"

	"github.com/gogpu/wgpu/internal/hal"
	"github.com/gogpu/wgpu/internal/raytracing"
)

// deviceRayTracingState holds the device-wide acceleration structure state
// shared by all submissions.
type deviceRayTracingState struct {
	mu sync.Mutex
	// nextBuildIndex is the index the next submitted build command gets.
	// Indices start at 1; 0 means "never built".
	//
	// Reference: wgpu-core device/resource.rs
	// CommandIndices.next_acceleration_structure_build_command_index
	nextBuildIndex uint64
}

func newDeviceRayTracingState() *deviceRayTracingState {
	return &deviceRayTracingState{nextBuildIndex: 1}
}

// RecordTlasUse records that a TLAS is used by a bind group set in this
// encoder. At submit the TLAS must be built, and must not be older than any
// BLAS it references.
//
// Reference: wgpu-core AsAction::UseTlas, recorded when a bind group is used.
func (e *CoreCommandEncoder) RecordTlasUse(tlas *Tlas) {
	if tlas == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.mutable != nil {
		e.mutable.asActions = append(e.mutable.asActions, asAction{useTlas: tlas})
	}
}

// takeTempBuffers removes and returns the encoder's temporary HAL buffers.
func (e *CoreCommandEncoder) takeTempBuffers() []hal.Buffer {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.mutable == nil {
		return nil
	}
	buffers := e.mutable.tempBuffers
	e.mutable.tempBuffers = nil
	return buffers
}

// TempBufferCount returns the number of temporary HAL buffers the encoder
// holds.
func (e *CoreCommandEncoder) TempBufferCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.mutable == nil {
		return 0
	}
	return len(e.mutable.tempBuffers)
}

// DestroyTempBuffers destroys the encoder's temporary HAL buffers
// (acceleration structure scratch and staging). Called when the encoder is
// discarded, or its Finish failed, so no command buffer will use them.
func (e *CoreCommandEncoder) DestroyTempBuffers() {
	if e == nil {
		return
	}
	e.device.destroyHALBuffers(e.takeTempBuffers())
}

// takeTempBuffers removes and returns the command buffer's temporary HAL
// buffers.
func (cb *CoreCommandBuffer) takeTempBuffers() []hal.Buffer {
	if cb.mutable == nil {
		return nil
	}
	buffers := cb.mutable.tempBuffers
	cb.mutable.tempBuffers = nil
	return buffers
}

// DestroyTempBuffers destroys the command buffer's temporary HAL buffers.
// Called when the command buffer is released without being submitted.
func (cb *CoreCommandBuffer) DestroyTempBuffers() {
	if cb == nil {
		return
	}
	cb.device.destroyHALBuffers(cb.takeTempBuffers())
}

// TakeTempBufferDestroyer removes the command buffer's temporary HAL buffers
// (acceleration structure scratch and staging) and returns a function that
// destroys them, or nil if there are none. The queue defers it until the GPU
// has executed the command buffer.
//
// Reference: wgpu-core command/mod.rs CommandBufferMutable.temp_resources
// (TempResource, freed when the submission completes)
func (cb *CoreCommandBuffer) TakeTempBufferDestroyer() func() {
	if cb == nil {
		return nil
	}
	buffers := cb.takeTempBuffers()
	if len(buffers) == 0 {
		return nil
	}
	device := cb.device
	return func() { device.destroyHALBuffers(buffers) }
}

// destroyHALBuffers destroys internal HAL buffers, unless the device is gone.
func (d *Device) destroyHALBuffers(buffers []hal.Buffer) {
	if d == nil || len(buffers) == 0 {
		return
	}
	guard := d.snatchLock.Read()
	defer guard.Release()
	raw := d.Raw(guard)
	if raw == nil {
		return
	}
	for _, buf := range buffers {
		raw.DestroyBuffer(buf)
	}
}

// ValidateAccelerationStructureActions validates the acceleration structure
// actions of command buffers about to be submitted, in submission order.
//
// Each build command gets the next build index; the BLASes and TLASes it
// builds take that index as their BuiltIndex, built BLASes return to the Idle
// compaction state, and built TLASes take their new BLAS dependencies. Each
// TLAS use requires the TLAS to be built and every BLAS it depends on to be
// built no later than it.
//
// Matches Rust wgpu-core command/ray_tracing.rs
// validate_acceleration_structure_actions.
func (d *Device) ValidateAccelerationStructureActions(commandBuffers []*CoreCommandBuffer) error {
	hasActions := false
	for _, cb := range commandBuffers {
		if cb != nil && cb.mutable != nil && len(cb.mutable.asActions) > 0 {
			hasActions = true
			break
		}
	}
	if !hasActions {
		return nil
	}
	if d.rayTracing == nil || d.snatchLock == nil {
		return ErrDeviceDestroyed
	}

	d.rayTracing.mu.Lock()
	defer d.rayTracing.mu.Unlock()

	// Dependencies replaced by new builds are dropped after the snatch guard
	// is released: dropping the last reference destroys the BLAS, which takes
	// the snatch lock exclusively.
	var replaced []*Blas
	defer func() {
		for _, blas := range replaced {
			blas.Ref.Drop()
		}
	}()

	guard := d.snatchLock.Read()
	defer guard.Release()

	for _, cb := range commandBuffers {
		if cb == nil || cb.mutable == nil {
			continue
		}
		for _, action := range cb.mutable.asActions {
			var err error
			if action.build != nil {
				replaced, err = d.applyBuild(action.build, replaced)
			} else {
				err = validateTlasUse(action.useTlas, guard)
			}
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// applyBuild assigns the next build index to everything a build command
// built. Caller holds d.rayTracing.mu.
func (d *Device) applyBuild(build *asBuild, replaced []*Blas) ([]*Blas, error) {
	index := d.rayTracing.nextBuildIndex
	d.rayTracing.nextBuildIndex++

	for _, blas := range build.blases {
		blas.mu.Lock()
		// Validated out at encode time, as in Rust.
		if blas.state.Compaction != raytracing.CompactionCompacted {
			// Reset the compaction state, so pending compaction preparations end.
			blas.state.Compaction = raytracing.CompactionIdle
		}
		blas.state.BuiltIndex = index
		blas.mu.Unlock()
	}

	for _, built := range build.tlases {
		for _, blas := range built.dependencies {
			if blas.BuiltIndex() == 0 {
				return replaced, &ValidateAsActionsError{
					Kind: ValidateAsActionsErrorUsedUnbuiltBlas,
					Message: fmt.Sprintf("BLAS %q is used before it is built (in TLAS %q)",
						blas.Label(), built.tlas.Label()),
				}
			}
		}
		for _, blas := range built.dependencies {
			blas.Ref.Clone()
		}
		tlas := built.tlas
		tlas.mu.Lock()
		tlas.state.BuiltIndex = index
		old := tlas.dependencies
		tlas.dependencies = append([]*Blas(nil), built.dependencies...)
		tlas.mu.Unlock()
		replaced = append(replaced, old...)
	}
	return replaced, nil
}

// validateTlasUse checks that a TLAS bound for shader use is built and not
// older than the BLASes it references.
func validateTlasUse(tlas *Tlas, guard SnatchGuard) error {
	if tlas.Raw(guard) == nil {
		return &ValidateAsActionsError{
			Kind:    ValidateAsActionsErrorDestroyedResource,
			Message: fmt.Sprintf("TLAS %q is destroyed", tlas.Label()),
		}
	}
	tlas.mu.Lock()
	tlasIndex := tlas.state.BuiltIndex
	deps := append([]*Blas(nil), tlas.dependencies...)
	tlas.mu.Unlock()

	if tlasIndex == 0 {
		return &ValidateAsActionsError{
			Kind:    ValidateAsActionsErrorUsedUnbuiltTlas,
			Message: fmt.Sprintf("TLAS %q is used before it is built", tlas.Label()),
		}
	}
	for _, blas := range deps {
		blasIndex := blas.BuiltIndex()
		if blasIndex == 0 {
			return &ValidateAsActionsError{
				Kind: ValidateAsActionsErrorUsedUnbuiltBlas,
				Message: fmt.Sprintf("BLAS %q is used before it is built (in TLAS %q)",
					blas.Label(), tlas.Label()),
			}
		}
		if blasIndex > tlasIndex {
			return &ValidateAsActionsError{
				Kind: ValidateAsActionsErrorBlasNewerThanTlas,
				Message: fmt.Sprintf("BLAS %q is newer than the containing TLAS %q",
					blas.Label(), tlas.Label()),
			}
		}
		if blas.Raw(guard) == nil {
			return &ValidateAsActionsError{
				Kind:    ValidateAsActionsErrorDestroyedResource,
				Message: fmt.Sprintf("BLAS %q (in TLAS %q) is destroyed", blas.Label(), tlas.Label()),
			}
		}
	}
	return nil
}
