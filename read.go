// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package quetzal

import (
	"encoding/binary"
	"errors"
	"io"
)

const (
	// chunkHeaderSize is the size of an IFF chunk's ID and length fields.
	chunkHeaderSize = 8

	// formTypeSize is the size of the form type that opens a FORM's contents.
	formTypeSize = 4
)

// File is the raw IFF container of a Quetzal save: every chunk, in the order
// it appeared, with payloads exactly as stored.
//
// A File is deliberately uninterpreted. It reports what the container holds,
// including chunks this package assigns no meaning to, and makes no claim that
// the save is complete or restorable. Use Read to reconstruct saved state.
type File struct {
	// Chunks holds the FORM's contents in file order.
	Chunks []Chunk

	// limits records what Decode was configured with, so that later
	// operations that allocate from a payload honor the same bounds. A File
	// built by hand leaves this zero and so gets the defaults.
	limits Limits

	// ignoreChunkOrder records whether Decode was told to overlook the
	// rule that IFhd comes first, so that Save applies the same leniency.
	// A File built by hand is strict, which is the safe default.
	ignoreChunkOrder bool
}

// First returns the first chunk with the given identifier.
//
// Where Quetzal expects a single instance of a chunk, the first is
// authoritative and later instances are ignored.
func (f *File) First(id ID) (Chunk, bool) {
	for _, c := range f.Chunks {
		if c.ID == id {
			return c, true
		}
	}
	return Chunk{}, false
}

// All returns every chunk with the given identifier, in file order. Repeated
// chunks are legal in IFF; ANNO in particular may appear more than once.
func (f *File) All(id ID) []Chunk {
	var found []Chunk
	for _, c := range f.Chunks {
		if c.ID == id {
			found = append(found, c)
		}
	}
	return found
}

// Save is a Quetzal saved game with its state reconstructed: the story it
// belongs to, the dynamic memory it recorded, and the call stack it was
// suspended on.
//
// A Save is what an interpreter needs in order to restore. It is the
// interpreted form of a File, and unlike a File it makes the claim that the
// save is complete and consistent with the story it names.
type Save struct {
	// Header is the story identification and saved program counter from the
	// IFhd chunk.
	Header Header

	// Memory is the dynamic memory the save recorded, already expanded, and
	// the encoding it was stored in.
	Memory Memory

	// Frames is the call stack, oldest frame first. On every Z-machine
	// version except 6 the first frame is the dummy frame that holds
	// top-level evaluation-stack state.
	Frames []Frame

	// Chunks holds the file's remaining chunks in their original relative
	// order: annotations, author and copyright text, interpreter data, and
	// chunks this package assigns no meaning to. Writing a Save writes them
	// after the three chunks the fields above describe.
	//
	// It never holds an IFhd, CMem, UMem, or Stks chunk. Those are
	// represented by the fields above, and a second copy of one would
	// contradict them.
	Chunks []Chunk
}

// Read reads a Quetzal save and reconstructs the state it holds.
//
// The story is required. Dynamic memory is usually stored as a difference
// against the story it came from and cannot be rebuilt without it, and the
// story's version decides what the call stack must contain. Read verifies that
// the save belongs to the story before rebuilding anything, and reports
// ErrStoryMismatch if it does not.
//
// Use Decode instead to examine a save's structure without a story.
//
// The returned Save owns its data. Neither the reader's bytes nor the story
// are retained or modified.
func Read(r io.Reader, story Story, opts ...ReadOption) (*Save, error) {
	f, err := Decode(r, opts...)
	if err != nil {
		return nil, err
	}
	return f.Save(story)
}

// Save reconstructs the saved state held in an already-decoded file, which is
// what Read does after decoding.
//
// The file must hold the chunks Quetzal requires, its IFhd must come before
// its memory and stack chunks, and the resulting save must be valid for the
// given story.
func (f *File) Save(story Story) (*Save, error) {
	header, err := f.Header()
	if err != nil {
		return nil, err
	}
	// Only now is it known that an IFhd exists, which is what makes a
	// memory or stack chunk found before one an ordering error rather than
	// a missing-chunk error.
	if !f.ignoreChunkOrder {
		if err := f.checkOrder(); err != nil {
			return nil, err
		}
	}

	// Memory verifies the story identity before expanding anything.
	memory, err := f.Memory(story)
	if err != nil {
		return nil, err
	}
	frames, err := f.Frames()
	if err != nil {
		return nil, err
	}

	save := &Save{Header: header, Memory: memory, Frames: frames}
	for _, c := range f.Chunks {
		switch c.ID {
		case IDIFhd, IDCMem, IDUMem, IDStks:
			// Represented by the fields above, including any duplicate
			// that the first-instance rule made irrelevant.
			continue
		case IDIntD:
			// Interpreter data may carry restrictions on being copied
			// into another file, and a payload too short to state its
			// restrictions cannot be shown to be free of them.
			if d, err := ParseInterpreterData(c.Data); err != nil || !d.Copyable() {
				continue
			}
		}
		save.Chunks = append(save.Chunks, Chunk{ID: c.ID, Data: append([]byte(nil), c.Data...)})
	}

	if err := save.Validate(story); err != nil {
		return nil, err
	}
	return save, nil
}

// checkOrder reports a memory or stack chunk that appears before the IFhd
// chunk. The format requires that order so that an interpreter finds out it
// has the wrong story before it decodes anything against it.
//
// The caller must already have established that an IFhd is present, since
// otherwise this reports the ordering problem that a missing chunk implies
// rather than the missing chunk itself.
func (f *File) checkOrder() error {
	for _, c := range f.Chunks {
		if c.ID == IDIFhd {
			break
		}
		switch c.ID {
		case IDCMem, IDUMem, IDStks:
			return prefixed(newErr(ErrInvalidFormat,
				"%s chunk appears before the %s chunk, which must come first so that a save for the wrong story is recognized before its memory is decoded",
				c.ID, IDIFhd))
		}
	}
	return nil
}

// ReadOption configures a decode.
type ReadOption func(*readConfig)

// readConfig holds the settings a ReadOption may adjust.
type readConfig struct {
	limits Limits

	// ignoreChunkOrder relaxes the rule that IFhd comes first.
	ignoreChunkOrder bool
}

// WithLimits sets the resource limits for a decode. Zero-valued fields keep
// their defaults, so a caller may override only the limits it cares about.
func WithLimits(l Limits) ReadOption {
	return func(c *readConfig) { c.limits = l.resolve() }
}

// IgnoreChunkOrder accepts a save whose IFhd chunk does not come before its
// memory and stack chunks.
//
// The format requires that order, so that an interpreter learns it has the
// wrong story before decoding anything against it, and this package enforces it
// by default. Frotz does not: it restores a save whose IFhd comes last without
// complaint. A file written by some interpreter that gets the order wrong would
// therefore work elsewhere and fail here, and this option is the way to accept
// it anyway.
//
// Nothing else is relaxed. The identity check still happens before memory is
// rebuilt — it is the ordering of the chunks in the file that is overlooked,
// not the verification they exist for.
func IgnoreChunkOrder() ReadOption {
	return func(c *readConfig) { c.ignoreChunkOrder = true }
}

// Decode parses the IFF container of a Quetzal save without reconstructing
// saved state, and therefore without needing the story image.
//
// Decode verifies that the input is a FORM of type IFZS, that every chunk lies
// within the FORM, and that odd-length chunks carry their pad byte. It does
// not check that the chunks required by Quetzal are present, nor interpret any
// payload; that is Read's work. Unknown chunks are retained rather than
// treated as errors.
//
// Decode stops at the end of the FORM. Any bytes following it are neither
// consumed nor examined, since a simple IFF file is a single FORM chunk.
//
// The returned File owns its payloads; they are not aliases of any buffer
// supplied by the caller.
func Decode(r io.Reader, opts ...ReadOption) (*File, error) {
	if r == nil {
		return nil, prefixed(newErr(ErrInvalidFormat, "nil reader"))
	}
	cfg := readConfig{limits: DefaultLimits()}
	for _, opt := range opts {
		opt(&cfg)
	}
	d := &decoder{
		r:                &countingReader{r: r},
		limits:           cfg.limits,
		ignoreChunkOrder: cfg.ignoreChunkOrder,
	}
	return d.form()
}

// decoder holds the state of one decode.
type decoder struct {
	r      *countingReader
	limits Limits

	// ignoreChunkOrder is carried into the File so that Save honors it.
	ignoreChunkOrder bool
}

// form parses the outer FORM chunk and its contents.
func (d *decoder) form() (*File, error) {
	var hdr [chunkHeaderSize + formTypeSize]byte
	if err := d.readFull(hdr[:], "FORM header"); err != nil {
		return nil, prefixed(err)
	}

	if id := ID(hdr[0:4]); id != IDFORM {
		return nil, prefixed(newErr(ErrInvalidFormat, "expected outer chunk %s, found %s", IDFORM, id))
	}

	// The FORM's length covers its form type and every chunk that follows,
	// including pad bytes, but not the eight bytes of its own header.
	formLen := uint64(binary.BigEndian.Uint32(hdr[4:8]))
	if formLen > d.limits.MaxFormBytes {
		return nil, prefixed(newErr(ErrLimitExceeded, "FORM length %d exceeds limit %d", formLen, d.limits.MaxFormBytes))
	}
	if formLen < formTypeSize {
		return nil, prefixed(newErr(ErrInvalidFormat, "FORM length %d is too small to hold a form type", formLen))
	}

	if ft := ID(hdr[8:12]); ft != IDIFZS {
		return nil, prefixed(newErr(ErrInvalidFormat, "expected FORM type %s, found %s", IDIFZS, ft))
	}

	file := &File{limits: d.limits, ignoreChunkOrder: d.ignoreChunkOrder}
	for remaining := formLen - formTypeSize; remaining > 0; {
		c, n, err := d.chunk(remaining)
		if err != nil {
			return nil, err
		}
		file.Chunks = append(file.Chunks, c)
		remaining -= n
	}
	return file, nil
}

// chunk parses one chunk, which must fit within the remaining bytes of the
// FORM. It returns the chunk and the number of FORM bytes it consumed.
//
// All arithmetic here is done in uint64 over values widened from uint32, so
// the bounds checks cannot themselves overflow.
func (d *decoder) chunk(remaining uint64) (Chunk, uint64, error) {
	offset := d.r.count

	if remaining < chunkHeaderSize {
		return Chunk{}, 0, prefixed(newErr(ErrInvalidFormat,
			"%d byte(s) left in FORM at offset %d, too few for a chunk header", remaining, offset))
	}

	var hdr [chunkHeaderSize]byte
	if err := d.readFull(hdr[:], "chunk header"); err != nil {
		return Chunk{}, 0, prefixed(err)
	}

	id := ID(hdr[0:4])
	if !id.valid() {
		return Chunk{}, 0, &ChunkError{ID: id, Offset: offset,
			Err: newErr(ErrInvalidFormat, "identifier is not four printable ASCII characters")}
	}

	size := uint64(binary.BigEndian.Uint32(hdr[4:8]))
	if size > d.limits.MaxChunkBytes {
		return Chunk{}, 0, &ChunkError{ID: id, Offset: offset,
			Err: newErr(ErrLimitExceeded, "length %d exceeds limit %d", size, d.limits.MaxChunkBytes)}
	}

	// An odd-length chunk is followed by a pad byte that the length excludes.
	padded := size + size&1
	if padded > remaining-chunkHeaderSize {
		return Chunk{}, 0, &ChunkError{ID: id, Offset: offset,
			Err: newErr(ErrInvalidFormat, "length %d overruns the end of the FORM by %d byte(s)",
				size, padded-(remaining-chunkHeaderSize))}
	}

	// The length is now known to fit inside the FORM, which the caller has
	// already bounded, so this allocation is safe.
	data := make([]byte, size)
	if err := d.readFull(data, "chunk data"); err != nil {
		return Chunk{}, 0, &ChunkError{ID: id, Offset: offset, Err: err}
	}

	if size != padded {
		// The pad byte is required. Its value should be zero, but a
		// non-zero byte carries no information and the chunk lengths keep
		// the stream aligned regardless, so it is accepted and discarded.
		var pad [1]byte
		if err := d.readFull(pad[:], "pad byte"); err != nil {
			return Chunk{}, 0, &ChunkError{ID: id, Offset: offset, Err: err}
		}
	}

	return Chunk{ID: id, Data: data}, chunkHeaderSize + padded, nil
}

// readFull fills buf, reporting a short read as truncated input.
func (d *decoder) readFull(buf []byte, what string) error {
	if _, err := io.ReadFull(d.r, buf); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return newErr(ErrTruncated, "input ended while reading %s at offset %d", what, d.r.count)
		}
		return err
	}
	return nil
}

// countingReader tracks how many bytes have been read, so that errors can
// report the offset at which a problem was found.
type countingReader struct {
	r     io.Reader
	count int64
}

// Read implements io.Reader.
func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.count += int64(n)
	return n, err
}
