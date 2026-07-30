// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package quetzal_test

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/maloquacious/quetzal"
)

// localDir holds story files and saves a maintainer fetched for themselves.
// Nothing in it is committed, because no story of a Z-machine version other
// than 3 carries a licence permitting redistribution — see D43 and
// testdata/local/README.md.
//
// Everything here therefore skips when the directory is empty, which is how it
// is in a fresh clone. These tests add coverage for whoever has the files; they
// never take it away from anyone who does not.
const localDir = "testdata/local"

// localFiles returns the paths in testdata/local matching a glob, or nothing.
func localFiles(t *testing.T, pattern string) []string {
	t.Helper()

	matches, err := filepath.Glob(filepath.Join(localDir, pattern))
	if err != nil {
		t.Fatalf("listing %s: %v", localDir, err)
	}
	return matches
}

// TestLocalStories checks every story a maintainer has fetched, which is the
// only way this package sees a Z-machine version other than 3.
//
// The checksum is the point. A story states its own checksum at $1C, and the
// scale factor used to find the end of the story differs by version band — 2
// for versions 1 to 3, 4 for 4 and 5, 8 for 6 and up. The committed fixtures
// are all version 3 and so exercise only the first of those. A version 5 or 6
// story is the only thing that can confirm the others.
func TestLocalStories(t *testing.T) {
	paths := localFiles(t, "*.z[1-8]")
	if len(paths) == 0 {
		t.Skipf("no story files in %s; see %s/README.md for how to fetch them", localDir, localDir)
	}

	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			image, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("reading the story image: %v", err)
			}

			story, err := quetzal.ParseStory(image)
			if err != nil {
				t.Fatalf("ParseStory: %v", err)
			}

			stored := binary.BigEndian.Uint16(image[0x1c:0x1e])
			if stored == 0 {
				// A pre-checksum story, which is what D27 exists for and
				// what nobody has found. Say so loudly if one turns up.
				if !story.ChecksumComputed {
					t.Error("the story carries no checksum and none was computed")
				}
				t.Logf("version %d, %s, checksum COMPUTED as %#04x — this is a D27 fixture, "+
					"the first one seen; record it in spec-deltas.md",
					story.Version, story.Identity(), story.Checksum)
				return
			}

			computed, ok := quetzal.StoryChecksum(image)
			if !ok {
				t.Fatal("StoryChecksum: got ok=false; the story declares no usable length")
			}
			if computed != stored {
				t.Errorf("StoryChecksum: got %#04x, but the header stores %#04x", computed, stored)
			}
			if story.ChecksumComputed {
				t.Error("ChecksumComputed: got true for a story that stores its own checksum")
			}
			t.Logf("version %d, %s, %d bytes dynamic, checksum %#04x confirmed",
				story.Version, story.Identity(), len(story.DynamicMemory), stored)
		})
	}
}

// TestLocalSaves reads any save a maintainer has made from those stories.
//
// Its reason for existing is D16: the discard bit is set only by the CALL_xN
// opcodes, which arrived in Z-machine version 5, so no save of a version 3
// story can carry a frame that has it. A version 5 save is the only way to see
// that path exercised by a real interpreter, and to find out what a real
// interpreter puts in the result byte when the standard calls it meaningless.
func TestLocalSaves(t *testing.T) {
	saves := localFiles(t, "*.qzl")
	if len(saves) == 0 {
		t.Skipf("no saves in %s; see %s/README.md for how to make them", localDir, localDir)
	}

	stories := localFiles(t, "*.z[1-8]")
	if len(stories) == 0 {
		t.Skipf("saves in %s but no stories to read them against", localDir)
	}

	for _, path := range saves {
		t.Run(filepath.Base(path), func(t *testing.T) {
			blob, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("reading the save: %v", err)
			}

			// Try each story until one matches, since the naming here is
			// the maintainer's own and cannot be relied on.
			var save *quetzal.Save
			var story quetzal.Story
			for _, storyPath := range stories {
				image, err := os.ReadFile(storyPath)
				if err != nil {
					t.Fatalf("reading %s: %v", storyPath, err)
				}
				candidate, err := quetzal.ParseStory(image)
				if err != nil {
					continue
				}
				if s, err := quetzal.Read(bytes.NewReader(blob), candidate); err == nil {
					save, story = s, candidate
					break
				}
			}
			if save == nil {
				t.Fatalf("no story in %s reads this save", localDir)
			}

			t.Logf("version %d, %s, PC %#x, %s, %d frames",
				story.Version, save.Header.Identity(), save.Header.PC,
				save.Memory.Encoding, len(save.Frames))

			// The dummy frame rule runs the other way for version 6.
			if story.Version != 6 && !save.Frames[0].IsDummy() {
				t.Error("the first frame is not the dummy frame")
			}
			if story.Version == 6 {
				t.Logf("a version 6 save: D9's other half, where no dummy frame is expected")
			}

			var discards int
			masks := map[uint8]bool{}
			for i, frame := range save.Frames {
				masks[frame.Arguments] = true
				if !frame.DiscardResult {
					continue
				}
				discards++

				// D16. The standard calls the result byte meaningless
				// when the p bit is set and asks writers to store zero.
				// What a real writer actually stores is the question.
				t.Logf("frame %d discards its result; the writer stored %#02x in the result byte",
					i, frame.ResultVariable)
				if frame.ResultVariable != 0 {
					t.Logf("  that is not zero, so this writer does not follow 4.6; "+
						"our writer would normalize it to zero (D16). Story: %s", story.Identity())
				}
			}
			t.Logf("%d of %d frame(s) discard their result; %d distinct argument mask(s)",
				discards, len(save.Frames), len(masks))

			// The round trip must hold for these files as for any other,
			// including the one asymmetry D16 permits.
			var buf bytes.Buffer
			if err := quetzal.Write(&buf, story, save); err != nil {
				t.Fatalf("Write: %v", err)
			}
			again, err := quetzal.Read(bytes.NewReader(buf.Bytes()), story)
			if err != nil {
				t.Fatalf("Read(rewritten): %v", err)
			}
			if !bytes.Equal(again.Memory.Data, save.Memory.Data) {
				t.Error("dynamic memory did not survive the round trip")
			}

			want := cloneFrames(save.Frames)
			for i := range want {
				if want[i].DiscardResult {
					want[i].ResultVariable = 0
				}
			}
			if !framesEqual(again.Frames, want) {
				t.Error("the call stack did not survive the round trip")
			}
		})
	}
}
