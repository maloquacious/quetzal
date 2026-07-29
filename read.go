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

// ReadOption configures a decode.
type ReadOption func(*readConfig)

// readConfig holds the settings a ReadOption may adjust.
type readConfig struct {
	limits Limits
}

// WithLimits sets the resource limits for a decode. Zero-valued fields keep
// their defaults, so a caller may override only the limits it cares about.
func WithLimits(l Limits) ReadOption {
	return func(c *readConfig) { c.limits = l.resolve() }
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
	d := &decoder{r: &countingReader{r: r}, limits: cfg.limits}
	return d.form()
}

// decoder holds the state of one decode.
type decoder struct {
	r      *countingReader
	limits Limits
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

	file := &File{limits: d.limits}
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
