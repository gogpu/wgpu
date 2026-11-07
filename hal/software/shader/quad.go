//go:build !(js && wasm)

package shader

import (
	"errors"
	"fmt"
)

// QuadLanes is the number of fragments in a quad. Lane i is the pixel
// (x0 + i&1, y0 + i>>1) of the 2x2 block whose top-left pixel (x0, y0) has
// even coordinates, as on a GPU (Vulkan "Derivative Operations").
const QuadLanes = 4

// Quad is the per-lane input and output of one ExecuteQuad call. The caller
// owns the maps and reuses them from quad to quad, so running a quad
// allocates nothing per fragment.
type Quad struct {
	// Inputs holds each lane's Input variable values, by variable ID.
	Inputs [QuadLanes]map[uint32]Value
	// Outputs receives the Output variable values of each lane that is not
	// a helper and did not discard; ExecuteQuad clears every map first and
	// creates the ones that are nil.
	Outputs [QuadLanes]map[uint32]Value
	// Helpers marks the helper lanes (bit i: lane i): fragments that are
	// not covered, or failed the early depth or stencil test, and run only
	// so their neighbors have values to take differences from.
	Helpers uint8
	// Discarded is set by ExecuteQuad: bit i when lane i discarded.
	Discarded uint8
}

// NeedsQuads reports whether the fragment entry point entryPoint, or a
// function it calls, has a derivative or an implicit-LOD sample: such an
// entry point must run through ExecuteQuad to get the values a GPU gives. An
// entry point without them gets the same results one fragment at a time,
// which is cheaper.
func (m *Module) NeedsQuads(entryPoint string) bool {
	return m.prepared().quadEntries[entryPoint]
}

// ExecuteQuad runs entry point entryPoint for the four lanes of a quad, in
// ctx (its Inputs field is ignored: each lane takes q.Inputs[i]).
//
// The lanes run in turn, each until it ends or reaches an instruction that
// needs its neighbors: a derivative (OpDPdx, OpDPdy, OpFwidth and their Fine
// and Coarse forms) or an implicit-LOD sample (OpImageSampleImplicitLod,
// OpImageSampleDrefImplicitLod). Once no lane can go further, the executor
// computes each such instruction from the lanes that stopped at it and
// resumes them, as a GPU runs a quad in lockstep.
//
// Fine derivatives (and the plain ones, which are fine here as on llvmpipe)
// take the difference between the two lanes of the lane's row (x) or column
// (y); coarse ones the difference in the top row and the left column, one
// value for the whole quad. An implicit-LOD sample takes its level of detail
// from the coarse derivatives of its coordinate, one level per quad.
//
// WGSL requires derivatives and implicit-LOD samples in uniform control
// flow, so every lane of the quad reaches them together. A lane that did not
// (it ended, discarded, or stopped at another instruction) gives no value:
// the difference is then taken in the other row or column, and is 0 if
// neither has both lanes.
//
// This differs from WGSL after a discard: discard does not make control flow
// non-uniform, and a discarded invocation becomes a helper that keeps feeding
// derivatives in its own row and column (WGSL "Discard Statement"). naga
// emits OpKill for discard, which ends the lane here; matching GPUs needs
// OpDemoteToHelperInvocation (P3-SW-035). Linear values give the same
// difference in either row, so only non-linear ones differ.
func (m *Module) ExecuteQuad(entryPoint string, ctx *ExecutionContext, q *Quad) error {
	ep, ok := m.EntryPoints[entryPoint]
	if !ok {
		return fmt.Errorf("spirv: entry point %q not found", entryPoint)
	}
	fn, ok := m.Functions[entryPoint]
	if !ok {
		return fmt.Errorf("spirv: function body for %q not found", entryPoint)
	}
	if ctx == nil {
		ctx = &ExecutionContext{}
	}

	var lanes [QuadLanes]*interpreter
	for i := range lanes {
		interp := m.getInterpreter()
		interp.ep = ep
		interp.fn = fn
		interp.ctx = ctx
		interp.prevBlock = 0
		interp.iterationCount = 0
		interp.callDepth = 0
		interp.returnValue = Value{}
		interp.quad = true
		interp.helper = q.Helpers&(1<<i) != 0
		interp.seedConstants()
		interp.initVariables(q.Inputs[i])
		interp.initResourceVariables()
		lanes[i] = interp
	}
	q.Discarded = 0
	for i, out := range q.Outputs {
		if out == nil {
			q.Outputs[i] = make(map[uint32]Value)
		} else {
			clear(out)
		}
	}

	running := uint8(1<<QuadLanes - 1)
	for running != 0 {
		for i, interp := range lanes {
			bit := uint8(1) << i
			if running&bit == 0 {
				continue
			}
			err := interp.run()
			if err == nil && interp.suspended {
				continue
			}
			running &^= bit
			lanes[i] = nil
			switch {
			case err == nil:
				if !interp.helper {
					interp.collectOutputs(q.Outputs[i])
				}
				m.putInterpreter(interp)
			case errors.Is(err, ErrDiscarded):
				q.Discarded |= bit
				m.release(interp)
			default:
				m.release(interp)
				for _, other := range lanes {
					if other != nil {
						m.release(other)
					}
				}
				return err
			}
		}
		if running != 0 {
			syncQuad(&lanes)
		}
	}
	return nil
}

// collectOutputs stores the value of every Output variable of the entry
// point in outputs, by variable ID.
func (interp *interpreter) collectOutputs(outputs map[uint32]Value) {
	m := interp.module
	for _, varID := range interp.ep.InterfaceIDs {
		vi, ok := m.Variables[varID]
		if !ok {
			continue
		}
		if vi.StorageClass != StorageClassOutput {
			continue
		}
		v := interp.values[varID]
		if v.Tag == TagPointer {
			outputs[varID] = v.AsPointer().Val
		}
	}
}

// syncQuad computes the instruction each suspended lane stopped at. The
// lanes stopped at the same instruction of the same function form a group
// and take their differences from each other only.
func syncQuad(lanes *[QuadLanes]*interpreter) {
	// at[i] is the interpreter lane i stopped in: the innermost callee of
	// its suspended call chain.
	var at [QuadLanes]*interpreter
	for i, interp := range lanes {
		if interp == nil {
			continue
		}
		for interp.pendingCall != nil {
			interp = interp.pendingCall
		}
		at[i] = interp
	}
	for i, first := range at {
		if first == nil {
			continue
		}
		var group [QuadLanes]*interpreter
		var have uint8
		for j := i; j < QuadLanes; j++ {
			if at[j] != nil && at[j].fn == first.fn && at[j].quadPC == first.quadPC {
				group[j] = at[j]
				have |= 1 << j
				at[j] = nil
			}
		}
		syncGroup(&group, have, first.fn.Instructions[first.quadPC])
	}
}

// syncGroup computes inst for every lane of group (have marks them) and
// stores each result in its lane.
func syncGroup(group *[QuadLanes]*interpreter, have uint8, inst Instruction) {
	var v [QuadLanes]Vec4
	switch inst.Opcode {
	case OpImageSampleImplicitLod, OpImageSampleDrefImplicitLod:
		for j, interp := range group {
			if interp != nil {
				v[j] = texelComponents(interp.values[inst.Operands[1]])
			}
		}
		grad := [2]Vec4{quadDX(&v, have, 0), quadDY(&v, have, 0)}
		for _, interp := range group {
			if interp != nil {
				interp.values[inst.ResultID] = interp.sampleImplicitLod(inst, &grad)
			}
		}
	default:
		for j, interp := range group {
			if interp != nil {
				v[j] = interp.values[inst.Operands[0]].F
			}
		}
		for j, interp := range group {
			if interp != nil {
				d := zeroDerivative(interp.values[inst.Operands[0]])
				d.F = laneDerivative(inst.Opcode, &v, have, j)
				interp.values[inst.ResultID] = d
			}
		}
	}
}

// laneDerivative is derivative opcode op of the lane values v for lane.
func laneDerivative(op uint16, v *[QuadLanes]Vec4, have uint8, lane int) Vec4 {
	row, col := lane>>1, lane&1
	switch op {
	case OpDPdxCoarse, OpDPdyCoarse, OpFwidthCoarse:
		row, col = 0, 0
	}
	switch op {
	case OpDPdx, OpDPdxFine, OpDPdxCoarse:
		return quadDX(v, have, row)
	case OpDPdy, OpDPdyFine, OpDPdyCoarse:
		return quadDY(v, have, col)
	}
	// OpFwidth, OpFwidthFine, OpFwidthCoarse: |dx| + |dy|.
	dx, dy := quadDX(v, have, row), quadDY(v, have, col)
	for k := range dx {
		dx[k] = abs32(dx[k]) + abs32(dy[k])
	}
	return dx
}

// quadDX is the difference along x in quad row row (right lane minus left
// lane), or in the other row when a lane of row row has no value, or 0.
func quadDX(v *[QuadLanes]Vec4, have uint8, row int) Vec4 {
	if d, ok := laneDiff(v, have, row*2, row*2+1); ok {
		return d
	}
	d, _ := laneDiff(v, have, (row^1)*2, (row^1)*2+1)
	return d
}

// quadDY is the difference along y in quad column col (bottom lane minus top
// lane), or in the other column when a lane of column col has no value, or 0.
func quadDY(v *[QuadLanes]Vec4, have uint8, col int) Vec4 {
	if d, ok := laneDiff(v, have, col, col+2); ok {
		return d
	}
	d, _ := laneDiff(v, have, col^1, (col^1)+2)
	return d
}

// laneDiff is v[b] - v[a], or false when either lane has no value.
func laneDiff(v *[QuadLanes]Vec4, have uint8, a, b int) (Vec4, bool) {
	if have&(1<<a) == 0 || have&(1<<b) == 0 {
		return Vec4{}, false
	}
	var d Vec4
	for k := range d {
		d[k] = v[b][k] - v[a][k]
	}
	return d, true
}

// suspendInQuad stops the lane at the instruction before pc, which needs
// the quad's other lanes (see ExecuteQuad); run resumes at pc.
func (interp *interpreter) suspendInQuad(pc int) {
	interp.quadPC = pc - 1
	interp.resumePC = pc
	interp.suspended = true
}

// sampleImplicitLod runs the implicit-LOD sample inst
// (OpImageSampleImplicitLod or OpImageSampleDrefImplicitLod). grad holds
// the screen-space derivatives of the coordinate in x and y, which give the
// level of detail; without them (an invocation run outside a quad) the level
// of detail is λ_base = 0, the view's base level. The Bias operand adds to
// either.
func (interp *interpreter) sampleImplicitLod(inst Instruction, grad *[2]Vec4) Value {
	first := 2 // the image operands follow the coordinate ...
	if inst.Opcode == OpImageSampleDrefImplicitLod {
		first = 3 // ... or the depth reference
	}
	ops := interp.parseImageOperands(inst.Operands[first:])
	ops.lod, ops.hasGrad = 0, false
	if grad != nil {
		ops.hasGrad, ops.dx, ops.dy = true, grad[0], grad[1]
	}
	img, coord := interp.values[inst.Operands[0]], interp.values[inst.Operands[1]]
	if inst.Opcode == OpImageSampleDrefImplicitLod {
		return interp.sampleCompare(img, coord, toFloat32(interp.values[inst.Operands[2]]), ops)
	}
	return interp.sampleTexture(img, coord, ops)
}

// isQuadOp reports whether opcode op needs the other lanes of a quad.
func isQuadOp(op uint16) bool {
	switch op {
	case OpDPdx, OpDPdy, OpFwidth, OpDPdxFine, OpDPdyFine, OpFwidthFine,
		OpDPdxCoarse, OpDPdyCoarse, OpFwidthCoarse,
		OpImageSampleImplicitLod, OpImageSampleDrefImplicitLod:
		return true
	}
	return false
}

// reachesQuadOp reports whether fn, or a function it calls, has a quad
// instruction (isQuadOp). seen holds the functions already searched.
func (m *Module) reachesQuadOp(fn *Function, seen map[*Function]bool) bool {
	if seen[fn] {
		return false
	}
	seen[fn] = true
	for i := range fn.Instructions {
		inst := &fn.Instructions[i]
		if isQuadOp(inst.Opcode) {
			return true
		}
		if inst.Opcode == OpFunctionCall && len(inst.Operands) >= 1 {
			if callee, ok := m.FunctionsByID[inst.Operands[0]]; ok && m.reachesQuadOp(callee, seen) {
				return true
			}
		}
	}
	return false
}
