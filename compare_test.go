// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package quetzal_test

import (
	"bytes"
	"os"
	"reflect"
	"testing"

	"github.com/maloquacious/quetzal"
)

// Comparison is specified by its doc comments and by
// https://github.com/maloquacious/quetzal/issues/1 rather than by
// specification.md, which §5.7 of that document places it outside. These tests
// are therefore the whole of the machine-checkable specification, and are
// written to that standard: every kind of difference is reported from a save
// that differs in that one way and nothing else, and every option is checked
// both for what it disregards and for what it leaves alone.

// comparePair returns two saves that are identical in every field, so that a
// test can change one thing about one of them and see exactly that difference
// reported and no other.
func comparePair(t *testing.T) (a, b *quetzal.Save) {
	t.Helper()

	_, a = sampleSave(t)
	_, b = sampleSave(t)

	// The premise of every test below. If sampleSave ever returns two saves
	// that are not identical, the failures would land on whichever field it
	// happened to differ in rather than here.
	if diffs := quetzal.Compare(a, b); len(diffs) != 0 {
		t.Fatalf("two sample saves already differ: %v", diffs)
	}
	return a, b
}

// theDifference returns the single difference a comparison reported, failing if
// there was not exactly one.
func theDifference(t *testing.T, diffs []quetzal.Difference) quetzal.Difference {
	t.Helper()

	if len(diffs) != 1 {
		t.Fatalf("got %d difference(s), want exactly 1: %v", len(diffs), diffs)
	}
	return diffs[0]
}

// TestCompareReportsOneFieldAtATime is the main body of the specification: for
// each field of a save, a pair differing only in that field, and the difference
// that must be reported for it. Comparing the whole slice rather than its length
// is deliberate — it pins the frame, the offset, the chunk identifier, and the
// types held in A and B, all of which a caller is documented to be able to rely
// on.
func TestCompareReportsOneFieldAtATime(t *testing.T) {
	for _, tt := range []struct {
		name   string
		change func(*quetzal.Save)
		want   []quetzal.Difference
	}{{
		name:   "release",
		change: func(s *quetzal.Save) { s.Header.Release = 89 },
		want: []quetzal.Difference{{
			Kind: quetzal.DiffRelease, Frame: -1, Offset: -1,
			A: uint16(88), B: uint16(89),
		}},
	}, {
		name: "serial",
		change: func(s *quetzal.Save) {
			s.Header.Serial = quetzal.Serial{'8', '8', '0', '4', '2', '9'}
		},
		want: []quetzal.Difference{{
			Kind: quetzal.DiffSerial, Frame: -1, Offset: -1,
			A: quetzal.Serial{'8', '4', '0', '7', '2', '6'},
			B: quetzal.Serial{'8', '8', '0', '4', '2', '9'},
		}},
	}, {
		name:   "checksum",
		change: func(s *quetzal.Save) { s.Header.Checksum = 0x5678 },
		want: []quetzal.Difference{{
			Kind: quetzal.DiffChecksum, Frame: -1, Offset: -1,
			A: uint16(0x1234), B: uint16(0x5678),
		}},
	}, {
		name:   "program counter",
		change: func(s *quetzal.Save) { s.Header.PC = 0x06789a },
		want: []quetzal.Difference{{
			Kind: quetzal.DiffProgramCounter, Frame: -1, Offset: -1,
			A: uint32(0x012345), B: uint32(0x06789a),
		}},
	}, {
		name:   "IFhd bytes beyond the first 13",
		change: func(s *quetzal.Save) { s.Header.Extra = []byte{0x01, 0x02} },
		want: []quetzal.Difference{{
			Kind: quetzal.DiffHeaderExtra, Frame: -1, Offset: -1,
			A: []byte(nil), B: []byte{0x01, 0x02},
		}},
	}, {
		name:   "memory encoding",
		change: func(s *quetzal.Save) { s.Memory.Encoding = quetzal.MemoryUncompressed },
		want: []quetzal.Difference{{
			Kind: quetzal.DiffMemoryEncoding, Frame: -1, Offset: -1,
			A: quetzal.MemoryCompressed, B: quetzal.MemoryUncompressed,
		}},
	}, {
		name:   "memory size",
		change: func(s *quetzal.Save) { s.Memory.Data = s.Memory.Data[:0xff] },
		want: []quetzal.Difference{{
			Kind: quetzal.DiffMemorySize, Frame: -1, Offset: -1,
			A: 0x100, B: 0xff,
		}},
	}, {
		name:   "one byte of memory",
		change: func(s *quetzal.Save) { s.Memory.Data[0x60] ^= 0xff },
		want: []quetzal.Difference{{
			Kind: quetzal.DiffMemoryBytes, Frame: -1, Offset: 0x60,
			A: []byte{0xaa}, B: []byte{0x55},
		}},
	}, {
		name: "adjacent bytes of memory, as one run",
		change: func(s *quetzal.Save) {
			s.Memory.Data[0x60] ^= 0xff
			s.Memory.Data[0x61] ^= 0x0f
		},
		want: []quetzal.Difference{{
			Kind: quetzal.DiffMemoryBytes, Frame: -1, Offset: 0x60,
			A: []byte{0xaa, 0xaa}, B: []byte{0x55, 0xa5},
		}},
	}, {
		name: "separated bytes of memory, as two runs",
		change: func(s *quetzal.Save) {
			s.Memory.Data[0x60] ^= 0xff
			s.Memory.Data[0x70] ^= 0xff
		},
		want: []quetzal.Difference{{
			Kind: quetzal.DiffMemoryBytes, Frame: -1, Offset: 0x60,
			A: []byte{0xaa}, B: []byte{0x55},
		}, {
			Kind: quetzal.DiffMemoryBytes, Frame: -1, Offset: 0x70,
			A: []byte{0xaa}, B: []byte{0x55},
		}},
	}, {
		name: "stack depth",
		change: func(s *quetzal.Save) {
			s.Frames = append(s.Frames, quetzal.Frame{ReturnPC: 0x001000})
		},
		want: []quetzal.Difference{{
			Kind: quetzal.DiffFrameCount, Frame: -1, Offset: -1,
			A: 2, B: 3,
		}},
	}, {
		name:   "return program counter",
		change: func(s *quetzal.Save) { s.Frames[1].ReturnPC = 0x001234 },
		want: []quetzal.Difference{{
			Kind: quetzal.DiffReturnPC, Frame: 1, Offset: -1,
			A: uint32(0x00abcd), B: uint32(0x001234),
		}},
	}, {
		name:   "discard-result flag",
		change: func(s *quetzal.Save) { s.Frames[1].DiscardResult = true },
		want: []quetzal.Difference{{
			Kind: quetzal.DiffDiscardResult, Frame: 1, Offset: -1,
			A: false, B: true,
		}},
	}, {
		name:   "result variable",
		change: func(s *quetzal.Save) { s.Frames[1].ResultVariable = 0x07 },
		want: []quetzal.Difference{{
			Kind: quetzal.DiffResultVariable, Frame: 1, Offset: -1,
			A: byte(0x05), B: byte(0x07),
		}},
	}, {
		name:   "arguments mask",
		change: func(s *quetzal.Save) { s.Frames[1].Arguments = 0x07 },
		want: []quetzal.Difference{{
			Kind: quetzal.DiffArguments, Frame: 1, Offset: -1,
			A: uint8(0x03), B: uint8(0x07),
		}},
	}, {
		name: "local count",
		change: func(s *quetzal.Save) {
			s.Frames[1].Locals = append(s.Frames[1].Locals, 0x1111)
		},
		want: []quetzal.Difference{{
			Kind: quetzal.DiffLocalCount, Frame: 1, Offset: -1,
			A: 2, B: 3,
		}},
	}, {
		name:   "one local variable",
		change: func(s *quetzal.Save) { s.Frames[1].Locals[1] = 0x9999 },
		want: []quetzal.Difference{{
			Kind: quetzal.DiffLocalValue, Frame: 1, Offset: 1,
			A: uint16(0x5678), B: uint16(0x9999),
		}},
	}, {
		name: "evaluation stack depth",
		change: func(s *quetzal.Save) {
			s.Frames[1].Evaluation = append(s.Frames[1].Evaluation, 0x1111)
		},
		want: []quetzal.Difference{{
			Kind: quetzal.DiffEvaluationDepth, Frame: 1, Offset: -1,
			A: 1, B: 2,
		}},
	}, {
		name:   "one evaluation stack word",
		change: func(s *quetzal.Save) { s.Frames[1].Evaluation[0] = 0x1111 },
		want: []quetzal.Difference{{
			Kind: quetzal.DiffEvaluationValue, Frame: 1, Offset: 0,
			A: uint16(0x9abc), B: uint16(0x1111),
		}},
	}, {
		name: "how many chunks of one identifier",
		change: func(s *quetzal.Save) {
			s.Chunks = append(s.Chunks, quetzal.Chunk{
				ID: quetzal.IDANNO, Data: []byte("and again"),
			})
		},
		want: []quetzal.Difference{{
			Kind: quetzal.DiffChunkCount, Frame: -1, Offset: -1, ID: quetzal.IDANNO,
			A: 1, B: 2,
		}},
	}, {
		name:   "a chunk's payload",
		change: func(s *quetzal.Save) { s.Chunks[0].Data = []byte("saved in the cellar") },
		want: []quetzal.Difference{{
			Kind: quetzal.DiffChunkData, Frame: -1, Offset: 0, ID: quetzal.IDANNO,
			A: []byte("saved in the kitchen"), B: []byte("saved in the cellar"),
		}},
	}, {
		name: "a chunk only one side carries",
		change: func(s *quetzal.Save) {
			s.Chunks = append(s.Chunks, quetzal.Chunk{
				ID: quetzal.IDAUTH, Data: []byte("mdhender"),
			})
		},
		want: []quetzal.Difference{{
			Kind: quetzal.DiffChunkCount, Frame: -1, Offset: -1, ID: quetzal.IDAUTH,
			A: 0, B: 1,
		}},
	}} {
		t.Run(tt.name, func(t *testing.T) {
			a, b := comparePair(t)
			tt.change(b)

			got := quetzal.Compare(a, b)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Compare:\n got %v\nwant %v", got, tt.want)
			}

			// Comparison says how the two differ, not which is right, so
			// exchanging the arguments must exchange A and B and change
			// nothing else.
			back := quetzal.Compare(b, a)
			if len(back) != len(got) {
				t.Fatalf("Compare(b, a): got %d difference(s), want %d", len(back), len(got))
			}
			for i, d := range back {
				if !reflect.DeepEqual(d.A, got[i].B) || !reflect.DeepEqual(d.B, got[i].A) {
					t.Errorf("Compare(b, a)[%d]: got %v, want the sides of %v exchanged", i, d, got[i])
				}
			}
		})
	}
}

// TestCompareIdenticalSavesReportNothing is the other half of the premise: not
// merely that two sample saves agree, but that a save agrees with itself, which
// is what an empty result has to mean for any of the above to be worth anything.
func TestCompareIdenticalSavesReportNothing(t *testing.T) {
	_, save := sampleSave(t)

	if diffs := quetzal.Compare(save, save); len(diffs) != 0 {
		t.Errorf("a save compared with itself: got %v, want no differences", diffs)
	}
}

// TestCompareDoesNotAliasItsArguments checks §17's ownership rule for the one
// place comparison could break it: a Difference holding a run of memory or a
// chunk payload must hold a copy, or a caller keeping the result would watch it
// change under them.
func TestCompareDoesNotAliasItsArguments(t *testing.T) {
	a, b := comparePair(t)
	b.Memory.Data[0x60] ^= 0xff
	b.Chunks[0].Data = []byte("saved in the cellar")

	diffs := quetzal.Compare(a, b)
	if len(diffs) != 2 {
		t.Fatalf("got %d difference(s), want 2: %v", len(diffs), diffs)
	}
	before := make([]string, len(diffs))
	for i, d := range diffs {
		before[i] = d.String()
	}

	// Scribble over everything the differences were taken from.
	for i := range b.Memory.Data {
		b.Memory.Data[i] = 0x11
	}
	for i := range b.Chunks[0].Data {
		b.Chunks[0].Data[i] = 'x'
	}

	for i, d := range diffs {
		if got := d.String(); got != before[i] {
			t.Errorf("difference %d changed with its save: got %q, want %q", i, got, before[i])
		}
	}
}

// TestCompareIgnoreMemoryEncoding covers the option a re-encoded save needs, and
// that it reaches nothing else: the encoding is a writer's choice, the bytes are
// not.
func TestCompareIgnoreMemoryEncoding(t *testing.T) {
	a, b := comparePair(t)
	b.Memory.Encoding = quetzal.MemoryUncompressed

	if diffs := quetzal.Compare(a, b, quetzal.IgnoreMemoryEncoding()); len(diffs) != 0 {
		t.Errorf("with IgnoreMemoryEncoding: got %v, want no differences", diffs)
	}

	b.Memory.Data[0x60] ^= 0xff
	got := theDifference(t, quetzal.Compare(a, b, quetzal.IgnoreMemoryEncoding()))
	if got.Kind != quetzal.DiffMemoryBytes {
		t.Errorf("Kind: got %s, want %s", got.Kind, quetzal.DiffMemoryBytes)
	}
}

// TestCompareIgnoreMemoryRange covers the bounds an option may be given,
// including the ones that disregard nothing. None of them is an error, because
// there is no error to return, so each has to have a defined meaning instead.
func TestCompareIgnoreMemoryRange(t *testing.T) {
	for _, tt := range []struct {
		name string
		opts []quetzal.CompareOption
		want bool // whether the difference at 0x60 is still reported
	}{{
		name: "a range covering the difference",
		opts: []quetzal.CompareOption{quetzal.IgnoreMemoryRange(0x50, 0x70)},
		want: false,
	}, {
		name: "a range of exactly the one byte",
		opts: []quetzal.CompareOption{quetzal.IgnoreMemoryRange(0x60, 0x61)},
		want: false,
	}, {
		name: "a range ending just before it, since the end is exclusive",
		opts: []quetzal.CompareOption{quetzal.IgnoreMemoryRange(0x50, 0x60)},
		want: true,
	}, {
		name: "a range starting just after it",
		opts: []quetzal.CompareOption{quetzal.IgnoreMemoryRange(0x61, 0x70)},
		want: true,
	}, {
		name: "an empty range",
		opts: []quetzal.CompareOption{quetzal.IgnoreMemoryRange(0x60, 0x60)},
		want: true,
	}, {
		name: "an inverted range",
		opts: []quetzal.CompareOption{quetzal.IgnoreMemoryRange(0x70, 0x50)},
		want: true,
	}, {
		name: "a range beyond dynamic memory",
		opts: []quetzal.CompareOption{quetzal.IgnoreMemoryRange(0x1000, 0x2000)},
		want: true,
	}, {
		name: "a range reaching in from before address zero",
		opts: []quetzal.CompareOption{quetzal.IgnoreMemoryRange(-0x100, 0x70)},
		want: false,
	}, {
		name: "two ranges, the second of which covers it",
		opts: []quetzal.CompareOption{
			quetzal.IgnoreMemoryRange(0x00, 0x10),
			quetzal.IgnoreMemoryRange(0x60, 0x61),
		},
		want: false,
	}} {
		t.Run(tt.name, func(t *testing.T) {
			a, b := comparePair(t)
			b.Memory.Data[0x60] ^= 0xff

			diffs := quetzal.Compare(a, b, tt.opts...)
			if got := len(diffs) != 0; got != tt.want {
				t.Errorf("difference reported: got %v, want %v (%v)", got, tt.want, diffs)
			}
		})
	}
}

// TestCompareIgnoredRangeSplitsARun pins the interaction the option's doc comment
// promises: an ignored address ends the run of differing bytes that reaches it,
// rather than being quietly folded into one run spanning it.
func TestCompareIgnoredRangeSplitsARun(t *testing.T) {
	a, b := comparePair(t)
	for _, addr := range []int{0x60, 0x61, 0x62} {
		b.Memory.Data[addr] ^= 0xff
	}

	want := []quetzal.Difference{{
		Kind: quetzal.DiffMemoryBytes, Frame: -1, Offset: 0x60,
		A: []byte{0xaa}, B: []byte{0x55},
	}, {
		Kind: quetzal.DiffMemoryBytes, Frame: -1, Offset: 0x62,
		A: []byte{0xaa}, B: []byte{0x55},
	}}

	got := quetzal.Compare(a, b, quetzal.IgnoreMemoryRange(0x61, 0x62))
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Compare:\n got %v\nwant %v", got, want)
	}
}

// TestCompareIgnoreInterpreterHeader walks every address of the Z-machine header
// one at a time, which is the only way to check a boundary: the option's value
// lies in disregarding exactly the fields the interpreter writes, and a range
// one byte too wide silently stops reporting a field of the game's own.
func TestCompareIgnoreInterpreterHeader(t *testing.T) {
	// The set the option documents: $01, $10 to $11, $1E to $27, $2C to $2D,
	// and $30 to $33. Restated here rather than shared with the
	// implementation, so that widening a range there fails here.
	ignored := make(map[int]bool)
	for _, r := range [][2]int{{0x01, 0x02}, {0x10, 0x12}, {0x1e, 0x28}, {0x2c, 0x2e}, {0x30, 0x34}} {
		for addr := r[0]; addr < r[1]; addr++ {
			ignored[addr] = true
		}
	}

	const headerSize = 0x40
	for addr := range headerSize {
		a, b := comparePair(t)
		b.Memory.Data[addr] ^= 0xff

		diffs := quetzal.Compare(a, b, quetzal.IgnoreInterpreterHeader())
		switch {
		case ignored[addr]:
			if len(diffs) != 0 {
				t.Errorf("address %#02x is written by the interpreter, but got %v", addr, diffs)
			}
		case len(diffs) != 1 || diffs[0].Kind != quetzal.DiffMemoryBytes || diffs[0].Offset != addr:
			t.Errorf("address %#02x is written by the game, but got %v", addr, diffs)
		}
	}

	// And the option reaches nothing outside the header.
	a, b := comparePair(t)
	b.Memory.Data[headerSize] ^= 0xff
	if got := theDifference(t, quetzal.Compare(a, b, quetzal.IgnoreInterpreterHeader())); got.Offset != headerSize {
		t.Errorf("Offset: got %#02x, want %#02x", got.Offset, headerSize)
	}
}

// TestCompareResultVariableOfADiscardedResult is D16 seen from the comparison
// side. This package zeroes the byte on write and preserves what it read, so two
// saves of one position can hold different values there; the byte carries no
// meaning when the result is discarded, and reporting it would be reporting
// noise no option asked to disregard.
func TestCompareResultVariableOfADiscardedResult(t *testing.T) {
	for _, tt := range []struct {
		name       string
		discardA   bool
		discardB   bool
		wantKinds  []quetzal.DifferenceKind
		wantReason string
	}{{
		name: "neither frame discards its result",
		wantKinds: []quetzal.DifferenceKind{
			quetzal.DiffResultVariable,
		},
		wantReason: "the byte means something on both sides",
	}, {
		name:       "both frames discard their result",
		discardA:   true,
		discardB:   true,
		wantKinds:  nil,
		wantReason: "the byte means nothing on either side",
	}, {
		name:     "only one frame discards its result",
		discardB: true,
		wantKinds: []quetzal.DifferenceKind{
			quetzal.DiffDiscardResult,
		},
		wantReason: "the flag is the difference, and the byte follows from it",
	}} {
		t.Run(tt.name, func(t *testing.T) {
			a, b := comparePair(t)
			a.Frames[1].DiscardResult, b.Frames[1].DiscardResult = tt.discardA, tt.discardB
			a.Frames[1].ResultVariable, b.Frames[1].ResultVariable = 0x05, 0x07

			var got []quetzal.DifferenceKind
			for _, d := range quetzal.Compare(a, b) {
				got = append(got, d.Kind)
			}
			if !reflect.DeepEqual(got, tt.wantKinds) {
				t.Errorf("got %v, want %v — %s", got, tt.wantKinds, tt.wantReason)
			}
		})
	}
}

// TestCompareChunksAreGroupedByIdentifier pins what separates Compare from
// CompareFiles. A save's additional chunks arrive in whatever order the file
// held them and Quetzal fixes no order among them, so two saves carrying the
// same chunks in the opposite order are not describing different positions.
func TestCompareChunksAreGroupedByIdentifier(t *testing.T) {
	a, b := comparePair(t)
	auth := quetzal.Chunk{ID: quetzal.IDAUTH, Data: []byte("mdhender")}

	a.Chunks = []quetzal.Chunk{a.Chunks[0], auth}
	b.Chunks = []quetzal.Chunk{auth, b.Chunks[0]}

	if diffs := quetzal.Compare(a, b); len(diffs) != 0 {
		t.Errorf("the same chunks in the opposite order: got %v, want no differences", diffs)
	}

	// Order within one identifier is another matter: two annotations are two
	// separate remarks, and which is which is part of what the save says.
	a.Chunks = []quetzal.Chunk{{ID: quetzal.IDANNO, Data: []byte("first")}, {ID: quetzal.IDANNO, Data: []byte("second")}}
	b.Chunks = []quetzal.Chunk{{ID: quetzal.IDANNO, Data: []byte("second")}, {ID: quetzal.IDANNO, Data: []byte("first")}}

	if diffs := quetzal.Compare(a, b); len(diffs) != 2 {
		t.Errorf("two annotations exchanged: got %d difference(s), want 2: %v", len(diffs), diffs)
	}
}

// TestCompareIgnoreChunks covers the option a cross-interpreter comparison needs
// for the chunks each interpreter writes for itself.
func TestCompareIgnoreChunks(t *testing.T) {
	a, b := comparePair(t)
	b.Chunks = []quetzal.Chunk{
		{ID: quetzal.IDANNO, Data: []byte("saved by another interpreter")},
		{ID: quetzal.IDIntD, Data: []byte("twelve bytes and then some")},
	}

	if diffs := quetzal.Compare(a, b); len(diffs) == 0 {
		t.Fatal("with no options: got no differences, want the two chunk differences")
	}

	// Two calls rather than one, since accumulating into a set already built
	// is how a caller composing options will reach this.
	diffs := quetzal.Compare(a, b, quetzal.IgnoreChunks(quetzal.IDANNO), quetzal.IgnoreChunks(quetzal.IDIntD))
	if len(diffs) != 0 {
		t.Errorf("with both identifiers ignored: got %v, want no differences", diffs)
	}

	// And ignoring one leaves the other.
	got := theDifference(t, quetzal.Compare(a, b, quetzal.IgnoreChunks(quetzal.IDANNO)))
	if got.ID != quetzal.IDIntD {
		t.Errorf("ID: got %s, want %s", got.ID, quetzal.IDIntD)
	}
}

// TestCompareNilSaves checks the promise that a nil save is compared as an empty
// one. Nothing about Compare's signature stops a caller passing the save a
// failed Read returned, and panicking on it would break §25 for the sake of a
// case that has a perfectly good answer.
func TestCompareNilSaves(t *testing.T) {
	_, save := sampleSave(t)

	if diffs := quetzal.Compare(nil, nil); len(diffs) != 0 {
		t.Errorf("Compare(nil, nil): got %v, want no differences", diffs)
	}

	want := []quetzal.DifferenceKind{
		quetzal.DiffRelease,
		quetzal.DiffSerial,
		quetzal.DiffChecksum,
		quetzal.DiffProgramCounter,
		quetzal.DiffMemoryEncoding,
		quetzal.DiffMemorySize,
		quetzal.DiffFrameCount,
		quetzal.DiffChunkCount,
	}
	for _, tt := range []struct {
		name string
		got  []quetzal.Difference
	}{
		{name: "Compare(nil, save)", got: quetzal.Compare(nil, save)},
		{name: "Compare(save, nil)", got: quetzal.Compare(save, nil)},
	} {
		kinds := make([]quetzal.DifferenceKind, len(tt.got))
		for i, d := range tt.got {
			kinds[i] = d.Kind
		}
		if !reflect.DeepEqual(kinds, want) {
			t.Errorf("%s: got %v, want %v", tt.name, kinds, want)
		}
	}
}

// TestCompareFiles covers the container layer, where chunks are compared by
// position because position is what a container has and a save does not.
func TestCompareFiles(t *testing.T) {
	ifhd := quetzal.Chunk{ID: quetzal.IDIFhd, Data: ifhdPayload}
	stks := quetzal.Chunk{ID: quetzal.IDStks, Data: []byte{}}
	anno := quetzal.Chunk{ID: quetzal.IDANNO, Data: []byte("in the kitchen")}

	for _, tt := range []struct {
		name string
		a, b []quetzal.Chunk
		opts []quetzal.CompareOption
		want []quetzal.Difference
	}{{
		name: "the same chunks in the same order",
		a:    []quetzal.Chunk{ifhd, stks, anno},
		b:    []quetzal.Chunk{ifhd, stks, anno},
		want: nil,
	}, {
		name: "one file carries a chunk the other does not",
		a:    []quetzal.Chunk{ifhd, stks, anno},
		b:    []quetzal.Chunk{ifhd, stks},
		want: []quetzal.Difference{{
			Kind: quetzal.DiffChunkCount, Frame: -1, Offset: -1, A: 3, B: 2,
		}},
	}, {
		name: "a different identifier at the same position",
		a:    []quetzal.Chunk{ifhd, anno},
		b:    []quetzal.Chunk{ifhd, stks},
		want: []quetzal.Difference{{
			Kind: quetzal.DiffChunkID, Frame: -1, Offset: 1,
			A: quetzal.IDANNO, B: quetzal.IDStks,
		}},
	}, {
		name: "the same identifier with a different payload",
		a:    []quetzal.Chunk{ifhd, anno},
		b:    []quetzal.Chunk{ifhd, {ID: quetzal.IDANNO, Data: []byte("in the cellar")}},
		want: []quetzal.Difference{{
			Kind: quetzal.DiffChunkData, Frame: -1, Offset: 1, ID: quetzal.IDANNO,
			A: []byte("in the kitchen"), B: []byte("in the cellar"),
		}},
	}, {
		name: "chunk order, which a save would not report",
		a:    []quetzal.Chunk{ifhd, stks, anno},
		b:    []quetzal.Chunk{ifhd, anno, stks},
		want: []quetzal.Difference{{
			Kind: quetzal.DiffChunkID, Frame: -1, Offset: 1,
			A: quetzal.IDStks, B: quetzal.IDANNO,
		}, {
			Kind: quetzal.DiffChunkID, Frame: -1, Offset: 2,
			A: quetzal.IDANNO, B: quetzal.IDStks,
		}},
	}, {
		name: "an ignored chunk, which realigns the chunks after it",
		a:    []quetzal.Chunk{ifhd, anno, stks},
		b:    []quetzal.Chunk{ifhd, stks},
		opts: []quetzal.CompareOption{quetzal.IgnoreChunks(quetzal.IDANNO)},
		want: nil,
	}, {
		name: "the memory options, which mean nothing to a container",
		a:    []quetzal.Chunk{ifhd, {ID: quetzal.IDCMem, Data: []byte{0x00, 0x01}}},
		b:    []quetzal.Chunk{ifhd, {ID: quetzal.IDUMem, Data: []byte{0xaa, 0xbb}}},
		opts: []quetzal.CompareOption{
			quetzal.IgnoreMemoryEncoding(),
			quetzal.IgnoreMemoryRange(0, 0x100),
		},
		want: []quetzal.Difference{{
			Kind: quetzal.DiffChunkID, Frame: -1, Offset: 1,
			A: quetzal.IDCMem, B: quetzal.IDUMem,
		}},
	}} {
		t.Run(tt.name, func(t *testing.T) {
			a := &quetzal.File{Chunks: tt.a}
			b := &quetzal.File{Chunks: tt.b}

			got := quetzal.CompareFiles(a, b, tt.opts...)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("CompareFiles:\n got %v\nwant %v", got, tt.want)
			}
		})
	}
}

// TestCompareFilesNil is the container layer's half of TestCompareNilSaves.
func TestCompareFilesNil(t *testing.T) {
	file := &quetzal.File{Chunks: []quetzal.Chunk{{ID: quetzal.IDANNO, Data: []byte("note")}}}

	if diffs := quetzal.CompareFiles(nil, nil); len(diffs) != 0 {
		t.Errorf("CompareFiles(nil, nil): got %v, want no differences", diffs)
	}
	for _, tt := range []struct {
		name string
		got  []quetzal.Difference
	}{
		{name: "CompareFiles(nil, file)", got: quetzal.CompareFiles(nil, file)},
		{name: "CompareFiles(file, nil)", got: quetzal.CompareFiles(file, nil)},
	} {
		got := theDifference(t, tt.got)
		if got.Kind != quetzal.DiffChunkCount {
			t.Errorf("%s: Kind: got %s, want %s", tt.name, got.Kind, quetzal.DiffChunkCount)
		}
	}
}

// TestCompareARoundTrippedFixture is the acceptance test, and the reason the
// feature exists: a save written by another interpreter, read, written again in
// the other encoding, and read back must differ in exactly one way — the
// encoding — and in no way at all once the option that disregards it is given.
//
// It is the same claim §18.1 makes about the semantic round trip, asserted by
// asking rather than by a hand-rolled comparison per field.
func TestCompareARoundTrippedFixture(t *testing.T) {
	const path = "testdata/frotz/zork1-r119-kitchen.qzl"

	blob, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	story := loadStory(t, "zork1-r119-880429.z3")

	first, err := quetzal.Read(bytes.NewReader(blob), story)
	if err != nil {
		t.Fatalf("Read: unexpected error: %v", err)
	}

	var buf bytes.Buffer
	if err := quetzal.Write(&buf, story, first, quetzal.WithEncoding(quetzal.MemoryUncompressed)); err != nil {
		t.Fatalf("Write: unexpected error: %v", err)
	}
	second, err := quetzal.Read(bytes.NewReader(buf.Bytes()), story)
	if err != nil {
		t.Fatalf("Read(rewritten): unexpected error: %v", err)
	}

	got := theDifference(t, quetzal.Compare(first, second))
	if got.Kind != quetzal.DiffMemoryEncoding {
		t.Errorf("the round trip changed something other than the encoding: %v", got)
	}

	if diffs := quetzal.Compare(first, second, quetzal.IgnoreMemoryEncoding()); len(diffs) != 0 {
		t.Errorf("with IgnoreMemoryEncoding: got %v, want no differences", diffs)
	}
}

// TestCompareSavesOfDifferentStories checks that comparison needs no story of
// its own and passes no judgment: two saves that belong to different games have
// an answer, and it is that their identities differ.
func TestCompareSavesOfDifferentStories(t *testing.T) {
	zork1 := loadStory(t, "zork1-r119-880429.z3")
	zork2 := loadStory(t, "zork2-r63-860811.z3")

	one := readFixtureSave(t, "testdata/frotz/zork1-r119-start.qzl", zork1)
	two := readFixtureSave(t, "testdata/frotz/zork2-r63-start.qzl", zork2)

	diffs := quetzal.Compare(one, two)
	for _, want := range []quetzal.DifferenceKind{
		quetzal.DiffRelease, quetzal.DiffSerial, quetzal.DiffChecksum, quetzal.DiffMemorySize,
	} {
		found := false
		for _, d := range diffs {
			if d.Kind == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("comparing saves of two games did not report a difference in %s", want)
		}
	}
}

// readFixtureSave reads a save fixture against the story it belongs to.
func readFixtureSave(t *testing.T, path string, story quetzal.Story) *quetzal.Save {
	t.Helper()

	blob, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	save, err := quetzal.Read(bytes.NewReader(blob), story)
	if err != nil {
		t.Fatalf("Read(%s): unexpected error: %v", path, err)
	}
	return save
}

// TestDifferenceKindString covers every kind's name, and the one a kind this
// package does not define falls back to. The exhaustiveness is the point: a kind
// added without a name would otherwise render as a number in the middle of an
// otherwise readable report.
func TestDifferenceKindString(t *testing.T) {
	for _, tt := range []struct {
		kind quetzal.DifferenceKind
		want string
	}{
		{quetzal.DiffRelease, "release"},
		{quetzal.DiffSerial, "serial"},
		{quetzal.DiffChecksum, "checksum"},
		{quetzal.DiffProgramCounter, "program counter"},
		{quetzal.DiffHeaderExtra, "IFhd extra bytes"},
		{quetzal.DiffMemoryEncoding, "memory encoding"},
		{quetzal.DiffMemorySize, "memory size"},
		{quetzal.DiffMemoryBytes, "memory bytes"},
		{quetzal.DiffFrameCount, "frame count"},
		{quetzal.DiffReturnPC, "return program counter"},
		{quetzal.DiffDiscardResult, "discard-result flag"},
		{quetzal.DiffResultVariable, "result variable"},
		{quetzal.DiffArguments, "arguments mask"},
		{quetzal.DiffLocalCount, "local count"},
		{quetzal.DiffLocalValue, "local variable"},
		{quetzal.DiffEvaluationDepth, "evaluation stack depth"},
		{quetzal.DiffEvaluationValue, "evaluation stack word"},
		{quetzal.DiffChunkCount, "chunk count"},
		{quetzal.DiffChunkID, "chunk identifier"},
		{quetzal.DiffChunkData, "chunk payload"},
		{quetzal.DifferenceKind(0), "DifferenceKind(0)"},
		{quetzal.DifferenceKind(200), "DifferenceKind(200)"},
	} {
		if got := tt.kind.String(); got != tt.want {
			t.Errorf("DifferenceKind(%d).String(): got %q, want %q", uint8(tt.kind), got, tt.want)
		}
	}

	// The kinds are consecutive from one, which is what lets a caller keep an
	// array indexed by kind and what makes the zero value mean "unset".
	if quetzal.DiffRelease != 1 {
		t.Errorf("DiffRelease: got %d, want 1", uint8(quetzal.DiffRelease))
	}
	if quetzal.DiffChunkData != 20 {
		t.Errorf("DiffChunkData: got %d, want 20; was a kind added without a name?",
			uint8(quetzal.DiffChunkData))
	}
}

// TestDifferenceString covers the rendering of every kind, since these strings
// are what a failing test prints and are therefore the whole of what most callers
// will ever see of this feature.
//
// The last few cases build a Difference whose A and B do not hold the type its
// kind implies. Only this package's own construction keeps those in step, and a
// caller assembling one by hand — or keeping one across a change to this package —
// must get a reading rather than a panic (§25).
func TestDifferenceString(t *testing.T) {
	for _, tt := range []struct {
		name string
		diff quetzal.Difference
		want string
	}{{
		name: "release",
		diff: quetzal.Difference{Kind: quetzal.DiffRelease, Frame: -1, Offset: -1, A: uint16(88), B: uint16(89)},
		want: "release: 88 vs 89",
	}, {
		name: "serial",
		diff: quetzal.Difference{Kind: quetzal.DiffSerial, Frame: -1, Offset: -1,
			A: quetzal.Serial{'8', '4', '0', '7', '2', '6'},
			B: quetzal.Serial{'8', '8', '0', '4', '2', '9'}},
		want: "serial: 840726 vs 880429",
	}, {
		name: "checksum",
		diff: quetzal.Difference{Kind: quetzal.DiffChecksum, Frame: -1, Offset: -1, A: uint16(0x1234), B: uint16(0x5678)},
		want: "checksum: 0x1234 vs 0x5678",
	}, {
		name: "program counter",
		diff: quetzal.Difference{Kind: quetzal.DiffProgramCounter, Frame: -1, Offset: -1, A: uint32(0x012345), B: uint32(0x06789a)},
		want: "program counter: 0x12345 vs 0x6789a",
	}, {
		name: "IFhd extra bytes, absent on one side",
		diff: quetzal.Difference{Kind: quetzal.DiffHeaderExtra, Frame: -1, Offset: -1, A: []byte(nil), B: []byte{0x01, 0x02}},
		want: "IFhd bytes beyond the first 13: none vs 2 byte(s)",
	}, {
		name: "memory encoding",
		diff: quetzal.Difference{Kind: quetzal.DiffMemoryEncoding, Frame: -1, Offset: -1,
			A: quetzal.MemoryCompressed, B: quetzal.MemoryUncompressed},
		want: "dynamic memory encoding: CMem vs UMem",
	}, {
		name: "memory size",
		diff: quetzal.Difference{Kind: quetzal.DiffMemorySize, Frame: -1, Offset: -1, A: 0x100, B: 0xff},
		want: "dynamic memory: 256 byte(s) vs 255",
	}, {
		name: "a short run of memory",
		diff: quetzal.Difference{Kind: quetzal.DiffMemoryBytes, Frame: -1, Offset: 0x60,
			A: []byte{0xaa, 0xaa}, B: []byte{0x55, 0xa5}},
		want: "dynamic memory at 0x0060: aa aa vs 55 a5",
	}, {
		name: "a run of memory too long to print",
		diff: quetzal.Difference{Kind: quetzal.DiffMemoryBytes, Frame: -1, Offset: 0,
			A: []byte{0, 1, 2, 3, 4, 5, 6, 7, 8}, B: []byte{9, 9, 9, 9, 9, 9, 9, 9, 9}},
		want: "dynamic memory at 0x0000: 00 01 02 03 04 05 06 07 … (9 byte(s)) vs 09 09 09 09 09 09 09 09 … (9 byte(s))",
	}, {
		name: "stack depth",
		diff: quetzal.Difference{Kind: quetzal.DiffFrameCount, Frame: -1, Offset: -1, A: 2, B: 3},
		want: "call stack: 2 frame(s) vs 3",
	}, {
		name: "return program counter, which names its frame",
		diff: quetzal.Difference{Kind: quetzal.DiffReturnPC, Frame: 1, Offset: -1, A: uint32(0x00abcd), B: uint32(0x001234)},
		want: "frame 1: return program counter: 0xabcd vs 0x1234",
	}, {
		name: "discard-result flag of the oldest frame",
		diff: quetzal.Difference{Kind: quetzal.DiffDiscardResult, Frame: 0, Offset: -1, A: false, B: true},
		want: "frame 0: discard-result flag: false vs true",
	}, {
		name: "result variable",
		diff: quetzal.Difference{Kind: quetzal.DiffResultVariable, Frame: 1, Offset: -1, A: byte(0x05), B: byte(0x07)},
		want: "frame 1: result variable: 5 vs 7",
	}, {
		name: "arguments mask",
		diff: quetzal.Difference{Kind: quetzal.DiffArguments, Frame: 1, Offset: -1, A: uint8(0x03), B: uint8(0x07)},
		want: "frame 1: arguments mask: 0x03 vs 0x07",
	}, {
		name: "local count",
		diff: quetzal.Difference{Kind: quetzal.DiffLocalCount, Frame: 1, Offset: -1, A: 2, B: 3},
		want: "frame 1: 2 local(s) vs 3",
	}, {
		name: "one local variable, numbered from one",
		diff: quetzal.Difference{Kind: quetzal.DiffLocalValue, Frame: 1, Offset: 1, A: uint16(0x5678), B: uint16(0x9999)},
		want: "frame 1: local 2: 0x5678 vs 0x9999",
	}, {
		name: "evaluation stack depth",
		diff: quetzal.Difference{Kind: quetzal.DiffEvaluationDepth, Frame: 1, Offset: -1, A: 1, B: 2},
		want: "frame 1: 1 evaluation word(s) vs 2",
	}, {
		name: "one evaluation stack word, numbered from zero",
		diff: quetzal.Difference{Kind: quetzal.DiffEvaluationValue, Frame: 1, Offset: 0, A: uint16(0x9abc), B: uint16(0x1111)},
		want: "frame 1: evaluation word 0: 0x9abc vs 0x1111",
	}, {
		name: "how many chunks of one identifier",
		diff: quetzal.Difference{Kind: quetzal.DiffChunkCount, Frame: -1, Offset: -1, ID: quetzal.IDANNO, A: 1, B: 2},
		want: "ANNO chunks: 1 vs 2",
	}, {
		name: "how many chunks in a container, which names no identifier",
		diff: quetzal.Difference{Kind: quetzal.DiffChunkCount, Frame: -1, Offset: -1, A: 3, B: 4},
		want: "chunks: 3 vs 4",
	}, {
		name: "chunk identifier",
		diff: quetzal.Difference{Kind: quetzal.DiffChunkID, Frame: -1, Offset: 2, A: quetzal.IDANNO, B: quetzal.IDAUTH},
		want: "chunk 2: ANNO vs AUTH",
	}, {
		name: "chunk payload",
		diff: quetzal.Difference{Kind: quetzal.DiffChunkData, Frame: -1, Offset: 0, ID: quetzal.IDANNO,
			A: []byte("saved in the kitchen"), B: []byte("saved in the cellar")},
		want: "ANNO chunk 0: payloads differ, 20 byte(s) vs 19 byte(s)",
	}, {
		name: "a kind this package does not define",
		diff: quetzal.Difference{Kind: quetzal.DifferenceKind(200), Frame: -1, Offset: -1, A: 1, B: 2},
		want: "DifferenceKind(200): 1 vs 2",
	}, {
		name: "a payload difference holding something other than a payload",
		diff: quetzal.Difference{Kind: quetzal.DiffHeaderExtra, Frame: -1, Offset: -1, A: 1, B: 2},
		want: "IFhd bytes beyond the first 13: 1 vs 2",
	}, {
		name: "a memory run holding something other than a run",
		diff: quetzal.Difference{Kind: quetzal.DiffMemoryBytes, Frame: -1, Offset: 0, A: "one", B: "two"},
		want: "dynamic memory at 0x0000: one vs two",
	}} {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.diff.String(); got != tt.want {
				t.Errorf("String():\n got %q\nwant %q", got, tt.want)
			}
		})
	}
}
