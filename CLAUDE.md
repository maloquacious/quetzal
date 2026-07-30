# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Status

Milestones 1–5 of §27 are done: the IFF container (`read.go`), `IFhd` and story identity (`header.go`, `story.go`), dynamic memory (`memory.go`), stack frames (`stack.go`), and the writer plus the whole-save layer (`write.go`, `Save`/`Read`/`Write`/`Validate`). Milestone 6 is underway: acceptance criteria 7 and 8 are met against Frotz 2.55 and Bocfel 2.5, and `interop_test.go` reads the committed fixtures. §7 of `spec-deltas.md` records every contact with another implementation and — more usefully — what each one did not establish. Work stays on `main` until the GitHub upstream repo exists. Module path is `github.com/maloquacious/quetzal`, Go 1.26.4.

Interop fixtures live in a directory per interpreter (`testdata/frotz`, `testdata/gargoyle`), named `<game>-r<release>-<where>.<ext>` with the interpreter's own extension. `interop_test.go` finds the story from the name, so a misnamed fixture fails there. Frotz is scriptable (`dfrotz`); Gargoyle is a GUI, so its saves and restores are manual — the recipes are in each directory's README. A Gargoyle save's `IntD` embeds the story's absolute path, so make saves with the story somewhere neutral.

Layering, in both directions: `Decode` → `File` → `File.Save` → `Save`, and `Save.Encode` → `File` → `File.WriteTo`. `Read` and `Write` are the two compositions. `Decode`/`File` need no story and judge nothing; `Read`/`Save` require the story and validate.

Version 3 is the committed scope, by decision (D43): only the MIT-released Zorks may be shipped — `historicalsource` was checked, and only its `zork1`/`zork2`/`zork3` repos have a LICENSE. Versions 5 and 6 *can* be fetched into the gitignored `testdata/local/` with `testdata/local/fetch.sh`; `local_test.go` uses them when present and skips otherwise, so a fresh clone and CI still see only V3. Versions 1 and 2 remain unexercised in any form — do not describe them as unsupported, and do not describe them as tested. Never commit anything under `testdata/local/` but its README and fetch script.

`ParseStory` computes the checksum when `$1C` holds zero (standard 5.5, D27) and records that in `Story.ChecksumComputed`. A stored checksum is never recomputed, even if it disagrees with the image: interpreters compare the stored value, so ours must too.

The intended caller is a server that caches parsed `Story` values and shares one across concurrent requests. Passing `Story` by value copies the struct but not the bytes behind `DynamicMemory`, so the no-mutation and no-retention guarantees are what make that safe; `TestReadDoesNotAliasTheStory`, `TestEncodeDoesNotAliasTheStory`, and `TestStorySurvivesConcurrentUse` are load-bearing, not decoration.

## Commands

```sh
go build ./...
go vet ./...          # acceptance criterion #14 — must stay clean
go test ./...         # must pass without any external interpreter installed
go test -run TestName ./...
go test -fuzz FuzzDecode -fuzztime 30s ./...
go doc ./...
```

Interoperability tests that shell out to an external interpreter (e.g. Frotz) must sit behind a build tag or a separate script so plain `go test ./...` never needs them.

## The specification is the contract

`specification.md` is the authoritative design document for this package — read the relevant section before implementing or changing anything. It uses RFC 2119 keywords deliberately: MUST items are acceptance criteria (§28), SHOULD items are defaults that can be argued with. Section map: §5 public data model, §7 reading, §9 dynamic memory, §10 stack frames, §11 writing, §14 validation, §15 errors, §16 limits, §24 non-goals, §27 milestones.

`spec-deltas.md` records every deliberate divergence from the spec and the standard, plus the known gaps, each with a stable `D<n>` identifier and an interoperability risk estimate. Read it before concluding that some behavior is a bug, and add an entry when you make a new judgment call.

`references/savefile_14.txt` is the normative Quetzal 1.4 standard by Martin Frost. It is **gitignored on purpose** (no redistribution license) but is present in the working tree — consult it for wire-format details rather than guessing. Do not commit it, and do not commit story files whose redistribution isn't clearly permitted.

## Architecture constraints that are easy to violate

These come from the spec and shape nearly every file:

- **Standard library only.** No third-party runtime dependencies. Implement the Quetzal subset of IFF in-package rather than pulling in a general IFF library; do not create a public `iff` package yet (§3.1, §4).
- **One public package**, flat file layout (`read.go`, `write.go`, `chunk.go`, `header.go`, `memory.go`, `stack.go`, `story.go`, `errors.go`). Internal files may differ (§4).
- **`io.Reader`/`io.Writer` only** in the core API. No filesystem paths, no filesystem or network access as a side effect of parsing — ever, including no searching for a matching story file (§3.2, §3.3, §25).
- **Story data is always passed in explicitly.** `CMem` cannot be decoded without the caller's original dynamic memory. `Decode(r) (*File, error)` inspects the container without a story; `Read(r, *Story) (*Save, error)` reconstructs memory and therefore verifies `IFhd` identity first (§7, §8).
- **Untrusted binary input.** Every length field is attacker-controlled: bounds-check reads, use overflow-safe arithmetic, validate before allocating, honor `Limits`, never panic (§16, §25).
- **Preserve, don't discard.** Unknown chunks keep their exact ID and payload and their relative order; unknown ≠ error. IFF padding bytes are structural and never appear in `Chunk.Data` (§3.4, §5.4, §7.3).
- **Strict by default.** Malformed input errors out; any leniency must be an explicit opt-in option (§3.5). There is exactly one: `IgnoreChunkOrder()`, because Frotz accepts mis-ordered chunks and we otherwise would not (D30, D32). Follow its shape if another is ever needed — name the rule being overlooked rather than adding a general "be lenient" switch.
- **No mutation.** Parsing must not touch the caller's story buffer; writing must not mutate `Save` or `Story`. Prefer copying over zero-copy parsing — these files are small (§17).

## Format details worth memorizing

- All multi-byte integers are big-endian. PCs are 3 bytes on the wire, `uint32` in Go; writers reject `> 0xFFFFFF`.
- Odd-length chunks get one zero padding byte, excluded from the chunk length and from the FORM length arithmetic.
- `CMem` is an XOR difference against original dynamic memory with zero-run encoding: a zero byte is followed by a length byte `n` meaning `n+1` zeros. A truncated difference stream means "remaining bytes are zero differences"; overrunning dynamic memory is an error.
- Frame flags byte: low four bits are the local count; the `p` bit means the result is discarded, in which case writers emit a zero result variable. The top three bits and the arguments byte's eighth bit are undefined and are **masked away on read, never rejected** (D1, D2) — so a frame header can no longer be structurally invalid, and a desynced `Stks` stream is caught only by running out of payload.
- `Evaluation[0]` is the least-recent word, `Evaluation[len-1]` is the top of stack — file order. Document this on every relevant exported type.
- The V<6 dummy first frame is preserved as-is at this layer, never silently dropped.
- Duplicate single-instance chunks: first wins, later ones ignored. Multiple `ANNO` chunks are legal. An ignored duplicate never reaches `Save.Chunks`, or the writer would emit two of it.
- `IFhd` must come before `CMem`/`UMem`/`Stks` (5.4). `Read` enforces this; `Decode` deliberately does not.
- `IntD` has a 12-byte fixed header and a flags byte `000000sc`: `c` means the contents belong to this saved position only, `s` means they belong to this machine only. Neither kind may be copied into another file, so `Read` leaves them out of the `Save` (7.10, 7.11, §13).

## Errors

Wrap the four sentinels (`ErrInvalidFormat`, `ErrStoryMismatch`, `ErrTruncated`, `ErrLimitExceeded`) so `errors.Is` works; use typed errors like `ChunkError` (with `Unwrap`) to carry chunk ID and offset. Error text should name the offending chunk or frame.

## Non-goals

No Z-machine execution, no object/dictionary/text systems, no Blorb, no undo history, no interpretation of `IntD` payloads, no session or HTTP concerns. If a task seems to need one of these, it belongs in a caller (§24).
