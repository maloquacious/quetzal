// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package quetzal

import (
	"encoding/binary"
	"fmt"
)

// Offsets and sizes within the Z-machine story header that Quetzal depends on.
const (
	storyHeaderSize = 0x40 // the Z-machine header occupies the first 64 bytes

	offVersion      = 0x00 // 1 byte:  Z-machine version
	offRelease      = 0x02 // 1 word:  release number
	offStaticMemory = 0x0e // 1 word:  byte address of the base of static memory
	offSerial       = 0x12 // 6 bytes: serial number
	offFileLength   = 0x1a // 1 word:  length of the story file, scaled
	offChecksum     = 0x1c // 1 word:  checksum of the story file
)

// The range of Z-machine versions this package supports, as represented by
// Quetzal 1.4.
//
// Quetzal is largely version-independent, so the code paths that differ by
// version are few: whether a save carries the dummy frame (version 6 does not),
// and how the story's declared length is scaled when a checksum has to be
// computed. Every version in this range is implemented. Not every one has been
// exercised against a real story file — see the version-coverage note in the
// README, which says which and why.
const (
	MinVersion = 1
	MaxVersion = 8
)

// Story carries the information about an original story image that Quetzal
// operations require. It is not a Z-machine and holds no execution state.
//
// A Story is needed because compressed memory (CMem) is stored as an XOR
// difference against the story's original dynamic memory and cannot be
// reconstructed without it. This package never looks for a story file itself;
// callers supply one.
type Story struct {
	// Version is the Z-machine version the story runs on.
	Version uint8

	// Release, Serial, and Checksum are the values at offsets $2, $12, and
	// $1C of the story header, which together identify the story.
	//
	// Checksum is not always the value stored at $1C. Games written before
	// the field was used carry zero there, and the format requires the
	// checksum to be computed from the story image in that case, so that a
	// save records an identity other interpreters agree with. See
	// ChecksumComputed and StoryChecksum.
	Release  uint16
	Serial   Serial
	Checksum uint16

	// ChecksumComputed reports that Checksum was computed from the story
	// image because the header carried none, rather than read from $1C.
	//
	// It is worth logging. A story that needs a computed checksum is old
	// enough that little else about it has been tested, and a save that
	// fails to match one is the first place to look.
	ChecksumComputed bool

	// DynamicMemory is the story's original dynamic memory: bytes 0 through
	// the base of static memory, exclusive. Its length is the length any
	// restored dynamic memory must have.
	DynamicMemory []byte
}

// Identity returns the triple that identifies this story.
func (s Story) Identity() Identity {
	return Identity{Release: s.Release, Serial: s.Serial, Checksum: s.Checksum}
}

// ParseStory extracts the information Quetzal needs from a complete Z-machine
// story image, reading the header fields that identify the story and taking
// the extent of dynamic memory from the base of static memory.
//
// The returned Story owns its dynamic memory. The caller's buffer is read but
// neither retained nor modified, so it remains safe to reuse or mutate.
func ParseStory(data []byte) (Story, error) {
	if len(data) < storyHeaderSize {
		return Story{}, prefixed(newErr(ErrInvalidFormat,
			"story image is %d byte(s), too short for a %d-byte Z-machine header",
			len(data), storyHeaderSize))
	}

	version := data[offVersion]
	if version < MinVersion || version > MaxVersion {
		return Story{}, prefixed(newErr(ErrInvalidFormat,
			"story image reports Z-machine version %d, want %d to %d",
			version, MinVersion, MaxVersion))
	}

	// Dynamic memory runs from address 0 up to the base of static memory, so
	// that base is also its length. It must leave room for the header and
	// must lie within the image.
	staticBase := int(binary.BigEndian.Uint16(data[offStaticMemory : offStaticMemory+2]))
	if staticBase < storyHeaderSize {
		return Story{}, prefixed(newErr(ErrInvalidFormat,
			"story image puts the base of static memory at %#x, inside the %#x-byte header",
			staticBase, storyHeaderSize))
	}
	if staticBase > len(data) {
		return Story{}, prefixed(newErr(ErrTruncated,
			"story image puts the base of static memory at %#x, beyond the end of the %d-byte image",
			staticBase, len(data)))
	}

	s := Story{
		Version:       version,
		Release:       binary.BigEndian.Uint16(data[offRelease : offRelease+2]),
		Checksum:      binary.BigEndian.Uint16(data[offChecksum : offChecksum+2]),
		DynamicMemory: append([]byte(nil), data[:staticBase]...),
	}
	copy(s.Serial[:], data[offSerial:offSerial+6])

	// A story from before the checksum field was used carries zero at $1C.
	// Standard 5.5 requires the saving interpreter to compute the value from
	// the story image in that case, so reading the field literally would
	// give this package an identity that no conforming interpreter shares.
	//
	// A stored checksum is never second-guessed. If $1C disagrees with what
	// the image sums to, the story is what it says it is: interpreters
	// compare the stored value and would all agree with each other, and
	// silently substituting our own arithmetic would break the very
	// matching this exists to make work.
	if s.Checksum == 0 {
		if sum, ok := StoryChecksum(data); ok {
			s.Checksum, s.ChecksumComputed = sum, true
		}
	}
	return s, nil
}

// StoryChecksum computes a story image's checksum the way the Z-machine
// defines it: the sum of every byte from the end of the 64-byte header to the
// end of the story, modulo 0x10000.
//
// Interpreters normally read this value from offset $1C rather than computing
// it, and Quetzal records it in IFhd so that a save can be matched to the story
// it belongs to. Games written before the field came into use carry zero there;
// standard 5.5 requires the value to be calculated instead, which is what this
// function is for. ParseStory calls it for exactly those stories.
//
// The length of the story comes from the header, not from the size of the
// image, because a story file may carry padding beyond its declared end. ok is
// false when the header declares no usable length — true of some of the same
// early games, which leave that field unused as well — and in that case no
// checksum can be computed from the image at all.
//
// The image is read but neither retained nor modified.
func StoryChecksum(data []byte) (checksum uint16, ok bool) {
	if len(data) < storyHeaderSize {
		return 0, false
	}

	// The declared length is scaled so that one word can describe a story
	// larger than 64 KB, by a factor that grew with the Z-machine version.
	length := int(binary.BigEndian.Uint16(data[offFileLength:offFileLength+2])) * fileLengthScale(data[offVersion])
	if length <= storyHeaderSize || length > len(data) {
		return 0, false
	}

	// The widest possible sum is 0xffff * 8 bytes of 0xff, which is far
	// inside the range of a uint32; truncating to uint16 is the modulo.
	var total uint32
	for _, b := range data[storyHeaderSize:length] {
		total += uint32(b)
	}
	return uint16(total), true
}

// fileLengthScale returns the factor by which the length at $1A is divided in
// the given Z-machine version.
func fileLengthScale(version uint8) int {
	switch {
	case version <= 3:
		return 2
	case version <= 5:
		return 4
	default:
		return 8
	}
}

// StoryMismatchError reports that a save does not belong to the story it was
// checked against, and carries both identities so a caller can say how they
// differ.
type StoryMismatchError struct {
	// Save is the identity recorded in the save's IFhd chunk.
	Save Identity

	// Story is the identity of the story image supplied by the caller.
	Story Identity
}

// Error implements the error interface.
func (e *StoryMismatchError) Error() string {
	return fmt.Sprintf("save is for %s, but story is %s", e.Save, e.Story)
}

// Unwrap reports this error as an ErrStoryMismatch for errors.Is.
func (e *StoryMismatchError) Unwrap() error { return ErrStoryMismatch }
