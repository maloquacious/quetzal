// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package quetzal_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/maloquacious/quetzal"
)

// ifhdPayload is a well-formed 13-byte IFhd payload: release 88, serial
// 840726, checksum 0x1234, PC 0x012345.
var ifhdPayload = []byte{
	0x00, 0x58,
	'8', '4', '0', '7', '2', '6',
	0x12, 0x34,
	0x01, 0x23, 0x45,
}

// wantHeader is the header ifhdPayload describes.
var wantHeader = quetzal.Header{
	Release:  88,
	Serial:   quetzal.Serial{'8', '4', '0', '7', '2', '6'},
	Checksum: 0x1234,
	PC:       0x012345,
}

func TestParseHeader(t *testing.T) {
	got, err := quetzal.ParseHeader(ifhdPayload)
	if err != nil {
		t.Fatalf("ParseHeader: unexpected error: %v", err)
	}
	if got.Release != wantHeader.Release {
		t.Errorf("Release: got %d, want %d", got.Release, wantHeader.Release)
	}
	if got.Serial != wantHeader.Serial {
		t.Errorf("Serial: got %s, want %s", got.Serial, wantHeader.Serial)
	}
	if got.Checksum != wantHeader.Checksum {
		t.Errorf("Checksum: got %#04x, want %#04x", got.Checksum, wantHeader.Checksum)
	}
	if got.PC != wantHeader.PC {
		t.Errorf("PC: got %#x, want %#x", got.PC, wantHeader.PC)
	}
	if got.Extra != nil {
		t.Errorf("Extra: got %x, want nil", got.Extra)
	}
}

func TestParseHeaderPCRange(t *testing.T) {
	// A three-byte program counter spans 0 to MaxPC, and every value in that
	// range must survive a decode.
	tests := []struct {
		name string
		pc   []byte
		want uint32
	}{
		{name: "zero", pc: []byte{0x00, 0x00, 0x00}, want: 0},
		{name: "low byte only", pc: []byte{0x00, 0x00, 0xff}, want: 0xff},
		{name: "middle byte", pc: []byte{0x00, 0xff, 0x00}, want: 0xff00},
		{name: "high byte", pc: []byte{0xff, 0x00, 0x00}, want: 0xff0000},
		{name: "maximum", pc: []byte{0xff, 0xff, 0xff}, want: quetzal.MaxPC},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := append(append([]byte(nil), ifhdPayload[:10]...), tt.pc...)
			h, err := quetzal.ParseHeader(payload)
			if err != nil {
				t.Fatalf("ParseHeader: unexpected error: %v", err)
			}
			if h.PC != tt.want {
				t.Errorf("PC: got %#x, want %#x", h.PC, tt.want)
			}
		})
	}
}

func TestParseHeaderTooShort(t *testing.T) {
	// The standard fixes the IFhd payload at 13 bytes; anything shorter
	// cannot carry the required fields.
	for n := range 13 {
		_, err := quetzal.ParseHeader(ifhdPayload[:n])
		if !errors.Is(err, quetzal.ErrInvalidFormat) {
			t.Errorf("ParseHeader(%d bytes): got %v, want ErrInvalidFormat", n, err)
		}
	}
}

func TestParseHeaderLongerPayload(t *testing.T) {
	// The standard reserves the right to extend IFhd while keeping the
	// meaning of the first 13 bytes, so a longer payload is not an error and
	// the remainder must be preserved.
	extra := []byte{0xde, 0xad, 0xbe, 0xef}
	payload := append(append([]byte(nil), ifhdPayload...), extra...)

	h, err := quetzal.ParseHeader(payload)
	if err != nil {
		t.Fatalf("ParseHeader: unexpected error: %v", err)
	}
	if h.PC != wantHeader.PC {
		t.Errorf("PC: got %#x, want %#x", h.PC, wantHeader.PC)
	}
	if !bytes.Equal(h.Extra, extra) {
		t.Errorf("Extra: got %x, want %x", h.Extra, extra)
	}

	// Mutating the caller's buffer must not disturb the parsed header.
	for i := range payload {
		payload[i] = 0
	}
	if !bytes.Equal(h.Extra, extra) {
		t.Errorf("Extra aliases the input: got %x, want %x", h.Extra, extra)
	}
}

func TestHeaderEncodeRoundTrip(t *testing.T) {
	t.Run("defined fields", func(t *testing.T) {
		got, err := wantHeader.Encode()
		if err != nil {
			t.Fatalf("Encode: unexpected error: %v", err)
		}
		if !bytes.Equal(got, ifhdPayload) {
			t.Fatalf("Encode: got %x, want %x", got, ifhdPayload)
		}
		// The payload is odd, which is why an IFhd chunk needs a pad byte.
		if len(got)%2 != 1 {
			t.Errorf("Encode: got an even payload of %d bytes, want 13", len(got))
		}
	})

	t.Run("extra bytes are written back", func(t *testing.T) {
		h := wantHeader
		h.Extra = []byte{0x01, 0x02}

		payload, err := h.Encode()
		if err != nil {
			t.Fatalf("Encode: unexpected error: %v", err)
		}
		back, err := quetzal.ParseHeader(payload)
		if err != nil {
			t.Fatalf("ParseHeader: unexpected error: %v", err)
		}
		if back.Identity() != h.Identity() || back.PC != h.PC {
			t.Errorf("round trip: got %+v, want %+v", back, h)
		}
		if !bytes.Equal(back.Extra, h.Extra) {
			t.Errorf("Extra: got %x, want %x", back.Extra, h.Extra)
		}
	})
}

func TestHeaderValidatePC(t *testing.T) {
	// A program counter is written in three bytes, so a larger value cannot
	// be represented and must be refused rather than silently truncated.
	h := wantHeader
	h.PC = quetzal.MaxPC + 1

	if err := h.Validate(); !errors.Is(err, quetzal.ErrInvalidFormat) {
		t.Errorf("Validate: got %v, want ErrInvalidFormat", err)
	}
	if _, err := h.Encode(); !errors.Is(err, quetzal.ErrInvalidFormat) {
		t.Errorf("Encode: got %v, want ErrInvalidFormat", err)
	}

	h.PC = quetzal.MaxPC
	if err := h.Validate(); err != nil {
		t.Errorf("Validate(MaxPC): unexpected error: %v", err)
	}
}

func TestHeaderMatches(t *testing.T) {
	story := quetzal.Story{
		Version:  3,
		Release:  88,
		Serial:   quetzal.Serial{'8', '4', '0', '7', '2', '6'},
		Checksum: 0x1234,
	}

	t.Run("matching story", func(t *testing.T) {
		if !wantHeader.Matches(story) {
			t.Error("Matches: got false, want true")
		}
		if err := wantHeader.Verify(story); err != nil {
			t.Errorf("Verify: unexpected error: %v", err)
		}
	})

	t.Run("a differing PC does not affect identity", func(t *testing.T) {
		// The program counter is saved state, not part of the story's
		// identity.
		h := wantHeader
		h.PC = 0xabcdef
		if !h.Matches(story) {
			t.Error("Matches: got false, want true")
		}
	})

	tests := []struct {
		name   string
		mutate func(*quetzal.Story)
	}{
		{"release differs", func(s *quetzal.Story) { s.Release = 89 }},
		{"serial differs", func(s *quetzal.Story) { s.Serial = quetzal.Serial{'8', '4', '0', '7', '2', '7'} }},
		{"checksum differs", func(s *quetzal.Story) { s.Checksum = 0x1235 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			other := story
			tt.mutate(&other)

			if wantHeader.Matches(other) {
				t.Error("Matches: got true, want false")
			}

			err := wantHeader.Verify(other)
			if !errors.Is(err, quetzal.ErrStoryMismatch) {
				t.Fatalf("Verify: got %v, want ErrStoryMismatch", err)
			}

			// The error should say which story was expected and which
			// was supplied.
			var me *quetzal.StoryMismatchError
			if !errors.As(err, &me) {
				t.Fatalf("Verify: got %T, want a *StoryMismatchError", err)
			}
			if me.Save != wantHeader.Identity() {
				t.Errorf("StoryMismatchError.Save: got %s, want %s", me.Save, wantHeader.Identity())
			}
			if me.Story != other.Identity() {
				t.Errorf("StoryMismatchError.Story: got %s, want %s", me.Story, other.Identity())
			}
			if msg := err.Error(); !strings.HasPrefix(msg, "quetzal: ") {
				t.Errorf("error message %q lacks the package prefix", msg)
			}
		})
	}
}

func TestFileHeader(t *testing.T) {
	t.Run("decodes the first IFhd", func(t *testing.T) {
		// A second IFhd is ignored: where Quetzal expects one instance, the
		// first is authoritative.
		other := append(append([]byte(nil), ifhdPayload[:10]...), 0xff, 0xff, 0xff)
		in := ifzs(
			chunkBytes("IFhd", ifhdPayload),
			chunkBytes("IFhd", other),
		)

		f, err := quetzal.Decode(bytes.NewReader(in))
		if err != nil {
			t.Fatalf("Decode: unexpected error: %v", err)
		}
		h, err := f.Header()
		if err != nil {
			t.Fatalf("Header: unexpected error: %v", err)
		}
		if h.PC != wantHeader.PC {
			t.Errorf("PC: got %#x, want %#x (the first IFhd wins)", h.PC, wantHeader.PC)
		}
	})

	t.Run("reports a missing IFhd", func(t *testing.T) {
		f, err := quetzal.Decode(bytes.NewReader(ifzs(chunkBytes("ANNO", []byte("hi")))))
		if err != nil {
			t.Fatalf("Decode: unexpected error: %v", err)
		}
		if _, err := f.Header(); !errors.Is(err, quetzal.ErrInvalidFormat) {
			t.Errorf("Header: got %v, want ErrInvalidFormat", err)
		}
	})

	t.Run("reports a malformed IFhd", func(t *testing.T) {
		f, err := quetzal.Decode(bytes.NewReader(ifzs(chunkBytes("IFhd", []byte{1, 2, 3}))))
		if err != nil {
			t.Fatalf("Decode: unexpected error: %v", err)
		}
		if _, err := f.Header(); !errors.Is(err, quetzal.ErrInvalidFormat) {
			t.Errorf("Header: got %v, want ErrInvalidFormat", err)
		}
	})
}

func TestSerialString(t *testing.T) {
	tests := []struct {
		name   string
		serial quetzal.Serial
		want   string
	}{
		{"printable", quetzal.Serial{'8', '4', '0', '7', '2', '6'}, "840726"},
		{"non-printable is quoted", quetzal.Serial{0, 1, 2, 3, 4, 5}, `"\x00\x01\x02\x03\x04\x05"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.serial.String(); got != tt.want {
				t.Errorf("String: got %s, want %s", got, tt.want)
			}
		})
	}
}
