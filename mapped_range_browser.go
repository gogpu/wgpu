//go:build js && wasm

package wgpu

// MappedRange is a safe view over a region of a mapped GPU buffer.
type MappedRange struct{}

// Bytes returns the underlying byte slice.
func (m *MappedRange) Bytes() []byte {
	panic("wgpu: browser backend not yet implemented")
}
