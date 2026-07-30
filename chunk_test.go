// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package quetzal_test

import (
	"bytes"
	"slices"
	"testing"

	"github.com/maloquacious/quetzal"
)

// TestTextChunks covers §12's helpers for the three chunks Quetzal defines to
// hold text: ANNO, AUTH, and "(c) ".
//
// The identifier of the last one is the reason these are worth having. It is
// the three characters "(c)" followed by a space, and a caller assembling that
// by hand gets it wrong by leaving the space off — at which point the chunk is
// silently absent rather than malformed.
func TestTextChunks(t *testing.T) {
	f := decodeIFZS(t,
		chunkBytes("ANNO", []byte("saved in the kitchen")),
		chunkBytes("AUTH", []byte("mdhender")),
		chunkBytes("(c) ", []byte("1980 Infocom")),
		chunkBytes("ANNO", []byte("score 10, 5 moves")),
	)

	wantAnnotations := []string{"saved in the kitchen", "score 10, 5 moves"}
	if got := f.Annotations(); !slices.Equal(got, wantAnnotations) {
		t.Errorf("Annotations: got %q, want %q", got, wantAnnotations)
	}

	if got, ok := f.Author(); !ok || got != "mdhender" {
		t.Errorf("Author: got %q, %v, want %q, true", got, ok, "mdhender")
	}
	if got, ok := f.Copyright(); !ok || got != "1980 Infocom" {
		t.Errorf("Copyright: got %q, %v, want %q, true", got, ok, "1980 Infocom")
	}
}

func TestTextChunksAbsent(t *testing.T) {
	// A save carrying none of them is in no way deficient: 7.6 says an
	// interpreter must not rely on their presence. Absence is reported, not
	// treated as an error, and the annotation case yields a nil slice a
	// caller can range over without checking.
	f := decodeIFZS(t, chunkBytes("IFhd", bytes.Repeat([]byte{0xab}, 13)))

	if got := f.Annotations(); got != nil {
		t.Errorf("Annotations: got %q, want nil", got)
	}
	if got, ok := f.Author(); ok {
		t.Errorf("Author: got %q, true, want \"\", false", got)
	}
	if got, ok := f.Copyright(); ok {
		t.Errorf("Copyright: got %q, true, want \"\", false", got)
	}
}

func TestTextChunksDuplicateSingletons(t *testing.T) {
	// 7.3 and 7.4 say there should be only one AUTH and one "(c) " per file.
	// A file with two is wrong, but the first-instance rule applies to them
	// as it does everywhere else, and both remain visible through All.
	f := decodeIFZS(t,
		chunkBytes("AUTH", []byte("first")),
		chunkBytes("AUTH", []byte("second")),
	)

	if got, _ := f.Author(); got != "first" {
		t.Errorf("Author: got %q, want %q", got, "first")
	}
	if got := len(f.All(quetzal.IDAUTH)); got != 2 {
		t.Errorf("All(AUTH): got %d chunks, want 2 — the duplicate must stay visible", got)
	}
}

func TestTextChunksReturnStoredBytes(t *testing.T) {
	// The format says these chunks hold characters in 0x20 to 0x7E and
	// nothing else. That is not enforced: the text is returned as stored,
	// because a chunk breaking the rule still carries what its writer meant
	// and discarding it would lose information. This test exists so that the
	// choice is deliberate rather than discovered.
	payload := []byte{'a', 0x00, 0x1b, '[', '3', '1', 'm', 0x7f, 'z'}
	f := decodeIFZS(t, chunkBytes("ANNO", payload))

	got := f.Annotations()
	if len(got) != 1 {
		t.Fatalf("Annotations: got %d, want 1", len(got))
	}
	if got[0] != string(payload) {
		t.Errorf("Annotations[0]: got %q, want the payload verbatim, %q", got[0], payload)
	}
}

// TestTextChunksOnSave checks that the helpers agree on a File and on the Save
// read from it, since a caller reaches these chunks through whichever of the
// two it happens to hold.
func TestTextChunksOnSave(t *testing.T) {
	story := loadStory(t, "zork1-r119-880429.z3")

	blob := saveWithChunks(t, story,
		quetzal.Chunk{ID: quetzal.IDANNO, Data: []byte("saved in the kitchen")},
		quetzal.Chunk{ID: quetzal.IDAUTH, Data: []byte("mdhender")},
		quetzal.Chunk{ID: quetzal.IDCopy, Data: []byte("1980 Infocom")},
	)

	save, err := quetzal.Read(bytes.NewReader(blob), story)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	f, err := quetzal.Decode(bytes.NewReader(blob))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	if got, want := save.Annotations(), f.Annotations(); !slices.Equal(got, want) {
		t.Errorf("Annotations: Save says %q, File says %q", got, want)
	}
	if got, _ := save.Author(); got != "mdhender" {
		t.Errorf("Save.Author: got %q, want %q", got, "mdhender")
	}
	if got, _ := save.Copyright(); got != "1980 Infocom" {
		t.Errorf("Save.Copyright: got %q, want %q", got, "1980 Infocom")
	}
}

// saveWithChunks writes a minimal valid save for the story, carrying the given
// extra chunks, and returns its bytes.
func saveWithChunks(t *testing.T, story quetzal.Story, extra ...quetzal.Chunk) []byte {
	t.Helper()

	save := &quetzal.Save{
		Header: quetzal.Header{
			Release:  story.Release,
			Serial:   story.Serial,
			Checksum: story.Checksum,
			PC:       0x1234,
		},
		Memory: quetzal.Memory{
			Data:     append([]byte(nil), story.DynamicMemory...),
			Encoding: quetzal.MemoryCompressed,
		},
		Frames: []quetzal.Frame{{}},
		Chunks: extra,
	}

	var buf bytes.Buffer
	if err := quetzal.Write(&buf, story, save); err != nil {
		t.Fatalf("Write: %v", err)
	}
	return buf.Bytes()
}
