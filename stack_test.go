// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package quetzal_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"reflect"
	"testing"

	"github.com/maloquacious/quetzal"
)

// frameHeaderSize is the fixed part of a frame, so that tests can index into
// an encoded chunk.
const frameHeaderSize = 8

// frameBytes builds one Stks frame. The local count is taken from the locals
// supplied, so that the two cannot disagree; extraFlags carries whatever else
// belongs in the flags byte, including bits the encoder would never set.
func frameBytes(pc uint32, extraFlags, result, arguments byte, locals, evaluation []uint16) []byte {
	b := []byte{byte(pc >> 16), byte(pc >> 8), byte(pc)}
	b = append(b, extraFlags|byte(len(locals)), result, arguments)
	b = binary.BigEndian.AppendUint16(b, uint16(len(evaluation)))
	for _, w := range locals {
		b = binary.BigEndian.AppendUint16(b, w)
	}
	for _, w := range evaluation {
		b = binary.BigEndian.AppendUint16(b, w)
	}
	return b
}

// dummyFrameBytes builds the dummy first frame that a save for any version
// other than 6 must begin with.
func dummyFrameBytes(evaluation []uint16) []byte {
	return frameBytes(0, 0, 0, 0, nil, evaluation)
}

func TestDecodeStks(t *testing.T) {
	payload := bytes.Join([][]byte{
		dummyFrameBytes([]uint16{0x1111}),
		frameBytes(0x012345, 0, 0x10, 0x03, []uint16{0xaaaa, 0xbbbb}, []uint16{0x0001, 0x0002}),
		frameBytes(0x00ffff, 0x10, 0x00, 0x7f, nil, nil),
	}, nil)

	frames, err := quetzal.DecodeStks(payload, quetzal.Limits{})
	if err != nil {
		t.Fatalf("DecodeStks: unexpected error: %v", err)
	}
	if len(frames) != 3 {
		t.Fatalf("got %d frame(s), want 3", len(frames))
	}

	t.Run("dummy frame", func(t *testing.T) {
		f := frames[0]
		if !f.IsDummy() {
			t.Errorf("frame 0 is not the dummy frame: %+v", f)
		}
		if want := []uint16{0x1111}; !reflect.DeepEqual(f.Evaluation, want) {
			t.Errorf("Evaluation: got %v, want %v", f.Evaluation, want)
		}
	})

	t.Run("call frame", func(t *testing.T) {
		f := frames[1]
		if f.ReturnPC != 0x012345 {
			t.Errorf("ReturnPC: got %#x, want %#x", f.ReturnPC, 0x012345)
		}
		if f.DiscardResult {
			t.Error("DiscardResult: got true, want false")
		}
		if f.ResultVariable != 0x10 {
			t.Errorf("ResultVariable: got %#02x, want %#02x", f.ResultVariable, 0x10)
		}
		if f.Arguments != 0x03 {
			t.Errorf("Arguments: got %#02x, want %#02x", f.Arguments, 0x03)
		}
		if want := []uint16{0xaaaa, 0xbbbb}; !reflect.DeepEqual(f.Locals, want) {
			t.Errorf("Locals: got %v, want %v", f.Locals, want)
		}
		if want := []uint16{0x0001, 0x0002}; !reflect.DeepEqual(f.Evaluation, want) {
			t.Errorf("Evaluation: got %v, want %v", f.Evaluation, want)
		}
	})

	t.Run("frame that discards its result", func(t *testing.T) {
		f := frames[2]
		if !f.DiscardResult {
			t.Error("DiscardResult: got false, want true")
		}
		if len(f.Locals) != 0 {
			t.Errorf("Locals: got %v, want none", f.Locals)
		}
		if f.Arguments != 0x7f {
			t.Errorf("Arguments: got %#02x, want %#02x (all seven bits)", f.Arguments, 0x7f)
		}
		// The dummy frame test must not match a real frame that happens to
		// have no locals.
		if f.IsDummy() {
			t.Error("IsDummy: got true for a call frame")
		}
	})
}

func TestDecodeStksEvaluationOrder(t *testing.T) {
	// Words are stored least recent first, and the slice keeps that order,
	// so the top of the stack is the last element.
	payload := frameBytes(0, 0, 0, 0, nil, []uint16{1, 2, 3})

	frames, err := quetzal.DecodeStks(payload, quetzal.Limits{})
	if err != nil {
		t.Fatalf("DecodeStks: unexpected error: %v", err)
	}
	eval := frames[0].Evaluation
	if eval[0] != 1 {
		t.Errorf("Evaluation[0]: got %d, want 1 (the least recent word)", eval[0])
	}
	if eval[len(eval)-1] != 3 {
		t.Errorf("Evaluation[%d]: got %d, want 3 (the top of the stack)", len(eval)-1, eval[len(eval)-1])
	}
}

func TestDecodeStksEmpty(t *testing.T) {
	// An empty chunk holds no frames. Whether that is legal depends on the
	// story's version, which is ValidateFrames' business.
	frames, err := quetzal.DecodeStks(nil, quetzal.Limits{})
	if err != nil {
		t.Fatalf("DecodeStks: unexpected error: %v", err)
	}
	if len(frames) != 0 {
		t.Errorf("got %d frame(s), want none", len(frames))
	}
}

func TestDecodeStksMaximumFrame(t *testing.T) {
	// Fifteen locals is the most a four-bit count can express.
	locals := make([]uint16, quetzal.MaxLocals)
	for i := range locals {
		locals[i] = uint16(0x1000 + i)
	}
	payload := frameBytes(quetzal.MaxPC, 0x10, 0, 0x7f, locals, []uint16{9})

	frames, err := quetzal.DecodeStks(payload, quetzal.Limits{})
	if err != nil {
		t.Fatalf("DecodeStks: unexpected error: %v", err)
	}
	f := frames[0]
	if f.ReturnPC != quetzal.MaxPC {
		t.Errorf("ReturnPC: got %#x, want %#x", f.ReturnPC, uint32(quetzal.MaxPC))
	}
	if !f.DiscardResult {
		t.Error("DiscardResult: got false, want true")
	}
	if !reflect.DeepEqual(f.Locals, locals) {
		t.Errorf("Locals: got %v, want %v", f.Locals, locals)
	}
}

func TestDecodeStksMalformed(t *testing.T) {
	valid := frameBytes(0x1234, 0, 0, 0x01, []uint16{1, 2}, []uint16{3})

	tests := []struct {
		name    string
		payload []byte
		want    error
	}{
		{
			name:    "frame header cut short",
			payload: valid[:5],
			want:    quetzal.ErrTruncated,
		},
		{
			name:    "frame body cut short",
			payload: valid[:len(valid)-2],
			want:    quetzal.ErrTruncated,
		},
		{
			name: "a second frame that is cut short",
			// The first frame is whole, so the error must name the second.
			payload: append(append([]byte(nil), valid...), valid[:3]...),
			want:    quetzal.ErrTruncated,
		},
		{
			name: "evaluation stack larger than the chunk",
			// The count claims 4096 words in an eight-byte frame.
			payload: []byte{0, 0, 0, 0x00, 0, 0, 0x10, 0x00},
			want:    quetzal.ErrTruncated,
		},
		{
			name:    "flags byte sets an undefined bit",
			payload: frameBytes(0, 0x20, 0, 0, nil, nil),
			want:    quetzal.ErrInvalidFormat,
		},
		{
			name:    "flags byte sets the top bit",
			payload: frameBytes(0, 0x80, 0, 0, nil, nil),
			want:    quetzal.ErrInvalidFormat,
		},
		{
			name: "arguments mask sets an eighth bit",
			// Only seven arguments are possible, so the eighth bit is not
			// defined.
			payload: frameBytes(0, 0, 0, 0x80, nil, nil),
			want:    quetzal.ErrInvalidFormat,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frames, err := quetzal.DecodeStks(tt.payload, quetzal.Limits{})
			if err == nil {
				t.Fatalf("DecodeStks: got %d frame(s) and no error, want %v", len(frames), tt.want)
			}
			if !errors.Is(err, tt.want) {
				t.Errorf("DecodeStks: got %v, want an error matching %v", err, tt.want)
			}

			// An error must say which frame it belongs to.
			var fe *quetzal.FrameError
			if !errors.As(err, &fe) {
				t.Fatalf("DecodeStks: got %T, want a *FrameError", err)
			}
			if msg := err.Error(); !bytes.HasPrefix([]byte(msg), []byte("quetzal: Stks frame ")) {
				t.Errorf("error message %q does not name the chunk and frame", msg)
			}
		})
	}
}

func TestDecodeStksFrameErrorIndex(t *testing.T) {
	// The index must count frames, not bytes, so that it points at the
	// frame a caller would have to look at.
	good := frameBytes(1, 0, 0, 0, nil, nil)
	payload := bytes.Join([][]byte{good, good, {0x00, 0x00}}, nil)

	_, err := quetzal.DecodeStks(payload, quetzal.Limits{})
	var fe *quetzal.FrameError
	if !errors.As(err, &fe) {
		t.Fatalf("DecodeStks: got %v, want a *FrameError", err)
	}
	if fe.Index != 2 {
		t.Errorf("Index: got %d, want 2", fe.Index)
	}
}

func TestDecodeStksLimits(t *testing.T) {
	frame := frameBytes(1, 0, 0, 0, []uint16{7}, []uint16{8})
	payload := bytes.Join([][]byte{frame, frame, frame}, nil)

	t.Run("frame count", func(t *testing.T) {
		_, err := quetzal.DecodeStks(payload, quetzal.Limits{MaxFrames: 2})
		if !errors.Is(err, quetzal.ErrLimitExceeded) {
			t.Errorf("DecodeStks: got %v, want ErrLimitExceeded", err)
		}
	})

	t.Run("stack words", func(t *testing.T) {
		// Each frame holds one local and one evaluation word.
		_, err := quetzal.DecodeStks(payload, quetzal.Limits{MaxStackWords: 5})
		if !errors.Is(err, quetzal.ErrLimitExceeded) {
			t.Errorf("DecodeStks: got %v, want ErrLimitExceeded", err)
		}
		if _, err := quetzal.DecodeStks(payload, quetzal.Limits{MaxStackWords: 6}); err != nil {
			t.Errorf("DecodeStks: unexpected error at the limit: %v", err)
		}
	})

	t.Run("zero fields take the defaults", func(t *testing.T) {
		if _, err := quetzal.DecodeStks(payload, quetzal.Limits{}); err != nil {
			t.Errorf("DecodeStks: unexpected error: %v", err)
		}
	})
}

func TestDecodeStksDoesNotAliasOrMutate(t *testing.T) {
	payload := frameBytes(0x0102, 0, 0x05, 0x01, []uint16{0x1234}, []uint16{0x5678})
	original := append([]byte(nil), payload...)

	frames, err := quetzal.DecodeStks(payload, quetzal.Limits{})
	if err != nil {
		t.Fatalf("DecodeStks: unexpected error: %v", err)
	}
	if !bytes.Equal(payload, original) {
		t.Error("DecodeStks modified the payload")
	}

	for i := range payload {
		payload[i] = 0xff
	}
	if frames[0].Locals[0] != 0x1234 || frames[0].Evaluation[0] != 0x5678 {
		t.Error("the decoded frame aliases the payload")
	}
}

func TestFrameValidate(t *testing.T) {
	tests := []struct {
		name  string
		frame quetzal.Frame
		ok    bool
	}{
		{
			name:  "an empty frame is the dummy frame and is valid",
			frame: quetzal.Frame{},
			ok:    true,
		},
		{
			name: "a full frame",
			frame: quetzal.Frame{
				ReturnPC:   quetzal.MaxPC,
				Arguments:  0x7f,
				Locals:     make([]uint16, quetzal.MaxLocals),
				Evaluation: make([]uint16, 100),
			},
			ok: true,
		},
		{
			name:  "return PC beyond 24 bits",
			frame: quetzal.Frame{ReturnPC: quetzal.MaxPC + 1},
		},
		{
			name:  "more locals than four bits can count",
			frame: quetzal.Frame{Locals: make([]uint16, quetzal.MaxLocals+1)},
		},
		{
			name:  "more evaluation words than one word can count",
			frame: quetzal.Frame{Evaluation: make([]uint16, quetzal.MaxEvaluationWords+1)},
		},
		{
			name:  "an arguments mask with an eighth bit",
			frame: quetzal.Frame{Arguments: 0x80},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.frame.Validate()
			if tt.ok {
				if err != nil {
					t.Fatalf("Validate: unexpected error: %v", err)
				}
				return
			}
			if !errors.Is(err, quetzal.ErrInvalidFormat) {
				t.Fatalf("Validate: got %v, want ErrInvalidFormat", err)
			}
			if msg := err.Error(); !bytes.HasPrefix([]byte(msg), []byte("quetzal: ")) {
				t.Errorf("error message %q lacks the package prefix", msg)
			}

			// The same problem must be reported when the frame is written,
			// and must then say which frame it was.
			_, err = quetzal.EncodeStks([]quetzal.Frame{{}, tt.frame})
			if !errors.Is(err, quetzal.ErrInvalidFormat) {
				t.Fatalf("EncodeStks: got %v, want ErrInvalidFormat", err)
			}
			var fe *quetzal.FrameError
			if !errors.As(err, &fe) {
				t.Fatalf("EncodeStks: got %T, want a *FrameError", err)
			}
			if fe.Index != 1 {
				t.Errorf("Index: got %d, want 1", fe.Index)
			}
		})
	}
}

func TestFrameIsDummy(t *testing.T) {
	tests := []struct {
		name  string
		frame quetzal.Frame
		want  bool
	}{
		{"empty", quetzal.Frame{}, true},
		{"with top-level evaluation stack", quetzal.Frame{Evaluation: []uint16{1, 2}}, true},
		{"with a return PC", quetzal.Frame{ReturnPC: 1}, false},
		{"discarding its result", quetzal.Frame{DiscardResult: true}, false},
		{"with a result variable", quetzal.Frame{ResultVariable: 1}, false},
		{"with arguments", quetzal.Frame{Arguments: 1}, false},
		{"with locals", quetzal.Frame{Locals: []uint16{0}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.frame.IsDummy(); got != tt.want {
				t.Errorf("IsDummy: got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEncodeStks(t *testing.T) {
	t.Run("round trip", func(t *testing.T) {
		frames := []quetzal.Frame{
			{Evaluation: []uint16{0x1111}},
			{
				ReturnPC:       0x012345,
				ResultVariable: 0x10,
				Arguments:      0x07,
				Locals:         []uint16{0xaaaa, 0xbbbb, 0xcccc},
				Evaluation:     []uint16{1, 2, 3, 4},
			},
			{ReturnPC: 0x00ffff, DiscardResult: true, Arguments: 0x01},
		}

		payload, err := quetzal.EncodeStks(frames)
		if err != nil {
			t.Fatalf("EncodeStks: unexpected error: %v", err)
		}
		back, err := quetzal.DecodeStks(payload, quetzal.Limits{})
		if err != nil {
			t.Fatalf("DecodeStks: unexpected error: %v", err)
		}
		if !reflect.DeepEqual(back, frames) {
			t.Errorf("round trip: got %+v, want %+v", back, frames)
		}
	})

	t.Run("frames keep their order", func(t *testing.T) {
		// The oldest frame is written first, so the top of the call stack
		// is the last frame in the chunk.
		frames := []quetzal.Frame{{}, {ReturnPC: 1}, {ReturnPC: 2}}

		payload, err := quetzal.EncodeStks(frames)
		if err != nil {
			t.Fatalf("EncodeStks: unexpected error: %v", err)
		}
		if payload[frameHeaderSize*2+2] != 2 {
			t.Errorf("the last frame written is not the newest: %x", payload)
		}
	})

	t.Run("a discarded result is written as zero", func(t *testing.T) {
		// The result variable carries no meaning when the p bit is set, so
		// the standard asks writers to zero it.
		frames := []quetzal.Frame{{DiscardResult: true, ResultVariable: 0x42}}

		payload, err := quetzal.EncodeStks(frames)
		if err != nil {
			t.Fatalf("EncodeStks: unexpected error: %v", err)
		}
		if payload[3] != 0x10 {
			t.Errorf("flags: got %#02x, want %#02x", payload[3], 0x10)
		}
		if payload[4] != 0 {
			t.Errorf("result variable: got %#02x, want 0", payload[4])
		}
	})

	t.Run("no frames encode to no bytes", func(t *testing.T) {
		payload, err := quetzal.EncodeStks(nil)
		if err != nil {
			t.Fatalf("EncodeStks: unexpected error: %v", err)
		}
		if len(payload) != 0 {
			t.Errorf("got %x, want no bytes", payload)
		}
	})

	t.Run("the frames are not modified", func(t *testing.T) {
		frames := []quetzal.Frame{{DiscardResult: true, ResultVariable: 0x42, Locals: []uint16{1}}}
		want := append([]quetzal.Frame(nil), frames...)

		if _, err := quetzal.EncodeStks(frames); err != nil {
			t.Fatalf("EncodeStks: unexpected error: %v", err)
		}
		if !reflect.DeepEqual(frames, want) {
			t.Error("EncodeStks modified the frames it was given")
		}
	})
}

func TestValidateFrames(t *testing.T) {
	v3, err := quetzal.ParseStory(storyImage(3, 1, "000000", 0, 0x40, 0x100))
	if err != nil {
		t.Fatalf("ParseStory: unexpected error: %v", err)
	}
	v6, err := quetzal.ParseStory(storyImage(6, 1, "000000", 0, 0x40, 0x100))
	if err != nil {
		t.Fatalf("ParseStory: unexpected error: %v", err)
	}

	dummy := quetzal.Frame{Evaluation: []uint16{1}}
	call := quetzal.Frame{ReturnPC: 0x100, Locals: []uint16{1}}

	t.Run("a version 3 save begins with the dummy frame", func(t *testing.T) {
		if err := quetzal.ValidateFrames([]quetzal.Frame{dummy, call}, v3); err != nil {
			t.Errorf("ValidateFrames: unexpected error: %v", err)
		}
	})

	t.Run("a version 3 save without the dummy frame is rejected", func(t *testing.T) {
		err := quetzal.ValidateFrames([]quetzal.Frame{call, call}, v3)
		if !errors.Is(err, quetzal.ErrInvalidFormat) {
			t.Fatalf("ValidateFrames: got %v, want ErrInvalidFormat", err)
		}
		var fe *quetzal.FrameError
		if !errors.As(err, &fe) || fe.Index != 0 {
			t.Errorf("ValidateFrames: got %v, want a *FrameError naming frame 0", err)
		}
	})

	t.Run("a version 3 save with no frames at all is rejected", func(t *testing.T) {
		if err := quetzal.ValidateFrames(nil, v3); !errors.Is(err, quetzal.ErrInvalidFormat) {
			t.Errorf("ValidateFrames: got %v, want ErrInvalidFormat", err)
		}
	})

	t.Run("version 6 needs no dummy frame", func(t *testing.T) {
		// Version 6 starts at a routine, so its first frame is a real call.
		if err := quetzal.ValidateFrames([]quetzal.Frame{call}, v6); err != nil {
			t.Errorf("ValidateFrames: unexpected error: %v", err)
		}
		if err := quetzal.ValidateFrames(nil, v6); err != nil {
			t.Errorf("ValidateFrames: unexpected error: %v", err)
		}
	})

	t.Run("a frame that cannot be represented is named", func(t *testing.T) {
		bad := quetzal.Frame{ReturnPC: quetzal.MaxPC + 1}
		err := quetzal.ValidateFrames([]quetzal.Frame{dummy, call, bad}, v3)
		if !errors.Is(err, quetzal.ErrInvalidFormat) {
			t.Fatalf("ValidateFrames: got %v, want ErrInvalidFormat", err)
		}
		var fe *quetzal.FrameError
		if !errors.As(err, &fe) {
			t.Fatalf("ValidateFrames: got %T, want a *FrameError", err)
		}
		if fe.Index != 2 {
			t.Errorf("Index: got %d, want 2", fe.Index)
		}
	})
}

func TestFileFrames(t *testing.T) {
	frames := []quetzal.Frame{
		{Evaluation: []uint16{0x1234}},
		{ReturnPC: 0x0abcde, ResultVariable: 3, Arguments: 0x03, Locals: []uint16{1, 2}},
	}
	payload, err := quetzal.EncodeStks(frames)
	if err != nil {
		t.Fatalf("EncodeStks: unexpected error: %v", err)
	}

	t.Run("decodes the Stks chunk", func(t *testing.T) {
		f := decodeIFZS(t, chunkBytes("IFhd", ifhdPayload), chunkBytes("Stks", payload))

		got, err := f.Frames()
		if err != nil {
			t.Fatalf("Frames: unexpected error: %v", err)
		}
		if !reflect.DeepEqual(got, frames) {
			t.Errorf("Frames: got %+v, want %+v", got, frames)
		}
	})

	t.Run("reports a missing Stks", func(t *testing.T) {
		f := decodeIFZS(t, chunkBytes("IFhd", ifhdPayload))
		if _, err := f.Frames(); !errors.Is(err, quetzal.ErrInvalidFormat) {
			t.Errorf("Frames: got %v, want ErrInvalidFormat", err)
		}
	})

	t.Run("reports a malformed Stks", func(t *testing.T) {
		f := decodeIFZS(t, chunkBytes("Stks", []byte{1, 2, 3}))
		if _, err := f.Frames(); !errors.Is(err, quetzal.ErrTruncated) {
			t.Errorf("Frames: got %v, want ErrTruncated", err)
		}
	})

	t.Run("honors the limits the file was decoded under", func(t *testing.T) {
		in := ifzs(chunkBytes("Stks", payload))

		f, err := quetzal.Decode(bytes.NewReader(in), quetzal.WithLimits(quetzal.Limits{MaxFrames: 1}))
		if err != nil {
			t.Fatalf("Decode: unexpected error: %v", err)
		}
		if _, err := f.Frames(); !errors.Is(err, quetzal.ErrLimitExceeded) {
			t.Errorf("Frames: got %v, want ErrLimitExceeded", err)
		}
	})

	t.Run("a hand-built file uses the default limits", func(t *testing.T) {
		f := &quetzal.File{Chunks: []quetzal.Chunk{{ID: quetzal.IDStks, Data: payload}}}

		got, err := f.Frames()
		if err != nil {
			t.Fatalf("Frames: unexpected error: %v", err)
		}
		if len(got) != len(frames) {
			t.Errorf("got %d frame(s), want %d", len(got), len(frames))
		}
	})
}

func FuzzStacks(f *testing.F) {
	f.Add([]byte{})
	f.Add(frameBytes(0, 0, 0, 0, nil, nil))
	f.Add(frameBytes(0x012345, 0x10, 0x10, 0x07, []uint16{1, 2, 3}, []uint16{4, 5}))
	f.Add(bytes.Join([][]byte{
		dummyFrameBytes([]uint16{1}),
		frameBytes(0x20, 0, 0, 0x01, []uint16{9, 9}, nil),
	}, nil))
	f.Add([]byte{0, 0, 0, 0, 0, 0, 0xff, 0xff})

	f.Fuzz(func(t *testing.T, payload []byte) {
		frames, err := quetzal.DecodeStks(payload, quetzal.Limits{})
		if err != nil {
			return
		}

		// Anything that decodes must be writable, and writing it then
		// reading it back must reach the same frames. A result variable
		// left over from a discarded result is the one exception: the
		// writer zeroes it, as the standard asks.
		encoded, err := quetzal.EncodeStks(frames)
		if err != nil {
			t.Fatalf("EncodeStks: unexpected error: %v", err)
		}
		back, err := quetzal.DecodeStks(encoded, quetzal.Limits{})
		if err != nil {
			t.Fatalf("DecodeStks: re-encoded frames did not decode: %v", err)
		}

		for i := range frames {
			if frames[i].DiscardResult {
				frames[i].ResultVariable = 0
			}
		}
		if !reflect.DeepEqual(back, frames) {
			t.Errorf("round trip: got %+v, want %+v", back, frames)
		}

		// Writing is settled after one pass: the same frames encode to the
		// same bytes.
		again, err := quetzal.EncodeStks(back)
		if err != nil {
			t.Fatalf("EncodeStks: unexpected error: %v", err)
		}
		if !bytes.Equal(again, encoded) {
			t.Error("encoding is not stable across a round trip")
		}
	})
}
