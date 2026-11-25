// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

// Command ray-scene is a port of Rust wgpu's examples/features/src/ray_scene.
//
// Where the Rust example loads two OBJ models (Ferris and a cube + Suzanne
// scene), this port loads the Go gopher. It builds one multi-geometry BLAS
// per model and instances it in a TLAS that is rebuilt every frame. A
// fullscreen fragment shader casts one ray query per pixel and shades hits
// from per-geometry material data. The Rust example is windowed; this port
// renders one frame headlessly at 1024x768 (like the Rust example's test
// mode), writes a PNG, and compares it against screenshot.png. Since there is
// no window, the Rust resize handler is not ported.
//
// It needs an adapter with FeatureRayQuery (Vulkan only for now; see
// docs/RAY-TRACING.md).
//
// Usage:
//
//	GOGPU_GRAPHICS_API=vulkan go run ./examples/ray-scene [-o out.png]
package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"math"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
	"github.com/gogpu/wgpu/examples/internal/rtexample"

	_ "github.com/gogpu/wgpu/internal/hal/allbackends"
)

var (
	//go:embed shader.wgsl
	shaderWGSL string
	//go:embed models
	modelsFS embed.FS
	// Reference render of the gopher scene (Vulkan, RTX 5080).
	//go:embed screenshot.png
	goldenPNG []byte
)

// gopherYaw turns gopher.obj, which faces -X, towards the camera.
const gopherYaw = math.Pi / 2

// Same size as the Rust example test (ExampleTestParams width/height).
const (
	width  = 1024
	height = 768
)

// Uniforms mirrors the Rust struct and the WGSL Uniforms block.
type Uniforms struct {
	ViewInverse rtexample.Mat4
	ProjInverse rtexample.Mat4
}

func (u Uniforms) bytes() []byte {
	return u.ProjInverse.AppendBytes(u.ViewInverse.AppendBytes(nil))
}

// SceneComponents mirrors the Rust struct of the same name.
type SceneComponents struct {
	vertices                          *wgpu.Buffer
	indices                           *wgpu.Buffer
	geometries                        *wgpu.Buffer
	instances                         *wgpu.Buffer
	bottomLevelAccelerationStructures []*wgpu.Blas
	// centers holds each model's bounding-box center in model space, so
	// instances can be placed by their middle rather than their origin.
	centers []rtexample.Vec3
}

// Release frees the scene's GPU resources.
func (s *SceneComponents) Release() {
	for _, b := range s.bottomLevelAccelerationStructures {
		b.Release()
	}
	for _, b := range []*wgpu.Buffer{s.instances, s.geometries, s.indices, s.vertices} {
		if b != nil {
			b.Release()
		}
	}
}

// InstanceEntry mirrors the Rust #[repr(C)] InstanceEntry (16 bytes).
type InstanceEntry struct {
	FirstVertex   uint32
	FirstGeometry uint32
	LastGeometry  uint32
	Pad           uint32
}

// GeometryEntry mirrors the Rust #[repr(C)] GeometryEntry (48 bytes).
type GeometryEntry struct {
	FirstIndex uint32
	_          [3]uint32
	Material   Material
}

// podBytes is the bytemuck::cast_slice equivalent for the example's
// fixed-layout structs (little-endian hosts only, like the GPU upload path).
// u32 converts a scene index or count to uint32, panicking if it doesn't fit.
func u32(n int) uint32 {
	if n < 0 || n > math.MaxUint32 {
		panic(fmt.Sprintf("ray-scene: %d does not fit in uint32", n))
	}
	return uint32(n)
}

func podBytes[T any](v []T) []byte {
	if len(v) == 0 {
		return nil
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(&v[0])), len(v)*int(unsafe.Sizeof(v[0]))) //nolint:gosec // reinterprets a slice of plain-data structs
}

func uploadSceneComponents(device *wgpu.Device, queue *wgpu.Queue, scene *RawSceneComponents) (*SceneComponents, error) {
	geometryBufferContent := make([]GeometryEntry, 0, len(scene.geometries))
	for _, g := range scene.geometries {
		geometryBufferContent = append(geometryBufferContent, GeometryEntry{
			FirstIndex: u32(g.indices.start),
			Material:   g.material,
		})
	}

	instanceBufferContent := make([]InstanceEntry, 0, len(scene.instances))
	for _, inst := range scene.instances {
		instanceBufferContent = append(instanceBufferContent, InstanceEntry{
			FirstVertex:   u32(inst.vertices.start),
			FirstGeometry: u32(inst.geometries.start),
			LastGeometry:  u32(inst.geometries.end),
			Pad:           1, // the Rust example sets _pad: 1
		})
	}

	sc := &SceneComponents{}
	fail := func(what string, err error) (*SceneComponents, error) {
		sc.Release()
		return nil, fmt.Errorf("%s: %w", what, err)
	}
	var err error

	sc.vertices, err = rtexample.CreateBufferInit(device, "Vertices", podBytes(scene.vertices),
		wgpu.BufferUsageVertex|wgpu.BufferUsageStorage|wgpu.BufferUsageBlasInput)
	if err != nil {
		return fail("create vertex buffer", err)
	}
	sc.indices, err = rtexample.CreateBufferInit(device, "Indices", rtexample.U32Bytes(scene.indices),
		wgpu.BufferUsageIndex|wgpu.BufferUsageStorage|wgpu.BufferUsageBlasInput)
	if err != nil {
		return fail("create index buffer", err)
	}
	sc.geometries, err = rtexample.CreateBufferInit(device, "Geometries", podBytes(geometryBufferContent),
		wgpu.BufferUsageStorage)
	if err != nil {
		return fail("create geometry buffer", err)
	}
	sc.instances, err = rtexample.CreateBufferInit(device, "Instances", podBytes(instanceBufferContent),
		wgpu.BufferUsageStorage)
	if err != nil {
		return fail("create instance buffer", err)
	}

	indexFormat := gputypes.IndexFormatUint32
	sizeDescriptors := make([][]gputypes.BlasTriangleGeometrySizeDescriptor, 0, len(scene.instances))
	for _, inst := range scene.instances {
		sizeDesc := make([]gputypes.BlasTriangleGeometrySizeDescriptor, 0, inst.geometries.end-inst.geometries.start)
		for i := inst.geometries.start; i < inst.geometries.end; i++ {
			indexCount := u32(scene.geometries[i].indices.end - scene.geometries[i].indices.start)
			sizeDesc = append(sizeDesc, gputypes.BlasTriangleGeometrySizeDescriptor{
				VertexFormat: gputypes.VertexFormatFloat32x3,
				VertexCount:  u32(inst.vertices.end - inst.vertices.start),
				IndexFormat:  &indexFormat,
				IndexCount:   &indexCount,
				Flags:        gputypes.ASGeometryFlagOpaque,
			})
		}

		blas, err := device.CreateBlas(
			&gputypes.CreateBlasDescriptor{
				Flags:      gputypes.ASFlagPreferFastTrace,
				UpdateMode: gputypes.AccelerationStructureUpdateModeBuild,
			},
			gputypes.BlasGeometrySizeDescriptors{Triangles: sizeDesc},
		)
		if err != nil {
			return fail("create BLAS", err)
		}
		sizeDescriptors = append(sizeDescriptors, sizeDesc)
		sc.bottomLevelAccelerationStructures = append(sc.bottomLevelAccelerationStructures, blas)
	}

	buildEntries := make([]wgpu.BlasBuildEntry, 0, len(scene.instances))
	for i, inst := range scene.instances {
		sizeDesc := sizeDescriptors[i]
		triangleGeometries := make([]wgpu.BlasTriangleGeometry, 0, len(sizeDesc))
		for j := range sizeDesc {
			firstIndex := u32(scene.geometries[inst.geometries.start+j].indices.start)
			triangleGeometries = append(triangleGeometries, wgpu.BlasTriangleGeometry{
				Size:         &sizeDesc[j],
				VertexBuffer: sc.vertices,
				FirstVertex:  u32(inst.vertices.start),
				VertexStride: uint64(unsafe.Sizeof(Vertex{})),
				IndexBuffer:  sc.indices,
				FirstIndex:   &firstIndex,
			})
		}
		buildEntries = append(buildEntries, wgpu.BlasBuildEntry{
			Blas:     sc.bottomLevelAccelerationStructures[i],
			Geometry: wgpu.BlasGeometries{Triangles: triangleGeometries},
		})
	}

	encoder, err := device.CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{})
	if err != nil {
		return fail("create encoder", err)
	}

	encoder.BuildAccelerationStructures(buildEntries, nil)

	cmd, err := encoder.Finish()
	if err != nil {
		return fail("finish build encoder", err)
	}
	if _, err := queue.Submit(cmd); err != nil {
		return fail("submit build", err)
	}
	return sc, nil
}

func loadScene(device *wgpu.Device, queue *wgpu.Queue) (*SceneComponents, error) {
	models, err := fs.Sub(modelsFS, "models")
	if err != nil {
		return nil, err
	}
	scene, err := loadRawScene(models)
	if err != nil {
		return nil, fmt.Errorf("load models: %w", err)
	}
	sc, err := uploadSceneComponents(device, queue, scene)
	if err != nil {
		return nil, err
	}
	for _, inst := range scene.instances {
		sc.centers = append(sc.centers, boundsCenter(scene.vertices[inst.vertices.start:inst.vertices.end]))
	}
	return sc, nil
}

// boundsCenter returns the center of the axis-aligned bounding box of vs.
func boundsCenter(vs []Vertex) rtexample.Vec3 {
	if len(vs) == 0 {
		return rtexample.Vec3{}
	}
	lo, hi := vs[0].Pos, vs[0].Pos
	for _, v := range vs[1:] {
		for i := range 3 {
			lo[i] = min(lo[i], v.Pos[i])
			hi[i] = max(hi[i], v.Pos[i])
		}
	}
	return rtexample.Vec3{(lo[0] + hi[0]) / 2, (lo[1] + hi[1]) / 2, (lo[2] + hi[2]) / 2}
}

// Example holds the state the Rust Example struct holds.
type Example struct {
	uniforms        Uniforms
	uniformBuf      *wgpu.Buffer
	tlas            *wgpu.Tlas
	pipeline        *wgpu.RenderPipeline
	bindGroup       *wgpu.BindGroup
	sceneComponents *SceneComponents
	animationTime   float32

	// Kept only so they can be released; the Rust version relies on Drop.
	release []interface{ Release() }
}

func requiredFeatures() gputypes.Features {
	return gputypes.Features(gputypes.FeatureRayQuery)
}

func requiredLimits() gputypes.Limits {
	return wgpu.LimitsWithMinimumAccelerationStructureValues(gputypes.DefaultLimits())
}

func main() {
	out := flag.String("o", "ray-scene.png", "output PNG path")
	threshold := flag.Float64("threshold", rtexample.DefaultMeanThreshold, "max mean difference vs screenshot.png (negative disables)")
	flag.Parse()
	if err := run(*out, *threshold); err != nil {
		log.Fatalf("FAIL: %v", err)
	}
}

func run(out string, threshold float64) error {
	device, cleanup, err := rtexample.InitDevice(requiredFeatures(), requiredLimits())
	if err != nil {
		return err
	}
	defer cleanup()

	// The Rust example framework renders to an sRGB surface (Example::SRGB
	// defaults to true), and its test mode uses Rgba8UnormSrgb.
	target, err := rtexample.NewTarget(device, width, height, gputypes.TextureFormatRGBA8UnormSrgb)
	if err != nil {
		return err
	}
	defer target.Release()

	ex, err := initExample(target, device, device.Queue())
	if err != nil {
		return err
	}
	defer ex.Release()

	if err := ex.render(target.View, device, device.Queue()); err != nil {
		return err
	}

	img, err := target.Read(device)
	if err != nil {
		return err
	}
	if err := rtexample.WritePNG(out, img); err != nil {
		return err
	}
	if err := rtexample.CheckValidation(); err != nil {
		return err
	}
	if threshold >= 0 {
		return rtexample.ComparePNG(img, goldenPNG, threshold)
	}
	return nil
}

func initExample(config *rtexample.Target, device *wgpu.Device, queue *wgpu.Queue) (*Example, error) {
	const sideCount = 8

	ex := &Example{}
	keep := func(r interface{ Release() }) { ex.release = append(ex.release, r) }
	fail := func(what string, err error) (*Example, error) {
		ex.Release()
		return nil, fmt.Errorf("%s: %w", what, err)
	}

	sceneComponents, err := loadScene(device, queue)
	if err != nil {
		return fail("load scene", err)
	}
	ex.sceneComponents = sceneComponents
	keep(sceneComponents)

	view := rtexample.LookAtRH(rtexample.Vec3{0.0, 0.0, 2.5}, rtexample.Vec3{}, rtexample.Vec3{0, 1, 0})
	proj := rtexample.PerspectiveRH(rtexample.Radians(59.0), float32(config.Width)/float32(config.Height), 0.001, 1000.0)
	ex.uniforms = Uniforms{ViewInverse: view.Inverse(), ProjInverse: proj.Inverse()}

	ex.uniformBuf, err = rtexample.CreateBufferInit(device, "Uniform Buffer", ex.uniforms.bytes(),
		wgpu.BufferUsageUniform|wgpu.BufferUsageCopyDst)
	if err != nil {
		return fail("create uniform buffer", err)
	}
	keep(ex.uniformBuf)

	ex.tlas, err = device.CreateTlas(&gputypes.CreateTlasDescriptor{
		Flags:        gputypes.ASFlagPreferFastTrace,
		UpdateMode:   gputypes.AccelerationStructureUpdateModeBuild,
		MaxInstances: sideCount * sideCount,
	})
	if err != nil {
		return fail("create TLAS", err)
	}
	keep(ex.tlas)

	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{WGSL: shaderWGSL})
	if err != nil {
		return fail("create shader", err)
	}
	keep(shader)

	// Rust passes `layout: None` and then calls pipeline.get_bind_group_layout(0).
	// Native Go has no implicit pipeline layouts, so the layout the Rust
	// auto-derivation would produce is spelled out here: every binding is
	// only referenced by fs_main.
	storage := func(binding uint32) gputypes.BindGroupLayoutEntry {
		return gputypes.BindGroupLayoutEntry{
			Binding:    binding,
			Visibility: gputypes.ShaderStageFragment,
			Buffer:     &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeReadOnlyStorage},
		}
	}
	bindGroupLayout, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Entries: []gputypes.BindGroupLayoutEntry{
			{
				Binding:    0,
				Visibility: gputypes.ShaderStageFragment,
				Buffer:     &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform},
			},
			storage(1),
			storage(2),
			storage(3),
			storage(4),
			{
				Binding:               5,
				Visibility:            gputypes.ShaderStageFragment,
				AccelerationStructure: &gputypes.AccelerationStructureBindingLayout{},
			},
		},
	})
	if err != nil {
		return fail("create bind group layout", err)
	}
	keep(bindGroupLayout)

	pipelineLayout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{
		BindGroupLayouts: []*wgpu.BindGroupLayout{bindGroupLayout},
	})
	if err != nil {
		return fail("create pipeline layout", err)
	}
	keep(pipelineLayout)

	ex.pipeline, err = device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Layout: pipelineLayout,
		Vertex: wgpu.VertexState{
			Module:     shader,
			EntryPoint: "vs_main",
		},
		Fragment: &wgpu.FragmentState{
			Module:     shader,
			EntryPoint: "fs_main",
			Targets: []gputypes.ColorTargetState{{
				Format:    config.Format,
				WriteMask: gputypes.ColorWriteMaskAll,
			}},
		},
		Primitive:   gputypes.PrimitiveState{Topology: gputypes.PrimitiveTopologyTriangleList},
		Multisample: gputypes.DefaultMultisampleState(),
	})
	if err != nil {
		return fail("create render pipeline", err)
	}
	keep(ex.pipeline)

	ex.bindGroup, err = device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Layout: bindGroupLayout,
		Entries: []wgpu.BindGroupEntry{
			{Binding: 0, Buffer: ex.uniformBuf},
			{Binding: 5, AccelerationStructure: ex.tlas},
			{Binding: 1, Buffer: sceneComponents.vertices},
			{Binding: 2, Buffer: sceneComponents.indices},
			{Binding: 3, Buffer: sceneComponents.geometries},
			{Binding: 4, Buffer: sceneComponents.instances},
		},
	})
	if err != nil {
		return fail("create bind group", err)
	}
	keep(ex.bindGroup)

	return ex, nil
}

func (ex *Example) render(view *wgpu.TextureView, device *wgpu.Device, queue *wgpu.Queue) error {
	// scene update
	{
		const dist = 3.5
		const sideCount = 2

		// Rust: AnimationTimer::time() returns 0.0 on its first call, which
		// is the only frame the test renders.
		animTime := ex.animationTime
		blases := ex.sceneComponents.bottomLevelAccelerationStructures

		for x := range sideCount {
			for y := range sideCount {
				blasIndex := (x + y) % len(blases)

				fx := float32(x)/float32(sideCount-1)*2.0 - 1.0
				fy := float32(y)/float32(sideCount-1)*2.0 - 1.0

				// Unlike the Rust example, the model is first moved so its
				// bounding-box center sits at the origin, and turned a
				// quarter turn about Y so the gopher (which faces -X in
				// gopher.obj) faces the camera.
				c := ex.sceneComponents.centers[blasIndex]
				transform := rtexample.RotationTranslationEulerXYZ(
					animTime*0.5*0.342,
					animTime*0.5*0.254+gopherYaw,
					animTime*0.5*0.832+math.Pi,
					rtexample.Vec3{fx * dist, fy * dist, -14.0},
				).Mul(rtexample.Translation(rtexample.Vec3{-c[0], -c[1], -c[2]}))
				ex.tlas.SetInstance(x+y*sideCount, wgpu.NewTlasInstance(
					blases[blasIndex],
					transform.Rows3x4(),
					u32(blasIndex),
					0xff,
				))
			}
		}
	}

	encoder, err := device.CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{})
	if err != nil {
		return fmt.Errorf("create encoder: %w", err)
	}

	encoder.BuildAccelerationStructures(nil, []*wgpu.Tlas{ex.tlas})

	rpass, err := encoder.BeginRenderPass(&wgpu.RenderPassDescriptor{
		ColorAttachments: []wgpu.RenderPassColorAttachment{{
			View:       view,
			LoadOp:     gputypes.LoadOpClear,
			StoreOp:    gputypes.StoreOpStore,
			ClearValue: gputypes.Color{R: 0, G: 1, B: 0, A: 1}, // wgpu::Color::GREEN
		}},
	})
	if err != nil {
		return fmt.Errorf("begin render pass: %w", err)
	}
	rpass.SetPipeline(ex.pipeline)
	rpass.SetBindGroup(0, ex.bindGroup, nil)
	rpass.Draw(gputypes.DrawArgs{VertexCount: 3, InstanceCount: 1})
	if err := rpass.End(); err != nil {
		return fmt.Errorf("end render pass: %w", err)
	}

	cmd, err := encoder.Finish()
	if err != nil {
		return fmt.Errorf("finish encoder: %w", err)
	}
	if _, err := queue.Submit(cmd); err != nil {
		return fmt.Errorf("submit: %w", err)
	}
	return nil
}

// Release frees everything init created, in reverse creation order.
func (ex *Example) Release() {
	for i := len(ex.release) - 1; i >= 0; i-- {
		ex.release[i].Release()
	}
	ex.release = nil
}
