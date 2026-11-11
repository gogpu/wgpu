//go:build !rust && !(js && wasm)

package wgpu

// CheckHealth returns an error when the backend reports the device as lost or
// unhealthy, and nil when the backend has no health check.
//
// A lost device's error satisfies errors.Is(err, ErrDeviceLost). Today only
// the DX12 backend implements the check (DEVICE_REMOVED diagnostics); the
// other backends have no health signal and return nil.
func (d *Device) CheckHealth() error {
	if d == nil {
		return nil
	}
	if d.released.Load() {
		return ErrReleased
	}
	halDevice := d.halDevice()
	if halDevice == nil {
		return nil
	}
	checker, ok := halDevice.(interface{ CheckHealth(string) error })
	if !ok {
		return nil
	}
	return checker.CheckHealth("Device.CheckHealth")
}
