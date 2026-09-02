//go:build !(js && wasm)

package testutil

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/core"
)

// AssertFeatureError asserts err is a *core.FeatureError for the given feature and resource.
func AssertFeatureError(t *testing.T, err error, feature gputypes.Feature, resource string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected feature error, got nil")
	}
	var fe *core.FeatureError
	if !errors.As(err, &fe) {
		t.Fatalf("expected *core.FeatureError, got %T: %v", err, err)
	}
	if fe.Feature != feature.String() {
		t.Errorf("Feature = %q, want %q", fe.Feature, feature.String())
	}
	if fe.Resource != resource {
		t.Errorf("Resource = %q, want %q", fe.Resource, resource)
	}
}

// AssertNoError fails the test if err is non-nil.
func AssertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
