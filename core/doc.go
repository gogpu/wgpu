// Package core provides validation and state management for WebGPU resources.
//
// This package implements the core layer between the user-facing API and
// the hardware abstraction layer (HAL). It handles:
//
//   - Type-safe resource identifiers (ID system)
//   - Resource lifecycle management (Registry)
//   - State tracking and validation
//   - Error handling with detailed messages
//
// Architecture:
//
//	types/  → Data structures (no logic)
//	core/   → Validation + State tracking (this package)
//	hal/    → Hardware abstraction layer
//
// The design follows wgpu-core from the Rust wgpu project, adapted for
// idiomatic Go 1.25+ with generics and modern concurrency patterns.
//
// ID System:
//
// Resources are identified by type-safe IDs that combine an index and epoch:
//
//	type DeviceID = ID[deviceMarker]
//	id := NewID[deviceMarker](index, epoch)
//	index, epoch := id.Unzip()
//
// The epoch prevents use-after-free bugs by invalidating old IDs when
// resources are recycled.
//
// Registry Pattern:
//
// Resources are stored in typed registries that manage their lifecycle:
//
//	registry := NewRegistry[Device, deviceMarker]()
//	id, err := registry.Register(device)
//	device, err := registry.Get(id)
//	registry.Unregister(id)
//
// Thread Safety:
//
// All types in this package are safe for concurrent use unless
// explicitly documented otherwise.
package core
