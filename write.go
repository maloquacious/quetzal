// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package quetzal

import (
	"encoding/binary"
	"io"
	"math"
)

// WriteOption configures a write.
type WriteOption func(*writeConfig)

// writeConfig holds the settings a WriteOption may adjust.
type writeConfig struct {
	// encoding overrides the encoding a Save names for its memory. Zero
	// means the Save decides.
	encoding MemoryEncoding
}

// WithEncoding chooses how dynamic memory is stored, overriding the encoding
// the Save carries. It is how a caller converts between the two encodings
// without disturbing the save itself.
//
// Compressed memory is much smaller and is what interpreters normally write.
// Uncompressed memory can be read back without the story.
func WithEncoding(e MemoryEncoding) WriteOption {
	return func(c *writeConfig) { c.encoding = e }
}

// Validate reports whether the save is complete, representable in Quetzal, and
// consistent with the given story.
//
// It checks story identity, the size and encoding of dynamic memory, the
// representability of every program counter, the local and evaluation-stack
// counts and argument mask of every frame, the dummy frame that versions other
// than 6 require, and the additional chunks.
//
// Validation is available on its own so that a caller can find out whether a
// save it has assembled is sound without producing a file. Write performs the
// same checks.
func (s *Save) Validate(story Story) error {
	if err := s.Header.Validate(); err != nil {
		return err
	}
	if err := s.Header.Verify(story); err != nil {
		return err
	}
	if err := s.Memory.Validate(story); err != nil {
		return err
	}
	if err := ValidateFrames(s.Frames, story); err != nil {
		return err
	}
	return checkExtraChunks(s.Chunks)
}

// Encode builds the IFF container that represents the save, in the order
// Quetzal requires: IFhd, then dynamic memory, then Stks, then whatever
// additional chunks the save carries, in the order it carries them.
//
// The story supplies the original dynamic memory that compressed memory is a
// difference against, and the version that decides what the call stack must
// contain.
//
// The returned File owns its payloads. Neither the save nor the story is
// retained or modified.
func (s *Save) Encode(story Story, opts ...WriteOption) (*File, error) {
	cfg := writeConfig{}
	for _, opt := range opts {
		opt(&cfg)
	}

	// Everything the encoders below cannot judge for themselves. They
	// report an unrepresentable program counter, a memory of the wrong
	// length or an unknown encoding, and a malformed frame; what is left is
	// whether the save belongs to this story, whether its call stack suits
	// the story's version, and whether its additional chunks may be written.
	if err := s.Header.Verify(story); err != nil {
		return nil, err
	}
	if err := checkDummyFrame(s.Frames, story); err != nil {
		return nil, err
	}
	if err := checkExtraChunks(s.Chunks); err != nil {
		return nil, err
	}

	header, err := s.Header.Encode()
	if err != nil {
		return nil, err
	}

	memory := s.Memory
	if cfg.encoding != 0 {
		memory.Encoding = cfg.encoding
	}
	memoryChunk, err := memory.Encode(story)
	if err != nil {
		return nil, err
	}

	stacks, err := EncodeStks(s.Frames)
	if err != nil {
		return nil, err
	}

	file := &File{Chunks: make([]Chunk, 0, 3+len(s.Chunks))}
	file.Chunks = append(file.Chunks,
		Chunk{ID: IDIFhd, Data: header},
		memoryChunk,
		Chunk{ID: IDStks, Data: stacks},
	)
	for _, c := range s.Chunks {
		file.Chunks = append(file.Chunks, Chunk{ID: c.ID, Data: append([]byte(nil), c.Data...)})
	}
	return file, nil
}

// Write writes a save as a Quetzal file.
//
// The story is required: it supplies the original dynamic memory that
// compressed memory is a difference against, and the Z-machine version that
// decides what the call stack must contain. Write refuses to write a save that
// does not belong to the story given, since the identity it would record would
// then be a lie about its own contents.
//
// Nothing is written until the whole save has been checked and encoded, so a
// rejected save leaves the writer untouched. The writer need not be seekable.
//
// Neither the save nor the story is retained or modified.
func Write(w io.Writer, story Story, save *Save, opts ...WriteOption) error {
	if w == nil {
		return prefixed(newErr(ErrInvalidFormat, "nil writer"))
	}
	if save == nil {
		return prefixed(newErr(ErrInvalidFormat, "nil save"))
	}

	file, err := save.Encode(story, opts...)
	if err != nil {
		return err
	}
	_, err = file.WriteTo(w)
	return err
}

// WriteTo writes the file as a FORM of type IFZS, computing every length and
// supplying the pad byte that follows an odd-length chunk. It implements
// io.WriterTo and returns the number of bytes written.
//
// The chunks are written in the order the File holds them. WriteTo makes no
// claim that they are the chunks Quetzal requires, or that they are in the
// order it requires; it writes the container it is given. Save.Encode is what
// puts a save's chunks in a conforming order.
func (f *File) WriteTo(w io.Writer) (int64, error) {
	if w == nil {
		return 0, prefixed(newErr(ErrInvalidFormat, "nil writer"))
	}

	// The FORM's length covers its type and every chunk that follows,
	// including pad bytes, and is itself a four-byte field. Summing first
	// means the length is known before anything is written, so the file
	// needs no seeking and no buffering of payloads.
	total := uint64(formTypeSize)
	for _, c := range f.Chunks {
		if !c.ID.valid() {
			return 0, prefixed(newErr(ErrInvalidFormat,
				"chunk identifier %s is not four printable ASCII characters", c.ID))
		}
		size := uint64(len(c.Data))
		total += chunkHeaderSize + size + size&1
		if total > math.MaxUint32 {
			return 0, prefixed(newErr(ErrInvalidFormat,
				"the FORM is longer than the %d bytes its length field can describe",
				uint64(math.MaxUint32)))
		}
	}

	cw := &countingWriter{w: w}
	var header [chunkHeaderSize + formTypeSize]byte
	copy(header[0:4], IDFORM[:])
	binary.BigEndian.PutUint32(header[4:8], uint32(total))
	copy(header[8:12], IDIFZS[:])
	if _, err := cw.Write(header[:]); err != nil {
		return cw.count, err
	}

	var chunkHeader [chunkHeaderSize]byte
	for _, c := range f.Chunks {
		copy(chunkHeader[0:4], c.ID[:])
		binary.BigEndian.PutUint32(chunkHeader[4:8], uint32(len(c.Data)))
		if _, err := cw.Write(chunkHeader[:]); err != nil {
			return cw.count, err
		}
		if _, err := cw.Write(c.Data); err != nil {
			return cw.count, err
		}
		if len(c.Data)%2 == 1 {
			// The pad byte keeps the next chunk on an even boundary. It
			// belongs to the container, not to the chunk, and so is
			// excluded from the length just written.
			if _, err := cw.Write([]byte{0}); err != nil {
				return cw.count, err
			}
		}
	}
	return cw.count, nil
}

// checkExtraChunks reports whether the additional chunks of a save can be
// written alongside the chunks its other fields describe.
func checkExtraChunks(chunks []Chunk) error {
	for i, c := range chunks {
		if !c.ID.valid() {
			return prefixed(newErr(ErrInvalidFormat,
				"chunk %d: identifier %s is not four printable ASCII characters", i, c.ID))
		}
		switch c.ID {
		case IDIFhd, IDCMem, IDUMem, IDStks:
			return prefixed(newErr(ErrInvalidFormat,
				"chunk %d: a %s chunk cannot be carried as an additional chunk, since the save already describes one",
				i, c.ID))
		}
	}
	return nil
}

// countingWriter tracks how many bytes have been written, so that WriteTo can
// report a total even when the write fails partway through.
type countingWriter struct {
	w     io.Writer
	count int64
}

// Write implements io.Writer. A writer that reports a short write without an
// error is treated as having failed, since accepting it would silently produce
// a file whose lengths no longer describe its contents.
func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.count += int64(n)
	if err == nil && n < len(p) {
		err = io.ErrShortWrite
	}
	return n, err
}
