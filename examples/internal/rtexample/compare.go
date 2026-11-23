// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package rtexample

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

// DefaultMeanThreshold matches the Rust examples' ComparisonType::Mean(0.02).
const DefaultMeanThreshold = 0.02

// MeanDifference returns the mean absolute per-channel difference between
// the RGB channels of a and b, normalized to [0, 1]. Alpha is ignored, as in
// the Rust framework (which strips alpha before comparing).
//
// The Rust framework computes the mean of an NVIDIA FLIP error map instead.
// FLIP is perceptual and generally forgives small differences more than a
// plain per-channel mean does, so this check tends to be at least as strict.
func MeanDifference(a, b image.Image) (float64, error) {
	if a.Bounds().Size() != b.Bounds().Size() {
		return 0, fmt.Errorf("image sizes differ: %v vs %v", a.Bounds().Size(), b.Bounds().Size())
	}
	ab, bb := a.Bounds(), b.Bounds()
	var sum float64
	n := 0
	for y := range ab.Dy() {
		for x := range ab.Dx() {
			r1, g1, b1, _ := a.At(ab.Min.X+x, ab.Min.Y+y).RGBA()
			r2, g2, b2, _ := b.At(bb.Min.X+x, bb.Min.Y+y).RGBA()
			sum += math.Abs(float64(r1)-float64(r2)) +
				math.Abs(float64(g1)-float64(g2)) +
				math.Abs(float64(b1)-float64(b2))
			n += 3
		}
	}
	if n == 0 {
		return 0, nil
	}
	return sum / float64(n) / 0xffff, nil
}

// ComparePNG decodes golden (PNG bytes) and compares it to img, failing if
// the mean difference exceeds threshold.
func ComparePNG(img image.Image, golden []byte, threshold float64) error {
	ref, err := png.Decode(bytes.NewReader(golden))
	if err != nil {
		return fmt.Errorf("decode golden image: %w", err)
	}
	mean, err := MeanDifference(img, ref)
	if err != nil {
		return err
	}
	fmt.Printf("Mean difference vs golden: %.6f (threshold %.6f)\n", mean, threshold)
	if mean > threshold {
		return fmt.Errorf("mean difference %.6f exceeds threshold %.6f", mean, threshold)
	}
	return nil
}

// WritePNG encodes img to path.
func WritePNG(path string, img image.Image) error {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return fmt.Errorf("encode png: %w", err)
	}
	// Write through os.Root so the file stays in the directory the caller
	// named (gosec G703), as examples/triangle-headless does.
	path = filepath.Clean(path)
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return fmt.Errorf("open output directory: %w", err)
	}
	writeErr := root.WriteFile(filepath.Base(path), buf.Bytes(), 0o600)
	closeErr := root.Close()
	if writeErr != nil {
		return fmt.Errorf("write png: %w", writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close output directory: %w", closeErr)
	}
	fmt.Printf("PNG written: %s (%d bytes)\n", path, buf.Len())
	return nil
}
