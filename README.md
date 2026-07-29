# quetzal

A Go package for reading, validating, and writing Quetzal saved-game files for
the Z-machine.

Quetzal is the common save-file format shared by Z-machine interpreters. This
package implements that format and nothing else: it does not execute Z-machine
instructions, emulate a terminal, or implement the object, dictionary, or text
systems. It is meant for interpreters, servers that persist sessions, save-file
inspection tools, and interoperability testing.

The normative reference is Martin Frost's [Z-machine Common Save-File Format
Standard (Quetzal), version 1.4](https://ifarchive.org/if-archive/infocom/interpreters/specification/savefile_14.txt).

## Status

Early development. The API may change until v1.0.

Implemented so far:

- The IFF container: `FORM`/`IFZS` parsing, chunk parsing, padding, retention of
  unknown chunks, and configurable resource limits.
- Story identification: the `IFhd` chunk, extraction of the Z-machine header
  fields Quetzal depends on, and story matching.
- Dynamic memory: the `CMem` and `UMem` chunks, compression and decompression of
  the `CMem` difference stream, and reconstruction of a save's dynamic memory
  against its story.
- Stack frames: the `Stks` chunk, frame decoding and encoding, the dummy frame
  that versions other than 6 require, and frame validation.
- Reading and writing whole saves: `Read`, `Write`, and `Validate`, the copy
  restrictions on interpreter-dependent `IntD` data, and semantic round trips.

Still to come: interoperability testing against an established interpreter,
which is what will decide whether this is finished.

**Version coverage.** The package implements Z-machine versions 1 through 8, but
is exercised only against version 3 stories, which are the only images whose
redistribution permits committing them as fixtures. Versions 1, 2, and 6 are
implemented and untested rather than unsupported — including the computed
checksum that stories predating the checksum field require. See D43 in
[spec-deltas.md](spec-deltas.md).

## Install

```sh
go get github.com/maloquacious/quetzal
```

Requires Go 1.26 or later. The package has no third-party dependencies.

## Usage

### Inspect a save

`Decode` parses a save's structure. It needs no story file, so it is the right
entry point for tools that only want to look at what a save contains.

```go
f, err := os.Open("save.sav")
if err != nil {
	log.Fatal(err)
}
defer f.Close()

save, err := quetzal.Decode(f)
if err != nil {
	log.Fatal(err)
}

header, err := save.Header()
if err != nil {
	log.Fatal(err)
}

fmt.Printf("story: %s\n", header.Identity())
fmt.Printf("PC:    %#x\n", header.PC)

for _, chunk := range save.Chunks {
	fmt.Printf("chunk %s, %d bytes\n", chunk.ID, len(chunk.Data))
}
```

### Check that a save belongs to a story

A save records the release number, serial number, and checksum of the story it
came from. `Verify` compares them and reports how they differ; `Matches` answers
the same question as a bool.

Stories written before the Z-machine header carried a checksum hold zero in that
field, and the format requires an interpreter to compute the value from the story
image instead. `ParseStory` does so, and sets `Story.ChecksumComputed` to say
that it did.

```go
image, err := os.ReadFile("zork1.z3")
if err != nil {
	log.Fatal(err)
}

story, err := quetzal.ParseStory(image)
if err != nil {
	log.Fatal(err)
}

if err := header.Verify(story); err != nil {
	log.Fatal(err) // wraps quetzal.ErrStoryMismatch
}
```

### Rebuild the saved dynamic memory

Most saves store dynamic memory as a difference against the story it came from,
so rebuilding it needs the story. `Memory` checks that the save and the story
agree before it does so, since a difference applied to the wrong story would
produce plausible nonsense rather than an error.

```go
mem, err := save.Memory(story)
if err != nil {
	log.Fatal(err)
}

fmt.Printf("%d bytes of dynamic memory, saved as %s\n", len(mem.Data), mem.Encoding)
```

### Walk the call stack

Frames run from the oldest to the newest, so the last one is the call that was
executing when the game was saved. Evaluation stacks use the same order:
`Evaluation[0]` is the least recent word and the last element is the top of the
stack.

```go
frames, err := save.Frames()
if err != nil {
	log.Fatal(err)
}

for i, frame := range frames {
	if frame.IsDummy() {
		fmt.Printf("frame %d: top-level, %d words on the stack\n", i, len(frame.Evaluation))
		continue
	}
	fmt.Printf("frame %d: returns to %#x, %d local(s), %d word(s) on the stack\n",
		i, frame.ReturnPC, len(frame.Locals), len(frame.Evaluation))
}
```

### Read and write whole saves

The examples above take a save apart a piece at a time. `Read` does all of it at
once and checks the result, which is what an interpreter restoring a game wants.
`Write` is its inverse.

```go
save, err := quetzal.Read(f, story)
if err != nil {
	log.Fatal(err)
}

// Resume the game from save.Memory.Data, save.Header.PC, and save.Frames,
// then save it again later.
out, err := os.Create("save.qzl")
if err != nil {
	log.Fatal(err)
}
defer out.Close()

if err := quetzal.Write(out, story, save); err != nil {
	log.Fatal(err)
}
```

Reading and writing round trip semantically, not byte for byte: the story
identity, dynamic memory, program counter, and stack frames all survive, but the
bytes need not, since the format leaves the choice of encoding open. Pass
`quetzal.WithEncoding(quetzal.MemoryUncompressed)` to write dynamic memory in
full rather than as a difference.

`Save.Validate(story)` runs the same checks without producing a file, for a
caller assembling a save of its own.

Errors wrap the sentinels `ErrInvalidFormat`, `ErrStoryMismatch`, `ErrTruncated`,
and `ErrLimitExceeded`, so test them with `errors.Is`. Container problems are
reported as a `*ChunkError` naming the chunk and its offset, and stack problems
as a `*FrameError` naming the frame.

## Design

- **Standard library only.** No third-party runtime dependencies.
- **Streams, not paths.** The core API works on `io.Reader` and `io.Writer`.
- **No hidden story lookup.** Compressed memory cannot be rebuilt without the
  original story, and this package never goes looking for one. Callers pass
  story data in explicitly.
- **Untrusted input.** Every length in a save file comes from the file itself.
  Reads are bounds-checked, arithmetic is overflow-safe, allocations are limited,
  and malformed input returns an error instead of panicking.
- **Information is preserved.** Unknown chunks are kept, in order, rather than
  treated as errors.

## Testing

```sh
go test ./...
```

The test suite runs without any external interpreter installed. Fuzz targets are
included:

```sh
go test -run XXX -fuzz FuzzDecode -fuzztime 30s ./...
```

## License

This package is released under the MIT License. See [LICENSE](LICENSE).

### Third-party material

Some files in this repository are not part of the package and are not covered by
the package's license.

**The story files under `testdata/stories/` are third-party works.** They are
included only as test fixtures, so that the package can be exercised against real
Version 3 Z-machine story files. Their presence here does not make them part of
this package, does not place them under this package's license, and grants you no
rights to them. Each story file carries its own license file alongside it, and
that license is what governs your use of it. See
[testdata/README.md](testdata/README.md) for the source and license of each file.

The Quetzal specification itself is the work of Martin Frost and is not
redistributed here. See [references/README.txt](references/README.txt).
