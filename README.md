# quetzal

[![Go Reference](https://pkg.go.dev/badge/github.com/maloquacious/quetzal.svg)](https://pkg.go.dev/github.com/maloquacious/quetzal)

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

Feature complete for v1.0, and the API is not expected to change further.
[specification.md](specification.md) is accepted rather than draft: every
deliberate departure from Quetzal 1.4 is enumerated in its §2.1, every accepted
limitation in its §30, and [spec-deltas.md](spec-deltas.md) carries the reasoning
and the interoperability evidence behind each one.

Implemented:

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
- The text chunks a save may carry: `Annotations`, `Author`, and `Copyright`.

Interoperability is done. Saves written by Frotz 2.55, Bocfel 2.5,
and jzip 2.1 all load here, and all three restore files this package writes.
Saving one game position in all three produces dynamic memory that agrees to the
byte outside the Z-machine header, where each interpreter records its own
capabilities. `ckifzs`, the conformance checker
distributed with the standard, reports every file this package writes as
conforming. The test suite reads the committed fixtures
and needs no interpreter installed.

**Version coverage.** The package implements Z-machine versions 1 through 8. Its
committed fixtures are all version 3, because those are the only stories whose
redistribution permits shipping them, but versions 5 and 6 are exercised against
stories a maintainer can fetch — including a version 6 save, which is the only
kind that carries no dummy frame. Versions 1 and 2 remain untested by any real
file: what they need is a story with no checksum in its header, and no copy of
one could be found. The code computes such a checksum as the format requires;
nothing has confirmed it against a story that needs it. See D43 and D27 in
[spec-deltas.md](spec-deltas.md).

## Install

```sh
go get github.com/maloquacious/quetzal
```

Requires Go 1.26 or later. The package has no third-party dependencies.

## Documentation

New to Quetzal, or to this package? Start with
**[the tutorial](tutorial.md)** — it takes one real Frotz save of Zork I,
takes it apart until you can say where the player was standing and what the game
was in the middle of doing, and writes it back out. Fifteen minutes, and it uses
fixtures committed here, so there is nothing to go and find first.

Everything else assumes you have done that or already know the format:

| | |
|---|---|
| [Tutorial](tutorial.md) | A lesson. Read your first save, start to finish. |
| [Usage](#usage), below | How-to guides for specific goals. |
| [pkg.go.dev](https://pkg.go.dev/github.com/maloquacious/quetzal) | The API reference. Also `go doc github.com/maloquacious/quetzal`. |
| [specification.md](specification.md) | What this package promises, and its accepted limitations. |
| [spec-deltas.md](spec-deltas.md) | Every deliberate departure from Quetzal 1.4, argued out. |
| [CHANGELOG.md](CHANGELOG.md) | What changed, by release. |

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

### Read the text a save carries

A save may carry an annotation, the name of whoever wrote the file, and a
copyright notice. Interpreters use them for whatever they see fit — Bocfel
records its own name and version — and nothing about restoring a game depends on
them, so a save with none of these is in no way deficient.

```go
for _, note := range save.Annotations() {
	fmt.Printf("annotation: %s\n", note)
}
if author, ok := save.Author(); ok {
	fmt.Printf("saved by:   %s\n", author)
}
```

The same three methods are available on a decoded `File`, so a tool inspecting a
save need not have the story it belongs to. The text comes back exactly as it was
stored: the format says it holds printable ASCII, but a file that breaks that rule
still carries what its writer meant, and treating the bytes as untrusted is left
to whoever displays them.

### Change what a save records about itself

There is no setter. Annotations and the rest live in `Save.Chunks` alongside
everything else the file carried, and editing that slice is how they change —
which keeps one rule visible: a chunk this package does not understand is
carried along untouched, so rewriting a save does not quietly discard what
another interpreter put in it.

```go
// Drop any annotation already present and record our own instead.
var chunks []quetzal.Chunk
for _, c := range save.Chunks {
	if c.ID != quetzal.IDANNO {
		chunks = append(chunks, c)
	}
}
save.Chunks = append(chunks, quetzal.Chunk{
	ID:   quetzal.IDANNO,
	Data: []byte("score 25, 140 moves"),
})

if err := quetzal.Write(out, story, save); err != nil {
	log.Fatal(err)
}
```

Two things the writer will not let you get away with. An `IFhd`, `CMem`, `UMem`,
or `Stks` chunk in `Save.Chunks` is rejected rather than written, since it would
contradict the fields that already describe those; and an `IntD` chunk whose
flags forbid copying is left out when a save is read, so it cannot be carried
into a file it does not belong to.

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

Reading is strict by default. The one exception a caller can ask for is
`quetzal.IgnoreChunkOrder()`, which accepts a save whose `IFhd` chunk does not
come first — the format requires that order, but not every interpreter enforces
it, and a file the rest of the world reads should not be unreadable here.

Errors wrap the sentinels `ErrInvalidFormat`, `ErrStoryMismatch`, `ErrTruncated`,
and `ErrLimitExceeded`, so test them with `errors.Is`. Container problems are
reported as a `*ChunkError` naming the chunk and its offset, and stack problems
as a `*FrameError` naming the frame.

### Compare two saves

`Compare` reports how two saves differ, which is what a test wants when the
question is whether this package and some other interpreter agree about a
position. It needs no story of its own, returns no error, and reports nothing at
all when the two agree.

```go
ours := readSave("ours.qzl", story)
theirs := readSave("dfrotz.qzl", story)

for _, d := range quetzal.Compare(ours, theirs) {
	fmt.Println(d)
}
```

Most of what that prints between two different interpreters is uninteresting,
and one part of it is actively misleading. The whole Z-machine header lives
inside dynamic memory, so a save carries the screen size, font size, interpreter
number, default colours, and claimed standard revision of whoever wrote it —
fields the interpreter fills in for itself and that say nothing about the saved
game. Options disregard those, and the chunks each interpreter writes for itself:

```go
diffs := quetzal.Compare(ours, theirs,
	quetzal.IgnoreInterpreterHeader(),
	quetzal.IgnoreMemoryEncoding(),
	quetzal.IgnoreChunks(quetzal.IDANNO, quetzal.IDIntD),
)
if len(diffs) != 0 {
	log.Fatalf("the two interpreters disagree: %v", diffs)
}
```

`IgnoreMemoryRange(start, end)` handles anything game-specific left over. Every
option is named for what it disregards, and none of them can turn agreement into
a difference, so a comparison with no options is exact.

`CompareFiles` asks the other question — whether two files *say* it the same way —
and compares containers chunk by chunk, in order, without interpreting anything.

Comparison is a testing and debugging facility rather than part of the format,
so `specification.md` deliberately says nothing about it. Its doc comments and
[issue 1](https://github.com/maloquacious/quetzal/issues/1) are where it is
specified; see `specification.md` §5.7 for why.

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
  treated as errors — up to a configurable total, since a chunk nothing
  understands is the one part of a file whose size nothing but the file itself
  constrains.

## Testing

```sh
go test ./...
```

The test suite runs without any external interpreter installed. Fuzz targets are
included:

```sh
go test -run XXX -fuzz FuzzDecode -fuzztime 30s ./...
```

Every Go snippet above is compiled. They live in `example_test.go` as example
functions, which the compiler checks and the test runner does not execute, and a
test asserts that the README and that file still agree — so a snippet cannot
quietly rot into something that no longer builds.

[The tutorial](tutorial.md) is held to more than that, because a reader who is
still learning cannot tell a stale lesson from their own mistake.
`tutorial_test.go` runs the program the tutorial builds against the same
committed fixtures the reader is told to copy, and asserts every number it
prints — the program counter, the frame count, both file sizes, and the single
difference `Compare` reports at the end.

The same suite checks that the two design documents stay in step: that every
entry in `spec-deltas.md` records what was done with it, and that every departure
from the standard reaches `specification.md` §2.1.

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
