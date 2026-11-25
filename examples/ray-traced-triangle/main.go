// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

// Command ray-traced-triangle is a port of Rust wgpu's
// examples/features/src/ray_traced_triangle.
//
// It builds one BLAS holding a single triangle and a TLAS holding three
// instances of it. A compute shader traces one ray query per pixel into a
// storage texture, and a fullscreen blit copies that texture into the render
// target. The Rust example is windowed; this port renders one frame
// headlessly (like the Rust example's test mode) at 1024x768, writes a PNG,
// and compares it against the Rust reference screenshot.
//
// It needs an adapter with FeatureRayQuery (Vulkan only for now; see
// docs/RAY-TRACING.md).
//
// Usage:
//
//	GOGPU_GRAPHICS_API=vulkan go run ./examples/ray-traced-triangle [-o out.png]
package main

import (
	_ "embed"
	"flag"
	"fmt"
	"log"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
	"github.com/gogpu/wgpu/examples/internal/rtexample"

	_ "github.com/gogpu/wgpu/internal/hal/allbackends"
)

var (
	//go:embed shader.wgsl
	shaderWGSL string
	//go:embed blit.wgsl
	blitWGSL string
	//go:embed screenshot.png
	goldenPNG []byte
)

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

// Example holds the state the Rust Example struct holds.
type Example struct {
	tlas          *wgpu.Tlas
	computePipe   *wgpu.ComputePipeline
	blitPipe      *wgpu.RenderPipeline
	bindGroup     *wgpu.BindGroup
	blitBindGroup *wgpu.BindGroup
	// storage_texture.width()/height(); wgpu.Texture has no size accessors yet.
	width, height uint32
	animationTime float32

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
	out := flag.String("o", "ray-traced-triangle.png", "output PNG path")
	threshold := flag.Float64("threshold", rtexample.DefaultMeanThreshold, "max mean difference vs the Rust screenshot (negative disables)")
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

func initExample(config *rtexample.Target, device *wgpu.Device, queue *wgpu.Queue) (*Example, error) { //nolint:gocyclo,cyclop,funlen,maintidx // example code — intentionally sequential
	ex := &Example{}
	keep := func(r interface{ Release() }) { ex.release = append(ex.release, r) }
	fail := func(what string, err error) (*Example, error) {
		ex.Release()
		return nil, fmt.Errorf("%s: %w", what, err)
	}

	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{Label: "shader.wgsl", WGSL: shaderWGSL})
	if err != nil {
		return fail("create shader", err)
	}
	keep(shader)
	blitShader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{Label: "blit.wgsl", WGSL: blitWGSL})
	if err != nil {
		return fail("create blit shader", err)
	}
	keep(blitShader)

	bgl, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label: "bgl for shader.wgsl",
		Entries: []gputypes.BindGroupLayoutEntry{
			{
				Binding:    0,
				Visibility: gputypes.ShaderStageCompute,
				Buffer:     &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform},
			},
			{
				Binding:    1,
				Visibility: gputypes.ShaderStageCompute,
				StorageTexture: &gputypes.StorageTextureBindingLayout{
					Access:        gputypes.StorageTextureAccessWriteOnly,
					Format:        gputypes.TextureFormatRGBA8Unorm,
					ViewDimension: gputypes.TextureViewDimension2D,
				},
			},
			{
				Binding:               2,
				Visibility:            gputypes.ShaderStageCompute,
				AccelerationStructure: &gputypes.AccelerationStructureBindingLayout{VertexReturn: false},
			},
		},
	})
	if err != nil {
		return fail("create bind group layout", err)
	}
	keep(bgl)

	blitBGL, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label: "bgl for blit.wgsl",
		Entries: []gputypes.BindGroupLayoutEntry{
			{
				Binding:    0,
				Visibility: gputypes.ShaderStageVertex | gputypes.ShaderStageFragment,
				Texture: &gputypes.TextureBindingLayout{
					SampleType:    gputypes.TextureSampleTypeUnfilterableFloat,
					ViewDimension: gputypes.TextureViewDimension2D,
				},
			},
			{
				Binding:    1,
				Visibility: gputypes.ShaderStageVertex | gputypes.ShaderStageFragment,
				Sampler:    &gputypes.SamplerBindingLayout{Type: gputypes.SamplerBindingTypeNonFiltering},
			},
		},
	})
	if err != nil {
		return fail("create blit bind group layout", err)
	}
	keep(blitBGL)

	vertices := []float32{1.0, 1.0, 0.0, -1.0, 1.0, 0.0, 0.0, -1.0, 0.0}
	indices := []uint32{0, 1, 2}

	vertexBuffer, err := rtexample.CreateBufferInit(device, "vertex buffer", rtexample.F32Bytes(vertices), wgpu.BufferUsageBlasInput)
	if err != nil {
		return fail("create vertex buffer", err)
	}
	keep(vertexBuffer)
	indexBuffer, err := rtexample.CreateBufferInit(device, "index buffer", rtexample.U32Bytes(indices), wgpu.BufferUsageBlasInput)
	if err != nil {
		return fail("create index buffer", err)
	}
	keep(indexBuffer)

	indexFormat := gputypes.IndexFormatUint32
	indexCount := uint32(len(indices)) //nolint:gosec // constant-size mesh
	blasSizeDesc := gputypes.BlasTriangleGeometrySizeDescriptor{
		VertexFormat: gputypes.VertexFormatFloat32x3,
		// 3 coordinates per vertex
		VertexCount: uint32(len(vertices) / 3), //nolint:gosec // constant-size mesh
		IndexFormat: &indexFormat,
		IndexCount:  &indexCount,
		Flags:       gputypes.ASGeometryFlagOpaque,
	}

	blas, err := device.CreateBlas(
		&gputypes.CreateBlasDescriptor{
			Flags:      gputypes.ASFlagPreferFastTrace,
			UpdateMode: gputypes.AccelerationStructureUpdateModeBuild,
		},
		gputypes.BlasGeometrySizeDescriptors{
			Triangles: []gputypes.BlasTriangleGeometrySizeDescriptor{blasSizeDesc},
		},
	)
	if err != nil {
		return fail("create BLAS", err)
	}
	keep(blas)

	tlas, err := device.CreateTlas(&gputypes.CreateTlasDescriptor{
		MaxInstances: 3,
		Flags:        gputypes.ASFlagPreferFastTrace,
		UpdateMode:   gputypes.AccelerationStructureUpdateModeBuild,
	})
	if err != nil {
		return fail("create TLAS", err)
	}
	ex.tlas = tlas
	keep(tlas)

	tlas.SetInstance(0, wgpu.NewTlasInstance(blas,
		rtexample.Translation(rtexample.Vec3{0.0, 0.0, 0.0}).Rows3x4(), 0, 0xff))
	tlas.SetInstance(1, wgpu.NewTlasInstance(blas,
		rtexample.Translation(rtexample.Vec3{-1.0, -1.0, -2.0}).Rows3x4(), 0, 0xff))
	tlas.SetInstance(2, wgpu.NewTlasInstance(blas,
		rtexample.Translation(rtexample.Vec3{1.0, -1.0, -2.0}).Rows3x4(), 0, 0xff))

	view := rtexample.LookAtRH(rtexample.Vec3{0.0, 0.0, 2.5}, rtexample.Vec3{}, rtexample.Vec3{0, 1, 0})
	proj := rtexample.PerspectiveRH(rtexample.Radians(59.0), 1.0, 0.001, 1000.0)
	uniforms := Uniforms{ViewInverse: view.Inverse(), ProjInverse: proj.Inverse()}

	uniformBuffer, err := rtexample.CreateBufferInit(device, "", uniforms.bytes(), wgpu.BufferUsageUniform)
	if err != nil {
		return fail("create uniform buffer", err)
	}
	keep(uniformBuffer)

	encoder, err := device.CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{})
	if err != nil {
		return fail("create encoder", err)
	}

	firstIndex := uint32(0)
	encoder.BuildAccelerationStructures(
		[]wgpu.BlasBuildEntry{{
			Blas: blas,
			Geometry: wgpu.BlasGeometries{
				Triangles: []wgpu.BlasTriangleGeometry{{
					Size:         &blasSizeDesc,
					VertexBuffer: vertexBuffer,
					FirstVertex:  0,
					VertexStride: 3 * 4, // size_of::<[f32; 3]>()
					// in this case since one triangle gets no compression from an index buffer
					// `IndexBuffer` and `FirstIndex` could be nil.
					IndexBuffer: indexBuffer,
					FirstIndex:  &firstIndex,
				}},
			},
		}},
		[]*wgpu.Tlas{tlas},
	)

	cmd, err := encoder.Finish()
	if err != nil {
		return fail("finish build encoder", err)
	}
	if _, err := queue.Submit(cmd); err != nil {
		return fail("submit build", err)
	}

	storageTex, err := device.CreateTexture(&wgpu.TextureDescriptor{
		Size:          wgpu.Extent3D{Width: config.Width, Height: config.Height, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         gputypes.TextureUsageStorageBinding | gputypes.TextureUsageTextureBinding,
	})
	if err != nil {
		return fail("create storage texture", err)
	}
	ex.width, ex.height = config.Width, config.Height
	keep(storageTex)

	sampler, err := device.CreateSampler(&wgpu.SamplerDescriptor{
		AddressModeU: gputypes.AddressModeClampToEdge,
		AddressModeV: gputypes.AddressModeClampToEdge,
		AddressModeW: gputypes.AddressModeClampToEdge,
		MagFilter:    gputypes.FilterModeNearest,
		MinFilter:    gputypes.FilterModeNearest,
		MipmapFilter: gputypes.FilterModeNearest,
		LodMinClamp:  1.0,
		LodMaxClamp:  1.0,
		Anisotropy:   1,
	})
	if err != nil {
		return fail("create sampler", err)
	}
	keep(sampler)

	computeLayout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{
		Label:            "pipeline layout for shader.wgsl",
		BindGroupLayouts: []*wgpu.BindGroupLayout{bgl},
	})
	if err != nil {
		return fail("create compute pipeline layout", err)
	}
	keep(computeLayout)

	ex.computePipe, err = device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
		Label:      "pipeline for shader.wgsl",
		Layout:     computeLayout,
		Module:     shader,
		EntryPoint: "main",
	})
	if err != nil {
		return fail("create compute pipeline", err)
	}
	keep(ex.computePipe)

	blitLayout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{
		Label:            "pipeline layout for blit.wgsl",
		BindGroupLayouts: []*wgpu.BindGroupLayout{blitBGL},
	})
	if err != nil {
		return fail("create blit pipeline layout", err)
	}
	keep(blitLayout)

	ex.blitPipe, err = device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label:  "pipeline for blit.wgsl",
		Layout: blitLayout,
		Vertex: wgpu.VertexState{
			Module:     blitShader,
			EntryPoint: "vs_main",
		},
		Primitive:   gputypes.PrimitiveState{Topology: gputypes.PrimitiveTopologyTriangleList},
		Multisample: gputypes.DefaultMultisampleState(),
		Fragment: &wgpu.FragmentState{
			Module:     blitShader,
			EntryPoint: "fs_main",
			Targets: []gputypes.ColorTargetState{{
				Format:    config.Format,
				WriteMask: gputypes.ColorWriteMaskAll,
			}},
		},
	})
	if err != nil {
		return fail("create blit pipeline", err)
	}
	keep(ex.blitPipe)

	// The Rust example creates a separate view for each bind group.
	storageView, err := device.CreateTextureView(storageTex, nil)
	if err != nil {
		return fail("create storage view", err)
	}
	keep(storageView)
	blitView, err := device.CreateTextureView(storageTex, nil)
	if err != nil {
		return fail("create blit view", err)
	}
	keep(blitView)

	ex.bindGroup, err = device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Label:  "bind group for shader.wgsl",
		Layout: bgl,
		Entries: []wgpu.BindGroupEntry{
			{Binding: 0, Buffer: uniformBuffer},
			{Binding: 1, TextureView: storageView},
			{Binding: 2, AccelerationStructure: tlas},
		},
	})
	if err != nil {
		return fail("create bind group", err)
	}
	keep(ex.bindGroup)

	ex.blitBindGroup, err = device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Label:  "bind group for blit.wgsl",
		Layout: blitBGL,
		Entries: []wgpu.BindGroupEntry{
			{Binding: 0, TextureView: blitView},
			{Binding: 1, Sampler: sampler},
		},
	})
	if err != nil {
		return fail("create blit bind group", err)
	}
	keep(ex.blitBindGroup)

	return ex, nil
}

func (ex *Example) render(view *wgpu.TextureView, device *wgpu.Device, queue *wgpu.Queue) error {
	// Rust: AnimationTimer::time() returns 0.0 on its first call, which is
	// the only frame the test renders.
	inst := *ex.tlas.Instances()[0]
	inst.Transform = rtexample.RotationY(ex.animationTime).Rows3x4()
	ex.tlas.SetInstance(0, &inst)

	encoder, err := device.CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{})
	if err != nil {
		return fmt.Errorf("create encoder: %w", err)
	}

	encoder.BuildAccelerationStructures(nil, []*wgpu.Tlas{ex.tlas})

	cpass, err := encoder.BeginComputePass(&wgpu.ComputePassDescriptor{})
	if err != nil {
		return fmt.Errorf("begin compute pass: %w", err)
	}
	cpass.SetPipeline(ex.computePipe)
	cpass.SetBindGroup(0, ex.bindGroup, nil)
	cpass.Dispatch(ex.width/8, ex.height/8, 1)
	if err := cpass.End(); err != nil {
		return fmt.Errorf("end compute pass: %w", err)
	}

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
	rpass.SetPipeline(ex.blitPipe)
	rpass.SetBindGroup(0, ex.blitBindGroup, nil)
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
