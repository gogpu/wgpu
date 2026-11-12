//go:build !(js && wasm)

package wgputestbridge

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
)

func TestHooksRecordPublicSemantics(t *testing.T) {
	var (
		seqs     []uint64
		encoders []int
		passes   []int
		binds    []uint32
		draws    []uint32
		submits  []int
	)
	observer := Observer{
		EncoderCreated: func(seq uint64, index int, label string) {
			if label != "frame" {
				t.Errorf("encoder label = %q, want frame", label)
			}
			seqs = append(seqs, seq)
			encoders = append(encoders, index)
		},
		PassBegan: func(seq uint64, pass, encoder int, compute bool, label string, loadOps []gputypes.LoadOp, clears []gputypes.Color) {
			if compute {
				t.Error("render pass recorded as compute")
			}
			if label != "main" {
				t.Errorf("pass label = %q, want main", label)
			}
			if len(loadOps) != 1 || loadOps[0] != gputypes.LoadOpClear {
				t.Errorf("load ops = %v, want [Clear]", loadOps)
			}
			if len(clears) != 1 || clears[0].R != 1 {
				t.Errorf("clears = %+v, want one red", clears)
			}
			if len(encoders) != 1 || encoder != encoders[0] {
				t.Errorf("pass encoder = %d, want the user encoder %v", encoder, encoders)
			}
			seqs = append(seqs, seq)
			passes = append(passes, pass)
		},
		BindGroupBound: func(seq uint64, pass int, index uint32, offsets []uint32) {
			if len(passes) != 1 || pass != passes[0] || index != 0 || len(offsets) != 1 || offsets[0] != 16 {
				t.Errorf("bind = pass %d index %d offsets %v", pass, index, offsets)
			}
			seqs = append(seqs, seq)
			binds = append(binds, index)
		},
		VertexBufferBound: func(seq uint64, pass int, slot uint32, label string, offset, size uint64) {
			if len(passes) != 1 || pass != passes[0] || slot != 0 || label != "vertex" || offset != 8 || size != 64 {
				t.Errorf("vertex bind = pass %d slot %d label %q offset %d size %d", pass, slot, label, offset, size)
			}
			seqs = append(seqs, seq)
		},
		IndexBufferBound: func(seq uint64, pass int, label string, format gputypes.IndexFormat, offset, size uint64) {
			if len(passes) != 1 || pass != passes[0] || label != "index" ||
				format != gputypes.IndexFormatUint16 || offset != 4 || size != 64 {
				t.Errorf("index bind = pass %d label %q format %v offset %d size %d", pass, label, format, offset, size)
			}
			seqs = append(seqs, seq)
		},
		Drew: func(seq uint64, pass int, args gputypes.DrawArgs) {
			if len(passes) != 1 || pass != passes[0] || args.VertexCount != 3 {
				t.Errorf("draw = pass %d args %+v", pass, args)
			}
			seqs = append(seqs, seq)
			draws = append(draws, args.VertexCount)
		},
		Submitted: func(seq uint64, index uint64, buffers int) {
			if index != 7 || buffers != 1 {
				t.Errorf("submit = index %d buffers %d, want 7 and 1", index, buffers)
			}
			seqs = append(seqs, seq)
			submits = append(submits, buffers)
		},
	}

	hooks := NewHooks(observer, Faults{})
	encoder := hooks.EncoderCreated("frame")
	pass := hooks.PassBegan(encoder, false, "main", []gputypes.LoadOp{gputypes.LoadOpClear}, []gputypes.Color{{R: 1, A: 1}})
	hooks.BindGroupBound(pass, 0, []uint32{16})
	hooks.VertexBufferBound(pass, 0, "vertex", 8, 64)
	hooks.IndexBufferBound(pass, "index", gputypes.IndexFormatUint16, 4, 64)
	hooks.Drew(pass, gputypes.DrawArgs{VertexCount: 3})
	hooks.Submitted(7, 1)

	if len(encoders) != 1 || encoders[0] != 0 {
		t.Errorf("encoders = %v, want one at index 0", encoders)
	}
	if len(passes) != 1 || passes[0] != 0 {
		t.Errorf("passes = %v, want one at index 0", passes)
	}
	if len(binds) != 1 || len(draws) != 1 || len(submits) != 1 {
		t.Errorf("binds %v, draws %v, submits %v", binds, draws, submits)
	}
	for i := 1; i < len(seqs); i++ {
		if seqs[i] <= seqs[i-1] {
			t.Fatalf("seq not strictly increasing: %v", seqs)
		}
	}
}

func TestHooksFaults(t *testing.T) {
	encoderErr := errors.New("encoder refused")
	writeErr := errors.New("write refused")
	hooks := NewHooks(Observer{}, Faults{CreateCommandEncoder: encoderErr, WriteBuffer: writeErr})
	if err := hooks.FaultCreateCommandEncoder(); !errors.Is(err, encoderErr) {
		t.Errorf("encoder fault = %v, want %v", err, encoderErr)
	}
	if err := hooks.FaultWriteBuffer(); !errors.Is(err, writeErr) {
		t.Errorf("write fault = %v, want %v", err, writeErr)
	}
}
