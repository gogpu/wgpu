//go:build !(js && wasm)

package hal_test

import (
	"encoding/binary"
	"math"
	"runtime"
	"testing"

	"github.com/gogpu/wgpu/hal"
)

// benchRTSink prevents the compiler from optimizing away benchmark results.
var benchRTSink any

// BenchmarkTlasInstancePacking verifies that TlasInstance -> 64-byte
// conversion is allocation-free on the hot path. Each backend implements
// TlasInstanceToBytes the same way: write 12 floats + bit-packed fields
// into a [64]byte. The noop backend returns nil, so this benchmark uses
// a standalone packing function that matches the real backends.
func BenchmarkTlasInstancePacking(b *testing.B) {
	b.ReportAllocs()

	instance := hal.TlasInstance{
		Transform: [12]float32{
			1, 0, 0, 5,
			0, 1, 0, 10,
			0, 0, 1, 15,
		},
		CustomData:                     42,
		Mask:                           0xFF,
		BlasAddress:                    0xDEADBEEF,
		ShaderBindingTableRecordOffset: 7,
	}

	// Pre-allocate the output buffer to simulate the zero-alloc path
	// that a real backend would use with a pooled or stack-allocated buffer.
	var buf [64]byte

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		packTlasInstanceInto(&buf, &instance)
	}
	benchRTSink = buf
}

// BenchmarkAccelerationStructureBarrier verifies that AS barrier
// construction is allocation-free (value type, no heap escape).
func BenchmarkAccelerationStructureBarrier(b *testing.B) {
	b.ReportAllocs()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		barrier := hal.AccelerationStructureBarrier{
			Usage: hal.AccelerationStructureUsageTransition{
				OldUsage: hal.AccelerationStructureUsesBuildOutput,
				NewUsage: hal.AccelerationStructureUsesShaderInput,
			},
		}
		benchRTSink = barrier
	}
}

// BenchmarkTlasInstanceRoundtrip measures pack + verify for a single
// instance, ensuring the full encode path has no allocations when using
// a pre-allocated buffer.
func BenchmarkTlasInstanceRoundtrip(b *testing.B) {
	b.ReportAllocs()

	instance := hal.TlasInstance{
		Transform: [12]float32{
			1, 0, 0, 100,
			0, 1, 0, 200,
			0, 0, 1, 300,
		},
		CustomData:                     0x00ABCDEF,
		Mask:                           0x42,
		BlasAddress:                    0x1234567890ABCDEF,
		ShaderBindingTableRecordOffset: 0x00000FFF,
	}

	var buf [64]byte

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		packTlasInstanceInto(&buf, &instance)

		// Verify BLAS address (bytes 56-63) to prevent dead code elimination.
		addr := binary.LittleEndian.Uint64(buf[56:])
		if addr != instance.BlasAddress {
			b.Fatal("BLAS address mismatch")
		}
	}
	runtime.KeepAlive(buf)
}

// BenchmarkBuildSizesQuery measures the overhead of creating a
// GetAccelerationStructureBuildSizesDescriptor (a descriptor allocation).
func BenchmarkBuildSizesQuery(b *testing.B) {
	b.ReportAllocs()

	triangles := []hal.AccelerationStructureTriangles{
		{VertexCount: 1000, VertexStride: 12},
		{VertexCount: 500, VertexStride: 12},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		desc := &hal.GetAccelerationStructureBuildSizesDescriptor{
			Entries: &hal.AccelerationStructureEntries{
				Triangles: triangles,
			},
		}
		benchRTSink = desc
	}
}

// ---------------------------------------------------------------------------
// packTlasInstanceInto packs a TlasInstance into a pre-allocated [64]byte.
// This matches the layout used by all 4 backends (Vulkan, DX12, Metal,
// Software) but writes into a caller-owned buffer for zero allocations.
// ---------------------------------------------------------------------------
func packTlasInstanceInto(buf *[64]byte, instance *hal.TlasInstance) {
	const maxU24 = (1 << 24) - 1

	for i, v := range instance.Transform {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}

	customAndMask := (instance.CustomData & maxU24) | (uint32(instance.Mask) << 24)
	binary.LittleEndian.PutUint32(buf[48:], customAndMask)

	sbtAndFlags := instance.ShaderBindingTableRecordOffset & maxU24
	binary.LittleEndian.PutUint32(buf[52:], sbtAndFlags)

	binary.LittleEndian.PutUint64(buf[56:], instance.BlasAddress)
}
