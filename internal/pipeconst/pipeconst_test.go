package pipeconst

import (
	"errors"
	"math"
	"testing"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name      string
		constants map[string]float64
		wantErr   error
		wantKey   string
		wantText  string
	}{
		{name: "nil", constants: nil},
		{name: "empty map", constants: map[string]float64{}},
		{name: "finite values", constants: map[string]float64{"a": 1, "b": -2.5, "0": 0, "big": math.MaxFloat64}},
		{name: "empty key", constants: map[string]float64{"": 1}, wantErr: ErrEmptyKey, wantText: "pipeline constant: key must not be empty"},
		{name: "NaN", constants: map[string]float64{"x": math.NaN()}, wantErr: ErrNotFinite, wantKey: "x", wantText: `pipeline constant "x": value must be finite, got NaN`},
		{name: "+Inf", constants: map[string]float64{"x": math.Inf(1)}, wantErr: ErrNotFinite, wantKey: "x", wantText: `pipeline constant "x": value must be finite, got +Inf`},
		{name: "-Inf", constants: map[string]float64{"x": math.Inf(-1)}, wantErr: ErrNotFinite, wantKey: "x"},
		// Sorted order: "a" is reported before "b" whatever the map order.
		{name: "first in key order", constants: map[string]float64{"b": math.NaN(), "a": math.Inf(1), "c": 1}, wantErr: ErrNotFinite, wantKey: "a"},
		{name: "empty key sorts first", constants: map[string]float64{"a": math.NaN(), "": 0}, wantErr: ErrEmptyKey, wantKey: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.constants)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("Validate = %v, want nil", err)
				}
				return
			}
			checkError(t, err, tt.wantErr, tt.wantKey, tt.wantText)
		})
	}
}

func checkError(t *testing.T, err, want error, key, text string) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("Validate = %v, want %v", err, want)
	}
	var pe *Error
	if !errors.As(err, &pe) {
		t.Fatalf("Validate = %T, want *Error", err)
	}
	if pe.Key != key {
		t.Errorf("Key = %q, want %q", pe.Key, key)
	}
	if text != "" && err.Error() != text {
		t.Errorf("Error() = %q, want %q", err.Error(), text)
	}
}
