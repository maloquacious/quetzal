# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Status

Milestones 1–3 of §27 are done: the IFF container (`read.go`), `IFhd` and story identity (`header.go`, `story.go`), and dynamic memory (`memory.go`). Milestone 4 (`Stks`) is next, then the writer, which is why no `Save`, `Read`, or `Write` exists yet. Work stays on `main` until the GitHub upstream repo exists. Module path is `github.com/maloquacious/quetzal`, Go 1.26.4.

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

`references/savefile_14.txt` is the normative Quetzal 1.4 standard by Martin Frost. It is **gitignored on purpose** (no redistribution license) but is present in the working tree — consult it for wire-format details rather than guessing. Do not commit it, and do not commit story files whose redistribution isn't clearly permitted.

## Architecture constraints that are easy to violate

These come from the spec and shape nearly every file:

- **Standard library only.** No third-party runtime dependencies. Implement the Quetzal subset of IFF in-package rather than pulling in a general IFF library; do not create a public `iff` package yet (§3.1, §4).
- **One public package**, flat file layout (`read.go`, `write.go`, `chunk.go`, `header.go`, `memory.go`, `stack.go`, `story.go`, `errors.go`). Internal files may differ (§4).
- **`io.Reader`/`io.Writer` only** in the core API. No filesystem paths, no filesystem or network access as a side effect of parsing — ever, including no searching for a matching story file (§3.2, §3.3, §25).
- **Story data is always passed in explicitly.** `CMem` cannot be decoded without the caller's original dynamic memory. `Decode(r) (*File, error)` inspects the container without a story; `Read(r, *Story) (*Save, error)` reconstructs memory and therefore verifies `IFhd` identity first (§7, §8).
- **Untrusted binary input.** Every length field is attacker-controlled: bounds-check reads, use overflow-safe arithmetic, validate before allocating, honor `Limits`, never panic (§16, §25).
- **Preserve, don't discard.** Unknown chunks keep their exact ID and payload and their relative order; unknown ≠ error. IFF padding bytes are structural and never appear in `Chunk.Data` (§3.4, §5.4, §7.3).
- **Strict by default.** Malformed input errors out; any leniency must be an explicit opt-in option (§3.5).
- **No mutation.** Parsing must not touch the caller's story buffer; writing must not mutate `Save` or `Story`. Prefer copying over zero-copy parsing — these files are small (§17).

## Format details worth memorizing

- All multi-byte integers are big-endian. PCs are 3 bytes on the wire, `uint32` in Go; writers reject `> 0xFFFFFF`.
- Odd-length chunks get one zero padding byte, excluded from the chunk length and from the FORM length arithmetic.
- `CMem` is an XOR difference against original dynamic memory with zero-run encoding: a zero byte is followed by a length byte `n` meaning `n+1` zeros. A truncated difference stream means "remaining bytes are zero differences"; overrunning dynamic memory is an error.
- Frame flags byte: low four bits are the local count (reject `> 15`); the `p` bit means the result is discarded, in which case writers emit a zero result variable.
- `Evaluation[0]` is the least-recent word, `Evaluation[len-1]` is the top of stack — file order. Document this on every relevant exported type.
- The V<6 dummy first frame is preserved as-is at this layer, never silently dropped.
- Duplicate single-instance chunks: first wins, later ones ignored. Multiple `ANNO` chunks are legal.

## Errors

Wrap the four sentinels (`ErrInvalidFormat`, `ErrStoryMismatch`, `ErrTruncated`, `ErrLimitExceeded`) so `errors.Is` works; use typed errors like `ChunkError` (with `Unwrap`) to carry chunk ID and offset. Error text should name the offending chunk or frame.

## Non-goals

No Z-machine execution, no object/dictionary/text systems, no Blorb, no undo history, no interpretation of `IntD` payloads, no session or HTTP concerns. If a task seems to need one of these, it belongs in a caller (§24).
