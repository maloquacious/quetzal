// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package quetzal

import (
	"encoding/binary"
	"fmt"
	"strconv"
)

const (
	// ifhdSize is the size of the IFhd payload defined by Quetzal 1.4. It is
	// odd, so an IFhd chunk is always written with a pad byte.
	ifhdSize = 13

	// MaxPC is the largest program counter a Quetzal file can represent,
	// since program counters are stored as three bytes.
	MaxPC = 0xffffff
)

// Serial is a story's six-byte serial number.
//
// It is held as raw bytes. Although Infocom serial numbers are conventionally
// a date in YYMMDD form, that is not guaranteed, so this package never parses
// one or assumes it is a date or an integer.
type Serial [6]byte

// String renders the serial as its six characters, quoting it instead if it
// holds bytes outside the printable ASCII range.
func (s Serial) String() string {
	for _, c := range s {
		if c < 0x20 || c > 0x7e {
			return strconv.Quote(string(s[:]))
		}
	}
	return string(s[:])
}

// Identity is the triple that identifies the story a save belongs to: the
// values at offsets $2, $12, and $1C of the Z-machine header. Interpreters
// compare these and refuse to restore a save whose identity differs.
type Identity struct {
	Release  uint16
	Serial   Serial
	Checksum uint16
}

// String describes the identity in a form suitable for error messages.
func (i Identity) String() string {
	return fmt.Sprintf("release %d, serial %s, checksum %#04x", i.Release, i.Serial, i.Checksum)
}

// Header is the content of a save's IFhd chunk: the identity of the story the
// save belongs to, and the program counter at which it was saved.
type Header struct {
	Release  uint16
	Serial   Serial
	Checksum uint16

	// PC is the saved program counter, held in three bytes on disk and so
	// never greater than MaxPC.
	//
	// What it points at depends on the Z-machine version. On versions 3 and
	// below it addresses the branch data of the SAVE instruction; on
	// versions 4 and above it addresses the byte describing where SAVE
	// stores its result.
	PC uint32

	// Extra holds any bytes beyond the 13 this version of Quetzal defines.
	// The standard anticipates a larger IFhd in a future revision and
	// guarantees that the first 13 bytes will keep their meaning, so such
	// bytes are preserved rather than rejected or discarded.
	Extra []byte
}

// Identity returns the story identity recorded in the save.
func (h Header) Identity() Identity {
	return Identity{Release: h.Release, Serial: h.Serial, Checksum: h.Checksum}
}

// Matches reports whether the save belongs to the given story, comparing
// release number, serial number, and checksum.
//
// A story predating checksums carries zero at offset $1C. Quetzal expects a
// checksum computed from the story image to be saved in that case, so a save
// written that way will not match a Story parsed straight from such an image.
func (h Header) Matches(story Story) bool {
	return h.Identity() == story.Identity()
}

// Verify reports whether the save belongs to the given story, describing the
// difference when it does not. The returned error matches ErrStoryMismatch and
// can be inspected with errors.As as a *StoryMismatchError.
func (h Header) Verify(story Story) error {
	if h.Matches(story) {
		return nil
	}
	return prefixed(&StoryMismatchError{Save: h.Identity(), Story: story.Identity()})
}

// Validate reports whether the header can be represented in Quetzal.
func (h Header) Validate() error {
	if h.PC > MaxPC {
		return prefixed(newErr(ErrInvalidFormat,
			"IFhd: program counter %#x exceeds the 24-bit maximum %#x", h.PC, MaxPC))
	}
	return nil
}

// ParseHeader decodes the payload of an IFhd chunk.
//
// A payload longer than 13 bytes is accepted and its remainder kept in
// Header.Extra, because the standard reserves the right to extend IFhd while
// preserving the meaning of these first 13 bytes.
func ParseHeader(data []byte) (Header, error) {
	if len(data) < ifhdSize {
		return Header{}, prefixed(newErr(ErrInvalidFormat,
			"IFhd: payload is %d byte(s), want at least %d", len(data), ifhdSize))
	}

	h := Header{
		Release:  binary.BigEndian.Uint16(data[0:2]),
		Checksum: binary.BigEndian.Uint16(data[8:10]),
		PC:       be24(data[10:13]),
	}
	copy(h.Serial[:], data[2:8])
	if len(data) > ifhdSize {
		h.Extra = append([]byte(nil), data[ifhdSize:]...)
	}
	return h, nil
}

// Encode returns the payload of an IFhd chunk describing the header. Any bytes
// in Extra are appended, so a header read from a longer IFhd re-encodes to the
// same payload.
func (h Header) Encode() ([]byte, error) {
	if err := h.Validate(); err != nil {
		return nil, err
	}

	b := make([]byte, ifhdSize, ifhdSize+len(h.Extra))
	binary.BigEndian.PutUint16(b[0:2], h.Release)
	copy(b[2:8], h.Serial[:])
	binary.BigEndian.PutUint16(b[8:10], h.Checksum)
	putBE24(b[10:13], h.PC)
	return append(b, h.Extra...), nil
}

// Header returns the save's story identification, decoded from the first IFhd
// chunk in the file. Later IFhd chunks, if any, are ignored.
func (f *File) Header() (Header, error) {
	c, ok := f.First(IDIFhd)
	if !ok {
		return Header{}, prefixed(newErr(ErrInvalidFormat, "missing %s chunk", IDIFhd))
	}
	return ParseHeader(c.Data)
}

// be24 reads a three-byte big-endian program counter.
func be24(b []byte) uint32 {
	return uint32(b[0])<<16 | uint32(b[1])<<8 | uint32(b[2])
}

// putBE24 writes a three-byte big-endian program counter. The caller must
// have established that v is no greater than MaxPC.
func putBE24(b []byte, v uint32) {
	b[0] = byte(v >> 16)
	b[1] = byte(v >> 8)
	b[2] = byte(v)
}
