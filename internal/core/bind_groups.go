package core

// MaxBindGroups is the HAL hard cap on bind groups (wgpu-hal's
// MAX_BIND_GROUPS = 8). It is the length of the fixed-size per-group arrays a
// render or compute pipeline keeps, and it lives here because the extraction
// packages that hold those arrays cannot import the root package. The root
// package re-exports it as its public MaxBindGroups (types.go).
const MaxBindGroups = 8
