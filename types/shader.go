package types

// ShaderStage represents a shader stage.
type ShaderStage uint8

const (
	// ShaderStageNone represents no shader stage.
	ShaderStageNone ShaderStage = 0
	// ShaderStageVertex is the vertex shader stage.
	ShaderStageVertex ShaderStage = 1 << iota
	// ShaderStageFragment is the fragment shader stage.
	ShaderStageFragment
	// ShaderStageCompute is the compute shader stage.
	ShaderStageCompute
)

// ShaderStages is a combination of shader stages.
type ShaderStages = ShaderStage

const (
	// ShaderStagesVertexFragment includes vertex and fragment.
	ShaderStagesVertexFragment = ShaderStageVertex | ShaderStageFragment
	// ShaderStagesAll includes all stages.
	ShaderStagesAll = ShaderStageVertex | ShaderStageFragment | ShaderStageCompute
)

// ShaderModuleDescriptor describes a shader module.
type ShaderModuleDescriptor struct {
	// Label is a debug label.
	Label string
	// Source is the shader source (WGSL, SPIR-V, etc.).
	Source ShaderSource
}

// ShaderSource represents shader source code.
type ShaderSource interface {
	shaderSource()
}

// ShaderSourceWGSL is WGSL shader source.
type ShaderSourceWGSL struct {
	// Code is the WGSL source code.
	Code string
}

func (ShaderSourceWGSL) shaderSource() {}

// ShaderSourceSPIRV is SPIR-V shader source.
type ShaderSourceSPIRV struct {
	// Code is the SPIR-V bytecode.
	Code []uint32
}

func (ShaderSourceSPIRV) shaderSource() {}

// ShaderSourceGLSL is GLSL shader source.
type ShaderSourceGLSL struct {
	// Code is the GLSL source code.
	Code string
	// Stage is the shader stage.
	Stage ShaderStage
	// Defines is a map of preprocessor defines.
	Defines map[string]string
}

func (ShaderSourceGLSL) shaderSource() {}

// ProgrammableStage describes a programmable shader stage.
type ProgrammableStage struct {
	// EntryPoint is the entry point function name.
	EntryPoint string
	// Constants are pipeline-overridable constants.
	Constants map[string]float64
}
