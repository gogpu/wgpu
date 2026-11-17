//go:build !(js && wasm)

// Result and operand ID validation of a parsed module.

package shader

import "fmt"

// maxIDBound is the largest header Bound accepted: the SPIR-V universal limit
// on the Result <id> bound (SPIR-V Specification 1.6, section 2.17
// "Universal Limits": 4,194,303).
const maxIDBound = 4_194_303

// Memory operand bits that carry extra operands (SPIR-V 1.6, section 3.26
// "Memory Operands"). Their extra operands follow the mask in bit order.
const (
	memoryOperandAligned              = 0x2  // one literal: the alignment
	memoryOperandMakePointerAvailable = 0x8  // one ID: the scope
	memoryOperandMakePointerVisible   = 0x10 // one ID: the scope
)

// operandTail says what follows an instruction's leading ID operands.
type operandTail uint8

const (
	tailIDs            operandTail = iota // every further operand is an ID
	tailLiterals                          // every further operand is a literal
	tailImageOperands                     // an optional Image Operands mask, then IDs
	tailMemoryOperands                    // an optional Memory Operands mask and its extras
)

// operandLayout describes where the IDs are among an instruction's operands
// (Instruction.Operands, after the result type and result ID of a typed
// opcode). The first skip operands are literals, the next ids are IDs, and
// tail describes the rest. ids < 0 means every operand after skip is an ID.
type operandLayout struct {
	skip int
	ids  int
	tail operandTail
}

// operandLayouts lists the opcodes of supportedOps whose operands are not
// all IDs. Every other supported opcode has only ID operands (arithmetic,
// conversions, comparisons, composites built from IDs, OpAccessChain,
// OpPhi, OpFunctionCall, atomics and barriers, whose scope and semantics
// operands are IDs too). The layouts follow the SPIR-V 1.6 grammar of each
// instruction (section 3.52). OpSwitch (switchIDs) and OpExtInst (set,
// literal instruction number, operand IDs) are handled in checkInstruction.
var operandLayouts = map[uint16]operandLayout{
	OpVariable:                   {skip: 1, ids: -1},                 // storage class, [initializer]
	OpBranchConditional:          {ids: 3, tail: tailLiterals},       // condition, true, false, [weights]
	OpSelectionMerge:             {ids: 1, tail: tailLiterals},       // merge block, selection control
	OpLoopMerge:                  {ids: 2, tail: tailLiterals},       // merge, continue, loop control [params]
	OpLoad:                       {ids: 1, tail: tailMemoryOperands}, // pointer
	OpStore:                      {ids: 2, tail: tailMemoryOperands}, // pointer, object
	OpArrayLength:                {ids: 1, tail: tailLiterals},       // structure, member index
	OpCompositeExtract:           {ids: 1, tail: tailLiterals},       // composite, indexes
	OpCompositeInsert:            {ids: 2, tail: tailLiterals},       // object, composite, indexes
	OpVectorShuffle:              {ids: 2, tail: tailLiterals},       // vectors, components
	OpSDotKHR:                    {ids: 2, tail: tailLiterals},       // vectors, [packed vector format]
	OpUDotKHR:                    {ids: 2, tail: tailLiterals},       // vectors, [packed vector format]
	OpImageSampleImplicitLod:     {ids: 2, tail: tailImageOperands},  // sampled image, coordinate
	OpImageSampleExplicitLod:     {ids: 2, tail: tailImageOperands},  // sampled image, coordinate
	OpImageSampleDrefImplicitLod: {ids: 3, tail: tailImageOperands},  // sampled image, coordinate, Dref
	OpImageSampleDrefExplicitLod: {ids: 3, tail: tailImageOperands},  // sampled image, coordinate, Dref
	OpImageGather:                {ids: 3, tail: tailImageOperands},  // sampled image, coordinate, component
	OpImageDrefGather:            {ids: 3, tail: tailImageOperands},  // sampled image, coordinate, Dref
	OpImageFetch:                 {ids: 2, tail: tailImageOperands},  // image, coordinate
	OpImageRead:                  {ids: 2, tail: tailImageOperands},  // image, coordinate
	OpImageWrite:                 {ids: 3, tail: tailImageOperands},  // image, coordinate, texel
}

// idChecker validates IDs against the module's Bound and the set of IDs the
// module defines.
type idChecker struct {
	m       *Module
	defined []bool // indexed by ID, len == Bound
	maxID   uint32
}

// validateIDs checks what the interpreter relies on when it indexes its
// per-invocation value slice by ID: the header Bound is within the SPIR-V
// limit, every result ID is in (0, Bound), and every ID operand of a
// function-body instruction, every result type, every entry point's function
// and interface, and every variable initializer names an ID the module
// defines. A module that breaks any of these would otherwise index out of
// range at the first draw or dispatch; the SPIR-V validator and Rust wgpu
// (naga's SPIR-V frontend, run at shader module creation) reject it too.
// Forward references are legal in SPIR-V, so this runs after the whole
// module is parsed. It also records the number of value slots the
// interpreter needs (the largest defined ID plus one).
func (m *Module) validateIDs() error {
	if m.Bound == 0 || m.Bound > maxIDBound {
		return fmt.Errorf("spirv: header ID bound %d is outside [1, %d]", m.Bound, maxIDBound)
	}
	c := &idChecker{m: m, defined: make([]bool, m.Bound)}
	if err := c.collectResults(); err != nil {
		return err
	}
	if err := c.checkReferences(); err != nil {
		return err
	}
	m.valueSlots = c.maxID + 1
	return nil
}

// requireType fails unless id is a type already declared: SPIR-V declares a
// type before any type that uses it (section 2.4), so a type can never
// contain itself and every lookup of a type's parts finds a type.
func (m *Module) requireType(result uint32, what string, id uint32) error {
	if _, ok := m.Types[id]; !ok {
		return fmt.Errorf("spirv: type %d: %s %d is not a type declared before it", result, what, id)
	}
	return nil
}

// define records id as defined, or fails when it is outside (0, Bound).
func (c *idChecker) define(id uint32, what string) error {
	if id == 0 || id >= c.m.Bound {
		return fmt.Errorf("spirv: %s result ID %d is outside [1, %d)", what, id, c.m.Bound)
	}
	c.defined[id] = true
	c.maxID = max(c.maxID, id)
	return nil
}

// use fails when id is not an ID the module defines.
func (c *idChecker) use(id uint32, what string) error {
	if id >= c.m.Bound {
		return fmt.Errorf("spirv: %s uses ID %d, not below the ID bound %d", what, id, c.m.Bound)
	}
	if !c.defined[id] {
		return fmt.Errorf("spirv: %s uses ID %d, which the module does not define", what, id)
	}
	return nil
}

// collectResults defines every result ID the parser stored.
func (c *idChecker) collectResults() error {
	m := c.m
	for id := range m.Types {
		if err := c.define(id, "type"); err != nil {
			return err
		}
	}
	for id := range m.Constants {
		if err := c.define(id, "constant"); err != nil {
			return err
		}
	}
	for id := range m.Variables {
		if err := c.define(id, "variable"); err != nil {
			return err
		}
	}
	for id := range m.ExtInstImports {
		if err := c.define(id, "OpExtInstImport"); err != nil {
			return err
		}
	}
	for id, fn := range m.FunctionsByID {
		if err := c.define(id, "function"); err != nil {
			return err
		}
		for i := range fn.Instructions {
			inst := &fn.Instructions[i]
			if inst.Opcode == OpLabel || supportedOps[inst.Opcode].typed {
				if err := c.define(inst.ResultID, opcodeName(inst.Opcode)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// checkReferences checks every ID the interpreter reads from the module.
func (c *idChecker) checkReferences() error {
	m := c.m
	for name, ep := range m.EntryPoints {
		what := "entry point " + name
		if err := c.use(ep.FunctionID, what); err != nil {
			return err
		}
		for _, id := range ep.InterfaceIDs {
			if err := c.use(id, what); err != nil {
				return err
			}
		}
	}
	for _, vi := range m.Variables {
		if err := c.use(vi.TypeID, "OpVariable result type"); err != nil {
			return err
		}
		if vi.Initializer != 0 {
			if err := c.use(vi.Initializer, "OpVariable initializer"); err != nil {
				return err
			}
		}
	}
	for _, fn := range m.FunctionsByID {
		for i := range fn.Instructions {
			if err := c.checkInstruction(&fn.Instructions[i]); err != nil {
				return err
			}
		}
	}
	return nil
}

// checkInstruction checks the result type and the ID operands of one
// function-body instruction.
func (c *idChecker) checkInstruction(inst *Instruction) error {
	name := opcodeName(inst.Opcode)
	if supportedOps[inst.Opcode].typed {
		if _, ok := c.m.Types[inst.TypeID]; !ok {
			return fmt.Errorf("spirv: %s result %d has result type %d, which is not a type", name, inst.ResultID, inst.TypeID)
		}
	}
	ops := inst.Operands
	switch inst.Opcode {
	case OpSwitch:
		return c.useAll(c.switchIDs(ops), name)
	case OpExtInst:
		// set, instruction number (literal), operand IDs...
		if len(ops) < 2 {
			return fmt.Errorf("spirv: OpExtInst result %d has %d operands, want at least 2", inst.ResultID, len(ops))
		}
		if err := c.use(ops[0], name); err != nil {
			return err
		}
		if c.m.ExtInstImports[ops[0]] != glslExtSetName {
			// run() executes GLSL.std.450 only and never reads the operands
			// of another set. Those of the non-semantic sets name IDs the
			// parser does not keep (OpString for NonSemantic.Shader.DebugInfo,
			// which glslang and DXC emit), as naga skips them (front/spv
			// next_block.rs).
			return nil
		}
		return c.useAll(ops[2:], name)
	}
	layout, ok := operandLayouts[inst.Opcode]
	if !ok {
		return c.useAll(ops, name)
	}
	ops = ops[min(layout.skip, len(ops)):]
	if layout.ids < 0 {
		return c.useAll(ops, name)
	}
	n := min(layout.ids, len(ops))
	if err := c.useAll(ops[:n], name); err != nil {
		return err
	}
	rest := ops[n:]
	switch layout.tail {
	case tailIDs:
		return c.useAll(rest, name)
	case tailImageOperands:
		if len(rest) > 0 {
			// The Image Operands mask is a literal; every operand it adds is an ID.
			return c.useAll(rest[1:], name)
		}
	case tailMemoryOperands:
		return c.useAll(memoryOperandIDs(rest), name)
	}
	return nil
}

// useAll checks each of ids with use.
func (c *idChecker) useAll(ids []uint32, what string) error {
	for _, id := range ids {
		if err := c.use(id, what); err != nil {
			return err
		}
	}
	return nil
}

// switchIDs returns the ID operands of an OpSwitch: the selector, the
// default label and each target label. Each case literal takes as many
// words as the selector's integer type is wide (one for 32 bits, two for
// 64), so the labels are found from the selector's type.
func (c *idChecker) switchIDs(ops []uint32) []uint32 {
	if len(ops) < 2 {
		return ops
	}
	ids := []uint32{ops[0], ops[1]}
	litWords := 1
	if ti := c.m.Types[c.m.ResultTypes[ops[0]]]; ti != nil && ti.Width == 64 {
		litWords = 2
	}
	for i := 2 + litWords; i < len(ops); i += litWords + 1 {
		ids = append(ids, ops[i])
	}
	return ids
}

// memoryOperandIDs returns the ID operands that follow an optional Memory
// Operands mask (SPIR-V 1.6, section 3.26): the scope IDs of
// MakePointerAvailable and MakePointerVisible, after the alignment literal
// of Aligned, in bit order.
func memoryOperandIDs(rest []uint32) []uint32 {
	if len(rest) == 0 {
		return nil
	}
	mask, extras := rest[0], rest[1:]
	var ids []uint32
	next := func() (uint32, bool) {
		if len(extras) == 0 {
			return 0, false
		}
		w := extras[0]
		extras = extras[1:]
		return w, true
	}
	if mask&memoryOperandAligned != 0 {
		next()
	}
	if mask&memoryOperandMakePointerAvailable != 0 {
		if id, ok := next(); ok {
			ids = append(ids, id)
		}
	}
	if mask&memoryOperandMakePointerVisible != 0 {
		if id, ok := next(); ok {
			ids = append(ids, id)
		}
	}
	return ids
}
