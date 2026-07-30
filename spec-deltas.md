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

### D1 — Stack frame flags: reserved bits are ignored

`DecodeStks` reads the local count and the `p` bit out of the flags byte,
`000pvvvv`, and ignores the top three bits entirely.

**This was a rejection path until 2026-07-29 and is now a masking one.** The
standard defines the layout (4.3.2) but says nothing about the undefined bits,
so a writer that set one would still be describing a frame this package
understands completely, and refusing the file would lose a save over bits with
no meaning.

The cost is real and worth stating. Frames are parsed one after another with
nothing to resynchronize on, and the two defined fields are legal at every value
they can hold, so the reserved bits were the only implausibility check available
on a frame header. Without it a desynced stack stream is caught only by running
out of payload rather than by an obviously wrong header. That remains a *bounds*
check — nothing is allocated before a frame is known to fit, and `MaxFrames` and
`MaxStackWords` still apply — so what degrades is diagnosis, not safety.

*Where:* `stack.go`, `DecodeStks`. *Interop risk:* **none**, down from medium.
Nothing can be rejected for these bits any more.

### D2 — Arguments mask: the eighth bit is masked away on reading

The arguments byte is `0gfedcba` and the eighth bit is undefined, since a
routine takes at most seven arguments (4.3.4, §5.3). `DecodeStks` clears it.

`Frame.Validate` still rejects it, which is deliberate asymmetry: a bit found in
a file is something to cope with, while a bit a caller put in a `Frame` is a
programming error worth reporting. Since reading always clears it, the write
check can only fire on a hand-built frame.

*Where:* `stack.go`. *Interop risk:* **none**, down from medium.

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

*Where:* `memory.go`, `decodeUMem`. *Interop risk:* **none**, down from low.
Both directions are exercised by files this package did not write:
`testdata/jzip/zork1-r119-kitchen-umem.qzl` was written by jzip's `-u` flag and
is read here, and Frotz and jzip both restore `UMem` files we write. The
qualification in section 7 stands — jzip's `UMem` path is new code — but the
gap this entry described, of never having read a `UMem` chunk from elsewhere,
is closed.

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
standard's own rule, and **both halves are now exercised by real files**: six V3
saves that carry the frame, and one V6 save from Gargoyle that correctly does
not (section 7). Note that the converse is still not checked: a V6 save carrying
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

**`IgnoreChunkOrder` relaxes it.** Frotz 2.55 restores a save whose `IFhd`
comes last (section 7), so this rejection is stricter than the most widely used
interpreter. Rather than choose between following the format and matching
Frotz, the check stays on by default and a caller meeting a writer that gets
the order wrong can opt out. Nothing else is relaxed with it — the identity
check the ordering rule exists to protect still happens before memory is
rebuilt, and a save missing its dummy frame is still refused.

Accepting a mis-ordered file does not mean producing one: a save read this way
is written back in the required order.

**`ckifzs` enforces the same rule.** The conformance checker from standard 9.2
reports `IFhd must come before CMem, UMem, or Stks` on a mis-ordered file
(section 7). That reframes this entry: it is not that we are stricter than
everyone, but that Frotz is lenient where the standard's own checker is not.
The option stays, because a file Frotz accepts is a file someone may hand us,
but the default needs no apology.

*Where:* `read.go`, `File.checkOrder`, `IgnoreChunkOrder`. *Interop risk:*
**low**.

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

*Where:* `read.go`, `File.Save`. *Interop risk:* **low**, lowered from medium on
evidence. **Frotz 2.55 refuses the same file** — a Kitchen save with its dummy
frame removed — with `Fatal error: Error reading save file` (section 7). Its
message does not say why, so this is not proof that it refuses for our reason;
what it establishes is that such a save is not a file real interpreters accept,
which is what the medium rating was hedging against. Two writers also emit the
frame, so nothing has ever produced one to reject.

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

**No committed fixture can exercise this.** Standard 4.6 sets the `p` bit "on
calls made by `CALL_xN`", and `CALL_1N`, `CALL_2N`, `CALL_VN`, and `CALL_VN2`
are all version 5 and later opcodes. In versions 1 through 4 every call stores a
result, so a V3 save cannot contain a frame with the bit set — which is exactly
what six fixtures from two interpreters show: zero discards, every time.

**A fetched version 5 story settles it.** A Border Zone save read through
`testdata/local` (section 7) contains `frame 1: DISCARD=true result=0x00`. So
the bit occurs in real files, and **Frotz stores zero in the result byte**, as
4.6 asks. The asymmetry this entry describes is therefore invisible in practice:
there is no non-zero result byte to preserve, and that save round trips byte for
byte. What `DecodeStks` preserves is a value real writers do not produce.

*Where:* `stack.go`. *Interop risk:* **none** — a writer that reads the byte
we zero would be reading a byte the standard already calls meaningless.

### D17 — `EncodeCMem` drops the trailing zero-difference region

Standard 3.4 permits this and does not require implementing it on writes. We do
it, so our `CMem` payloads end at the last changed byte.

*Where:* `memory.go`, `EncodeCMem`. *Interop risk:* **low**, lowered from
medium. Standard 3.4 says readers "must understand" this, so it needed
confirming. It is now confirmed twice over in each direction: Frotz and Bocfel
both *write* streams that stop short of the end (section 7), and both *restored*
files this package wrote, which use the same shortcut.

### D18 — Zero runs longer than 256 bytes are split into consecutive runs

One length byte holds `n` for a run of `n+1`, so 256 is the maximum. Longer
gaps become several adjacent maximum-length runs (standard 3.3, §9.3).

*Where:* `memory.go`, `EncodeCMem`. *Interop risk:* **low**, lowered from
medium. Real dynamic memory has long unchanged stretches, so almost every file
we write exercises this, and both interpreters do the same: a four-move Frotz
save contains 34 maximum-length runs and Bocfel's 30 (section 7). Files we wrote
this way restore in both.

### D19 — `Header.Encode` writes `Header.Extra` back out

A save read from a longer-than-13-byte `IFhd` (D12) re-encodes to the same
longer payload. This is faithful preservation, but it means we can emit an
`IFhd` whose length is not 13, which an interpreter reading standard 5.4.2
literally may reject.

*Where:* `header.go`, `Header.Encode`. *Interop risk:* **low**, lowered from
medium on evidence. A save was written through `Write` with
`Header.Extra` set, producing a 24-byte `IFhd`, and **Frotz restored it**
(section 7). So an over-long `IFhd` emitted by this writer is not a file real
interpreters choke on. If it ever does bite, the fix is a write option that
drops `Extra` rather than a change to the reader.

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

### D30 — Lenient mode: one option exists, and most rules need none

**Partly resolved.** §3.5 permits leniency only as an explicit opt-in, and there
is now one such option: `IgnoreChunkOrder` (D32). It is a `ReadOption`, so it
travels through `Read` and through `Decode` into `File.Save`, and it is off by
default.

The two entries this was originally written for — D1 and D2, the undefined bits
in a frame header — needed no option in the end. They were relaxed outright,
because an undefined bit carries no information worth refusing a file over, so
there was nothing for a caller to choose between.

What remains without an escape hatch is D33, the dummy-frame requirement, which
is the other place `Read` refuses a file that decodes cleanly. Frotz refuses the
same file, so there is no evidence an option is needed. If one turns out to be,
it should follow `IgnoreChunkOrder`'s shape rather than becoming a general
"be lenient" switch: a caller should have to name what it is willing to overlook.

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

- **Versions 5 and later.** D16 — the discarded result variable — cannot be
  exercised by any V3 save, because the `CALL_xN` opcodes that set the `p` bit
  do not exist before version 5. This is a consequence of the fixture
  limitation, not a separate problem, but it means the list of things a
  non-version-3 story would unlock is longer than it first appeared.
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

#### Fetchable, if not shippable

Since this entry was written, `testdata/local/` was added: a gitignored
directory, a `fetch.sh` that downloads version 5 and version 6 stories from
`historicalsource`, and tests that use whatever is present and skip when it is
not. That closes the practical half of the problem for a maintainer willing to
fetch, while leaving the repository shippable. Section 7 records what those
stories established — D16 exercised, D27's scale factor confirmed in all three
version bands, D2 better probed.

What it does not change: nothing of this reaches a fresh clone or CI, so every
entry below still stands as far as the committed suite is concerned. And no
version 1 or 2 story turned up in fetchable form either, so D27's trigger is
still untested by any real file.

#### Why the fixtures cannot be committed

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

So this is not a matter of looking harder. Committing a version 1, 2, 5, or 6
story would need one released under terms that permit redistribution, and no
such story is known to exist. Fetching one is a different question, answered
above.

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

| Fixture | Deltas | Status |
|---|---|---|
| Valid compressed save | D4, D5 | **have 7** — Frotz ×5, Bocfel ×1, jzip ×1 |
| Valid uncompressed save | D6 | **have 1** — jzip's `-u` flag |
| Save with annotations | D29, D36 | **have 1** — Bocfel's `ANNO` |
| Save with unknown chunks | D26, D36, D40 | **have 1** — Bocfel's 2508-byte `Bfhs` |
| Odd-length chunks and padding | D11 | **every fixture** — `IFhd` is 13 bytes |
| Multiple stack frames | D1, D2 | **have 7** — six at 5 frames, one fetched V5 at 7 |
| A frame with the discard bit set | D16 | **fetched** — Border Zone V5, `testdata/local` |
| Long zero runs | D18 | **confirmed both ways** — 34 max runs from Frotz, 30 from Bocfel |
| Trailing omitted `CMem` differences | D17 | **confirmed both ways** — both stop 40 bytes short |
| Chunks in a non-standard order | D32 | **built in-test**; `ckifzs` rejects it as we do, Frotz accepts it |
| A save with no dummy frame | D33 | **built in-test**; Frotz refuses it too |
| A save carrying an `IntD` chunk | D34, D35, D41 | **have 1** — Bocfel's story-path reference |
| An `IFhd` longer than 13 bytes | D12, D19 | **built and written**; Frotz restores both |
| A V6 game | D9 (dummy frame absent) | **fetched, and saved** — Journey via Gargoyle, `testdata/local` |
| A V1 or V2 game | D27 | **deferred** — no copy exists in any fetchable form |

The last seven rows are additions to §19's list, one per question that only a
real file can settle. **One remains open**: no version 1 or 2 story has been
found in any form, so D27's trigger is still untested (D43). Everything else on
this list has a file behind it, though the last two rows live in the gitignored
`testdata/local` and so reach neither a fresh clone nor CI.

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

### 2026-07-29 — a second implementation: Bocfel 2.5, via Gargoyle

The first save from an implementation unrelated to Frotz. Bocfel 2.5, running
under Gargoyle, same story and same position as the two entries above — Zork I
r119, the Kitchen, four moves in. Written by hand, not by script; Gargoyle is a
GUI application.

This one is a much richer file than Frotz's, and it moved several entries.

**Container.** `IFhd` 13, `IntD` 94, `CMem` 291, `Stks` 92, `ANNO` 23, `Bfhs`
2514 — and **eight bytes after the end of the FORM**.

| Delta | Observation |
|---|---|
| D13 | The eight trailing bytes make this the entry that matters most. Our decoder ignores anything past the FORM, which was filed as leniency with nothing behind it. It turns out that **a reader demanding end-of-input at the end of the FORM would reject every Gargoyle save.** The lenient reading was right, and not by luck. |
| D34 | A real `IntD`, and exactly the case the drop rule was written for: operating system `UNIX`, interpreter `    ` (four spaces), flags `0x02` — the `s` bit — and a payload holding the **absolute filesystem path of the story file**. This is standard 7.12's "magical OS-dependent reference to the original story file", and 7.10 is why it must not be copied to another machine. `Read` dropped it; `Decode` kept it. |
| D26, D40 | `Bfhs` is Bocfel's own chunk, unregistered anywhere, and it is not small: 2514 bytes holding the game's scrollback as 16-bit characters. Preserved, carried into `Save.Chunks`, and written back out. |
| D29 | A real `ANNO`: `"Interpreter: Bocfel 2.5"`. §12's text-chunk helpers now have something to be useful about. |
| D9, D33 | The dummy frame is present, with six words of evaluation stack. Two independent implementations, two dummy frames. |
| D17 | The difference stream covers 11242 of 11282 bytes — **the same 40-byte trailing omission Frotz made**. Two unrelated implementations dropping the same trailing region is about as strong as this kind of evidence gets. |
| D18 | 85 runs, 30 of them at the 256-byte maximum, in a four-move save. Same picture as Frotz. |

**Cross-implementation agreement.** Frotz and Bocfel, saving the same position,
produced the same PC (`0x7590`), the same number of frames (5), and dynamic
memory differing in **exactly one byte** — at address `0x0001`, which is Flags 1
in the Z-machine header. That byte is interpreter capability, not game state:
Frotz wrote `0x20`, Bocfel `0x60`.

Worth understanding rather than filing away. It means two correct saves of one
position are *expected* to differ in dynamic memory, which is why §18.1 states
the round trip semantically. It also means a caller resuming a restored save
inherits whatever capability bits the writing interpreter advertised, and must
reassert its own — a Z-machine concern, outside this package by §24, but the
kind of thing that produces a confusing bug rather than an error.

**And Gargoyle restored what we wrote.** The rewrite of Bocfel's own save — 2988
bytes against its 3098 — was restored in Gargoyle by hand and resumed in the
Kitchen as expected. Two things follow beyond the criterion itself:

- **Bocfel does not mind losing its own private chunk.** Our rewrite carries no
  `Bfhs`, because `Save.Chunks` held it but the scrollback it describes was
  never ours to reproduce — and Bocfel restored anyway. Standard 7.6's rule that
  interpreters "must not rely on the presence or absence of these chunks" holds
  in practice for the interpreter's own extension, not just for other people's.
- **Dropping the machine-specific `IntD` cost nothing.** The reference to the
  story file went missing, exactly as D34 intends, and the restore did not need
  it. That is the expected outcome — the user had already chosen the story — but
  it is the outcome the conservative default was betting on.

**What this establishes.** Acceptance criteria 7 **and** 8 for a second
implementation: Bocfel reads what we write and we read what Bocfel writes. D13,
D26, D29, D34, and D40 all move from "reasoned about" to "seen". Two of §19's
fixtures that Frotz could not supply — a save with annotations, a save with
unknown chunks — are now obtainable from a real interpreter.

Criteria 7 and 8 are now met against two unrelated implementations, which was
the point of adding the second one. §19's "at least one independent Quetzal
implementation" is satisfied twice over.

**What it does not.**

- **D16 is still completely unexercised.** Two implementations, ten frames
  between them, and not one has the discard bit set. The read/write asymmetry on
  the result variable has now survived two chances to be tested.
- **D2 is still barely probed.** Argument masks `0x00` and `0x01` again, and
  nothing else, from either interpreter.
- **D33 is confirmed twice and falsified never**, which is reassuring and not
  the same as safe: both writers emit the dummy frame, so a writer that omits
  one remains hypothetical.
- Both interpreters compress, so D6 still has no inbound `UMem` fixture. The
  encoding is known to work outbound in both directions of the standard —
  Frotz restored our `UMem` file — but no real writer has produced one for us
  to read.
- **The restore was performed by hand and cannot be repeated by `go test`.**
  Gargoyle is a GUI application. The inbound direction is now a committed
  fixture with tests behind it; the outbound direction stays a manual step,
  recorded here because there is nowhere else to record it.

**Committed.** `testdata/gargoyle/zork1-r119-kitchen.glksave`, remade with the
story at `/private/tmp/zork/zork1.z3` so that the path in its `IntD` carries no
username. `interop_test.go` reads it: every committed save must load, match the
story its name claims, validate, round trip through both encodings, and begin
with the dummy frame. `TestInteropBocfelSpecifics` pins the four findings above
to that file, so that a fixture replaced by one lacking them fails loudly rather
than quietly testing less.

### 2026-07-29 — a third interpreter, an uncompressed save, and a conformance checker

jzip 2.1 (John Holder, 2000), from a local fork that adds a `-u` flag for
uncompressed saves. Three things came out of it, in increasing order of value.

**D6 inbound is closed.** jzip wrote an 11424-byte save whose `UMem` chunk is
exactly the 11282 bytes of Zork I's dynamic memory, and this package read it.
Until now no `UMem` file existed that we had not written ourselves, and a
decoder tested only against its own encoder is not tested.

Worth qualifying rather than overclaiming: the `-u` path is **new code in the
fork, written in 2026**, so this is evidence that two independent readings of
standard 3.6 agree — not evidence from a long-established implementation, which
is what the `CMem` fixtures are. jzip's `CMem` code does date from 2000.

**Three interpreters agree on dynamic memory, to the byte, outside the header.**
Frotz, Bocfel, and jzip each saved the Kitchen four moves in:

| Pair | Bytes differing | Where |
|---|---|---|
| Frotz vs jzip | 10 | `0x1e`–`0x33`, **all inside the 64-byte header** |
| Frotz vs Bocfel | 107 | `0x01`, plus `0x2524`–`0x2880` from a separate play session |
| Bocfel vs jzip | 117 | the union of the two above |

Frotz against jzip is the clean comparison, both saved from a scripted run of
the same moves: **every difference is a header byte** — interpreter number,
screen dimensions, font metrics — and there are none anywhere else. Three
programs and two encodings reconstruct the same memory everywhere the
interpreter is not stamping its own identity on it.

**And `ckifzs` disagrees with Frotz about D32, in our favour.** The same source
tree builds the conformance checker standard 9.2 mentions. Unlike an
interpreter, it judges the file rather than trying to play it, and it says of
every file this package writes:

```
Save file is valid.
```

Run against the four deliberately broken variants:

| File | ckifzs |
|---|---|
| `IFhd` last | `*** warning: IFhd must come before CMem, UMem, or Stks.` — 1 error |
| Duplicate `IFhd` | 2 errors |
| No dummy frame | valid — it does not inspect stack contents |
| `IFhd` longer than 13 bytes | valid |

So the ordering rule has an enforcer after all. D32 was raised to medium on the
grounds that our rejection was stricter than the most widely used interpreter;
the honest reading now is that **Frotz is lenient where the standard's own
checker is not**, and our behaviour is the conforming one. Lowered again.

A footnote on our writer: its output is byte-for-byte identical to jzip's in
*both* encodings, as it was with Frotz. Three interpreters now, all reached
independently. Still a coincidence of compression choices rather than a
contract, and still not tested for.

### 2026-07-29 — a version 6 save, and the dummy frame's other half

Journey, saved in Gargoyle by hand, since `dfrotz` cannot run a V6 game. The
first save this package has seen from a Z-machine version other than 3, and the
only file that can exercise the branch D9 and D33 turn on.

4840 bytes: `IFhd`(13) `IntD`(40) `CMem`(609) `Stks`(76) `ANNO`(23) `Bfhs`(4008),
and eight bytes past the FORM, exactly as Bocfel's V3 save had. Five frames, and
**the first is not a dummy frame** — it is a real call returning to `0x0008cc`:

```
frame 0: dummy=false pc=0x0008cc discard=true  locals=0
frame 1: dummy=false pc=0x0069d6 discard=true  locals=5
frame 2: dummy=false pc=0x00543c discard=false locals=6 args=0b0011
frame 3: dummy=false pc=0x005e43 discard=false locals=5 args=0b0001
frame 4: dummy=false pc=0x005d81 discard=true  locals=2
```

Standard 4.11 asks for the dummy frame "in all versions other than V6", because
V6 execution begins at a routine rather than an address and there is no
top-level to hold evaluation stack for. This file is what that looks like, and
`ValidateFrames` accepts it for version 6 while rejecting the identical stack
for version 3. **Both halves of D9 are now exercised by real files** — one that
must have the frame and one that must not.

**D16 gains its strongest evidence.** Three of these five frames discard their
result, and all three carry `0x00` in the result byte. That is a second
interpreter, on a different Z-machine version, making the same choice standard
4.6 asks for. Between this and the Border Zone save, the discard bit has now
been seen four times and the result byte has been zero every time.

**Where it lives.** `testdata/local/journey-r83-forest.glksave`, not
`testdata/gargoyle/`, because its story cannot be committed — a save is no use
in a clone that has no story to read it against, and `interop_test.go` fails on
a fixture whose story it cannot find. `local_test.go` reads it when present.

### 2026-07-29 — an over-long `IFhd`, written by us and read by Frotz

D19 writes `Header.Extra` back out, so a save read from a file with a
non-conforming `IFhd` is written with the same one. That was rated medium on the
grounds that nobody knew what an interpreter would do with it.

A save was written through `Write` with `Header.Extra` set to eleven bytes,
producing a 24-byte `IFhd` — not assembled by hand, so what Frotz saw is what
`Header.Encode` actually emits. **Frotz restored it**, Kitchen at move 4. The
round trip also keeps the bytes.

Together with the hand-built variant in the strictness table below, that is both
directions: Frotz reads an over-long `IFhd` whether we wrote it or not. D19 and
D12 drop to low.

### 2026-07-29 — versions 5 and 6, fetched but not shippable

D43 said the stories needed to close the remaining gaps did not exist in
redistributable form. That is still true, and it turns out to be the wrong
constraint to have stopped at: a maintainer can *fetch* what the repository
cannot *ship*. `testdata/local/` is gitignored, `testdata/local/fetch.sh`
downloads three stories from `historicalsource`, and `local_test.go` uses
whatever is there and skips when the directory is empty, which is how it is in
a fresh clone.

| Story | Version | Release | Dynamic memory |
|---|---|---|---|
| Border Zone (`spy.z5`) | 5 | 9 | 21562 |
| Beyond Zork beta (`bzbeta.z5`) | 5 | 1 | 34800 |
| Journey (`journey.z6`) | 6 | 83 | 15763 |

**D27's scale factor is confirmed across all three version bands.** The story
length at `$1A` is scaled by 2 for versions 1–3, 4 for 4–5, and 8 for 6 and up,
and until now only the ×2 branch had ever met a real file. All three stories
above recompute their stored checksums exactly — `0x2b37`, `0xe040`, `0xd2b8`.
That is the arithmetic D27 depends on, now checked where it actually varies.

Journey carries 312 bytes of padding past its declared end, which is the case
that motivated summing to the declared length rather than to the end of the
image. Worth being precise: **the padding is all zeros, so this file does not
actually discriminate between the two rules.** It shows the situation is real;
it does not show the choice was necessary.

**D16 is exercised at last.** A Border Zone save — version 5, seven frames —
contains a frame with the discard bit set:

```
frame 1: pc=0x00845e DISCARD=true result=0x00 args=0b0000 locals=1
```

Two things follow. The bit does occur in practice, so the code path is real
rather than theoretical. And **Frotz stores zero in the result byte**, which is
what standard 4.6 asks and what `EncodeStks` writes — so the asymmetry D16
describes is invisible in practice: there is nothing non-zero to preserve, and
the round trip of this save is byte-identical, 510 bytes in and 510 out.

That save also carries **three distinct argument masks** — `0b0000`, `0b0011`,
`0b0111` — against the two that six Zork fixtures managed between them, and
seven frames rather than the invariable five. A version 5 game exercises the
stack decoder considerably harder than a version 3 one, which is the concrete
form of what D2 was complaining about.

**What is still missing.** No version 1 or 2 story could be found in any
fetchable form — the `historicalsource` copies of the early games are all V3
re-releases — so D27's *trigger*, a story with no checksum at `$1C`, remains
untested by any real file. `TestLocalStories` is written to shout if one ever
turns up. And no version 6 *save* exists yet: `dfrotz` cannot run Journey, since
the dumb interface has no graphics and the game beeps rather than prompting.
Gargoyle handles V6, so that one is a manual step nobody has taken.

### 2026-07-29 — asking Frotz about four non-conforming files

The first test of *our strictness* rather than our correctness. Four files were
built by taking the Frotz Kitchen save and breaking it in one way each, then
offering them back to `dfrotz` to see whether it is as strict as we are.

| File | Standard | Us | Frotz 2.55 |
|---|---|---|---|
| Memory and stacks before the `IFhd` | 5.4: `IFhd` "must come before the [CU]Mem and Stks chunks" | **refuse** (D32) | **restored it** |
| No dummy frame, version 3 | 4.11: "a dummy stack frame must be stored as the first in the file" | **refuse** (D33) | **refused it** |
| `IFhd` longer than 13 bytes | 5.5: a future version may enlarge it; the first 13 bytes keep their meaning | accept (D12) | restored it |
| A second `IFhd` naming another story | 8.8: "the later chunks should simply be ignored" | accept, first wins (D40) | **refused it** |

Two agreements and two disagreements, and the disagreements run in *opposite*
directions. In both, this package does what the standard says and Frotz does
not — so neither behavior can claim the reference implementation as support.

**D33 is vindicated, which was the important one.** It was filed at medium risk
because rejecting a file that decodes cleanly is the most dangerous kind of
strictness, and because both writers happen to emit the dummy frame, so nothing
tested it. Frotz refuses the same file — `Fatal error: Error reading save file`.
Its message does not say why, so this is not proof it refuses *for our reason*;
what it establishes is that a save missing the dummy frame is not a file real
interpreters accept. Risk drops to **low**.

**D32 is the opposite, and its risk goes up, not down.** Frotz restored a save
whose `IFhd` came last, without complaint. Our rejection follows 5.4 and §7.1
item 6 — both say the reader must verify the order — but it is now known to be
stricter than the most widely used interpreter. That matters because it changes
what a failure would mean: if some writer emits chunks out of order, its files
work everywhere except here. Raised to **medium**, and it is the first candidate
for the lenient option D30 anticipates.

**D40's leniency is stricter-than-us in Frotz**, which is worth noticing for the
opposite reason. Frotz rejects a duplicate `IFhd` outright — `Save file has two
IFZS chunks!` — where 8.8 says later chunks "should simply be ignored". Our
first-wins behavior is what the standard asks for and is more permissive than
Frotz, so it cannot cause us to reject anything; it only means a file we happily
read might be refused elsewhere. No change to its risk, which was already none.

`TestGoldenNonConformingVariants` builds all four in-test from the committed
Kitchen save and records the Frotz verdict alongside each, so the claims in this
table stay attached to running code.

### 2026-07-29 — five Frotz fixtures, and the limit of this approach

Committed under `testdata/frotz/`, generated by the scripted recipe in that
directory's README and each one verified to restore in `dfrotz` before being
committed:

| Fixture | Story | Dynamic memory | `CMem` | Bytes |
|---|---|---|---|---|
| `zork1-r119-start.qzl` | Zork I | 11282 | 218 | 360 |
| `zork1-r119-kitchen.qzl` | Zork I | 11282 | 291 | 434 |
| `zork1-r119-cellar.qzl` | Zork I | 11282 | 348 | 490 |
| `zork2-r63-start.qzl` | Zork II | 11767 | 187 | 324 |
| `zork3-r25-start.qzl` | Zork III | 12548 | 194 | 330 |

The three stories give three identities, three dynamic-memory sizes, and three
distinct saved program counters — `0x7590`, `0x75aa`, `0x75e0`. Turn zero
against ten moves in the Cellar gives a spread of difference-stream densities to
decode.

**And that is as far as this approach goes.** All five, plus Bocfel's, have
**exactly five frames**, use only argument masks `0x00` and `0x01`, and set the
discard bit zero times:

- Frame count does not vary with play. `SAVE` is reached from the same depth of
  the game's main loop wherever the player happens to be, so a save from turn
  zero and a save ten moves down a trap door describe the same stack shape. More
  Zork fixtures cannot make D1 or D2 better tested.
- The discard bit is not merely absent, it is **impossible** — see D16. The
  opcodes that set it arrived in version 5.

Worth stating plainly because the obvious next move — collect more saves from
more places in more Zork games — would produce files that look different and
test nothing new. The remaining stack deltas need a different *game*, not a
different position, and the games that would do it cannot be committed (D43).

### The two Bocfel saves compared

The two Bocfel saves of the same position — the first probe and this one —
differed in 106 bytes of dynamic memory, all within `0x2524`–`0x2880`, with the
first mostly zero there. Reaching the same room twice does not produce the same
memory: parser and game state outside the player's position differ between
sessions. Another reason §18.1 is stated semantically, and a reason not to write
a golden test that compares a save byte for byte against a stored copy.
