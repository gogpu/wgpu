// Package pipeconst checks the pipeline-overridable constants of a pipeline
// stage (WebGPU GPUProgrammableStage.constants) before any backend sees them.
//
// It is the one check every build path shares: the native backends, -tags
// rust (wgpu-native) and js/wasm (the browser). It has no dependencies, so the
// browser build does not link naga for it.
//
// What it checks is what a value can be wrong about without the shader:
//   - a key is never empty: webgpu.h refuses a null or empty key string
//     (NonNullInputString), and no WGSL override has an empty identifier;
//   - a value is finite: GPUPipelineConstantValue is a WebIDL double, which
//     refuses NaN and the infinities with a TypeError, and webgpu.h reports
//     NonFiniteFloatValueError. This holds for a bool override too, where
//     naga's own conversion would read NaN as false.
//
// Unknown keys, values out of the override's range and overrides without a
// value or a default need the shader; naga.ProcessOverrides reports them.
package pipeconst

import (
	"errors"
	"fmt"
	"math"
	"slices"
)

var (
	// ErrEmptyKey is the reason for a constant whose key is "".
	ErrEmptyKey = errors.New("key must not be empty")
	// ErrNotFinite is the reason for a constant whose value is NaN or an
	// infinity.
	ErrNotFinite = errors.New("value must be finite")
)

// Error is a pipeline constant that cannot be used. Err is ErrEmptyKey or
// ErrNotFinite.
type Error struct {
	Key   string
	Value float64
	Err   error
}

// Error implements the error interface.
func (e *Error) Error() string {
	if errors.Is(e.Err, ErrEmptyKey) {
		return "pipeline constant: " + e.Err.Error()
	}
	return fmt.Sprintf("pipeline constant %q: %v, got %v", e.Key, e.Err, e.Value)
}

// Unwrap returns Err, for errors.Is.
func (e *Error) Unwrap() error { return e.Err }

// Validate returns an *Error for the first invalid constant, keys in sorted
// order so the same input reports the same key, or nil.
func Validate(constants map[string]float64) error {
	if len(constants) == 0 {
		return nil
	}
	keys := make([]string, 0, len(constants))
	for k := range constants {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		v := constants[k]
		if k == "" {
			return &Error{Key: k, Value: v, Err: ErrEmptyKey}
		}
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return &Error{Key: k, Value: v, Err: ErrNotFinite}
		}
	}
	return nil
}
