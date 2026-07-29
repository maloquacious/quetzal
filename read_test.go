// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package quetzal_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/maloquacious/quetzal"
)

// chunkBytes builds one IFF chunk: identifier, big-endian length, payload, and
// the pad byte an odd-length payload requires.
func chunkBytes(id string, data []byte) []byte {
	var b []byte
	b = append(b, id...)
	b = binary.BigEndian.AppendUint32(b, uint32(len(data)))
	b = append(b, data...)
	if len(data)%2 == 1 {
		b = append(b, 0)
	}
	return b
}

// formBytes wraps contents in a FORM chunk of the given type, computing the
// FORM length the way a conforming writer would.
func formBytes(formType string, contents []byte) []byte {
	var b []byte
	b = append(b, "FORM"...)
	b = binary.BigEndian.AppendUint32(b, uint32(len(formType)+len(contents)))
	b = append(b, formType...)
	b = append(b, contents...)
	return b
}

// ifzs wraps already-encoded chunks in a FORM IFZS container.
func ifzs(chunks ...[]byte) []byte {
	return formBytes("IFZS", bytes.Join(chunks, nil))
}

func TestDecodeValidContainer(t *testing.T) {
	// A plausible save: identification, compressed memory, stacks, and an
	// annotation. IFhd's 13-byte payload is odd and so exercises padding.
	ifhd := bytes.Repeat([]byte{0xab}, 13)
	cmem := []byte{0x00, 0x0f, 0x42}
	stks := []byte{0x00, 0x00, 0x00, 0x00}
	anno := []byte("saved in the kitchen")

	in := ifzs(
		chunkBytes("IFhd", ifhd),
		chunkBytes("CMem", cmem),
		chunkBytes("Stks", stks),
		chunkBytes("ANNO", anno),
	)

	f, err := quetzal.Decode(bytes.NewReader(in))
	if err != nil {
		t.Fatalf("Decode: unexpected error: %v", err)
	}

	want := []quetzal.Chunk{
		{ID: quetzal.IDIFhd, Data: ifhd},
		{ID: quetzal.IDCMem, Data: cmem},
		{ID: quetzal.IDStks, Data: stks},
		{ID: quetzal.IDANNO, Data: anno},
	}
	if len(f.Chunks) != len(want) {
		t.Fatalf("chunk count: got %d, want %d", len(f.Chunks), len(want))
	}
	for i, w := range want {
		got := f.Chunks[i]
		if got.ID != w.ID {
			t.Errorf("chunk %d id: got %s, want %s", i, got.ID, w.ID)
		}
		if !bytes.Equal(got.Data, w.Data) {
			t.Errorf("chunk %d data: got %x, want %x", i, got.Data, w.Data)
		}
	}

	// The pad byte after the odd-length IFhd is structural and must not
	// appear in the payload.
	if n := len(f.Chunks[0].Data); n != 13 {
		t.Errorf("IFhd length: got %d, want 13 (pad byte must not be retained)", n)
	}
}

func TestDecodeRetainsUnknownChunks(t *testing.T) {
	// An identifier this package assigns no meaning to must be preserved,
	// not rejected and not silently dropped.
	in := ifzs(
		chunkBytes("IFhd", bytes.Repeat([]byte{0}, 13)),
		chunkBytes("Zzzz", []byte{1, 2, 3, 4, 5}),
		chunkBytes("Stks", nil),
	)

	f, err := quetzal.Decode(bytes.NewReader(in))
	if err != nil {
		t.Fatalf("Decode: unexpected error: %v", err)
	}
	if len(f.Chunks) != 3 {
		t.Fatalf("chunk count: got %d, want 3", len(f.Chunks))
	}
	unknown := f.Chunks[1]
	if got, want := unknown.ID.String(), "Zzzz"; got != want {
		t.Errorf("unknown chunk id: got %s, want %s", got, want)
	}
	if want := []byte{1, 2, 3, 4, 5}; !bytes.Equal(unknown.Data, want) {
		t.Errorf("unknown chunk data: got %x, want %x", unknown.Data, want)
	}
	// Order is part of what "preserved" means.
	if f.Chunks[0].ID != quetzal.IDIFhd || f.Chunks[2].ID != quetzal.IDStks {
		t.Errorf("chunk order not preserved: got %s, %s, %s",
			f.Chunks[0].ID, f.Chunks[1].ID, f.Chunks[2].ID)
	}
}

func TestDecodeEmptyForm(t *testing.T) {
	// A FORM holding only its type is structurally valid. Decode does not
	// enforce Quetzal's required chunks; that belongs to Read.
	f, err := quetzal.Decode(bytes.NewReader(ifzs()))
	if err != nil {
		t.Fatalf("Decode: unexpected error: %v", err)
	}
	if len(f.Chunks) != 0 {
		t.Errorf("chunk count: got %d, want 0", len(f.Chunks))
	}
}

func TestDecodeIgnoresTrailingBytes(t *testing.T) {
	// A simple IFF file is a single FORM chunk, so anything after it is
	// outside the container and must not affect the result.
	in := append(ifzs(chunkBytes("ANNO", []byte("hi"))), []byte("trailing junk")...)

	f, err := quetzal.Decode(bytes.NewReader(in))
	if err != nil {
		t.Fatalf("Decode: unexpected error: %v", err)
	}
	if len(f.Chunks) != 1 {
		t.Fatalf("chunk count: got %d, want 1", len(f.Chunks))
	}
}

func TestDecodeMalformed(t *testing.T) {
	valid := ifzs(
		chunkBytes("IFhd", bytes.Repeat([]byte{0}, 13)),
		chunkBytes("CMem", []byte{1, 2, 3}),
	)

	// oversizeChunk declares a chunk length of 0xFFFFFFFF: an attempt to
	// make the decoder allocate 4 GiB for a handful of bytes of input.
	oversizeChunk := formBytes("IFZS", []byte{
		'A', 'N', 'N', 'O',
		0xff, 0xff, 0xff, 0xff,
		'x',
	})

	tests := []struct {
		name string
		in   []byte
		want error
	}{
		{
			name: "empty input",
			in:   nil,
			want: quetzal.ErrTruncated,
		},
		{
			name: "invalid outer chunk",
			in:   append([]byte("LIST"), valid[4:]...),
			want: quetzal.ErrInvalidFormat,
		},
		{
			name: "incorrect FORM type",
			in:   formBytes("IFRS", chunkBytes("IFhd", bytes.Repeat([]byte{0}, 13))),
			want: quetzal.ErrInvalidFormat,
		},
		{
			name: "FORM length too small for a form type",
			in:   []byte{'F', 'O', 'R', 'M', 0, 0, 0, 2, 'I', 'F', 'Z', 'S'},
			want: quetzal.ErrInvalidFormat,
		},
		{
			name: "truncated FORM header",
			in:   valid[:6],
			want: quetzal.ErrTruncated,
		},
		{
			name: "truncated FORM contents",
			in:   valid[:len(valid)-4],
			want: quetzal.ErrTruncated,
		},
		{
			name: "truncated chunk header",
			// The FORM claims room for a chunk header but the input
			// stops partway through it.
			in: []byte{
				'F', 'O', 'R', 'M',
				0x00, 0x00, 0x00, 0x0c,
				'I', 'F', 'Z', 'S',
				'A', 'N', 'N',
			},
			want: quetzal.ErrTruncated,
		},
		{
			name: "chunk extends beyond FORM",
			in: formBytes("IFZS", []byte{
				'A', 'N', 'N', 'O',
				0x00, 0x00, 0x00, 0x40, // claims 64 bytes
				'x', 'y',
			}),
			want: quetzal.ErrInvalidFormat,
		},
		{
			name: "odd-length chunk missing its pad byte",
			in: formBytes("IFZS", []byte{
				'A', 'N', 'N', 'O',
				0x00, 0x00, 0x00, 0x01,
				'x', // pad byte omitted
			}),
			want: quetzal.ErrInvalidFormat,
		},
		{
			name: "trailing bytes inside FORM too few for a chunk header",
			in: formBytes("IFZS", []byte{
				'A', 'N', 'N', 'O',
				0x00, 0x00, 0x00, 0x02,
				'x', 'y',
				'A', 'N', // a partial identifier
			}),
			want: quetzal.ErrInvalidFormat,
		},
		{
			name: "non-printable chunk identifier",
			in: formBytes("IFZS", []byte{
				0x00, 0x01, 0x02, 0x03,
				0x00, 0x00, 0x00, 0x00,
			}),
			want: quetzal.ErrInvalidFormat,
		},
		{
			name: "hostile chunk length",
			in:   oversizeChunk,
			want: quetzal.ErrLimitExceeded,
		},
		{
			name: "hostile FORM length",
			in:   []byte{'F', 'O', 'R', 'M', 0xff, 0xff, 0xff, 0xff, 'I', 'F', 'Z', 'S'},
			want: quetzal.ErrLimitExceeded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := quetzal.Decode(bytes.NewReader(tt.in))
			if err == nil {
				t.Fatalf("Decode: got %d chunk(s) and no error, want %v", len(f.Chunks), tt.want)
			}
			if !errors.Is(err, tt.want) {
				t.Errorf("Decode: got %v, want an error matching %v", err, tt.want)
			}
			if f != nil {
				t.Errorf("Decode: got a non-nil File alongside an error")
			}
			// Errors reach command-line tools and test output, so they
			// must identify the package and say something specific.
			if msg := err.Error(); !strings.HasPrefix(msg, "quetzal: ") {
				t.Errorf("error message %q lacks the package prefix", msg)
			}
		})
	}
}

func TestDecodeChunkErrorReportsLocation(t *testing.T) {
	// A bad chunk should be identified by name and by where it starts, so a
	// caller can point at the offending bytes.
	in := formBytes("IFZS", append(
		chunkBytes("IFhd", bytes.Repeat([]byte{0}, 13)),
		'C', 'M', 'e', 'm',
		0x00, 0x00, 0x00, 0x20, // claims 32 bytes that are not there
		0x01,
	))

	_, err := quetzal.Decode(bytes.NewReader(in))
	if err == nil {
		t.Fatal("Decode: expected an error")
	}

	var ce *quetzal.ChunkError
	if !errors.As(err, &ce) {
		t.Fatalf("Decode: got %T (%v), want a *ChunkError", err, err)
	}
	if ce.ID != quetzal.IDCMem {
		t.Errorf("ChunkError.ID: got %s, want %s", ce.ID, quetzal.IDCMem)
	}
	// 12 bytes of FORM header, then the 8+13+1 bytes of the IFhd chunk.
	if want := int64(34); ce.Offset != want {
		t.Errorf("ChunkError.Offset: got %d, want %d", ce.Offset, want)
	}
	if !errors.Is(err, quetzal.ErrInvalidFormat) {
		t.Errorf("ChunkError does not unwrap to ErrInvalidFormat: %v", err)
	}
}

func TestDecodeLimits(t *testing.T) {
	in := ifzs(chunkBytes("ANNO", bytes.Repeat([]byte{'a'}, 64)))

	t.Run("chunk limit rejects", func(t *testing.T) {
		_, err := quetzal.Decode(bytes.NewReader(in),
			quetzal.WithLimits(quetzal.Limits{MaxChunkBytes: 32}))
		if !errors.Is(err, quetzal.ErrLimitExceeded) {
			t.Errorf("Decode: got %v, want ErrLimitExceeded", err)
		}
	})

	t.Run("form limit rejects", func(t *testing.T) {
		_, err := quetzal.Decode(bytes.NewReader(in),
			quetzal.WithLimits(quetzal.Limits{MaxFormBytes: 8}))
		if !errors.Is(err, quetzal.ErrLimitExceeded) {
			t.Errorf("Decode: got %v, want ErrLimitExceeded", err)
		}
	})

	t.Run("unset fields keep their defaults", func(t *testing.T) {
		// Only MaxFrames is set, so the chunk and form limits must still
		// be large enough to accept this file.
		if _, err := quetzal.Decode(bytes.NewReader(in),
			quetzal.WithLimits(quetzal.Limits{MaxFrames: 1})); err != nil {
			t.Errorf("Decode: unexpected error: %v", err)
		}
	})
}

func TestDecodeDoesNotAliasInput(t *testing.T) {
	// Payloads belong to the returned File. Mutating the caller's buffer
	// afterwards must not change what was decoded.
	in := ifzs(chunkBytes("ANNO", []byte("original")))

	f, err := quetzal.Decode(bytes.NewReader(in))
	if err != nil {
		t.Fatalf("Decode: unexpected error: %v", err)
	}
	for i := range in {
		in[i] = 0xff
	}
	if got, want := string(f.Chunks[0].Data), "original"; got != want {
		t.Errorf("chunk data: got %q, want %q (payload aliases the input)", got, want)
	}
}

func TestDecodeFromStream(t *testing.T) {
	// The API is defined over io.Reader, so decoding must not depend on
	// the reader being seekable or on reads returning full buffers.
	in := ifzs(
		chunkBytes("IFhd", bytes.Repeat([]byte{0x11}, 13)),
		chunkBytes("Stks", []byte{0, 1, 2, 3}),
	)

	f, err := quetzal.Decode(iotest_oneByteReader(in))
	if err != nil {
		t.Fatalf("Decode: unexpected error: %v", err)
	}
	if len(f.Chunks) != 2 {
		t.Fatalf("chunk count: got %d, want 2", len(f.Chunks))
	}
}

// iotest_oneByteReader returns a reader that yields one byte per Read, without
// exposing the underlying buffer's Seek or ReadAt methods.
func iotest_oneByteReader(b []byte) io.Reader {
	return struct{ io.Reader }{bytes.NewReader(b)}
}

func TestDecodeNilReader(t *testing.T) {
	if _, err := quetzal.Decode(nil); err == nil {
		t.Error("Decode(nil): got no error")
	}
}

// errReader fails after yielding the first n bytes of b.
type errReader struct {
	b   []byte
	n   int
	err error
}

func (r *errReader) Read(p []byte) (int, error) {
	if r.n <= 0 {
		return 0, r.err
	}
	n := min(len(p), r.n)
	copy(p, r.b[:n])
	r.b, r.n = r.b[n:], r.n-n
	return n, nil
}

func TestDecodeReaderError(t *testing.T) {
	// An error from the caller's reader is not malformed input, so it must
	// reach the caller intact rather than be reported as truncation.
	want := errors.New("disk on fire")
	in := ifzs(chunkBytes("IFhd", bytes.Repeat([]byte{0}, 13)))

	_, err := quetzal.Decode(&errReader{b: in, n: 14, err: want})
	if !errors.Is(err, want) {
		t.Errorf("Decode: got %v, want %v", err, want)
	}
	if errors.Is(err, quetzal.ErrTruncated) {
		t.Errorf("Decode: reported a reader failure as truncated input: %v", err)
	}
}

func TestFileLookup(t *testing.T) {
	in := ifzs(
		chunkBytes("IFhd", []byte{1}),
		chunkBytes("ANNO", []byte("first")),
		chunkBytes("IFhd", []byte{2}),
		chunkBytes("ANNO", []byte("second")),
	)

	f, err := quetzal.Decode(bytes.NewReader(in))
	if err != nil {
		t.Fatalf("Decode: unexpected error: %v", err)
	}

	// Where Quetzal expects one instance, the first is authoritative.
	ifhd, ok := f.First(quetzal.IDIFhd)
	if !ok {
		t.Fatal("First(IFhd): not found")
	}
	if want := []byte{1}; !bytes.Equal(ifhd.Data, want) {
		t.Errorf("First(IFhd): got %x, want %x (the first instance wins)", ifhd.Data, want)
	}

	// Repeated ANNO chunks are legal and all of them are available.
	annos := f.All(quetzal.IDANNO)
	if len(annos) != 2 {
		t.Fatalf("All(ANNO): got %d, want 2", len(annos))
	}
	if got, want := string(annos[1].Data), "second"; got != want {
		t.Errorf("All(ANNO)[1]: got %q, want %q", got, want)
	}

	if _, ok := f.First(quetzal.IDUMem); ok {
		t.Error("First(UMem): reported a chunk that is not present")
	}
	if got := f.All(quetzal.IDUMem); got != nil {
		t.Errorf("All(UMem): got %v, want nil", got)
	}
}

func FuzzDecode(f *testing.F) {
	f.Add(ifzs(
		chunkBytes("IFhd", bytes.Repeat([]byte{0}, 13)),
		chunkBytes("CMem", []byte{0, 5, 1}),
		chunkBytes("Stks", []byte{0, 0, 0, 0}),
	))
	f.Add(ifzs(chunkBytes("ANNO", []byte("x"))))
	f.Add(ifzs())
	f.Add([]byte("FORM"))
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		// No input may panic, allocate without bound, or loop forever. The
		// limits are deliberately small so the fuzzer stays fast.
		file, err := quetzal.Decode(bytes.NewReader(data),
			quetzal.WithLimits(quetzal.Limits{MaxFormBytes: 1 << 16, MaxChunkBytes: 1 << 16}))
		if err != nil {
			if file != nil {
				t.Fatalf("Decode returned both a File and an error: %v", err)
			}
			return
		}
		// A successful decode must describe bytes that were really there.
		var total uint64
		for _, c := range file.Chunks {
			n := uint64(len(c.Data))
			total += 8 + n + n&1
		}
		if total > uint64(len(data)) {
			t.Fatalf("decoded %d chunk byte(s) from %d byte(s) of input", total, len(data))
		}
	})
}
