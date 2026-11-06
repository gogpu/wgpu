package shader

import "math"

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
	if lambda != lambda { // NaN
		lambda = 0
	}
	lambda = min(max(lambda, samp.LodMinClamp), samp.LodMaxClamp)
	filter := samp.MagFilter
	if lambda > 0 {
		filter = samp.MinFilter
	}
	q := float32(tex.levelCount() - 1)
	d := min(max(lambda, 0), q)
	if samp.MipmapFilter != FilterLinear {
		level := int(math.Ceil(float64(d)+0.5)) - 1
		return sampleLevel(tex, samp, filter, level, c, offset)
	}
	lo := float32(math.Floor(float64(d)))
	a := sampleLevel(tex, samp, filter, int(lo), c, offset)
	frac := d - lo
	if frac == 0 {
		return a
	}
	b := sampleLevel(tex, samp, filter, int(lo)+1, c, offset)
	for i := range a {
		a[i] += (b[i] - a[i]) * frac
	}
	return a
}

// sampleLevel samples mip level l of the view with filter.
func sampleLevel(tex *Texture2D, samp *Sampler, filter uint32, l int, c Vec4, offset [3]int) Vec4 {
	lv := tex.level(l)
	if lv.Width == 0 || lv.Height == 0 {
		return Vec4{}
	}
	ou := float32(offset[0]) / float32(lv.Width)
	ov := float32(offset[1]) / float32(lv.Height)
	switch tex.Dimension {
	case TexDimCube, TexDimCubeArray:
		face, s, t := cubeFace(c)
		if tex.Dimension == TexDimCubeArray {
			face += 6 * arrayIndex(c[3], max(lv.Images, 1)/6)
		}
		img, _ := tex.imageAt(lv, face)
		return filter2D(&img, filter, applyWrapMode(s, WrapClampToEdge), applyWrapMode(t, WrapClampToEdge))
	case TexDim3D:
		return sample3D(tex, lv, samp, filter, c, offset)
	case TexDim1D:
		img, _ := tex.imageAt(lv, 0)
		return filter2D(&img, filter, applyWrapMode(c[0]+ou, samp.WrapU), 0.5)
	}
	z := 0
	if tex.Dimension == TexDim2DArray {
		z = arrayIndex(c[2], lv.Images)
	}
	img, _ := tex.imageAt(lv, z)
	return filter2D(&img, filter, applyWrapMode(c[0]+ou, samp.WrapU), applyWrapMode(c[1]+ov, samp.WrapV))
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
