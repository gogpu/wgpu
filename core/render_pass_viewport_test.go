//go:build !(js && wasm)

package core

import (
	"errors"
	"math"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// The recording HAL types below wrap the package's mocks so a test can see
// what reached the backend: every viewport the render pass was given.

// recordingViewportPass records the viewports it is given.
type recordingViewportPass struct {
	mockRenderPassEncoder
	viewports []gputypes.Viewport
}

var _ hal.RenderPassEncoder = (*recordingViewportPass)(nil)

func (p *recordingViewportPass) SetViewport(vp gputypes.Viewport) {
	p.viewports = append(p.viewports, vp)
}

// recordingViewportEncoder hands out the shared recording pass.
type recordingViewportEncoder struct {
	mockCommandEncoder
	pass *recordingViewportPass
}

var _ hal.CommandEncoder = (*recordingViewportEncoder)(nil)

func (e *recordingViewportEncoder) BeginRenderPass(_ *hal.RenderPassDescriptor) hal.RenderPassEncoder {
	return e.pass
}

// recordingViewportDevice hands out recording viewport encoders.
type recordingViewportDevice struct {
	mockHALDevice
	pass *recordingViewportPass
}

var _ hal.Device = (*recordingViewportDevice)(nil)

func (d *recordingViewportDevice) CreateCommandEncoder(_ *hal.CommandEncoderDescriptor) (hal.CommandEncoder, error) {
	return &recordingViewportEncoder{pass: d.pass}, nil
}

// newViewportPass returns a command encoder and an open render pass on a
// device with the given limits, plus the HAL pass that records viewports.
func newViewportPass(t *testing.T, limits gputypes.Limits) (*CoreCommandEncoder, *CoreRenderPassEncoder, *recordingViewportPass) {
	t.Helper()
	rec := &recordingViewportPass{}
	device := NewDevice(&recordingViewportDevice{pass: rec}, &Adapter{}, gputypes.Features(0), limits, "TestDevice")
	encoder, err := device.CreateCommandEncoder("TestEncoder")
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	pass, err := encoder.BeginRenderPass(&RenderPassDescriptor{Label: "TestPass"})
	if err != nil {
		t.Fatalf("BeginRenderPass: %v", err)
	}
	return encoder, pass, rec
}

// viewportCases builds the valid viewport table. Each case is checked against
// the WebGPU spec's setViewport valid-usage ranges (spec/index.bs:13695-13708)
// and Rust wgpu-core's set_viewport (command/render.rs:2986-3026).
func validViewportCases() []struct {
	name   string
	vp     gputypes.Viewport
	limits gputypes.Limits
} {
	return []struct {
		name   string
		vp     gputypes.Viewport
		limits gputypes.Limits
	}{
		{
			name:   "common viewport",
			vp:     gputypes.Viewport{X: 0, Y: 0, Width: 800, Height: 600, MinDepth: 0, MaxDepth: 1},
			limits: gputypes.DefaultLimits(),
		},
		{
			name:   "zero size",
			vp:     gputypes.Viewport{X: 4, Y: 5, Width: 0, Height: 0, MinDepth: 0, MaxDepth: 1},
			limits: gputypes.DefaultLimits(),
		},
		{
			name:   "negative origin",
			vp:     gputypes.Viewport{X: -100, Y: -50, Width: 20, Height: 10, MinDepth: 0, MaxDepth: 1},
			limits: gputypes.DefaultLimits(),
		},
		{
			// maxTextureDimension2D itself is a legal width and height.
			name:   "size at maxTextureDimension2D",
			vp:     gputypes.Viewport{X: 0, Y: 0, Width: 8192, Height: 8192, MinDepth: 0, MaxDepth: 1},
			limits: gputypes.DefaultLimits(),
		},
		{
			// Origin -2 * maxTextureDimension2D is the lowest legal x and y.
			name:   "origin at the lower range edge",
			vp:     gputypes.Viewport{X: -16384, Y: -16384, Width: 0, Height: 0, MinDepth: 0, MaxDepth: 1},
			limits: gputypes.DefaultLimits(),
		},
		{
			// x + width and y + height may reach 2 * maxTextureDimension2D - 1.
			name:   "extent at the upper range edge",
			vp:     gputypes.Viewport{X: 8192, Y: 8192, Width: 8191, Height: 8191, MinDepth: 0, MaxDepth: 1},
			limits: gputypes.DefaultLimits(),
		},
		{
			name:   "equal depths",
			vp:     gputypes.Viewport{Width: 10, Height: 10, MinDepth: 0.5, MaxDepth: 0.5},
			limits: gputypes.DefaultLimits(),
		},
		{
			// Limits that were never populated skip the range checks (the same
			// convention as the vertex stride checks in validateVertexBuffers);
			// only the spec's own ranges apply.
			name:   "unset limits skip the range checks",
			vp:     gputypes.Viewport{X: 1e9, Y: -1e9, Width: 1e9, Height: 1e9, MinDepth: 0, MaxDepth: 1},
			limits: gputypes.Limits{},
		},
	}
}

func TestCoreRenderPassEncoder_SetViewport_Valid(t *testing.T) {
	for _, tt := range validViewportCases() {
		t.Run(tt.name, func(t *testing.T) {
			encoder, pass, rec := newViewportPass(t, tt.limits)

			pass.SetViewport(tt.vp)

			if err := encoder.Error(); err != nil {
				t.Fatalf("valid viewport recorded a deferred error: %v", err)
			}
			if got := encoder.Status(); got != CommandEncoderStatusLocked {
				t.Fatalf("status = %v, want %v", got, CommandEncoderStatusLocked)
			}
			if len(rec.viewports) != 1 {
				t.Fatalf("HAL viewports = %v, want the one valid viewport", rec.viewports)
			}
			if rec.viewports[0] != tt.vp {
				t.Errorf("HAL viewport = %+v, want %+v", rec.viewports[0], tt.vp)
			}
			if err := pass.End(); err != nil {
				t.Fatalf("End: %v", err)
			}
			if _, err := encoder.Finish(); err != nil {
				t.Fatalf("Finish: %v", err)
			}
		})
	}
}

func TestCoreRenderPassEncoder_SetViewport_Validation(t *testing.T) {
	nan := float32(math.NaN())
	inf := float32(math.Inf(1))
	limits := gputypes.DefaultLimits()
	valid := gputypes.Viewport{X: 0, Y: 0, Width: 100, Height: 100, MinDepth: 0, MaxDepth: 1}
	with := func(change func(vp *gputypes.Viewport)) gputypes.Viewport {
		vp := valid
		change(&vp)
		return vp
	}

	tests := []struct {
		name     string
		vp       gputypes.Viewport
		wantKind RenderPassValidationErrorKind
	}{
		{
			name:     "negative width",
			vp:       with(func(vp *gputypes.Viewport) { vp.Width = -1 }),
			wantKind: RenderPassErrorInvalidViewportRectSize,
		},
		{
			name:     "negative height",
			vp:       with(func(vp *gputypes.Viewport) { vp.Height = -0.5 }),
			wantKind: RenderPassErrorInvalidViewportRectSize,
		},
		{
			name:     "NaN width",
			vp:       with(func(vp *gputypes.Viewport) { vp.Width = nan }),
			wantKind: RenderPassErrorInvalidViewportRectSize,
		},
		{
			name:     "infinite height",
			vp:       with(func(vp *gputypes.Viewport) { vp.Height = inf }),
			wantKind: RenderPassErrorInvalidViewportRectSize,
		},
		{
			name:     "width above maxTextureDimension2D",
			vp:       with(func(vp *gputypes.Viewport) { vp.Width = float32(limits.MaxTextureDimension2D) + 1 }),
			wantKind: RenderPassErrorInvalidViewportRectSize,
		},
		{
			name:     "height above maxTextureDimension2D",
			vp:       with(func(vp *gputypes.Viewport) { vp.Height = float32(limits.MaxTextureDimension2D) + 1 }),
			wantKind: RenderPassErrorInvalidViewportRectSize,
		},
		{
			name:     "NaN x",
			vp:       with(func(vp *gputypes.Viewport) { vp.X = nan }),
			wantKind: RenderPassErrorInvalidViewportRectPosition,
		},
		{
			name:     "infinite y",
			vp:       with(func(vp *gputypes.Viewport) { vp.Y = inf }),
			wantKind: RenderPassErrorInvalidViewportRectPosition,
		},
		{
			name:     "x below -2 * maxTextureDimension2D",
			vp:       with(func(vp *gputypes.Viewport) { vp.X = -2*float32(limits.MaxTextureDimension2D) - 1 }),
			wantKind: RenderPassErrorInvalidViewportRectPosition,
		},
		{
			name:     "y below -2 * maxTextureDimension2D",
			vp:       with(func(vp *gputypes.Viewport) { vp.Y = -2*float32(limits.MaxTextureDimension2D) - 1 }),
			wantKind: RenderPassErrorInvalidViewportRectPosition,
		},
		{
			name:     "x + width past 2 * maxTextureDimension2D - 1",
			vp:       with(func(vp *gputypes.Viewport) { vp.X = 16000; vp.Width = 1000 }),
			wantKind: RenderPassErrorInvalidViewportRectPosition,
		},
		{
			name:     "y + height past 2 * maxTextureDimension2D - 1",
			vp:       with(func(vp *gputypes.Viewport) { vp.Y = 16000; vp.Height = 1000 }),
			wantKind: RenderPassErrorInvalidViewportRectPosition,
		},
		{
			name:     "origin past the upper range edge",
			vp:       with(func(vp *gputypes.Viewport) { vp.X = 2 * float32(limits.MaxTextureDimension2D) }),
			wantKind: RenderPassErrorInvalidViewportRectPosition,
		},
		{
			name:     "minDepth below 0",
			vp:       with(func(vp *gputypes.Viewport) { vp.MinDepth = -0.1 }),
			wantKind: RenderPassErrorInvalidViewportDepth,
		},
		{
			name:     "maxDepth above 1",
			vp:       with(func(vp *gputypes.Viewport) { vp.MaxDepth = 1.5 }),
			wantKind: RenderPassErrorInvalidViewportDepth,
		},
		{
			name:     "NaN minDepth",
			vp:       with(func(vp *gputypes.Viewport) { vp.MinDepth = nan }),
			wantKind: RenderPassErrorInvalidViewportDepth,
		},
		{
			name:     "minDepth above maxDepth",
			vp:       with(func(vp *gputypes.Viewport) { vp.MinDepth = 0.8; vp.MaxDepth = 0.2 }),
			wantKind: RenderPassErrorInvalidViewportDepth,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoder, pass, rec := newViewportPass(t, limits)

			pass.SetViewport(tt.vp)

			if len(rec.viewports) != 0 {
				t.Errorf("invalid viewport reached the HAL pass: %+v", rec.viewports)
			}
			if got := encoder.Status(); got != CommandEncoderStatusError {
				t.Fatalf("status = %v, want %v", got, CommandEncoderStatusError)
			}
			var rpve *RenderPassValidationError
			if !errors.As(encoder.Error(), &rpve) {
				t.Fatalf("deferred error = %T (%v), want *RenderPassValidationError", encoder.Error(), encoder.Error())
			}
			if rpve.Kind != tt.wantKind {
				t.Errorf("kind = %v, want %v (%v)", rpve.Kind, tt.wantKind, rpve)
			}
			// The invalid viewport must not produce a command buffer: End and
			// Finish report the deferred error.
			if err := pass.End(); !errors.As(err, &rpve) {
				t.Errorf("End error = %v, want the deferred viewport error", err)
			}
			if _, err := encoder.Finish(); err == nil {
				t.Error("Finish must fail after a deferred viewport error")
			}
		})
	}
}

// TestCoreRenderPassEncoder_SetViewport_Validation_UnknownLimit checks the
// checks that do not depend on maxTextureDimension2D still run when the limit
// was never populated.
func TestCoreRenderPassEncoder_SetViewport_Validation_UnknownLimit(t *testing.T) {
	tests := []struct {
		name     string
		vp       gputypes.Viewport
		wantKind RenderPassValidationErrorKind
	}{
		{
			name:     "negative width",
			vp:       gputypes.Viewport{Width: -10, Height: 10, MaxDepth: 1},
			wantKind: RenderPassErrorInvalidViewportRectSize,
		},
		{
			name:     "NaN height",
			vp:       gputypes.Viewport{Width: 10, Height: float32(math.NaN()), MaxDepth: 1},
			wantKind: RenderPassErrorInvalidViewportRectSize,
		},
		{
			name:     "NaN x",
			vp:       gputypes.Viewport{X: float32(math.NaN()), Width: 10, Height: 10, MaxDepth: 1},
			wantKind: RenderPassErrorInvalidViewportRectPosition,
		},
		{
			name:     "minDepth above maxDepth",
			vp:       gputypes.Viewport{Width: 10, Height: 10, MinDepth: 0.9, MaxDepth: 0.1},
			wantKind: RenderPassErrorInvalidViewportDepth,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoder, pass, rec := newViewportPass(t, gputypes.Limits{})
			pass.SetViewport(tt.vp)

			if len(rec.viewports) != 0 {
				t.Errorf("invalid viewport reached the HAL pass: %+v", rec.viewports)
			}
			var rpve *RenderPassValidationError
			if !errors.As(encoder.Error(), &rpve) {
				t.Fatalf("deferred error = %v, want *RenderPassValidationError", encoder.Error())
			}
			if rpve.Kind != tt.wantKind {
				t.Errorf("kind = %v, want %v (%v)", rpve.Kind, tt.wantKind, rpve)
			}
		})
	}
}

// TestRenderPassValidationError_ViewportMessages checks the wording of the
// three viewport kinds and, for the position kind, both forms: with a known
// limit and with the finite-only form of an unpopulated one.
func TestRenderPassValidationError_ViewportMessages(t *testing.T) {
	vp := gputypes.Viewport{X: -1, Y: 2, Width: 30, Height: 40, MinDepth: 0.1, MaxDepth: 0.2}
	tests := []struct {
		name     string
		err      *RenderPassValidationError
		contains string
	}{
		{
			name:     "size",
			err:      &RenderPassValidationError{Kind: RenderPassErrorInvalidViewportRectSize, Label: "p", Viewport: vp, Limit: 8192},
			contains: "maxTextureDimension2D 8192",
		},
		{
			name:     "size without a limit",
			err:      &RenderPassValidationError{Kind: RenderPassErrorInvalidViewportRectSize, Label: "p", Viewport: vp},
			contains: "negative or not finite",
		},
		{
			name:     "position with a known limit",
			err:      &RenderPassValidationError{Kind: RenderPassErrorInvalidViewportRectPosition, Label: "p", Viewport: vp, ViewportMin: -16384, ViewportMax: 16383},
			contains: "16383",
		},
		{
			name:     "position without a limit",
			err:      &RenderPassValidationError{Kind: RenderPassErrorInvalidViewportRectPosition, Label: "p", Viewport: vp},
			contains: "not finite",
		},
		{
			name:     "depth",
			err:      &RenderPassValidationError{Kind: RenderPassErrorInvalidViewportDepth, Label: "p", Viewport: vp},
			contains: "minDepth",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := tt.err.Error()
			if !containsString(msg, tt.contains) {
				t.Errorf("message %q does not contain %q", msg, tt.contains)
			}
		})
	}
}
