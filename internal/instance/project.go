// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package instance

// ProjectAdapters maps implementation adapters through wrap and returns the
// projected slice. The loop lives here, not in the public declaration layer:
// internal cannot name the public Adapter type (the root imports internal, not
// the reverse — ADR-070 §15.1, R5), so root passes a method value as wrap and
// keeps its method a single delegation with no construct the public-layer rule
// bans. An empty list projects to nil.
func ProjectAdapters[T any](impls []*Adapter, wrap func(*Adapter) T) []T {
	if len(impls) == 0 {
		return nil
	}
	out := make([]T, len(impls))
	for i, impl := range impls {
		out[i] = wrap(impl)
	}
	return out
}
