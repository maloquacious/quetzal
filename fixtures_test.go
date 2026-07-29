// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package quetzal_test

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"github.com/maloquacious/quetzal"
)

// storiesDir holds the Z-machine story images. Saves made from them live in
// testdata/frotz, kept apart because they are different kinds of thing: a
// story is an input this package never writes, while a save is what it reads
// and produces.
const storiesDir = "testdata/stories"

// storyFileName matches a story fixture's name, which carries the release
// number and serial number from the story's own header:
//
//	zork1-r119-880429.z3
//
// Encoding them in the name means a save paired with the wrong story is
// obvious on sight rather than at the point where a checksum fails.
var storyFileName = regexp.MustCompile(`^(.+?)-r(\d+)-(\d{6})\.z3$`)

// storyFixtures returns the name of every story image under testdata/stories.
func storyFixtures(t *testing.T) []string {
	t.Helper()

	matches, err := filepath.Glob(filepath.Join(storiesDir, "*.z3"))
	if err != nil {
		t.Fatalf("listing story fixtures: %v", err)
	}
	if len(matches) == 0 {
		t.Fatalf("no story fixtures in %s", storiesDir)
	}

	names := make([]string, len(matches))
	for i, m := range matches {
		names[i] = filepath.Base(m)
	}
	return names
}

// loadStory reads a story fixture and checks that the release and serial its
// name claims are the ones the story header actually holds, so that a
// misnamed or replaced fixture fails here rather than somewhere confusing.
func loadStory(t *testing.T, name string) quetzal.Story {
	t.Helper()

	image, err := os.ReadFile(filepath.Join(storiesDir, name))
	if err != nil {
		t.Fatalf("reading the story image: %v", err)
	}
	story, err := quetzal.ParseStory(image)
	if err != nil {
		t.Fatalf("ParseStory(%s): unexpected error: %v", name, err)
	}

	m := storyFileName.FindStringSubmatch(name)
	if m == nil {
		t.Fatalf("story fixture %s is not named <story>-r<release>-<serial>.z3", name)
	}
	release, err := strconv.ParseUint(m[2], 10, 16)
	if err != nil {
		t.Fatalf("story fixture %s claims an unusable release number: %v", name, err)
	}
	if uint16(release) != story.Release {
		t.Errorf("%s: the name claims release %d, but the story header says %d",
			name, release, story.Release)
	}
	if got := story.Serial.String(); got != m[3] {
		t.Errorf("%s: the name claims serial %s, but the story header says %s", name, m[3], got)
	}
	return story
}

// TestStoryFixtures checks every story fixture against its own name, and
// reports what each one is so that a failure elsewhere can be read against a
// known list.
func TestStoryFixtures(t *testing.T) {
	for _, name := range storyFixtures(t) {
		t.Run(name, func(t *testing.T) {
			story := loadStory(t, name)
			t.Logf("version %d, %s, %d bytes of dynamic memory",
				story.Version, story.Identity(), len(story.DynamicMemory))
		})
	}
}

// TestStoryFixtureChecksums checks the checksum algorithm against real story
// files, which is the only evidence available that it is the right algorithm.
//
// Every fixture stores a checksum at $1C, computed by Infocom's own tools, so
// recomputing it from the image must reproduce the stored value exactly. This
// is what makes the computed checksum trustworthy for the pre-checksum games
// that need it, where no stored value exists to check against — see D27 and
// D43 in spec-deltas.md.
func TestStoryFixtureChecksums(t *testing.T) {
	for _, name := range storyFixtures(t) {
		t.Run(name, func(t *testing.T) {
			image, err := os.ReadFile(filepath.Join(storiesDir, name))
			if err != nil {
				t.Fatalf("reading the story image: %v", err)
			}

			stored := binary.BigEndian.Uint16(image[0x1c:0x1e])
			if stored == 0 {
				t.Skip("this story carries no checksum, so there is nothing to check against")
			}

			computed, ok := quetzal.StoryChecksum(image)
			if !ok {
				t.Fatal("StoryChecksum: got ok=false; the story declares no usable length")
			}
			if computed != stored {
				t.Errorf("StoryChecksum: got %#04x, but the story header stores %#04x",
					computed, stored)
			}

			// And ParseStory must leave the stored value alone.
			story, err := quetzal.ParseStory(image)
			if err != nil {
				t.Fatalf("ParseStory: unexpected error: %v", err)
			}
			if story.ChecksumComputed {
				t.Error("ChecksumComputed: got true for a story that stores its own checksum")
			}
			t.Logf("stored %#04x, computed %#04x from %d bytes", stored, computed, len(image))
		})
	}
}
