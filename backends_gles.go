//go:build ((windows && (386 || amd64 || arm64)) || (linux && !android && (amd64 || arm64)) || (android && arm64)) && !rust && !(js && wasm) && !wgpu_nogles

package wgpu

// The OpenGL/GLES backend is registered by default (ADR-083 §2.1). Build with
// -tags wgpu_nogles to leave it out and shrink the binary.
import _ "github.com/gogpu/wgpu/hal/gles"
