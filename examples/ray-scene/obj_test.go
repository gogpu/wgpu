// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"testing"
	"testing/fstest"
	"unsafe"
)

func TestStructLayoutsMatchWGSL(t *testing.T) {
	if s := unsafe.Sizeof(Vertex{}); s != 48 {
		t.Errorf("Vertex size %d, want 48", s)
	}
	if s := unsafe.Sizeof(Material{}); s != 32 {
		t.Errorf("Material size %d, want 32", s)
	}
}

func TestObjGrouping(t *testing.T) {
	fsys := fstest.MapFS{
		"m.obj": {Data: []byte(`mtllib m.mtl
o A
usemtl red
v 0 0 0
v 1 0 0
v 1 1 0
v 0 1 0
vn 0 0 1
f 1//1 2//1 3//1 4//1
usemtl blue
usemtl red
f 1//1 3//1 4//1
o B
f -1//1 -2//1 -3//1
`)},
		"m.mtl": {Data: []byte("newmtl red\nKd 1 0 0\nNs 10\nKa 0.5 0 0\nKs 0.25 0 0\nnewmtl blue\nKd 0 0 1\n")},
	}
	var scene RawSceneComponents
	if err := loadModel(&scene, fsys, "m.obj"); err != nil {
		t.Fatal(err)
	}
	// Object A: "usemtl red" opens group 1; the quad fans into 2 triangles.
	// "usemtl blue" splits off group 1; the following "usemtl red" just
	// renames the still-empty group 2, which gets 1 triangle. Object B has a
	// default group with no material and 1 triangle.
	if len(scene.geometries) != 3 {
		t.Fatalf("geometries = %d, want 3: %+v", len(scene.geometries), scene.geometries)
	}
	wantIdx := []int{6, 3, 3}
	for i, g := range scene.geometries {
		if n := g.indices.end - g.indices.start; n != wantIdx[i] {
			t.Errorf("geometry %d has %d indices, want %d", i, n, wantIdx[i])
		}
	}
	red := scene.geometries[0].material
	if red.Albedo != [3]float32{1, 0, 0} || red.RoughnessExponent != 10 || red.Metalness != 0.5 || red.Specularity != 0.25 {
		t.Errorf("red material = %+v", red)
	}
	if scene.geometries[2].material != (Material{}) {
		t.Errorf("default-group material = %+v, want zero", scene.geometries[2].material)
	}
	// Fan triangulation order is (0, end-1, end).
	if got := scene.indices[:6]; got[0] != 0 || got[1] != 1 || got[2] != 2 || got[3] != 0 || got[4] != 2 || got[5] != 3 {
		t.Errorf("fan indices = %v", got)
	}
	// Vertices are de-duplicated by (position, texture, normal) per model.
	if len(scene.vertices) != 4 {
		t.Errorf("vertices = %d, want 4", len(scene.vertices))
	}
	if len(scene.instances) != 1 || scene.instances[0].geometries != (indexRange{0, 3}) {
		t.Errorf("instances = %+v", scene.instances)
	}
}

// TestExampleModels loads the real example models in the same order as the
// Rust example and checks that every geometry is a whole number of triangles.
func TestExampleModels(t *testing.T) {
	scene, err := loadRawScene(os.DirFS("models"))
	if err != nil {
		t.Fatal(err)
	}
	if len(scene.instances) != 2 {
		t.Fatalf("instances = %d, want 2", len(scene.instances))
	}
	// rustacean-3d.obj: one object, four usemtl groups. cube.obj: two objects.
	if got := scene.instances[0].geometries; got != (indexRange{0, 4}) {
		t.Errorf("ferris geometries = %+v, want {0 4}", got)
	}
	if got := scene.instances[1].geometries; got != (indexRange{4, 6}) {
		t.Errorf("cube geometries = %+v, want {4 6}", got)
	}
	for i, g := range scene.geometries {
		n := g.indices.end - g.indices.start
		if n == 0 || n%3 != 0 {
			t.Errorf("geometry %d has %d indices", i, n)
		}
	}
	for i, inst := range scene.instances {
		nv := uint32(inst.vertices.end - inst.vertices.start)
		for g := inst.geometries.start; g < inst.geometries.end; g++ {
			r := scene.geometries[g].indices
			for _, idx := range scene.indices[r.start:r.end] {
				if idx >= nv {
					t.Fatalf("instance %d geometry %d: index %d out of range %d", i, g, idx, nv)
				}
			}
		}
	}
}
