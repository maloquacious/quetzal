// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package quetzal_test

import (
	"bytes"
	"errors"
	"os"
	"testing"

	"github.com/maloquacious/quetzal"
)

// Fixtures nobody's interpreter wrote. See testdata/handbuilt/README.md for why
// there is only one of them.
const (
	handbuiltUMem = "testdata/handbuilt/zork1-r119-umem.qzl"

	// frotzKitchen is the real save the hand-built fixture was derived from,
	// and the base every variant below is built by mutating.
	frotzKitchen = "testdata/frotz/zork1-r119-kitchen.qzl"
)

// TestGoldenUMem covers §19's "valid uncompressed saves", the one fixture on
// that list no interpreter available here can produce: both Frotz and Bocfel
// compress by default.
//
// The state is stated explicitly rather than merely compared, per §21, and is
// also checked against the compressed save it was built from. Two encodings of
// one position must reconstruct to the same game, which is the whole claim
// §18.1 makes about encoding being a free choice.
func TestGoldenUMem(t *testing.T) {
	story := loadStory(t, "zork1-r119-880429.z3")

	blob, err := os.ReadFile(handbuiltUMem)
	if err != nil {
		t.Fatalf("reading the fixture: %v", err)
	}

	save, err := quetzal.Read(bytes.NewReader(blob), story)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	// What this file is supposed to hold, written out rather than derived.
	want := quetzal.Header{
		Release:  119,
		Serial:   quetzal.Serial{'8', '8', '0', '4', '2', '9'},
		Checksum: 0xbf44,
		PC:       0x007590,
	}
	if !headersEqual(save.Header, want) {
		t.Errorf("Header: got %+v, want %+v", save.Header, want)
	}
	if save.Memory.Encoding != quetzal.MemoryUncompressed {
		t.Errorf("Encoding: got %s, want %s", save.Memory.Encoding, quetzal.MemoryUncompressed)
	}
	if len(save.Frames) != 5 {
		t.Errorf("got %d frames, want 5", len(save.Frames))
	}
	if len(save.Frames) > 0 && !save.Frames[0].IsDummy() {
		t.Error("the first frame is not the dummy frame")
	}

	// A UMem payload is dynamic memory verbatim, so the chunk must be
	// exactly as long as the story's dynamic memory and no shorter.
	f, err := quetzal.Decode(bytes.NewReader(blob))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	umem, ok := f.First(quetzal.IDUMem)
	if !ok {
		t.Fatal("the fixture holds no UMem chunk")
	}
	if len(umem.Data) != len(story.DynamicMemory) {
		t.Errorf("UMem payload is %d bytes, want %d", len(umem.Data), len(story.DynamicMemory))
	}
	if _, ok := f.First(quetzal.IDCMem); ok {
		t.Error("the fixture holds a CMem chunk as well")
	}

	t.Run("describes the same game as the compressed save", func(t *testing.T) {
		compressed, err := os.ReadFile(frotzKitchen)
		if err != nil {
			t.Fatalf("reading %s: %v", frotzKitchen, err)
		}
		other, err := quetzal.Read(bytes.NewReader(compressed), story)
		if err != nil {
			t.Fatalf("Read(%s): %v", frotzKitchen, err)
		}

		if other.Memory.Encoding != quetzal.MemoryCompressed {
			t.Fatalf("%s is %s, so this comparison tests nothing",
				frotzKitchen, other.Memory.Encoding)
		}
		if !bytes.Equal(save.Memory.Data, other.Memory.Data) {
			t.Error("the two encodings reconstruct different dynamic memory")
		}
		if !headersEqual(save.Header, other.Header) || !framesEqual(save.Frames, other.Frames) {
			t.Error("the two encodings describe different saved state")
		}
	})
}

// variantOf returns the bytes of the Frotz Kitchen save with its chunk list
// transformed, which is how the deliberately non-conforming files below are
// built. Starting from a real interpreter's save rather than from a synthetic
// container means these test the handling of files that are wrong in one
// specific way and right in every other.
func variantOf(t *testing.T, transform func(*quetzal.File)) []byte {
	t.Helper()

	blob, err := os.ReadFile(frotzKitchen)
	if err != nil {
		t.Fatalf("reading %s: %v", frotzKitchen, err)
	}
	f, err := quetzal.Decode(bytes.NewReader(blob))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	transform(f)

	var buf bytes.Buffer
	if _, err := f.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	return buf.Bytes()
}

// moveIFhdLast puts the identification chunk after the memory and stack chunks,
// which standard 5.4 forbids.
func moveIFhdLast(f *quetzal.File) {
	var ifhd quetzal.Chunk
	rest := make([]quetzal.Chunk, 0, len(f.Chunks))
	for _, c := range f.Chunks {
		if c.ID == quetzal.IDIFhd {
			ifhd = c
			continue
		}
		rest = append(rest, c)
	}
	f.Chunks = append(rest, ifhd)
}

// dropDummyFrame removes the first stack frame, which for a version 3 save is
// the dummy frame standard 4.11 requires.
func dropDummyFrame(t *testing.T) func(*quetzal.File) {
	t.Helper()

	return func(f *quetzal.File) {
		c, ok := f.First(quetzal.IDStks)
		if !ok {
			t.Fatal("the base save has no Stks chunk")
		}
		frames, err := quetzal.DecodeStks(c.Data, quetzal.Limits{})
		if err != nil {
			t.Fatalf("DecodeStks: %v", err)
		}
		if len(frames) < 2 || !frames[0].IsDummy() {
			t.Fatalf("the base save does not begin with a dummy frame: %+v", frames)
		}
		payload, err := quetzal.EncodeStks(frames[1:])
		if err != nil {
			t.Fatalf("EncodeStks: %v", err)
		}
		for i := range f.Chunks {
			if f.Chunks[i].ID == quetzal.IDStks {
				f.Chunks[i].Data = payload
				return
			}
		}
	}
}

// TestGoldenNonConformingVariants takes one real save and breaks it in one way
// at a time, then records both what this package does and what Frotz 2.55 does
// with the same bytes.
//
// The two disagreements are the point. Where they exist, this package follows
// the written standard and Frotz does not, in opposite directions — so neither
// behavior can claim the reference implementation as support. Section 7 of
// spec-deltas.md carries the analysis; this test is what keeps the claims
// honest as the code changes.
func TestGoldenNonConformingVariants(t *testing.T) {
	story := loadStory(t, "zork1-r119-880429.z3")

	tests := []struct {
		name string

		// transform breaks the base save in one specific way.
		transform func(*quetzal.File)

		// wantErr is whether Read should refuse the result.
		wantErr bool

		// standard and frotz record what the documents require and what
		// Frotz 2.55 actually did, measured 2026-07-29.
		standard string
		frotz    string
	}{
		{
			name:      "memory and stacks before the IFhd",
			transform: moveIFhdLast,
			wantErr:   true,
			standard:  "5.4: IFhd must come before the [CU]Mem and Stks chunks",
			frotz:     "restored it; Frotz does not enforce the ordering",
		},
		{
			name:      "no dummy frame in a version 3 save",
			transform: dropDummyFrame(t),
			wantErr:   true,
			standard:  "4.11: a dummy frame must be stored as the first in the file",
			frotz:     `refused it: "Fatal error: Error reading save file"`,
		},
		{
			name: "an IFhd longer than 13 bytes",
			transform: func(f *quetzal.File) {
				for i := range f.Chunks {
					if f.Chunks[i].ID == quetzal.IDIFhd {
						f.Chunks[i].Data = append(append([]byte(nil), f.Chunks[i].Data...), "extra"...)
						return
					}
				}
			},
			standard: "5.5: a future version may have a larger IFhd; the first 13 bytes keep their meaning",
			frotz:    "restored it",
		},
		{
			name: "a second IFhd naming a different story",
			transform: func(f *quetzal.File) {
				c, _ := f.First(quetzal.IDIFhd)
				other := append([]byte(nil), c.Data...)
				other[1] = 0x58 // release 88 rather than 119
				f.Chunks = append(f.Chunks, quetzal.Chunk{ID: quetzal.IDIFhd, Data: other})
			},
			standard: "8.8: the later chunks should simply be ignored",
			frotz:    `refused it: "Save file has two IFZS chunks!"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blob := variantOf(t, tt.transform)

			// Every variant is a well-formed IFF container. What differs
			// is whether it is a well-formed *save*, which is the layer
			// that is supposed to notice.
			if _, err := quetzal.Decode(bytes.NewReader(blob)); err != nil {
				t.Errorf("Decode: %v; the container itself should still be sound", err)
			}

			save, err := quetzal.Read(bytes.NewReader(blob), story)
			switch {
			case tt.wantErr && !errors.Is(err, quetzal.ErrInvalidFormat):
				t.Errorf("Read: got %v, want ErrInvalidFormat", err)
			case !tt.wantErr && err != nil:
				t.Errorf("Read: %v", err)
			}

			// A variant we accept must still describe the right game.
			if !tt.wantErr && err == nil {
				if save.Header.Release != 119 {
					t.Errorf("Release: got %d, want the first IFhd's 119", save.Header.Release)
				}
				if save.Header.PC != 0x007590 {
					t.Errorf("PC: got %#x, want 0x7590", save.Header.PC)
				}
			}

			t.Logf("standard %s", tt.standard)
			t.Logf("us: %s | frotz: %s",
				map[bool]string{true: "refused", false: "accepted"}[tt.wantErr], tt.frotz)
		})
	}
}
