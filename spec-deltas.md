# Specification Deltas

Every place this package's behavior differs from `specification.md` or from
Martin Frost's Quetzal 1.4 standard, and why.

## Why this file exists

Milestone 6 is interoperability: exchanging saves with an established
interpreter. When that milestone finds a disagreement, the first question is
always *did we choose this, or is it a bug?* This file answers that question
without a code archaeology expedition.

Entries have stable identifiers (D1, D2, ...) so commits, issues, and test
names can point at them. **Interop risk** estimates the chance that the entry
causes a disagreement with a real interpreter:

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

*Interop risk:* **none**.

---

## 5. Not implemented yet

Gaps, not decisions. Listed because an interoperability failure may trace to
one of them rather than to a delta above.

### D25 — §7.1 item 6 is unenforced: nothing checks that `IFhd` precedes `CMem`/`UMem`/`Stks`

Standard 5.4 requires this ordering so that an interpreter does not decode
memory only to discover the wrong story. We read the chunks by identifier, so
order does not affect *our* results, and nothing currently rejects a file that
gets it wrong. The writer (Milestone 5) must still emit them in order, which
§11 requires.

*Interop risk:* **medium** in the outbound direction — an external interpreter
may well enforce what we do not. Verify our own output ordering as part of
Milestone 5, not 6.

### D26 — `Limits.MaxUnknownBytes` is declared but never enforced

Nothing sums the payloads of chunks the package does not interpret.
`MaxChunkBytes` bounds each chunk individually and `MaxFormBytes` bounds the
whole file, so the exposure is limited, but the field currently does nothing.

*Interop risk:* **none**. Security-relevant, and a §16 item.

### D27 — No pre-checksum story support

Standard 5.5: if a story has no checksum, the saving interpreter "should
calculate it in the normal way from the original story file". `ParseStory`
reads `$1C` literally, so for a story with a zero checksum our `Story.Identity`
carries zero while a conforming interpreter's save carries a computed value —
and `Header.Verify` reports a mismatch that is our fault, not the file's.

This affects V1 and V2 games and any V3 image with a zero at `$1C`.
`Header.Matches` documents the situation; nothing fixes it.

The fix is to compute the checksum when `$1C` is zero. The algorithm is the sum
of every byte from `$40` to the file length, modulo `0x10000`, where the length
is the word at `$1A` times 2 for V1–3, 4 for V4–5, and 8 for V6–8. Verified
against all three `testdata` images, whose stored checksums match exactly.

*Interop risk:* **high** for pre-checksum games, none otherwise. The most
likely genuine interoperability failure on this list.

### D28 — No diagnostic mechanism for ignored duplicate chunks

§7.2 asks that later instances of a single-instance chunk "be ignored with a
diagnostic mechanism if diagnostics are enabled". They are ignored silently;
there is no diagnostics facility.

*Interop risk:* **none**.

### D29 — No `IntD` parsing, no text-chunk helpers

§13's `InterpreterData` and §12's `Annotations`/`Author` helpers do not exist.
Both chunk kinds survive as raw chunks, so nothing is lost. §13's requirement
that rewriting respect `IntD` copy restrictions has no implementation to
violate yet, and must be honored when the writer gains the ability to copy
chunks forward.

*Interop risk:* **low** now, **medium** once the writer copies unknown chunks —
standard 7.22-style `IntD` payloads are machine-specific and must not be copied
blindly.

### D30 — No lenient mode

§3.5 permits leniency only as an explicit opt-in. Nothing in section 1 above
can currently be relaxed without a code change. If interoperability testing
finds a writer that trips D1 or D2, the choice is between relaxing the rule
outright and introducing the option §3.5 anticipates.

*Interop risk:* n/a — this is the escape hatch for everything else.

### D31 — No `Save`, `Read`, or `Write`

Milestone 5. The pieces exist: `File.Header`, `File.Memory`, `File.Frames`,
`Header.Encode`, `Memory.Encode`, `EncodeStks`.

---

## 6. Fixture checklist for Milestone 6

§19's fixture list, annotated with the deltas each one exercises. Section 7
records what has actually been checked against another implementation so far.

| Fixture | Exercises |
|---|---|
| Valid compressed save | D4, D5, D27 |
| Valid uncompressed save | D6 |
| Save with annotations | D29 |
| Save with unknown chunks | D26, D29 |
| Odd-length chunks and padding | D11 |
| Multiple stack frames | D1, D2, D16 |
| Long zero runs | D18 |
| Trailing omitted `CMem` differences | D17 |
| A V1 or V2 game | D27 |
| A V6 game | D9 (dummy frame absent) |
| Chunks in a non-standard order | D25 |

The last three are additions to §19's list, one per open gap that only a real
file can settle.

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
