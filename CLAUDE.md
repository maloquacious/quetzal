# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Status

All seven milestones of §27 are done, and `specification.md` is accepted. Milestones 1–6 delivered the IFF container (`read.go`), `IFhd` and story identity (`header.go`, `story.go`), dynamic memory (`memory.go`), stack frames (`stack.go`), the writer plus the whole-save layer (`write.go`, `Save`/`Read`/`Write`/`Validate`), and interoperability — acceptance criteria 7 and 8 are met against Frotz 2.55, Bocfel 2.5, and jzip 2.1, and `interop_test.go` reads the committed fixtures. §7 of `spec-deltas.md` records every contact with another implementation and — more usefully — what each one did not establish. Work stays on `main` until the GitHub upstream repo exists. Module path is `github.com/maloquacious/quetzal`, Go 1.26.4.

`version.go` still reports `0.1.0` from `Version()`, which also returns the spec version `1.4`. That is deliberate and not an oversight to correct in passing: the specification is *accepted for* v1.0, and bumping the constant is the release, not the acceptance. `version_test.go` pins both strings, so a bump is a visible edit.

Milestone 7 (hardening) is done, and with it all fifteen acceptance criteria of §28: four fuzz targets (`FuzzDecode`, `FuzzCMem`, `FuzzStacks`, `FuzzWriteRoundTrip`), §20's malformed-input list, `Limits` enforcement including `MaxUnknownBytes` (D26), §12's text helpers (D29), §7.2's duplicate diagnostics — which needed documentation rather than code (D28) — and the API and documentation review (D44). With that done, `specification.md` was accepted: the deltas were triaged one by one, the ones that were really the spec being early were absorbed into it, the departures from Quetzal 1.4 were enumerated in a new §2.1, and the three that stay open became stated limitations in §30 rather than pending work. Coverage is at 100% of statements and is worth keeping there: a dead branch here usually means a check that cannot fire, which is a design question, not a testing one. D44 records the four API asymmetries that look like oversights and are deliberate; read it before "fixing" one. Every Go snippet in `README.md` lives in `example_test.go` as an example function, so the compiler checks it; `TestREADMESnippetsAreCompiled` fails if you edit one without the other.

Interop fixtures live in a directory per interpreter (`testdata/frotz`, `testdata/gargoyle`, `testdata/jzip`), named `<game>-r<release>-<where>.<ext>` with the interpreter's own extension. `interop_test.go` finds the story from the name, so a misnamed fixture fails there. Frotz (`dfrotz`) and jzip are scriptable; Gargoyle is a GUI, so its saves and restores are manual — the recipes are in each directory's README. jzip is the only interpreter here that writes `UMem` (`-u`), and its source tree also builds `ckifzs`, the conformance checker from standard 9.2 — run it against anything the writer produces (`ckifzs f >/dev/null || ...`; it exits 1 on errors), since it judges our output without being our output. Its "valid" covers container soundness, chunk presence, and ordering only: it passes a 0-byte `IFhd`, a ragged `Stks`, and a dangling `CMem` zero, all of which we reject. A Gargoyle save's `IntD` embeds the story's absolute path, so make saves with the story somewhere neutral.

Layering, in both directions: `Decode` → `File` → `File.Save` → `Save`, and `Save.Encode` → `File` → `File.WriteTo`. `Read` and `Write` are the two compositions. `Decode`/`File` need no story and judge nothing; `Read`/`Save` require the story and validate.

Version 3 is the committed scope, by decision (D43): only the MIT-released Zorks may be shipped — `historicalsource` was checked, and only its `zork1`/`zork2`/`zork3` repos have a LICENSE. Versions 5 and 6 *can* be fetched into the gitignored `testdata/local/` with `testdata/local/fetch.sh`; `local_test.go` uses them when present and skips otherwise, so a fresh clone and CI still see only V3. Versions 1 and 2 remain unexercised in any form — do not describe them as unsupported, and do not describe them as tested. Never commit anything under `testdata/local/` but its README and fetch script.

`ParseStory` computes the checksum when `$1C` holds zero (standard 5.5, D27) and records that in `Story.ChecksumComputed`. A stored checksum is never recomputed, even if it disagrees with the image: interpreters compare the stored value, so ours must too.

The intended caller is a server that caches parsed `Story` values and shares one across concurrent requests. Passing `Story` by value copies the struct but not the bytes behind `DynamicMemory`, so the no-mutation and no-retention guarantees are what make that safe; `TestReadDoesNotAliasTheStory`, `TestEncodeDoesNotAliasTheStory`, and `TestStorySurvivesConcurrentUse` are load-bearing, not decoration.

## Commands

```sh
go build ./...
go vet ./...                 # acceptance criterion #14 — must stay clean
go test ./...                # must pass without any external interpreter installed
go test -cover ./...         # expect 100.0% of statements
go test -run TestName ./...
go test -run XXX -fuzz FuzzDecode -fuzztime 30s ./...   # or FuzzCMem, FuzzStacks, FuzzWriteRoundTrip
go doc -all .                # §5.5: this, not the spec, defines the exported API
```

`-run XXX` matches no test, so fuzzing starts without running the suite first. One target per invocation — `-fuzz` takes a single pattern.

Interoperability tests that shell out to an external interpreter (e.g. Frotz) must sit behind a build tag or a separate script so plain `go test ./...` never needs them. Nothing in the suite shells out today: `interop_test.go` reads committed fixtures, and the outbound half — files we wrote, restored elsewhere — is a manual recipe recorded in `spec-deltas.md` section 7 and in each `testdata/<interpreter>/README`. §30 lists that as an accepted limitation rather than something to automate.

## The specification is the contract

`specification.md` is **Accepted for v1.0**, not Draft, and that changes how to treat a disagreement with it. Read the relevant section before implementing or changing anything. It uses RFC 2119 keywords deliberately: MUST items are acceptance criteria (§28), SHOULD items are defaults that can be argued with. Section map: §2.1 the closed list of departures from Quetzal 1.4, §5 public data model, §5.5 why `go doc` and not this document defines the exported API, §7 reading, §9 dynamic memory, §10 stack frames, §11 writing, §14 validation, §15 errors, §16 limits, §24 non-goals, §27 milestones, §30 accepted limitations, §31 how to amend it.

**`go doc -all` is authoritative for the exported surface**; the specification governs semantics. So a Go declaration in the spec that disagrees with the code means the spec is out of date, not that there is a delta — and changing the API needs no spec amendment, while changing behavior does. The corollary is that the doc comments *are* the API specification, which is why §22 demands so much of them.

`spec-deltas.md` records every deliberate divergence from the standard, the accepted limitations, the fixture inventory, and the interoperability evidence log, each entry with a stable `D<n>` identifier, an interoperability risk estimate, and a `*Fate:*` line saying what was done with it (absorbed / divergence / limitation / resolved). Read it before concluding that some behavior is a bug.

Two rules about adding to it, both enforced by `deltas_test.go`. **Its section 4 — deviations from `specification.md` — is closed**: a behavior that differs from the spec is now a defect in one of the two documents, so fix one of them rather than filing an entry (§31). And a new divergence from the standard needs a row in §2.1 as well as an entry here, because §2.1 claims to be the complete list.

`references/savefile_14.txt` is the normative Quetzal 1.4 standard by Martin Frost. It is **gitignored on purpose** (no redistribution license) but is present in the working tree — consult it for wire-format details rather than guessing. Do not commit it, and do not commit story files whose redistribution isn't clearly permitted.

## Architecture constraints that are easy to violate

These come from the spec and shape nearly every file:

- **Standard library only.** No third-party runtime dependencies. Implement the Quetzal subset of IFF in-package rather than pulling in a general IFF library; do not create a public `iff` package yet (§3.1, §4).
- **One public package**, flat file layout. §4 sketches `read.go`, `write.go`, `chunk.go`, `header.go`, `memory.go`, `stack.go`, `story.go`, `errors.go`; the package also has `doc.go` (the package documentation, which §5.5 makes normative for the API), `limits.go`, and `version.go`. Internal files may differ (§4).
- **`io.Reader`/`io.Writer` only** in the core API. No filesystem paths, no filesystem or network access as a side effect of parsing — ever, including no searching for a matching story file (§3.2, §3.3, §25).
- **Story data is always passed in explicitly.** `CMem` cannot be decoded without the caller's original dynamic memory. `Decode(r io.Reader, opts ...ReadOption) (*File, error)` inspects the container without a story; `Read(r io.Reader, story Story, opts ...ReadOption) (*Save, error)` reconstructs memory and therefore verifies `IFhd` identity first (§7, §8). The story is a value and is not optional — §7 and D39 say why a `*Story` would be worse, so do not "fix" it into a pointer.
- **Untrusted binary input.** Every length field is attacker-controlled: bounds-check reads, use overflow-safe arithmetic, validate before allocating, honor `Limits`, never panic (§16, §25).
- **Preserve, don't discard.** Unknown chunks keep their exact ID and payload and their relative order; unknown ≠ error. IFF padding bytes are structural and never appear in `Chunk.Data` (§3.4, §5.4, §7.3). Their combined payload is the one thing in a file that nothing but the file bounds, so `Limits.MaxUnknownBytes` bounds it; `ID.known()` is the set that is exempt, and adding a chunk ID this package understands means adding it there too. The same principle governs `ANNO`/`AUTH`/`(c) ` text: it is returned exactly as stored, control bytes and all, because the standard's printable-ASCII rule (7.2) is not ours to enforce by rewriting someone's data.
- **Strict by default.** Malformed input errors out; any leniency must be an explicit opt-in option (§3.5). There is exactly one: `IgnoreChunkOrder()`, because Frotz accepts mis-ordered chunks and we otherwise would not (D30, D32). Follow its shape if another is ever needed — name the rule being overlooked rather than adding a general "be lenient" switch. The complete option set is three: `WithLimits` and `IgnoreChunkOrder` as `ReadOption`s, `WithEncoding` as the only `WriteOption`. Both option types are functions over unexported config structs, so callers cannot add their own; D44 explains why that wart is kept.
- **No mutation.** Parsing must not touch the caller's story buffer; writing must not mutate `Save` or `Story`. Prefer copying over zero-copy parsing — these files are small (§17).

## Format details worth memorizing

- All multi-byte integers are big-endian. PCs are 3 bytes on the wire, `uint32` in Go; writers reject anything above the exported `MaxPC` (`0xffffff`). The other exported ceilings are `MaxLocals` (15), `MaxEvaluationWords` (`0xffff`), and `MinVersion`/`MaxVersion` (1 and 8). All are limits the *format* imposes, so a decoded value cannot exceed one — each is read from a field too small — and every check on them is therefore a write-side check on a value a caller built.
- Odd-length chunks get one zero padding byte, excluded from the chunk length and from the FORM length arithmetic.
- `CMem` is an XOR difference against original dynamic memory with zero-run encoding: a zero byte is followed by a length byte `n` meaning `n+1` zeros. A truncated difference stream means "remaining bytes are zero differences"; overrunning dynamic memory is an error.
- Frame flags byte: low four bits are the local count; the `p` bit means the result is discarded, in which case writers emit a zero result variable. The top three bits and the arguments byte's eighth bit are undefined and are **masked away on read, never rejected** (D1, D2) — so a frame header can no longer be structurally invalid, and a desynced `Stks` stream is caught only by running out of payload.
- `Evaluation[0]` is the least-recent word, `Evaluation[len-1]` is the top of stack — file order. Document this on every relevant exported type.
- The V<6 dummy first frame is preserved as-is at this layer, never silently dropped.
- Duplicate single-instance chunks: first wins, later ones ignored. Multiple `ANNO` chunks are legal. An ignored duplicate never reaches `Save.Chunks`, or the writer would emit two of it (§5.6) — but `Decode` keeps every chunk, and `File.All` returning two `IFhd`s is how a caller sees the duplicate. That retention *is* §7.2's diagnostic mechanism; there is no separate diagnostics facility and D28 says why none should be added.
- `IFhd` must come before `CMem`/`UMem`/`Stks` (5.4). `Read` enforces this; `Decode` deliberately does not.
- `IntD` has a 12-byte fixed header and a flags byte `000000sc`: `c` means the contents belong to this saved position only, `s` means they belong to this machine only. Neither kind may be copied into another file, so `Read` leaves them out of the `Save` (7.10, 7.11, §13).

## Errors

Wrap the four sentinels (`ErrInvalidFormat`, `ErrStoryMismatch`, `ErrTruncated`, `ErrLimitExceeded`) so `errors.Is` works. Three typed errors carry the detail, each with `Unwrap`: `*ChunkError` (chunk ID and byte offset), `*FrameError` (frame index, not an offset — the frame number is what a caller can act on, D24), and `*StoryMismatchError` (both identities, so a message can say how they differ). Error text should name the offending chunk or frame.

## Non-goals

No Z-machine execution, no object/dictionary/text systems, no Blorb, no undo history, no interpretation of `IntD` payloads, no session or HTTP concerns. If a task seems to need one of these, it belongs in a caller (§24).
