//go:build !(js && wasm)

package core

import (
	"github.com/gogpu/naga/ir"
	"github.com/gogpu/naga/valid"
)

// workgroupStorageGranularity is the WebGPU roundUp(16, SizeOf(T)) step of
// workgroupStorageUsed.
const workgroupStorageGranularity = 16

// WorkgroupStorageSize returns the workgroup storage an entry point uses:
// WebGPU's workgroupStorageUsed (createComputePipeline valid usage), the sum
// of roundUp(16, SizeOf(T)) over the workgroup-address-space variables the
// entry point uses statically. uses is the entry point's
// valid.FunctionInfo.GlobalUses, callees included. Override-sized arrays
// must be resolved first (naga.ProcessOverrides); an unresolved one has no
// size and counts as zero.
func WorkgroupStorageSize(module *ir.Module, uses []valid.GlobalUse) uint64 {
	if module == nil {
		return 0
	}
	var layouter ir.Layouter
	if err := layouter.Update(module); err != nil {
		return 0 // only a type arena validation rejects fails to lay out
	}
	var total uint64
	for h := range module.GlobalVariables {
		gv := &module.GlobalVariables[h]
		if gv.Space != ir.SpaceWorkGroup || h >= len(uses) || uses[h] == 0 {
			continue
		}
		size := uint64(layouter.Layout(gv.Type).Size)
		total += (size + workgroupStorageGranularity - 1) &^ (workgroupStorageGranularity - 1)
	}
	return total
}
