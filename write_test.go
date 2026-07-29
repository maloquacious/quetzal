// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package quetzal_test

import (
	"bytes"
	"errors"
	"io"
	"sync"
	"testing"

	"github.com/maloquacious/quetzal"
)

// saveStory returns the story that ifhdPayload identifies, together with a
// dynamic memory in which one byte has been changed, which is the smallest
// thing a save can differ from its story by.
func saveStory(t *testing.T) (quetzal.Story, []byte) {
	t.Helper()

	story := memoryStory(t)
	current := append([]byte(nil), story.DynamicMemory...)
	current[0x50] ^= 0x3c
	return story, current
}

// sampleSave returns a save for that story: the dummy frame that a version 3
// game requires, one real call frame, and an annotation.
func sampleSave(t *testing.T) (quetzal.Story, *quetzal.Save) {
	t.Helper()

	story, current := saveStory(t)
	return story, &quetzal.Save{
		Header: wantHeader,
		Memory: quetzal.Memory{Encoding: quetzal.MemoryCompressed, Data: current},
		Frames: []quetzal.Frame{
			{Evaluation: []uint16{0x1111}},
			{
				ReturnPC:       0x00abcd,
				ResultVariable: 0x05,
				Arguments:      0x03,
				Locals:         []uint16{0x1234, 0x5678},
				Evaluation:     []uint16{0x9abc},
			},
		},
		Chunks: []quetzal.Chunk{
			{ID: quetzal.IDANNO, Data: []byte("saved in the kitchen")},
		},
	}
}

// writeSave writes a save and returns the bytes it produced.
func writeSave(t *testing.T, story quetzal.Story, save *quetzal.Save, opts ...quetzal.WriteOption) []byte {
	t.Helper()

	var buf bytes.Buffer
	if err := quetzal.Write(&buf, story, save, opts...); err != nil {
		t.Fatalf("Write: unexpected error: %v", err)
	}
	return buf.Bytes()
}

func TestWriteProducesAConformingFile(t *testing.T) {
	story, save := sampleSave(t)
	data := writeSave(t, story, save)

	// The container must be readable by the decoder that knows nothing
	// about what the chunks mean.
	f, err := quetzal.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Decode: unexpected error: %v", err)
	}

	wantOrder := []quetzal.ID{quetzal.IDIFhd, quetzal.IDCMem, quetzal.IDStks, quetzal.IDANNO}
	if len(f.Chunks) != len(wantOrder) {
		t.Fatalf("got %d chunks, want %d", len(f.Chunks), len(wantOrder))
	}
	for i, want := range wantOrder {
		if f.Chunks[i].ID != want {
			t.Errorf("chunk %d: got %s, want %s", i, f.Chunks[i].ID, want)
		}
	}

	// The standard fixes the size of an IFhd payload, and its oddness is
	// what makes every Quetzal file exercise chunk padding.
	if got := len(f.Chunks[0].Data); got != 13 {
		t.Errorf("IFhd payload: got %d bytes, want 13", got)
	}

	// A FORM's length excludes its own eight-byte header.
	if got, want := len(data), 8+int(be32(t, data[4:8])); got != want {
		t.Errorf("file is %d bytes, but the FORM length describes %d", got, want)
	}
}

func TestWriteRoundTrip(t *testing.T) {
	story, save := sampleSave(t)

	for _, encoding := range []quetzal.MemoryEncoding{quetzal.MemoryCompressed, quetzal.MemoryUncompressed} {
		t.Run(encoding.String(), func(t *testing.T) {
			data := writeSave(t, story, save, quetzal.WithEncoding(encoding))

			got, err := quetzal.Read(bytes.NewReader(data), story)
			if err != nil {
				t.Fatalf("Read: unexpected error: %v", err)
			}

			if !headersEqual(got.Header, save.Header) {
				t.Errorf("Header: got %+v, want %+v", got.Header, save.Header)
			}
			if got.Memory.Encoding != encoding {
				t.Errorf("Memory.Encoding: got %s, want %s", got.Memory.Encoding, encoding)
			}
			if !bytes.Equal(got.Memory.Data, save.Memory.Data) {
				t.Error("Memory.Data: dynamic memory did not survive the round trip")
			}
			if !framesEqual(got.Frames, save.Frames) {
				t.Errorf("Frames: got %+v, want %+v", got.Frames, save.Frames)
			}
			if len(got.Chunks) != 1 || got.Chunks[0].ID != quetzal.IDANNO ||
				string(got.Chunks[0].Data) != "saved in the kitchen" {
				t.Errorf("Chunks: got %+v, want the annotation back unchanged", got.Chunks)
			}
		})
	}
}

// TestWriteRoundTripAcrossEncodings checks the property that matters more than
// byte identity: a save rewritten in the other encoding still describes the
// same game state.
func TestWriteRoundTripAcrossEncodings(t *testing.T) {
	story, save := sampleSave(t)

	compressed := writeSave(t, story, save, quetzal.WithEncoding(quetzal.MemoryCompressed))
	uncompressed := writeSave(t, story, save, quetzal.WithEncoding(quetzal.MemoryUncompressed))

	if bytes.Equal(compressed, uncompressed) {
		t.Fatal("the two encodings produced identical bytes, so nothing was compared")
	}
	if len(compressed) >= len(uncompressed) {
		t.Errorf("compressed file is %d bytes and uncompressed is %d; compression gained nothing",
			len(compressed), len(uncompressed))
	}

	first, err := quetzal.Read(bytes.NewReader(compressed), story)
	if err != nil {
		t.Fatalf("Read(compressed): unexpected error: %v", err)
	}
	second, err := quetzal.Read(bytes.NewReader(uncompressed), story)
	if err != nil {
		t.Fatalf("Read(uncompressed): unexpected error: %v", err)
	}

	if !bytes.Equal(first.Memory.Data, second.Memory.Data) {
		t.Error("the two encodings describe different dynamic memory")
	}
	if !headersEqual(first.Header, second.Header) || !framesEqual(first.Frames, second.Frames) {
		t.Error("the two encodings describe different saved state")
	}
}

func TestWriteRejects(t *testing.T) {
	story, current := saveStory(t)

	valid := func() *quetzal.Save {
		return &quetzal.Save{
			Header: wantHeader,
			Memory: quetzal.Memory{Encoding: quetzal.MemoryCompressed, Data: current},
			Frames: []quetzal.Frame{{}},
		}
	}

	tests := []struct {
		name  string
		build func() *quetzal.Save
		story quetzal.Story
		want  error
	}{
		{
			name: "a save for a different story",
			build: func() *quetzal.Save {
				s := valid()
				s.Header.Release = 999
				return s
			},
			story: story,
			want:  quetzal.ErrStoryMismatch,
		},
		{
			name: "a program counter too large for three bytes",
			build: func() *quetzal.Save {
				s := valid()
				s.Header.PC = quetzal.MaxPC + 1
				return s
			},
			story: story,
			want:  quetzal.ErrInvalidFormat,
		},
		{
			name: "dynamic memory of the wrong length",
			build: func() *quetzal.Save {
				s := valid()
				s.Memory.Data = current[:len(current)-1]
				return s
			},
			story: story,
			want:  quetzal.ErrInvalidFormat,
		},
		{
			name: "an encoding that was never set",
			build: func() *quetzal.Save {
				s := valid()
				s.Memory.Encoding = 0
				return s
			},
			story: story,
			want:  quetzal.ErrInvalidFormat,
		},
		{
			name: "a version 3 save with no dummy frame",
			build: func() *quetzal.Save {
				s := valid()
				s.Frames = nil
				return s
			},
			story: story,
			want:  quetzal.ErrInvalidFormat,
		},
		{
			name: "a version 3 save whose first frame is a real call",
			build: func() *quetzal.Save {
				s := valid()
				s.Frames = []quetzal.Frame{{ReturnPC: 0x1234}}
				return s
			},
			story: story,
			want:  quetzal.ErrInvalidFormat,
		},
		{
			name: "a frame with too many locals",
			build: func() *quetzal.Save {
				s := valid()
				s.Frames = append(s.Frames, quetzal.Frame{
					Locals: make([]uint16, quetzal.MaxLocals+1),
				})
				return s
			},
			story: story,
			want:  quetzal.ErrInvalidFormat,
		},
		{
			name: "an additional chunk that duplicates a required one",
			build: func() *quetzal.Save {
				s := valid()
				s.Chunks = []quetzal.Chunk{{ID: quetzal.IDIFhd, Data: ifhdPayload}}
				return s
			},
			story: story,
			want:  quetzal.ErrInvalidFormat,
		},
		{
			name: "an additional chunk with an unwritable identifier",
			build: func() *quetzal.Save {
				s := valid()
				s.Chunks = []quetzal.Chunk{{ID: quetzal.ID{'A', 'N', 'N', 0x00}}}
				return s
			},
			story: story,
			want:  quetzal.ErrInvalidFormat,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			save := tt.build()

			var buf bytes.Buffer
			err := quetzal.Write(&buf, tt.story, save)
			if !errors.Is(err, tt.want) {
				t.Errorf("Write: got %v, want %v", err, tt.want)
			}
			if buf.Len() != 0 {
				t.Errorf("Write wrote %d bytes despite failing", buf.Len())
			}

			// Validate must reach the same verdict on its own, since a
			// caller checking a save before writing it should not be
			// told a different story by the writer.
			if err := save.Validate(tt.story); !errors.Is(err, tt.want) {
				t.Errorf("Validate: got %v, want %v", err, tt.want)
			}
		})
	}
}

func TestWriteRejectsNilArguments(t *testing.T) {
	story, save := sampleSave(t)

	if err := quetzal.Write(nil, story, save); !errors.Is(err, quetzal.ErrInvalidFormat) {
		t.Errorf("Write(nil writer): got %v, want ErrInvalidFormat", err)
	}
	if err := quetzal.Write(io.Discard, story, nil); !errors.Is(err, quetzal.ErrInvalidFormat) {
		t.Errorf("Write(nil save): got %v, want ErrInvalidFormat", err)
	}

	f := &quetzal.File{}
	if _, err := f.WriteTo(nil); !errors.Is(err, quetzal.ErrInvalidFormat) {
		t.Errorf("WriteTo(nil): got %v, want ErrInvalidFormat", err)
	}
}

// TestValidateAcceptsAWellFormedSave guards against a Validate that rejects
// everything and so passes the tests above for the wrong reason.
func TestValidateAcceptsAWellFormedSave(t *testing.T) {
	story, save := sampleSave(t)
	if err := save.Validate(story); err != nil {
		t.Errorf("Validate: unexpected error: %v", err)
	}
}

// TestWriteDoesNotMutate checks the ownership rule for the writing direction:
// writing reads the save and the story and changes neither.
func TestWriteDoesNotMutate(t *testing.T) {
	story, save := sampleSave(t)

	storyMemory := append([]byte(nil), story.DynamicMemory...)
	saveMemory := append([]byte(nil), save.Memory.Data...)
	annotation := append([]byte(nil), save.Chunks[0].Data...)
	frames := cloneFrames(save.Frames)
	header := save.Header

	// The overriding option is the one that most obviously might reach
	// back into the save, since it changes how the memory is written.
	writeSave(t, story, save, quetzal.WithEncoding(quetzal.MemoryUncompressed))

	if !bytes.Equal(story.DynamicMemory, storyMemory) {
		t.Error("the story's dynamic memory was modified")
	}
	if !bytes.Equal(save.Memory.Data, saveMemory) {
		t.Error("the save's dynamic memory was modified")
	}
	if save.Memory.Encoding != quetzal.MemoryCompressed {
		t.Errorf("the save's encoding became %s; the option must not reach back into the save",
			save.Memory.Encoding)
	}
	if !bytes.Equal(save.Chunks[0].Data, annotation) {
		t.Error("the save's annotation was modified")
	}
	if !framesEqual(save.Frames, frames) {
		t.Error("the save's frames were modified")
	}
	if !headersEqual(save.Header, header) {
		t.Error("the save's header was modified")
	}
}

// TestEncodeDoesNotAliasTheSave checks that a caller can go on using a save
// after encoding it without the file it produced changing underneath.
func TestEncodeDoesNotAliasTheSave(t *testing.T) {
	story, save := sampleSave(t)

	file, err := save.Encode(story)
	if err != nil {
		t.Fatalf("Encode: unexpected error: %v", err)
	}
	before := append([]byte(nil), file.Chunks[3].Data...)

	save.Chunks[0].Data[0] = 'X'
	if !bytes.Equal(file.Chunks[3].Data, before) {
		t.Error("the encoded file aliases the save's chunk payloads")
	}
}

// TestReadDoesNotAliasTheStory checks the ownership rule that matters most to
// a caller holding one story and serving many saves from it: nothing a Read
// returns may share memory with the story it was read against.
//
// Sharing would be invisible until it was expensive. A server caching a parsed
// story and handing it to every request would find one caller's writes to
// save.Memory.Data corrupting the cached story, and through it every other
// caller's saves.
func TestReadDoesNotAliasTheStory(t *testing.T) {
	for _, encoding := range []quetzal.MemoryEncoding{quetzal.MemoryCompressed, quetzal.MemoryUncompressed} {
		t.Run(encoding.String(), func(t *testing.T) {
			story, save := sampleSave(t)
			data := writeSave(t, story, save, quetzal.WithEncoding(encoding))

			got, err := quetzal.Read(bytes.NewReader(data), story)
			if err != nil {
				t.Fatalf("Read: unexpected error: %v", err)
			}

			// The story must not reach into the save.
			restored := append([]byte(nil), got.Memory.Data...)
			for i := range story.DynamicMemory {
				story.DynamicMemory[i] ^= 0xff
			}
			if !bytes.Equal(got.Memory.Data, restored) {
				t.Error("the save's memory changed when the story's did; Read aliased the story")
			}
			for i := range story.DynamicMemory {
				story.DynamicMemory[i] ^= 0xff
			}

			// And the save must not reach into the story.
			original := append([]byte(nil), story.DynamicMemory...)
			for i := range got.Memory.Data {
				got.Memory.Data[i] ^= 0xff
			}
			if !bytes.Equal(story.DynamicMemory, original) {
				t.Error("the story's memory changed when the save's did; Read aliased the story")
			}
		})
	}
}

// TestEncodeDoesNotAliasTheStory checks the same rule in the writing
// direction. Uncompressed memory is where a zero-copy shortcut would be most
// tempting, since the payload and the memory are the same bytes.
func TestEncodeDoesNotAliasTheStory(t *testing.T) {
	story, save := sampleSave(t)

	file, err := save.Encode(story, quetzal.WithEncoding(quetzal.MemoryUncompressed))
	if err != nil {
		t.Fatalf("Encode: unexpected error: %v", err)
	}

	umem := file.Chunks[1]
	if umem.ID != quetzal.IDUMem {
		t.Fatalf("chunk 1 is %s, want %s", umem.ID, quetzal.IDUMem)
	}
	payload := append([]byte(nil), umem.Data...)

	for i := range save.Memory.Data {
		save.Memory.Data[i] ^= 0xff
	}
	if !bytes.Equal(umem.Data, payload) {
		t.Error("the encoded payload changed with the save; Encode aliased the save's memory")
	}
}

// TestStorySurvivesConcurrentUse checks that one story can serve many
// simultaneous reads and writes, which is what a server caching parsed stories
// will do. It is a race-detector test above all: run it with -race.
func TestStorySurvivesConcurrentUse(t *testing.T) {
	story, save := sampleSave(t)
	want := writeSave(t, story, save)

	before := append([]byte(nil), story.DynamicMemory...)

	const workers = 16
	results := make([][]byte, workers)

	var wg sync.WaitGroup
	for i := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()

			// Each worker reads the shared story and writes a save
			// back against it, with no synchronization of its own.
			got, err := quetzal.Read(bytes.NewReader(want), story)
			if err != nil {
				t.Errorf("worker %d: Read: %v", i, err)
				return
			}
			var buf bytes.Buffer
			if err := quetzal.Write(&buf, story, got); err != nil {
				t.Errorf("worker %d: Write: %v", i, err)
				return
			}
			results[i] = buf.Bytes()
		}()
	}
	wg.Wait()

	if !bytes.Equal(story.DynamicMemory, before) {
		t.Fatal("the shared story was modified")
	}
	for i, got := range results {
		if !bytes.Equal(got, want) {
			t.Errorf("worker %d produced a different file from the same story and save", i)
		}
	}
}

func TestWriteToPadsOddLengthChunks(t *testing.T) {
	// Two chunks whose payloads are odd and even, so that the pad byte and
	// its absence both appear.
	f := &quetzal.File{Chunks: []quetzal.Chunk{
		{ID: quetzal.IDANNO, Data: []byte("odd")},
		{ID: quetzal.IDAUTH, Data: []byte("even")},
	}}

	var buf bytes.Buffer
	n, err := f.WriteTo(&buf)
	if err != nil {
		t.Fatalf("WriteTo: unexpected error: %v", err)
	}
	if n != int64(buf.Len()) {
		t.Errorf("WriteTo reported %d bytes but wrote %d", n, buf.Len())
	}

	want := ifzs(
		chunkBytes("ANNO", []byte("odd")),
		chunkBytes("AUTH", []byte("even")),
	)
	if !bytes.Equal(buf.Bytes(), want) {
		t.Errorf("WriteTo produced\n\t%x\nwant\n\t%x", buf.Bytes(), want)
	}

	// The pad byte is structural: it must not reappear in the payload.
	back, err := quetzal.Decode(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("Decode: unexpected error: %v", err)
	}
	if got := string(back.Chunks[0].Data); got != "odd" {
		t.Errorf("odd-length payload came back as %q", got)
	}
}

func TestWriteToEmptyFile(t *testing.T) {
	var buf bytes.Buffer
	if _, err := (&quetzal.File{}).WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: unexpected error: %v", err)
	}

	// A FORM holding nothing but its type is still a FORM.
	if want := ifzs(); !bytes.Equal(buf.Bytes(), want) {
		t.Errorf("WriteTo produced %x, want %x", buf.Bytes(), want)
	}
}

func TestWriteToRejectsAnUnwritableIdentifier(t *testing.T) {
	f := &quetzal.File{Chunks: []quetzal.Chunk{{ID: quetzal.ID{'A', 'N', 'N', 0x7f}}}}

	var buf bytes.Buffer
	if _, err := f.WriteTo(&buf); !errors.Is(err, quetzal.ErrInvalidFormat) {
		t.Errorf("WriteTo: got %v, want ErrInvalidFormat", err)
	}
	if buf.Len() != 0 {
		t.Errorf("WriteTo wrote %d bytes despite failing", buf.Len())
	}
}

// failingWriter fails after accepting a fixed number of bytes, so that a write
// can be interrupted at a chosen point.
type failingWriter struct {
	remaining int
	short     bool
	written   int
}

func (w *failingWriter) Write(p []byte) (int, error) {
	if len(p) <= w.remaining {
		w.remaining -= len(p)
		w.written += len(p)
		return len(p), nil
	}
	n := w.remaining
	w.remaining = 0
	w.written += n
	if w.short {
		// A writer that reports a short write without an error, which
		// io.Writer forbids but nothing prevents.
		return n, nil
	}
	return n, io.ErrClosedPipe
}

func TestWriteToReportsWriterErrors(t *testing.T) {
	// One odd-length and one even-length chunk, so that failing at
	// different points covers the header, the chunk header, the payload,
	// and the pad byte.
	f := &quetzal.File{Chunks: []quetzal.Chunk{
		{ID: quetzal.IDANNO, Data: []byte("odd")},
		{ID: quetzal.IDAUTH, Data: []byte("even")},
	}}

	// 12 for the FORM header, then 8+3+1 for the odd chunk and 8+4 for the
	// even one. Stopping at 23 interrupts the pad byte, and at 32 the second
	// payload.
	for _, stop := range []int{0, 5, 12, 16, 19, 20, 23, 25, 30, 32} {
		w := &failingWriter{remaining: stop}
		n, err := f.WriteTo(w)
		if !errors.Is(err, io.ErrClosedPipe) {
			t.Errorf("WriteTo stopping at %d: got %v, want io.ErrClosedPipe", stop, err)
		}
		if n != int64(w.written) {
			t.Errorf("WriteTo stopping at %d: reported %d bytes, writer accepted %d", stop, n, w.written)
		}
	}

	t.Run("a short write without an error is still a failure", func(t *testing.T) {
		w := &failingWriter{remaining: 4, short: true}
		if _, err := f.WriteTo(w); !errors.Is(err, io.ErrShortWrite) {
			t.Errorf("WriteTo: got %v, want io.ErrShortWrite", err)
		}
	})
}

func TestWriteToRejectsAFormTooLongToDescribe(t *testing.T) {
	// A FORM's length is four bytes. Reaching the limit for real would need
	// four gigabytes, so the chunk here describes a payload it does not
	// hold; WriteTo must reject it on the length it computes, before it
	// tries to write anything.
	huge := make([]byte, 1<<20)
	chunks := make([]quetzal.Chunk, 4096)
	for i := range chunks {
		chunks[i] = quetzal.Chunk{ID: quetzal.IDANNO, Data: huge}
	}

	var buf bytes.Buffer
	if _, err := (&quetzal.File{Chunks: chunks}).WriteTo(&buf); !errors.Is(err, quetzal.ErrInvalidFormat) {
		t.Errorf("WriteTo: got %v, want ErrInvalidFormat", err)
	}
	if buf.Len() != 0 {
		t.Errorf("WriteTo wrote %d bytes despite failing", buf.Len())
	}
}

func TestReadRequiresTheRequiredChunks(t *testing.T) {
	story, current := saveStory(t)

	cmem, err := quetzal.EncodeCMem(current, story.DynamicMemory)
	if err != nil {
		t.Fatalf("EncodeCMem: unexpected error: %v", err)
	}
	stks := dummyFrameBytes(nil)

	tests := []struct {
		name   string
		chunks [][]byte
	}{
		{
			name:   "no IFhd",
			chunks: [][]byte{chunkBytes("CMem", cmem), chunkBytes("Stks", stks)},
		},
		{
			name:   "no memory chunk",
			chunks: [][]byte{chunkBytes("IFhd", ifhdPayload), chunkBytes("Stks", stks)},
		},
		{
			name:   "no Stks",
			chunks: [][]byte{chunkBytes("IFhd", ifhdPayload), chunkBytes("CMem", cmem)},
		},
		{
			name: "both CMem and UMem",
			chunks: [][]byte{
				chunkBytes("IFhd", ifhdPayload),
				chunkBytes("CMem", cmem),
				chunkBytes("UMem", current),
				chunkBytes("Stks", stks),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := quetzal.Read(bytes.NewReader(ifzs(tt.chunks...)), story)
			if !errors.Is(err, quetzal.ErrInvalidFormat) {
				t.Errorf("Read: got %v, want ErrInvalidFormat", err)
			}
		})
	}
}

// TestReadRequiresIFhdFirst covers the rule that the IFhd must precede the
// memory and stack chunks, which exists so that an interpreter learns it has
// the wrong story before it decodes anything against it.
func TestReadRequiresIFhdFirst(t *testing.T) {
	story, current := saveStory(t)

	cmem, err := quetzal.EncodeCMem(current, story.DynamicMemory)
	if err != nil {
		t.Fatalf("EncodeCMem: unexpected error: %v", err)
	}
	ifhd := chunkBytes("IFhd", ifhdPayload)
	stks := chunkBytes("Stks", dummyFrameBytes(nil))

	tests := []struct {
		name    string
		chunks  [][]byte
		wantErr bool
	}{
		{
			name:   "IFhd first",
			chunks: [][]byte{ifhd, chunkBytes("CMem", cmem), stks},
		},
		{
			name:   "an annotation may come first",
			chunks: [][]byte{chunkBytes("ANNO", []byte("hello")), ifhd, chunkBytes("CMem", cmem), stks},
		},
		{
			name:    "memory before IFhd",
			chunks:  [][]byte{chunkBytes("CMem", cmem), ifhd, stks},
			wantErr: true,
		},
		{
			name:    "stacks before IFhd",
			chunks:  [][]byte{stks, ifhd, chunkBytes("CMem", cmem)},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := quetzal.Read(bytes.NewReader(ifzs(tt.chunks...)), story)
			switch {
			case tt.wantErr && !errors.Is(err, quetzal.ErrInvalidFormat):
				t.Errorf("Read: got %v, want ErrInvalidFormat", err)
			case !tt.wantErr && err != nil:
				t.Errorf("Read: unexpected error: %v", err)
			}
		})
	}
}

// TestReadRequiresTheDummyFrame covers the one thing a decoded file can be
// wrong about that decoding alone cannot detect: whether the call stack suits
// the story's Z-machine version. Every version but 6 must begin with the dummy
// frame that holds top-level evaluation-stack state, and a save without one
// would restore with that state missing.
func TestReadRequiresTheDummyFrame(t *testing.T) {
	story, current := saveStory(t)

	cmem, err := quetzal.EncodeCMem(current, story.DynamicMemory)
	if err != nil {
		t.Fatalf("EncodeCMem: unexpected error: %v", err)
	}

	// A first frame that returns somewhere is a real call, not the dummy.
	call := frameBytes(0x00abcd, 0, 0x05, 0x01, []uint16{0x1234}, nil)

	_, err = quetzal.Read(bytes.NewReader(ifzs(
		chunkBytes("IFhd", ifhdPayload),
		chunkBytes("CMem", cmem),
		chunkBytes("Stks", call),
	)), story)
	if !errors.Is(err, quetzal.ErrInvalidFormat) {
		t.Errorf("Read: got %v, want ErrInvalidFormat", err)
	}

	var frame *quetzal.FrameError
	if !errors.As(err, &frame) {
		t.Fatalf("Read: error is not a *FrameError: %v", err)
	}
	if frame.Index != 0 {
		t.Errorf("FrameError.Index: got %d, want 0", frame.Index)
	}
}

func TestReadRejectsAMismatchedStory(t *testing.T) {
	story, save := sampleSave(t)
	data := writeSave(t, story, save)

	other, err := quetzal.ParseStory(storyImage(3, 89, "840726", 0x1234, 0x100, 0x200))
	if err != nil {
		t.Fatalf("ParseStory: unexpected error: %v", err)
	}

	_, err = quetzal.Read(bytes.NewReader(data), other)
	if !errors.Is(err, quetzal.ErrStoryMismatch) {
		t.Errorf("Read: got %v, want ErrStoryMismatch", err)
	}

	var mismatch *quetzal.StoryMismatchError
	if !errors.As(err, &mismatch) {
		t.Fatalf("Read: error is not a *StoryMismatchError: %v", err)
	}
	if mismatch.Save.Release != 88 || mismatch.Story.Release != 89 {
		t.Errorf("error reports save %v and story %v", mismatch.Save, mismatch.Story)
	}
}

func TestReadReportsDecodeErrors(t *testing.T) {
	story, _ := saveStory(t)

	// Long enough to be a FORM header, so that the complaint is about what
	// the bytes say rather than about there not being enough of them.
	if _, err := quetzal.Read(bytes.NewReader([]byte("NOPE\x00\x00\x00\x04IFZS")), story); !errors.Is(err, quetzal.ErrInvalidFormat) {
		t.Errorf("Read: got %v, want ErrInvalidFormat", err)
	}
	if _, err := quetzal.Read(bytes.NewReader([]byte("FORM")), story); !errors.Is(err, quetzal.ErrTruncated) {
		t.Errorf("Read: got %v, want ErrTruncated", err)
	}
}

// TestReadHonorsOptions checks that the options Read forwards to the decoder
// still take effect, since a writer-side change could easily drop them.
func TestReadHonorsOptions(t *testing.T) {
	story, save := sampleSave(t)
	data := writeSave(t, story, save)

	limits := quetzal.Limits{MaxFormBytes: 16}
	if _, err := quetzal.Read(bytes.NewReader(data), story, quetzal.WithLimits(limits)); !errors.Is(err, quetzal.ErrLimitExceeded) {
		t.Errorf("Read: got %v, want ErrLimitExceeded", err)
	}
}

// TestReadKeepsDuplicatesOutOfTheSave checks that the first-instance rule and
// the round trip agree: a duplicate that the reader ignores must not come back
// as an additional chunk, since writing it again would produce a file with two
// of a chunk that may appear once.
func TestReadKeepsDuplicatesOutOfTheSave(t *testing.T) {
	story, current := saveStory(t)

	cmem, err := quetzal.EncodeCMem(current, story.DynamicMemory)
	if err != nil {
		t.Fatalf("EncodeCMem: unexpected error: %v", err)
	}

	// A second IFhd for a different story, and a second Stks. Both must be
	// ignored, and neither may reach the save.
	other := append([]byte(nil), ifhdPayload...)
	other[1] = 0x59

	save, err := quetzal.Read(bytes.NewReader(ifzs(
		chunkBytes("IFhd", ifhdPayload),
		chunkBytes("CMem", cmem),
		chunkBytes("Stks", dummyFrameBytes(nil)),
		chunkBytes("IFhd", other),
		chunkBytes("Stks", dummyFrameBytes([]uint16{0x4321})),
		chunkBytes("ANNO", []byte("kept")),
	)), story)
	if err != nil {
		t.Fatalf("Read: unexpected error: %v", err)
	}

	if save.Header.Release != 88 {
		t.Errorf("Header: got release %d, want the first IFhd's 88", save.Header.Release)
	}
	if len(save.Frames) != 1 || len(save.Frames[0].Evaluation) != 0 {
		t.Errorf("Frames: got %+v, want only the first Stks chunk's dummy frame", save.Frames)
	}
	if len(save.Chunks) != 1 || save.Chunks[0].ID != quetzal.IDANNO {
		t.Errorf("Chunks: got %+v, want only the annotation", save.Chunks)
	}

	// Writing it back must therefore produce a file with one of each.
	back, err := quetzal.Decode(bytes.NewReader(writeSave(t, story, save)))
	if err != nil {
		t.Fatalf("Decode: unexpected error: %v", err)
	}
	if len(back.Chunks) != 4 {
		t.Errorf("rewritten file has %d chunks, want 4", len(back.Chunks))
	}
}

func TestReadDropsInterpreterDataThatMustNotBeCopied(t *testing.T) {
	story, current := saveStory(t)

	cmem, err := quetzal.EncodeCMem(current, story.DynamicMemory)
	if err != nil {
		t.Fatalf("EncodeCMem: unexpected error: %v", err)
	}

	// An IntD payload: operating system, flags, contents ID, the reserved
	// word, the interpreter, and then whatever the interpreter stored.
	intD := func(flags byte) []byte {
		b := []byte("UNIX")
		b = append(b, flags, 0x00, 0x00, 0x00)
		b = append(b, "JZIP"...)
		return append(b, "/home/player/zork1.z3"...)
	}

	tests := []struct {
		name    string
		payload []byte
		keep    bool
	}{
		{name: "no restrictions", payload: intD(0x00), keep: true},
		{name: "position-specific", payload: intD(0x01)},
		{name: "machine-specific", payload: intD(0x02)},
		{name: "both", payload: intD(0x03)},
		{name: "too short to state its restrictions", payload: []byte("UNIX")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := ifzs(
				chunkBytes("IFhd", ifhdPayload),
				chunkBytes("CMem", cmem),
				chunkBytes("Stks", dummyFrameBytes(nil)),
				chunkBytes("IntD", tt.payload),
			)

			save, err := quetzal.Read(bytes.NewReader(data), story)
			if err != nil {
				t.Fatalf("Read: unexpected error: %v", err)
			}
			if got := len(save.Chunks); got != boolToInt(tt.keep) {
				t.Errorf("Chunks: got %d, want %d", got, boolToInt(tt.keep))
			}

			// Whatever the save does with it, the decoded container
			// keeps every chunk, which is what makes the drop
			// recoverable by a caller that knows its own machine.
			f, err := quetzal.Decode(bytes.NewReader(data))
			if err != nil {
				t.Fatalf("Decode: unexpected error: %v", err)
			}
			if _, ok := f.First(quetzal.IDIntD); !ok {
				t.Error("the decoded file lost the IntD chunk")
			}
		})
	}
}

func TestParseInterpreterData(t *testing.T) {
	payload := []byte{'M', 'A', 'C', 'S', 0x02, 0x00, 0x00, 0x00, ' ', ' ', ' ', ' ', 0xde, 0xad}

	d, err := quetzal.ParseInterpreterData(payload)
	if err != nil {
		t.Fatalf("ParseInterpreterData: unexpected error: %v", err)
	}
	if d.OperatingSystem != (quetzal.ID{'M', 'A', 'C', 'S'}) {
		t.Errorf("OperatingSystem: got %s, want MACS", d.OperatingSystem)
	}
	if d.Interpreter != (quetzal.ID{' ', ' ', ' ', ' '}) {
		t.Errorf("Interpreter: got %s, want four spaces", d.Interpreter)
	}
	if d.Flags != 0x02 || d.ContentsID != 0x00 {
		t.Errorf("Flags/ContentsID: got %#02x/%#02x, want 0x02/0x00", d.Flags, d.ContentsID)
	}
	if !bytes.Equal(d.Data, []byte{0xde, 0xad}) {
		t.Errorf("Data: got %x, want dead", d.Data)
	}
	if d.PositionSpecific() {
		t.Error("PositionSpecific: got true, want false")
	}
	if !d.MachineSpecific() {
		t.Error("MachineSpecific: got false, want true")
	}
	if d.Copyable() {
		t.Error("Copyable: got true; a machine-specific chunk cannot be shown to be copyable")
	}

	t.Run("a payload with no data of its own", func(t *testing.T) {
		d, err := quetzal.ParseInterpreterData(payload[:12])
		if err != nil {
			t.Fatalf("ParseInterpreterData: unexpected error: %v", err)
		}
		if d.Data != nil {
			t.Errorf("Data: got %x, want nil", d.Data)
		}
	})

	t.Run("a payload too short for the fixed header", func(t *testing.T) {
		if _, err := quetzal.ParseInterpreterData(payload[:11]); !errors.Is(err, quetzal.ErrInvalidFormat) {
			t.Errorf("ParseInterpreterData: got %v, want ErrInvalidFormat", err)
		}
	})

	t.Run("does not alias its input", func(t *testing.T) {
		input := append([]byte(nil), payload...)
		d, err := quetzal.ParseInterpreterData(input)
		if err != nil {
			t.Fatalf("ParseInterpreterData: unexpected error: %v", err)
		}
		input[12] = 0x00
		if d.Data[0] != 0xde {
			t.Error("the parsed data aliases the payload")
		}
	})
}

// TestRoundTripPreChecksumStory runs the whole chain against a story with no
// checksum of its own, which is the case standard 5.5 exists for and the one
// no fixture can cover — every story that may be committed here has a checksum.
//
// The synthetic image stands in for a version 1 or 2 game. What it proves is
// that the computed checksum flows all the way through: into the IFhd this
// package writes, and back out through the identity check on the way in.
func TestRoundTripPreChecksumStory(t *testing.T) {
	image := setFileLength(storyImage(2, 7, "820516", 0, 0x80, 0x400), 0x400)

	story, err := quetzal.ParseStory(image)
	if err != nil {
		t.Fatalf("ParseStory: unexpected error: %v", err)
	}
	if !story.ChecksumComputed {
		t.Fatal("the fixture did not need a computed checksum, so nothing was tested")
	}

	current := append([]byte(nil), story.DynamicMemory...)
	current[0x70] ^= 0x11

	save := &quetzal.Save{
		Header: quetzal.Header{
			Release:  story.Release,
			Serial:   story.Serial,
			Checksum: story.Checksum,
			PC:       0x001234,
		},
		Memory: quetzal.Memory{Encoding: quetzal.MemoryCompressed, Data: current},
		Frames: []quetzal.Frame{{Evaluation: []uint16{0x0042}}},
	}

	data := writeSave(t, story, save)

	// The checksum in the file must be the computed one, since that is what
	// another interpreter would have written and will compare against.
	f, err := quetzal.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Decode: unexpected error: %v", err)
	}
	header, err := f.Header()
	if err != nil {
		t.Fatalf("Header: unexpected error: %v", err)
	}
	if header.Checksum != story.Checksum {
		t.Errorf("the written IFhd carries checksum %#04x, want the computed %#04x",
			header.Checksum, story.Checksum)
	}
	if header.Checksum == 0 {
		t.Error("the written IFhd carries a zero checksum, which is what 5.5 forbids")
	}

	got, err := quetzal.Read(bytes.NewReader(data), story)
	if err != nil {
		t.Fatalf("Read: unexpected error: %v", err)
	}
	if !bytes.Equal(got.Memory.Data, current) {
		t.Error("dynamic memory did not survive the round trip")
	}
}

// TestRoundTripRealStories writes a save for each story fixture and reads it
// back, which is the closest this package can come to the real thing without
// an interpreter: real dynamic memory, real identities, real sizes.
func TestRoundTripRealStories(t *testing.T) {
	for _, name := range storyFixtures(t) {
		t.Run(name, func(t *testing.T) {
			story := loadStory(t, name)

			// A plausible amount of change: the game has been played a
			// while, so scattered bytes differ and long stretches do not.
			current := append([]byte(nil), story.DynamicMemory...)
			for i := 0x40; i < len(current); i += 97 {
				current[i] ^= byte(i)
			}

			save := &quetzal.Save{
				Header: quetzal.Header{
					Release:  story.Release,
					Serial:   story.Serial,
					Checksum: story.Checksum,
					PC:       0x00f00d,
				},
				Memory: quetzal.Memory{Encoding: quetzal.MemoryCompressed, Data: current},
				Frames: []quetzal.Frame{
					{Evaluation: []uint16{0x0001, 0x0002}},
					{ReturnPC: 0x004321, ResultVariable: 0x10, Arguments: 0x07,
						Locals: []uint16{1, 2, 3}, Evaluation: []uint16{0xffff}},
					{ReturnPC: 0x00fedc, DiscardResult: true, Arguments: 0x01,
						Locals: make([]uint16, quetzal.MaxLocals)},
				},
				Chunks: []quetzal.Chunk{
					{ID: quetzal.IDAUTH, Data: []byte("quetzal round-trip test")},
				},
			}

			data := writeSave(t, story, save)
			if len(data) >= len(story.DynamicMemory) {
				t.Errorf("the compressed save is %d bytes for %d bytes of dynamic memory",
					len(data), len(story.DynamicMemory))
			}

			got, err := quetzal.Read(bytes.NewReader(data), story)
			if err != nil {
				t.Fatalf("Read: unexpected error: %v", err)
			}
			if !headersEqual(got.Header, save.Header) {
				t.Errorf("Header: got %+v, want %+v", got.Header, save.Header)
			}
			if !bytes.Equal(got.Memory.Data, current) {
				t.Error("Memory.Data: dynamic memory did not survive the round trip")
			}

			// The discarded result variable is the one field a round
			// trip is allowed to lose, since the format gives it no
			// meaning once the p bit is set.
			want := cloneFrames(save.Frames)
			want[2].ResultVariable = 0
			if !framesEqual(got.Frames, want) {
				t.Errorf("Frames: got %+v, want %+v", got.Frames, want)
			}

			// Writing what was read must produce the same file, which is
			// the property a save converter depends on.
			again := writeSave(t, story, got)
			if !bytes.Equal(again, data) {
				t.Error("writing a save that was just read produced a different file")
			}
		})
	}
}

// FuzzWriteRoundTrip checks that any container this package accepts can be
// written back out and decoded again unchanged. It needs no story, since it
// tests the container rather than the state it carries.
func FuzzWriteRoundTrip(f *testing.F) {
	f.Add(ifzs())
	f.Add(ifzs(chunkBytes("IFhd", ifhdPayload)))
	f.Add(ifzs(chunkBytes("ANNO", []byte("odd")), chunkBytes("AUTH", []byte("even"))))
	f.Add(ifzs(chunkBytes("CMem", []byte{0x00, 0xff, 0x01})))

	f.Fuzz(func(t *testing.T, data []byte) {
		first, err := quetzal.Decode(bytes.NewReader(data))
		if err != nil {
			return
		}

		var buf bytes.Buffer
		n, err := first.WriteTo(&buf)
		if err != nil {
			t.Fatalf("WriteTo rejected a file that decoded: %v", err)
		}
		if n != int64(buf.Len()) {
			t.Fatalf("WriteTo reported %d bytes but wrote %d", n, buf.Len())
		}

		second, err := quetzal.Decode(bytes.NewReader(buf.Bytes()))
		if err != nil {
			t.Fatalf("a written file did not decode: %v", err)
		}
		if len(second.Chunks) != len(first.Chunks) {
			t.Fatalf("got %d chunks after the round trip, want %d",
				len(second.Chunks), len(first.Chunks))
		}
		for i := range first.Chunks {
			if second.Chunks[i].ID != first.Chunks[i].ID ||
				!bytes.Equal(second.Chunks[i].Data, first.Chunks[i].Data) {
				t.Fatalf("chunk %d changed: got %s %x, want %s %x", i,
					second.Chunks[i].ID, second.Chunks[i].Data,
					first.Chunks[i].ID, first.Chunks[i].Data)
			}
		}
	})
}

// headersEqual compares two headers, since a header holds the bytes of any
// IFhd longer than this version of Quetzal defines and so cannot be compared
// directly.
func headersEqual(a, b quetzal.Header) bool {
	return a.Identity() == b.Identity() && a.PC == b.PC && bytes.Equal(a.Extra, b.Extra)
}

// framesEqual compares call stacks field by field, since frames hold slices
// and so cannot be compared directly.
func framesEqual(a, b []quetzal.Frame) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		x, y := a[i], b[i]
		if x.ReturnPC != y.ReturnPC || x.DiscardResult != y.DiscardResult ||
			x.ResultVariable != y.ResultVariable || x.Arguments != y.Arguments ||
			!wordsEqual(x.Locals, y.Locals) || !wordsEqual(x.Evaluation, y.Evaluation) {
			return false
		}
	}
	return true
}

// wordsEqual compares two word slices, treating nil and empty as the same,
// since a frame with no locals may hold either.
func wordsEqual(a, b []uint16) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// cloneFrames copies frames deeply, so that a test can hold on to what a save
// looked like before something was done to it.
func cloneFrames(frames []quetzal.Frame) []quetzal.Frame {
	out := make([]quetzal.Frame, len(frames))
	for i, f := range frames {
		f.Locals = append([]uint16(nil), f.Locals...)
		f.Evaluation = append([]uint16(nil), f.Evaluation...)
		out[i] = f
	}
	return out
}

// be32 reads a big-endian length from a file the test has written.
func be32(t *testing.T, b []byte) uint32 {
	t.Helper()
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}

// boolToInt turns a "was it kept" into the chunk count that implies.
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
