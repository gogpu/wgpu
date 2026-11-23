// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package main

import (
	"bufio"
	"fmt"
	"io/fs"
	"path"
	"strconv"
	"strings"
)

// Vertex mirrors the Rust example's #[repr(C)] Vertex (48 bytes), which
// matches the WGSL Vertex struct layout (vec3 members are 16-byte aligned).
type Vertex struct {
	Pos    [3]float32
	_      uint32
	Normal [3]float32
	_      uint32
	UV     [2]float32
	_      [2]uint32
}

// Material mirrors the Rust example's Material (32 bytes).
type Material struct {
	RoughnessExponent float32
	Metalness         float32
	Specularity       float32
	_                 uint32
	Albedo            [3]float32
	_                 uint32
}

// indexRange is a half-open range, the Go stand-in for Rust's Range<usize>.
type indexRange struct{ start, end int }

// RawSceneComponents mirrors the Rust struct of the same name.
type RawSceneComponents struct {
	vertices   []Vertex
	indices    []uint32
	geometries []geometry // index range, material
	instances  []instance // vertex range, geometry range
}

type geometry struct {
	indices  indexRange
	material Material
}

type instance struct {
	vertices   indexRange
	geometries indexRange
}

// The types below reproduce just enough of the Rust `obj` crate (0.10) for
// load_model: objects contain groups; a group has polygons and an optional
// material. Group boundaries follow that crate's rules, because they decide
// how many BLAS geometries each model gets:
//   - "o" pushes the current group (if any) into the current object, pushes
//     the object, and starts a new object.
//   - "usemtl" pushes a copy of the current group if it already has polygons,
//     then sets the material on the (possibly new, "default") current group.
//   - "f" appends to the current group, creating a "default" group with no
//     material if there is none.
//   - At EOF the current group (even if empty) and object are pushed.
type objIndexTuple struct {
	position int
	texture  int // -1 when absent
	normal   int // -1 when absent
}

type objGroup struct {
	material string // "" when none
	polys    [][]objIndexTuple
}

type objObject struct {
	groups []objGroup
}

type objData struct {
	position  [][3]float32
	texture   [][2]float32
	normal    [][3]float32
	objects   []objObject
	materials map[string]objMtl
}

type objMtl struct {
	kd, ka, ks *[3]float32
	ns         *float32
}

// loadObj parses name (and any mtllib it references, relative to name) from fsys.
func loadObj(fsys fs.FS, name string) (*objData, error) { //nolint:gocognit,gocyclo,cyclop,funlen // line-oriented OBJ parser, kept close to tobj
	f, err := fsys.Open(name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	data := &objData{materials: map[string]objMtl{}}
	var object objObject
	var group *objGroup
	var mtllibs []string

	sc := bufio.NewScanner(f)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		words := strings.Fields(sc.Text())
		if len(words) == 0 || strings.HasPrefix(words[0], "#") {
			continue
		}
		args := words[1:]
		switch words[0] {
		case "v":
			v, err := parseFloats3(args)
			if err != nil {
				return nil, fmt.Errorf("%s:%d: %w", name, lineNo, err)
			}
			data.position = append(data.position, v)
		case "vt":
			if len(args) < 2 {
				return nil, fmt.Errorf("%s:%d: vt needs 2 components", name, lineNo)
			}
			u, err1 := strconv.ParseFloat(args[0], 32)
			v, err2 := strconv.ParseFloat(args[1], 32)
			if err1 != nil || err2 != nil {
				return nil, fmt.Errorf("%s:%d: bad vt", name, lineNo)
			}
			data.texture = append(data.texture, [2]float32{float32(u), float32(v)})
		case "vn":
			n, err := parseFloats3(args)
			if err != nil {
				return nil, fmt.Errorf("%s:%d: %w", name, lineNo, err)
			}
			data.normal = append(data.normal, n)
		case "f":
			poly := make([]objIndexTuple, 0, len(args))
			for _, a := range args {
				t, err := data.parseIndexTuple(a)
				if err != nil {
					return nil, fmt.Errorf("%s:%d: %w", name, lineNo, err)
				}
				poly = append(poly, t)
			}
			if group == nil {
				group = &objGroup{}
			}
			group.polys = append(group.polys, poly)
		case "o":
			if group != nil {
				object.groups = append(object.groups, *group)
				group = nil
				data.objects = append(data.objects, object)
			}
			object = objObject{}
		case "usemtl":
			if group == nil {
				group = &objGroup{}
			} else if len(group.polys) > 0 {
				object.groups = append(object.groups, *group)
				group = &objGroup{}
			}
			group.material = ""
			if len(args) > 0 {
				group.material = args[0]
			}
		case "mtllib":
			mtllibs = append(mtllibs, args...)
		default:
			// s, g, l, ... are not used by the example models.
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if group != nil {
		object.groups = append(object.groups, *group)
	}
	data.objects = append(data.objects, object)

	for _, lib := range mtllibs {
		if err := data.loadMtl(fsys, path.Join(path.Dir(name), lib)); err != nil {
			return nil, err
		}
	}
	return data, nil
}

// parseIndexTuple parses "p", "p/t", "p//n" or "p/t/n" with 1-based or
// negative (relative) indices. Zero texture/normal indices are ignored, as in
// the obj crate.
func (d *objData) parseIndexTuple(s string) (objIndexTuple, error) {
	parts := strings.Split(s, "/")
	t := objIndexTuple{texture: -1, normal: -1}
	idx := func(i int, n int) (int, error) {
		if i >= len(parts) || parts[i] == "" {
			return -1, nil
		}
		v, err := strconv.Atoi(parts[i])
		if err != nil {
			return -1, fmt.Errorf("bad index %q", s)
		}
		switch {
		case v > 0:
			return v - 1, nil
		case v < 0:
			return n + v, nil
		default:
			return -1, nil
		}
	}
	var err error
	if t.position, err = idx(0, len(d.position)); err != nil {
		return t, err
	}
	if t.position < 0 {
		return t, fmt.Errorf("zero or missing position index in %q", s)
	}
	if t.texture, err = idx(1, len(d.texture)); err != nil {
		return t, err
	}
	if t.normal, err = idx(2, len(d.normal)); err != nil {
		return t, err
	}
	return t, nil
}

func (d *objData) loadMtl(fsys fs.FS, name string) error {
	f, err := fsys.Open(name)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	var cur string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		words := strings.Fields(sc.Text())
		if len(words) == 0 || strings.HasPrefix(words[0], "#") {
			continue
		}
		if words[0] == "newmtl" {
			if len(words) > 1 {
				cur = words[1]
			}
			d.materials[cur] = objMtl{}
			continue
		}
		m := d.materials[cur]
		switch words[0] {
		case "Kd", "Ka", "Ks":
			v, err := parseFloats3(words[1:])
			if err != nil {
				return fmt.Errorf("%s: %s: %w", name, words[0], err)
			}
			switch words[0] {
			case "Kd":
				m.kd = &v
			case "Ka":
				m.ka = &v
			case "Ks":
				m.ks = &v
			}
		case "Ns":
			if len(words) < 2 {
				return fmt.Errorf("%s: Ns needs a value", name)
			}
			v, err := strconv.ParseFloat(words[1], 32)
			if err != nil {
				return fmt.Errorf("%s: Ns: %w", name, err)
			}
			ns := float32(v)
			m.ns = &ns
		}
		d.materials[cur] = m
	}
	return sc.Err()
}

func parseFloats3(args []string) ([3]float32, error) {
	var out [3]float32
	if len(args) < 3 {
		return out, fmt.Errorf("expected 3 components, got %d", len(args))
	}
	for i := range 3 {
		v, err := strconv.ParseFloat(args[i], 32)
		if err != nil {
			return out, err
		}
		out[i] = float32(v)
	}
	return out, nil
}

// loadModel is a port of the Rust example's load_model.
func loadModel(scene *RawSceneComponents, fsys fs.FS, name string) error { //nolint:gocognit // mirrors the Rust example's load_model
	data, err := loadObj(fsys, name)
	if err != nil {
		return err
	}

	startVertexIndex := len(scene.vertices)
	startGeometryIndex := len(scene.geometries)

	mapping := map[objIndexTuple]int{}
	nextIndex := 0

	for _, object := range data.objects {
		for _, group := range object.groups {
			startIndexIndex := len(scene.indices)
			for _, poly := range group.polys {
				for endIndex := 2; endIndex < len(poly); endIndex++ {
					for _, i := range [3]int{0, endIndex - 1, endIndex} {
						tuple := poly[i]
						var uv [2]float32
						if tuple.texture >= 0 {
							uv = data.texture[tuple.texture]
						}
						if tuple.normal < 0 {
							return fmt.Errorf("%s: normals required", name)
						}

						index, ok := mapping[tuple]
						if !ok {
							index = nextIndex
							mapping[tuple] = index
							nextIndex++

							scene.vertices = append(scene.vertices, Vertex{
								Pos:    data.position[tuple.position],
								UV:     uv,
								Normal: data.normal[tuple.normal],
							})
						}

						scene.indices = append(scene.indices, uint32(index))
					}
				}
			}

			var material Material
			// Rust only uses materials that resolved to an MTL entry.
			if mat, ok := data.materials[group.material]; ok && group.material != "" { //nolint:nestif // material fields map one-to-one onto the Rust example
				if mat.kd != nil {
					material.Albedo = *mat.kd
				}
				if mat.ns != nil {
					material.RoughnessExponent = *mat.ns
				}
				if mat.ka != nil {
					material.Metalness = mat.ka[0]
				}
				if mat.ks != nil {
					material.Specularity = mat.ks[0]
				}
			}

			scene.geometries = append(scene.geometries, geometry{
				indices:  indexRange{startIndexIndex, len(scene.indices)},
				material: material,
			})
		}
	}
	scene.instances = append(scene.instances, instance{
		vertices:   indexRange{startVertexIndex, len(scene.vertices)},
		geometries: indexRange{startGeometryIndex, len(scene.geometries)},
	})
	return nil
}

// loadRawScene is the CPU half of the Rust example's load_scene: it loads the
// two models, in the same order, from fsys (rooted at the models directory).
func loadRawScene(fsys fs.FS) (*RawSceneComponents, error) {
	scene := &RawSceneComponents{}
	if err := loadModel(scene, fsys, "rustacean-3d.obj"); err != nil {
		return nil, err
	}
	if err := loadModel(scene, fsys, "cube.obj"); err != nil {
		return nil, err
	}
	return scene, nil
}
