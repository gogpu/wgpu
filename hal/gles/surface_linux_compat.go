// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build linux && !(js && wasm)

package gles

import "github.com/gogpu/gputypes"

// Compatibility wrappers for Linux Surface which still owns its GL context
// directly (Phase 1 = Windows only). These delegate to the *With() methods
// using the Surface's own glCtx. Will be removed in Phase 2 when Linux
// migrates to AdapterContext.

func (s *Surface) reconfigureSwapchainFBO(format gputypes.TextureFormat, width, height uint32) error {
	return s.reconfigureSwapchainFBOWith(s.glCtx, format, width, height)
}

func (s *Surface) blitSwapchainToDefault() {
	s.blitSwapchainToDefaultWith(s.glCtx)
}
