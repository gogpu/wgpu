package wgpu

import "testing"

var (
	_ func(*Instance, SurfaceTarget) (*Surface, error)       = (*Instance).CreateSurfaceFromTarget
	_ func(*Instance, SurfaceTargetUnsafe) (*Surface, error) = (*Instance).CreateSurfaceUnsafe
	_ SurfaceTarget                                          = HeadlessSurfaceTarget{}
	_ func(*Surface) ([]byte, error)                         = (*Surface).ReadPixels
)

func TestHeadlessSurfaceTargetContract(t *testing.T) {
	target, err := (HeadlessSurfaceTarget{}).SurfaceTarget()
	if err != nil {
		t.Fatalf("SurfaceTarget: %v", err)
	}
	if err := target.validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if target.kind != surfaceTargetHeadless {
		t.Fatalf("target kind = %v, want headless", target.kind)
	}
	if target.displayHandle != 0 || target.windowHandle != 0 {
		t.Fatalf("headless target carries handles: %+v", target)
	}
}
