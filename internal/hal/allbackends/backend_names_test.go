package allbackends_test

// backendName names a backend variant the way these tests and naga's GPU
// readback corpus spell it (a known failure's "backend"). Tests compare and
// key maps by it, so every use goes through a constant: a misspelled literal
// would compile and silently match nothing.
type backendName string

// The corpus backend names (naga testdata/readback README, "Known
// failures"), plus backendDX12 for tests that do not split DX12 by shader
// compiler.
const (
	backendVulkan   backendName = "vulkan"
	backendDX12FXC  backendName = "dx12-fxc"
	backendDX12DXIL backendName = "dx12-dxil"
	backendGLES     backendName = "gles"
	backendMetal    backendName = "metal"
	backendSoftware backendName = "software"
	backendDX12     backendName = "dx12"
)
