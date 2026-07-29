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
	offChecksum     = 0x1c // 1 word:  checksum of the story file
)

// The range of Z-machine versions this package supports, as represented by
// Quetzal 1.4.
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
	Release  uint16
	Serial   Serial
	Checksum uint16

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
	return s, nil
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
