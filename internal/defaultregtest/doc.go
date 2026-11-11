//go:build !rust && !(js && wasm)

// Package defaultregtest holds end-to-end checks of the public default-backend
// registration contract (ADR-083 Phase 0). It lives in its own package so its
// test binary imports only the public wgpu API and the HAL registry: no test
// helper and no explicit backend, which is what a normal program sees.
package defaultregtest
