// Ray tracing headless verification — proves RT pipeline correctness on software backend.
//
// Creates one triangle, builds BLAS, traces one ray → verifies hit.
// Then renders a small image (160x120) to PNG for visual confirmation.
//
// No GPU required — runs entirely on CPU via software BVH.
//
// Usage:
//
//	go run ./examples/raytracing-headless/
package main

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/software"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	fmt.Println("=== RT Verification (Software BVH) ===")

	dev, queue, cleanup, err := createSoftwareDevice()
	if err != nil {
		return err
	}
	defer cleanup()

	blas, bvh, err := buildTriangleBLAS(dev, queue)
	if err != nil {
		return err
	}
	defer dev.DestroyAccelerationStructure(blas)
	fmt.Printf("BVH built: %d nodes\n", bvh.NodeCount())

	if err := verifySingleRay(bvh); err != nil {
		return err
	}

	return renderImage(bvh)
}

func buildTriangleBLAS(dev hal.Device, queue hal.Queue) (hal.AccelerationStructure, *software.BVHNode, error) {
	verts := [][3]float32{{-1, 0, -3}, {1, 0, -3}, {0, 2, -3}}
	vertexData := vertexBytes(verts)

	vbuf, err := dev.CreateBuffer(&hal.BufferDescriptor{
		Label: "tri-verts",
		Size:  uint64(len(vertexData)),
		Usage: gputypes.BufferUsageVertex | gputypes.BufferUsageBlasInput,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("vertex buffer: %w", err)
	}
	defer dev.DestroyBuffer(vbuf)
	_ = queue.WriteBuffer(vbuf, 0, vertexData)

	blas, err := dev.CreateAccelerationStructure(&hal.AccelerationStructureDescriptor{
		Label:  "tri-blas",
		Size:   4096,
		Format: hal.AccelerationStructureFormatBottomLevel,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("create BLAS: %w", err)
	}

	enc, err := dev.CreateCommandEncoder(&hal.CommandEncoderDescriptor{Label: "rt"})
	if err != nil {
		return nil, nil, fmt.Errorf("encoder: %w", err)
	}
	if err := enc.BeginEncoding("rt"); err != nil {
		return nil, nil, fmt.Errorf("begin: %w", err)
	}
	enc.BuildAccelerationStructures([]hal.BuildAccelerationStructureDescriptor{{
		Entries: &hal.AccelerationStructureEntries{
			Triangles: []hal.AccelerationStructureTriangles{{
				VertexBuffer: vbuf,
				VertexFormat: gputypes.VertexFormatFloat32x3,
				VertexCount:  3,
				VertexStride: 12,
			}},
		},
		Mode:                             hal.AccelerationStructureBuildModeBuild,
		DestinationAccelerationStructure: blas,
	}})
	cb, err := enc.EndEncoding()
	if err != nil {
		return nil, nil, fmt.Errorf("end: %w", err)
	}
	_, _ = queue.Submit([]hal.CommandBuffer{cb})

	swBlas := blas.(*software.AccelerationStructure)
	bvh := swBlas.BVH()
	if bvh == nil {
		return nil, nil, fmt.Errorf("BVH is nil — build failed")
	}
	return blas, bvh, nil
}

func verifySingleRay(bvh *software.BVHNode) error {
	origin := [3]float32{0, 1, 0}

	hit, t, _, _ := software.TraverseBVH(bvh, origin, [3]float32{0, 0, -1}, 100)
	if !hit {
		return fmt.Errorf("FAIL: ray should hit triangle at z=-3")
	}
	if math.Abs(float64(t-3.0)) > 0.01 {
		return fmt.Errorf("FAIL: hit t=%.4f, expected ~3.0", t)
	}
	fmt.Printf("Single ray: HIT at t=%.4f ✓\n", t)

	miss, _, _, _ := software.TraverseBVH(bvh, origin, [3]float32{0, 0, 1}, 100) //nolint:dogsled // only need hit bool
	if miss {
		return fmt.Errorf("FAIL: ray in +Z should miss")
	}
	fmt.Println("Miss ray: NO HIT ✓")
	return nil
}

func renderImage(bvh *software.BVHNode) error {
	const w, h = 160, 120
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	fovRad := 60.0 * math.Pi / 180.0
	halfH := math.Tan(fovRad / 2.0)
	halfW := halfH * float64(w) / float64(h)
	camPos := [3]float32{0, 1, 3}
	hits := 0

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			u := (2.0*float64(x)/float64(w) - 1.0) * halfW
			v := (1.0 - 2.0*float64(y)/float64(h)) * halfH
			rd := norm([3]float32{float32(u), float32(v), -1})

			if ok, dist, _, _ := software.TraverseBVH(bvh, camPos, rd, 100); ok {
				hits++
				shade := clamp(1.0-dist/10.0, 0.15, 1.0)
				img.SetRGBA(x, y, color.RGBA{
					R: uint8(shade * 80),  					G: uint8(shade * 160), 					B: uint8(shade * 255), 					A: 255,
				})
			} else {
				ty := float64(y) / float64(h)
				img.SetRGBA(x, y, color.RGBA{
					R: uint8(100 + ty*80),  					G: uint8(180 + ty*40),  					B: 235,
					A: 255,
				})
			}
		}
	}

	fmt.Printf("Rendered: %dx%d, %d hits (%.1f%%)\n", w, h, hits, float64(hits)/float64(w*h)*100)

	outPath := "rt_output.png"
	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create output: %w", err)
	}
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		return fmt.Errorf("encode PNG: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close output: %w", err)
	}

	fmt.Printf("Output: %s\n", outPath)
	fmt.Println("=== ALL CHECKS PASSED ===")
	return nil
}

func createSoftwareDevice() (hal.Device, hal.Queue, func(), error) {
	backend := software.API{}
	inst, err := backend.CreateInstance(&hal.InstanceDescriptor{})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("instance: %w", err)
	}
	adapters := inst.EnumerateAdapters(nil)
	if len(adapters) == 0 {
		inst.Destroy()
		return nil, nil, nil, fmt.Errorf("no adapters")
	}
	od, err := adapters[0].Adapter.Open(0, gputypes.DefaultLimits())
	if err != nil {
		inst.Destroy()
		return nil, nil, nil, fmt.Errorf("open: %w", err)
	}
	return od.Device, od.Queue, func() { od.Device.Destroy(); inst.Destroy() }, nil
}

func vertexBytes(verts [][3]float32) []byte {
	buf := make([]byte, len(verts)*12)
	for i, v := range verts {
		off := i * 12
		binary.LittleEndian.PutUint32(buf[off:], math.Float32bits(v[0]))
		binary.LittleEndian.PutUint32(buf[off+4:], math.Float32bits(v[1]))
		binary.LittleEndian.PutUint32(buf[off+8:], math.Float32bits(v[2]))
	}
	return buf
}

func norm(v [3]float32) [3]float32 {
	l := float32(math.Sqrt(float64(v[0]*v[0] + v[1]*v[1] + v[2]*v[2])))
	if l == 0 {
		return v
	}
	return [3]float32{v[0] / l, v[1] / l, v[2] / l}
}

func clamp(v, lo, hi float32) float32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
