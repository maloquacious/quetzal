// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package quetzal_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"

	"github.com/maloquacious/quetzal"
)

// storyImage builds a synthetic Z-machine story image: a 64-byte header with
// the fields Quetzal reads, followed by enough bytes to reach size.
func storyImage(version uint8, release uint16, serial string, checksum uint16, staticBase, size int) []byte {
	data := make([]byte, size)
	data[0x00] = version
	binary.BigEndian.PutUint16(data[0x02:0x04], release)
	binary.BigEndian.PutUint16(data[0x0e:0x10], uint16(staticBase))
	copy(data[0x12:0x18], serial)
	binary.BigEndian.PutUint16(data[0x1c:0x1e], checksum)

	// Mark dynamic memory so tests can tell which bytes were taken.
	for i := 0x40; i < staticBase && i < size; i++ {
		data[i] = 0xaa
	}
	// Static memory must not be copied into the Story.
	for i := staticBase; i < size; i++ {
		data[i] = 0x55
	}
	return data
}

func TestParseStory(t *testing.T) {
	image := storyImage(3, 88, "840726", 0x1234, 0x80, 0x200)

	story, err := quetzal.ParseStory(image)
	if err != nil {
		t.Fatalf("ParseStory: unexpected error: %v", err)
	}

	if story.Version != 3 {
		t.Errorf("Version: got %d, want 3", story.Version)
	}
	if story.Release != 88 {
		t.Errorf("Release: got %d, want 88", story.Release)
	}
	if want := (quetzal.Serial{'8', '4', '0', '7', '2', '6'}); story.Serial != want {
		t.Errorf("Serial: got %s, want %s", story.Serial, want)
	}
	if story.Checksum != 0x1234 {
		t.Errorf("Checksum: got %#04x, want %#04x", story.Checksum, 0x1234)
	}

	// Dynamic memory runs from 0 up to the base of static memory, so the
	// base is also its length.
	if got, want := len(story.DynamicMemory), 0x80; got != want {
		t.Fatalf("DynamicMemory length: got %d, want %d", got, want)
	}
	if bytes.ContainsRune(story.DynamicMemory[0x40:], 0x55) {
		t.Error("DynamicMemory contains bytes from static memory")
	}
	if story.DynamicMemory[0x00] != 3 {
		t.Error("DynamicMemory does not begin with the story header")
	}
}

func TestParseStoryDoesNotAliasOrMutateInput(t *testing.T) {
	// Parsing must neither retain nor modify the caller's story buffer.
	image := storyImage(5, 1, "abcdef", 0x9999, 0x100, 0x180)
	original := append([]byte(nil), image...)

	story, err := quetzal.ParseStory(image)
	if err != nil {
		t.Fatalf("ParseStory: unexpected error: %v", err)
	}
	if !bytes.Equal(image, original) {
		t.Error("ParseStory modified the caller's story image")
	}

	for i := range image {
		image[i] = 0xff
	}
	if !bytes.Equal(story.DynamicMemory, original[:0x100]) {
		t.Error("Story.DynamicMemory aliases the caller's story image")
	}
}

func TestParseStoryVersions(t *testing.T) {
	// Quetzal 1.4 covers Z-machine versions 1 through 8.
	for v := 1; v <= 8; v++ {
		image := storyImage(uint8(v), 1, "000000", 0, 0x80, 0x100)
		story, err := quetzal.ParseStory(image)
		if err != nil {
			t.Errorf("ParseStory(version %d): unexpected error: %v", v, err)
			continue
		}
		if story.Version != uint8(v) {
			t.Errorf("Version: got %d, want %d", story.Version, v)
		}
	}

	for _, v := range []uint8{0, 9, 0xff} {
		image := storyImage(v, 1, "000000", 0, 0x80, 0x100)
		if _, err := quetzal.ParseStory(image); !errors.Is(err, quetzal.ErrInvalidFormat) {
			t.Errorf("ParseStory(version %d): got %v, want ErrInvalidFormat", v, err)
		}
	}
}

func TestParseStoryMalformed(t *testing.T) {
	tests := []struct {
		name  string
		image []byte
		want  error
	}{
		{
			name:  "empty image",
			image: nil,
			want:  quetzal.ErrInvalidFormat,
		},
		{
			name:  "shorter than the Z-machine header",
			image: storyImage(3, 1, "000000", 0, 0x80, 0x100)[:0x3f],
			want:  quetzal.ErrInvalidFormat,
		},
		{
			name: "static memory inside the header",
			// Dynamic memory must at least contain the header, so a base
			// below 0x40 is impossible.
			image: storyImage(3, 1, "000000", 0, 0x20, 0x100),
			want:  quetzal.ErrInvalidFormat,
		},
		{
			name:  "static memory at zero",
			image: storyImage(3, 1, "000000", 0, 0x00, 0x100),
			want:  quetzal.ErrInvalidFormat,
		},
		{
			name: "static memory beyond the image",
			// The image is truncated: it does not hold the dynamic memory
			// its own header describes.
			image: storyImage(3, 1, "000000", 0, 0x200, 0x100),
			want:  quetzal.ErrTruncated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			story, err := quetzal.ParseStory(tt.image)
			if err == nil {
				t.Fatalf("ParseStory: got %d byte(s) of dynamic memory and no error, want %v",
					len(story.DynamicMemory), tt.want)
			}
			if !errors.Is(err, tt.want) {
				t.Errorf("ParseStory: got %v, want an error matching %v", err, tt.want)
			}
		})
	}
}

func TestParseStoryBoundaryStaticBase(t *testing.T) {
	t.Run("static memory immediately after the header", func(t *testing.T) {
		// The smallest legal dynamic memory is the header alone.
		story, err := quetzal.ParseStory(storyImage(3, 1, "000000", 0, 0x40, 0x100))
		if err != nil {
			t.Fatalf("ParseStory: unexpected error: %v", err)
		}
		if got := len(story.DynamicMemory); got != 0x40 {
			t.Errorf("DynamicMemory length: got %d, want %d", got, 0x40)
		}
	})

	t.Run("story that is entirely dynamic memory", func(t *testing.T) {
		// A base equal to the image length is in range: every byte of the
		// image is dynamic memory.
		story, err := quetzal.ParseStory(storyImage(3, 1, "000000", 0, 0x100, 0x100))
		if err != nil {
			t.Fatalf("ParseStory: unexpected error: %v", err)
		}
		if got := len(story.DynamicMemory); got != 0x100 {
			t.Errorf("DynamicMemory length: got %d, want %d", got, 0x100)
		}
	})
}

// setFileLength writes the scaled story length that the Z-machine header holds
// at $1A, which is what a checksum is computed over. storyImage leaves it zero,
// as the early games that need a computed checksum tend to.
func setFileLength(image []byte, length int) []byte {
	scale := 2
	switch v := image[0x00]; {
	case v <= 3:
		scale = 2
	case v <= 5:
		scale = 4
	default:
		scale = 8
	}
	binary.BigEndian.PutUint16(image[0x1a:0x1c], uint16(length/scale))
	return image
}

// sumBytes adds up a range of an image the way the Z-machine checksum does,
// independently of the package, so that the test is not merely the
// implementation restated.
func sumBytes(image []byte, from, to int) uint16 {
	var total uint32
	for _, b := range image[from:to] {
		total += uint32(b)
	}
	return uint16(total)
}

func TestStoryChecksum(t *testing.T) {
	// storyImage fills dynamic memory with 0xaa and static memory with 0x55,
	// so the sum is over a known pattern rather than over zeros.
	image := setFileLength(storyImage(3, 1, "000000", 0, 0x80, 0x200), 0x200)

	got, ok := quetzal.StoryChecksum(image)
	if !ok {
		t.Fatal("StoryChecksum: got ok=false, want a checksum")
	}
	if want := sumBytes(image, 0x40, 0x200); got != want {
		t.Errorf("StoryChecksum: got %#04x, want %#04x", got, want)
	}

	t.Run("the declared length is what is summed, not the image size", func(t *testing.T) {
		// A story file may carry padding past its declared end, which the
		// checksum must not include.
		padded := append(append([]byte(nil), image...), bytes.Repeat([]byte{0xff}, 0x100)...)
		padded = setFileLength(padded, 0x200)

		got, ok := quetzal.StoryChecksum(padded)
		if !ok {
			t.Fatal("StoryChecksum: got ok=false, want a checksum")
		}
		if want := sumBytes(image, 0x40, 0x200); got != want {
			t.Errorf("StoryChecksum: got %#04x, want %#04x; the padding was summed", got, want)
		}
	})

	t.Run("the scale factor follows the version", func(t *testing.T) {
		// The same declared length means a different word at $1A in each
		// version band, so a scale mix-up shows up as a different sum.
		for _, v := range []uint8{1, 3, 4, 5, 6, 8} {
			image := setFileLength(storyImage(v, 1, "000000", 0, 0x80, 0x400), 0x400)

			got, ok := quetzal.StoryChecksum(image)
			if !ok {
				t.Errorf("version %d: StoryChecksum: got ok=false", v)
				continue
			}
			if want := sumBytes(image, 0x40, 0x400); got != want {
				t.Errorf("version %d: got %#04x, want %#04x", v, got, want)
			}
		}
	})
}

func TestStoryChecksumUncomputable(t *testing.T) {
	tests := []struct {
		name  string
		image []byte
	}{
		{
			name: "no declared length, as the earliest games leave it",
			// storyImage leaves $1A zero.
			image: storyImage(3, 1, "000000", 0, 0x80, 0x200),
		},
		{
			name:  "a declared length reaching past the image",
			image: setFileLength(storyImage(3, 1, "000000", 0, 0x80, 0x200), 0x400),
		},
		{
			name:  "a declared length of nothing but the header",
			image: setFileLength(storyImage(3, 1, "000000", 0, 0x40, 0x100), 0x40),
		},
		{
			name:  "an image shorter than the header",
			image: storyImage(3, 1, "000000", 0, 0x40, 0x100)[:0x20],
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, ok := quetzal.StoryChecksum(tt.image); ok {
				t.Errorf("StoryChecksum: got %#04x with ok=true, want ok=false", got)
			}
		})
	}
}

// TestParseStoryComputesAMissingChecksum covers standard 5.5: a story with no
// checksum of its own gets one computed, so that the identity this package
// records is the one a conforming interpreter would record.
func TestParseStoryComputesAMissingChecksum(t *testing.T) {
	image := setFileLength(storyImage(3, 42, "820101", 0, 0x80, 0x200), 0x200)

	story, err := quetzal.ParseStory(image)
	if err != nil {
		t.Fatalf("ParseStory: unexpected error: %v", err)
	}
	if !story.ChecksumComputed {
		t.Error("ChecksumComputed: got false, want true")
	}
	if want := sumBytes(image, 0x40, 0x200); story.Checksum != want {
		t.Errorf("Checksum: got %#04x, want the computed %#04x", story.Checksum, want)
	}
	if story.Checksum == 0 {
		t.Fatal("the fixture sums to zero, so nothing was tested")
	}

	t.Run("a save recording the computed checksum matches", func(t *testing.T) {
		// The point of computing it: a save written by a conforming
		// interpreter carries the computed value, and must match.
		header := quetzal.Header{
			Release:  story.Release,
			Serial:   story.Serial,
			Checksum: story.Checksum,
		}
		if err := header.Verify(story); err != nil {
			t.Errorf("Verify: unexpected error: %v", err)
		}

		// And a save carrying the literal zero from the header does not,
		// which is the behavior before this change.
		header.Checksum = 0
		if err := header.Verify(story); !errors.Is(err, quetzal.ErrStoryMismatch) {
			t.Errorf("Verify(zero checksum): got %v, want ErrStoryMismatch", err)
		}
	})
}

// TestParseStoryKeepsAStoredChecksum is the other half, and the more important
// one: a checksum the header does carry is never recomputed or second-guessed.
// Interpreters compare the stored value, so substituting our own arithmetic
// would break the matching this is meant to make work.
func TestParseStoryKeepsAStoredChecksum(t *testing.T) {
	// A stored checksum that deliberately disagrees with what the image sums
	// to.
	image := setFileLength(storyImage(3, 42, "820101", 0x0001, 0x80, 0x200), 0x200)
	if computed, ok := quetzal.StoryChecksum(image); !ok || computed == 0x0001 {
		t.Fatalf("the fixture's computed checksum is %#04x, which does not disagree with the stored one", computed)
	}

	story, err := quetzal.ParseStory(image)
	if err != nil {
		t.Fatalf("ParseStory: unexpected error: %v", err)
	}
	if story.Checksum != 0x0001 {
		t.Errorf("Checksum: got %#04x, want the stored 0x0001", story.Checksum)
	}
	if story.ChecksumComputed {
		t.Error("ChecksumComputed: got true, want false")
	}
}

// TestParseStoryLeavesAnUncomputableChecksumAlone covers the case this package
// cannot fix: a story with neither a checksum nor a usable length. Nothing is
// invented, and the caller can tell from ChecksumComputed that nothing was.
func TestParseStoryLeavesAnUncomputableChecksumAlone(t *testing.T) {
	story, err := quetzal.ParseStory(storyImage(3, 1, "000000", 0, 0x80, 0x200))
	if err != nil {
		t.Fatalf("ParseStory: unexpected error: %v", err)
	}
	if story.Checksum != 0 {
		t.Errorf("Checksum: got %#04x, want 0", story.Checksum)
	}
	if story.ChecksumComputed {
		t.Error("ChecksumComputed: got true, want false")
	}
}

func TestStoryIdentity(t *testing.T) {
	// A story and a save that came from it must report the same identity, so
	// that identity comparison is the whole of the matching rule.
	image := storyImage(3, 88, "840726", 0x1234, 0x80, 0x100)
	story, err := quetzal.ParseStory(image)
	if err != nil {
		t.Fatalf("ParseStory: unexpected error: %v", err)
	}

	if got, want := story.Identity(), wantHeader.Identity(); got != want {
		t.Errorf("Identity: got %s, want %s", got, want)
	}
	if !wantHeader.Matches(story) {
		t.Error("Matches: got false, want true")
	}
}
