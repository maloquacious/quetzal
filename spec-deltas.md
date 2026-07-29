# Specification Deltas

Every place this package's behavior differs from `specification.md` or from
Martin Frost's Quetzal 1.4 standard, and why.

## Why this file exists

Milestone 6 is interoperability: exchanging saves with an established
interpreter. When that milestone finds a disagreement, the first question is
always *did we choose this, or is it a bug?* This file answers that question
without a code archaeology expedition.

Entries have stable identifiers (D1, D2, ...) so commits, issues, and test
names can point at them. Identifiers are assigned in the order entries are
written, and entries are filed by subject, so the numbering within a section is
not consecutive. An identifier is never reused or renumbered; an entry that is
resolved stays where it is, marked, so that an old reference still lands
somewhere.

**Interop risk** estimates the chance that the entry causes a disagreement with
a real interpreter:

- **high** — expected to bite; plan a test for it.
- **medium** — plausible; worth a fixture.
- **low** — no known writer does this, but it is a rejection path.
- **none** — cannot affect files exchanged with another implementation.

Section 5.x references are to the standard (`references/savefile_14.txt`);
section references with a § are to `specification.md`.

---

## 1. Stricter than the standard requires

These are rejection paths. Each one can turn a file another implementation
considers valid into an error. If interoperability testing trips one of these
on a file a real interpreter wrote, the delta is the thing to change — not the
file.

### D1 — Stack frame flags: reserved bits must be zero

`DecodeStks` rejects a frame whose flags byte sets any bit outside `000pvvvv`
(mask `0xE0`).

The standard defines the layout (4.3.2) but does not say what to do with the
undefined bits. Frames are parsed one after another with nothing to
resynchronize on, and the two fields that *are* defined — a four-bit local
count and a one-word evaluation count — are legal at every value they can hold.
The reserved bits are therefore the only structural check available on a frame
header, which is why they are enforced rather than masked off.

*Where:* `stack.go`, `DecodeStks`. *Interop risk:* **medium**. A writer that
builds the byte from a local count and a discard flag cannot set them, which is
presumably every writer — but this is the delta most likely to reject a file
from an interpreter nobody has tested. Masking instead of rejecting is a
one-line change if a real writer turns out to set them. One Frotz 2.55 save
sets none of them (section 7), which is a start and no more.

### D2 — Arguments mask: the eighth bit must be zero

`DecodeStks` rejects an arguments byte with bit 7 set; `Frame.Validate` rejects
it on write. A routine takes at most seven arguments, so `0gfedcba` leaves the
top bit undefined (4.3.4, §5.3).

*Where:* `stack.go`. *Interop risk:* **medium**, for the same reason as D1, and
less well probed: the only real save examined so far uses just two mask values
(section 7).

### D3 — A file holding both `CMem` and `UMem` is rejected

`File.Memory` errors rather than picking one. The standard says dynamic memory
is stored one way *or* the other (7.18) and gives no rule for choosing between
two competing statements of the same state. §7.2's "first instance wins" covers
repeats of the *same* chunk ID, which is a different situation. This is §14's
"contradictory memory representations".

*Where:* `memory.go`, `File.Memory`. *Interop risk:* **low**.

### D4 — Rebuilding memory requires an `IFhd`, and the story must match

`File.Memory` decodes the save's `IFhd` and verifies it against the supplied
story before expanding `CMem`, returning `ErrStoryMismatch` if they disagree.

§8 requires this check in `Read`. It is applied one layer lower because a
difference stream applied to the wrong story decodes *without error* into
plausible nonsense — silent corruption is the worst available outcome.
`DecodeCMem` remains the unchecked primitive for callers who mean to do that.

*Where:* `memory.go`, `File.Memory`. *Interop risk:* **low**, but see D27:
pre-checksum stories can fail this check for a reason that is our fault.

### D5 — `CMem` that expands past the end of dynamic memory is an error

Standard 3.5 lists this as an error case and leaves the handling "in whatever
way seems appropriate to the interpreter writer". We error. Likewise a zero
byte with no run-length byte after it.

*Where:* `memory.go`, `DecodeCMem`. *Interop risk:* **low**. A writer that
compresses page-at-a-time (3.3) emits adjacent runs, not an overrun.

### D6 — `UMem` length must equal the story's dynamic memory exactly

Both shorter and longer are rejected, per standard 3.6 and §9.1.

*Where:* `memory.go`, `decodeUMem`. *Interop risk:* **low**.

### D7 — Chunk identifiers must be four printable ASCII characters

Enforced because a non-printable ID is the earliest signal that the chunk
stream has desynced.

*Where:* `chunk.go`, `ID.valid`. *Interop risk:* **low**. See D10 for the part
of the ID rule that is deliberately *not* enforced.

### D8 — `ParseStory` rejects Z-machine versions outside 1–8

Also rejects a static-memory base inside the 64-byte header, or past the end of
the image. Quetzal 1.4 covers versions 1–8.

*Where:* `story.go`, `ParseStory`. *Interop risk:* **none** (concerns story
images, not saves).

### D9 — Versions other than 6 must begin with the dummy frame

`ValidateFrames` rejects a call stack whose first frame is not the dummy frame,
for every Z-machine version except 6. Standard 4.11.2 says the frame "must be
written even if no evaluation stack is used at the top level, and therefore
interpreters may assume its presence".

`DecodeStks` does not enforce it — decoding needs no story, and therefore no
version — so this bites only when a caller validates. `Frame.IsDummy`
identifies the frame; nothing removes or reinterprets it, per §10.4.

*Where:* `stack.go`, `ValidateFrames`. *Interop risk:* **low**; it is the
standard's own rule. Note that the converse is not checked: a V6 save carrying
a dummy frame anyway is accepted.

### D32 — `Read` rejects a file whose `IFhd` does not come before `CMem`/`UMem`/`Stks`

Standard 5.4 requires that order so that an interpreter learns it has the wrong
story before decoding memory against it, and §7.1 item 6 repeats the
requirement. This closes the gap that D25 recorded.

The check lives in `Read`, not in `Decode`. `Decode` reports what the container
holds and deliberately judges nothing about which chunks are present or what
they mean; ordering is only meaningful relative to chunks whose presence is
required, so it belongs with the layer that requires them. A caller that wants
a mis-ordered file anyway can still `Decode` it and pull the chunks out by
identifier.

Chunks that are not `IFhd`, `CMem`, `UMem`, or `Stks` may appear anywhere,
including before the `IFhd`. The standard constrains only those four.

*Where:* `read.go`, `File.checkOrder`. *Interop risk:* **low**. Every save seen
so far is in the required order, and a writer that gets it wrong is one an
interpreter would likely reject too.

### D33 — `Read` validates the save it reconstructs, and so requires the dummy frame

`Read` finishes by calling `Save.Validate`, which makes a `*Save` returned by
`Read` a save that could be written straight back out. Everything Validate
checks is already guaranteed by decoding except one thing: D9's rule that a
save for any version but 6 must begin with the dummy frame. So in practice this
entry *is* D9, applied on the way in.

This is a deliberate choice to be strict where the standard is unambiguous —
4.11.2 tells interpreters they "may assume its presence" — and it is the one
place where reading rejects a file that decodes cleanly. `Decode` plus
`File.Frames` remains the lenient path, and returns the frames as stored.

*Where:* `read.go`, `File.Save`. *Interop risk:* **medium**. A single missing
dummy frame from any real writer turns this from strictness into an
interoperability bug. Confirm against more than one interpreter in Milestone 6;
if it fires, the fix is an explicit lenient option, per §3.5.

---

## 2. More lenient than a literal reading

These accept files a stricter reader might reject. They cannot cause us to
reject a valid file, but they can let a malformed one through.

### D10 — The "spaces trailing only" rule for chunk IDs is not enforced

Standard 8.3.3 requires that any spaces in a four-character ID be trailing.
That rule distinguishes no valid file from any real one, and enforcing it would
reject otherwise sound saves for no safety gain. Such IDs are preserved
exactly.

*Where:* `chunk.go`, `ID.valid` (documented in the comment). *Interop risk:*
**none**.

### D11 — A pad byte must be present but may hold any value

The standard says the pad byte is zero. Its presence is required, because chunk
lengths keep the stream aligned; its value is ignored and discarded, because a
non-zero pad carries no information.

*Where:* `read.go`, `decoder.chunk`. *Interop risk:* **none**.

### D12 — An `IFhd` longer than 13 bytes is accepted

Standard 5.4.2 fixes the length at 13; standard 5.5 explicitly reserves the
right to extend the chunk while keeping the meaning of the first 13 bytes.
Extra bytes are preserved in `Header.Extra` rather than rejected or dropped.
See D19 for what this means on write.

*Where:* `header.go`, `ParseHeader`. *Interop risk:* **none** on read.

### D13 — Bytes after the FORM are ignored

A simple IFF file is a single FORM chunk, so `Decode` stops at its end and
neither consumes nor examines what follows.

*Where:* `read.go`, `Decode`. *Interop risk:* **none**.

### D14 — An empty `FORM IFZS` decodes successfully

`Decode` performs container-structural checks only (§7.1 items 1–4). Whether
the chunks Quetzal requires are present is decided by `File.Header`,
`File.Memory`, and `File.Frames`, each of which reports its own chunk missing.
See D25 for the item this leaves unchecked.

*Where:* `read.go`. *Interop risk:* **none**.

### D15 — An empty `Stks` chunk decodes to zero frames without error

Whether zero frames is legal depends on the story's version — versions other
than 6 require the dummy frame — so the check belongs to `ValidateFrames`,
which has the story.

*Where:* `stack.go`, `DecodeStks`. *Interop risk:* **none**.

### D35 — Standard 7.14 is not enforced: an `IntD` may name neither a system nor an interpreter

7.14 says the interpreter and operating-system IDs "may not both be `    `"
(four spaces), since a chunk useful to every interpreter on every system is a
contradiction. `ParseInterpreterData` accepts it, as it accepts a non-zero
reserved word and any contents ID.

Rejecting would mean failing a restore over an optional chunk whose payload we
do not read, which trades a real capability for the enforcement of a rule that
distinguishes no file anyone writes.

*Where:* `chunk.go`, `ParseInterpreterData`. *Interop risk:* **none**.

---

## 3. Writer choices

Where the standard permits a choice, this is the choice made. These decide what
*we* emit, so they are what an external interpreter will judge.

### D16 — A discarded result variable is preserved on read, zeroed on write

When the `p` bit is set the result-variable byte carries no meaning. Standard
4.6 asks writers to store zero, and `EncodeStks` does. `DecodeStks` keeps
whatever was there, because discarding information is the thing this package
avoids and the byte costs nothing to carry.

The asymmetry means `EncodeStks(DecodeStks(x))` is not always byte-identical to
`x`. That is within §18.1, which requires a *semantic* round trip; `FuzzStacks`
normalizes for it explicitly.

*Where:* `stack.go`. *Interop risk:* **none**.

### D17 — `EncodeCMem` drops the trailing zero-difference region

Standard 3.4 permits this and does not require implementing it on writes. We do
it, so our `CMem` payloads end at the last changed byte.

*Where:* `memory.go`, `EncodeCMem`. *Interop risk:* **medium** — this is
precisely the case standard 3.4 says readers "must understand", so it is worth
confirming that the target interpreter does. Frotz does it too (section 7),
which is fair evidence that it also reads it.

### D18 — Zero runs longer than 256 bytes are split into consecutive runs

One length byte holds `n` for a run of `n+1`, so 256 is the maximum. Longer
gaps become several adjacent maximum-length runs (standard 3.3, §9.3).

*Where:* `memory.go`, `EncodeCMem`. *Interop risk:* **medium**. Real dynamic
memory has long unchanged stretches, so almost every file we write exercises
this — a four-move Frotz save already contains 34 maximum-length runs
(section 7).

### D19 — `Header.Encode` writes `Header.Extra` back out

A save read from a longer-than-13-byte `IFhd` (D12) re-encodes to the same
longer payload. This is faithful preservation, but it means we can emit an
`IFhd` whose length is not 13, which an interpreter reading standard 5.4.2
literally may reject.

*Where:* `header.go`, `Header.Encode`. *Interop risk:* **medium** — but only
for files that already contained a non-conforming `IFhd`. If this bites, the
fix is a write option that drops `Extra` rather than a change to the reader.

### D20 — Compression is not optimal

`EncodeCMem` makes a single pass and does not search for a shorter encoding.
Standard 3.3 and §9.3 both say this is fine.

*Where:* `memory.go`. *Interop risk:* **none**.

### D34 — `Read` does not carry forward `IntD` chunks the standard forbids copying

Standard 7.11: if the `c` flag is set, the contents "must not be copied" into
another file, because they describe the exact saved position that carries them.
7.10: if the `s` flag is set and the operating-system ID does not match the
current system, the chunk "should not be copied". §13 asks for a conservative
default.

`Read` therefore leaves three kinds of `IntD` chunk out of the `Save`:

- `c` set — forbidden outright.
- `s` set — this package has no notion of what system it is running on, so it
  cannot distinguish the permitted case (same machine) from the forbidden one.
  The conservative reading is the only one available to it.
- shorter than the 12-byte fixed header — a payload that cannot state its
  restrictions cannot be shown to be free of them.

The drop is on the reading side because the standard's restriction is on the
*copy*, which is the load-then-save path. A caller that builds its own `IntD`
and puts it in `Save.Chunks` gets it written, flags and all; the writer imposes
nothing. And nothing is lost outright: `Decode` retains every chunk, so a
caller that does know its own machine can take the chunk from the `File` and
carry it forward deliberately.

*Where:* `read.go`, `File.Save`; `chunk.go`, `InterpreterData.Copyable`.
*Interop risk:* **low** outbound (dropping a chunk cannot make a file invalid);
**none** inbound. The cost is a MacOS alias or similar being discarded when it
would in fact have been usable — see 7.22.

### D36 — Additional chunks are always written after the three required ones

`Save.Encode` writes `IFhd`, then memory, then `Stks`, then everything in
`Save.Chunks` in the order the save holds them. A file read with an annotation
*before* its `IFhd` is therefore written back with the annotation last.

The relative order of the additional chunks is preserved, per §5.4; their
position relative to the required three is not. Standard 5.4 fixes only that
`IFhd` comes first, and §18.2 does not promise a byte-identical rewrite.

*Where:* `write.go`, `Save.Encode`. *Interop risk:* **none**.

### D37 — An unset memory encoding is an error, not a default

`Memory{Data: mem}` with no `Encoding` fails to write. Quetzal recommends
compression and defaulting to it would always produce a valid file, so this is
strictness for its own sake — but a save whose encoding was never chosen is
more likely to be a half-built value than a request for the default, and
`WithEncoding` makes saying so a single call.

*Where:* `memory.go`, `Memory.Validate`. *Interop risk:* **none**.

### D38 — `File.WriteTo` refuses a FORM larger than 4 GiB

The FORM length is a four-byte field, so a longer container cannot describe
itself. §11 requires rejecting fields that cannot be represented. Dynamic
memory tops out at 64 KB, so reaching this needs deliberate effort.

*Where:* `write.go`, `File.WriteTo`. *Interop risk:* **none**.

---

## 4. Data model deviations from `specification.md`

§5's API is explicitly representative — "the exact API MAY evolve during
implementation, but the semantics in this specification MUST be retained".
These are the evolutions.

### D21 — `Memory.Data` always holds decoded dynamic memory

Never the stored payload. A `CMem` payload is expanded against the story before
it reaches the type, and `Memory.Encoding` records only *how the save stored
it*, so a file can be rewritten the way it arrived. §18.1 states the semantic
round trip over dynamic memory rather than over the payload that encoded it.

*Where:* `memory.go`. *Interop risk:* **none**.

### D22 — `MemoryEncoding` also serves as the writer's mode

§9.4 sketches a separate `MemoryMode` enum (`CompressMemory`/`StoreMemory`) for
the writer. Two parallel enums for one distinction would be redundant, so
`MemoryEncoding` does both jobs: descriptive when read, directive when written.
The Milestone 5 write option will take a `MemoryEncoding`.

*Where:* `memory.go`. *Interop risk:* **none**.

### D23 — `Story` carries a `Version` field

§6's representative type has release, serial, checksum, and dynamic memory.
Version was added because the dummy-frame rule (standard 4.11) is
version-dependent and `ValidateFrames` needs it.

*Where:* `story.go`. *Interop risk:* **none**.

### D24 — Additions with no counterpart in §5

- `Identity` — the release/serial/checksum triple, so story matching is one
  comparison and error messages can print both sides.
- `Header.Extra` — see D12.
- `FrameError` — §15 names `ChunkError` only, but says error text should name
  the offending chunk *or frame*. Carries an index, not a byte offset: the
  frame number is what a caller can act on.
- `File.limits` (unexported) — `File` remembers what `Decode` was configured
  with, so `File.Frames` bounds allocation the way the caller asked. A `File`
  built by hand leaves it zero and gets the defaults.
- `MaxLocals`, `MaxEvaluationWords` — exported so callers can validate before
  building frames.
- `Save.Encode`, `File.WriteTo`, `File.Save` — the halves of `Read` and
  `Write`, exposed for the same reason `Decode` is: a caller may want the
  container without the state, or the state without the bytes.

*Interop risk:* **none**.

### D39 — `Read`, `Write`, and `Validate` take `Story` by value, not `*Story`

§7, §11, and §14 all write `*Story`. This package takes it by value, as
`Header.Verify`, `File.Memory`, and `ValidateFrames` already did.

A pointer would mean a nil story is meaningful: *reconstruct what you can
without one*. That case is already served, and served better, by `Decode` —
which needs no story precisely because it reconstructs nothing. `Read` cannot
do its job without a story: compressed memory is a difference against one, and
the Z-machine version decides what the call stack must contain. Making the
parameter unmissable says so.

`Story` holds one slice header and four small fields, so passing it by value
costs nothing worth measuring, and it removes the question of whether the
package retains the pointer.

*Interop risk:* **none**.

### D40 — `Save.Chunks` holds only the chunks the other fields do not

§5's `Save` has both the interpreted fields and a `Chunks []Chunk`, without
saying whether the latter repeats the former. It does not: `IFhd`, `CMem`,
`UMem`, and `Stks` are represented by `Header`, `Memory`, and `Frames`, and
`Save.Validate` rejects a save that also carries one of them as an additional
chunk. Writing both would mean writing a file that contradicts itself, and
there would be no rule for which copy wins.

Two consequences on the reading side:

- A duplicate of a single-instance chunk is dropped, not preserved. Standard
  7.2 makes the first instance authoritative and the rest ignorable; carrying
  an ignored `IFhd` into `Save.Chunks` would only cause the writer to emit a
  file with two of them. `Decode` still keeps every chunk, duplicates included.
- Multiple `ANNO` chunks are unaffected. They are legal in quantity, so they
  are all kept, in order.

*Where:* `read.go`, `File.Save`; `write.go`, `checkExtraChunks`.
*Interop risk:* **none**.

### D41 — `InterpreterData` omits the reserved word and cannot be encoded

§13's representative type is followed except that the two reserved bytes
between the contents ID and the interpreter ID are not represented: 7.8.6 fixes
them at zero, so they carry nothing.

There is no `Encode` counterpart. §13 says the payload is opaque and this
package "MUST NOT assign semantics to interpreter-specific payloads it does not
understand"; a caller that wants to write an `IntD` builds the `Chunk` itself,
which is a dozen bytes of `append`. Parsing exists only because the writer has
to read the flags to honor 7.11.

*Interop risk:* **none**.

---

## 5. Gaps

Gaps, not decisions. Listed because an interoperability failure may trace to
one of them rather than to a delta above. Entries resolved by later work stay
here, marked, so that a reference to their identifier still lands somewhere.

### D25 — §7.1 item 6 is unenforced: nothing checks that `IFhd` precedes `CMem`/`UMem`/`Stks`

**Resolved in Milestone 5.** `Read` enforces the ordering and `Save.Encode`
produces it; `Decode` still does not check. See D32 for the reasoning and for
where the check lives.

### D26 — `Limits.MaxUnknownBytes` is declared but never enforced

Nothing sums the payloads of chunks the package does not interpret.
`MaxChunkBytes` bounds each chunk individually and `MaxFormBytes` bounds the
whole file, so the exposure is limited, but the field currently does nothing.

*Interop risk:* **none**. Security-relevant, and a §16 item.

### D27 — Pre-checksum stories get a computed checksum

**Implemented.** Standard 5.5: if a story has no checksum, the saving
interpreter "should calculate it in the normal way from the original story
file". `ParseStory` does so when `$1C` holds zero, and records the fact in
`Story.ChecksumComputed`. `StoryChecksum` exposes the calculation on its own.

The algorithm is the sum of every byte from `$40` to the declared end of the
story, modulo `0x10000`, where the length is the word at `$1A` times 2 for
V1–3, 4 for V4–5, and 8 for V6–8. The declared length is used rather than the
size of the image, since a story file may carry padding past its end.

Three decisions inside it:

- **A stored checksum is never recomputed or second-guessed.** If `$1C`
  disagrees with what the image sums to, the stored value wins. Interpreters
  compare the stored value and therefore agree with each other; substituting
  our own arithmetic would break the matching this exists to make work. Only a
  zero triggers computation.
- **A story with neither a checksum nor a usable length keeps its zero.** Some
  of the same early games leave `$1A` unused as well, and there is then no
  "normal way" to calculate anything — the Z-machine's own definition of the
  file length is the field that is missing. `StoryChecksum` reports `ok=false`
  and nothing is invented. `ChecksumComputed` stays false, so a caller can tell
  this case from a successful computation.
- **`ChecksumComputed` is exported** rather than kept private, because a
  `Story.Checksum` that is not the value in the file is surprising, and a
  server logging story-mismatch errors wants that bit in the log line.

*Verification:* recomputing the checksum of all three `testdata` stories
reproduces their stored values exactly — `0xbf44`, `0x4492`, `0xf645` —
which is the only evidence available that this is the right algorithm, since no
pre-checksum story can be committed to test the path that needs it. A synthetic
version 2 image carries the end-to-end round trip in
`TestRoundTripPreChecksumStory`.

*Interop risk:* **low**, down from high. What remains is that no real
pre-checksum story has ever been through it: the arithmetic is confirmed
against three files, the *decision to apply it* is not. See D43.

### D28 — No diagnostic mechanism for ignored duplicate chunks

§7.2 asks that later instances of a single-instance chunk "be ignored with a
diagnostic mechanism if diagnostics are enabled". They are ignored silently;
there is no diagnostics facility.

*Interop risk:* **none**.

### D29 — No text-chunk helpers

§12's `Annotations` and `Author` helpers do not exist. Both chunk kinds survive
as raw chunks in `Save.Chunks` and in the decoded `File`, so nothing is lost;
what is missing is the convenience of reading them as text.

§13's `InterpreterData` **does** now exist, along with the copy restrictions it
was needed for — see D34 and D41. That half of this entry is resolved.

*Interop risk:* **none**. The chunks round-trip whether or not anything
interprets them.

### D30 — No lenient mode

§3.5 permits leniency only as an explicit opt-in. Nothing in section 1 above
can currently be relaxed without a code change. If interoperability testing
finds a writer that trips D1 or D2, the choice is between relaxing the rule
outright and introducing the option §3.5 anticipates.

*Interop risk:* n/a — this is the escape hatch for everything else.

### D31 — No `Save`, `Read`, or `Write`

**Resolved in Milestone 5.** All three exist, along with `Save.Validate`,
`Save.Encode`, `File.Save`, `File.WriteTo`, and `WithEncoding`. The choices
made along the way are D32–D41.

### D43 — Only version 3 stories are exercised, and that is an accepted limit for v1.0

Every story fixture is a version 3 Infocom game — Zork I, II, and III — because
those are the only images whose redistribution is clearly permitted. After D27
was implemented, **nothing here is a missing feature; what remains is entirely
untested paths**:

- **Versions 1 and 2.** D27's computed checksum is what these need, and it is
  now written and verified against three real stories. What has not happened is
  a real pre-checksum story going through it, which would confirm that
  computing on a zero at `$1C` is the right trigger and that our arithmetic
  matches what an interpreter of that era's games records.
- **Version 6.** Execution begins at a routine, so a V6 save carries no dummy
  frame. `checkDummyFrame` returns early for version 6 and `ParseStory` accepts
  versions 1 through 8, so the code handles it — but no real V6 file has run
  through it, and D33's strictness makes this branch the one where being wrong
  is most expensive.

#### Why the fixtures cannot be had

Checked 2026-07-29, so that nobody repeats the search. `historicalsource` on
GitHub hosts the Infocom catalogue — `arthur`, `journey`, `shogun`, `zorkzero`
for version 6; `deadline`, `starcross`, `suspended`, `witness`, `zork-1` among
the early games — but **only `zork1`, `zork2`, and `zork3` carry a LICENSE
file.** Those three are MIT, Copyright Microsoft. Every other Infocom
repository there has no license at all: it is archived source, not a rights
grant, and §21 forbids committing story files without clear permission.

Those three MIT repositories contain one compiled artifact each —
`COMPILED/zork1.z3`, `zork2.z3`, `zork3.z3` — which are exactly the three
fixtures already in `testdata/stories`. There is nothing further to extract
from that source.

So this is not a matter of looking harder. Closing it needs a version 1, 2, or
6 story released under terms that permit redistribution, and no such story is
known to exist. The fixtures are the entire cost of closing it; they are simply
not obtainable.

#### What to say in a release note

*Implements Z-machine versions 1 through 8; tested against version 3 stories.
Versions 1, 2, and 6 are implemented but unexercised, for want of a story file
that may legally be redistributed as a test fixture.*

*Interop risk:* **low** for V1 and V2, now that D27 is implemented; **unknown**
for V6, which is worse than a number, because nothing has looked.

### D42 — `Limits` are not applied when writing

`Limits` bounds a decode, because a decode allocates from lengths the file
supplies. Writing allocates from values the caller supplies, so there is
nothing hostile to bound and no `WriteOption` for limits. The one size check on
the writing side is D38's four-byte FORM length.

This is a deliberate asymmetry rather than an oversight, recorded here in case
a caller ever wants "write nothing larger than *n*" — which would be a new
option, not a reuse of `Limits`.

*Interop risk:* **none**.

---

## 6. Fixture checklist for Milestone 6

§19's fixture list, annotated with the deltas each one exercises. Section 7
records what has actually been checked against another implementation so far.

| Fixture | Exercises |
|---|---|
| Valid compressed save | D4, D5 |
| Valid uncompressed save | D6 |
| Save with annotations | D29, D36 |
| Save with unknown chunks | D26, D36, D40 |
| Odd-length chunks and padding | D11 |
| Multiple stack frames | D1, D2, D16 |
| Long zero runs | D18 |
| Trailing omitted `CMem` differences | D17 |
| A V1 or V2 game | D27 — *deferred, D43* |
| A V6 game | D9, D33 (dummy frame absent) — *deferred, D43* |
| Chunks in a non-standard order | D32 |
| A save with no dummy frame | D33 |
| A save carrying an `IntD` chunk | D34, D35, D41 |

The last five are additions to §19's list, one per open question that only a
real file can settle.

---

## 7. Interoperability evidence so far

### 2026-07-29 — one Frotz 2.55 save, read successfully

A single save produced by `dfrotz` (Frotz 2.55) from
`testdata/stories/zork1-r119-880429.z3`, four moves in, read with this package.
A one-off probe run while restructuring `testdata/`, not a test in the suite.
Recorded because it is the only contact with another implementation to date.

`Decode`, `File.Header`, `File.Memory`, and `File.Frames` all succeeded, and
`ValidateFrames` was clean. Nothing in section 1 fired.

**What it establishes.**

| Delta | Observation |
|---|---|
| D1 | Flags bytes `0x00 0x01 0x0c 0x07 0x00` — pure local counts. No reserved bit set. |
| D5, D18 | `CMem` was 289 bytes for 11282 bytes of dynamic memory: 123 literal differences and 83 zero runs, **34 of them at the 256-byte maximum**. Frotz splits long runs, and we decode what it splits. |
| D17 | The difference stream stops **40 bytes short of the end**. Frotz omits the trailing zero region, so standard 3.4's "must be understood on reads" is a live requirement, not a theoretical one. |
| D25 | Chunk order was `IFhd`, `CMem`, `Stks` — the order standard 5.4 requires, which we do not enforce but evidently receive. |
| D3 | `CMem` only; no `UMem` alongside it. |
| D12, D19 | The `IFhd` payload was exactly 13 bytes, so no `Extra`. |

**What it does not establish.** More than it does, and the gaps matter:

- **D16 is completely unexercised.** Not one frame had the discard bit set, so
  the result-variable asymmetry has never met a real file.
- **D2 is barely exercised.** Only `0x00` and `0x01` appeared as arguments
  masks. A routine called with several arguments would say more.
- **D6 is unexercised.** Frotz compresses by default, so no `UMem` fixture
  comes from this route at all.
- **D27 is untouched.** Zork I carries a checksum, so the pre-checksum gap —
  the highest-risk entry on this list — remains entirely untested.
- One interpreter, one version, one game, one position, one direction. Nothing
  here says anything about files *we* write being accepted elsewhere, which is
  the half of §19 that matters more.

Treat this as a smoke test that passed, not as evidence that section 1 is
safe.

### 2026-07-29 — the same save read, rewritten, and restored by Frotz

Run when Milestone 5 landed, to find out whether the writer produces files
another interpreter accepts. Same probe save, same story, same `dfrotz` 2.55.
Again a one-off, not part of `go test ./...`.

`Read` accepted Frotz's file: release 119, serial 880429, checksum `0xbf44`,
PC `0x7590`, `CMem`, five frames, no additional chunks. `Write` then produced
**434 bytes, byte for byte identical to the file Frotz wrote.**

That is a stronger result than §18.1 asks for, and it was not aimed at: the
standard permits any correct encoding, and D17, D18, and D20 are all places
where a different choice would have produced a different file of equal
validity. It means our compression happens to make the same choices Frotz's
does on this input — a coincidence worth noticing and not worth relying on. The
round-trip test in `write_test.go` checks semantic equality, not this.

Three files written by this package were then restored in `dfrotz`, each
landing in the Kitchen with score 10 at move 4, which is the saved position:

| File | What it tests | Result |
|---|---|---|
| The rewrite above | Acceptance criterion 7, outbound `CMem` | restored |
| The same save with `WithEncoding(MemoryUncompressed)`, 11424 bytes | D6 outbound — Frotz never *writes* `UMem` | restored |
| The same save plus an `ANNO` and an unregistered `Zzzz` chunk | D36, D40 — unknown chunks in a file someone else reads | restored |

**What this establishes.** Acceptance criteria 7 and 8 are met for one
interpreter: Frotz reads what we write and we read what Frotz writes. D6 is no
longer entirely unexercised — the `UMem` path is now known to work outbound,
though still never inbound from a real writer. An interpreter tolerating a
chunk it has no name for is confirmed rather than assumed.

**What it does not establish.** Everything section 7's first entry could not,
minus the two lines above. In particular D33 is the new strictness and nothing
here tests it: Frotz writes the dummy frame, so the one file that would matter
— a real save without one — still does not exist. D27 remains untouched.

One interpreter is one interpreter. Frotz agreeing with us proves the pair is
consistent, not that either is right.
