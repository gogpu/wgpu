// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package allbackends

import (
	// Import all HAL backends for side-effect registration.
	// Each backend's init() function registers it with hal.RegisterBackend().

	// No-op backend - always available, useful for testing.
	_ "github.com/gogpu/wgpu/hal/noop"
)
