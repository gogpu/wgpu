// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package allbackends

import (
	// Import all HAL backends for side-effect registration.
	// Each backend's init() function registers it with hal.RegisterBackend().

	// Software backend - CPU-based renderer, always available as fallback.
	// Note: noop backend is NOT included here — it's for testing only and
	// should be imported explicitly when needed. Both noop and software
	// register as BackendEmpty, so only one can be active at a time.
	_ "github.com/gogpu/wgpu/hal/software"
)
