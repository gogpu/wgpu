module github.com/gogpu/wgpu

go 1.25

require (
	github.com/go-webgpu/goffi v0.3.7
	github.com/gogpu/naga v0.8.4
	golang.org/x/sys v0.39.0
)

// Use local naga for development
replace github.com/gogpu/naga => ../naga
