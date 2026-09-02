//go:build !(js && wasm)

package core

import (
	"fmt"

	"github.com/gogpu/gputypes"
)

const (
	spirvMagic     = 0x07230203
	spirvOpCodeCap = 17
)

// SPIR-V capability IDs (Khronos SPIR-V spec, matching naga/spirv/internal/codegen).
const (
	spirvCapFloat16                   = 9
	spirvCapFloat64                   = 10
	spirvCapInt64                     = 18
	spirvCapGroupNonUniform           = 61
	spirvCapGroupNonUniformVote       = 62
	spirvCapGroupNonUniformArithmetic = 63
	spirvCapGroupNonUniformBallot     = 64
	spirvCapGroupNonUniformShuffle    = 65
	spirvCapGroupNonUniformShuffleRel = 66
	spirvCapGroupNonUniformQuad       = 68
)

func scanSPIRVCapabilities(spirv []uint32) ([]uint32, error) {
	if len(spirv) < 5 {
		return nil, fmt.Errorf("shader module: SPIR-V module too short")
	}
	if spirv[0] != spirvMagic {
		return nil, fmt.Errorf("shader module: invalid SPIR-V magic")
	}

	var caps []uint32
	for i := 5; i < len(spirv); {
		wordCount := int(spirv[i] >> 16)
		opcode := spirv[i] & 0xFFFF
		if wordCount <= 0 {
			break
		}
		if i+wordCount > len(spirv) {
			break
		}
		if opcode == spirvOpCodeCap && wordCount >= 2 && i+1 < len(spirv) {
			caps = append(caps, spirv[i+1])
		}
		i += wordCount
	}
	return caps, nil
}

func validateSPIRVShaderFeatures(spirv []uint32, features gputypes.Features) error {
	caps, err := scanSPIRVCapabilities(spirv)
	if err != nil {
		// Malformed modules are rejected by HAL shader compilation.
		return nil //nolint:nilerr
	}

	for _, capID := range caps {
		switch capID {
		case spirvCapFloat16:
			if err := RequireFeature(features, gputypes.FeatureShaderF16, "CreateShaderModule"); err != nil {
				return err
			}
		case spirvCapFloat64, spirvCapInt64:
			if err := RequireFeature(features, gputypes.FeatureShaderFloat64, "CreateShaderModule"); err != nil {
				return err
			}
		case spirvCapGroupNonUniform,
			spirvCapGroupNonUniformVote,
			spirvCapGroupNonUniformArithmetic,
			spirvCapGroupNonUniformBallot,
			spirvCapGroupNonUniformShuffle,
			spirvCapGroupNonUniformShuffleRel,
			spirvCapGroupNonUniformQuad:
			if err := RequireFeature(features, gputypes.FeatureSubgroupOperations, "CreateShaderModule"); err != nil {
				return err
			}
		}
	}

	return nil
}
