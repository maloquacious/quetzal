# Changelog

Notable changes to this package, newest first. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this package
follows [semantic versioning](https://semver.org/spec/v2.0.0.html).

Two conventions worth stating, because they are what a reader of this file
usually wants to know:

- Versions below 1.0 do not promise a stable API. In practice the exported API
  has not changed incompatibly and none is planned; `specification.md` is
  accepted rather than draft.
- The Quetzal version implemented is 1.4 throughout, and is reported separately
  by `Version()`. It is not this package's version and does not move with it.

## [Unreleased]

### Added

- A README section, **Stored files**, and `specification.md` §26.1 behind it,
  saying what becomes of a file after it is written. The format on disk is
  Quetzal 1.4's rather than this package's, and nothing the writer emits records
  a package version, so a file written by any release stays readable by every
  later one — before v1.0 as well as after, since the pre-1.0 latitude is over
  the Go API and no stored file depends on it. §26.1 also names the only two
  grounds that would break that, requires either to be announced here, and
  states the two things a caller storing these bytes should not assume: that a
  rewrite reproduces them, and that a save can be read without its story.
  ([#9](https://github.com/maloquacious/quetzal/issues/9))

Documentation only. No API or behavior changed, so `Version()` does not move.

## [0.2.3] — 2026-07-30

A build requirement relaxed. No API or behavior changed.

### Changed

- The `go` directive in `go.mod` is now `go 1.26` rather than `go 1.26.4`. Go
  requires a module's directive to be at least that of everything it depends
  on, so the patch level was inherited by every consumer: a module depending on
  this one could not declare `go 1.26`, and builds on Go 1.26.0 through 1.26.3
  were refused. Nothing here needs a language or standard-library feature added
  after 1.26.0, and this module has no dependencies of its own to force the
  level back up. ([#8](https://github.com/maloquacious/quetzal/issues/8))

## [0.2.2] — 2026-07-30

Documentation, and the test that holds it. No API or behavior changed; the
version moves because a `.go` file was added, and test files ship in the module
zip like any other.

### Added

- `tutorial.md`, a lesson for readers new to Quetzal or to this package. It
  reads a real Frotz save of Zork I against its story, finds the program counter
  and call stack inside it, writes it back out, and rewrites it in the other
  memory encoding to show what the semantic round trip does and does not
  promise. It runs on fixtures already committed here, so it needs no story file
  the reader has to go and find.
- `tutorial_test.go`, which runs the tutorial's program against those fixtures
  and asserts every number it prints, and checks that the code in the markdown
  is the code that ran.
- A documentation map in the README, and a pkg.go.dev badge and link.
- This file.

## [0.2.1] — 2026-07-30

### Fixed

- `IgnoreInterpreterHeader`'s doc comment listed the header ranges it disregards
  without saying that they are the union across Z-machine Versions rather than
  the set that applies to the Version of the save being compared. Standard 11.1
  introduces `$1E`–`$21` at Version 4, `$22`–`$27` and `$2C`–`$2D` at Version 5,
  and `$30`–`$31` at Version 6, so on a Version 3 save ten of the disregarded
  bytes are ordinary dynamic memory that the option hides anyway. Nothing
  observable is hidden in practice — neither the game nor the interpreter writes
  those bytes below the Version that introduces them — and the comment now
  records why the set is not narrowed per save. ([#4](https://github.com/maloquacious/quetzal/issues/4))

No behavior changed in this release.

## [0.2.0] — 2026-07-30

### Added

- `Compare` and `CompareFiles`, which report how two saves or two containers
  differ. `Compare` works on reconstructed state, needs no story of its own,
  returns no error, and reports nothing when the two agree. `CompareFiles`
  answers the narrower question of whether two files *say* it the same way,
  walking chunks in order without interpreting them.
- `CompareOption`: `IgnoreInterpreterHeader`, `IgnoreMemoryEncoding`,
  `IgnoreMemoryRange`, and `IgnoreChunks`. Each is named for what it disregards,
  and none can turn agreement into a difference, so a comparison with no options
  is exact.

Comparison is a testing and debugging facility rather than part of the format,
so `specification.md` deliberately says nothing about it; see its §5.7, and
[issue 1](https://github.com/maloquacious/quetzal/issues/1) for the design.
([#2](https://github.com/maloquacious/quetzal/pull/2))

## 0.1.0 — never tagged

`Version()` reported `0.1.0` through this work, but no `v0.1.0` tag was ever
pushed, so there is nothing for a Go module or a release page to point at. The
first tag in this repository is `v0.2.0`, applied retroactively. This section is
here so the history reads continuously rather than beginning in the middle.

The initial implementation, feature complete for v1.0 of `specification.md`.

### Added

- The IFF container: `FORM`/`IFZS` parsing, chunk parsing and padding, retention
  of unknown chunks in file order, and configurable resource limits through
  `Limits` and `WithLimits`.
- Story identification: `ParseStory`, the `IFhd` chunk, and story matching by
  release, serial, and checksum — including computing a checksum for stories
  written before the header carried one.
- Dynamic memory: the `CMem` and `UMem` chunks, compression and decompression of
  the `CMem` XOR difference stream, and reconstruction against the story.
- Stack frames: the `Stks` chunk, frame decoding and encoding, the dummy first
  frame that Versions other than 6 require, and frame validation.
- Whole saves: `Decode`, `Read`, `Write`, `Validate`, and the copy restrictions
  on interpreter-dependent `IntD` data.
- The text chunks a save may carry: `Annotations`, `Author`, and `Copyright`,
  preserved byte for byte.
- `IgnoreChunkOrder` and `WithEncoding`.

### Interoperability

Saves written by Frotz 2.55, Bocfel 2.5, and jzip 2.1 all load here, and all
three restore files this package writes. `ckifzs`, the conformance checker
distributed with the standard, reports every file this package writes as
conforming.

[Unreleased]: https://github.com/maloquacious/quetzal/compare/v0.2.3...HEAD
[0.2.3]: https://github.com/maloquacious/quetzal/compare/v0.2.2...v0.2.3
[0.2.2]: https://github.com/maloquacious/quetzal/compare/v0.2.1...v0.2.2
[0.2.1]: https://github.com/maloquacious/quetzal/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/maloquacious/quetzal/releases/tag/v0.2.0
