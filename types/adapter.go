package types

// DeviceType identifies the type of GPU device.
type DeviceType uint8

const (
	// DeviceTypeOther is an unknown or other device type.
	DeviceTypeOther DeviceType = iota
	// DeviceTypeIntegratedGPU is integrated into the CPU (shared memory).
	DeviceTypeIntegratedGPU
	// DeviceTypeDiscreteGPU is a separate GPU with dedicated memory.
	DeviceTypeDiscreteGPU
	// DeviceTypeVirtualGPU is a virtual GPU (e.g., in a VM).
	DeviceTypeVirtualGPU
	// DeviceTypeCPU is software rendering on the CPU.
	DeviceTypeCPU
)

// String returns the device type name.
func (d DeviceType) String() string {
	switch d {
	case DeviceTypeOther:
		return "Other"
	case DeviceTypeIntegratedGPU:
		return "IntegratedGpu"
	case DeviceTypeDiscreteGPU:
		return "DiscreteGpu"
	case DeviceTypeVirtualGPU:
		return "VirtualGpu"
	case DeviceTypeCPU:
		return "Cpu"
	default:
		return "Unknown"
	}
}

// AdapterInfo contains information about a GPU adapter.
type AdapterInfo struct {
	// Name is the name of the adapter (e.g., "NVIDIA GeForce RTX 3080").
	Name string
	// Vendor is the adapter vendor (e.g., "NVIDIA").
	Vendor string
	// VendorID is the PCI vendor ID.
	VendorID uint32
	// DeviceID is the PCI device ID.
	DeviceID uint32
	// DeviceType indicates the type of GPU.
	DeviceType DeviceType
	// Driver is the driver version.
	Driver string
	// DriverInfo is additional driver information.
	DriverInfo string
	// Backend is the graphics backend in use.
	Backend Backend
}

// PowerPreference specifies power consumption preference.
type PowerPreference uint8

const (
	// PowerPreferenceNone has no preference.
	PowerPreferenceNone PowerPreference = iota
	// PowerPreferenceLowPower prefers low power consumption (integrated GPU).
	PowerPreferenceLowPower
	// PowerPreferenceHighPerformance prefers high performance (discrete GPU).
	PowerPreferenceHighPerformance
)

// RequestAdapterOptions controls adapter selection.
type RequestAdapterOptions struct {
	// PowerPreference indicates power consumption preference.
	PowerPreference PowerPreference
	// ForceFallbackAdapter forces the use of a fallback adapter.
	ForceFallbackAdapter bool
	// CompatibleSurface is an optional surface the adapter must support.
	CompatibleSurface *Surface
}

// Surface represents a window surface (placeholder).
type Surface struct {
	// Handle is platform-specific surface handle.
	Handle uintptr
}

// MemoryHints provides memory allocation hints.
type MemoryHints uint8

const (
	// MemoryHintsPerformance optimizes for performance.
	MemoryHintsPerformance MemoryHints = iota
	// MemoryHintsMemoryUsage optimizes for low memory usage.
	MemoryHintsMemoryUsage
)

// DeviceDescriptor describes how to create a device.
type DeviceDescriptor struct {
	// Label is a debug label for the device.
	Label string
	// RequiredFeatures lists features the device must support.
	RequiredFeatures []Feature
	// RequiredLimits specifies limits the device must meet.
	RequiredLimits Limits
	// MemoryHints provides memory allocation hints.
	MemoryHints MemoryHints
}

// DefaultDeviceDescriptor returns the default device descriptor.
func DefaultDeviceDescriptor() DeviceDescriptor {
	return DeviceDescriptor{
		RequiredFeatures: nil,
		RequiredLimits:   DefaultLimits(),
		MemoryHints:      MemoryHintsPerformance,
	}
}
