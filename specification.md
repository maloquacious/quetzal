# Quetzal: Go Package Specification

**Status:** Accepted for v1.0, 2026-07-29\
**Describes:** package behavior as of commit `56dc353`\
**Amendment:** §31\
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

The implementation MUST follow **The Quetzal Z-Machine Saved Game Standard, version 1.4**, except where §2.1 says otherwise.

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

### 2.1 Conformance and deliberate divergence

The requirement above is qualified by this section, and by nothing else.

Quetzal 1.4 leaves some questions open, answers others in ways that no
implementation follows, and describes error cases without saying what to do
about them. An implementation therefore cannot be conforming and silent at the
same time: it either enumerates where it departs from the standard, or it makes
a claim its own behavior contradicts. What follows is that enumeration.

Each row states the rule this package applies, and carries the identifier of the
`spec-deltas.md` entry that holds the reasoning and the interoperability
evidence behind it. The rules are normative here; the reasoning is not repeated.

**Stricter than the standard requires.** Every row is a path on which this
package rejects a file some other implementation may accept.

| Rule | Entry |
|---|---|
| A file holding both a `CMem` and a `UMem` chunk MUST be rejected rather than one of them chosen. Standard 7.18 stores memory one way or the other and gives no rule for reconciling two statements of the same state. | D3 |
| Reconstructing dynamic memory MUST require an `IFhd`, and MUST verify story identity before a `CMem` difference is applied. | D4 |
| A `CMem` stream that expands past the end of dynamic memory MUST be an error, as MUST a zero byte with no run-length byte after it. Standard 3.5 leaves the handling to the implementation. | D5 |
| A `UMem` payload whose length differs from the story's dynamic memory MUST be rejected, in either direction. | D6 |
| A chunk identifier MUST be four printable ASCII characters. A non-printable identifier is the earliest available signal that the chunk stream has desynchronized. | D7 |
| A story image MUST be rejected if its version lies outside 1 through 8, or if its static-memory base lies inside the 64-byte header or past the end of the image. | D8 |
| For every Z-machine version but 6, a save whose first frame is not the dummy frame MUST be rejected on validation. Standard 4.11.2 requires the frame and permits interpreters to assume its presence. | D9, D33 |
| `IFhd` MUST precede the memory and stack chunks, as standard 5.4 requires, rather than a mis-ordered file being tolerated. This is the one rule a caller may explicitly overlook (§3.5). | D32 |

**More lenient than a literal reading.** Every row accepts a file a stricter
reader might reject. None can cause a valid file to be refused.

| Rule | Entry |
|---|---|
| Undefined bits — the top three of a frame's flags byte, and the eighth bit of its arguments mask — MUST be masked away on reading rather than rejected. A bit with no defined meaning is not grounds for losing a save. | D1, D2 |
| Standard 8.3.3's requirement that any spaces in a chunk identifier be trailing is NOT enforced. It distinguishes no real file from any other. | D10 |
| The pad byte following an odd-length chunk MUST be present, because chunk lengths keep the stream aligned, but MAY hold any value. | D11 |
| An `IFhd` payload longer than 13 bytes MUST be accepted and its remainder preserved. Standard 5.5 reserves the right to extend the chunk while keeping the meaning of the first 13 bytes. | D12 |
| Bytes following the outer FORM MUST be ignored rather than treated as trailing garbage. A simple IFF file is a single FORM chunk, and real interpreters emit such bytes. | D13 |
| An empty `FORM IFZS`, and an empty `Stks` chunk, MUST decode without error. Whether the required chunks are present, and whether zero frames is legal, are judged by the layers that have the story (§7.1). | D14, D15 |
| An `IntD` chunk naming neither an operating system nor an interpreter MUST be accepted, though standard 7.14 forbids writing one. Refusing would fail a restore over an optional chunk whose payload is never read. | D35 |

**Choices where the standard permits latitude.** These decide what this package
emits, and are therefore what another implementation judges.

| Rule | Entry |
|---|---|
| A discarded result variable is preserved on reading and written as zero, per standard 4.6. The round trip is therefore semantic rather than byte-exact (§18.1). | D16 |
| A `CMem` payload ends at the last changed byte, omitting the trailing zero-difference region, which standard 3.4 permits. | D17 |
| A zero run longer than 256 bytes is written as consecutive maximum-length runs, since one length byte cannot describe more (standard 3.3). | D18 |
| Preserved `IFhd` bytes beyond the first 13 are written back out, so a save read from an over-long `IFhd` reproduces it. | D19 |
| Compression is single-pass and need not be globally optimal, per standard 3.3. | D20 |
| An `IntD` chunk whose flags forbid copying is not carried from a file into a save, per standard 7.10 and 7.11. A caller may still take it from the decoded container deliberately (§13). | D34 |

This set is closed. Introducing a new divergence requires an entry in
`spec-deltas.md`, a row here, and therefore an amendment to this document under
§31.

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

The Go declarations in this section and throughout this document are
**illustrative, not normative**. Several of them differ from what the package
exports. §5.5 says where the exported surface is defined instead, and why.

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

The semantics in this specification MUST be retained. The exact API is defined
elsewhere; see §5.5.

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

### 5.5 Authority over the exported API

This document is normative for the **semantics** of the format and of the
package's behavior. It is not normative for the exact set, shape, or naming of
exported Go identifiers.

The authoritative description of the exported surface is the package's own
documentation, as rendered by:

``` sh
go doc -all .
```

Two reasons, and the second is the operative one. The first is ordinary: a
specification written before the code cannot state a Go API as precisely as the
declarations themselves, and where the two disagree the declarations are what
callers compile against. The second is that doc comments are read at the point
of use. A caller learns that `Header.Checksum` may not be the value at `$1C` of
their own story image while looking at `Header.Checksum`, which is where that
trap is survivable; the same sentence in §8 of a design document is a sentence
nobody reads in time.

The practical consequences:

-   A change to the exported API requires no amendment to this document,
    provided the semantics stated here are retained. Semantic changes require
    an amendment (§31), and a change that makes a previously valid file invalid
    additionally requires the justification §26 demands.
-   Where an illustrative declaration here conflicts with the package, the
    package is correct and this document is merely out of date. Such conflicts
    are not deltas and MUST NOT be recorded as such.
-   The obligations in §22 therefore carry the weight this section removes: the
    doc comments are the specification of the API, so every exported identifier
    MUST be documented, and the naming rules the package follows MUST be stated
    in the package documentation rather than left to be inferred.

For the record, the exported surface adds the following to what the
illustrations in §5, §6, and §13 show. Each is described where it is declared.

| Addition | Why | Entry |
|---|---|---|
| `Identity`, `Serial`, `StoryMismatchError` | Story matching is one comparison, and a mismatch can name both sides. | D24 |
| `Header.Extra` | Preserves `IFhd` bytes beyond the first 13. | D12, D19 |
| `FrameError` | §15 names `ChunkError` only, but asks that errors name the offending chunk *or frame*. Carries a frame index, which is what a caller can act on. | D24 |
| `Story.Version` | The dummy-frame rule is version-dependent, so validation needs it. | D23 |
| `Story.ChecksumComputed` | A `Checksum` that is not the value in the file is surprising, and worth logging (§6). | D27 |
| `MaxPC`, `MaxLocals`, `MaxEvaluationWords`, `MinVersion`, `MaxVersion` | Callers can check a value before building a frame or a save. | D24 |
| `Save.Encode`, `File.Save`, `File.WriteTo` | The halves of `Read` and `Write`, exposed for the reason `Decode` is: a caller may want the container without the state, or the state without the bytes. | D24 |

### 5.6 Additional chunks carried by a save

A save's interpreted fields and its retained chunks MUST NOT describe the same
chunk. `IFhd`, `CMem`, `UMem`, and `Stks` are represented by the interpreted
fields, and a save MUST NOT also carry one of them as an additional chunk:
writing both would produce a file contradicting itself, with no rule for which
copy wins. Validation MUST reject such a save.

Two consequences follow on the reading side:

-   A duplicate of a single-instance chunk is not carried forward. Standard 7.2
    makes the first instance authoritative, and retaining an ignored `IFhd`
    would only cause the writer to emit a file with two of them. The container
    layer still retains every chunk it read (§7.2).
-   Multiple `ANNO` chunks are unaffected, since they are legal in quantity.
    All of them are retained, in order.

*Entry:* D40.

### 5.7 Facilities for testing and debugging

The package MAY export facilities that serve the people testing it, and the
people testing its callers, rather than the format. Comparing two saves and
reporting how they differ is the first of them.

Such a facility is **outside the scope of this document**, and this is a
deliberate limit rather than an omission. Quetzal 1.4 defines a file format; it
says nothing about comparing two files, and a section here stating rules for
comparison would be this document legislating about something the standard it
implements does not describe. Every rule in the sections that follow can be
traced to the standard or to a security or interoperability justification. A
comparison facility has no such ancestry, and inventing one for it would make it
harder, not easier, to tell which of this document's requirements a caller can
rely on other implementations to honor.

Such a facility is therefore specified by:

-   its **doc comments**, which are authoritative for it in the same way and for
    the same reasons §5.5 makes them authoritative for the exported API
    generally; and
-   a **GitHub issue**, which records the motivation, the design decisions taken,
    and the alternatives rejected — the material this document would carry for a
    format feature.

Four requirements bind these facilities, and they are what keep the exclusion
from becoming a loophole:

1.  Such a facility MUST NOT change the semantics this document states. It
    observes values; it does not decide what a valid file is. A change to reading
    or writing made in order to serve one is an amendment like any other (§31).
2.  Such a facility MUST NOT introduce a divergence from Quetzal 1.4, and so
    requires no entry in `spec-deltas.md` and no row in §2.1. A facility that
    appears to need one is doing something other than observing.
3.  Such a facility MUST hold to the obligations that are the package's rather
    than the format's: the ownership and mutation rules of §17, the prohibition
    on panicking (§25), and the testing standards of §28. The resource limits of
    §16 do not apply, because §16.2 bounds reading and these facilities read
    nothing.
4.  Its issue MUST remain reachable. An issue is closed when the work is done,
    not deleted, and the doc comments name it.

The reader wanting to know what this package's comparison facility does should
run `go doc -all .` and read the issue it names. This document will not answer
the question, and does not intend to.

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

The story's Z-machine version MUST also be represented, because two rules
depend on it: whether a save carries the dummy frame (§10.4), and how a
story's declared length is scaled when a checksum has to be computed (§6.1).
The illustration above omits it (D23).

The package MAY provide lower-level construction for callers that
already have these values.

The library MUST NOT retain or mutate the caller's story buffer unless
explicitly documented.

### 6.1 Stories with no stored checksum

Games written before the Z-machine header carried a checksum hold zero at
offset `$1C`. Standard 5.5 requires the saving interpreter to "calculate it in
the normal way from the original story file" in that case, so that the identity
recorded in `IFhd` is the one other interpreters agree with.

`ParseStory` MUST perform that calculation when, and only when, `$1C` holds
zero, and MUST report that it did so, since a `Checksum` that is not the value
in the caller's story image is otherwise a silent surprise.

The calculation is the sum of every byte from `$40` to the story's declared end,
modulo `0x10000`, where the declared length is the word at `$1A` scaled by 2 for
versions 1 through 3, 4 for versions 4 and 5, and 8 for versions 6 through 8.
The **declared** length is used rather than the size of the image, because a
story file may carry padding past its end.

Three rules constrain this:

-   A stored checksum MUST NOT be recomputed or second-guessed. If `$1C`
    disagrees with what the image sums to, the stored value wins: interpreters
    compare the stored value and therefore agree with one another, and
    substituting different arithmetic would break the matching this exists to
    serve. Only a zero triggers computation.
-   A story carrying neither a checksum nor a usable length at `$1A` MUST keep
    its zero. Some of the same early games leave that field unused, and there is
    then no "normal way" to calculate anything, since the Z-machine's own
    definition of the file length is the field that is missing. Nothing is to be
    invented, and a caller MUST be able to distinguish this case from a
    successful computation.
-   The calculation SHOULD be available on its own, for a caller that wants the
    value without parsing a story.

*Entry:* D27, whose remaining limitation is recorded in §30.

## 7. Reading

The primary reader SHOULD resemble:

``` go
func Read(r io.Reader, story Story, opts ...ReadOption) (*Save, error)
```

A lower-level operation that parses the container without reconstructing
compressed memory SHOULD also be considered:

``` go
func Decode(r io.Reader, opts ...ReadOption) (*File, error)
```

This separation is useful because inspecting `IFhd`, annotations, or
chunk structure does not inherently require the story file.

The story MUST be a required parameter of the reader rather than an optional
one. A pointer would make a nil story meaningful — *reconstruct what you can
without one* — and that case is already served, and served better, by the
container operation, which needs no story precisely because it reconstructs
nothing. Reading cannot do its job without a story: compressed memory is a
difference against one, and the Z-machine version decides what the call stack
must contain (D39).

### 7.1 Container validation

Reading MUST verify:

1.  the outer chunk is `FORM`;
2.  the FORM type is `IFZS`;
3.  chunk lengths remain within the FORM;
4.  odd-sized chunks consume one padding byte;
5.  required chunks are present;
6.  `IFhd` occurs before the memory and stack chunks;
7.  at least one supported memory chunk is present.

Malformed lengths or truncated data MUST return an error.

These checks belong to two different layers, and the split is normative rather
than incidental. Items 1 through 4 are properties of the IFF container and MUST
be verified by the container operation. Items 5 through 7 are Quetzal's
requirements about which chunks a save holds, and MUST be verified by the
operation that reconstructs saved state; the container operation MUST NOT
enforce them, because it reports what a file contains without judging whether
the file is a restorable save (D14, D15).

Item 6 follows items 5 and 7 rather than preceding them: an ordering rule is
meaningful only relative to chunks whose presence is required, so it belongs
with the layer that requires them (D32). A caller that wants a mis-ordered file
anyway can decode the container and select chunks by identifier.

### 7.2 Duplicate chunks

IFF permits repeated chunks in general.

Where Quetzal expects only one instance, the first instance MUST be
authoritative and later instances MUST be ignored — meaning that nothing is
decoded from them, not that they are discarded.

That distinction is the whole of the diagnostic requirement. The container
operation MUST retain every chunk it read, duplicates included, and MUST offer a
way to enumerate the chunks bearing a given identifier. A caller that wants to
report a duplicate — a save-file inspector, or a server logging what it was
handed — asks the container, and receives two chunks where Quetzal allows one.
No separate diagnostics facility is required, and none SHOULD be added: it would
introduce API surface, an ordering question about whether diagnostics precede
the error that abandons a decode, and a second way to learn something the data
model already reports.

Two limits on this, so it is not read as broader than it is. A save does not
carry duplicates (§5.6), so a caller wanting them must ask the container rather
than the reader. And a duplicate is distinguishable from a legitimately repeated
chunk only by knowing which identifiers Quetzal allows one of; the package need
not expose that judgment.

Multiple `ANNO` chunks are valid.

*Entry:* D28.

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

### 9.2.1 What the data model holds

A save's dynamic memory MUST be presented as dynamic memory — the expanded,
reconstructed bytes, whose length always equals the length of the story's
dynamic memory — and never as the payload that encoded it. A `CMem` payload is
expanded against the story before it reaches the caller.

The encoding MUST be recorded alongside it, so that a file can be rewritten the
way it arrived. It is descriptive when read and directive when written (§9.4).

This is why §18.1 states the round trip over dynamic memory rather than over the
payload: the payload is one of several valid encodings of the same state, and the
state is the thing that must survive. *Entry:* D21.

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

One enumeration MUST serve both roles: the encoding a save was read from and the
encoding it is to be written in are the same distinction, and two parallel types
for it would be redundant. The illustration below, which sketched a separate
writer-side `MemoryMode`, is superseded (D22).

``` go
type MemoryEncoding uint8

const (
    MemoryCompressed MemoryEncoding = iota + 1
    MemoryUncompressed
)
```

`CMem` SHOULD remain the encoding a writer produces when a caller expresses no
preference, per standard 3.3's recommendation.

An encoding that was never set MUST be an error rather than silently taking that
default. Defaulting would always produce a valid file, so this is strictness for
its own sake — but memory whose encoding was never chosen is more likely a
half-built value than a request for the default, and the write option makes
saying so a single call. Note that the enumeration therefore starts at one
rather than at zero, so that the unset value is distinguishable (D37).

A write option MUST be able to override the encoding a save carries, so that a
caller can convert between the two without disturbing the save.

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

The package MUST reject a frame claiming more than 15 locals **when writing**.
The check cannot exist on the reading side: the count occupies four bits, so no
value a file can store is out of range, and the bits around it are masked rather
than rejected (§2.1, D1). What remains is a caller assembling a frame in memory
with more locals than the format can express, which validation and the writer
MUST refuse. §20 states the same thing about the corresponding test.

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

The last of these includes the container itself: a FORM whose length exceeds
what its four-byte length field can describe MUST be rejected rather than
truncated. Dynamic memory tops out at 64 KB, so reaching this needs deliberate
effort, but a size that cannot describe itself is exactly the class of field
this rule exists for (D38).

As with reading, the story MUST be a required parameter (§7, D39).

**Ordering of additional chunks.** A save's remaining chunks MUST be written
after the three that Quetzal requires, in the relative order the save holds them
(§5.4). Their position relative to the required three is not preserved: a file
read with an annotation *before* its `IFhd` is written back with the annotation
last. Standard 5.4 fixes only that `IFhd` comes first, and §18.2 does not promise
a byte-identical rewrite (D36).

A save read from a file whose chunks were mis-ordered — which requires the
caller to have overlooked that rule explicitly (§3.5) — MUST still be written in
the required order. Accepting a mis-ordered file does not mean producing one.

The writer SHOULD be capable of writing to a non-seekable `io.Writer`.

This may require buffering individual chunks or the complete FORM before
output. The implementation SHOULD avoid unnecessary duplication for
ordinary save sizes.

Nothing SHOULD be written until the whole save has been checked and encoded, so
that a rejected save leaves the writer untouched.

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

Four rules govern them, none of which the illustration shows (D29):

-   **A helper MUST exist for all three chunks**, `(c) ` included. Leaving it out
    would mean a caller assembling the identifier by hand, and `(c) ` is three
    characters and a trailing space — get the space wrong and the chunk is
    silently absent rather than malformed. That is exactly the mistake a helper
    should absorb.
-   **The helpers MUST be available on the decoded container as well as on a
    save.** Inspecting a save without its story is a first-class use (§7), and an
    annotation is the single most inspectable thing a save contains, so requiring
    a story image to read one would be backwards.
-   **The text MUST be returned exactly as stored**, control bytes and all.
    Standard 7.2 says these chunks hold characters in `0x20`–`0x7E` and nothing
    else; that rule is not enforced here, because a chunk breaking it still
    carries the text its writer meant, and dropping or rewriting it would discard
    information over a defect that harms nobody at this layer (§3.4). The
    documentation MUST tell callers that they are displaying bytes someone else
    chose.
-   **`AUTH` and `(c) ` return the first instance**, per standard 7.3 and 7.4 and
    the general first-instance rule (§7.2), while `ANNO` returns all of them,
    since multiple annotations are several separate remarks rather than one split
    across chunks.

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

The two reserved bytes that the format places between the contents identifier
and the interpreter identifier need not be represented: standard 7.8.6 fixes
them at zero, so they carry nothing (D41).

The package MUST NOT assign semantics to interpreter-specific payloads
it does not understand.

For the same reason there need be no encoder counterpart. The payload is opaque,
and a caller that wants to write an `IntD` builds the chunk itself, which is a
dozen bytes of appending. Parsing exists because the writer has to read the flags
in order to honor the copy restrictions, not because the package understands what
it is parsing.

When rewriting a file, handling of `IntD` MUST respect the Quetzal copy
restrictions represented by its flags.

A conservative default is preferable: do not blindly copy
machine-specific or state-specific data when the standard says it must
not be copied.

Three kinds of `IntD` MUST therefore be left out when a file is read into a
save (D34):

-   position-specific contents, which standard 7.11 forbids copying outright;
-   machine-specific contents, because this package has no notion of what system
    it is running on and so cannot distinguish standard 7.10's permitted case —
    the same machine — from the forbidden one. The conservative reading is the
    only one available to it;
-   a payload shorter than the fixed header, since a chunk that cannot state its
    restrictions cannot be shown to be free of them.

The restriction is on the **copy**, which is the load-then-save path, so the drop
belongs to reading rather than to writing. A caller that builds its own `IntD`
and puts it in a save MUST get it written, flags and all; the writer imposes
nothing. And nothing is lost outright: the container layer retains every chunk,
so a caller that does know its own machine can take the chunk from the decoded
file and carry it forward deliberately.

## 14. Validation API

Validation SHOULD be available independently of writing, so that a caller can
find out whether a save it has assembled is sound without producing a file. The
writer MUST perform the same checks.

For example:

``` go
func (s *Save) Validate(story Story) error
```

Validation SHOULD check:

-   required logical fields;
-   story identity;
-   representable PC values;
-   dynamic-memory length;
-   frame local counts;
-   evaluation-stack counts;
-   argument-mask validity;
-   incompatible or contradictory memory representations;
-   the dummy frame that versions other than 6 require (§10.4);
-   the additional chunks a save carries (§5.6).

The story is required here as it is for reading and writing (§7, D39), so the
identity check is not conditional. Validation of a value smaller than a whole
save — a single frame, or a header on its own — necessarily checks less, and the
documentation MUST say what each one covers rather than leaving a caller to
assume that any `Validate` checks everything.

Errors SHOULD identify the relevant chunk or frame whenever possible.

Reading MUST finish by validating the save it reconstructed, so that a save
obtained by reading is one that could be written straight back out. Everything
validation checks is already guaranteed by decoding except the dummy-frame
requirement, so in practice this is the one place where reading refuses a file
that decoded cleanly (D33). The container operation plus frame decoding remains
the lenient path, and returns frames as stored.

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

A zero-valued field SHOULD mean "use the default for that field", so that a
caller may set only the limits it cares about.

The implementation MUST perform overflow-safe length arithmetic before allocation or slicing.

### 16.1 The unknown-chunk budget

`MaxUnknownBytes` bounds the combined payload of the chunks the package assigns
no meaning to and therefore retains whole. Those are the only chunks whose size
nothing but the file itself constrains: every chunk the package understands is
bounded by what it can validly contain, an `IFhd` being 13 bytes and a `UMem`
being as long as dynamic memory. Neither of the other two size limits covers
this, because one bounds each chunk alone and the other bounds the whole file,
and neither bounds *how many* chunks a file may spend on payloads nothing will
read.

Three rules make it usable rather than merely present (D26):

-   **Chunks the package understands MUST be charged nothing**, rather than
    counted against a separate allowance. Including them would put ordinary
    saves at the mercy of a setting that exists for junk. The set of understood
    identifiers is therefore part of the implementation's contract, and adding
    an identifier the package interprets means adding it to that set.
-   **The check MUST follow the containment check of §7.1 item 3, not precede
    it.** A chunk whose declared length runs past the end of the FORM would also
    exhaust this budget, and reporting that as a resource limit would send a
    caller to inspect its configuration instead of its file. Malformed input is
    to be diagnosed as malformed.
-   **The error MUST name the chunk that crossed the limit**, which is the one a
    caller would have to remove, rather than the first unknown chunk in the file.
    It MUST be reported before that chunk's payload is allocated.

### 16.2 Limits apply to reading only

Limits bound a decode because a decode allocates from lengths the file supplies.
Writing allocates from values the caller supplies, so there is nothing hostile to
bound and no write option for limits. The one size check on the writing side is
§11's rejection of a FORM that cannot describe its own length.

This asymmetry is deliberate. Should a caller ever want "write nothing larger
than *n*", that is a new option rather than a reuse of these limits (D42).

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

This list has been superseded by the annotated table in `spec-deltas.md`
section 6, which is normative in its place. That table is a superset: it adds one
row per question only a real file can settle — a frame with the discard bit set, a
save carrying `IntD`, a mis-ordered file, a save with no dummy frame, an over-long
`IFhd`, and a version 6 game — and it records, per row, which entries the fixture
exercises and whether a file behind it exists. Maintaining the annotation is the
point: a fixture list without it cannot show which fixtures are still missing.

Fixtures SHOULD be named so that the story they belong to is recoverable from the
name, and the test that reads them SHOULD fail rather than skip when it cannot
find that story, so a misnamed fixture is not silently untested.

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

save, err := quetzal.Read(f, story)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("PC: %#x\n", save.Header.PC)
fmt.Printf("frames: %d\n", len(save.Frames))
```

Because §5.5 makes the package's own documentation the specification of the API,
that documentation carries obligations this section would otherwise only imply:

-   Every exported identifier MUST be documented, including struct fields whose
    meaning is not evident from their name and type.
-   Naming conventions the package follows MUST be stated in the package
    documentation rather than left to be inferred from examples. A reader should
    not have to discover by comparison that `Parse` and `Decode` differ in
    whether the result count is fixed.
-   Documented behavior that a reader would find surprising — a field that does
    not hold what its name suggests, a deliberate asymmetry between two
    operations — MUST be documented where it is declared, not only in this
    document or in `spec-deltas.md` (D44).

Every Go example in the README MUST compile against the package, and something
automated MUST check that it does. An example that has drifted out of date is
worse than no example, because it is read as though it were tested.

## 23. Version Support

The package SHOULD implement Quetzal independently of any particular Z-machine version where the file format permits this.

Version-specific interpretation SHOULD be introduced only where required by Quetzal.

The initial release SHOULD aim to support Quetzal saves for Z-machine versions 1 through 8 as represented by the Quetzal 1.4 standard.

Support MUST NOT be advertised merely because fields can be parsed; interoperability tests should substantiate version claims.

At v1.0 that rule bites, and the claim is narrowed to match the evidence rather
than the code. Every version from 1 through 8 is implemented, and the
version-dependent paths are few — whether a save carries the dummy frame, and how
a story's declared length is scaled when a checksum must be computed. What has
been exercised against real files is less than that:

| Versions | Exercised by |
|---|---|
| 3 | Committed fixtures, in the ordinary test suite. Three stories, three interpreters, both encodings, both directions. |
| 5, 6 | Stories a maintainer can fetch, in tests that skip when they are absent. Reaches neither a fresh clone nor CI. |
| 1, 2 | Nothing. |

Documentation and release notes MUST state the distinction rather than claiming
1 through 8 without qualification. The wording SHOULD be no stronger than:
*implements Z-machine versions 1 through 8; tested against version 3 stories,
with versions 5 and 6 exercised against stories that cannot be redistributed.*

The reasons are recorded in §30 and in `spec-deltas.md` D43. They are limits on
what may legally be committed as a test fixture, not gaps in the implementation,
and no amount of further work on this package changes them.

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

Every one of these is met. Interoperability is established against three
unrelated implementations rather than one, and additionally against the
conformance checker standard 9.2 describes; `spec-deltas.md` section 7 records
each contact and, more usefully, what each one did not establish.

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

**All seven milestones are complete.** Milestones 1 through 6 delivered the
container, story identity, dynamic memory, stack frames, the writer, and
interoperability; Milestone 7 delivered four fuzz targets, §20's malformed-input
list, the limits of §16 including the unknown-chunk budget, §12's text helpers,
and the API and documentation review whose conclusions are D44 and §5.5.

## 28. Acceptance Criteria

The package is ready for v1.0 when all of the following are true.

**All fifteen are met.** Statement coverage is at 100%, which is worth keeping
there for a reason particular to this package: an unreachable branch here usually
means a check that cannot fire, and that is a design question rather than a
testing one — §2.1's masking rules and §10.1's write-side-only check are both
cases where the question was asked and the answer changed the specification.

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

## 30. Known Limitations at v1.0

Accepted limitations, not open work. Each is stated here so that the acceptance
of this document is not read as a claim these were addressed, and so that a
maintainer who rediscovers one can tell that it was known.

**Versions 1 and 2 are untested by any real file.** What they need is a story
whose header carries no checksum, which is what §6.1 exists for. The arithmetic
is verified against six stories that do carry one, in all three of the version
bands where the scale factor differs; the *decision to apply it* — that a zero at
`$1C` is the right trigger — is confirmed by nothing. No copy of a version 1 or 2
story could be found in redistributable or even fetchable form: the archived
copies of the early games are all version 3 re-releases. Entries D27 and D43.

**Version 6 reaches neither a fresh clone nor CI.** A real version 6 save
exercises the one branch where §10.4's dummy-frame rule inverts, and D33's
strictness makes it the branch where being wrong is most expensive. Such a save
exists and is read by tests, but its story cannot be committed, so the test skips
in any checkout that has not fetched one. Entry D43.

**Story fixtures are limited by copyright, permanently.** Of the archived Infocom
catalogue only Zork I, II, and III carry a license permitting redistribution,
and all three are version 3. §21 forbids committing the rest. This is not a
matter of looking harder, and the search has been done once so that it need not
be repeated.

**One leniency option exists, and the others are hypothetical.** §3.5 permits
non-conforming input only by explicit opt-in, and there is exactly one such
option: overlooking the chunk-ordering rule of §2.1, because the most widely used
interpreter accepts a file this package refuses. The other place where reading
refuses a file that decodes cleanly is the dummy-frame requirement, which has no
escape hatch because the same interpreter refuses the same file. Should one prove
necessary, it MUST follow the existing option's shape — naming the single rule
being overlooked — rather than becoming a general leniency switch. Entry D30.

**Half of the interoperability evidence cannot be re-run by `go test`.** Files
this package writes have been restored in three interpreters, but two of those
checks are manual: one interpreter is a GUI application, and restoring a save is
not something a test can assert from outside. The inbound direction is committed
fixtures with tests behind them; the outbound direction is a recipe and a record.
`spec-deltas.md` section 7 is that record, and is the only place it exists.

**A clean run of the conformance checker proves less than it appears to.** The
checker distributed with the standard reports every file this package writes as
valid, and its own usage text says it does not do in-depth checking. It also
accepts a zero-length `IFhd`, a `Stks` whose length is not a whole number of
frames, and a `CMem` ending in a dangling zero, all of which this package
rejects. Its verdict covers container soundness, chunk presence, and ordering. It
is an independent opinion on those, which is what makes it worth running, and it
is not a second test suite.

## 31. Amendment Policy

This document is accepted, which changes what its companion is for.

`spec-deltas.md` was written because this specification came before the
implementation, so its section 4 records cases where the implementation was
right and the specification merely early. Those are now absorbed into the
sections above, and that section is closed: **an implementation choice that
differs from this document is no longer a delta to be recorded, it is a defect in
one of the two, and one of them must change.**

The two documents therefore divide as follows, and this division is normative:

| Document | Records |
|---|---|
| `specification.md` | What this package does and why, in RFC 2119 terms. Amended directly when behavior changes. |
| `spec-deltas.md` | Where this package departs from Quetzal 1.4 (sections 1 through 3), the limitations of §30, the fixture inventory, and the interoperability evidence log. |
| GitHub issues | The design of the facilities §5.7 places outside this document — comparison, and any later feature that serves testing or debugging rather than the format. |

Amending this document requires:

1.  A behavior change, or the discovery that stated behavior was never
    implemented. Documentation-only corrections need no ceremony.
2.  For a new divergence from Quetzal 1.4: an entry in `spec-deltas.md` carrying
    a fresh identifier, an interoperability risk estimate, and a row in §2.1.
    An entry without a row in §2.1 is not accepted, and the test suite enforces
    this.
3.  For a change that makes a previously valid file invalid: the standards or
    security justification §26 requires, recorded with it.
4.  For a change to the exported API alone: nothing here, per §5.5 — but the
    package documentation is where it must be described, and §22 applies.
5.  For a facility §5.7 covers: nothing here, and nothing in `spec-deltas.md`.
    Its doc comments and its issue are where it is specified, and §5.7's four
    requirements are what it must meet. Adding one does not move the
    **Describes** line below, since the behavior this document states is
    unchanged by a facility it does not describe.

Delta identifiers are never reused and never renumbered, so a reference from a
commit message or a test name always lands somewhere. An entry that is resolved
stays where it is, marked.

The **Describes** line at the head of this document names the commit whose
behavior it states, and moves when behavior does. A documentation-only change
leaves it alone, which is what makes it worth having: a reader comparing that
commit against `HEAD` sees every change this document has not been checked
against.
