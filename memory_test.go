// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package quetzal_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/maloquacious/quetzal"
)

// original is a short stand-in for a story's dynamic memory. Its bytes are
// distinct so that a misplaced difference shows up as a wrong value rather
// than a coincidence.
var original = []byte{0x10, 0x20, 0x30, 0x40, 0x50, 0x60, 0x70, 0x80}

func TestDecodeCMem(t *testing.T) {
	tests := []struct {
		name    string
		payload []byte
		want    []byte
	}{
		{
			name:    "empty payload leaves memory unchanged",
			payload: nil,
			want:    original,
		},
		{
			name:    "a non-zero byte is exclusive-ored in",
			payload: []byte{0x11},
			want:    []byte{0x01, 0x20, 0x30, 0x40, 0x50, 0x60, 0x70, 0x80},
		},
		{
			name: "a zero run skips unchanged bytes",
			// Skip 3, change the fourth.
			payload: []byte{0x00, 0x02, 0xff},
			want:    []byte{0x10, 0x20, 0x30, 0xbf, 0x50, 0x60, 0x70, 0x80},
		},
		{
			name:    "the shortest run is one byte",
			payload: []byte{0x00, 0x00, 0x01},
			want:    []byte{0x10, 0x21, 0x30, 0x40, 0x50, 0x60, 0x70, 0x80},
		},
		{
			name: "a truncated stream leaves the rest unchanged",
			// Only the first two bytes are described.
			payload: []byte{0x01, 0x02},
			want:    []byte{0x11, 0x22, 0x30, 0x40, 0x50, 0x60, 0x70, 0x80},
		},
		{
			name:    "a run may reach exactly the end",
			payload: []byte{0x00, 0x07},
			want:    original,
		},
		{
			name:    "consecutive runs are allowed",
			payload: []byte{0x00, 0x01, 0x00, 0x01, 0x0f},
			want:    []byte{0x10, 0x20, 0x30, 0x40, 0x5f, 0x60, 0x70, 0x80},
		},
		{
			name:    "a difference may cancel a byte to zero",
			payload: []byte{0x10},
			want:    []byte{0x00, 0x20, 0x30, 0x40, 0x50, 0x60, 0x70, 0x80},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := quetzal.DecodeCMem(tt.payload, original)
			if err != nil {
				t.Fatalf("DecodeCMem: unexpected error: %v", err)
			}
			if !bytes.Equal(got, tt.want) {
				t.Errorf("DecodeCMem: got %x, want %x", got, tt.want)
			}
		})
	}
}

func TestDecodeCMemLongRun(t *testing.T) {
	// A run length byte of 0xff means 256 unchanged bytes, the most one run
	// can describe.
	mem := make([]byte, 512)
	for i := range mem {
		mem[i] = byte(i)
	}

	got, err := quetzal.DecodeCMem([]byte{0x00, 0xff, 0x01}, mem)
	if err != nil {
		t.Fatalf("DecodeCMem: unexpected error: %v", err)
	}
	if got[255] != mem[255] {
		t.Errorf("byte 255: got %#02x, want %#02x (still inside the run)", got[255], mem[255])
	}
	if want := mem[256] ^ 0x01; got[256] != want {
		t.Errorf("byte 256: got %#02x, want %#02x (the run covers 256 bytes)", got[256], want)
	}
}

func TestDecodeCMemMalformed(t *testing.T) {
	tests := []struct {
		name    string
		payload []byte
		want    error
	}{
		{
			name:    "zero byte with no run length",
			payload: []byte{0x01, 0x00},
			want:    quetzal.ErrInvalidFormat,
		},
		{
			name:    "zero byte at the very start with no run length",
			payload: []byte{0x00},
			want:    quetzal.ErrInvalidFormat,
		},
		{
			name: "differences run past the end of dynamic memory",
			// Nine differences for eight bytes of memory.
			payload: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9},
			want:    quetzal.ErrInvalidFormat,
		},
		{
			name:    "a run reaches past the end of dynamic memory",
			payload: []byte{0x00, 0x08},
			want:    quetzal.ErrInvalidFormat,
		},
		{
			name:    "a run past the end after earlier differences",
			payload: []byte{0x01, 0x00, 0x07},
			want:    quetzal.ErrInvalidFormat,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := quetzal.DecodeCMem(tt.payload, original)
			if err == nil {
				t.Fatalf("DecodeCMem: got %x and no error, want %v", got, tt.want)
			}
			if !errors.Is(err, tt.want) {
				t.Errorf("DecodeCMem: got %v, want an error matching %v", err, tt.want)
			}
		})
	}
}

func TestDecodeCMemEmptyMemory(t *testing.T) {
	// A story with no dynamic memory can hold no differences, so any
	// difference at all overruns it.
	got, err := quetzal.DecodeCMem(nil, nil)
	if err != nil {
		t.Fatalf("DecodeCMem: unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("DecodeCMem: got %x, want no bytes", got)
	}

	if _, err := quetzal.DecodeCMem([]byte{0x01}, nil); !errors.Is(err, quetzal.ErrInvalidFormat) {
		t.Errorf("DecodeCMem: got %v, want ErrInvalidFormat", err)
	}
}

func TestDecodeCMemDoesNotAliasOrMutate(t *testing.T) {
	payload := []byte{0x11, 0x00, 0x01, 0x22}
	story := append([]byte(nil), original...)

	got, err := quetzal.DecodeCMem(payload, story)
	if err != nil {
		t.Fatalf("DecodeCMem: unexpected error: %v", err)
	}
	want := append([]byte(nil), got...)

	if !bytes.Equal(story, original) {
		t.Error("DecodeCMem modified the story's dynamic memory")
	}
	for i := range payload {
		payload[i] = 0xff
	}
	for i := range story {
		story[i] = 0xff
	}
	if !bytes.Equal(got, want) {
		t.Errorf("DecodeCMem aliases its arguments: got %x, want %x", got, want)
	}
}

func TestEncodeCMem(t *testing.T) {
	tests := []struct {
		name    string
		current []byte
		want    []byte
	}{
		{
			name:    "memory that never changed encodes to nothing",
			current: original,
			want:    []byte{},
		},
		{
			name:    "a change in the first byte",
			current: []byte{0x11, 0x20, 0x30, 0x40, 0x50, 0x60, 0x70, 0x80},
			want:    []byte{0x01},
		},
		{
			name: "unchanged bytes before a change become a run",
			// Three unchanged bytes, encoded as a run of n+1 where n is 2.
			current: []byte{0x10, 0x20, 0x30, 0x41, 0x50, 0x60, 0x70, 0x80},
			want:    []byte{0x00, 0x02, 0x01},
		},
		{
			name:    "unchanged bytes after the last change are dropped",
			current: []byte{0x10, 0x21, 0x30, 0x40, 0x50, 0x60, 0x70, 0x80},
			want:    []byte{0x00, 0x00, 0x01},
		},
		{
			name:    "every byte changed",
			current: []byte{0x11, 0x21, 0x31, 0x41, 0x51, 0x61, 0x71, 0x81},
			want:    []byte{0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := quetzal.EncodeCMem(tt.current, original)
			if err != nil {
				t.Fatalf("EncodeCMem: unexpected error: %v", err)
			}
			if !bytes.Equal(got, tt.want) {
				t.Fatalf("EncodeCMem: got %x, want %x", got, tt.want)
			}

			back, err := quetzal.DecodeCMem(got, original)
			if err != nil {
				t.Fatalf("DecodeCMem: unexpected error: %v", err)
			}
			if !bytes.Equal(back, tt.current) {
				t.Errorf("round trip: got %x, want %x", back, tt.current)
			}
		})
	}
}

func TestEncodeCMemSplitsLongRuns(t *testing.T) {
	// One run byte holds n for a run of n+1, so 256 bytes is the longest a
	// single run can describe and anything longer must be split.
	mem := make([]byte, 600)
	current := append([]byte(nil), mem...)
	current[599] = 0xff

	payload, err := quetzal.EncodeCMem(current, mem)
	if err != nil {
		t.Fatalf("EncodeCMem: unexpected error: %v", err)
	}

	// 599 unchanged bytes: two full runs of 256 and one of 87, then the
	// changed byte.
	want := []byte{0x00, 0xff, 0x00, 0xff, 0x00, 0x56, 0xff}
	if !bytes.Equal(payload, want) {
		t.Fatalf("EncodeCMem: got %x, want %x", payload, want)
	}

	back, err := quetzal.DecodeCMem(payload, mem)
	if err != nil {
		t.Fatalf("DecodeCMem: unexpected error: %v", err)
	}
	if !bytes.Equal(back, current) {
		t.Error("round trip did not restore memory")
	}
}

func TestEncodeCMemLengthMismatch(t *testing.T) {
	// Dynamic memory does not change size, so a difference cannot be taken
	// against memory of another length.
	for _, n := range []int{0, len(original) - 1, len(original) + 1} {
		if _, err := quetzal.EncodeCMem(make([]byte, n), original); !errors.Is(err, quetzal.ErrInvalidFormat) {
			t.Errorf("EncodeCMem(%d bytes): got %v, want ErrInvalidFormat", n, err)
		}
	}
}

func TestEncodeCMemDoesNotMutate(t *testing.T) {
	current := []byte{0x11, 0x20, 0x30, 0x40, 0x50, 0x60, 0x70, 0x81}
	wantCurrent := append([]byte(nil), current...)
	story := append([]byte(nil), original...)

	if _, err := quetzal.EncodeCMem(current, story); err != nil {
		t.Fatalf("EncodeCMem: unexpected error: %v", err)
	}
	if !bytes.Equal(current, wantCurrent) {
		t.Error("EncodeCMem modified the memory it was given")
	}
	if !bytes.Equal(story, original) {
		t.Error("EncodeCMem modified the story's dynamic memory")
	}
}

// memoryStory is a story matching the identity in ifhdPayload, with dynamic
// memory long enough to exercise runs.
func memoryStory(t *testing.T) quetzal.Story {
	t.Helper()
	story, err := quetzal.ParseStory(storyImage(3, 88, "840726", 0x1234, 0x100, 0x200))
	if err != nil {
		t.Fatalf("ParseStory: unexpected error: %v", err)
	}
	return story
}

func TestFileMemory(t *testing.T) {
	story := memoryStory(t)

	// A save in which one byte of dynamic memory changed.
	current := append([]byte(nil), story.DynamicMemory...)
	current[0x50] ^= 0x3c

	cmem, err := quetzal.EncodeCMem(current, story.DynamicMemory)
	if err != nil {
		t.Fatalf("EncodeCMem: unexpected error: %v", err)
	}

	t.Run("compressed memory", func(t *testing.T) {
		f := decodeIFZS(t, chunkBytes("IFhd", ifhdPayload), chunkBytes("CMem", cmem))

		mem, err := f.Memory(story)
		if err != nil {
			t.Fatalf("Memory: unexpected error: %v", err)
		}
		if mem.Encoding != quetzal.MemoryCompressed {
			t.Errorf("Encoding: got %s, want %s", mem.Encoding, quetzal.MemoryCompressed)
		}
		if !bytes.Equal(mem.Data, current) {
			t.Error("Data: memory was not restored")
		}
	})

	t.Run("uncompressed memory", func(t *testing.T) {
		f := decodeIFZS(t, chunkBytes("IFhd", ifhdPayload), chunkBytes("UMem", current))

		mem, err := f.Memory(story)
		if err != nil {
			t.Fatalf("Memory: unexpected error: %v", err)
		}
		if mem.Encoding != quetzal.MemoryUncompressed {
			t.Errorf("Encoding: got %s, want %s", mem.Encoding, quetzal.MemoryUncompressed)
		}
		if !bytes.Equal(mem.Data, current) {
			t.Error("Data: memory does not match the dump")
		}
	})

	t.Run("a UMem of the wrong length is rejected", func(t *testing.T) {
		for _, n := range []int{len(current) - 1, len(current) + 1} {
			dump := make([]byte, n)
			copy(dump, current)

			f := decodeIFZS(t, chunkBytes("IFhd", ifhdPayload), chunkBytes("UMem", dump))
			if _, err := f.Memory(story); !errors.Is(err, quetzal.ErrInvalidFormat) {
				t.Errorf("Memory(%d byte UMem): got %v, want ErrInvalidFormat", n, err)
			}
		}
	})

	t.Run("a missing memory chunk is reported", func(t *testing.T) {
		f := decodeIFZS(t, chunkBytes("IFhd", ifhdPayload))
		if _, err := f.Memory(story); !errors.Is(err, quetzal.ErrInvalidFormat) {
			t.Errorf("Memory: got %v, want ErrInvalidFormat", err)
		}
	})

	t.Run("both memory chunks at once is an error", func(t *testing.T) {
		f := decodeIFZS(t,
			chunkBytes("IFhd", ifhdPayload),
			chunkBytes("CMem", cmem),
			chunkBytes("UMem", current),
		)
		if _, err := f.Memory(story); !errors.Is(err, quetzal.ErrInvalidFormat) {
			t.Errorf("Memory: got %v, want ErrInvalidFormat", err)
		}
	})

	t.Run("the story is checked before memory is rebuilt", func(t *testing.T) {
		// Decoding against the wrong story would produce plausible
		// nonsense, so identity must be verified first.
		other := story
		other.Release = 89

		f := decodeIFZS(t, chunkBytes("IFhd", ifhdPayload), chunkBytes("CMem", cmem))
		if _, err := f.Memory(other); !errors.Is(err, quetzal.ErrStoryMismatch) {
			t.Errorf("Memory: got %v, want ErrStoryMismatch", err)
		}
	})

	t.Run("memory cannot be rebuilt without an IFhd", func(t *testing.T) {
		f := decodeIFZS(t, chunkBytes("CMem", cmem))
		if _, err := f.Memory(story); !errors.Is(err, quetzal.ErrInvalidFormat) {
			t.Errorf("Memory: got %v, want ErrInvalidFormat", err)
		}
	})

	t.Run("a malformed CMem is reported", func(t *testing.T) {
		f := decodeIFZS(t, chunkBytes("IFhd", ifhdPayload), chunkBytes("CMem", []byte{0x01, 0x00}))
		if _, err := f.Memory(story); !errors.Is(err, quetzal.ErrInvalidFormat) {
			t.Errorf("Memory: got %v, want ErrInvalidFormat", err)
		}
	})
}

func TestMemoryEncode(t *testing.T) {
	story := memoryStory(t)
	current := append([]byte(nil), story.DynamicMemory...)
	current[0x10] ^= 0xff
	current[0xf0] ^= 0x01

	t.Run("compressed", func(t *testing.T) {
		mem := quetzal.Memory{Encoding: quetzal.MemoryCompressed, Data: current}

		c, err := mem.Encode(story)
		if err != nil {
			t.Fatalf("Encode: unexpected error: %v", err)
		}
		if c.ID != quetzal.IDCMem {
			t.Errorf("ID: got %s, want %s", c.ID, quetzal.IDCMem)
		}
		if len(c.Data) >= len(current) {
			t.Errorf("payload is %d byte(s) for %d byte(s) of memory: nothing was compressed",
				len(c.Data), len(current))
		}

		back, err := quetzal.DecodeCMem(c.Data, story.DynamicMemory)
		if err != nil {
			t.Fatalf("DecodeCMem: unexpected error: %v", err)
		}
		if !bytes.Equal(back, current) {
			t.Error("round trip did not restore memory")
		}
	})

	t.Run("uncompressed", func(t *testing.T) {
		mem := quetzal.Memory{Encoding: quetzal.MemoryUncompressed, Data: current}

		c, err := mem.Encode(story)
		if err != nil {
			t.Fatalf("Encode: unexpected error: %v", err)
		}
		if c.ID != quetzal.IDUMem {
			t.Errorf("ID: got %s, want %s", c.ID, quetzal.IDUMem)
		}
		if !bytes.Equal(c.Data, current) {
			t.Error("payload is not a plain dump of dynamic memory")
		}

		// The payload must own its bytes.
		c.Data[0] ^= 0xff
		if bytes.Equal(c.Data, current) {
			t.Error("the chunk payload aliases the memory it was built from")
		}
	})

	t.Run("an unknown encoding is refused", func(t *testing.T) {
		mem := quetzal.Memory{Data: current}
		if _, err := mem.Encode(story); !errors.Is(err, quetzal.ErrInvalidFormat) {
			t.Errorf("Encode: got %v, want ErrInvalidFormat", err)
		}
	})

	t.Run("memory of the wrong length is refused", func(t *testing.T) {
		for _, enc := range []quetzal.MemoryEncoding{quetzal.MemoryCompressed, quetzal.MemoryUncompressed} {
			mem := quetzal.Memory{Encoding: enc, Data: current[:len(current)-1]}
			if _, err := mem.Encode(story); !errors.Is(err, quetzal.ErrInvalidFormat) {
				t.Errorf("Encode(%s): got %v, want ErrInvalidFormat", enc, err)
			}
		}
	})
}

func TestMemoryValidate(t *testing.T) {
	story := memoryStory(t)
	data := append([]byte(nil), story.DynamicMemory...)

	t.Run("either encoding is acceptable", func(t *testing.T) {
		for _, enc := range []quetzal.MemoryEncoding{quetzal.MemoryCompressed, quetzal.MemoryUncompressed} {
			mem := quetzal.Memory{Encoding: enc, Data: data}
			if err := mem.Validate(story); err != nil {
				t.Errorf("Validate(%s): unexpected error: %v", enc, err)
			}
		}
	})

	t.Run("an unknown encoding is refused", func(t *testing.T) {
		mem := quetzal.Memory{Data: data}
		if err := mem.Validate(story); !errors.Is(err, quetzal.ErrInvalidFormat) {
			t.Errorf("Validate: got %v, want ErrInvalidFormat", err)
		}
	})

	t.Run("memory of the wrong length is refused", func(t *testing.T) {
		// Dynamic memory keeps its size for as long as a game runs.
		mem := quetzal.Memory{Encoding: quetzal.MemoryCompressed, Data: data[:len(data)-1]}
		if err := mem.Validate(story); !errors.Is(err, quetzal.ErrInvalidFormat) {
			t.Errorf("Validate: got %v, want ErrInvalidFormat", err)
		}
	})
}

func TestMemoryEncodingString(t *testing.T) {
	tests := []struct {
		enc  quetzal.MemoryEncoding
		want string
	}{
		{quetzal.MemoryCompressed, "CMem"},
		{quetzal.MemoryUncompressed, "UMem"},
		{0, "MemoryEncoding(0)"},
		{99, "MemoryEncoding(99)"},
	}
	for _, tt := range tests {
		if got := tt.enc.String(); got != tt.want {
			t.Errorf("String: got %s, want %s", got, tt.want)
		}
	}
}

// TestMemoryRoundTripRealStory exercises compression against the dynamic
// memory of real story files rather than synthetic patterns.
func TestMemoryRoundTripRealStory(t *testing.T) {
	for _, name := range storyFixtures(t) {
		t.Run(name, func(t *testing.T) {
			story := loadStory(t, name)

			// Change dynamic memory the way play would: scattered bytes,
			// most of memory untouched. The pattern is fixed so a failure
			// can be reproduced.
			current := append([]byte(nil), story.DynamicMemory...)
			seed := uint32(1)
			for range 200 {
				seed = seed*1664525 + 1013904223
				current[int(seed>>8)%len(current)] ^= byte(seed)
			}

			payload, err := quetzal.EncodeCMem(current, story.DynamicMemory)
			if err != nil {
				t.Fatalf("EncodeCMem: unexpected error: %v", err)
			}
			// A save that changed 200 bytes of 11 KB should be far smaller
			// than a dump of the whole of dynamic memory.
			if len(payload) > len(current)/4 {
				t.Errorf("CMem payload is %d byte(s) for %d byte(s) of dynamic memory",
					len(payload), len(current))
			}

			back, err := quetzal.DecodeCMem(payload, story.DynamicMemory)
			if err != nil {
				t.Fatalf("DecodeCMem: unexpected error: %v", err)
			}
			if !bytes.Equal(back, current) {
				t.Error("round trip did not restore dynamic memory")
			}

			// Memory that changed everywhere still round-trips, even though
			// it compresses to more than it started with.
			for i := range current {
				current[i] ^= 0xa5
			}
			payload, err = quetzal.EncodeCMem(current, story.DynamicMemory)
			if err != nil {
				t.Fatalf("EncodeCMem: unexpected error: %v", err)
			}
			back, err = quetzal.DecodeCMem(payload, story.DynamicMemory)
			if err != nil {
				t.Fatalf("DecodeCMem: unexpected error: %v", err)
			}
			if !bytes.Equal(back, current) {
				t.Error("round trip did not restore fully changed dynamic memory")
			}
		})
	}
}

func FuzzCMem(f *testing.F) {
	f.Add([]byte{}, uint16(8))
	f.Add([]byte{0x01, 0x02, 0x03}, uint16(8))
	f.Add([]byte{0x00, 0xff, 0x01}, uint16(512))
	f.Add([]byte{0x00}, uint16(8))
	f.Add([]byte{0x00, 0x00, 0x00, 0x00}, uint16(2))
	f.Add([]byte{0xff, 0xff, 0xff, 0xff}, uint16(0))

	f.Fuzz(func(t *testing.T, payload []byte, size uint16) {
		story := make([]byte, size%4096)
		for i := range story {
			story[i] = byte(i * 7)
		}
		keep := append([]byte(nil), story...)

		restored, err := quetzal.DecodeCMem(payload, story)
		if err != nil {
			return
		}
		if !bytes.Equal(story, keep) {
			t.Fatal("DecodeCMem modified the story's dynamic memory")
		}
		if len(restored) != len(story) {
			t.Fatalf("restored %d byte(s) of dynamic memory, want %d", len(restored), len(story))
		}

		// Whatever a valid difference stream decodes to must survive being
		// compressed and decoded again.
		again, err := quetzal.EncodeCMem(restored, story)
		if err != nil {
			t.Fatalf("EncodeCMem: unexpected error: %v", err)
		}
		back, err := quetzal.DecodeCMem(again, story)
		if err != nil {
			t.Fatalf("DecodeCMem: re-encoded memory did not decode: %v", err)
		}
		if !bytes.Equal(back, restored) {
			t.Error("round trip did not restore memory")
		}
	})
}
