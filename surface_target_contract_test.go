package wgpu

var (
	_ func(*Instance, SurfaceTarget) (*Surface, error)       = (*Instance).CreateSurfaceFromTarget
	_ func(*Instance, SurfaceTargetUnsafe) (*Surface, error) = (*Instance).CreateSurfaceUnsafe
)
