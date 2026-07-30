# CLAUDE.md

Guidance for coding agents working in this repository.

## Project state

- Module: `github.com/maloquacious/quetzal`; Go 1.26.4; standard library only.
- `specification.md` is accepted for v1.0. All milestones and acceptance criteria are complete.
- Statement coverage is 100%; preserve it. Dead branches often indicate an impossible check rather than a missing test.
- `Version()` deliberately reports library version `0.2.2` and Quetzal version `1.4`. Do not bump it incidentally.
- Adding or removing a `.go` file bumps the patch level at least, test files included: they ship in the module zip, so a consumer receives them.

## Commands

```sh
go build ./...
go vet ./...
go test ./...                # must not require external interpreters
go test -cover ./...         # expect 100.0%
go test -run TestName ./...
go test -run XXX -fuzz FuzzDecode -fuzztime 30s ./...
go doc -all .
```

Other fuzz targets are `FuzzCMem`, `FuzzStacks`, and `FuzzWriteRoundTrip`. Run one target per invocation. Keep any test that invokes an external interpreter behind a build tag or in a separate script.

README Go snippets are compiled examples in `example_test.go`; update both together.

`tutorial.md` is held to a stricter standard than the README, because a reader
still learning cannot tell a stale lesson from their own mistake.
`tutorial_test.go` runs the tutorial's program against committed fixtures and
asserts every number it prints, and checks that the markdown's code is the code
that ran. Changing behavior the tutorial shows means updating all three.

## Sources of truth

- **Exported API:** declarations and doc comments shown by `go doc -all .`.
- **Behavior:** `specification.md`. Read the relevant section before changing semantics.
- **Quetzal 1.4 wire format:** `references/savefile_14.txt`. It is gitignored because it has no redistribution license; never commit it.
- **Deliberate divergences and limitations:** `spec-deltas.md`. Check it before treating surprising behavior as a bug, especially D44's intentional API asymmetries.
- **Testing and debugging facilities:** doc comments plus a GitHub issue. `specification.md` §5.7 puts them outside that document, so do not add spec sections or `spec-deltas.md` entries for them. Comparison (`Compare`, `CompareFiles`, `compare.go`) is the first, and its issue is #1. Such a facility must observe values only: it may not change reading or writing semantics, and one that seems to need a divergence is doing something other than observing.

Do not add deviations from `specification.md` to section 4 of `spec-deltas.md`; that section is closed. Resolve a disagreement by correcting the implementation or specification under §31. A new divergence from Quetzal requires both a `spec-deltas.md` entry and a row in `specification.md` §2.1.

## Architecture and invariants

- Keep one flat public package. Do not add a public IFF package or third-party runtime dependencies.
- Core APIs use `io.Reader`/`io.Writer`. Parsing must not access filesystems, networks, or search for story files.
- Layering is `Decode` → `File` → `File.Save` → `Save` and `Save.Encode` → `File` → `File.WriteTo`; `Read` and `Write` compose those layers. `Decode` inspects a container without a story; `Read` reconstructs and validates a save against an explicitly supplied `Story`.
- Keep `Story` a required value, not a pointer. Parsed stories are shared concurrently, so parsing and writing must not mutate or retain caller-owned buffers. Prefer copying over zero-copy parsing.
- Treat all binary input as hostile: bounds-check reads, use overflow-safe arithmetic, validate before allocation, honor `Limits`, and never panic.
- Preserve unknown chunks exactly, including order; exclude structural IFF padding from `Chunk.Data`. `Limits.MaxUnknownBytes` bounds their combined payload. Add every newly understood chunk ID to `ID.known()`.
- Preserve `ANNO`, `AUTH`, and `(c) ` bytes exactly; do not normalize text.
- Parsing is strict by default. Leniency must be an explicit, narrowly named option following `IgnoreChunkOrder`, not a general lenient mode. The complete option set is currently `WithLimits`, `IgnoreChunkOrder`, and `WithEncoding`.

## Required format behavior

- Multi-byte integers are big-endian. Odd chunk payloads have one zero padding byte, excluded from the chunk length.
- `CMem` is an XOR diff against original dynamic memory with zero-run encoding. A truncated stream means the remaining differences are zero; a stream that overruns dynamic memory is invalid.
- Mask undefined frame and argument flag bits on read; do not reject them (D1, D2). Preserve the V<6 dummy first frame.
- `Evaluation` is in file order: index 0 is least recent and the last element is the stack top.
- For duplicate single-instance chunks, the first wins. `Decode` retains all chunks for diagnostics, while `Save` excludes ignored duplicates. Multiple `ANNO` chunks are valid.
- `Read` enforces `IFhd` before `CMem`, `UMem`, or `Stks`; `Decode` intentionally does not.
- `Read` omits machine- or position-specific `IntD` chunks from `Save`.
- `ParseStory` computes a checksum only when header word `$1C` is zero. Never replace a stored checksum with a recomputed one.

## Errors

Wrap `ErrInvalidFormat`, `ErrStoryMismatch`, `ErrTruncated`, and `ErrLimitExceeded` so `errors.Is` works. Preserve typed detail through `*ChunkError`, `*FrameError`, and `*StoryMismatchError`; error text must identify the relevant chunk or frame.

## Fixtures and scope

- Committed fixtures are limited to redistributable V3 stories. Versions 1 and 2 are untested, not unsupported.
- `testdata/local/fetch.sh` can fetch V5/V6 fixtures locally. Never commit anything under `testdata/local/` except its README and fetch script.
- Interpreter fixtures use `testdata/<interpreter>/<game>-r<release>-<where>.<ext>`. Follow the README in each interpreter directory for manual interoperability checks.
- Do not commit story files without a clear redistribution license.

Out of scope: Z-machine execution, object/dictionary/text systems, Blorb, undo history, interpreting `IntD` payloads, and session or HTTP concerns. These belong in callers.
