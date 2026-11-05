//go:build integration && linux && !(js && wasm)

package gles

import (
	"runtime"
	"testing"

	"github.com/gogpu/wgpu/hal/gles/egl"
	"github.com/gogpu/wgpu/hal/gles/gl"
)

// TestSecondInstanceSurvivesFirstDestroy is the shared-EGLDisplay case of
// ADR-077 R: Mesa returns one EGLDisplay for one native display, so two
// Instances on the surfaceless platform share it. Destroying the first
// Instance terminated the display under the second, whose context then
// could not be made current. With the display registry, eglTerminate runs
// only with the last user.
func TestSecondInstanceSurvivesFirstDestroy(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	newInstance := func() *Instance {
		t.Helper()
		raw, err := Backend{}.CreateInstance(nil)
		if err != nil {
			t.Skipf("CreateInstance: %v", err)
		}
		inst := raw.(*Instance)
		if inst.context() == nil || inst.context().EGL() == nil {
			inst.Destroy()
			t.Skip("no EGL instance context (no display or driver)")
		}
		return inst
	}
	first := newInstance()
	second := newInstance()
	defer second.Destroy()

	if first.context().EGL().Display() != second.context().EGL().Display() {
		first.Destroy()
		t.Skip("this platform gave the two instances different EGLDisplays; the shared-display case does not arise")
	}

	first.Destroy()

	ctx := second.context()
	if err := ctx.EGL().MakeCurrent(); err != nil {
		t.Fatalf("second instance's context after the first instance was destroyed: %v", err)
	}
	version := ctx.GL().GetString(gl.VERSION)
	if err := ctx.EGL().ReleaseCurrent(); err != nil {
		t.Fatalf("ReleaseCurrent: %v", err)
	}
	if version == "" {
		t.Fatal("GL_VERSION is empty: the second instance's context is unusable")
	}
	if egl.GetCurrentContext() != egl.NoContext {
		t.Fatal("a context stayed current after ReleaseCurrent")
	}
}
