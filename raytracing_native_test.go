//go:build !rust && !(js && wasm)

package wgpu

import (
	"encoding/binary"
	"errors"
	"math"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/internal/core"

	_ "github.com/gogpu/wgpu/internal/hal/allbackends"
)

// newRayTracingTestDevice returns a device on an adapter that offers
// FeatureRayQuery, skipping the test if there is none. The software backend
// does not offer it (its SPIR-V interpreter cannot run ray queries), so this
// needs a GPU with ray query support. If rayQuery is true the device enables
// FeatureRayQuery.
func newRayTracingTestDevice(t *testing.T, rayQuery bool) *Device {
	t.Helper()
	instance, err := CreateInstance(nil)
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	t.Cleanup(instance.Release)
	var adapter *Adapter
	for _, candidate := range instance.EnumerateAdapters() {
		if adapter == nil && candidate.Features().Contains(gputypes.FeatureRayQuery) {
			adapter = candidate
			continue
		}
		candidate.Release()
	}
	if adapter == nil {
		t.Skip("no adapter offers FeatureRayQuery")
	}
	t.Cleanup(adapter.Release)
	var features gputypes.Features
	if rayQuery {
		features = gputypes.Features(FeatureRayQuery)
	}
	device, err := adapter.RequestDevice(&DeviceDescriptor{RequiredFeatures: features})
	if err != nil {
		t.Fatalf("RequestDevice: %v", err)
	}
	t.Cleanup(device.Release)
	return device
}

func f32Bytes(values ...float32) []byte {
	out := make([]byte, 4*len(values))
	for i, v := range values {
		binary.LittleEndian.PutUint32(out[4*i:], math.Float32bits(v))
	}
	return out
}

// rtTriangle is a one-triangle BLAS with the buffers it is built from.
type rtTriangle struct {
	blas     *Blas
	size     gputypes.BlasTriangleGeometrySizeDescriptor
	vertices *Buffer
}

func newRTTriangle(t *testing.T, device *Device, vertexUsage gputypes.BufferUsage) *rtTriangle {
	t.Helper()
	data := f32Bytes(-1, -1, 0, 1, -1, 0, 0, 1, 0)
	vertices, err := device.CreateBuffer(&BufferDescriptor{
		Label: "rt-vertices",
		Size:  uint64(len(data)),
		Usage: vertexUsage | BufferUsageCopyDst,
	})
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	t.Cleanup(vertices.Release)
	if err := device.Queue().WriteBuffer(vertices, 0, data); err != nil {
		t.Fatalf("WriteBuffer: %v", err)
	}

	size := gputypes.BlasTriangleGeometrySizeDescriptor{
		VertexFormat: gputypes.VertexFormatFloat32x3,
		VertexCount:  3,
		Flags:        ASGeometryFlagOpaque,
	}
	blas, err := device.CreateBlas(&gputypes.CreateBlasDescriptor{Label: "rt-blas", Flags: ASFlagPreferFastTrace},
		gputypes.BlasGeometrySizeDescriptors{Triangles: []gputypes.BlasTriangleGeometrySizeDescriptor{size}})
	if err != nil {
		t.Fatalf("CreateBlas: %v", err)
	}
	t.Cleanup(blas.Release)
	return &rtTriangle{blas: blas, size: size, vertices: vertices}
}

func (tri *rtTriangle) buildEntry() BlasBuildEntry {
	return BlasBuildEntry{
		Blas: tri.blas,
		Geometry: BlasGeometries{Triangles: []BlasTriangleGeometry{{
			Size:         &tri.size,
			VertexBuffer: tri.vertices,
			VertexStride: 12,
		}}},
	}
}

func newRTTlas(t *testing.T, device *Device, maxInstances uint32) *Tlas {
	t.Helper()
	tlas, err := device.CreateTlas(&gputypes.CreateTlasDescriptor{Label: "rt-tlas", MaxInstances: maxInstances})
	if err != nil {
		t.Fatalf("CreateTlas: %v", err)
	}
	t.Cleanup(tlas.Release)
	return tlas
}

var identity3x4 = [12]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0}

// buildAndSubmit records one BuildAccelerationStructures call and submits it.
func buildAndSubmit(t *testing.T, device *Device, blas []BlasBuildEntry, tlas []*Tlas) error {
	t.Helper()
	encoder, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	encoder.BuildAccelerationStructures(blas, tlas)
	cmd, err := encoder.Finish()
	if err != nil {
		return err
	}
	if _, err := device.Queue().Submit(cmd); err != nil {
		cmd.Release()
		return err
	}
	return nil
}

func TestCreateAccelerationStructures_RequireFeature(t *testing.T) {
	device := newRayTracingTestDevice(t, false)

	_, err := device.CreateBlas(&gputypes.CreateBlasDescriptor{}, gputypes.BlasGeometrySizeDescriptors{})
	var blasErr *core.CreateBlasError
	if !errors.As(err, &blasErr) || blasErr.Kind != core.CreateBlasErrorMissingFeature {
		t.Errorf("CreateBlas without FeatureRayQuery: got %v, want CreateBlasErrorMissingFeature", err)
	}
	_, err = device.CreateTlas(&gputypes.CreateTlasDescriptor{MaxInstances: 1})
	var tlasErr *core.CreateTlasError
	if !errors.As(err, &tlasErr) || tlasErr.Kind != core.CreateTlasErrorMissingFeature {
		t.Errorf("CreateTlas without FeatureRayQuery: got %v, want CreateTlasErrorMissingFeature", err)
	}
}

func TestCreateBlas_Validation(t *testing.T) {
	device := newRayTracingTestDevice(t, true)
	u32 := gputypes.IndexFormatUint32
	three := uint32(3)
	limits := device.Limits()

	tests := []struct {
		name  string
		sizes gputypes.BlasGeometrySizeDescriptors
		want  core.CreateBlasErrorKind
	}{
		{
			name: "index count without format",
			sizes: gputypes.BlasGeometrySizeDescriptors{Triangles: []gputypes.BlasTriangleGeometrySizeDescriptor{{
				VertexFormat: gputypes.VertexFormatFloat32x3, VertexCount: 3, IndexCount: &three,
			}}},
			want: core.CreateBlasErrorMissingIndexData,
		},
		{
			name: "index format without count",
			sizes: gputypes.BlasGeometrySizeDescriptors{Triangles: []gputypes.BlasTriangleGeometrySizeDescriptor{{
				VertexFormat: gputypes.VertexFormatFloat32x3, VertexCount: 3, IndexFormat: &u32,
			}}},
			want: core.CreateBlasErrorMissingIndexData,
		},
		{
			name: "vertex format",
			sizes: gputypes.BlasGeometrySizeDescriptors{Triangles: []gputypes.BlasTriangleGeometrySizeDescriptor{{
				VertexFormat: gputypes.VertexFormatFloat32x4, VertexCount: 3,
			}}},
			want: core.CreateBlasErrorInvalidVertexFormat,
		},
		{
			name: "too many primitives",
			sizes: gputypes.BlasGeometrySizeDescriptors{AABBs: []gputypes.BlasAABBGeometrySizeDescriptor{{
				PrimitiveCount: limits.MaxBlasPrimitiveCount + 1,
			}}},
			want: core.CreateBlasErrorTooManyPrimitives,
		},
		{
			name: "too many geometries",
			sizes: gputypes.BlasGeometrySizeDescriptors{
				AABBs: make([]gputypes.BlasAABBGeometrySizeDescriptor, limits.MaxBlasGeometryCount+1),
			},
			want: core.CreateBlasErrorTooManyGeometries,
		},
		{
			name: "triangles and AABBs",
			sizes: gputypes.BlasGeometrySizeDescriptors{
				Triangles: []gputypes.BlasTriangleGeometrySizeDescriptor{{VertexFormat: gputypes.VertexFormatFloat32x3, VertexCount: 3}},
				AABBs:     []gputypes.BlasAABBGeometrySizeDescriptor{{PrimitiveCount: 1}},
			},
			want: core.CreateBlasErrorGeometryKind,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blas, err := device.CreateBlas(&gputypes.CreateBlasDescriptor{}, tt.sizes)
			if err == nil {
				blas.Release()
			}
			var blasErr *core.CreateBlasError
			if !errors.As(err, &blasErr) || blasErr.Kind != tt.want {
				t.Fatalf("got %v, want kind %d", err, tt.want)
			}
		})
	}
}

func TestCreateTlas_Validation(t *testing.T) {
	device := newRayTracingTestDevice(t, true)

	_, err := device.CreateTlas(&gputypes.CreateTlasDescriptor{MaxInstances: device.Limits().MaxTlasInstanceCount + 1})
	var tlasErr *core.CreateTlasError
	if !errors.As(err, &tlasErr) || tlasErr.Kind != core.CreateTlasErrorTooManyInstances {
		t.Errorf("too many instances: got %v", err)
	}
	_, err = device.CreateTlas(&gputypes.CreateTlasDescriptor{MaxInstances: 1, Flags: ASFlagUseTransform})
	if !errors.As(err, &tlasErr) || tlasErr.Kind != core.CreateTlasErrorDisallowedFlag {
		t.Errorf("UseTransform: got %v", err)
	}

	tlas := newRTTlas(t, device, 4)
	if got := tlas.MaxInstances(); got != 4 {
		t.Errorf("MaxInstances = %d, want 4", got)
	}
	if got := len(tlas.Instances()); got != 4 {
		t.Errorf("len(Instances) = %d, want 4", got)
	}
	if tlas.InstancesMut(2, 5) != nil {
		t.Error("InstancesMut(2, 5) should be nil for 4 slots")
	}
}

func TestBuildAccelerationStructures_SubmitAssignsBuildIndices(t *testing.T) {
	device := newRayTracingTestDevice(t, true)
	tri := newRTTriangle(t, device, BufferUsageBlasInput)
	tlas := newRTTlas(t, device, 2)
	tlas.SetInstance(1, NewTlasInstance(tri.blas, identity3x4, 7, 0xff))

	if handle, ok := tri.blas.Handle(); !ok || handle == 0 {
		t.Errorf("Blas.Handle() = %d, %v; want a non-zero address", handle, ok)
	}

	if err := buildAndSubmit(t, device, []BlasBuildEntry{tri.buildEntry()}, []*Tlas{tlas}); err != nil {
		t.Fatalf("build: %v", err)
	}
	blasIndex, tlasIndex := tri.blas.impl.Core().BuiltIndex(), tlas.impl.Core().BuiltIndex()
	if blasIndex == 0 || blasIndex != tlasIndex {
		t.Fatalf("built indices: BLAS %d, TLAS %d; want equal and non-zero", blasIndex, tlasIndex)
	}

	// Rebuilding only the TLAS gives it a newer index than the BLAS.
	if err := buildAndSubmit(t, device, nil, []*Tlas{tlas}); err != nil {
		t.Fatalf("TLAS rebuild: %v", err)
	}
	if got := tlas.impl.Core().BuiltIndex(); got <= blasIndex {
		t.Errorf("TLAS index after rebuild = %d, want > %d", got, blasIndex)
	}
}

func TestBuildAccelerationStructures_TlasWithUnbuiltBlas(t *testing.T) {
	device := newRayTracingTestDevice(t, true)
	tri := newRTTriangle(t, device, BufferUsageBlasInput)
	tlas := newRTTlas(t, device, 1)
	tlas.SetInstance(0, NewTlasInstance(tri.blas, identity3x4, 0, 0xff))

	err := buildAndSubmit(t, device, nil, []*Tlas{tlas})
	var asErr *core.ValidateAsActionsError
	if !errors.As(err, &asErr) || asErr.Kind != core.ValidateAsActionsErrorUsedUnbuiltBlas {
		t.Fatalf("got %v, want ValidateAsActionsErrorUsedUnbuiltBlas", err)
	}
}

func TestSubmit_TlasUseValidation(t *testing.T) {
	device := newRayTracingTestDevice(t, true)
	tri := newRTTriangle(t, device, BufferUsageBlasInput)
	tlas := newRTTlas(t, device, 1)
	tlas.SetInstance(0, NewTlasInstance(tri.blas, identity3x4, 0, 0xff))

	// useTlas records what binding a bind group with the TLAS does.
	useTlas := func() error {
		t.Helper()
		encoder, err := device.CreateCommandEncoder(nil)
		if err != nil {
			t.Fatalf("CreateCommandEncoder: %v", err)
		}
		encoder.impl.Core().RecordTlasUse(tlas.impl.Core())
		cmd, err := encoder.Finish()
		if err != nil {
			t.Fatalf("Finish: %v", err)
		}
		_, err = device.Queue().Submit(cmd)
		if err != nil {
			cmd.Release()
		}
		return err
	}
	wantKind := func(err error, kind core.ValidateAsActionsErrorKind) {
		t.Helper()
		var asErr *core.ValidateAsActionsError
		if !errors.As(err, &asErr) || asErr.Kind != kind {
			t.Fatalf("got %v, want ValidateAsActionsError kind %d", err, kind)
		}
	}

	wantKind(useTlas(), core.ValidateAsActionsErrorUsedUnbuiltTlas)

	if err := buildAndSubmit(t, device, []BlasBuildEntry{tri.buildEntry()}, []*Tlas{tlas}); err != nil {
		t.Fatalf("build: %v", err)
	}
	if err := useTlas(); err != nil {
		t.Fatalf("use after build: %v", err)
	}

	// Rebuilding the BLAS alone makes it newer than the TLAS built from it.
	if err := buildAndSubmit(t, device, []BlasBuildEntry{tri.buildEntry()}, nil); err != nil {
		t.Fatalf("BLAS rebuild: %v", err)
	}
	wantKind(useTlas(), core.ValidateAsActionsErrorBlasNewerThanTlas)
}

func TestBuildAccelerationStructures_Validation(t *testing.T) {
	device := newRayTracingTestDevice(t, true)

	wantBuildError := func(t *testing.T, err error, kind core.BuildAccelerationStructureErrorKind) {
		t.Helper()
		var buildErr *core.BuildAccelerationStructureError
		if !errors.As(err, &buildErr) || buildErr.Kind != kind {
			t.Fatalf("got %v, want BuildAccelerationStructureError kind %d", err, kind)
		}
	}

	t.Run("missing BLAS_INPUT usage", func(t *testing.T) {
		tri := newRTTriangle(t, device, BufferUsageVertex)
		err := buildAndSubmit(t, device, []BlasBuildEntry{tri.buildEntry()}, nil)
		wantBuildError(t, err, core.BuildASErrorMissingBufferUsage)
	})
	t.Run("more vertices than at creation", func(t *testing.T) {
		tri := newRTTriangle(t, device, BufferUsageBlasInput)
		entry := tri.buildEntry()
		size := tri.size
		size.VertexCount = 6
		entry.Geometry.Triangles[0].Size = &size
		wantBuildError(t, buildAndSubmit(t, device, []BlasBuildEntry{entry}, nil),
			core.BuildASErrorIncompatibleBlasVertexCount)
	})
	t.Run("vertex stride too small", func(t *testing.T) {
		tri := newRTTriangle(t, device, BufferUsageBlasInput)
		entry := tri.buildEntry()
		entry.Geometry.Triangles[0].VertexStride = 8
		wantBuildError(t, buildAndSubmit(t, device, []BlasBuildEntry{entry}, nil),
			core.BuildASErrorVertexStrideTooSmall)
	})
	t.Run("vertex buffer too small", func(t *testing.T) {
		tri := newRTTriangle(t, device, BufferUsageBlasInput)
		entry := tri.buildEntry()
		entry.Geometry.Triangles[0].FirstVertex = 1
		wantBuildError(t, buildAndSubmit(t, device, []BlasBuildEntry{entry}, nil),
			core.BuildASErrorInsufficientBufferSize)
	})
	t.Run("geometry kind mismatch", func(t *testing.T) {
		tri := newRTTriangle(t, device, BufferUsageBlasInput)
		entry := BlasBuildEntry{Blas: tri.blas, Geometry: BlasGeometries{AABBs: []BlasAabbGeometry{{
			Size:       &gputypes.BlasAABBGeometrySizeDescriptor{PrimitiveCount: 1},
			Stride:     24,
			AabbBuffer: tri.vertices,
		}}}}
		wantBuildError(t, buildAndSubmit(t, device, []BlasBuildEntry{entry}, nil),
			core.BuildASErrorBlasGeometryKindMismatch)
	})
	t.Run("custom data out of range", func(t *testing.T) {
		tri := newRTTriangle(t, device, BufferUsageBlasInput)
		tlas := newRTTlas(t, device, 1)
		tlas.SetInstance(0, NewTlasInstance(tri.blas, identity3x4, 1<<24, 0xff))
		wantBuildError(t, buildAndSubmit(t, device, []BlasBuildEntry{tri.buildEntry()}, []*Tlas{tlas}),
			core.BuildASErrorTlasInvalidCustomIndex)
	})
}

func TestTlas_InstanceSlotsKeepBlasAlive(t *testing.T) {
	device := newRayTracingTestDevice(t, true)
	tri := newRTTriangle(t, device, BufferUsageBlasInput)
	tlas := newRTTlas(t, device, 1)
	coreBlas := tri.blas.impl.Core()

	if err := buildAndSubmit(t, device, []BlasBuildEntry{tri.buildEntry()}, nil); err != nil {
		t.Fatalf("BLAS build: %v", err)
	}

	// The TLAS slot holds the BLAS after the application releases it, as a
	// Rust TlasInstance holds an Arc<Blas>.
	tlas.SetInstance(0, NewTlasInstance(tri.blas, identity3x4, 0, 0xff))
	tri.blas.Release()
	if coreBlas.IsDestroyed() {
		t.Fatal("BLAS destroyed while a TLAS instance slot references it")
	}
	if err := buildAndSubmit(t, device, nil, []*Tlas{tlas}); err != nil {
		t.Fatalf("TLAS build after BLAS release: %v", err)
	}

	// Emptying the slot leaves the built TLAS's dependency reference.
	tlas.SetInstance(0, nil)
	device.Poll(PollWait)
	if coreBlas.IsDestroyed() {
		t.Fatal("BLAS destroyed while a built TLAS depends on it")
	}

	// Releasing the TLAS drops the last reference once the GPU is done.
	tlas.Release()
	device.Poll(PollWait)
	device.Queue().Poll()
	if dq := device.TestDestroyQueue(); dq != nil {
		dq.Triage(device.Queue().Poll())
	}
	if !coreBlas.IsDestroyed() {
		t.Errorf("BLAS still alive after TLAS release (refcount %d)", coreBlas.Ref.RefCount())
	}
}

func TestBuildAccelerationStructures_DiscardDestroysTempBuffers(t *testing.T) {
	device := newRayTracingTestDevice(t, true)
	tri := newRTTriangle(t, device, BufferUsageBlasInput)

	encoder, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("CreateCommandEncoder: %v", err)
	}
	encoder.BuildAccelerationStructures([]BlasBuildEntry{tri.buildEntry()}, nil)
	if encoder.impl.Core().Error() != nil {
		t.Fatalf("build: %v", encoder.impl.Core().Error())
	}
	encoder.DiscardEncoding()
	if n := encoder.impl.Core().TempBufferCount(); n != 0 {
		t.Errorf("%d temp buffers left after DiscardEncoding", n)
	}
}

func TestBindGroup_AccelerationStructureLayoutMismatch(t *testing.T) {
	device := newRayTracingTestDevice(t, true)
	tlas := newRTTlas(t, device, 1)

	layout, err := device.CreateBindGroupLayout(&BindGroupLayoutDescriptor{
		Entries: []gputypes.BindGroupLayoutEntry{{
			Binding:    0,
			Visibility: gputypes.ShaderStageCompute,
			Buffer:     &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform},
		}},
	})
	if err != nil {
		t.Fatalf("CreateBindGroupLayout: %v", err)
	}
	defer layout.Release()

	_, err = device.CreateBindGroup(&BindGroupDescriptor{
		Layout:  layout,
		Entries: []BindGroupEntry{{Binding: 0, AccelerationStructure: tlas}},
	})
	if err == nil {
		t.Fatal("binding a TLAS to a uniform buffer slot succeeded")
	}
}
