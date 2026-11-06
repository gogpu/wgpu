package shader

import (
	"math"

	"github.com/gogpu/gputypes"
)

// This file selects the mip level, array layer, cube face or depth slice a
// texture instruction reads, following the Vulkan specification's "Texel
// Filtering" chapter (level-of-detail operation, cube map face selection,
// array layer and mipmap selection). Every helper works on values; nothing
// here allocates per texel.

// LodClampNone is the WebGPU default LodMaxClamp (32): no upper clamp. The
// fallback samplers used when no sampler is bound set it; a Sampler's
// LodMaxClamp is otherwise taken literally, as the HAL descriptor's is.
const LodClampNone = 32

// imageOperands are the image operands of a sample instruction the
// interpreter honors.
type imageOperands struct {
	bias    float32
	lod     float32
	hasGrad bool
	dx, dy  Vec4
	offset  [3]int
}

// parseImageOperands reads the image operands of a sample instruction: ops
// starts at the operand mask, and the operand ids follow in the order of
// the mask bits (SPIR-V "Image Operands").
func (interp *interpreter) parseImageOperands(ops []uint32) imageOperands {
	var io imageOperands
	if len(ops) == 0 {
		return io
	}
	mask, ids := ops[0], ops[1:]
	next := 0
	value := func() (Value, bool) {
		if next >= len(ids) {
			return Value{}, false
		}
		v := interp.values[ids[next]]
		next++
		return v, true
	}
	if mask&ImageOperandBiasMask != 0 {
		if v, ok := value(); ok {
			io.bias = toFloat32(v)
		}
	}
	if mask&ImageOperandLodMask != 0 {
		if v, ok := value(); ok {
			io.lod = toFloat32(v)
		}
	}
	if mask&ImageOperandGradMask != 0 {
		dx, okx := value()
		dy, oky := value()
		if okx && oky {
			io.hasGrad = true
			io.dx, io.dy = texelComponents(dx), texelComponents(dy)
		}
	}
	if mask&(ImageOperandConstOffsetMask|ImageOperandOffsetMask) != 0 {
		if v, ok := value(); ok {
			io.offset = imageCoord3(v)
		}
	}
	return io
}

// lambda is the unclamped level of detail of a sample at coordinate c of
// tex: the Lod operand, or the one the Grad operands give, plus the Bias.
func (io *imageOperands) lambda(tex *Texture2D, c Vec4) float32 {
	l := io.lod
	if io.hasGrad {
		l = gradLod(tex, c, io.dx, io.dy)
	}
	return l + io.bias
}

// sampleCompare is OpImageSampleDrefImplicitLod and
// OpImageSampleDrefExplicitLod: the depth-compare sample of a sampled image
// with reference ref, an f32. A missing texture compares as 0.
func (interp *interpreter) sampleCompare(sampledImg, coord Value, ref float32, ops imageOperands) Value {
	tex, samp, ok := interp.sampledResources(sampledImg)
	if !ok {
		return ValFloat(0)
	}
	c := texelComponents(coord)
	cmp := depthCompare{ref: ref, fn: samp.Compare}
	return ValFloat(sampleCompareView(tex, samp, c, ops.lambda(tex, c), ops.offset, &cmp))
}

// gather is OpImageGather (operand is the component to gather) and
// OpImageDrefGather (operand is the reference depth): a vec4 of the four
// texels of the footprint. A missing texture gathers zeros.
func (interp *interpreter) gather(opcode uint16, sampledImg, coord, operand Value, ops imageOperands) Value {
	tex, samp, ok := interp.sampledResources(sampledImg)
	if !ok {
		return ValVec4(0, 0, 0, 0)
	}
	c := texelComponents(coord)
	if opcode == OpImageDrefGather {
		cmp := depthCompare{ref: toFloat32(operand), fn: samp.Compare}
		return ValVec4From(gatherView(tex, samp, c, ops.offset, 0, &cmp))
	}
	return ValVec4From(gatherView(tex, samp, c, ops.offset, toUint32(operand), nil))
}

// levelCount is the number of mip levels the view holds data for.
func (tex *Texture2D) levelCount() int { return max(len(tex.Levels), 1) }

// baseImages is the image count of the view's base level: its array layers,
// or the depth of a 3D view.
func (tex *Texture2D) baseImages() uint32 {
	if tex.Dimension == TexDim3D {
		return max(tex.Depth, 1)
	}
	return max(tex.Layers, 1)
}

// level returns mip level l of the view (0 is its base level), clamped to
// the levels it has.
func (tex *Texture2D) level(l int) TextureLevel {
	if len(tex.Levels) == 0 {
		return TextureLevel{Width: tex.Width, Height: tex.Height, Images: tex.baseImages(), Data: tex.Data}
	}
	return tex.Levels[clampInt(l, len(tex.Levels)-1)]
}

// imageIndex is the image an integer coordinate xyz addresses: its third
// component, the array layer or depth slice, for a 2D-array or 3D view, and
// the one image of any other view.
func (tex *Texture2D) imageIndex(xyz [3]int) int {
	switch tex.Dimension {
	case TexDim2DArray, TexDim3D:
		return xyz[2]
	}
	return 0
}

// texelSize is the storage size of one texel.
func (tex *Texture2D) texelSize() int {
	if tex.BytesPerPixel != 0 {
		return int(tex.BytesPerPixel)
	}
	if layout, ok := sampledLayout(tex); ok {
		return layout.texelStride()
	}
	return 4
}

// imageAt returns image z of level lv as a one-image Texture2D whose Data
// aliases the level's, or false when the level has no image z. A level
// whose data is short keeps what it has: texel reads past it return zero.
func (tex *Texture2D) imageAt(lv TextureLevel, z int) (Texture2D, bool) {
	if z < 0 || z >= int(max(lv.Images, 1)) {
		return Texture2D{}, false
	}
	size := int(lv.Width) * int(lv.Height) * tex.texelSize()
	off := min(z*size, len(lv.Data))
	end := min(off+size, len(lv.Data))
	return Texture2D{
		Width: lv.Width, Height: lv.Height, Data: lv.Data[off:end:end],
		Format: tex.Format, BytesPerPixel: tex.BytesPerPixel, Aspect: tex.Aspect,
	}, true
}

// sampleView samples the view at coordinate c (by dimension: u; u, v; u, v,
// layer; a cube direction and the cube index; u, v, w) with the
// unclamped level of detail lambda (Vulkan λ', bias included).
//
// lambda is clamped to the sampler's [LodMinClamp, LodMaxClamp]; a positive
// result selects the minification filter, otherwise the magnification
// filter. The level is then clamped to the view's levels and picked by the
// mipmap filter: nearest takes ceil(d+0.5)-1, linear blends the two levels
// around d.
func sampleView(tex *Texture2D, samp *Sampler, c Vec4, lambda float32, offset [3]int) Vec4 {
	filter, l, frac := selectLevel(tex, samp, lambda)
	a := sampleLevel(tex, samp, filter, l, c, offset)
	if frac == 0 {
		return a
	}
	b := sampleLevel(tex, samp, filter, l+1, c, offset)
	for i := range a {
		a[i] += (b[i] - a[i]) * frac
	}
	return a
}

// selectLevel applies the level-of-detail steps sampleView describes to
// lambda: it returns the filter, the mip level l to sample and the weight
// frac of level l+1 to blend in (0 unless the mipmap filter is linear).
func selectLevel(tex *Texture2D, samp *Sampler, lambda float32) (filter uint32, l int, frac float32) {
	if lambda != lambda { // NaN
		lambda = 0
	}
	lambda = min(max(lambda, samp.LodMinClamp), samp.LodMaxClamp)
	filter = samp.MagFilter
	if lambda > 0 {
		filter = samp.MinFilter
	}
	q := float32(tex.levelCount() - 1)
	d := min(max(lambda, 0), q)
	if samp.MipmapFilter != FilterLinear {
		return filter, int(math.Ceil(float64(d)+0.5)) - 1, 0
	}
	lo := float32(math.Floor(float64(d)))
	return filter, int(lo), d - lo
}

// sampleLevel samples mip level l of the view with filter.
func sampleLevel(tex *Texture2D, samp *Sampler, filter uint32, l int, c Vec4, offset [3]int) Vec4 {
	lv := tex.level(l)
	if lv.Width == 0 || lv.Height == 0 {
		return Vec4{}
	}
	if tex.Dimension == TexDim3D {
		return sample3D(tex, lv, samp, filter, c, offset)
	}
	lc := levelImage(tex, samp, lv, c, offset)
	ou := float32(lc.offset[0]) / float32(lv.Width)
	ov := float32(lc.offset[1]) / float32(lv.Height)
	return filter2D(&lc.img, filter, applyWrapMode(lc.u+ou, lc.wrapU), applyWrapMode(lc.v+ov, lc.wrapV))
}

// levelCoord is the image of a mip level a coordinate addresses and the
// coordinate in it.
type levelCoord struct {
	img          Texture2D // the one image: array layer or cube face
	u, v         float32   // the coordinate in the image, not yet wrapped
	offset       [2]int    // the texel offset to add to u and v
	wrapU, wrapV uint32    // the address modes of u and v
}

// levelImage returns the levelCoord of coordinate c in level lv of a 1D, 2D,
// 2D-array, cube or cube-array view: the array layer or cube face c selects,
// and the coordinate in it. A cube face is addressed on its own, clamped at
// its edges; a 1D view is one row, read at its center.
func levelImage(tex *Texture2D, samp *Sampler, lv TextureLevel, c Vec4, offset [3]int) levelCoord {
	switch tex.Dimension {
	case TexDimCube, TexDimCubeArray:
		face, s, t := cubeFace(c)
		if tex.Dimension == TexDimCubeArray {
			face += 6 * arrayIndex(c[3], max(lv.Images, 1)/6)
		}
		img, _ := tex.imageAt(lv, face)
		return levelCoord{img: img, u: s, v: t, wrapU: WrapClampToEdge, wrapV: WrapClampToEdge}
	case TexDim1D:
		img, _ := tex.imageAt(lv, 0)
		return levelCoord{img: img, u: c[0], v: 0.5, offset: [2]int{offset[0], 0}, wrapU: samp.WrapU, wrapV: WrapClampToEdge}
	}
	z := 0
	if tex.Dimension == TexDim2DArray {
		z = arrayIndex(c[2], lv.Images)
	}
	img, _ := tex.imageAt(lv, z)
	return levelCoord{img: img, u: c[0], v: c[1], offset: [2]int{offset[0], offset[1]}, wrapU: samp.WrapU, wrapV: samp.WrapV}
}

// footprint is the 2x2 texel footprint of a linear filter (Vulkan "Texel
// Filtering"): columns x0 and x1 and rows y0 and y1, each wrapped by its
// address mode, and the weights fx of x1 and fy of y1.
type footprint struct {
	x0, x1, y0, y1 int
	fx, fy         float32
}

// linearFootprint returns the footprint of lc's coordinate. The texel offset
// is added to the integer texel coordinates, and every texel index is
// wrapped on its own: a repeating footprint at the edge of the image reads
// the texel of the opposite edge.
func linearFootprint(lc *levelCoord) footprint {
	w, h := int(lc.img.Width), int(lc.img.Height)
	su := lc.u*float32(w) - 0.5
	sv := lc.v*float32(h) - 0.5
	i0 := int(math.Floor(float64(su)))
	j0 := int(math.Floor(float64(sv)))
	f := footprint{fx: su - float32(i0), fy: sv - float32(j0)}
	i0 += lc.offset[0]
	j0 += lc.offset[1]
	f.x0, f.x1 = wrapTexel(i0, w, lc.wrapU), wrapTexel(i0+1, w, lc.wrapU)
	f.y0, f.y1 = wrapTexel(j0, h, lc.wrapV), wrapTexel(j0+1, h, lc.wrapV)
	return f
}

// nearestTexel returns the texel lc's coordinate falls in, wrapped.
func nearestTexel(lc *levelCoord) (x, y int) {
	w, h := int(lc.img.Width), int(lc.img.Height)
	i := int(math.Floor(float64(lc.u*float32(w)))) + lc.offset[0]
	j := int(math.Floor(float64(lc.v*float32(h)))) + lc.offset[1]
	return wrapTexel(i, w, lc.wrapU), wrapTexel(j, h, lc.wrapV)
}

// wrapTexel applies address mode wrap to texel index i of an axis of n
// texels (Vulkan "Wrapping Operation"): repeat takes i modulo n, mirrored
// repeat reflects every other period, and clamp-to-edge clamps to [0, n).
func wrapTexel(i, n int, wrap uint32) int {
	if n <= 0 { // an image missing from a short view: its reads are zero
		return 0
	}
	switch wrap {
	case WrapRepeat:
		i %= n
		if i < 0 {
			i += n
		}
		return i
	case WrapMirroredRepeat:
		t := i % (2 * n)
		if t < 0 {
			t += 2 * n
		}
		if t >= n {
			t = 2*n - 1 - t
		}
		return t
	}
	return clampInt(i, n-1)
}

// gatherView is OpImageGather (cmp nil: component comp of each texel) and
// OpImageDrefGather (cmp set: the comparison result of each texel's depth)
// over the linear footprint of coordinate c in the view's base level. The
// lanes are the texels (x0, y1), (x1, y1), (x1, y0) and (x0, y0), the order
// of Vulkan and of WGSL textureGather. The sampler's filters and level of
// detail play no part; its address modes do.
func gatherView(tex *Texture2D, samp *Sampler, c Vec4, offset [3]int, comp uint32, cmp *depthCompare) Vec4 {
	lv := tex.level(0)
	if lv.Width == 0 || lv.Height == 0 || tex.Dimension == TexDim3D {
		return Vec4{}
	}
	lc := levelImage(tex, samp, lv, c, offset)
	f := linearFootprint(&lc)
	texels := [4][2]int{{f.x0, f.y1}, {f.x1, f.y1}, {f.x1, f.y0}, {f.x0, f.y0}}
	var r Vec4
	for i, xy := range texels {
		t := readTexel(&lc.img, xy[0], xy[1])
		if cmp != nil {
			r[i] = cmp.test(tex.Format, t[0])
		} else {
			r[i] = t[comp&3]
		}
	}
	return r
}

// depthCompare is the comparison of a depth-compare instruction: the
// reference value and the sampler's compare function.
type depthCompare struct {
	ref float32
	fn  gputypes.CompareFunction
}

// test compares the reference with a texel's depth d and returns 1 when the
// comparison passes and 0 when it fails: ref < d for Less, and so on (WebGPU
// "compare function": the provided value is compared with the sampled one).
// The reference is clamped to [0, 1] for a unorm depth format, as Vulkan and
// D3D12 do for fixed-point depth.
func (dc *depthCompare) test(format uint32, d float32) float32 {
	ref := dc.ref
	if format == uint32(gputypes.TextureFormatDepth16Unorm) {
		ref = min(max(ref, 0), 1)
	}
	var pass bool
	switch dc.fn {
	case gputypes.CompareFunctionLess:
		pass = ref < d
	case gputypes.CompareFunctionEqual:
		pass = ref == d
	case gputypes.CompareFunctionLessEqual:
		pass = ref <= d
	case gputypes.CompareFunctionGreater:
		pass = ref > d
	case gputypes.CompareFunctionNotEqual:
		pass = ref != d
	case gputypes.CompareFunctionGreaterEqual:
		pass = ref >= d
	case gputypes.CompareFunctionAlways:
		pass = true
	}
	if pass {
		return 1
	}
	return 0
}

// sampleCompareView is the depth-compare sample of the view at coordinate c
// (OpImageSampleDref*): every texel the filter reads is compared with the
// reference first, and the 0 or 1 results are filtered as sampleView filters
// texels, so a linear filter gives the fraction of the footprint that passes.
func sampleCompareView(tex *Texture2D, samp *Sampler, c Vec4, lambda float32, offset [3]int, cmp *depthCompare) float32 {
	filter, l, frac := selectLevel(tex, samp, lambda)
	a := compareLevel(tex, samp, filter, l, c, offset, cmp)
	if frac == 0 {
		return a
	}
	b := compareLevel(tex, samp, filter, l+1, c, offset, cmp)
	return a + (b-a)*frac
}

// compareLevel is the depth-compare sample of mip level l with filter.
func compareLevel(tex *Texture2D, samp *Sampler, filter uint32, l int, c Vec4, offset [3]int, cmp *depthCompare) float32 {
	lv := tex.level(l)
	if lv.Width == 0 || lv.Height == 0 || tex.Dimension == TexDim3D {
		return 0
	}
	lc := levelImage(tex, samp, lv, c, offset)
	if filter != FilterLinear {
		x, y := nearestTexel(&lc)
		return cmp.test(tex.Format, readTexel(&lc.img, x, y)[0])
	}
	f := linearFootprint(&lc)
	t00 := cmp.test(tex.Format, readTexel(&lc.img, f.x0, f.y0)[0])
	t10 := cmp.test(tex.Format, readTexel(&lc.img, f.x1, f.y0)[0])
	t01 := cmp.test(tex.Format, readTexel(&lc.img, f.x0, f.y1)[0])
	t11 := cmp.test(tex.Format, readTexel(&lc.img, f.x1, f.y1)[0])
	top := t00 + (t10-t00)*f.fx
	bot := t01 + (t11-t01)*f.fx
	return top + (bot-top)*f.fy
}

// filter2D samples one image at the wrapped coordinate (u, v).
func filter2D(img *Texture2D, filter uint32, u, v float32) Vec4 {
	if img.Width == 0 || img.Height == 0 {
		return Vec4{}
	}
	if filter == FilterLinear {
		return sampleBilinear(img, u, v)
	}
	return sampleNearest(img, u, v)
}

// sample3D samples level lv of a 3D view: the nearest depth slice, or a
// blend of the two slices around w for a linear filter.
func sample3D(tex *Texture2D, lv TextureLevel, samp *Sampler, filter uint32, c Vec4, offset [3]int) Vec4 {
	u := applyWrapMode(c[0]+float32(offset[0])/float32(lv.Width), samp.WrapU)
	v := applyWrapMode(c[1]+float32(offset[1])/float32(lv.Height), samp.WrapV)
	depth := int(max(lv.Images, 1))
	w := applyWrapMode(c[2]+float32(offset[2])/float32(depth), samp.WrapW) * float32(depth)
	if filter != FilterLinear {
		img, _ := tex.imageAt(lv, clampInt(int(w), depth-1))
		return filter2D(&img, filter, u, v)
	}
	fz := w - 0.5
	z0 := int(math.Floor(float64(fz)))
	frac := fz - float32(z0)
	img0, _ := tex.imageAt(lv, clampInt(z0, depth-1))
	img1, _ := tex.imageAt(lv, clampInt(z0+1, depth-1))
	a := filter2D(&img0, filter, u, v)
	b := filter2D(&img1, filter, u, v)
	for i := range a {
		a[i] += (b[i] - a[i]) * frac
	}
	return a
}

// arrayIndex is the array layer a layer coordinate selects: rounded to the
// nearest even integer and clamped to [0, layers).
func arrayIndex(layer float32, layers uint32) int {
	return clampInt(int(math.RoundToEven(float64(layer))), int(max(layers, 1))-1)
}

// cubeAxes is the cube face a direction selects (Vulkan "Cube Map Face
// Selection"): sc = ss*c[si], tc = ts*c[ti], and the major axis.
type cubeAxes struct {
	face   int // +x, -x, +y, -y, +z, -z
	si, ti int
	ss, ts float32
	major  int
}

// cubeFaces are the cubeAxes of each face, in face order.
var cubeFaces = [6]cubeAxes{
	{face: 0, si: 2, ss: -1, ti: 1, ts: -1, major: 0}, // +x: sc = -rz, tc = -ry
	{face: 1, si: 2, ss: 1, ti: 1, ts: -1, major: 0},  // -x: sc = +rz, tc = -ry
	{face: 2, si: 0, ss: 1, ti: 2, ts: 1, major: 1},   // +y: sc = +rx, tc = +rz
	{face: 3, si: 0, ss: 1, ti: 2, ts: -1, major: 1},  // -y: sc = +rx, tc = -rz
	{face: 4, si: 0, ss: 1, ti: 1, ts: -1, major: 2},  // +z: sc = +rx, tc = -ry
	{face: 5, si: 0, ss: -1, ti: 1, ts: -1, major: 2}, // -z: sc = -rx, tc = -ry
}

// selectCubeFace returns the cubeAxes of the face direction c points at:
// the component of largest magnitude, x before y before z on a tie.
func selectCubeFace(c Vec4) cubeAxes {
	ax, ay, az := abs32(c[0]), abs32(c[1]), abs32(c[2])
	axis := 2
	switch {
	case ax >= ay && ax >= az:
		axis = 0
	case ay >= az:
		axis = 1
	}
	face := 2 * axis
	if c[axis] < 0 {
		face++
	}
	return cubeFaces[face]
}

// cubeFace returns the face direction c points at and the face coordinate
// (s, t) in [0, 1]. Faces are sampled on their own: a bilinear footprint
// clamps at the face edge rather than reading the adjacent face.
func cubeFace(c Vec4) (face int, s, t float32) {
	a := selectCubeFace(c)
	ma := abs32(c[a.major])
	if ma == 0 {
		return a.face, 0.5, 0.5
	}
	return a.face, 0.5 * (a.ss*c[a.si]/ma + 1), 0.5 * (a.ts*c[a.ti]/ma + 1)
}

// gradLod is the level of detail of explicit derivatives dx and dy of
// coordinate c (Vulkan "Scale Factor Operation"): log2 of the larger of the
// two derivative lengths in texels of the view's base level.
func gradLod(tex *Texture2D, c, dx, dy Vec4) float32 {
	rho := max(derivTexels(tex, c, dx), derivTexels(tex, c, dy))
	return float32(math.Log2(float64(rho)))
}

// derivTexels is the length of coordinate derivative g in texels.
func derivTexels(tex *Texture2D, c, g Vec4) float32 {
	w, h := float32(tex.Width), float32(tex.Height)
	switch tex.Dimension {
	case TexDim1D:
		return abs32(g[0]) * w
	case TexDim3D:
		d := float32(max(tex.Depth, 1))
		return hypot3(g[0]*w, g[1]*h, g[2]*d)
	case TexDimCube, TexDimCubeArray:
		ds, dt := cubeDerivative(c, g)
		return hypot3(ds*w, dt*h, 0)
	}
	return hypot3(g[0]*w, g[1]*h, 0)
}

// cubeDerivative is the derivative of the face coordinate (s, t) along the
// direction derivative g: s = (sc/|ma| + 1)/2 differentiates to
// (dsc*|ma| - sc*d|ma|) / (2*ma²).
func cubeDerivative(c, g Vec4) (ds, dt float32) {
	a := selectCubeFace(c)
	ma := c[a.major]
	if ma == 0 {
		return 0, 0
	}
	sign := float32(1)
	if ma < 0 {
		sign = -1
	}
	am, dam := abs32(ma), sign*g[a.major]
	den := 2 * ma * ma
	ds = a.ss * (g[a.si]*am - c[a.si]*dam) / den
	dt = a.ts * (g[a.ti]*am - c[a.ti]*dam) / den
	return ds, dt
}

func abs32(f float32) float32 { return float32(math.Abs(float64(f))) }

func hypot3(a, b, c float32) float32 {
	return float32(math.Sqrt(float64(a*a + b*b + c*c)))
}

// imageCoord3 returns the integer texel coordinate x, y, z of an image
// instruction's coordinate operand; missing components are zero.
func imageCoord3(coord Value) [3]int {
	switch coord.Tag {
	case TagVec2, TagVec3, TagVec4:
		return [3]int{int(coord.F[0]), int(coord.F[1]), int(coord.F[2])}
	case TagIVec2, TagIVec3, TagIVec4:
		return [3]int{int(int32(coord.U[0])), int(int32(coord.U[1])), int(int32(coord.U[2]))}
	case TagUVec2, TagUVec3, TagUVec4:
		return [3]int{int(coord.U[0]), int(coord.U[1]), int(coord.U[2])}
	case TagArray:
		var xyz [3]int
		for i, e := range coord.AsArray() {
			if i < len(xyz) {
				xyz[i] = scalarInt(e)
			}
		}
		return xyz
	default:
		return [3]int{scalarInt(coord), 0, 0}
	}
}
