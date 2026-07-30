# Quetzal: Go Package Specification

**Status:** Draft\
**Target language:** Go\
**Package purpose:** Read, validate, manipulate, and write Quetzal
Z-machine saved-game files\
**Primary standard:** Quetzal 1.4\
**Scope:** Independent library; no dependency on a Z-machine interpreter

## 1. Purpose

This document specifies an independent Go package for the Quetzal
saved-game format used by Z-machine interpreters.

The package is intended to be usable by:

-   Z-machine interpreters written in Go.
-   Servers that persist Z-machine sessions as Quetzal files or blobs.
-   Save-file inspection and conversion tools.
-   Test utilities and interoperability tools.
-   Applications that need to read or write Quetzal without implementing a complete Z-machine.

The package MUST NOT require a running Z-machine interpreter. It SHOULD
model Quetzal as a file format rather than expose interpreter-specific
abstractions.

A central design goal is interoperability: a conforming file written by
this package should be restorable by other conforming Quetzal
implementations, and conforming Quetzal files produced by other
interpreters should be readable by this package.

## 2. Standards

The implementation MUST follow **The Quetzal Z-Machine Saved Game Standard, version 1.4**.

Quetzal is an IFF `FORM` whose form type is `IFZS`. Its required logical
contents are:

-   one `IFhd` chunk;
-   one of `CMem` or `UMem`;
-   one `Stks` chunk.

The implementation MUST follow the Quetzal rules for IFF chunk lengths,
padding, byte order, duplicate chunks, and unknown chunks.

All multi-byte integers in the Quetzal representation are big-endian.

This package is not itself a Z-machine implementation. Where
interpretation of a field depends on Z-machine semantics, the package
SHOULD expose the information faithfully and avoid unnecessarily
reproducing interpreter behavior.

## 3. Design Principles

### 3.1 Standard library first

The package SHOULD depend only on the Go standard library unless a dependency provides a substantial and clearly documented benefit.

In particular, implementing the small subset of IFF required by Quetzal is preferred to introducing a general IFF dependency.

### 3.2 `io.Reader` and `io.Writer`

The primary API MUST operate on `io.Reader` and `io.Writer`.

The core package MUST NOT require filesystem paths. Convenience functions using files MAY be provided separately, but file I/O is not part of the core abstraction.

### 3.3 No hidden story-file lookup

The library MUST NOT search the filesystem for a matching story file.

Operations requiring the original Z-machine story MUST receive the required story data explicitly from the caller.

### 3.4 Preserve information

A reader SHOULD preserve unknown chunks so that applications can inspect
them and, when appropriate, write them again.

The package MUST NOT interpret unknown chunks as errors merely because
they are unknown.

### 3.5 Strict by default

Malformed Quetzal data SHOULD produce errors rather than silently being
repaired.

Compatibility options MAY permit selected non-conforming inputs where
real-world interoperability justifies them, but such behavior MUST be
explicit.

### 3.6 Resource limits

Parsing untrusted binary files MUST NOT permit file-controlled length
fields to cause unreasonable allocations.

The API SHOULD provide configurable limits for FORM size, chunk size,
stack frames, stack words, and unknown-chunk retention.

## 4. Package Layout

The initial implementation SHOULD use one public package:

``` text
quetzal/
    doc.go
    read.go
    write.go
    chunk.go
    header.go
    memory.go
    stack.go
    story.go
    errors.go
```

Internal implementation files MAY differ.

A separate public `iff` package SHOULD NOT be introduced initially. If
the IFF implementation later proves generally useful, it may be
extracted without changing the Quetzal API.

The import path is deliberately unspecified by this document.

## 5. Public Data Model

The API SHOULD use values that correspond closely to concepts in the
Quetzal format.

A representative API is:

``` go
package quetzal

type Save struct {
    Header Header
    Memory Memory
    Frames []Frame

    Chunks []Chunk
}

type Header struct {
    Release  uint16
    Serial   [6]byte
    Checksum uint16
    PC       uint32
}

type Memory struct {
    Encoding MemoryEncoding
    Data     []byte
}

type MemoryEncoding uint8

const (
    MemoryCompressed MemoryEncoding = iota + 1
    MemoryUncompressed
)

type Frame struct {
    ReturnPC      uint32
    DiscardResult bool
    ResultVariable byte
    Arguments     uint8
    Locals        []uint16
    Evaluation    []uint16
}

type Chunk struct {
    ID   [4]byte
    Data []byte
}
```

The exact API MAY evolve during implementation, but the semantics in
this specification MUST be retained.

### 5.1 Program counters

Quetzal stores program counters as 3-byte unsigned values.

The Go API SHOULD expose them as `uint32`. Writers MUST reject values
greater than `0xFFFFFF`.

### 5.2 Serial numbers

The six-byte story serial number MUST be represented as raw bytes, not
assumed to be a date or integer.

Convenience formatting MAY expose it as a string.

### 5.3 Arguments supplied

The `Arguments` field in a stack frame represents the Quetzal
argument-supplied bit mask.

The package MUST preserve all seven defined bits.

### 5.4 Unknown chunks

Unknown chunks SHOULD retain their four-byte ID and exact payload.

The library SHOULD preserve their relative order where practical.

IFF padding bytes are structural and SHOULD NOT appear in `Chunk.Data`.

## 6. Story Information

Compressed Quetzal memory cannot be reconstructed without the original
story memory. The package therefore needs an explicit representation of
the relevant story information.

A representative type is:

``` go
type Story struct {
    Release      uint16
    Serial       [6]byte
    Checksum     uint16
    DynamicMemory []byte
}
```

A helper SHOULD construct this from a complete Z-machine story image:

``` go
func ParseStory(data []byte) (Story, error)
```

`ParseStory` SHOULD read the Z-machine header fields required by Quetzal
and determine the dynamic-memory extent from the story header.

The package MAY provide lower-level construction for callers that
already have these values.

The library MUST NOT retain or mutate the caller's story buffer unless
explicitly documented.

## 7. Reading

The primary reader SHOULD resemble:

``` go
func Read(r io.Reader, story *Story) (*Save, error)
```

A lower-level operation that parses the container without reconstructing
compressed memory SHOULD also be considered:

``` go
func Decode(r io.Reader) (*File, error)
```

This separation is useful because inspecting `IFhd`, annotations, or
chunk structure does not inherently require the story file.

### 7.1 Container validation

The reader MUST verify:

1.  the outer chunk is `FORM`;
2.  the FORM type is `IFZS`;
3.  chunk lengths remain within the FORM;
4.  odd-sized chunks consume one padding byte;
5.  required chunks are present;
6.  `IFhd` occurs before the memory and stack chunks;
7.  at least one supported memory chunk is present.

Malformed lengths or truncated data MUST return an error.

### 7.2 Duplicate chunks

IFF permits repeated chunks in general.

Where Quetzal expects only one instance, the first instance MUST be
authoritative and later instances SHOULD be ignored with a diagnostic
mechanism if diagnostics are enabled.

Multiple `ANNO` chunks are valid.

### 7.3 Unknown chunks

Unknown chunks MUST be skipped safely.

They SHOULD be retained by default, subject to configured resource
limits.

An option MAY allow callers to discard them.

## 8. Story Identification

The `IFhd` chunk identifies the story using:

-   release number;
-   six-byte serial number;
-   checksum;
-   saved program counter.

The package SHOULD expose:

``` go
func (h Header) Matches(story Story) bool
```

When a story is supplied to `Read`, its identity MUST be checked before
compressed memory is reconstructed.

A mismatch MUST return a distinguishable error.

For example:

``` go
var ErrStoryMismatch = errors.New("quetzal: story does not match save")
```

The error MAY carry the expected and actual identifiers through a typed
error.

## 9. Dynamic Memory

### 9.1 Uncompressed memory

`UMem` contains the complete dynamic-memory image.

When reading `UMem`, its payload length MUST exactly equal the expected
dynamic-memory length.

A shorter or longer `UMem` is invalid.

### 9.2 Compressed memory

`CMem` represents the XOR difference between the current dynamic memory
and the original dynamic memory.

Decoding proceeds conceptually as follows:

``` text
compressed CMem
      |
      v
expand zero runs
      |
      v
XOR difference bytes
      |
      + XOR original dynamic memory
      |
      v
restored dynamic memory
```

A non-zero encoded byte represents itself.

A zero encoded byte MUST be followed by one length byte `n`; the pair
represents `n + 1` zero bytes.

If the decoded difference stream ends before dynamic memory is
exhausted, the missing bytes MUST be treated as zero differences.

The reader MUST reject:

-   a zero byte with no following run-length byte;
-   decoded output exceeding the dynamic-memory size.

### 9.3 Compression

The writer SHOULD emit `CMem` by default.

Compression MUST:

1.  XOR current dynamic memory against original dynamic memory;
2.  encode zero runs using the Quetzal scheme;
3.  encode runs longer than 256 bytes as multiple runs;
4.  MAY omit a trailing zero-difference region.

Compression need not be globally optimal.

### 9.4 UMem writing

The writer SHOULD support an option to emit `UMem`.

For example:

``` go
type MemoryMode uint8

const (
    CompressMemory MemoryMode = iota
    StoreMemory
)
```

`CMem` SHOULD remain the default.

## 10. Stack Frames

The `Stks` chunk contains frames from oldest to newest.

Each frame contains:

``` text
3 bytes   return PC
1 byte    flags / local-variable count
1 byte    result variable
1 byte    arguments-supplied mask
2 bytes   evaluation-stack word count
2*v bytes local variables
2*n bytes evaluation stack
```

All words are big-endian.

### 10.1 Local count

The low four bits of the flags byte contain the number of local variables.

The package MUST reject a frame claiming more than 15 locals.

### 10.2 Discard-result flag

The `p` bit indicates that the routine call discards its result.

When this flag is set, the result-variable byte has no semantic meaning.

Writers SHOULD encode the result variable as zero in this case.

### 10.3 Evaluation stack

Evaluation-stack words are stored least-recent first.

The Go slice SHOULD use file order:

``` go
Evaluation[0] // least recent
Evaluation[len(Evaluation)-1] // top of stack
```

This convention MUST be documented.

### 10.4 Dummy frame

For Z-machine versions other than V6, Quetzal requires a dummy first frame representing top-level evaluation-stack state.

The low-level Quetzal representation SHOULD preserve this frame rather than silently removing it.

Higher-level interpreter integrations may translate it into their own stack representation.

## 11. Writing

The primary writer SHOULD resemble:

``` go
func Write(w io.Writer, story Story, save *Save, opts ...WriteOption) error
```

The writer MUST produce:

``` text
FORM
  IFZS
    IFhd
    CMem | UMem
    Stks
    ...
```

`IFhd` MUST precede the memory and stack chunks.

The writer MUST:

-   encode integers in big-endian order;
-   calculate all chunk lengths correctly;
-   add a zero padding byte after odd-length chunks;
-   exclude padding bytes from chunk lengths;
-   calculate the FORM length correctly;
-   reject fields that cannot be represented in Quetzal.

The writer SHOULD be capable of writing to a non-seekable `io.Writer`.

This may require buffering individual chunks or the complete FORM before
output. The implementation SHOULD avoid unnecessary duplication for
ordinary save sizes.

## 12. Optional Standard Chunks

The library SHOULD recognize the conventional text chunks:

-   `AUTH`;
-   `(c)`;
-   `ANNO`.

Their contents are printable ASCII according to the Quetzal
specification.

The API MAY expose helpers such as:

``` go
func (s *Save) Annotations() []string
func (s *Save) Author() (string, bool)
```

Raw chunks SHOULD remain available so applications do not lose
information.

The library MUST NOT depend on these chunks for restoring game state.

## 13. Interpreter-Dependent Data

The package SHOULD parse the fixed header of `IntD` while treating its
remaining data as opaque.

A representative type is:

``` go
type InterpreterData struct {
    OperatingSystem [4]byte
    Flags           byte
    ContentsID      byte
    Interpreter     [4]byte
    Data            []byte
}
```

The package MUST NOT assign semantics to interpreter-specific payloads
it does not understand.

When rewriting a file, handling of `IntD` MUST respect the Quetzal copy
restrictions represented by its flags.

A conservative default is preferable: do not blindly copy
machine-specific or state-specific data when the standard says it must
not be copied.

## 14. Validation API

Validation SHOULD be available independently of writing.

For example:

``` go
func (s *Save) Validate(story *Story) error
```

Validation SHOULD check:

-   required logical fields;
-   story identity where a story is supplied;
-   representable PC values;
-   dynamic-memory length;
-   frame local counts;
-   evaluation-stack counts;
-   argument-mask validity;
-   incompatible or contradictory memory representations.

Errors SHOULD identify the relevant chunk or frame whenever possible.

## 15. Errors

Errors MUST support `errors.Is` and, where useful, `errors.As`.

Sentinel errors SHOULD be limited to conditions callers are likely to
branch on:

``` go
var (
    ErrInvalidFormat = errors.New("quetzal: invalid format")
    ErrStoryMismatch = errors.New("quetzal: story mismatch")
    ErrTruncated     = errors.New("quetzal: truncated data")
    ErrLimitExceeded = errors.New("quetzal: resource limit exceeded")
)
```

Detailed errors SHOULD wrap these.

For example:

``` go
type ChunkError struct {
    ID     [4]byte
    Offset int64
    Err    error
}

func (e *ChunkError) Error() string
func (e *ChunkError) Unwrap() error
```

Error strings SHOULD include enough context for command-line tools and
tests without requiring callers to decode internal implementation
details.

## 16. Resource Limits

The decoder SHOULD accept configurable limits.

A representative structure is:

``` go
type Limits struct {
    MaxFormBytes        uint64
    MaxChunkBytes       uint64
    MaxUnknownBytes     uint64
    MaxFrames           int
    MaxStackWords       int
}
```

Sensible defaults MUST be provided.

A caller processing trusted historical save files SHOULD normally need no configuration.

The implementation MUST perform overflow-safe length arithmetic before allocation or slicing.

## 17. API Ownership and Mutation

Unless explicitly documented otherwise:

-   returned byte slices belong to the returned object;
-   parsing MUST NOT mutate the input story;
-   writing MUST NOT mutate `Save` or `Story`;
-   slices supplied by callers MAY be read during the call but MUST NOT be retained unexpectedly.

The implementation SHOULD favor clear ownership over zero-copy parsing.

Quetzal saves are small enough that correctness is more important than avoiding modest allocations.

## 18. Round-Trip Behavior

Two forms of round trip are important.

### 18.1 Semantic round trip

For any valid supported save:

``` text
read -> write -> read
```

MUST preserve the game state represented by:

-   story identity;
-   dynamic memory;
-   program counter;
-   stack frames;
-   local variables;
-   evaluation stacks.

Binary identity is not required because compression choices and chunk ordering may differ.

### 18.2 Structural round trip

Where unknown chunks are retained, the package SHOULD preserve their payloads exactly.

The library is not required to reproduce the original byte-for-byte file unless an explicit lossless mode is later introduced.

## 19. Interoperability Testing

Interoperability is a primary acceptance criterion.

Tests SHOULD include save files produced by established Z-machine
interpreters such as Frotz.

The project SHOULD maintain fixtures containing:

-   valid compressed saves;
-   valid uncompressed saves;
-   saves containing annotations;
-   saves containing unknown chunks;
-   saves with odd-length chunks and padding;
-   multiple stack frames;
-   long zero runs;
-   trailing omitted CMem differences.

Generated files SHOULD be restored successfully by at least one independent Quetzal implementation.

Where practical, continuous integration SHOULD test both directions:

``` text
external interpreter -> Go reader
Go writer -> external interpreter
```

External interpreter binaries SHOULD NOT be required for ordinary
`go test`; interoperability tests may be placed behind a build tag or
separate test script.

## 20. Malformed-Input Tests

The test suite MUST exercise at least:

-   invalid outer chunk;
-   incorrect FORM type;
-   truncated FORM;
-   chunk extending beyond FORM;
-   missing `IFhd`;
-   missing memory chunk;
-   missing `Stks`;
-   malformed 13-byte `IFhd`;
-   incorrect story identity;
-   malformed CMem zero run;
-   CMem expansion beyond dynamic memory;
-   incorrect UMem length;
-   truncated stack frame;
-   local count greater than 15 **on writing**;
-   stack word count exceeding available bytes;
-   24-bit PC overflow on writing;
-   odd-length chunk with missing padding;
-   aggregate payload of unknown chunks exceeding `MaxUnknownBytes`;
-   hostile length values intended to cause integer overflow or excessive allocation.

A local count greater than 15 is reachable only on writing, and the list says
so. The count occupies the low four bits of the frame flags byte, so no value a
file can store is out of range; the undefined bits around it are masked on read
rather than rejected (D1, D2). What remains testable is a caller assembling a
`Frame` with more than 15 locals in memory, which `Frame.Validate` and the
writer reject. The reading side of this item was never a check that could exist,
and treating it as one would mean pretending to a guarantee the format does not
offer: a desynchronized `Stks` stream is caught by running out of payload, not
by an implausible header.

Fuzzing SHOULD be used for the container, memory, and stack decoders.

Suitable fuzz targets include:

``` go
func FuzzDecode(f *testing.F)
func FuzzCMem(f *testing.F)
func FuzzStacks(f *testing.F)
```

A fuzz input MUST NOT cause a panic, uncontrolled allocation, or infinite loop.

## 21. Golden Tests

The project SHOULD maintain human-auditable golden fixtures.

For small synthetic files, tests SHOULD describe the intended state explicitly rather than relying solely on binary fixtures.

Example:

``` go
want := Save{
    Header: Header{
        Release:  88,
        Serial:   [6]byte{'8', '4', '0', '7', '2', '6'},
        Checksum: 0x1234,
        PC:       0x012345,
    },
    // ...
}
```

Fixture provenance MUST be documented.

Copyrighted story files MUST NOT be committed merely to support tests unless redistribution is clearly permitted.

## 22. Documentation

Package documentation MUST explain that Quetzal is a saved-state format and not a Z-machine interpreter.

The README SHOULD include examples for:

1.  inspecting a save;
2.  reading a save with its story file;
3.  modifying metadata or annotations;
4.  writing a save;
5.  validating story identity.

An example should resemble:

``` go
storyData, err := os.ReadFile("story.z3")
if err != nil {
    log.Fatal(err)
}

story, err := quetzal.ParseStory(storyData)
if err != nil {
    log.Fatal(err)
}

f, err := os.Open("save.sav")
if err != nil {
    log.Fatal(err)
}
defer f.Close()

save, err := quetzal.Read(f, &story)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("PC: %#x\n", save.Header.PC)
fmt.Printf("frames: %d\n", len(save.Frames))
```

## 23. Version Support

The package SHOULD implement Quetzal independently of any particular Z-machine version where the file format permits this.

Version-specific interpretation SHOULD be introduced only where required by Quetzal.

The initial release SHOULD aim to support Quetzal saves for Z-machine versions 1 through 8 as represented by the Quetzal 1.4 standard.

Support MUST NOT be advertised merely because fields can be parsed; interoperability tests should substantiate version claims.

## 24. Non-Goals

The package will NOT:

-   execute Z-machine instructions;
-   provide terminal or screen emulation;
-   implement Z-machine object, dictionary, or text systems;
-   locate story files automatically;
-   download story files;
-   implement Blorb;
-   implement undo history;
-   interpret arbitrary `IntD` payloads;
-   provide database persistence;
-   provide HTTP APIs;
-   manage player sessions.

These belong in callers or separate packages.

## 25. Security Considerations

Quetzal files are binary input and MUST be treated as untrusted.

The implementation MUST:

-   bounds-check every read;
-   validate lengths before allocation;
-   use overflow-safe arithmetic;
-   reject impossible frame sizes;
-   limit resource consumption;
-   never use chunk IDs or annotations as filesystem paths;
-   never perform filesystem or network access as a side effect of parsing;
-   avoid panics for malformed input.

The package MUST NOT execute or otherwise trust data found in unknown or interpreter-dependent chunks.

## 26. Compatibility Policy

The package SHOULD follow semantic versioning.

Before v1.0, the public API may evolve as interoperability experience is gained.

A v1.0 release SHOULD require:

-   complete required-chunk support;
-   CMem reading and writing;
-   UMem reading and writing;
-   stack reading and writing;
-   story identification;
-   unknown-chunk handling;
-   configurable resource limits;
-   fuzz coverage;
-   interoperability with at least one established interpreter.

After v1.0, valid files accepted by a prior minor release SHOULD not become invalid without a standards or security justification.

## 27. Suggested Initial Milestones

### Milestone 1 --- IFF container

Implement:

-   FORM/IFZS parsing;
-   chunk parsing;
-   padding;
-   unknown chunks;
-   safe length handling.

### Milestone 2 --- IFhd and story identity

Implement:

-   `IFhd`;
-   Z-machine story-header extraction;
-   story matching;
-   24-bit PC helpers.

### Milestone 3 --- memory

Implement:

-   UMem;
-   CMem decompression;
-   CMem compression;
-   round-trip tests.

### Milestone 4 --- stacks

Implement:

-   frame decoder;
-   frame encoder;
-   dummy frames;
-   validation.

### Milestone 5 --- writer

Produce complete Quetzal files and verify structural and semantic round
trips.

### Milestone 6 --- interoperability

Exchange saves with an established interpreter and add fixtures derived from those tests.

### Milestone 7 --- hardening

Add:

-   fuzz tests;
-   hostile-length tests;
-   configurable limits;
-   API/documentation review.

A v1.0 release should follow only after interoperability tests are reliable.

## 28. Acceptance Criteria

The package is ready for v1.0 when all of the following are true:

1.  It reads valid Quetzal 1.4 `FORM IFZS` files.
2.  It reads both `CMem` and `UMem`.
3.  It reconstructs compressed dynamic memory using the supplied original story.
4.  It verifies `IFhd` story identity.
5.  It reads and writes `Stks` frames correctly.
6.  It writes standards-conforming Quetzal files.
7.  Files written by the package restore in an independent established interpreter.
8.  Saves written by an independent established interpreter load correctly in the package.
9.  Unknown chunks do not break parsing.
10. Malformed input returns errors rather than panicking.
11. Resource-controlled hostile inputs cannot force unreasonable allocations.
12. `go test ./...` passes without requiring external interpreter software.
13. Public APIs and exported identifiers have Go documentation.
14. The module passes `go vet ./...`.
15. The package has no required third-party runtime dependencies unless a later design decision explicitly justifies them.

## 29. Reference

The normative reference for this package is Martin Frost's
[Z-machine Common Save-File Format Standard (Quetzal), version 1.4, 3 November 1997](https://ifarchive.org/if-archive/infocom/interpreters/specification/savefile_14.txt).

The implementation should also consult the applicable Z-machine standard
for the definition of Z-machine saved state and story-header fields.
Quetzal remains a companion saved-game format rather than a required
part of the Z-machine execution standard.
