// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package quetzal

import (
	"fmt"
)

// maxZeroRun is the longest run of unchanged bytes a single CMem run can
// describe. The run length is stored in one byte holding n, and the pair
// represents n+1 bytes, so a run of 256 is the most that fits. Longer runs are
// written as several adjacent runs.
const maxZeroRun = 256

// MemoryEncoding identifies the way a save stores dynamic memory. Quetzal
// defines two, and a reader must understand both.
type MemoryEncoding uint8

const (
	// MemoryCompressed is the CMem encoding: dynamic memory exclusive-ored
	// against the story's original dynamic memory, with runs of unchanged
	// bytes collapsed. Reconstructing it requires the original story.
	MemoryCompressed MemoryEncoding = iota + 1

	// MemoryUncompressed is the UMem encoding: dynamic memory dumped
	// unchanged. It is larger but needs no story to read.
	MemoryUncompressed
)

// String names the encoding by the chunk that carries it.
func (e MemoryEncoding) String() string {
	switch e {
	case MemoryCompressed:
		return IDCMem.String()
	case MemoryUncompressed:
		return IDUMem.String()
	default:
		return fmt.Sprintf("MemoryEncoding(%d)", uint8(e))
	}
}

// Memory is a save's dynamic memory.
//
// Data is the dynamic memory itself, however the save happened to store it: a
// CMem payload is expanded against the story before it reaches this type. Its
// length always equals the length of the story's dynamic memory.
//
// Encoding records how the save held that memory. Reading it describes the
// file that was read; setting it chooses how the memory will be written.
type Memory struct {
	Encoding MemoryEncoding
	Data     []byte
}

// Validate reports whether the memory can be written for the given story.
func (m Memory) Validate(story Story) error {
	switch m.Encoding {
	case MemoryCompressed, MemoryUncompressed:
	default:
		return unknownEncoding(m.Encoding)
	}
	return checkMemoryLength("memory", len(m.Data), len(story.DynamicMemory))
}

// Encode returns the chunk that stores this dynamic memory, in the encoding
// the Memory names. The story supplies the original dynamic memory that
// MemoryCompressed is a difference against.
//
// The returned chunk owns its payload and neither aliases nor modifies the
// memory or the story.
func (m Memory) Encode(story Story) (Chunk, error) {
	switch m.Encoding {
	case MemoryCompressed:
		payload, err := EncodeCMem(m.Data, story.DynamicMemory)
		if err != nil {
			return Chunk{}, err
		}
		return Chunk{ID: IDCMem, Data: payload}, nil
	case MemoryUncompressed:
		if err := checkMemoryLength(IDUMem.String(), len(m.Data), len(story.DynamicMemory)); err != nil {
			return Chunk{}, err
		}
		return Chunk{ID: IDUMem, Data: append([]byte(nil), m.Data...)}, nil
	default:
		return Chunk{}, unknownEncoding(m.Encoding)
	}
}

// Memory reconstructs the save's dynamic memory from its CMem or UMem chunk.
//
// Because compressed memory is a difference against the story it came from,
// decoding it against the wrong story would yield plausible nonsense rather
// than an error. Memory therefore verifies the save's IFhd against the story
// first, and reports ErrStoryMismatch if they disagree.
//
// A file holding both a CMem and a UMem chunk is rejected. The two are
// competing statements of the same state, and Quetzal gives no rule for
// choosing between them.
func (f *File) Memory(story Story) (Memory, error) {
	compressed, hasCompressed := f.First(IDCMem)
	uncompressed, hasUncompressed := f.First(IDUMem)

	switch {
	case hasCompressed && hasUncompressed:
		return Memory{}, prefixed(newErr(ErrInvalidFormat,
			"save holds both %s and %s chunks, but dynamic memory is stored one way or the other",
			IDCMem, IDUMem))
	case !hasCompressed && !hasUncompressed:
		return Memory{}, prefixed(newErr(ErrInvalidFormat,
			"missing dynamic memory: no %s or %s chunk", IDCMem, IDUMem))
	}

	header, err := f.Header()
	if err != nil {
		return Memory{}, err
	}
	if err := header.Verify(story); err != nil {
		return Memory{}, err
	}

	if hasCompressed {
		data, err := DecodeCMem(compressed.Data, story.DynamicMemory)
		if err != nil {
			return Memory{}, err
		}
		return Memory{Encoding: MemoryCompressed, Data: data}, nil
	}

	data, err := decodeUMem(uncompressed.Data, story.DynamicMemory)
	if err != nil {
		return Memory{}, err
	}
	return Memory{Encoding: MemoryUncompressed, Data: data}, nil
}

// DecodeCMem expands the payload of a CMem chunk into dynamic memory, given
// the original dynamic memory of the story the save was made from.
//
// The payload is a difference stream: a non-zero byte is exclusive-ored with
// the byte at the current position, and a zero byte followed by a length byte
// n leaves the next n+1 bytes unchanged. A stream that ends early leaves the
// rest of dynamic memory unchanged, which is how writers drop a redundant run
// at the end.
//
// The result is a new buffer the length of original. Neither argument is
// retained or modified.
func DecodeCMem(payload, original []byte) ([]byte, error) {
	restored := make([]byte, len(original))
	copy(restored, original)

	// out is how far into dynamic memory the difference stream has reached.
	out := 0
	for i := 0; i < len(payload); i++ {
		if d := payload[i]; d != 0 {
			if out >= len(restored) {
				return nil, cmemOverrun(i, out+1, len(restored))
			}
			restored[out] ^= d
			out++
			continue
		}

		// A zero byte introduces a run of unchanged bytes and must be
		// followed by the run's length.
		i++
		if i == len(payload) {
			return nil, prefixed(newErr(ErrInvalidFormat,
				"CMem: zero byte at offset %d is not followed by a run length", i-1))
		}
		run := int(payload[i]) + 1
		if run > len(restored)-out {
			return nil, cmemOverrun(i-1, out+run, len(restored))
		}
		out += run
	}
	return restored, nil
}

// EncodeCMem compresses dynamic memory against the original dynamic memory of
// the story it came from, producing the payload of a CMem chunk.
//
// Any run of unchanged bytes at the end is omitted, since a reader treats a
// stream that ends early as unchanged to the end. The result is not
// necessarily the shortest possible encoding, which the standard does not
// require, but it is the shortest this scheme allows for a single pass.
//
// Neither argument is retained or modified.
func EncodeCMem(current, original []byte) ([]byte, error) {
	if err := checkMemoryLength(IDCMem.String(), len(current), len(original)); err != nil {
		return nil, err
	}

	// Trailing unchanged bytes need not be written at all.
	end := len(current)
	for end > 0 && current[end-1] == original[end-1] {
		end--
	}

	payload := make([]byte, 0, end)
	for i := 0; i < end; {
		if d := current[i] ^ original[i]; d != 0 {
			payload = append(payload, d)
			i++
			continue
		}
		run := 0
		for i < end && current[i] == original[i] && run < maxZeroRun {
			run++
			i++
		}
		payload = append(payload, 0, byte(run-1))
	}
	return payload, nil
}

// decodeUMem copies the payload of a UMem chunk, which is dynamic memory
// dumped unchanged and must therefore be exactly as long as the story's.
func decodeUMem(payload, original []byte) ([]byte, error) {
	if err := checkMemoryLength(IDUMem.String(), len(payload), len(original)); err != nil {
		return nil, err
	}
	return append([]byte(nil), payload...), nil
}

// checkMemoryLength reports whether memory of length have can stand for the
// story's dynamic memory of length want. Dynamic memory does not change size
// while a game runs, so the two must agree exactly.
func checkMemoryLength(what string, have, want int) error {
	if have != want {
		return prefixed(newErr(ErrInvalidFormat,
			"%s: %d byte(s) of dynamic memory, but the story has %d", what, have, want))
	}
	return nil
}

// unknownEncoding reports a MemoryEncoding this package cannot write.
func unknownEncoding(e MemoryEncoding) error {
	return prefixed(newErr(ErrInvalidFormat, "memory: unknown encoding %s", e))
}

// cmemOverrun reports a difference stream that describes more dynamic memory
// than the story has.
func cmemOverrun(offset, reached, size int) error {
	return prefixed(newErr(ErrInvalidFormat,
		"CMem: difference at offset %d reaches byte %d of %d byte(s) of dynamic memory",
		offset, reached, size))
}
